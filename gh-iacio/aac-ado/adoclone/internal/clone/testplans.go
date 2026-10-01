package clone

import (
	"context"
	"fmt"

	"github.com/coolado/adoclone/internal/jx"
)

// CloneTestPlans copies test variables, configurations, plans, suites (in tree
// order) and the test cases in static suites. Requirement-based suites fill
// themselves from the Tested By links copied by workitems; query-based suites
// get their query rewritten. Test runs, results and outcomes can't be copied.
// Run this after workitems.
func CloneTestPlans(ctx context.Context, x *Ctx) error {
	if _, err := x.TgtProject(ctx); err != nil {
		return err
	}
	if err := x.copyNamed(ctx, "testVariable", "testplan/variables", []string{"name", "description", "values"}); err != nil {
		return err
	}
	if err := x.copyNamed(ctx, "testConfig", "testplan/configurations", []string{"name", "description", "isDefault", "state", "values"}); err != nil {
		return err
	}
	plans, err := listAs[jx.M](ctx, x, x.src("_apis/testplan/plans?includePlanDetails=true&api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(plans))
	for _, p := range plans {
		pid, _ := jx.Int(p, "id")
		tpid, ok := x.mappedInt("testPlan", pid)
		if !ok {
			var created jx.M
			if err := x.C.Post(ctx, x.tgt("_apis/testplan/plans?api-version=7.1"), x.planBody(p), &created); err != nil {
				x.fail("test plan "+jx.Str(p, "name"), err)
				continue
			}
			x.wrote()
			if x.Dry() {
				continue
			}
			tpid, _ = jx.Int(created, "id")
			x.record("testPlan", itoa(pid), itoa(tpid))
			x.record("workitem", itoa(pid), itoa(tpid))
			sRoot, _ := jx.Int(p, "rootSuite", "id")
			tRoot, _ := jx.Int(created, "rootSuite", "id")
			x.record("testSuite", itoa(sRoot), itoa(tRoot))
			x.record("workitem", itoa(sRoot), itoa(tRoot))
		}
		if err := x.cloneSuites(ctx, pid, tpid); err != nil {
			x.fail("suites of test plan "+jx.Str(p, "name"), err)
		}
	}
	return nil
}

// copyNamed copies a list of named objects (test variables, configurations),
// matching existing target objects by name.
func (x *Ctx) copyNamed(ctx context.Context, kind, path string, keep []string) error {
	items, err := listAs[jx.M](ctx, x, x.src("_apis/"+path+"?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(items))
	byName := map[string]string{}
	if have, err := listAs[jx.M](ctx, x, x.tgt("_apis/"+path+"?api-version=7.1")); err == nil {
		for _, h := range have {
			byName[fold(jx.Str(h, "name"))] = jx.Str(h, "id")
		}
	}
	for _, it := range items {
		sid := jx.Str(it, "id")
		if id, ok := byName[fold(jx.Str(it, "name"))]; ok {
			x.record(kind, sid, id)
			continue
		}
		body := jx.M{}
		for _, k := range keep {
			if v, ok := it[k]; ok {
				body[k] = v
			}
		}
		var created jx.M
		if err := x.C.Post(ctx, x.tgt("_apis/"+path+"?api-version=7.1"), body, &created); err != nil {
			x.fail(kind+" "+jx.Str(it, "name"), err)
			continue
		}
		x.wrote()
		x.record(kind, sid, jx.Str(created, "id"))
	}
	return nil
}

func (x *Ctx) planBody(p jx.M) jx.M {
	b := jx.M{
		"name":        p["name"],
		"description": p["description"],
		"areaPath":    jx.RebasePath(jx.Str(p, "areaPath"), x.Src, x.Tgt),
		"iteration":   jx.RebasePath(jx.Str(p, "iteration"), x.Src, x.Tgt),
		"startDate":   p["startDate"],
		"endDate":     p["endDate"],
		"state":       p["state"],
	}
	if id := jx.Str(p, "owner", "id"); id != "" {
		b["owner"] = jx.M{"id": id}
	}
	if v, ok := p["testOutcomeSettings"]; ok {
		b["testOutcomeSettings"] = v
	}
	if id, ok := jx.Int(p, "buildDefinition", "id"); ok {
		if t, ok := x.mappedInt("buildDef", id); ok {
			b["buildDefinition"] = jx.M{"id": t}
		}
	}
	if def, ok := jx.Int(p, "releaseEnvironmentDefinition", "definitionId"); ok {
		env, _ := jx.Int(p, "releaseEnvironmentDefinition", "environmentDefinitionId")
		td, ok1 := x.mappedInt("releaseDef", def)
		te, ok2 := x.mappedInt("releaseEnv", env)
		if ok1 && ok2 {
			b["releaseEnvironmentDefinition"] = jx.M{"definitionId": td, "environmentDefinitionId": te}
		}
	}
	return b
}

func (x *Ctx) cloneSuites(ctx context.Context, srcPlan, tgtPlan int) error {
	suites, err := listAs[jx.M](ctx, x, x.src(fmt.Sprintf("_apis/testplan/Plans/%d/suites?asTreeView=false&api-version=7.1", srcPlan)))
	if err != nil {
		return err
	}
	children := map[int][]jx.M{}
	var root jx.M
	for _, s := range suites {
		if parent, ok := jx.Int(s, "parentSuite", "id"); ok && parent > 0 {
			children[parent] = append(children[parent], s)
		} else {
			root = s
		}
	}
	if root == nil {
		return nil
	}
	queue := []jx.M{root}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		sid, _ := jx.Int(s, "id")
		tsid, ok := x.mappedInt("testSuite", sid)
		if !ok {
			parent, _ := jx.Int(s, "parentSuite", "id")
			tparent, ok := x.mappedInt("testSuite", parent)
			if !ok {
				x.todo("test suite "+jx.Str(s, "name"), "its parent suite wasn't copied")
				continue
			}
			body, why := x.suiteBody(s, tparent)
			if body == nil {
				x.todo("test suite "+jx.Str(s, "name"), why)
				continue
			}
			var created jx.M
			if err := x.C.Post(ctx, x.tgt(fmt.Sprintf("_apis/testplan/Plans/%d/suites?api-version=7.1", tgtPlan)), body, &created); err != nil {
				x.fail("test suite "+jx.Str(s, "name"), err)
				continue
			}
			x.wrote()
			if x.Dry() {
				continue
			}
			tsid, _ = jx.Int(created, "id")
			x.record("testSuite", itoa(sid), itoa(tsid))
			x.record("workitem", itoa(sid), itoa(tsid))
		}
		if jx.Str(s, "suiteType") == "staticTestSuite" {
			if done, _ := x.mapped("suiteCases", itoa(sid)); done != "1" {
				if err := x.copySuiteCases(ctx, srcPlan, sid, tgtPlan, tsid); err != nil {
					x.fail("test cases of suite "+jx.Str(s, "name"), err)
				} else {
					x.record("suiteCases", itoa(sid), "1")
				}
			}
		}
		queue = append(queue, children[sid]...)
	}
	return nil
}

func (x *Ctx) suiteBody(s jx.M, tparent int) (jx.M, string) {
	b := jx.M{
		"suiteType":                    s["suiteType"],
		"name":                         s["name"],
		"parentSuite":                  jx.M{"id": tparent},
		"inheritDefaultConfigurations": s["inheritDefaultConfigurations"],
	}
	var cfgs []any
	for _, c := range jx.Arr(s, "defaultConfigurations") {
		if id, ok := jx.Int(c, "id"); ok {
			if t, ok := x.mapped("testConfig", itoa(id)); ok {
				cfgs = append(cfgs, jx.M{"id": atoi(t)})
			}
		}
	}
	if len(cfgs) > 0 {
		b["defaultConfigurations"] = cfgs
	}
	if v, ok := s["defaultTesters"]; ok {
		b["defaultTesters"] = v
	}
	switch jx.Str(s, "suiteType") {
	case "dynamicTestSuite":
		b["queryString"] = x.rewriteWIQL(jx.Str(s, "queryString"))
	case "requirementTestSuite":
		rid, _ := jx.Int(s, "requirementId")
		t, ok := x.mapped("workitem", itoa(rid))
		if !ok {
			return nil, "its requirement (work item " + itoa(rid) + ") wasn't copied"
		}
		b["requirementId"] = atoi(t)
	}
	return b, ""
}

func (x *Ctx) copySuiteCases(ctx context.Context, srcPlan, srcSuite, tgtPlan, tgtSuite int) error {
	cases, err := listAs[jx.M](ctx, x, x.src(fmt.Sprintf("_apis/testplan/Plans/%d/Suites/%d/TestCase?api-version=7.1", srcPlan, srcSuite)))
	if err != nil {
		return err
	}
	var body []any
	for _, c := range cases {
		wid, _ := jx.Int(c, "workItem", "id")
		t, ok := x.mapped("workitem", itoa(wid))
		if !ok {
			continue
		}
		var points []any
		for _, pa := range jx.Arr(c, "pointAssignments") {
			if cid, ok := jx.Int(pa, "configurationId"); ok {
				if tc, ok := x.mapped("testConfig", itoa(cid)); ok {
					points = append(points, jx.M{"configurationId": atoi(tc)})
				}
			}
		}
		item := jx.M{"workItem": jx.M{"id": atoi(t)}}
		if len(points) > 0 {
			item["pointAssignments"] = points
		}
		body = append(body, item)
	}
	u := x.tgt(fmt.Sprintf("_apis/testplan/Plans/%d/Suites/%d/TestCase?api-version=7.1", tgtPlan, tgtSuite))
	for i := 0; i < len(body); i += 100 {
		j := i + 100
		if j > len(body) {
			j = len(body)
		}
		if err := x.C.Post(ctx, u, body[i:j], nil); err != nil {
			return err
		}
		x.wrote()
	}
	return nil
}
