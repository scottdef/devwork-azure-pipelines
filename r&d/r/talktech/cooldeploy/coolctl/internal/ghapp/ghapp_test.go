package ghapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/gitops"
)

// fakeGitHub implements just enough of the REST API: one installation token
// endpoint and a contents API over an in-memory map.
type fakeGitHub struct {
	mu     sync.Mutex
	files  map[string]string // path -> content
	tokens int
	log    []string // commit messages
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasSuffix(r.URL.Path, "/access_tokens") {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
			http.Error(w, "want an App JWT", http.StatusUnauthorized)
			return
		}
		f.tokens++
		json.NewEncoder(w).Encode(map[string]any{"token": "ghs_test", "expires_at": time.Now().Add(time.Hour)})
		return
	}
	if r.Header.Get("Authorization") != "Bearer ghs_test" {
		http.Error(w, "bad token", http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/repos/CoolEngOrg/cool-gitops/contents/")
	sha := func(c string) string { return "sha-" + base64.RawURLEncoding.EncodeToString([]byte(c)) }
	var body struct{ Message, Content, SHA, Branch string }
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	cur, exists := f.files[path]
	switch r.Method {
	case http.MethodGet:
		if exists {
			json.NewEncoder(w).Encode(map[string]string{
				"type": "file", "encoding": "base64", "sha": sha(cur),
				"content": base64.StdEncoding.EncodeToString([]byte(cur)) + "\n",
			})
			return
		}
		var entries []map[string]string
		seen := map[string]bool{}
		for p := range f.files {
			rest, ok := strings.CutPrefix(p, path+"/")
			if !ok {
				continue
			}
			name, _, deeper := strings.Cut(rest, "/")
			if seen[name] {
				continue
			}
			seen[name] = true
			typ := "file"
			if deeper {
				typ = "dir"
			}
			entries = append(entries, map[string]string{"name": name, "type": typ})
		}
		if entries == nil {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(entries)
	case http.MethodPut:
		if exists && body.SHA != sha(cur) || !exists && body.SHA != "" {
			http.Error(w, `{"message":"sha does not match"}`, http.StatusConflict)
			return
		}
		b, _ := base64.StdEncoding.DecodeString(body.Content)
		f.files[path] = string(b)
		f.log = append(f.log, body.Message)
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if !exists {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		if body.SHA != sha(cur) {
			http.Error(w, `{"message":"sha does not match"}`, http.StatusConflict)
			return
		}
		delete(f.files, path)
		f.log = append(f.log, body.Message)
	}
}

func newStore(t *testing.T) (*Store, *fakeGitHub) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trip the key through PEM, as production reads it from a file.
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	parsed, err := ParseKey(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	gh := &fakeGitHub{files: map[string]string{}}
	srv := httptest.NewServer(gh)
	t.Cleanup(srv.Close)
	app := &App{ClientID: "Iv1.test", Installation: 42, Key: parsed, API: srv.URL}
	return &Store{App: app, Owner: "CoolEngOrg", Repo: "cool-gitops", Branch: "main"}, gh
}

func TestStoreRoundTrip(t *testing.T) {
	s, gh := newStore(t)
	ctx := context.Background()

	if _, _, err := s.Read(ctx, "apps/payments/checkout/app.json"); !errors.Is(err, gitops.ErrNotFound) {
		t.Fatalf("read of a missing file: %v", err)
	}
	if err := s.Write(ctx, "apps/payments/checkout/app.json", []byte("{}\n"), "", "register"); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, "apps/payments/checkout/app.json", []byte("{}\n"), "", "again"); !errors.Is(err, gitops.ErrConflict) {
		t.Fatalf("create over an existing file: %v", err)
	}
	b, rev, err := s.Read(ctx, "apps/payments/checkout/app.json")
	if err != nil || string(b) != "{}\n" {
		t.Fatalf("read back %q, %v", b, err)
	}
	if err := s.Write(ctx, "apps/payments/checkout/dev/values.json", []byte("{}"), "", "deploy"); err != nil {
		t.Fatal(err)
	}
	names, err := s.List(ctx, "apps/payments/checkout")
	if err != nil || strings.Join(names, " ") != "app.json dev/" {
		t.Fatalf("List = %v, %v", names, err)
	}
	if err := s.Delete(ctx, "apps/payments/checkout/app.json", "stale", "retire"); !errors.Is(err, gitops.ErrConflict) {
		t.Fatalf("delete with a stale rev: %v", err)
	}
	if err := s.Delete(ctx, "apps/payments/checkout/app.json", rev, "retire"); err != nil {
		t.Fatal(err)
	}
	if gh.tokens != 1 {
		t.Errorf("minted %d installation tokens, want 1: the token should be cached", gh.tokens)
	}
	if got := strings.Join(gh.log, ","); got != "register,deploy,retire" {
		t.Errorf("commits = %s", got)
	}
}

func TestParseKeyErrors(t *testing.T) {
	if _, err := ParseKey([]byte("not pem")); err == nil {
		t.Error("ParseKey accepted garbage")
	}
}
