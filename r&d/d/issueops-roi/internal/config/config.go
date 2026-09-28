// Package config loads the IssueOps request-type registry (config/issueops.json).
//
// The registry is the single source of truth that binds an issue form template
// to its type label, approval policy, execution workflow, GitHub environment
// and type-specific settings. Everything is JSON so the Go tooling needs only
// the standard library.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Registry is the root of config/issueops.json.
type Registry struct {
	Version       int           `json:"version"`
	Enterprise    string        `json:"enterprise"`
	Organization  string        `json:"organization"`
	Repository    string        `json:"repository"`
	PlatformLabel string        `json:"platform_label"`
	DocsURL       string        `json:"docs_url"`
	Pricing       Pricing       `json:"pricing"`
	RequestTypes  []RequestType `json:"request_types"`
}

// Pricing holds the unit prices used for cost attribution.
type Pricing struct {
	AICreditUSD    float64 `json:"ai_credit_usd"`
	SeatMonthlyUSD float64 `json:"seat_monthly_usd"`
	SeatPlan       string  `json:"seat_plan"`
	Note           string  `json:"note,omitempty"`
}

// RequestType binds a form to its policy and execution.
type RequestType struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Template        string          `json:"template"`
	Label           string          `json:"label"`
	Execute         Execute         `json:"execute"`
	CloseOnComplete bool            `json:"close_on_complete"`
	QA              bool            `json:"qa,omitempty"`
	Requestors      *Requestors     `json:"requestors,omitempty"`
	Approval        Approval        `json:"approval"`
	Settings        json.RawMessage `json:"settings,omitempty"`
}

// Execute names the reusable workflow and the GitHub environment that gates it.
// Environment may contain {fact} placeholders resolved from request facts.
type Execute struct {
	Workflow    string `json:"workflow"`
	Environment string `json:"environment"`
}

// Requestors restricts who may submit a request type.
type Requestors struct {
	TargetRepoField string   `json:"target_repo_field,omitempty"`
	AnyOf           Selector `json:"any_of"`
}

// Approval is the approval policy of a request type.
type Approval struct {
	RequireSubmit     bool         `json:"require_submit"`
	AllowSelfApproval bool         `json:"allow_self_approval"`
	DistinctApprovers bool         `json:"distinct_approvers"`
	TargetRepoField   string       `json:"target_repo_field,omitempty"`
	AutoApprove       *Condition   `json:"auto_approve,omitempty"`
	Rules             []Rule       `json:"rules"`
	Escalations       []Escalation `json:"escalations,omitempty"`
}

// Rule requires Min distinct approvers matching AnyOf.
type Rule struct {
	Name  string   `json:"name"`
	Min   int      `json:"min"`
	AnyOf Selector `json:"any_of"`
}

// Selector matches approvers (or requestors). A user matches when ANY listed
// criterion matches.
type Selector struct {
	Teams                 []string `json:"teams,omitempty"`
	Users                 []string `json:"users,omitempty"`
	OrgRoles              []string `json:"org_roles,omitempty"`
	RepoPermissions       []string `json:"repo_permissions,omitempty"`
	TargetRepoPermissions []string `json:"target_repo_permissions,omitempty"`
	FieldUsers            []string `json:"field_users,omitempty"`
	Requestor             bool     `json:"requestor,omitempty"`
}

// IsEmpty reports whether the selector can never match.
func (s Selector) IsEmpty() bool {
	return len(s.Teams)+len(s.Users)+len(s.OrgRoles)+len(s.RepoPermissions)+
		len(s.TargetRepoPermissions)+len(s.FieldUsers) == 0 && !s.Requestor
}

// Escalation adds rules (and optionally overrides the environment) when its
// condition matches the request facts.
type Escalation struct {
	Name        string    `json:"name"`
	When        Condition `json:"when"`
	Rules       []Rule    `json:"rules"`
	Environment string    `json:"environment,omitempty"`
}

// Condition is satisfied when all of All and at least one of Any (if present) hold.
type Condition struct {
	All []Predicate `json:"all,omitempty"`
	Any []Predicate `json:"any,omitempty"`
}

// Predicate compares a request fact with a value.
// Ops: eq, ne, gt, gte, lt, lte, in, not_in, contains, matches, empty, not_empty.
type Predicate struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

// Load reads and validates a registry file.
func Load(path string) (*Registry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return Parse(b)
}

// Parse decodes and validates registry JSON.
func Parse(b []byte) (*Registry, error) {
	var r Registry
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("config: decode registry: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

var (
	idPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	validOps     = map[string]bool{"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true, "in": true, "not_in": true, "contains": true, "matches": true, "empty": true, "not_empty": true}
	validRepoPrm = map[string]bool{"read": true, "triage": true, "write": true, "maintain": true, "admin": true}
)

// Validate checks structural invariants so a bad registry fails CI, not a request.
func (r *Registry) Validate() error {
	var errs []error
	if r.Organization == "" {
		errs = append(errs, errors.New("organization is required"))
	}
	if r.PlatformLabel == "" {
		errs = append(errs, errors.New("platform_label is required"))
	}
	seenID := map[string]bool{}
	seenLabel := map[string]bool{}
	seenTemplate := map[string]bool{}
	for i, t := range r.RequestTypes {
		p := fmt.Sprintf("request_types[%d] (%s)", i, t.ID)
		if !idPattern.MatchString(t.ID) {
			errs = append(errs, fmt.Errorf("%s: invalid id", p))
		}
		if seenID[t.ID] || seenLabel[t.Label] || seenTemplate[t.Template] {
			errs = append(errs, fmt.Errorf("%s: duplicate id, label or template", p))
		}
		seenID[t.ID], seenLabel[t.Label], seenTemplate[t.Template] = true, true, true
		if !strings.HasPrefix(t.Label, r.PlatformLabel+":") {
			errs = append(errs, fmt.Errorf("%s: label must start with %q", p, r.PlatformLabel+":"))
		}
		if !strings.HasSuffix(t.Template, ".yml") {
			errs = append(errs, fmt.Errorf("%s: template must be a .yml issue form", p))
		}
		if t.Execute.Workflow == "" {
			errs = append(errs, fmt.Errorf("%s: execute.workflow is required", p))
		}
		errs = append(errs, validateRules(p+".approval.rules", t.Approval.Rules)...)
		for j, e := range t.Approval.Escalations {
			ep := fmt.Sprintf("%s.approval.escalations[%d]", p, j)
			errs = append(errs, validateCondition(ep+".when", e.When)...)
			errs = append(errs, validateRules(ep+".rules", e.Rules)...)
		}
		if t.Approval.AutoApprove != nil {
			errs = append(errs, validateCondition(p+".approval.auto_approve", *t.Approval.AutoApprove)...)
		}
		usesTarget := false
		for _, rule := range t.AllRules() {
			if len(rule.AnyOf.TargetRepoPermissions) > 0 {
				usesTarget = true
			}
		}
		if usesTarget && t.Approval.TargetRepoField == "" {
			errs = append(errs, fmt.Errorf("%s: target_repo_permissions requires approval.target_repo_field", p))
		}
	}
	return errors.Join(errs...)
}

func validateRules(p string, rules []Rule) []error {
	var errs []error
	for i, rule := range rules {
		if rule.Min < 1 {
			errs = append(errs, fmt.Errorf("%s[%d]: min must be >= 1", p, i))
		}
		if rule.AnyOf.IsEmpty() {
			errs = append(errs, fmt.Errorf("%s[%d]: any_of selects nobody", p, i))
		}
		for _, perm := range append(append([]string{}, rule.AnyOf.RepoPermissions...), rule.AnyOf.TargetRepoPermissions...) {
			if !validRepoPrm[perm] {
				errs = append(errs, fmt.Errorf("%s[%d]: unknown repository permission %q", p, i, perm))
			}
		}
	}
	return errs
}

func validateCondition(p string, c Condition) []error {
	var errs []error
	if len(c.All)+len(c.Any) == 0 {
		errs = append(errs, fmt.Errorf("%s: condition has no predicates", p))
	}
	for _, pr := range append(append([]Predicate{}, c.All...), c.Any...) {
		if !validOps[pr.Op] {
			errs = append(errs, fmt.Errorf("%s: unknown op %q", p, pr.Op))
		}
		if pr.Field == "" {
			errs = append(errs, fmt.Errorf("%s: predicate field is required", p))
		}
		if pr.Op == "matches" {
			if s, ok := pr.Value.(string); !ok {
				errs = append(errs, fmt.Errorf("%s: matches needs a string regex", p))
			} else if _, err := regexp.Compile(s); err != nil {
				errs = append(errs, fmt.Errorf("%s: bad regex: %v", p, err))
			}
		}
	}
	return errs
}

// AllRules returns base rules plus every escalation's rules (used for static checks).
func (t *RequestType) AllRules() []Rule {
	out := append([]Rule{}, t.Approval.Rules...)
	for _, e := range t.Approval.Escalations {
		out = append(out, e.Rules...)
	}
	return out
}

// DecodeSettings unmarshals the type-specific settings block into v.
func (t *RequestType) DecodeSettings(v any) error {
	if len(t.Settings) == 0 {
		return nil
	}
	if err := json.Unmarshal(t.Settings, v); err != nil {
		return fmt.Errorf("config: settings for %s: %w", t.ID, err)
	}
	return nil
}

// ByID returns a request type by id.
func (r *Registry) ByID(id string) (*RequestType, error) {
	for i := range r.RequestTypes {
		if r.RequestTypes[i].ID == id {
			return &r.RequestTypes[i], nil
		}
	}
	return nil, fmt.Errorf("config: unknown request type %q", id)
}

// ResolveLabels finds exactly one request type from an issue's labels. It
// fails closed on zero or multiple matches (a tampered or mislabelled issue).
func (r *Registry) ResolveLabels(labels []string) (*RequestType, error) {
	var hits []*RequestType
	has := map[string]bool{}
	for _, l := range labels {
		has[l] = true
	}
	for i := range r.RequestTypes {
		if has[r.RequestTypes[i].Label] {
			hits = append(hits, &r.RequestTypes[i])
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return nil, fmt.Errorf("config: no IssueOps type label among %v", labels)
	default:
		ids := make([]string, len(hits))
		for i, h := range hits {
			ids[i] = h.ID
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("config: multiple IssueOps type labels present: %s", strings.Join(ids, ", "))
	}
}

// ResolveEnvironment expands {fact} placeholders in an environment name.
func ResolveEnvironment(tmpl string, facts map[string]any) string {
	out := tmpl
	for {
		i := strings.Index(out, "{")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "}")
		if j < 0 {
			break
		}
		key := out[i+1 : i+j]
		val := fmt.Sprint(facts[strings.TrimPrefix(key, "$")])
		if v, ok := facts["$"+strings.TrimPrefix(key, "$")]; ok {
			val = fmt.Sprint(v)
		}
		if val == "<nil>" {
			val = ""
		}
		out = out[:i] + val + out[i+j+1:]
	}
	return out
}
