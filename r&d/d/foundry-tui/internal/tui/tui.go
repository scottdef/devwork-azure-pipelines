// Package tui is the screen. It owns no logic worth testing twice:
// it loads with packages azure and github, writes with package
// dispatch, and draws tables. If a feature cannot be reached from the
// command line as well, it does not belong here.
package tui

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/dispatch"
	"github.com/CoolGitOrg/foundry-tui/internal/github"
	"github.com/CoolGitOrg/foundry-tui/internal/perms"
	"github.com/CoolGitOrg/foundry-tui/internal/probe"
	"github.com/CoolGitOrg/foundry-tui/internal/report"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

// Deps is everything the screen is given. It creates nothing itself.
type Deps struct {
	Cfg     config.Config
	Azure   azure.Client
	GitHub  *github.Client // nil: GitHub views stay empty
	Perms   perms.Set
	Disp    dispatch.Dispatcher
	Log     *audit.Log
	History probe.History
	TFS     fs.FS
	Runner  run.Runner
	Actor   string
	Version string
	Theme   string // "term" keeps the terminal's colours; "acme" paints Plan 9's
}

// data is the last thing each loader returned.
type data struct {
	account     azure.Account
	deployments []azure.Deployment
	models      []azure.Model
	usage       []azure.Usage
	access      []azure.RoleAssignment
	health      azure.Health
	alerts      []azure.Alert
	stats       []github.Stats
	runs        []github.Run
	records     []audit.Record
	auditErr    error
	errs        map[string]error
}

// App is the running screen.
type App struct {
	Deps
	app    *tview.Application
	root   *tview.Pages // "main" plus whatever dialog is open
	tabs   *tview.Pages
	bar    *tview.TextView
	status *tview.TextView

	deployments, catalog, quota, runs, agents, trail *tview.Table
	overview                                         *tview.TextView

	mu     sync.Mutex
	d      data
	filter string
	tab    int
	ctx    context.Context
}

var tabNames = []string{"Deployments", "Catalog", "Quota", "Runs", "Agents", "Audit", "Status"}

const keys = "[::b]1-7[::-] view  [::b]r[::-] refresh  [::b]n[::-] new  [::b]s[::-] scale  [::b]d[::-] delete  " +
	"[::b]t[::-] probe API  [::b]c[::-] probe Copilot  [::b]T[::-] test workflow  [::b]R[::-] write report  " +
	"[::b]W[::-] report workflow  [::b]/[::-] filter  [::b]?[::-] help  [::b]q[::-] quit"

// New builds the screen. screen may be nil; tests pass a simulation.
func New(ctx context.Context, deps Deps, screen tcell.Screen) *App {
	theme(deps.Theme)
	a := &App{Deps: deps, app: tview.NewApplication(), ctx: ctx}
	a.d.errs = map[string]error{}
	if screen != nil {
		a.app.SetScreen(screen)
	}
	table := func() *tview.Table {
		t := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
		t.SetBorderPadding(0, 0, 1, 1)
		return t
	}
	a.deployments, a.catalog, a.quota = table(), table(), table()
	a.runs, a.agents, a.trail = table(), table(), table()
	a.overview = tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	a.overview.SetBorderPadding(0, 0, 1, 1)

	a.tabs = tview.NewPages()
	for i, p := range []tview.Primitive{a.deployments, a.catalog, a.quota, a.runs, a.agents, a.trail, a.overview} {
		a.tabs.AddPage(tabNames[i], p, true, i == 0)
	}
	a.bar = tview.NewTextView().SetDynamicColors(true)
	a.status = tview.NewTextView().SetDynamicColors(true)
	help := tview.NewTextView().SetDynamicColors(true).SetText(keys)

	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.bar, 1, 0, false).
		AddItem(a.tabs, 0, 1, true).
		AddItem(a.status, 1, 0, false).
		AddItem(help, 2, 0, false)
	a.root = tview.NewPages().AddPage("main", main, true, true)
	a.app.SetRoot(a.root, true).SetInputCapture(a.onKey)

	a.deployments.SetSelectedFunc(func(row, _ int) { a.showDeployment(row) })
	a.catalog.SetSelectedFunc(func(row, _ int) { a.newFromCatalog(row) })
	a.runs.SetSelectedFunc(func(row, _ int) { a.showRun(row) })
	a.trail.SetSelectedFunc(func(row, _ int) { a.showRecord(row) })
	a.show(0)
	return a
}

// Run draws until the user quits.
func (a *App) Run() error {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.ctx = ctx
	go a.refresh()
	if n := a.Cfg.RefreshSeconds; n > 0 {
		go func() {
			t := time.NewTicker(time.Duration(n) * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					a.refresh()
				}
			}
		}()
	}
	return a.app.Run()
}

// Stop ends Run. Tests use it.
func (a *App) Stop() { a.app.Stop() }

func theme(name string) {
	if name == "acme" {
		tview.Styles = tview.Theme{
			PrimitiveBackgroundColor:    tcell.NewHexColor(0xffffea),
			ContrastBackgroundColor:     tcell.NewHexColor(0xeaffff),
			MoreContrastBackgroundColor: tcell.NewHexColor(0x99994c),
			BorderColor:                 tcell.NewHexColor(0x8888cc),
			TitleColor:                  tcell.ColorBlack,
			GraphicsColor:               tcell.NewHexColor(0x8888cc),
			PrimaryTextColor:            tcell.ColorBlack,
			SecondaryTextColor:          tcell.NewHexColor(0x55554a),
			TertiaryTextColor:           tcell.NewHexColor(0x2d6a2d),
			InverseTextColor:            tcell.ColorBlack,
			ContrastSecondaryTextColor:  tcell.NewHexColor(0x55554a),
		}
		return
	}
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = tcell.ColorDefault
}

func (a *App) show(i int) {
	a.tab = i
	a.tabs.SwitchToPage(tabNames[i])
	var sb strings.Builder
	fmt.Fprintf(&sb, " [::b]%s[::-] in %s  ", a.Cfg.Account, a.Cfg.ResourceGroup)
	for j, n := range tabNames {
		if j == i {
			fmt.Fprintf(&sb, "[::r] %d %s [::-]", j+1, n)
		} else {
			fmt.Fprintf(&sb, " %d %s ", j+1, n)
		}
	}
	a.bar.SetText(sb.String())
}

func (a *App) say(format string, args ...any) {
	a.status.SetText(" " + tview.Escape(fmt.Sprintf(format, args...)))
}

// sayLater is say for goroutines.
func (a *App) sayLater(format string, args ...any) {
	a.app.QueueUpdateDraw(func() { a.say(format, args...) })
}

func (a *App) dialogOpen() bool {
	name, _ := a.root.GetFrontPage()
	return name != "main"
}

func (a *App) onKey(ev *tcell.EventKey) *tcell.EventKey {
	if a.dialogOpen() {
		if ev.Key() == tcell.KeyEscape {
			a.closeDialog()
			return nil
		}
		return ev
	}
	if ev.Key() == tcell.KeyCtrlC {
		return ev
	}
	if ev.Key() != tcell.KeyRune {
		return ev
	}
	switch r := ev.Rune(); r {
	case '1', '2', '3', '4', '5', '6', '7':
		a.show(int(r - '1'))
	case 'q':
		a.app.Stop()
	case 'r':
		go a.refresh()
	case '?':
		a.popup("Help", helpText)
	case 'R':
		go a.writeReport()
	case 'W':
		a.simpleForm("report", "Run the report workflow", nil)
	case '/':
		a.askFilter()
	case 'n':
		if a.tab == 1 {
			row, _ := a.catalog.GetSelection()
			a.newFromCatalog(row)
		} else {
			a.deployForm("create", nil, nil)
		}
	case 's', 'd', 't', 'c', 'T':
		if d, ok := a.selected(); ok {
			switch r {
			case 's':
				a.deployForm("update", &d, nil)
			case 'd':
				a.deployForm("delete", &d, nil)
			case 't':
				go a.probe(d, "api")
			case 'c':
				go a.probe(d, "copilot")
			case 'T':
				a.simpleForm("test", "Run the model test workflow", map[string]string{"deployment": d.Name, "mode": "both"})
			}
		} else {
			a.say("select a deployment first (press 1)")
		}
	default:
		return ev
	}
	return nil
}

func (a *App) selected() (azure.Deployment, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	row, _ := a.deployments.GetSelection()
	if a.tab != 0 || row < 1 || row > len(a.d.deployments) {
		return azure.Deployment{}, false
	}
	return a.d.deployments[row-1], true
}

// refresh reloads everything, each source on its own goroutine.
func (a *App) refresh() {
	a.sayLater("loading…")
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	load := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := fn()
			a.mu.Lock()
			a.d.errs[name] = err
			a.mu.Unlock()
		}()
	}
	set := func(fn func(*data)) { a.mu.Lock(); fn(&a.d); a.mu.Unlock() }

	acct, err := a.Azure.Account(ctx)
	set(func(d *data) { d.account, d.errs["account"] = acct, err })
	load("deployments", func() error {
		v, err := a.Azure.Deployments(ctx)
		set(func(d *data) { d.deployments = v })
		return err
	})
	load("models", func() error {
		v, err := a.Azure.Models(ctx)
		set(func(d *data) { d.models = v })
		return err
	})
	load("usage", func() error {
		v, err := a.Azure.Usage(ctx)
		set(func(d *data) { d.usage = v })
		return err
	})
	if acct.ID != "" {
		load("access", func() error {
			v, err := a.Azure.RoleAssignments(ctx, acct.ID)
			set(func(d *data) { d.access = v })
			return err
		})
		load("health", func() error {
			v, err := a.Azure.Health(ctx, acct.ID)
			set(func(d *data) { d.health = v })
			return err
		})
		load("alerts", func() error {
			v, err := a.Azure.Alerts(ctx, acct.ID)
			set(func(d *data) { d.alerts = v })
			return err
		})
	}
	if a.GitHub != nil {
		load("github", func() error {
			wfs, err := a.GitHub.Workflows(ctx)
			if err != nil {
				return err
			}
			runs, err := a.GitHub.Runs(ctx, "", 100)
			set(func(d *data) { d.stats, d.runs = github.Summarize(wfs, runs), runs })
			return err
		})
	}
	load("audit", func() error {
		recs, err := a.Log.Read()
		var verr error
		if err == nil {
			_, verr = audit.Verify(recs)
		}
		set(func(d *data) { d.records, d.auditErr = recs, verr })
		return err
	})
	wg.Wait()
	a.app.QueueUpdateDraw(a.draw)
}

func (a *App) draw() {
	a.mu.Lock()
	defer a.mu.Unlock()
	d := &a.d

	rows := [][]string{}
	for _, x := range d.deployments {
		m := x.Properties.Model
		rows = append(rows, []string{x.Name, m.Format, m.Name, m.Version, x.SKU.Name, strconv.Itoa(x.SKU.Capacity),
			x.Properties.ProvisioningState, x.SystemData.LastModifiedBy, short(x.SystemData.LastModifiedAt)})
	}
	fill(a.deployments, []string{"Deployment", "Format", "Model", "Version", "SKU", "Cap", "State", "Changed by", "Changed"}, rows, 6)

	rows = rows[:0]
	for _, m := range d.models {
		line := strings.ToLower(m.Format + " " + m.Name + " " + m.Version)
		if a.filter != "" && !strings.Contains(line, strings.ToLower(a.filter)) {
			continue
		}
		rows = append(rows, []string{m.Format, m.Name, m.Version, m.LifecycleStatus, m.Deprecation.Inference, strings.Join(m.SKUNames(), ",")})
	}
	fill(a.catalog, []string{"Format", "Model", "Version", "Lifecycle", "Retires", "SKUs"}, rows, -1)

	rows = rows[:0]
	for _, u := range d.usage {
		rows = append(rows, []string{u.Name.Value, fmt.Sprintf("%.0f", u.CurrentValue), fmt.Sprintf("%.0f", u.Limit),
			meter(u.Percent()), fmt.Sprintf("%.0f%%", u.Percent())})
	}
	fill(a.quota, []string{"Quota", "Used", "Limit", "", "Share"}, rows, -1)

	rows = rows[:0]
	for _, r := range d.runs {
		rows = append(rows, []string{r.Name, strconv.Itoa(r.Number), r.DisplayTitle, r.Event, r.Actor.Login, r.State(), r.Duration().String(), short(r.CreatedAt)})
	}
	fill(a.runs, []string{"Workflow", "#", "Title", "Event", "By", "Result", "Took", "Started"}, rows, 5)

	rows = rows[:0]
	for _, s := range d.stats {
		if !s.Agentic {
			continue
		}
		rate, last := "", "none"
		if s.Succeeded+s.Failed > 0 {
			rate = fmt.Sprintf("%.0f%%", s.SuccessRate)
		}
		if s.Last.ID != 0 {
			last = s.Last.State()
		}
		rows = append(rows, []string{s.Workflow, s.State, strconv.Itoa(s.Runs), strconv.Itoa(s.Succeeded), strconv.Itoa(s.Failed),
			strconv.Itoa(s.Active), rate, s.AvgDuration.String(), last, short(s.Last.CreatedAt)})
	}
	fill(a.agents, []string{"Agentic workflow", "State", "Runs", "Pass", "Fail", "Live", "Success", "Mean", "Last", "When"}, rows, 8)

	rows = rows[:0]
	for i := len(d.records) - 1; i >= 0; i-- {
		r := d.records[i]
		rows = append(rows, []string{strconv.Itoa(r.Seq), short(r.Time), r.Actor, r.Kind, r.Subject, r.Detail["request_id"], r.Hash[:min(12, len(r.Hash))]})
	}
	fill(a.trail, []string{"Seq", "When", "Who", "What", "Subject", "Request", "Hash"}, rows, -1)

	a.overview.SetText(a.overviewText())

	var bad []string
	for k, err := range d.errs {
		if err != nil {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		a.say("loaded %s with errors in: %s (press 7)", time.Now().Format("15:04:05"), strings.Join(bad, ", "))
	} else {
		a.say("loaded %s as %s, bounded by %s (%s)", time.Now().Format("15:04:05"), a.Actor, a.Perms.Role, a.Perms.Source)
	}
}

func (a *App) overviewText() string {
	d := &a.d
	var sb strings.Builder
	p := d.account.Properties
	fmt.Fprintf(&sb, "[::b]Account[::-]\n  %s  kind %s  sku %s  state %s\n  endpoint %s\n  public network %s, key auth disabled %v\n\n",
		d.account.Name, d.account.Kind, d.account.SKU.Name, p.ProvisioningState, p.Endpoint, p.PublicNetworkAccess, p.DisableLocalAuth)
	fmt.Fprintf(&sb, "[::b]Resource Health[::-]\n  %s %s\n\n", d.health.Properties.AvailabilityState, d.health.Properties.Summary)
	fmt.Fprintf(&sb, "[::b]Alerts, last 30 days[::-]\n")
	if len(d.alerts) == 0 {
		sb.WriteString("  none\n")
	}
	for _, al := range d.alerts {
		e := al.Properties.Essentials
		fmt.Fprintf(&sb, "  %s %s %s since %s\n", e.Severity, tview.Escape(e.AlertRule), e.MonitorCondition, e.StartDateTime)
	}
	fmt.Fprintf(&sb, "\n[::b]Audit chain[::-]\n  %d records, ", len(d.records))
	if d.auditErr != nil {
		fmt.Fprintf(&sb, "[red]BROKEN: %s[-]\n", tview.Escape(d.auditErr.Error()))
	} else {
		sb.WriteString("verified\n")
	}
	fmt.Fprintf(&sb, "\n[::b]What %s may do[::-] (%s)\n", a.Perms.Role, a.Perms.Source)
	for _, c := range a.Perms.Matrix() {
		mark := "[green]yes[-]"
		if !c.Allowed {
			mark = "[yellow]no [-]"
		}
		fmt.Fprintf(&sb, "  %s  %-48s %s\n", mark, c.What, c.Action)
	}
	fmt.Fprintf(&sb, "\n[::b]Access[::-]\n")
	for _, r := range d.access {
		who := r.PrincipalName
		if who == "" {
			who = r.PrincipalID
		}
		where := "account"
		if r.Inherited(d.account.ID) {
			where = "inherited"
		}
		fmt.Fprintf(&sb, "  %-32s %-18s %-10s %s\n", r.Role, r.PrincipalType, where, tview.Escape(who))
	}
	fmt.Fprintf(&sb, "\n[::b]Load errors[::-]\n")
	n := 0
	for k, err := range d.errs {
		if err != nil {
			n++
			fmt.Fprintf(&sb, "  [red]%s[-]: %s\n", k, tview.Escape(err.Error()))
		}
	}
	if n == 0 {
		sb.WriteString("  none\n")
	}
	return sb.String()
}

// fill replaces a table's contents. state is the column to colour by
// outcome, or -1.
func fill(t *tview.Table, head []string, rows [][]string, state int) {
	row, _ := t.GetSelection()
	t.Clear()
	for c, h := range head {
		t.SetCell(0, c, tview.NewTableCell(h).SetSelectable(false).SetAttributes(tcell.AttrBold|tcell.AttrUnderline))
	}
	for r, cells := range rows {
		for c, v := range cells {
			cell := tview.NewTableCell(tview.Escape(v)).SetMaxWidth(44)
			if c == state {
				switch v {
				case "Succeeded", "success":
					cell.SetTextColor(tcell.ColorGreen)
				case "failure", "Failed", "timed_out", "startup_failure":
					cell.SetTextColor(tcell.ColorRed)
				case "in_progress", "queued", "Creating", "Updating", "Deleting", "Accepted":
					cell.SetTextColor(tcell.ColorOlive)
				}
			}
			t.SetCell(r+1, c, cell)
		}
	}
	if len(rows) == 0 {
		t.SetCell(1, 0, tview.NewTableCell("nothing to show").SetSelectable(false))
		return
	}
	t.Select(max(1, min(row, len(rows))), 0)
}

func meter(pct float64) string {
	n := int(pct / 5)
	n = max(0, min(n, 20))
	return strings.Repeat("█", n) + strings.Repeat("░", 20-n)
}

func short(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("01-02 15:04")
}

// Dialogs.

func (a *App) closeDialog() {
	name, _ := a.root.GetFrontPage()
	if name != "main" {
		a.root.RemovePage(name)
	}
}

func center(p tview.Primitive, w, h int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).AddItem(p, h, 0, true).AddItem(nil, 0, 1, false), w, 0, true).
		AddItem(nil, 0, 1, false)
}

func (a *App) popup(title, text string) {
	tv := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetText(text)
	tv.SetBorder(true).SetTitle(" " + title + " (Esc closes) ")
	tv.SetDoneFunc(func(tcell.Key) { a.closeDialog() })
	a.root.AddPage("popup", center(tv, 96, 28), true, true)
}

func (a *App) popupLater(title, text string) {
	a.app.QueueUpdateDraw(func() { a.popup(title, text) })
}

func (a *App) showDeployment(row int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row < 1 || row > len(a.d.deployments) {
		return
	}
	d := a.d.deployments[row-1]
	p, m, sd := d.Properties, d.Properties.Model, d.SystemData
	text := fmt.Sprintf("[::b]%s[::-]\n\nmodel       %s / %s @ %s\nsku         %s, capacity %d\nstate       %s\nupgrade     %s\ncontent     %s\nrate        %.0f requests/min, %.0f tokens/min\n\ncreated     %s by %s\nchanged     %s by %s\n\n%s",
		d.Name, m.Format, m.Name, m.Version, d.SKU.Name, d.SKU.Capacity, p.ProvisioningState, p.VersionUpgradeOption, p.RAIPolicyName,
		d.RateLimit("request"), d.RateLimit("token"), short(sd.CreatedAt), tview.Escape(sd.CreatedBy), short(sd.LastModifiedAt), tview.Escape(sd.LastModifiedBy), tview.Escape(d.ID))
	a.popup("Deployment", text)
}

func (a *App) showRun(row int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row < 1 || row > len(a.d.runs) {
		return
	}
	r := a.d.runs[row-1]
	a.popup("Run", fmt.Sprintf("[::b]%s #%d[::-] attempt %d\n\ntitle    %s\nfile     %s\nevent    %s on %s\nby       %s\nresult   %s in %s\nstarted  %s\n\n%s\n\nAgentic runs: gh aw audit %d",
		tview.Escape(r.Name), r.Number, r.Attempt, tview.Escape(r.DisplayTitle), r.Path, r.Event, r.Branch, r.Actor.Login, r.State(), r.Duration(), short(r.CreatedAt), r.URL, r.ID))
}

func (a *App) showRecord(row int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := len(a.d.records) - row
	if row < 1 || i < 0 {
		return
	}
	r := a.d.records[i]
	var sb strings.Builder
	fmt.Fprintf(&sb, "[::b]#%d %s[::-]\n\nwhen     %s\nwho      %s\nsubject  %s\nprev     %s\nhash     %s\n\n", r.Seq, r.Kind, r.Time.Format(time.RFC3339), r.Actor, r.Subject, r.Prev, r.Hash)
	for k, v := range r.Detail {
		fmt.Fprintf(&sb, "[::b]%s[::-]\n%s\n\n", k, tview.Escape(v))
	}
	a.popup("Audit record", sb.String())
}

func (a *App) askFilter() {
	in := tview.NewInputField().SetLabel("Filter catalog: ").SetText(a.filter).SetFieldWidth(40)
	in.SetBorder(true)
	in.SetDoneFunc(func(tcell.Key) {
		a.filter = in.GetText()
		a.closeDialog()
		a.show(1)
		a.draw()
	})
	a.root.AddPage("filter", center(in, 64, 3), true, true)
}

func (a *App) newFromCatalog(row int) {
	if row < 1 {
		return
	}
	format, name, version := a.catalog.GetCell(row, 0).Text, a.catalog.GetCell(row, 1).Text, a.catalog.GetCell(row, 2).Text
	a.mu.Lock()
	var pick *azure.Model
	for i, m := range a.d.models {
		if m.Format == format && m.Name == name && m.Version == version {
			pick = &a.d.models[i]
		}
	}
	a.mu.Unlock()
	if pick != nil {
		a.deployForm("create", nil, pick)
	}
}

// deployForm collects parameters for create, update or delete. It
// renders nothing itself: Preview hands the map to package dispatch.
func (a *App) deployForm(action string, d *azure.Deployment, m *azure.Model) {
	p := map[string]string{"action": action, "sku_name": "GlobalStandard", "sku_capacity": "10", "dry_run": "true"}
	if d != nil {
		dm := d.Properties.Model
		p["deployment"], p["model_format"], p["model_name"], p["model_version"] = d.Name, dm.Format, dm.Name, dm.Version
		p["sku_name"], p["sku_capacity"] = d.SKU.Name, strconv.Itoa(d.SKU.Capacity)
	}
	if m != nil {
		p["deployment"], p["model_format"], p["model_name"], p["model_version"] = m.Name, m.Format, m.Name, m.Version
		if len(m.SKUs) > 0 {
			p["sku_name"] = m.SKUs[0].Name
			if c := m.SKUs[0].Capacity.Default; c > 0 {
				p["sku_capacity"] = strconv.Itoa(c)
			}
		}
	}
	f := tview.NewForm()
	field := func(label, key string, width int) {
		f.AddInputField(label, p[key], width, nil, func(v string) { p[key] = v })
	}
	field("Deployment", "deployment", 40)
	if action != "delete" {
		field("Model format", "model_format", 30)
		field("Model name", "model_name", 40)
		field("Model version", "model_version", 30)
		field("SKU", "sku_name", 30)
		field("Capacity", "sku_capacity", 10)
	} else {
		for _, k := range []string{"model_format", "model_name", "model_version", "sku_name", "sku_capacity"} {
			delete(p, k)
		}
	}
	field("Reason", "reason", 60)
	f.AddCheckbox("Dry run", true, func(on bool) { p["dry_run"] = strconv.FormatBool(on) })
	f.AddButton("Preview", func() { a.preview("deploy", p) })
	f.AddButton("Cancel", a.closeDialog)
	f.SetBorder(true).SetTitle(fmt.Sprintf(" %s deployment: nothing is sent until you confirm ", action))
	a.root.AddPage("form", center(f, 84, 23), true, true)
}

// simpleForm asks only for a reason.
func (a *App) simpleForm(kind, title string, p map[string]string) {
	if p == nil {
		p = map[string]string{}
	}
	f := tview.NewForm()
	f.AddInputField("Reason", "", 60, nil, func(v string) { p["reason"] = v })
	f.AddButton("Preview", func() { a.preview(kind, p) })
	f.AddButton("Cancel", a.closeDialog)
	f.SetBorder(true).SetTitle(" " + title + " ")
	a.root.AddPage("form", center(f, 84, 9), true, true)
}

// preview shows exactly what will be sent and the command that sends it.
func (a *App) preview(kind string, params map[string]string) {
	req, err := a.Disp.Render(kind, params)
	if err != nil {
		a.say("%v", err)
		return
	}
	mode := "[green]DRY RUN: the workflow will change nothing[-]"
	if kind == "deploy" && !req.DryRun() {
		mode = "[red]LIVE: the workflow will change Foundry[-]"
	}
	text := fmt.Sprintf("%s\n\n$ %s < payload\n\n%s\n\nsha256 %s", mode, tview.Escape(req.Cmd().String()), tview.Escape(string(req.Payload)), req.SHA256)
	m := tview.NewModal().SetText(text).AddButtons([]string{"Send", "Back", "Cancel"})
	m.SetDoneFunc(func(_ int, label string) {
		a.root.RemovePage("confirm")
		switch label {
		case "Send":
			a.root.RemovePage("form")
			go a.send(req)
		case "Cancel":
			a.root.RemovePage("form")
		}
	})
	a.root.AddPage("confirm", m, true, true)
}

func (a *App) send(req dispatch.Request) {
	a.sayLater("dispatching %s…", req.ID)
	out, err := a.Disp.Send(a.ctx, req)
	if err != nil {
		a.sayLater("dispatch failed: %v", err)
		return
	}
	a.sayLater("dispatched %s %s", req.ID, out)
	if a.GitHub == nil {
		return
	}
	for i := 0; i < 6; i++ { // the run takes a few seconds to exist
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		runs, err := a.GitHub.Runs(a.ctx, req.Workflow, 10)
		if err != nil {
			continue
		}
		if r, ok := github.FindRun(runs, req.ID); ok {
			_, _ = a.Log.Append(a.Actor, "dispatch.run", req.Workflow, map[string]string{"request_id": req.ID, "run_url": r.URL, "run_id": strconv.FormatInt(r.ID, 10)})
			a.sayLater("%s is run #%d (%s): %s", req.ID, r.Number, r.State(), r.URL)
			a.refresh()
			return
		}
	}
	a.sayLater("dispatched %s; the run has not appeared yet (press 4, then r)", req.ID)
}

func (a *App) probe(d azure.Deployment, kind string) {
	a.sayLater("probing %s via %s…", d.Name, kind)
	a.mu.Lock()
	acct := a.d.account
	a.mu.Unlock()
	tok, err := a.Azure.Token(a.ctx)
	if err != nil {
		a.sayLater("no data-plane token: %v", err)
		return
	}
	t := probe.NewTarget(acct, d)
	var res probe.Result
	if kind == "api" {
		res = probe.API(a.ctx, &http.Client{Timeout: 90 * time.Second}, t, tok)
	} else {
		res = probe.Copilot(a.ctx, a.Runner, a.Cfg.Copilot, a.TFS, t, tok)
	}
	_ = a.History.Append(res)
	_, _ = a.Log.Append(a.Actor, "probe."+kind, d.Name, map[string]string{
		"ok": strconv.FormatBool(res.OK), "latency_ms": strconv.FormatInt(res.LatencyMS, 10), "detail": res.Detail})
	verdict := "[green]pass[-]"
	if !res.OK {
		verdict = "[red]fail[-]"
	}
	a.popupLater("Probe", fmt.Sprintf("[::b]%s[::-] via %s: %s\n\nlatency  %d ms\nstatus   %d\ntokens   %d in, %d out\n\n%s",
		d.Name, kind, verdict, res.LatencyMS, res.Status, res.TokensIn, res.TokensOut, tview.Escape(res.Detail)))
	a.sayLater("probe %s via %s: ok=%v in %d ms", d.Name, kind, res.OK, res.LatencyMS)
}

func (a *App) writeReport() {
	a.sayLater("collecting a record…")
	snap := report.Collect(a.ctx, report.Sources{Cfg: a.Cfg, Azure: a.Azure, GitHub: a.GitHub, Perms: a.Perms,
		Log: a.Log, History: a.History, Version: a.Version, Actor: a.Actor})
	files, err := report.Write(a.TFS, a.Cfg.ReportDir, snap)
	if err != nil {
		a.sayLater("report failed: %v", err)
		return
	}
	_, _ = a.Log.Append(a.Actor, "report.write", files.HTML, map[string]string{"findings_high": strconv.Itoa(snap.Count("high"))})
	a.sayLater("wrote %s (%d high, %d to watch)", files.HTML, snap.Count("high"), snap.Count("warn"))
}

const helpText = `[::b]Reading[::-]
  Every table comes from az or from the GitHub REST API. Press r to
  reload; the screen also reloads on a timer.

[::b]Changing things[::-]
  n, s and d open a form. Preview renders a Go template into the JSON
  inputs of a workflow_dispatch and shows the gh command that would
  send it. Send runs that command. The workflow, not this program,
  talks to Azure. Dry run is on unless you turn it off.

[::b]Testing[::-]
  t asks the selected deployment for one word over HTTPS.
  c asks the same through the Copilot CLI in BYOK mode.
  T runs both in GitHub Actions and commits the result.

[::b]Records[::-]
  R writes an HTML and JSON record to the report directory.
  W asks the report workflow to do the same and commit it.
  View 6 is the local audit chain; view 7 shows what the
  Foundry Owner role can and cannot see.`
