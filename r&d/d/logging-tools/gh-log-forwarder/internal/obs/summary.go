// Package obs reports what a ghlog2dt run did: a JSON summary for
// machines, a Markdown table for $GITHUB_STEP_SUMMARY, and one
// self-monitoring log record for Dynatrace.
package obs

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/dynatrace"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/forward"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/record"
)

// Summary is the result of one invocation.
type Summary struct {
	Tool            string                `json:"tool"`
	Version         string                `json:"version"`
	Command         string                `json:"command"`
	StartedAt       time.Time             `json:"started_at"`
	DurationMS      int64                 `json:"duration_ms"`
	DryRun          bool                  `json:"dry_run"`
	Repos           []forward.RepoSummary `json:"repos"`
	Totals          forward.RepoSummary   `json:"totals"`
	Dynatrace       dynatrace.Stats       `json:"dynatrace"`
	GitHubCalls     int                   `json:"github_calls"`
	GitHubRemaining int                   `json:"github_rate_remaining"`
	Errors          int                   `json:"errors"`
}

// Add folds one repository into the totals.
func (s *Summary) Add(r forward.RepoSummary) {
	s.Repos = append(s.Repos, r)
	t := &s.Totals
	t.Runs += r.Runs
	t.RunsPending += r.RunsPending
	t.PendingAbandoned += r.PendingAbandoned
	t.Jobs += r.Jobs
	t.JobsAlreadyShipped += r.JobsAlreadyShipped
	t.JobsNotReady += r.JobsNotReady
	t.LogBytes += r.LogBytes
	t.Lines += r.Lines
	t.Records += r.Records
	t.StaleDropped += r.StaleDropped
	t.StaleRewritten += r.StaleRewritten
	t.Redactions += r.Redactions
	t.Truncated += r.Truncated
	if r.Error != "" {
		s.Errors++
	}
}

// WriteJSON writes the summary as one JSON line.
func (s *Summary) WriteJSON(w io.Writer) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// WriteMarkdown renders a table for $GITHUB_STEP_SUMMARY.
func (s *Summary) WriteMarkdown(w io.Writer) error {
	var b strings.Builder
	mode := "live"
	if s.DryRun {
		mode = "dry-run"
	}
	fmt.Fprintf(&b, "### ghlog2dt %s (%s, %s)\n\n", s.Command, mode, time.Duration(s.DurationMS)*time.Millisecond)
	b.WriteString("| repo | runs | jobs | records | stale dropped | redactions | pending | error |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---|\n")
	for _, r := range s.Repos {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %d | %s |\n",
			r.Repo, r.Runs, r.Jobs, r.Records, r.StaleDropped, r.Redactions, r.RunsPending, mdEscape(r.Error))
	}
	t := s.Totals
	fmt.Fprintf(&b, "| **total** | %d | %d | %d | %d | %d | %d | %d errors |\n\n",
		t.Runs, t.Jobs, t.Records, t.StaleDropped, t.Redactions, t.RunsPending, s.Errors)
	d := s.Dynatrace
	fmt.Fprintf(&b, "Dynatrace: %d requests, %d records, %d bytes, %d partial, %d stale, %d rejected, %d retries. ",
		d.Requests, d.Records, d.Bytes, d.Partial, d.Stale, d.Rejected, d.Retries)
	fmt.Fprintf(&b, "GitHub: %d calls, %d remaining.\n", s.GitHubCalls, s.GitHubRemaining)
	_, err := io.WriteString(w, b.String())
	return err
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

// SelfRecord is the forwarder's own heartbeat: log.source=ghlog2dt.self.
// Alert when it stops arriving.
func (s *Summary) SelfRecord(now time.Time, attrs map[string]string) []byte {
	sev := "INFO"
	switch {
	case s.Errors > 0 || s.Dynatrace.Rejected > 0:
		sev = "ERROR"
	case s.Totals.StaleDropped > 0 || s.Dynatrace.Partial > 0 || s.Totals.PendingAbandoned > 0:
		sev = "WARN"
	}
	var b record.Builder
	b.Reset()
	b.Str("timestamp", now.UTC().Format(time.RFC3339Nano))
	b.Str("content", fmt.Sprintf("ghlog2dt %s: %d repos, %d jobs, %d records, %d stale dropped, %d errors",
		s.Command, len(s.Repos), s.Totals.Jobs, s.Totals.Records, s.Totals.StaleDropped, s.Errors))
	b.Str("severity", sev)
	b.Str("log.source", "ghlog2dt.self")
	if v := attrs["dt.security_context"]; v != "" {
		b.Str("dt.security_context", v)
	}
	b.Str("ghlog2dt.version", s.Version)
	b.Str("ghlog2dt.command", s.Command)
	b.Int("ghlog2dt.repos", int64(len(s.Repos)))
	b.Int("ghlog2dt.runs", int64(s.Totals.Runs))
	b.Int("ghlog2dt.jobs", int64(s.Totals.Jobs))
	b.Int("ghlog2dt.records", int64(s.Totals.Records))
	b.Int("ghlog2dt.bytes", s.Dynatrace.Bytes)
	b.Int("ghlog2dt.stale_dropped", int64(s.Totals.StaleDropped))
	b.Int("ghlog2dt.pending", int64(s.Totals.RunsPending))
	b.Int("ghlog2dt.pending_abandoned", int64(s.Totals.PendingAbandoned))
	b.Int("ghlog2dt.partial_batches", s.Dynatrace.Partial)
	b.Int("ghlog2dt.rejected", s.Dynatrace.Rejected)
	b.Int("ghlog2dt.retries", s.Dynatrace.Retries)
	b.Int("ghlog2dt.errors", int64(s.Errors))
	b.Int("ghlog2dt.gh_rate_remaining", int64(s.GitHubRemaining))
	b.Int("ghlog2dt.duration_ms", s.DurationMS)
	return b.Bytes()
}
