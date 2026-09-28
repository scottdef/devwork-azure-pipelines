package budget

import (
	"testing"
	"time"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/request"
	"github.com/CoolEngOrg/issueops/internal/testutil"
)

func ctx(t *testing.T, v issueform.Values) *request.Context {
	reg, err := config.Load(testutil.Path(t, "config", "issueops.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt, _ := reg.ByID("copilot-budget-request")
	return &request.Context{Registry: reg, Type: rt, Values: v, Requestor: "dana-dev", Issue: 1, Now: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)}
}

func base() issueform.Values {
	return issueform.Values{
		"budget_scope": []string{"repository"}, "budget_target": "CoolEngOrg/payments-api", "budget_product": []string{"AI credits"},
		"budget_amount": "$2,500", "budget_enforcement": []string{"Stop usage when the budget is reached"},
		"budget_alert_recipients": "@dana-dev", "budget_cost_center": "CC-1", "budget_justification": "x",
	}
}

func TestRepositoryPlan(t *testing.T) {
	p, errs := BuildPlan(ctx(t, base()))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if p.Scope != "repository" || p.EntityName != "CoolEngOrg/payments-api" || p.Amount != 2500 || !p.Prevent || p.SKU != "ai_credits" {
		t.Fatalf("plan = %+v", p)
	}
	cp := p.CreatePayload()
	if cp.BudgetType != "BundlePricing" || !cp.BudgetAlerting.WillAlert || cp.BudgetAlerting.AlertRecipients[0] != "dana-dev" {
		t.Fatalf("payload = %+v", cp)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]func(v issueform.Values){
		"user budgets cannot have alert recipients": func(v issueform.Values) { v["budget_scope"] = []string{"user"}; v["budget_target"] = "sam" },
		"user budgets must stop usage": func(v issueform.Values) {
			v["budget_scope"] = []string{"user"}
			v["budget_target"] = "sam"
			v["budget_alert_recipients"] = ""
			v["budget_enforcement"] = []string{"Alert only"}
		},
		"amount above max":           func(v issueform.Values) { v["budget_amount"] = "25000" },
		"non-numeric amount":         func(v issueform.Values) { v["budget_amount"] = "lots" },
		"repo outside org":           func(v issueform.Values) { v["budget_target"] = "OtherOrg/app" },
		"cost-center disabled":       func(v issueform.Values) { v["budget_scope"] = []string{"cost-center"}; v["budget_target"] = "Payments" },
		"expiry only for user scope": func(v issueform.Values) { v["budget_expires_on"] = "2026-12-31" },
		"expiry in the past": func(v issueform.Values) {
			v["budget_scope"] = []string{"user"}
			v["budget_target"] = "sam"
			v["budget_alert_recipients"] = ""
			v["budget_expires_on"] = "2026-01-01"
		},
	}
	for name, mutate := range cases {
		v := base()
		mutate(v)
		if _, errs := BuildPlan(ctx(t, v)); len(errs) == 0 {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestUnchangedComparesAlerting(t *testing.T) {
	p, errs := BuildPlan(ctx(t, base()))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	cur := &ghapi.Budget{BudgetAmount: 2500, PreventFurtherUsage: true,
		BudgetAlerting: ghapi.BudgetAlerting{WillAlert: true, AlertRecipients: []string{"DANA-DEV"}}}
	if !Unchanged(cur, p) {
		t.Fatal("same amount, enforcement and recipients (case-insensitive) must be unchanged")
	}
	cur.BudgetAlerting.AlertRecipients = []string{"someone-else"}
	if Unchanged(cur, p) {
		t.Fatal("a recipient change must be applied")
	}
}
