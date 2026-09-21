// Package perms answers one question: may the Foundry Owner role do this?
//
// Reports are bounded by that role on purpose. A report that silently
// depends on Monitoring Reader or Contributor cannot be reproduced by
// the next Foundry Owner who runs it. So every section of the report
// names the Azure action it needs, and sections the role does not
// cover are skipped and say so.
package perms

import (
	"context"
	"regexp"
	"strings"

	"github.com/CoolGitOrg/foundry-tui/internal/azure"
)

// FoundryOwnerID is the built-in role's GUID. The display name changed
// from "Azure AI Owner" in May 2026; the GUID did not.
const FoundryOwnerID = "c883944f-8b7b-4483-af10-35834be79c4a"

// Set is a role's permissions, flattened.
type Set struct {
	Role           string
	Source         string // "live" or "snapshot 2026-07-16"
	Actions        []string
	NotActions     []string
	DataActions    []string
	NotDataActions []string
}

// Snapshot is Foundry Owner as published on 2026-07-16. It is used when
// az cannot fetch the live definition. Note what is absent:
// Microsoft.Insights/metrics/read and the activity log.
func Snapshot() Set {
	return Set{
		Role:   "Foundry Owner",
		Source: "snapshot 2026-07-16",
		Actions: []string{
			"Microsoft.AlertsManagement/actionRules/*",
			"Microsoft.AlertsManagement/alerts/*",
			"Microsoft.AlertsManagement/issues/*",
			"Microsoft.AlertsManagement/prometheusRuleGroups/*",
			"Microsoft.AlertsManagement/smartDetectorAlertRules/*",
			"Microsoft.Authorization/*/read",
			"Microsoft.Authorization/roleAssignments/write",
			"Microsoft.Authorization/roleAssignments/delete",
			"Microsoft.CognitiveServices/*",
			"Microsoft.Insights/activityLogAlerts/*",
			"Microsoft.Insights/metricalerts/*",
			"Microsoft.Insights/scheduledqueryrules/*",
			"Microsoft.ResourceHealth/availabilityStatuses/read",
			"Microsoft.Resources/deployments/*",
			"Microsoft.Resources/deployments/operations/read",
			"Microsoft.Resources/subscriptions/operationresults/read",
			"Microsoft.Resources/subscriptions/read",
			"Microsoft.Resources/subscriptions/resourcegroups/deployments/*",
			"Microsoft.Resources/subscriptions/resourceGroups/read",
			"Microsoft.Support/*",
		},
		DataActions: []string{"Microsoft.CognitiveServices/*"},
		NotDataActions: []string{
			"Microsoft.CognitiveServices/accounts/AIServices/agents/endpoints/UserIdentityImpersonation/action",
		},
	}
}

// Load fetches the live role definition and falls back to the snapshot.
func Load(ctx context.Context, az azure.Client) Set {
	def, err := az.RoleDefinition(ctx, FoundryOwnerID)
	if err != nil || len(def.Permissions) == 0 {
		return Snapshot()
	}
	s := Set{Role: def.RoleName, Source: "live"}
	for _, p := range def.Permissions {
		s.Actions = append(s.Actions, p.Actions...)
		s.NotActions = append(s.NotActions, p.NotActions...)
		s.DataActions = append(s.DataActions, p.DataActions...)
		s.NotDataActions = append(s.NotDataActions, p.NotDataActions...)
	}
	return s
}

// Allows reports whether a control-plane action is granted.
func (s Set) Allows(action string) bool {
	return matchAny(s.Actions, action) && !matchAny(s.NotActions, action)
}

// AllowsData reports whether a data-plane action is granted.
func (s Set) AllowsData(action string) bool {
	return matchAny(s.DataActions, action) && !matchAny(s.NotDataActions, action)
}

func matchAny(patterns []string, action string) bool {
	for _, p := range patterns {
		if match(p, action) {
			return true
		}
	}
	return false
}

// match implements Azure's rule: case-insensitive, '*' spans anything,
// slashes included.
func match(pattern, action string) bool {
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	re, err := regexp.Compile("(?i)^" + strings.Join(parts, ".*") + "$")
	return err == nil && re.MatchString(action)
}

// Requirement ties a feature of this program to the action it needs.
type Requirement struct {
	Key    string // stable identifier used by package report
	What   string
	Action string
	Data   bool   // data plane rather than control plane
	Note   string // what to do when it is denied
}

// Requirements is the whole surface of the program, reads and writes.
var Requirements = []Requirement{
	{Key: "account", What: "Foundry account settings", Action: "Microsoft.CognitiveServices/accounts/read"},
	{Key: "deployments", What: "Model deployments and authorship", Action: "Microsoft.CognitiveServices/accounts/deployments/read"},
	{Key: "models", What: "Deployable model catalog and retirement dates", Action: "Microsoft.CognitiveServices/accounts/models/read"},
	{Key: "usage", What: "Regional quota", Action: "Microsoft.CognitiveServices/locations/usages/read"},
	{Key: "access", What: "Role assignments on the account", Action: "Microsoft.Authorization/roleAssignments/read"},
	{Key: "health", What: "Resource Health", Action: "Microsoft.ResourceHealth/availabilityStatuses/read"},
	{Key: "alerts", What: "Fired Azure Monitor alerts", Action: "Microsoft.AlertsManagement/alerts/read"},
	{Key: "metrics", What: "Azure Monitor metrics (requests, tokens)", Action: "Microsoft.Insights/metrics/read",
		Note: "Not in Foundry Owner. Add Monitoring Reader on the account, or rely on alerts and probes."},
	{Key: "activity", What: "Azure activity log", Action: "Microsoft.Insights/eventtypes/values/read",
		Note: "Not in Foundry Owner and not collected. Deployment systemData and the workflow run history cover authorship."},
	{Key: "inference", What: "Model probes (chat completions)", Data: true,
		Action: "Microsoft.CognitiveServices/accounts/OpenAI/deployments/chat/completions/action"},
	{Key: "deploy.write", What: "Create or scale a deployment (workflow identity)", Action: "Microsoft.CognitiveServices/accounts/deployments/write"},
	{Key: "deploy.delete", What: "Delete a deployment (workflow identity)", Action: "Microsoft.CognitiveServices/accounts/deployments/delete"},
}

// Check is one row of the permission matrix.
type Check struct {
	Requirement
	Allowed bool
}

// Matrix evaluates every requirement against the set.
func (s Set) Matrix() []Check {
	out := make([]Check, 0, len(Requirements))
	for _, r := range Requirements {
		ok := s.Allows(r.Action)
		if r.Data {
			ok = s.AllowsData(r.Action)
		}
		out = append(out, Check{r, ok})
	}
	return out
}

// Can looks a requirement up by key.
func (s Set) Can(key string) (Check, bool) {
	for _, c := range s.Matrix() {
		if c.Key == key {
			return c, true
		}
	}
	return Check{}, false
}
