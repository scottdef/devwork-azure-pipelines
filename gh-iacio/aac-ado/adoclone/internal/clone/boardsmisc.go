package clone

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
)

// Queries, dashboards, delivery plans and wikis.

type query struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	IsFolder    bool    `json:"isFolder"`
	HasChildren bool    `json:"hasChildren"`
	Wiql        string  `json:"wiql"`
	Children    []query `json:"children"`
}

func (x *Ctx) getQuery(ctx context.Context, project, idOrPath string, depth int) (query, error) {
	var q query
	u := x.C.URL("", client.P(project)+fmt.Sprintf("/_apis/wit/queries/%s?$depth=%d&$expand=all&api-version=7.1", idOrPath, depth))
	err := x.C.Get(ctx, u, &q)
	return q, err
}

// rewriteWIQL points project-name references (project literal, area and
// iteration paths, [Project]\Group names) at the target.
func (x *Ctx) rewriteWIQL(w string) string {
	w = jx.ReplaceFold(w, "'"+x.Src+"\\", "'"+x.Tgt+"\\")
	w = jx.ReplaceFold(w, "'"+x.Src+"'", "'"+x.Tgt+"'")
	w = jx.ReplaceFold(w, "["+x.Src+"]\\", "["+x.Tgt+"]\\")
	return w
}

// CloneQueries copies the Shared Queries tree (folders and queries). Personal
// "My Queries" belong to each user and can't be created for them.
func CloneQueries(ctx context.Context, x *Ctx) error {
	root, err := x.getQuery(ctx, x.Src, "Shared%20Queries", 2)
	if err != nil {
		return err
	}
	tgtRootID := ""
	if t, err := x.getQuery(ctx, x.Tgt, "Shared%20Queries", 1); err == nil {
		tgtRootID = t.ID
		x.record("query", strings.ToLower(root.ID), t.ID)
	} else if !(x.Dry() && client.IsNotFound(err)) {
		return err
	}
	return x.copyQueryChildren(ctx, root, tgtRootID)
}

func (x *Ctx) copyQueryChildren(ctx context.Context, parent query, tgtParent string) error {
	if parent.HasChildren && parent.Children == nil {
		p, err := x.getQuery(ctx, x.Src, parent.ID, 2)
		if err != nil {
			return err
		}
		parent = p
	}
	have := map[string]query{}
	if tgtParent != "" {
		if t, err := x.getQuery(ctx, x.Tgt, tgtParent, 1); err == nil {
			for _, c := range t.Children {
				have[fold(c.Name)] = c
			}
		}
	}
	for _, c := range parent.Children {
		x.read(1)
		t, ok := have[fold(c.Name)]
		if !ok {
			body := jx.M{"name": c.Name}
			if c.IsFolder {
				body["isFolder"] = true
			} else {
				body["wiql"] = x.rewriteWIQL(c.Wiql)
			}
			if err := x.C.Post(ctx, x.tgt("_apis/wit/queries/"+tgtParent+"?api-version=7.1"), body, &t); err != nil {
				x.fail("query "+c.Name, err)
				continue
			}
			x.wrote()
		}
		x.record("query", strings.ToLower(c.ID), t.ID)
		if c.IsFolder {
			if err := x.copyQueryChildren(ctx, c, t.ID); err != nil {
				x.fail("query folder "+c.Name, err)
			}
		}
	}
	return nil
}

// CloneDashboards copies project dashboards and each mapped team's dashboards
// with their widgets. Widget settings are rewritten: every mapped GUID
// (queries, teams, repos, project), build and release definition IDs, project
// names and area paths.
func CloneDashboards(ctx context.Context, x *Ctx) error {
	gm := x.guidMap()
	type scope struct{ src, tgt, want string }
	scopes := []scope{{"", "", "project"}}
	for s, t := range x.S.Map("team") {
		scopes = append(scopes, scope{s, t, "project_Team"})
	}
	const ver = "api-version=7.1-preview.3"
	for _, sc := range scopes {
		sbase, tbase := x.src("_apis/dashboard/dashboards"), x.tgt("_apis/dashboard/dashboards")
		if sc.src != "" {
			sbase, tbase = x.src(sc.src+"/_apis/dashboard/dashboards"), x.tgt(sc.tgt+"/_apis/dashboard/dashboards")
		}
		list, err := valueOf[jx.M](ctx, x, sbase+"?"+ver)
		if err != nil {
			x.fail("dashboards "+sc.want+" "+sc.src, err)
			continue
		}
		have := map[string]string{}
		if t, err := valueOf[jx.M](ctx, x, tbase+"?"+ver); err == nil {
			for _, d := range t {
				have[fold(jx.Str(d, "name"))] = jx.Str(d, "id")
			}
		}
		for _, d := range list {
			if s := jx.Str(d, "dashboardScope"); s != "" && !strings.EqualFold(s, sc.want) {
				continue
			}
			did := jx.Str(d, "id")
			if _, done := x.mapped("dashboard", did); done {
				continue
			}
			x.read(1)
			if id, ok := have[fold(jx.Str(d, "name"))]; ok {
				x.record("dashboard", strings.ToLower(did), id)
				continue
			}
			var full jx.M
			if err := x.C.Get(ctx, sbase+"/"+did+"?"+ver, &full); err != nil {
				x.fail("dashboard "+jx.Str(d, "name"), err)
				continue
			}
			body := jx.M{}
			for _, k := range []string{"name", "description", "refreshInterval", "position", "dashboardScope"} {
				if v, ok := full[k]; ok {
					body[k] = v
				}
			}
			body["widgets"] = x.transformWidgets(jx.Arr(full, "widgets"), gm)
			var created jx.M
			if err := x.C.Post(ctx, tbase+"?"+ver, body, &created); err != nil {
				x.fail("dashboard "+jx.Str(d, "name"), err)
				continue
			}
			x.wrote()
			x.record("dashboard", strings.ToLower(did), jx.Str(created, "id"))
		}
	}
	return nil
}

func (x *Ctx) transformWidgets(widgets []any, gm map[string]string) []any {
	out := make([]any, 0, len(widgets))
	for _, w := range widgets {
		c, ok := jx.Copy(w).(jx.M)
		if !ok {
			continue
		}
		jx.Del(c, "id", "eTag", "url", "_links", "dashboard")
		if s, ok := c["settings"].(string); ok {
			c["settings"] = x.remapWidgetSettings(s, gm)
		}
		out = append(out, c)
	}
	return out
}

// remapWidgetSettings rewrites a widget's settings JSON string.
func (x *Ctx) remapWidgetSettings(s string, gm map[string]string) string {
	var v any
	if s == "" || json.Unmarshal([]byte(s), &v) != nil {
		return jx.RemapGUIDs(s, gm)
	}
	bd, rd := x.S.Map("buildDef"), x.S.Map("releaseDef")
	jx.Walk(v, func(parent jx.M, key string, val any) (any, bool) {
		lk := fold(key)
		switch {
		case lk == "builddefinitionid" || lk == "pipelineid":
			return mapNumber(val, bd)
		case lk == "releasedefinitionid":
			return mapNumber(val, rd)
		case lk == "builddefinition":
			if m, ok := val.(jx.M); ok {
				if nv, ok := mapNumber(m["id"], bd); ok {
					m["id"] = nv
				}
			}
		}
		if str, ok := val.(string); ok {
			if strings.Contains(lk, "project") && strings.EqualFold(str, x.Src) {
				return x.Tgt, true
			}
			if strings.Contains(lk, "path") {
				return jx.RebasePath(str, x.Src, x.Tgt), true
			}
		}
		return nil, false
	})
	b, err := json.Marshal(v)
	if err != nil {
		return jx.RemapGUIDs(s, gm)
	}
	return jx.RemapGUIDs(string(b), gm)
}

// mapNumber maps a numeric ID (number or numeric string) and keeps its JSON type.
func mapNumber(val any, m map[string]string) (any, bool) {
	switch n := val.(type) {
	case float64:
		if t, ok := m[itoa(int(n))]; ok {
			return float64(atoi(t)), true
		}
	case string:
		if t, ok := m[n]; ok {
			return t, true
		}
	}
	return nil, false
}

// CloneDeliveryPlans copies delivery plans. Team rows are mapped to the copied
// teams; rows for teams in other projects stay as they are.
func CloneDeliveryPlans(ctx context.Context, x *Ctx) error {
	plans, err := valueOf[jx.M](ctx, x, x.src("_apis/work/plans?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(plans))
	have := map[string]string{}
	if t, err := valueOf[jx.M](ctx, x, x.tgt("_apis/work/plans?api-version=7.1")); err == nil {
		for _, p := range t {
			have[fold(jx.Str(p, "name"))] = jx.Str(p, "id")
		}
	}
	gm := x.guidMap()
	for _, p := range plans {
		pid := jx.Str(p, "id")
		if _, done := x.mapped("deliveryPlan", pid); done {
			continue
		}
		if id, ok := have[fold(jx.Str(p, "name"))]; ok {
			x.record("deliveryPlan", pid, id)
			continue
		}
		var full jx.M
		if err := x.C.Get(ctx, x.src("_apis/work/plans/"+pid+"?api-version=7.1"), &full); err != nil {
			x.fail("delivery plan "+jx.Str(p, "name"), err)
			continue
		}
		props := jx.Copy(jx.Obj(full, "properties"))
		for _, c := range jx.Arr(props, "criteria") {
			if m, ok := c.(jx.M); ok {
				if f := jx.Str(m, "fieldName"); f == "System.AreaPath" || f == "System.IterationPath" {
					m["value"] = jx.RebasePath(jx.Str(m, "value"), x.Src, x.Tgt)
				}
			}
		}
		body := jx.M{"name": full["name"], "description": full["description"], "type": full["type"], "properties": jx.RemapJSON(props, gm)}
		var created jx.M
		if err := x.C.Post(ctx, x.tgt("_apis/work/plans?api-version=7.1"), body, &created); err != nil {
			x.fail("delivery plan "+jx.Str(p, "name"), err)
			continue
		}
		x.wrote()
		x.record("deliveryPlan", pid, jx.Str(created, "id"))
	}
	return nil
}

type wiki struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	RemoteURL    string `json:"remoteUrl"`
	RepositoryID string `json:"repositoryId"`
	MappedPath   string `json:"mappedPath"`
	Versions     []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

// CloneWiki copies the project wiki's content (its hidden Git repo, branch
// wikiMaster; needs git) and re-creates code wikis on the copied repos.
func CloneWiki(ctx context.Context, x *Ctx) error {
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	wikis, err := valueOf[wiki](ctx, x, x.src("_apis/wiki/wikis?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(wikis))
	var have []wiki
	if tgt.ID != "" {
		if have, err = valueOf[wiki](ctx, x, x.tgt("_apis/wiki/wikis?api-version=7.1")); err != nil {
			return err
		}
	}
	find := func(typ, name string) (wiki, bool) {
		for _, w := range have {
			if w.Type == typ && (typ == "projectWiki" || strings.EqualFold(w.Name, name)) {
				return w, true
			}
		}
		return wiki{}, false
	}
	for _, w := range wikis {
		switch w.Type {
		case "projectWiki":
			t, ok := find("projectWiki", "")
			if !ok {
				body := jx.M{"type": "projectWiki", "name": x.Tgt + ".wiki", "projectId": tgt.ID}
				if err := x.C.Post(ctx, x.tgt("_apis/wiki/wikis?api-version=7.1"), body, &t); err != nil {
					x.fail("project wiki", err)
					continue
				}
				x.wrote()
			}
			if x.Dry() {
				continue
			}
			x.record("wiki", strings.ToLower(w.ID), t.ID)
			if done, _ := x.mapped("wikiContent", w.ID); done == "1" {
				continue
			}
			if _, err := exec.LookPath(x.Opt.Git); err != nil {
				x.todo("project wiki", "copying wiki pages needs git; run scripts/wiki-mirror.sh or rerun wiki where git is installed")
				continue
			}
			ref := "refs/heads/wikiMaster"
			if len(w.Versions) > 0 && w.Versions[0].Version != "" {
				ref = "refs/heads/" + w.Versions[0].Version
			}
			if err := x.pushBranch(ctx, w.RemoteURL, t.RemoteURL, ref); err != nil {
				x.fail("project wiki content", err)
				continue
			}
			x.record("wikiContent", w.ID, "1")
		case "codeWiki":
			if t, ok := find("codeWiki", w.Name); ok {
				x.record("wiki", strings.ToLower(w.ID), t.ID)
				continue
			}
			repo, ok := x.mapped("repo", w.RepositoryID)
			if !ok {
				x.todo("code wiki "+w.Name, "its repo wasn't copied; run repos first")
				continue
			}
			body := jx.M{"type": "codeWiki", "name": w.Name, "projectId": tgt.ID, "repositoryId": repo, "mappedPath": w.MappedPath}
			if len(w.Versions) > 0 {
				body["version"] = jx.M{"version": w.Versions[0].Version}
			}
			var t wiki
			if err := x.C.Post(ctx, x.tgt("_apis/wiki/wikis?api-version=7.1"), body, &t); err != nil {
				x.fail("code wiki "+w.Name, err)
				continue
			}
			x.wrote()
			x.record("wiki", strings.ToLower(w.ID), t.ID)
		}
	}
	return nil
}
