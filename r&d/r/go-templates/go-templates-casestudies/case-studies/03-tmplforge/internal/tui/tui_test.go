package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func session(t *testing.T, script string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "one.json"), []byte(`{"name":"glenda","items":["a","b"]}`), 0o644)
	os.WriteFile(filepath.Join(dir, "two.json"), []byte(`{"name":"rob","items":["a","c","d"]}`), 0o644)
	var out bytes.Buffer
	s := New(strings.NewReader(script), &out)
	s.Dir = dir
	s.Getenv = func(string) string { return "" } // no $EDITOR
	err := s.Run()
	return out.String(), dir, err
}

// TestScriptedSession drives the REPL exactly as `tmplforge sh < x.tf`
// would and checks the whole transcript.
func TestScriptedSession(t *testing.T) {
	script := `# comments and blank lines are ignored

f txt
v name string world
v items list x,y
a
hello, {{.name | upper}}
{{range .items}}- {{.}}
{{end -}}
bye
.
n
p
l
D one.json
p
t one.json two.json
d 4
=
W hello.tmpl
w hello.txt
V
zz
q
`
	out, dir, err := session(t, script)
	if err == nil || err.Error() != "" {
		// the "zz" line is an error; in a script the first error is returned
		t.Fatalf("Run() = %v, want the unknown-command error", err)
	}
	want := `1	hello, {{.name | upper}}
2	{{range .items}}- {{.}}
3	{{end -}}
4	bye
hello, WORLD
- x
- y
bye
ok
one.json: 2 keys
hello, GLENDA
- a
- b
bye
--- one.json
+++ two.json
@@ -1,4 +1,5 @@
-hello, GLENDA
+hello, ROB
 - a
-- b
+- c
+- d
 bye
3
59
22
name	string	world
items	list	x,y
?
`
	if out != want {
		t.Errorf("transcript\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if string(b) != "hello, GLENDA\n- a\n- b\n" {
		t.Errorf("hello.txt = %q", b)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "hello.tmpl"))
	if !strings.HasPrefix(string(b), "hello, {{.name | upper}}\n") {
		t.Errorf("hello.tmpl = %q", b)
	}
}

func TestCatalogLoadAndLint(t *testing.T) {
	out, _, err := session(t, "c sql-migration.sql\nl\nf\ns\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sql-migration.sql: 13 lines, txt, 5 vars\n", "ok\n", "txt\n", "fixture fixtures/sql-migration.sql.json\n", "delims {{,}}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	out, _, _ = session(t, "c workflow.yml\ns\np\n")
	if !strings.Contains(out, "delims [[,]]") || !strings.Contains(out, "runs-on: ${{ matrix.os }}") {
		t.Errorf("delims not carried from catalog:\n%s", out)
	}
	out, _, err = session(t, "c nope\n")
	if err == nil || !strings.Contains(out, `? no catalog entry "nope"`) {
		t.Errorf("unknown entry: %v\n%s", err, out)
	}
}

func TestLintFindings(t *testing.T) {
	out, _, err := session(t, "v a string\na\n{{.a}}\n{{.b}\n.\nl\n")
	if err == nil || !strings.Contains(out, "untitled:2: bad character U+007D '}'") {
		t.Errorf("parse error: %v\n%s", err, out)
	}
	out, _, err = session(t, "v a string\nv unused int\na\n{{.a}}\n{{.b}}\n.\nl\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"untitled:2: warning: .b referenced but not declared", `untitled:1: warning: variable "unused" declared but never referenced`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestEditWithoutEditor(t *testing.T) {
	out, _, err := session(t, "a\nold\n.\ne\nnew {{.x}}\n.\nv x int 7\np\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, "new 7\n") {
		t.Errorf("e did not replace the template:\n%s", out)
	}
}

func TestEditWithEditor(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	ed := filepath.Join(dir, "ed.sh")
	// a scripted "editor" that rewrites the file it is given
	os.WriteFile(ed, []byte("#!/bin/sh\nprintf 'edited {{.n}}\\n' > \"$1\"\n"), 0o755)
	var out bytes.Buffer
	s := New(strings.NewReader("v n int 1\ne\np\n"), &out)
	s.Getenv = func(k string) string {
		if k == "EDITOR" {
			return ed
		}
		return ""
	}
	s.EdIn, s.EdOut = strings.NewReader(""), &out
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "edited 1\n") {
		t.Errorf("got\n%s", out.String())
	}
}

func TestDeleteRanges(t *testing.T) {
	out, _, err := session(t, "a\n1\n2\n3\n4\n5\n.\nd 2,3\nd 2,$\nn\nd 9\n")
	if err == nil {
		t.Error("d 9 should fail")
	}
	if !strings.Contains(out, "1\t1\n?") {
		t.Errorf("got\n%s", out)
	}
}

func TestInsert(t *testing.T) {
	out, _, _ := session(t, "a\nb\n.\ni 1\na\n.\ni 3\nc\n.\nn\n")
	if out != "1\ta\n2\tb\n3\tc\n" {
		t.Errorf("got %q", out)
	}
}

func TestShellBang(t *testing.T) {
	if runtime.GOOS == "plan9" {
		t.Skip("rc")
	}
	out, _, err := session(t, "a\nx\ny\n.\n!wc -l\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2\n!\n") {
		t.Errorf("got %q", out)
	}
}

func TestExportAndGenerate(t *testing.T) {
	out, dir, err := session(t, "c motd.txt\nx bundle\ng cicd dispatch gen\ng issueops motd-request gen\ng bogus x\n")
	if err == nil {
		t.Error("g bogus should fail")
	}
	for _, p := range []string{"bundle/SHA256SUMS", "bundle/run.go", "bundle/motd.txt", "bundle/motd.txt.tmpl", "gen/.github/workflows/dispatch.yml", "gen/.github/ISSUE_TEMPLATE/motd-request.yml", "gen/cmd/motd-request-parse/main.go"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s: %v\n%s", p, err, out)
		}
	}
	wf, _ := os.ReadFile(filepath.Join(dir, "gen/.github/workflows/dispatch.yml"))
	if !strings.Contains(string(wf), "      updates:\n        description: \"Updates\"\n        type: number") {
		t.Errorf("vars did not become dispatch inputs:\n%s", wf)
	}
}

func TestQuitWarnsWhenInteractive(t *testing.T) {
	var out bytes.Buffer
	s := New(strings.NewReader("a\nx\n.\nq\nq\n"), &out)
	s.Prompt = true
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "q again to quit") || strings.Count(out.String(), Prompt) != 3 {
		t.Errorf("got %q", out.String())
	}
}

func TestVarErrors(t *testing.T) {
	for _, line := range []string{"v", "v x", "v 9x string", "v x float", "v x int nope", "v x map [1]", "f pdf", "D", "t one.json", "w", "x", "g cicd"} {
		out, _, err := session(t, line+"\n")
		if err == nil || !strings.HasPrefix(out, "?") {
			t.Errorf("%q: err=%v out=%q", line, err, out)
		}
	}
}

func TestDefaultKeepsSpaces(t *testing.T) {
	out, _, _ := session(t, "v greet string hello,   world\nV\n")
	if out != "greet\tstring\thello,   world\n" {
		t.Errorf("got %q", out)
	}
}

func TestColor(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	var b bytes.Buffer
	if ColorEnabled(&b, env(map[string]string{"TERM": "xterm"})) {
		t.Error("colour on a non-file writer")
	}
	if ColorEnabled(os.Stdout, env(map[string]string{"TERM": "dumb"})) {
		t.Error("colour with TERM=dumb")
	}
	if ColorEnabled(os.Stdout, env(map[string]string{})) {
		t.Error("colour with TERM unset")
	}
	if ColorEnabled(os.Stdout, env(map[string]string{"TERM": "xterm", "NO_COLOR": "1"})) {
		t.Error("colour with NO_COLOR")
	}
	if got := painter(true).paint(red, "x"); got != "\x1b[31mx\x1b[0m" {
		t.Errorf("paint = %q", got)
	}
	if got := painter(false).paint(red, "x"); got != "x" {
		t.Errorf("paint off = %q", got)
	}
}

func TestMainScriptFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.tf")
	os.WriteFile(p, []byte("a\nhi {{ 1 | add 2 }}\n.\np\n"), 0o644)
	var o, e bytes.Buffer
	if code := Main([]string{p}, strings.NewReader(""), &o, &e); code != 0 || o.String() != "hi 3\n" {
		t.Errorf("code %d out %q err %q", code, o.String(), e.String())
	}
	o.Reset()
	if code := Main([]string{"-c", "hello.c"}, strings.NewReader("=\n"), &o, &e); code != 0 || o.String() != "hello.c: 35 lines, txt, 3 vars\n35\n" {
		t.Errorf("code %d out %q err %q", code, o.String(), e.String())
	}
	if code := Main([]string{"-c", "nope"}, strings.NewReader(""), &o, &e); code != 1 {
		t.Errorf("bad -c: code %d", code)
	}
}
