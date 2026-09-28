package agent

import (
	"strings"
	"testing"

	"github.com/CoolEngOrg/issueops/internal/qa"
	"github.com/CoolEngOrg/issueops/internal/tmpl"
)

func TestResultRoundTrip(t *testing.T) {
	in := &Result{Status: "needs_input", Iterations: 1, Model: "gpt-4.1-agents", Questions: []qa.Question{{ID: "D1", Key: "k", Text: "?"}}}
	log := "noise\n" + EncodeResult(&Result{Status: "failed"}) + "more noise\n" + EncodeResult(in) + "trailer\n"
	out, err := ParseResult(log)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "needs_input" || len(out.Questions) != 1 {
		t.Fatalf("last block must win: %+v", out)
	}
	if _, err := ParseResult("no block"); err == nil {
		t.Fatal("expected an error without a result block")
	}
	bad := ResultBegin + "\n" + "eyJzdGF0dXMiOiJwd25lZCJ9" + "\n" + ResultEnd // {"status":"pwned"}
	if _, err := ParseResult(bad); err == nil {
		t.Fatal("unknown status must be rejected")
	}
}

func TestLooksLikeSecret(t *testing.T) {
	for _, s := range []string{"token ghp_abcdefghijklmnopqrstuvwxyz0123", "AKIAABCDEFGHIJKLMNOP", "-----BEGIN RSA PRIVATE KEY-----", "DefaultEndpointsProtocol=https;AccountKey=abcdefghijklmnopqrstuvwxyz0123456789=="} {
		if !LooksLikeSecret(s) {
			t.Errorf("not detected: %q", s)
		}
	}
	if LooksLikeSecret("rotate the GitHub App private key every 90 days") {
		t.Error("false positive on prose")
	}
}

func TestRenderAKSRejectsUnapprovedDeployments(t *testing.T) {
	set := tmpl.MustLoad()
	be := Backend{Namespace: "issueops-agents", Image: "img:1", ServiceAccount: "sa", FoundryEndpoint: "https://x.openai.azure.com",
		AllowedDeployments: []string{"gpt-4.1-agents", "o4-mini-agents"}, ConfidentialDeployments: []string{"gpt-4.1-agents"}, MaxIterations: 4, TimeoutSeconds: 900}
	sp := &Spec{RequestRef: "CoolEngOrg/issueops#7", Title: "t", Objective: "o", Classification: "confidential", Digest: "sha256:0123456789abcdef",
		Answers: map[string]string{"model_deployment": "o4-mini-agents", "deliverable_type": "runbook", "audience": "sre"}}
	if _, _, err := RenderAKS(set, sp, be, 7); err == nil {
		t.Fatal("confidential data on a non-confidential deployment must be refused")
	}
	sp.Answers["model_deployment"] = "gpt-5-unapproved"
	if _, _, err := RenderAKS(set, sp, be, 7); err == nil {
		t.Fatal("unapproved deployment must be refused")
	}
	sp.Answers["model_deployment"] = "gpt-4.1-agents"
	sp.Answers["max_iterations"] = "99"
	sp.Title = "evil\"\n  namespace: kube-system"
	out, jd, err := RenderAKS(set, sp, be, 7)
	if err != nil {
		t.Fatal(err)
	}
	if jd.JobName != "issueops-agent-7-01234567" || jd.MaxIterations != 4 || jd.RunnerTimeoutSeconds != 810 {
		t.Fatalf("job data = %+v", jd)
	}
	if strings.Contains(out, "\n  namespace: kube-system") {
		t.Fatal("user text escaped into YAML structure")
	}
	for _, want := range []string{"kind: Job", "kind: ConfigMap", "activeDeadlineSeconds: 900", `value: "810"`, "runAsNonRoot: true", "azure.workload.identity/use: \"true\"", "readOnlyRootFilesystem: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("manifest lacks %q", want)
		}
	}
}
