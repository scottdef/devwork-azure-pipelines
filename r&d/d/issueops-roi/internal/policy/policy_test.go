package policy

import (
	"testing"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/testutil"
)

func TestPredicates(t *testing.T) {
	f := Facts{"amount": "1,500", "sku": "GlobalProvisionedManaged", "opts": []string{"CSV exports", "Interactive HTML report"}, "env": "prod", "blank": ""}
	cases := []struct {
		p    config.Predicate
		want bool
	}{
		{config.Predicate{Field: "amount", Op: "gt", Value: 1000}, true},
		{config.Predicate{Field: "amount", Op: "lte", Value: 1000.0}, false},
		{config.Predicate{Field: "amount", Op: "eq", Value: "1500"}, true},
		{config.Predicate{Field: "sku", Op: "matches", Value: "ProvisionedManaged$"}, true},
		{config.Predicate{Field: "sku", Op: "in", Value: []any{"GlobalStandard", "DataZoneStandard"}}, false},
		{config.Predicate{Field: "opts", Op: "contains", Value: "CSV exports"}, true},
		{config.Predicate{Field: "opts", Op: "contains", Value: "CSV"}, false},
		{config.Predicate{Field: "env", Op: "ne", Value: "dev"}, true},
		{config.Predicate{Field: "blank", Op: "empty"}, true},
		{config.Predicate{Field: "missing", Op: "empty"}, true},
		{config.Predicate{Field: "missing", Op: "eq", Value: "x"}, false},
		{config.Predicate{Field: "sku", Op: "gt", Value: 3}, false}, // non-numeric never satisfies a bound
	}
	for _, c := range cases {
		got, err := evalPredicate(c.p, f)
		if err != nil || got != c.want {
			t.Errorf("%+v = %v (%v), want %v", c.p, got, err, c.want)
		}
	}
}

func TestPlansFromRegistry(t *testing.T) {
	reg, err := config.Load(testutil.Path(t, "config", "issueops.json"))
	if err != nil {
		t.Fatal(err)
	}
	foundry, _ := reg.ByID("foundry-model-deployment")
	p, err := BuildPlan(foundry, Facts{"$environment": "dev", "foundry_sku": "GlobalStandard", "foundry_capacity": "10"})
	if err != nil || !p.AutoApproved || len(p.Rules) != 0 || p.Environment != "foundry-dev" {
		t.Fatalf("dev plan = %+v, %v", p, err)
	}
	p, _ = BuildPlan(foundry, Facts{"$environment": "prod", "foundry_sku": "GlobalProvisionedManaged", "foundry_capacity": "100"})
	if p.AutoApproved || len(p.Rules) != 3 || len(p.Escalations) != 2 || p.Environment != "foundry-prod" {
		t.Fatalf("prod PTU plan = %+v", p)
	}
	budget, _ := reg.ByID("copilot-budget-request")
	p, _ = BuildPlan(budget, Facts{"budget_amount": "2500", "budget_scope": "repository"})
	if p.Environment != "copilot-budgets-high" || len(p.Rules) != 2 {
		t.Fatalf("budget plan = %+v", p)
	}
	if got := StaticAllowlist(reg.Organization, p.Rules); got != "CoolEngOrg/copilot-admins,CoolEngOrg/finops-approvers" {
		t.Fatalf("allowlist = %q", got)
	}
	agent, _ := reg.ByID("agentic-task-request")
	p, _ = BuildPlan(agent, Facts{"agent_backend": "copilot-cloud-agent", "agent_data_classification": "internal", "$backend_environment": "agentic-copilot"})
	if StaticAllowlist(reg.Organization, p.Rules) != "false" || p.Environment != "agentic-copilot" {
		t.Fatalf("agent plan = %+v", p)
	}
}
