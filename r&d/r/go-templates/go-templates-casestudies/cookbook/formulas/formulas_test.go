package formulas

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "write testdata/out/<id>.txt")

var outDir = filepath.Join("..", "testdata", "out")

// TestAll is the point of the package: every formula renders to exactly its
// Want, and to exactly the committed file in testdata/out.
func TestAll(t *testing.T) {
	for _, f := range All {
		t.Run(f.ID, func(t *testing.T) {
			got, err := Render(f)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != f.Want {
				t.Fatalf("output differs from Want\n--- got\n%s\n--- want\n%s\n--- got %%q\n%q\n--- want %%q\n%q", got, f.Want, got, f.Want)
			}
			path := filepath.Join(outDir, f.ID+".txt")
			if *update {
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			disk, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run: go test ./formulas -update)", err)
			}
			if string(disk) != got {
				t.Fatalf("%s is stale (run: go test ./formulas -update)", path)
			}
		})
	}
}

func TestTable(t *testing.T) {
	if len(All) < 40 {
		t.Fatalf("%d formulas, want at least 40", len(All))
	}
	seen := map[string]bool{}
	for i, f := range All {
		if want := fmt.Sprintf("F%02d", i+1); f.ID != want {
			t.Errorf("position %d has ID %s, want %s", i+1, f.ID, want)
		}
		if seen[f.ID] {
			t.Errorf("duplicate %s", f.ID)
		}
		seen[f.ID] = true
		for name, v := range map[string]string{"Name": f.Name, "Problem": f.Problem, "Template": f.Template, "Notes": f.Notes} {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s is empty", f.ID, name)
			}
		}
		if f.Engine != "" && f.Engine != "text" && f.Engine != "html" {
			t.Errorf("%s: unknown engine %q", f.ID, f.Engine)
		}
		if f.Want == "" && f.ID != "" {
			t.Errorf("%s: Want is empty; every formula prints something", f.ID)
		}
		if (f.Delims[0] == "") != (f.Delims[1] == "") {
			t.Errorf("%s: set both delimiters or neither", f.ID)
		}
	}
	if got, ok := Lookup("F07"); !ok || got.ID != "F07" {
		t.Errorf("Lookup(F07) = %v, %v", got.ID, ok)
	}
	if _, ok := Lookup("F99"); ok {
		t.Error("Lookup(F99) found something")
	}
}

// TestGlue: every GlueRef resolved to real source, and the source is the
// declaration it names.
func TestGlue(t *testing.T) {
	for _, f := range All {
		if f.GlueRef == "" {
			if f.Run != nil || f.PostProcess != nil || len(f.Funcs) > 0 {
				t.Errorf("%s uses Go glue but has no GlueRef", f.ID)
			}
			continue
		}
		if f.Glue == "" || strings.HasPrefix(f.Glue, "// glue.go") {
			t.Errorf("%s: glue did not resolve: %q", f.ID, f.Glue)
			continue
		}
		for _, ref := range strings.Split(f.GlueRef, ",") {
			name := ref[strings.LastIndex(ref, ".")+1:]
			if !strings.Contains(f.Glue, name) {
				t.Errorf("%s: glue lacks %s", f.ID, ref)
			}
		}
	}
	if _, err := glueSource("noSuchThing"); err == nil {
		t.Error("glueSource accepted an unknown name")
	}
	src, err := glueSource("Item.HumanSize")
	if err != nil || !strings.HasPrefix(src, "// HumanSize formats") || !strings.HasSuffix(src, "}") {
		t.Errorf("method extraction: %q, %v", src, err)
	}
}

func TestDeterministic(t *testing.T) {
	for _, f := range All {
		a, err1 := Render(f)
		b, err2 := Render(f)
		if err1 != nil || err2 != nil || a != b {
			t.Errorf("%s: renders differ or fail: %v %v", f.ID, err1, err2)
		}
	}
}

func TestDataIsJSON(t *testing.T) {
	for _, f := range All {
		if f.Data == nil {
			continue
		}
		j, err := f.DataJSON()
		if err != nil || !json.Valid([]byte(j)) {
			t.Errorf("%s: data is not JSON: %v", f.ID, err)
		}
	}
}

func rendered(t *testing.T, id string) string {
	t.Helper()
	f, ok := Lookup(id)
	if !ok {
		t.Fatalf("no %s", id)
	}
	got, err := Render(f)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return got
}

func TestF15AndF16ValidJSON(t *testing.T) {
	lines := strings.Split(rendered(t, "F15"), "\n")
	for _, l := range lines[:2] { // loop, whole
		doc := strings.TrimSpace(l[strings.Index(l, ":")+1:])
		var v []string
		if err := json.Unmarshal([]byte(doc), &v); err != nil || len(v) != 3 {
			t.Errorf("%q: %v %v", doc, v, err)
		}
	}
	var bad []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(lines[2][strings.Index(lines[2], ":")+1:])), &bad); err == nil {
		t.Error("the printf-q line should not be valid JSON; that is the point of F15")
	}
	out := rendered(t, "F16")
	var m map[string]string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.Split(out, "\n")[0], "compact: ")), &m); err != nil || len(m) != 3 {
		t.Errorf("compact line: %v %v", m, err)
	}
	byHand := out[strings.Index(out, "by hand: ")+len("by hand: "):]
	if err := json.Unmarshal([]byte(byHand), &m); err != nil {
		t.Errorf("hand-built JSON: %v", err)
	}
}

func TestF27EntrypointIsJSONArray(t *testing.T) {
	for _, l := range strings.Split(rendered(t, "F27"), "\n") {
		if arg, ok := strings.CutPrefix(l, "ENTRYPOINT "); ok {
			var v []string
			if err := json.Unmarshal([]byte(arg), &v); err != nil || len(v) != 5 {
				t.Errorf("ENTRYPOINT %q: %v", arg, err)
			}
			return
		}
	}
	t.Error("no ENTRYPOINT line")
}

func TestMakeAndMkRecipesUseTabs(t *testing.T) {
	for _, id := range []string{"F28", "F29"} {
		recipes := 0
		for _, l := range strings.Split(rendered(t, id), "\n") {
			if strings.HasPrefix(l, " ") {
				t.Errorf("%s: line starts with a space: %q", id, l)
			}
			if strings.HasPrefix(l, "\t") {
				recipes++
			}
		}
		if recipes == 0 {
			t.Errorf("%s: no tab-indented recipe lines", id)
		}
	}
}

func TestShellOutputParses(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	cmd := exec.Command(bash, "-n")
	cmd.Stdin = strings.NewReader(rendered(t, "F30"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

func TestShellQuotingRoundTrip(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	script := rendered(t, "F30")
	script = strings.Replace(script, "mkdir -p -- \"$dest\"", "", 1)
	script = script[:strings.Index(script, "printf")] + `printf '%s|%s' "$name" "$dest"` + "\n"
	out, err := exec.Command(bash, "-c", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "it's \"quoted\"; rm -rf /|/tmp/my dir"; string(out) != want {
		t.Fatalf("shell saw %q, want %q", out, want)
	}
}

func TestGeneratedGoParses(t *testing.T) {
	if _, err := parser.ParseFile(token.NewFileSet(), "zone.go", rendered(t, "F32"), parser.AllErrors); err != nil {
		t.Fatal(err)
	}
}

func TestEscapingActuallyEscapes(t *testing.T) {
	out := rendered(t, "F48")
	for _, bad := range []string{"<b>", `javascript:alert`, "url(//evil)"} {
		if strings.Contains(out, bad) {
			t.Errorf("F48 output contains %q", bad)
		}
	}
	if n := strings.Count(out, "ZgotmplZ"); n != 2 {
		t.Errorf("want 2 ZgotmplZ, got %d", n)
	}
	email := rendered(t, "F35")
	if !strings.Contains(email, "&lt;100&gt;") || !strings.Contains(email, "<100>") {
		t.Error("F35 should escape in the HTML part and not in the text part")
	}
	if strings.Contains(rendered(t, "F42"), "</script><b>") {
		t.Error("F42 let </script> through")
	}
}

func TestF54LinesAgree(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(rendered(t, "F54")), "\n")
	if len(lines) != 2 || lines[0] != lines[1] {
		t.Fatalf("the two spellings differ: %q", lines)
	}
}

func TestF42ViaMapIsJSON(t *testing.T) {
	out := rendered(t, "F42")
	line, _, _ := strings.Cut(out[strings.Index(out, "var viaMap = ")+len("var viaMap = "):], ";\n")
	var v map[string]any
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		t.Fatalf("viaMap: %v", err)
	}
}

func TestOptionsAreFixed(t *testing.T) {
	o := Options()
	if !o.Now().Equal(FixedNow) || o.Env("APP_ENV") != "staging" || o.Env("SECRET") != "" || o.FS == nil {
		t.Fatal("Options is not the fixed world")
	}
}

func BenchmarkAll(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, f := range All {
			if _, err := Render(f); err != nil {
				b.Fatal(err)
			}
		}
	}
}
