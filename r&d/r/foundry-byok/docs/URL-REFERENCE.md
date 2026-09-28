# URL reference

Every URL the tool builds, what to send to it, and where the shape comes from.
`{origin}` is `https://{resource}.openai.azure.com` unless you chose another
`host_kind` or set `endpoint`.

## Hostnames

| `host_kind` | Origin | Notes |
|---|---|---|
| `openai` (default) | `https://{resource}.openai.azure.com` | The form every Copilot BYOK document uses for Foundry |
| `services-ai` | `https://{resource}.services.ai.azure.com` | Host of the Anthropic surface and the retired model-inference API |
| `cognitiveservices` | `https://{resource}.cognitiveservices.azure.com` | Same resource, older alias. Not accepted by the Copilot enterprise form |
| `custom` (set `endpoint`) | whatever you give | Private DNS names, APIM, sovereign clouds (`*.openai.azure.us`), local tests |

A Foundry (AI Services kind) resource answers on all three Azure suffixes; a
classic Azure OpenAI-kind resource may not have the `services.ai` name, which
`verify` will reveal as a DNS or 404 failure. With a private
endpoint the *name* stays the same and resolves privately, so nothing here
changes; only DNS on the caller does.

If you paste an endpoint that already ends in `/openai/v1`, `/openai`,
`/anthropic…` or `/models`, the suffix is stripped and rebuilt.

## OpenAI-compatible v1 API (recommended)

Source: `azure-v1-v1-generated.json`. Server template `{endpoint}/openai/v1`.

| Operation | Method and URL |
|---|---|
| Chat completions | `POST {origin}/openai/v1/chat/completions` |
| Responses | `POST {origin}/openai/v1/responses` |
| Embeddings | `POST {origin}/openai/v1/embeddings` |
| List models | `GET {origin}/openai/v1/models` |
| One model | `GET {origin}/openai/v1/models/{model}` |

- **No `api-version` is needed.** The spec declares it optional with values
  `v1` (default) and `preview`. The DIAL README likewise notes that API version
  does not apply to v1 and Responses upstreams.
- **`model` is required in the body and carries the deployment name.** The DIAL
  README's sequence diagram shows the adapter posting to the v1 URL with
  `model` set to the upstream deployment ID.
- **Non-OpenAI Foundry models** (gpt-oss, Mistral, Llama, Grok, DeepSeek…) are
  served on this same endpoint; the DIAL README lists the Azure OpenAI endpoint
  as the current route for them.

```bash
curl -sS "https://$RES.openai.azure.com/openai/v1/chat/completions" \
  -H "api-key: $KEY" -H "Content-Type: application/json" \
  -d '{"model":"<deployment-name>","messages":[{"role":"user","content":"ping"}]}'

curl -sS "https://$RES.openai.azure.com/openai/v1/responses" \
  -H "api-key: $KEY" -H "Content-Type: application/json" \
  -d '{"model":"<deployment-name>","input":"ping"}'
```

## Legacy deployment-in-path API

Source: DIAL README ("legacy"), which recommends v1 instead because the
deployment can end up configured in two places.

| Operation | Method and URL |
|---|---|
| Chat completions | `POST {origin}/openai/deployments/{deployment}/chat/completions?api-version={v}` |
| Embeddings | `POST {origin}/openai/deployments/{deployment}/embeddings?api-version={v}` |

`api-version` is mandatory. The default is `2024-10-21`, which is also the
Copilot SDK's documented default for provider type `azure`; change it with
`legacy_api_version`. Do not send `model`. This is the shape of the "Target
URI" many Foundry portal pages display, and the shape users report VS Code's
personal Azure BYOK provider asking for.

## Authentication (both of the above)

Source: `securitySchemes` in the v1 spec.

| Method | Header |
|---|---|
| Resource key | `api-key: <key>` (the spec also accepts `authorization`) |
| Microsoft Entra ID | `Authorization: Bearer <token>` for scope `https://cognitiveservices.azure.com/.default` |

```bash
az account get-access-token --resource https://cognitiveservices.azure.com --query accessToken -o tsv
```

The caller typically needs the **Cognitive Services OpenAI User** role on the
resource for OpenAI models. Foundry-native and partner models can require a
broader role (for example Azure AI User); a 403 from `verify` is the signal.

## Anthropic Messages API on Foundry (Claude)

Source: DIAL README, "Anthropic Messages API".

`POST https://{resource}.services.ai.azure.com/anthropic/v1/messages`

Headers: `x-api-key: <key>` or `Authorization: Bearer <token>`, plus
`anthropic-version: 2023-06-01`. Body needs `model` (the deployment name),
`max_tokens` and `messages`. There is no models listing on this surface, so
`verify` skips that check for Claude.

A model whose name starts with `claude` selects this surface automatically;
force it with `surface`.

## Azure AI Model Inference API (retired)

Source: `ModelInference/preview/2024-05-01-preview/openapi.json`, host template
`https://{resource}.services.ai.azure.com/models`; the DIAL README marks it
retired and links Microsoft's migration guide.

`POST https://{resource}.services.ai.azure.com/models/chat/completions?api-version=2024-05-01-preview`

Emitted only so migration inventories can show old and new side by side. Use
v1 for anything new.

## Copilot consumers

| Consumer | Setting | Value |
|---|---|---|
| gh-aw, Copilot CLI | `COPILOT_PROVIDER_BASE_URL` | `{origin}/openai/v1` (no trailing slash) |
| | `engine.model` / `COPILOT_MODEL` | Azure model ID |
| | `COPILOT_PROVIDER_MODEL_ID` | deployment name, only when it differs |
| | `COPILOT_PROVIDER_WIRE_API` | `responses` for GPT-5, o-series and codex models; omitted otherwise |
| Copilot SDK | `provider.baseUrl` / `type` / `model` | `{origin}/openai/v1/` / `openai` / deployment name |
| Copilot CLI, native Azure type | `COPILOT_PROVIDER_TYPE=azure` | base URL `{origin}`; `COPILOT_MODEL` = deployment name |
| Enterprise custom models | Deployment URL / Model ID | per `enterprise_url_style` / deployment name |

Two editions of GitHub's Copilot CLI page disagree on the native Azure base
URL (resource root on github.com docs, `/openai/deployments/{name}` on the
Enterprise Cloud edition), and copilot-cli issue 3208 reports the `azure` type
ignoring the wire API and pinning an api-version the Responses route rejects.
The OpenAI-compatible `/openai/v1` form avoids all of that and is what gh-aw
and the SDK document for Foundry, so it is the default everywhere. The native
form is left as a commented alternative in `copilot-cli.env`.

## Wire API inference

| Model name | Wire API |
|---|---|
| starts with `gpt-5` | `responses` |
| `o1`, `o3`, `o4-mini`, `o3-pro`… (`o` + digit) | `responses` |
| contains `codex` | `responses` |
| anything else | `completions` (the Copilot CLI default, so the variable is omitted) |

Override with `wire_api`. Forcing `completions` on a GPT-5 or o-series model
produces a warning, and `verify --probe` will tell you if the deployment
rejects the chosen wire.
