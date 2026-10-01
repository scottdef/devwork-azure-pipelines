package clone

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/coolado/adoclone/internal/jx"
)

// CloneSecurity copies group memberships, ACLs and role assignments. Run it
// last: it needs every ID map to rewrite ACL tokens.
//
// Nothing granted to a source-project identity reaches the target unless it
// maps to a target identity: copied groups map to their target groups, the
// source's build service maps to the target's, and anything else from the
// source project is dropped and listed in todo.json. ACLs are merged into the
// target's existing ACLs rather than replacing them.
func CloneSecurity(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	if tgt.ID == "" {
		x.Log.Info("dry-run: target doesn't exist; nothing to compare security against")
		return nil
	}
	if err := x.copyMemberships(ctx, src); err != nil {
		return err
	}
	if err := x.copyACLs(ctx, src); err != nil {
		return err
	}
	return x.copyRoleAssignments(ctx, src, tgt)
}

func (x *Ctx) copyMemberships(ctx context.Context, src *Project) error {
	groups, err := x.projectGroups(ctx, src.ID)
	if err != nil {
		return err
	}
	sg, err := x.sourceGroups(ctx)
	if err != nil {
		return err
	}
	subj, teams, gm := x.S.Map("subject"), x.S.Map("teamSubject"), x.guidMap()
	for _, g := range groups {
		tgtD, ok := subj[g.Descriptor]
		if !ok || teams[g.Descriptor] != "" || strings.EqualFold(g.DisplayName, "Project Valid Users") {
			continue // unmapped, a team (teams copies its members), or system-managed
		}
		var r struct {
			Value []struct {
				MemberDescriptor string `json:"memberDescriptor"`
			} `json:"value"`
		}
		if err := x.C.Get(ctx, x.vssps("_apis/graph/Memberships/"+g.Descriptor+"?direction=down&api-version=7.1-preview.1"), &r); err != nil {
			x.fail("members of "+g.DisplayName, err)
			continue
		}
		x.read(len(r.Value))
		for _, m := range r.Value {
			md, ok := x.mapMember(m.MemberDescriptor, subj, gm, sg)
			if !ok {
				x.todo("member "+m.MemberDescriptor+" of "+g.DisplayName, "a source project group with no target equivalent; add the right target group by hand")
				continue
			}
			if err := x.C.Put(ctx, x.vssps("_apis/graph/memberships/"+md+"/"+tgtD+"?api-version=7.1-preview.1"), nil, nil); err != nil {
				x.fail("member "+m.MemberDescriptor+" of "+g.DisplayName, err)
				continue
			}
			x.wrote()
		}
	}
	return nil
}

type ace struct {
	Descriptor string `json:"descriptor"`
	Allow      int    `json:"allow"`
	Deny       int    `json:"deny"`
}

type acl struct {
	Token              string         `json:"token"`
	InheritPermissions bool           `json:"inheritPermissions"`
	AcesDictionary     map[string]ace `json:"acesDictionary"`
}

type aclNamespace struct {
	name, id string
	numeric  string // map kind for a trailing numeric definition ID in the token
}

var aclNamespaces = []aclNamespace{
	{"Git Repositories", "2e9eb7ed-3c0a-47d4-87c1-0ffdd275fd87", ""},
	{"Project", "52d39943-cb85-4d7f-8fa8-c6baac873819", ""},
	{"CSS", "83e28ad4-2d72-4ceb-97b0-c7726d5502c3", ""},
	{"Iteration", "bf7bfa03-b2b7-47db-8113-fa2e002cc5b1", ""},
	{"Build", "33344d9c-fc72-4d6f-aba5-fa317101a7e9", "buildDef"},
	{"ReleaseManagement", "c788c23e-1b46-4162-8f5e-d7585343b5de", "releaseDef"},
	{"WorkItemQueryFolders", "71356614-aad7-4757-8f2c-0fb3bff6f680", ""},
	{"Identity", "5a27515b-ccd7-42c9-84f1-54c998f03866", ""},
}

func (x *Ctx) aclPrefixes(ctx context.Context, ns aclNamespace, src *Project) []string {
	p := src.ID
	switch ns.name {
	case "Git Repositories":
		return []string{"repoV2/" + p}
	case "Project":
		return []string{"$PROJECT:vstfs:///Classification/TeamProject/" + p}
	case "CSS", "Iteration":
		group := "Areas"
		if ns.name == "Iteration" {
			group = "Iterations"
		}
		root, err := getTree(ctx, x, x.Src, group, 0)
		if err != nil {
			x.fail(ns.name+" root node", err)
			return nil
		}
		return []string{"vstfs:///Classification/Node/" + root.Identifier}
	case "WorkItemQueryFolders":
		// The docs show "/{project}/..." and Microsoft's Terraform provider
		// uses "$/{project}/..."; read both.
		return []string{"$/" + p, "/" + p}
	default: // Build, ReleaseManagement, Identity
		return []string{p}
	}
}

func (x *Ctx) readACLs(ctx context.Context, nsID, token string) ([]acl, error) {
	var r struct {
		Value []acl `json:"value"`
	}
	u := x.org("_apis/accesscontrollists/" + nsID + "?token=" + url.QueryEscape(token) + "&recurse=true&includeExtendedInfo=false&api-version=7.1")
	err := x.C.Get(ctx, u, &r)
	return r.Value, err
}

// copyACLs reads every ACL under the source project's tokens in each
// namespace, rewrites tokens and identities, and merges the result into the
// target's ACL for the same token: the target's own entries (its build
// service, its default groups) stay, source entries for the same identity win.
// Tokens that mention something that wasn't copied are skipped.
func (x *Ctx) copyACLs(ctx context.Context, src *Project) error {
	gm := x.guidMap()
	idd := lowerKeys(x.S.Map("identityDescriptor"))
	sg, err := x.sourceGroups(ctx)
	if err != nil {
		return err
	}
	for _, ns := range aclNamespaces {
		var out []any
		seen := map[string]bool{}
		for _, prefix := range x.aclPrefixes(ctx, ns, src) {
			srcACLs, err := x.readACLs(ctx, ns.id, prefix)
			if err != nil {
				x.fail("ACLs in "+ns.name, err)
				continue
			}
			x.read(len(srcACLs))
			if len(srcACLs) == 0 {
				continue
			}
			tprefix, ok := x.rewriteToken(ns, prefix, gm)
			if !ok {
				continue
			}
			have := map[string]acl{}
			tgtACLs, err := x.readACLs(ctx, ns.id, tprefix)
			if err != nil {
				x.fail("target ACLs in "+ns.name, err)
				continue
			}
			for _, a := range tgtACLs {
				have[strings.ToLower(a.Token)] = a
			}
			for _, a := range srcACLs {
				tok, ok := x.rewriteToken(ns, a.Token, gm)
				if !ok || seen[strings.ToLower(tok)] {
					continue
				}
				seen[strings.ToLower(tok)] = true
				merged, changed := mergeACEs(have[strings.ToLower(tok)], a, func(d string) (string, bool) {
					return x.mapDescriptor(d, idd, gm, sg)
				})
				cur, exists := have[strings.ToLower(tok)]
				if exists && cur.InheritPermissions != a.InheritPermissions {
					changed = true
				}
				if !changed || len(merged) == 0 {
					continue
				}
				out = append(out, acl{Token: tok, InheritPermissions: a.InheritPermissions, AcesDictionary: merged})
			}
		}
		for i := 0; i < len(out); i += 100 {
			j := i + 100
			if j > len(out) {
				j = len(out)
			}
			if err := x.C.Post(ctx, x.org("_apis/accesscontrollists/"+ns.id+"?api-version=7.1"), jx.M{"value": out[i:j]}, nil); err != nil {
				x.fail("ACLs in "+ns.name, err)
				continue
			}
			for range out[i:j] {
				x.wrote()
			}
		}
	}
	return nil
}

// mergeACEs starts from the target's entries and applies the source's mapped
// entries on top. It reports whether anything changed.
func mergeACEs(target, source acl, mapDesc func(string) (string, bool)) (map[string]ace, bool) {
	merged := map[string]ace{}
	for _, e := range target.AcesDictionary {
		merged[strings.ToLower(e.Descriptor)] = e
	}
	changed := false
	for d, e := range source.AcesDictionary {
		nd, ok := mapDesc(d)
		if !ok {
			continue // a source project group with no target equivalent
		}
		e.Descriptor = nd
		if cur, ok := merged[strings.ToLower(nd)]; !ok || cur.Allow != e.Allow || cur.Deny != e.Deny {
			changed = true
		}
		merged[strings.ToLower(nd)] = e
	}
	out := make(map[string]ace, len(merged))
	for _, e := range merged {
		out[e.Descriptor] = e
	}
	return out, changed
}

// rewriteToken maps every GUID in a security token, plus definition IDs for
// Build and ReleaseManagement (including release stage tokens,
// {project}/{folders}/{definition}/Environment/{stage}). It reports false when
// the token refers to something that wasn't copied.
func (x *Ctx) rewriteToken(ns aclNamespace, token string, gm map[string]string) (string, bool) {
	for _, g := range jx.GUIDs(token) {
		if _, ok := gm[strings.ToLower(g)]; !ok {
			return "", false
		}
	}
	t := jx.RemapGUIDs(token, gm)
	if ns.numeric == "" {
		return t, true
	}
	parts := strings.Split(t, "/")
	n := len(parts)
	if ns.name == "ReleaseManagement" && n >= 4 && parts[n-2] == "Environment" {
		def, ok1 := x.mapped("releaseDef", parts[n-3])
		env, ok2 := x.mapped("releaseEnv", parts[n-1])
		if !ok1 || !ok2 {
			return "", false
		}
		parts[n-3], parts[n-1] = def, env
		return strings.Join(parts, "/"), true
	}
	if n > 1 {
		if _, err := strconv.Atoi(parts[n-1]); err == nil {
			id, ok := x.mapped(ns.numeric, parts[n-1])
			if !ok {
				return "", false
			}
			parts[n-1] = id
		}
	}
	return strings.Join(parts, "/"), true
}

// copyRoleAssignments copies explicit (not inherited) role assignments on the
// Library, variable groups, secure files, environments, agent queues and
// service connections.
func (x *Ctx) copyRoleAssignments(ctx context.Context, src, tgt *Project) error {
	type pair struct{ scope, srcRes, tgtRes string }
	s, t := src.ID, tgt.ID
	pairs := []pair{
		{"distributedtask.library", s + "$0", t + "$0"},
		{"distributedtask.project.serviceendpointrole", s, t},
		{"distributedtask.globalenvironmentreferencerole", s, t},
		{"distributedtask.agentqueuerole", s, t},
	}
	add := func(scope, kind, sep string) {
		for a, b := range x.S.Map(kind) {
			pairs = append(pairs, pair{scope, s + sep + a, t + sep + b})
		}
	}
	add("distributedtask.variablegroup", "vargroup", "$")
	add("distributedtask.securefile", "secureFile", "$")
	add("distributedtask.environmentreferencerole", "environment", "_")
	add("distributedtask.agentqueuerole", "queue", "_")
	add("distributedtask.serviceendpointrole", "endpoint", "_")

	ids := lowerKeys(x.S.Map("identity"))
	sg, err := x.sourceGroups(ctx)
	if err != nil {
		return err
	}
	esc := func(r string) string { return strings.ReplaceAll(url.PathEscape(r), "$", "%24") }
	for _, p := range pairs {
		var r struct {
			Value []struct {
				Identity struct {
					ID          string `json:"id"`
					DisplayName string `json:"displayName"`
					UniqueName  string `json:"uniqueName"`
				} `json:"identity"`
				Role struct {
					Name string `json:"name"`
				} `json:"role"`
				Access string `json:"access"`
			} `json:"value"`
		}
		base := x.org("_apis/securityroles/scopes/" + p.scope + "/roleassignments/resources/")
		if err := x.C.Get(ctx, base+esc(p.srcRes)+"?api-version=7.1-preview.1", &r); err != nil {
			x.fail("role assignments "+p.scope+" "+p.srcRes, err)
			continue
		}
		var body []any
		for _, a := range r.Value {
			if a.Access != "assigned" {
				continue
			}
			uid, ok := x.mapIdentityID(ctx, a.Identity.ID, a.Identity.DisplayName, a.Identity.UniqueName, ids, sg)
			if !ok {
				x.todo("role "+a.Role.Name+" for "+a.Identity.DisplayName+" on "+p.scope, "a source project identity with no target equivalent; grant the right target identity by hand")
				continue
			}
			body = append(body, jx.M{"roleName": a.Role.Name, "userId": uid})
		}
		x.read(len(body))
		if len(body) == 0 {
			continue
		}
		if err := x.C.Put(ctx, base+esc(p.tgtRes)+"?api-version=7.1-preview.1", body, nil); err != nil {
			x.fail("role assignments "+p.scope+" "+p.srcRes, err)
			continue
		}
		x.wrote()
	}
	return nil
}

// mapIdentityID maps an identity id for role assignments: copied groups via
// the identity map, the source build service to the target's, and other
// source-project identities are refused.
func (x *Ctx) mapIdentityID(ctx context.Context, id, display, unique string, ids map[string]string, sg *groupIDs) (string, bool) {
	if t, ok := ids[strings.ToLower(id)]; ok {
		return t, true
	}
	if strings.HasPrefix(strings.ToLower(display), strings.ToLower(x.Src+" Build Service")) {
		t, err := x.buildServiceID(ctx, x.Tgt)
		return t, err == nil
	}
	if sg.ids[strings.ToLower(id)] || x.sourceScoped(display, unique) {
		return "", false
	}
	return id, true
}
