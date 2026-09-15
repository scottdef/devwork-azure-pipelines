# Developer guide — changing the Foundry Owner workflows

The workflows are deliberately flat: one job, inline bash, everything configurable at the top of the
file in `env:`. There is no shared library to learn. This guide walks the changes people actually make,
in the order they usually make them.

## 1. Anatomy of a workflow

```
on.workflow_dispatch.inputs   ← what the person running it can vary
permissions                   ← id-token (OIDC), contents:read, issues:write — nothing else
env                           ← EVERY hard-coded decision lives here
jobs.deploy.steps
  azure/login                 ← UAMI via federated credential
  probe                       ← permission + local-auth checks; emits location, endpoint, account_id
  pick                        ← catalog query; emits format/name/version (+ fallback set)
  deploy                      ← az cognitiveservices … or az rest PUT; emits kind/format/name/version/sku/capacity
  key                         ← listKeys, mask, validate with the key; emits chat_url, byok_base
  secret                      ← gh secret set (optional)
  issue                       ← gh issue create
  summary                     ← $GITHUB_STEP_SUMMARY
```

Steps talk to each other only through `$GITHUB_OUTPUT`. If you add a value one step needs from an
earlier one, `echo "name=value" >> "$GITHUB_OUTPUT"` there and read `${{ steps.<id>.outputs.name }}` here.
Nothing is shared via files between steps except `r.json`/`t.json` in the validator.

## 2. Change the model (most common)

### 2a. Serverless chat model — edit one line

`deploy-chat-fallback-model.yml`:

```yaml
CANDIDATES: "Qwen|qwen3-32b;DeepSeek|DeepSeek-V3.1;Meta|Llama-4-Maverick-17B-128E-Instruct-FP8;OpenAI|gpt-4.1-mini"
```

Format is `Format|Name`, candidates separated by `;`, **most preferred first**. The `pick` step deploys the
first one the region's catalog lists. To make Mistral Large the primary:

```yaml
CANDIDATES: "Mistral AI|Mistral-Large-2411;Qwen|qwen3-32b;OpenAI|gpt-4.1-mini"
```

Get exact `Format` and `Name` strings from the catalog — they are case-sensitive and publishers rename:

```bash
az cognitiveservices model list -l eastus2 \
  --query "[?contains(model.name,'Mistral')].{format:model.format,name:model.name,version:model.version,skus:join(',',model.skus[].name)}" -o table
```

Check the `skus` column contains `GlobalStandard` (or change `SKU:` — §3). Then dry-run.

Keep `gpt-4.1-mini` (or another OpenAI model) as the **last** candidate: OpenAI models exist on every
AIServices account, so the workflow can never end with "nothing available".

### 2b. Serverless coder fallback

`deploy-coder-model.yml`, same syntax:

```yaml
SERVERLESS_CANDIDATES: "Qwen|Qwen3-Coder-480B-A35B-Instruct;DeepSeek|DeepSeek-V3.1;Mistral AI|Codestral-2501"
```

### 2c. Managed-compute model (Hugging Face)

Two lines in `deploy-coder-model.yml`:

```yaml
MANAGED_MODEL: azureml://registries/azure-huggingface/models/qwen--qwen3-coder-next/labels/latest
MANAGED_TMPL:  azureml://registries/azure-huggingface/deploymenttemplates/qwen--qwen3-coder-next--h100/labels/latest
```

The model id is `<org>--<repo>` lower-cased. The template id encodes model + accelerator and is the part
that drifts; find the current one in the Foundry portal (model card → *Deploy* → *Managed compute* →
view template) and paste it. The `acceleratorType` in the `deploy` step (`H100_80GB`) must match the
template. To swap to another HF model, e.g. `Qwen/Qwen2.5-Coder-32B-Instruct`:

```yaml
MANAGED_MODEL: azureml://registries/azure-huggingface/models/qwen--qwen2.5-coder-32b-instruct/labels/latest
MANAGED_TMPL:  azureml://registries/azure-huggingface/deploymenttemplates/qwen--qwen2.5-coder-32b-instruct--a100/labels/latest
```

and change `acceleratorType` to `A100_80GB`. Always dry-run: the step prints the full PUT body.

### 2d. Add a model that needs a different endpoint route

The validator tries three routes in order and uses the first that returns 200:

```bash
for u in "$EP/openai/deployments/$DEPLOYMENT/chat/completions?api-version=2024-10-21" \
         "$EP/openai/v1/chat/completions" \
         "$EP/models/chat/completions?api-version=2024-05-01-preview"; do
```

If a new model only answers on a fourth shape, add it to that list. The chosen route becomes
`chat_url`/`byok_base` in the issue, so users automatically get the right one.

## 3. Change SKU or capacity

Serverless SKU and capacity are `env` values (`SKU`, `CAPACITY`) in the chat workflow, and the
`case` block in the coder workflow's `pick` step:

```bash
case "${{ inputs.capacity }}" in small) inst=1; tpm=50;; medium) inst=2; tpm=150;; large) inst=4; tpm=300;; esac
```

Rules:
- `GlobalStandard` — pay-per-token, capacity in **thousands of TPM**. Default.
- `DataZoneStandard` / `Standard` — same, region-bound. Only some models list them.
- `ProvisionedManaged` — capacity is **PTUs**, billed per hour whether used or not. Do not use as a fallback.
- Confirm the model lists the SKU (`model.skus[].name` in the catalog query above) or the PUT returns `InvalidSku`.

The quota pre-check builds its key as `<Format>.<SKU>.<Name>`; if you see
`quota: current=? limit=?` it means no matching entry — fine for new SKUs, but then the deploy is the check.

## 4. Change the deployment name

`DEPLOYMENT:` in `env`. Rules: lowercase, digits, hyphens, ≤64 chars. This is what users put in
`COPILOT_MODEL` and what the enterprise owner types into AI controls — keep it stable and generic
(`coder`, `chat-fallback`) so swapping the backing model is invisible to them. Also update `concurrency.group`
if two workflows could ever target the same name.

Re-running with a different model behind the same name is an in-place PUT; the chat workflow warns when it
is about to switch the backing model.

## 5. Change the target account / environment

`RESOURCE_GROUP`, `ACCOUNT` in `env`; `environment: foundry-prod` on the job. If you add a dev account:

1. Copy the workflow, set `environment: foundry-dev`, `ACCOUNT: ais-coolgit-copilot-dev`.
2. Create a federated credential on the UAMI with subject `repo:CoolGitOrg/copilot-foundry:environment:foundry-dev`
   (the subject is the environment name — a new environment is a new credential).
3. Assign Foundry Owner on the dev account; set the three `AZURE_*`/`UAMI_CLIENT_ID` vars on `foundry-dev`.

Or make the account an input: add a `choice` input and `env` lookups — but every option still needs its own
federated credential subject, which is why we didn't.

## 6. Change the RAI (content filter) policy

`RAI_POLICY: Microsoft.DefaultV2`. Alternatives: `Microsoft.Default`, a custom policy name created on the
account, or `Microsoft.Nil` (no filter — get sign-off). Custom policies are account-scoped:
`az cognitiveservices account list --query …` won't show them; use the portal or
`az rest --url ".../accounts/<acct>/raiPolicies?api-version=2025-06-01"`.

## 7. Change what "validated" means

The `key` step fails the run if any of these are false: HTTP 200 on a chat route, >1 SSE chunk when
streaming, a `tool_calls[0].function.name == "get_weather"` on a tool turn. These are Copilot's minimum
requirements. To relax (e.g. a chat-only model you don't intend for agent mode), change the tool-call
line from `exit 1` to a warning:

```bash
[[ $tools == true ]] || echo "::warning::no tool call — chat-only model"
```

The tool prompt contains `/no_think` so hybrid-thinking Qwen models answer in non-thinking mode; remove it
if a model treats it as literal text.

## 8. Change the issue

The body is a heredoc in the `issue` step. It uses `${{ steps.*.outputs.* }}` for facts and shell
`$(…)` for conditionals. Add a row to the table by adding a `| … | … |` line. Keep the
**Base URL / deployment / provider** trio — that's what the enterprise owner copies. To route the issue
to a person or project: `--assignee`, `--project` on `gh issue create`. To also post to Slack, add a
step with `curl` to `secrets.SLACK_WEBHOOK` after the issue step.

## 9. Add a third workflow (e.g. a reasoning model)

Copy `deploy-chat-fallback-model.yml`, then change: `name:`, `concurrency.group`, `DEPLOYMENT`,
`CANDIDATES`, and the issue title. That's the whole diff. Keep `permissions` as-is.

## 10. Test before you push

```bash
actionlint                                  # syntax + embedded-shell check (needs a git repo)
shellcheck -S warning scripts/*.sh
```

Then run the workflow with `dry_run = true`. Dry run performs the login, the probe, the catalog query and
the quota check for real, prints the exact `az` command or PUT body, and creates nothing — so it also
tells you whether the *identity* still works after a role or credential change.

For logic changes without Azure access, the `pick` step is pure bash + jq over a JSON catalog; stub `az`
on `$PATH` to return a canned catalog and run the step body locally:

```bash
mkdir -p /tmp/stub && printf '#!/bin/sh\necho "[{\\"model\\":{\\"format\\":\\"OpenAI\\",\\"name\\":\\"gpt-4.1-mini\\",\\"version\\":\\"1\\"}}]"\n' > /tmp/stub/az && chmod +x /tmp/stub/az
PATH=/tmp/stub:$PATH GITHUB_OUTPUT=/dev/stdout CANDIDATES="Qwen|qwen3-32b;OpenAI|gpt-4.1-mini" LOC=eastus2 bash -c '<paste the pick step body>'
```

## 11. Things not to change casually

- `permissions:` — `id-token: write` is the OIDC exchange; `issues: write` is the report. Adding
  `contents: write` or `secrets` does nothing useful here and widens the token.
- `::add-mask::` before any `echo` of the key. Removing it prints the key in the log for everyone with
  Actions read.
- `set -euo pipefail` at the top of each `run:`. The fallback logic in the coder workflow relies on
  explicit `if`/`||` handling around the managed-compute PUT; don't wrap that in `set +e`.
- The `probe` step. It looks redundant right up until a role rename or credential expiry, when it
  turns a confusing 403 halfway through a deploy into a one-line answer.
