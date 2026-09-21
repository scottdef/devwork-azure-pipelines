// Package azuretest is a canned az, in the spirit of net/http/httptest.
// It answers the commands package azure issues with plausible JSON for
// an account with three deployments, one of them about to retire.
package azuretest

import (
	"fmt"
	"time"

	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

// AccountID is the ARM id every fixture agrees on.
const AccountID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-coolgit-copilot/providers/Microsoft.CognitiveServices/accounts/ais-coolgit-copilot-prod"

// Fake returns a Runner for az, plus "gh workflow run" and "copilot".
func Fake() *run.Fake {
	soon := time.Now().Add(30 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	return &run.Fake{Replies: map[string]run.Reply{
		"az account show":             {Out: `{"id":"00000000-0000-0000-0000-000000000000","name":"sub","tenantId":"t","user":{"name":"crafty@coolgit.example","type":"user"}}`},
		"az account get-access-token": {Out: `{"accessToken":"SECRET-TOKEN"}`},
		"az cognitiveservices account show": {Out: fmt.Sprintf(`{"id":%q,"name":"ais-coolgit-copilot-prod","kind":"AIServices","location":"eastus2","sku":{"name":"S0"},
		  "properties":{"endpoint":"https://ais-coolgit-copilot-prod.cognitiveservices.azure.com/","customSubDomainName":"ais-coolgit-copilot-prod",
		  "provisioningState":"Succeeded","publicNetworkAccess":"Enabled","disableLocalAuth":false}}`, AccountID)},
		"az cognitiveservices account deployment list": {Out: `[
		  {"name":"gpt-4o","sku":{"name":"GlobalStandard","capacity":50},"properties":{"model":{"format":"OpenAI","name":"gpt-4o","version":"2024-11-20"},
		   "provisioningState":"Succeeded","versionUpgradeOption":"NoAutoUpgrade","raiPolicyName":"Microsoft.DefaultV2","capabilities":{"chatCompletion":"true"},
		   "rateLimits":[{"key":"request","count":500,"renewalPeriod":60},{"key":"token","count":50000,"renewalPeriod":60}]},
		   "systemData":{"createdBy":"crafty@coolgit.example","createdAt":"2026-06-01T10:00:00Z","lastModifiedBy":"uami-foundry-deploy","lastModifiedAt":"2026-09-01T12:30:00Z"}},
		  {"name":"claude-sonnet","sku":{"name":"GlobalStandard","capacity":20},"properties":{"model":{"format":"Anthropic","name":"claude-sonnet-4-6","version":"1"},
		   "provisioningState":"Succeeded","capabilities":{"chatCompletion":"true"}},"systemData":{"createdBy":"uami-foundry-deploy","createdAt":"2026-08-11T09:00:00Z"}},
		  {"name":"embed-small","sku":{"name":"Standard","capacity":120},"properties":{"model":{"format":"OpenAI","name":"text-embedding-3-small","version":"1"},
		   "provisioningState":"Failed","capabilities":{"embeddings":"true"}},"systemData":{}}]`},
		"az cognitiveservices account list-models": {Out: fmt.Sprintf(`[
		  {"name":"gpt-4o","format":"OpenAI","version":"2024-11-20","lifecycleStatus":"GenerallyAvailable","skus":[{"name":"GlobalStandard","capacity":{"default":50,"maximum":1000}},{"name":"Standard","capacity":{"default":10,"maximum":450}}],"deprecation":{"inference":%q}},
		  {"name":"claude-sonnet-4-6","format":"Anthropic","version":"1","lifecycleStatus":"GenerallyAvailable","skus":[{"name":"GlobalStandard","capacity":{"default":20,"maximum":200}}],"deprecation":{}},
		  {"name":"mistral-large-2411","format":"Mistral AI","version":"2","lifecycleStatus":"GenerallyAvailable","skus":[{"name":"GlobalStandard","capacity":{"default":1,"maximum":10}}],"deprecation":{}}]`, soon)},
		"az cognitiveservices usage list": {Out: `[
		  {"name":{"value":"OpenAI.Standard.text-embedding-3-small"},"currentValue":120,"limit":140,"unit":"Count"},
		  {"name":{"value":"OpenAI.GlobalStandard.gpt-4o"},"currentValue":50,"limit":1000,"unit":"Count"},
		  {"name":{"value":"OpenAI.Standard.gpt-35-turbo"},"currentValue":0,"limit":300,"unit":"Count"}]`},
		"az role assignment list": {Out: fmt.Sprintf(`[
		  {"principalName":"uami-foundry-deploy","principalId":"p1","principalType":"ServicePrincipal","roleDefinitionName":"Foundry Owner","scope":%q,"createdOn":"2026-06-01T09:00:00Z"},
		  {"principalName":"","principalId":"p2","principalType":"Group","roleDefinitionName":"Owner","scope":"/subscriptions/00000000-0000-0000-0000-000000000000","createdOn":"2025-01-01T00:00:00Z"}]`, AccountID)},
		"az role definition list": {Out: `[]`}, // forces the embedded snapshot
		"az rest --method get --url " + AccountID + "/providers/Microsoft.ResourceHealth":   {Out: `{"properties":{"availabilityState":"Available","summary":"There aren't any known Azure platform problems affecting this resource."}}`},
		"az rest --method get --url " + AccountID + "/providers/Microsoft.AlertsManagement": {Out: `{"value":[{"name":"a1","properties":{"essentials":{"severity":"Sev2","alertState":"New","monitorCondition":"Fired","alertRule":"foundry-429-rate","startDateTime":"2026-09-20T08:00:00Z"}}}]}`},
		"gh workflow run": {Out: "https://github.com/CoolGitOrg/foundry-ops/actions/runs/42\n"},
		"copilot":         {Out: "FOUNDRY_OK\n"},
	}}
}

// Lines renders every command a Fake has seen, for assertions.
func Lines(r run.Runner) []string {
	f, ok := r.(*run.Fake)
	if !ok {
		return nil
	}
	var out []string
	for _, c := range f.Calls() {
		out = append(out, c.String())
	}
	return out
}
