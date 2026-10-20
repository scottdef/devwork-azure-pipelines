package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/golden"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// fakeItems makes item directories with the given phases.
func fakeItems(t *testing.T, phases map[string]int) string {
	root := t.TempDir()
	for id, ph := range phases {
		d := filepath.Join(root, filepath.FromSlash(id))
		os.MkdirAll(d, 0o755)
		cfg := `{"id":"` + id + `","phase":` + string(rune('0'+ph)) + `}`
		os.WriteFile(filepath.Join(d, "config.json"), []byte(cfg), 0o644)
	}
	return root
}

func newEnv(t *testing.T, items string, now time.Time) (env, *bytes.Buffer) {
	var out bytes.Buffer
	return env{file: filepath.Join(t.TempDir(), "ledger.json"), items: items, now: now, stdout: &out, stderr: &bytes.Buffer{}}, &out
}

func TestBorrowReturnFlow(t *testing.T) {
	items := fakeItems(t, map[string]int{"a1/readme/1": 1, "a1/readme/2": 1, "a1/readme/3": 2, "a1/readme/4": 1, "a1/readme/9": 3})
	e, out := newEnv(t, items, t0)
	for i, id := range []string{"a1/readme/1", "a1/readme/2", "a1/readme/3"} {
		out.Reset()
		if rc := run(e, []string{"borrow", "-item", id, "-by", "bob", "-days", "7", "-issue", string(rune('1' + i))}); rc != 0 {
			t.Fatalf("borrow %s: rc=%d %s", id, rc, out)
		}
	}
	golden.Check(t, "../../testdata/golden/ledger/borrow.result.json", out.Bytes())
	cases := []struct {
		args []string
		rc   int
		want string
	}{
		{[]string{"borrow", "-item", "a1/readme/4", "-by", "bob", "-days", "7", "-issue", "9"}, 3, "already holds 3"},
		{[]string{"borrow", "-item", "a1/readme/1", "-by", "carol", "-days", "7", "-issue", "9"}, 3, "already borrowed"},
		{[]string{"borrow", "-item", "a1/readme/5", "-by", "carol", "-days", "7", "-issue", "9"}, 3, "no such item"},
		{[]string{"borrow", "-item", "a1/readme/9", "-by", "carol", "-days", "7", "-issue", "9"}, 3, "final phase"},
		{[]string{"borrow", "-item", "a1/readme/4", "-by", "carol", "-days", "30", "-issue", "9"}, 3, "7 or 14"},
		{[]string{"borrow", "-item", "a1/readme/4"}, 2, ""},
		{[]string{"return", "-item", "a1/readme/1", "-by", "carol"}, 3, "only the borrower"},
		{[]string{"return", "-item", "a1/readme/1", "-by", "bob"}, 0, `"ok": true`},
		{[]string{"return", "-item", "a1/readme/1", "-by", "bob"}, 3, "not borrowed"},
		{[]string{"borrow", "-item", "a1/readme/4", "-by", "bob", "-days", "14", "-issue", "9"}, 0, `"held": 3`},
		{[]string{"nope"}, 2, ""},
		{[]string{}, 2, ""},
	}
	for _, c := range cases {
		out.Reset()
		if rc := run(e, c.args); rc != c.rc || !strings.Contains(out.String(), c.want) {
			t.Errorf("%v: rc=%d want %d; out=%s", c.args, rc, c.rc, out)
		}
	}
	out.Reset()
	e.now = t0.Add(10 * 24 * time.Hour)
	if rc := run(e, []string{"list", "-table"}); rc != 0 {
		t.Fatal(rc)
	}
	golden.Check(t, "../../testdata/golden/ledger/list.txt", out.Bytes())
	out.Reset()
	run(e, []string{"list", "-json", "-all"})
	golden.Check(t, "../../testdata/golden/ledger/list.json", out.Bytes())
}

func TestOverdueCommand(t *testing.T) {
	items := fakeItems(t, map[string]int{"a1/readme/1": 1, "a1/readme/2": 1})
	e, out := newEnv(t, items, t0)
	run(e, []string{"borrow", "-item", "a1/readme/1", "-by", "bob", "-days", "7", "-issue", "1"})
	e.now = t0.Add(3 * 24 * time.Hour)
	run(e, []string{"borrow", "-item", "a1/readme/2", "-by", "bob", "-days", "7", "-issue", "2"})
	e.now = t0.Add(15 * 24 * time.Hour) // #1 is 8 days late, #2 is 5 days late
	out.Reset()
	if rc := run(e, []string{"overdue", "-grace", "7d", "-n"}); rc != 0 {
		t.Fatal(rc)
	}
	if !strings.Contains(out.String(), `"applied": false`) {
		t.Error("dry run claims applied")
	}
	out.Reset()
	if rc := run(e, []string{"overdue", "-grace", "7d"}); rc != 0 {
		t.Fatal(rc)
	}
	golden.Check(t, "../../testdata/golden/ledger/overdue.json", out.Bytes())
	out.Reset()
	run(e, []string{"list", "-all"})
	if !strings.Contains(out.String(), "(auto)") || !strings.Contains(out.String(), "overdue 5d") {
		t.Errorf("list after overdue:\n%s", out)
	}
	out.Reset()
	if rc := run(e, []string{"overdue", "-grace", "soon"}); rc != 2 {
		t.Errorf("bad grace rc=%d", rc)
	}
}

func TestRender(t *testing.T) {
	items := fakeItems(t, map[string]int{"a1/readme/1": 1})
	e, out := newEnv(t, items, t0)
	run(e, []string{"borrow", "-item", "a1/readme/1", "-by", "bob", "-days", "7", "-issue", "11"})
	e.now = t0.Add(9 * 24 * time.Hour)
	out.Reset()
	if rc := run(e, []string{"render", "-t", "../../templates/comments/overdue-warning.md.tmpl", "-d", "../../testdata/tpl/overdue-notice.json"}); rc != 0 {
		t.Fatalf("rc=%d %s", rc, e.stderr)
	}
	golden.Check(t, "../../testdata/golden/ledger/overdue-warning.md", out.Bytes())
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{"7d": 7 * 24 * time.Hour, "36h": 36 * time.Hour, "0d": 0} {
		if got, err := parseDuration(in); err != nil || got != want {
			t.Errorf("%s = %v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "d", "-1d", "x", "-3h"} {
		if _, err := parseDuration(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}
