# Troubleshooting

Start with `foundry-byok verify --probe`: it separates "Azure is wrong" from
"GitHub is wrong" in one step. If `curl.sh` works and the workflow does not,
the problem is in the gh-aw layer.

## gh-aw

| Symptom | Likely cause | Fix |
|---|---|---|
| Proxy returns `model not found`, but curl works | `engine.model` is not an ID from `GET /openai/v1/models` (usually the version suffix is missing) | Set `model_version`; `verify` prints the closest IDs |
| HTTP 404 from Azure on the inference call | Deployment name is not reaching Azure: either it differs from the model ID and `COPILOT_PROVIDER_MODEL_ID` is missing, or AWF model fallback rewrote it | The tool emits `MODEL_ID` whenever names differ; if 404 persists set `disable_model_fallback: true` |
| HTTP 400 "unsupported" / empty output on GPT-5 or o-series | Wire API is `completions` | Leave `wire_api` on `auto`, or set `responses` |
| HTTP 400 on gpt-4o / gpt-4.1 / gpt-oss | Wire API was forced to `responses` for a deployment that lacks it | Set `wire_api: completions` |
| Audit shows the Azure host blocked | Host not in `network.allowed`, often because the base URL came from a secret/variable expression | Use the generated literal URL and `network.allowed` block |
| Audit shows `login.microsoftonline.com` blocked | Entra auth without the authority host allowed | Regenerate with `auth: entra`; for sovereign clouds set `entra_authority_host` |
| `AADSTS70021` no matching federated identity | Federated credential `subject` does not match the trigger | Match `ref:refs/heads/<branch>`, `environment:<name>` or `pull_request` |
| HTTP 401 with Entra | Token audience wrong or role missing | Role *Cognitive Services OpenAI User* on the resource; scope `https://cognitiveservices.azure.com/.default` |
| HTTP 401 with API key | Wrong resource's key, or `disableLocalAuth=true` | `az cognitiveservices account show … --query properties.disableLocalAuth` |
| Key set in top-level `secrets:` is ignored | BYOK proxy only reads provider credentials from `engine.env` | Use the generated fragment unchanged |
| Run fails with no output at all | Agent finished without a safe-output call | Keep the `noop` instruction in the prompt |
| `gh aw compile` rejects `runs-on` | Key differs in your gh-aw version | Remove `runs_on` from the inventory and configure runners per your version's reference |
| `gh aw compile --strict` rejects `id-token: write` | Strict mode's no-write-permission rule | Compile that workflow without `--strict`; report upstream |
| Edits have no effect | `.lock.yml` not regenerated | `gh aw compile`, commit both files |
| DNS failure on a self-hosted runner | Private endpoint zone not linked to the runner's VNet | `nslookup {res}.openai.azure.com` from the runner must return a private IP |

## Copilot enterprise custom models

| Symptom | Likely cause | Fix |
|---|---|---|
| Form rejects the Deployment URL immediately | Hostname is not `*.openai.azure.com` / `*.services.ai.azure.com`, or the format is not the one the form expects | Use the native hostname; then try the runbook's fallback URLs and pin `enterprise_url_style` |
| Two models will not save under one key | Their deployment URLs differ | One key entry per URL; the runbook already groups this way |
| Model saved but not in the picker | Not enabled, or not granted to the organization | Added models → Configure → Enabled → Access |
| Works for some models only | Possibly a model without tool calling or streaming (required per the Copilot CLI BYOK page) | Test with a deployment that supports both |
| Requests never reach Azure | Public network access disabled or firewall restricts sources | See "Reachability" in the enterprise document |

## Direct API

| Symptom | Likely cause | Fix |
|---|---|---|
| 404 `DeploymentNotFound` on v1 | `model` in the body is the catalog name, not the deployment name | Send the deployment name |
| 404 on the legacy URL | Deployment name in the path is wrong, or `api-version` missing | Copy the URL from `urls.json` |
| 400 about `api-version` on `/openai/v1/...` | A legacy `api-version=YYYY-MM-DD` was appended to a v1 URL | v1 takes none (or `v1` / `preview`) |
| 400 about `max_tokens` on o-series / GPT-5 | Reasoning models want `max_completion_tokens` | The generated scripts send no limit for this reason |
| 404 on `/anthropic/v1/messages` | Wrong host | Claude is only on `{res}.services.ai.azure.com` |
| 429 | Deployment capacity | `verify` retries twice honouring `Retry-After`; raise TPM or retry later |

## The tool

| Symptom | Cause | Fix |
|---|---|---|
| `--inventory cannot be combined with single-deployment flags` | A flag would have been silently ignored | Put it in the inventory `defaults` |
| `unknown field` when loading the inventory | Typo; parsing is strict on purpose | Fix the field name (flag names with underscores) |
| `enterprise key … maps to two deployment URLs` | Same `enterprise_key_name` across different URLs | Give one a different name |
| `generate --check` exits 3 | Inventory changed without regenerating, or a generated file was edited | Run `generate` and commit |
| Template error `map has no entry` / `can't evaluate field` | Override template references a missing field (`missingkey=error`) | See `internal/foundry/types.go` for field names |
| A template containing `${{ secrets.X }}` fails to parse | `{{` starts a template action | Emit expressions from data, e.g. `{{.GhAw.APIKeyExpr}}` |
