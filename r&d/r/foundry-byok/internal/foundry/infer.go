package foundry

import (
	"regexp"
	"strings"
)

// oSeries matches OpenAI reasoning models: o1, o3, o4-mini, o3-pro, ...
var oSeries = regexp.MustCompile(`^o[0-9]+($|[-.])`)

// InferWireAPI returns the Copilot wire API for an OpenAI-compatible model.
//
// The gh-aw Azure BYOK reference states the Copilot CLI defaults custom
// providers to "completions" and that GPT-5 and o-series deployments require
// "responses". Codex-tuned models are Responses-only as well. Everything else
// (gpt-4o, gpt-4.1, gpt-oss, Mistral, Llama, Grok, DeepSeek, ...) stays on the
// chat-completions wire.
func InferWireAPI(model string) WireAPI {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-5"):
		return WireResponses
	case oSeries.MatchString(m):
		return WireResponses
	case strings.Contains(m, "codex"):
		return WireResponses
	default:
		return WireCompletions
	}
}

// InferSurface returns the API surface for a model name. Claude models on
// Foundry are served by the Anthropic Messages API; every other Foundry model
// is reachable on the OpenAI-compatible v1 endpoint.
func InferSurface(model string) APISurface {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(m, "claude") {
		return SurfaceAnthropic
	}
	return SurfaceOpenAIV1
}

// ComposeModelID returns the Azure model ID for a catalog name and version.
// Azure lists versioned models as "{name}-{version}" (for example
// "gpt-5.4-2026-03-05"). A name that already carries the version suffix is
// returned unchanged so callers can pass either form.
func ComposeModelID(model, version string) string {
	model = strings.TrimSpace(model)
	version = strings.TrimSpace(version)
	if version == "" || strings.HasSuffix(model, "-"+version) {
		return model
	}
	return model + "-" + version
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug lower-cases s and collapses every run of characters outside [a-z0-9]
// into a single hyphen. The result is safe for file names, GitHub Actions
// workflow file names and gh-aw workflow identifiers.
func Slug(parts ...string) string {
	var kept []string
	for _, p := range parts {
		p = slugUnsafe.ReplaceAllString(strings.ToLower(p), "-")
		p = strings.Trim(p, "-")
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "-")
}
