package foundry

import (
	"reflect"
	"strings"
	"testing"
)

func warnText(r Resolved) string {
	var parts []string
	for _, w := range r.Warnings {
		parts = append(parts, w.String())
	}
	return strings.Join(parts, "\n")
}

func mustResolve(t *testing.T, s Spec) Resolved {
	t.Helper()
	r, err := Resolve(s)
	if err != nil {
		t.Fatalf("Resolve(%+v): %v", s, err)
	}
	return r
}

// TestGhAwReferenceExample reproduces the API-key example from
// github.github.com/gh-aw/reference/azure-openai-byok verbatim.
func TestGhAwReferenceExample(t *testing.T) {
	r := mustResolve(t, Spec{
		Resource: "RESOURCE", Model: "gpt-5.4", ModelVersion: "2026-03-05", Deployment: "gpt-5.4",
	})
	want := GhAw{
		EngineModel:     "gpt-5.4-2026-03-05",
		ProviderBaseURL: "https://resource.openai.azure.com/openai/v1",
		ProviderModelID: "gpt-5.4",
		WireAPI:         WireResponses,
		APIKeyExpr:      "${{ secrets.AZURE_OPENAI_API_KEY }}",
		NetworkAllowed:  []string{"defaults", "resource.openai.azure.com"},
		WorkflowSlug:    "smoke-foundry-resource-gpt-5-4",
	}
	if !reflect.DeepEqual(r.GhAw, want) {
		t.Errorf("GhAw mismatch\n got: %+v\nwant: %+v", r.GhAw, want)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", r.Warnings)
	}
}

// TestGhAwEntraExample reproduces the Microsoft Entra example: no API key,
// and login.microsoftonline.com added to the firewall allow-list.
func TestGhAwEntraExample(t *testing.T) {
	r := mustResolve(t, Spec{
		Resource: "myres", Model: "gpt-5.4", ModelVersion: "2026-03-05", Deployment: "gpt-5.4",
		Auth:     AuthEntra,
		TenantID: "11111111-2222-3333-4444-555555555555",
		ClientID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	})
	if r.GhAw.APIKeyExpr != "" {
		t.Errorf("APIKeyExpr = %q, want empty for entra", r.GhAw.APIKeyExpr)
	}
	want := []string{"defaults", "myres.openai.azure.com", "login.microsoftonline.com"}
	if !reflect.DeepEqual(r.GhAw.NetworkAllowed, want) {
		t.Errorf("NetworkAllowed = %v, want %v", r.GhAw.NetworkAllowed, want)
	}
	if r.Enterprise.Supported {
		t.Error("Enterprise.Supported = true; the enterprise form is API-key only")
	}
}

// TestDirectAPIURLs checks every direct-API shape against the Azure specs and
// the DIAL adapter README.
func TestDirectAPIURLs(t *testing.T) {
	r := mustResolve(t, Spec{Resource: "svc", Model: "gpt-4o", Deployment: "gpt-4o-prod"})
	got := r.URLs
	want := URLSet{
		Origin:                        "https://svc.openai.azure.com",
		V1Base:                        "https://svc.openai.azure.com/openai/v1",
		V1ChatCompletions:             "https://svc.openai.azure.com/openai/v1/chat/completions",
		V1Responses:                   "https://svc.openai.azure.com/openai/v1/responses",
		V1Embeddings:                  "https://svc.openai.azure.com/openai/v1/embeddings",
		V1Models:                      "https://svc.openai.azure.com/openai/v1/models",
		V1Model:                       "https://svc.openai.azure.com/openai/v1/models/gpt-4o-prod",
		LegacyDeploymentBase:          "https://svc.openai.azure.com/openai/deployments/gpt-4o-prod",
		LegacyChatCompletions:         "https://svc.openai.azure.com/openai/deployments/gpt-4o-prod/chat/completions?api-version=2024-10-21",
		LegacyEmbeddings:              "https://svc.openai.azure.com/openai/deployments/gpt-4o-prod/embeddings?api-version=2024-10-21",
		ModelInferenceChatCompletions: "https://svc.services.ai.azure.com/models/chat/completions?api-version=2024-05-01-preview",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("URLs mismatch\n got: %+v\nwant: %+v", got, want)
	}
	if r.WireAPI != WireCompletions {
		t.Errorf("WireAPI = %q, want completions for gpt-4o", r.WireAPI)
	}
}

func TestHostKinds(t *testing.T) {
	for kind, host := range map[HostKind]string{
		HostOpenAI:     "svc.openai.azure.com",
		HostCognitive:  "svc.cognitiveservices.azure.com",
		HostServicesAI: "svc.services.ai.azure.com",
	} {
		r := mustResolve(t, Spec{Resource: "svc", HostKind: kind, Model: "gpt-4o", Deployment: "d1"})
		if r.Host != host {
			t.Errorf("%s: Host = %q, want %q", kind, r.Host, host)
		}
		wantAccepted := kind != HostCognitive
		if r.Enterprise.HostAccepted != wantAccepted {
			t.Errorf("%s: HostAccepted = %v, want %v", kind, r.Enterprise.HostAccepted, wantAccepted)
		}
	}
}

// TestAnthropicSurface checks the Foundry Claude endpoint from the DIAL README.
func TestAnthropicSurface(t *testing.T) {
	r := mustResolve(t, Spec{Resource: "fdy", Model: "claude-opus-4-5", Deployment: "claude-opus-4-5"})
	if r.Surface != SurfaceAnthropic {
		t.Fatalf("Surface = %q", r.Surface)
	}
	if got, want := r.URLs.AnthropicMessages, "https://fdy.services.ai.azure.com/anthropic/v1/messages"; got != want {
		t.Errorf("AnthropicMessages = %q, want %q", got, want)
	}
	if r.URLs.V1Base != "" || r.WireAPI != "" {
		t.Errorf("openai fields should be empty: V1Base=%q WireAPI=%q", r.URLs.V1Base, r.WireAPI)
	}
	if r.GhAw.ProviderType != "anthropic" || r.GhAw.ProviderBaseURL != "https://fdy.services.ai.azure.com/anthropic" {
		t.Errorf("GhAw = %+v", r.GhAw)
	}
	if r.GhAw.ProviderModelID != "" {
		t.Errorf("ProviderModelID = %q, want empty when deployment == model ID", r.GhAw.ProviderModelID)
	}
	if !strings.Contains(warnText(r), "untested") {
		t.Errorf("expected an 'untested' warning, got %v", r.Warnings)
	}
	// A defaulted host_kind must not be reported as "ignored"; an explicit one must.
	if strings.Contains(warnText(r), "was ignored") {
		t.Errorf("defaulted host_kind produced an 'ignored' warning: %v", r.Warnings)
	}
	r = mustResolve(t, Spec{Resource: "fdy", HostKind: HostOpenAI, Model: "claude-opus-4-5", Deployment: "c"})
	if !strings.Contains(warnText(r), "was ignored") {
		t.Errorf("explicit host_kind=openai should warn that it was ignored: %v", r.Warnings)
	}
}

func TestCustomEndpoint(t *testing.T) {
	// A pasted API suffix and trailing slash are tolerated.
	r := mustResolve(t, Spec{
		Endpoint: "https://apim.contoso.com/ai/openai/v1/", Resource: "fdy-prod",
		Model: "gpt-4.1", Deployment: "gpt41",
	})
	if got, want := r.URLs.V1Base, "https://apim.contoso.com/ai/openai/v1"; got != want {
		t.Errorf("V1Base = %q, want %q", got, want)
	}
	if r.Host != "apim.contoso.com" {
		t.Errorf("Host = %q", r.Host)
	}
	if r.Enterprise.HostAccepted {
		t.Error("HostAccepted = true for a gateway host; the enterprise form rejects these")
	}
	// The firewall entry must be a bare hostname even when a port is present.
	r = mustResolve(t, Spec{Endpoint: "http://127.0.0.1:8080", Model: "gpt-4o", Deployment: "d"})
	if got := r.GhAw.NetworkAllowed[1]; got != "127.0.0.1" {
		t.Errorf("NetworkAllowed host = %q, want 127.0.0.1", got)
	}
}

func TestEnterpriseStyles(t *testing.T) {
	base := Spec{Environment: "prod", Resource: "svc", Model: "gpt-4o", Deployment: "gpt-4o-prod"}
	cases := []struct {
		style   EnterpriseURLStyle
		wantURL string
		wantKey string
	}{
		{EnterpriseURLV1, "https://svc.openai.azure.com/openai/v1", "foundry-prod-svc"},
		{EnterpriseURLDeployment, "https://svc.openai.azure.com/openai/deployments/gpt-4o-prod/chat/completions?api-version=2024-10-21", "foundry-prod-svc-gpt-4o-prod"},
		{EnterpriseURLRoot, "https://svc.openai.azure.com", "foundry-prod-svc"},
	}
	for _, c := range cases {
		s := base
		s.EnterpriseURLStyle = c.style
		r := mustResolve(t, s)
		e := r.Enterprise
		if e.DeploymentURL != c.wantURL || e.KeyName != c.wantKey {
			t.Errorf("%s: got URL=%q key=%q, want URL=%q key=%q", c.style, e.DeploymentURL, e.KeyName, c.wantURL, c.wantKey)
		}
		if e.ModelID != "gpt-4o-prod" {
			t.Errorf("%s: ModelID = %q, want the deployment name", c.style, e.ModelID)
		}
		if len(e.Alternates) != 2 {
			t.Errorf("%s: Alternates = %v, want 2", c.style, e.Alternates)
		}
		for _, a := range e.Alternates {
			if a == e.DeploymentURL {
				t.Errorf("%s: primary URL repeated in Alternates", c.style)
			}
		}
	}
}

func TestCopilotClient(t *testing.T) {
	r := mustResolve(t, Spec{Resource: "svc", Model: "gpt-5.2-codex", Deployment: "codex"})
	want := CopilotClient{
		CLIBaseURL:      "https://svc.openai.azure.com/openai/v1",
		CLIModel:        "gpt-5.2-codex",
		CLIAzureBaseURL: "https://svc.openai.azure.com",
		SDKBaseURL:      "https://svc.openai.azure.com/openai/v1/", // trailing slash as documented
		SDKType:         "openai",
		SDKModel:        "codex",
	}
	if !reflect.DeepEqual(r.Client, want) {
		t.Errorf("Client mismatch\n got: %+v\nwant: %+v", r.Client, want)
	}
}

func TestInferWireAPI(t *testing.T) {
	cases := map[string]WireAPI{
		"gpt-5": WireResponses, "gpt-5.4": WireResponses, "GPT-5-mini": WireResponses,
		"o1": WireResponses, "o3-pro": WireResponses, "o4-mini": WireResponses,
		"gpt-5.2-codex": WireResponses, "codex-mini": WireResponses,
		"gpt-4o": WireCompletions, "gpt-4.1": WireCompletions, "gpt-oss-120b": WireCompletions,
		"Mistral-Large-2411": WireCompletions, "grok-3": WireCompletions,
		"omni-moderation": WireCompletions, // starts with 'o' but is not o-series
	}
	for model, want := range cases {
		if got := InferWireAPI(model); got != want {
			t.Errorf("InferWireAPI(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestComposeModelID(t *testing.T) {
	cases := []struct{ model, version, want string }{
		{"gpt-5.4", "2026-03-05", "gpt-5.4-2026-03-05"},
		{"gpt-5.4-2026-03-05", "2026-03-05", "gpt-5.4-2026-03-05"},
		{"gpt-4o", "", "gpt-4o"},
	}
	for _, c := range cases {
		if got := ComposeModelID(c.model, c.version); got != c.want {
			t.Errorf("ComposeModelID(%q,%q) = %q, want %q", c.model, c.version, got, c.want)
		}
	}
}

func TestWarnings(t *testing.T) {
	r := mustResolve(t, Spec{Resource: "svc", Model: "gpt-5.4", Deployment: "gpt-5.4", WireAPI: WireCompletions})
	joined := warnText(r)
	for _, frag := range []string{"responses wire API", "no version suffix"} {
		if !strings.Contains(joined, frag) {
			t.Errorf("missing warning containing %q in:\n%s", frag, joined)
		}
	}
}

func TestValidationAggregatesProblems(t *testing.T) {
	_, err := Resolve(Spec{
		Resource: "-bad-", Model: "", Deployment: "has space",
		APIKeySecret: "GITHUB_TOKEN", EnterpriseURLStyle: "nope", RunsOn: "self-hosted, bad label",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	p, ok := err.(Problems)
	if !ok {
		t.Fatalf("error type %T, want Problems", err)
	}
	if len(p) < 6 {
		t.Errorf("got %d problems, want all 6 reported at once:\n%v", len(p), err)
	}
}

func TestValidationRejections(t *testing.T) {
	ok := Spec{Resource: "svc", Model: "gpt-4o", Deployment: "d"}
	mutate := map[string]func(*Spec){
		"no resource or endpoint":   func(s *Spec) { s.Resource = "" },
		"http non-loopback":         func(s *Spec) { s.Resource = ""; s.Endpoint = "http://example.com" },
		"endpoint with credentials": func(s *Spec) { s.Resource = ""; s.Endpoint = "https://u:p@example.com" },
		"endpoint with query":       func(s *Spec) { s.Resource = ""; s.Endpoint = "https://example.com/?x=1" },
		"host_kind with endpoint":   func(s *Spec) { s.Endpoint = "https://example.com"; s.HostKind = HostOpenAI },
		"custom without endpoint":   func(s *Spec) { s.HostKind = HostCustom },
		"entra without ids":         func(s *Spec) { s.Auth = AuthEntra },
		"bad api version":           func(s *Spec) { s.LegacyAPIVersion = "2024/10/21" },
		"resource too short":        func(s *Spec) { s.Resource = "a" },
	}
	for name, fn := range mutate {
		s := ok
		fn(&s)
		if _, err := Resolve(s); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := Resolve(ok); err != nil {
		t.Errorf("baseline spec rejected: %v", err)
	}
}

func TestSlug(t *testing.T) {
	if got, want := Slug("Smoke Foundry", "", "gpt-5.4_Prod"), "smoke-foundry-gpt-5-4-prod"; got != want {
		t.Errorf("Slug = %q, want %q", got, want)
	}
}
