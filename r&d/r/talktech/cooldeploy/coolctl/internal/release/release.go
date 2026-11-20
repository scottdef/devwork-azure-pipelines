// Package release holds the one data structure every deployment operation
// edits: the values file of an app in an environment, and its release list.
//
// Deploy, promote, rollback and rolling release are all small, pure changes to
// that list. Everything else in the platform exists to carry one of those
// changes to a cluster.
package release

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/CoolEngOrg/coolctl/internal/manifest"
)

// Release is one immutable image that may receive traffic.
type Release struct {
	ID      string `json:"id"`
	Digest  string `json:"digest"`
	Weight  int    `json:"weight"`
	Standby bool   `json:"standby,omitempty"`
	SHA     string `json:"sha,omitempty"`
	At      string `json:"at,omitempty"`
}

// Values is the values.json of one app in one environment. The cool-app chart
// reads it as Helm values; cool-api writes it.
type Values struct {
	Team        string             `json:"team"`
	App         string             `json:"app"`
	Environment string             `json:"environment"`
	Chart       string             `json:"chart"`
	Preview     int                `json:"preview,omitempty"`
	Group       string             `json:"group,omitempty"`
	Identity    string             `json:"identity,omitempty"`
	Expose      string             `json:"expose"`
	Port        int                `json:"port"`
	Health      string             `json:"health"`
	Scale       manifest.Scale     `json:"scale"`
	Resources   manifest.Resources `json:"resources"`
	Env         map[string]string  `json:"env,omitempty"`
	Overrides   map[string]string  `json:"overrides,omitempty"`
	Secrets     []string           `json:"secrets,omitempty"`
	Routes      []manifest.Route   `json:"routes,omitempty"`
	Crons       []manifest.Cron    `json:"crons,omitempty"`
	Flags       map[string]any     `json:"flags,omitempty"`
	Domains     []string           `json:"domains,omitempty"`
	AI          *manifest.AI       `json:"ai,omitempty"`
	Releases    []Release          `json:"releases"`
	Manifest    *manifest.Manifest `json:"manifest,omitempty"`
}

var (
	reID     = regexp.MustCompile(`^r[0-9a-f]{7}$`)
	reDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	reSHA    = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// ErrNoStandby is returned when a rollback has nothing to roll back to.
var ErrNoStandby = errors.New("no other release to roll back to")

// ID derives a release ID from a commit SHA: r plus its first seven digits.
func ID(sha string) (string, error) {
	sha = strings.ToLower(sha)
	if !reSHA.MatchString(sha) {
		return "", fmt.Errorf("commit %q: want 7 to 40 hex digits", sha)
	}
	return "r" + sha[:7], nil
}

// ValidDigest reports whether s is an image digest.
func ValidDigest(s string) bool { return reDigest.MatchString(s) }

// New returns the values of an app that has no release yet.
func New(team, app, env, chart string) Values {
	m := manifest.Default(team, app, "dockerfile")
	v := Values{Team: team, App: app, Environment: env, Chart: chart}
	v.Apply(m)
	v.Manifest = nil
	return v
}

// Apply copies the manifest's settings into v, with the overrides the manifest
// declares for v's environment. Releases, domains and IssueOps overrides are
// left alone: they belong to the environment, not to the source tree.
func (v *Values) Apply(m manifest.Manifest) {
	v.Expose, v.Port, v.Health = m.Expose, m.Port, m.Health
	v.Scale, v.Resources = m.Scale, m.Resources
	v.Env = maps.Clone(m.Env)
	v.Secrets, v.Routes, v.Crons, v.Flags, v.AI = m.Secrets, m.Routes, m.Crons, m.Flags, m.AI
	if o, ok := m.Environments[v.Environment]; ok {
		if o.Scale != nil {
			v.Scale = *o.Scale
		}
		if o.Resources != nil {
			v.Resources = *o.Resources
		}
		if len(o.Env) > 0 {
			if v.Env == nil {
				v.Env = map[string]string{}
			}
			maps.Copy(v.Env, o.Env)
		}
	}
	v.Manifest = &m
}

// Active returns the index of the release that carries the most traffic,
// or -1 when rs is empty. Ties go to the earlier, newer entry.
func Active(rs []Release) int {
	a := -1
	for i, r := range rs {
		if a < 0 || r.Weight > rs[a].Weight {
			a = i
		}
	}
	return a
}

// Find returns the release with the given ID.
func Find(rs []Release, id string) (Release, bool) {
	for _, r := range rs {
		if r.ID == id {
			return r, true
		}
	}
	return Release{}, false
}

// Deploy makes n the only release with traffic. The release that was active
// stays as the standby, so a rollback needs no build and no pull.
func Deploy(rs []Release, n Release) []Release {
	n.Weight, n.Standby = 100, false
	out := []Release{n}
	a := Active(rs)
	switch {
	case a < 0:
	case rs[a].ID != n.ID:
		old := rs[a]
		old.Weight, old.Standby = 0, true
		out = append(out, old)
	default: // n is already active: keep whatever stood by.
		for _, r := range rs {
			if r.ID != n.ID {
				r.Weight, r.Standby = 0, true
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Rollback swaps the active release with the other one.
// Calling it twice rolls forward again.
func Rollback(rs []Release) ([]Release, error) {
	a := Active(rs)
	if a < 0 || len(rs) < 2 {
		return nil, ErrNoStandby
	}
	b := 0
	if a == 0 {
		b = 1
	}
	out := []Release{rs[min(a, b)], rs[max(a, b)]}
	for i := range out {
		if out[i].ID == rs[a].ID {
			out[i].Weight, out[i].Standby = 0, true
		} else {
			out[i].Weight, out[i].Standby = 100, false
		}
	}
	return out, nil
}

// Stage sends weight percent of traffic to n and the rest to the release n is
// replacing. Weight 100 completes a rolling release; weight 0 halts it and
// leaves n on standby.
func Stage(rs []Release, n Release, weight int) ([]Release, error) {
	if weight < 0 || weight > 100 {
		return nil, fmt.Errorf("weight %d: want 0 to 100", weight)
	}
	if old, ok := Find(rs, n.ID); ok { // n is already listed: keep what is known of it.
		if n.Digest == "" {
			n.Digest = old.Digest
		}
		if n.SHA == "" {
			n.SHA = old.SHA
		}
		if n.At == "" {
			n.At = old.At
		}
	}
	base := -1
	for i, r := range rs {
		if r.ID != n.ID && (base < 0 || r.Weight > rs[base].Weight) {
			base = i
		}
	}
	if base < 0 {
		if weight != 100 {
			return nil, errors.New("a first release takes all traffic; there is nothing to share it with")
		}
		n.Weight, n.Standby = 100, false
		return []Release{n}, nil
	}
	old := rs[base]
	n.Weight, old.Weight = weight, 100-weight
	n.Standby, old.Standby = weight == 0, weight == 100
	return []Release{n, old}, nil
}

// Retire drops releases that carry no traffic.
func Retire(rs []Release) []Release {
	var out []Release
	for _, r := range rs {
		if r.Weight > 0 {
			out = append(out, r)
		}
	}
	return out
}

// Validate checks the invariants the chart relies on.
func Validate(rs []Release) error {
	if len(rs) == 0 || len(rs) > 3 {
		return fmt.Errorf("want 1 to 3 releases, got %d", len(rs))
	}
	sum, seen := 0, map[string]bool{}
	for _, r := range rs {
		switch {
		case !reID.MatchString(r.ID):
			return fmt.Errorf("release id %q: want r plus 7 hex digits", r.ID)
		case seen[r.ID]:
			return fmt.Errorf("release %s listed twice", r.ID)
		case !ValidDigest(r.Digest):
			return fmt.Errorf("release %s: digest %q is not sha256:<64 hex>", r.ID, r.Digest)
		case r.Weight < 0 || r.Weight > 100:
			return fmt.Errorf("release %s: weight %d out of range", r.ID, r.Weight)
		case r.Standby && r.Weight != 0:
			return fmt.Errorf("release %s: a standby carries no traffic", r.ID)
		}
		seen[r.ID] = true
		sum += r.Weight
	}
	if sum != 100 {
		return fmt.Errorf("weights sum to %d, want 100", sum)
	}
	return nil
}
