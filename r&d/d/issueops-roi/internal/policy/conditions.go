// Package policy evaluates registry conditions against request facts and
// computes the effective approval plan (base rules + matching escalations).
package policy

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/issueform"
)

// Facts are the values conditions are evaluated against: every parsed form
// field (dropdowns collapse to a string when single-valued, checkboxes become
// the list of selected labels) plus derived facts prefixed with "$".
type Facts map[string]any

// FactsFromValues normalizes parsed form values into facts.
func FactsFromValues(v issueform.Values) Facts {
	f := Facts{}
	for _, k := range v.Keys() {
		switch v[k].(type) {
		case issueform.Checkboxes, *issueform.Checkboxes, map[string]any:
			f[k] = v.Selected(k)
		case []string, []any:
			l := v.List(k)
			if len(l) == 1 {
				f[k] = l[0]
			} else {
				f[k] = l
			}
		default:
			f[k] = v.String(k)
		}
	}
	return f
}

// Merge copies extra facts into f and returns f.
func (f Facts) Merge(extra map[string]any) Facts {
	for k, v := range extra {
		f[k] = v
	}
	return f
}

// Eval reports whether the condition holds.
func Eval(c config.Condition, f Facts) (bool, error) {
	for _, p := range c.All {
		ok, err := evalPredicate(p, f)
		if err != nil || !ok {
			return false, err
		}
	}
	if len(c.Any) == 0 {
		return len(c.All) > 0, nil
	}
	for _, p := range c.Any {
		ok, err := evalPredicate(p, f)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		s := strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(x), "$"), ",", "")
		n, err := strconv.ParseFloat(s, 64)
		return n, err == nil
	}
	return 0, false
}

func asStrings(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []string{x}
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	}
	return []string{fmt.Sprint(v)}
}

func evalPredicate(p config.Predicate, f Facts) (bool, error) {
	fact, present := f[p.Field]
	switch p.Op {
	case "empty":
		return !present || len(strings.TrimSpace(strings.Join(asStrings(fact), ""))) == 0, nil
	case "not_empty":
		return present && len(strings.TrimSpace(strings.Join(asStrings(fact), ""))) > 0, nil
	}
	if !present {
		return false, nil
	}
	switch p.Op {
	case "eq", "ne":
		eq := false
		if a, ok := asFloat(fact); ok {
			if b, ok2 := asFloat(p.Value); ok2 {
				eq = a == b
				if p.Op == "ne" {
					return !eq, nil
				}
				return eq, nil
			}
		}
		for _, s := range asStrings(fact) {
			if strings.EqualFold(s, fmt.Sprint(p.Value)) {
				eq = true
			}
		}
		if p.Op == "ne" {
			return !eq, nil
		}
		return eq, nil
	case "gt", "gte", "lt", "lte":
		a, ok := asFloat(fact)
		b, ok2 := asFloat(p.Value)
		if !ok || !ok2 {
			return false, nil // non-numeric input never satisfies a numeric bound
		}
		switch p.Op {
		case "gt":
			return a > b, nil
		case "gte":
			return a >= b, nil
		case "lt":
			return a < b, nil
		default:
			return a <= b, nil
		}
	case "in", "not_in":
		set := asStrings(p.Value)
		in := false
		for _, s := range asStrings(fact) {
			for _, c := range set {
				if strings.EqualFold(s, c) {
					in = true
				}
			}
		}
		if p.Op == "not_in" {
			return !in, nil
		}
		return in, nil
	case "contains":
		want := fmt.Sprint(p.Value)
		switch x := fact.(type) {
		case string:
			return strings.Contains(strings.ToLower(x), strings.ToLower(want)), nil
		default:
			for _, s := range asStrings(x) {
				if s == want {
					return true, nil
				}
			}
			return false, nil
		}
	case "matches":
		re, err := regexp.Compile(fmt.Sprint(p.Value))
		if err != nil {
			return false, err
		}
		for _, s := range asStrings(fact) {
			if re.MatchString(s) {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("policy: unknown op %q", p.Op)
}

// Plan is the effective approval requirement for one request.
type Plan struct {
	AutoApproved bool          `json:"auto_approved"`
	Rules        []config.Rule `json:"rules"`
	Escalations  []string      `json:"escalations"`
	Environment  string        `json:"environment"`
}

// BuildPlan evaluates auto-approval and escalations for a request.
func BuildPlan(rt *config.RequestType, f Facts) (*Plan, error) {
	p := &Plan{Environment: config.ResolveEnvironment(rt.Execute.Environment, f)}
	for _, e := range rt.Approval.Escalations {
		ok, err := Eval(e.When, f)
		if err != nil {
			return nil, fmt.Errorf("policy: escalation %q: %w", e.Name, err)
		}
		if ok {
			p.Escalations = append(p.Escalations, e.Name)
			p.Rules = append(p.Rules, e.Rules...)
			if e.Environment != "" {
				p.Environment = config.ResolveEnvironment(e.Environment, f)
			}
		}
	}
	auto := false
	if rt.Approval.AutoApprove != nil && len(p.Escalations) == 0 {
		ok, err := Eval(*rt.Approval.AutoApprove, f)
		if err != nil {
			return nil, fmt.Errorf("policy: auto_approve: %w", err)
		}
		auto = ok
	}
	if auto {
		p.AutoApproved = true
		p.Rules = nil
		return p, nil
	}
	p.Rules = append(append([]config.Rule{}, rt.Approval.Rules...), p.Rules...)
	return p, nil
}

// StaticAllowlist returns the github/command `allowlist` value for approver
// commands: "org/team" and user handles. When any rule depends on roles,
// repository permissions, named-field users or the requestor, the static
// allowlist cannot express it and "false" (disabled) is returned; the Go
// authorizer remains the authoritative gate in every case.
func StaticAllowlist(org string, rules []config.Rule) string {
	set := map[string]bool{}
	for _, r := range rules {
		s := r.AnyOf
		if len(s.OrgRoles)+len(s.RepoPermissions)+len(s.TargetRepoPermissions)+len(s.FieldUsers) > 0 || s.Requestor {
			return "false"
		}
		for _, t := range s.Teams {
			set[org+"/"+t] = true
		}
		for _, u := range s.Users {
			set[u] = true
		}
	}
	if len(set) == 0 {
		return "false"
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
