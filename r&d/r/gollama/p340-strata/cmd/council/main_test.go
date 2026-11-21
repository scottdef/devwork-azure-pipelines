package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractVote(t *testing.T) {
	if got := extract("thinking...\nanswer: 41\nwait.\nANSWER: 42.\n"); got != "42." {
		t.Fatalf("extract: %q", got)
	}
	if got := extract("no final line"); got != "" {
		t.Fatalf("extract without marker: %q", got)
	}
	best, n := vote([]string{"Paris.", "paris", "", "Lyon"})
	if best != "Paris." || n != 2 {
		t.Fatalf("vote: %q %d", best, n)
	}
}

// server answers by model name: seats named "a" say 7, "b" says 9, "c" errors.
func server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m struct {
			Model    string
			Messages []struct{ Content string }
		}
		json.NewDecoder(r.Body).Decode(&m)
		reply := "ANSWER: 7"
		switch {
		case m.Model == "c":
			http.Error(w, "down", 503)
			return
		case m.Model == "b":
			reply = "I think\nANSWER: 9"
		case m.Model == "judge":
			if !strings.Contains(m.Messages[0].Content, "Draft 2") {
				http.Error(w, "judge got no drafts", 400)
				return
			}
			reply = "ANSWER: 9"
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
}

func TestRun(t *testing.T) {
	s := server()
	defer s.Close()
	c := http.DefaultClient

	ans, ok, err := run(c, s.URL, "", []string{"a", "a", "b"}, "", "q", 0.8, 64, io.Discard)
	if err != nil || !ok || ans != "7" {
		t.Fatalf("majority: %q %v %v", ans, ok, err)
	}
	// One seat down: 1 of 3 each is not a majority, and there is no judge.
	if _, ok, _ := run(c, s.URL, "", []string{"a", "b", "c"}, "", "q", 0.8, 64, io.Discard); ok {
		t.Fatal("split vote reported as decided")
	}
	ans, ok, err = run(c, s.URL, "", []string{"a", "b", "c"}, "judge", "q", 0.8, 64, io.Discard)
	if err != nil || !ok || ans != "9" {
		t.Fatalf("judge: %q %v %v", ans, ok, err)
	}
	if _, _, err := run(c, s.URL, "", []string{"c", "c"}, "", "q", 0.8, 64, io.Discard); err == nil {
		t.Fatal("all seats down should be an error")
	}
}
