# Examples

Every example is a **scenario**: an issue opened from the real form, followed by the comments people post, followed by execution. The scenario is run by `issueops simulate` through the same engine calls the workflows make: intake, command handling, gate, the real per-type planners against fixtures, and finish. The outputs under `output/` are therefore what the bot would actually post.

The scenarios are also the platform's **end-to-end tests** (`go test ./cmd/issueops`). Their `expect` blocks assert actions, phases, environments, rejections and comment content.

| Request type | Scenario | Demonstrates |
|---|---|---|
| [copilot-budget-request](copilot-budget-request/) | [scenario](copilot-budget-request/output/scenario/transcript.md) | $2,500 repository budget, FinOps escalation, self-approval refused, update of an existing $800 budget |
| | [scenario-user-budget](copilot-budget-request/output/scenario-user-budget/transcript.md) | Invalid → fixed, approval voided by an edit, beneficiary confirmation, expiring user budget |
| [copilot-usage-report](copilot-usage-report/) | [scenario](copilot-usage-report/output/scenario/transcript.md) | Two-team, 28-day ROI report with per-user detail (Copilot-admin escalation) → [HTML report](copilot-usage-report/output/scenario/report/copilot-roi.html) |
| | [scenario-org-auto](copilot-usage-report/output/scenario-org-auto/transcript.md) | Organization report, auto-approved on `.submit` |
| | [scenario-not-eligible](copilot-usage-report/output/scenario-not-eligible/transcript.md) | Requestor gating, `.cancel` |
| [foundry-model-deployment](foundry-model-deployment/) | [scenario](foundry-model-deployment/output/scenario/transcript.md) | Dev deployment auto-approved by policy, Bicep parameters, pre-flight |
| | [scenario-prod-ptu](foundry-model-deployment/output/scenario-prod-ptu/transcript.md) | Prod PTU: platform-ai + security_manager role + FinOps, quota failure, `.retry` |
| [agentic-task-request](agentic-task-request/) | [scenario](agentic-task-request/output/scenario/transcript.md) | AKS runner: Q&A, confidential escalation, agent follow-up question, re-approval, runbook deliverable |
| | [scenario-copilot-agent](agentic-task-request/output/scenario-copilot-agent/transcript.md) | Copilot cloud agent: answers, maintainer approval, issue assigned to Copilot |
| | [scenario-catalog](agentic-task-request/output/scenario-catalog/transcript.md) | Catalog agentic workflow: answer validation, workflow_dispatch inputs |

## Files per scenario

```
<type>/
  scenario*.json          input: form values, simulated directory (teams, roles, permissions), steps, expectations
  fixtures/               API/CLI fixtures (Copilot NDJSON reports, az JSON)
  output/<scenario>/
    issue-body.md         the issue body GitHub renders from the form
    parsed.json           the parsed form (same shape as issue-ops/parser output)
    transcript.md         every step: comment, decision, labels, bot comment (rendered Go templates)
    decisions.json        machine-readable decisions
    timeline.json         final issue + comments (input for `issueops ops-metrics --issues-file`)
    execution/            plans, payloads, parameters, pre-flight, manifests, runner logs, outcomes
    report/               (reports) copilot-roi.html, report.json, CSV, metrics.prom
```

## Run

```bash
make examples                                   # regenerate all outputs
bin/issueops simulate --scenario examples/copilot-budget-request/scenario.json --out /tmp/budget
bin/issueops ops-metrics --issues-file examples/copilot-budget-request/output/scenario/timeline.json --out /tmp/ops
```

All people, teams, repositories, budgets and usage numbers are fictitious. The Copilot fixtures are generated deterministically by [copilot-usage-report/fixtures/generate.go](copilot-usage-report/fixtures/generate.go).
