# The DevOps Engineer's Guide to the GitHub API on GitHub Enterprise Cloud: Copilot Metrics, AI Credits, and Agentic Automation

## TL;DR
- **The Copilot data plane was re-architected in 2025–2026:** the legacy aggregate `/copilot/usage` and `/copilot/metrics` endpoints were closed down **April 2, 2026** (deprecation announced in the Jan 29, 2026 changelog "Closing down notice of legacy Copilot metrics APIs"), replaced by the report-based **Copilot usage metrics API** (signed NDJSON download links) plus new **billing usage** endpoints (`/settings/billing/ai_credit/usage` and `/premium_request/usage`) reflecting the **June 1, 2026** shift from Premium Request Units to **GitHub AI Credits**.
- **Use the right credential for each layer:** classic PATs for enterprise billing endpoints (fine-grained PATs and App tokens are rejected there), fine-grained PATs/GitHub Apps for metrics and agent tasks, and the workflow `GITHUB_TOKEN` (with a `permissions:` block) or `copilot-requests: write` for in-Actions automation. The Agent tasks API is user-to-server only.
- **Two distinct agent surfaces exist:** GitHub Agentic Workflows (`gh aw`, markdown-defined, read-only-by-default Actions jobs) and the Copilot cloud/coding agent (the "Agent tasks" REST API at `/agents/repos/{owner}/{repo}/tasks`), plus the retired `gh copilot` extension now superseded by the standalone `@github/copilot` CLI (GA February 25, 2026).

## Key Findings

1. **REST vs GraphQL:** For Copilot metrics, billing, and seat management, **REST is the only fully-featured surface** — the newest data (AI credits, premium requests, agent tasks, usage reports) is REST-only. GraphQL's role for Copilot is narrow: assigning issues to the coding agent (`copilot-swe-agent` bot) via `addAssigneesToAssignable`/`createIssue` mutations.
2. **Endpoint sunset & rename:** `GET /orgs/{org}/copilot/usage` and `GET /orgs/{org}/copilot/metrics` (aggregate JSON) were retired April 2, 2026 — GitHub Docs state verbatim: *"These Copilot metrics endpoints were closed down on April 2, 2026. Use the Copilot usage metrics endpoints instead, which provide more depth and flexibility."* Hitting the old paths now returns 404 by design. The successor is the **usage metrics report API** returning signed URLs to NDJSON files, at org, enterprise, user, and (via join) team scope.
3. **Billing/metering endpoints are new and classic-PAT-gated.** AI credit and premium-request usage live under `/settings/billing/...`, spelled `/organizations/{org}/` and `/enterprises/{enterprise}/`.
4. **Copilot cloud agent is now scriptable** via five public-preview "Agent tasks" endpoints, user-to-server tokens only.
5. **GitHub Models** provides an OpenAI-compatible inference API at `models.github.ai` requiring the `models: read` scope, usable in Actions via `actions/ai-inference` — but it is being **fully retired July 30, 2026**.

## Details

### 1. GitHub REST & GraphQL API fundamentals for GHEC

#### Authentication methods
GitHub offers four practical credential types, in rough order of preference for automation:

- **Fine-grained PATs (recommended default):** GA since March 18, 2025 and now enabled by default for all orgs. Each token is scoped to a single user OR single org, to specific repos, with granular permissions (e.g. "GitHub Copilot Business" org permission read, "Administration" org read, "Agent tasks" repo read/write). There is a limit of 50 fine-grained PATs per account. Use the `X-Accepted-GitHub-Permissions` response header to discover exactly what an endpoint needs — add one permission at a time.
- **Classic PATs:** Still required for a few endpoints — most importantly the **enterprise billing/usage endpoints**, which explicitly "do not work with GitHub App user access tokens, GitHub App installation access tokens, or fine-grained personal access tokens." For those, mint a classic PAT with `manage_billing:copilot` / `manage_billing:enterprise` / `read:enterprise`. Classic PATs also need SAML SSO authorization when hitting SSO-enforced orgs (otherwise 403/404 with an `X-GitHub-SSO` header pointing to the authorization URL).
- **GitHub Apps:** Best for long-lived, scalable automation. Own identity, short-lived installation tokens, higher rate limits (15,000 req/hr per installation on GHEC vs 5,000/hr for a user). Note: installation tokens are **not** accepted by the agent-tasks API or billing endpoints.
- **`GITHUB_TOKEN` in workflows:** Auto-minted per job; scope it with a `permissions:` block. Rate limit is 1,000 req/hr per repo on standard, 15,000/hr per repo on GHEC.

On GHE.com (dedicated data-residency subdomains), replace `api.github.com` with `api.SUBDOMAIN.ghe.com`, and `models.github.ai` accordingly.

#### Rate limiting
- Primary: 60/hr unauthenticated, 5,000/hr authenticated user, 15,000/hr GitHub App on GHEC, 1,000/hr (standard) or 15,000/hr (GHEC) for `GITHUB_TOKEN` per repo.
- Secondary limits (shared REST+GraphQL): no more than 100 concurrent requests; no more than 900 points/min per REST endpoint (2,000 points/min for GraphQL); no more than 90s CPU per 60s real time. Point cost: GET/HEAD/OPTIONS = 1, POST/PATCH/PUT/DELETE = 5.
- Handling: on `403`/`429`, honor `retry-after` if present; else if `x-ratelimit-remaining: 0`, wait until `x-ratelimit-reset` (UTC epoch seconds); else wait ≥1 minute, then exponential backoff. Note GitHub does **not** always send `retry-after` for secondary limits. Use conditional requests (`if-none-match` with ETag, `if-modified-since`) — a `304` does not count against your primary limit.

#### Pagination
REST uses `Link` headers; never hand-construct page URLs. `gh api --paginate` follows them automatically. For GraphQL, the query must accept `$endCursor: String` and select `pageInfo { hasNextPage endCursor }`. GraphQL caps 100 nodes per page.

#### REST vs GraphQL tradeoffs
- **GraphQL** returns exactly the requested fields, collapses N REST calls into one (e.g. followers-of-followers in a single request), is strongly typed, and is far more efficient at scale (one practitioner reported reading 2,177 repos in ~8s via GraphQL vs 50 repos in ~30s via REST). Costs: harder caching, learning curve, 100-node page cap.
- **REST** is simpler, HTTP-cacheable, more widely tooled, and — critically for this guide — the **only** surface exposing Copilot metrics reports, billing/AI-credit usage, seat management, and agent tasks.
- Node IDs let you hop between the two APIs.

#### GHEC-specific features
Enterprise-scoped endpoints (`/enterprises/{enterprise}/...`) for Copilot metrics, billing usage, cost centers, budgets, enterprise teams, and the coding-agent enterprise policy. Enterprise-level Copilot metrics require the "Copilot usage metrics" policy set to **Enabled everywhere**.

### 2. GitHub Copilot usage metrics through the API

#### The lifecycle: from `/usage` to `/metrics` to report-based API
- **Preview `/copilot/usage`** → retired.
- **GA aggregate `/copilot/metrics`** (announced GA Oct 30, 2024) returned a JSON array of daily objects. **Closed down April 2, 2026.** Hitting the old path now returns 404 by design.
- **Current: Copilot usage metrics API** (`/copilot/metrics/reports/...`) returns **signed download links to NDJSON files**, not inline data. Org-level analytics data begins December 12, 2025; enterprise per-user reports began October 10, 2025; historical data up to 1 year.

#### The current report endpoints (API version `2026-03-10`)
Organization scope:
- `GET /orgs/{org}/copilot/metrics/reports/organization-1-day?day=DAY`
- `GET /orgs/{org}/copilot/metrics/reports/organization-28-day/latest`
- `GET /orgs/{org}/copilot/metrics/reports/users-1-day?day=DAY` and `users-28-day/latest`
- `GET /orgs/{org}/copilot/metrics/reports/user-teams-1-day?day=DAY` (team membership join data)

Enterprise scope (swap `/orgs/{org}` for `/enterprises/{enterprise}`):
- `GET /enterprises/{enterprise}/copilot/metrics/reports/enterprise-1-day` / `enterprise-28-day/latest`
- `GET /enterprises/{enterprise}/copilot/metrics/reports/users-1-day` / `users-28-day/latest`
- `GET /enterprises/{enterprise}/copilot/metrics/reports/user-teams-1-day`

Response shape:
```json
{
  "download_links": [
    "https://example.com/copilot-usage-report-1.ndjson",
    "https://example.com/copilot-usage-report-2.ndjson"
  ],
  "report_start_day": "2025-07-01",
  "report_end_day": "2025-07-28"
}
```
Each file is **NDJSON** (one JSON object per line — parse line-by-line, not as an array). Signed URLs expire, so download promptly.

#### Authentication & policy
- Org reports: OAuth/classic PATs need `read:org`; fine-grained tokens need the "View Organization Copilot Metrics" permission. Only org owners / enterprise owners / billing managers.
- Enterprise reports: `manage_billing:copilot` or `read:enterprise`; enterprise owners/billing managers only.
- The "Copilot Metrics API access policy" (or "no policy") must be enabled.

#### Report fields (from "Data available in Copilot usage metrics")
- **Per-user reports** (`*-users-1-day` / `-28-day`): one record per user — `user_id`, `user_login`, `ai_credits_used`, `used_*` indicators, `ai_adoption_phase`, `totals_by_ide[]`, `totals_by_feature[]`, `totals_by_language`, `totals_by_model`, `totals_by_cli`, `last_known_cli_version`. No active-user counts or PR data.
- **Aggregated reports** (`enterprise-1-day` / `org-1-day`): active-user counts (`daily_active_users`), `pull_requests` object (creation, review, merge, median time-to-merge, suggestion activity), `totals_by_ai_adoption_phase`. No per-user identifiers.
- **User-teams report**: one row per (user, team, day) — team id, team slug, user id/login. Teams with <5 seated users are excluded.
- Volume counters: `code_generation_activity_count`, `code_acceptance_activity_count`, and `loc_*_sum` — these aggregate across inline completions, chat, and agent edits, so expect higher numbers than legacy inline-only metrics; re-baseline rather than diff across the cutover.

*(For historical context, the legacy aggregate `/copilot/metrics` array carried per-day objects like `total_active_users`, `total_engaged_users`, and nested `copilot_ide_code_completions` → `editors[]` → `models[]` → `languages[]` with `total_code_suggestions`, `total_code_acceptances`, `total_code_lines_suggested`, `total_code_lines_accepted`. If you maintained pipelines against that schema, note field names have changed and now split by language/model in the report NDJSON — a 1:1 mapping is not possible.)*

Caveats baked into the data: two full UTC days of lag; users need IDE telemetry enabled (server-side telemetry backfills some active users); Chat on GitHub.com and Mobile are excluded; org metrics attributed by membership, not seat assignment (a user can appear in multiple org dashboards but counts once at enterprise level).

#### Team-level metrics — the join recipe
There is no pre-aggregated team endpoint. Download the daily **user-teams** NDJSON and the daily **per-user usage** NDJSON, then **join on `user_id` + `day`** and aggregate by `team_id`. Always join *daily* activity to *daily* membership (never 28-day activity to a single-day membership snapshot) to avoid mis-attribution when membership changes. Team totals cannot be summed to an org/enterprise total (multi-team users are double-counted by design). Team-level breakdowns cover IDE completions, chat, Copilot CLI, code review, and cloud-agent activity, sliceable by language/IDE/feature/model.

#### Seat management (Copilot user management API)
- `GET /orgs/{org}/copilot/billing` — `seat_breakdown` (total, added_this_cycle, active_this_cycle, inactive_this_cycle), `seat_management_setting`, `plan_type` (`business`/`enterprise`), feature toggles.
- `GET /orgs/{org}/copilot/billing/seats` — paginated; returns an **object** `{ "total_seats": N, "seats": [...] }` (not a bare array). Each seat has `last_activity_at`, `assignee`, `assigning_team`.
- `GET /orgs/{org}/members/{username}/copilot` — single member's seat.
- `POST`/`DELETE /orgs/{org}/copilot/billing/selected_users` — add/remove seats (`{"selected_usernames":[...]}`; returns `{"seats_cancelled": N}`).
- Enterprise: `GET /enterprises/{enterprise}/copilot/billing/seats` (classic PAT with `manage_billing:copilot`/`read:enterprise`; fine-grained/App tokens not supported).

Auth: `manage_billing:copilot` or `read:org`; fine-grained needs "GitHub Copilot Business" (read) or "Administration" (read). Several of these are labeled public preview. `--paginate` merges the inner `seats` array across pages; because it is an object-wrapped array, extract deliberately with `--jq '.seats[]'` (and remember `--slurp` cannot be combined with `--jq`).

#### Copilot Business vs Copilot Enterprise API differences
The endpoints are identical; `plan_type` distinguishes them. Copilot Business is **$19/user/month, including $19 in monthly AI Credits**; Copilot Enterprise is **$39/user/month, including $39 in monthly AI Credits** (per the GitHub Blog usage-based-billing announcement). In credit terms, GitHub Docs list standard included allowances of ~1,900 credits/user/mo for Business and ~3,900 for Enterprise, with promotional allowances of 3,000/7,000 from June 1–Sept 1, 2026. Copilot Enterprise additionally unlocks features (e.g. `.com` chat, coding agent by default) but the seat/metrics/billing API surface is common.

### 3. GitHub AI credit usage & premium-request metering

#### The billing model shift
GitHub's blog post "GitHub Copilot is moving to usage-based billing" (by CPO Mario Rodriguez) states: *"all GitHub Copilot plans will transition to usage-based billing on June 1, 2026. Instead of counting premium requests, every Copilot plan will include a monthly allotment of GitHub AI Credits."* The stated rationale: *"Agentic usage is becoming the default, and it brings significantly higher compute and inference demands. The current premium request model is no longer sustainable."* Credits are consumed by token usage (input + output + cached) at each model's API rates. Base seat prices are unchanged; code completions and Next Edit suggestions remain free and consume no credits. Annual Pro/Pro+ subscribers stay on legacy PRU billing until expiry. Premium requests for Spark and Copilot cloud agent moved to dedicated SKUs November 1, 2025 for granular budget control.

#### The metering endpoints (`2026-03-10`)
Organization (note: `/organizations/`, spelled out — **not** `/orgs/`):
- `GET /organizations/{org}/settings/billing/ai_credit/usage`
- `GET /organizations/{org}/settings/billing/premium_request/usage`
- `GET /organizations/{org}/settings/billing/usage` (total)

Enterprise:
- `GET /enterprises/{enterprise}/settings/billing/ai_credit/usage`
- `GET /enterprises/{enterprise}/settings/billing/premium_request/usage`
- `GET /enterprises/{enterprise}/settings/billing/usage`

User:
- `GET /users/{username}/settings/billing/ai_credit/usage` / `premium_request/usage`

Response shape:
```json
{
  "timePeriod": { "year": 2025 },
  "organization": "GitHub",
  "usageItems": [
    { "product": "Copilot", "sku": "Copilot AI Credits", "model": "GPT-5",
      "unitType": "credits", "pricePerUnit": 0.01,
      "grossQuantity": 100, "grossAmount": 1,
      "discountQuantity": 0, "discountAmount": 0,
      "netQuantity": 100, "netAmount": 1 }
  ]
}
```
Premium-request items use `"sku": "Copilot Premium Request"`, `"unitType": "requests"`, `pricePerUnit` ~0.04. Optional filters: `year`, `month`, `day`, `user`, `model`, `product`. Only 24 months of data.

**Token restrictions (critical):** enterprise billing endpoints **reject fine-grained PATs, App user tokens, and App installation tokens** — use a **classic PAT**. Org-level AI-credit/premium endpoints do accept some fine-grained token types, but the `?user=` per-user filter is **blocked for enterprise-owned orgs** at the org level (community discussion #184208, navikt/copilot #111) — per-user premium usage is only retrievable at enterprise level by an enterprise owner/billing manager, or via the UI/CSV export.

#### Bulk CSV usage reports (public preview)
`POST /enterprises/{enterprise}/settings/billing/reports` with `{"report_type":"summarized|detailed|premium_request","start_date":"…","end_date":"…","send_email":true}` returns a report id; poll `GET /enterprises/{enterprise}/settings/billing/reports` for `status: completed` and `download_urls`. Requires enterprise admin/billing manager and classic PAT with `manage_billing:enterprise`.

#### Copilot coding agent & Actions cost tracking
Each Copilot cloud agent session consumes **one premium request** (dedicated Copilot cloud agent SKU) plus **GitHub Actions minutes** (the agent runs on GitHub-hosted runners). Copilot code review on PRs also consumes Actions minutes from the repo-owner's account. Budget via `/settings/billing/budgets` (alerts at 75/90/100%). Under the legacy PRU model, Copilot Business seats included 300 premium requests/mo and Enterprise 1,000, with overages at $0.04/request.

#### GitHub Models usage tracking
GitHub Models inference runs at `https://models.github.ai` (OpenAI-compatible). Org-attributed inference (`POST /orgs/{org}/inference/chat/completions`) tracks usage against the org; the model catalog is at `GET /catalog/models`. Paid GitHub Models usage appears in the billing usage endpoints. **Important:** per the July 1, 2026 GitHub changelog, *"GitHub Models is being fully retired on July 30, 2026 … including the playground, model catalog, inference API, and bring your own key (BYOK)"* — with brownouts on July 16 and 23. Do not build new long-term dependencies on it.

### 4. gh CLI cheatsheet

#### Basic section
```bash
# Auth
gh auth login                          # interactive OAuth
gh auth token                          # print current token (pipe to curl)
gh auth refresh -s read:enterprise,manage_billing:copilot,read:org
gh auth status

# Repos
gh repo clone OWNER/REPO
gh repo create NAME --private --clone
gh repo list ORG --limit 1000 --json name,visibility
gh repo view --json defaultBranchRef,diskUsage

# Pull requests
gh pr create -t "Title" -b "Body" -B main
gh pr list --state open --json number,title,author
gh pr checkout 123
gh pr merge 123 --squash --delete-branch
gh pr review 123 --approve

# Issues
gh issue create -t "Bug" -b "Repro"
gh issue list --label bug --json number,title
gh issue develop 123 --checkout      # branch from issue

# Releases
gh release create v1.2.3 ./dist/* --notes "Changelog"
gh release list

# Workflows / runs
gh workflow list
gh workflow run deploy.yml -f env=prod
gh run list --workflow deploy.yml
gh run watch ; gh run view --log

# Secrets & variables
gh secret set API_KEY --body "$VALUE"
gh secret set API_KEY --org MYORG --visibility selected --repos repo1,repo2
gh variable set REGION --body "us-east-1"
gh secret list ; gh variable list

# Extensions
gh extension install owner/gh-extension
gh extension list ; gh extension upgrade --all
```

#### Advanced section — Copilot/AI data via `gh api`
```bash
API=2026-03-10

# Org Copilot usage-metrics report → download links, then fetch NDJSON
gh api /orgs/ORG/copilot/metrics/reports/organization-28-day/latest \
  -H "X-GitHub-Api-Version: $API" --jq '.download_links[]' \
| while read -r url; do curl -s "$url"; done > org-metrics.ndjson

# Enterprise report
gh api /enterprises/ENT/copilot/metrics/reports/enterprise-28-day/latest \
  -H "X-GitHub-Api-Version: $API" --jq '.download_links[]'

# Seats (object with total_seats + seats[]) — auto-paginates
gh api /orgs/ORG/copilot/billing/seats --paginate \
  -H "X-GitHub-Api-Version: $API" \
  --jq '.seats[] | [.assignee.login, .last_activity_at] | @tsv'

# Seat breakdown
gh api /orgs/ORG/copilot/billing --jq '.seat_breakdown'

# AI credits & premium requests (note /organizations/, classic PAT for enterprise)
gh api /organizations/ORG/settings/billing/ai_credit/usage \
  -H "X-GitHub-Api-Version: $API" \
  --jq '.usageItems[] | {model, netQuantity, netAmount}'
gh api /enterprises/ENT/settings/billing/premium_request/usage \
  -H "X-GitHub-Api-Version: $API"

# Filters
gh api "/organizations/ORG/settings/billing/premium_request/usage?year=2026&month=6&model=gpt-5.2"

# Arbitrary GraphQL (coding-agent assignment bot lookup)
gh api graphql -f query='{ repository(owner:"o",name:"r"){ suggestedActors(capabilities:[CAN_BE_ASSIGNED], first:100){ nodes{ login __typename } } } }'

# Pagination patterns
gh api /orgs/ORG/members --paginate --jq '.[].login'         # array endpoint
gh api /orgs/ORG/copilot/billing/seats --paginate --slurp    # wrap pages (no --jq allowed with --slurp)
```
Key rules: `--jq` is built-in (no external jq). `--slurp` cannot be combined with `--jq`/`--template` (you get "the --slurp option is not supported with --jq or --template"). Placeholders `{owner}/{repo}/{branch}` resolve from the current repo. `--cache 1h` caches responses. Add `--hostname api.SUBDOMAIN.ghe.com` for GHE.com. In Actions, `gh` uses `GH_TOKEN` from the environment automatically.

#### `gh copilot` extension vs standalone Copilot CLI
The old `gh copilot` extension (`gh copilot suggest`/`explain`, originally GA March 21, 2024) is **retired**. GitHub Docs state: *"The GitHub Copilot extension for GitHub CLI is retired. It has been replaced by the new GitHub Copilot CLI."* The standalone agentic CLI reached **GA on February 25, 2026** ("the terminal-native coding agent … is now generally available for all Copilot subscribers"). Install with `npm install -g @github/copilot` (Node.js 22+), Homebrew, WinGet, or `curl -fsSL https://gh.io/copilot-install | bash`. It is included in all Copilot plans and each interaction draws on your plan's AI Credits allowance. Running `gh copilot` now bootstraps/forwards to the `copilot` binary. Headless: `copilot -p "prompt"`. Auth precedence for headless: `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`; classic `ghp_` tokens are not supported — use a fine-grained PAT with the "Copilot Requests" permission. It supports MCP servers, custom agents, `/plan`, `/fleet` subagents, and per-task `/model` selection (Anthropic/OpenAI/Google).

### 5. Four target use cases

#### Use Case 1 — Local machine
Bash one-liner (seat inactivity audit):
```bash
gh api /orgs/MYORG/copilot/billing/seats --paginate \
  --jq '.seats[] | select(.last_activity_at < "2026-06-01") | .assignee.login'
```
Python with `uv` (PEP-723 inline dependencies; run with `uv run script.py`):
```python
# /// script
# requires-python = ">=3.12"
# dependencies = ["httpx"]
# ///
import os, json, httpx

TOKEN = os.environ["GH_TOKEN"]        # classic PAT for enterprise billing endpoints
ENT   = os.environ["GH_ENTERPRISE"]
H = {"Authorization": f"Bearer {TOKEN}",
     "Accept": "application/vnd.github+json",
     "X-GitHub-Api-Version": "2026-03-10"}

with httpx.Client(base_url="https://api.github.com", headers=H, timeout=30) as c:
    # 1) get signed NDJSON links, then download & parse line-by-line
    r = c.get(f"/enterprises/{ENT}/copilot/metrics/reports/enterprise-28-day/latest")
    r.raise_for_status()
    rows = []
    for link in r.json()["download_links"]:
        text = httpx.get(link, timeout=60).text
        rows += [json.loads(l) for l in text.splitlines() if l.strip()]
    # 2) AI credit spend
    credits = c.get(f"/enterprises/{ENT}/settings/billing/ai_credit/usage").json()
    total = sum(i["netAmount"] for i in credits.get("usageItems", []))
    print(f"Records: {len(rows)}  AI-credit net spend: ${total:.2f}")
```
Rationale: `uv` gives reproducible, inline-dependency scripts with no venv ceremony; `httpx` handles retries/timeouts cleanly.

#### Use Case 2 — GitHub Actions workflow
```yaml
name: copilot-seat-report
on:
  schedule: [{ cron: "0 6 * * 1" }]
  workflow_dispatch:
permissions: {}                 # start from zero
jobs:
  report:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - name: Pull seats with gh
        env:
          GH_TOKEN: ${{ secrets.COPILOT_BILLING_PAT }}   # classic PAT; GITHUB_TOKEN can't read org billing
        run: |
          gh api /orgs/${{ vars.ORG }}/copilot/billing/seats --paginate \
            -H "X-GitHub-Api-Version: 2026-03-10" \
            --jq '.seats[] | [.assignee.login, .last_activity_at] | @tsv' \
            | tee seats.tsv
      - uses: actions/upload-artifact@v4
        with: { name: seats, path: seats.tsv }
```
For repo-scoped calls the built-in `GITHUB_TOKEN` (with a `permissions:` block) suffices; org billing/metrics require a stored PAT. To use Copilot CLI in a workflow, install it (`npm i -g @github/copilot`) and authenticate via `COPILOT_GITHUB_TOKEN` (a fine-grained PAT with "Copilot Requests" permission stored as a secret).

#### Use Case 3 — GitHub Agentic Workflow (GitHub Models / `actions/ai-inference`)
Direct AI-inference step:
```yaml
permissions:
  models: read          # required for GitHub Models inference
  contents: read
jobs:
  triage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/ai-inference@v1
        id: infer
        with:
          prompt: "Summarize open Copilot adoption gaps from this JSON: ${{ needs.pull.outputs.data }}"
          model: openai/gpt-4.1
      - run: echo "${{ steps.infer.outputs.response }}"
```
Markdown-defined agentic workflow (`gh aw`): frontmatter declares `on:`, `permissions:` (read-only by default), `tools:`, and `safe-outputs:`; the markdown body is the natural-language instruction. Compile with `gh aw` to a hardened `.lock.yml`, commit both files, and run like any Actions workflow. For Copilot-billed inference in org repos, set `copilot-requests: write` in frontmatter (uses `GITHUB_TOKEN`, bills the org, and ignores `COPILOT_GITHUB_TOKEN`/`GH_AW_GITHUB_TOKEN`). Cap spend with `max-ai-credits` (default 1,000 AIC/run); audit with `gh aw logs` / `gh aw audit`. Guardrails: sandboxing, the Agent Workflow Firewall (network egress allowlist), and safe-outputs that run as separate permission-scoped jobs so the agent never gets direct write access (a downstream threat-detection job scans buffered outputs before any write executes).

> Note: as of the sources reviewed, GitHub positions `gh aw`/Agentic Workflows as a GitHub Next research demonstrator plus a documented product surface; treat frontmatter keys as subject to change and pin the `gh aw` version.

#### Use Case 4 — GitHub Copilot coding agent task (Agent tasks API)
Five public-preview endpoints (user-to-server tokens only; App installation tokens **not** supported):
- `POST /agents/repos/{owner}/{repo}/tasks` — start a task (requires Copilot Business/Enterprise; extended to Pro/Pro+/Max June 4, 2026)
- `GET /agents/repos/{owner}/{repo}/tasks` — list repo tasks
- `GET /agents/repos/{owner}/{repo}/tasks/{task_id}` — get a task (embeds `sessions[]`)
- `GET /agents/tasks` — list all tasks for the authenticated user
- `GET /agents/tasks/{task_id}` — get a task by id

Start-task body: `prompt` (required), optional `model` (e.g. `claude-sonnet-4.6`, `claude-opus-4.6`, `gpt-5.2-codex`, `gpt-5.3-codex`, `gpt-5.4`), `base_ref`, `head_ref`, `create_pull_request` (default `false`). Permissions: "Agent tasks" repo **read** (GETs) / **read+write** (POST). State values: `queued`, `in_progress`, `completed`, `failed`, `idle`, `waiting_for_user`, `timed_out`, `cancelled`. There is **no** separate sessions/logs endpoint — session data (`id`, `state`, `head_ref`, `base_ref`, `model`, timestamps, `prompt`) is embedded in the single-task GET; list endpoints only expose `session_count`.

```bash
# Fan out a migration across an org's repos
for repo in $(gh repo list MYORG --json name --jq '.[].name'); do
  gh api -X POST /agents/repos/MYORG/$repo/tasks \
    -H "X-GitHub-Api-Version: 2026-03-10" \
    -f prompt="Migrate fetch() calls to the ApiClient wrapper" \
    -f base_ref="main" -F create_pull_request=true
done
# Poll status across all your tasks
gh api /agents/tasks --jq '.tasks[] | [.name, .state] | @tsv'
```
Sample create response:
```json
{ "id": "a1b2c3d4-…", "name": "Fix the login button on the homepage",
  "state": "queued", "session_count": 1, "artifacts": [],
  "html_url": "https://github.com/octocat/hello-world/copilot/tasks/a1b2c3d4-…" }
```
You can also assign an issue to the coding agent (`copilot-swe-agent` bot) via the GraphQL `addAssigneesToAssignable`/`createIssue` mutations with the `agentAssignment` input (custom instructions, base ref, model), sending the `GraphQL-Features: issues_copilot_assignment_api_support,coding_agent_model_selection` header, or via REST issue-assignment endpoints. The agent runs read-only in a GitHub Actions sandbox, pushes to a draft PR, and requires human approval before CI runs — existing branch protections and org policies still apply.

## Recommendations
1. **Build your metrics pipeline on the report-based API now (immediate).** Any integration still calling `/orgs/{org}/copilot/metrics` or `/copilot/usage` is broken (404 since April 2, 2026). Migrate to `/copilot/metrics/reports/...`, parse NDJSON line-by-line, and store daily snapshots (the API exposes only a 28-day rolling window / limited history). Trigger to act: any 404 or empty response from a legacy path.
2. **Segregate credentials by layer (this quarter).** Provision one **classic PAT** (`manage_billing:*`) solely for enterprise billing/usage/CSV endpoints; use **fine-grained PATs or a GitHub App** for metrics, seats, and agent tasks. Rotate on ≤90-day expirations. Migrate long-lived automation to GitHub Apps for the 15,000/hr limit and identity separation.
3. **Instrument AI-credit spend before the June 1, 2026 model fully bites (now).** Poll `/settings/billing/ai_credit/usage` and `/premium_request/usage` weekly; set budgets with alerts at 75/90/100%. Track dedicated SKUs (Copilot cloud agent, Spark) separately. Escalation threshold: net AI-credit spend trending above the seat-included allowance (~1,900 credits/user/mo Business, ~3,900 Enterprise).
4. **For team-level reporting, implement the daily user-teams ⋈ per-user join** rather than waiting for a team endpoint — none exists. Join daily-to-daily only.
5. **Prefer agentic workflows (`gh aw`, read-only + safe-outputs) over raw coding-agent CLIs in Actions** for least-privilege automation; reserve the Agent tasks API for scripted fan-out migrations where you review every resulting PR. Cap every run with `max-ai-credits`.
6. **Standardize on the new `@github/copilot` CLI**; retire any scripts calling `gh copilot suggest/explain`. Migrate off GitHub Models before the July 30, 2026 retirement.

## Caveats
- **Public preview churn:** Agent tasks, seat management, and CSV usage-report endpoints are explicitly "in public preview and subject to change." Pin `X-GitHub-Api-Version: 2026-03-10` and monitor the changelog.
- **Model lists are time-sensitive.** Supported `model` values for agent tasks and Copilot CLI change frequently; enumerate the catalog at runtime rather than hardcoding.
- **GitHub Models retirement (July 30, 2026, with brownouts July 16 and 23):** per changelog, the playground, catalog, inference API, and BYOK all go away — do not build new long-term dependencies on `models.github.ai`.
- **Per-user premium usage gap:** not available via API for enterprise-owned orgs at org level; enterprise-level classic-PAT access or UI/CSV only.
- **Billing endpoints reject fine-grained/App tokens** at enterprise scope — a common source of silent 403s.
- **Two-day data lag and telemetry dependency** mean metrics undercount users with telemetry disabled; treat them as adoption indicators, not precise productivity measures.
- Some figures (seat allowances, overage pricing, model multipliers, promotional credit grants) reflect a billing model in active transition; verify against current billing docs for your specific plan and contract.