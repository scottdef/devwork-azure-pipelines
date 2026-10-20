package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/issueops-intake/internal/dispatch"
	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func opts() options {
	return options{repo: "CoolGitOrg/issueops-library", event: dispatch.EventPublish, issue: 42,
		source: "payments-api", requester: "alice", origin: "CoolGitOrg/issueops-intake",
		base: "https://coolgitorg.github.io/issueops-library/", timeout: time.Minute}
}

func TestDryRun(t *testing.T) {
	o := opts()
	o.dry = true
	var out bytes.Buffer
	if err := run(o, "../../testdata/golden/issueparse/request-basic.json", nil, &out, nil); err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "../../testdata/golden/dispatch/publish.dry.json", out.Bytes())
}

func TestSendReceipt(t *testing.T) {
	var keys int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		var b struct {
			ClientPayload map[string]any `json:"client_payload"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		keys = len(b.ClientPayload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := dispatch.New(srv.URL, "secret")
	var out bytes.Buffer
	if err := run(opts(), "../../testdata/golden/issueparse/request-basic.json", nil, &out, c); err != nil {
		t.Fatal(err)
	}
	if keys == 0 || keys > dispatch.MaxKeys {
		t.Errorf("server saw %d keys", keys)
	}
	golden.Check(t, "../../testdata/golden/dispatch/publish.receipt.json", out.Bytes())
}

func TestSourceMismatch(t *testing.T) {
	o := opts()
	o.source = "other"
	o.dry = true
	err := run(o, "../../testdata/golden/issueparse/request-basic.json", nil, &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("err = %v", err)
	}
}

func TestBorrowDry(t *testing.T) {
	o := opts()
	o.event, o.source, o.dry = dispatch.EventBorrow, "", true
	var out bytes.Buffer
	if err := run(o, "../../testdata/golden/issueparse/borrow-basic.json", nil, &out, nil); err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "../../testdata/golden/dispatch/borrow.dry.json", out.Bytes())
}
