package dispatch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	foundrytui "github.com/CoolGitOrg/foundry-tui"
	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/azure/azuretest"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

func dispatcher(t *testing.T) Dispatcher {
	t.Helper()
	tfs, err := TemplateFS(foundrytui.Templates, "")
	if err != nil {
		t.Fatal(err)
	}
	log, err := audit.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{R: azuretest.Fake(), Cfg: config.Default(), FS: tfs, Log: log, Actor: "crafty"}
}

func create() map[string]string {
	return map[string]string{
		"action": "create", "deployment": "gpt-4o-eu", "model_format": "OpenAI", "model_name": "gpt-4o",
		"model_version": "2024-11-20", "sku_name": "GlobalStandard", "sku_capacity": "50",
		"reason": "capacity for the platform team",
	}
}

func TestRenderDeploy(t *testing.T) {
	req, err := dispatcher(t).Render("deploy", create())
	if err != nil {
		t.Fatal(err)
	}
	var in map[string]string
	if err := json.Unmarshal(req.Payload, &in); err != nil {
		t.Fatal(err)
	}
	if in["dry_run"] != "true" || !req.DryRun() {
		t.Error("dry_run must default to true")
	}
	if in["request_id"] == "" || in["request_id"] != req.ID {
		t.Error("request_id missing from payload")
	}
	if len(in) != MaxInputs {
		t.Errorf("deploy renders %d inputs, want %d", len(in), MaxInputs)
	}
	got := req.Cmd().String()
	want := "gh workflow run foundry-deploy.yml --repo CoolGitOrg/foundry-ops --ref main --json"
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

func TestRenderRejects(t *testing.T) {
	d := dispatcher(t)
	for name, mutate := range map[string]func(map[string]string){
		"shell in name":      func(p map[string]string) { p["deployment"] = "x; rm -rf /" },
		"newline in reason":  func(p map[string]string) { p["reason"] = "ok reason\n\"injected\": \"1\"" },
		"short reason":       func(p map[string]string) { p["reason"] = "because" },
		"unknown parameter":  func(p map[string]string) { p["sku"] = "Standard" },
		"missing model":      func(p map[string]string) { delete(p, "model_name") },
		"zero capacity":      func(p map[string]string) { p["sku_capacity"] = "0" },
		"bad action":         func(p map[string]string) { p["action"] = "destroy" },
		"dry_run not a bool": func(p map[string]string) { p["dry_run"] = "yes" },
	} {
		p := create()
		mutate(p)
		if _, err := d.Render("deploy", p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A quote in a value must stay inside its JSON string.
func TestQuotesCannotEscape(t *testing.T) {
	p := create()
	p["reason"] = `move to "prod", then {"dry_run":"false"}`
	req, err := dispatcher(t).Render("deploy", p)
	if err != nil {
		t.Fatal(err)
	}
	var in map[string]string
	json.Unmarshal(req.Payload, &in)
	if in["reason"] != p["reason"] || in["dry_run"] != "true" {
		t.Errorf("payload was altered by its own data: %v", in)
	}
}

func TestDeleteNeedsNoModel(t *testing.T) {
	_, err := dispatcher(t).Render("deploy", map[string]string{"action": "delete", "deployment": "old-one", "reason": "retired by vendor"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendWritesBothSidesOfTheTrail(t *testing.T) {
	d := dispatcher(t)
	req, _ := d.Render("deploy", create())
	out, err := d.Send(context.Background(), req)
	if err != nil || !strings.Contains(out, "/actions/runs/42") {
		t.Fatalf("Send = %q, %v", out, err)
	}
	calls := d.R.(interface{ Calls() []run.Cmd }).Calls()
	if string(calls[0].Stdin) != string(req.Payload) {
		t.Error("gh did not receive the previewed payload on stdin")
	}
	recs, _ := d.Log.Read()
	if len(recs) != 2 || recs[0].Kind != "dispatch.request" || recs[1].Kind != "dispatch.sent" {
		t.Fatalf("audit trail = %+v", recs)
	}
	if recs[0].Detail["payload_sha256"] != req.SHA256 {
		t.Error("audit record does not pin the payload")
	}
}
