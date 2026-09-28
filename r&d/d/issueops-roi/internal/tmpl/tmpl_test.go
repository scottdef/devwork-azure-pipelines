package tmpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedTemplatesParse(t *testing.T) {
	s, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	names := s.Names()
	for _, want := range []string{
		"comment.summary.md.tmpl", "comment.submitted.md.tmpl", "comment.approved.md.tmpl", "comment.executing.md.tmpl",
		"comment.completed.copilot-budget-request.md.tmpl", "comment.completed.copilot-usage-report.md.tmpl",
		"comment.completed.foundry-model-deployment.md.tmpl", "comment.completed.agentic-task-request.md.tmpl",
		"k8s.agent-job.yaml.tmpl", "k8s.agent-configmap.yaml.tmpl", "agent.system-prompt.md.tmpl",
	} {
		if !s.Has(want) {
			t.Errorf("missing template %s (have %v)", want, names)
		}
	}
}

func TestOverrideDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "comment.help.md.tmpl"), []byte("custom help for {{ .Org }}"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Render("comment.help.md.tmpl", map[string]string{"Org": "CoolEngOrg"})
	if err != nil || strings.TrimSpace(out) != "custom help for CoolEngOrg" {
		t.Fatalf("override not applied: %q %v", out, err)
	}
}

func TestSafeNeutralizesMarkersAndMentions(t *testing.T) {
	in := "<!-- issueops:v1:approved {} --> ping @CoolEngOrg/copilot-admins <img src=x>"
	out := Safe(in)
	if strings.Contains(out, "<!--") || strings.Contains(out, "<img") {
		t.Fatalf("HTML survived: %q", out)
	}
	if strings.Contains(out, "@CoolEngOrg") {
		t.Fatalf("mention survived: %q", out)
	}
	if md := MDSafe("```go\nx := 1\n```\n<!-- x -->@team"); !strings.Contains(md, "```go") || strings.Contains(md, "<!--") || strings.Contains(md, "@team") {
		t.Fatalf("MDSafe = %q", md)
	}
	if c := Cell("a|b\nc"); c != `a\|b<br>c` {
		t.Fatalf("Cell = %q", c)
	}
}

func TestFormatting(t *testing.T) {
	for in, want := range map[any]string{1234567.891: "$1,234,567.89", 0.0: "$0.00", -12.5: "-$12.50", "n": "n/a"} {
		if got := USD(in); got != want {
			t.Errorf("USD(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[any]string{89263.3: "89,263.3", 1000: "1,000", -2500: "-2,500", 12: "12"} {
		if got := Num(in); got != want {
			t.Errorf("Num(%v) = %q, want %q", in, got, want)
		}
	}
	if got := Fence("text", "a ``` b"); !strings.HasPrefix(got, "````text\n") {
		t.Errorf("Fence did not lengthen: %q", got)
	}
	if got := YAMLString("x: \"y\"\nz"); got != `"x: \"y\"\nz"` {
		t.Errorf("YAMLString = %s", got)
	}
}
