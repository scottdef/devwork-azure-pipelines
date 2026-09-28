// Package foundry implements the foundry-model-deployment type: allow-list
// validation against the registry, Bicep parameter generation, pre-flight
// checks over `az` JSON output (model availability, quota, existing
// deployments), and result reporting.
package foundry

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/request"
)

// Account is an approved Foundry (Azure AI Services) resource.
type Account struct {
	Environment   string `json:"environment"`
	ResourceGroup string `json:"resource_group"`
	Region        string `json:"region"`
}

// Model is an allow-listed model.
type Model struct {
	Name         string         `json:"name"`
	Format       string         `json:"format"`
	Versions     []string       `json:"versions"`
	SKUs         []string       `json:"skus"`
	Environments []string       `json:"environments"`
	MaxCapacity  map[string]int `json:"max_capacity"`
}

// Settings is the registry settings block.
type Settings struct {
	Accounts              map[string]Account `json:"accounts"`
	Models                []Model            `json:"models"`
	DefaultRAIPolicy      string             `json:"default_rai_policy"`
	DeploymentNamePattern string             `json:"deployment_name_pattern"`
	QuotaStrict           bool               `json:"quota_strict"`
}

// Plan is a validated deployment request.
type Plan struct {
	Account        string `json:"account"`
	Environment    string `json:"environment"`
	ResourceGroup  string `json:"resource_group"`
	Region         string `json:"region"`
	Model          string `json:"model"`
	Version        string `json:"version"`
	Format         string `json:"format"`
	SKU            string `json:"sku"`
	Capacity       int    `json:"capacity"`
	DeploymentName string `json:"deployment_name"`
	VersionUpgrade string `json:"version_upgrade"`
	RAIPolicy      string `json:"rai_policy"`
	UseCase        string `json:"use_case"`
	Consumers      string `json:"consumers"`
	CostCenter     string `json:"cost_center"`
	ReviewDate     string `json:"review_date"`
	Provisioned    bool   `json:"provisioned"`
	RequestRef     string `json:"request_ref"`
}

var sanitize = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func settings(c *request.Context) (Settings, error) {
	var s Settings
	err := c.Type.DecodeSettings(&s)
	if s.DeploymentNamePattern == "" {
		s.DeploymentNamePattern = `^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`
	}
	if s.DefaultRAIPolicy == "" {
		s.DefaultRAIPolicy = "Microsoft.DefaultV2"
	}
	return s, err
}

// BuildPlan validates the request against the registry allow-list.
func BuildPlan(c *request.Context) (*Plan, []string) {
	s, err := settings(c)
	if err != nil {
		return nil, []string{err.Error()}
	}
	v := c.Values
	var errs []string
	p := &Plan{
		Account:        v.String("foundry_account"),
		Model:          strings.TrimSpace(v.String("foundry_model")),
		Version:        strings.TrimSpace(v.String("foundry_model_version")),
		Format:         v.String("foundry_model_format"),
		SKU:            v.String("foundry_sku"),
		DeploymentName: strings.TrimSpace(v.String("foundry_deployment_name")),
		VersionUpgrade: v.String("foundry_version_upgrade"),
		RAIPolicy:      s.DefaultRAIPolicy,
		UseCase:        v.String("foundry_use_case"),
		Consumers:      v.String("foundry_consumers"),
		CostCenter:     v.String("foundry_cost_center"),
		ReviewDate:     v.String("foundry_review_date"),
		RequestRef:     fmt.Sprintf("%s/%s#%d", c.Registry.Organization, c.Registry.Repository, c.Issue),
	}
	acc, ok := s.Accounts[p.Account]
	if !ok {
		errs = append(errs, fmt.Sprintf("Foundry resource `%s` is not on the approved list.", p.Account))
	} else {
		p.Environment, p.ResourceGroup, p.Region = acc.Environment, acc.ResourceGroup, acc.Region
	}
	capacity, cerr := v.Int("foundry_capacity")
	if cerr != nil || capacity < 1 {
		errs = append(errs, "Capacity must be a positive whole number.")
	}
	p.Capacity = capacity
	p.Provisioned = strings.HasSuffix(p.SKU, "ProvisionedManaged")

	var model *Model
	for i := range s.Models {
		if strings.EqualFold(s.Models[i].Name, p.Model) {
			model = &s.Models[i]
		}
	}
	if model == nil {
		names := make([]string, 0, len(s.Models))
		for _, m := range s.Models {
			names = append(names, m.Name)
		}
		sort.Strings(names)
		errs = append(errs, fmt.Sprintf("Model `%s` is not on the allow-list. Approved models: %s.", p.Model, strings.Join(names, ", ")))
	} else {
		p.Model = model.Name
		if !strings.EqualFold(model.Format, p.Format) {
			errs = append(errs, fmt.Sprintf("Model `%s` is published with format `%s`, not `%s`.", model.Name, model.Format, p.Format))
		}
		if !contains(model.Versions, p.Version) {
			errs = append(errs, fmt.Sprintf("Version `%s` of `%s` is not approved (approved: %s).", p.Version, model.Name, strings.Join(model.Versions, ", ")))
		}
		if !contains(model.SKUs, p.SKU) {
			errs = append(errs, fmt.Sprintf("Deployment type `%s` is not approved for `%s` (approved: %s).", p.SKU, model.Name, strings.Join(model.SKUs, ", ")))
		}
		if p.Environment != "" && !contains(model.Environments, p.Environment) {
			errs = append(errs, fmt.Sprintf("`%s` is not approved for the %s environment.", model.Name, p.Environment))
		}
		if max, ok := model.MaxCapacity[p.Environment]; ok && p.Capacity > max {
			errs = append(errs, fmt.Sprintf("Capacity %d exceeds the self-service maximum of %d for `%s` in %s.", p.Capacity, max, model.Name, p.Environment))
		}
	}
	if p.DeploymentName == "" {
		p.DeploymentName = strings.Trim(sanitize.ReplaceAllString(p.Model+"-"+p.Environment, "-"), "-")
	}
	if re, err := regexp.Compile(s.DeploymentNamePattern); err == nil && !re.MatchString(p.DeploymentName) {
		errs = append(errs, fmt.Sprintf("Deployment name `%s` must match `%s`.", p.DeploymentName, s.DeploymentNamePattern))
	}
	switch p.VersionUpgrade {
	case "OnceNewDefaultVersionAvailable", "OnceCurrentVersionExpired", "NoAutoUpgrade":
	default:
		errs = append(errs, "Choose a valid version upgrade option.")
	}
	if t, err := time.Parse("2006-01-02", p.ReviewDate); err != nil {
		errs = append(errs, "Review date must use YYYY-MM-DD.")
	} else if !t.After(c.Now) {
		errs = append(errs, "Review date must be in the future.")
	} else if t.After(c.Now.AddDate(1, 0, 0)) {
		errs = append(errs, "Review date must be within 12 months.")
	}
	if strings.TrimSpace(p.UseCase) == "" {
		errs = append(errs, "Describe the intended agentic use.")
	}
	if p.CostCenter == "" {
		errs = append(errs, "A chargeback cost center is required.")
	}
	return p, errs
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// Handler implements request.Handler.
type Handler struct{}

// Facts implements request.Handler.
func (Handler) Facts(c *request.Context) (map[string]any, error) {
	p, _ := BuildPlan(c)
	f := map[string]any{"$type": c.Type.ID}
	if p != nil {
		f["$environment"] = p.Environment
		f["$provisioned"] = p.Provisioned
		f["$capacity"] = float64(p.Capacity)
	}
	return f, nil
}

// Validate implements request.Handler. Azure-side checks (model availability,
// quota) run in the execution job after OIDC login; see Preflight.
func (Handler) Validate(_ context.Context, c *request.Context) ([]string, []string) {
	p, errs := BuildPlan(c)
	var warns []string
	if p != nil && p.Provisioned {
		warns = append(warns, "Provisioned (PTU) deployments are billed hourly for reserved capacity whether or not they are used.")
	}
	return errs, warns
}

// Summary implements request.Handler.
func (Handler) Summary(c *request.Context) []request.Row {
	p, _ := BuildPlan(c)
	if p == nil {
		return nil
	}
	unit := "k TPM"
	if p.Provisioned {
		unit = " PTU"
	}
	return []request.Row{
		{Label: "Foundry resource", Value: fmt.Sprintf("`%s` (%s, %s, `%s`)", p.Account, p.Environment, p.Region, p.ResourceGroup)},
		{Label: "Model", Value: fmt.Sprintf("`%s` version `%s` (%s)", p.Model, p.Version, p.Format)},
		{Label: "Deployment", Value: fmt.Sprintf("`%s` · %s · %d%s", p.DeploymentName, p.SKU, p.Capacity, unit)},
		{Label: "Upgrade policy", Value: p.VersionUpgrade},
		{Label: "Content filter", Value: p.RAIPolicy, Code: true},
		{Label: "Review date", Value: p.ReviewDate},
		{Label: "Cost center", Value: p.CostCenter, Code: true},
	}
}

// Subject implements request.Handler.
func (Handler) Subject(*request.Context) (string, map[string]string) { return "", nil }

// BicepParameters renders an ARM deployment-parameters document for
// deploy/bicep/model-deployment.bicep (encoding/json; never text templating).
func (p *Plan) BicepParameters() ([]byte, error) {
	param := func(v any) map[string]any { return map[string]any{"value": v} }
	doc := map[string]any{
		"$schema":        "https://schema.management.azure.com/schemas/2019-04-01/deploymentParameters.json#",
		"contentVersion": "1.0.0.0",
		"parameters": map[string]any{
			"accountName":          param(p.Account),
			"deploymentName":       param(p.DeploymentName),
			"modelName":            param(p.Model),
			"modelVersion":         param(p.Version),
			"modelFormat":          param(p.Format),
			"skuName":              param(p.SKU),
			"capacity":             param(p.Capacity),
			"versionUpgradeOption": param(p.VersionUpgrade),
			"raiPolicyName":        param(p.RAIPolicy),
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// ---- pre-flight over `az` JSON output -----------------------------------

// AzModel is an entry of `az cognitiveservices account list-models -o json`.
type AzModel struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Version string `json:"version"`
	SKUs    []struct {
		Name string `json:"name"`
	} `json:"skus"`
	LifecycleStatus string `json:"lifecycleStatus"`
}

// AzUsage is an entry of `az cognitiveservices usage list -l <region> -o json`.
type AzUsage struct {
	Name struct {
		Value          string `json:"value"`
		LocalizedValue string `json:"localizedValue"`
	} `json:"name"`
	CurrentValue float64 `json:"currentValue"`
	Limit        float64 `json:"limit"`
}

// AzDeployment is an entry of `az cognitiveservices account deployment list -o json`.
type AzDeployment struct {
	Name string `json:"name"`
	SKU  struct {
		Name     string `json:"name"`
		Capacity int    `json:"capacity"`
	} `json:"sku"`
	Properties struct {
		Model struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Format  string `json:"format"`
		} `json:"model"`
		ProvisioningState string `json:"provisioningState"`
	} `json:"properties"`
}

// PreflightResult summarizes Azure-side checks.
type PreflightResult struct {
	OK             bool     `json:"ok"`
	Errors         []string `json:"errors"`
	Warnings       []string `json:"warnings"`
	Mode           string   `json:"mode"` // create | update
	QuotaName      string   `json:"quota_name,omitempty"`
	QuotaLimit     float64  `json:"quota_limit,omitempty"`
	QuotaUsed      float64  `json:"quota_used,omitempty"`
	QuotaAvailable float64  `json:"quota_available,omitempty"`
	ExistingCap    int      `json:"existing_capacity,omitempty"`
}

// quotaNames lists usage names to look for. Standard/Global/DataZone quota
// is per model ("OpenAI.GlobalStandard.gpt-4.1"); provisioned (PTU) quota is
// model-independent per deployment type, so "OpenAI.GlobalProvisionedManaged"
// is accepted as a fallback for PTU SKUs.
func quotaNames(p *Plan) []string {
	prefix := "OpenAI"
	if !strings.EqualFold(p.Format, "OpenAI") {
		prefix = "AIServices"
	}
	names := []string{fmt.Sprintf("%s.%s.%s", prefix, p.SKU, p.Model)}
	if p.Provisioned {
		names = append(names, fmt.Sprintf("%s.%s", prefix, p.SKU))
	}
	return names
}

// Preflight evaluates `az` outputs against the plan.
func Preflight(p *Plan, models []AzModel, usages []AzUsage, deployments []AzDeployment, strictQuota bool) *PreflightResult {
	r := &PreflightResult{Mode: "create"}
	available := false
	for _, m := range models {
		if strings.EqualFold(m.Name, p.Model) && m.Version == p.Version && strings.EqualFold(m.Format, p.Format) {
			for _, s := range m.SKUs {
				if strings.EqualFold(s.Name, p.SKU) {
					available = true
				}
			}
			if strings.EqualFold(m.LifecycleStatus, "Deprecating") || strings.EqualFold(m.LifecycleStatus, "Deprecated") {
				r.Warnings = append(r.Warnings, fmt.Sprintf("Model %s %s lifecycle status is %s.", m.Name, m.Version, m.LifecycleStatus))
			}
		}
	}
	if !available {
		r.Errors = append(r.Errors, fmt.Sprintf("`%s` version `%s` with SKU `%s` is not offered on `%s` in %s (az cognitiveservices account list-models).", p.Model, p.Version, p.SKU, p.Account, p.Region))
	}
	for _, d := range deployments {
		if d.Name == p.DeploymentName {
			r.Mode, r.ExistingCap = "update", d.SKU.Capacity
			if !strings.EqualFold(d.Properties.Model.Name, p.Model) {
				r.Errors = append(r.Errors, fmt.Sprintf("Deployment `%s` already exists for a different model (`%s`); choose another name.", d.Name, d.Properties.Model.Name))
			}
			if !strings.EqualFold(d.SKU.Name, p.SKU) {
				r.Errors = append(r.Errors, fmt.Sprintf("Deployment `%s` exists with SKU `%s`; changing deployment type in place is not supported.", d.Name, d.SKU.Name))
			}
		}
	}
	names := quotaNames(p)
	r.QuotaName = names[0]
	found := false
	for _, name := range names {
		for _, u := range usages {
			if found || !strings.EqualFold(u.Name.Value, name) {
				continue
			}
			found = true
			r.QuotaName = u.Name.Value
			r.QuotaLimit, r.QuotaUsed = u.Limit, u.CurrentValue
			r.QuotaAvailable = u.Limit - u.CurrentValue + float64(r.ExistingCap)
			if float64(p.Capacity) > r.QuotaAvailable {
				r.Errors = append(r.Errors, fmt.Sprintf("Insufficient quota for `%s` in %s: requested %d, available %s (limit %s, used %s). Request a quota increase or lower the capacity.",
					r.QuotaName, p.Region, p.Capacity, trim(r.QuotaAvailable), trim(u.Limit), trim(u.CurrentValue)))
			}
		}
	}
	if !found {
		msg := fmt.Sprintf("No quota entry named `%s` was found in `az cognitiveservices usage list`; quota could not be verified.", strings.Join(names, "` or `"))
		if strictQuota && !p.Provisioned && strings.EqualFold(p.Format, "OpenAI") {
			r.Errors = append(r.Errors, msg)
		} else {
			r.Warnings = append(r.Warnings, msg)
		}
	}
	r.OK = len(r.Errors) == 0
	return r
}

func trim(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
