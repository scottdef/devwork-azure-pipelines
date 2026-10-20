package ghapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParseKeyBothForms(t *testing.T) {
	k := testKey(t)
	p1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	p8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	for name, p := range map[string][]byte{"pkcs1": p1, "pkcs8": p8} {
		got, err := ParseKey(p)
		if err != nil || !got.Equal(k) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := ParseKey([]byte("junk")); err == nil {
		t.Error("junk parsed")
	}
}

func TestJWTVerifies(t *testing.T) {
	k := testKey(t)
	now := time.Unix(1_800_000_000, 0)
	tok, err := JWT("Iv23liClientID", k, now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Verify(tok, &k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer != "Iv23liClientID" || c.IssuedAt != now.Unix()-60 || c.ExpiresAt != now.Unix()+540 {
		t.Errorf("claims %+v", c)
	}
	// Tamper with the payload: verification must fail.
	parts := strings.Split(tok, ".")
	bad := parts[0] + "." + b64.EncodeToString([]byte(`{"iss":"evil"}`)) + "." + parts[2]
	if _, err := Verify(bad, &k.PublicKey); err == nil {
		t.Error("tampered token verified")
	}
	other := testKey(t)
	if _, err := Verify(tok, &other.PublicKey); err == nil {
		t.Error("verified with wrong key")
	}
}

func TestMinter(t *testing.T) {
	k := testKey(t)
	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwt := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if _, err := Verify(jwt, &k.PublicKey); err != nil {
			http.Error(w, err.Error(), 401)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/CoolGitOrg/tmplwatch-kv/installation":
			fmt.Fprint(w, `{"id":42}`)
		case r.Method == "POST" && r.URL.Path == "/app/installations/42/access_tokens":
			var body struct {
				Repositories []string
				Permissions  map[string]string
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Repositories) != 1 || body.Permissions["contents"] != "read" {
				http.Error(w, "bad scope", 422)
				return
			}
			n := mints.Add(1)
			fmt.Fprintf(w, `{"token":"ghs_%d","expires_at":%q}`, n, time.Now().Add(time.Hour).Format(time.RFC3339))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	m := &Minter{API: srv.URL, ClientID: "Iv1", Key: k, Owner: "CoolGitOrg", Repo: "tmplwatch-kv"}
	for range 3 {
		tok, err := m.Token(context.Background())
		if err != nil || tok != "ghs_1" {
			t.Fatalf("token %q %v", tok, err)
		}
	}
	// Advance the clock past refresh: a new token is minted.
	m.Now = func() time.Time { return time.Now().Add(56 * time.Minute) }
	if tok, _ := m.Token(context.Background()); tok != "ghs_2" {
		t.Errorf("refresh: got %q", tok)
	}
}
