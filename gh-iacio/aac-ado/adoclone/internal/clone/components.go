package clone

import (
	"context"
	"fmt"

	"github.com/coolado/adoclone/internal/metrics"
)

// Component is one step of a clone run.
type Component struct {
	Name  string
	Phase int
	About string
	Run   func(context.Context, *Ctx) error
}

// Components lists every component in run order. The phases follow the
// guide's implementation plan; later components rely on ID maps from earlier ones.
var Components = []Component{
	{"project", 1, "project shell with the same process (private by default)", CloneProject},
	{"groups", 1, "custom project security groups; memberships are copied by security", CloneGroups},
	{"nodes", 2, "area and iteration paths, with iteration dates", CloneNodes},
	{"teams", 2, "teams, team settings, team iterations, area values and members", CloneTeams},
	{"boards", 2, "board columns, swimlanes, card settings and card rules", CloneBoards},
	{"repos", 3, "Git repositories with every branch and tag (and LFS objects)", CloneRepos},
	{"endpoints", 4, "share service connections with the target", CloneEndpoints},
	{"vargroups", 4, "variable groups (copy or share)", CloneVarGroups},
	{"securefiles", 4, "secure files from -securefiles-dir", CloneSecureFiles},
	{"queues", 4, "agent queues for the same pools", CloneQueues},
	{"deploymentgroups", 4, "deployment groups on the same deployment pools", CloneDeploymentGroups},
	{"envs", 4, "environments and their Kubernetes resources", CloneEnvs},
	{"taskgroups", 5, "task groups", CloneTaskGroups},
	{"pipelines", 5, "build and YAML pipeline definitions and folders", ClonePipelines},
	{"releases", 5, "classic release definitions and folders", CloneReleases},
	{"checks", 5, "approvals and checks on environments, queues, variable groups, secure files and repos", CloneChecks},
	{"permissions", 5, "pipeline authorizations on resources", ClonePermissions},
	{"policies", 5, "branch and repository policies", ClonePolicies},
	{"settings", 5, "pipeline general settings and retention", CloneSettings},
	{"workitems", 6, "work items with comments, attachments and links", CloneWorkItems},
	{"testplans", 7, "test variables, configurations, plans, suites and suite test cases", CloneTestPlans},
	{"queries", 8, "shared queries and folders", CloneQueries},
	{"dashboards", 8, "project and team dashboards with widgets", CloneDashboards},
	{"plans", 8, "delivery plans", CloneDeliveryPlans},
	{"wiki", 8, "project wiki content and code wikis", CloneWiki},
	{"feeds", 8, "project-scoped Artifacts feeds, views, upstreams and permissions", CloneFeeds},
	{"security", 9, "group memberships, ACLs and role assignments", CloneSecurity},
	{"hooks", 9, "service hook subscriptions", CloneHooks},
}

// Names returns component names in run order.
func Names() []string {
	out := make([]string, len(Components))
	for i, c := range Components {
		out[i] = c.Name
	}
	return out
}

// Lookup finds a component by name.
func Lookup(name string) (Component, bool) {
	for _, c := range Components {
		if c.Name == name {
			return c, true
		}
	}
	return Component{}, false
}

// Run runs the named components in order. A component error stops the run
// (exit 1, safe to retry); per-item failures are counted in x.Failures().
func Run(ctx context.Context, x *Ctx, names []string) error {
	for _, n := range names {
		x.M.Set("adoclone_phase_status", metrics.Pending, "component", n)
	}
	for _, n := range names {
		c, ok := Lookup(n)
		if !ok {
			return fmt.Errorf("unknown component %q", n)
		}
		x.component = n
		x.M.Set("adoclone_phase_status", metrics.Running, "component", n)
		x.Log.Info("component start", "component", n, "phase", c.Phase)
		before := x.failures
		if err := c.Run(ctx, x); err != nil {
			x.M.Set("adoclone_phase_status", metrics.Failed, "component", n)
			return fmt.Errorf("%s: %w", n, err)
		}
		x.M.Set("adoclone_phase_status", metrics.Done, "component", n)
		x.Log.Info("component done", "component", n, "itemFailures", x.failures-before)
		if !x.Dry() {
			_ = x.S.SetPhase(n, "status", "done")
		}
	}
	return nil
}
