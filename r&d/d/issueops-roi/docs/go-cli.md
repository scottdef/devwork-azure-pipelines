# `issueops` CLI reference

`cmd/issueops` is the decision engine, used by every workflow. It is written in Go 1.21 with the standard library; `go-echarts` is used for report charts only. Build it with `make build`, or `go build ./cmd/issueops`.

## Common flags and environment

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--config` | `ISSUEOPS_CONFIG` | `config/issueops.json` | Registry |
| `--forms-dir` | `ISSUEOPS_FORMS_DIR` | `config/forms` | Issue forms as JSON |
| `--bot-login` | `ISSUEOPS_BOT_LOGIN` | set by `setup-issueops` | The only author whose markers are trusted |
| `--templates` | `ISSUEOPS_TEMPLATES` | embedded | Directory of `*.tmpl` overrides |
| `--out` | | `$RUNNER_TEMP/issueops` or `out` | Output directory (decision, comment, plans, outcome) |
| `--offline` | | false | No GitHub API (local runs) |
| `--now` | `ISSUEOPS_NOW` | now | Fixed clock (RFC3339) for reproducible runs |
| | `GH_TOKEN` / `GITHUB_TOKEN` | | API token (App installation token in workflows) |
| | `PARSED_JSON` | | `issue-ops/parser` output. Without it, the Go parser parses the issue body |
| | `FORM_RESULT`, `FORM_ERRORS` | | `issue-ops/validator` outputs, merged into validation errors |

**Exit codes:** `0` ok; `1` error; `2` usage; `3` the execution gate refused. On `3`, the CLI writes a refusal comment and sets the `refused` and `comment_path` outputs.

## Decision outputs

`intake`, `command handle`, `gate` and `finish` write `decision.json` and `comment.md`, and set these step outputs:

`action`, `phase`, `execute`, `environment`, `workflow`, `request_type`, `digest`, `escalated`, `rejected`, `reason`, `allowlist`, `close`, `labels_add`, `labels_remove`, `comment_path`, `comment_event`, `comment_update_prefix`.

The `apply-decision` composite action consumes them.

## Lifecycle commands

| Command | Used by | Purpose |
|---|---|---|
| `types` | people | List request types, labels, templates, workflows, environments |
| `resolve [--labels a,b]` | intake, router | Request type from the event's labels. Outputs `request_type`, `template`, `label`, `workflow`, `qa` |
| `parse --type T --body-file F` | people | Parse a body with the Go parser (mirrors `issue-ops/parser`) |
| `check-forms` | CI | Form labels, registry field references, dropdown options without `", "`, no `render:` textareas |
| `intake [--event E] [--parsed P]` | issue-opened | Validate, check requestor eligibility, plan approvals, render the summary or invalid comment |
| `command classify` | router | Is the comment a known command? Outputs `command`, `trigger`, `privileged` |
| `command handle` | router | Rebuild state, authorize and decide (submit, approve, deny, retry, answers, cancel, status, help) |
| `command allowlist` | optional | Static `github/command` allowlist for the current policy (`false` when dynamic selectors apply) |
| `gate --issue N --type T [--force]` | execute | Re-verify the digest-bound approval and recompute eligibility, then emit `executing` |
| `finish --issue N --status completed\|failed\|needs_input --result outcome.json [--error msg]` | execute | Completion, failure or follow-up comment; labels; close |
| `issue fetch\|close --issue N [--reason completed\|not_planned]` | actions | Fetch the body and metadata, or close |
| `labels [--dry-run]` | create-labels | Create or update labels from `config/labels.json` |
| `render --template NAME --data data.json` / `render --list` | people | Render any template (development aid) |

## Execution commands

All of these accept `--issue N --expect-digest D`. They **refuse** if the fetched content no longer hashes to `D` or no longer validates.

| Command | Purpose |
|---|---|
| `budget plan` / `budget apply [--dry-run]` | Build the budget payloads, then upsert through the budgets API. Enterprise scope uses `GH_ENTERPRISE_TOKEN` |
| `report build [--fixtures DIR --parsed parsed.json] [--search-pace 2.1s] [--reviews=false]` | Download usage-metrics NDJSON, search output, compute and render HTML, CSV, JSON and Prometheus. `--fixtures` runs offline and needs `--parsed` (e.g. an example's `parsed.json`) |
| `report push --file metrics.prom [--pushgateway URL] [--job J] [--grouping k=v,…]` | PUT to the Pushgateway. `PUSHGATEWAY_AUTH=user:pass` adds basic auth |
| `foundry plan` | Validated plan plus ARM `parameters.json`. Outputs account, RG, region, deployment name and paths |
| `foundry preflight --plan --models --usage --deployments` | Evaluate `az` JSON: availability, quota, collisions |
| `foundry result --plan --preflight --deployment --account` | Record provisioning state and endpoint |
| `agent spec` | Build the agent specification (validated fields and answers) |
| `agent dispatch` | Copilot cloud agent (`COPILOT_AGENT_TOKEN`) or catalog `workflow_dispatch` |
| `agent render-k8s` | ConfigMap and Job manifests from Go templates |
| `agent result --log runner.log` | Parse the runner's result block into `outcome.json` |

## Offline and ops commands

| Command | Purpose |
|---|---|
| `simulate --scenario examples/<type>/scenario.json --out DIR` | Run a whole lifecycle offline: render the body from values with the real form, parse it back, then run Intake, Handle, Gate, the real per-type planners against fixtures, and Finish. Writes `transcript.md`, `issue-body.md`, `parsed.json`, `decisions.json`, `timeline.json` and execution artifacts. `expect` blocks are asserted |
| `ops-metrics [--days 90] [--deep] [--push] [--issues-file F]` | Platform metrics (requests by phase, lead time, approval latency, escalations, oldest open request) as Prometheus text |

### Scenario format

```json
{
  "name": "…", "description": "…",
  "type": "foundry-model-deployment",
  "start": "2026-09-21T14:00:00Z",
  "issue": { "number": 301, "title": "…", "requestor": "lea-ml" },
  "values": { "<field id>": "text" | ["dropdown option"] | { "selected": ["checkbox label"] } },
  "directory": {
    "teams": { "platform-ai": ["priya-ai"] },
    "owners": ["olivia-owner"], "members": ["…"],
    "custom_roles": { "security_manager": ["sec-sofia"] },
    "repo_permissions": { "payments-api": { "hal-api": "maintain" } }
  },
  "steps": [
    { "do": "open", "expect": { "phase": "validated" } },
    { "do": "comment", "user": "lea-ml", "body": ".submit", "after": "2m", "expect": { "action": "approved", "environment": "foundry-dev" } },
    { "do": "edit", "values": { "foundry_capacity": "20" } },
    { "do": "reopen" },
    { "do": "execute", "execution": { "azure": { "models": "fixtures/az-list-models.json", "usage": "…", "deployments": "…" } },
      "expect": { "action": "completed", "close": "completed" } }
  ]
}
```

**Execution fixtures by type:**

| Type | Fixture |
|---|---|
| budget | `existing_budget` |
| report | `fixtures` (a directory in `copilot.FixtureSource` layout) |
| Foundry | `azure.{models,usage,deployments,endpoint,provisioning_state}` |
| agentic | `agent_result` or `runner_log`, `dispatch` |

**Expect keys:** `action`, `phase`, `execute`, `environment`, `rejected`, `escalated`, `close`, `gate_refused`, `contains` (comment substrings), `labels`.

## `agent-runner`

```
agent-runner --spec /spec/spec.json --system /spec/system-prompt.md --task /spec/task-prompt.md
  [--endpoint $FOUNDRY_ENDPOINT] [--deployment $FOUNDRY_DEPLOYMENT] [--api-version 2024-10-21]
  [--max-iterations $AGENT_MAX_ITERATIONS] [--timeout $AGENT_TIMEOUT_SECONDS]
  [--max-completion-tokens 6000] [--replay replies.json]
```

**Credentials:** Azure Workload Identity (`AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_FEDERATED_TOKEN_FILE`, `AZURE_AUTHORITY_HOST`), or `AZURE_OPENAI_API_KEY` for local development only. The exit code is `1` only for a `failed` result; the result block is always printed.
