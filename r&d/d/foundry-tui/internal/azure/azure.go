// Package azure reads Microsoft Foundry through the az command.
//
// It only reads. There is no create, update or delete here, by
// construction: changes travel through GitHub workflows (see package
// dispatch) so that every one of them has an actor, a run and a log.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

// API versions for the two ARM reads that have no az subcommand.
const (
	healthAPI = "2022-10-01"
	alertsAPI = "2023-07-12-preview"
)

// Client runs az with the configured account, group and subscription.
type Client struct {
	R   run.Runner
	Cfg config.Config
}

func (c Client) az(ctx context.Context, v any, args ...string) error {
	if c.Cfg.Subscription != "" {
		args = append(args, "--subscription", c.Cfg.Subscription)
	}
	args = append(args, "-o", "json", "--only-show-errors")
	out, err := c.R.Run(ctx, run.Cmd{Name: "az", Args: args})
	if err != nil {
		return err
	}
	if v == nil || len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("az %s: %w", strings.Join(args[:min(3, len(args))], " "), err)
	}
	return nil
}

func (c Client) scope() []string {
	return []string{"-g", c.Cfg.ResourceGroup, "-n", c.Cfg.Account}
}

// Identity is the signed-in principal.
type Identity struct {
	Subscription string `json:"id"`
	Name         string `json:"name"`
	TenantID     string `json:"tenantId"`
	User         struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"user"`
}

func (c Client) Whoami(ctx context.Context) (Identity, error) {
	var id Identity
	return id, c.az(ctx, &id, "account", "show")
}

// Account is the Foundry resource (Microsoft.CognitiveServices/accounts).
type Account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Location string `json:"location"`
	SKU      struct {
		Name string `json:"name"`
	} `json:"sku"`
	Properties struct {
		Endpoint            string            `json:"endpoint"`
		Endpoints           map[string]string `json:"endpoints"`
		CustomSubDomainName string            `json:"customSubDomainName"`
		ProvisioningState   string            `json:"provisioningState"`
		PublicNetworkAccess string            `json:"publicNetworkAccess"`
		DisableLocalAuth    bool              `json:"disableLocalAuth"`
	} `json:"properties"`
}

// OpenAIBase is the host that serves /openai/v1 for every deployment.
func (a Account) OpenAIBase() string { return "https://" + a.subdomain() + ".openai.azure.com" }

// AnthropicBase is the host prefix that serves the Anthropic Messages API.
func (a Account) AnthropicBase() string {
	return "https://" + a.subdomain() + ".services.ai.azure.com/anthropic"
}

func (a Account) subdomain() string {
	if s := a.Properties.CustomSubDomainName; s != "" {
		return s
	}
	return a.Name
}

func (c Client) Account(ctx context.Context) (Account, error) {
	var a Account
	err := c.az(ctx, &a, append([]string{"cognitiveservices", "account", "show"}, c.scope()...)...)
	return a, err
}

// Deployment is one model deployment. SystemData is the audit trail
// Azure keeps for free: who created it, who touched it last, and when.
type Deployment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SKU  struct {
		Name     string `json:"name"`
		Capacity int    `json:"capacity"`
	} `json:"sku"`
	Properties struct {
		Model struct {
			Format  string `json:"format"`
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"model"`
		ProvisioningState    string            `json:"provisioningState"`
		VersionUpgradeOption string            `json:"versionUpgradeOption"`
		RAIPolicyName        string            `json:"raiPolicyName"`
		Capabilities         map[string]string `json:"capabilities"`
		RateLimits           []struct {
			Key           string  `json:"key"`
			Count         float64 `json:"count"`
			RenewalPeriod float64 `json:"renewalPeriod"`
		} `json:"rateLimits"`
	} `json:"properties"`
	SystemData SystemData `json:"systemData"`
}

// SystemData is ARM's record of authorship.
type SystemData struct {
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	LastModifiedBy string    `json:"lastModifiedBy"`
	LastModifiedAt time.Time `json:"lastModifiedAt"`
}

// Chat reports whether the deployment can answer a chat probe.
// Deployments that declare no capabilities are given the benefit of the doubt.
func (d Deployment) Chat() bool {
	caps := d.Properties.Capabilities
	if len(caps) == 0 {
		return true
	}
	return caps["chatCompletion"] == "true" || caps["responses"] == "true"
}

// RateLimit returns the limit for key ("request" or "token"), or 0.
func (d Deployment) RateLimit(key string) float64 {
	for _, r := range d.Properties.RateLimits {
		if r.Key == key {
			return r.Count
		}
	}
	return 0
}

func (c Client) Deployments(ctx context.Context) ([]Deployment, error) {
	var ds []Deployment
	err := c.az(ctx, &ds, append([]string{"cognitiveservices", "account", "deployment", "list"}, c.scope()...)...)
	sort.Slice(ds, func(i, j int) bool { return ds[i].Name < ds[j].Name })
	return ds, err
}

// Model is one entry in the account's deployable catalog.
type Model struct {
	Name             string `json:"name"`
	Format           string `json:"format"`
	Version          string `json:"version"`
	LifecycleStatus  string `json:"lifecycleStatus"`
	IsDefaultVersion bool   `json:"isDefaultVersion"`
	SKUs             []struct {
		Name     string `json:"name"`
		Capacity struct {
			Default int `json:"default"`
			Maximum int `json:"maximum"`
		} `json:"capacity"`
	} `json:"skus"`
	Deprecation struct {
		Inference string `json:"inference"`
	} `json:"deprecation"`
}

// SKUNames lists the deployment types the model supports.
func (m Model) SKUNames() []string {
	var s []string
	for _, k := range m.SKUs {
		s = append(s, k.Name)
	}
	return s
}

// Retires parses the inference retirement date. ok is false when unset.
func (m Model) Retires() (t time.Time, ok bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, m.Deprecation.Inference); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (c Client) Models(ctx context.Context) ([]Model, error) {
	var ms []Model
	err := c.az(ctx, &ms, append([]string{"cognitiveservices", "account", "list-models"}, c.scope()...)...)
	sort.Slice(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if a.Format != b.Format {
			return a.Format < b.Format
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version > b.Version
	})
	return ms, err
}

// Usage is one quota line for the region.
type Usage struct {
	Name struct {
		Value string `json:"value"`
	} `json:"name"`
	CurrentValue float64 `json:"currentValue"`
	Limit        float64 `json:"limit"`
	Unit         string  `json:"unit"`
}

// Percent is current/limit, or 0 when there is no limit.
func (u Usage) Percent() float64 {
	if u.Limit <= 0 {
		return 0
	}
	return 100 * u.CurrentValue / u.Limit
}

// Usage returns regional quota, busiest first, idle lines dropped.
func (c Client) Usage(ctx context.Context) ([]Usage, error) {
	var all []Usage
	if err := c.az(ctx, &all, "cognitiveservices", "usage", "list", "-l", c.Cfg.Location); err != nil {
		return nil, err
	}
	us := all[:0]
	for _, u := range all {
		if u.CurrentValue > 0 {
			us = append(us, u)
		}
	}
	sort.Slice(us, func(i, j int) bool { return us[i].Percent() > us[j].Percent() })
	return us, nil
}

// RoleAssignment is who can do what at the account scope.
type RoleAssignment struct {
	PrincipalName string `json:"principalName"`
	PrincipalID   string `json:"principalId"`
	PrincipalType string `json:"principalType"`
	Role          string `json:"roleDefinitionName"`
	Scope         string `json:"scope"`
	CreatedOn     string `json:"createdOn"`
}

// Inherited reports whether the assignment was made above the account.
func (r RoleAssignment) Inherited(accountID string) bool {
	return !strings.EqualFold(r.Scope, accountID)
}

func (c Client) RoleAssignments(ctx context.Context, accountID string) ([]RoleAssignment, error) {
	var ra []RoleAssignment
	err := c.az(ctx, &ra, "role", "assignment", "list", "--scope", accountID, "--include-inherited")
	sort.Slice(ra, func(i, j int) bool {
		if ra[i].Role != ra[j].Role {
			return ra[i].Role < ra[j].Role
		}
		return ra[i].PrincipalName < ra[j].PrincipalName
	})
	return ra, err
}

// RoleDefinition is the shape az returns for a built-in or custom role.
type RoleDefinition struct {
	RoleName    string `json:"roleName"`
	Name        string `json:"name"`
	UpdatedOn   string `json:"updatedOn"`
	Permissions []struct {
		Actions        []string `json:"actions"`
		NotActions     []string `json:"notActions"`
		DataActions    []string `json:"dataActions"`
		NotDataActions []string `json:"notDataActions"`
	} `json:"permissions"`
}

// RoleDefinition looks a role up by name or GUID.
func (c Client) RoleDefinition(ctx context.Context, nameOrID string) (RoleDefinition, error) {
	var defs []RoleDefinition
	if err := c.az(ctx, &defs, "role", "definition", "list", "--name", nameOrID); err != nil {
		return RoleDefinition{}, err
	}
	if len(defs) == 0 {
		return RoleDefinition{}, fmt.Errorf("role %q not found", nameOrID)
	}
	return defs[0], nil
}

// Health is Azure Resource Health's opinion of the account.
type Health struct {
	Properties struct {
		AvailabilityState string `json:"availabilityState"`
		Summary           string `json:"summary"`
		ReasonType        string `json:"reasonType"`
		ReportedTime      string `json:"reportedTime"`
	} `json:"properties"`
}

func (c Client) Health(ctx context.Context, accountID string) (Health, error) {
	var h Health
	url := accountID + "/providers/Microsoft.ResourceHealth/availabilityStatuses/current?api-version=" + healthAPI
	return h, c.az(ctx, &h, "rest", "--method", "get", "--url", url)
}

// Alert is one fired Azure Monitor alert targeting the account.
type Alert struct {
	Name       string `json:"name"`
	Properties struct {
		Essentials struct {
			Severity         string `json:"severity"`
			AlertState       string `json:"alertState"`
			MonitorCondition string `json:"monitorCondition"`
			AlertRule        string `json:"alertRule"`
			StartDateTime    string `json:"startDateTime"`
		} `json:"essentials"`
	} `json:"properties"`
}

// Alerts lists alerts fired in the last 30 days at the account scope.
func (c Client) Alerts(ctx context.Context, accountID string) ([]Alert, error) {
	var page struct {
		Value []Alert `json:"value"`
	}
	url := accountID + "/providers/Microsoft.AlertsManagement/alerts?api-version=" + alertsAPI + "&timeRange=30d"
	return page.Value, c.az(ctx, &page, "rest", "--method", "get", "--url", url)
}

// Metric is a 24 hour total for one metric and one deployment.
type Metric struct {
	Name       string
	Deployment string
	Unit       string
	Total      float64
}

// Metrics needs Microsoft.Insights/metrics/read. Foundry Owner does not
// carry it; callers ask package perms before calling.
func (c Client) Metrics(ctx context.Context, accountID string, names []string) ([]Metric, error) {
	var resp struct {
		Value []struct {
			Name struct {
				Value string `json:"value"`
			} `json:"name"`
			Unit       string `json:"unit"`
			Timeseries []struct {
				Metadata []struct {
					Name struct {
						Value string `json:"value"`
					} `json:"name"`
					Value string `json:"value"`
				} `json:"metadatavalues"`
				Data []struct {
					Total float64 `json:"total"`
				} `json:"data"`
			} `json:"timeseries"`
		} `json:"value"`
	}
	args := []string{"monitor", "metrics", "list", "--resource", accountID, "--metrics"}
	args = append(args, names...)
	args = append(args, "--aggregation", "Total", "--interval", "PT1H", "--offset", "24h",
		"--filter", "ModelDeploymentName eq '*'")
	if err := c.az(ctx, &resp, args...); err != nil {
		return nil, err
	}
	var ms []Metric
	for _, v := range resp.Value {
		for _, ts := range v.Timeseries {
			m := Metric{Name: v.Name.Value, Unit: v.Unit}
			for _, md := range ts.Metadata {
				if strings.EqualFold(md.Name.Value, "modeldeploymentname") {
					m.Deployment = md.Value
				}
			}
			for _, d := range ts.Data {
				m.Total += d.Total
			}
			ms = append(ms, m)
		}
	}
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].Deployment != ms[j].Deployment {
			return ms[i].Deployment < ms[j].Deployment
		}
		return ms[i].Name < ms[j].Name
	})
	return ms, nil
}

// Token returns a short-lived data-plane bearer token. Keyless: the
// account keys are never read, so they can be disabled.
func (c Client) Token(ctx context.Context) (string, error) {
	var t struct {
		AccessToken string `json:"accessToken"`
	}
	err := c.az(ctx, &t, "account", "get-access-token", "--resource", c.Cfg.TokenResource)
	if err == nil && t.AccessToken == "" {
		err = fmt.Errorf("az returned an empty access token")
	}
	return t.AccessToken, err
}
