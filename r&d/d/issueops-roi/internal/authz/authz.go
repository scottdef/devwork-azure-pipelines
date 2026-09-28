// Package authz decides whether a GitHub user satisfies an approval or
// requestor selector: team membership, explicit users, organization roles,
// repository permissions (on the IssueOps repository or on the request's
// target repository), users named in a form field, or the requestor.
package authz

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

// Directory answers identity questions. The GitHub implementation calls the
// REST API; tests use a static implementation.
type Directory interface {
	IsTeamMember(ctx context.Context, org, team, user string) (bool, error)
	OrgRole(ctx context.Context, org, user string) (string, error)
	HasOrgRole(ctx context.Context, org, role, user string) (bool, error)
	RepoPermission(ctx context.Context, owner, repo, user string) (string, error)
}

// Subject is the request context a selector is evaluated in.
type Subject struct {
	Org          string
	IssueOpsRepo string
	Requestor    string
	TargetRepo   string            // "repo" or "owner/repo"
	FieldUsers   map[string]string // form field id -> GitHub login
}

// Match is a positive selector match with a human-readable reason.
type Match struct {
	OK     bool
	Reason string
}

func eq(a, b string) bool {
	return strings.EqualFold(strings.TrimPrefix(a, "@"), strings.TrimPrefix(b, "@"))
}

func splitRepo(org, repo string) (string, string) {
	if o, r, ok := strings.Cut(repo, "/"); ok {
		return o, r
	}
	return org, repo
}

// Evaluate checks whether user satisfies selector s.
func Evaluate(ctx context.Context, dir Directory, s config.Selector, user string, sub Subject) (Match, error) {
	if user == "" {
		return Match{}, nil
	}
	if s.Requestor && eq(user, sub.Requestor) {
		return Match{true, "is the requestor"}, nil
	}
	for _, u := range s.Users {
		if eq(u, user) {
			return Match{true, "listed approver @" + strings.TrimPrefix(u, "@")}, nil
		}
	}
	for _, f := range s.FieldUsers {
		if v := sub.FieldUsers[f]; v != "" && eq(v, user) {
			return Match{true, fmt.Sprintf("named in field %q", f)}, nil
		}
	}
	for _, t := range s.Teams {
		org, team := sub.Org, t
		if o, tm, ok := strings.Cut(t, "/"); ok {
			org, team = strings.TrimPrefix(o, "@"), tm
		}
		ok, err := dir.IsTeamMember(ctx, org, team, user)
		if err != nil {
			return Match{}, fmt.Errorf("authz: team %s/%s: %w", org, team, err)
		}
		if ok {
			return Match{true, fmt.Sprintf("member of @%s/%s", org, team)}, nil
		}
	}
	for _, role := range s.OrgRoles {
		if role == "owner" {
			r, err := dir.OrgRole(ctx, sub.Org, user)
			if err != nil {
				return Match{}, fmt.Errorf("authz: org membership: %w", err)
			}
			if r == "admin" {
				return Match{true, "organization owner of " + sub.Org}, nil
			}
			continue
		}
		ok, err := dir.HasOrgRole(ctx, sub.Org, role, user)
		if err != nil {
			return Match{}, fmt.Errorf("authz: org role %s: %w", role, err)
		}
		if ok {
			return Match{true, fmt.Sprintf("holds organization role %q", role)}, nil
		}
	}
	if len(s.RepoPermissions) > 0 && sub.IssueOpsRepo != "" {
		owner, repo := splitRepo(sub.Org, sub.IssueOpsRepo)
		if m, err := repoMatch(ctx, dir, owner, repo, user, s.RepoPermissions); err != nil || m.OK {
			return m, err
		}
	}
	if len(s.TargetRepoPermissions) > 0 && sub.TargetRepo != "" {
		owner, repo := splitRepo(sub.Org, sub.TargetRepo)
		if m, err := repoMatch(ctx, dir, owner, repo, user, s.TargetRepoPermissions); err != nil || m.OK {
			return m, err
		}
	}
	return Match{}, nil
}

func repoMatch(ctx context.Context, dir Directory, owner, repo, user string, perms []string) (Match, error) {
	p, err := dir.RepoPermission(ctx, owner, repo, user)
	if err != nil {
		return Match{}, fmt.Errorf("authz: permission on %s/%s: %w", owner, repo, err)
	}
	for _, want := range perms {
		if strings.EqualFold(p, want) {
			return Match{true, fmt.Sprintf("has %s on %s/%s", p, owner, repo)}, nil
		}
	}
	return Match{}, nil
}

// GitHub is a Directory backed by the REST API with per-process caching.
type GitHub struct {
	C     *ghapi.Client
	mu    sync.Mutex
	cache map[string]any
}

// NewGitHub wraps a client.
func NewGitHub(c *ghapi.Client) *GitHub { return &GitHub{C: c, cache: map[string]any{}} }

func (g *GitHub) get(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if v, ok := g.cache[key]; ok {
		g.mu.Unlock()
		return v, nil
	}
	g.mu.Unlock()
	v, err := fn()
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.cache[key] = v
	g.mu.Unlock()
	return v, nil
}

// IsTeamMember implements Directory.
func (g *GitHub) IsTeamMember(ctx context.Context, org, team, user string) (bool, error) {
	v, err := g.get(strings.ToLower("team|"+org+"|"+team+"|"+user), func() (any, error) { return g.C.IsTeamMember(ctx, org, team, user) })
	if err != nil {
		return false, err
	}
	return v.(bool), nil
}

// OrgRole implements Directory.
func (g *GitHub) OrgRole(ctx context.Context, org, user string) (string, error) {
	v, err := g.get(strings.ToLower("orgrole|"+org+"|"+user), func() (any, error) { return g.C.OrgMembership(ctx, org, user) })
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

type roleAssignees struct{ users, teams []string }

// HasOrgRole implements Directory for predefined and custom organization roles.
func (g *GitHub) HasOrgRole(ctx context.Context, org, role, user string) (bool, error) {
	v, err := g.get(strings.ToLower("roleassign|"+org+"|"+role), func() (any, error) {
		roles, err := g.C.ListOrgRoles(ctx, org)
		if err != nil {
			return nil, err
		}
		for _, r := range roles {
			if strings.EqualFold(r.Name, role) {
				u, t, err := g.C.OrgRoleAssignees(ctx, org, r.ID)
				return roleAssignees{u, t}, err
			}
		}
		return roleAssignees{}, nil
	})
	if err != nil {
		return false, err
	}
	ra := v.(roleAssignees)
	for _, u := range ra.users {
		if eq(u, user) {
			return true, nil
		}
	}
	for _, t := range ra.teams {
		ok, err := g.IsTeamMember(ctx, org, t, user)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// RepoPermission implements Directory.
func (g *GitHub) RepoPermission(ctx context.Context, owner, repo, user string) (string, error) {
	v, err := g.get(strings.ToLower("repoperm|"+owner+"|"+repo+"|"+user), func() (any, error) { return g.C.RepoPermission(ctx, owner, repo, user) })
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// Static is an in-memory Directory for tests and dry runs.
type Static struct {
	Teams     map[string][]string // "org/team" -> logins
	OrgRoles  map[string]string   // login -> "admin" | "member"
	Custom    map[string][]string // role name -> logins
	RepoPerms map[string]string   // "owner/repo|login" -> permission
}

// IsTeamMember implements Directory.
func (s *Static) IsTeamMember(_ context.Context, org, team, user string) (bool, error) {
	for _, u := range s.Teams[strings.ToLower(org+"/"+team)] {
		if eq(u, user) {
			return true, nil
		}
	}
	return false, nil
}

// OrgRole implements Directory.
func (s *Static) OrgRole(_ context.Context, _, user string) (string, error) {
	return s.OrgRoles[strings.ToLower(user)], nil
}

// HasOrgRole implements Directory.
func (s *Static) HasOrgRole(_ context.Context, _, role, user string) (bool, error) {
	for _, u := range s.Custom[role] {
		if eq(u, user) {
			return true, nil
		}
	}
	return false, nil
}

// RepoPermission implements Directory.
func (s *Static) RepoPermission(_ context.Context, owner, repo, user string) (string, error) {
	if p, ok := s.RepoPerms[strings.ToLower(owner+"/"+repo+"|"+user)]; ok {
		return p, nil
	}
	return "none", nil
}
