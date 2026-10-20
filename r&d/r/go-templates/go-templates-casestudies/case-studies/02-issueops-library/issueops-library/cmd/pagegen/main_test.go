package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/golden"
	"github.com/CoolGitOrg/issueops-library/internal/item"
	"github.com/CoolGitOrg/issueops-library/internal/ledger"
	"github.com/CoolGitOrg/issueops-library/internal/site"
)

func opts(t *testing.T, dir string, ev string, now time.Time) options {
	return options{
		event: "../../testdata/events/" + ev + ".json", siteDir: filepath.Join(dir, "site"),
		items: filepath.Join(dir, "items"), ledger: filepath.Join(dir, "ledger", "ledger.json"),
		templates: "../../templates/items", meta: "../../templates/meta", now: now,
		cfg: site.Config{BaseURL: "https://coolgitorg.github.io/issueops-library/", GitHub: "https://github.com",
			Repo: "CoolGitOrg/issueops-library", IntakeRepo: "CoolGitOrg/issueops-intake",
			IntakeURL: "https://coolgitorg.github.io/issueops-intake/", Version: "test", Now: now},
	}
}

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestPublishBorrowPhase2(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	res, err := run(opts(t, dir, "publish", t0), nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "../../testdata/golden/pagegen/publish.result.json", out.Bytes())
	if res.ItemURL != "https://coolgitorg.github.io/issueops-library/items/payments-api/caddyfile/42/" {
		t.Errorf("item url = %s", res.ItemURL)
	}
	for _, f := range []string{"items/payments-api/caddyfile/42/artifact.caddyfile", "site/index.html",
		"site/items/payments-api/caddyfile/42/index.html", "site/items/payments-api/caddyfile/42/bundle.tar.gz"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Error(err)
		}
	}

	// bob borrows (the workflow runs the ledger command before pagegen).
	l, _ := ledger.Load(filepath.Join(dir, "ledger", "ledger.json"))
	if _, err := l.Borrow("payments-api/caddyfile/42", "bob", 14, 50, "", true, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	l.Save(filepath.Join(dir, "ledger", "ledger.json"))

	// Phase 2 from someone else is refused and changes nothing.
	o := opts(t, dir, "publish-phase2", t0.Add(time.Hour))
	ev, _ := os.ReadFile(o.event)
	other := filepath.Join(dir, "p2-other.json")
	os.WriteFile(other, bytes.Replace(ev, []byte(`"requester": "bob"`), []byte(`"requester": "mallory"`), 1), 0o644)
	o.event = other
	out.Reset()
	if _, err := run(o, nil, &out); err == nil || !strings.Contains(out.String(), "reserved by bob") {
		t.Fatalf("mallory phase 2: err=%v out=%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "items/payments-api/caddyfile/52")); !os.IsNotExist(err) {
		t.Error("refused publish wrote an item")
	}

	// Phase 2 from bob publishes and closes the loan.
	out.Reset()
	res, err = run(opts(t, dir, "publish-phase2", t0.Add(time.Hour)), nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClosedBorrow != 50 {
		t.Errorf("closed_borrow_issue = %d, want 50", res.ClosedBorrow)
	}
	l, _ = ledger.Load(filepath.Join(dir, "ledger", "ledger.json"))
	if l.OpenLoan("payments-api/caddyfile/42") != nil {
		t.Error("loan still open after phase 2 published")
	}
	var cfg map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, "items/payments-api/caddyfile/52/config.json"))
	json.Unmarshal(b, &cfg)
	if cfg["supersedes"] != "payments-api/caddyfile/42" {
		t.Errorf("supersedes = %v", cfg["supersedes"])
	}
	page, _ := os.ReadFile(filepath.Join(dir, "site/items/payments-api/caddyfile/42/index.html"))
	if !strings.Contains(string(page), "Phase 2 was published as") {
		t.Error("phase 1 page does not point at phase 2")
	}
}

func TestBorrowEventRebuildsOnly(t *testing.T) {
	dir := t.TempDir()
	if _, err := run(opts(t, dir, "publish", t0), nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res, err := run(opts(t, dir, "borrow", t0), nil, &out)
	if err != nil || res.Action != "library-borrow" || res.Items != 1 {
		t.Errorf("%+v %v", res, err)
	}
}

func TestDescribe(t *testing.T) {
	o := opts(t, t.TempDir(), "borrow", t0)
	o.describe = true
	var out bytes.Buffer
	if _, err := run(o, nil, &out); err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "../../testdata/golden/pagegen/borrow.describe.txt", out.Bytes())
	if strings.Contains(out.String(), "phase 2 rollout") {
		t.Error("free text leaked into describe output")
	}
}

func TestBadEvents(t *testing.T) {
	for _, n := range []string{"bad-sha", "bad-path"} {
		if _, err := run(opts(t, t.TempDir(), n, t0), nil, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want error", n)
		}
	}
}

func TestPayloadMissingKeyRefused(t *testing.T) {
	dir := t.TempDir()
	o := opts(t, dir, "publish-nomad", t0)
	b, _ := os.ReadFile(o.event)
	var ev map[string]any
	json.Unmarshal(b, &ev)
	cp := ev["client_payload"].(map[string]any)
	cp["payload"] = `{"name":"ledger-svc"}`
	cp["sha256"] = item.SHA256Hex([]byte(cp["payload"].(string))) // valid, so only the template can fail
	nb, _ := json.Marshal(ev)
	o.event = filepath.Join(dir, "ev.json")
	os.WriteFile(o.event, nb, 0o644)
	var out bytes.Buffer
	_, err := run(o, nil, &out)
	if err == nil || !strings.Contains(out.String(), `"ok": false`) {
		t.Errorf("err=%v out=%s", err, out.String())
	}
}
