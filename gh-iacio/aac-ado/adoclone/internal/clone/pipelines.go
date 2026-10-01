package clone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
)

// CloneTaskGroups copies task groups, creating nested ones first.
func CloneTaskGroups(ctx context.Context, x *Ctx) error {
	all, err := valueOf[jx.M](ctx, x, x.src("_apis/distributedtask/taskgroups?api-version=7.1"))
	if err != nil {
		return err
	}
	var groups []jx.M
	for _, g := range all {
		if p, _ := g["preview"].(bool); !p {
			groups = append(groups, g)
		}
	}
	x.read(len(groups))
	byName := map[string]string{}
	if have, err := valueOf[jx.M](ctx, x, x.tgt("_apis/distributedtask/taskgroups?api-version=7.1")); err == nil {
		for _, g := range have {
			byName[fold(jx.Str(g, "name"))] = jx.Str(g, "id")
		}
	}
	for _, g := range orderTaskGroups(groups) {
		sid := jx.Str(g, "id")
		if _, done := x.mapped("taskGroup", sid); done {
			continue
		}
		if id, ok := byName[fold(jx.Str(g, "name"))]; ok {
			x.record("taskGroup", strings.ToLower(sid), id)
			continue
		}
		body := jx.M{}
		for _, k := range []string{"author", "category", "description", "friendlyName", "iconUrl", "inputs", "instanceNameFormat", "name", "runsOn", "tasks", "version"} {
			if v, ok := g[k]; ok {
				body[k] = v
			}
		}
		body = jx.RemapJSON(body, x.guidMap()) // nested task group IDs
		var created struct {
			ID string `json:"id"`
		}
		if err := x.C.Post(ctx, x.tgt("_apis/distributedtask/taskgroups?api-version=7.1"), body, &created); err != nil {
			x.fail("task group "+jx.Str(g, "name"), err)
			continue
		}
		x.wrote()
		x.record("taskGroup", strings.ToLower(sid), created.ID)
	}
	return nil
}

// orderTaskGroups puts task groups after the task groups they use.
func orderTaskGroups(groups []jx.M) []jx.M {
	byID := map[string]jx.M{}
	for _, g := range groups {
		byID[fold(jx.Str(g, "id"))] = g
	}
	var out []jx.M
	seen := map[string]bool{}
	var visit func(g jx.M)
	visit = func(g jx.M) {
		id := fold(jx.Str(g, "id"))
		if seen[id] {
			return
		}
		seen[id] = true
		for _, t := range jx.Arr(g, "tasks") {
			if jx.Str(t, "task", "definitionType") == "metaTask" {
				if dep, ok := byID[fold(jx.Str(t, "task", "id"))]; ok {
					visit(dep)
				}
			}
		}
		out = append(out, g)
	}
	for _, g := range groups {
		visit(g)
	}
	return out
}

type defRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func defKey(path, name string) string { return fold(path + "\\" + name) }

// ClonePipelines copies build folders and definitions (YAML and classic).
// Build-completion triggers that point at definitions created later are set
// in a second pass.
func ClonePipelines(ctx context.Context, x *Ctx) error {
	type folder struct {
		Path        string `json:"path"`
		Description string `json:"description"`
	}
	folders, err := valueOf[folder](ctx, x, x.src("_apis/build/folders?api-version=7.1-preview.2"))
	if err != nil {
		return err
	}
	haveFolder := map[string]bool{}
	if have, err := valueOf[folder](ctx, x, x.tgt("_apis/build/folders?api-version=7.1-preview.2")); err == nil {
		for _, f := range have {
			haveFolder[fold(f.Path)] = true
		}
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Path < folders[j].Path })
	for _, f := range folders {
		if f.Path == "\\" || haveFolder[fold(f.Path)] {
			continue
		}
		if err := x.C.Put(ctx, x.tgt("_apis/build/folders?path="+url.QueryEscape(f.Path)+"&api-version=7.1-preview.2"), f, nil); err != nil {
			x.fail("build folder "+f.Path, err)
		}
	}

	defs, err := listAs[defRef](ctx, x, x.src("_apis/build/definitions?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(defs))
	have := map[string]int{}
	if tdefs, err := listAs[defRef](ctx, x, x.tgt("_apis/build/definitions?api-version=7.1")); err == nil {
		for _, d := range tdefs {
			have[defKey(d.Path, d.Name)] = d.ID
		}
	}
	gm := x.guidMap()
	for _, d := range defs {
		if _, done := x.mappedInt("buildDef", d.ID); done {
			continue
		}
		if id, ok := have[defKey(d.Path, d.Name)]; ok {
			x.record("buildDef", itoa(d.ID), itoa(id))
			continue
		}
		var full jx.M
		if err := x.C.Get(ctx, x.src(fmt.Sprintf("_apis/build/definitions/%d?api-version=7.1", d.ID)), &full); err != nil {
			x.fail("pipeline "+d.Name, err)
			continue
		}
		body, deferred := x.transformBuildDef(full, gm)
		var created defRef
		if err := x.C.Post(ctx, x.tgt("_apis/build/definitions?api-version=7.1"), body, &created); err != nil {
			x.fail("pipeline "+d.Name, err)
			continue
		}
		x.wrote()
		x.record("buildDef", itoa(d.ID), itoa(created.ID))
		if len(deferred) > 0 {
			b, _ := json.Marshal(deferred)
			x.record("pendingTrigger", itoa(created.ID), string(b))
		}
	}
	x.applyPendingTriggers(ctx)
	return nil
}

// applyPendingTriggers adds build-completion triggers whose upstream pipeline
// didn't exist yet when their pipeline was created. Pending triggers live in
// the checkpoint, so a later run (or rerun) finishes the job.
func (x *Ctx) applyPendingTriggers(ctx context.Context) {
	for tgtID, js := range x.S.Map("pendingTrigger") {
		if js == "-" {
			continue
		}
		var triggers []any
		if err := json.Unmarshal([]byte(js), &triggers); err != nil {
			continue
		}
		var ready, waiting []any
		for _, t := range triggers {
			id, _ := jx.Int(t, "definition", "id")
			if nid, ok := x.mappedInt("buildDef", id); ok {
				jx.Set(t.(jx.M), nid, "definition", "id")
				ready = append(ready, t)
			} else {
				waiting = append(waiting, t)
			}
		}
		if len(ready) > 0 {
			var cur jx.M
			u := x.tgt("_apis/build/definitions/" + tgtID + "?api-version=7.1")
			if err := x.C.Get(ctx, u, &cur); err != nil {
				x.fail("pipeline triggers "+tgtID, err)
				continue
			}
			cur["triggers"] = append(jx.Arr(cur, "triggers"), ready...)
			if err := x.C.Put(ctx, u, cur, nil); err != nil {
				x.fail("pipeline triggers "+tgtID, err)
				continue
			}
		}
		if len(waiting) > 0 {
			b, _ := json.Marshal(waiting)
			x.record("pendingTrigger", tgtID, string(b))
			x.todo("pipeline "+tgtID+" build-completion trigger", "its upstream pipeline wasn't copied; rerun pipelines once it is")
		} else {
			x.record("pendingTrigger", tgtID, "-")
		}
	}
}

// transformBuildDef strips server-owned fields and points repository, queue,
// variable group and task group references at the target. Build-completion
// triggers on definitions that aren't mapped yet are returned separately.
func (x *Ctx) transformBuildDef(full jx.M, gm map[string]string) (jx.M, []any) {
	d := jx.Copy(full)
	jx.Del(d, "id", "revision", "url", "uri", "_links", "project", "createdDate", "authoredBy",
		"latestBuild", "latestCompletedBuild", "metrics", "drafts", "draftOf")
	if repo := jx.Obj(d, "repository"); repo != nil && strings.EqualFold(jx.Str(repo, "type"), "TfsGit") {
		if t, ok := x.mapped("repo", jx.Str(repo, "id")); ok {
			repo["id"] = t
		}
		if n, ok := x.mapped("repoName", jx.Str(repo, "name")); ok {
			repo["name"] = n
		}
		delete(repo, "url")
	}
	x.mapQueue(jx.Obj(d, "queue"))
	for _, ph := range jx.Arr(d, "process", "phases") {
		x.mapQueue(jx.Obj(ph, "target", "queue"))
	}
	for _, vg := range jx.Arr(d, "variableGroups") {
		if m, ok := vg.(jx.M); ok {
			if id, ok := jx.Int(m, "id"); ok {
				if t, ok := x.mappedInt("vargroup", id); ok {
					m["id"] = t
				}
			}
		}
	}
	srcID := ""
	if x.srcProj != nil {
		srcID = x.srcProj.ID
	}
	var keep, deferred []any
	for _, t := range jx.Arr(d, "triggers") {
		if jx.Str(t, "triggerType") == "buildCompletion" {
			if p := jx.Str(t, "definition", "project", "id"); p != "" && srcID != "" && !strings.EqualFold(p, srcID) {
				keep = append(keep, t) // upstream pipeline in another project: unchanged
				continue
			}
			id, _ := jx.Int(t, "definition", "id")
			if nid, ok := x.mappedInt("buildDef", id); ok {
				jx.Set(t.(jx.M), nid, "definition", "id")
			} else {
				deferred = append(deferred, t)
				continue
			}
		}
		keep = append(keep, t)
	}
	if d["triggers"] != nil {
		d["triggers"] = keep
	}
	if deferred != nil {
		deferred = jx.RemapJSON(deferred, gm) // project IDs inside the trigger
	}
	return jx.RemapJSON(d, gm), deferred
}

func (x *Ctx) mapQueue(q jx.M) {
	if q == nil {
		return
	}
	if id, ok := jx.Int(q, "id"); ok {
		if t, ok := x.mappedInt("queue", id); ok {
			q["id"] = t
		}
	}
	jx.Del(q, "_links", "url")
}

// CloneReleases copies release folders and classic release definitions.
func CloneReleases(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	type folder struct {
		Path        string `json:"path"`
		Description string `json:"description"`
	}
	folders, err := valueOf[folder](ctx, x, x.srcRM("_apis/release/folders?api-version=7.1"))
	if err != nil {
		return err
	}
	haveFolder := map[string]bool{}
	if have, err := valueOf[folder](ctx, x, x.tgtRM("_apis/release/folders?api-version=7.1")); err == nil {
		for _, f := range have {
			haveFolder[fold(f.Path)] = true
		}
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Path < folders[j].Path })
	for _, f := range folders {
		if f.Path == "\\" || haveFolder[fold(f.Path)] {
			continue
		}
		seg := url.PathEscape(strings.TrimPrefix(f.Path, "\\"))
		if err := x.C.Post(ctx, x.tgtRM("_apis/release/folders/"+seg+"?api-version=7.1"), f, nil); err != nil {
			x.fail("release folder "+f.Path, err)
		}
	}
	defs, err := listAs[defRef](ctx, x, x.srcRM("_apis/release/definitions?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(defs))
	have := map[string]int{}
	if tdefs, err := listAs[defRef](ctx, x, x.tgtRM("_apis/release/definitions?api-version=7.1")); err == nil {
		for _, d := range tdefs {
			have[defKey(d.Path, d.Name)] = d.ID
		}
	}
	gm := x.guidMap()
	for _, d := range defs {
		if _, done := x.mappedInt("releaseDef", d.ID); done {
			continue
		}
		if id, ok := have[defKey(d.Path, d.Name)]; ok {
			x.record("releaseDef", itoa(d.ID), itoa(id))
			continue
		}
		var full jx.M
		if err := x.C.Get(ctx, x.srcRM(fmt.Sprintf("_apis/release/definitions/%d?api-version=7.1", d.ID)), &full); err != nil {
			x.fail("release "+d.Name, err)
			continue
		}
		body := x.transformReleaseDef(full, gm, src, tgt)
		var created jx.M
		if err := x.C.Post(ctx, x.tgtRM("_apis/release/definitions?api-version=7.1"), body, &created); err != nil {
			x.fail("release "+d.Name, err)
			continue
		}
		x.wrote()
		if id, ok := jx.Int(created, "id"); ok {
			x.record("releaseDef", itoa(d.ID), itoa(id))
		}
		tgtEnv := map[string]int{}
		for _, e := range jx.Arr(created, "environments") {
			if id, ok := jx.Int(e, "id"); ok {
				tgtEnv[fold(jx.Str(e, "name"))] = id
			}
		}
		for _, e := range jx.Arr(full, "environments") {
			sid, _ := jx.Int(e, "id")
			if tid, ok := tgtEnv[fold(jx.Str(e, "name"))]; ok {
				x.record("releaseEnv", itoa(sid), itoa(tid))
			}
		}
	}
	return nil
}

// transformReleaseDef strips server-owned fields, re-points artifacts at the
// copied build definitions and repos, and maps queues, deployment groups,
// variable groups and task groups.
func (x *Ctx) transformReleaseDef(full jx.M, gm map[string]string, src, tgt *Project) jx.M {
	d := jx.Copy(full)
	jx.Del(d, "id", "revision", "url", "_links", "createdBy", "createdOn", "modifiedBy", "modifiedOn",
		"lastRelease", "badgeUrl", "projectReference", "isDeleted")
	for _, a := range jx.Arr(d, "artifacts") {
		am, _ := a.(jx.M)
		ref := jx.Obj(am, "definitionReference")
		if ref == nil {
			continue
		}
		inSource := strings.EqualFold(jx.Str(ref, "project", "id"), src.ID)
		if inSource {
			jx.Set(ref, tgt.ID, "project", "id")
			jx.Set(ref, tgt.Name, "project", "name")
		}
		defID := jx.Str(ref, "definition", "id")
		switch jx.Str(am, "type") {
		case "Build":
			if t, ok := x.mapped("buildDef", defID); ok && inSource {
				jx.Set(ref, t, "definition", "id")
				am["sourceId"] = tgt.ID + ":" + t
			}
		case "Git":
			if t, ok := x.mapped("repo", defID); ok && inSource {
				jx.Set(ref, t, "definition", "id")
				if n, ok := x.mapped("repoName", jx.Str(ref, "definition", "name")); ok {
					jx.Set(ref, n, "definition", "name")
				}
				am["sourceId"] = tgt.ID + ":" + t
			}
		}
	}
	d["variableGroups"] = x.mapIntList(jx.Arr(d, "variableGroups"), "vargroup")
	for _, e := range jx.Arr(d, "environments") {
		env, _ := e.(jx.M)
		if env == nil {
			continue
		}
		env["id"] = 0
		jx.Del(env, "currentRelease", "badgeUrl")
		env["variableGroups"] = x.mapIntList(jx.Arr(env, "variableGroups"), "vargroup")
		for _, k := range []string{"preDeployApprovals", "postDeployApprovals"} {
			for _, ap := range jx.Arr(env, k, "approvals") {
				if m, ok := ap.(jx.M); ok {
					m["id"] = 0
				}
			}
		}
		for _, k := range []string{"preDeploymentGates", "postDeploymentGates", "deployStep"} {
			if m := jx.Obj(env, k); m != nil {
				m["id"] = 0
			}
		}
		for _, ph := range jx.Arr(env, "deployPhases") {
			di := jx.Obj(ph, "deploymentInput")
			if di == nil {
				continue
			}
			kind := "queue"
			if jx.Str(ph, "phaseType") == "machineGroupBasedDeployment" {
				kind = "deploymentGroup"
			}
			if id, ok := jx.Int(di, "queueId"); ok && id > 0 {
				if t, ok := x.mappedInt(kind, id); ok {
					di["queueId"] = t
				}
			}
		}
	}
	return jx.RemapJSON(d, gm)
}

func (x *Ctx) mapIntList(list []any, kind string) []any {
	out := make([]any, 0, len(list))
	for _, v := range list {
		if f, ok := v.(float64); ok {
			if t, ok := x.mappedInt(kind, int(f)); ok {
				out = append(out, t)
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

// CloneChecks copies approvals and checks on the resources that were copied.
// Shared service connections (and shared variable groups) are the same
// resource in both projects, so their checks already apply and are skipped.
func CloneChecks(ctx context.Context, x *Ctx) error {
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	type res struct{ typ, sid, tid string }
	var list []res
	add := func(typ, kind string, skipSame bool) {
		for s, t := range x.S.Map(kind) {
			if skipSame && s == t {
				continue
			}
			list = append(list, res{typ, s, t})
		}
	}
	add("environment", "environment", false)
	add("queue", "queue", false)
	add("variablegroup", "vargroup", true)
	add("securefile", "secureFile", false)
	for s, t := range x.S.Map("repo") {
		list = append(list, res{"repository", src.ID + "." + s, tgt.ID + "." + t})
	}
	gm := x.guidMap()
	for _, r := range list {
		q := "_apis/pipelines/checks/configurations?resourceType=" + r.typ + "&resourceId=%s&$expand=settings&api-version=7.1-preview.1"
		checks, err := valueOf[jx.M](ctx, x, x.src(fmt.Sprintf(q, url.QueryEscape(r.sid))))
		if err != nil {
			x.fail("checks on "+r.typ+" "+r.sid, err)
			continue
		}
		x.read(len(checks))
		if len(checks) == 0 {
			continue
		}
		have := map[string]bool{}
		if existing, err := valueOf[jx.M](ctx, x, x.tgt(fmt.Sprintf(q, url.QueryEscape(r.tid)))); err == nil {
			for _, c := range existing {
				have[checkSig(c["type"], c["settings"])] = true
			}
		}
		for _, c := range checks {
			settings := jx.RemapJSON(c["settings"], gm)
			if have[checkSig(c["type"], settings)] {
				continue
			}
			body := jx.M{"type": c["type"], "settings": settings, "timeout": c["timeout"],
				"resource": jx.M{"type": r.typ, "id": r.tid}}
			if err := x.C.Post(ctx, x.tgt("_apis/pipelines/checks/configurations?api-version=7.1-preview.1"), body, nil); err != nil {
				x.fail("check "+jx.Str(c, "type", "name")+" on "+r.typ+" "+r.sid, err)
				continue
			}
			x.wrote()
		}
	}
	return nil
}

// checkSig identifies a check for rerun detection. Display fields (names,
// descriptors, links) are left out because the server fills them in for the
// target and they'd never match the source's.
func checkSig(typ, settings any) string {
	b, _ := json.Marshal(stripVolatile(settings))
	return fold(jx.Str(typ, "id")) + string(b)
}

var volatileKeys = map[string]bool{"displayname": true, "uniquename": true, "descriptor": true, "imageurl": true, "url": true, "_links": true}

func stripVolatile(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			if !volatileKeys[fold(k)] {
				out[k] = stripVolatile(e)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = stripVolatile(e)
		}
		return out
	}
	return v
}

// ClonePermissions copies pipeline authorizations: which pipelines may use
// each queue, service connection, variable group, secure file, environment
// and repo.
func ClonePermissions(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	type res struct{ typ, sid, tid string }
	var list []res
	for typ, kind := range map[string]string{"queue": "queue", "endpoint": "endpoint", "variablegroup": "vargroup", "securefile": "secureFile", "environment": "environment"} {
		for s, t := range x.S.Map(kind) {
			list = append(list, res{typ, s, t})
		}
	}
	for s, t := range x.S.Map("repo") {
		list = append(list, res{"repository", src.ID + "." + s, tgt.ID + "." + t})
	}
	for _, r := range list {
		var p struct {
			AllPipelines *struct {
				Authorized bool `json:"authorized"`
			} `json:"allPipelines"`
			Pipelines []struct {
				ID         int  `json:"id"`
				Authorized bool `json:"authorized"`
			} `json:"pipelines"`
		}
		path := "_apis/pipelines/pipelinepermissions/" + r.typ + "/"
		if err := x.C.Get(ctx, x.src(path+url.PathEscape(r.sid)+"?api-version=7.1-preview.1"), &p); err != nil {
			if !client.IsNotFound(err) {
				x.fail("pipeline permissions on "+r.typ+" "+r.sid, err)
			}
			continue
		}
		x.read(1)
		body := jx.M{}
		if p.AllPipelines != nil && p.AllPipelines.Authorized {
			body["allPipelines"] = jx.M{"authorized": true}
		}
		var pl []any
		for _, pp := range p.Pipelines {
			if t, ok := x.mappedInt("buildDef", pp.ID); ok && pp.Authorized {
				pl = append(pl, jx.M{"id": t, "authorized": true})
			}
		}
		if len(pl) == 0 && body["allPipelines"] == nil {
			continue
		}
		body["pipelines"] = pl
		if err := x.C.Patch(ctx, x.tgt(path+url.PathEscape(r.tid)+"?api-version=7.1-preview.1"), body, nil); err != nil {
			x.fail("pipeline permissions on "+r.typ+" "+r.sid, err)
			continue
		}
		x.wrote()
	}
	return nil
}

const buildValidationPolicy = "0609b952-1397-4640-95ec-e00a01b2c241"

// ClonePolicies copies branch and repository policies. Policies scoped to a
// repo that wasn't copied, or validating a pipeline that wasn't copied, are
// skipped and listed in todo.json.
func ClonePolicies(ctx context.Context, x *Ctx) error {
	pols, err := listAs[jx.M](ctx, x, x.src("_apis/policy/configurations?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(pols))
	have := map[string]bool{}
	if existing, err := listAs[jx.M](ctx, x, x.tgt("_apis/policy/configurations?api-version=7.1")); err == nil {
		for _, p := range existing {
			have[policySig(p)] = true
		}
	}
	gm := x.guidMap()
	for _, p := range pols {
		if del, _ := p["isDeleted"].(bool); del {
			continue
		}
		body, why := x.transformPolicy(p, gm)
		item := "policy " + jx.Str(p, "type", "displayName") + " #" + jx.Str(p, "id")
		if body == nil {
			x.todo(item, why)
			continue
		}
		if have[policySig(body)] {
			continue
		}
		var created jx.M
		if err := x.C.Post(ctx, x.tgt("_apis/policy/configurations?api-version=7.1"), body, &created); err != nil {
			x.fail(item, err)
			continue
		}
		x.wrote()
		x.record("policy", jx.Str(p, "id"), jx.Str(created, "id"))
	}
	return nil
}

func (x *Ctx) transformPolicy(p jx.M, gm map[string]string) (jx.M, string) {
	settings := jx.Copy(jx.Obj(p, "settings"))
	for _, sc := range jx.Arr(settings, "scope") {
		if rid := jx.Str(sc, "repositoryId"); rid != "" {
			if _, ok := x.mapped("repo", rid); !ok {
				return nil, "its repo (" + rid + ") wasn't copied"
			}
		}
	}
	typeID := jx.Str(p, "type", "id")
	if strings.EqualFold(typeID, buildValidationPolicy) {
		id, _ := jx.Int(settings, "buildDefinitionId")
		t, ok := x.mappedInt("buildDef", id)
		if !ok {
			return nil, "its build validation pipeline (" + itoa(id) + ") wasn't copied"
		}
		settings["buildDefinitionId"] = t
	}
	return jx.M{
		"isEnabled":  p["isEnabled"],
		"isBlocking": p["isBlocking"],
		"type":       jx.M{"id": typeID},
		"settings":   jx.RemapJSON(settings, gm),
	}, ""
}

func policySig(p jx.M) string {
	b, _ := json.Marshal(p["settings"])
	return fold(jx.Str(p, "type", "id")) + string(b)
}

// CloneSettings copies project pipeline settings and retention.
func CloneSettings(ctx context.Context, x *Ctx) error {
	var gs jx.M
	if err := x.C.Get(ctx, x.src("_apis/build/generalsettings?api-version=7.1"), &gs); err != nil {
		return err
	}
	x.read(1)
	if err := x.C.Patch(ctx, x.tgt("_apis/build/generalsettings?api-version=7.1"), gs, nil); err != nil {
		x.fail("pipeline general settings", err)
	} else {
		x.wrote()
	}
	var rs jx.M
	if err := x.C.Get(ctx, x.src("_apis/build/retention?api-version=7.1"), &rs); err != nil {
		return err
	}
	body := jx.M{}
	for from, to := range map[string]string{
		"purgeArtifacts":               "artifactsRetention",
		"purgePullRequestRuns":         "pullRequestRunRetention",
		"purgeRuns":                    "runRetention",
		"retainRunsPerProtectedBranch": "retainRunsPerProtectedBranch",
	} {
		if v, ok := jx.Int(rs, from, "value"); ok {
			body[to] = jx.M{"value": v}
		}
	}
	if err := x.C.Patch(ctx, x.tgt("_apis/build/retention?api-version=7.1"), body, nil); err != nil {
		x.fail("pipeline retention", err)
	} else {
		x.wrote()
	}
	return nil
}
