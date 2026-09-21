package foundry

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Problems aggregates every validation failure for a Spec so an operator can
// fix an inventory in one pass rather than one error at a time.
type Problems []string

func (p Problems) Error() string {
	if len(p) == 1 {
		return p[0]
	}
	return fmt.Sprintf("%d problems:\n  - %s", len(p), strings.Join(p, "\n  - "))
}

func (p *Problems) addf(format string, args ...any) {
	*p = append(*p, fmt.Sprintf(format, args...))
}

// orNil returns nil when no problems were recorded, so callers can return the
// result directly as an error without the typed-nil interface pitfall.
func (p Problems) orNil() error {
	if len(p) == 0 {
		return nil
	}
	return p
}

var (
	// Azure AI services account / custom-subdomain names: 2-64 characters,
	// alphanumerics and hyphens, must start and end with an alphanumeric.
	reResource = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])$`)
	// Deployment names additionally allow '.' and '_' (e.g. "gpt-5.4").
	reDeployment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	// Model names / IDs, including fine-tune IDs such as
	// "gpt-4o-2024-08-06.ft-0123abcd" and catalog IDs with ':'.
	reModel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	// Model versions are dates ("2026-03-05") or small integers ("1", "0613").
	reModelVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,31}$`)
	// GitHub Actions secret names.
	reSecret = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,254}$`)
	reGUID   = regexp.MustCompile(`^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$`)
	// A single GitHub Actions expression, e.g. ${{ vars.AZURE_TENANT_ID }}.
	reGHExpr     = regexp.MustCompile(`^\$\{\{\s*(vars|secrets)\.[A-Za-z_][A-Za-z0-9_]*\s*\}\}$`)
	reAPIVersion = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2}(-preview)?|v1|preview|latest)$`)
	reLabel      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	reHostname   = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	reEnv        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)
)

// Validate checks a normalized Spec. Call normalize first (Resolve does).
func (s Spec) Validate() error {
	var p Problems

	if s.Environment != "" && !reEnv.MatchString(s.Environment) {
		p.addf("environment %q: use 1-32 characters from [A-Za-z0-9._-]", s.Environment)
	}

	switch {
	case s.Endpoint == "" && s.Resource == "":
		p.addf("one of resource or endpoint is required")
	case s.Endpoint == "" && !reResource.MatchString(s.Resource):
		p.addf("resource %q: Azure resource names are 2-64 characters of [a-z0-9-] and must start and end with a letter or digit", s.Resource)
	case s.Resource != "" && !reResource.MatchString(s.Resource):
		// Resource is only a label when Endpoint is set, but keep it sane
		// because it is used in file paths and key names.
		p.addf("resource %q: use 2-64 characters of [a-z0-9-]", s.Resource)
	}

	if s.Endpoint != "" {
		if err := validateEndpoint(s.Endpoint); err != nil {
			p.addf("endpoint %q: %v", s.Endpoint, err)
		}
	}

	switch s.HostKind {
	case HostOpenAI, HostCognitive, HostServicesAI:
		if s.Endpoint != "" {
			p.addf("host_kind %q conflicts with endpoint; omit host_kind or use %q", s.HostKind, HostCustom)
		}
	case HostCustom:
		if s.Endpoint == "" {
			p.addf("host_kind %q requires endpoint", HostCustom)
		}
	default:
		p.addf("host_kind %q: want one of openai, cognitiveservices, services-ai, custom", s.HostKind)
	}

	if !reModel.MatchString(s.Model) {
		p.addf("model %q: required; 1-128 characters of [A-Za-z0-9._:-]", s.Model)
	}
	if s.ModelVersion != "" && !reModelVersion.MatchString(s.ModelVersion) {
		p.addf("model_version %q: expected a version such as 2026-03-05", s.ModelVersion)
	}
	if s.ModelID != "" && !reModel.MatchString(s.ModelID) {
		p.addf("model_id %q: 1-128 characters of [A-Za-z0-9._:-]", s.ModelID)
	}
	if !reDeployment.MatchString(s.Deployment) {
		p.addf("deployment %q: required; 1-64 characters of [A-Za-z0-9._-] starting with a letter or digit", s.Deployment)
	}

	switch s.Surface {
	case SurfaceAuto, SurfaceOpenAIV1, SurfaceAnthropic:
	default:
		p.addf("surface %q: want one of auto, openai-v1, anthropic", s.Surface)
	}
	switch s.WireAPI {
	case WireAuto, WireResponses, WireCompletions:
	default:
		p.addf("wire_api %q: want one of auto, responses, completions", s.WireAPI)
	}

	switch s.Auth {
	case AuthAPIKey:
		if !reSecret.MatchString(s.APIKeySecret) {
			p.addf("api_key_secret %q: not a valid GitHub secret name ([A-Za-z_][A-Za-z0-9_]*)", s.APIKeySecret)
		} else if strings.HasPrefix(strings.ToUpper(s.APIKeySecret), "GITHUB_") {
			p.addf("api_key_secret %q: GitHub reserves the GITHUB_ prefix", s.APIKeySecret)
		}
	case AuthEntra:
		for _, f := range []struct{ name, val string }{
			{"tenant_id", s.TenantID}, {"client_id", s.ClientID},
		} {
			if !reGUID.MatchString(f.val) && !reGHExpr.MatchString(f.val) {
				p.addf("%s %q: auth=entra needs a GUID or a single ${{ vars.NAME }} expression", f.name, f.val)
			}
		}
		if !reHostname.MatchString(s.EntraAuthorityHost) {
			p.addf("entra_authority_host %q: expected a bare hostname", s.EntraAuthorityHost)
		}
	default:
		p.addf("auth %q: want api-key or entra", s.Auth)
	}

	if !reAPIVersion.MatchString(s.LegacyAPIVersion) {
		p.addf("legacy_api_version %q: expected YYYY-MM-DD or YYYY-MM-DD-preview", s.LegacyAPIVersion)
	}
	if !reAPIVersion.MatchString(s.InferenceAPIVersion) {
		p.addf("inference_api_version %q: expected YYYY-MM-DD or YYYY-MM-DD-preview", s.InferenceAPIVersion)
	}

	switch s.EnterpriseURLStyle {
	case EnterpriseURLV1, EnterpriseURLDeployment, EnterpriseURLRoot:
	default:
		p.addf("enterprise_url_style %q: want one of v1, deployment, root", s.EnterpriseURLStyle)
	}
	if n := len(s.EnterpriseKeyName); n > 100 {
		p.addf("enterprise_key_name: %d characters; keep it under 100 (it is shown in the model picker)", n)
	}

	for _, l := range RunsOnLabels(s.RunsOn) {
		if !reLabel.MatchString(l) {
			p.addf("runs_on label %q: use [A-Za-z0-9._-]", l)
		}
	}

	return p.orNil()
}

// RunsOnLabels splits a comma-separated runs-on value into trimmed labels.
func RunsOnLabels(v string) []string {
	var out []string
	for _, l := range strings.Split(v, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func validateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("must be an absolute URL such as https://host")
	}
	if u.User != nil {
		return fmt.Errorf("must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must not contain a query string or fragment")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopback(u.Hostname()) {
			return fmt.Errorf("plain http is only allowed for loopback hosts")
		}
	default:
		return fmt.Errorf("scheme must be https")
	}
	return nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
