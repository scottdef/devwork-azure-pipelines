package clone

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/coolado/adoclone/internal/jx"
	"github.com/coolado/adoclone/internal/metrics"
)

const (
	tgSrc    = "aaaaaaaa-0000-0000-0000-00000000000a"
	tgTgt    = "bbbbbbbb-0000-0000-0000-00000000000b"
	qSrc     = "cccccccc-0000-0000-0000-00000000000c"
	qTgt     = "dddddddd-0000-0000-0000-00000000000d"
	teamSrc  = "eeeeeeee-0000-0000-0000-00000000000e"
	teamTgt  = "ffffffff-0000-0000-0000-00000000000f"
	userSrc  = "12121212-0000-0000-0000-000000000012"
	groupSrc = "13131313-0000-0000-0000-000000000013"
	groupTgt = "14141414-0000-0000-0000-000000000014"
)

func decode(t *testing.T, s string) jx.M {
	t.Helper()
	m, err := jx.Decode([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func baseMaps() map[string]map[string]string {
	return map[string]map[string]string{
		"repo": {srcRepo: tgtRepo}, "repoName": {"Src": "Tgt"}, "queue": {"10": "20"},
		"vargroup": {"7": "8"}, "taskGroup": {tgSrc: tgTgt}, "buildDef": {"5": "50"},
		"releaseDef": {"9": "90"}, "releaseEnv": {"11": "110"}, "deploymentGroup": {"3": "4"},
		"query": {qSrc: qTgt}, "team": {teamSrc: teamTgt}, "identity": {groupSrc: groupTgt},
	}
}

func TestTransformBuildDef(t *testing.T) {
	x := mapCtx(t, baseMaps())
	full := decode(t, `{"id":5,"revision":3,"name":"CI","path":"\\","project":{"id":"`+srcID+`"},
		"repository":{"id":"`+srcRepo+`","name":"Src","type":"TfsGit","url":"https://x"},
		"queue":{"id":10,"_links":{}},"variableGroups":[{"id":7}],
		"process":{"type":1,"phases":[{"target":{"queue":{"id":10}},"steps":[{"task":{"id":"`+tgSrc+`","definitionType":"metaTask"}}]}]},
		"triggers":[{"triggerType":"buildCompletion","definition":{"id":6}},{"triggerType":"continuousIntegration"}],
		"properties":{"p":"`+srcID+`"}}`)
	d, deferred := x.transformBuildDef(full, x.guidMap())
	for _, k := range []string{"id", "revision", "project"} {
		if _, ok := d[k]; ok {
			t.Errorf("%s not stripped", k)
		}
	}
	checks := map[string]string{
		jx.Str(d, "repository", "id"):                                               tgtRepo,
		jx.Str(d, "repository", "name"):                                             "Tgt",
		jx.Str(d, "queue", "id"):                                                    "20",
		jx.Str(jx.Arr(d, "variableGroups")[0], "id"):                                "8",
		jx.Str(jx.Arr(d, "process", "phases")[0], "target", "queue", "id"):          "20",
		jx.Str(jx.Arr(jx.Arr(d, "process", "phases")[0], "steps")[0], "task", "id"): tgTgt,
		jx.Str(d, "properties", "p"):                                                tgtID,
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
	if len(jx.Arr(d, "triggers")) != 1 || len(deferred) != 1 {
		t.Errorf("triggers %v deferred %v", jx.Arr(d, "triggers"), deferred)
	}
	if jx.Obj(full, "repository")["id"] != srcRepo {
		t.Error("input modified")
	}
}

func TestTransformReleaseDef(t *testing.T) {
	x := mapCtx(t, baseMaps())
	src, tgt := &Project{ID: srcID, Name: "Src"}, &Project{ID: tgtID, Name: "Tgt"}
	full := decode(t, `{"id":9,"name":"R","variableGroups":[7],
		"artifacts":[{"type":"Build","sourceId":"x","definitionReference":{"definition":{"id":"5"},"project":{"id":"`+srcID+`","name":"Src"}}},
		             {"type":"Git","definitionReference":{"definition":{"id":"`+srcRepo+`","name":"Src"},"project":{"id":"`+srcID+`"}}}],
		"environments":[{"id":11,"name":"prod","variableGroups":[7],
			"preDeployApprovals":{"approvals":[{"id":4,"approver":{"id":"`+groupSrc+`"}}]},
			"deployPhases":[{"phaseType":"agentBasedDeployment","deploymentInput":{"queueId":10},"workflowTasks":[{"taskId":"`+tgSrc+`","definitionType":"metaTask"}]},
			                {"phaseType":"machineGroupBasedDeployment","deploymentInput":{"queueId":3}}]}]}`)
	d := x.transformReleaseDef(full, x.guidMap(), src, tgt)
	b, _ := json.Marshal(d)
	s := string(b)
	for _, want := range []string{`"sourceId":"` + tgtID + `:50"`, `"sourceId":"` + tgtID + `:` + tgtRepo + `"`,
		`"queueId":20`, `"queueId":4`, `"variableGroups":[8]`, `"taskId":"` + tgTgt + `"`, `"id":"` + groupTgt + `"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	env := jx.Arr(d, "environments")[0]
	if id, _ := jx.Int(env, "id"); id != 0 {
		t.Error("environment id not reset")
	}
	if strings.Contains(s, `"id":9,`) || strings.Contains(s, srcID) {
		t.Errorf("source ids left: %s", s)
	}
}

func TestTransformPolicy(t *testing.T) {
	x := mapCtx(t, baseMaps())
	gm := x.guidMap()
	p := decode(t, `{"id":1,"isEnabled":true,"isBlocking":true,"type":{"id":"`+buildValidationPolicy+`"},
		"settings":{"buildDefinitionId":5,"requiredReviewerIds":["`+groupSrc+`"],"scope":[{"repositoryId":"`+srcRepo+`","refName":"refs/heads/main"}]}}`)
	body, why := x.transformPolicy(p, gm)
	if body == nil {
		t.Fatal(why)
	}
	s, _ := json.Marshal(body)
	for _, want := range []string{`"buildDefinitionId":50`, tgtRepo, groupTgt} {
		if !strings.Contains(string(s), want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	p2 := decode(t, `{"type":{"id":"x"},"settings":{"scope":[{"repositoryId":"99999999-9999-9999-9999-999999999999"}]}}`)
	if body, why := x.transformPolicy(p2, gm); body != nil || !strings.Contains(why, "wasn't copied") {
		t.Errorf("unmapped repo: %v %q", body, why)
	}
}

func TestWidgetSettings(t *testing.T) {
	x := mapCtx(t, baseMaps())
	in := `{"queryId":"` + qSrc + `","buildDefinition":{"id":5,"name":"CI"},"releaseDefinitionId":"9","projectName":"Src","areaPath":"Src\\Team","teamId":"` + teamSrc + `"}`
	out := decode(t, x.remapWidgetSettings(in, x.guidMap()))
	if jx.Str(out, "queryId") != qTgt || jx.Str(out, "teamId") != teamTgt || jx.Str(out, "buildDefinition", "id") != "50" ||
		jx.Str(out, "releaseDefinitionId") != "90" || jx.Str(out, "projectName") != "Tgt" || jx.Str(out, "areaPath") != `Tgt\Team` {
		t.Fatalf("got %v", out)
	}
	if got := x.remapWidgetSettings("not json "+qSrc, x.guidMap()); got != "not json "+qTgt {
		t.Fatalf("non-JSON settings: %s", got)
	}
}

func TestRewriteToken(t *testing.T) {
	x := mapCtx(t, baseMaps())
	gm := x.guidMap()
	git, build := aclNamespaces[0], aclNamespaces[4]
	if tok, ok := x.rewriteToken(git, "repoV2/"+srcID+"/"+srcRepo, gm); !ok || tok != "repoV2/"+tgtID+"/"+tgtRepo {
		t.Errorf("git: %s %v", tok, ok)
	}
	if _, ok := x.rewriteToken(git, "repoV2/"+srcID+"/99999999-9999-9999-9999-999999999999", gm); ok {
		t.Error("token for an uncopied repo should be skipped")
	}
	if tok, ok := x.rewriteToken(build, srcID+"/Folder/5", gm); !ok || tok != tgtID+"/Folder/50" {
		t.Errorf("build: %s %v", tok, ok)
	}
	if _, ok := x.rewriteToken(build, srcID+"/6", gm); ok {
		t.Error("token for an uncopied pipeline should be skipped")
	}
}

func TestMergeColumnsAndRows(t *testing.T) {
	src := []jx.M{{"id": "a", "name": "New", "columnType": "incoming"}, {"id": "b", "name": "Doing", "columnType": "inProgress"}, {"id": "c", "name": "Done", "columnType": "outgoing"}}
	tgt := []jx.M{{"id": "x", "name": "To Do", "columnType": "incoming"}, {"id": "z", "name": "Closed", "columnType": "outgoing"}}
	out := mergeColumns(src, tgt)
	if jx.Str(out[0], "id") != "x" || jx.Get(out[1], "id") != nil || jx.Str(out[2], "id") != "z" {
		t.Fatalf("columns %v", out)
	}
	rows := mergeRows([]jx.M{{"id": defaultLane, "name": nil}, {"id": "s1", "name": "Expedite"}, {"id": "s2", "name": "New lane"}},
		[]jx.M{{"id": defaultLane}, {"id": "t1", "name": "expedite"}})
	if jx.Str(rows[0], "id") != defaultLane || jx.Str(rows[1], "id") != "t1" || jx.Get(rows[2], "id") != nil {
		t.Fatalf("rows %v", rows)
	}
}

func TestSmallHelpers(t *testing.T) {
	x := mapCtx(t, baseMaps())
	w := `SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = 'src' AND [System.AreaPath] UNDER 'Src\Team' AND [System.AssignedTo] IN GROUP '[Src]\Leads'`
	want := `SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = 'Tgt' AND [System.AreaPath] UNDER 'Tgt\Team' AND [System.AssignedTo] IN GROUP '[Tgt]\Leads'`
	if got := x.rewriteWIQL(w); got != want {
		t.Errorf("wiql:\n%s", got)
	}
	if got := SecretEnv("My Group", "db.password"); got != "SECRET_MY_GROUP_DB_PASSWORD" {
		t.Errorf("SecretEnv = %s", got)
	}
	a := jx.M{"id": "A", "tasks": []any{jx.M{"task": jx.M{"id": "B", "definitionType": "metaTask"}}}}
	b := jx.M{"id": "B"}
	if o := orderTaskGroups([]jx.M{a, b}); jx.Str(o[0], "id") != "B" {
		t.Errorf("task group order %v", o)
	}
	hook := decode(t, `{"eventType":"git.push","consumerId":"webHooks","publisherInputs":{"projectId":"`+srcID+`","repository":"`+srcRepo+`","areaPath":"Src\\X","releaseDefinitionId":"9","tfsSubscriptionId":"z"},"consumerInputs":{"url":"https://h"}}`)
	hb := x.transformHook(hook, x.guidMap())
	pin := jx.Obj(hb, "publisherInputs")
	if jx.Str(pin, "projectId") != tgtID || jx.Str(pin, "repository") != tgtRepo || jx.Str(pin, "areaPath") != `Tgt\X` ||
		jx.Str(pin, "releaseDefinitionId") != "90" || pin["tfsSubscriptionId"] != nil {
		t.Errorf("hook inputs %v", pin)
	}
	if !hasMaskedSecret(jx.M{"basicAuthPassword": "********"}) {
		t.Error("masked secret not detected")
	}
}

func TestRunStatusAndFailures(t *testing.T) {
	saved := Components
	defer func() { Components = saved }()
	Components = []Component{
		{"a", 1, "", func(_ ctxT, x *Ctx) error { x.fail("item", errors.New("bad")); return nil }},
		{"b", 1, "", func(_ ctxT, x *Ctx) error { return errors.New("stop") }},
		{"c", 1, "", func(_ ctxT, x *Ctx) error { return nil }},
	}
	x := mapCtx(t, nil)
	err := Run(context.Background(), x, []string{"a", "b", "c"})
	if err == nil || !strings.HasPrefix(err.Error(), "b: stop") {
		t.Fatalf("err = %v", err)
	}
	if x.Failures() != 1 || len(x.S.Todos()) != 1 {
		t.Fatalf("failures %d todos %v", x.Failures(), x.S.Todos())
	}
	for comp, want := range map[string]float64{"a": metrics.Done, "b": metrics.Failed, "c": metrics.Pending} {
		if got := x.M.Value("adoclone_phase_status", "component", comp); got != want {
			t.Errorf("%s status %v want %v", comp, got, want)
		}
	}
}

func TestCleanupRefusesForeignTarget(t *testing.T) {
	f := &fake{}
	projectRoutes(f)
	x := newTestCtx(t, f, false, "")
	x.S.SetProjects(srcID, "99999999-9999-9999-9999-999999999999")
	if err := Cleanup(context.Background(), x); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("err = %v", err)
	}
	if f.count("DELETE", "") != 0 {
		t.Fatal("deleted something")
	}
}

func TestVarGroupSecrets(t *testing.T) {
	f := &fake{}
	projectRoutes(f)
	var posted []jx.M
	f.on("GET", "/CoolADO/Src/_apis/distributedtask/variablegroups", func(w http.ResponseWriter, _ *http.Request, _ []string, _ []byte) {
		writeJSON(w, jx.M{"value": []any{
			jx.M{"id": 7, "name": "App", "type": "Vsts", "variables": jx.M{"user": jx.M{"value": "u"}, "pass": jx.M{"value": nil, "isSecret": true}, "token": jx.M{"value": nil, "isSecret": true}}},
			jx.M{"id": 8, "name": "Vault", "type": "AzureKeyVault", "variables": jx.M{"s": jx.M{"isSecret": true}}},
		}})
	})
	f.on("GET", "/CoolADO/Tgt/_apis/distributedtask/variablegroups", func(w http.ResponseWriter, _ *http.Request, _ []string, _ []byte) {
		writeJSON(w, jx.M{"value": []any{}})
	})
	f.on("POST", "/CoolADO/_apis/distributedtask/variablegroups", func(w http.ResponseWriter, _ *http.Request, _ []string, body []byte) {
		m, _ := jx.Decode(body)
		posted = append(posted, m)
		writeJSON(w, jx.M{"id": 70 + len(posted)})
	})
	x := newTestCtx(t, f, false, "")
	x.S.SetProjects(srcID, tgtID)
	x.Getenv = func(k string) string {
		if k == "SECRET_APP_PASS" {
			return "s3cret"
		}
		return ""
	}
	if err := CloneVarGroups(context.Background(), x); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 || jx.Str(posted[0], "variables", "pass", "value") != "s3cret" || jx.Str(posted[0], "variables", "user", "value") != "u" {
		t.Fatalf("posted %v", posted)
	}
	if jx.Str(jx.Arr(posted[0], "variableGroupProjectReferences")[0], "projectReference", "id") != tgtID {
		t.Error("project reference")
	}
	todos := x.S.Todos()
	if len(todos) != 1 || !strings.Contains(todos[0].Item, "App / token") || !strings.Contains(todos[0].Action, "SECRET_APP_TOKEN") {
		t.Fatalf("todos %v", todos)
	}
	if m := x.S.Map("vargroup"); m["7"] != "71" || m["8"] != "72" {
		t.Fatalf("map %v", m)
	}
}
