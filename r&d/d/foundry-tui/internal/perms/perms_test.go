package perms

import "testing"

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, action string
		want            bool
	}{
		{"Microsoft.CognitiveServices/*", "Microsoft.CognitiveServices/accounts/deployments/write", true},
		{"Microsoft.Authorization/*/read", "Microsoft.Authorization/roleAssignments/read", true},
		{"Microsoft.Authorization/*/read", "Microsoft.Authorization/roleAssignments/write", false},
		{"microsoft.cognitiveservices/*", "Microsoft.CognitiveServices/accounts/read", true}, // case-insensitive
		{"Microsoft.Insights/metricalerts/*", "Microsoft.Insights/metrics/read", false},
		{"*", "Anything/at/all", true},
		{"Microsoft.Support/*", "Microsoft.SupportX/y", false},
	} {
		if got := match(c.pattern, c.action); got != c.want {
			t.Errorf("match(%q, %q) = %v, want %v", c.pattern, c.action, got, c.want)
		}
	}
}

// The design rests on these facts about the role. If Microsoft changes
// it, the live definition takes over; the snapshot must stay honest.
func TestSnapshotBoundary(t *testing.T) {
	s := Snapshot()
	want := map[string]bool{
		"deployments": true, "models": true, "usage": true, "access": true, "health": true, "alerts": true,
		"inference": true, "deploy.write": true, "deploy.delete": true,
		"metrics": false, "activity": false,
	}
	for key, allowed := range want {
		c, ok := s.Can(key)
		if !ok {
			t.Fatalf("no requirement %q", key)
		}
		if c.Allowed != allowed {
			t.Errorf("%s (%s): allowed=%v, want %v", key, c.Action, c.Allowed, allowed)
		}
	}
	if s.AllowsData("Microsoft.CognitiveServices/accounts/AIServices/agents/endpoints/UserIdentityImpersonation/action") {
		t.Error("notDataActions must win over dataActions")
	}
}
