package stream

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTokensArriveOneByOne(t *testing.T) {
	tokens := make(chan string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Tokens(w, r, "test", tokens)
	}))
	defer srv.Close()

	// The request must return before any token exists: headers are flushed first.
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	next := func() string {
		for lines.Scan() {
			if l := lines.Text(); l != "" {
				return strings.TrimPrefix(l, "data: ")
			}
		}
		return ""
	}
	// Each token must be readable before the next one is sent: the channel is
	// unbuffered, so this only passes if the handler flushes per token.
	for _, want := range []string{"Hello", ",", " world"} {
		select {
		case tokens <- want:
		case <-time.After(2 * time.Second):
			t.Fatal("handler is not consuming tokens")
		}
		var c Chunk
		if err := json.Unmarshal([]byte(next()), &c); err != nil || c.Delta != want {
			t.Fatalf("got %+v (%v), want delta %q", c, err, want)
		}
	}
	close(tokens)
	if last := next(); last != "[DONE]" {
		t.Fatalf("stream ended with %q", last)
	}
}
