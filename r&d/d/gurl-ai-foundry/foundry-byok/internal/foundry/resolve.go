package foundry

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// hostSuffix maps a HostKind to its public-cloud DNS suffix.
var hostSuffix = map[HostKind]string{
	HostOpenAI:     "openai.azure.com",
	HostCognitive:  "cognitiveservices.azure.com",
	HostServicesAI: "services.ai.azure.com",
}

// enterpriseHostSuffixes are the hostname patterns the GitHub "Microsoft
// Foundry" custom-model form enforces before it sends any request
// (github.com/orgs/community/discussions/198472).
var enterpriseHostSuffixes = []string{".openai.azure.com", ".services.ai.azure.com"}

// endpointSuffixes are API path suffixes people paste by habit. They are
// stripped from Spec.Endpoint so the origin can be recomposed consistently.
// Order matters: longest first.
var endpointSuffixes = []string{
	"/anthropic/v1/messages", "/anthropic/v1", "/anthropic",
	"/openai/v1", "/openai", "/models",
}

var reDateSuffix = regexp.MustCompile(`-[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// normalize trims input and applies defaults. It never fails.
func normalize(in Spec) Spec {
	s := in
	s.Environment = strings.TrimSpace(s.Environment)
	s.Resource = strings.ToLower(strings.TrimSpace(s.Resource))
	s.Model = strings.TrimSpace(s.Model)
	s.ModelVersion = strings.TrimSpace(s.ModelVersion)
	s.ModelID = strings.TrimSpace(s.ModelID)
	s.Deployment = strings.TrimSpace(s.Deployment)
	s.APIKeySecret = strings.TrimSpace(s.APIKeySecret)
	s.TenantID = strings.TrimSpace(s.TenantID)
	s.ClientID = strings.TrimSpace(s.ClientID)
	s.EnterpriseKeyName = strings.TrimSpace(s.EnterpriseKeyName)
	s.RunsOn = strings.Join(RunsOnLabels(s.RunsOn), ", ")

	s.Endpoint = strings.TrimRight(strings.TrimSpace(s.Endpoint), "/")
	for _, suf := range endpointSuffixes {
		if strings.HasSuffix(strings.ToLower(s.Endpoint), suf) {
			s.Endpoint = s.Endpoint[:len(s.Endpoint)-len(suf)]
			break
		}
	}
	s.Endpoint = strings.TrimRight(s.Endpoint, "/")

	if s.HostKind == "" {
		if s.Endpoint != "" {
			s.HostKind = HostCustom
		} else {
			s.HostKind = HostOpenAI
		}
	}
	if s.Surface == "" {
		s.Surface = SurfaceAuto
	}
	if s.WireAPI == "" {
		s.WireAPI = WireAuto
	}
	if s.Auth == "" {
		s.Auth = AuthAPIKey
	}
	if s.Auth == AuthAPIKey && s.APIKeySecret == "" {
		s.APIKeySecret = DefaultAPIKeySecret
	}
	if s.EntraAuthorityHost == "" {
		s.EntraAuthorityHost = DefaultEntraAuthorityHost
	}
	if s.LegacyAPIVersion == "" {
		s.LegacyAPIVersion = DefaultLegacyAPIVersion
	}
	if s.InferenceAPIVersion == "" {
		s.InferenceAPIVersion = DefaultInferenceAPIVersion
	}
	if s.EnterpriseURLStyle == "" {
		s.EnterpriseURLStyle = EnterpriseURLV1
	}
	return s
}

// Resolve validates a Spec and derives every URL and Copilot setting from it.
func Resolve(in Spec) (Resolved, error) {
	s := normalize(in)
	if err := s.Validate(); err != nil {
		return Resolved{}, err
	}

	r := Resolved{Spec: s}
	r.ModelID = s.ModelID
	if r.ModelID == "" {
		r.ModelID = ComposeModelID(s.Model, s.ModelVersion)
	}

	r.Surface = s.Surface
	if r.Surface == SurfaceAuto {
		r.Surface = InferSurface(s.Model)
	}
	if r.Surface == SurfaceOpenAIV1 {
		r.WireAPI = s.WireAPI
		if r.WireAPI == WireAuto {
			r.WireAPI = InferWireAPI(s.Model)
		}
	}

	// Origins. The OpenAI-compatible surface lives on the selected host kind;
	// the Anthropic surface and the retired model-inference API only exist on
	// services.ai.azure.com. A custom endpoint fronts all of them.
	openaiOrigin, servicesOrigin := s.Endpoint, s.Endpoint
	if s.Endpoint == "" {
		openaiOrigin = "https://" + s.Resource + "." + hostSuffix[s.HostKind]
		servicesOrigin = "https://" + s.Resource + "." + hostSuffix[HostServicesAI]
	}
	origin := openaiOrigin
	if r.Surface == SurfaceAnthropic {
		origin = servicesOrigin
	}
	u, err := url.Parse(origin)
	if err != nil { // unreachable after Validate; kept for safety
		return Resolved{}, fmt.Errorf("origin %q: %w", origin, err)
	}
	r.Host = u.Hostname()

	r.URLs = buildURLs(s, r.Surface, origin, servicesOrigin)
	r.GhAw = buildGhAw(s, r)
	r.Client = buildClient(s, r)
	r.Enterprise = buildEnterprise(s, r)
	r.Warnings = warnings(s, r, in.HostKind != "")
	return r, nil
}

func buildURLs(s Spec, surface APISurface, origin, servicesOrigin string) URLSet {
	dep := url.PathEscape(s.Deployment)
	out := URLSet{Origin: origin}

	if surface == SurfaceAnthropic {
		out.AnthropicBase = origin + "/anthropic"
		out.AnthropicMessages = out.AnthropicBase + "/v1/messages"
		return out
	}

	// v1: unversioned; the deployment name travels in the request body.
	out.V1Base = origin + "/openai/v1"
	out.V1ChatCompletions = out.V1Base + "/chat/completions"
	out.V1Responses = out.V1Base + "/responses"
	out.V1Embeddings = out.V1Base + "/embeddings"
	out.V1Models = out.V1Base + "/models"
	out.V1Model = out.V1Models + "/" + dep

	// Legacy: deployment in the path, api-version required.
	q := "?api-version=" + url.QueryEscape(s.LegacyAPIVersion)
	out.LegacyDeploymentBase = origin + "/openai/deployments/" + dep
	out.LegacyChatCompletions = out.LegacyDeploymentBase + "/chat/completions" + q
	out.LegacyEmbeddings = out.LegacyDeploymentBase + "/embeddings" + q

	// Retired model-inference API (kept for migration inventories).
	out.ModelInferenceChatCompletions = servicesOrigin + "/models/chat/completions?api-version=" +
		url.QueryEscape(s.InferenceAPIVersion)
	return out
}

func buildGhAw(s Spec, r Resolved) GhAw {
	g := GhAw{
		EngineModel:    r.ModelID,
		NetworkAllowed: []string{"defaults", r.Host},
		WorkflowSlug:   Slug("smoke-foundry", s.Environment, s.Resource, s.Deployment),
	}
	if s.Deployment != r.ModelID {
		g.ProviderModelID = s.Deployment
	}
	if r.Surface == SurfaceAnthropic {
		g.ProviderBaseURL = r.URLs.AnthropicBase
		g.ProviderType = "anthropic"
	} else {
		g.ProviderBaseURL = r.URLs.V1Base
		g.WireAPI = r.WireAPI
	}
	if s.Auth == AuthAPIKey {
		g.APIKeyExpr = "${{ secrets." + s.APIKeySecret + " }}"
	} else {
		g.NetworkAllowed = append(g.NetworkAllowed, s.EntraAuthorityHost)
	}
	return g
}

func buildClient(s Spec, r Resolved) CopilotClient {
	c := CopilotClient{
		CLIBaseURL: r.GhAw.ProviderBaseURL,
		CLIModel:   r.ModelID,
		SDKModel:   s.Deployment,
	}
	if r.Surface == SurfaceAnthropic {
		c.SDKBaseURL = r.URLs.AnthropicBase
		c.SDKType = "anthropic"
		return c
	}
	c.CLIAzureBaseURL = r.URLs.Origin
	c.SDKBaseURL = r.URLs.V1Base + "/"
	c.SDKType = "openai"
	return c
}

func buildEnterprise(s Spec, r Resolved) Enterprise {
	e := Enterprise{
		Provider:  "Microsoft Foundry",
		ModelID:   s.Deployment,
		Supported: s.Auth == AuthAPIKey,
	}
	host := strings.ToLower(r.Host)
	for _, suf := range enterpriseHostSuffixes {
		if strings.HasSuffix(host, suf) {
			e.HostAccepted = true
			break
		}
	}

	// Candidate URLs in preference order for the selected style.
	var byStyle map[EnterpriseURLStyle]string
	if r.Surface == SurfaceAnthropic {
		byStyle = map[EnterpriseURLStyle]string{
			EnterpriseURLV1:         r.URLs.AnthropicMessages,
			EnterpriseURLDeployment: r.URLs.AnthropicBase,
			EnterpriseURLRoot:       r.URLs.Origin,
		}
	} else {
		byStyle = map[EnterpriseURLStyle]string{
			EnterpriseURLV1:         r.URLs.V1Base,
			EnterpriseURLDeployment: r.URLs.LegacyChatCompletions,
			EnterpriseURLRoot:       r.URLs.Origin,
		}
	}
	e.DeploymentURL = byStyle[s.EnterpriseURLStyle]
	for _, st := range []EnterpriseURLStyle{EnterpriseURLV1, EnterpriseURLDeployment, EnterpriseURLRoot} {
		if st != s.EnterpriseURLStyle {
			e.Alternates = append(e.Alternates, byStyle[st])
		}
	}

	e.KeyName = s.EnterpriseKeyName
	if e.KeyName == "" {
		label := s.Resource
		if label == "" {
			label = r.Host
		}
		parts := []string{"foundry", s.Environment, label}
		if r.Surface == SurfaceAnthropic {
			parts = append(parts, "anthropic")
		}
		// A per-deployment URL cannot share a key entry with its siblings.
		if s.EnterpriseURLStyle == EnterpriseURLDeployment && r.Surface != SurfaceAnthropic {
			parts = append(parts, s.Deployment)
		}
		e.KeyName = Slug(parts...)
	}
	return e
}

// warnings lists non-fatal findings. hostKindExplicit is true when the caller
// chose host_kind (rather than receiving the default), so that a defaulted
// value never produces an "ignored" warning.
func warnings(s Spec, r Resolved, hostKindExplicit bool) []Warning {
	var w []Warning
	add := func(scope, format string, args ...any) {
		w = append(w, Warning{Scope: scope, Message: fmt.Sprintf(format, args...)})
	}

	if r.Surface == SurfaceOpenAIV1 {
		if s.WireAPI == WireCompletions && InferWireAPI(s.Model) == WireResponses {
			add(ScopeGhAw, "wire_api=completions was forced, but gh-aw documents that GPT-5 and o-series deployments require the responses wire API")
		}
		if strings.HasPrefix(strings.ToLower(r.ModelID), "gpt-5") && !reDateSuffix.MatchString(r.ModelID) {
			add(ScopeGhAw, "model ID %q has no version suffix; Azure usually lists GPT-5 models fully qualified (e.g. gpt-5.4-2026-03-05). Set model_version or confirm with `foundry-byok verify`", r.ModelID)
		}
	}
	if r.Surface == SurfaceAnthropic {
		add(ScopeGhAw, "anthropic surface: the Foundry URL is confirmed, but gh-aw only documents Azure BYOK for the OpenAI-compatible endpoint. COPILOT_PROVIDER_TYPE=anthropic against Foundry is untested by this tool; run the generated smoke workflow before relying on it")
		if hostKindExplicit && s.HostKind != HostServicesAI && s.HostKind != HostCustom {
			add(ScopeAll, "anthropic surface is only served from %s.services.ai.azure.com; host_kind=%s was ignored for this deployment", s.Resource, s.HostKind)
		}
	}
	if !r.Enterprise.HostAccepted {
		add(ScopeEnterprise, "host %q does not match *.openai.azure.com or *.services.ai.azure.com, which the Microsoft Foundry custom-models form is reported to enforce. gh-aw and direct API output are unaffected", r.Host)
	}
	if s.Auth == AuthEntra {
		add(ScopeEnterprise, "the custom-models form takes an API key only, so this auth=entra deployment is not registered there. Entra applies to gh-aw (github-oidc) and direct API calls")
		if reGHExpr.MatchString(s.TenantID) || reGHExpr.MatchString(s.ClientID) {
			add(ScopeGhAw, "tenant_id/client_id use an Actions expression; gh-aw documents literal IDs in engine.auth. `gh aw compile` will tell you if expressions are rejected")
		}
	}
	if s.HostKind == HostCustom && s.Auth == AuthEntra {
		add(ScopeAll, "custom endpoint with auth=entra: make sure the gateway forwards or re-issues the bearer token for scope %s", EntraScope)
	}
	if u, err := url.Parse(r.URLs.Origin); err == nil && u.Scheme == "http" {
		add(ScopeAll, "plain-http endpoint: for local testing only")
	}
	return w
}
