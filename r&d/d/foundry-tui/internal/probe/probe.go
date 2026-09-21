// Package probe asks a deployment to say one word, two ways:
// straight over HTTPS, and through the Copilot CLI acting as a BYOK
// client. The first proves the deployment answers. The second proves
// it answers the client your developers actually use.
//
// Both are keyless: a short-lived Entra token, never an account key.
package probe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

// Marker is the word a healthy deployment is asked to repeat.
const Marker = "FOUNDRY_OK"

// Target is what a probe needs to know about a deployment.
type Target struct {
	Deployment    string
	Format        string // model format: OpenAI, Anthropic, ...
	Chat          bool
	OpenAIBase    string // https://NAME.openai.azure.com
	AnthropicBase string // https://NAME.services.ai.azure.com/anthropic
}

// NewTarget builds a Target from what az reported.
func NewTarget(a azure.Account, d azure.Deployment) Target {
	return Target{
		Deployment: d.Name, Format: d.Properties.Model.Format, Chat: d.Chat(),
		OpenAIBase: a.OpenAIBase(), AnthropicBase: a.AnthropicBase(),
	}
}

func (t Target) anthropic() bool { return strings.EqualFold(t.Format, "Anthropic") }

// Result is one probe outcome. It is what gets logged and reported.
type Result struct {
	Time       time.Time `json:"time"`
	Deployment string    `json:"deployment"`
	Kind       string    `json:"kind"` // api | copilot
	OK         bool      `json:"ok"`
	Status     int       `json:"status,omitempty"`
	LatencyMS  int64     `json:"latency_ms"`
	TokensIn   int       `json:"tokens_in,omitempty"`
	TokensOut  int       `json:"tokens_out,omitempty"`
	Detail     string    `json:"detail"`
}

// API sends one chat request and times it.
func API(ctx context.Context, hc *http.Client, t Target, token string) Result {
	res := Result{Time: time.Now().UTC(), Deployment: t.Deployment, Kind: "api"}
	if !t.Chat {
		res.Detail = "skipped: deployment does not declare chat completion"
		return res
	}
	prompt := "Reply with exactly this single word and nothing else: " + Marker
	msgs := []map[string]string{{"role": "user", "content": prompt}}
	url, body := t.OpenAIBase+"/openai/v1/chat/completions", map[string]any{"model": t.Deployment, "messages": msgs}
	if t.anthropic() {
		url, body = t.AnthropicBase+"/v1/messages", map[string]any{"model": t.Deployment, "messages": msgs, "max_tokens": 64}
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if t.anthropic() {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	start := time.Now()
	resp, err := hc.Do(req)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	res.Status = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		res.Detail = fmt.Sprintf("%s: %s", resp.Status, oneLine(string(raw), 240))
		return res
	}
	text := parse(raw, &res)
	res.OK = strings.TrimSpace(text) != ""
	switch {
	case !res.OK:
		res.Detail = "200 OK but the reply was empty"
	case strings.Contains(text, Marker):
		res.Detail = "replied " + Marker
	default:
		res.Detail = "replied without the marker: " + oneLine(text, 120)
	}
	return res
}

// parse understands both wire formats and fills token counts.
func parse(raw []byte, res *Result) string {
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	res.TokensIn, res.TokensOut = r.Usage.PromptTokens+r.Usage.InputTokens, r.Usage.CompletionTokens+r.Usage.OutputTokens
	var sb strings.Builder
	for _, c := range r.Choices {
		sb.WriteString(c.Message.Content)
	}
	for _, c := range r.Content {
		sb.WriteString(c.Text)
	}
	return sb.String()
}

// CopilotEnv is the BYOK environment for one deployment. The token is
// the only secret and it lives only in the child's environment.
func CopilotEnv(cfg config.Copilot, t Target, token string) []string {
	typ, base := cfg.ProviderType, t.OpenAIBase+"/openai/v1"
	switch {
	case t.anthropic():
		typ, base = "anthropic", t.AnthropicBase
	case typ == "azure":
		base = t.OpenAIBase
	}
	env := []string{
		"COPILOT_PROVIDER_TYPE=" + typ,
		"COPILOT_PROVIDER_BASE_URL=" + base,
		"COPILOT_PROVIDER_BEARER_TOKEN=" + token,
		"COPILOT_MODEL=" + t.Deployment,
		"NO_COLOR=1",
	}
	if typ == "azure" {
		env = append(env, "COPILOT_PROVIDER_WIRE_MODEL="+t.Deployment)
	}
	if typ != "anthropic" && cfg.WireAPI != "" {
		env = append(env, "COPILOT_PROVIDER_WIRE_API="+cfg.WireAPI)
	}
	if cfg.Offline {
		env = append(env, "COPILOT_OFFLINE=true")
	}
	return env
}

// Copilot runs "copilot -p PROMPT" against the deployment in an empty
// directory, so there is no repository for the agent to read.
func Copilot(ctx context.Context, r run.Runner, cfg config.Copilot, tfs fs.FS, t Target, token string) Result {
	res := Result{Time: time.Now().UTC(), Deployment: t.Deployment, Kind: "copilot"}
	if !t.Chat {
		res.Detail = "skipped: deployment does not declare chat completion"
		return res
	}
	tmpl, err := template.ParseFS(tfs, "probe/copilot-prompt.txt.tmpl")
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	var prompt bytes.Buffer
	if err := tmpl.Execute(&prompt, map[string]string{"Deployment": t.Deployment, "Marker": Marker}); err != nil {
		res.Detail = err.Error()
		return res
	}
	dir, err := os.MkdirTemp("", "foundry-tui-copilot-")
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	defer os.RemoveAll(dir)

	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	args := append([]string{"-p", strings.TrimSpace(prompt.String())}, cfg.Args...)
	start := time.Now()
	out, err := r.Run(ctx, run.Cmd{Name: cfg.Bin, Args: args, Env: CopilotEnv(cfg, t, token), Dir: dir})
	res.LatencyMS = time.Since(start).Milliseconds()
	switch {
	case err != nil:
		res.Detail = oneLine(err.Error(), 240)
	case strings.Contains(string(out), Marker):
		res.OK, res.Detail = true, "replied "+Marker
	default:
		res.Detail = "exit 0 but no marker on stdout: " + oneLine(string(out), 120)
	}
	return res
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		s = s[:n] + "…"
	}
	return s
}

// History is probes.jsonl in the state directory: the program's own
// latency record, available to any role because the program made it.
type History struct{ Path string }

func NewHistory(dir string) History { return History{filepath.Join(dir, "probes.jsonl")} }

func (h History) Append(r Result) error {
	if err := os.MkdirAll(filepath.Dir(h.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(h.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(r)
	_, err = f.Write(append(b, '\n'))
	return err
}

// Recent returns up to n results, newest first.
func (h History) Recent(n int) ([]Result, error) {
	f, err := os.Open(h.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []Result
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Result
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			all = append(all, r)
		}
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if len(all) > n {
		all = all[:n]
	}
	return all, sc.Err()
}
