# copilot-foundry — IssueOps for Azure AI Foundry model deployments

Deploys models into Azure AI Foundry for **GitHub Copilot BYOK** in `CoolGitEnterprise` / `CoolGitOrg`,
driven from a GitHub issue form, executed with `az` CLI over OIDC. No stored cloud secrets.

```
issue form ──► issueops-foundry-model.yml ──/approve──► foundry-model-deploy.yml ──► az deploy ──► validate
   (UX)          parse · validate · plan                 resolve · OIDC · record · comment      ──► repository_dispatch ──► notify.yml
                                                                                                                         (Slack / webhook)
models/registry.yml ◄── the only thing you edit to add a model. Everything reads it.
```

## Layout

```
models/registry.yml                  accounts + models (9 alternatives: 2× Qwen, DeepSeek, GPT-4.1, Codestral, Llama 4, Phi-4, Grok, GPT-4.1 PTU)
models/catalog/<region>.json         weekly snapshot of the live Foundry catalog (generated)
deployments/<account>/<name>.json    one record per live deployment (generated, committed by the workflow)
.github/ISSUE_TEMPLATE/              deploy + decommission forms (dropdowns rendered from the registry)
.github/workflows/
  issueops-foundry-model.yml         intake → plan → /approve → dispatch   (GITHUB_TOKEN only)
  foundry-model-deploy.yml           resolve → deploy → validate → record  (OIDC)
  foundry-model-validate.yml         reusable: chat + streaming + tool-call
  foundry-model-decommission.yml     delete + record removal
  foundry-catalog-refresh.yml        weekly catalog snapshot, drift issue, re-render forms
  notify.yml                         repository_dispatch → Slack / HMAC-signed webhook
scripts/                             every workflow step is a script you can run locally
Makefile                             the front door: make models | deploy | validate | key | user-config
```

## Model kinds (why three deploy paths)

| kind | what | billing | `az` path |
|---|---|---|---|
| `serverless` | Foundry Models sold by Azure | per token, no idle cost | `az cognitiveservices account deployment create` |
| `provisioned` | same catalog, PTU reserved | per PTU-hour, idle billed | same command, `--sku-name ProvisionedManaged` |
| `managed` | Hugging Face weights on dedicated GPU | per GPU-hour, idle billed | `az rest PUT …/deployments/<name>` (preview shape — dry-run first) |

Default fallback for Copilot is `qwen3-32b` (serverless). `qwen3-coder-next` is managed compute; deploy it only
when you'll use it, and decommission it when you won't.

## One-time setup

```bash
az login                                                     # subscription owner
ORG=CoolGitOrg REPO=copilot-foundry scripts/bootstrap-azure-oidc.sh   # Entra app, federated creds, RBAC, env vars
gh api -X PUT repos/CoolGitOrg/copilot-foundry/environments/foundry-prod \
  -F 'reviewers[][type]=Team' -F 'reviewers[][id]=<foundry-approvers team id>'  # or in Settings → Environments
make bootstrap-labels
gh secret set GH_ADMIN_TOKEN --repo CoolGitOrg/copilot-foundry   # optional: classic PAT read:org → team-based /approve
gh variable set SLACK_ENABLED --body true; gh secret set SLACK_WEBHOOK   # optional
```

Foundry accounts themselves (`ais-coolgit-copilot-prod`, `-dev`) come from the Terraform stack in the
companion guide; this repo deploys *models into* them.

## Request flow

1. **New issue → "Foundry model: deploy"**. Pick model, account, optional SKU/capacity/version, tick options.
2. Bot parses the form (`scripts/parse-issue.sh`), validates against the registry (`resolve-model.sh --offline`),
   posts a plan table, labels `pending-approval`.
3. An approver comments **`/approve`** (team `foundry-approvers`, or OWNER/MEMBER fallback). The bot re-reads the
   *issue body* (never the comment) and dispatches `foundry-model-deploy.yml`.
4. Deploy runs under the account's GitHub Environment — required reviewers there are a **second gate**.
   `latest` is resolved from `az cognitiveservices model list -l <region>`; quota is pre-checked with
   `az cognitiveservices usage list`.
5. Validation hits the endpoint keyless (Entra) — chat, streaming, **and a tool-call turn** (the thing Copilot needs).
6. The record is committed to `deployments/`, the issue gets endpoint + next steps, `repository_dispatch` fires `notify.yml`.

Tick **Dry run** first; it prints the exact `az` command / REST body to the log and bounces the issue back to
`pending-approval`.

## After deploy: registering in Copilot (UI-only)

There is **no API** for BYOK key registration. Enterprise owner: *AI controls → Copilot → Custom models →
Add API key*, provider **Microsoft Foundry**, base URL = account endpoint, deployment = the deployment name,
key = `make key ACCOUNT=prod-eastus2`. Enable the model for `CoolGitOrg`. It then appears in the model picker in
VS Code, JetBrains, Copilot CLI and github.com chat. BYOK usage bills to Azure, not AI Credits.

Per-user CLI snippet: `make user-config ACCOUNT=prod-eastus2 DEPLOYMENT=qwen3-32b`.

## Adding a model

Edit `models/registry.yml`, run `make render` (or let the catalog workflow do it on push), open a PR.
CODEOWNERS routes it to `foundry-approvers`. Verify `format`/`name` with
`az cognitiveservices model list -l eastus2 --query "[?contains(model.name,'qwen')].[model.format,model.name,model.version]" -o table`.

## Gotchas (learned the hard way)

- **`GITHUB_TOKEN` and downstream triggers.** It cannot *cause* `push`/`issues`-driven runs, but `workflow_dispatch`
  and `repository_dispatch` are explicit exceptions — which is why IssueOps → deploy → notify chains on
  `GITHUB_TOKEN` alone. A PAT/App token is only needed for the team-membership check or cross-repo dispatch.
- **Ruleset-protected `main`.** The record commit uses `GITHUB_TOKEN`; add the Actions app to the ruleset bypass
  list, or swap in `actions/create-github-app-token` and bypass the App.
- **Managed-compute body shape is preview.** `deploymentTemplate` ids and the api-version move. Always dry-run,
  diff against current docs, then deploy. Idle GPU is billed.
- **Thinking models + Copilot.** Qwen/DeepSeek hybrid-thinking models can echo `reasoning_content` and 400 on
  multi-turn tool calls under `COPILOT_PROVIDER_TYPE=azure|openai`. Use non-thinking mode or an
  Anthropic-compatible route with `COPILOT_PROVIDER_TYPE=anthropic`.
- **`COPILOT_MODEL` is the deployment name**, not the catalog name. Base URL ends in `/openai/deployments/<name>/v1`.
- **Two endpoint URL shapes.** `validate-endpoint.sh` tries `/openai/deployments/<name>/chat/completions` then
  `/models/chat/completions` and reports which one answered; wire Copilot to that one.
- **No tool calling → not BYOK-eligible.** `phi-4` is in the registry as a chat-only example; `copilot_byok: false`
  makes the validator warn instead of fail.
- **Quota names.** `az cognitiveservices usage list` keys look like `<Format>.<SKU>.<model>`; if the pre-check finds
  no match it warns and proceeds — check the portal for new SKUs.
- **Issue-form parsing is label-based.** Renaming a field label in the template breaks `parse-issue.sh`. Change both.
- **`gh workflow run` inputs are strings.** Booleans arrive as `"true"`; the deploy workflow declares them
  `type: boolean` so `inputs.dry_run` compares cleanly.
