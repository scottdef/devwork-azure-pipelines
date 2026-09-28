# Operations runbook

**Owners:** `@CoolEngOrg/platform-admins` (platform), `@CoolEngOrg/platform-ai` (Foundry and AKS), `@CoolEngOrg/copilot-admins` (Copilot).

## Everyday views

- **Queue:** issues with the `issueops` label. Filter by state label, for example `label:issueops:submitted` for requests awaiting approval, or `label:issueops:failed`.
- **Dashboards:** Grafana → IssueOps → *Platform operations* (volume, phases, approval latency, lead time, oldest open request) and *Copilot ROI*.
- **Audit:** the issue timeline (markers include digests and actors), workflow run logs, GitHub environment deployment history, and the Azure Activity Log (`issueops-<issue>-<run>` deployments).

## Procedures

### A request is stuck in `executing`

The execution run ended without reporting, for example because it was cancelled or the runner was lost.

1. Open the run linked in the *Executing* comment and find out what happened:
   - **Foundry:** check `az cognitiveservices account deployment show`.
   - **Budgets:** check `GET …/budgets`.
   - **AKS:** check `kubectl -n issueops-agents get jobs`.
2. If the change **was applied**, re-run *only the report job* of that run (Actions → re-run job). If that isn't possible, dispatch the execute workflow with `force: true`. The apply steps are idempotent: update/unchanged for budgets, what-if and incremental deploy for Bicep.
3. If it **was not applied**, dispatch the execute workflow for the issue with `force: true`.

### Re-run a failed execution

An **eligible approver** comments `.retry <note>`. The gate re-checks the approvals for the unchanged content. If the fix needs different content (for example, less capacity), the requestor edits the issue and runs `.submit` again, and the approvers approve again.

### Pause the platform

Disable the workflows *IssueOps · command router* and all *execute* workflows (Actions → workflow → ⋯ → Disable). Intake keeps validating. Re-enable them to resume; pending commands must be re-posted.

### Emergency: revoke an approval in progress

Comment `.deny <reason>` as an eligible approver. If execution already started, cancel the workflow run. For an environment with reviewers, reject the pending deployment.

### Rotate the GitHub App key

1. App settings → Generate a private key.
2. Update the `ISSUEOPS_APP_PRIVATE_KEY` secret.
3. Run *IssueOps · sync labels* as a smoke test.
4. Delete the old key.

Rotate every 90 days, or immediately on suspicion of compromise.

### Rotate `ENTERPRISE_BILLING_PAT` / `COPILOT_AGENT_TOKEN`

Create the new token on the machine account and update the environment secret. Test with `issueops budget apply --dry-run` on a sandbox cost-center request (it lists enterprise budgets with the new PAT without changing anything), or a sandbox agentic task for the Copilot token. Then revoke the old token.

### Add a model to the Foundry allow-list

1. `az cognitiveservices account list-models -n <account> -g <rg> -o table`: confirm the version and SKU in the region.
2. Add it to `settings.models` with `environments` and `max_capacity`.
3. Open a PR (CODEOWNERS: AI governance). `make test` must pass.

### Onboard a new catalog agentic workflow

1. The owning team adds the compiled `*.lock.yml` to its repository (reviewed like code).
2. Add `settings.catalog.<name>` (workflow file, description, input → question id) and `settings.question_sets.agentic-workflow-catalog.<name>`.
3. Add the option to the form's `agent_catalog_workflow` dropdown.
4. Run `scripts/forms-to-json.sh` and `make test`, and add a scenario.

### Change approvers or thresholds

Edit `config/issueops.json` (see [approval-gating.md](approval-gating.md)), add or adjust a scenario, and open a PR. The change takes effect for the next event on every open issue, because approvals are recomputed at the gate.

### Update the agent runner

Merge changes, run *IssueOps · build agent runner* with a version, then update `settings.backends.aks-foundry-agent.image` (by digest) through a PR.

### Upgrade Grafana or the Pushgateway

`helm upgrade` with the pinned chart versions from [setup-guide.md](setup-guide.md#10-metrics-and-grafana-oss-12). Grafana stays on OSS 12.x (`image.tag`). Dashboards are provisioned from ConfigMap `issueops-dashboards` and are read-only in the UI.

### Clean up old report metrics

Each report is its own Pushgateway group (`job=issueops_copilot_roi`, `org`, `request`). Delete an old group with:

```bash
curl -X DELETE -u "$PUSHGATEWAY_AUTH" "$PUSHGATEWAY_URL/metrics/job/issueops_copilot_roi/org/CoolEngOrg/request/<n>"
```

## Troubleshooting index

| Symptom | Look at |
|---|---|
| No summary comment after opening | The issue lacks the `issueops` label (not created from a form?), the workflow is disabled, or the App token failed (check the private key and installation) |
| "The request did not pass validation" with heading errors | The user edited the `###` headings. Ask them to restore the form structure |
| A command gets 👀 but no reply | The *Handle command* step failed. Check the run log for API errors (permissions: Members, Custom org roles, Metadata) |
| `.approve` rejected unexpectedly | The comment lists eligible approvers. Check team membership (active), org role assignment, or target-repo permission |
| Gate refused | Read the refusal comment: content changed (`.submit` again), approvals no longer valid, already executing or completed |
| Foundry/Azure errors | [foundry-model-deployment.md](request-types/foundry-model-deployment.md#troubleshooting) |
| Copilot report issues | [copilot-usage-report.md](request-types/copilot-usage-report.md#troubleshooting) |
| Agent issues | [agentic-task-request.md](request-types/agentic-task-request.md#troubleshooting) |
