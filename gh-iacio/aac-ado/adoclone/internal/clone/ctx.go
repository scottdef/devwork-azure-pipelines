// Package clone copies one Azure DevOps project into another project in the
// same organization, one component at a time. Every component follows the same
// pattern: list the source objects, skip anything already in the ID map,
// strip server-owned fields and remap references, create the object in the
// target, and record the new ID in the checkpoint.
package clone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/metrics"
	"github.com/coolado/adoclone/internal/state"
)

// Options are the run settings that change cloner behavior.
type Options struct {
	BypassRules    bool
	Visibility     string // private, public or source
	RepoMode       string // mirror or import
	VarGroupMode   string // copy or share
	SecureFilesDir string
	Git            string
	WorkDir        string
	PAT            string
	// AdoptExisting lets clone and cleanup work on a target project that this
	// checkpoint didn't create.
	AdoptExisting bool
}

// Ctx is shared by every component in a run.
type Ctx struct {
	C        *client.Client
	S        *state.State
	M        *metrics.Registry
	Log      *slog.Logger
	Src, Tgt string
	Opt      Options
	Getenv   func(string) string

	srcProj, tgtProj *Project
	srcGroups        *groupIDs
	buildService     map[string]string
	component        string
	failures         int
}

// Dry reports dry-run mode.
func (x *Ctx) Dry() bool { return x.C.DryRun }

// Failures is the number of per-item failures so far.
func (x *Ctx) Failures() int { return x.failures }

// record stores a source->target mapping. Dry runs never record, so a dry run
// can't make a later real run think something was already copied.
func (x *Ctx) record(kind, src, tgt string) {
	if x.Dry() || src == "" || tgt == "" || tgt == "0" {
		return
	}
	if err := x.S.Put(kind, src, tgt); err != nil {
		x.Log.Error("checkpoint write failed", "kind", kind, "err", err)
	}
}

func (x *Ctx) mapped(kind, src string) (string, bool) {
	if t, ok := x.S.Get(kind, src); ok {
		return t, true
	}
	return x.S.Get(kind, strings.ToLower(src))
}

func (x *Ctx) mappedInt(kind string, src int) (int, bool) {
	t, ok := x.S.Get(kind, strconv.Itoa(src))
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(t)
	return n, err == nil
}

func (x *Ctx) read(n int) {
	x.M.Add("adoclone_items_total", float64(n), "component", x.component, "side", "source")
}

func (x *Ctx) wrote() {
	if !x.Dry() {
		x.M.Inc("adoclone_items_total", "component", x.component, "side", "target")
	}
}

// fail records a per-item failure. The component carries on; the run exits 3
// at the end so the operator looks at todo.json.
func (x *Ctx) fail(item string, err error) {
	x.failures++
	x.M.Inc("adoclone_errors_total")
	x.Log.Error("item failed", "component", x.component, "item", item, "err", err)
	_ = x.S.AddTodo(x.component, item, "failed: "+err.Error())
}

// todo records something the operator has to do by hand.
func (x *Ctx) todo(item, action string) {
	x.Log.Warn("manual follow-up", "component", x.component, "item", item, "action", action)
	_ = x.S.AddTodo(x.component, item, action)
}

// URL helpers.
func (x *Ctx) src(path string) string   { return x.C.URL("", client.P(x.Src)+"/"+path) }
func (x *Ctx) tgt(path string) string   { return x.C.URL("", client.P(x.Tgt)+"/"+path) }
func (x *Ctx) org(path string) string   { return x.C.URL("", path) }
func (x *Ctx) vssps(path string) string { return x.C.URL(client.VSSPS, path) }
func (x *Ctx) srcRM(path string) string { return x.C.URL(client.VSRM, client.P(x.Src)+"/"+path) }
func (x *Ctx) tgtRM(path string) string { return x.C.URL(client.VSRM, client.P(x.Tgt)+"/"+path) }
func (x *Ctx) srcFeeds(path string) string {
	return x.C.URL(client.Feeds, client.P(x.Src)+"/"+path)
}
func (x *Ctx) tgtFeeds(path string) string {
	return x.C.URL(client.Feeds, client.P(x.Tgt)+"/"+path)
}

// guidKinds are the ID maps whose keys are GUIDs; they're applied wherever a
// source GUID can appear inside JSON (settings, tokens, widget config...).
var guidKinds = []string{"repo", "team", "classNode", "query", "identity", "wiki", "feed", "feedView", "secureFile", "taskGroup", "dashboard"}

// guidMap is every GUID mapping plus source project -> target project.
func (x *Ctx) guidMap() map[string]string {
	m := map[string]string{}
	for _, k := range guidKinds {
		for s, t := range x.S.Map(k) {
			m[strings.ToLower(s)] = t
		}
	}
	srcID, tgtID := x.S.SourceProjectID, x.S.TargetProjectID
	if x.srcProj != nil && x.srcProj.ID != "" {
		srcID = x.srcProj.ID
	}
	if x.tgtProj != nil && x.tgtProj.ID != "" {
		tgtID = x.tgtProj.ID
	}
	if srcID != "" && tgtID != "" {
		m[strings.ToLower(srcID)] = tgtID
	}
	return m
}

// listAs pages through a list API and decodes each item.
func listAs[T any](ctx context.Context, x *Ctx, u string) ([]T, error) {
	raws, err := x.C.List(ctx, u)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raws))
	for _, r := range raws {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// valueOf GETs a {count, value} response without paging.
func valueOf[T any](ctx context.Context, x *Ctx, u string) ([]T, error) {
	var r struct {
		Value []T `json:"value"`
	}
	err := x.C.Get(ctx, u, &r)
	return r.Value, err
}

func fold(s string) string { return strings.ToLower(s) }

func itoa(i int) string { return strconv.Itoa(i) }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// waitOperation polls an async operation (project create/delete) to completion.
func waitOperation(ctx context.Context, x *Ctx, id string) error {
	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		var op struct {
			Status        string `json:"status"`
			ResultMessage string `json:"resultMessage"`
		}
		if err := x.C.Get(ctx, x.org("_apis/operations/"+id+"?api-version=7.1"), &op); err != nil {
			return err
		}
		switch op.Status {
		case "succeeded":
			return nil
		case "failed", "cancelled":
			return fmt.Errorf("operation %s: %s", op.Status, op.ResultMessage)
		}
		if err := x.C.Sleep(ctx, 5*time.Second); err != nil {
			return err
		}
	}
	return errors.New("operation timed out after 20 minutes")
}
