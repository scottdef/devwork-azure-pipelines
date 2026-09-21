# foundry-byok

Build the correct URL for a deployed Azure AI Foundry model, for every consumer
that needs one, and generate the GitHub Copilot "bring your own key" (BYOK)
configuration that goes with it.

Given a **model name** and a **deployment name**, the tool produces:

| You asked for | What you get |
|---|---|
| Complete URL for direct API calls (curl / code) | `url` command; per-deployment `curl.sh` and a stdlib-only `client.go` |
| Complete URL for Copilot BYOK (Copilot Chat and agentic workflows) | `url` command; `urls.json`; both values are different, see below |
| Configuration for GitHub Agentic Workflows (gh-aw) | `gh-aw/frontmatter.yml` fragment and a ready-to-compile smoke-test workflow |
| Configuration for Copilot custom models BYOK | `copilot-enterprise/custom-models-runbook.md` and `custom-models.json` |

Go 1.21, standard library only (no third-party modules, no `go.sum`).
`go-echarts` is permitted by the project rules but nothing here draws charts,
so it is not used.

## The one rule that causes most failures

Azure has three names for "the model", and each consumer wants a different one.

| Name | Example | Where it comes from |
|---|---|---|
| Model name | `gpt-5.4` | Foundry catalog |
| Azure model ID | `gpt-5.4-2026-03-05` | model name + version; what `GET /openai/v1/models` lists |
| Deployment name | `gpt-5.4`, `gpt5-prod`, anything | you chose it when deploying |

| Consumer | URL | Which name, and where |
|---|---|---|
| curl / code, v1 API | `https://{res}.openai.azure.com/openai/v1/chat/completions` or `/responses` | **deployment name** in the JSON body `model` |
| curl / code, legacy API | `https://{res}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version=…` | **deployment name** in the path; no `model` in the body |
| gh-aw and Copilot CLI | `COPILOT_PROVIDER_BASE_URL=https://{res}.openai.azure.com/openai/v1` | `engine.model` = **model ID**; `COPILOT_PROVIDER_MODEL_ID` = **deployment name** when they differ |
| Copilot SDK | `baseUrl: https://{res}.openai.azure.com/openai/v1/`, `type: "openai"` | **deployment name** |
| Claude on Foundry | `https://{res}.services.ai.azure.com/anthropic/v1/messages` | **deployment name** in `model` |
| Copilot enterprise custom models | "Deployment URL" field (format not documented by GitHub; see [docs](docs/COPILOT-ENTERPRISE-BYOK.md)) | **deployment name** as "Model ID" |

GPT-5 and o-series deployments additionally need
`COPILOT_PROVIDER_WIRE_API: responses`; the tool infers that from the model
name. Full detail and sources: [docs/URL-REFERENCE.md](docs/URL-REFERENCE.md).

## Build

```bash
make build            # bin/foundry-byok   (or: go build ./cmd/foundry-byok)
make all              # gofmt check, go vet, tests, build
make dist             # linux/amd64, linux/arm64, darwin/arm64, windows/amd64 + SHA256SUMS
make docker           # distroless, non-root image
```

`GOTOOLCHAIN=local` is exported by the Makefile so the build always uses the
pinned Go 1.21 and never downloads a newer toolchain.

## Quick start

One deployment, from flags:

```bash
foundry-byok url \
  --resource fdy-myass-dev-eus2 --environment dev \
  --model gpt-5.4 --model-version 2026-03-05 --deployment gpt-5.4 \
  --api-key-secret FOUNDRY_DEV_API_KEY
```

```text
DIRECT API (curl / code)
  v1 chat completions   https://fdy-myass-dev-eus2.openai.azure.com/openai/v1/chat/completions
  v1 responses          https://fdy-myass-dev-eus2.openai.azure.com/openai/v1/responses
  legacy chat ...       https://fdy-myass-dev-eus2.openai.azure.com/openai/deployments/gpt-5.4/chat/completions?api-version=2024-10-21
  request body          "model": "gpt-5.4"  (v1: deployment name; legacy: omit)

GITHUB AGENTIC WORKFLOWS (gh-aw) and COPILOT CLI
  COPILOT_PROVIDER_BASE_URL     https://fdy-myass-dev-eus2.openai.azure.com/openai/v1
  engine.model / COPILOT_MODEL  gpt-5.4-2026-03-05
  COPILOT_PROVIDER_MODEL_ID     gpt-5.4
  COPILOT_PROVIDER_WIRE_API     responses
  network.allowed               defaults, fdy-myass-dev-eus2.openai.azure.com

COPILOT ENTERPRISE / ORG CUSTOM MODELS (Copilot Chat, CLI, IDEs)
  Provider        Microsoft Foundry
  Deployment URL  https://fdy-myass-dev-eus2.openai.azure.com/openai/v1
  Model ID        gpt-5.4
```

`--format json` prints everything machine-readable; `--format env` prints
`KEY=value` lines you can append to `$GITHUB_OUTPUT`.

Many deployments, from an inventory:

```bash
foundry-byok validate --inventory examples/inventory.json
foundry-byok generate --inventory examples/inventory.json --out foundry/generated \
                      --workflows-dir .github/workflows
gh aw compile --strict          # then commit the .md and the .lock.yml files
```

Check the real resource before you ship (reads the key from the environment
variable named by `api_key_secret`, or a token from `AZURE_FOUNDRY_BEARER_TOKEN`):

```bash
export FOUNDRY_DEV_API_KEY="$(az cognitiveservices account keys list -g rg-myass-dev -n fdy-myass-dev-eus2 --query key1 -o tsv)"
foundry-byok verify --inventory examples/inventory.json --only-env dev          # free: lists models
foundry-byok verify --inventory examples/inventory.json --only-env dev --probe  # one tiny billable call each
```

Illustrative output when `model_version` was forgotten:

```text
DEPLOYMENT  CHECK             STATUS  DETAIL
gpt-5.4     model-id-listed   fail    "gpt-5.4" is not among the 41 model IDs the resource lists
                                        hint: closest IDs: gpt-5.4-2026-03-05. Set model_version ...
```

That failure is the usual cause of gh-aw's `model not found`, caught before a
workflow ever runs.

## Commands

| Command | Purpose | Notable flags |
|---|---|---|
| `url` | Print every URL and Copilot setting | `--format text\|json\|env` |
| `generate` | Render all files | `--out`, `--workflows-dir`, `--template-dir`, `--check` |
| `validate` | Resolve and render in memory, write nothing | `--strict` (warnings fail) |
| `verify` | Live checks against Azure | `--probe`, `--json`, `--timeout` |
| `templates` | List or export built-in templates | `--export DIR` |
| `version` | Print the version | |

Every command accepts either `--inventory FILE` (optionally `--only-env ENV`)
or the single-deployment flags. They cannot be mixed: a flag that would be
silently ignored is refused instead, and shared settings belong in the
inventory's `defaults`.

Exit codes: `0` ok, `1` error (including validation), `2` usage, `3` drift
found by `generate --check`, `4` a `verify` check failed.

### Single-deployment flags

`--resource` or `--endpoint`, `--model`, `--deployment` are required. Optional:
`--model-version`, `--model-id`, `--environment`, `--host-kind`
(`openai`\|`cognitiveservices`\|`services-ai`), `--surface`
(`auto`\|`openai-v1`\|`anthropic`), `--wire-api`
(`auto`\|`responses`\|`completions`), `--auth` (`api-key`\|`entra`),
`--api-key-secret`, `--tenant-id`, `--client-id`, `--entra-authority-host`,
`--legacy-api-version`, `--enterprise-url-style` (`v1`\|`deployment`\|`root`),
`--enterprise-key-name`, `--runs-on`, `--disable-model-fallback`.

## Inventory

JSON, parsed strictly (unknown fields are errors, so typos are caught).
Settings merge `defaults` → resource `settings` → deployment; the most specific
wins. `disable_model_fallback` is sticky (true at any level wins).

```json
{
  "version": 1,
  "defaults": { "auth": "api-key", "runs_on": "self-hosted, linux, x64" },
  "resources": [
    {
      "name": "fdy-myass-dev-eus2",
      "settings": { "environment": "dev", "api_key_secret": "FOUNDRY_DEV_API_KEY" },
      "deployments": [
        { "model": "gpt-5.4", "model_version": "2026-03-05", "deployment": "gpt-5.4" },
        { "model": "gpt-oss-120b", "deployment": "gpt-oss-120b" }
      ]
    }
  ]
}
```

Field names are the flag names with underscores. See
[examples/inventory.json](examples/inventory.json) for dev/uat/prod with Entra
auth, a Claude deployment and a per-deployment override.

Private endpoints, APIM gateways and sovereign clouds: set `endpoint` on the
resource (for example `https://fdy.openai.azure.us` or
`https://apim.contoso.com/ai`) and, for Entra in a sovereign cloud,
`entra_authority_host`.

## Output

```text
foundry/generated/
├── SUMMARY.md                      table of everything + scoped warnings
├── manifest.json                   every resolved value
├── copilot-enterprise/
│   ├── custom-models-runbook.md    what to type into the GitHub form, grouped per key entry
│   └── custom-models.json          the same, as desired-state data
└── <env>/<resource>/<deployment>/
    ├── urls.json
    ├── curl.sh                     models | chat | responses | legacy | all
    ├── client.go                   go run client.go "prompt"   (//go:build ignore)
    ├── copilot-cli.env             source it, then run `copilot`
    └── gh-aw/
        ├── frontmatter.yml         keys to merge into any agentic workflow
        └── smoke-foundry-<env>-<resource>-<deployment>.md
```

Output is deterministic: no timestamps, sorted input, byte-identical on every
run. Commit it, and let `generate --check` fail a pull request when someone
edits the inventory without regenerating (or hand-edits a generated file).
The tool version is recorded in `manifest.json` only, so upgrading the tool
changes that one file unless a template really changed.

## Templates

All files except the JSON are Go `text/template` templates embedded in the
binary.

```bash
foundry-byok templates --export foundry/templates     # copy the built-ins
$EDITOR foundry/templates/gh-aw-smoke.md.tmpl
foundry-byok generate --inventory ... --template-dir foundry/templates
```

Only the files present in `--template-dir` override; the rest fall back to the
built-ins. Templates run with `missingkey=error`, so a typo in a field name
fails the run instead of printing `<no value>`.

Per-deployment templates receive a `foundry.Resolved` (fields `.Spec`, `.Host`,
`.ModelID`, `.Surface`, `.WireAPI`, `.URLs`, `.GhAw`, `.Client`, `.Enterprise`,
method `.WarningsFor "gh-aw"`) plus `.ToolVersion`, `.KeyEnv`, `.EntraScope`,
`.ClientKind`, `.ClientEndpoint`, `.ModelsURLHint`. Bundle templates
(`SUMMARY.md`, the runbook) receive `.Items`, `.EnterpriseGroups`,
`.EnterpriseSkipped`. The authoritative list is
[internal/foundry/types.go](internal/foundry/types.go).

Quoting helpers, one per output syntax: `shq` (shell), `shvar` (shell
`${VAR…}`), `yamlq` (always-quoted YAML scalar), `yamlv` (same, but a lone
`${{ … }}` expression stays bare), `jsonq`, `goq`, `runsOn`.

GitHub expressions such as `${{ secrets.X }}` collide with template
delimiters. Do not type them into a template; emit them from data
(`{{.GhAw.APIKeyExpr}}`), which is how the built-ins do it.

## GitOps

[.github/workflows/foundry-byok-configs.yml](.github/workflows/foundry-byok-configs.yml)
is a drop-in workflow for the repository that owns your agentic workflows:

- **Pull request:** validate, fail on drift, then `gh aw compile --strict` and
  fail if a `.lock.yml` was not committed.
- **Manual / weekly:** `verify` one environment, gated by a GitHub Environment
  (so prod inherits your required reviewers), with results in the step summary.

## Security

- The tool never accepts a key or token as a flag. It handles secret *names*;
  `verify` reads values from the environment only.
- `verify` does not follow redirects, so a credential header cannot be
  replayed to another host, and no credential appears in any output or error.
- Generated files contain no secrets. `copilot-cli.env` reads the key from
  your shell when sourced.
- gh-aw keeps BYOK credentials out of the agent container; they must sit in
  `engine.env`, which the generated frontmatter does.
- An Azure resource key grants every deployment on that resource. For the
  enterprise form, prefer a resource that hosts only what you intend to expose.
- Output paths are confined to the output root; endpoint URLs with
  credentials, queries or (non-loopback) plain http are rejected.

## What is verified, and what is not

| Item | Status |
|---|---|
| v1, legacy, model-inference and Anthropic URL shapes; auth headers; Entra scope | From the Azure OpenAPI specs and the DIAL adapter README |
| gh-aw frontmatter keys and values | Match the gh-aw Azure BYOK reference; a golden test pins the documented example |
| Generated Go, shell, YAML, JSON | In the test suite: Go is parsed and gofmt-checked, shell passes `bash -n`, JSON is validated. During authoring only (not in the suite): the scripts and all three Go client variants were executed against a fake Azure server, and every YAML frontmatter was parsed with a YAML library |
| Enterprise "Deployment URL" format | **Not documented by GitHub.** Default `v1`; two fallbacks emitted; confirm once and pin `enterprise_url_style` |
| Enterprise hostname restriction | From a community report, not GitHub documentation |
| Claude via gh-aw (`COPILOT_PROVIDER_TYPE=anthropic` against Foundry) | **Untested inference.** The Foundry URL is confirmed; every such output carries a warning |
| `runs-on` in gh-aw frontmatter | Not confirmed against the reference; opt-in, and `gh aw compile` validates the schema |
| `id-token: write` under `gh aw compile --strict` | The gh-aw Entra example uses it; whether `--strict` accepts it was not tested |
| Dockerfile | Not built in the authoring environment |

No test in this repository calls a real Azure resource or a real GitHub
enterprise. Run `verify --probe` and the generated smoke workflow in dev first.

## Documentation

- [docs/URL-REFERENCE.md](docs/URL-REFERENCE.md): every URL shape, its auth and body, with sources
- [docs/GH-AW-BYOK.md](docs/GH-AW-BYOK.md): agentic workflows, API key and Entra ID
- [docs/COPILOT-ENTERPRISE-BYOK.md](docs/COPILOT-ENTERPRISE-BYOK.md): custom models for Copilot Chat, CLI and IDEs
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md): symptom → cause → fix

## Sources

1. Azure REST API specs, `specification/ai/data-plane/OpenAI.v1/azure-v1-v1-generated.json` and `ModelInference/preview/2024-05-01-preview/openapi.json`: <https://github.com/Azure/azure-rest-api-specs/tree/main/specification/ai/data-plane>
2. EPAM DIAL OpenAI adapter README: <https://github.com/epam/ai-dial-adapter-openai/blob/development/README.md>
3. gh-aw, How to use Azure OpenAI with Copilot BYOK: <https://github.github.com/gh-aw/reference/azure-openai-byok/>
4. gh-aw, AI Engines reference (BYOK variables): <https://github.github.com/gh-aw/reference/engines/>
5. GitHub Docs, Enabling custom models for Copilot in your enterprise: <https://docs.github.com/en/enterprise-cloud@latest/copilot/how-tos/administer-copilot/manage-for-enterprise/enable-custom-models>
6. GitHub Docs, Using your own LLM models in Copilot CLI: <https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-byok-models>
7. GitHub Docs, Copilot SDK BYOK: <https://docs.github.com/en/copilot/how-tos/copilot-sdk/auth/byok>
8. GitHub Community discussion 198472 (Foundry hostname validation): <https://github.com/orgs/community/discussions/198472>
9. github/copilot-cli issue 3208 (provider type `azure`): <https://github.com/github/copilot-cli/issues/3208>
