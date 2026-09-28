package render

import (
	"bytes"
	"encoding/json"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
	"github.com/CoolGitOrg/foundry-byok/internal/inventory"
)

func exampleBundle(t *testing.T) inventory.Bundle {
	t.Helper()
	f, err := inventory.LoadFile("../../examples/inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	specs, err := f.Specs()
	if err != nil {
		t.Fatal(err)
	}
	b, err := inventory.Build("v-test", specs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func renderExample(t *testing.T) map[string]File {
	t.Helper()
	files, err := Renderer{}.Render(exampleBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]File{}
	for _, f := range files {
		out[f.Path] = f
	}
	return out
}

func TestRenderProducesExpectedTree(t *testing.T) {
	files := renderExample(t)
	// 4 bundle-level files + 8 deployments x 6 files.
	if got, want := len(files), 4+8*6; got != want {
		t.Errorf("rendered %d files, want %d", got, want)
	}
	for _, p := range []string{
		"manifest.json", "SUMMARY.md",
		"copilot-enterprise/custom-models-runbook.md", "copilot-enterprise/custom-models.json",
		"dev/fdy-myass-dev-eus2/gpt-5.4/urls.json",
		"dev/fdy-myass-dev-eus2/gpt-5.4/curl.sh",
		"dev/fdy-myass-dev-eus2/gpt-5.4/client.go",
		"dev/fdy-myass-dev-eus2/gpt-5.4/copilot-cli.env",
		"dev/fdy-myass-dev-eus2/gpt-5.4/gh-aw/frontmatter.yml",
		"dev/fdy-myass-dev-eus2/gpt-5.4/gh-aw/smoke-foundry-dev-fdy-myass-dev-eus2-gpt-5-4.md",
	} {
		if _, ok := files[p]; !ok {
			t.Errorf("missing output %s", p)
		}
	}
	if m := files["dev/fdy-myass-dev-eus2/gpt-5.4/curl.sh"].Mode; m != 0o755 {
		t.Errorf("curl.sh mode = %o, want 755", m)
	}
}

func TestNoTemplateResidue(t *testing.T) {
	for p, f := range renderExample(t) {
		s := string(f.Data)
		if strings.Contains(s, "<no value>") {
			t.Errorf("%s contains <no value>", p)
		}
		// "{{" is only legitimate as part of a GitHub Actions ${{ expression.
		if n := strings.Count(s, "{{") - strings.Count(s, "${{"); n != 0 {
			t.Errorf("%s has %d unrendered template action(s)", p, n)
		}
	}
}

// TestFrontmatterMatchesGhAwReference renders the exact scenario from
// github.github.com/gh-aw/reference/azure-openai-byok and checks every
// documented key/value is present.
func TestFrontmatterMatchesGhAwReference(t *testing.T) {
	b, err := inventory.Build("v-test", []foundry.Spec{{
		Resource: "RESOURCE", Model: "gpt-5.4", ModelVersion: "2026-03-05", Deployment: "gpt-5.4",
	}})
	if err != nil {
		t.Fatal(err)
	}
	files, err := Renderer{}.Render(b)
	if err != nil {
		t.Fatal(err)
	}
	var fm string
	for _, f := range files {
		if strings.HasSuffix(f.Path, "gh-aw/frontmatter.yml") {
			fm = stripComments(string(f.Data))
		}
	}
	want := `engine:
  id: copilot
  model: "gpt-5.4-2026-03-05"
  env:
    COPILOT_PROVIDER_BASE_URL: "https://resource.openai.azure.com/openai/v1"
    COPILOT_PROVIDER_API_KEY: ${{ secrets.AZURE_OPENAI_API_KEY }}
    COPILOT_PROVIDER_MODEL_ID: "gpt-5.4"
    COPILOT_PROVIDER_WIRE_API: responses
network:
  allowed:
    - defaults
    - resource.openai.azure.com
`
	if fm != want {
		t.Errorf("frontmatter mismatch\n--- got ---\n%s--- want ---\n%s", fm, want)
	}
}

func TestEntraWorkflow(t *testing.T) {
	files := renderExample(t)
	wf := string(files["prod/fdy-myass-prod-eus2/gpt-5.4-prod/gh-aw/smoke-foundry-prod-fdy-myass-prod-eus2-gpt-5-4-prod.md"].Data)
	for _, frag := range []string{
		"  id-token: write\n",
		"    type: github-oidc\n    provider: azure\n",
		`    azure-tenant-id: "11111111-2222-3333-4444-555555555555"`,
		"    - login.microsoftonline.com\n",
		"    model-fallback: false\n",
		`runs-on: ["self-hosted", "linux", "x64"]`,
		`COPILOT_PROVIDER_MODEL_ID: "gpt-5.4-prod"`,
	} {
		if !strings.Contains(wf, frag) {
			t.Errorf("entra workflow missing %q\n%s", frag, wf)
		}
	}
	if strings.Contains(wf, "COPILOT_PROVIDER_API_KEY") {
		t.Error("entra workflow must not set COPILOT_PROVIDER_API_KEY")
	}
	// Frontmatter must open and close, with a body that mandates noop.
	if !strings.HasPrefix(wf, "---\n") || strings.Count(wf, "\n---\n") != 1 {
		t.Error("workflow frontmatter delimiters are malformed")
	}
	if !strings.Contains(wf, "`noop`") {
		t.Error("smoke body must instruct the agent to call noop")
	}
}

// TestWarningsAreScoped: an enterprise-form caveat must not clutter a gh-aw
// workflow, but it must reach the summary.
func TestWarningsAreScoped(t *testing.T) {
	files := renderExample(t)
	wf := string(files["prod/fdy-myass-prod-eus2/gpt-5.4-prod/gh-aw/smoke-foundry-prod-fdy-myass-prod-eus2-gpt-5-4-prod.md"].Data)
	if strings.Contains(wf, "custom-models form") {
		t.Error("enterprise-scoped warning leaked into a gh-aw workflow")
	}
	claude := string(files["uat/fdy-myass-uat-eus2/claude-opus-4-5/gh-aw/frontmatter.yml"].Data)
	if !strings.Contains(claude, "# WARNING:") || !strings.Contains(claude, "untested") {
		t.Error("gh-aw-scoped warning missing from the Claude frontmatter")
	}
	if summary := string(files["SUMMARY.md"].Data); !strings.Contains(summary, "*enterprise:*") {
		t.Error("summary must list enterprise-scoped warnings")
	}
}

func TestGeneratedGoIsValidAndFormatted(t *testing.T) {
	kinds := map[string]bool{}
	for p, f := range renderExample(t) {
		if f.Kind != KindCode {
			continue
		}
		if _, err := parser.ParseFile(token.NewFileSet(), p, f.Data, parser.AllErrors); err != nil {
			t.Errorf("%s does not parse: %v", p, err)
			continue
		}
		formatted, err := format.Source(f.Data)
		if err != nil {
			t.Errorf("%s: gofmt: %v", p, err)
			continue
		}
		if !bytes.Equal(formatted, f.Data) {
			t.Errorf("%s is not gofmt-clean", p)
		}
		switch s := string(f.Data); {
		case strings.Contains(s, `"anthropic-version"`):
			kinds["anthropic"] = true
		case strings.Contains(s, "output_text"):
			kinds["responses"] = true
		default:
			kinds["chat"] = true
		}
	}
	for _, k := range []string{"chat", "responses", "anthropic"} {
		if !kinds[k] {
			t.Errorf("example inventory did not exercise the %s client variant", k)
		}
	}
}

func TestGeneratedShellParses(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	for p, f := range renderExample(t) {
		if f.Kind != KindScript && !strings.HasSuffix(p, ".env") {
			continue
		}
		cmd := exec.Command(bash, "-n")
		cmd.Stdin = bytes.NewReader(f.Data)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: bash -n: %v\n%s", p, err, out)
		}
	}
}

func TestJSONOutputsAreValidAndUnescaped(t *testing.T) {
	for p, f := range renderExample(t) {
		if !strings.HasSuffix(p, ".json") {
			continue
		}
		if !json.Valid(f.Data) {
			t.Errorf("%s is not valid JSON", p)
		}
		if bytes.Contains(f.Data, []byte(`\u0026`)) {
			t.Errorf("%s has HTML-escaped '&'; URLs must stay copy-pasteable", p)
		}
	}
}

// TestToolVersionOnlyInManifest: upgrading the tool must not rewrite every
// generated file, or drift checks become noise.
func TestToolVersionOnlyInManifest(t *testing.T) {
	for p, f := range renderExample(t) {
		has := bytes.Contains(f.Data, []byte("v-test"))
		if p == "manifest.json" && !has {
			t.Error("manifest.json must record the tool version")
		}
		if p != "manifest.json" && has {
			t.Errorf("%s embeds the tool version", p)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	a, b := renderExample(t), renderExample(t)
	for p, fa := range a {
		if !bytes.Equal(fa.Data, b[p].Data) {
			t.Errorf("%s differs between two renders", p)
		}
	}
}

func TestOverrideDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.md.tmpl"), []byte("custom {{len .Items}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Renderer{OverrideDir: dir}.Render(exampleBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	var summary, runbook string
	for _, f := range files {
		switch f.Path {
		case "SUMMARY.md":
			summary = string(f.Data)
		case "copilot-enterprise/custom-models-runbook.md":
			runbook = string(f.Data)
		}
	}
	if summary != "custom 8\n" {
		t.Errorf("override not applied: %q", summary)
	}
	if !strings.Contains(runbook, "Microsoft Foundry") {
		t.Error("non-overridden template did not fall back to the embedded copy")
	}

	// A broken override must fail loudly, never fall back silently.
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.md.tmpl"), []byte("{{.Nope}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Renderer{OverrideDir: dir}).Render(exampleBundle(t)); err == nil {
		t.Error("expected an error for a template referencing a missing field")
	}
}

func TestWriteAndDiff(t *testing.T) {
	files, err := Renderer{}.Render(exampleBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()

	changed, err := Diff(root, files)
	if err != nil || len(changed) != len(files) {
		t.Fatalf("empty root: changed=%d err=%v, want all %d missing", len(changed), err, len(files))
	}
	if err := Write(root, files); err != nil {
		t.Fatal(err)
	}
	if changed, err = Diff(root, files); err != nil || len(changed) != 0 {
		t.Fatalf("after write: changed=%v err=%v", changed, err)
	}
	info, err := os.Stat(filepath.Join(root, "dev", "fdy-myass-dev-eus2", "gpt-5.4", "curl.sh"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("curl.sh on disk: %v mode=%v", err, info.Mode())
	}

	// Simulate a hand edit; drift must be reported.
	if err := os.WriteFile(filepath.Join(root, "SUMMARY.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, _ = Diff(root, files); len(changed) != 1 || changed[0] != "SUMMARY.md" {
		t.Errorf("drift = %v, want [SUMMARY.md]", changed)
	}
	// No temp files may be left behind.
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if strings.HasPrefix(d.Name(), ".foundry-byok-") {
			t.Errorf("leftover temp file %s", p)
		}
		return nil
	})
}

func TestWriteRefusesTraversal(t *testing.T) {
	err := Write(t.TempDir(), []File{{Path: "../escape.txt", Mode: 0o644, Data: []byte("x")}})
	if err == nil {
		t.Error("expected a traversal error")
	}
}

func TestExportTemplates(t *testing.T) {
	dir := t.TempDir()
	written, err := ExportTemplates(dir)
	if err != nil || len(written) != len(TemplateNames()) || len(written) == 0 {
		t.Fatalf("written=%d names=%d err=%v", len(written), len(TemplateNames()), err)
	}
	again, err := ExportTemplates(dir)
	if err != nil || len(again) != 0 {
		t.Errorf("second export overwrote files: %v %v", again, err)
	}
}

func TestQuotingFuncs(t *testing.T) {
	shq := funcs["shq"].(func(any) string)
	if got := shq("it's"); got != `'it'\''s'` {
		t.Errorf("shq = %s", got)
	}
	yamlv := funcs["yamlv"].(func(any) string)
	if got := yamlv("${{ vars.AZURE_TENANT_ID }}"); got != "${{ vars.AZURE_TENANT_ID }}" {
		t.Errorf("yamlv expr = %s", got)
	}
	if got := yamlv("5.4"); got != `"5.4"` {
		t.Errorf("yamlv scalar = %s", got)
	}
	runsOn := funcs["runsOn"].(func(string) string)
	if got := runsOn("self-hosted"); got != `"self-hosted"` {
		t.Errorf("runsOn single = %s", got)
	}
}

// stripComments drops YAML comment lines so golden text stays readable.
func stripComments(s string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			b.WriteString(line)
		}
	}
	return b.String()
}
