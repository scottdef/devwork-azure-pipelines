package ghapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// IsTeamMember reports whether user is an active member (or maintainer) of org/team.
func (c *Client) IsTeamMember(ctx context.Context, org, team, user string) (bool, error) {
	var m struct {
		State string `json:"state"`
		Role  string `json:"role"`
	}
	resp, err := c.JSON(ctx, Request{Path: fmt.Sprintf("orgs/%s/teams/%s/memberships/%s", org, team, user), OK: []int{http.StatusNotFound}}, &m)
	if err != nil {
		return false, err
	}
	if resp.Status == http.StatusNotFound {
		return false, nil
	}
	return m.State == "active", nil
}

// OrgMembership returns the user's organization role: "admin" (owner),
// "member", or "" when not a member.
func (c *Client) OrgMembership(ctx context.Context, org, user string) (string, error) {
	var m struct {
		State string `json:"state"`
		Role  string `json:"role"`
	}
	resp, err := c.JSON(ctx, Request{Path: fmt.Sprintf("orgs/%s/memberships/%s", org, user), OK: []int{http.StatusNotFound}}, &m)
	if err != nil {
		return "", err
	}
	if resp.Status == http.StatusNotFound || m.State != "active" {
		return "", nil
	}
	return m.Role, nil
}

// OrgRole is an organization role (predefined or custom).
type OrgRole struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ListOrgRoles lists organization roles.
func (c *Client) ListOrgRoles(ctx context.Context, org string) ([]OrgRole, error) {
	var out struct {
		Roles []OrgRole `json:"roles"`
	}
	_, err := c.JSON(ctx, Request{Path: fmt.Sprintf("orgs/%s/organization-roles", org)}, &out)
	return out.Roles, err
}

// OrgRoleAssignees returns user logins and team slugs assigned to a role.
func (c *Client) OrgRoleAssignees(ctx context.Context, org string, roleID int64) (users, teams []string, err error) {
	err = c.PaginateArray(ctx, Request{Path: fmt.Sprintf("orgs/%s/organization-roles/%d/users", org, roleID)}, func(items []json.RawMessage) error {
		for _, raw := range items {
			var u User
			if err := json.Unmarshal(raw, &u); err != nil {
				return err
			}
			users = append(users, u.Login)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	err = c.PaginateArray(ctx, Request{Path: fmt.Sprintf("orgs/%s/organization-roles/%d/teams", org, roleID)}, func(items []json.RawMessage) error {
		for _, raw := range items {
			var t struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(raw, &t); err != nil {
				return err
			}
			teams = append(teams, t.Slug)
		}
		return nil
	})
	return users, teams, err
}

// RepoPermission returns the user's role on a repository: admin, maintain,
// write, triage, read, or "none" (also for unknown repos/users).
func (c *Client) RepoPermission(ctx context.Context, owner, repo, user string) (string, error) {
	var p struct {
		Permission string `json:"permission"`
		RoleName   string `json:"role_name"`
	}
	resp, err := c.JSON(ctx, Request{Path: fmt.Sprintf("repos/%s/%s/collaborators/%s/permission", owner, repo, user), OK: []int{http.StatusNotFound}}, &p)
	if err != nil {
		return "", err
	}
	if resp.Status == http.StatusNotFound {
		return "none", nil
	}
	if p.RoleName != "" {
		return strings.ToLower(p.RoleName), nil
	}
	return strings.ToLower(p.Permission), nil
}

// Repository is the subset of repository metadata used for validation.
type Repository struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Visibility    string `json:"visibility"`
}

// GetRepo returns a repository or (nil, nil) if it does not exist.
func (c *Client) GetRepo(ctx context.Context, owner, repo string) (*Repository, error) {
	var r Repository
	resp, err := c.JSON(ctx, Request{Path: fmt.Sprintf("repos/%s/%s", owner, repo), OK: []int{http.StatusNotFound}}, &r)
	if err != nil {
		return nil, err
	}
	if resp.Status == http.StatusNotFound {
		return nil, nil
	}
	return &r, nil
}

// TeamExists reports whether a team slug exists in the org.
func (c *Client) TeamExists(ctx context.Context, org, team string) (bool, error) {
	resp, err := c.Do(ctx, Request{Path: fmt.Sprintf("orgs/%s/teams/%s", org, team), OK: []int{http.StatusNotFound}})
	if err != nil {
		return false, err
	}
	return resp.Status == http.StatusOK, nil
}

// UserExists reports whether a GitHub user exists.
func (c *Client) UserExists(ctx context.Context, login string) (bool, error) {
	resp, err := c.Do(ctx, Request{Path: "users/" + login, OK: []int{http.StatusNotFound}})
	if err != nil {
		return false, err
	}
	return resp.Status == http.StatusOK, nil
}
