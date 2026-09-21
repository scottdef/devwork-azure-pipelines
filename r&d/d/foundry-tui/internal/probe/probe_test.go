package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	foundrytui "github.com/CoolGitOrg/foundry-tui"
	"github.com/CoolGitOrg/foundry-tui/internal/azure/azuretest"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/dispatch"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

func server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/openai/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Model string }
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		switch {
		case r.Header.Get("Authorization") != "Bearer tok":
			http.Error(w, `{"error":{"code":"401","message":"PermissionDenied"}}`, 401)
		case body.Model == "missing":
			http.Error(w, `{"error":{"code":"DeploymentNotFound"}}`, 404)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"FOUNDRY_OK"}}],"usage":{"prompt_tokens":17,"completion_tokens":4}}`)
		}
	})
	mux.HandleFunc("/anthropic/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-version") == "" {
			http.Error(w, "missing version header", 400)
			return
		}
		fmt.Fprint(w, `{"content":[{"type":"text","text":"FOUNDRY_OK"}],"usage":{"input_tokens":20,"output_tokens":5}}`)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestAPI(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	openai := Target{Deployment: "gpt-4o", Format: "OpenAI", Chat: true, OpenAIBase: s.URL}

	if r := API(ctx, s.Client(), openai, "tok"); !r.OK || r.TokensIn != 17 || r.TokensOut != 4 {
		t.Errorf("openai: %+v", r)
	}
	claude := Target{Deployment: "claude-sonnet", Format: "Anthropic", Chat: true, AnthropicBase: s.URL + "/anthropic"}
	if r := API(ctx, s.Client(), claude, "tok"); !r.OK || r.TokensOut != 5 {
		t.Errorf("anthropic: %+v", r)
	}
	if r := API(ctx, s.Client(), openai, "bad"); r.OK || r.Status != 401 || !strings.Contains(r.Detail, "PermissionDenied") {
		t.Errorf("401: %+v", r)
	}
	openai.Deployment = "missing"
	if r := API(ctx, s.Client(), openai, "tok"); r.OK || r.Status != 404 {
		t.Errorf("404: %+v", r)
	}
	embed := Target{Deployment: "embed-small", Chat: false}
	if r := API(ctx, s.Client(), embed, "tok"); r.OK || !strings.HasPrefix(r.Detail, "skipped") {
		t.Errorf("embeddings deployment: %+v", r)
	}
}

func TestCopilot(t *testing.T) {
	tfs, _ := dispatch.TemplateFS(foundrytui.Templates, "")
	fake := azuretest.Fake()
	cfg := config.Default().Copilot
	tgt := Target{Deployment: "gpt-4o", Format: "OpenAI", Chat: true, OpenAIBase: "https://x.openai.azure.com"}

	r := Copilot(context.Background(), fake, cfg, tfs, tgt, "SECRET-TOKEN")
	if !r.OK {
		t.Fatalf("copilot probe: %+v", r)
	}
	call := fake.Calls()[0]
	if call.Args[0] != "-p" || !strings.Contains(call.Args[1], Marker) || call.Args[2] != "-s" {
		t.Errorf("args = %q", call.Args)
	}
	if strings.Contains(call.String(), "SECRET-TOKEN") {
		t.Error("the token leaked into the printable command line")
	}
	env := strings.Join(call.Env, "\n")
	for _, want := range []string{
		"COPILOT_PROVIDER_TYPE=openai", "COPILOT_PROVIDER_BASE_URL=https://x.openai.azure.com/openai/v1",
		"COPILOT_PROVIDER_BEARER_TOKEN=SECRET-TOKEN", "COPILOT_MODEL=gpt-4o", "COPILOT_OFFLINE=true",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("env lacks %s", want)
		}
	}
	if call.Dir == "" {
		t.Error("copilot must run in an empty scratch directory")
	}

	// Anthropic deployments switch provider type whatever the config says.
	claude := Target{Deployment: "claude-sonnet", Format: "Anthropic", Chat: true, AnthropicBase: "https://x.services.ai.azure.com/anthropic"}
	env = strings.Join(CopilotEnv(cfg, claude, "t"), "\n")
	if !strings.Contains(env, "COPILOT_PROVIDER_TYPE=anthropic") || strings.Contains(env, "WIRE_API") {
		t.Errorf("anthropic env = %s", env)
	}

	// Exit 0 with no marker is a failure, not a pass.
	silent := &run.Fake{Replies: map[string]run.Reply{"copilot": {Out: "Requests 1\n"}}}
	if r := Copilot(context.Background(), silent, cfg, tfs, tgt, "t"); r.OK {
		t.Error("a reply without the marker passed")
	}
}

func TestHistory(t *testing.T) {
	h := NewHistory(t.TempDir())
	for _, d := range []string{"a", "b", "c"} {
		if err := h.Append(Result{Deployment: d, Kind: "api", OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := h.Recent(2)
	if err != nil || len(got) != 2 || got[0].Deployment != "c" {
		t.Errorf("Recent = %+v, %v", got, err)
	}
}
