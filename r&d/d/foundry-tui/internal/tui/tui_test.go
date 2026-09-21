package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	foundrytui "github.com/CoolGitOrg/foundry-tui"
	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/azure/azuretest"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/dispatch"
	"github.com/CoolGitOrg/foundry-tui/internal/perms"
	"github.com/CoolGitOrg/foundry-tui/internal/probe"
)

// screenText reads the simulated terminal from inside the event loop,
// so the read never races a draw.
func screenText(a *App, s tcell.SimulationScreen) string {
	var sb strings.Builder
	a.app.QueueUpdate(func() { readCells(s, &sb) })
	return sb.String()
}

func readCells(s tcell.SimulationScreen, sb *strings.Builder) {
	cells, w, _ := s.GetContents()
	for i, c := range cells {
		if i%w == 0 {
			sb.WriteByte('\n')
		}
		if len(c.Runes) > 0 {
			sb.WriteRune(c.Runes[0])
		}
	}
}

func waitFor(t *testing.T, a *App, s tcell.SimulationScreen, want string) string {
	t.Helper()
	var text string
	for i := 0; i < 100; i++ {
		if text = screenText(a, s); strings.Contains(text, want) {
			return text
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("screen never showed %q:\n%s", want, text)
	return ""
}

// Drive the real application on a simulated terminal: load, change
// views, open the delete form, preview, send, and read the trail.
func TestScreen(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.RefreshSeconds = 0
	cfg.ReportDir = t.TempDir()
	fake := azuretest.Fake()
	tfs, _ := dispatch.TemplateFS(foundrytui.Templates, "")
	log, _ := audit.Open(t.TempDir())
	app := New(context.Background(), Deps{
		Cfg: cfg, Azure: azure.Client{R: fake, Cfg: cfg}, Perms: perms.Snapshot(), Log: log, TFS: tfs, Runner: fake,
		Disp:    dispatch.Dispatcher{R: fake, Cfg: cfg, FS: tfs, Log: log, Actor: "crafty"},
		History: probe.NewHistory(t.TempDir()), Actor: "crafty", Version: "test",
	}, screen)
	screen.SetSize(200, 60) // after New: tview's SetScreen calls Init, which resets the size

	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	key := func(r rune) { screen.InjectKey(tcell.KeyRune, r, tcell.ModNone) }
	special := func(k tcell.Key) { screen.InjectKey(k, 0, tcell.ModNone) }

	text := waitFor(t, app, screen, "claude-sonnet")
	for _, want := range []string{"gpt-4o", "GlobalStandard", "Failed", "uami-foundry-deploy"} {
		if !strings.Contains(text, want) {
			t.Errorf("deployments view lacks %q", want)
		}
	}

	key('2')
	waitFor(t, app, screen, "mistral-large-2411")
	key('3')
	waitFor(t, app, screen, "86%")
	key('7')
	text = waitFor(t, app, screen, "Microsoft.Insights/metrics/read")
	if !strings.Contains(text, "foundry-429-rate") {
		t.Error("status view lacks the fired alert")
	}

	// Delete the first deployment, as a dry run.
	key('1')
	waitFor(t, app, screen, "claude-sonnet")
	key('d')
	waitFor(t, app, screen, "delete deployment")
	special(tcell.KeyTab) // Deployment → Reason
	for _, r := range "superseded by a newer model" {
		key(r)
	}
	special(tcell.KeyTab) // → Dry run
	special(tcell.KeyTab) // → Preview
	special(tcell.KeyEnter)
	text = waitFor(t, app, screen, "DRY RUN")
	if !strings.Contains(text, "gh workflow run foundry-deploy.yml") {
		t.Errorf("preview does not show the command:\n%s", text)
	}
	special(tcell.KeyEnter) // Send
	waitFor(t, app, screen, "dispatched ftui-")

	var sent bool
	for _, line := range azuretest.Lines(fake) {
		sent = sent || strings.HasPrefix(line, "gh workflow run foundry-deploy.yml")
	}
	if !sent {
		t.Error("gh was never run")
	}
	recs, _ := log.Read()
	if len(recs) != 2 || recs[1].Kind != "dispatch.sent" || recs[0].Subject != "claude-sonnet" {
		t.Errorf("audit trail = %+v", recs)
	}

	key('q')
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("q did not quit")
	}
}
