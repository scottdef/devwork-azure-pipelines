// Package stream writes generated tokens to an HTTP client as server-sent
// events, in the shape OpenAI-compatible clients expect: one "data:" line of
// JSON per token, then "data: [DONE]".
package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Chunk is the payload of one event.
type Chunk struct {
	Model string `json:"model"`
	Delta string `json:"delta"`
	Done  bool   `json:"done"`
}

// Tokens streams from the channel until it closes or the client goes away.
// Each token is flushed as soon as it is written: latency to the first token
// is a number users feel, and a buffered token is a late one.
func Tokens(w http.ResponseWriter, r *http.Request, model string, tokens <-chan string) error {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // ask reverse proxies not to buffer
	// Send the headers now. Without this the client's request does not return
	// until the first token exists, and a slow prompt looks like a hung server.
	if err := rc.Flush(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return r.Context().Err() // the caller must stop generating
		case tok, ok := <-tokens:
			if !ok {
				fmt.Fprint(w, "data: [DONE]\n\n")
				return rc.Flush()
			}
			fmt.Fprint(w, "data: ")
			if err := enc.Encode(Chunk{Model: model, Delta: tok}); err != nil { // Encode ends the line
				return err
			}
			fmt.Fprint(w, "\n")
			if err := rc.Flush(); err != nil {
				return err
			}
		}
	}
}
