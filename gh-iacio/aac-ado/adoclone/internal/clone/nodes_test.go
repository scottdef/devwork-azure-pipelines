package clone

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/coolado/adoclone/internal/jx"
)

type ctxT = context.Context

func tree(root string, ids map[string]string, children ...jx.M) jx.M {
	return jx.M{"id": 1, "identifier": ids[root], "name": root, "children": children}
}

func TestNodesCreateOnlyMissingAndMap(t *testing.T) {
	f := &fake{}
	var mu sync.Mutex
	var created []string
	srcAreas := jx.M{"id": 1, "identifier": "s-root", "name": "Src", "children": []any{
		jx.M{"id": 2, "identifier": "s-a", "name": "A", "children": []any{jx.M{"id": 3, "identifier": "s-a1", "name": "A 1"}}},
		jx.M{"id": 4, "identifier": "s-b", "name": "B", "attributes": jx.M{"startDate": "2026-01-01T00:00:00Z", "finishDate": "2026-01-14T00:00:00Z"}},
	}}
	tgtBefore := jx.M{"id": 10, "identifier": "t-root", "name": "Tgt", "children": []any{jx.M{"id": 11, "identifier": "t-a", "name": "A"}}}
	tgtAfter := jx.M{"id": 10, "identifier": "t-root", "name": "Tgt", "children": []any{
		jx.M{"id": 11, "identifier": "t-a", "name": "A", "children": []any{jx.M{"id": 12, "identifier": "t-a1", "name": "A 1"}}},
		jx.M{"id": 13, "identifier": "t-b", "name": "B"},
	}}
	f.on("GET", "/CoolADO/Src/_apis/wit/classificationnodes/(Areas|Iterations)", func(w http.ResponseWriter, _ *http.Request, m []string, _ []byte) {
		if m[1] == "Areas" {
			writeJSON(w, srcAreas)
			return
		}
		writeJSON(w, jx.M{"id": 5, "identifier": "s-iroot", "name": "Src"})
	})
	f.on("GET", "/CoolADO/Tgt/_apis/wit/classificationnodes/(Areas|Iterations)", func(w http.ResponseWriter, _ *http.Request, m []string, _ []byte) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case m[1] == "Iterations":
			writeJSON(w, jx.M{"id": 15, "identifier": "t-iroot", "name": "Tgt"})
		case len(created) == 0:
			writeJSON(w, tgtBefore)
		default:
			writeJSON(w, tgtAfter)
		}
	})
	f.on("POST", "/CoolADO/Tgt/_apis/wit/classificationnodes/(.+)", func(w http.ResponseWriter, _ *http.Request, m []string, body []byte) {
		b, _ := jx.Decode(body)
		mu.Lock()
		created = append(created, m[1]+" <- "+jx.Str(b, "name")+" "+jx.Str(b, "attributes", "startDate"))
		mu.Unlock()
		writeJSON(w, jx.M{})
	})
	x := newTestCtx(t, f, false, "")
	if err := CloneNodes(context.Background(), x); err != nil {
		t.Fatal(err)
	}
	sort.Strings(created)
	want := "Areas <- B 2026-01-01T00:00:00Z,Areas/A <- A 1 "
	if got := strings.Join(created, ","); got != want {
		t.Fatalf("created %q\nwant    %q", got, want)
	}
	m := x.S.Map("classNode")
	for s, d := range map[string]string{"s-root": "t-root", "s-a": "t-a", "s-a1": "t-a1", "s-b": "t-b", "s-iroot": "t-iroot"} {
		if m[s] != d {
			t.Errorf("classNode %s -> %q, want %q", s, m[s], d)
		}
	}
}
