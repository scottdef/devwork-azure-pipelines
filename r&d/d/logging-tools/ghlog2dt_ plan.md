# Plan: `ghlog2dt` — Exporting GitHub Actions Build Logs to Dynatrace via the Log Monitoring API v2

Build a small, stdlib-only Go binary (`ghlog2dt`) that runs from a central `CoolGitOrg/gh-log-forwarder` repository on a Terraform-rendered cron (default every 15 minutes). It pulls completed job logs through the GitHub REST API using the `issueops-autoadmin-app` installation token and POSTs them in batches to `https://abc12345.live.dynatrace.com/api/v2/logs/ingest`. It checkpoints by job ID in a state branch. An optional `workflow_run` → `repository_dispatch` push path cuts latency. The most important constraint is that Dynatrace drops log records more than 24 hours old at ingest, so the export interval and lookback window must stay well under a day. Historical backfill is not possible without rewriting timestamps.

## TL;DR

- **Use a central pull-based Go forwarder as the source of truth, with event-driven push as an optional accelerator.** Use `POST /api/v2/logs/ingest` with a Classic access token scoped to `logs.ingest` only. Platform tokens (`Bearer`, `openpipeline:logs:ingest`) also work on the same `.live` URL. Send JSON arrays capped at about 4 MiB and 5,000 records, well under the documented 10 MB / 50,000-record limits. Retry 429/502/503/504 with exponential backoff, and split the batch on 413.
- **Frequency is one Terraform variable (`interval_minutes`).** It renders the cron expression into the workflow file and sets the lookback window. GitHub's floor is 5 minutes, and scheduled runs can be delayed or dropped at busy times. Dynatrace's 24-hour log-age limit is the hard upper bound. Run every 10–30 minutes at off-peak minute offsets, and keep a "pending runs" set so long-running workflows are not missed.
- **Some details are still unverified and need a Phase-0 spike before rollout.** gzip `Content-Encoding` on this endpoint is undocumented. `Retry-After` is not documented by Dynatrace. Ingest is billed per the DPS metric `builtin:billing.log.ingest.usage`, defined as the "Number of raw bytes sent to Dynatrace before enrichment and transformation", at $0.20/GiB on Dynatrace's public rate card. Per-record attribute overhead, not log text, can therefore dominate cost. Keep attributes lean.

## Key Findings (verified against current docs, Oct 2026)

### Dynatrace Log Monitoring API v2 (endpoint page updated Sep 28, 2026)

| Item | Verified value |
|---|---|
| SaaS endpoint | `POST https://{env}.live.dynatrace.com/api/v2/logs/ingest` |
| ActiveGate endpoint | `POST https://{ag}:9999/e/{env}/api/v2/logs/ingest` (Log analytics collector module, enabled by default) |
| Classic auth | `Authorization: Api-Token <token>`, scope `logs.ingest` |
| Platform/OAuth auth | `Authorization: Bearer <token>`, scope `openpipeline:logs:ingest` (or legacy `storage:logs:write`)\[1\] |
| Payload types | `text/plain` (single event); `application/json` (array or object); `application/jsonl`, `application/x-ndjson`, etc. (JSON lines)\[1\] |
| Max request size | 10 MB; rejected with 413 if the batch exceeds 16 MB *after* preprocessing ("Message size limit exceeded after preprocessing on ingest endpoint") |
| Max records/request | 50,000 |
| Content per record | 10 MB on OpenPipeline; **512 kB if routed to the Classic pipeline** |
| Attributes | 500 per record; key 100 bytes; value 32 kB; 32 values per attribute; nesting 5 levels |
| Log age | **24 hours.** All-stale payload → `400 "All logs are out of correct time range."`; partially stale → `200` with a message; >10 min in the future → reset to ingest time |
| Events/min | No limit (Grail/LMA) |
| Latency | Per the Dynatrace "Log Management and Analytics default limits" page, "Logs ingested by API are available for analysis in Dynatrace after 10 seconds on average." |
| Responses | 204 success; 200 partial; 400; 402 (DPS exhausted); 404; 413; 429/502/503/504 "retryable with exponential backoff strategy"; 501\[1\] |
| Throttling | Per the Dynatrace API "Access limit" page, the environment has a bounded thread pool and queue. Requests get a 429 "when both thread pool and its queue are full or the request times out in the queue (timeout is 10 seconds)" |

Field mapping (raw data model is the default for Dynatrace 1.331+ tenants):
- `timestamp` comes from the first key found in `timestamp`, `@timestamp`, … Supported formats are RFC3339 and Unix epoch (seconds or ms; fractional seconds on 1.339+). No timezone means UTC.
- `loglevel` comes from `loglevel`, `status`, `severity`, `level`, `syslog.severity`. The default is `NONE`.
- `content` comes from `content`, `message`, `payload`, `body`, `log`.
- Every other key becomes an attribute. `dt.auth.origin` is added automatically.
- With OpenPipeline custom processing (SaaS 1.295+), all JSON value types are preserved and keys are case-sensitive. Routed to the Classic pipeline, keys are lowercased and values stringified.\[2\]
- Request-wide attributes can be passed with the `X-Dynatrace-Attr` header (ampersand-separated pairs) or as query parameters. These are merged into every record.\[1\]

Older "Logs Classic" pages list tighter limits (5 MB, 8 kB content, 50 attributes) for Classic tenants only.\[3\] The conservative defaults here work on either.

**Newer endpoints and whether they supersede v2:** `/api/v2/logs/ingest` remains a built-in OpenPipeline ingest source (`dt.openpipeline.source = /api/v2/logs/ingest`).\[4\] The alternatives are OTLP (`/api/v2/otlp/v1/logs`, protobuf only, which needs an encoder, so it is not stdlib-friendly) and `/platform/ingest/v1/...`, which exists for events, SDLC events and security events, not logs. Log Monitoring API v2 is not superseded and is the correct target.

### GitHub REST API

| Item | Verified value |
|---|---|
| List runs | `GET /repos/{o}/{r}/actions/runs`, `per_page` max 100, filters `created`, `status`, `exclude_pull_requests`, etc. **Capped at 1,000 results** when filtering by `actor`, `branch`, `check_suite_id`, `created`, `event`, `head_sha`, `status`\[5\]\[6\]\[7\] |
| List jobs | `GET /runs/{run_id}/jobs` and `GET /runs/{run_id}/attempts/{n}/jobs` |
| Job logs | `GET /repos/{o}/{r}/actions/jobs/{job_id}/logs` → redirect to plain text; **link expires after 1 minute** |
| Step logs | `GET /actions/jobs/{job_id}/steps/{step_number}/logs` (newer; useful for targeted re-fetch) |
| Permission | GitHub App "Actions" repository permission: read |
| API version header | `X-GitHub-Api-Version: 2026-03-10` appears in current docs (2022-11-28 still widely used)\[8\] |
| Rate limit | App installation on a GHEC org: **15,000 req/h**; secondary: 100 concurrent, 900 points/min on REST\[9\]\[10\] |
| Retention | Default 90 days; private/internal 1–400 days; public 1–90. **Per the GitHub Changelog of Aug 27, 2026, since Oct 1, 2026 checks, workflow runs and statuses "will be governed by the same Actions retention setting… with a default of 90 days". Before that they were kept for 400+ days** |

Log line format: each line is `<RFC3339 timestamp with 7 fractional digits> <text>`, for example `2026-10-27T10:15:02.1234567Z ##[group]Run make build`. The first line may carry a UTF-8 BOM. `##[group]`/`##[endgroup]` delimit steps, and `##[error]`, `##[warning]` and `##[debug]` mark severity. Existing parsers (for example Maple's OTLP importer) assign each line to the step whose time window contains it and derive severity from these markers.\[11\] This plan does the same.

### Triggers

- **schedule:** POSIX cron, minimum every 5 minutes, runs from the latest commit on the default branch, and optional IANA timezone support. It "can be delayed during periods of high loads… High load times include the start of every hour… some queued jobs may be dropped." Public repositories auto-disable schedules after 60 days of inactivity (the forwarder repo should be internal/private).\[12\]
- **workflow_run:** triggers only if the workflow file is on the default branch, cannot chain more than three levels, and gets secrets and write tokens even if the upstream did not.\[13\]
- **repository_dispatch:** `POST /repos/{o}/{r}/dispatches`. A GitHub App needs `contents: write` + `metadata: read`. `client_payload` allows at most 10 top-level properties.\[14\]\[15\]
- **actions/create-github-app-token (v3):** `owner` without `repositories` gives a token for all repos in the installation. `permission-<name>` inputs narrow scope. Tokens are scoped to a single owner.\[16\]\[17\]

### Prior art (and why a custom CLI)

| Option | What it does | Gap for this requirement |
|---|---|---|
| `dynatrace-oss/dynatrace-github-action` v9 | Sends metrics, events and SDLC events (`/platform/ingest/v1/events.sdlc`) from a workflow step | No build-log export\[18\] |
| Dynatrace CI/CD Observability app | Webhooks → OpenPipeline → SDLC events for pipelines, jobs and stages | Run/job telemetry, not log lines\[19\] |
| OTel Collector `githubreceiver` | Webhook → traces (spans per workflow, job and step) plus VCS metrics scraper | No log ingestion; needs a hosted collector\[20\] |
| `plengauer/opentelemetry-github` (action `plengauer/opentelemetry-github/actions/instrument/workflow`). Built by Philipp Lengauer, Sr Principal Product Manager at Dynatrace, per a Dynatrace Community video post | Traces/logs from inside instrumented jobs | Requires changing every workflow |
| **ghlog2dt (this plan)** | Pulls existing logs from every repo, with zero changes to target workflows | You own the code (small, stdlib-only) |

Dynatrace's own guidance lists OpenTelemetry, SDLC events, and OpenPipeline extracting SDLC events *from pipeline logs*.\[21\] The custom forwarder feeds that last path. Run it alongside the CI/CD Observability app or SDLC events, not instead of them.

## Architecture

```mermaid
flowchart LR
  subgraph Targets["CoolGitOrg target repos (1..N)"]
    A1[app-a workflows] -->|optional workflow_run completed| D1[notify-forwarder.yml]
  end
  D1 -->|repository_dispatch ghlog-export\n{repo, run_id, attempt}| F
  subgraph Central["CoolGitOrg/gh-log-forwarder"]
    S[forward.yml\nschedule: rendered cron] --> P[plan job: resolve targets]
    P --> M[matrix shards\nghlog2dt run --repo ...]
    F[forward-event.yml\non repository_dispatch] --> R[ghlog2dt run --run-id ...]
    M --> C[commit-state job\nstate branch: forwarder-state]
    R --> C
  end
  M -->|GET runs/jobs/logs\nApp installation token| GH[(GitHub REST API)]
  R --> GH
  M -->|POST /api/v2/logs/ingest\nApi-Token logs.ingest| DT[(Dynatrace abc12345.live)]
  R --> DT
  DT --> OP[OpenPipeline: route log.source==github-actions\n→ pipeline → bucket ci_github_actions_logs]
  TF[Terraform: github + dynatrace providers] -.secrets/vars/workflow file.-> Central
  TF -.token, bucket, pipeline.-> DT
```

**Modes**
1. **Scheduled pull (default, authoritative).** Each sweep lists completed runs per target repo since a high-water mark minus an overlap, fetches unseen jobs, ships them, and checkpoints. Missed events heal themselves.
2. **Event-driven push (optional).** A 10-line `notify-forwarder.yml` in each target repo fires on `workflow_run: completed` and dispatches `{repo, run_id, run_attempt}` to the central repo. Latency becomes about one minute. Dedupe against the same state makes double delivery harmless.
3. **Targeting.** `--repo CoolGitOrg/app-a` (single), `--repos-file targets.txt` / `GHLOG_TARGET_REPOS` (list), or `--all` (enumerate `GET /installation/repositories`, filtered by `--include`/`--exclude` globs).

## Go CLI design (`ghlog2dt`, Go ≤ 1.24, stdlib only)

**Subcommands** (each composable over stdin/stdout NDJSON, Unix-style):

| Command | Input → Output | Purpose |
|---|---|---|
| `list` | flags → NDJSON of `{repo, run_id, attempt, job_id, ...}` | Discover unseen completed jobs |
| `fetch` | job refs (stdin) → NDJSON Dynatrace records | Download and parse logs |
| `ship` | NDJSON records (stdin) → summary JSON | Batch and POST to Dynatrace |
| `run` | flags → summary JSON | `list | fetch | ship` in-process plus checkpoint |
| `checkpoint` | `show`/`prune`/`mark` | Inspect and maintain state |
| `token` | App ID + PEM → installation token on stdout | Go-minted RS256 JWT → `POST /app/installations/{id}/access_tokens` (no Action needed) |
| `version` | — | Version, commit, Go version |

`ghlog2dt list --repo X | ghlog2dt fetch | ghlog2dt ship --dry-run` is the debugging pipeline. `run` is the production path.

**Flags and environment** (flags override env, env overrides config):

| Flag | Env | Default |
|---|---|---|
| `--config` | `GHLOG_CONFIG` | `ghlog2dt.json` |
| `--repo`, `--repos-file`, `--all` | `GHLOG_TARGET_REPOS` (comma list) | — |
| `--since` / `--lookback` | `GHLOG_LOOKBACK` | `2h` (hard cap `20h`) |
| `--state` | `GHLOG_STATE_DIR` | `state/` |
| `--gh-api` | `GITHUB_API_URL` | `https://api.github.com` |
| — | `GH_TOKEN` | required |
| `--dt-url` | `DT_URL` | required, must be `https://` |
| — | `DT_LOGS_TOKEN` | required (never a flag; avoids `ps` leaks) |
| `--dt-auth` | `DT_AUTH_SCHEME` | `Api-Token` (or `Bearer` for platform token) |
| `--batch-bytes`, `--batch-records` | — | `4194304`, `5000` |
| `--gzip` | `GHLOG_GZIP` | `false` (enable only after the Phase-0 spike) |
| `--stale` | — | `drop` (`rewrite` sets ingest time and keeps the original in `gh.log.timestamp_original`) |
| `--max-line-bytes` | — | `65536` (truncate and set `gh.log.truncated=true`) |
| `--concurrency` | — | `4` (download workers, kept low for secondary limits) |
| `--dry-run` | — | writes records to stdout, no POST |
| `--log-format` | — | `json` (structured stderr) |

**Config file** (JSON, because there is no YAML in stdlib):

```json
{
  "owner": "CoolGitOrg",
  "targets": ["CoolGitOrg/app-a", "CoolGitOrg/app-b"],
  "include": ["CoolGitOrg/*"],
  "exclude": ["CoolGitOrg/gh-log-forwarder", "CoolGitOrg/*-archive"],
  "lookback": "2h",
  "pending_ttl": "12h",
  "dt": { "url": "https://abc12345.live.dynatrace.com", "auth_scheme": "Api-Token",
          "batch_bytes": 4194304, "batch_records": 5000, "gzip": false },
  "attributes": { "log.source": "github-actions", "dt.security_context": "ci" },
  "redact": ["ghp_[A-Za-z0-9]{36}", "ghs_[A-Za-z0-9]{36}", "github_pat_[A-Za-z0-9_]{80,}",
             "dt0[a-z]\\d{2}\\.[A-Z0-9]{24}\\.[A-Za-z0-9]{64}", "AKIA[0-9A-Z]{16}"]
}
```

**Exit codes:** `0` ok · `1` unexpected error · `2` usage/config · `3` partial success (Dynatrace 200 partial or stale drops above threshold) · `4` auth (GitHub 401, Dynatrace 401/403) · `5` budget (GitHub rate limit or DPS 402) · `6` state conflict.

**Package layout**

```
ghlog2dt/
  cmd/ghlog2dt/main.go          # flag parsing, subcommand dispatch, exit codes
  internal/config/config.go     # JSON config + env + flags merge, validation
  internal/github/client.go     # REST client: auth, retries, Link pagination, rate limit
  internal/github/runs.go       # ListRuns, ListJobs, JobLog (streaming)
  internal/github/apptoken.go   # RS256 JWT + installation token (crypto/rsa, crypto/x509)
  internal/parse/parse.go       # line → Record; severity; step attribution; redaction
  internal/dynatrace/client.go  # POST, gzip, retry/backoff, partial-success handling
  internal/dynatrace/batch.go   # byte/count-bounded batcher, 413 split
  internal/state/state.go       # checkpoint JSON (atomic write via os.Root)
  internal/obs/summary.go       # run summary → stderr JSON, $GITHUB_STEP_SUMMARY, self-log
  testdata/                     # golden job logs, recorded API responses
  Makefile  scripts/  .github/workflows/  terraform/
```

### Record schema (lean on purpose, because billing counts raw bytes)

Per-line fields: `timestamp`, `content`, `severity`, `gh.line` (line number), `gh.step.number`, `gh.step.name`.
Per-job fields (sent once per request via `X-Dynatrace-Attr` when `--header-attrs` is on, otherwise repeated in the body): `log.source=github-actions`, `vcs.repository.name`, `vcs.ref.head.name`, `vcs.ref.head.revision`, `cicd.pipeline.name` (workflow), `cicd.pipeline.run.id`, `cicd.pipeline.task.name` (job), `cicd.pipeline.task.run.id` (job_id), `cicd.pipeline.result` (conclusion), `gh.run_attempt`, `gh.event`, `gh.actor`, `gh.runner.name`, `gh.job.url`.

The `cicd.*`/`vcs.*` names follow OpenTelemetry CI/CD semantic conventions, which the Dynatrace CI/CD app accepts as alternatives (`cicd.pipeline.result`, `cicd.pipeline.task.name`, …).\[22\]

### Representative code skeletons

`cmd/ghlog2dt/main.go` is a flat `map[string]func(ctx, args) error` dispatch over `os.Args[1]`. It uses `signal.NotifyContext` for SIGINT/SIGTERM, stamps `main.version`/`main.commit` via `-ldflags -X`, and uses `errors.As` to map typed `ExitError`s to the exit codes above.

`internal/github/client.go` (pagination, retries, rate limits, safe redirect)

```go
package github

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const apiVersion = "2026-03-10" // pin; fall back to 2022-11-28 if rejected

type Client struct {
	Base, Token string
	HC          *http.Client // Timeout 60s; default CheckRedirect
	MaxRetries  int
}

var nextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func NextLink(h http.Header) string {
	if m := nextRe.FindStringSubmatch(h.Get("Link")); m != nil {
		return m[1]
	}
	return ""
}

// Do sends a GET with retry. Go's http.Client strips Authorization when a
// redirect crosses to a different host, so the 302 to the signed log-blob URL
// never receives the installation token.
func (c *Client) Do(ctx context.Context, url string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("X-GitHub-Api-Version", apiVersion)
		req.Header.Set("User-Agent", "ghlog2dt")

		resp, err := c.HC.Do(req)
		if err == nil && resp.StatusCode < 400 {
			return resp, nil
		}
		wait, retry := c.backoff(resp, attempt)
		if resp != nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
		}
		if !retry || attempt >= c.MaxRetries {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("github: %d %s", resp.StatusCode, url)
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) backoff(resp *http.Response, attempt int) (time.Duration, bool) {
	exp := time.Duration(1<<attempt) * time.Second
	jitter := time.Duration(rand.Int64N(int64(time.Second)))
	if resp == nil { // network error
		return exp + jitter, true
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" { // secondary limit
		if s, err := strconv.Atoi(ra); err == nil {
			return time.Duration(s)*time.Second + jitter, true
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" { // primary limit
		if r, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			d := time.Until(time.Unix(r, 0)) + jitter
			return d, d < 15*time.Minute // else give up -> exit 5
		}
	}
	switch resp.StatusCode {
	case 429, 500, 502, 503, 504:
		return exp + jitter, true
	case 404: // job log not uploaded yet right after completion
		return exp + jitter, attempt < 3
	}
	return 0, false
}
```

`internal/github/runs.go` lists runs with `status=completed&exclude_pull_requests=true&per_page=100&created=FROM..TO` and follows `Link: rel="next"`. Whenever `total_count >= 1000`, it recursively halves the time window, because filtered queries are capped at 1,000 results.\[7\]\[23\]

`internal/parse/parse.go`

```go
package parse

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"
)

type Step struct {
	Number     int
	Name       string
	Start, End time.Time
}

type Record struct {
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
	Severity  string `json:"severity"`
	Line      int    `json:"gh.line"`
	StepNum   int    `json:"gh.step.number,omitempty"`
	StepName  string `json:"gh.step.name,omitempty"`
	Truncated bool   `json:"gh.log.truncated,omitempty"`
}

var markers = map[string]string{
	"##[error]": "error", "##[warning]": "warn", "##[debug]": "debug", "##[notice]": "info",
}

// Parse streams a job log. Lines older than minTS are counted, not emitted.
func Parse(r io.Reader, steps []Step, redact []*regexp.Regexp, maxLine int,
	minTS time.Time, emit func(Record) error) (stale int, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var last time.Time
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimPrefix(sc.Text(), "\uFEFF")
		ts, text := last, line
		if i := strings.IndexByte(line, ' '); i > 0 {
			if t, perr := time.Parse(time.RFC3339Nano, line[:i]); perr == nil {
				ts, text = t, line[i+1:]
			}
		}
		last = ts
		if ts.Before(minTS) {
			stale++
			continue
		}
		sev := "info"
		for m, s := range markers {
			if strings.HasPrefix(text, m) {
				sev, text = s, strings.TrimPrefix(text, m)
				break
			}
		}
		for _, re := range redact {
			text = re.ReplaceAllString(text, "[REDACTED]")
		}
		rec := Record{Timestamp: ts.UTC().Format(time.RFC3339Nano), Content: text, Severity: sev, Line: n}
		if len(rec.Content) > maxLine {
			rec.Content, rec.Truncated = rec.Content[:maxLine], true
		}
		if s := stepFor(steps, ts); s != nil {
			rec.StepNum, rec.StepName = s.Number, s.Name
		}
		if err := emit(rec); err != nil {
			return stale, err
		}
	}
	return stale, sc.Err()
}

func stepFor(steps []Step, ts time.Time) *Step {
	for i := len(steps) - 1; i >= 0; i-- {
		if !steps[i].Start.IsZero() && !ts.Before(steps[i].Start) {
			return &steps[i]
		}
	}
	return nil
}
```

`internal/dynatrace/batch.go` and `client.go`

```go
package dynatrace

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Batcher struct {
	MaxBytes, MaxRecords int
	buf                  bytes.Buffer
	n                    int
	Flush                func(ctx context.Context, payload []byte, n int) error
}

func (b *Batcher) Add(ctx context.Context, rec any) error {
	enc, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if b.n > 0 && (b.buf.Len()+len(enc)+2 > b.MaxBytes || b.n >= b.MaxRecords) {
		if err := b.Close(ctx); err != nil {
			return err
		}
	}
	if b.n == 0 {
		b.buf.WriteByte('[')
	} else {
		b.buf.WriteByte(',')
	}
	b.buf.Write(enc)
	b.n++
	return nil
}

func (b *Batcher) Close(ctx context.Context) error {
	if b.n == 0 {
		return nil
	}
	b.buf.WriteByte(']')
	p := append([]byte(nil), b.buf.Bytes()...)
	n := b.n
	b.buf.Reset()
	b.n = 0
	return b.Flush(ctx, p, n)
}

type Client struct {
	URL, Token, Scheme string // Scheme: "Api-Token" | "Bearer"
	Gzip                bool
	HC                  *http.Client
	MaxRetries          int
	HeaderAttrs         string // pre-encoded "k=v&k2=v2" for X-Dynatrace-Attr
}

type Result struct{ Accepted, Partial, Stale int }

var ErrTooLarge = fmt.Errorf("dynatrace: 413 payload too large")

func (c *Client) Post(ctx context.Context, payload []byte, n int) (Result, error) {
	body := payload
	if c.Gzip {
		var z bytes.Buffer
		zw := gzip.NewWriter(&z)
		zw.Write(payload)
		zw.Close()
		body = z.Bytes()
	}
	for attempt := 0; ; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
			strings.TrimRight(c.URL, "/")+"/api/v2/logs/ingest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Authorization", c.Scheme+" "+c.Token)
		if c.Gzip {
			req.Header.Set("Content-Encoding", "gzip") // UNVERIFIED for this endpoint
		}
		if c.HeaderAttrs != "" {
			req.Header.Set("X-Dynatrace-Attr", c.HeaderAttrs)
		}
		resp, err := c.HC.Do(req)
		if err == nil {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			switch resp.StatusCode {
			case 204:
				return Result{Accepted: n}, nil
			case 200: // partial: some events invalid or out of time range
				return Result{Accepted: n, Partial: 1}, nil // log msg; parse count if present
			case 400:
				if strings.Contains(string(msg), "out of correct time range") {
					return Result{Stale: n}, nil
				}
				return Result{}, fmt.Errorf("dynatrace 400: %s", msg)
			case 401, 403:
				return Result{}, fmt.Errorf("dynatrace auth %d", resp.StatusCode) // -> exit 4
			case 402:
				return Result{}, fmt.Errorf("dynatrace 402: DPS/licensing") // -> exit 5
			case 413:
				return Result{}, ErrTooLarge // caller splits batch in half
			case 429, 502, 503, 504:
				// retry below
			default:
				return Result{}, fmt.Errorf("dynatrace %d: %s", resp.StatusCode, msg)
			}
			if attempt >= c.MaxRetries {
				return Result{}, fmt.Errorf("dynatrace %d after %d retries", resp.StatusCode, attempt)
			}
			wait := time.Duration(1<<attempt)*time.Second + time.Duration(rand.Int64N(int64(time.Second)))
			if s, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil { // honored if present; not documented
				wait = time.Duration(s) * time.Second
			}
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
			continue
		}
		if attempt >= c.MaxRetries {
			return Result{}, err
		}
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
}
```

**Checkpoint model** (`internal/state`): there is one `state/<owner>__<repo>.json` per repo, so matrix shards never conflict. Each file holds `high_water` (max `run.created_at` fully processed), `pending_runs` (run_id → first seen while queued or in progress), and `shipped_jobs` (job_id → shipped_at, pruned after 48h). Writes go through `os.Root` (Go 1.24) to a temp file followed by a rename.\[24\]\[25\]

**Correctness rules built into `run`:**
1. The dedupe key is `job_id`. Re-run attempts get new job records, so `filter=all` on the jobs list plus the `ShippedJobs` set exports each attempt exactly once. Lines carry `gh.run_attempt`.
2. A job is shipped only when `status==completed`. Runs seen as queued or in-progress go into `PendingRuns` and are re-polled via `GET /runs/{id}` until done or until `pending_ttl` (default 12h) passes, so the log lines are still under 24h old.
3. State is committed only after a successful ship, giving at-least-once delivery. If a crash happens between ship and commit, a job may be duplicated. Every record carries `cicd.pipeline.task.run.id` + `gh.line`, so DQL can dedupe (`dedup {cicd.pipeline.task.run.id, gh.line}`).
4. Lines older than `now − 23h45m` are dropped and counted (or rewritten with `--stale=rewrite`). Without this, Dynatrace would reject the whole batch with a 400.

## GitHub workflows

### 1. Release (`.github/workflows/release.yml`)

On `push` of a `v*` tag (`permissions: contents: write`, `ubuntu-24.04`), the workflow runs `actions/setup-go@v5` with `go-version: 1.24.x`, then `make preflight test vet release checksums`. It publishes with `gh release create "$GITHUB_REF_NAME" dist/ghlog2dt_* dist/SHA256SUMS --generate-notes`.

### 2. Scheduled forwarder (`forward.yml`, rendered by Terraform; `${cron}` filled from `interval_minutes`)

```yaml
name: forward-logs
on:
  schedule:
    - cron: "${cron}"            # e.g. "7,22,37,52 * * * *" for 15 min, off the hour
  workflow_dispatch:
    inputs:
      repos: { description: "comma list override", required: false }
      lookback: { description: "e.g. 2h", required: false }
concurrency:
  group: forward-logs
  cancel-in-progress: false      # never kill a sweep mid-ship
permissions:
  contents: write                # state branch only
jobs:
  plan:
    runs-on: ubuntu-24.04
    outputs:
      matrix: ${{ steps.p.outputs.matrix }}
    steps:
      - id: p
        env:
          TARGETS: ${{ inputs.repos || vars.GHLOG_TARGET_REPOS }}
        run: |
          set -euo pipefail
          [[ -n "${TARGETS}" ]] || { echo "::error::GHLOG_TARGET_REPOS empty"; exit 2; }
          printf 'matrix=%s\n' "$(jq -cn --arg t "$TARGETS" '$t|split(",")|map(gsub("\\s";""))|map(select(length>0))')" >> "$GITHUB_OUTPUT"
  ship:
    needs: plan
    runs-on: ubuntu-24.04
    timeout-minutes: 30
    strategy:
      fail-fast: false
      max-parallel: 8
      matrix:
        repo: ${{ fromJson(needs.plan.outputs.matrix) }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/checkout@v4
        with: { ref: forwarder-state, path: state-branch }
      - id: app
        uses: actions/create-github-app-token@v3
        with:
          app-id: ${{ vars.GH_APP_ID }}
          private-key: ${{ secrets.GH_APP_PRIVATE_KEY }}
          owner: CoolGitOrg
          permission-actions: read
      - name: Install ghlog2dt (pinned + SHA256 verified)
        env: { GHLOG_VERSION: "${{ vars.GHLOG_VERSION }}", GH_TOKEN: "${{ github.token }}" }
        run: ./scripts/install-ghlog2dt.sh
      - name: Export
        env:
          GH_TOKEN: ${{ steps.app.outputs.token }}
          DT_URL: ${{ vars.DT_URL }}
          DT_LOGS_TOKEN: ${{ secrets.DT_LOGS_TOKEN }}
          GHLOG_LOOKBACK: ${{ inputs.lookback || vars.GHLOG_LOOKBACK }}
        run: |
          set -euo pipefail
          ghlog2dt run --repo "${{ matrix.repo }}" --state state-branch/state \
            | tee summary.json
          jq -r '"| \(.repo) | \(.jobs) jobs | \(.records) recs | \(.stale) stale |"' summary.json >> "$GITHUB_STEP_SUMMARY"
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: state-${{ strategy.job-index }}
          path: state-branch/state/
          retention-days: 1
  commit-state:
    needs: ship
    if: always()
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
        with: { ref: forwarder-state }
      - uses: actions/download-artifact@v4
        with: { pattern: "state-*", path: incoming, merge-multiple: true }
      - run: ./scripts/commit-state.sh incoming state
```

Pushing the state branch with `GITHUB_TOKEN` will not trigger further workflows, which is the anti-loop property you want.

### 3. Per-repo notifier (`notify-forwarder.yml` in each target repo, optional push mode)

```yaml
name: notify-log-forwarder
on:
  workflow_run:
    workflows: ["CI", "Release"]     # name them explicitly
    types: [completed]
jobs:
  dispatch:
    runs-on: ubuntu-24.04
    steps:
      - id: app
        uses: actions/create-github-app-token@v3
        with:
          app-id: ${{ vars.GH_APP_ID }}
          private-key: ${{ secrets.GH_APP_PRIVATE_KEY }}
          owner: CoolGitOrg
          repositories: gh-log-forwarder
          permission-contents: write
      - env:
          GH_TOKEN: ${{ steps.app.outputs.token }}
          RUN_ID: ${{ github.event.workflow_run.id }}
          ATTEMPT: ${{ github.event.workflow_run.run_attempt }}
        run: |
          set -euo pipefail
          gh api repos/CoolGitOrg/gh-log-forwarder/dispatches \
            -f event_type=ghlog-export \
            -f "client_payload[repo]=${GITHUB_REPOSITORY}" \
            -F "client_payload[run_id]=${RUN_ID}" \
            -F "client_payload[run_attempt]=${ATTEMPT}"
```

### 4. Event receiver in the central repo (`forward-event.yml`)

This workflow triggers on `repository_dispatch: types: [ghlog-export]` with `concurrency: forward-${{ github.event.client_payload.repo }}` (`cancel-in-progress: false`). It reuses the checkout, app-token and install steps from `forward.yml`, runs `ghlog2dt run --repo <payload.repo> --run-id <payload.run_id>`, then calls `scripts/commit-state.sh`. Both paths share state through per-repo files and the job-ID set, so a run delivered by push and later swept by the schedule is shipped once.

## Bash and Makefile (strict mode, pre-flight checks, checksums)

`scripts/install-ghlog2dt.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

die() { printf '::error::%s\n' "$*" >&2; exit 2; }

# --- pre-flight ---
for c in gh sha256sum install uname; do command -v "$c" >/dev/null || die "missing: $c"; done
[[ -n "${GHLOG_VERSION:-}" ]] || die "GHLOG_VERSION not set"
[[ "${GHLOG_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "bad version: ${GHLOG_VERSION}"
[[ -n "${GH_TOKEN:-}" ]] || die "GH_TOKEN not set"

arch="$(uname -m)"; case "$arch" in x86_64) arch=amd64;; aarch64) arch=arm64;; *) die "arch $arch";; esac
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

gh release download "${GHLOG_VERSION}" -R CoolGitOrg/gh-log-forwarder \
  -p "ghlog2dt_linux_${arch}" -p SHA256SUMS -D "$tmp"
( cd "$tmp" && grep " ghlog2dt_linux_${arch}\$" SHA256SUMS | sha256sum -c --strict - ) \
  || die "checksum mismatch"
install -m 0755 "$tmp/ghlog2dt_linux_${arch}" /usr/local/bin/ghlog2dt
ghlog2dt version
```

`scripts/commit-state.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
src="${1:?src dir}"; dst="${2:?dst dir}"
[[ -d "$src" ]] || { echo "no state to commit"; exit 0; }
git config user.name  "ghlog2dt[bot]"
git config user.email "ghlog2dt@users.noreply.github.com"
mkdir -p "$dst"; cp -f "$src"/*.json "$dst"/ 2>/dev/null || true
git add "$dst"
git diff --cached --quiet && { echo "state unchanged"; exit 0; }
git commit -m "state: $(date -u +%FT%TZ) run ${GITHUB_RUN_ID:-local}"
for i in 1 2 3 4 5; do
  git push origin HEAD:forwarder-state && exit 0
  sleep $((i * 3)); git pull --rebase --strategy-option=theirs origin forwarder-state
done
echo "::error::state push failed"; exit 6
```

`Makefile`

```make
SHELL := bash
.SHELLFLAGS := -euo pipefail -c
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 windows/amd64

.PHONY: preflight build test vet release checksums verify dry-run
preflight:
	@go version | grep -Eq 'go1\.(2[2-4])' || { echo "need Go <=1.24"; exit 2; }
	@test -z "$$(go list -m all | tail -n +2)" || { echo "non-stdlib deps found"; exit 2; }
build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/ghlog2dt ./cmd/ghlog2dt
test:
	go test -race -count=1 ./...
vet:
	go vet ./...
release:
	@mkdir -p dist
	@for p in $(PLATFORMS); do os=$${p%/*}; arch=$${p#*/}; ext=; [[ $$os == windows ]] && ext=.exe; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' \
	    -o dist/ghlog2dt_$${os}_$${arch}$$ext ./cmd/ghlog2dt; done
checksums:
	cd dist && sha256sum ghlog2dt_* > SHA256SUMS
verify:
	cd dist && sha256sum -c --strict SHA256SUMS
dry-run: build
	: "$${GH_TOKEN:?}"; bin/ghlog2dt run --repo "$${REPO:?}" --dry-run --lookback 2h | head -n 20
```

The `preflight` dependency check enforces the stdlib-only rule: any `require` other than the module itself fails the build. If go-echarts or upspin are later adopted, add them to an allow-list.

## Terraform

`terraform/github.tf`

```hcl
terraform {
  required_providers {
    github    = { source = "integrations/github" }
    dynatrace = { source = "dynatrace-oss/dynatrace" }
  }
}

provider "github" {
  owner = "CoolGitOrg"
  app_auth {
    id              = var.gh_app_id
    installation_id = var.gh_app_installation_id
    pem_file        = var.gh_app_pem
  }
}

variable "interval_minutes" {
  type    = number
  default = 15
  validation {
    condition     = contains([5, 10, 15, 20, 30, 60], var.interval_minutes)
    error_message = "Must divide 60; GitHub minimum is 5; keep well under Dynatrace's 24h log-age limit."
  }
}
variable "minute_offset" { default = 7 } # avoid :00 high-load window

locals {
  minutes  = join(",", [for m in range(var.minute_offset % var.interval_minutes, 60, var.interval_minutes) : tostring(m)])
  cron     = "${local.minutes} * * * *"
  lookback = "${max(2 * var.interval_minutes, 60) + 30}m" # overlap; pending set covers long runs
}

resource "github_repository_file" "forward_workflow" {
  repository          = "gh-log-forwarder"
  branch              = "main"
  file                = ".github/workflows/forward.yml"
  content             = templatefile("${path.module}/templates/forward.yml.tftpl", { cron = local.cron })
  commit_message      = "ci: set log export cron to ${local.cron}"
  overwrite_on_create = true
}

resource "github_actions_variable" "lookback" {
  repository    = "gh-log-forwarder"
  variable_name = "GHLOG_LOOKBACK"
  value         = local.lookback
}
resource "github_actions_variable" "targets" {
  repository    = "gh-log-forwarder"
  variable_name = "GHLOG_TARGET_REPOS"
  value         = join(",", var.target_repos)
}
resource "github_actions_variable" "dt_url" {
  repository    = "gh-log-forwarder"
  variable_name = "DT_URL"
  value         = "https://abc12345.live.dynatrace.com"
}
resource "github_actions_secret" "dt_token" {
  repository      = "gh-log-forwarder"
  secret_name     = "DT_LOGS_TOKEN"
  plaintext_value = dynatrace_api_token.gh_logs_ingest.token
}
```

`terraform/dynatrace.tf`

```hcl
provider "dynatrace" {
  dt_env_url   = "https://abc12345.live.dynatrace.com"
  dt_api_token = var.dt_admin_token # apiTokens.read/write + settings.read/write
  # OAuth client env vars for platform resources (bucket, OpenPipeline):
  # DT_CLIENT_ID / DT_CLIENT_SECRET / DT_ACCOUNT_ID; set DYNATRACE_HTTP_OAUTH_PREFERENCE=true
}

resource "dynatrace_api_token" "gh_logs_ingest" {
  name    = "gh-log-forwarder-ingest"
  enabled = true
  scopes  = ["logs.ingest"]
}

resource "dynatrace_platform_bucket" "ci_logs" {
  name         = "ci_github_actions_logs"
  display_name = "GitHub Actions build logs"
  kind         = "logs"
  retention    = 35
}

# Illustrative only: verify block schema against the provider version you pin
# (OpenPipeline v2 logs schemas moved 1.51 -> 1.65 -> 1.69 across 2026 releases).
resource "dynatrace_openpipeline_v2_logs_pipelines" "github_actions" {
  display_name = "GitHub Actions build logs"
  custom_id    = "pipeline_github_actions_logs"
  # processing: drop "##[endgroup]" noise, parse "Process completed with exit code N"
  # storage:    route to bucket ci_github_actions_logs
}
# plus dynatrace_openpipeline_v2_logs_routing: matcher log.source == "github-actions"
```

Terraform caveats:
- `dynatrace_api_token.token` is "stored as plain-text" in state, so use an encrypted remote backend with restricted access.\[26\]
- The provider has no resource for platform tokens or OAuth clients. The Dynatrace Community thread "Terraform provider Platform tokens/OAuth client resource" (Jun 26, 2026) confirms: "Currently, the Dynatrace Terraform provider doesn't have dedicated resources for Platform tokens or OAuth clients." If you switch to a `Bearer` platform token, create it manually and store it via `github_actions_secret`.
- OpenPipeline v2 resources need `settings.read`/`settings.write`. Dynatrace recommends `DYNATRACE_HTTP_OAUTH_PREFERENCE=true` so settings objects get an owner instead of becoming public.\[27\]

## Configurable frequency: design and tradeoffs

**Single knob:** `interval_minutes`. Terraform derives (a) the cron expression, rendered into `forward.yml` because `schedule` cannot read Actions variables at evaluation time, and (b) `GHLOG_LOOKBACK`.

| Interval | Latency to Dynatrace | GitHub API calls/h (200 repos, ~1,000 jobs/h) | Actions minutes | Risk |
|---|---|---|---|---|
| 5 min | 5–20 min (schedule jitter) | ~2,400 list calls + ~1,000 jobs + ~1,000 logs ≈ 4,400 | Highest | Peak-hour delays and drops, more state commits |
| **15 min (default)** | 15–30 min | ~800 + 2,000 ≈ 2,800 | Moderate | Good balance |
| 60 min | 1–1.5 h | ~200 + 2,000 ≈ 2,200 | Lowest | Long jobs push lines toward the 24h edge only if pending tracking fails |
| >6 h | — | — | — | **Do not use**; any delay pushes lines past Dynatrace's 24h limit |

All options fit comfortably in the 15,000 req/h GHEC installation budget. Watch secondary limits (900 points/min, 100 concurrent) and keep `--concurrency` ≤ 8.\[28\]

There are two alternatives to rendering the cron:
- A fixed `*/5` cron with a gate step that exits unless `now % GHLOG_INTERVAL == 0`. This lets you change frequency through a variable without a commit, but every tick burns runner minutes.
- Push mode only, with a daily sweep as a safety net.

## Security

- **Least privilege.**
  - The GitHub App token for reading is minted with only `permission-actions: read`.
  - The dispatch token in target repos is limited to `repositories: gh-log-forwarder` + `contents: write`.
  - State commits use the forwarder repo's own `GITHUB_TOKEN` (`contents: write`).
  - The Dynatrace token has `logs.ingest` only.
- **Secret placement.** Prefer repo-level secrets on the forwarder repo over org secrets. If org secrets are needed, use `visibility = "selected"`. Only the GitHub App private key needs to exist in target repos, and only if push mode is used. For push mode, consider a lower-privilege second app.
- **Redaction (defence in depth).** GitHub masks registered secrets in logs, but tokens printed by tools (or derived and base64-encoded values) can slip through. `ghlog2dt` applies the configured regex list before batching (GitHub PAT/App/fine-grained tokens, Dynatrace `dt0c01.*`, AWS keys, PEM headers). Mirror it in an OpenPipeline masking processor so a regex miss is caught server-side.
- **Token hygiene in Go.** Tokens come from env only and are never logged. `%v` of the config struct redacts token fields. Go's client drops `Authorization` on cross-host redirects, so the signed blob URL never sees the token.
- **OIDC.** Dynatrace log ingest does not accept GitHub OIDC tokens directly (not documented). Keep a static ingest token, rotate it with Terraform (`terraform apply -replace=dynatrace_api_token.gh_logs_ingest`), and set a calendar rotation of 90 days or less.
- **Data classification.** Use a dedicated bucket (`ci_github_actions_logs`) plus `dt.security_context` so Dynatrace IAM policies can restrict who reads build logs.

## Observability of the forwarder itself

- Every `run` emits one summary record to Dynatrace: `log.source=ghlog2dt.self`, `severity=info|warn|error`, and fields `repos`, `runs`, `jobs`, `records`, `bytes`, `stale_dropped`, `partial_batches`, `retries`, `gh_rate_remaining`, `duration_ms`. It also writes a Markdown table to `$GITHUB_STEP_SUMMARY`.
- Optionally post an SDLC event to `/platform/ingest/v1/events.sdlc` (scope `openpipeline.events_sdlc`) per sweep so the forwarder shows up beside the pipelines it observes.\[18\]
- Alerting (DQL in a Dynatrace Workflow):
  - No `ghlog2dt.self` record in 3× the interval → forwarder down (catches disabled schedules).
  - `stale_dropped > 0` → interval too long or a backlog.
  - `gh_rate_remaining < 2000`.
- Verification query: `fetch logs | filter log.source == "github-actions" | summarize lines = count(), by:{vcs.repository.name, cicd.pipeline.task.run.id}`. Compare against line counts the forwarder recorded per job.

## Cost and volume estimation (DPS list prices)

- Ingest & Process **$0.20/GiB** (Dynatrace public rate card), metered by `builtin:billing.log.ingest.usage` as the "Number of raw bytes sent to Dynatrace before enrichment and transformation". Retain **$0.0007/GiB-day**. Query **$0.0035/GiB scanned**. Retain with Included Queries is $0.02/GiB-day. Contract rates vary.
- **Worked example (assumptions, not measurements):** 1,000 jobs/day × 400 KB of raw log text = about 0.4 GiB/day of content. JSON envelope plus per-record attributes typically inflate this 2–4× when job-level attributes are repeated in every record, so about 1.2 GiB/day billed ingest ≈ **$0.24/day (~$7/month)**. 35-day retention of about 1.2 GiB/day × 35 ≈ 42 GiB × $0.0007 ≈ $0.03/day.
- **Levers:**
  - Send job-level attributes once via `X-Dynatrace-Attr`. Whether header-supplied attributes count toward billed bytes is unverified.
  - Drop `##[endgroup]` and empty lines client-side.
  - Use a shorter bucket retention.
  - Use log metrics instead of repeated `fetch logs | summarize` dashboards, since Dynatrace notes log metrics avoid per-query scan charges.\[29\]

## Failure modes

| Failure | Detection | Handling |
|---|---|---|
| Schedule delayed or dropped | Self-monitoring gap alert | Overlapping lookback + pending set; next sweep catches up |
| Schedule disabled (inactivity) | Gap alert | Forwarder repo is private/internal; state commits count as activity anyway |
| Job log 404 right after completion | 404 on `/logs` | 3 retries with backoff, then leave job unshipped for the next sweep\[11\] |
| Redirect URL expired (1 min) | 403/404 from blob host | Re-request `/logs` for a fresh redirect; stream immediately |
| >1,000 runs in window | `total_count ≥ 1000` | Recursive window split |
| GitHub primary/secondary limit | `X-RateLimit-Remaining: 0`, `Retry-After` | Sleep until reset (≤15 min), else exit 5; state preserved |
| Dynatrace 413 | 413 | Halve batch recursively down to 1 record; truncate oversized line |
| Dynatrace 429/502/503/504 | Status | Exponential backoff with jitter; exit 1 after max retries (no checkpoint) |
| Stale lines (>24h) | 400 "out of correct time range" or 200 partial | Count as `stale_dropped`; optional `--stale=rewrite` |
| DPS exhausted | 402 | Exit 5, alert; do not retry |
| State push race | Non-fast-forward | Per-repo files + rebase retry ×5; exit 6 |
| Crash between ship and commit | Duplicate lines | Query-time `dedup` on job ID + line |
| Log retention shorter than outage | Job logs gone (retention 1–400 days) | Alert on gap; retention ≥ 7 days recommended |

## Validation and testing plan

1. **Unit tests** (`go test -race`):
   - Golden job logs in `testdata/`: BOM, `##[group]` nesting, `##[error]`, lines with no timestamp, 1 MB lines, CRLF.
   - Parser output compared byte-for-byte.
   - Redaction regex tests with real-format fake tokens.
2. **HTTP contract tests** with `net/http/httptest`:
   - GitHub fake covering Link pagination, a 302 to another host (assert no `Authorization` header arrives), 403 with `X-RateLimit-Remaining: 0`, and `total_count=1000` splitting.
   - Dynatrace fake returning 204, 200 partial, 400 stale, 413 (assert split), and 429 then 204 (assert backoff).
3. **Phase-0 live spike** against a non-production Dynatrace environment:
   - (a) gzip `Content-Encoding` accepted or rejected.
   - (b) `X-Dynatrace-Attr` merging and billing via `builtin:billing.log.ingest.usage`.
   - (c) whether `Retry-After` appears on 429.
   - (d) raw vs. flattened model behaviour on your tenant.
   - (e) the `.apps…/platform/classic/environment-api/v2/logs/ingest` path with a platform token (expect: not supported).
4. **End-to-end:** a fixture repo with a workflow that prints known lines, including `::error::` and a fake token. Assert in DQL that the line count matches, severity is mapped, the token is redacted, and attributes are present.
5. **Release integrity:** `make verify` in CI; the install script refuses binaries whose SHA256 does not match.

## Rollout phases

1. **Phase 0 — Spike (1 week):** verify the unverified items above; finalize batch defaults.
2. **Phase 1 — Single repo, dry-run:** `ghlog2dt run --repo CoolGitOrg/app-a --dry-run`; compare record counts with `gh run view --log`.\[30\]
3. **Phase 2 — Single repo, live:** a 15-min cron; the OpenPipeline route to `ci_github_actions_logs`; self-monitoring alerts on.
4. **Phase 3 — 10 repos via matrix:** watch GitHub rate headroom, Dynatrace ingest bytes per day, and stale drops.
5. **Phase 4 — Org-wide (`--all` with excludes):** per-team `dt.security_context`; cost review after 2 weeks.
6. **Phase 5 — Push mode:** add `notify-forwarder.yml` to latency-sensitive repos, delivered by Terraform `github_repository_file` across target repos.

## Recommendations

- **Do:**
  - Ship the scheduled pull forwarder first, at `interval_minutes = 15`, minute offset 7.
  - Use a Classic `logs.ingest` token managed in Terraform.
  - Use JSON arrays ≤ 4 MiB / 5,000 records, gzip off until verified.
  - Route to a dedicated Grail bucket via OpenPipeline.
- **Do:** treat GitHub's 90-day default retention (which now also deletes run metadata since Oct 1, 2026) and Dynatrace's 24h ingest window as the two clocks that bound the design. Keep the export lag in minutes, not hours.
- **Plan to:** move to a platform token (`Bearer`, `openpipeline:logs:ingest`). The endpoint already accepts it, but it cannot yet be provisioned via Terraform.
- **Do not:** use OTLP (it needs a protobuf encoder, which breaks the stdlib constraint and gains nothing here), or rely on `workflow_run` alone (default-branch-only workflow files, three-level chain limit, and every target repo needs a file).
- **Go version:** the constraint caps you at 1.24. Go 1.24 reached end of life on Feb 10, 2026 (last patch 1.24.13, Feb 4, 2026), so it no longer gets security fixes, including for `net/http`.\[31\] Re-evaluate the cap. The code above uses nothing newer than 1.22 apart from `os.Root` (1.24), so moving up is trivial.

## Caveats and unverified items

- **gzip (`Content-Encoding: gzip`) on `/api/v2/logs/ingest` is not documented.** The endpoint page mentions only Content-Type/UTF-8.\[1\] Dynatrace's Logstash plugin offers "Optional gzip compression", which suggests support, but this is inference.\[32\] Whether the 10 MB limit applies before or after compression is also undocumented.
- **The `.apps.dynatrace.com/platform/classic/environment-api/v2/logs/ingest` path is not documented for log ingest.** Docs show that pattern only for Settings.\[33\] Platform tokens are documented on the `.live` URL.\[34\]
- **Dynatrace documents no `Retry-After` or `X-RateLimit-*` headers.** It specifies only "exponential backoff". Unofficial sources mention `X-RateLimit-Reset`.\[35\]\[36\] The client honours these headers if present but does not depend on them.
- **Whether the redirected log-blob download counts against GitHub's REST rate limit** is unverified. The budget estimates above count only the `/logs` call.
- **`workflow_run` wildcard workflow names** were not verified. The plan names workflows explicitly.
- **Job IDs are unique per attempt** is the standard behaviour this plan relies on for dedupe. Confirm it in the Phase-0 fixture by re-running a job.
- **OpenPipeline v2 Terraform block schemas** change often (schema 1.51 → 1.65 → 1.69 across 2026 provider releases), so the pipeline resource above is illustrative.\[37\]\[38\]\[39\] `dynatrace_platform_bucket` attribute names and the provider auth attributes should be checked against the pinned provider version.
- **Semantic attribute coverage:** `cicd.*`/`vcs.*` names follow OpenTelemetry conventions and the Dynatrace CI/CD app accepts several of them. Full Semantic Dictionary mapping for logs was not confirmed.
- **Header-supplied attributes (`X-Dynatrace-Attr`) and billing:** whether they count toward "raw bytes sent" is unverified. Measure it with `builtin:billing.log.ingest.usage` in Phase 0.
- **Pricing figures are list prices** from the public rate card. Your DPS contract governs the actual cost.
- **GitHub workflow and Terraform `github` resource names** were not re-checked against current provider docs in this research pass.

## Sources

1. [Log Monitoring API v2 - POST ingest logs](https://docs.dynatrace.com/docs/dynatrace-api/environment-api/log-monitoring-v2/post-ingest-logs)
2. [Ingest JSON and TXT logs](https://docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-ingestion/lma-log-ingestion-via-api/lma-ingest-json-txt-logs)
3. [Log Monitoring default limits (Logs Classic) - Dynatrace Docs](https://docs.dynatrace.com/docs/observe-and-explore/log-monitoring/log-monitoring-limits)
4. [Ingest sources in OpenPipeline — Dynatrace Docs](https://docs.dynatrace.com/docs/platform/openpipeline/reference/api-ingestion-reference)
5. [workflow runs](https://docs.github.com/ru/enterprise-cloud@latest/rest/actions/workflow-runs)
6. [workflow runs](https://docs.github.com/en/enterprise-server@3.15/rest/actions/workflow-runs)
7. [REST API endpoints for workflow runs - GitHub Docs](https://docs.github.com/en/rest/actions/workflow-runs)
8. [REST API endpoints for workflow jobs - GitHub Docs](https://docs.github.com/en/rest/actions/workflow-jobs)
9. [Rate limits for the REST API - GitHub Docs](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2026-03-10)
10. [Rate Limit Cheatsheet for Self-Hosting Github Runners - WarpBuild Blog](https://www.warpbuild.com/blog/rate-limits-self-hosted-runners)
11. [feat(vcs): GitHub Actions job logs as logs on their CI traces by JeremyFunk · Pull Request #1512 · MapleTechLabs/maple · GitHub](https://github.com/MapleTechLabs/maple/pull/1512)
12. [Events that trigger workflows - GitHub Docs](https://docs.github.com/actions/using-workflows/events-that-trigger-workflows)
13. [Events that trigger workflows - GitHub Docs](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)
14. [REST API endpoints for repositories - GitHub Docs](https://docs.github.com/en/rest/repos/repos)
15. [Create a repository dispatch event - GitHub v3 REST API](https://github.apidog.io/api-3489322)
16. [GitHub - actions/create-github-app-token: GitHub Action for creating a GitHub App Installation Access Token · GitHub](https://github.com/actions/create-github-app-token)
17. [Add support for multiple owner/org spaces · actions/create-github-app-token · Discussion #83 · GitHub](https://github.com/actions/create-github-app-token/discussions/83)
18. [DynatraceAction · Actions · GitHub Marketplace · GitHub](https://github.com/marketplace/actions/dynatraceaction)
19. [CI/CD Observability monitoring & observability](https://www.dynatrace.com/hub/detail/community-cicd-observability/)
20. [opentelemetry-collector-contrib/receiver/githubreceiver/README.md at main · open-telemetry/opentelemetry-collector-contrib · GitHub](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/githubreceiver/README.md)
21. [Unlocking CI/CD success with observability](https://www.dynatrace.com/news/blog/unlocking-ci-cd-success-with-observability/)
22. [GitHub - dynatrace-wwse/cicd-observability-app: This repo holds the releases of the CICD Observability App for Dynatrace. Monitor different providers, such as Azure Devops, Gitlab, Github, ArgoCD and more. · GitHub](https://github.com/dynatrace-wwse/cicd-observability-app)
23. [List a Workflow's Runs in a Repository](https://www.withone.ai/knowledge/github/conn_mod_def::GJ3YfVeYEEY::8cau0s5EQ8me4n-NqNU9iQ)
24. [Go 1.24 Release Notes - The Go Programming Language](https://go.dev/doc/go1.24)
25. [Go 1.24 is released! - The Go Programming Language](https://go.dev/blog/go1.24)
26. [dynatrace\_api\_token](https://registry.terraform.io/providers/dynatrace-oss/dynatrace/1.59.2/docs/resources/api_token)
27. [dynatrace\_openpipeline\_v2\_logs\_pipelines](https://registry.terraform.io/providers/dynatrace-oss/dynatrace/latest/docs/resources/openpipeline_v2_logs_pipelines)
28. [GitHub API Rate Limit Guide: Best Practices for AI Agents](https://fast.io/resources/github-api-rate-limiting-strategies/)
29. [Calculate your consumption of Metrics powered by Grail - Query (DPS) — Dynatrace Docs](https://docs.dynatrace.com/docs/license/capabilities/metrics/dps-metrics-query)
30. [GitHub Actions Tutorial — Reading Logs -](https://timesofcloud.com/github-actions/reading-logs/)
31. [Go 1.24 Release History and End-of-Life Status](https://versionlog.com/golang/1.24/)
32. [Stream logs to Dynatrace with Logstash — Dynatrace Docs](https://docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-ingestion/lma-stream-logs-with-logstash)
33. [Classic Environment API with Platform Token - Community Home](https://community.dynatrace.com/t5/Dynatrace-tips/Classic-Environment-API-with-Platform-Token/td-p/295999)
34. [Dynatrace API - Tokens and authentication](https://docs.dynatrace.com/docs/dynatrace-api/basics/dynatrace-api-authentication)
35. [Solved: Dynatrace API Rate Limit Exceeded Error - Community Home](https://community.dynatrace.com/t5/Dynatrace-API/Dynatrace-API-Rate-Limit-Exceeded-Error/m-p/248636)
36. [Add rate limit support to Dynatrace implementation · Issue #2398 · micrometer-metrics/micrometer · GitHub](https://github.com/micrometer-metrics/micrometer/issues/2398)
37. [dynatrace-oss/terraform-provider-dynatrace v1.97.0 on GitHub](https://newreleases.io/project/github/dynatrace-oss/terraform-provider-dynatrace/release/v1.97.0)
38. [dynatrace-oss/terraform-provider-dynatrace v1.91.0 on GitHub](https://newreleases.io/project/github/dynatrace-oss/terraform-provider-dynatrace/release/v1.91.0)
39. [pipelines package - github.com/dynatrace-oss/terraform-provider-dynatrace/dynatrace/api/builtin/openpipeline/logs/pipelines - Go Packages](https://pkg.go.dev/github.com/dynatrace-oss/terraform-provider-dynatrace/dynatrace/api/builtin/openpipeline/logs/pipelines)
