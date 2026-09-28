package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleInventory = "../../examples/inventory.json"

func exec(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestUsageAndUnknownCommand(t *testing.T) {
	if code, _, errOut := exec(t); code != exitUsage || !strings.Contains(errOut, "Commands:") {
		t.Errorf("no args: code=%d stderr=%q", code, errOut)
	}
	if code, _, _ := exec(t, "bogus"); code != exitUsage {
		t.Errorf("unknown command: code=%d", code)
	}
	if code, out, _ := exec(t, "help"); code != exitOK || !strings.Contains(out, "generate") {
		t.Errorf("help: code=%d", code)
	}
	if code, _, _ := exec(t, "url", "-h"); code != exitOK {
		t.Errorf("url -h: code=%d, want 0", code)
	}
	if code, _, _ := exec(t, "url", "--no-such-flag"); code != exitUsage {
		t.Errorf("bad flag: code=%d, want %d", code, exitUsage)
	}
	if code, _, _ := exec(t, "url", "stray"); code != exitUsage {
		t.Errorf("stray arg: code=%d, want %d", code, exitUsage)
	}
}

func TestURLFormats(t *testing.T) {
	base := []string{"url", "--resource", "svc", "--model", "gpt-5.4", "--model-version", "2026-03-05", "--deployment", "gpt-5.4"}

	code, out, _ := exec(t, append(base, "--format", "json")...)
	if code != exitOK {
		t.Fatalf("json: code=%d", code)
	}
	var got struct {
		ModelID string `json:"model_id"`
		URLs    struct {
			V1Responses string `json:"v1_responses"`
		} `json:"urls"`
		GhAw struct {
			ProviderModelID string `json:"provider_model_id"`
		} `json:"gh_aw"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json output invalid: %v\n%s", err, out)
	}
	if got.ModelID != "gpt-5.4-2026-03-05" || got.GhAw.ProviderModelID != "gpt-5.4" ||
		got.URLs.V1Responses != "https://svc.openai.azure.com/openai/v1/responses" {
		t.Errorf("json content: %+v", got)
	}

	code, out, _ = exec(t, append(base, "--format", "env")...)
	if code != exitOK || !strings.Contains(out, "COPILOT_PROVIDER_BASE_URL=https://svc.openai.azure.com/openai/v1\n") {
		t.Errorf("env: code=%d out=%s", code, out)
	}
	// env output must be sorted and free of empty values (safe for $GITHUB_OUTPUT).
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasSuffix(line, "=") {
			t.Errorf("env output has an empty value: %q", line)
		}
	}

	if code, out, _ = exec(t, base...); code != exitOK || !strings.Contains(out, "COPILOT ENTERPRISE") {
		t.Errorf("text: code=%d", code)
	}
	if code, _, _ = exec(t, append(base, "--format", "xml")...); code != exitUsage {
		t.Errorf("bad format: code=%d", code)
	}
}

func TestValidationErrorIsExit1(t *testing.T) {
	code, _, errOut := exec(t, "url", "--resource", "svc", "--model", "gpt-4o", "--deployment", "has space")
	if code != exitError || !strings.Contains(errOut, "deployment") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestInventoryRejectsSingleDeploymentFlags(t *testing.T) {
	// A silently ignored --runs-on would be a trap; it must be refused.
	code, _, errOut := exec(t, "validate", "--inventory", exampleInventory, "--runs-on", "self-hosted")
	if code != exitError || !strings.Contains(errOut, "defaults") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestValidate(t *testing.T) {
	code, out, _ := exec(t, "validate", "--inventory", exampleInventory)
	if code != exitOK || !strings.Contains(out, "8 deployment(s)") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if code, _, _ = exec(t, "validate", "--inventory", exampleInventory, "--strict"); code != exitError {
		t.Errorf("--strict with warnings: code=%d, want %d", code, exitError)
	}
	code, out, _ = exec(t, "validate", "--inventory", exampleInventory, "--only-env", "dev")
	if code != exitOK || !strings.Contains(out, "3 deployment(s)") {
		t.Errorf("--only-env dev: code=%d out=%q", code, out)
	}
	if code, _, _ = exec(t, "validate", "--inventory", exampleInventory, "--only-env", "nope"); code != exitError {
		t.Errorf("--only-env nope: code=%d", code)
	}
}

func TestGenerateCheckDetectsDrift(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	wf := filepath.Join(t.TempDir(), "workflows")
	args := []string{"generate", "--inventory", exampleInventory, "--out", out, "--workflows-dir", wf}

	// Nothing on disk yet: that is drift.
	if code, _, _ := exec(t, append(args, "--check")...); code != exitDrift {
		t.Fatalf("check before generate: code=%d, want %d", code, exitDrift)
	}
	if code, _, errOut := exec(t, args...); code != exitOK {
		t.Fatalf("generate: code=%d stderr=%s", code, errOut)
	}
	if code, _, _ := exec(t, append(args, "--check")...); code != exitOK {
		t.Fatalf("check after generate: code=%d, want 0", code)
	}

	entries, err := os.ReadDir(wf)
	if err != nil || len(entries) != 8 {
		t.Fatalf("workflows dir: %d entries, err=%v, want 8 flat .md files", len(entries), err)
	}

	// Hand-edit a compiled-from workflow; the check must name it and exit 3.
	target := filepath.Join(wf, entries[0].Name())
	if err := os.WriteFile(target, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := exec(t, append(args, "--check")...)
	if code != exitDrift || !strings.Contains(errOut, entries[0].Name()) {
		t.Errorf("after tamper: code=%d stderr=%q", code, errOut)
	}
}

func TestVerifyWithoutCredentialsFails(t *testing.T) {
	t.Setenv(tokenEnv, "")
	t.Setenv("FOUNDRY_BYOK_TEST_KEY", "")
	code, out, _ := exec(t, "verify", "--resource", "svc", "--model", "gpt-4o", "--deployment", "d",
		"--api-key-secret", "FOUNDRY_BYOK_TEST_KEY")
	if code != exitVerifyFailed || !strings.Contains(out, "no credential available") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

// TestNoSecretFlags guards the design rule that keys are never accepted on the
// command line (they would leak into shell history and process listings).
func TestNoSecretFlags(t *testing.T) {
	for _, cmd := range []string{"url", "generate", "validate", "verify"} {
		_, _, help := exec(t, cmd, "-h")
		for _, banned := range []string{"-api-key ", "-token ", "-bearer", "-password"} {
			if strings.Contains(help, banned) {
				t.Errorf("%s exposes a secret-bearing flag matching %q", cmd, banned)
			}
		}
	}
}

func TestTemplatesCommand(t *testing.T) {
	code, out, _ := exec(t, "templates")
	if code != exitOK || !strings.Contains(out, "gh-aw-smoke.md.tmpl") {
		t.Errorf("list: code=%d out=%q", code, out)
	}
	dir := t.TempDir()
	if code, out, _ = exec(t, "templates", "--export", dir); code != exitOK || strings.Count(out, "\n") != 7 {
		t.Errorf("export: code=%d out=%q", code, out)
	}
}
