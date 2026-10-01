package clone

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/coolado/adoclone/internal/jx"
)

// Graph and identity helpers shared by groups, teams and security.

type graphGroup struct {
	Descriptor    string `json:"descriptor"`
	DisplayName   string `json:"displayName"`
	Description   string `json:"description"`
	PrincipalName string `json:"principalName"`
}

type identityRef struct {
	ID         string `json:"id"`
	Descriptor string `json:"descriptor"`
}

// descriptorOf returns the graph subject descriptor for a storage key
// (project id -> scope descriptor, team id -> team group, identity id -> subject).
func (x *Ctx) descriptorOf(ctx context.Context, storageKey string) (string, error) {
	var r struct {
		Value string `json:"value"`
	}
	err := x.C.Get(ctx, x.vssps("_apis/graph/descriptors/"+storageKey+"?api-version=7.1"), &r)
	return r.Value, err
}

func (x *Ctx) projectGroups(ctx context.Context, projectID string) ([]graphGroup, error) {
	scope, err := x.descriptorOf(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("scope descriptor: %w", err)
	}
	return listAs[graphGroup](ctx, x, x.vssps("_apis/graph/groups?scopeDescriptor="+url.QueryEscape(scope)+"&api-version=7.1-preview.1"))
}

func (x *Ctx) identityOf(ctx context.Context, subject string) (identityRef, error) {
	var r struct {
		Value []identityRef `json:"value"`
	}
	if err := x.C.Get(ctx, x.vssps("_apis/identities?subjectDescriptors="+url.QueryEscape(subject)+"&queryMembership=None&api-version=7.1"), &r); err != nil {
		return identityRef{}, err
	}
	if len(r.Value) == 0 || r.Value[0].ID == "" {
		return identityRef{}, fmt.Errorf("no identity for %s", subject)
	}
	return r.Value[0], nil
}

// mapSubject records the three ways a group is referenced: graph subject
// descriptor (memberships), identity id (approvers, reviewers, role
// assignments) and identity descriptor (ACLs).
func (x *Ctx) mapSubject(ctx context.Context, srcSubject, tgtSubject string) error {
	if x.Dry() || srcSubject == "" || tgtSubject == "" {
		return nil
	}
	si, err := x.identityOf(ctx, srcSubject)
	if err != nil {
		return err
	}
	ti, err := x.identityOf(ctx, tgtSubject)
	if err != nil {
		return err
	}
	x.record("subject", srcSubject, tgtSubject)
	x.record("identity", strings.ToLower(si.ID), ti.ID)
	x.record("identityDescriptor", si.Descriptor, ti.Descriptor)
	return nil
}

// rebaseGroupName turns "[Src]\Group" into "[Tgt]\Group".
func (x *Ctx) rebaseGroupName(s string) string {
	p := "[" + x.Src + "]\\"
	if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
		return "[" + x.Tgt + "]\\" + s[len(p):]
	}
	return s
}

type team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func listTeams(ctx context.Context, x *Ctx, project string) ([]team, error) {
	var all []team
	for skip := 0; ; skip += 500 {
		page, err := valueOf[team](ctx, x, x.org(fmt.Sprintf("_apis/projects/%s/teams?$top=500&$skip=%d&api-version=7.1", url.PathEscape(project), skip)))
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < 500 {
			return all, nil
		}
	}
}

// CloneGroups maps every source project group to a target group with the same
// name, creating custom groups that don't exist yet. Team groups are left to
// the teams component. Memberships are copied later by security.
func CloneGroups(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	srcGroups, err := x.projectGroups(ctx, src.ID)
	if err != nil {
		return err
	}
	x.read(len(srcGroups))
	teams, err := listTeams(ctx, x, src.ID)
	if err != nil {
		return err
	}
	isTeam := map[string]bool{}
	for _, t := range teams {
		isTeam[fold(t.Name)] = true
	}
	if tgt.ID == "" {
		x.Log.Info("dry-run: target doesn't exist; would map or create groups", "count", len(srcGroups))
		return nil
	}
	tgtGroups, err := x.projectGroups(ctx, tgt.ID)
	if err != nil {
		return err
	}
	byName := map[string]graphGroup{}
	for _, g := range tgtGroups {
		byName[fold(g.DisplayName)] = g
	}
	scope, err := x.descriptorOf(ctx, tgt.ID)
	if err != nil {
		return err
	}
	for _, g := range srcGroups {
		if isTeam[fold(g.DisplayName)] {
			continue
		}
		if _, done := x.mapped("subject", g.Descriptor); done {
			continue
		}
		t, ok := byName[fold(g.DisplayName)]
		if !ok {
			body := map[string]string{"displayName": g.DisplayName, "description": g.Description}
			if err := x.C.Post(ctx, x.vssps("_apis/graph/groups?scopeDescriptor="+url.QueryEscape(scope)+"&api-version=7.1-preview.1"), body, &t); err != nil {
				x.fail("group "+g.DisplayName, err)
				continue
			}
			x.wrote()
		}
		if err := x.mapSubject(ctx, g.Descriptor, t.Descriptor); err != nil {
			x.fail("group "+g.DisplayName, err)
		}
	}
	return nil
}

// groupIDs holds every source project group's subject descriptor, identity id
// and identity descriptor. Anything in here that has no target mapping must
// not be copied into the target's permissions or memberships.
type groupIDs struct {
	subjects, ids, descriptors map[string]bool
}

func (x *Ctx) sourceGroups(ctx context.Context) (*groupIDs, error) {
	if x.srcGroups != nil {
		return x.srcGroups, nil
	}
	src, err := x.SrcProject(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := x.projectGroups(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	g := &groupIDs{map[string]bool{}, map[string]bool{}, map[string]bool{}}
	for _, gr := range groups {
		g.subjects[gr.Descriptor] = true
		if id, err := x.identityOf(ctx, gr.Descriptor); err == nil {
			g.ids[strings.ToLower(id.ID)] = true
			g.descriptors[strings.ToLower(id.Descriptor)] = true
		}
	}
	x.srcGroups = g
	return g, nil
}

func lowerKeys(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}

// mapDescriptor maps an identity descriptor (ACEs, feed permissions) to the
// target. Copied groups use the identityDescriptor map. Project-scoped service
// identities such as the project build service
// ("Microsoft.TeamFoundation.ServiceIdentity;{collection}:Build:{project}")
// are mapped by swapping the project GUID. Users and organization groups are
// kept. A source project group with no target equivalent returns false.
func (x *Ctx) mapDescriptor(d string, idd, gm map[string]string, sg *groupIDs) (string, bool) {
	if t, ok := idd[strings.ToLower(d)]; ok {
		return t, true
	}
	if sg.descriptors[strings.ToLower(d)] {
		return "", false
	}
	return jx.RemapGUIDs(d, gm), true
}

// mapMember maps a graph subject descriptor for a group membership: copied
// groups through the subject map, project service identities ("svc." +
// base64url of "{collection}:Build:{project}") by swapping the project GUID.
// A source project group with no target equivalent returns false.
func (x *Ctx) mapMember(d string, subj, gm map[string]string, sg *groupIDs) (string, bool) {
	if t, ok := subj[d]; ok {
		return t, true
	}
	if sg.subjects[d] {
		return "", false
	}
	if strings.HasPrefix(d, "svc.") {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(d[4:], "="))
		if err != nil {
			return d, true
		}
		mapped := jx.RemapGUIDs(string(raw), gm)
		if mapped != string(raw) {
			return "svc." + base64.RawURLEncoding.EncodeToString([]byte(mapped)), true
		}
	}
	return d, true
}

// buildServiceID returns the identity id of a project's build service
// ("<Project> Build Service (<Org>)"), so role assignments granted to the
// source's build service can be granted to the target's.
func (x *Ctx) buildServiceID(ctx context.Context, project string) (string, error) {
	if id, ok := x.buildService[project]; ok {
		return id, nil
	}
	name := project + " Build Service (" + x.C.Org + ")"
	var r struct {
		Value []identityRef `json:"value"`
	}
	if err := x.C.Get(ctx, x.vssps("_apis/identities?searchFilter=General&filterValue="+url.QueryEscape(name)+"&queryMembership=None&api-version=7.1"), &r); err != nil {
		return "", err
	}
	if len(r.Value) == 0 {
		return "", fmt.Errorf("no identity named %q", name)
	}
	if x.buildService == nil {
		x.buildService = map[string]string{}
	}
	x.buildService[project] = r.Value[0].ID
	return r.Value[0].ID, nil
}

// sourceScoped reports a source-project identity by name: "[Src]\..." groups
// and "Src Build Service (Org)".
func (x *Ctx) sourceScoped(names ...string) bool {
	for _, n := range names {
		n = strings.ToLower(n)
		if strings.HasPrefix(n, strings.ToLower("["+x.Src+"]")) || strings.HasPrefix(n, strings.ToLower(x.Src+" Build Service")) {
			return true
		}
	}
	return false
}
