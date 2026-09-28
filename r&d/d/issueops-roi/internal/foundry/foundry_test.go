package foundry

import (
	"encoding/json"
	"strings"
	"testing"
)

func usage(name string, used, limit float64) AzUsage {
	var u AzUsage
	u.Name.Value, u.CurrentValue, u.Limit = name, used, limit
	return u
}

func model(name, version, sku string) AzModel {
	var m AzModel
	_ = json.Unmarshal([]byte(`{"name":"`+name+`","format":"OpenAI","version":"`+version+`","skus":[{"name":"`+sku+`"}]}`), &m)
	return m
}

func TestPreflightQuota(t *testing.T) {
	p := &Plan{Account: "a", Region: "eastus2", Model: "gpt-4.1", Version: "2025-04-14", Format: "OpenAI", SKU: "GlobalStandard", Capacity: 100, DeploymentName: "d"}
	ms := []AzModel{model("gpt-4.1", "2025-04-14", "GlobalStandard")}
	r := Preflight(p, ms, []AzUsage{usage("OpenAI.GlobalStandard.gpt-4.1", 950, 1000)}, nil, true)
	if r.OK || !strings.Contains(strings.Join(r.Errors, " "), "Insufficient quota") {
		t.Fatalf("expected quota failure: %+v", r)
	}
	// Updating an existing deployment frees its current capacity.
	var d AzDeployment
	d.Name, d.SKU.Name, d.SKU.Capacity, d.Properties.Model.Name = "d", "GlobalStandard", 80, "gpt-4.1"
	r = Preflight(p, ms, []AzUsage{usage("OpenAI.GlobalStandard.gpt-4.1", 950, 1000)}, []AzDeployment{d}, true)
	if !r.OK || r.Mode != "update" || r.QuotaAvailable != 130 {
		t.Fatalf("update mode: %+v", r)
	}
	// Strict quota: a missing usage entry fails for OpenAI standard SKUs.
	r = Preflight(p, ms, nil, nil, true)
	if r.OK {
		t.Fatal("missing quota entry must fail in strict mode")
	}
}

func TestPreflightPTUFallbackAndDrift(t *testing.T) {
	p := &Plan{Model: "gpt-4.1", Version: "2025-04-14", Format: "OpenAI", SKU: "GlobalProvisionedManaged", Capacity: 100, DeploymentName: "agents", Provisioned: true}
	ms := []AzModel{model("gpt-4.1", "2025-04-14", "GlobalProvisionedManaged")}
	r := Preflight(p, ms, []AzUsage{usage("OpenAI.GlobalProvisionedManaged", 250, 500)}, nil, true)
	if !r.OK || r.QuotaName != "OpenAI.GlobalProvisionedManaged" || r.QuotaAvailable != 250 {
		t.Fatalf("PTU fallback: %+v", r)
	}
	var d AzDeployment
	d.Name, d.SKU.Name, d.Properties.Model.Name = "agents", "GlobalStandard", "o4-mini"
	r = Preflight(p, ms, []AzUsage{usage("OpenAI.GlobalProvisionedManaged", 0, 500)}, []AzDeployment{d}, true)
	if r.OK || len(r.Errors) != 2 {
		t.Fatalf("expected model and SKU drift errors: %+v", r.Errors)
	}
	r = Preflight(p, []AzModel{model("gpt-4.1", "2025-04-14", "GlobalStandard")}, nil, nil, true)
	if r.OK {
		t.Fatal("SKU not offered must fail")
	}
}

func TestBicepParametersAreJSON(t *testing.T) {
	p := &Plan{Account: "aif", DeploymentName: `x","evil":"y`, Model: "m", Version: "1", Format: "OpenAI", SKU: "GlobalStandard", Capacity: 5, VersionUpgrade: "NoAutoUpgrade", RAIPolicy: "Microsoft.DefaultV2"}
	b, err := p.BicepParameters()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Parameters map[string]struct {
			Value any `json:"value"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Parameters["deploymentName"].Value != `x","evil":"y` || len(doc.Parameters) != 9 {
		t.Fatalf("parameters = %+v", doc.Parameters)
	}
}
