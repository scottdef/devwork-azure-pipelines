# Building an IssueOps Self-Service Platform in GitHub Enterprise Cloud (CoolEngEnt / CoolEngOrg)

## TL;DR
- **IssueOps is the right pattern for CoolEngOrg, and it is buildable today.** Model each request as a GitHub issue created from an issue form, parse the body to JSON with `issue-ops/parser`, validate with `issue-ops/validator`, track state with labels via `issue-ops/labeler`, gate on `/approve`-style comments with `github/command`, execute in GitHub Actions authenticated as a GitHub App (or to Azure via OIDC), and report results back as issue comments — exactly the lifecycle documented by the `issue-ops` org (issue form → parse → validate → label state machine → approve/deny → execute → report → close).
- **All four use cases work, with named caveats.** Copilot billing budgets are now fully manageable via REST (`/organizations/{org}/settings/billing/budgets`, GA June 4, 2026) — though enterprise-scope budget endpoints reject GitHub App and fine-grained tokens (classic PAT required). Foundry model deployment is fully scriptable via `az cognitiveservices account deployment create` (Cognitive Services Contributor role; `--sku-capacity 1` = 1,000 TPM). Copilot cost↔output correlation is achievable via the Copilot usage-metrics reports joined to PR/commit REST data (a Go 1.21 + go-echarts CLI is provided). Agentic execution can use the Copilot coding agent, `github/gh-aw`, a Foundry Agent Service agent, or a custom agent on AKS.
- **Security is the hard part and is non-negotiable.** Use `issues`/`issue_comment` triggers (never `pull_request_target` with checkout of untrusted code), pass issue bodies as environment variables (never inline `${{ }}` interpolation into shell), pin actions to commit SHAs, use short-lived GitHub App installation tokens and Azure OIDC federated credentials with least privilege, and re-validate every issue body on every transition because bodies and labels can be edited at any time.

## Key Findings

1. **IssueOps reference lifecycle** (issue-ops.github.io/docs): issue form → parse → validate → label-based state machine (`opened → validated → submitted → approved/denied → closed`) → approval → execute → report. GitHub's engineering blog frames it explicitly as a state diagram; labels are the state nodes and comment commands are the transitions.
2. **Core actions**: `issue-ops/parser` (body→JSON), `issue-ops/validator` (schema + custom JS/ESM validators), `issue-ops/labeler` (state), `github/command` (command detection + allowlist/permission gating), `actions/create-github-app-token` (elevated cross-repo/org tokens), `peter-evans/create-or-update-comment` (feedback).
3. **Copilot moved to usage-based billing on June 1, 2026** — per The GitHub Blog, "all GitHub Copilot plans will transition to usage-based billing on June 1, 2026. Instead of counting premium requests, every Copilot plan will include a monthly allotment of GitHub AI Credits… Usage will be calculated based on token consumption, including input, output, and cached tokens." Budget lifecycle is now API-managed (GA June 4, 2026): "You can now manage the full lifecycle of budgets via API. Previously, budgets could only be managed through the UI. Now, you can programmatically create, update, and delete budgets, as well as adjust the budget amount and alert notifications."
4. **Copilot metrics**: the legacy `/orgs/{org}/copilot/metrics` endpoint sunset April 2, 2026 (per the GitHub Changelog "Closing down notice of legacy Copilot metrics APIs": "The Copilot Metrics API will sunset on April 2nd, 2026. We strongly recommend transitioning any existing workflows to the latest Copilot usage metrics endpoints"). Use the reports endpoints (`/orgs/{org}/copilot/metrics/reports/…`) returning signed NDJSON links; team-level metrics come from joining the user-teams report to the per-user usage report.
5. **Foundry deployment**: `az cognitiveservices account deployment create --sku-name GlobalStandard --sku-capacity N` where capacity 1 = 1,000 TPM (Microsoft Learn: `--sku-capacity 10` creates "a 10K TPM limit"). RBAC: Cognitive Services Contributor (control-plane action `Microsoft.CognitiveServices/accounts/deployments/write`). Authenticate from Actions via `azure/login` OIDC federated credentials.
6. **Agentic execution**: assign issues to the Copilot coding agent (`copilot-swe-agent[bot]` — requires a PAT because "GitHub Copilot is billed at the user level"); or use `github/gh-aw` GitHub Agentic Workflows (technical preview Feb 13, 2026; advanced to public preview June 11, 2026), GitHub Models, a Foundry Agent Service agent, or a custom agent on AKS with `kubectl 1.30`.

## Details

### 1. Executive overview of IssueOps

IssueOps uses GitHub Issues as the user interface, issue forms as structured input, and GitHub Actions as the automation backend. GitHub's engineering blog frames an IssueOps workflow as a **state machine**: "IssueOps can be thought of as a state diagram where an issue transitions through different states in response to events and conditions." Common states are `opened`, `submitted`, `approved`, `denied`, `closed`, all tracked with labels.

**Why it fits CoolEngOrg**: no custom web app to build or operate; every request is transparent and auditable in the issue timeline; the request, the approval trail, and the audit log live in one place; and it reuses existing GitHub identity, teams, CODEOWNERS, and Actions. As a CKA-minded platform engineer, treat the issue as the declarative "desired state" object and the workflows as the controllers reconciling it — labels are the status subresource.

**Reference architecture** (the canonical issue-ops lifecycle):

```
[User] --opens issue form--> [Issue created]
   |
   v
[issues: opened/edited workflow]   (the "issue workflow")
   - issue-ops/parser   : body -> JSON
   - issue-ops/validator: JSON -> valid? (schema + custom validators)
   - issue-ops/labeler  : add issueops:validated (or issueops:invalid)
   - post comment with parsed summary + instruction to `.submit`
   |
   v
[issue_comment: created workflow]  (the "comment workflow" - the main driver)
   - github/command detects `.submit` / `.approve` / `.deny`
   - RE-parse + RE-validate (bodies can be edited!)
   - check state label, check user permissions (allowlist / team)
   - process the command (call GitHub/Azure APIs as a GitHub App / via OIDC)
   - issue-ops/labeler : transition state
   - comment result
   |
   v
[approved] --> execute (create repo / adjust budget / deploy model / run agent)
           --> verify --> report --> close
```

**Diagram description**: a three-lane swimlane diagram — *Requestor* (opens form, answers questions, runs `.submit`), *Approver* (runs `.approve`/`.deny`), and *Automation / GitHub Actions* (parse, validate, label, execute, verify, comment). Rounded rectangles are states; arrows are transitions labelled with the triggering event or command. The deny transition is *guarded* (requires an authorized `.deny`); the approve→execute transition is *unguarded* — once the issue is approved it proceeds immediately, which the IssueOps docs call an "unguarded transition."

### 2. Prerequisites and org/enterprise setup

**Repository layout** — a dedicated `CoolEngOrg/issueops` repository; forking `issue-ops/self-service` is a strong starting point. That template repo's README instructs you to: fork it into an org you manage, enable GitHub Pages, configure GitHub Actions, create a GitHub App and install it, optionally create an enterprise PAT (for enterprise-scoped IssueOps or multi-org use — "It is recommended to create a machine user account to use for this token!"), run the "Create IssueOps Labels" workflow, then run the "Continuous Delivery" workflow to deploy the portal. Supported operations are declared in an `issue-ops.json` file grouped by permission scope; some are gated (e.g., "Transfer a repository. Requires enterprise admin approval.").

Proposed tree:

```
CoolEngOrg/issueops/
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── config.yml
│   │   ├── copilot-budget-request.yml
│   │   ├── copilot-usage-report.yml
│   │   ├── foundry-model-deployment.yml
│   │   └── agentic-task-request.yml
│   ├── workflows/
│   │   ├── issue-opened.yml          # parse + validate + label + summary
│   │   ├── comment-router.yml        # github/command: .submit/.approve/.deny
│   │   ├── execute-copilot-budget.yml
│   │   ├── execute-copilot-report.yml
│   │   ├── execute-foundry-deploy.yml
│   │   ├── execute-agentic-task.yml
│   │   └── create-labels.yml
│   ├── validator/                    # custom validator scripts (ESM)
│   │   └── team.js
│   └── CODEOWNERS
├── cmd/copilot-roi/                  # Go 1.21 CLI (stdlib + go-echarts)
│   └── main.go
├── bicep/model-deployment.bicep
├── go.mod
└── README.md
```

**Required org settings (CoolEngOrg)**:
- Actions permissions: allow only the actions you pin; set default `GITHUB_TOKEN` to read-only and elevate per-workflow.
- Enable the "Copilot Metrics API access policy" for the org and set "Copilot usage metrics" = Enabled everywhere at the enterprise (both are required for the metrics endpoints).
- If workflows create/approve PRs, note "Allow GitHub Actions to create and approve pull requests" is subject to the enterprise→org→repo hierarchy: a setting disabled at a higher tier cannot be enabled lower down and will appear greyed out.

**GitHub App for elevated permissions** — the IssueOps docs are explicit: "If your IssueOps workflow requires access to anything outside of the repository it is running in, you will need to provide it with a token… GitHub Apps are a better choice because they can be scoped [to least privilege]." PATs are discouraged: "Since PATs are scoped to a single user, they are not recommended for use in IssueOps workflows." Create a GitHub App in CoolEngOrg, grant only the permissions needed (e.g., Organization Administration read/write for team management; Issues read/write; Organization Copilot metrics read; Contents read), install it on the `issueops` repo and org, store the **App ID/Client ID** as a variable (not sensitive — "The GitHub App ID is not a sensitive value and can be stored as a variable instead of a secret") and the **private key** as a secret.

Token generation (from the IssueOps GitHub App doc):

```yaml
permissions:
  contents: read
  id-token: write
steps:
  - uses: actions/create-github-app-token@<sha>   # pin to a commit SHA
    id: token
    with:
      app-id: ${{ vars.APP_ID }}
      private-key: ${{ secrets.APP_PRIVATE_KEY }}
      owner: ${{ github.repository_owner }}
```

`actions/create-github-app-token` calls `POST /app/installations/{installation_id}/access_tokens`; the token inherits the installation's permissions, is masked, is auto-revoked in the post step, and expires after 1 hour.

**Secrets/variables**: `APP_PRIVATE_KEY` (secret), `APP_ID`/`APP_CLIENT_ID` (variable). For enterprise budget management, a classic PAT with `manage_billing:copilot`/`admin:enterprise` held by an enterprise admin/billing manager, stored as `ENTERPRISE_BILLING_PAT`, is required because the enterprise budgets endpoints do not accept App tokens. For Azure: `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` (variables) — no client secret with OIDC.

**Environments**: create Actions Environments (e.g., `production-foundry`, `copilot-budgets`) with **required reviewers** as a native second approval gate for execute jobs. Environment protection rules pause the job until a designated reviewer approves in the UI — a first-class deployment audit record and a strong complement to comment-based approvals.

**Teams**: `@CoolEngOrg/issueops-approvers`, `@CoolEngOrg/platform-admins`, `@CoolEngOrg/copilot-admins`. **CODEOWNERS** on the `issueops` repo protects workflow and template files so changes require platform-admin review; the ApproveOps/allow-list-as-code patterns show CODEOWNERS auto-requesting the right reviewers. **Branch protection / rulesets** on `main`: require CODEOWNERS review, require status checks, disallow direct pushes.

### 3. Issue forms deep-dive

Issue forms are YAML files in `.github/ISSUE_TEMPLATE/*.yml` following GitHub's form schema (still noted as public preview and subject to change in GitHub Docs). Supported top-level keys include `name`, `description`, `title`, `labels`, `projects`, `assignees`, and `body`. The `body` is an array of elements; each has a `type` (`markdown`, `input`, `textarea`, `dropdown`, `checkboxes`), an `id`, `attributes` (label, description, placeholder, options, `multiple`, `render`), and `validations` (e.g., `required: true`).

**Critical parsing insight** (IssueOps issue-form doc): when a form is submitted, each input is rendered to Markdown with the label as an `###` heading and the answer beneath it; `markdown`-type fields are NOT included in the submitted body. `issue-ops/parser` relies on this: "Issues submitted using issue forms use a structured Markdown format. So long as the issue body is not heavily modified by the user, we can reliably parse the issue body into a JSON object." Example — this form:

```yaml
name: New Repo Request
description: Submit a request to create a new GitHub repository
title: '[Request] New Repository'
labels:
  - issueops:new-repository
body:
  - type: input
    id: name
    attributes:
      label: Repository Name
    validations:
      required: true
  - type: dropdown
    id: visibility
    attributes:
      label: Repository Visibility
      options: [private, public]
    validations:
      required: true
```

…produces this body Markdown:

```markdown
### Repository Name

octorepo

### Repository Visibility

public
```

…which the parser converts to JSON. The parser inputs are `body` (default `${{ github.event.issue.body }}`), `issue-form-template` (e.g. `example.yml`), and `workspace`; outputs are `json` (full JSON string) and `parsed_<key>` (per field). `checkboxes` become comma-separated selected options (`- [X]` = selected).

**parser + validator usage** (comment-workflow doc):

```yaml
- name: Parse Issue Body
  id: parse
  uses: issue-ops/parser@<sha>
  with:
    body: ${{ github.event.issue.body }}
    issue-form-template: copilot-budget-request.yml

- name: Validate Issue Body
  id: validate
  uses: issue-ops/validator@<sha>
  with:
    issue-form-template: copilot-budget-request.yml
    issue-number: ${{ github.event.issue.number }}
    parsed-issue-body: ${{ steps.parse.outputs.json }}
```

The validator "supports validating the following details based on the issue forms template used to create the issue" out of the box (required fields, dropdown membership, etc.) and supports **custom validators**: a config file listing `validators: [{ field, script }]` where each script is ESM (as of v2 the action is ESM-only — CommonJS must be converted). A `team.js` custom validator confirming a team name is a real GitHub Team needs a `read:org` token passed via `github-token` (use `actions/create-github-app-token`), and reads an `ORGANIZATION` env var for the org scope. Complete templates for each use case are in the Appendix.

### 4. Use case A — Approval gating

The IssueOps approval model centers on `github/command`, which: (1) detects a command keyword in a comment (e.g., `.approve`), (2) adds a reaction to acknowledge receipt, (3) confirms the command is allowed to run (authorized user, required checks/reviews), and (4) exposes a `continue` output that gates subsequent steps. Standard commands: `.submit`, `.approve`, `.deny`.

**(a) Named GitHub team** — `allowlist` with a team slug plus a `read:org` token (`allowlist_pat`):

```yaml
- uses: actions/create-github-app-token@<sha>
  id: token
  with:
    app-id: ${{ vars.APP_ID }}
    private-key: ${{ secrets.APP_PRIVATE_KEY }}
    owner: ${{ github.repository_owner }}
- name: Approve Command
  id: approve
  uses: github/command@<sha>
  with:
    command: .approve
    allowlist: CoolEngOrg/issueops-approvers
    allowlist_pat: ${{ steps.token.outputs.token }}
```

An alternative is the **ApproveOps** action (`joshjohanning/approveops`), which checks whether someone in a named team commented `/approve` and lists team membership using an App token:

```yaml
- uses: joshjohanning/approveops@v4
  id: check-approval
  with:
    token: ${{ steps.app-token.outputs.token }}
    approve-command: '/approve'
    team-name: 'approver-team'
    fail-if-approval-not-found: true
```

**(b) Individual users** — `allowlist: octocat,mona`.

**(c) Roles** — `github/command`'s `permissions` input restricts by repo role: `permissions: admin,maintain` (by default write/maintain/admin can run commands). For org-owner or custom-org-role checks, call REST in an `actions/github-script` step with the App token (e.g., `GET /orgs/{org}/memberships/{username}` and test `role == 'admin'`, or the org custom-roles endpoints).

**(d) Requestor confirmation** — add a `checkboxes` confirmation field (`required: true`) and/or require the requestor to comment `.submit`; compare `github.event.comment.user.login` to `github.event.issue.user.login` to enforce that only the requestor confirms.

**Alternative — GitHub Environments with required reviewers**: put the execute step in a job with `environment: production-foundry`; the job pauses until a required reviewer approves in the UI. Strong, GitHub-native, and audit-friendly.

**State-tracking security note** from the docs: anyone with issue access can edit labels, so "Labels are good for state tracking, but should not be used to determine if a request is valid!" Always re-parse and re-validate on every comment event. The complete approval router is in the Appendix (`comment-router.yml`).

### 5. Use case B — Self-service GitHub Copilot

#### B.1 Requesting a Copilot budget increase

**Background**: Copilot moved to **usage-based billing on June 1, 2026** — premium request units (PRUs) were replaced by **GitHub AI Credits**, "calculated based on token consumption, including input, output, and cached tokens." Historically, "By default, every enterprise has a $0 budget for the Premium Request SKU," so requests were rejected once the included allowance was exhausted unless the budget was edited/deleted; account-level $0 budgets for Enterprise/Team were being removed beginning Dec 2, 2025.

**What can be automated via API**: budget management is now fully programmatic. Per the June 4, 2026 GitHub Changelog: "You can now manage the full lifecycle of budgets via API. Previously, budgets could only be managed through the UI. Now, you can programmatically create, update, and delete budgets, as well as adjust the budget amount and alert notifications." The endpoints are `GET/POST /organizations/{org}/settings/billing/budgets`, `GET/PATCH/DELETE /organizations/{org}/settings/billing/budgets/{budget_id}`, and the enterprise equivalents under `/enterprises/{enterprise}/settings/billing/budgets`. A create body:

```json
{
  "budget_amount": 30,
  "prevent_further_usage": true,
  "budget_scope": "user",
  "budget_entity_name": "",
  "budget_type": "BundlePricing",
  "budget_product_sku": "ai_credits",
  "budget_alerting": { "will_alert": false, "alert_recipients": [] },
  "user": "USERNAME"
}
```

Budget scopes are user, organization, cost center, and enterprise; budgets can monitor-only or `prevent_further_usage` (block). Alerts fire at 75%, 90%, and 100%. For a user-scoped budget you must include the `user` field (omitting it returns HTTP 400).

**Caveats to state plainly**:
- The **enterprise** budgets endpoints "do not work with GitHub App user access tokens, GitHub App installation access tokens, or fine-grained personal access tokens" — you need a classic PAT held by an enterprise admin/billing manager. That is why the enterprise-scope branch of the execute workflow uses `ENTERPRISE_BILLING_PAT`.
- For licensed products "a budget is for monitoring purposes only and will not prevent usage beyond the budget"; blocking (`prevent_further_usage`) "is not available for licensed-based products" — it applies to metered products (AI credits / premium requests).
- Because billing has migrated to AI Credits, prefer `budget_product_sku: ai_credits`; PRU-based semantics are historical.

**Pattern**: the form captures scope (user/org/cost-center), amount, enforcement, and justification; the issue-opened workflow validates; an approver from `@CoolEngOrg/copilot-admins` runs `.approve`; the execute workflow calls the budgets API (org scope with the App token; enterprise scope with the classic PAT) and comments the new budget ID and amount. If a request exceeds a threshold (recommend > $500/month), route it to the environment-gated job for a second UI approval.

#### B.2 Copilot usage reports on request

**Endpoints**: the legacy `/orgs/{org}/copilot/metrics` endpoint sunset **April 2, 2026** (per the GitHub Changelog "Closing down notice of legacy Copilot metrics APIs": "The Copilot Metrics API will sunset on April 2nd, 2026. We strongly recommend transitioning any existing workflows to the latest Copilot usage metrics endpoints"). It now returns 404. Use the **Copilot usage metrics** report endpoints, which return signed NDJSON download links:
- `GET /orgs/{org}/copilot/metrics/reports/organization-1-day?day=DAY`
- `GET /orgs/{org}/copilot/metrics/reports/organization-28-day/latest`
- `GET /orgs/{org}/copilot/metrics/reports/users-1-day` / `users-28-day/latest`
- `GET /orgs/{org}/copilot/metrics/reports/user-teams-1-day` and the enterprise equivalent — the **user-teams** report maps each licensed user to their teams; join it to the per-user usage report on `user_id` + `day` to produce **team-level** metrics.

Access requires the "Copilot usage metrics" policy Enabled everywhere; allowed principals: enterprise admins, org owners, billing managers, or an enterprise custom role with "View Enterprise Copilot Metrics." Fine-grained tokens need "Organization Copilot metrics" (read). Teams with fewer than five Copilot-seated users are excluded from the user-teams report. Use header `X-GitHub-Api-Version: 2026-03-10` for the new endpoints.

The (sunsetting) legacy schema is useful for field names — each day object contains `total_active_users`, `total_engaged_users`, and nested `copilot_ide_code_completions` (with `languages[]`, `editors[]`, and per-language `total_code_suggestions`, `total_code_acceptances`, `total_code_lines_suggested`, `total_code_lines_accepted`), plus `copilot_ide_chat`, `copilot_dotcom_chat`, and `copilot_dotcom_pull_requests`. Recent additions include `used_copilot_code_review_active/passive`, `used_agent`, and `used_copilot_coding_agent` at the user level.

**Billing/premium usage reports**: `GET /organizations/{org}/settings/billing/usage` (usage summary), plus the premium-request usage report and AI-credit usage report endpoints (org and user scope). The premium-request usage report "includes all premium request usage by user, both within and beyond the allowance."

#### B.3 Correlating AI cost to developer output

The analytically valuable step is joining Copilot cost (AI credits / premium-request spend / seat cost) to measurable output at the team or repo level: PRs merged, PR cycle time (open→merge), commits, additions/deletions (lines changed), and review turnaround. REST endpoints: `GET /repos/{owner}/{repo}/pulls?state=closed` (filter on `merged_at`), `GET /repos/{owner}/{repo}/pulls/{n}` for additions/deletions, `GET /repos/{owner}/{repo}/commits`, plus the Copilot metrics reports above.

**Interpretation guidance** (what the numbers *mean*): a healthy signal is *engaged* users (not just *active*) rising alongside merged-PR throughput and stable or falling cycle time — i.e., AI credits translating into shipped work. A red flag is spend and active users rising while merged PRs and cycle time are flat, which suggests exploration without delivery impact; segment by team (via the user-teams join) to find where enablement is needed. GitHub's own cohorting guidance warns that "a flat active-user count treats someone who occasionally accepts a code completion the same as someone who orchestrates multiple agent-driven workflows," so prefer engaged/cohort metrics over raw active counts.

**Go 1.21 CLI** (standard library + `go-echarts` only) pulls Copilot metrics + PR/commit data and renders an HTML report (full source in Appendix 9.8). It uses only `net/http`, `encoding/json`, `time`, `io`, `os`, `fmt` from the stdlib and `github.com/go-echarts/go-echarts/v2/{charts,opts,components}` for a combined bar+line page. The IssueOps workflow runs `go run ./cmd/copilot-roi`, uploads the HTML as an artifact, and comments a summary + artifact link back on the issue.

**Optional Grafana OSS 12 on AKS**: push derived metrics to a data source (Prometheus via a small exporter, or Postgres) and build a "Copilot ROI" dashboard. Deploy Grafana OSS 12 via the official Helm chart on AKS behind an internal ingress. This is optional visualization; the go-echarts HTML is the primary artifact.

### 6. Use case C — Self-service Foundry model deployment

**Issue form** captures: model name, model version, model format (`OpenAI` / `Microsoft` / partner), deployment type/SKU, capacity (TPM-in-thousands, or PTUs for provisioned), region, target Foundry project/resource + resource group, and intended agentic use.

**Deployment types / SKUs** (Microsoft Learn): valid `--sku-name` values include **Standard, GlobalStandard, DataZoneStandard, GlobalBatch, DataZoneBatch, ProvisionedManaged, GlobalProvisionedManaged, DataZoneProvisionedManaged**. Global Standard "uses Azure's global infrastructure to dynamically route traffic to available datacenters… provides the highest default quota and eliminates the need to load balance across multiple resources"; DataZone processes only within the US/EU/APAC data zone; Standard/regional processes in the deployment region; Provisioned reserves capacity (PTUs). "Global Standard is a common starting point for most workloads."

**RBAC**: to create a deployment you need the **Cognitive Services Contributor** role at the resource scope — "You need the Cognitive Services Contributor role or equivalent permissions for the Foundry resource." The underlying control-plane action is `Microsoft.CognitiveServices/accounts/deployments/write` (also granted by Cognitive Services OpenAI Contributor). The newer Foundry roles Foundry Owner / Foundry Account Owner also grant deploy; Foundry Project Manager may not — verify against the Foundry administrator guide. Assign the role at the narrowest scope (a single resource group).

**Quota**: quota is measured in TPM per subscription + region + model + deployment type; per Microsoft Learn, TPM "can be modified in increments of 1,000, and will map to the TPM and RPM rate limits enforced on your deployment." `--sku-capacity` maps directly to TPM: "A value of 1 equals 1,000 Tokens per Minute (TPM). A value of 10 equals 10k Tokens per Minute (TPM)" (the docs create "a 10K TPM limit" with `--sku-capacity 10`). For provisioned SKUs, capacity represents PTUs. Check available quota before deploying with `az cognitiveservices usage list --location <region>` (returns per-model/SKU records with `currentValue`/`limit`); discover deployable models/SKUs/default capacity with `az cognitiveservices account list-models` (e.g., `{ "name": "Phi-4-mini-instruct", "format": "Microsoft", "version": "1", "sku": "GlobalStandard", "capacity": 1000 }`). Require Azure CLI ≥ 2.60 (older CLI has a `--sku-capacity` recognition bug).

**OIDC auth from Actions**: create a Microsoft Entra app + service principal and a federated credential trusting GitHub's OIDC issuer (`https://token.actions.githubusercontent.com`), subject scoped narrowly — e.g., `repo:CoolEngOrg/issueops:environment:production-foundry`:

```bash
az ad app federated-credential create --id <APP_ID> --parameters '{
  "name": "issueops-foundry",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:CoolEngOrg/issueops:environment:production-foundry",
  "audiences": ["api://AzureADTokenExchange"]
}'
```

Assign the app Cognitive Services Contributor on only the target resource group. The workflow needs `permissions: id-token: write` and uses `azure/login` (no stored secret). Subject casing must match exactly, and environment-scoped subjects require the job to declare that environment (otherwise: "No matching federated identity record found").

**Execute workflow** (Appendix 9.6): after `.approve`, OIDC login → `az cognitiveservices account deployment create` (or `az deployment group create` with the Bicep template) → verify with `az cognitiveservices account deployment show … | jq '.properties.provisioningState'` (expect `"Succeeded"`) → retrieve the endpoint from `az cognitiveservices account show` (`properties.endpoint`, form `https://<resource-name>.openai.azure.com/`) → comment the deployment name + endpoint back on the issue.

**CLI create example**:
```bash
az cognitiveservices account deployment create \
  -g "$RG" -n "$ACCOUNT" \
  --deployment-name "$DEPLOYMENT" \
  --model-name "$MODEL" --model-version "$VERSION" \
  --model-format "$FORMAT" \
  --sku-name "$SKU" --sku-capacity "$CAPACITY"
```

**Intended agentic use**: if the deployment is destined for Foundry Agent Service, note the project uses a `PROJECT_ENDPOINT` and the deployment's name as `MODEL_DEPLOYMENT_NAME` when creating an agent.

### 7. Use case D — Agentic workflow driven by issue comments

Pattern: the workflow asks clarifying questions as issue comments; the requestor answers in comments (a structured convention or a follow-up comment "form"); the workflow parses answers, tracks readiness with labels, and once ready triggers an agent.

**Q&A loop with state**:
1. On issue open, post a numbered questions comment; label `issueops:awaiting-answers`.
2. On each `issue_comment`, parse the requestor's answers. A robust convention is a fenced ```json block or `Q1: … / Q2: …` lines parsed by an `actions/github-script` step or the Go CLI. Only the requestor's answers count (`comment.user.login == issue.user.login`).
3. When all required answers are present, `issue-ops/labeler` sets `issueops:ready` and removes `issueops:awaiting-answers`.
4. Trigger the agent.

**Agent options**:
- **GitHub Copilot coding agent**: assign the issue to Copilot (`copilot-swe-agent[bot]`). It adds a 👀 reaction, creates a `copilot/` branch and a draft PR, works in a GitHub Actions-powered environment, pushes commits, and requests review when done. Constraints: assignment "requires a Personal Access Token… because GitHub Copilot is billed at the user level" and "you generally cannot use a GitHub App for this" (use a PAT and the exact assignee string `copilot-swe-agent[bot]`); the agent only sees the issue title/description/comments present at assignment time, so post the finalized spec *before* assigning; agent PRs require human approval before CI runs.
- **GitHub Agentic Workflows (`github/gh-aw`)**: technical preview announced Feb 13, 2026 ("Workflows run with read-only permissions by default and use preapproved 'safe outputs' for write operations. The feature is available in technical preview through the gh aw CLI extension"), advanced to public preview June 11, 2026. Workflows are Markdown + YAML frontmatter compiled by `gh aw compile` into a `.lock.yml`; engines include Copilot, Claude, Codex, Gemini, Pi. Guardrails include SHA-pinned dependencies, tool allow-listing, network isolation, and human approval gates; `max-ai-credits` sets a per-run hard budget. Treat as preview.
- **Foundry Agent Service agent**: create a Foundry project, deploy a model, create an agent (portal or SDK/REST), set `PROJECT_ENDPOINT` and `MODEL_DEPLOYMENT_NAME`; the execute workflow authenticates via OIDC and calls the agent, streaming results back as comments. Foundry Agent Service "manages conversations, orchestrates tool calls, enforces content safety, and integrates with identity, networking, and observability."
- **Custom agent on AKS**: run a containerized agent on Azure Kubernetes Service; the workflow authenticates (OIDC + `az aks get-credentials`, or Workload Identity) and uses `kubectl 1.30` to apply a `Job` that runs the agent against the collected Q&A, then tails logs and comments results. Complete `execute-agentic-task.yml` in Appendix 9.10.

### 8. Operations

**Observability**: workflow run logs are the primary audit trail (every parse/validate/approve/execute/verify step). Optionally ship metrics to **Grafana OSS 12** on AKS (Helm-deployed) for dashboards of request volume, approval latency, execution success rate, and Copilot ROI. For `gh-aw`, OpenTelemetry exports traces and token/AI-credit data to OTLP backends (`gh aw logs` / `gh aw audit` surface the heaviest runs).

**Security hardening (mandatory)**:
- **Never use `pull_request_target` with checkout of untrusted PR code** — such workflows run with the base repo's `GITHUB_TOKEN` and secrets and are the classic "pwn request" escalation. Prefer `issues`/`issue_comment` triggers. GitHub Security Lab warns IssueOps `issue_comment` flows are vulnerable to TOCTOU attacks if they check out and execute PR code after an approval comment ("an attacker could quickly update the pull request with malicious code… executed by the workflow").
- **Prevent script/prompt injection**: never interpolate `${{ github.event.issue.body }}` / `.title` / comment bodies directly into `run:` shell steps. Pass them as env vars and reference `"$BODY"` — "Use workflow input as environment variables instead of interpolation." Treat every issue/comment body as untrusted, especially before feeding it to an LLM/agent; allow-list fields from the parsed JSON rather than passing raw text.
- **Least-privilege tokens**: default `GITHUB_TOKEN` to read-only; elevate per job. Use short-lived GitHub App installation tokens (1-hour, auto-revoked) scoped to the minimum permissions. Use Azure OIDC federated credentials (no stored secrets) scoped to a specific repo+environment and a single resource group.
- **Pin actions to full commit SHAs** (not tags) for supply-chain integrity.
- **Re-validate on every transition** — bodies and labels can change at any time.
- **Fork/permission considerations**: this platform runs in a private internal org repo; keep "Approving workflow runs from public forks" restrictive and control the fork-PR approval policy (`/orgs/{org}/actions/permissions/fork-pr-contributor-approval`).
- **Protect secrets**: "Any user with write access to your repository has read access to all secrets configured in your repository" — keep write access to `issueops` limited to platform admins, and use environment-scoped secrets for the most sensitive credentials (Azure, enterprise PAT).

**Testing templates locally**: validate form YAML against GitHub's form schema (a bad `type` yields errors like `body[0]`); use a scratch repo to submit each form and confirm parser output; run the validator with sample parsed JSON. Lint workflows with `zizmor` and CodeQL-for-Actions to catch injection/`pull_request_target` patterns.

**Troubleshooting**: parser returns empty fields → the user edited the heading structure (re-instruct, or fall back to a JSON block); Copilot metrics 404 → legacy endpoint sunset, switch to reports endpoints; "No matching federated identity record found" on `azure/login` → subject/casing mismatch or numeric-ID subject; Copilot agent "the token used to assign the agent doesn't have the necessary permissions" → use a PAT and assignee `copilot-swe-agent[bot]`; Foundry "Authorization failed" → assign Cognitive Services Contributor on the resource; `--sku-capacity` ignored → upgrade Azure CLI to ≥ 2.60.

### 9. Appendix

#### 9.1 File tree — see §2.

#### 9.2 Issue form — Copilot budget request (`.github/ISSUE_TEMPLATE/copilot-budget-request.yml`)

```yaml
name: Copilot Budget Request
description: Request a GitHub Copilot AI-credit / premium-request budget increase
title: '[Copilot] Budget request'
labels:
  - issueops:copilot-budget
body:
  - type: markdown
    attributes:
      value: Fill out this form to request a Copilot budget. It will be reviewed by @CoolEngOrg/copilot-admins.
  - type: dropdown
    id: scope
    attributes:
      label: Budget scope
      options: [user, organization, cost_center]
    validations:
      required: true
  - type: input
    id: entity
    attributes:
      label: Scope target (username / org login / cost center name)
    validations:
      required: true
  - type: input
    id: amount
    attributes:
      label: Monthly budget amount (USD)
      placeholder: "50"
    validations:
      required: true
  - type: dropdown
    id: enforcement
    attributes:
      label: Enforcement
      options: ["monitor only", "block when exceeded"]
    validations:
      required: true
  - type: textarea
    id: justification
    attributes:
      label: Business justification
    validations:
      required: true
  - type: checkboxes
    id: confirm
    attributes:
      label: Confirmation
      options:
        - label: I understand overages are billed to our account
          required: true
```

#### 9.3 Issue form — Foundry model deployment (`.github/ISSUE_TEMPLATE/foundry-model-deployment.yml`)

```yaml
name: Foundry Model Deployment
description: Request deployment of a model to a Microsoft Foundry resource
title: '[Foundry] Deploy model'
labels:
  - issueops:foundry-deploy
body:
  - type: input
    id: model
    attributes: { label: Model name (e.g. gpt-4o, Phi-4-mini-instruct) }
    validations: { required: true }
  - type: input
    id: version
    attributes: { label: Model version }
    validations: { required: true }
  - type: dropdown
    id: format
    attributes: { label: Model format, options: [OpenAI, Microsoft, Cohere, Meta, Mistral AI] }
    validations: { required: true }
  - type: dropdown
    id: sku
    attributes:
      label: Deployment type (SKU)
      options: [Standard, GlobalStandard, DataZoneStandard, ProvisionedManaged, GlobalBatch]
    validations: { required: true }
  - type: input
    id: capacity
    attributes: { label: Capacity (TPM in thousands; PTUs for provisioned), placeholder: "10" }
    validations: { required: true }
  - type: input
    id: region
    attributes: { label: Azure region }
    validations: { required: true }
  - type: input
    id: account
    attributes: { label: Foundry / Cognitive Services account name }
    validations: { required: true }
  - type: input
    id: resource_group
    attributes: { label: Resource group }
    validations: { required: true }
  - type: input
    id: deployment_name
    attributes: { label: Desired deployment name }
    validations: { required: true }
  - type: textarea
    id: use
    attributes: { label: Intended agentic use }
    validations: { required: true }
```

#### 9.4 Issue-opened workflow (`.github/workflows/issue-opened.yml`)

```yaml
name: IssueOps - Issue Opened
on:
  issues:
    types: [opened, edited, reopened]
permissions:
  contents: read
  issues: write
jobs:
  process:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<sha>
      - uses: actions/create-github-app-token@<sha>
        id: token
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}
      - name: Parse
        id: parse
        uses: issue-ops/parser@<sha>
        with:
          body: ${{ github.event.issue.body }}
          issue-form-template: copilot-budget-request.yml
      - name: Validate
        id: validate
        uses: issue-ops/validator@<sha>
        env:
          ORGANIZATION: CoolEngOrg
        with:
          issue-form-template: copilot-budget-request.yml
          issue-number: ${{ github.event.issue.number }}
          parsed-issue-body: ${{ steps.parse.outputs.json }}
          github-token: ${{ steps.token.outputs.token }}
      - name: Label validated
        uses: issue-ops/labeler@<sha>
        with:
          action: add
          issue_number: ${{ github.event.issue.number }}
          labels: issueops:validated
      - name: Comment next steps
        uses: peter-evans/create-or-update-comment@<sha>
        with:
          token: ${{ steps.token.outputs.token }}
          issue-number: ${{ github.event.issue.number }}
          body: |
            ✅ Request validated. An approver from @CoolEngOrg/copilot-admins can run `.approve` to proceed.
```

#### 9.5 Comment router (`.github/workflows/comment-router.yml`)

```yaml
name: IssueOps - Comment Router
on:
  issue_comment:
    types: [created]
permissions:
  contents: read
  issues: write
  id-token: write
jobs:
  route:
    if: ${{ !github.event.pull_request }}   # only issue comments
    runs-on: ubuntu-latest
    steps:
      - uses: actions/create-github-app-token@<sha>
        id: token
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}
      - name: Approve command
        id: approve
        uses: github/command@<sha>
        with:
          command: .approve
          allowlist: CoolEngOrg/copilot-admins
          allowlist_pat: ${{ steps.token.outputs.token }}
          reaction: eyes
      - if: ${{ steps.approve.outputs.continue == 'true' }}
        name: Re-parse (bodies can change)
        id: parse
        uses: issue-ops/parser@<sha>
        with:
          body: ${{ github.event.issue.body }}
          issue-form-template: copilot-budget-request.yml
      - if: ${{ steps.approve.outputs.continue == 'true' }}
        name: Set approved
        uses: issue-ops/labeler@<sha>
        with:
          action: add
          issue_number: ${{ github.event.issue.number }}
          labels: issueops:approved
```

#### 9.6 Execute — Copilot budget (`.github/workflows/execute-copilot-budget.yml`)

```yaml
name: IssueOps - Execute Copilot Budget
on:
  issues:
    types: [labeled]
permissions:
  contents: read
  issues: write
jobs:
  create-budget:
    if: ${{ github.event.label.name == 'issueops:approved' }}
    runs-on: ubuntu-latest
    environment: copilot-budgets   # required reviewers gate
    steps:
      - uses: actions/create-github-app-token@<sha>
        id: token
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}
      - uses: issue-ops/parser@<sha>
        id: parse
        with:
          body: ${{ github.event.issue.body }}
          issue-form-template: copilot-budget-request.yml
      - name: Create org budget via REST
        env:
          GH_TOKEN: ${{ steps.token.outputs.token }}     # org scope OK
          BODY_JSON: ${{ steps.parse.outputs.json }}
        run: |
          AMOUNT=$(echo "$BODY_JSON" | jq -r '.amount')
          gh api -X POST /organizations/CoolEngOrg/settings/billing/budgets \
            -f budget_amount="$AMOUNT" -F prevent_further_usage=true \
            -f budget_scope=organization -f budget_type=BundlePricing \
            -f budget_product_sku=ai_credits
      # Enterprise-scope budgets require a classic PAT (App/fine-grained tokens rejected):
      #   env: GH_TOKEN: ${{ secrets.ENTERPRISE_BILLING_PAT }}
      #   gh api -X POST /enterprises/CoolEngEnt/settings/billing/budgets ...
      - uses: peter-evans/create-or-update-comment@<sha>
        with:
          token: ${{ steps.token.outputs.token }}
          issue-number: ${{ github.event.issue.number }}
          body: "✅ Budget created."
```

#### 9.7 Execute — Foundry deploy (`.github/workflows/execute-foundry-deploy.yml`)

```yaml
name: IssueOps - Execute Foundry Deploy
on:
  issues:
    types: [labeled]
permissions:
  contents: read
  issues: write
  id-token: write
jobs:
  deploy:
    if: ${{ github.event.label.name == 'issueops:approved' }}
    runs-on: ubuntu-latest
    environment: production-foundry
    steps:
      - uses: actions/create-github-app-token@<sha>
        id: token
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}
      - uses: issue-ops/parser@<sha>
        id: parse
        with:
          body: ${{ github.event.issue.body }}
          issue-form-template: foundry-model-deployment.yml
      - uses: azure/login@<sha>
        with:
          client-id: ${{ vars.AZURE_CLIENT_ID }}
          tenant-id: ${{ vars.AZURE_TENANT_ID }}
          subscription-id: ${{ vars.AZURE_SUBSCRIPTION_ID }}
      - name: Pre-check quota, deploy, verify
        env:
          J: ${{ steps.parse.outputs.json }}
        run: |
          RG=$(echo "$J" | jq -r '.resource_group'); ACC=$(echo "$J" | jq -r '.account')
          DEP=$(echo "$J" | jq -r '.deployment_name'); MODEL=$(echo "$J" | jq -r '.model')
          VER=$(echo "$J" | jq -r '.version'); FMT=$(echo "$J" | jq -r '.format')
          SKU=$(echo "$J" | jq -r '.sku'); CAP=$(echo "$J" | jq -r '.capacity')
          REGION=$(echo "$J" | jq -r '.region')
          az cognitiveservices usage list --location "$REGION" -o table || true
          az cognitiveservices account deployment create -g "$RG" -n "$ACC" \
            --deployment-name "$DEP" --model-name "$MODEL" --model-version "$VER" \
            --model-format "$FMT" --sku-name "$SKU" --sku-capacity "$CAP"
          STATE=$(az cognitiveservices account deployment show --deployment-name "$DEP" \
                    -n "$ACC" -g "$RG" | jq -r '.properties.provisioningState')
          echo "provisioningState=$STATE"
          ENDPOINT=$(az cognitiveservices account show -n "$ACC" -g "$RG" | jq -r '.properties.endpoint')
          echo "ENDPOINT=$ENDPOINT" >> "$GITHUB_ENV"
      - uses: peter-evans/create-or-update-comment@<sha>
        with:
          token: ${{ steps.token.outputs.token }}
          issue-number: ${{ github.event.issue.number }}
          body: "✅ Model deployed. Endpoint: ${{ env.ENDPOINT }}"
```

#### 9.8 Bicep (`bicep/model-deployment.bicep`)

```bicep
param accountName string
param modelName string
param modelVersion string
param modelFormat string = 'OpenAI'
param skuName string = 'GlobalStandard'
param capacity int = 10   // 10 => 10K TPM

resource deployment 'Microsoft.CognitiveServices/accounts/deployments@2023-05-01' = {
  name: '${accountName}/${modelName}'
  sku: {
    name: skuName
    capacity: capacity   // capacity: 10 => 10K TPM limit
  }
  properties: {
    model: {
      format: modelFormat
      name: modelName
      version: modelVersion
    }
  }
}
```

Deploy with: `az deployment group create -g "$RG" --template-file bicep/model-deployment.bicep --parameters accountName=$ACC modelName=$MODEL modelVersion=$VER capacity=$CAP`.

#### 9.9 Go 1.21 Copilot ROI CLI (`cmd/copilot-roi/main.go`)

```go
// Package main renders a Copilot cost-vs-output HTML report.
// Uses only the Go standard library plus go-echarts.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

type dayMetric struct {
	Date              string `json:"date"`
	TotalActiveUsers  int    `json:"total_active_users"`
	TotalEngagedUsers int    `json:"total_engaged_users"`
	IDECompletions    struct {
		Languages []struct {
			Name                 string `json:"name"`
			TotalCodeLinesAccept int    `json:"total_code_lines_accepted"`
		} `json:"languages"`
	} `json:"copilot_ide_code_completions"`
}

type pr struct {
	Number    int        `json:"number"`
	MergedAt  *time.Time `json:"merged_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func ghGet(url, token string, v any) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d: %s", url, resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func main() {
	token := os.Getenv("GH_TOKEN")
	org := os.Getenv("ORG")
	owner := os.Getenv("REPO_OWNER")
	repo := os.Getenv("REPO_NAME")

	// 1. Copilot metrics (legacy shape shown; swap to the reports NDJSON
	//    endpoints for production and download+decode the signed links).
	var metrics []dayMetric
	_ = ghGet(fmt.Sprintf("https://api.github.com/orgs/%s/copilot/metrics", org), token, &metrics)

	// 2. Merged PRs (paginate in production).
	var prs []pr
	_ = ghGet(fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=closed&per_page=100", owner, repo), token, &prs)

	var dates []string
	var linesAccepted []opts.BarData
	for _, m := range metrics {
		dates = append(dates, m.Date)
		total := 0
		for _, l := range m.IDECompletions.Languages {
			total += l.TotalCodeLinesAccept
		}
		linesAccepted = append(linesAccepted, opts.BarData{Value: total})
	}

	mergedCount := 0
	var cycleHours []opts.LineData
	var xs []string
	for _, p := range prs {
		if p.MergedAt != nil {
			mergedCount++
			cycleHours = append(cycleHours, opts.LineData{Value: p.MergedAt.Sub(p.CreatedAt).Hours()})
			xs = append(xs, fmt.Sprintf("PR-%d", p.Number))
		}
	}

	bar := charts.NewBar()
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title:    "Copilot lines accepted per day",
		Subtitle: fmt.Sprintf("Merged PRs in window: %d", mergedCount),
	}))
	bar.SetXAxis(dates).AddSeries("Lines accepted", linesAccepted)

	line := charts.NewLine()
	line.SetGlobalOptions(charts.WithTitleOpts(opts.Title{Title: "PR cycle time (hours)"}))
	line.SetXAxis(xs).AddSeries("Cycle hours", cycleHours)

	page := components.NewPage()
	page.AddCharts(bar, line)
	f, err := os.Create("copilot-roi.html")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := page.Render(io.MultiWriter(f)); err != nil {
		panic(err)
	}
	fmt.Println("wrote copilot-roi.html")
}
```

`go.mod`:
```
module github.com/CoolEngOrg/issueops

go 1.21

require github.com/go-echarts/go-echarts/v2 v2.4.0
```

#### 9.10 Execute — agentic task (`.github/workflows/execute-agentic-task.yml`)

```yaml
name: IssueOps - Execute Agentic Task
on:
  issues:
    types: [labeled]
permissions:
  contents: read
  issues: write
  id-token: write
jobs:
  run-agent:
    if: ${{ github.event.label.name == 'issueops:ready' }}
    runs-on: ubuntu-latest
    environment: agentic
    steps:
      - uses: actions/create-github-app-token@<sha>
        id: token
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}
          owner: ${{ github.repository_owner }}

      # Option A: assign to Copilot coding agent (needs a PAT; user-billed)
      - name: Assign Copilot
        if: ${{ contains(github.event.issue.labels.*.name, 'agent:copilot') }}
        env:
          GH_TOKEN: ${{ secrets.COPILOT_ASSIGN_PAT }}
          NUM: ${{ github.event.issue.number }}
        run: gh issue edit "$NUM" --add-assignee "copilot-swe-agent[bot]"

      # Option B: custom agent on AKS via kubectl 1.30
      - name: Run agent on AKS
        if: ${{ contains(github.event.issue.labels.*.name, 'agent:aks') }}
        run: |
          az login --service-principal -u ${{ vars.AZURE_CLIENT_ID }} \
            --tenant ${{ vars.AZURE_TENANT_ID }} --federated-token "$ACTIONS_ID_TOKEN_REQUEST_TOKEN" || true
          az aks get-credentials -g "$RG" -n "$AKS" --overwrite-existing
          kubectl version --client   # kubectl 1.30
          kubectl apply -f k8s/agent-job.yaml
          kubectl wait --for=condition=complete job/issue-agent --timeout=600s
          kubectl logs job/issue-agent > agent.log
        env:
          RG: ${{ vars.AKS_RG }}
          AKS: ${{ vars.AKS_NAME }}
```

#### 9.11 Custom validator (`.github/validator/team.js`, ESM)

```js
// Confirms the requested team exists in the org (ORGANIZATION env var).
export default async function ({ github, value }) {
  const org = process.env.ORGANIZATION
  try {
    await github.rest.teams.getByName({ org, team_slug: value })
    return { valid: true }
  } catch {
    return { valid: false, message: `Team '${value}' not found in ${org}` }
  }
}
```

#### 9.12 References
Inline throughout. Key sources: issue-ops.github.io/docs (setup, states-and-transitions, comment/issue workflow, GitHub App, issue form); github.com/issue-ops/{parser, validator, labeler, self-service}; github.com/github/command; github.com/actions/create-github-app-token; github.com/joshjohanning/approveops; GitHub Docs (issue forms schema, Copilot metrics & usage-metrics REST, billing budgets REST, Copilot premium requests / usage-based billing, Copilot coding agent, Actions secure-use & pull_request_target, OIDC in Azure); GitHub Blog & Changelog (usage-based billing June 1 2026; budget/usage APIs GA June 4 2026; legacy metrics sunset April 2 2026; team-level metrics; Agentic Workflows preview); github.com/github/gh-aw; Microsoft Learn (Foundry deployment via CLI & Bicep, deployment types, quotas, RBAC, Foundry Agent Service, azure/login OIDC); github.com/go-echarts/go-echarts.

## Recommendations

1. **Stage 1 (week 1–2) — foundation.** Fork `issue-ops/self-service` into `CoolEngOrg/issueops`. Create the least-privilege GitHub App, store `APP_ID`/`APP_PRIVATE_KEY`, run the labels workflow, and stand up the parse→validate→label→comment loop with ONE low-risk, read-only use case (Copilot usage report). *Benchmark:* a submitted form produces a validated issue and a report comment within a single workflow run.
2. **Stage 2 (week 3–4) — approvals + Copilot budgets.** Add `github/command` team-gated approvals and the budgets execute workflow (org scope with the App token). Add an environment with required reviewers for amounts above a threshold you set (recommend > $500/month). Provision `ENTERPRISE_BILLING_PAT` (classic, enterprise admin) only if you need enterprise-scope budgets — the API rejects App/fine-grained tokens there.
3. **Stage 3 (week 5–7) — Foundry deployment.** Configure Azure OIDC federated credentials scoped to `repo:CoolEngOrg/issueops:environment:production-foundry`; assign Cognitive Services Contributor on a single resource group; require Azure CLI ≥ 2.60. Ship the deploy workflow with a quota pre-check (`az cognitiveservices usage list`) and endpoint verification (`provisioningState == "Succeeded"`).
4. **Stage 4 (week 8+) — agentic + ROI analytics.** Add the Q&A comment loop and wire one agent backend first (Copilot coding agent for code tasks, or a Foundry Agent Service / AKS agent for domain tasks). Ship the Go ROI CLI; if leadership wants dashboards, deploy Grafana OSS 12 on AKS.
5. **Continuously**: pin all actions to SHAs, run `zizmor` and CodeQL-for-Actions in CI, and review CODEOWNERS-protected workflow files on every change.

**Thresholds that change the plan**: if the enterprise budget API remains App-token-incompatible for your compliance posture, keep enterprise budget changes UI-only and have the workflow instead assign the request to a human billing manager. If Copilot coding-agent PAT-based assignment violates your token policy, use `gh-aw`, GitHub Models, or a Foundry/AKS agent. If Foundry quota is insufficient in the requested region, the workflow should fail fast and comment the quota gap rather than partially deploy.

## Caveats

- **Preview / UI-only / token-restricted**: GitHub Agentic Workflows (`github/gh-aw`) was technical preview (Feb 13, 2026) and advanced to public preview (June 11, 2026); Copilot coding agent is in public preview subject to change; Foundry managed compute is public preview; issue forms are still labelled public preview in GitHub Docs. Copilot **enterprise** budget endpoints do **not** accept GitHub App or fine-grained tokens — a classic PAT is required; verify current behavior before automating enterprise-scope budgets.
- **Billing model in flux**: Copilot moved to usage-based AI Credits on June 1, 2026; PRU-based semantics are historical. Budget blocking (`prevent_further_usage`) applies to metered products, not licensed seats (for licensed products a budget is monitoring-only).
- **Deprecations**: the legacy `/orgs/{org}/copilot/metrics` endpoint sunset April 2, 2026 — use the reports endpoints. Team-level metrics are REST-only (no dashboard) and exclude teams with fewer than five seated users.
- **Foundry role nuance**: the doc-endorsed role is Cognitive Services Contributor (action `Microsoft.CognitiveServices/accounts/deployments/write`); the newer Foundry roles (Foundry Owner vs Project Manager) may differ on deploy permission — confirm against the Foundry administrator guide.
- **The Go CLI is illustrative**: the legacy metrics JSON shape is shown for clarity; for production, adapt the fetch to the NDJSON reports endpoints (download the signed links and decode line-by-line) and paginate PR/commit calls. Pin `go-echarts` to a specific version; verify the current v2 tag before building.
- **Security is your largest risk surface**: TOCTOU and script/prompt injection are real for IssueOps and agentic flows. The mitigations in §8 are mandatory, not optional.