package oidc_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/oidc"
	"github.com/CoolEngOrg/coolctl/internal/oidctest"
)

const (
	ciRef  = "CoolEngOrg/cool-actions/.github/workflows/cool-deploy.yml@refs/tags/v1"
	opsRef = "CoolEngOrg/issueops/.github/workflows/execute-cool-change.yml@refs/heads/main"
)

func setup(t *testing.T) (*oidctest.Issuer, *oidc.Verifier) {
	t.Helper()
	iss, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(iss.Handler())
	t.Cleanup(srv.Close)
	iss.URL = srv.URL
	return iss, &oidc.Verifier{Issuer: srv.URL, Audience: "cool-api"}
}

func TestVerify(t *testing.T) {
	iss, v := setup(t)
	ctx := context.Background()

	tok, _ := iss.Token(map[string]any{"aud": "cool-api", "repository": "CoolEngOrg/checkout", "job_workflow_ref": ciRef}, time.Minute)
	c, err := v.Verify(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.Repository != "CoolEngOrg/checkout" || c.JobWorkflowRef != ciRef {
		t.Errorf("claims = %+v", c)
	}

	bad := map[string]string{}
	bad["audience"], _ = iss.Token(map[string]any{"aud": "someone-else"}, time.Minute)
	bad["expired"], _ = iss.Token(map[string]any{"aud": "cool-api"}, -time.Hour)
	bad["issuer"], _ = iss.Token(map[string]any{"aud": "cool-api", "iss": "https://evil.example"}, time.Minute)
	bad["tampered"] = tok[:len(tok)-4] + "AAAA"
	bad["not a jwt"] = "abc"
	// Same claims, signed by a different issuer's key.
	other, _ := oidctest.New()
	other.URL = iss.URL
	bad["foreign key"], _ = other.Token(map[string]any{"aud": "cool-api"}, time.Minute)

	for name, tok := range bad {
		if _, err := v.Verify(ctx, tok); err == nil {
			t.Errorf("%s: token was accepted", name)
		}
	}
}

func TestVerifyAudienceList(t *testing.T) {
	iss, v := setup(t)
	tok, _ := iss.Token(map[string]any{"aud": []string{"other", "cool-api"}}, time.Minute)
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorize(t *testing.T) {
	p := oidc.DefaultPolicy
	const repo = "CoolEngOrg/checkout"
	ci := oidc.Claims{Repository: repo, JobWorkflowRef: ciRef}
	ops := func(env string) oidc.Claims {
		return oidc.Claims{Repository: "CoolEngOrg/issueops", JobWorkflowRef: opsRef, Environment: env}
	}
	tests := []struct {
		name string
		c    oidc.Claims
		env  string
		act  oidc.Action
		ok   bool
	}{
		{"ci deploys its own app to dev", ci, "dev", oidc.Deploy, true},
		{"ci manages its previews", ci, "dev", oidc.Preview, true},
		{"ci cannot deploy to prd", ci, "prd", oidc.Deploy, false},
		{"ci cannot promote", ci, "dev", oidc.Promote, false},
		{"ci cannot deploy another repo's app", oidc.Claims{Repository: "CoolEngOrg/other", JobWorkflowRef: ciRef}, "dev", oidc.Deploy, false},
		{"issueops promotes to prd", ops("cool-prd"), "prd", oidc.Promote, true},
		{"issueops rolls back prd", ops("cool-prd"), "prd", oidc.Rollback, true},
		{"issueops with the tst environment cannot touch prd", ops("cool-tst"), "prd", oidc.Promote, false},
		{"issueops with no environment is refused", ops(""), "prd", oidc.Promote, false},
		{"a branch of cool-actions is not trusted",
			oidc.Claims{Repository: repo, JobWorkflowRef: strings.Replace(ciRef, "refs/tags/v1", "refs/heads/evil", 1)}, "dev", oidc.Deploy, false},
		{"an app repo's own workflow is not trusted",
			oidc.Claims{Repository: repo, JobWorkflowRef: repo + "/.github/workflows/cool.yml@refs/heads/main"}, "dev", oidc.Deploy, false},
		{"a look-alike issueops workflow name is not trusted",
			oidc.Claims{Repository: "CoolEngOrg/issueops", JobWorkflowRef: "CoolEngOrg/issueops/.github/workflows/ci.yml@refs/heads/main", Environment: "cool-prd"}, "prd", oidc.Promote, false},
		{"issueops jobs must come from the issueops repo",
			oidc.Claims{Repository: repo, JobWorkflowRef: opsRef, Environment: "cool-prd"}, "prd", oidc.Promote, false},
	}
	for _, tt := range tests {
		err := p.Authorize(tt.c, repo, tt.env, tt.act)
		if (err == nil) != tt.ok {
			t.Errorf("%s: err = %v, want ok = %v", tt.name, err, tt.ok)
		}
	}
}
