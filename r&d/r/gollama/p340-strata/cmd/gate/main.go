// gate is a small front door for OpenAI- and Anthropic-style model servers.
// It maps a model alias to a list of upstreams, rewrites the model name each
// upstream expects, and falls through to the next upstream when one is down
// or answers 5xx. Clients keep one address and one alias; the engine behind
// the alias can change (llama-server today, Strata tomorrow) without them.
package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// Upstream is one server that can answer for an alias.
type Upstream struct {
	URL    string `json:"url"`     // base address, e.g. http://127.0.0.1:8080
	Model  string `json:"model"`   // name that server expects; empty keeps the alias
	KeyEnv string `json:"key_env"` // environment variable holding that server's key
}

// Gate routes requests by the "model" field of their JSON body.
type Gate struct {
	Routes map[string][]Upstream
	Key    string
	Client *http.Client
}

func (g *Gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		io.WriteString(w, "ok\n")
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		http.NotFound(w, r)
		return
	}
	if g.Key != "" {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" {
			got = r.Header.Get("x-api-key") // Anthropic-style clients
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(g.Key)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		type m struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		}
		var data []m
		for a := range g.Routes {
			data = append(data, m{a, "model"})
		}
		sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		http.Error(w, "body too large or unreadable", http.StatusBadRequest)
		return
	}
	var msg map[string]json.RawMessage
	var alias string
	if json.Unmarshal(body, &msg) == nil {
		json.Unmarshal(msg["model"], &alias)
	}
	ups, ok := g.Routes[alias]
	if !ok {
		http.Error(w, "unknown model alias: "+alias, http.StatusNotFound)
		return
	}
	for _, u := range ups {
		b := body
		if u.Model != "" {
			msg["model"], _ = json.Marshal(u.Model)
			b, _ = json.Marshal(msg)
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method,
			strings.TrimRight(u.URL, "/")+r.URL.RequestURI(), bytes.NewReader(b))
		if err != nil {
			continue
		}
		for _, h := range []string{"Content-Type", "Accept", "Anthropic-Version", "Anthropic-Beta"} {
			if v := r.Header.Get(h); v != "" {
				req.Header.Set(h, v)
			}
		}
		if k := os.Getenv(u.KeyEnv); u.KeyEnv != "" && k != "" {
			req.Header.Set("Authorization", "Bearer "+k)
			req.Header.Set("x-api-key", k)
		}
		resp, err := g.Client.Do(req)
		if err != nil {
			log.Printf("alias %s: %s unreachable: %v", alias, u.URL, err)
			continue
		}
		if resp.StatusCode >= 500 {
			log.Printf("alias %s: %s answered %d, trying next", alias, u.URL, resp.StatusCode)
			resp.Body.Close()
			continue
		}
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("X-Gate-Upstream", u.URL)
		w.WriteHeader(resp.StatusCode)
		rc := http.NewResponseController(w)
		buf := make([]byte, 16<<10)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					break
				}
				rc.Flush() // token streams must not sit in a buffer
			}
			if rerr != nil {
				break
			}
		}
		resp.Body.Close()
		return
	}
	http.Error(w, "every upstream failed for alias "+alias, http.StatusBadGateway)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:4000", "address to serve on")
	routes := flag.String("routes", "etc/routes.json", "alias table")
	keyEnv := flag.String("key-env", "GATE_KEY", "environment variable holding the key clients must send; unset means no key")
	wait := flag.Duration("header-timeout", 20*time.Minute, "how long an upstream may read a prompt before its first byte")
	flag.Parse()

	raw, err := os.ReadFile(*routes)
	if err != nil {
		log.Fatal(err)
	}
	g := &Gate{Key: os.Getenv(*keyEnv), Client: &http.Client{Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		ResponseHeaderTimeout: *wait,
	}}}
	if err := json.Unmarshal(raw, &g.Routes); err != nil {
		log.Fatalf("%s: %v", *routes, err)
	}
	if g.Key == "" && !strings.HasPrefix(*listen, "127.0.0.1:") && !strings.HasPrefix(*listen, "localhost:") {
		log.Fatalf("refusing to listen on %s without a key in $%s", *listen, *keyEnv)
	}
	log.Printf("gate: %d aliases on %s", len(g.Routes), *listen)
	log.Fatal(http.ListenAndServe(*listen, g))
}
