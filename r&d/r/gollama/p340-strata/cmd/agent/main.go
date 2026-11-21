// agent runs one question through a tool loop against an OpenAI-compatible
// chat endpoint: gate, Strata, llama-server or Unsloth Studio. The model may
// call two tools, web_search (a SearXNG instance) and fetch_url, until it
// answers or the step or size budget runs out. The tools run here, in the
// client, so the same loop behaves the same on every engine.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// toolSpec is the schema the model sees. Keep descriptions short: on an 8K
// context every token of schema is a token the conversation cannot use.
const toolSpec = `[
 {"type":"function","function":{"name":"web_search",
  "description":"Search the web. Returns the top results with title, URL and a snippet.",
  "parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}},
 {"type":"function","function":{"name":"fetch_url",
  "description":"Fetch one web page and return its text.",
  "parameters":{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}}}
]`

// Agent holds the endpoints and limits of one run.
type Agent struct {
	LLM, Key, Model string
	Searx           string
	Steps           int // model calls allowed
	Budget          int // characters of tool output allowed in total
	PageChars       int // characters kept from one fetched page
	MaxTokens       int
	AllowPrivate    bool // let fetch_url reach loopback and private addresses
	Client          *http.Client
	Web             *http.Client
	Log             io.Writer
}

var (
	reDrop = regexp.MustCompile(`(?is)<(script|style|noscript|svg)\b.*?</(script|style|noscript|svg)>`)
	reTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	reWS   = regexp.MustCompile(`\s+`)
)

// text reduces an HTML page to plain text and cuts it to max characters.
func text(page string, max int) string {
	s := reDrop.ReplaceAllString(page, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = strings.TrimSpace(reWS.ReplaceAllString(html.UnescapeString(s), " "))
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + " [cut]"
	}
	return s
}

// guard is a dial hook that refuses addresses a fetched page has no business
// reaching. It sees the address actually dialled, so a redirect or a DNS
// answer that points inward is refused too.
func guard(allow bool) func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if allow {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("refused: %s is not a public address", host)
		}
		return nil
	}
}

func webClient(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: guard(allowPrivate)}
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: d.DialContext}}
}

func (a *Agent) search(ctx context.Context, q string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		strings.TrimRight(a.Searx, "/")+"/search?format=json&q="+url.QueryEscape(q), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	resp, err := a.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search answered %d (403 means settings.yml lacks formats: [html, json])", resp.StatusCode)
	}
	var out struct {
		Results []struct{ Title, URL, Content string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, r := range out.Results {
		if i == 5 {
			break
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, text(r.Content, 300))
	}
	if b.Len() == 0 {
		return "no results", nil
	}
	return b.String(), nil
}

func (a *Agent) fetch(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("refused: only http and https URLs")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	resp, err := a.Web.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("page answered %d", resp.StatusCode)
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return text(string(page), a.PageChars), nil
}

// call runs one tool call and returns what the model should read. Errors are
// returned as text: the model can often recover from "page answered 404".
func (a *Agent) call(ctx context.Context, tc toolCall) string {
	var arg struct{ Query, URL string }
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &arg); err != nil {
		return "error: arguments were not valid JSON"
	}
	var out string
	var err error
	switch tc.Function.Name {
	case "web_search":
		out, err = a.search(ctx, arg.Query)
	case "fetch_url":
		out, err = a.fetch(ctx, arg.URL)
	default:
		err = fmt.Errorf("no tool named %q", tc.Function.Name)
	}
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

func (a *Agent) chat(ctx context.Context, msgs []message, tools bool) (message, error) {
	body := map[string]any{"model": a.Model, "messages": msgs, "stream": false,
		"temperature": 0.2, "max_tokens": a.MaxTokens}
	if tools {
		body["tools"] = json.RawMessage(toolSpec)
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(a.LLM, "/")+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return message{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.Key != "" {
		req.Header.Set("Authorization", "Bearer "+a.Key)
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return message{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != 200 {
		return message{}, fmt.Errorf("model answered %d: %.200s", resp.StatusCode, raw)
	}
	var out struct {
		Choices []struct{ Message message }
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return message{}, fmt.Errorf("unreadable reply: %.200s", raw)
	}
	return out.Choices[0].Message, nil
}

// Run answers one question. Tools are offered until the step or size budget
// is spent; the last call offers none, so the model has to answer.
func (a *Agent) Run(ctx context.Context, system, question string) (string, error) {
	msgs := []message{{Role: "system", Content: system}, {Role: "user", Content: question}}
	spent := 0
	for step := 1; step <= a.Steps; step++ {
		tools := step < a.Steps && spent < a.Budget
		m, err := a.chat(ctx, msgs, tools)
		if err != nil {
			return "", err
		}
		if len(m.ToolCalls) == 0 || !tools {
			return strings.TrimSpace(m.Content), nil
		}
		m.Role = "assistant"
		msgs = append(msgs, m)
		for _, tc := range m.ToolCalls {
			out := a.call(ctx, tc)
			spent += len(out)
			fmt.Fprintf(a.Log, "step %d: %s %s -> %d chars\n", step, tc.Function.Name, tc.Function.Arguments, len(out))
			msgs = append(msgs, message{Role: "tool", ToolCallID: tc.ID, Content: out})
		}
	}
	return "", errors.New("no answer within the step budget")
}

const defaultSystem = "You answer questions using the tools when facts may have changed or you are unsure. " +
	"Search first, fetch at most two pages, then answer in plain text and name the URLs you relied on."

func main() {
	a := &Agent{Log: os.Stderr}
	flag.StringVar(&a.LLM, "url", "http://127.0.0.1:4000", "chat endpoint (gate, Strata, llama-server or Studio)")
	flag.StringVar(&a.Model, "model", "brain", "model or alias to ask")
	flag.StringVar(&a.Searx, "searx", "http://127.0.0.1:8889", "SearXNG address")
	flag.IntVar(&a.Steps, "steps", 6, "model calls allowed, the answer included")
	flag.IntVar(&a.Budget, "budget", 16000, "characters of tool output allowed in total (4 characters is about 1 token)")
	flag.IntVar(&a.PageChars, "page", 6000, "characters kept from one fetched page")
	flag.IntVar(&a.MaxTokens, "max", 700, "tokens per model reply")
	flag.BoolVar(&a.AllowPrivate, "allow-private", false, "let fetch_url reach loopback and private addresses")
	keyEnv := flag.String("key-env", "GATE_KEY", "environment variable holding the endpoint key")
	system := flag.String("system", defaultSystem, "system prompt")
	timeout := flag.Duration("timeout", 30*time.Minute, "limit for the whole run")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: agent [flags] question")
		os.Exit(2)
	}
	a.Key = os.Getenv(*keyEnv)
	a.Client = &http.Client{}
	a.Web = webClient(a.AllowPrivate)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	out, err := a.Run(ctx, *system, strings.Join(flag.Args(), " "))
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		os.Exit(1)
	}
	fmt.Println(out)
}
