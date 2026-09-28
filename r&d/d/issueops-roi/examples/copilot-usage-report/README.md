# Example: Copilot usage and ROI report

## 1. Two teams, 28 days, with per-user detail ([transcript](output/scenario/transcript.md))

`dana-dev` requests an ROI report for `platform-eng` and `payments`, with the HTML report, CSV exports and Grafana metrics, and a per-user breakdown for a quarterly business review.

- **Intake:** `dana-dev` is in `engineering-managers`, so she may request. Per-user detail triggers the Copilot-admin escalation.
- `.submit`, then another manager's `.approve` is rejected (not an eligible approver), then `cara-copilot` approves.
- **Execution** reads the fixtures below and produces:
  - [copilot-roi.html](output/scenario/report/copilot-roi.html): KPI tiles, findings, six go-echarts charts each with a table view, team and user tables, definitions. Light and dark themes.
  - [report.json](output/scenario/report/report.json), [teams.csv](output/scenario/report/teams.csv), [users.csv](output/scenario/report/users.csv), [daily.csv](output/scenario/report/daily.csv)
  - [metrics.prom](output/scenario/report/metrics.prom), pushed to the Pushgateway for the *IssueOps / Copilot ROI* Grafana dashboard
  - a completion comment with the headline metrics and the artifact link

The HTML loads ECharts from the go-echarts assets host. Open it with network access, or change `assets_host` in the registry to an internal mirror.

## 2. Organization report, auto-approved ([transcript](output/scenario-org-auto/transcript.md))

Without per-user detail there are no approval rules, so `.submit` approves and the router dispatches execution at once. Organization scope also reads the `organization-1-day` and `repos-1-day` reports: Copilot-authored PR share, merged PRs by repository.

## 3. Requestor gating ([transcript](output/scenario-not-eligible/transcript.md))

A developer outside the eligible teams gets an *invalid* summary that names the teams allowed to request. `.submit` is refused, and the requestor cancels.

## Fixtures

[fixtures/](fixtures/) follows the `copilot.FixtureSource` layout. It holds 35 days (2026-08-17 → 2026-09-20) for 18 fictitious users in 4 teams and 5 repositories:

| Path | Content |
|---|---|
| `reports/users-1-day/<day>.ndjson` | per-user daily usage (AI credits, generations and acceptances, agent/CLI/cloud-agent flags, adoption phase) |
| `reports/user-teams-1-day/<day>.ndjson` | user → team |
| `reports/repos-1-day/<day>.ndjson` | per-repository PR metrics (Copilot-authored, reviewed, suggestions) |
| `reports/organization-1-day/<day>.ndjson` | organization day totals |
| `prs.json`, `reviews.json`, `commits.json`, `code_frequency/*.json`, `seats.json` | output data normally from search and stats APIs |

Regenerate with `make fixtures`. Deterministic: seed 20260921.
