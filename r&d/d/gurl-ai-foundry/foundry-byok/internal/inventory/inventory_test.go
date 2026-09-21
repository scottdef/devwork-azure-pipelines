package inventory

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
)

func loadExample(t *testing.T) Bundle {
	t.Helper()
	f, err := LoadFile("../../examples/inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	specs, err := f.Specs()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build("test", specs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExampleInventory(t *testing.T) {
	b := loadExample(t)
	if got := len(b.Items); got != 8 {
		t.Fatalf("items = %d, want 8", got)
	}

	// Sorted by environment, resource, deployment.
	var order []string
	for _, it := range b.Items {
		order = append(order, it.Spec.Environment+"/"+it.Spec.Deployment)
	}
	want := []string{
		"dev/gpt-41-dev", "dev/gpt-5.4", "dev/gpt-oss-120b",
		"prod/gpt-5.4-chat", "prod/gpt-5.4-prod",
		"uat/claude-opus-4-5", "uat/gpt-5.4", "uat/o4-mini-aw",
	}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v\n want %v", order, want)
	}

	// Defaults flowed down; deployment-level override won.
	for _, it := range b.Items {
		if it.Spec.RunsOn != "self-hosted, linux, x64" {
			t.Errorf("%s: RunsOn = %q (default not inherited)", it.Spec.Deployment, it.Spec.RunsOn)
		}
		if it.Spec.Deployment == "gpt-5.4-chat" {
			if it.Spec.Auth != foundry.AuthAPIKey || it.Spec.APIKeySecret != "FOUNDRY_PROD_API_KEY" {
				t.Errorf("gpt-5.4-chat: deployment override lost: %+v", it.Spec)
			}
			if !it.Spec.DisableModelFallback {
				t.Error("gpt-5.4-chat: sticky disable_model_fallback lost")
			}
		}
	}
}

func TestEnterpriseGrouping(t *testing.T) {
	b := loadExample(t)

	byName := map[string]EnterpriseGroup{}
	for _, g := range b.EnterpriseGroups {
		byName[g.KeyName] = g
	}
	// dev: three models share one /openai/v1 URL -> one key entry.
	dev, ok := byName["foundry-dev-fdy-myass-dev-eus2"]
	if !ok {
		t.Fatalf("dev group missing; have %v", keys(byName))
	}
	if want := []string{"gpt-41-dev", "gpt-5.4", "gpt-oss-120b"}; !reflect.DeepEqual(dev.ModelIDs, want) {
		t.Errorf("dev ModelIDs = %v, want %v", dev.ModelIDs, want)
	}
	if dev.DeploymentURL != "https://fdy-myass-dev-eus2.openai.azure.com/openai/v1" {
		t.Errorf("dev URL = %q", dev.DeploymentURL)
	}
	// uat: Claude lives on a different URL, so it must be its own key entry.
	if _, ok := byName["foundry-uat-fdy-myass-uat-eus2-anthropic"]; !ok {
		t.Errorf("uat anthropic group missing; have %v", keys(byName))
	}
	if g := byName["foundry-uat-fdy-myass-uat-eus2"]; len(g.ModelIDs) != 2 {
		t.Errorf("uat openai ModelIDs = %v, want 2", g.ModelIDs)
	}
	// prod: the entra deployment is skipped, the api-key one is registered.
	if g := byName["foundry-prod-fdy-myass-prod-eus2"]; !reflect.DeepEqual(g.ModelIDs, []string{"gpt-5.4-chat"}) {
		t.Errorf("prod ModelIDs = %v", g.ModelIDs)
	}
	if len(b.EnterpriseSkipped) != 1 || b.EnterpriseSkipped[0].Deployment != "gpt-5.4-prod" {
		t.Errorf("skipped = %+v", b.EnterpriseSkipped)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	a, _ := json.Marshal(loadExample(t))
	b, _ := json.Marshal(loadExample(t))
	if string(a) != string(b) {
		t.Error("two builds of the same inventory differ; generated files would churn in git")
	}
}

func TestStrictParsing(t *testing.T) {
	cases := map[string]string{
		"unknown field":   `{"version":1,"resources":[{"name":"abc","deploymnts":[]}]}`,
		"wrong version":   `{"version":2,"resources":[{"name":"abc","deployments":[]}]}`,
		"no resources":    `{"version":1,"resources":[]}`,
		"trailing json":   `{"version":1,"resources":[{"name":"abc","deployments":[]}]}{}`,
		"not json at all": `version: 1`,
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected a parse error", name)
		}
	}
}

func TestKeyNameCollisionIsRejected(t *testing.T) {
	// Same explicit key name, different URLs: GitHub allows one URL per key.
	specs := []foundry.Spec{
		{Resource: "res-one", Model: "gpt-4o", Deployment: "a", EnterpriseKeyName: "shared"},
		{Resource: "res-two", Model: "gpt-4o", Deployment: "b", EnterpriseKeyName: "shared"},
	}
	_, err := Build("test", specs)
	if err == nil || !strings.Contains(err.Error(), "one URL per key") {
		t.Errorf("err = %v, want a one-URL-per-key error", err)
	}
}

func TestDuplicateDeploymentIsRejected(t *testing.T) {
	s := foundry.Spec{Resource: "res-one", Model: "gpt-4o", Deployment: "a"}
	if _, err := Build("test", []foundry.Spec{s, s}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err = %v, want duplicate error", err)
	}
}

func TestAllProblemsReportedTogether(t *testing.T) {
	specs := []foundry.Spec{
		{Resource: "ok-res", Model: "gpt-4o", Deployment: "bad name"},
		{Resource: "x", Model: "gpt-4o", Deployment: "d"},
	}
	_, err := Build("test", specs)
	if err == nil || !strings.Contains(err.Error(), "2 problem(s)") {
		t.Errorf("err = %v, want both problems in one error", err)
	}
}

func TestEmptyDeploymentsRejected(t *testing.T) {
	f, err := Parse([]byte(`{"version":1,"resources":[{"name":"abc","deployments":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Specs(); err == nil {
		t.Error("expected an error for a resource without deployments")
	}
}

func TestLoadFileMissing(t *testing.T) {
	if _, err := LoadFile(os.DevNull + "/nope.json"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func keys(m map[string]EnterpriseGroup) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
