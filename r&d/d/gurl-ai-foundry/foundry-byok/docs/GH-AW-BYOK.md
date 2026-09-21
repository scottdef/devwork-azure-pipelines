# GitHub Agentic Workflows with Azure AI Foundry (BYOK)

How the generated `gh-aw/` files map to the gh-aw reference, how to roll them
out, and what to watch for. Primary source:
<https://github.github.com/gh-aw/reference/azure-openai-byok/>.

## How it works

gh-aw's Copilot engine runs the Copilot CLI inside the AWF sandbox. Setting
`COPILOT_PROVIDER_BASE_URL` in `engine.env` switches it to BYOK mode: inference
goes to your Azure resource instead of GitHub's model routing. The provider
credential stays in the AWF API-proxy sidecar; the agent container only sees a
placeholder key. That is why gh-aw requires the credential in `engine.env`
rather than top-level `secrets:`.

This is configured **per workflow**. It is independent of the enterprise
custom-models registry used by Copilot Chat (see
[COPILOT-ENTERPRISE-BYOK.md](COPILOT-ENTERPRISE-BYOK.md)).

## The generated frontmatter, line by line

```yaml
engine:
  id: copilot
  model: "gpt-5.4-2026-03-05"            # Azure model ID, must be in GET /openai/v1/models
  env:
    COPILOT_PROVIDER_BASE_URL: "https://fdy-myass-dev-eus2.openai.azure.com/openai/v1"
    COPILOT_PROVIDER_API_KEY: ${{ secrets.FOUNDRY_DEV_API_KEY }}
    COPILOT_PROVIDER_MODEL_ID: "gpt-5.4" # deployment name, only when it differs from model
    COPILOT_PROVIDER_WIRE_API: responses # GPT-5 / o-series / codex only
network:
  allowed:
    - defaults
    - fdy-myass-dev-eus2.openai.azure.com
```

| Key | Rule the tool applies |
|---|---|
| `engine.model` | `model_id`, else `model` + `-` + `model_version`, else `model` |
| `COPILOT_PROVIDER_BASE_URL` | `{origin}/openai/v1`, literal. gh-aw documents that a literal URL is added to the firewall allow-list automatically (also for the threat-detection step), while a URL supplied through a secret or variable expression is not. The tool always emits a literal and lists the host under `network.allowed` anyway, so the intent is explicit in review |
| `COPILOT_PROVIDER_API_KEY` | `${{ secrets.<api_key_secret> }}`; omitted for `auth: entra` |
| `COPILOT_PROVIDER_MODEL_ID` | emitted only when the deployment name ≠ `engine.model` |
| `COPILOT_PROVIDER_WIRE_API` | `responses` when inferred or forced; otherwise omitted (CLI default is `completions`) |
| `sandbox.agent.model-fallback: false` | only with `disable_model_fallback`; gh-aw's documented workaround when the proxy rewrites the deployment name and Azure returns 404 |

Every data-driven scalar is double-quoted so a deployment named `5.4`, `true`
or `null` can never be re-typed by YAML. `${{ … }}` expressions are left bare,
as in GitHub's documentation.

## Rollout

1. **Describe** the deployments in `foundry/inventory.json`.
2. **Verify** against Azure before touching GitHub:
   `foundry-byok verify --inventory foundry/inventory.json --only-env dev --probe`
3. **Create the secret** named by `api_key_secret`, scoped as narrowly as you
   can (repository or environment, not organization-wide, for prod):
   `gh secret set FOUNDRY_DEV_API_KEY --repo CoolGitOrg/central-control`
4. **Generate**, including the smoke workflows:
   `foundry-byok generate --inventory foundry/inventory.json --out foundry/generated --workflows-dir .github/workflows`
5. **Compile and commit** both files for each workflow:
   `gh aw compile --strict` → commit `smoke-foundry-*.md` and `*.lock.yml`.
6. **Run the smoke test:** `gh aw run smoke-foundry-dev-fdy-myass-dev-eus2-gpt-5-4`.
   Success is a `noop` safe output containing `FOUNDRY_BYOK_OK`. Then
   `gh aw audit <run-id>`: the firewall section must show no blocked domains.
7. **Adopt:** merge `gh-aw/frontmatter.yml` into your real workflows' frontmatter
   (keep their own `permissions`, `tools`, `safe-outputs`) and recompile.
8. **Guard:** add the `foundry-byok-configs.yml` workflow so drift fails pull
   requests.

The smoke workflow is `workflow_dispatch`-only, has read-only permissions,
declares no safe outputs of its own, and instructs the agent to call `noop`.
A gh-aw run that ends without any safe-output call is treated as a failure,
which is exactly what you want from a connectivity test.

## Microsoft Entra ID (no stored key)

`auth: entra` with `tenant_id` and `client_id` generates:

```yaml
permissions:
  contents: read
  id-token: write
engine:
  id: copilot
  model: "gpt-5.4-2026-03-05"
  auth:
    type: github-oidc
    provider: azure
    azure-tenant-id: "<tenant-guid>"
    azure-client-id: "<client-guid>"
  env:
    COPILOT_PROVIDER_BASE_URL: "https://…/openai/v1"
    COPILOT_PROVIDER_MODEL_ID: "gpt-5.4-prod"
    COPILOT_PROVIDER_WIRE_API: responses
network:
  allowed: [defaults, <host>, login.microsoftonline.com]
```

Azure side, once per identity:

```bash
APP_ID=<client-guid>
RES_ID=$(az cognitiveservices account show -g rg-myass-prod -n fdy-myass-prod-eus2 --query id -o tsv)

# 1. Trust GitHub's OIDC issuer for the exact workflow identity.
az ad app federated-credential create --id "$APP_ID" --parameters '{
  "name": "gh-aw-central-control-main",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:CoolGitOrg/central-control:ref:refs/heads/main",
  "audiences": ["api://AzureADTokenExchange"]
}'

# 2. Data-plane permission only.
az role assignment create --assignee "$APP_ID" \
  --role "Cognitive Services OpenAI User" --scope "$RES_ID"
```

The `subject` must match how the workflow is triggered (`ref:refs/heads/<branch>`,
`environment:<name>`, or `pull_request`). A mismatch shows up as an
`AADSTS70021` error in the run log. For a user-assigned managed identity use
`az identity federated-credential create` instead.

Things to know:

- `tenant_id` / `client_id` are identifiers, not secrets; literals are what the
  gh-aw reference shows. The tool also accepts a single `${{ vars.NAME }}`
  expression and warns, because whether gh-aw accepts expressions there is not
  documented.
- `--strict` forbids write permissions in general. The gh-aw Entra example
  itself uses `id-token: write`, so it should be accepted; this was not tested
  here. If strict mode rejects it, compile that one workflow without `--strict`
  and raise it with the gh-aw maintainers.
- Sovereign clouds: set `entra_authority_host` (for example
  `login.microsoftonline.us`) so the right host lands in `network.allowed`.
- Enterprise custom models cannot use Entra. A deployment with `auth: entra`
  is listed under "Not registered" in the runbook. Override `auth` and
  `api_key_secret` on a single deployment if Chat needs it too (the example
  inventory's `gpt-5.4-chat` does this).

## Self-hosted runners, AKS and private endpoints

- The AWF firewall is a Squid proxy in a container on the runner. The runner
  therefore needs Docker and must itself be able to resolve and reach the
  Foundry host.
- With a **private endpoint** the hostname is unchanged
  (`{res}.openai.azure.com` resolves through `privatelink.openai.azure.com`).
  Nothing in the generated config changes. The runner's VNet must be linked to
  the private DNS zone; for runners on AKS that means the cluster's VNet, and
  CoreDNS forwarding to Azure DNS (the AKS default). Quick check from a runner
  pod: `kubectl exec -n <runner-ns> <pod> -- nslookup <res>.openai.azure.com`
  should return a private address.
- Actions Runner Controller with Docker-in-Docker needs gh-aw's
  `runner.topology: arc-dind`; add it to your real workflows (it is not part of
  the BYOK fragment). gVisor and sbx runtimes are incompatible with it.
- `runs_on` in the inventory becomes `runs-on:` in the smoke workflow. It was
  not checked against the gh-aw schema; `gh aw compile` will reject it if the
  key is wrong for your gh-aw version, in which case drop `runs_on` and set
  runners the way your gh-aw version documents.
- **An APIM or other gateway** in front of Foundry works for gh-aw: set
  `endpoint`, and the gateway host lands in `network.allowed`. (It does not
  work for the enterprise form; see that document.)

## Applying it to an existing pattern

Any workflow from your gh-aw field guide becomes a Foundry-backed one by
merging the fragment. IssueOps triage, for example:

```yaml
---
name: "Issue Triage"
on:
  issues:
    types: [opened, reopened]
permissions:
  contents: read
  issues: read
engine:                                   # ← from gh-aw/frontmatter.yml
  id: copilot
  model: "gpt-5.4-2026-03-05"
  env:
    COPILOT_PROVIDER_BASE_URL: "https://fdy-myass-prod-eus2.openai.azure.com/openai/v1"
    COPILOT_PROVIDER_API_KEY: ${{ secrets.FOUNDRY_PROD_API_KEY }}
    COPILOT_PROVIDER_MODEL_ID: "gpt-5.4-chat"
    COPILOT_PROVIDER_WIRE_API: responses
network:                                  # ← replaces `network: defaults`
  allowed:
    - defaults
    - fdy-myass-prod-eus2.openai.azure.com
tools:
  github:
    toolsets: [issues, repos]
safe-outputs:
  add-comment: { max: 1 }
  add-labels:  { allowed: [bug, enhancement, question], max: 3 }
---
```

Note `network: defaults` (a scalar) must become the `allowed:` list form.

Governance still applies: `GH_AW_DEFAULT_*` model variables set at org or
enterprise level are overridden by an explicit `engine.model` in frontmatter,
which is what BYOK needs, since a GitHub-hosted default model name would not
exist on your Azure resource.

## Claude on Foundry

For a `claude*` model the tool emits `COPILOT_PROVIDER_TYPE: "anthropic"` and
`COPILOT_PROVIDER_BASE_URL: https://{res}.services.ai.azure.com/anthropic`.
The Foundry endpoint is confirmed and `anthropic` is a documented provider
type, but gh-aw documents Azure BYOK only for the OpenAI-compatible endpoint,
and nothing here has exercised the combination. Treat the generated smoke
workflow as the test, and expect that Entra auth in particular may not work on
this path. Every such file carries a `# WARNING:` comment saying so.
