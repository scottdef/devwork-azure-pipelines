package clone

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/coolado/adoclone/internal/jx"
)

// Regression tests for the independent review's findings.

func TestTargetMustBelongToCheckpoint(t *testing.T) {
	f := &fake{}
	projectRoutes(f)
	x := newTestCtx(t, f, false, "")
	err := CloneProject(context.Background(), x)
	if err == nil || !strings.Contains(err.Error(), "-adopt-existing") {
		t.Fatalf("existing target with a fresh checkpoint: %v", err)
	}
	x = newTestCtx(t, f, false, "")
	x.Opt.AdoptExisting = true
	if err := CloneProject(context.Background(), x); err != nil || x.S.TargetProjectID != tgtID {
		t.Fatalf("adopt: err=%v target=%q", err, x.S.TargetProjectID)
	}
	x = newTestCtx(t, f, false, "")
	x.S.SetProjects(srcID, "99999999-9999-9999-9999-999999999999")
	if _, err := x.TgtProject(context.Background()); err == nil || !strings.Contains(err.Error(), "belongs to target project") {
		t.Fatalf("mismatched checkpoint: %v", err)
	}
	x = newTestCtx(t, f, false, "")
	if err := Cleanup(context.Background(), x); err == nil || !strings.Contains(err.Error(), "refusing") || f.count("DELETE", "") != 0 {
		t.Fatalf("cleanup with an empty checkpoint: %v", err)
	}
}

func TestMergeACEsKeepsTargetAndDropsUnmappedGroups(t *testing.T) {
	x := mapCtx(t, baseMaps())
	gm := x.guidMap()
	idd := map[string]string{"microsoft.teamfoundation.identity;s-1-9-src-contrib": "Microsoft.TeamFoundation.Identity;S-1-9-tgt-contrib"}
	sg := &groupIDs{descriptors: map[string]bool{"microsoft.teamfoundation.identity;s-1-9-src-contrib": true, "microsoft.teamfoundation.identity;s-1-9-src-orphan": true}}
	srcBuild := "Microsoft.TeamFoundation.ServiceIdentity;c0ffee00-0000-0000-0000-000000000000:Build:" + srcID
	tgtBuild := "Microsoft.TeamFoundation.ServiceIdentity;c0ffee00-0000-0000-0000-000000000000:Build:" + tgtID
	target := acl{AcesDictionary: map[string]ace{tgtBuild: {tgtBuild, 7, 0}, "Microsoft.TeamFoundation.Identity;S-1-9-tgt-admins": {"Microsoft.TeamFoundation.Identity;S-1-9-tgt-admins", 255, 0}}}
	source := acl{AcesDictionary: map[string]ace{
		"Microsoft.TeamFoundation.Identity;S-1-9-src-contrib": {Allow: 3},
		"Microsoft.TeamFoundation.Identity;S-1-9-src-orphan":  {Allow: 1},
		srcBuild: {Allow: 15},
		"Microsoft.IdentityModel.Claims.ClaimsIdentity;tenant\\user@x.com": {Allow: 2},
	}}
	merged, changed := mergeACEs(target, source, func(d string) (string, bool) { return x.mapDescriptor(d, idd, gm, sg) })
	if !changed {
		t.Fatal("expected a change")
	}
	want := map[string]int{tgtBuild: 15, "Microsoft.TeamFoundation.Identity;S-1-9-tgt-admins": 255,
		"Microsoft.TeamFoundation.Identity;S-1-9-tgt-contrib": 3, "Microsoft.IdentityModel.Claims.ClaimsIdentity;tenant\\user@x.com": 2}
	if len(merged) != len(want) {
		t.Fatalf("merged %v", merged)
	}
	for d, allow := range want {
		if merged[d].Allow != allow || merged[d].Descriptor != d {
			t.Errorf("%s: %+v", d, merged[d])
		}
	}
	if _, changed := mergeACEs(acl{AcesDictionary: merged}, source, func(d string) (string, bool) { return x.mapDescriptor(d, idd, gm, sg) }); changed {
		t.Error("second merge should be a no-op")
	}
}

func TestReleaseStageToken(t *testing.T) {
	x := mapCtx(t, map[string]map[string]string{"releaseDef": {"3": "7"}, "releaseEnv": {"1": "11"}})
	rm := aclNamespaces[5]
	if tok, ok := x.rewriteToken(rm, srcID+"/Folder/3/Environment/1", x.guidMap()); !ok || tok != tgtID+"/Folder/7/Environment/11" {
		t.Fatalf("got %q %v", tok, ok)
	}
	if _, ok := x.rewriteToken(rm, srcID+"/3/Environment/2", x.guidMap()); ok {
		t.Fatal("unmapped stage should be skipped")
	}
}

func TestMapMemberServiceIdentity(t *testing.T) {
	x := mapCtx(t, nil)
	enc := func(s string) string { return "svc." + base64.RawURLEncoding.EncodeToString([]byte(s)) }
	src := enc("c0ffee00-0000-0000-0000-000000000000:Build:" + srcID)
	want := enc("c0ffee00-0000-0000-0000-000000000000:Build:" + tgtID)
	sg := &groupIDs{subjects: map[string]bool{"vssgp.orphan": true}}
	if got, ok := x.mapMember(src, nil, x.guidMap(), sg); !ok || got != want {
		t.Fatalf("got %s %v", got, ok)
	}
	if _, ok := x.mapMember("vssgp.orphan", nil, x.guidMap(), sg); ok {
		t.Fatal("unmapped source group must be refused")
	}
	if got, ok := x.mapMember("aad.user", nil, x.guidMap(), sg); !ok || got != "aad.user" {
		t.Fatal("users pass through")
	}
}

func TestDeferredTriggersRemappedAndCrossProjectKept(t *testing.T) {
	x := mapCtx(t, baseMaps())
	x.srcProj = &Project{ID: srcID}
	full := decode(t, `{"triggers":[
		{"triggerType":"buildCompletion","definition":{"id":6,"project":{"id":"`+srcID+`"}}},
		{"triggerType":"buildCompletion","definition":{"id":77,"project":{"id":"99999999-9999-9999-9999-999999999999"}}}]}`)
	d, deferred := x.transformBuildDef(full, x.guidMap())
	if len(deferred) != 1 || jx.Str(deferred[0], "definition", "project", "id") != tgtID {
		t.Fatalf("deferred %v", deferred)
	}
	if tr := jx.Arr(d, "triggers"); len(tr) != 1 || jx.Str(tr[0], "definition", "id") != "77" {
		t.Fatalf("kept %v", tr)
	}
}

func TestPendingTriggersApplyLater(t *testing.T) {
	f := &fake{}
	var put jx.M
	f.on("GET", "/CoolADO/Tgt/_apis/build/definitions/50", func(w http.ResponseWriter, _ *http.Request, _ []string, _ []byte) {
		writeJSON(w, jx.M{"id": 50, "triggers": []any{jx.M{"triggerType": "continuousIntegration"}}})
	})
	f.on("PUT", "/CoolADO/Tgt/_apis/build/definitions/50", func(w http.ResponseWriter, _ *http.Request, _ []string, b []byte) {
		json.Unmarshal(b, &put)
		writeJSON(w, jx.M{})
	})
	x := newTestCtx(t, f, false, "")
	x.S.Put("pendingTrigger", "50", `[{"triggerType":"buildCompletion","definition":{"id":6}}]`)
	x.applyPendingTriggers(context.Background())
	if put != nil {
		t.Fatal("upstream not mapped yet: nothing should be written")
	}
	x.S.Put("buildDef", "6", "60")
	x.applyPendingTriggers(context.Background())
	if tr := jx.Arr(put, "triggers"); len(tr) != 2 || jx.Str(tr[1], "definition", "id") != "60" {
		t.Fatalf("put %v", put)
	}
	if v, _ := x.S.Get("pendingTrigger", "50"); v != "-" {
		t.Fatalf("pending = %q", v)
	}
}

func TestCommentsResumeWithoutDuplicates(t *testing.T) {
	f := &fake{}
	posted, failOnce := []string{}, true
	f.on("GET", `/CoolADO/Src/_apis/wit/workItems/1/comments`, func(w http.ResponseWriter, _ *http.Request, _ []string, _ []byte) {
		writeJSON(w, jx.M{"comments": []any{jx.M{"id": 10, "text": "a"}, jx.M{"id": 11, "text": "b"}, jx.M{"id": 12, "text": "c"}}})
	})
	f.on("POST", `/CoolADO/Tgt/_apis/wit/workItems/101/comments`, func(w http.ResponseWriter, _ *http.Request, _ []string, b []byte) {
		var c struct{ Text string }
		json.Unmarshal(b, &c)
		if strings.HasSuffix(c.Text, "b") && failOnce {
			failOnce = false
			w.WriteHeader(400)
			return
		}
		posted = append(posted, c.Text[len(c.Text)-1:])
		writeJSON(w, jx.M{})
	})
	x := newTestCtx(t, f, false, "")
	if err := x.copyComments(context.Background(), 1, 101); err == nil {
		t.Fatal("expected the injected failure")
	}
	if err := x.copyComments(context.Background(), 1, 101); err != nil {
		t.Fatal(err)
	}
	if strings.Join(posted, "") != "abc" {
		t.Fatalf("posted %v", posted)
	}
}

func TestLinksWaitForUncopiedItems(t *testing.T) {
	x := mapCtx(t, map[string]map[string]string{"workitem": {"1": "101"}})
	w := workItem{ID: 1, Relations: []relation{
		{Rel: "System.LinkTypes.Related", URL: wiLinkURL(2)},
		{Rel: "System.LinkTypes.Related", URL: wiLinkURL(3)},
	}}
	_, waiting, err := x.linkOps(context.Background(), w, map[int]bool{1: true, 2: true, 3: true}, map[int]bool{3: true}, x.guidMap())
	if err != nil || strings.Join(waiting, ",") != "2" {
		t.Fatalf("waiting=%v err=%v (3 is a test plan item and shouldn't block)", waiting, err)
	}
}

func TestCheckSigIgnoresDisplayFields(t *testing.T) {
	a := jx.M{"approvers": []any{jx.M{"id": "g1", "displayName": `[Src]\Approvers`, "descriptor": "vssgp.a"}}, "minRequiredApprovers": 1.0}
	b := jx.M{"approvers": []any{jx.M{"id": "g1", "displayName": `[Tgt]\Approvers`}}, "minRequiredApprovers": 1.0}
	typ := jx.M{"id": "8c6f20a7-a545-4486-9777-f762fafe0d4d"}
	if checkSig(typ, a) != checkSig(typ, b) {
		t.Fatal("signatures should match")
	}
}
