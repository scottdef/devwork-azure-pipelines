// Package foundry builds Azure AI Foundry / Azure OpenAI endpoint URLs and the
// GitHub Copilot BYOK settings derived from them.
//
// Every URL shape in this package is traceable to a primary source:
//
//   - Azure/azure-rest-api-specs  specification/ai/data-plane/OpenAI.v1
//     (azure-v1-v1-generated.json): server template "{endpoint}/openai/v1",
//     paths /chat/completions, /responses, /embeddings, /models, /models/{model};
//     auth headers "api-key" and "authorization", OAuth2 scope
//     https://cognitiveservices.azure.com/.default; optional api-version query
//     parameter with enum [v1, preview].
//   - Azure/azure-rest-api-specs  specification/ai/data-plane/ModelInference
//     (2024-05-01-preview): host template
//     "https://{resource}.services.ai.azure.com/models", required api-version.
//   - epam/ai-dial-adapter-openai README: v1 vs. legacy deployment-in-path
//     upstreams, Foundry Anthropic messages endpoint, and the rule that on the
//     v1 API the request body "model" carries the Azure *deployment* name.
//   - github.github.com/gh-aw/reference/azure-openai-byok: COPILOT_PROVIDER_*
//     variables, engine.model vs. COPILOT_PROVIDER_MODEL_ID, responses wire API.
//
// The package is pure (no I/O) so it is trivially testable.
package foundry

// HostKind selects which Azure hostname family the resource is addressed by.
type HostKind string

const (
	// HostOpenAI is https://{resource}.openai.azure.com (default; the only
	// family every Copilot BYOK surface is documented to accept).
	HostOpenAI HostKind = "openai"
	// HostCognitive is https://{resource}.cognitiveservices.azure.com.
	HostCognitive HostKind = "cognitiveservices"
	// HostServicesAI is https://{resource}.services.ai.azure.com.
	HostServicesAI HostKind = "services-ai"
	// HostCustom means Spec.Endpoint is used verbatim (private endpoint DNS,
	// APIM gateway, sovereign cloud, local test server).
	HostCustom HostKind = "custom"
)

// APISurface is the protocol a deployment speaks.
type APISurface string

const (
	// SurfaceAuto infers the surface from the model name.
	SurfaceAuto APISurface = "auto"
	// SurfaceOpenAIV1 is the OpenAI-compatible v1 API ({endpoint}/openai/v1).
	SurfaceOpenAIV1 APISurface = "openai-v1"
	// SurfaceAnthropic is the Foundry-hosted Anthropic Messages API.
	SurfaceAnthropic APISurface = "anthropic"
)

// WireAPI is the Copilot CLI "wire" protocol for OpenAI-compatible providers.
type WireAPI string

const (
	WireAuto        WireAPI = "auto"
	WireResponses   WireAPI = "responses"
	WireCompletions WireAPI = "completions"
)

// AuthMode is how callers authenticate to the Azure resource.
type AuthMode string

const (
	AuthAPIKey AuthMode = "api-key"
	AuthEntra  AuthMode = "entra"
)

// EnterpriseURLStyle selects the value emitted for the "Deployment URL" field
// of the GitHub Copilot "Microsoft Foundry" custom-model provider form.
//
// GitHub's documentation names the field but does not specify its format, so
// the style is explicit and switchable. See docs/COPILOT-ENTERPRISE-BYOK.md.
type EnterpriseURLStyle string

const (
	// EnterpriseURLV1 emits {origin}/openai/v1 (shared by every deployment on
	// the resource, so many Model IDs fit under one key entry).
	EnterpriseURLV1 EnterpriseURLStyle = "v1"
	// EnterpriseURLDeployment emits the legacy per-deployment "Target URI":
	// {origin}/openai/deployments/{deployment}/chat/completions?api-version=...
	EnterpriseURLDeployment EnterpriseURLStyle = "deployment"
	// EnterpriseURLRoot emits the bare resource origin.
	EnterpriseURLRoot EnterpriseURLStyle = "root"
)

// Defaults applied by Resolve when a Spec leaves a field empty.
const (
	// DefaultLegacyAPIVersion is the latest GA data-plane version for the
	// deployment-in-path API and the documented Copilot SDK default for
	// provider type "azure".
	DefaultLegacyAPIVersion = "2024-10-21"
	// DefaultInferenceAPIVersion is the only published version of the
	// (retired) Azure AI Model Inference API.
	DefaultInferenceAPIVersion = "2024-05-01-preview"
	// DefaultAPIKeySecret is the GitHub Actions secret name used in generated
	// workflows. It matches the name used in the gh-aw reference.
	DefaultAPIKeySecret = "AZURE_OPENAI_API_KEY"
	// DefaultEntraAuthorityHost must be reachable from the gh-aw firewall when
	// github-oidc auth is used.
	DefaultEntraAuthorityHost = "login.microsoftonline.com"
	// EntraScope is the OAuth2 scope declared by both Azure specs.
	EntraScope = "https://cognitiveservices.azure.com/.default"
)

// Spec is the caller-supplied description of one deployed model.
// Only Model, Deployment and one of Resource/Endpoint are required.
type Spec struct {
	// Environment is a free-form tier label (dev, uat, prod). Optional.
	Environment string `json:"environment,omitempty"`
	// Resource is the Azure AI Foundry / Azure OpenAI resource (custom
	// subdomain) name. Required unless Endpoint is set.
	Resource string `json:"resource,omitempty"`
	// Endpoint overrides host construction, e.g. "https://apim.contoso.com/ai".
	// Do not include the "/openai/v1" suffix; it is stripped if present.
	Endpoint string `json:"endpoint,omitempty"`
	// HostKind defaults to "openai" (or "custom" when Endpoint is set).
	HostKind HostKind `json:"host_kind,omitempty"`

	// Model is the model name as listed in the Foundry catalog, e.g. "gpt-5.4".
	Model string `json:"model"`
	// ModelVersion is the catalog version, e.g. "2026-03-05". Optional.
	ModelVersion string `json:"model_version,omitempty"`
	// ModelID overrides the computed Azure model ID (what GET /openai/v1/models
	// returns). Normally computed as Model or Model-ModelVersion.
	ModelID string `json:"model_id,omitempty"`
	// Deployment is the Azure deployment name (what goes on the wire).
	Deployment string `json:"deployment"`

	// Surface defaults to "auto".
	Surface APISurface `json:"surface,omitempty"`
	// WireAPI defaults to "auto".
	WireAPI WireAPI `json:"wire_api,omitempty"`

	// Auth defaults to "api-key".
	Auth AuthMode `json:"auth,omitempty"`
	// APIKeySecret is the *name* of the GitHub secret / env var that holds the
	// key. The key itself is never accepted by this tool.
	APIKeySecret string `json:"api_key_secret,omitempty"`
	// TenantID and ClientID configure gh-aw github-oidc federation (Auth=entra).
	TenantID string `json:"tenant_id,omitempty"`
	ClientID string `json:"client_id,omitempty"`
	// EntraAuthorityHost overrides login.microsoftonline.com (sovereign clouds).
	EntraAuthorityHost string `json:"entra_authority_host,omitempty"`

	// LegacyAPIVersion is the api-version for deployment-in-path URLs.
	LegacyAPIVersion string `json:"legacy_api_version,omitempty"`
	// InferenceAPIVersion is the api-version for the retired /models API.
	InferenceAPIVersion string `json:"inference_api_version,omitempty"`

	// EnterpriseURLStyle defaults to "v1".
	EnterpriseURLStyle EnterpriseURLStyle `json:"enterprise_url_style,omitempty"`
	// EnterpriseKeyName is the "Name" shown in the Copilot model picker for
	// the key entry. Defaults to "foundry-{environment}-{resource}".
	EnterpriseKeyName string `json:"enterprise_key_name,omitempty"`

	// RunsOn optionally sets runs-on in generated gh-aw workflows
	// (self-hosted runner labels). Empty means gh-aw's default.
	RunsOn string `json:"runs_on,omitempty"`
	// DisableModelFallback emits sandbox.agent.model-fallback: false, the
	// documented workaround when AWF rewrites the deployment name.
	DisableModelFallback bool `json:"disable_model_fallback,omitempty"`
}

// URLSet holds every direct-API URL for a deployment.
// Fields that do not apply to the resolved surface are empty.
type URLSet struct {
	// Origin is scheme://host[:port][/prefix] with no trailing slash.
	Origin string `json:"origin"`

	V1Base            string `json:"v1_base,omitempty"`
	V1ChatCompletions string `json:"v1_chat_completions,omitempty"`
	V1Responses       string `json:"v1_responses,omitempty"`
	V1Embeddings      string `json:"v1_embeddings,omitempty"`
	V1Models          string `json:"v1_models,omitempty"`
	V1Model           string `json:"v1_model,omitempty"`

	LegacyDeploymentBase  string `json:"legacy_deployment_base,omitempty"`
	LegacyChatCompletions string `json:"legacy_chat_completions,omitempty"`
	LegacyEmbeddings      string `json:"legacy_embeddings,omitempty"`

	// ModelInferenceChatCompletions is the retired services.ai /models API.
	ModelInferenceChatCompletions string `json:"model_inference_chat_completions,omitempty"`

	AnthropicBase     string `json:"anthropic_base,omitempty"`
	AnthropicMessages string `json:"anthropic_messages,omitempty"`
}

// GhAw holds the values for a gh-aw workflow's engine/network frontmatter.
type GhAw struct {
	// EngineModel is engine.model (compiled to COPILOT_MODEL): the Azure
	// model ID as listed by GET /openai/v1/models.
	EngineModel string `json:"engine_model"`
	// ProviderBaseURL is COPILOT_PROVIDER_BASE_URL.
	ProviderBaseURL string `json:"provider_base_url"`
	// ProviderType is COPILOT_PROVIDER_TYPE; empty means the default (openai).
	ProviderType string `json:"provider_type,omitempty"`
	// ProviderModelID is COPILOT_PROVIDER_MODEL_ID; empty when the deployment
	// name equals EngineModel.
	ProviderModelID string `json:"provider_model_id,omitempty"`
	// WireAPI is COPILOT_PROVIDER_WIRE_API; empty when not applicable.
	WireAPI WireAPI `json:"wire_api,omitempty"`
	// APIKeyExpr is the ${{ secrets.NAME }} expression; empty for Entra auth.
	APIKeyExpr string `json:"api_key_expr,omitempty"`
	// NetworkAllowed is the network.allowed list (always starts with defaults).
	NetworkAllowed []string `json:"network_allowed"`
	// WorkflowSlug is a filesystem- and Actions-safe identifier.
	WorkflowSlug string `json:"workflow_slug"`
}

// CopilotClient holds settings for the local Copilot CLI and the Copilot SDK.
type CopilotClient struct {
	// CLIBaseURL / CLIModel are for COPILOT_PROVIDER_TYPE unset (openai) or
	// "anthropic": COPILOT_PROVIDER_BASE_URL and COPILOT_MODEL.
	CLIBaseURL string `json:"cli_base_url"`
	CLIModel   string `json:"cli_model"`
	// CLIAzureBaseURL is the alternative for COPILOT_PROVIDER_TYPE=azure.
	CLIAzureBaseURL string `json:"cli_azure_base_url,omitempty"`
	// SDKBaseURL is ProviderConfig.baseUrl (trailing slash, as documented).
	SDKBaseURL string `json:"sdk_base_url"`
	SDKType    string `json:"sdk_type"`
	SDKModel   string `json:"sdk_model"`
}

// Enterprise holds the values typed into the GitHub enterprise/organization
// "Configure custom models" form for the Microsoft Foundry provider.
type Enterprise struct {
	Provider      string `json:"provider"`
	KeyName       string `json:"key_name"`
	DeploymentURL string `json:"deployment_url"`
	ModelID       string `json:"model_id"`
	// Alternates lists the other URL styles, in the order to try them if the
	// form rejects DeploymentURL.
	Alternates []string `json:"deployment_url_alternates,omitempty"`
	// HostAccepted reports whether the hostname matches the patterns the form
	// is known to enforce (*.openai.azure.com, *.services.ai.azure.com).
	HostAccepted bool `json:"host_accepted"`
	// Supported is false when the enterprise form cannot work at all for this
	// spec (for example Entra-only resources: the form takes an API key).
	Supported bool `json:"supported"`
}

// Resolved is the full, normalized result for one deployment.
type Resolved struct {
	Spec       Spec          `json:"spec"`
	Host       string        `json:"host"`
	ModelID    string        `json:"model_id"`
	Surface    APISurface    `json:"surface"`
	WireAPI    WireAPI       `json:"wire_api,omitempty"`
	URLs       URLSet        `json:"urls"`
	GhAw       GhAw          `json:"gh_aw"`
	Client     CopilotClient `json:"copilot_client"`
	Enterprise Enterprise    `json:"copilot_enterprise"`
	// Warnings are non-fatal findings the operator should read.
	Warnings []Warning `json:"warnings,omitempty"`
}

// Warning scopes. A generated artifact only repeats the warnings that concern
// it, so a gh-aw workflow is not cluttered with enterprise-form caveats.
const (
	ScopeAll        = "all"
	ScopeGhAw       = "gh-aw"
	ScopeEnterprise = "enterprise"
)

// Warning is a non-fatal finding, tagged with the consumer it concerns.
type Warning struct {
	Scope   string `json:"scope"`
	Message string `json:"message"`
}

func (w Warning) String() string { return "[" + w.Scope + "] " + w.Message }

// WarningsFor returns the messages that apply to the given scope: those tagged
// with it plus those tagged ScopeAll.
func (r Resolved) WarningsFor(scope string) []string {
	var out []string
	for _, w := range r.Warnings {
		if w.Scope == ScopeAll || w.Scope == scope {
			out = append(out, w.Message)
		}
	}
	return out
}
