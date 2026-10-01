package clone

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/coolado/adoclone/internal/client"
)

type node struct {
	ID         int            `json:"id"`
	Identifier string         `json:"identifier"`
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Children   []node         `json:"children"`
}

func getTree(ctx context.Context, x *Ctx, project, group string, depth int) (*node, error) {
	var root node
	u := x.C.URL("", client.P(project)+fmt.Sprintf("/_apis/wit/classificationnodes/%s?$depth=%d&api-version=7.1", group, depth))
	if err := x.C.Get(ctx, u, &root); err != nil {
		return nil, err
	}
	return &root, nil
}

// flatten indexes a tree by path relative to the root ("" is the root).
func flatten(n *node, prefix string, out map[string]*node) {
	out[prefix] = n
	for i := range n.Children {
		c := &n.Children[i]
		p := c.Name
		if prefix != "" {
			p = prefix + "/" + c.Name
		}
		flatten(c, p, out)
	}
}

func escapeSegments(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// CloneNodes creates every missing area and iteration path (with iteration
// dates), then records source->target node identifiers by path. Existing
// nodes are left alone, so reruns are safe.
func CloneNodes(ctx context.Context, x *Ctx) error {
	for _, group := range []string{"Areas", "Iterations"} {
		srcRoot, err := getTree(ctx, x, x.Src, group, 100)
		if err != nil {
			return err
		}
		srcFlat := map[string]*node{}
		flatten(srcRoot, "", srcFlat)
		x.read(len(srcFlat) - 1)

		have := map[string]*node{}
		if tgtRoot, err := getTree(ctx, x, x.Tgt, group, 100); err == nil {
			flatten(tgtRoot, "", have)
		} else if !(x.Dry() && client.IsNotFound(err)) {
			return err
		}
		if err := createMissing(ctx, x, group, srcRoot, "", have); err != nil {
			return err
		}
		if x.Dry() {
			continue
		}
		tgtRoot, err := getTree(ctx, x, x.Tgt, group, 100)
		if err != nil {
			return err
		}
		tgtFlat := map[string]*node{}
		flatten(tgtRoot, "", tgtFlat)
		for p, s := range srcFlat {
			if t, ok := tgtFlat[p]; ok {
				x.record("classNode", strings.ToLower(s.Identifier), t.Identifier)
				x.record("classNodeId", itoa(s.ID), itoa(t.ID))
			}
		}
	}
	return nil
}

func createMissing(ctx context.Context, x *Ctx, group string, n *node, prefix string, have map[string]*node) error {
	for i := range n.Children {
		c := &n.Children[i]
		p := c.Name
		if prefix != "" {
			p = prefix + "/" + c.Name
		}
		if _, ok := have[p]; !ok {
			u := x.tgt("_apis/wit/classificationnodes/" + group)
			if prefix != "" {
				u += "/" + escapeSegments(prefix)
			}
			body := map[string]any{"name": c.Name}
			if len(c.Attributes) > 0 {
				body["attributes"] = c.Attributes
			}
			if err := x.C.Post(ctx, u+"?api-version=7.1", body, nil); err != nil {
				return fmt.Errorf("create %s %s: %w", group, p, err)
			}
			x.wrote()
		}
		if err := createMissing(ctx, x, group, c, p, have); err != nil {
			return err
		}
	}
	return nil
}
