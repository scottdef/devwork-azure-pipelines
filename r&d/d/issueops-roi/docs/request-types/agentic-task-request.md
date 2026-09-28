# D. Agentic task request (`agentic-task-request`)

This request runs an **agentic workflow on request**. The task is described in the form, then made precise through **clarifying questions answered in issue comments**. It is approved as a whole, form plus answers, and executed on one of three backends:

| Backend | What happens | Data up to | Environment |
|---|---|---|---|
| `copilot-cloud-agent` | An issue with the spec and custom instructions is created in the target repository and assigned to `copilot-swe-agent[bot]`. The Copilot cloud agent opens a draft PR | internal | `agentic-copilot` |
| `agentic-workflow-catalog` | A pre-approved GitHub Agentic Workflow (`*.lock.yml`) in the target repo is dispatched with inputs mapped from answers | internal | `agentic-catalog` |
| `aks-foundry-agent` | The agent-runner Job on AKS drafts a document (runbook, design doc, analysis, test plan, ADR) with a Foundry deployment through Workload Identity. It runs a bounded draft → critique → revise loop | confidential | `agentic-aks` |

- Form: [.github/ISSUE_TEMPLATE/agentic-task-request.yml](../../.github/ISSUE_TEMPLATE/agentic-task-request.yml)
- Handler: [internal/agent](../../internal/agent/agent.go)
- Runner: [cmd/agent-runner](../../cmd/agent-runner/)
- Workflow: [execute-agentic-task.yml](../../.github/workflows/execute-agentic-task.yml)
- Examples: [examples/agentic-task-request](../../examples/agentic-task-request/)

## Q&A protocol

1. On open, the summary lists the question set for the chosen backend (or catalog entry) with an answer template. The state is `awaiting-answers`.
2. The **requestor** replies with a comment that starts with `.answers`:

   ```text
   .answers
   A1: runbook
   A2: Platform SRE on-call engineers
   A3: Production cluster runs 1.29 with two user node pools.
       Continuation lines belong to A3.
   A5: gpt-4.1-agents
   ```

   - Later answers replace earlier ones.
   - `skip` blanks an optional answer.
   - Fenced blocks are kept verbatim.
   - Answers are validated against choices, patterns and maximum lengths.
   - Only the requestor's answers count.
3. The bot updates one **Q&A status** comment in place. When everything required is answered, the phase becomes `validated` and a spec preview is shown.
4. `.submit` binds the digest (form body **and** answers). Approvers approve exactly that specification.
5. **Follow-up questions:** if the AKS agent needs more information, the run ends `needs_input`. Its questions are appended as `Q7`, `Q8`, and so on (a `questions` marker), and the request returns to `awaiting-answers`. New answers change the digest, so the request is re-submitted and re-approved before the next run. See the [runbook example](../../examples/agentic-task-request/output/scenario/transcript.md).

Question sets live in the registry (`settings.question_sets`). Catalog workflows map inputs to question ids (`settings.catalog.<name>.inputs`).

## Requestors and approval (default)

- **Requestor:** needs `write`, `maintain` or `admin` on the **target repository**.
- **Base rule:** one approver with `maintain` or `admin` on the target repository. The requestor never counts.
- **Escalations:** `aks-foundry-agent` requires `@CoolEngOrg/platform-ai`; `confidential` data requires `@CoolEngOrg/ai-governance`.
- **Repositories:** `repository_allow` (globs) minus `repository_deny` (`issueops`, `.github`, `enterprise-policies`).

## Safety design

- **The spec is data.** It contains validated form fields and validated answers only: never raw comments, titles of other comments, or bot text. The system prompt ([agent.system-prompt.md.tmpl](../../internal/tmpl/templates/agent.system-prompt.md.tmpl)) tells the model to treat instructions inside the spec as data.
- **Secrets are refused.** The form, answers and agent questions are scanned for tokens and keys (GitHub, AWS, Slack, Azure storage, JWT, PEM).
- **Classification limits** per backend. Only `confidential_deployments` may receive confidential data.
- **The AKS runner:**
  - has no GitHub credentials
  - runs with a read-only root filesystem, non-root, all capabilities dropped, PSA `restricted`, and DNS plus 443 egress only
  - has bounded iterations and deadline, and a quota on concurrent jobs
- **Copilot cloud agent:** it works on a branch and opens a **draft PR** that humans review. Repository rulesets still apply.
- **Catalog workflows** are pre-reviewed `gh aw` lock files, with their own safe-outputs and permissions. GitHub Agentic Workflows are in preview; treat them accordingly.

## Execution

| Step | copilot-cloud-agent | agentic-workflow-catalog | aks-foundry-agent |
|---|---|---|---|
| Gate + spec | `issueops gate`, `issueops agent spec` | same | same |
| Credentials | App token (read) + `COPILOT_AGENT_TOKEN` (machine user) | App token for the issueops and target repos with `actions: write` | App token (read) + Azure OIDC → AKS user + namespace RBAC |
| Action | `POST /repos/{o}/{r}/issues` with `assignees: [copilot-swe-agent[bot]]` and `agent_assignment` (target repo, base branch, custom instructions, custom agent); or the agent tasks API when `mode: task` | `POST /repos/{o}/{r}/actions/workflows/{wf}/dispatches` with the mapped inputs | `issueops agent render-k8s` (Go templates → ConfigMap + Job), `kubectl apply --dry-run=server`, `kubectl apply`, poll the Job, `kubectl logs`, `issueops agent result` |
| Result | Link to the new issue (and then the PR) | Link to the workflow | Deliverable in the comment (plus artifact), or follow-up questions, or failure |

### Agent runner protocol

The Job runs `/agent-runner --spec /spec/spec.json --system /spec/system-prompt.md --task /spec/task-prompt.md` with `FOUNDRY_ENDPOINT`, `FOUNDRY_DEPLOYMENT`, `AGENT_MAX_ITERATIONS` and `AGENT_TIMEOUT_SECONDS`.

1. It exchanges the projected service-account token for an Entra token for `https://cognitiveservices.azure.com/.default`, using the client-credentials flow with a federated assertion.
2. It calls `POST {endpoint}/openai/deployments/{deployment}/chat/completions?api-version=2024-10-21` in JSON mode, with `max_completion_tokens` and no temperature, so it works with o-series models.
3. It prints a base64 JSON result between `ISSUEOPS_RESULT_BEGIN` and `ISSUEOPS_RESULT_END`. `issueops agent result` takes the last block. Status is `completed`, `needs_input` or `failed`; the result also carries the deliverable, questions, critique, iterations, model and token usage.

Test locally without Azure:

```bash
go run ./cmd/agent-runner --spec spec.json --system sys.md --task task.md --replay replies.json
```

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| "the token used to assign the agent doesn't have the necessary permissions" | `COPILOT_AGENT_TOKEN` must be a user token of a Copilot-licensed user with write on the target repo |
| Catalog dispatch 404 | The lock file isn't on the target's default branch, or the App lacks `actions: write` there |
| Job rejected by admission | The namespace isn't PSA restricted-compatible, or quota is exhausted (`kubectl describe quota -n issueops-agents`) |
| Runner: workload identity error | The ServiceAccount annotation client ID or the federated credential subject doesn't match. The pod label `azure.workload.identity/use: "true"` is required |
| `needs_input` repeatedly | The spec lacks key facts. Answer the follow-ups completely; each round requires re-approval by design |
