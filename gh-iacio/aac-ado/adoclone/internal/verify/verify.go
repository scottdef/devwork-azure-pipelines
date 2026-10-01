// Package verify compares the source and target projects: object counts per
// component, and a field-level check of a sample of copied work items.
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
	"github.com/coolado/adoclone/internal/state"
)

// Row is one component's count in each project. -1 means the count failed.
type Row struct {
	Component string `json:"component"`
	Source    int    `json:"source"`
	Target    int    `json:"target"`
	Note      string `json:"note,omitempty"`
}

// Missing reports a target that has fewer objects than the source (or a
// failed count). A target with more is fine: new projects come with a default
// repo, queues and so on.
func (r Row) Missing() bool { return r.Source < 0 || r.Target < 0 || r.Target < r.Source }

// Mismatch is one sampled work item whose copy differs.
type Mismatch struct {
	Source int      `json:"source"`
	Target int      `json:"target"`
	Fields []string `json:"fields"`
}

// FieldCheck is the work item sample result.
type FieldCheck struct {
	Sampled    int        `json:"sampled"`
	Matched    int        `json:"matched"`
	Mismatches []Mismatch `json:"mismatches,omitempty"`
}

// Result is everything verify and report produce.
type Result struct {
	Source string      `json:"source"`
	Target string      `json:"target,omitempty"`
	Counts []Row       `json:"counts"`
	Fields *FieldCheck `json:"fields,omitempty"`
	Todos  int         `json:"todos"`
}

// Problems counts missing objects and work item mismatches.
func (r Result) Problems() int {
	n := 0
	if r.Target == "" {
		return 0
	}
	for _, c := range r.Counts {
		if c.Missing() {
			n++
		}
	}
	if r.Fields != nil {
		n += len(r.Fields.Mismatches)
	}
	return n
}

type counter struct {
	name  string
	count func(ctx context.Context, c *client.Client, project string) (int, error)
}

func list(host, path string) func(context.Context, *client.Client, string) (int, error) {
	return func(ctx context.Context, c *client.Client, project string) (int, error) {
		items, err := c.List(ctx, c.URL(host, client.P(project)+"/_apis/"+path))
		return len(items), err
	}
}

func nodes(group string) func(context.Context, *client.Client, string) (int, error) {
	return func(ctx context.Context, c *client.Client, project string) (int, error) {
		type node struct {
			Children []node `json:"children"`
		}
		var root node
		if err := c.Get(ctx, c.URL("", client.P(project)+"/_apis/wit/classificationnodes/"+group+"?$depth=100&api-version=7.1"), &root); err != nil {
			return 0, err
		}
		var walk func(n node) int
		walk = func(n node) int {
			t := len(n.Children)
			for _, ch := range n.Children {
				t += walk(ch)
			}
			return t
		}
		return walk(root), nil
	}
}

var counters = []counter{
	{"workItems", func(ctx context.Context, c *client.Client, p string) (int, error) {
		ids, err := c.WorkItemIDs(ctx, p, "")
		return len(ids), err
	}},
	{"areas", nodes("Areas")},
	{"iterations", nodes("Iterations")},
	{"teams", func(ctx context.Context, c *client.Client, p string) (int, error) {
		items, err := c.List(ctx, c.URL("", "_apis/projects/"+client.P(p)+"/teams?$top=5000&api-version=7.1"))
		return len(items), err
	}},
	{"repos", list("", "git/repositories?api-version=7.1")},
	{"buildDefinitions", list("", "build/definitions?api-version=7.1")},
	{"releaseDefinitions", list(client.VSRM, "release/definitions?api-version=7.1")},
	{"taskGroups", list("", "distributedtask/taskgroups?api-version=7.1")},
	{"variableGroups", list("", "distributedtask/variablegroups?api-version=7.1")},
	{"queues", list("", "distributedtask/queues?api-version=7.1")},
	{"environments", list("", "distributedtask/environments?$top=1000&api-version=7.1")},
	{"deploymentGroups", list("", "distributedtask/deploymentgroups?api-version=7.1")},
	{"serviceConnections", list("", "serviceendpoint/endpoints?api-version=7.1")},
	{"policies", list("", "policy/configurations?api-version=7.1")},
	{"testPlans", list("", "testplan/plans?api-version=7.1")},
	{"deliveryPlans", list("", "work/plans?api-version=7.1")},
	{"wikis", list("", "wiki/wikis?api-version=7.1")},
	{"feeds", list(client.Feeds, "packaging/feeds?api-version=7.1")},
}

// Counts counts each component in src and, if tgt isn't empty, in tgt.
func Counts(ctx context.Context, c *client.Client, src, tgt string) []Row {
	rows := make([]Row, 0, len(counters))
	for _, k := range counters {
		row := Row{Component: k.name, Target: -1}
		n, err := k.count(ctx, c, src)
		row.Source = n
		if err != nil {
			row.Source, row.Note = -1, "source: "+short(err)
		}
		if tgt != "" {
			n, err := k.count(ctx, c, tgt)
			row.Target = n
			if err != nil {
				row.Target, row.Note = -1, strings.TrimSpace(row.Note+" target: "+short(err))
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func short(err error) string {
	s := err.Error()
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

type wi struct {
	ID        int            `json:"id"`
	Fields    map[string]any `json:"fields"`
	Relations []struct {
		Rel string `json:"rel"`
		URL string `json:"url"`
	} `json:"relations"`
}

func fetch(ctx context.Context, c *client.Client, ids []int) (map[int]wi, error) {
	out := map[int]wi{}
	for i := 0; i < len(ids); i += 200 {
		j := i + 200
		if j > len(ids) {
			j = len(ids)
		}
		var r struct {
			Value []*wi `json:"value"`
		}
		body := jx.M{"ids": ids[i:j], "$expand": "Relations", "errorPolicy": "omit"}
		if err := c.Post(ctx, c.URL("", "_apis/wit/workitemsbatch?api-version=7.1"), body, &r); err != nil {
			return nil, err
		}
		for _, w := range r.Value {
			if w != nil {
				out[w.ID] = *w
			}
		}
	}
	return out, nil
}

var wiLink = regexp.MustCompile(`(?i)/_apis/wit/workItems/\d+$`)

func linkCounts(w wi) (links, files int) {
	for _, r := range w.Relations {
		switch {
		case r.Rel == "AttachedFile":
			files++
		case !strings.Contains(r.Rel, ".Remote.") && wiLink.MatchString(r.URL):
			links++
		}
	}
	return
}

// Fields compares pct percent of the copied work items (every Nth by source
// ID): type, title, state, area and iteration path (rebased to the target
// project), work item link count and attachment count.
func Fields(ctx context.Context, c *client.Client, s *state.State, src, tgt string, pct float64) (*FieldCheck, error) {
	fc := &FieldCheck{}
	m := s.Map("workitem")
	if pct <= 0 || len(m) == 0 {
		return fc, nil
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		if n, err := strconv.Atoi(k); err == nil {
			keys = append(keys, n)
		}
	}
	sort.Ints(keys)
	step := int(math.Ceil(100 / pct))
	var srcIDs, tgtIDs []int
	for i := 0; i < len(keys); i += step {
		t, _ := strconv.Atoi(m[strconv.Itoa(keys[i])])
		srcIDs = append(srcIDs, keys[i])
		tgtIDs = append(tgtIDs, t)
	}
	sw, err := fetch(ctx, c, srcIDs)
	if err != nil {
		return nil, err
	}
	tw, err := fetch(ctx, c, tgtIDs)
	if err != nil {
		return nil, err
	}
	for i, sid := range srcIDs {
		a, okA := sw[sid]
		b, okB := tw[tgtIDs[i]]
		if !okA {
			continue // deleted from the source since the copy
		}
		fc.Sampled++
		if !okB {
			fc.Mismatches = append(fc.Mismatches, Mismatch{sid, tgtIDs[i], []string{"missing in target"}})
			continue
		}
		var bad []string
		for _, f := range []string{"System.WorkItemType", "System.Title", "System.State"} {
			if fmt.Sprint(a.Fields[f]) != fmt.Sprint(b.Fields[f]) {
				bad = append(bad, f)
			}
		}
		for _, f := range []string{"System.AreaPath", "System.IterationPath"} {
			if !strings.EqualFold(jx.RebasePath(jx.Str(a.Fields, f), src, tgt), jx.Str(b.Fields, f)) {
				bad = append(bad, f)
			}
		}
		la, fa := linkCounts(a)
		lb, fb := linkCounts(b)
		if la != lb {
			bad = append(bad, fmt.Sprintf("links %d/%d", la, lb))
		}
		if fa != fb {
			bad = append(bad, fmt.Sprintf("attachments %d/%d", fa, fb))
		}
		if len(bad) == 0 {
			fc.Matched++
		} else {
			fc.Mismatches = append(fc.Mismatches, Mismatch{sid, tgtIDs[i], bad})
		}
	}
	return fc, nil
}

// JSON renders a result for stdout.
func (r Result) JSON() []byte {
	b, _ := json.MarshalIndent(r, "", "  ")
	return append(b, '\n')
}
