// Package broker tracks every device the staff can call on and decides where
// a piece of work should run.
//
// The placement rule is the system's standing order about resources: when
// work needs something (CPU for OCR, a PDF converter, a shell on a machine),
// look first for a reachable computer or device that can lend it, then for a
// staff board, and spend the chief of staff's own operating system only when
// nobody else can do the job.
package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tier orders providers. Lower tiers are tried first.
type Tier int

const (
	Lender Tier = iota // a reachable computer or device lending resources
	Staff              // a satellite or worker board belonging to the system
	Self               // the chief of staff's own OS: the last resort
)

var tierNames = []string{"lender", "staff", "self"}

func (t Tier) String() string {
	if int(t) < len(tierNames) {
		return tierNames[t]
	}
	return fmt.Sprintf("tier(%d)", int(t))
}

func (t Tier) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

func (t *Tier) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for i, n := range tierNames {
		if strings.EqualFold(s, n) {
			*t = Tier(i)
			return nil
		}
	}
	return fmt.Errorf("broker: unknown tier %q", s)
}

// Node is one tracked device.
//
// Caps are plain strings. General ones ("stt", "ocr", "pdf", "tpu") may be
// offered by several nodes. Target-bound ones ("hands:server3",
// "eyes:server3", "exec:server3") name the node that holds that target's
// tether or shell.
type Node struct {
	Name      string    `json:"name"`
	Tier      Tier      `json:"tier"`
	Addr      string    `json:"addr,omitempty"` // host:port of its cos API
	SSH       string    `json:"ssh,omitempty"`  // user@host for agentless lenders
	Caps      []string  `json:"caps"`
	MemFreeMB int       `json:"mem_free_mb,omitempty"` // 0 means not reported
	Load      float64   `json:"load,omitempty"`
	Seen      time.Time `json:"seen"`
}

// Has reports whether the node offers a capability.
func (n Node) Has(cap string) bool {
	for _, c := range n.Caps {
		if c == cap {
			return true
		}
	}
	return false
}

// Need describes what a piece of work requires.
type Need struct {
	Cap   string
	MemMB int
}

// ErrNoProvider means no live node offers the capability.
var ErrNoProvider = errors.New("no live provider")

// Registry is the tracked set of nodes.
type Registry struct {
	mu      sync.Mutex
	nodes   map[string]*Node
	TTL     time.Duration // a node unseen for longer is treated as down
	MaxLoad float64       // a node at or above this load is passed over
}

// New builds a registry from configured nodes.
func New(nodes []Node) (*Registry, error) {
	r := &Registry{nodes: map[string]*Node{}, TTL: 45 * time.Second, MaxLoad: 4}
	for i := range nodes {
		n := nodes[i]
		if n.Name == "" {
			return nil, errors.New("broker: node without a name")
		}
		if _, dup := r.nodes[n.Name]; dup {
			return nil, fmt.Errorf("broker: duplicate node %q", n.Name)
		}
		r.nodes[n.Name] = &n
	}
	return r, nil
}

// Beat records a heartbeat with fresh load figures.
func (r *Registry) Beat(name string, load float64, memFreeMB int, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.nodes[name]
	if !ok {
		return fmt.Errorf("broker: unknown node %q", name)
	}
	n.Load, n.MemFreeMB, n.Seen = load, memFreeMB, now
	return nil
}

// Touch records that a node answered a probe.
func (r *Registry) Touch(name string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[name]; ok {
		n.Seen = now
	}
}

// Alive reports whether a node has been seen recently. The chief of staff's
// own node is always alive.
func (r *Registry) Alive(n Node, now time.Time) bool {
	return n.Tier == Self || (!n.Seen.IsZero() && now.Sub(n.Seen) <= r.TTL)
}

// Snapshot returns all nodes, ordered by tier then name.
func (r *Registry) Snapshot() []Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		c := *n
		c.Caps = append([]string(nil), n.Caps...)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tier != out[j].Tier {
			return out[i].Tier < out[j].Tier
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Place picks the node that should do the work: among live nodes offering
// the capability with room to spare, the lowest tier wins, then the lowest
// load, then the name for a stable answer.
func (r *Registry) Place(need Need, now time.Time) (Node, error) {
	var fit []Node
	for _, n := range r.Snapshot() {
		if !n.Has(need.Cap) || !r.Alive(n, now) {
			continue
		}
		if n.MemFreeMB != 0 && n.MemFreeMB < need.MemMB {
			continue
		}
		if n.Tier != Self && n.Load >= r.MaxLoad {
			continue
		}
		fit = append(fit, n)
	}
	if len(fit) == 0 {
		return Node{}, fmt.Errorf("%w for %q", ErrNoProvider, need.Cap)
	}
	sort.SliceStable(fit, func(i, j int) bool {
		if fit[i].Tier != fit[j].Tier {
			return fit[i].Tier < fit[j].Tier
		}
		if fit[i].Load != fit[j].Load {
			return fit[i].Load < fit[j].Load
		}
		return fit[i].Name < fit[j].Name
	})
	return fit[0], nil
}
