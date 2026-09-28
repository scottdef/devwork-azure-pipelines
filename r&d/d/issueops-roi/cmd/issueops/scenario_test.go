package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolEngOrg/issueops/internal/testutil"
)

// TestScenarios runs every examples/*/scenario*.json through the engine
// offline and fails on any `expect` mismatch. The scenarios are the
// end-to-end tests of the platform: intake, commands, approval gating,
// digest-bound approvals, gate, type-specific execution and finish.
func TestScenarios(t *testing.T) {
	root := testutil.Root(t)
	files, err := filepath.Glob(filepath.Join(root, "examples", "*", "scenario*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenarios found: %v", err)
	}
	for _, f := range files {
		f := f
		name := filepath.Base(filepath.Dir(f)) + "/" + strings.TrimSuffix(filepath.Base(f), ".json")
		t.Run(name, func(t *testing.T) {
			c := &common{
				configPath: filepath.Join(root, "config", "issueops.json"),
				formsDir:   filepath.Join(root, "config", "forms"),
				botLogin:   "coolengorg-issueops[bot]",
				outDir:     t.TempDir(),
				offline:    true,
			}
			s, err := runScenario(context.Background(), c, f)
			if err != nil {
				t.Fatal(err)
			}
			for _, fail := range s.failures {
				t.Error(fail)
			}
		})
	}
}

func TestOpsMetricsFromSimulatedTimelines(t *testing.T) {
	root := testutil.Root(t)
	c := &common{configPath: filepath.Join(root, "config", "issueops.json"), formsDir: filepath.Join(root, "config", "forms"),
		botLogin: "coolengorg-issueops[bot]", outDir: t.TempDir(), offline: true}
	s, err := runScenario(context.Background(), c, filepath.Join(root, "examples", "copilot-budget-request", "scenario.json"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.load()
	if err != nil {
		t.Fatal(err)
	}
	a.now = s.clock
	text := opsPrometheus(a, []opsIssue{{Issue: *s.issue, Comments: s.comments}})
	for _, want := range []string{
		`issueops_requests{org="CoolEngOrg",type="copilot-budget-request",phase="completed"} 1`,
		`issueops_approval_latency_hours_count{org="CoolEngOrg",type="copilot-budget-request"} 1`,
		`issueops_requests_escalated{org="CoolEngOrg",type="copilot-budget-request"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in:\n%s", want, text)
		}
	}
}
