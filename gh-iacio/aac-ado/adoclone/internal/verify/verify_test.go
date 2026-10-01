package verify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/state"
)

func TestProblems(t *testing.T) {
	r := Result{Source: "S", Target: "T", Counts: []Row{
		{"repos", 3, 4, ""},       // more in target: fine
		{"policies", 5, 4, ""},    // missing
		{"feeds", -1, 0, "error"}, // failed count
	}, Fields: &FieldCheck{Mismatches: []Mismatch{{1, 2, []string{"System.Title"}}}}}
	if r.Problems() != 3 {
		t.Fatalf("problems = %d", r.Problems())
	}
	if (Result{Source: "S", Counts: r.Counts}).Problems() != 0 {
		t.Fatal("plan (no target) never has problems")
	}
}

func TestFieldsSample(t *testing.T) {
	items := map[int]map[string]any{
		1:   {"System.WorkItemType": "Bug", "System.Title": "A", "System.State": "New", "System.AreaPath": `Src\X`, "System.IterationPath": "Src"},
		2:   {"System.WorkItemType": "Bug", "System.Title": "B", "System.State": "New", "System.AreaPath": "Src", "System.IterationPath": "Src"},
		101: {"System.WorkItemType": "Bug", "System.Title": "A", "System.State": "New", "System.AreaPath": `Tgt\X`, "System.IterationPath": "Tgt"},
		102: {"System.WorkItemType": "Bug", "System.Title": "changed", "System.State": "New", "System.AreaPath": "Tgt", "System.IterationPath": "Tgt"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ IDs []int }
		json.NewDecoder(r.Body).Decode(&b)
		var out []any
		for _, id := range b.IDs {
			out = append(out, map[string]any{"id": id, "fields": items[id]})
		}
		json.NewEncoder(w).Encode(map[string]any{"value": out})
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c := client.New("CoolADO", "pat", slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	c.Rewrite = func(x *url.URL) { x.Scheme, x.Host = "http", u.Host }
	s, _ := state.Load(filepath.Join(t.TempDir(), "state.json"))
	s.Put("workitem", "1", "101")
	s.Put("workitem", "2", "102")
	fc, err := Fields(context.Background(), c, s, "Src", "Tgt", 100)
	if err != nil {
		t.Fatal(err)
	}
	if fc.Sampled != 2 || fc.Matched != 1 || len(fc.Mismatches) != 1 || fc.Mismatches[0].Fields[0] != "System.Title" {
		t.Fatalf("%+v", fc)
	}
}
