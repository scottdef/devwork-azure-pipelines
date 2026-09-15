# copilot-foundry — Foundry Owner workflows

Two manually-triggered GitHub Actions workflows that deploy models into an Azure AI Foundry
account for **GitHub Copilot BYOK**, using a user-assigned managed identity that holds exactly
one Azure role: **Foundry Owner** on the account. Every step is inline bash over `az`.

| Workflow | Deployment name | What it deploys | Inputs |
|---|---|---|---|
| `deploy-coder-model.yml` | `coder` | Qwen3-Coder-Next on managed compute, falling back to the first serverless coder in the region (Qwen3-Coder-480B → DeepSeek-V3.1 → Codestral-2501) | `capacity` small/medium/large · `try_managed_compute` · `dry_run` · `post_key_in_issue` |
| `deploy-chat-fallback-model.yml` | `chat-fallback` | First available serverless chat model: qwen3-32b → DeepSeek-V3.1 → Llama 4 Maverick → gpt-4.1-mini | `dry_run` · `post_key_in_issue` |

Both end the same way: fetch the account key, validate chat + streaming + tool-calling **with that
key**, store it as the repo secret `FOUNDRY_BYOK_KEY` (when `GH_ADMIN_TOKEN` exists), and open an
issue labelled `foundry-byok` with everything an enterprise owner needs to register the model in
Copilot AI controls.

```
workflow_dispatch ─► azure/login (UAMI, OIDC) ─► probe perms ─► pick model ─► deploy ─► key+validate ─► secret ─► issue
                     no secrets stored          fail early     catalog-driven  az cli      curl ×3        gh        gh
```

## Why "Foundry Owner only" shapes the design

Foundry Owner (formerly *Azure AI Owner*) grants full control over `Microsoft.CognitiveServices/accounts`
and their projects — deployments, keys, catalog and quota reads — and **nothing** on
`Microsoft.MachineLearningServices`. Consequences baked into these workflows:

- Only the **new Foundry resource** (`azurerm_cognitive_account`, kind `AIServices`) is targeted. Legacy
  hub/project stacks need `az ml` and different roles; see `hub-workflows.zip` for that variant.
- Managed compute is *attempted*, never assumed: the role can issue the PUT but cannot grant GPU quota,
  so the coder workflow falls back to serverless when the PUT is rejected.
- Keys are read with `accounts/listKeys/action` and stored on the GitHub side; Key Vault is out of scope
  (the role has no Key Vault permissions).
- The `probe` step exercises every control-plane call up front and reports the missing action by name.

## Prerequisites

| Azure | GitHub |
|---|---|
| Foundry account `ais-coolgit-copilot-prod` in `rg-coolgit-copilot` with `local_auth_enabled = true` and a `custom_subdomain_name` (needed for the endpoint URL) | Repo `CoolGitOrg/copilot-foundry` with environment `foundry-prod` (add required reviewers — this is the approval gate) |
| UAMI with federated credential, subject `repo:CoolGitOrg/copilot-foundry:environment:foundry-prod` | Environment vars `UAMI_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` |
| Role assignment: Foundry Owner on the account | Optional secret `GH_ADMIN_TOKEN` (classic PAT, `repo`) so the key can be stored as an Actions secret |
| TPM quota for the chosen SKU in the region; H100 quota if you want managed compute | Label `foundry-byok` (the issue step degrades gracefully without it) |

`scripts/bootstrap-uami-foundry-owner.sh` does the Azure and GitHub half in one go and prints the
role's actions so you can confirm coverage during the rename rollout.

## Running

1. **Actions → Deploy — chat fallback model → Run workflow** with `dry_run = true`. Read the log: it prints
   the catalog decision, the quota line and the exact `az` command. Nothing is created.
2. Re-run with `dry_run = false`. Approve in the environment. ~2 minutes for serverless.
3. Open the created issue. Enterprise owner: **CoolGitEnterprise → AI controls → Copilot → Custom models →
   Add API key**, provider *Microsoft Foundry*, base URL and deployment name from the issue, key from the
   Actions secret (or `az cognitiveservices account keys list …`). Enable, grant CoolGitOrg.
4. For the coder model, same sequence. Dry-run first: managed compute bills per GPU-hour from the moment
   the deployment exists.

Users then pick the model in the Copilot picker (VS Code, github.com, CLI) or, for CLI, use the export
block in the issue. `COPILOT_MODEL` is the *deployment name* (`coder` / `chat-fallback`), never the
catalog model name — which is why the deployment names are stable across model swaps.

## Key handling

| Where the key goes | When | Who can read it |
|---|---|---|
| Workflow log | never (masked with `::add-mask::`) | — |
| Actions secret `FOUNDRY_BYOK_KEY` | when `GH_ADMIN_TOKEN` is set | workflows in this repo |
| Issue body | only when `post_key_in_issue = true` | anyone with repo read, plus notification emails and search |
| Step summary | never | — |

Default is off. Enterprise BYOK registration is a one-time paste by an owner; retrieving it with
`az cognitiveservices account keys list` at that moment is cheaper than living with it in an issue.
Rotate with `az cognitiveservices account keys regenerate --key-name key1` and re-run the workflow
(it re-validates and re-issues).

## Failure modes you will actually hit

| Symptom | Cause | Fix |
|---|---|---|
| probe: `missing …/listKeys/action` | role assignment missing or still propagating (≤10 min) | wait, or check `az role assignment list --assignee <client-id> --scope <account-id>` |
| probe: `disableLocalAuth=true` | Terraform `local_auth_enabled = false` | flip it; BYOK cannot use Entra |
| `none of […] is in the catalog` | region lacks all candidates | pick another region or add a candidate (developer guide) |
| managed compute `::warning::… falling back` | no H100 quota, template id drift, or preview API change | expected; serverless takes over. Request quota / update `MANAGED_TMPL` |
| `TPM quota exceeded` | `usage list` shows headroom < capacity | smaller `capacity`, or request an increase |
| validate: `no tool call` | model or route doesn't return `tool_calls` | model isn't BYOK-usable; move it below a candidate that is |
| validate: `400 reasoning_content…` | hybrid-thinking model echoing reasoning on a tool turn | the validator sends `/no_think`; for CLI users prefer non-thinking models or `COPILOT_PROVIDER_TYPE=anthropic` via a gateway |
| issue step: label error | `foundry-byok` label absent | first `gh issue create` fails, second (no label) succeeds — create the label to keep issues filterable |
| `gh secret set` 403 | `GH_ADMIN_TOKEN` lacks `repo` scope or is fine-grained without *Secrets: write* | classic PAT with `repo`, or a GitHub App token |

See [docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) for changing models, SKUs, accounts and validation.
