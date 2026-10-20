package scaffold

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, rel string, got []byte) {
	t.Helper()
	p := filepath.Join("../../testdata/golden/scaffold", rel)
	if *update {
		os.MkdirAll(filepath.Dir(p), 0o755)
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
		t.Errorf("%s differs from golden:\n%s", rel, forge.Unified("golden", "got", forge.Diff(forge.Lines(string(want)), forge.Lines(string(got))), 2))
	}
}

const fieldSpec = "repo_name:string!,visibility:dropdown=internal|private,team_size:int,archived:bool,reason:text"

func TestParseFields(t *testing.T) {
	fs, err := ParseFields(fieldSpec)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 5 || !fs[0].Required || fs[1].Type != "dropdown" || len(fs[1].Options) != 2 || fs[0].Label != "Repo name" {
		t.Errorf("%+v", fs)
	}
	for _, bad := range []string{"a:nope", "a:dropdown", "a:string=x", "1a:string", "a,a", "a b:string"} {
		if _, err := ParseFields(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestIssueOpsGolden(t *testing.T) {
	fs, err := ParseFields(fieldSpec)
	if err != nil {
		t.Fatal(err)
	}
	files, err := IssueOps("repo-request", fs)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		golden(t, filepath.Join("issueops", f.Name), f.Data)
	}
	if _, err := IssueOps("Bad Name", fs); err == nil {
		t.Error("bad name accepted")
	}
	if _, err := IssueOps("x", nil); err == nil {
		t.Error("no fields accepted")
	}
	many := make([]Field, MaxPayloadKeys+1)
	for i := range many {
		many[i] = Field{ID: fmt.Sprintf("f%d", i), Label: "F", Type: "string"}
	}
	if _, err := IssueOps("x", many); err == nil {
		t.Error("11 fields accepted")
	}
}

// TestIssueOpsParserRuns builds the generated parser and feeds it the body
// GitHub produces for an issue form.
func TestIssueOpsParserRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	fs, _ := ParseFields(fieldSpec)
	files, err := IssueOps("repo-request", fs)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, f.Name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, f.Data, 0o644)
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.24\n"), 0o644)
	run := func(body string) (string, string, error) {
		cmd := exec.Command("go", "run", "./cmd/repo-request-parse", "-dispatch", "repo-request")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
		cmd.Stdin = strings.NewReader(body)
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		return o.String(), e.String(), err
	}
	out, stderr, err := run("### Repo name\r\n\r\nforge\r\n\r\n### Visibility\r\n\r\nprivate\r\n\r\n### Team size\r\n\r\n4\r\n\r\n### Archived\r\n\r\n- [X] Yes\r\n\r\n### Reason\r\n\r\n_No response_\r\n")
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	for _, want := range []string{`"event_type": "repo-request"`, `"repo_name": "forge"`, `"team_size": 4`, `"visibility": "private"`, `"archived": false`} {
		if !strings.Contains(out, want) && !(want == `"archived": false` && strings.Contains(out, `"archived": true`)) {
			t.Errorf("output lacks %s:\n%s", want, out)
		}
	}
	_, stderr, err = run("### Visibility\n\npublic\n\n### Team size\n\nmany\n")
	if err == nil {
		t.Fatal("invalid issue accepted")
	}
	for _, want := range []string{"**Repo name** is required", `"public" is not one of`, `"many" is not a whole number`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
}

func TestWorkflowGolden(t *testing.T) {
	inputs, err := ParseInputs("environment:choice!=beta|prod,dry_run:boolean=true,replicas:number=2,note:string")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range Kinds {
		c := CICD{Kind: k, Binary: "tmplforge", Inputs: inputs}
		if k == "repository_dispatch" {
			c.Types = []string{"tmplforge-render", "tmplforge-release"}
		}
		if k == "release" || k == "build" {
			c.Inputs = nil
		}
		f, err := Workflow(c)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if strings.Contains(string(f.Data), "[[") {
			t.Errorf("%s: unexpanded [[ in output", k)
		}
		if !strings.Contains(string(f.Data), "${{") {
			t.Errorf("%s: no ${{ }} expression survived", k)
		}
		// Untrusted values must never be interpolated into run: scripts.
		for _, line := range strings.Split(string(f.Data), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "run:") && strings.Contains(l, "${{") {
				t.Errorf("%s: expression in run: %s", k, l)
			}
		}
		golden(t, filepath.Join("cicd", f.Name), f.Data)
	}
}

func TestWorkflowLimits(t *testing.T) {
	var spec []string
	for i := 0; i < MaxDispatchInputs+1; i++ {
		spec = append(spec, fmt.Sprintf("in%d:string", i))
	}
	if _, err := ParseInputs(strings.Join(spec, ",")); err == nil {
		t.Error("26 inputs accepted")
	}
	in, err := ParseInputs(strings.Join(spec[:MaxDispatchInputs], ","))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Workflow(CICD{Kind: "dispatch", Inputs: in}); err != nil {
		t.Errorf("25 inputs rejected: %v", err)
	}
	// tag adds a version input: 25 + 1 is over
	if _, err := Workflow(CICD{Kind: "tag", Inputs: in}); err == nil {
		t.Error("tag with 26 inputs accepted")
	}
	if _, err := Workflow(CICD{Kind: "repository_dispatch", Inputs: in[:MaxPayloadKeys+1]}); err == nil {
		t.Error("11 payload keys accepted")
	}
	for _, c := range []CICD{{Kind: "nope"}, {Kind: "build", Name: "Bad Name"}, {Kind: "build", Binary: "a b"}, {Kind: "build", Targets: []string{"linux"}}} {
		if _, err := Workflow(c); err == nil {
			t.Errorf("%+v accepted", c)
		}
	}
	for _, bad := range []string{"a:choice", "a:nope", "a:string,a:string", "$x:string"} {
		if _, err := ParseInputs(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestVarConversions(t *testing.T) {
	vars := []Var{{"name", "string", "x"}, {"n", "int", "3"}, {"on", "bool", "true"}, {"l", "list", "a, b"}, {"lj", "list", `[1,"x"]`}, {"m", "map", `{"k":2}`}}
	for _, v := range vars {
		if err := ValidVar(v); err != nil {
			t.Errorf("%+v: %v", v, err)
		}
	}
	d := Merge(vars, map[string]any{"name": "override"})
	if d["name"] != "override" || d["n"] != 3 || d["on"] != true || len(d["l"].([]any)) != 2 || d["lj"].([]any)[0] != 1 || d["m"].(map[string]any)["k"] != 2 {
		t.Errorf("%#v", d)
	}
	for _, bad := range []Var{{"1x", "string", ""}, {"x", "float", ""}, {"x", "int", "q"}, {"x", "map", "[1]"}} {
		if ValidVar(bad) == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	in := InputsFromVars(vars)
	if in[1].Type != "number" || in[2].Type != "boolean" || in[3].Type != "string" {
		t.Errorf("%+v", in)
	}
	fs := FieldsFromVars(vars)
	if fs[1].Type != "int" || fs[3].Type != "text" {
		t.Errorf("%+v", fs)
	}
}
