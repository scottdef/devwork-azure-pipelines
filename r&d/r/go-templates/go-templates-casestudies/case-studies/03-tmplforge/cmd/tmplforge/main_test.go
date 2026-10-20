package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	forge.SandboxChildMain()
	os.Exit(m.Run())
}

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var o, e bytes.Buffer
	code := run(args, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("../../testdata/golden/cli", name)
	if *update {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(got), 0o644)
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (go test -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s:\n%s", name, forge.Unified("golden", "got", forge.Diff(forge.Lines(string(want)), forge.Lines(got)), 2))
	}
}

const cat = "../../catalog/"

func TestCLIGolden(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1792368000")
	for _, tc := range []struct {
		golden string
		code   int
		args   []string
	}{
		{"catalog.txt", 0, []string{"catalog"}},
		{"vars-sql.json", 0, []string{"vars", "-t", cat + "sql-migration.sql.tmpl"}},
		{"vars-refs-changelog.txt", 0, []string{"vars", "-refs", "-t", cat + "CHANGELOG.md.tmpl"}},
		{"render-motd.txt", 0, []string{"render", "-t", cat + "motd.txt.tmpl", "-d", cat + "fixtures/motd.txt.json"}},
		{"lint-ok.txt", 0, []string{"lint", "-t", cat + "Dockerfile.tmpl", "-d", cat + "fixtures/Dockerfile.json"}},
		{"lint-delims.txt", 0, []string{"lint", "-t", cat + "workflow.yml.tmpl", "-delims", "[[,]]", "-vars", "name,branches,os"}},
	} {
		code, out, errOut := runCLI(t, "", tc.args...)
		if code != tc.code {
			t.Errorf("%v: exit %d, want %d; stderr %s", tc.args, code, tc.code, errOut)
		}
		golden(t, tc.golden, out)
	}
}

func TestCLIRenderStdinSandbox(t *testing.T) {
	code, out, _ := runCLI(t, "{{ 3 | add 4 }}", "render", "-t", "-", "-sandbox")
	if code != 0 || out != "7" {
		t.Errorf("%d %q", code, out)
	}
	code, _, errOut := runCLI(t, `{{ env "HOME" }}`, "render", "-t", "-", "-sandbox")
	if code != forge.ExitRenderError || !strings.HasPrefix(errOut, "sandbox:render: ") {
		t.Errorf("%d %q", code, errOut)
	}
	code, out, _ = runCLI(t, `{{ env "TMPLFORGE_CLI_TEST" }}`, "render", "-t", "-")
	_ = out
	if code != 0 {
		t.Errorf("env outside sandbox: %d", code)
	}
}

func TestCLILintError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.tmpl")
	os.WriteFile(p, []byte("ok\n{{ if .a }}\n"), 0o644)
	code, out, _ := runCLI(t, "", "lint", "-t", p)
	if code != 1 || out != p+":3: unexpected EOF\n" {
		t.Errorf("%d %q", code, out)
	}
}

func TestCLITestDiff(t *testing.T) {
	dir := t.TempDir()
	tp := filepath.Join(dir, "t.tmpl")
	os.WriteFile(tp, []byte("{{.a}}\nfixed\n"), 0o644)
	a, b, c := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json"), filepath.Join(dir, "c.json")
	os.WriteFile(a, []byte(`{"a":1}`), 0o644)
	os.WriteFile(b, []byte(`{"a":2}`), 0o644)
	os.WriteFile(c, []byte(`{"a":2}`), 0o644)
	code, out, _ := runCLI(t, "", "test", "-t", tp, a, b, c)
	want := "--- " + a + "\n+++ " + b + "\n@@ -1,2 +1,2 @@\n-1\n+2\n fixed\n" + b + " == " + c + "\n"
	if code != 1 || out != want {
		t.Errorf("%d\n%s\nwant\n%s", code, out, want)
	}
	if code, _, _ := runCLI(t, "", "test", "-t", tp, b, c); code != 0 {
		t.Errorf("same: %d", code)
	}
}

func TestCLIExportAndNew(t *testing.T) {
	dir := t.TempDir()
	code, out, e := runCLI(t, "", "export", "-t", cat+"hello.c.tmpl", "-d", cat+"fixtures/hello.c.json", "-o", dir)
	if code != 0 || !strings.Contains(out, "hello.c.tmpl") {
		t.Fatalf("%d %s %s", code, out, e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "hello.c"))
	g, _ := os.ReadFile("../../testdata/golden/catalog/hello.c")
	if !bytes.Equal(b, g) {
		t.Error("exported output differs from catalog golden")
	}
	code, _, _ = runCLI(t, "", "new", "issueops", "access-request", "-fields", "user:string!,team:dropdown=a|b", "-o", dir)
	if code != 0 {
		t.Fatal(code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/ISSUE_TEMPLATE/access-request.yml")); err != nil {
		t.Error(err)
	}
	code, _, _ = runCLI(t, "", "new", "cicd", "tag", "-binary", "tmplforge", "-o", dir)
	if code != 0 {
		t.Fatal(code)
	}
	if code, _, _ := runCLI(t, "", "new", "cicd", "bogus"); code != 1 {
		t.Errorf("bogus kind: %d", code)
	}
}

func TestCLIMisc(t *testing.T) {
	if code, out, _ := runCLI(t, "", "version"); code != 0 || !strings.HasPrefix(out, "tmplforge dev go") {
		t.Errorf("%d %q", code, out)
	}
	if code, _, _ := runCLI(t, ""); code != 2 {
		t.Error("no args")
	}
	if code, _, _ := runCLI(t, "", "frob"); code != 2 {
		t.Error("unknown")
	}
	if code, out, _ := runCLI(t, "", "catalog", "-json"); code != 0 || !strings.Contains(out, `"name": "mkfile"`) {
		t.Error("catalog -json")
	}
	if code, out, _ := runCLI(t, "", "catalog", "-src", "mkfile"); code != 0 || !strings.HasPrefix(out, "</$objtype/mkfile") {
		t.Errorf("catalog -src: %q", out)
	}
	if code, out, _ := runCLI(t, "p\n", "sh", "-c", "hello.go"); code != 0 || !strings.Contains(out, "package main") {
		t.Errorf("sh: %d %q", code, out)
	}
	if code, _, e := runCLI(t, "", "serve", "-a", "0.0.0.0:0"); code != 1 || !strings.Contains(e, "-token is required") {
		t.Errorf("serve without token: %d %q", code, e)
	}
}
