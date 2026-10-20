package forge

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/tmplforge/internal/funcs"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestMain lets this test binary act as the sandbox child: Sandbox
// re-executes os.Args[0] with "render", and SandboxChildMain takes over.
func TestMain(m *testing.M) {
	SandboxChildMain()
	os.Setenv("SOURCE_DATE_EPOCH", "1792368000")
	os.Exit(m.Run())
}

const goldenDir = "../../testdata/golden"

func golden(t *testing.T, rel string, got []byte) {
	t.Helper()
	p := filepath.Join(goldenDir, rel)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./... -update)", err)
	}
	if !bytes.Equal(got, want) {
		ops := Diff(Lines(string(want)), Lines(string(got)))
		t.Errorf("%s differs from golden:\n%s", rel, Unified("golden", "got", ops, 2))
	}
}

// TestCatalogGolden renders every catalog entry against its fixture with
// the Safe FuncMap (what the GUI uses) and compares with testdata/golden.
func TestCatalogGolden(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) < 25 {
		t.Errorf("catalog has %d entries, want >= 25", len(c.Entries))
	}
	for _, e := range c.Entries {
		t.Run(e.Name, func(t *testing.T) {
			src, err := c.Source(e)
			if err != nil {
				t.Fatal(err)
			}
			data, err := c.Fixture(e)
			if err != nil {
				t.Fatal(err)
			}
			o, err := e.Options()
			if err != nil {
				t.Fatal(err)
			}
			o.Funcs = funcs.Safe()
			out, err := Render(src, data, o)
			if err != nil {
				t.Fatal(err)
			}
			if err := CheckOutput(e.Format, out); err != nil {
				t.Fatal(err)
			}
			if fs := Lint(src, LintOptions{Options: o, Fixture: data}); len(fs) > 0 {
				t.Errorf("lint findings on a catalog entry: %v", fs)
			}
			golden(t, filepath.Join("catalog", e.Output), out)
		})
	}
}

func TestCatalogIndexMatchesFiles(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range c.Entries {
		if e.File != e.Name+".tmpl" || e.Fixture != "fixtures/"+e.Name+".json" || e.Output != e.Name {
			t.Errorf("%s: file/fixture/output naming is inconsistent: %+v", e.Name, e)
		}
		if e.Description == "" {
			t.Errorf("%s: no description", e.Name)
		}
	}
}

func TestRenderHTMLEscapes(t *testing.T) {
	data := map[string]any{"x": `<script>alert(1)</script>`, "u": "javascript:alert(1)"}
	out, err := Render(`<p>{{.x}}</p><a href="{{.u}}">l</a>`, data, Options{Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "<script>") || strings.Contains(string(out), "javascript:") {
		t.Errorf("html format did not escape: %s", out)
	}
	out, err = Render(`<p>{{.x}}</p>`, data, Options{Format: "txt"})
	if err != nil || !strings.Contains(string(out), "<script>") {
		t.Errorf("txt format must not escape: %s %v", out, err)
	}
}

func TestRenderLimits(t *testing.T) {
	_, err := Render(strings.Repeat("x", 100), nil, Options{MaxTemplate: 10})
	if !errors.Is(err, ErrTemplateLimit) {
		t.Errorf("template cap: %v", err)
	}
	out, err := Render(`{{range seq 100}}0123456789{{end}}`, nil, Options{MaxOutput: 55})
	if !errors.Is(err, ErrOutputLimit) || len(out) != 55 {
		t.Errorf("output cap: %d bytes, %v", len(out), err)
	}
	if _, err := Render(`x`, nil, Options{Format: "pdf"}); !errors.Is(err, ErrFormat) {
		t.Errorf("format: %v", err)
	}
	out, err = Render(`[[ .a ]] {{ .a }}`, map[string]any{"a": 1}, Options{Left: "[[", Right: "]]"})
	if err != nil || string(out) != "1 {{ .a }}" {
		t.Errorf("delims: %q %v", out, err)
	}
	if err := CheckOutput("json", []byte(`{"a":`)); !errors.Is(err, ErrInvalidOutput) {
		t.Errorf("json check: %v", err)
	}
}

func TestLoadFixture(t *testing.T) {
	m, err := LoadFixture(strings.NewReader(`{"port": 8080, "f": 1.5, "l": [1], "m": {"n": 2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["port"] != 8080 || m["f"] != 1.5 || m["l"].([]any)[0] != 1 || m["m"].(map[string]any)["n"] != 2 {
		t.Errorf("numbers not normalised: %#v", m)
	}
	for _, bad := range []string{`[1]`, `"x"`, `{"a":1} {"b":2}`, `{`} {
		if _, err := LoadFixture(strings.NewReader(bad)); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
	if m, err := LoadFixture(strings.NewReader("")); err != nil || len(m) != 0 {
		t.Errorf("empty fixture: %v %v", m, err)
	}
}

func TestLintLines(t *testing.T) {
	tests := []struct {
		src     string
		line    int
		contain string
	}{
		{"ok\n{{.a}\n", 2, "bad character"},
		{"a\nb\n{{if .x}}\nc", 4, "unexpected EOF"},
		{"a\n\n\n{{nofunc .x}}", 4, `function "nofunc" not defined`},
		{"{{range .a}}\n{{else}}\n{{else}}\n{{end}}", 3, "expected end"},
		{"{{.a}}\n{{template \"nope\"}}", 0, ""}, // parses; exec-time error, not lint
	}
	for _, tc := range tests {
		fs := Lint(tc.src, LintOptions{})
		if tc.line == 0 {
			if HasError(fs) {
				t.Errorf("%q: unexpected %v", tc.src, fs)
			}
			continue
		}
		if len(fs) != 1 || fs[0].Sev != SevError || fs[0].Line != tc.line || !strings.Contains(fs[0].Msg, tc.contain) {
			t.Errorf("%q: got %+v, want error at line %d containing %q", tc.src, fs, tc.line, tc.contain)
		}
		if strings.HasPrefix(fs[0].Msg, "template:") {
			t.Errorf("message keeps location prefix: %q", fs[0].Msg)
		}
	}
	// html/template's stricter checks are linted when format is html
	fs := Lint(`<a href="{{.u}}`, LintOptions{Options: Options{Format: "html"}})
	_ = fs // html/template defers most context errors to Execute; parse succeeds
}

func TestLintWarnings(t *testing.T) {
	src := "{{.a}}\n{{range .items}}{{.name}}{{end}}\n{{$.c}}\n"
	fs := Lint(src, LintOptions{Declared: []string{"a", "items", "unused"}, Fixture: map[string]any{"a": 1}})
	var got []string
	for _, f := range fs {
		got = append(got, f.Format("t"))
	}
	want := []string{
		"t:3: warning: .c referenced but not declared",
		`t:1: warning: variable "unused" declared but never referenced`,
		"t:2: warning: .items missing from fixture",
		"t:3: warning: .c missing from fixture",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if HasError(fs) {
		t.Error("warnings must not be errors")
	}
}

func TestVarsWalker(t *testing.T) {
	src := `{{.title}}
{{range .users}}{{.name}} {{.email}}
  {{range .roles}}{{.id}}{{end}}
  {{with .address}}{{.city}}{{end}}
  {{$.footer}}
{{end}}
{{with .meta}}{{.version}}{{range $k, $v := .tags}}{{$v.label}}{{end}}{{end}}
{{$u := .owner}}{{$u.login}}
{{if .enabled}}on{{end}}
{{add .count 1}}
{{index .labels "env" | printf "%s"}}
{{define "row"}}{{.cell}}{{end}}{{range .rows}}{{template "row" .}}{{end}}
{{(.cfg).port}}
{{range $i, $e := .pairs}}{{$e.k}}{{end}}
{{range seq 3}}{{.}}{{end}}
{{len .things}}`
	v, err := Vars(src, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"title":   "",
		"users":   []any{map[string]any{"name": "", "email": "", "roles": []any{map[string]any{"id": ""}}, "address": map[string]any{"city": ""}}},
		"footer":  "",
		"meta":    map[string]any{"version": "", "tags": []any{map[string]any{"label": ""}}},
		"owner":   map[string]any{"login": ""},
		"enabled": false,
		"count":   0,
		"labels":  map[string]any{"env": ""},
		"rows":    []any{map[string]any{"cell": ""}},
		"cfg":     map[string]any{"port": ""},
		"pairs":   []any{map[string]any{"k": ""}},
		"things":  "",
	}
	if !reflect.DeepEqual(v.Skeleton, want) {
		t.Errorf("skeleton\n got %#v\nwant %#v", v.Skeleton, want)
	}
	lines := map[string]int{}
	for _, r := range v.Refs {
		lines[r.Path] = r.Line
	}
	for p, l := range map[string]int{".title": 1, ".users": 2, ".users[].roles[].id": 3, ".users[].address.city": 4, ".footer": 5, ".meta.version": 7, ".owner.login": 8} {
		if lines[p] != l {
			t.Errorf("%s: line %d, want %d", p, lines[p], l)
		}
	}
}

func TestVarsRecursiveTemplate(t *testing.T) {
	// self-recursive define must terminate
	v, err := Vars(`{{define "t"}}{{.x}}{{template "t" .child}}{{end}}{{template "t" .root}}`, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.Skeleton["root"]; !ok {
		t.Errorf("root missing: %v", v.Skeleton)
	}
}

func TestVarsDelims(t *testing.T) {
	v, err := Vars("[[ .a ]] ${{ github.sha }}", "[[", "]]")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Skeleton, map[string]any{"a": ""}) {
		t.Errorf("%v", v.Skeleton)
	}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		a, b string
		want string // ops as kind+text joined by |
	}{
		{"", "", ""},
		{"a", "a", " a"},
		{"a", "", "-a"},
		{"", "a", "+a"},
		{"a\nb\nc", "a\nc", " a|-b| c"},
		{"a\nc", "a\nb\nc", " a|+b| c"},
		{"a\nb\nc", "x\ny\nz", "-a|-b|-c|+x|+y|+z"},
		{"a\nb\nc\nd", "b\nc\nd\ne", "-a| b| c| d|+e"},
		{"x\na\nb", "a\nb\nx", "-x| a| b|+x"},
	}
	for _, tc := range tests {
		var parts []string
		for _, o := range Diff(Lines(tc.a), Lines(tc.b)) {
			parts = append(parts, string(o.Kind)+o.Text)
		}
		if got := strings.Join(parts, "|"); got != tc.want {
			t.Errorf("Diff(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDiffIsLCS(t *testing.T) {
	// The kept lines must be a common subsequence of maximal length.
	a := Lines("a\nb\nc\nb\nd\na\nb")
	b := Lines("b\nd\nc\na\nb\na")
	ops := Diff(a, b)
	keep := 0
	var ra, rb []string
	for _, o := range ops {
		if o.Kind != '+' {
			ra = append(ra, o.Text)
		}
		if o.Kind != '-' {
			rb = append(rb, o.Text)
		}
		if o.Kind == ' ' {
			keep++
		}
	}
	if !reflect.DeepEqual(ra, a) || !reflect.DeepEqual(rb, b) {
		t.Fatalf("script does not reproduce inputs")
	}
	if keep != 4 { // LCS("abcbdab","bdcaba") = 4, the textbook example
		t.Errorf("kept %d lines, LCS is 4", keep)
	}
}

func TestUnified(t *testing.T) {
	a := Lines("1\n2\n3\n4\n5\n6\n7\n8\n9\n10")
	b := Lines("1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11")
	got := Unified("a", "b", Diff(a, b), 1)
	want := "--- a\n+++ b\n@@ -2,3 +2,3 @@\n 2\n-3\n+three\n 4\n@@ -10 +10,2 @@\n 10\n+11\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if Unified("a", "b", Diff(a, a), 3) != "" {
		t.Error("no-change diff must be empty")
	}
}

func TestDiffHugeFallsBack(t *testing.T) {
	a := make([]string, 3000)
	b := make([]string, 3000)
	for i := range a {
		a[i], b[i] = fmt.Sprint("a", i), fmt.Sprint("b", i)
	}
	ops := Diff(a, b)
	if len(ops) != 6000 {
		t.Errorf("fallback produced %d ops", len(ops))
	}
}

func TestExportBundleBuilds(t *testing.T) {
	dir := t.TempDir()
	src := "{{ .name | upper }} has {{ len .items }} {{ len .items | plural \"item\" \"items\" }}\n{{ toYAML .items }}\n"
	files, err := Export(dir, Bundle{Name: "report.txt", Template: src, Fixture: []byte(`{"name":"glenda","items":["a","b"]}`), Options: Options{Format: "txt"}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	want := []string{"Makefile", "fixture.json", "funcs.go", "go.mod", "report.txt", "report.txt.tmpl", "run.go", "yaml.go", "SHA256SUMS"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("files %v, want %v", names, want)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "report.txt"))
	if string(out) != "GLENDA has 2 items\n- a\n- b\n" {
		t.Errorf("output %q", out)
	}
	if testing.Short() {
		return
	}
	// The bundle must build and reproduce its own output with only the
	// standard library: no network, no module cache.
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	got, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("go run: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go run: %v", err)
	}
	if !bytes.Equal(got, out) {
		t.Errorf("bundle run.go output %q != exported %q", got, out)
	}
}

func TestExportTarGzReproducible(t *testing.T) {
	b := Bundle{Name: "x", Template: "{{.a}}", Fixture: []byte(`{"a":1}`), Options: Options{Format: "txt"}}
	files, err := ExportFiles(b)
	if err != nil {
		t.Fatal(err)
	}
	var one, two bytes.Buffer
	WriteTarGz(&one, "x", files)
	WriteTarGz(&two, "x", files)
	if !bytes.Equal(one.Bytes(), two.Bytes()) {
		t.Error("tar.gz not reproducible")
	}
	zr, err := gzip.NewReader(&one)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(h.Name, "x/") {
			t.Errorf("entry %s outside prefix", h.Name)
		}
		n++
	}
	if n != len(files) {
		t.Errorf("%d entries, want %d", n, len(files))
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{"../../etc/passwd": "passwd", "a b/c d": "c-d", "": "template", "..": "template", "ok.yml": "ok.yml"} {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func sandbox() *Sandbox {
	return &Sandbox{Exe: os.Args[0], Timeout: 2 * time.Second}
}

func TestSandboxRender(t *testing.T) {
	out, err := sandbox().Render(context.Background(), "hi {{.name | upper}}", []byte(`{"name":"rob"}`), Options{})
	if err != nil || string(out) != "hi ROB" {
		t.Fatalf("%q %v", out, err)
	}
	out, err = sandbox().Render(context.Background(), "[[.a]]{{x}}", []byte(`{"a":1}`), Options{Left: "[[", Right: "]]"})
	if err != nil || string(out) != "1{{x}}" {
		t.Fatalf("delims: %q %v", out, err)
	}
}

func TestSandboxTemplateError(t *testing.T) {
	_, err := sandbox().Render(context.Background(), "a\nb\n{{.x | nofunc}}", nil, Options{})
	var re *RenderError
	if !errors.As(err, &re) || re.Line != 3 {
		t.Fatalf("want RenderError at line 3, got %#v", err)
	}
	_, err = sandbox().Render(context.Background(), "{{env \"HOME\"}}", nil, Options{})
	if !errors.As(err, &re) || !strings.Contains(re.Msg, `"env" not defined`) {
		t.Fatalf("env must be unavailable in the sandbox: %v", err)
	}
}

func TestSandboxTimeoutKillsLoop(t *testing.T) {
	s := sandbox()
	s.Timeout = 300 * time.Millisecond
	start := time.Now()
	_, err := s.Render(context.Background(), "{{range seq 100000000}}{{end}}", nil, Options{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v to time out", d)
	}
}

func TestSandboxHardKill(t *testing.T) {
	// A child that ignores its own deadline (here: a shell that sleeps;
	// the render flags become its unused positional parameters) is killed
	// by the parent at Timeout+Grace.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	s := &Sandbox{Exe: sh, Args: []string{"-c", "exec sleep 30", "sh"}, Timeout: 200 * time.Millisecond, Grace: 100 * time.Millisecond}
	start := time.Now()
	_, err = s.Render(context.Background(), "x", nil, Options{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("hard kill took %v", d)
	}
}

func TestSandboxOutputCap(t *testing.T) {
	s := sandbox()
	s.MaxOutput = 1000
	out, err := s.Render(context.Background(), `{{range seq 1000}}0123456789{{end}}`, nil, Options{})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("want ErrOutputLimit, got %v", err)
	}
	if len(out) > 1000 {
		t.Errorf("parent kept %d bytes", len(out))
	}
	// default cap: 1 MiB
	_, err = sandbox().Render(context.Background(), `{{range seq 2000}}{{repeat 1000 "x"}}{{end}}`, nil, Options{})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("want ErrOutputLimit at 1 MiB, got %v", err)
	}
}

func TestSandboxTemplateCap(t *testing.T) {
	_, err := sandbox().Render(context.Background(), strings.Repeat("x", MaxTemplate+1), nil, Options{})
	if !errors.Is(err, ErrTemplateLimit) {
		t.Fatalf("want ErrTemplateLimit, got %v", err)
	}
}

func TestSandboxCrashIsContained(t *testing.T) {
	// Unbounded {{template}} recursion is a Go runtime fatal error (stack
	// overflow), which recover cannot catch. In the child it is just a
	// dead process; the parent reports it and carries on.
	_, err := sandbox().Render(context.Background(), `{{define "r"}}{{template "r" .}}{{end}}{{template "r" .}}`, nil, Options{})
	if !errors.Is(err, ErrCrashed) {
		t.Fatalf("want ErrCrashed, got %v", err)
	}
	// and the parent still works
	out, err := sandbox().Render(context.Background(), "ok", nil, Options{})
	if err != nil || string(out) != "ok" {
		t.Fatalf("after crash: %q %v", out, err)
	}
}

func TestSandboxMinimalEnv(t *testing.T) {
	t.Setenv("TMPLFORGE_SECRET", "hunter2")
	for _, kv := range MinimalEnv() {
		if strings.Contains(kv, "hunter2") {
			t.Fatal("secret leaked into child env")
		}
	}
}

func TestSandboxContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s := sandbox()
	s.Timeout = 5 * time.Second
	start := time.Now()
	_, err := s.Render(ctx, "{{range seq 100000000}}{{end}}", nil, Options{})
	if err == nil {
		t.Fatal("want error on cancelled context")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("cancel took %v", d)
	}
}

func TestRenderCommandUsage(t *testing.T) {
	var o, e bytes.Buffer
	if code := RenderCommand(nil, nil, &o, &e); code != ExitUsage {
		t.Errorf("no -t: exit %d", code)
	}
	if code := RenderCommand([]string{"-t", "-", "-d", "-"}, nil, &o, &e); code != ExitUsage {
		t.Errorf("both stdin: exit %d", code)
	}
	e.Reset()
	code := RenderCommand([]string{"-t", "-", "-f", "json"}, strings.NewReader(`{"a":`), &o, &e)
	if code != ExitRenderError {
		t.Errorf("invalid json output: exit %d %s", code, e.String())
	}
}

func TestFormatFromName(t *testing.T) {
	for in, want := range map[string]string{"a.html.tmpl": "html", "x.svg": "html", "R.md": "md", "c.json.tmpl": "json", "w.yml": "yaml", "w.yaml.tpl": "yaml", "Makefile": "txt"} {
		if got := FormatFromName(in); got != want {
			t.Errorf("FormatFromName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorLine(t *testing.T) {
	for msg, want := range map[string]int{
		`template: x:12: function "f" not defined`:                   12,
		`template: x:3:5: executing "x" at <.a>: error calling f: e`: 3,
		`html/template:x:7:3: no such template`:                      7,
		`fixture: bad`:                                               0,
	} {
		if got := ErrorLine(errors.New(msg)); got != want {
			t.Errorf("ErrorLine(%q) = %d, want %d", msg, got, want)
		}
	}
}

func FuzzLint(f *testing.F) {
	for _, s := range []string{"{{.a}}", "{{range .x}}{{.y}}{{end}}", "{{if}}", "{{define \"a\"}}{{template \"a\" .}}{{end}}", "{{$x := .a}}{{$x.b}}", "{{index .m \"k\" 1}}", "{{(.a).b}}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			return
		}
		fs := Lint(src, LintOptions{Declared: []string{"a"}})
		for _, x := range fs {
			if x.Line < 1 && x.Sev == SevError {
				t.Errorf("error without line: %+v", x)
			}
		}
		Vars(src, "", "")
	})
}

func FuzzSandbox(f *testing.F) {
	for _, s := range []string{"{{.a}}", "{{range seq 3}}{{.}}{{end}}", "{{toJSON .}}", "{{repeat 3 \"ab\"}}", "{{template \"x\"}}"} {
		f.Add(s, `{"a":1}`)
	}
	f.Fuzz(func(t *testing.T, src, fixture string) {
		if len(src) > 2048 || len(fixture) > 2048 {
			return
		}
		// In-process with the sandbox's own caps: the property is that
		// nothing panics and caps hold. (The process boundary itself is
		// covered by the TestSandbox* tests; spawning per fuzz input would
		// make the fuzzer useless.)
		data, err := LoadFixture(strings.NewReader(fixture))
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			out, _ := Render(src, data, Options{Funcs: funcs.Safe(), MaxTemplate: MaxTemplate, MaxOutput: 1 << 16})
			if len(out) > 1<<16 {
				t.Errorf("output cap exceeded: %d", len(out))
			}
		}()
		select {
		case <-done:
		case <-ctx.Done():
			// long-running inputs are the sandbox's timeout's job
		}
	})
}

func TestCatalogMDCurrent(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	want, err := c.Markdown()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/CATALOG.md")
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("docs/CATALOG.md is stale; run: make docs (%v)", err)
	}
}
