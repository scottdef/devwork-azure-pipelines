// Package inventory loads a JSON description of many Foundry deployments
// across environments and resolves it into a deterministic Bundle.
//
// JSON (not YAML) is deliberate: the project is restricted to the Go standard
// library, and encoding/json with DisallowUnknownFields gives strict,
// typo-catching parsing for free.
package inventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
)

// SchemaVersion is the only inventory version this build understands.
const SchemaVersion = 1

// File is the on-disk inventory document.
type File struct {
	Version int `json:"version"`
	// Defaults apply to every deployment unless overridden.
	Defaults foundry.Spec `json:"defaults"`
	// Resources lists Azure resources, each with its deployments.
	Resources []Resource `json:"resources"`
}

// Resource is one Azure AI Foundry / Azure OpenAI resource.
type Resource struct {
	// Name is the Azure resource (custom subdomain) name.
	Name string `json:"name"`
	// Settings override File.Defaults for every deployment on this resource.
	// Its resource/model/deployment fields are ignored.
	Settings foundry.Spec `json:"settings"`
	// Deployments lists the deployed models. Any Spec field may be set here
	// to override the resource-level settings for one deployment.
	Deployments []foundry.Spec `json:"deployments"`
}

// EnterpriseGroup is one "API key" entry in the GitHub Copilot custom-models
// form. GitHub requires one Deployment URL per entry; every model that shares
// the URL is added as a Model ID under it.
type EnterpriseGroup struct {
	KeyName       string   `json:"key_name"`
	Provider      string   `json:"provider"`
	DeploymentURL string   `json:"deployment_url"`
	Alternates    []string `json:"deployment_url_alternates,omitempty"`
	APIKeySecret  string   `json:"api_key_secret"`
	Environment   string   `json:"environment,omitempty"`
	Resource      string   `json:"resource,omitempty"`
	ModelIDs      []string `json:"model_ids"`
	HostAccepted  bool     `json:"host_accepted"`
}

// Skipped records a deployment that cannot be registered in the enterprise
// form, with the reason, so the runbook can say so explicitly.
type Skipped struct {
	Environment string `json:"environment,omitempty"`
	Resource    string `json:"resource,omitempty"`
	Deployment  string `json:"deployment"`
	Reason      string `json:"reason"`
}

// Bundle is the fully resolved inventory handed to the renderer.
type Bundle struct {
	ToolVersion       string             `json:"tool_version"`
	Items             []foundry.Resolved `json:"items"`
	EnterpriseGroups  []EnterpriseGroup  `json:"copilot_enterprise_groups"`
	EnterpriseSkipped []Skipped          `json:"copilot_enterprise_skipped,omitempty"`
}

// LoadFile reads and strictly parses an inventory. path "-" reads stdin.
func LoadFile(path string) (File, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return File{}, fmt.Errorf("read inventory: %w", err)
	}
	return Parse(data)
}

// Parse strictly decodes an inventory document.
func Parse(data []byte) (File, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("parse inventory: %w", err)
	}
	// Reject trailing content ("{...}{...}" or stray text after the document).
	if _, err := dec.Token(); err != io.EOF {
		return File{}, fmt.Errorf("parse inventory: unexpected content after the JSON document")
	}
	if f.Version != SchemaVersion {
		return File{}, fmt.Errorf("inventory version %d is not supported (want %d)", f.Version, SchemaVersion)
	}
	if len(f.Resources) == 0 {
		return File{}, fmt.Errorf("inventory has no resources")
	}
	return f, nil
}

// Specs flattens the inventory into one merged Spec per deployment.
func (f File) Specs() ([]foundry.Spec, error) {
	var (
		out      []foundry.Spec
		problems []string
	)
	for ri, res := range f.Resources {
		if len(res.Deployments) == 0 {
			problems = append(problems, fmt.Sprintf("resources[%d] (%s): no deployments", ri, res.Name))
			continue
		}
		base := overlay(f.Defaults, res.Settings)
		base.Resource = res.Name
		for _, dep := range res.Deployments {
			s := overlay(base, dep)
			s.Resource = res.Name
			out = append(out, s)
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return out, nil
}

// overlay returns base with every non-zero field of over applied on top.
// DisableModelFallback is sticky: true at any level wins.
func overlay(base, over foundry.Spec) foundry.Spec {
	out := base
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&out.Environment, over.Environment)
	set(&out.Resource, over.Resource)
	set(&out.Endpoint, over.Endpoint)
	set(&out.Model, over.Model)
	set(&out.ModelVersion, over.ModelVersion)
	set(&out.ModelID, over.ModelID)
	set(&out.Deployment, over.Deployment)
	set(&out.APIKeySecret, over.APIKeySecret)
	set(&out.TenantID, over.TenantID)
	set(&out.ClientID, over.ClientID)
	set(&out.EntraAuthorityHost, over.EntraAuthorityHost)
	set(&out.LegacyAPIVersion, over.LegacyAPIVersion)
	set(&out.InferenceAPIVersion, over.InferenceAPIVersion)
	set(&out.EnterpriseKeyName, over.EnterpriseKeyName)
	set(&out.RunsOn, over.RunsOn)
	if over.HostKind != "" {
		out.HostKind = over.HostKind
	}
	if over.Surface != "" {
		out.Surface = over.Surface
	}
	if over.WireAPI != "" {
		out.WireAPI = over.WireAPI
	}
	if over.Auth != "" {
		out.Auth = over.Auth
	}
	if over.EnterpriseURLStyle != "" {
		out.EnterpriseURLStyle = over.EnterpriseURLStyle
	}
	out.DisableModelFallback = base.DisableModelFallback || over.DisableModelFallback
	return out
}

// Build resolves specs into a Bundle. Every failure across every spec is
// reported together.
func Build(toolVersion string, specs []foundry.Spec) (Bundle, error) {
	b := Bundle{ToolVersion: toolVersion}
	var problems []string
	seen := map[string]bool{}

	for i, s := range specs {
		r, err := foundry.Resolve(s)
		if err != nil {
			problems = append(problems, fmt.Sprintf("deployment #%d (%s/%s/%s): %v",
				i+1, s.Environment, s.Resource, s.Deployment, err))
			continue
		}
		id := strings.Join([]string{r.Spec.Environment, r.URLs.Origin, r.Spec.Deployment}, "|")
		if seen[id] {
			problems = append(problems, fmt.Sprintf("duplicate deployment %q on %s (environment %q)",
				r.Spec.Deployment, r.URLs.Origin, r.Spec.Environment))
			continue
		}
		seen[id] = true
		b.Items = append(b.Items, r)
	}

	sort.SliceStable(b.Items, func(i, j int) bool {
		a, c := b.Items[i].Spec, b.Items[j].Spec
		if a.Environment != c.Environment {
			return a.Environment < c.Environment
		}
		if a.Resource != c.Resource {
			return a.Resource < c.Resource
		}
		return a.Deployment < c.Deployment
	})

	groups, skipped, gp := groupEnterprise(b.Items)
	b.EnterpriseGroups, b.EnterpriseSkipped = groups, skipped
	problems = append(problems, gp...)

	// Two deployments must never render to the same workflow file.
	slugs := map[string]string{}
	for _, it := range b.Items {
		key := it.Spec.Environment + "/" + it.Spec.Resource + "/" + it.Spec.Deployment
		if prev, ok := slugs[it.GhAw.WorkflowSlug]; ok {
			problems = append(problems, fmt.Sprintf("workflow slug %q is produced by both %s and %s; give custom-endpoint entries a distinct resource name",
				it.GhAw.WorkflowSlug, prev, key))
		}
		slugs[it.GhAw.WorkflowSlug] = key
	}

	if len(problems) > 0 {
		return Bundle{}, fmt.Errorf("inventory has %d problem(s):\n  - %s", len(problems), strings.Join(problems, "\n  - "))
	}
	return b, nil
}

func groupEnterprise(items []foundry.Resolved) ([]EnterpriseGroup, []Skipped, []string) {
	var (
		problems []string
		skipped  []Skipped
		order    []string
		byKey    = map[string]*EnterpriseGroup{}
	)
	for _, it := range items {
		e := it.Enterprise
		if !e.Supported {
			skipped = append(skipped, Skipped{
				Environment: it.Spec.Environment, Resource: it.Spec.Resource, Deployment: it.Spec.Deployment,
				Reason: "auth=entra: the custom-models form accepts an API key only",
			})
			continue
		}
		g, ok := byKey[e.KeyName]
		if !ok {
			g = &EnterpriseGroup{
				KeyName: e.KeyName, Provider: e.Provider, DeploymentURL: e.DeploymentURL,
				Alternates: e.Alternates, APIKeySecret: it.Spec.APIKeySecret,
				Environment: it.Spec.Environment, Resource: it.Spec.Resource,
				HostAccepted: e.HostAccepted,
			}
			byKey[e.KeyName] = g
			order = append(order, e.KeyName)
		}
		// GitHub: "If your models have different deployment URLs, they cannot
		// be added to the same API key."
		if g.DeploymentURL != e.DeploymentURL {
			problems = append(problems, fmt.Sprintf("enterprise key %q maps to two deployment URLs (%s and %s); GitHub allows one URL per key entry. Set enterprise_key_name on one of them",
				e.KeyName, g.DeploymentURL, e.DeploymentURL))
			continue
		}
		if g.APIKeySecret != it.Spec.APIKeySecret {
			problems = append(problems, fmt.Sprintf("enterprise key %q is fed by two different secrets (%s and %s)",
				e.KeyName, g.APIKeySecret, it.Spec.APIKeySecret))
			continue
		}
		g.ModelIDs = append(g.ModelIDs, e.ModelID)
	}

	sort.Strings(order)
	out := make([]EnterpriseGroup, 0, len(order))
	for _, k := range order {
		g := byKey[k]
		sort.Strings(g.ModelIDs)
		g.ModelIDs = dedupe(g.ModelIDs)
		out = append(out, *g)
	}
	return out, skipped, problems
}

// dedupe removes adjacent duplicates from a sorted slice.
func dedupe(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
