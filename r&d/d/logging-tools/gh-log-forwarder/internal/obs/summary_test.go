package obs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/forward"
)

func TestSummary(t *testing.T) {
	s := &Summary{Command: "run", Version: "v1.0.0"}
	s.Add(forward.RepoSummary{Repo: "CoolGitOrg/a", Jobs: 2, Records: 10})
	s.Add(forward.RepoSummary{Repo: "CoolGitOrg/b", Jobs: 1, Records: 3, StaleDropped: 1, Error: "boom | pipe\nnewline"})
	if s.Totals.Jobs != 3 || s.Totals.Records != 13 || s.Errors != 1 {
		t.Fatalf("totals %+v errors %d", s.Totals, s.Errors)
	}

	var js bytes.Buffer
	if err := s.WriteJSON(&js); err != nil || !json.Valid(js.Bytes()) {
		t.Fatalf("json %q %v", js.String(), err)
	}

	var md bytes.Buffer
	s.WriteMarkdown(&md)
	if !strings.Contains(md.String(), `boom \| pipe newline`) || strings.Count(md.String(), "\n| `") != 2 {
		t.Fatalf("markdown:\n%s", md.String())
	}

	rec := s.SelfRecord(time.Unix(0, 0), map[string]string{"dt.security_context": "ci"})
	var m map[string]any
	if err := json.Unmarshal(rec, &m); err != nil {
		t.Fatal(err)
	}
	if m["log.source"] != "ghlog2dt.self" || m["severity"] != "ERROR" || m["ghlog2dt.records"] != float64(13) || m["dt.security_context"] != "ci" {
		t.Fatalf("self record %v", m)
	}
	s.Errors = 0
	m = nil
	json.Unmarshal(s.SelfRecord(time.Unix(0, 0), nil), &m)
	if m["severity"] != "WARN" {
		t.Fatalf("stale drops should warn: %v", m["severity"])
	}
}
