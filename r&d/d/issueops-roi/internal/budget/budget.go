// Package budget implements the copilot-budget-request type: validation,
// policy facts, and an idempotent create-or-update ("upsert") of a GitHub
// billing budget through the budgets REST API.
package budget

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/request"
	"github.com/CoolEngOrg/issueops/internal/tmpl"
)

// Product maps a form option to an API SKU and budget type.
type Product struct {
	SKU        string `json:"sku"`
	BudgetType string `json:"budget_type"`
}

// Settings is the registry settings block.
type Settings struct {
	MaxAmountUSD            int                `json:"max_amount_usd"`
	EnterpriseScopesEnabled bool               `json:"enterprise_scopes_enabled"`
	Products                map[string]Product `json:"products"`
	DefaultAlertRecipients  []string           `json:"default_alert_recipients"`
}

// Form scopes -> API scopes.
var apiScope = map[string]string{
	"organization": "organization",
	"repository":   "repository",
	"user":         "user",
	"all-users":    "multi_user_customer",
	"cost-center":  "cost_center",
}

const enforcementStop = "Stop usage when the budget is reached"

var (
	loginRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9]|-(?:[A-Za-z0-9])){0,38}$`)
	repoRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	ccRe    = regexp.MustCompile(`^[A-Za-z0-9 ._-]{1,100}$`)
)

// Handler implements request.Handler.
type Handler struct{}

func settings(c *request.Context) (Settings, error) {
	var s Settings
	err := c.Type.DecodeSettings(&s)
	if s.MaxAmountUSD == 0 {
		s.MaxAmountUSD = 10000
	}
	return s, err
}

// Plan is the resolved budget operation.
type Plan struct {
	Owner         ghapi.BudgetOwner    `json:"-"`
	OwnerKind     string               `json:"owner_kind"` // organization | enterprise
	OwnerName     string               `json:"owner_name"`
	FormScope     string               `json:"form_scope"`
	Scope         string               `json:"scope"`
	EntityName    string               `json:"entity_name"`
	User          string               `json:"user,omitempty"`
	SKU           string               `json:"sku"`
	BudgetType    string               `json:"budget_type"`
	Amount        int                  `json:"amount"`
	Prevent       bool                 `json:"prevent_further_usage"`
	Alerting      ghapi.BudgetAlerting `json:"alerting"`
	ExpiresAt     string               `json:"expires_at,omitempty"`
	CostCenter    string               `json:"cost_center"`
	Justification string               `json:"justification"`
}

// BuildPlan converts parsed values into a budget plan (no network access).
func BuildPlan(c *request.Context) (*Plan, []string) {
	s, err := settings(c)
	if err != nil {
		return nil, []string{err.Error()}
	}
	v := c.Values
	var errs []string
	p := &Plan{
		FormScope:     v.String("budget_scope"),
		CostCenter:    v.String("budget_cost_center"),
		Justification: v.String("budget_justification"),
	}
	p.Scope = apiScope[p.FormScope]
	if p.Scope == "" {
		errs = append(errs, fmt.Sprintf("Unknown budget scope %q.", p.FormScope))
	}
	target := strings.TrimPrefix(v.String("budget_target"), "@")
	prod, ok := s.Products[v.String("budget_product")]
	if !ok {
		errs = append(errs, fmt.Sprintf("Unknown metered product %q.", v.String("budget_product")))
	}
	p.SKU, p.BudgetType = prod.SKU, prod.BudgetType

	amount, aerr := v.Int("budget_amount")
	switch {
	case aerr != nil:
		errs = append(errs, "Requested monthly budget must be a whole number of US dollars (for example `500`).")
	case amount < 1:
		errs = append(errs, "Requested monthly budget must be at least $1.")
	case amount > s.MaxAmountUSD:
		errs = append(errs, fmt.Sprintf("Requested monthly budget $%d exceeds the self-service maximum of $%d; contact FinOps.", amount, s.MaxAmountUSD))
	}
	p.Amount = amount
	p.Prevent = v.String("budget_enforcement") == enforcementStop

	p.OwnerKind, p.OwnerName = "organization", c.Registry.Organization
	switch p.FormScope {
	case "organization":
		// The API documents an empty entity name for organization budgets and
		// echoes the organization name back.
		p.EntityName = ""
		if target != "" && !strings.EqualFold(target, c.Registry.Organization) {
			errs = append(errs, "Leave the scope target blank for organization budgets.")
		}
	case "repository":
		repo := target
		if o, r, ok := strings.Cut(target, "/"); ok {
			if !strings.EqualFold(o, c.Registry.Organization) {
				errs = append(errs, fmt.Sprintf("Repository budgets are limited to %s repositories.", c.Registry.Organization))
			}
			repo = r
		}
		if !repoRe.MatchString(repo) {
			errs = append(errs, "Repository scope needs a repository name in the scope target.")
		}
		p.EntityName = c.Registry.Organization + "/" + repo
	case "user":
		if !loginRe.MatchString(target) {
			errs = append(errs, "User scope needs a valid GitHub username in the scope target.")
		}
		p.User, p.EntityName = target, ""
		if !p.Prevent {
			errs = append(errs, "User budgets must stop usage when the budget is reached (GitHub requirement).")
		}
	case "all-users":
		p.EntityName = ""
		if !p.Prevent {
			errs = append(errs, "All-users budgets must stop usage when the budget is reached (GitHub requirement).")
		}
	case "cost-center":
		if !s.EnterpriseScopesEnabled {
			errs = append(errs, "Enterprise cost-center budgets are disabled on this platform. Ask platform-admins to enable `enterprise_scopes_enabled`.")
		}
		if !ccRe.MatchString(target) {
			errs = append(errs, "Cost-center scope needs the cost center name in the scope target.")
		}
		p.OwnerKind, p.OwnerName, p.EntityName = "enterprise", c.Registry.Enterprise, target
	}
	if (p.Scope == "user" || p.Scope == "multi_user_customer") && p.SKU != "" && p.SKU != "ai_credits" && p.SKU != "premium_requests" {
		errs = append(errs, "User and all-users budgets are only supported for AI credits or premium requests.")
	}
	p.Owner = ghapi.BudgetOwner{Enterprise: p.OwnerKind == "enterprise", Name: p.OwnerName}

	recips := issueform.SplitCSV(v.String("budget_alert_recipients"))
	switch p.Scope {
	case "user", "multi_user_customer":
		if len(recips) > 0 {
			errs = append(errs, "Alert recipients are not supported for user or all-users budgets (GitHub disables alerts for them).")
		}
		p.Alerting = ghapi.BudgetAlerting{WillAlert: false, AlertRecipients: []string{}}
	default:
		if len(recips) == 0 {
			recips = s.DefaultAlertRecipients
		}
		clean := []string{}
		for _, r := range recips {
			r = strings.TrimPrefix(r, "@")
			if !loginRe.MatchString(r) {
				errs = append(errs, fmt.Sprintf("Alert recipient %q is not a valid GitHub username.", r))
				continue
			}
			clean = append(clean, r)
		}
		p.Alerting = ghapi.BudgetAlerting{WillAlert: len(clean) > 0, AlertRecipients: clean}
	}

	if exp := v.String("budget_expires_on"); exp != "" {
		t, err := time.Parse("2006-01-02", exp)
		switch {
		case err != nil:
			errs = append(errs, "Budget expiry date must use YYYY-MM-DD.")
		case p.Scope != "user":
			errs = append(errs, "Budget expiry is only supported for user-scope budgets.")
		case !t.After(c.Now):
			errs = append(errs, "Budget expiry date must be in the future.")
		default:
			p.ExpiresAt = exp
		}
	}
	if p.CostCenter == "" {
		errs = append(errs, "A chargeback cost center is required.")
	}
	return p, errs
}

// Facts implements request.Handler.
func (Handler) Facts(c *request.Context) (map[string]any, error) {
	p, _ := BuildPlan(c)
	f := map[string]any{"$type": c.Type.ID}
	if p != nil {
		f["$amount_usd"] = float64(p.Amount)
		f["$api_scope"] = p.Scope
		f["$owner_kind"] = p.OwnerKind
	}
	return f, nil
}

// Validate implements request.Handler.
func (Handler) Validate(ctx context.Context, c *request.Context) ([]string, []string) {
	p, errs := BuildPlan(c)
	var warns []string
	if p == nil || c.GH == nil {
		return errs, warns
	}
	switch p.FormScope {
	case "repository":
		_, repo, _ := strings.Cut(p.EntityName, "/")
		r, err := c.GH.GetRepo(ctx, c.Registry.Organization, repo)
		if err != nil {
			warns = append(warns, "Could not verify the repository: "+err.Error())
		} else if r == nil {
			errs = append(errs, fmt.Sprintf("Repository `%s` does not exist.", p.EntityName))
		}
	case "user":
		role, err := c.GH.OrgMembership(ctx, c.Registry.Organization, p.User)
		if err != nil {
			warns = append(warns, "Could not verify organization membership: "+err.Error())
		} else if role == "" {
			errs = append(errs, fmt.Sprintf("@%s is not a member of %s.", p.User, c.Registry.Organization))
		}
	}
	if len(errs) == 0 && p.OwnerKind == "organization" {
		if cur, err := FindExisting(ctx, c.GH, p); err == nil {
			if cur != nil {
				c.SetInfo("current_amount", cur.BudgetAmount)
				c.SetInfo("current_id", cur.ID)
			}
		} else {
			warns = append(warns, "Could not read existing budgets (the GitHub App needs organization Administration: read): "+err.Error())
		}
	}
	return errs, warns
}

// Summary implements request.Handler.
func (Handler) Summary(c *request.Context) []request.Row {
	p, _ := BuildPlan(c)
	if p == nil {
		return nil
	}
	target := p.EntityName
	if p.Scope == "organization" {
		target = p.OwnerName
	}
	if p.Scope == "user" {
		target = "@" + p.User
	}
	if p.Scope == "multi_user_customer" {
		target = "every Copilot user in " + c.Registry.Organization
	}
	enf := "alert only"
	if p.Prevent {
		enf = "stop usage at the limit"
	}
	rows := []request.Row{
		{Label: "Endpoint", Value: fmt.Sprintf("%s `%s`", p.OwnerKind, p.OwnerName)},
		{Label: "Budget scope", Value: fmt.Sprintf("`%s` (%s)", p.Scope, target)},
		{Label: "Product SKU", Value: p.SKU, Code: true},
		{Label: "Requested amount", Value: tmpl.USD(p.Amount) + " per month"},
		{Label: "Enforcement", Value: enf},
	}
	if cur, ok := c.Info["current_amount"].(float64); ok {
		rows = append(rows, request.Row{Label: "Current budget", Value: fmt.Sprintf("%s (change %s)", tmpl.USD(cur), signedUSD(float64(p.Amount)-cur))})
	}
	if len(p.Alerting.AlertRecipients) > 0 {
		rows = append(rows, request.Row{Label: "Alert recipients", Value: "@" + strings.Join(p.Alerting.AlertRecipients, ", @")})
	}
	if p.ExpiresAt != "" {
		rows = append(rows, request.Row{Label: "Expires", Value: p.ExpiresAt})
	}
	rows = append(rows, request.Row{Label: "Cost center", Value: p.CostCenter, Code: true})
	return rows
}

func signedUSD(v float64) string {
	if v >= 0 {
		return "+" + tmpl.USD(v)
	}
	return tmpl.USD(v)
}

// Subject implements request.Handler.
func (Handler) Subject(c *request.Context) (string, map[string]string) {
	fu := map[string]string{}
	if c.Values.String("budget_scope") == "user" {
		fu["budget_target"] = strings.TrimPrefix(c.Values.String("budget_target"), "@")
	}
	return "", fu
}

func entityMatches(p *Plan, b ghapi.Budget) bool {
	switch p.Scope {
	case "user":
		return strings.EqualFold(b.User, p.User) || strings.EqualFold(b.BudgetEntityName, p.User)
	case "multi_user_customer":
		return true
	case "organization":
		return b.BudgetEntityName == "" || strings.EqualFold(b.BudgetEntityName, p.OwnerName)
	case "repository":
		_, bare, _ := strings.Cut(p.EntityName, "/")
		return strings.EqualFold(b.BudgetEntityName, p.EntityName) || strings.EqualFold(b.BudgetEntityName, bare)
	default:
		return strings.EqualFold(b.BudgetEntityName, p.EntityName)
	}
}

// FindExisting returns the budget this plan would update, if any.
func FindExisting(ctx context.Context, gh *ghapi.Client, p *Plan) (*ghapi.Budget, error) {
	budgets, err := gh.ListBudgets(ctx, p.Owner, p.Scope, p.User)
	if err != nil {
		return nil, err
	}
	for i := range budgets {
		b := budgets[i]
		if !strings.EqualFold(b.BudgetScope, p.Scope) {
			continue
		}
		skuOK := false
		for _, s := range b.SKUs() {
			if strings.EqualFold(s, p.SKU) {
				skuOK = true
			}
		}
		if skuOK && entityMatches(p, b) {
			return &b, nil
		}
	}
	return nil, nil
}

// Result describes an applied change.
type Result struct {
	Action string              `json:"action"` // created | updated | unchanged | dry-run
	Before *ghapi.Budget       `json:"before,omitempty"`
	After  *ghapi.Budget       `json:"after,omitempty"`
	Create *ghapi.BudgetCreate `json:"create_payload,omitempty"`
	Update *ghapi.BudgetUpdate `json:"update_payload,omitempty"`
}

// CreatePayload builds the POST body.
func (p *Plan) CreatePayload() ghapi.BudgetCreate {
	return ghapi.BudgetCreate{
		BudgetAmount:        p.Amount,
		PreventFurtherUsage: p.Prevent,
		BudgetAlerting:      p.Alerting,
		BudgetScope:         p.Scope,
		BudgetEntityName:    p.EntityName,
		BudgetType:          p.BudgetType,
		BudgetProductSKU:    p.SKU,
		User:                p.User,
		ExpiresAt:           p.ExpiresAt,
	}
}

// UpdatePayload builds the PATCH body.
func (p *Plan) UpdatePayload() ghapi.BudgetUpdate {
	u := ghapi.BudgetUpdate{BudgetAmount: p.Amount, PreventFurtherUsage: p.Prevent, ExpiresAt: p.ExpiresAt}
	if p.Scope != "user" && p.Scope != "multi_user_customer" {
		a := p.Alerting
		u.BudgetAlerting = &a
	}
	return u
}

// Unchanged reports whether an existing budget already matches the plan
// (amount, enforcement, and alerting where the scope supports alerts).
func Unchanged(cur *ghapi.Budget, p *Plan) bool {
	if int(cur.BudgetAmount) != p.Amount || cur.PreventFurtherUsage != p.Prevent || p.ExpiresAt != "" {
		return false
	}
	if p.Scope == "user" || p.Scope == "multi_user_customer" {
		return true // alerts are not configurable for these scopes
	}
	if cur.BudgetAlerting.WillAlert != p.Alerting.WillAlert {
		return false
	}
	a := append([]string{}, cur.BudgetAlerting.AlertRecipients...)
	b := append([]string{}, p.Alerting.AlertRecipients...)
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Apply upserts the budget. With dryRun it only reports what would change.
func Apply(ctx context.Context, gh *ghapi.Client, p *Plan, dryRun bool) (*Result, error) {
	cur, err := FindExisting(ctx, gh, p)
	if err != nil {
		return nil, fmt.Errorf("budget: list existing budgets: %w", err)
	}
	if cur != nil {
		if Unchanged(cur, p) {
			return &Result{Action: "unchanged", Before: cur, After: cur}, nil
		}
		up := p.UpdatePayload()
		if dryRun {
			return &Result{Action: "dry-run", Before: cur, Update: &up}, nil
		}
		after, err := gh.UpdateBudget(ctx, p.Owner, cur.ID, up)
		if err != nil {
			return nil, fmt.Errorf("budget: update %s: %w", cur.ID, err)
		}
		return &Result{Action: "updated", Before: cur, After: after, Update: &up}, nil
	}
	cr := p.CreatePayload()
	if dryRun {
		return &Result{Action: "dry-run", Create: &cr}, nil
	}
	after, err := gh.CreateBudget(ctx, p.Owner, cr)
	if err != nil {
		return nil, fmt.Errorf("budget: create: %w", err)
	}
	return &Result{Action: "created", After: after, Create: &cr}, nil
}
