# Example: agentic task request

## 1. AKS agent runner with Q&A and a follow-up question ([transcript](output/scenario/transcript.md))

`ada-platform` asks the AKS/Foundry agent for a zero-downtime AKS node-pool upgrade runbook for Kubernetes 1.30. The data is **confidential**.

1. **Intake:** the form is valid. The summary lists questions Q1–Q6 with an answer template. The phase is `awaiting-answers`. The plan needs a target-repo maintainer, platform-ai (AKS escalation) and ai-governance (confidential escalation).
2. `.answers` with A1–A4. The Q&A status comment is updated in place; Q5 is still missing.
3. `.submit` is refused ("answer all required questions").
4. `.answers` with A5 and A6. Q&A is complete, and the spec preview is shown.
5. `.submit`, then approvals from `mia-maint` (maintain on `platform-infra`), `priya-ai` and `gwen-gov`. Execution runs in `agentic-aks`.
6. **Run 1:** `issueops agent render-k8s` produces the [ConfigMap and Job](output/scenario/execution/agent-job-1.yaml) (PSA restricted, workload identity). The runner returns `needs_input` with one question, which becomes **Q7**. The request goes back to `awaiting-answers`.
7. The requestor answers A7. The digest changes, so an old-style `.approve` is refused. The requestor runs `.submit` again, and all three approve again.
8. **Run 2:** the runner completes in 3 iterations. The runbook is posted as the deliverable, with token usage and critique. The issue stays open for follow-up.

## 2. Copilot cloud agent ([transcript](output/scenario-copilot-agent/transcript.md))

`gia-pay` delegates "idempotency keys for POST /v2/refunds" in `payments-api`:

- Another user's `.answers` is refused: only the requestor answers.
- The requestor answers in one comment. A1 (base branch) falls back to its default `main`, and A5 is `skip`.
- A `write` user's `.approve` is refused, because the rule needs maintain or admin. `hal-api` (maintain) approves.
- Execution renders [copilot-issue.md](output/scenario-copilot-agent/execution/copilot-issue.md) and [custom instructions](output/scenario-copilot-agent/execution/copilot-custom-instructions.md). It creates the issue in `payments-api`, assigned to `copilot-swe-agent[bot]`.

## 3. Agentic workflow catalog ([transcript](output/scenario-catalog/transcript.md))

A `dependency-hygiene` catalog run on `ml-pipelines`. `A2: maybe` fails the `yes/no` choice validation until it is corrected. The answers map to `workflow_dispatch` inputs: [workflow-dispatch-inputs.json](output/scenario-catalog/execution/workflow-dispatch-inputs.json).

## Agent results in simulation

Scenarios supply `agent_result` objects. The simulator encodes them into a runner log exactly as `cmd/agent-runner` prints them (`ISSUEOPS_RESULT_BEGIN` … base64 … `ISSUEOPS_RESULT_END`) and parses them back with `issueops agent result`. See `output/scenario/execution/runner-*.log`.
