package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestText(t *testing.T) {
	got := text(`<html><style>p{}</style><script>x()</script><p>Hello &amp; <b>world</b></p></html>`, 100)
	if got != "Hello & world" {
		t.Fatalf("got %q", got)
	}
	if got := text("abcdef", 3); got != "abc [cut]" {
		t.Fatalf("got %q", got)
	}
}

// mock model: asks for a search on its first call, answers on its second.
func mockLLM(t *testing.T, calls *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		var req struct {
			Messages []message
			Tools    json.RawMessage
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("key not sent")
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "tool" {
			if !strings.Contains(last.Content, "Example Domain") {
				t.Errorf("tool result missing from conversation: %q", last.Content)
			}
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":" It is example.com. "}}]}`)
			return
		}
		if len(req.Tools) == 0 {
			t.Errorf("tools not offered on the first call")
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[
		  {"id":"c1","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"example\"}"}}]}}]}`)
	}))
}

func TestRunSearchThenAnswer(t *testing.T) {
	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" || r.URL.Query().Get("q") != "example" {
			t.Errorf("unexpected search query %q", r.URL.RawQuery)
		}
		io.WriteString(w, `{"results":[{"title":"Example Domain","url":"https://example.com","content":"For <b>use</b> in documents."}]}`)
	}))
	defer searx.Close()
	calls := 0
	llm := mockLLM(t, &calls)
	defer llm.Close()

	a := &Agent{LLM: llm.URL, Key: "k", Model: "brain", Searx: searx.URL, Steps: 4, Budget: 1000,
		PageChars: 500, MaxTokens: 100, Client: http.DefaultClient, Web: webClient(false), Log: io.Discard}
	got, err := a.Run(context.Background(), "sys", "what is the example domain?")
	if err != nil || got != "It is example.com." || calls != 2 {
		t.Fatalf("got %q, err %v, %d model calls", got, err, calls)
	}
}

func TestFetchRefusesPrivateAddresses(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<p>internal</p>")
	}))
	defer page.Close()
	a := &Agent{PageChars: 100, Web: webClient(false)}
	if _, err := a.fetch(context.Background(), page.URL); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("loopback fetch was not refused: %v", err)
	}
	a.Web = webClient(true)
	if got, err := a.fetch(context.Background(), page.URL); err != nil || got != "internal" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if _, err := a.fetch(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("file URL was not refused")
	}
}

func TestBudgetWithdrawsTools(t *testing.T) {
	offered := []bool{}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Tools json.RawMessage }
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &req)
		offered = append(offered, len(req.Tools) > 0)
		if len(req.Tools) > 0 {
			io.WriteString(w, `{"choices":[{"message":{"content":"","tool_calls":[
			  {"id":"c","type":"function","function":{"name":"nope","arguments":"{}"}}]}}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer llm.Close()
	a := &Agent{LLM: llm.URL, Model: "m", Steps: 5, Budget: 10, MaxTokens: 10,
		Client: http.DefaultClient, Web: webClient(false), Log: io.Discard}
	got, err := a.Run(context.Background(), "s", "q")
	// The unknown tool returns an error string longer than the 10-character
	// budget, so the second call must come without tools.
	if err != nil || got != "done" || len(offered) != 2 || !offered[0] || offered[1] {
		t.Fatalf("got %q, err %v, offered %v", got, err, offered)
	}
}
