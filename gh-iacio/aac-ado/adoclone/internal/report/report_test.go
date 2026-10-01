package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coolado/adoclone/internal/state"
	"github.com/coolado/adoclone/internal/verify"
)

func TestWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.html")
	res := verify.Result{Source: "Src", Target: "Tgt",
		Counts: []verify.Row{{Component: "repos", Source: 3, Target: 2}},
		Fields: &verify.FieldCheck{Sampled: 4, Matched: 3, Mismatches: []verify.Mismatch{{Source: 7, Target: 70, Fields: []string{"System.Title", "links 2/1"}}}},
		Todos:  1}
	todos := []state.Todo{{Component: "vargroups", Item: "App / <pass>", Action: "set it"}}
	if err := Write(p, res, todos); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	html := string(b)
	for _, want := range []string{"echarts", "adoclone: Src vs Tgt", "Work item sample: 3 of 4 match", "System.Title, links 2/1", "App / &lt;pass&gt;", "#fdecea"} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
}
