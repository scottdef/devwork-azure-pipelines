package site

import (
	"os/exec"
	"strings"
	"testing"
)

// TestIntakeJS runs static/intake.js under node with a tiny DOM stub
// (testdata/js/intake_test.js) to check the browser half: prefill URL,
// JSON check, pattern check and the URL length guard. Skipped without node.
func TestIntakeJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "../../testdata/js/intake_test.js", "static/intake.js").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		"OK    https://github.com/CoolGitOrg/issueops-intake/issues/new?template=request.yml&title=%5Brequest%5D%20payments-api%20caddyfile&source-id=payments-api&payload=%7B%22name%22%3A%22payments-api%22%2C%22port%22%3A8443%7D",
		"JSON  out bad | payload: JSON does not parse",
		"ARR   out bad | payload: must be a JSON object",
		"PAT   out bad | source-id: does not match",
		"LONG  out bad | The prefilled link would be 8181 characters (limit 7000). | navigated: false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
