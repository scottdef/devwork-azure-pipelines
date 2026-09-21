package azure_test

import (
	"context"
	"strings"
	"testing"

	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/azure/azuretest"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
)

func TestReads(t *testing.T) {
	ctx := context.Background()
	fake := azuretest.Fake()
	cfg := config.Default()
	cfg.Subscription = "sub-1"
	c := azure.Client{R: fake, Cfg: cfg}

	acct, err := c.Account(ctx)
	if err != nil || acct.ID != azuretest.AccountID {
		t.Fatalf("Account = %+v, %v", acct, err)
	}
	if got := acct.OpenAIBase(); got != "https://ais-coolgit-copilot-prod.openai.azure.com" {
		t.Errorf("OpenAIBase = %s", got)
	}

	ds, err := c.Deployments(ctx)
	if err != nil || len(ds) != 3 {
		t.Fatalf("Deployments = %d, %v", len(ds), err)
	}
	if ds[0].Name != "claude-sonnet" || !ds[0].Chat() {
		t.Errorf("deployments are not sorted, or chat detection failed: %+v", ds[0].Name)
	}
	if ds[1].Chat() { // embed-small
		t.Error("an embeddings deployment was taken for a chat model")
	}
	if ds[2].RateLimit("token") != 50000 || ds[2].SystemData.LastModifiedBy != "uami-foundry-deploy" {
		t.Errorf("gpt-4o parsed wrong: %+v", ds[2])
	}

	us, err := c.Usage(ctx)
	if err != nil || len(us) != 2 || us[0].Name.Value != "OpenAI.Standard.text-embedding-3-small" {
		t.Fatalf("Usage should drop idle lines and sort by share: %+v, %v", us, err)
	}

	ms, _ := c.Models(ctx)
	if _, ok := ms[2].Retires(); !ok || ms[2].Name != "gpt-4o" {
		t.Errorf("retirement date not parsed: %+v", ms[2])
	}

	tok, err := c.Token(ctx)
	if err != nil || tok != "SECRET-TOKEN" {
		t.Fatalf("Token = %q, %v", tok, err)
	}

	// Every call is a read, scoped to the configured subscription.
	for _, call := range fake.Calls() {
		line := call.String()
		if !strings.Contains(line, "--subscription sub-1") {
			t.Errorf("unscoped call: %s", line)
		}
		for _, verb := range []string{" create", " delete", " update", " set ", " purge", " regenerate"} {
			if strings.Contains(line, verb) {
				t.Errorf("package azure issued a write: %s", line)
			}
		}
	}
}
