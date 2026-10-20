package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/go-template-cookbook/formulas"
)

func do(t *testing.T, stdin string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func TestList(t *testing.T) {
	code, out, _ := do(t, "", "list")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != len(formulas.All) || !strings.HasPrefix(lines[0], "F01\t") {
		t.Fatalf("got %d lines, first %q", len(lines), lines[0])
	}
}

func TestShow(t *testing.T) {
	code, out, _ := do(t, "", "show", "f31")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"F31  SQL", "-- template --", "-- data --", "-- glue --", "func glueSQL", "-- output --", `WHERE "name" = $1;`, "-- notes --"} {
		if !strings.Contains(out, want) {
			t.Errorf("show F31 lacks %q", want)
		}
	}
}

func TestRun(t *testing.T) {
	code, out, _ := do(t, "", "run", "F07")
	f, _ := formulas.Lookup("F07")
	if code != 0 || out != f.Want {
		t.Fatalf("exit %d, output %q", code, out)
	}
}

func TestRunWithData(t *testing.T) {
	code, out, errOut := do(t, `{"Env":{"B":"2","A":"1"}}`, "run", "F07", "-d", "-")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := "A=1\nB=2\nkeys: A,B\n"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	// the flag may come before or after the ID
	code, out2, _ := do(t, `{"Env":{"B":"2","A":"1"}}`, "run", "-d", "-", "F07")
	if code != 0 || out2 != out {
		t.Fatalf("flag before ID: exit %d, %q", code, out2)
	}
}

func TestRunWithDataFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	if err := os.WriteFile(path, []byte(`{"Msg":"hello"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := do(t, "", "run", "F01", "-d", path)
	if code != 0 || !strings.Contains(out, "<info:hello>") {
		t.Fatalf("exit %d, %q, %s", code, out, errOut)
	}
}

// Formulas whose glue wants a struct keep their type under -d.
func TestRunWithTypedData(t *testing.T) {
	code, out, errOut := do(t, `{"Prog":"x","Flags":[{"Name":"q","Kind":"bool","Def":"false","Usage":"quiet"}]}`, "run", "F38", "-d", "-")
	if code != 0 || !strings.Contains(out, "usage: x [flags]") || !strings.Contains(out, "-q") {
		t.Fatalf("exit %d, %q, %s", code, out, errOut)
	}
}

func TestRunErrors(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		stdin string
		code  int
		msg   string
	}{
		{[]string{"run"}, "", 2, "usage"},
		{[]string{"run", "F99"}, "", 2, "no formula"},
		{[]string{"run", "F01", "-d", "/nonexistent/x.json"}, "", 1, "no such file"},
		{[]string{"run", "F01", "-d", "-"}, "{not json", 1, "invalid character"},
		{[]string{"run", "F01", "-zzz"}, "", 2, "flag provided but not defined"},
		{[]string{"show"}, "", 2, "usage"},
		{[]string{"list", "extra"}, "", 2, "usage"},
		{[]string{"frobnicate"}, "", 2, "unknown command"},
		{nil, "", 2, "usage"},
	} {
		code, _, errOut := do(t, tc.stdin, tc.args...)
		if code != tc.code || !strings.Contains(errOut, tc.msg) {
			t.Errorf("%v: exit %d, stderr %q; want exit %d containing %q", tc.args, code, errOut, tc.code, tc.msg)
		}
	}
}

func TestHelpAndVersion(t *testing.T) {
	if code, out, _ := do(t, "", "help"); code != 0 || !strings.Contains(out, "usage: cookbook") {
		t.Errorf("help: %d %q", code, out)
	}
	if code, out, _ := do(t, "", "version"); code != 0 || strings.TrimSpace(out) != version {
		t.Errorf("version: %d %q", code, out)
	}
}

func TestGenIsDeterministicAndComplete(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "sub", "b.md")
	if code, _, e := do(t, "", "gen", "-o", a); code != 0 {
		t.Fatalf("gen: %s", e)
	}
	if code, _, e := do(t, "", "gen", "-o", b); code != 0 { // also creates the directory
		t.Fatalf("gen: %s", e)
	}
	ba, _ := os.ReadFile(a)
	bb, _ := os.ReadFile(b)
	if !bytes.Equal(ba, bb) {
		t.Fatal("two runs differ")
	}
	doc := string(ba)
	for _, f := range formulas.All {
		if !strings.Contains(doc, "## "+f.ID+". "+f.Name) {
			t.Errorf("no section for %s", f.ID)
		}
		if !strings.Contains(doc, "| ["+f.ID+"](#"+strings.ToLower(f.ID)+") |") {
			t.Errorf("no TOC row for %s", f.ID)
		}
		if !strings.Contains(doc, f.Notes) {
			t.Errorf("%s: notes missing", f.ID)
		}
		if f.Glue != "" && !strings.Contains(doc, f.Glue) {
			t.Errorf("%s: glue missing", f.ID)
		}
	}
	for _, want := range []string{"```gotemplate", "```json", "```go", "### Output", "${{ github.token }}"} {
		if !strings.Contains(doc, want) {
			t.Errorf("document lacks %q", want)
		}
	}
	if strings.Count(doc, "```gotemplate") != len(formulas.All) {
		t.Errorf("want one gotemplate fence per formula")
	}
}

func TestGenFuncs(t *testing.T) {
	code, out, _ := do(t, "", "gen", "-funcs", "-o", "-")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"# Shared FuncMap", "| Function | Signature | Example | Output | Notes |", "`toYAML`", "`regexReplaceAll`", "`c<aaa>ndy`"} {
		if !strings.Contains(out, want) {
			t.Errorf("funcs doc lacks %q", want)
		}
	}
}

func TestFenceGrowsAroundBackticks(t *testing.T) {
	if got := fence("a ```` b"); got != "`````" {
		t.Errorf("fence = %q", got)
	}
	if got := fence("plain"); got != "```" {
		t.Errorf("fence = %q", got)
	}
	if got := code("a`b"); got != "``a`b``" {
		t.Errorf("code = %q", got)
	}
	if got := code("`x`"); got != "`` `x` ``" {
		t.Errorf("code = %q", got)
	}
	if got := code("a|b\nc\td"); got != ``+"`"+`a\|b\nc\td`+"`" {
		t.Errorf("code = %q", got)
	}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if code, _, e := do(t, "", "gen", "-o", filepath.Join(docs, "COOKBOOK.md")); code != 0 {
		t.Fatal(e)
	}
	if code, _, e := do(t, "", "gen", "-funcs", "-o", filepath.Join(docs, "FUNCS-SHARED.md")); code != 0 {
		t.Fatal(e)
	}
	out := filepath.Join("..", "..", "testdata", "out")
	code, stdout, e := do(t, "", "check", "-docs", docs, "-out", out)
	if code != 0 || !strings.Contains(stdout, "ok: 55 formulas") {
		t.Fatalf("check: exit %d, %q, %s", code, stdout, e)
	}
	// stale docs are reported
	if err := os.WriteFile(filepath.Join(docs, "COOKBOOK.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, e = do(t, "", "check", "-docs", docs, "-out", out)
	if code != 1 || !strings.Contains(e, "stale") {
		t.Fatalf("stale docs: exit %d, %s", code, e)
	}
}
