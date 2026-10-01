package clone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/coolado/adoclone/internal/jx"
)

const attURL = "https://dev.azure.com/CoolADO/" + srcID + "/_apis/wit/attachments/55555555-5555-5555-5555-555555555555"

func wiLinkURL(id int) string {
	return fmt.Sprintf("https://dev.azure.com/CoolADO/_apis/wit/workItems/%d", id)
}

var sourceItems = map[int]jx.M{
	1: {"id": 1, "fields": jx.M{
		"System.WorkItemType": "Epic", "System.Title": "E", "System.AreaPath": `Src\Team`, "System.IterationPath": "Src",
		"System.TeamProject": "Src", "System.CommentCount": 2, "WEF_1234_Kanban.Column": "Doing",
		"System.CreatedBy":  jx.M{"displayName": "A", "uniqueName": "a@x.com"},
		"System.AssignedTo": jx.M{"displayName": "Leads", "uniqueName": `[Src]\Leads`},
	}, "relations": []any{
		jx.M{"rel": "System.LinkTypes.Hierarchy-Forward", "url": wiLinkURL(2), "attributes": jx.M{}},
		jx.M{"rel": "System.LinkTypes.Related", "url": wiLinkURL(999), "attributes": jx.M{"comment": "elsewhere"}},
		jx.M{"rel": "ArtifactLink", "url": "vstfs:///Git/Commit/" + srcID + "%2F" + srcRepo + "%2Fabc123", "attributes": jx.M{"name": "Fixed in Commit"}},
		jx.M{"rel": "ArtifactLink", "url": "vstfs:///Git/PullRequestId/" + srcID + "%2F" + srcRepo + "%2F7", "attributes": jx.M{"name": "Pull Request"}},
		jx.M{"rel": "AttachedFile", "url": attURL, "attributes": jx.M{"name": "a.txt"}},
	}},
	2: {"id": 2, "fields": jx.M{"System.WorkItemType": "Task", "System.Title": "T", "System.AreaPath": "Src", "System.Parent": 1},
		"relations": []any{jx.M{"rel": "System.LinkTypes.Hierarchy-Reverse", "url": wiLinkURL(1), "attributes": jx.M{}}}},
	3: {"id": 3, "fields": jx.M{"System.WorkItemType": "Test Case", "System.Title": "TC",
		"Microsoft.VSTS.TCM.Steps": `<steps id="0" last="3"><compref id="2" ref="4"><step id="3"/></compref></steps>`}},
	4: {"id": 4, "fields": jx.M{"System.WorkItemType": "Shared Steps", "System.Title": "SS"}},
	5: {"id": 5, "fields": jx.M{"System.WorkItemType": "Test Plan", "System.Title": "Plan"}},
}

type wiFake struct {
	*fake
	mu       sync.Mutex
	next     int
	created  map[int][]jx.M // target id -> create ops
	types    map[int]string
	patches  map[int][]jx.M // target id -> applied ops
	comments map[string][]string
	uploads  map[string]string // fileName -> body
	ranges   []string
}

func newWIFake() *wiFake {
	f := &wiFake{fake: &fake{}, next: 101, created: map[int][]jx.M{}, types: map[int]string{},
		patches: map[int][]jx.M{}, comments: map[string][]string{}, uploads: map[string]string{}}
	projectRoutes(f.fake)
	f.on("POST", "/CoolADO/Src/_apis/wit/wiql", func(w http.ResponseWriter, _ *http.Request, _ []string, body []byte) {
		var q struct{ Query string }
		json.Unmarshal(body, &q)
		if strings.Contains(q.Query, "> 0 ") {
			writeJSON(w, jx.M{"workItems": []any{jx.M{"id": 1}, jx.M{"id": 2}, jx.M{"id": 3}, jx.M{"id": 4}, jx.M{"id": 5}}})
			return
		}
		writeJSON(w, jx.M{"workItems": []any{}})
	})
	f.on("POST", "/CoolADO/_apis/wit/workitemsbatch", func(w http.ResponseWriter, _ *http.Request, _ []string, body []byte) {
		var b struct{ IDs []int }
		json.Unmarshal(body, &b)
		var out []any
		for _, id := range b.IDs {
			out = append(out, sourceItems[id])
		}
		writeJSON(w, jx.M{"value": out})
	})
	f.on("POST", `/CoolADO/Tgt/_apis/wit/workitems/\$(.+)`, func(w http.ResponseWriter, r *http.Request, m []string, body []byte) {
		if r.Header.Get("Content-Type") != "application/json-patch+json" || r.URL.Query().Get("bypassRules") != "true" {
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		id := f.next
		f.next++
		var ops []jx.M
		json.Unmarshal(body, &ops)
		f.created[id], f.types[id] = ops, m[1]
		f.mu.Unlock()
		writeJSON(w, jx.M{"id": id})
	})
	f.on("GET", `/CoolADO/Src/_apis/wit/workItems/(\d+)/comments`, func(w http.ResponseWriter, r *http.Request, m []string, _ []byte) {
		if m[1] != "1" {
			writeJSON(w, jx.M{"comments": []any{}})
			return
		}
		if r.URL.Query().Get("continuationToken") == "p2" {
			writeJSON(w, jx.M{"comments": []any{jx.M{"text": "second", "createdBy": jx.M{"displayName": "B"}}}})
			return
		}
		writeJSON(w, jx.M{"comments": []any{jx.M{"text": "first", "createdBy": jx.M{"displayName": "A"}, "createdDate": "2026-01-01"}}, "continuationToken": "p2"})
	})
	f.on("POST", `/CoolADO/Tgt/_apis/wit/workItems/(\d+)/comments`, func(w http.ResponseWriter, _ *http.Request, m []string, body []byte) {
		var c struct{ Text string }
		json.Unmarshal(body, &c)
		f.mu.Lock()
		f.comments[m[1]] = append(f.comments[m[1]], c.Text)
		f.mu.Unlock()
		writeJSON(w, jx.M{})
	})
	f.on("GET", "/CoolADO/"+srcID+"/_apis/wit/attachments/(.+)", func(w http.ResponseWriter, r *http.Request, _ []string, _ []byte) {
		w.Write([]byte("hello world"))
	})
	f.on("POST", "/CoolADO/Tgt/_apis/wit/attachments", func(w http.ResponseWriter, r *http.Request, _ []string, body []byte) {
		name := r.URL.Query().Get("fileName")
		if r.URL.Query().Get("uploadType") == "Chunked" {
			writeJSON(w, jx.M{"id": "chunked-1", "url": "https://dev.azure.com/CoolADO/_apis/wit/attachments/chunked-1"})
			return
		}
		f.mu.Lock()
		f.uploads[name] = string(body)
		f.mu.Unlock()
		writeJSON(w, jx.M{"id": "new-1", "url": "https://dev.azure.com/CoolADO/_apis/wit/attachments/new-1"})
	})
	f.on("PUT", "/CoolADO/Tgt/_apis/wit/attachments/(.+)", func(w http.ResponseWriter, r *http.Request, _ []string, body []byte) {
		f.mu.Lock()
		f.ranges = append(f.ranges, r.Header.Get("Content-Range"))
		f.uploads["chunked"] += string(body)
		f.mu.Unlock()
		writeJSON(w, jx.M{})
	})
	// The link to work item 999 "already exists": the batch fails and the
	// client has to fall back to one op at a time.
	f.on("PATCH", `/CoolADO/_apis/wit/workitems/(\d+)`, func(w http.ResponseWriter, _ *http.Request, m []string, body []byte) {
		var ops []jx.M
		json.Unmarshal(body, &ops)
		for _, op := range ops {
			if strings.Contains(jx.Str(op, "value", "url"), "/workItems/999") {
				w.WriteHeader(400)
				io := `{"message":"TF201035: The link already exists"}`
				w.Write([]byte(io))
				return
			}
		}
		f.mu.Lock()
		var id int
		fmt.Sscan(m[1], &id)
		f.patches[id] = append(f.patches[id], ops...)
		f.mu.Unlock()
		writeJSON(w, jx.M{})
	})
	return f
}

func opValue(ops []jx.M, path string) (any, bool) {
	for _, op := range ops {
		if jx.Str(op, "path") == path {
			return op["value"], true
		}
	}
	return nil, false
}

func relURLs(ops []jx.M) []string {
	var out []string
	for _, op := range ops {
		if jx.Str(op, "path") == "/relations/-" {
			out = append(out, jx.Str(op, "value", "rel")+" "+jx.Str(op, "value", "url"))
		}
	}
	return out
}

func TestWorkItemsDryRunWritesNothing(t *testing.T) {
	f := newWIFake()
	sp := filepath.Join(t.TempDir(), "state.json")
	x := newTestCtx(t, f.fake, true, sp)
	if err := CloneWorkItems(context.Background(), x); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 0 || len(f.patches) != 0 || len(x.S.Map("workitem")) != 0 {
		t.Fatalf("dry run wrote: created=%d patches=%d map=%v", len(f.created), len(f.patches), x.S.Map("workitem"))
	}
	if f.count("POST", "/wiql") == 0 || f.count("POST", "/workitemsbatch") == 0 {
		t.Fatal("dry run should still read the source")
	}
}

func TestWorkItemsCopy(t *testing.T) {
	f := newWIFake()
	sp := filepath.Join(t.TempDir(), "state.json")
	x := newTestCtx(t, f.fake, false, sp)
	x.S.SetProjects(srcID, tgtID) // as if the project component created the target
	x.S.Put("repo", srcRepo, tgtRepo)
	if err := CloneWorkItems(context.Background(), x); err != nil {
		t.Fatal(err)
	}
	if x.Failures() != 0 {
		t.Fatalf("failures: %v", x.S.Todos())
	}
	// Created in ID order; the Test Plan item is left to testplans.
	want := map[int]string{101: "Epic", 102: "Task", 103: "Test Case", 104: "Shared Steps"}
	for id, typ := range want {
		if f.types[id] != typ {
			t.Errorf("target %d type %q, want %q", id, f.types[id], typ)
		}
	}
	if len(f.types) != 4 {
		t.Fatalf("created %d items: %v", len(f.types), f.types)
	}
	epic := f.created[101]
	for _, skipped := range []string{"/fields/System.TeamProject", "/fields/System.CommentCount", "/fields/WEF_1234_Kanban.Column"} {
		if _, ok := opValue(epic, skipped); ok {
			t.Errorf("%s should be skipped", skipped)
		}
	}
	if _, ok := opValue(f.created[102], "/fields/System.Parent"); ok {
		t.Error("System.Parent must not be copied")
	}
	if v, _ := opValue(epic, "/fields/System.AreaPath"); v != `Tgt\Team` {
		t.Errorf("area path %v", v)
	}
	if v, _ := opValue(epic, "/fields/System.CreatedBy"); v != "a@x.com" {
		t.Errorf("created by %v", v)
	}
	if v, _ := opValue(epic, "/fields/System.AssignedTo"); v != `[Tgt]\Leads` {
		t.Errorf("project group identity %v", v)
	}
	if rels := strings.Join(relURLs(epic), "\n"); !strings.Contains(rels, "Hyperlink https://dev.azure.com/CoolADO/Src/_workitems/edit/1") {
		t.Errorf("no backlink: %s", rels)
	}

	links := strings.Join(relURLs(f.patches[101]), "\n")
	for _, want := range []string{
		"System.LinkTypes.Hierarchy-Forward https://dev.azure.com/CoolADO/_apis/wit/workItems/102",
		"ArtifactLink vstfs:///Git/Commit/" + tgtID + "%2F" + tgtRepo + "%2Fabc123",
		"ArtifactLink vstfs:///Git/PullRequestId/" + srcID + "%2F" + srcRepo + "%2F7",
		"AttachedFile https://dev.azure.com/CoolADO/_apis/wit/attachments/new-1",
	} {
		if !strings.Contains(links, want) {
			t.Errorf("missing link %q in\n%s", want, links)
		}
	}
	if strings.Contains(links, "/999") {
		t.Error("duplicate external link should have been skipped, not applied")
	}
	if len(f.patches[102]) != 0 {
		t.Errorf("the child shouldn't add the reverse link itself: %v", relURLs(f.patches[102]))
	}
	if v, _ := opValue(f.patches[103], "/fields/Microsoft.VSTS.TCM.Steps"); !strings.Contains(fmt.Sprint(v), `ref="104"`) {
		t.Errorf("shared step ref not remapped: %v", v)
	}
	if f.uploads["a.txt"] != "hello world" {
		t.Errorf("attachment body %q", f.uploads["a.txt"])
	}
	if got := f.comments["101"]; len(got) != 2 || !strings.Contains(got[0], "[copied] A") || !strings.Contains(got[1], "second") {
		t.Errorf("comments %v", got)
	}

	// A rerun with the same checkpoint does nothing.
	creates, patches := len(f.created), f.count("PATCH", "")
	x2 := newTestCtx(t, f.fake, false, sp)
	if err := CloneWorkItems(context.Background(), x2); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != creates || f.count("PATCH", "") != patches || len(f.comments["101"]) != 2 {
		t.Fatal("rerun wrote again")
	}
}

func TestWorkItemsDryRunThenRealRun(t *testing.T) {
	f := newWIFake()
	sp := filepath.Join(t.TempDir(), "state.json")
	if err := CloneWorkItems(context.Background(), newTestCtx(t, f.fake, true, sp)); err != nil {
		t.Fatal(err)
	}
	x := newTestCtx(t, f.fake, false, sp)
	x.S.SetProjects(srcID, tgtID)
	if err := CloneWorkItems(context.Background(), x); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 4 {
		t.Fatalf("real run after a dry run created %d items, want 4", len(f.created))
	}
}

func TestChunkedAttachment(t *testing.T) {
	old1, old2 := singleUploadMax, chunkSize
	singleUploadMax, chunkSize = 4, 4
	defer func() { singleUploadMax, chunkSize = old1, old2 }()
	f := newWIFake()
	x := newTestCtx(t, f.fake, false, "")
	u, err := x.copyAttachment(context.Background(), attURL, "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(u, "chunked-1") || f.uploads["chunked"] != "hello world" {
		t.Fatalf("url=%s body=%q", u, f.uploads["chunked"])
	}
	if strings.Join(f.ranges, ",") != "bytes 0-3/11,bytes 4-7/11,bytes 8-10/11" {
		t.Fatalf("ranges %v", f.ranges)
	}
}
