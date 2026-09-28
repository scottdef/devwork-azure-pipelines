# B. Copilot usage and ROI report (`copilot-usage-report`)

This request produces a report that relates **Copilot AI cost to measurable developer output** for the organization, selected teams, or selected repositories. Cost is AI credits, optionally plus prorated seats. Output is merged pull requests, cycle time, reviews, commits and code churn.

- Form: [.github/ISSUE_TEMPLATE/copilot-usage-report.yml](../../.github/ISSUE_TEMPLATE/copilot-usage-report.yml)
- Code: [internal/copilot](../../internal/copilot/)
- Workflow: [execute-copilot-report.yml](../../.github/workflows/execute-copilot-report.yml)
- Examples: [examples/copilot-usage-report](../../examples/copilot-usage-report/), including the [HTML report](../../examples/copilot-usage-report/output/scenario/report/copilot-roi.html)

## Form fields

| Field id | Notes |
|---|---|
| `report_scope` | `organization`, `teams` (≤ 20 slugs), `repositories` (≤ 25) |
| `report_teams` / `report_repositories` | Comma-separated; `@CoolEngOrg/slug` and `CoolEngOrg/repo` accepted. The validator checks that each exists |
| `report_period` | Last 7, 14, 28, 56 or 90 days, or *Custom range* (`report_start_date`, `report_end_date`) |
| `report_cost_basis` | *AI credits and seats* or *AI credits only* |
| `report_outputs` | Interactive HTML report, CSV exports, Publish metrics to Grafana |
| `report_user_detail` | Per-user breakdown. Triggers the Copilot-admin escalation |
| `report_purpose` | Why the report is needed (audit trail) |

**Periods:** data is usually available with a two-day delay, so "Last N days" ends two days ago. Usage-metrics reports start at `earliest_report_day` (2025-10-10) and go back at most one year. The period is clamped, and the summary warns when that happens.

## Who can request and approve

- **Requestors:** members of `engineering-managers`, `copilot-admins` or `platform-admins`. Anyone else gets an *invalid* summary explaining eligibility (see [scenario-not-eligible](../../examples/copilot-usage-report/output/scenario-not-eligible/transcript.md)).
- **Approval:** none for aggregate reports. `.submit` auto-approves ([scenario-org-auto](../../examples/copilot-usage-report/output/scenario-org-auto/transcript.md)). With **per-user detail**, one `copilot-admins` approval is required ([scenario](../../examples/copilot-usage-report/output/scenario/transcript.md)).

## Data sources

| Data | Endpoint | Notes |
|---|---|---|
| Per-user daily usage | `GET /orgs/{org}/copilot/metrics/reports/users-1-day?day=` | Signed NDJSON links, downloaded without the GitHub token |
| User → team mapping | `…/reports/user-teams-1-day?day=` | Joined on `user_id` and `day`. Teams with fewer than 5 seated users are omitted by GitHub |
| Repository PR metrics | `…/reports/repos-1-day?day=` | Copilot-authored and reviewed PRs, median time to merge |
| Organization totals | `…/reports/organization-1-day?day=` | Organization scope |
| Merged PRs | `GET /search/issues?q=org:CoolEngOrg is:pr is:merged merged:… author:…` | Paced at 30 requests/min (`--search-pace`) |
| Reviews | `GET /search/issues?q=… reviewed-by:…` | Teams scope, per member |
| Commits, churn | `GET /repos/{o}/{r}/commits` (count via `Link`), `GET /repos/{o}/{r}/stats/code_frequency` | Repositories scope |
| Seats | `GET /orgs/{org}/copilot/billing` | Organization-wide seat count |

All report endpoints use `X-GitHub-Api-Version: 2026-03-10`. The legacy `/copilot/metrics` endpoint was sunset on 2026-04-02 and is not used.

## Metrics and definitions

| Metric | Definition |
|---|---|
| Active user | A seated user with any interaction, generation, acceptance or agent use that day |
| Engaged user | Accepted code, or used an agentic surface (agent mode, CLI, Copilot app, cloud agent) that day |
| AI cost | `ai_credits_used × pricing.ai_credit_usd`. Set the registry price to the unit price on your AI-credit billing usage report; automatic reconciliation is on the [roadmap](../roadmap.md) |
| Seat cost | `seats × seat_monthly_usd × days / 30` (only with *AI credits and seats*) |
| Cost per merged PR | Total cost in scope ÷ merged PRs in scope (plus an AI-only variant) |
| Cycle time | PR created → merged, in hours (median and p90) |
| Acceptance rate | Code acceptance activities ÷ code generation activities |

**Scope semantics:**

- **teams:** users on the selected teams. Users on several teams count toward each team, so team rows can sum to more than the total.
- **repositories:** the users who authored merged PRs in those repositories. Repository rows also get cost attributed from their authors.
- **organization:** everyone.

## Outputs

Each output is uploaded as the workflow artifact `copilot-report-<issue>`, kept for 30 days, and linked from the completion comment.

- **`copilot-roi.html`:** a self-contained page with go-echarts (SVG renderer) charts. It has a table view for every chart, light and dark themes, a colour-blind-validated palette, and hover tooltips. Charts:
  - daily active vs engaged users
  - daily AI cost
  - cost per merged PR by team
  - spend vs output scatter
  - merged PRs by repository, Copilot-authored vs other
  - weekly merged PRs
- **`report.json`:** the full model (totals, daily, weekly, teams, repositories, users).
- **CSV:** `daily.csv`, `teams.csv`, `repositories.csv`, `users.csv` (only with approved per-user detail).
- **`metrics.prom`:** pushed to the Pushgateway (`job=issueops_copilot_roi`, `org`, `request`) for the **IssueOps / Copilot ROI** Grafana dashboard.
- **Completion comment:** headline table, findings, team table, artifact link (Go template [comment.completed.copilot-usage-report.md.tmpl](../../internal/tmpl/templates/comment.completed.copilot-usage-report.md.tmpl)).

## Reading the numbers

Read spend together with output. A healthy pattern is engaged users rising with merged-PR throughput while cycle time stays flat or falls. A red flag is spend and active users rising while merged PRs and cycle time stay flat; segment by team to find where enablement is needed. AI-assisted lines of code are directional only. The report states its caveats in the *Notes* section.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| "No Copilot usage-metrics data was available" | Enable the *Copilot usage metrics* policy. Grant the App **Copilot metrics: read**. Check that the period isn't before 2025-10-10 |
| Teams missing | Fewer than 5 Copilot-seated users, or the team slug differs (the validator checks existence) |
| Search slow or rate-limited | Many team members. Output search is capped at `max_search_users` (300); narrow the teams |
| Metrics not in Grafana | The runner can't reach `PUSHGATEWAY_URL` (use the VNet runner), or Prometheus scrapes without `honor_labels: true` |
