package clone

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
)

// Pipeline resources: service connections, variable groups, secure files,
// agent queues, deployment groups and environments.

type projectRef struct {
	ProjectReference struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"projectReference"`
}

func refersTo(refs []projectRef, projectID string) bool {
	for _, r := range refs {
		if projectID != "" && strings.EqualFold(r.ProjectReference.ID, projectID) {
			return true
		}
	}
	return false
}

// CloneEndpoints shares every source service connection with the target. The
// connection keeps its ID and credentials; nothing secret is read or copied.
func CloneEndpoints(ctx context.Context, x *Ctx) error {
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	type endpoint struct {
		ID          string       `json:"id"`
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Refs        []projectRef `json:"serviceEndpointProjectReferences"`
	}
	eps, err := valueOf[endpoint](ctx, x, x.src("_apis/serviceendpoint/endpoints?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(eps))
	for _, e := range eps {
		if strings.HasPrefix(e.Name, "adoclone-import-") {
			continue
		}
		if !refersTo(e.Refs, tgt.ID) {
			body := []any{jx.M{
				"projectReference": jx.M{"id": tgt.ID, "name": tgt.Name},
				"name":             e.Name,
				"description":      e.Description,
			}}
			if err := x.C.Patch(ctx, x.org("_apis/serviceendpoint/endpoints/"+e.ID+"?api-version=7.1"), body, nil); err != nil {
				x.fail("service connection "+e.Name, err)
				continue
			}
			x.wrote()
			x.record("endpointShared", e.ID, "1") // cleanup only unshares what adoclone shared
		}
		x.record("endpoint", e.ID, e.ID)
	}
	return nil
}

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]+`)

// SecretEnv is the environment variable that supplies a secret variable's
// value when variable groups are copied: SECRET_<GROUP>_<VARIABLE>.
func SecretEnv(group, variable string) string {
	n := func(s string) string { return strings.Trim(nonAlnum.ReplaceAllString(strings.ToUpper(s), "_"), "_") }
	return "SECRET_" + n(group) + "_" + n(variable)
}

// CloneVarGroups copies variable groups (default) or shares them with the
// target (-vargroup-mode=share). Copied secrets come from SECRET_<GROUP>_<VAR>
// when set; otherwise they're left empty and listed in todo.json. Key
// Vault-linked groups need no secrets: they point at the same vault through
// the shared service connection.
func CloneVarGroups(ctx context.Context, x *Ctx) error {
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	type group struct {
		ID           int             `json:"id"`
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		Type         string          `json:"type"`
		Variables    map[string]jx.M `json:"variables"`
		ProviderData any             `json:"providerData"`
		Refs         []projectRef    `json:"variableGroupProjectReferences"`
	}
	groups, err := valueOf[group](ctx, x, x.src("_apis/distributedtask/variablegroups?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(groups))
	byName := map[string]int{}
	if tgt.ID != "" && x.Opt.VarGroupMode == "copy" {
		have, err := valueOf[group](ctx, x, x.tgt("_apis/distributedtask/variablegroups?api-version=7.1"))
		if err != nil {
			return err
		}
		for _, g := range have {
			byName[fold(g.Name)] = g.ID
		}
	}
	ref := jx.M{"id": tgt.ID, "name": tgt.Name}
	for _, g := range groups {
		sid := itoa(g.ID)
		if _, done := x.mapped("vargroup", sid); done {
			continue
		}
		if x.Opt.VarGroupMode == "share" {
			if !refersTo(g.Refs, tgt.ID) {
				body := []any{jx.M{"projectReference": ref, "name": g.Name, "description": g.Description}}
				if err := x.C.Patch(ctx, x.org("_apis/distributedtask/variablegroups?variableGroupId="+sid+"&api-version=7.1"), body, nil); err != nil {
					x.fail("variable group "+g.Name, err)
					continue
				}
				x.wrote()
				x.record("vargroupShared", sid, "1")
			}
			x.record("vargroup", sid, sid)
			continue
		}
		if id, ok := byName[fold(g.Name)]; ok {
			x.record("vargroup", sid, itoa(id))
			continue
		}
		vars := jx.M{}
		for name, v := range g.Variables {
			nv := jx.Copy(v)
			if secret, _ := v["isSecret"].(bool); secret && !strings.EqualFold(g.Type, "AzureKeyVault") {
				env := SecretEnv(g.Name, name)
				if val := x.Getenv(env); val != "" {
					nv["value"] = val
				} else {
					nv["value"] = ""
					x.todo("variable group "+g.Name+" / "+name, "set the secret value in the target (or set "+env+" before copying)")
				}
			}
			vars[name] = nv
		}
		body := jx.M{
			"name": g.Name, "description": g.Description, "type": g.Type,
			"variables": vars, "providerData": g.ProviderData,
			"variableGroupProjectReferences": []any{jx.M{"projectReference": ref, "name": g.Name, "description": g.Description}},
		}
		var created struct {
			ID int `json:"id"`
		}
		if err := x.C.Post(ctx, x.org("_apis/distributedtask/variablegroups?api-version=7.1"), body, &created); err != nil {
			x.fail("variable group "+g.Name, err)
			continue
		}
		x.wrote()
		x.record("vargroup", sid, itoa(created.ID))
	}
	return nil
}

// CloneSecureFiles uploads secure files found by name in -securefiles-dir.
// Secure file contents can't be downloaded through the API, so anything not
// in that directory goes to todo.json.
func CloneSecureFiles(ctx context.Context, x *Ctx) error {
	type file struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	const ver = "api-version=7.2-preview.1" // the only documented version
	files, err := valueOf[file](ctx, x, x.src("_apis/distributedtask/securefiles?"+ver))
	if err != nil {
		return err
	}
	x.read(len(files))
	byName := map[string]string{}
	if have, err := valueOf[file](ctx, x, x.tgt("_apis/distributedtask/securefiles?"+ver)); err == nil {
		for _, f := range have {
			byName[fold(f.Name)] = f.ID
		}
	}
	for _, f := range files {
		if id, ok := byName[fold(f.Name)]; ok {
			x.record("secureFile", strings.ToLower(f.ID), id)
			continue
		}
		var data []byte
		if x.Opt.SecureFilesDir != "" {
			data, err = os.ReadFile(filepath.Join(x.Opt.SecureFilesDir, f.Name))
		}
		if x.Opt.SecureFilesDir == "" || err != nil {
			x.todo("secure file "+f.Name, "upload it to the target Library (contents can't be read through the API), or put it in -securefiles-dir and rerun securefiles")
			err = nil
			continue
		}
		var created file
		u := x.tgt("_apis/distributedtask/securefiles?name=" + url.QueryEscape(f.Name) + "&" + ver)
		if _, err := x.C.Do(ctx, http.MethodPost, u, "application/octet-stream", data, &created); err != nil {
			x.fail("secure file "+f.Name, err)
			continue
		}
		x.wrote()
		x.record("secureFile", strings.ToLower(f.ID), created.ID)
	}
	return nil
}

// CloneQueues makes sure the target has a queue for every pool the source uses.
func CloneQueues(ctx context.Context, x *Ctx) error {
	type queue struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Pool struct {
			ID int `json:"id"`
		} `json:"pool"`
	}
	qs, err := valueOf[queue](ctx, x, x.src("_apis/distributedtask/queues?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(qs))
	byPool := map[int]int{}
	if have, err := valueOf[queue](ctx, x, x.tgt("_apis/distributedtask/queues?api-version=7.1")); err == nil {
		for _, q := range have {
			byPool[q.Pool.ID] = q.ID
		}
	}
	for _, q := range qs {
		if id, ok := byPool[q.Pool.ID]; ok {
			x.record("queue", itoa(q.ID), itoa(id))
			continue
		}
		var created queue
		body := jx.M{"name": q.Name, "pool": jx.M{"id": q.Pool.ID}}
		if err := x.C.Post(ctx, x.tgt("_apis/distributedtask/queues?authorizePipelines=false&api-version=7.1"), body, &created); err != nil {
			x.fail("queue "+q.Name, err)
			continue
		}
		x.wrote()
		x.record("queue", itoa(q.ID), itoa(created.ID))
	}
	return nil
}

// CloneDeploymentGroups creates each deployment group on the same deployment
// pool, so the registered machines are shared and nothing is re-registered.
// Machine tags are per group, so they're copied by agent name.
func CloneDeploymentGroups(ctx context.Context, x *Ctx) error {
	type dg struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Pool        struct {
			ID int `json:"id"`
		} `json:"pool"`
	}
	type target struct {
		ID    int      `json:"id"`
		Tags  []string `json:"tags"`
		Agent struct {
			Name string `json:"name"`
		} `json:"agent"`
	}
	dgs, err := valueOf[dg](ctx, x, x.src("_apis/distributedtask/deploymentgroups?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(dgs))
	byName := map[string]int{}
	if have, err := valueOf[dg](ctx, x, x.tgt("_apis/distributedtask/deploymentgroups?api-version=7.1")); err == nil {
		for _, g := range have {
			byName[fold(g.Name)] = g.ID
		}
	}
	for _, g := range dgs {
		tid, ok := byName[fold(g.Name)]
		if !ok {
			var created dg
			body := jx.M{"name": g.Name, "description": g.Description, "poolId": g.Pool.ID}
			if err := x.C.Post(ctx, x.tgt("_apis/distributedtask/deploymentgroups?api-version=7.1"), body, &created); err != nil {
				x.fail("deployment group "+g.Name, err)
				continue
			}
			x.wrote()
			tid = created.ID
		}
		if x.Dry() {
			continue
		}
		x.record("deploymentGroup", itoa(g.ID), itoa(tid))
		srcT, err := valueOf[target](ctx, x, x.src("_apis/distributedtask/deploymentgroups/"+itoa(g.ID)+"/targets?api-version=7.1"))
		if err != nil {
			x.fail("deployment targets of "+g.Name, err)
			continue
		}
		tgtT, err := valueOf[target](ctx, x, x.tgt("_apis/distributedtask/deploymentgroups/"+itoa(tid)+"/targets?api-version=7.1"))
		if err != nil {
			x.fail("deployment targets of "+g.Name, err)
			continue
		}
		byAgent := map[string]int{}
		for _, t := range tgtT {
			byAgent[fold(t.Agent.Name)] = t.ID
		}
		var updates []any
		for _, t := range srcT {
			if id, ok := byAgent[fold(t.Agent.Name)]; ok && len(t.Tags) > 0 {
				updates = append(updates, jx.M{"id": id, "tags": t.Tags})
			}
		}
		if len(updates) > 0 {
			if err := x.C.Patch(ctx, x.tgt("_apis/distributedtask/deploymentgroups/"+itoa(tid)+"/targets?api-version=7.1"), updates, nil); err != nil {
				x.fail("deployment target tags of "+g.Name, err)
			}
		}
	}
	return nil
}

// CloneEnvs creates environments and their Kubernetes resources. Kubernetes
// resources reuse the source's service connection, which endpoints shares
// with the target. Virtual machine resources need their agent re-registered.
func CloneEnvs(ctx context.Context, x *Ctx) error {
	type resource struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	type env struct {
		ID          int        `json:"id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Resources   []resource `json:"resources"`
	}
	envs, err := listAs[env](ctx, x, x.src("_apis/distributedtask/environments?$top=1000&api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(envs))
	byName := map[string]int{}
	if have, err := listAs[env](ctx, x, x.tgt("_apis/distributedtask/environments?$top=1000&api-version=7.1")); err == nil {
		for _, e := range have {
			byName[fold(e.Name)] = e.ID
		}
	}
	for _, e := range envs {
		tid, ok := byName[fold(e.Name)]
		if !ok {
			var created env
			if err := x.C.Post(ctx, x.tgt("_apis/distributedtask/environments?api-version=7.1"), jx.M{"name": e.Name, "description": e.Description}, &created); err != nil {
				x.fail("environment "+e.Name, err)
				continue
			}
			x.wrote()
			tid = created.ID
		}
		if x.Dry() {
			continue
		}
		x.record("environment", itoa(e.ID), itoa(tid))
		var sd, td env
		if err := x.C.Get(ctx, x.src("_apis/distributedtask/environments/"+itoa(e.ID)+"?expands=resourceReferences&api-version=7.1"), &sd); err != nil {
			x.fail("environment resources "+e.Name, err)
			continue
		}
		if err := x.C.Get(ctx, x.tgt("_apis/distributedtask/environments/"+itoa(tid)+"?expands=resourceReferences&api-version=7.1"), &td); err != nil {
			x.fail("environment resources "+e.Name, err)
			continue
		}
		have := map[string]bool{}
		for _, r := range td.Resources {
			have[fold(r.Name)] = true
		}
		for _, r := range sd.Resources {
			if have[fold(r.Name)] {
				continue
			}
			item := "environment " + e.Name + " / " + r.Name
			switch fold(r.Type) {
			case "kubernetes":
				if err := x.copyK8sResource(ctx, e.ID, tid, r.ID); err != nil {
					x.fail(item, err)
				}
			case "virtualmachine":
				x.todo(item, "register the VM's agent with the target environment (Environments > Add resource > Virtual machines)")
			default:
				x.todo(item, "add this "+r.Type+" resource to the target environment by hand")
			}
		}
	}
	return nil
}

func (x *Ctx) copyK8sResource(ctx context.Context, srcEnv, tgtEnv, resID int) error {
	var k jx.M
	if err := x.C.Get(ctx, x.src("_apis/distributedtask/environments/"+itoa(srcEnv)+"/providers/kubernetes/"+itoa(resID)+"?api-version=7.1"), &k); err != nil {
		return err
	}
	ep := jx.Str(k, "serviceEndpointId")
	if _, ok := x.mapped("endpoint", ep); !ok {
		return errors.New("its service connection isn't shared with the target; run endpoints first")
	}
	body := jx.M{"name": k["name"], "namespace": k["namespace"], "clusterName": k["clusterName"], "serviceEndpointId": ep, "tags": k["tags"]}
	err := x.C.Post(ctx, x.tgt("_apis/distributedtask/environments/"+itoa(tgtEnv)+"/providers/kubernetes?api-version=7.1"), body, nil)
	if s := client.StatusOf(err); s == http.StatusBadRequest || s == http.StatusNotFound {
		// 7.2 documents serviceEndpointId on the newer pipelines/environments route.
		err = x.C.Post(ctx, x.tgt("_apis/pipelines/environments/"+itoa(tgtEnv)+"/providers/kubernetes?api-version=7.2-preview.2"), body, nil)
	}
	if err == nil {
		x.wrote()
	}
	return err
}
