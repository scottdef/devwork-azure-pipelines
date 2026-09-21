// Package report takes a snapshot of Foundry and GitHub and writes it
// down as a self-contained HTML page plus the JSON it was made from.
//
// The page has no scripts, no fonts to fetch and no links it needs:
// it should open in ten years. The JSON is the record; the HTML is a
// view of it; the .sha256 file says neither has changed.
package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/github"
	"github.com/CoolGitOrg/foundry-tui/internal/perms"
	"github.com/CoolGitOrg/foundry-tui/internal/probe"
)

// Snapshot is everything known at one moment.
type Snapshot struct {
	Generated time.Time `json:"generated"`
	Version   string    `json:"version"`
	Actor     string    `json:"actor"`
	Scope     struct {
		Subscription  string `json:"subscription"`
		ResourceGroup string `json:"resource_group"`
		Account       string `json:"account"`
		Location      string `json:"location"`
		Repo          string `json:"repo"`
	} `json:"scope"`

	Role        string        `json:"role"`
	RoleSource  string        `json:"role_source"`
	Permissions []perms.Check `json:"permissions"`

	Account     azure.Account          `json:"account"`
	Deployments []azure.Deployment     `json:"deployments"`
	Models      []azure.Model          `json:"models"` // catalog entries for deployed models only
	Usage       []azure.Usage          `json:"usage"`
	Access      []azure.RoleAssignment `json:"access"`
	Health      azure.Health           `json:"health"`
	Alerts      []azure.Alert          `json:"alerts"`
	Metrics     []azure.Metric         `json:"metrics"`

	Workflows []github.Stats `json:"workflows"`
	Runs      []github.Run   `json:"runs"`

	Probes    []probe.Result `json:"probes"`
	Audit     []audit.Record `json:"audit"`
	AuditHead string         `json:"audit_head"` // hash of the newest record
	AuditErr  string         `json:"audit_error,omitempty"`

	Findings []Finding         `json:"findings"`
	Skipped  map[string]string `json:"skipped"` // section → why it is missing
}

// Finding is something a reader should look at.
type Finding struct {
	Severity string `json:"severity"` // high | warn | info
	Subject  string `json:"subject"`
	Text     string `json:"text"`
}

// Sources are the things a snapshot is collected from. GitHub may be nil.
type Sources struct {
	Cfg     config.Config
	Azure   azure.Client
	GitHub  *github.Client
	Perms   perms.Set
	Log     *audit.Log
	History probe.History
	Version string
	Actor   string
}

// Collect gathers every section the role allows, concurrently. One
// failing section never costs the others: its error lands in Skipped.
func Collect(ctx context.Context, src Sources) *Snapshot {
	s := &Snapshot{
		Generated: time.Now().UTC().Truncate(time.Second),
		Version:   src.Version, Actor: src.Actor,
		Role: src.Perms.Role, RoleSource: src.Perms.Source, Permissions: src.Perms.Matrix(),
		Skipped: map[string]string{},
	}
	s.Scope.Subscription, s.Scope.ResourceGroup = src.Cfg.Subscription, src.Cfg.ResourceGroup
	s.Scope.Account, s.Scope.Location, s.Scope.Repo = src.Cfg.Account, src.Cfg.Location, src.Cfg.Repo

	var mu sync.Mutex
	var wg sync.WaitGroup
	// section runs fn if the role covers key; fn stores its own result.
	section := func(key string, fn func() error) {
		c, _ := src.Perms.Can(key)
		if !c.Allowed {
			mu.Lock()
			s.Skipped[key] = "not collected: " + c.Action + " is outside " + src.Perms.Role + ". " + c.Note
			mu.Unlock()
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				s.Skipped[key] = err.Error()
				mu.Unlock()
			}
		}()
	}

	// The account comes first: health, alerts, access and metrics need its id.
	var err error
	if s.Account, err = src.Azure.Account(ctx); err != nil {
		s.Skipped["account"] = err.Error()
	}
	id := s.Account.ID
	var catalog []azure.Model

	section("deployments", func() (err error) { s.Deployments, err = src.Azure.Deployments(ctx); return })
	section("models", func() (err error) { catalog, err = src.Azure.Models(ctx); return })
	section("usage", func() (err error) { s.Usage, err = src.Azure.Usage(ctx); return })
	if id != "" {
		section("access", func() (err error) { s.Access, err = src.Azure.RoleAssignments(ctx, id); return })
		section("health", func() (err error) { s.Health, err = src.Azure.Health(ctx, id); return })
		section("alerts", func() (err error) { s.Alerts, err = src.Azure.Alerts(ctx, id); return })
		section("metrics", func() (err error) { s.Metrics, err = src.Azure.Metrics(ctx, id, src.Cfg.Metrics); return })
	}
	section("activity", func() error { return fmt.Errorf("the role allows it, but this version does not collect it") })
	if src.GitHub != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wfs, err1 := src.GitHub.Workflows(ctx)
			runs, err2 := src.GitHub.Runs(ctx, "", 100)
			mu.Lock()
			defer mu.Unlock()
			if err := firstErr(err1, err2); err != nil {
				s.Skipped["github"] = err.Error()
				return
			}
			s.Workflows, s.Runs = github.Summarize(wfs, runs), runs
		}()
	} else {
		s.Skipped["github"] = "no GitHub token"
	}
	wg.Wait()

	s.Models = deployed(catalog, s.Deployments)
	s.Probes, _ = src.History.Recent(50)
	if src.Log != nil {
		recs, err := src.Log.Read()
		if err == nil {
			_, err = audit.Verify(recs)
		}
		if err != nil {
			s.AuditErr = err.Error()
		}
		if n := len(recs); n > 0 {
			s.AuditHead = recs[n-1].Hash
			if n > 100 {
				recs = recs[n-100:]
			}
		}
		s.Audit = recs
	}
	s.Findings = Findings(s, time.Now())
	return s
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// deployed keeps the catalog rows that match a deployment exactly.
func deployed(catalog []azure.Model, ds []azure.Deployment) []azure.Model {
	var out []azure.Model
	for _, m := range catalog {
		for _, d := range ds {
			dm := d.Properties.Model
			if m.Format == dm.Format && m.Name == dm.Name && m.Version == dm.Version {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// Findings applies the house rules to a snapshot. The rules are few
// and mechanical; judgement belongs to the reader.
func Findings(s *Snapshot, now time.Time) []Finding {
	var fs []Finding
	add := func(sev, subj, format string, a ...any) {
		fs = append(fs, Finding{sev, subj, fmt.Sprintf(format, a...)})
	}
	for _, d := range s.Deployments {
		if st := d.Properties.ProvisioningState; st != "" && st != "Succeeded" {
			add("high", d.Name, "provisioning state is %s", st)
		}
		if d.Properties.VersionUpgradeOption == "NoAutoUpgrade" {
			add("info", d.Name, "auto-upgrade is off; the version will not move before retirement")
		}
		for _, m := range s.Models {
			dm := d.Properties.Model
			if m.Format != dm.Format || m.Name != dm.Name || m.Version != dm.Version {
				continue
			}
			if t, ok := m.Retires(); ok {
				switch days := int(t.Sub(now).Hours() / 24); {
				case days < 0:
					add("high", d.Name, "%s %s retired on %s", m.Name, m.Version, t.Format("2006-01-02"))
				case days <= 90:
					add("warn", d.Name, "%s %s retires in %d days (%s)", m.Name, m.Version, days, t.Format("2006-01-02"))
				}
			}
		}
	}
	for _, u := range s.Usage {
		if p := u.Percent(); p >= 80 {
			add("warn", u.Name.Value, "quota is %.0f%% used (%.0f of %.0f)", p, u.CurrentValue, u.Limit)
		}
	}
	if s.Account.ID != "" {
		if !s.Account.Properties.DisableLocalAuth {
			add("info", s.Account.Name, "key authentication is enabled; this tool and its workflows need only Entra ID")
		}
		if strings.EqualFold(s.Account.Properties.PublicNetworkAccess, "Enabled") {
			add("info", s.Account.Name, "public network access is enabled")
		}
	}
	if st := s.Health.Properties.AvailabilityState; st != "" && st != "Available" {
		add("high", s.Account.Name, "Resource Health reports %s: %s", st, s.Health.Properties.Summary)
	}
	for _, a := range s.Alerts {
		e := a.Properties.Essentials
		if e.MonitorCondition == "Fired" && e.AlertState != "Closed" {
			add("warn", e.AlertRule, "%s alert is firing since %s", e.Severity, e.StartDateTime)
		}
	}
	for _, w := range s.Workflows {
		done := w.Succeeded + w.Failed
		switch {
		case w.Agentic && done >= 5 && w.SuccessRate < 90:
			add("warn", w.Workflow, "agentic workflow succeeds %.0f%% of the time over %d runs", w.SuccessRate, done)
		case !w.Agentic && w.Last.State() == "failure":
			add("warn", w.Workflow, "last run failed: %s", w.Last.URL)
		}
	}
	seen := map[string]bool{}
	for _, p := range s.Probes { // newest first: only the latest per deployment and kind counts
		k := p.Deployment + "/" + p.Kind
		if seen[k] {
			continue
		}
		seen[k] = true
		if !p.OK && !strings.HasPrefix(p.Detail, "skipped") {
			add("warn", p.Deployment, "latest %s probe failed: %s", p.Kind, p.Detail)
		}
	}
	if s.AuditErr != "" {
		add("high", "audit log", "%s", s.AuditErr)
	}
	rank := map[string]int{"high": 0, "warn": 1, "info": 2}
	sort.SliceStable(fs, func(i, j int) bool { return rank[fs[i].Severity] < rank[fs[j].Severity] })
	return fs
}

// Count returns how many findings have the given severity.
func (s *Snapshot) Count(sev string) int {
	n := 0
	for _, f := range s.Findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// SkippedKeys returns the skipped sections in a stable order.
func (s *Snapshot) SkippedKeys() []string {
	ks := make([]string, 0, len(s.Skipped))
	for k := range s.Skipped {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// AgenticWorkflows filters the workflow stats.
func (s *Snapshot) AgenticWorkflows() []github.Stats {
	var out []github.Stats
	for _, w := range s.Workflows {
		if w.Agentic {
			out = append(out, w)
		}
	}
	return out
}

var funcs = template.FuncMap{
	"date": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("2006-01-02 15:04Z")
	},
	"short": func(s string) string {
		if len(s) > 12 {
			return s[:12]
		}
		return s
	},
	"pct":  func(f float64) string { return fmt.Sprintf("%.0f%%", f) },
	"num":  func(f float64) string { return fmt.Sprintf("%.0f", f) },
	"join": strings.Join,
	"last": func(runs []github.Run, n int) []github.Run {
		if len(runs) > n {
			return runs[:n]
		}
		return runs
	},
}

// Files are the paths Write produced.
type Files struct{ HTML, JSON, Sum string }

// Write stores the snapshot under dir as HTML, JSON and a checksum
// file in sha256sum format, then rebuilds dir/index.html.
func Write(tfs fs.FS, dir string, s *Snapshot) (Files, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Files{}, err
	}
	base := fmt.Sprintf("foundry-%s-%s", s.Scope.Account, s.Generated.Format("20060102T150405Z"))
	f := Files{
		HTML: filepath.Join(dir, base+".html"),
		JSON: filepath.Join(dir, base+".json"),
		Sum:  filepath.Join(dir, base+".sha256"),
	}
	js, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return f, err
	}
	jsum := sha256.Sum256(js)

	t, err := template.New("report.html.tmpl").Funcs(funcs).ParseFS(tfs, "report/report.html.tmpl")
	if err != nil {
		return f, err
	}
	var page strings.Builder
	err = t.Execute(&page, map[string]any{"S": s, "JSONFile": base + ".json", "JSONSum": hex.EncodeToString(jsum[:])})
	if err != nil {
		return f, err
	}
	hsum := sha256.Sum256([]byte(page.String()))
	sums := fmt.Sprintf("%x  %s.json\n%x  %s.html\n", jsum, base, hsum, base)

	for path, data := range map[string][]byte{f.JSON: js, f.HTML: []byte(page.String()), f.Sum: []byte(sums)} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return f, err
		}
	}
	return f, WriteIndex(tfs, dir)
}

// WriteIndex lists every report in dir, newest first.
func WriteIndex(tfs fs.FS, dir string) error {
	names, err := filepath.Glob(filepath.Join(dir, "foundry-*.html"))
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := range names {
		names[i] = filepath.Base(names[i])
	}
	t, err := template.New("index.html.tmpl").ParseFS(tfs, "report/index.html.tmpl")
	if err != nil {
		return err
	}
	var page strings.Builder
	if err := t.Execute(&page, names); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), []byte(page.String()), 0o644)
}
