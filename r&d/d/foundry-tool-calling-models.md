# Azure AI Foundry — eastus2 tool-calling models and GitHub Copilot custom-model configuration

Snapshot basis: Microsoft's *Region availability for Foundry Models sold by Azure* matrix, last updated
2026-09-03 (https://learn.microsoft.com/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure-region-availability).
Tool-calling support comes from publisher documentation, **not** from the catalog API — verify empirically with
`scripts/foundry-tools-table.sh` (§5). Live truth for a given subscription is always
`az cognitiveservices model list -l eastus2`.

Legend — SKUs: **GS** GlobalStandard · **DZS** DataZoneStandard · **S** Standard (regional) ·
**GPM** GlobalProvisionedManaged · **DZPM** DataZoneProvisionedManaged · **PM** ProvisionedManaged (regional).
⚠️ = not verified from an authoritative source; confirm before relying on it.

---

## 1. Parameters common to every deployment

| `az cognitiveservices account deployment create` flag | Value |
|---|---|
| `-g` / `-n` | `rg-coolgit-copilot` / `ais-coolgit-copilot-prod` — account in `eastus2`, kind `AIServices`, `local_auth_enabled = true`, custom subdomain set |
| `--deployment-name` | your choice, lowercase/digits/hyphens ≤ 64; this string becomes the Copilot model id |
| `--model-format` | from the tables (case-sensitive; publishers occasionally rename) |
| `--model-name` / `--model-version` | from the tables; `latest` must be resolved from the catalog at deploy time |
| `--sku-name` | one of the SKUs listed for the row; `GlobalStandard` unless data-zone residency or PTU is required |
| `--sku-capacity` | Standard SKUs: **thousands of TPM** · Provisioned SKUs: **PTUs** |
| `--rai-policy-name` | `Microsoft.DefaultV2` (or a custom account-scoped policy) |

Azure role required: **Foundry Owner** on the account (covers deployments, keys, catalog and usage reads).

---

## 2. Azure OpenAI models — `--model-format OpenAI` — eastus2

| model-name | model-version(s) | SKUs in eastus2 | Tool calling | Copilot BYOK | Notes |
|---|---|---|---|---|---|
| gpt-4.1 | 2025-04-14 | GS DZS S GPM DZPM PM | ✅ | ✅ | 1M context |
| gpt-4.1-mini | 2025-04-14 | GS DZS S GPM DZPM PM | ✅ | ✅ | cheapest safe fallback |
| gpt-4.1-nano | 2025-04-14 | GS DZS GPM DZPM PM | ✅ | ✅ | |
| gpt-4o | 2024-05-13, 2024-08-06, 2024-11-20 | GS DZS S GPM DZPM PM | ✅ | ✅ | |
| gpt-4o-mini | 2024-07-18 | GS DZS S GPM DZPM PM | ✅ | ✅ | |
| gpt-5 | 2025-08-07 | GS DZS GPM DZPM PM | ✅ | ✅ | reasoning; slow first token at default effort |
| gpt-5-mini | 2025-08-07 | GS DZS GPM DZPM PM | ✅ | ✅ | |
| gpt-5-nano | 2025-08-07 | GS DZS GPM DZPM | ✅ | ✅ | |
| gpt-5.1 | 2025-11-13 | GS DZS S GPM DZPM PM | ✅ | ✅ | |
| gpt-5.2 | 2025-12-11 | GS DZS GPM DZPM | ✅ | ✅ | |
| gpt-5.4 | 2026-03-05 | GS DZS GPM DZPM | ✅ | ✅ | |
| gpt-5.4-mini | 2026-03-17 | GS DZS GPM DZPM | ✅ | ✅ | |
| gpt-5.4-nano | 2026-03-17 | GS DZS | ✅ | ✅ | |
| gpt-5.5 | 2026-04-24 | GS DZS GPM DZPM | ✅ | ✅ | |
| gpt-5.6-luna | 2026-07-09 | GS DZS GPM DZPM PM | ✅ | ✅ | |
| gpt-5.6-sol | 2026-07-09 | GS DZS DZPM | ✅ | ✅ | no GPM in eastus2 |
| gpt-5.6-terra | 2026-07-09 | GS DZS GPM DZPM | ✅ | ✅ | |
| gpt-6-astra | 2026-09-03 | GS DZS GPM DZPM | ✅ | ✅ | added to the matrix 2026-09-03 |
| o1 | 2024-12-17 | GS DZS S GPM DZPM | ✅ | ✅ | reasoning |
| o3 | 2025-04-16 | GS DZS GPM DZPM PM | ✅ | ✅ | |
| o3-mini | 2025-01-31 | GS DZS GPM DZPM PM | ✅ | ✅ | |
| o4-mini | 2025-04-16 | GS DZS S GPM DZPM PM | ✅ | ✅ | |
| model-router | 2025-05-19, 2025-08-07, 2025-11-18 | GS DZS | ⚠️ depends on routed model | ⚠️ | probe before use |
| gpt-chat-latest | 2026-05-05, 2026-05-28, 2026-06-24, 2026-08-06 | GS | ⚠️ | ⚠️ | chat-tuned alias; historically no function calling |
| gpt-5-codex | 2025-09-15 | GS | ✅ | ⚠️ Responses API only | enterprise BYOK only (Responses support added Jan 2026); not the VS Code local `azure` provider |
| gpt-5.1-codex / -max / -mini | 2025-11-13, 2025-12-04, 2025-11-13 | GS (+GPM for codex) | ✅ | ⚠️ Responses API only | same |
| gpt-5.2-codex | 2026-01-14 | GS GPM | ✅ | ⚠️ Responses API only | same |
| gpt-5.3-codex | 2026-02-24 | GS DZS GPM | ✅ | ⚠️ Responses API only | same |
| gpt-5-pro, gpt-5.4-pro, o3-pro | 2025-10-06, 2026-03-05, 2025-06-10 | GS | ✅ | ⚠️ Responses API only | high latency; poor fit for interactive chat |
| codex-mini, computer-use-preview | 2025-05-16, 2025-03-11 | GS | ✅ | ❌ | Responses-only and agent-specific |

Excluded as non-chat: gpt-image-*, gpt-audio*, gpt-realtime*, gpt-4o-*-transcribe / -tts, whisper, tts, sora-2,
text-embedding-*, o3-deep-research (Agent Service only; not in eastus2).

---

## 3. Other Foundry Models sold by Azure — eastus2

| model-name | model-format | model-version(s) | SKUs in eastus2 | Tool calling | Copilot BYOK | Notes |
|---|---|---|---|---|---|---|
| DeepSeek-V3.2 | DeepSeek | 1 | GS GPM DZPM | ✅ | ✅ | only partner model with PTU |
| DeepSeek-V3.2-Speciale | DeepSeek | 1 | GS | ❌ | ❌ | reasoning-only variant |
| DeepSeek-V4-Flash | DeepSeek | 2026-04-23 | GS DZS | ✅ | ✅ | hybrid thinking → `/no_think` on tool turns |
| DeepSeek-V4-Flash-0731 | DeepSeek | 2026-07-31 | GS | ✅ | ✅ | |
| DeepSeek-V4-Pro | DeepSeek | 2026-04-23 | GS DZS | ✅ | ✅ | |
| grok-4-1-fast-non-reasoning | xAI | 1 | GS DZS | ✅ | ✅ | lowest latency of the Grok set |
| grok-4-1-fast-reasoning | xAI | 1 | GS DZS | ✅ | ✅ | |
| grok-4-20-non-reasoning | xAI | 1 | GS DZS | ✅ | ✅ | |
| grok-4-20-reasoning | xAI | 1 | GS DZS | ✅ | ✅ | |
| grok-4.3 | xAI | 1 | GS DZS | ✅ | ✅ | |
| grok-4.6 | xAI | 1 | GS | ✅ | ✅ | |
| Kimi-K2.5 | Moonshot AI ⚠️ | 1 | GS | ✅ | ✅ | format string unverified — read from catalog |
| Kimi-K2.6 | Moonshot AI ⚠️ | 2026-04-20 | GS | ✅ | ✅ | |
| Kimi-K2.7-Code | Moonshot AI ⚠️ | 2026-06-12 | GS | ✅ | ✅ | coding-tuned; strong `coder` candidate |
| Llama-3.3-70B-Instruct | Meta | 1, 2, 3, 4, 5, 9 | GS; v9 also GPM DZPM | ✅ | ✅ | |
| Llama-4-Maverick-17B-128E-Instruct-FP8 | Meta | 1 | GS | ✅ | ✅ | 1M context |
| Mistral-Large-3 | Mistral AI | 1 | GS DZS | ✅ | ✅ | |
| mistral-medium-3-5 | Mistral AI | 1 | GS DZS | ✅ | ✅ | |
| cohere-command-a | Cohere | 1 | GS | ✅ | ✅ | |
| Cohere-command-a-plus-05-2026 | Cohere | 1 | GS | ✅ | ✅ | |
| MAI-Thinking-1 | Microsoft | 2026-06-01 | GS | ⚠️ | ⚠️ | new Microsoft reasoning model; probe |
| Phi-4-mini-instruct | Microsoft | 1 | GS | ✅ | ✅ | |
| Phi-4-multimodal-instruct | Microsoft | 1 | GS | ✅ | ✅ | |
| Phi-4 | Microsoft | 2, 3, 4, 5, 6, 7 | GS | ❌ | ❌ | chat only |
| Phi-4-reasoning, Phi-4-mini-reasoning | Microsoft | 1 | GS | ❌ | ❌ | |

Excluded as non-chat: FLUX-1.1-pro, FLUX.1-Kontext-pro, FLUX.2-flex, FLUX.2-pro, Cohere-rerank-v4.0-fast/-pro.
MAI-Image-* is not offered in eastus2.

**Qwen.** No Qwen model appears in the Direct-from-Azure eastus2 matrix. Microsoft's catalog page lists Qwen-32B
among open-source models supported only on Foundry resources, but it is absent from the regional tables — treat
`Qwen/qwen3-32b` as *possibly present for your subscription* and let the catalog check decide. Qwen3-Coder-Next is
managed compute (Hugging Face collection) only.

---

## 4. GitHub Copilot custom-model configuration per model

How the fields map. In **CoolGitEnterprise → AI controls → Copilot → Custom models → Add API key** you enter a
provider, a key, a base URL, one or more model ids, and a max context window. For a Foundry account the same values
hold for every model; only the model id and context window change per row.

| Field | Value for every row below |
|---|---|
| Provider | **Microsoft Foundry** (Chat Completions models). For the Responses-only rows, use **OpenAI-compatible** with the Responses API option enabled |
| API key | `az cognitiveservices account keys list -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --query key1 -o tsv` (or the `FOUNDRY_BYOK_KEY` Actions secret) |
| Base URL | `https://ais-coolgit-copilot-prod.openai.azure.com` — host only, no path, no trailing slash (Copilot appends `/openai/deployments/<model id>/…`) |
| Model id | **the deployment name**, never the catalog model name |
| Access | Enable the model, then grant **CoolGitOrg** on the Access tab |

Per-user Copilot CLI equivalent: `COPILOT_PROVIDER_TYPE=azure`,
`COPILOT_PROVIDER_BASE_URL=https://ais-coolgit-copilot-prod.openai.azure.com/openai/deployments/<deployment>/v1`,
`COPILOT_PROVIDER_API_KEY=<key>`, `COPILOT_MODEL=<deployment>`.

Context-window figures are the model's documented total context; enter that (or lower) as *Max context window*.
Values marked ⚠️ are not confirmed from Microsoft's model page — check the Foundry model card before entering.

| Deployment (model id) | Backing model | Provider | API | Max context window | Streaming | Tool calling | Thinking / notes |
|---|---|---|---|---|---|---|---|
| gpt-4-1 | gpt-4.1 @ 2025-04-14 | Microsoft Foundry | Chat Completions | 1,047,576 | ✅ | ✅ | none |
| gpt-4-1-mini | gpt-4.1-mini | Microsoft Foundry | Chat Completions | 1,047,576 | ✅ | ✅ | none; recommended default fallback |
| gpt-4-1-nano | gpt-4.1-nano | Microsoft Foundry | Chat Completions | 1,047,576 | ✅ | ✅ | none |
| gpt-4o | gpt-4o @ 2024-11-20 | Microsoft Foundry | Chat Completions | 128,000 | ✅ | ✅ | none |
| gpt-4o-mini | gpt-4o-mini | Microsoft Foundry | Chat Completions | 128,000 | ✅ | ✅ | none |
| gpt-5 | gpt-5 @ 2025-08-07 | Microsoft Foundry | Chat Completions | 400,000 | ✅ | ✅ | reasoning; `reasoning_effort` not settable from Copilot — expect latency |
| gpt-5-mini | gpt-5-mini | Microsoft Foundry | Chat Completions | 400,000 | ✅ | ✅ | reasoning |
| gpt-5-nano | gpt-5-nano | Microsoft Foundry | Chat Completions | 400,000 | ✅ | ✅ | reasoning |
| gpt-5-1 | gpt-5.1 | Microsoft Foundry | Chat Completions | 400,000 | ✅ | ✅ | reasoning (adaptive) |
| gpt-5-2 | gpt-5.2 | Microsoft Foundry | Chat Completions | 400,000 ⚠️ | ✅ | ✅ | |
| gpt-5-4 / -mini / -nano | gpt-5.4 family | Microsoft Foundry | Chat Completions | 400,000 ⚠️ | ✅ | ✅ | |
| gpt-5-5 | gpt-5.5 | Microsoft Foundry | Chat Completions | 400,000 ⚠️ | ✅ | ✅ | |
| gpt-5-6-luna / -sol / -terra | gpt-5.6 family | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ✅ | |
| gpt-6-astra | gpt-6-astra | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ✅ | |
| o1 | o1 @ 2024-12-17 | Microsoft Foundry | Chat Completions | 200,000 | ✅ | ✅ | reasoning; no `system` role → Copilot sends `developer` |
| o3 | o3 | Microsoft Foundry | Chat Completions | 200,000 | ✅ | ✅ | reasoning |
| o3-mini | o3-mini | Microsoft Foundry | Chat Completions | 200,000 | ✅ | ✅ | reasoning |
| o4-mini | o4-mini | Microsoft Foundry | Chat Completions | 200,000 | ✅ | ✅ | reasoning |
| model-router | model-router @ 2025-11-18 | Microsoft Foundry | Chat Completions | 200,000 ⚠️ (lowest of routed set) | ✅ | ⚠️ | set context to the smallest routed model |
| gpt-5-codex, gpt-5-1-codex*, gpt-5-2-codex, gpt-5-3-codex | codex family | OpenAI-compatible (Responses) | **Responses** | 400,000 | ✅ | ✅ | enterprise BYOK only; base URL `…/openai/v1`; not usable from VS Code local BYOK `azure` provider |
| gpt-5-pro, gpt-5-4-pro, o3-pro | pro family | OpenAI-compatible (Responses) | **Responses** | 400,000 / 200,000 | ✅ | ✅ | enterprise BYOK only; multi-minute latency |
| deepseek-v3-2 | DeepSeek-V3.2 | Microsoft Foundry | Chat Completions | 131,072 | ✅ | ✅ | hybrid thinking; `reasoning_content` echo risk on tool turns |
| deepseek-v4-flash | DeepSeek-V4-Flash @ 2026-04-23 | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ✅ | hybrid thinking |
| deepseek-v4-pro | DeepSeek-V4-Pro | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ✅ | hybrid thinking |
| grok-4-1-fast | grok-4-1-fast-non-reasoning | Microsoft Foundry | Chat Completions | 2,000,000 | ✅ | ✅ | none; cap context at 262,144 in Copilot for latency |
| grok-4-1-fast-r | grok-4-1-fast-reasoning | Microsoft Foundry | Chat Completions | 2,000,000 | ✅ | ✅ | reasoning |
| grok-4-20 / grok-4-20-r | grok-4-20-* | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ✅ | |
| grok-4-3, grok-4-6 | grok-4.3 / grok-4.6 | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ✅ | |
| kimi-k2-5 | Kimi-K2.5 | Microsoft Foundry | Chat Completions | 262,144 | ✅ | ✅ | none |
| kimi-k2-6 | Kimi-K2.6 | Microsoft Foundry | Chat Completions | 262,144 ⚠️ | ✅ | ✅ | |
| kimi-k2-7-code | Kimi-K2.7-Code | Microsoft Foundry | Chat Completions | 262,144 ⚠️ | ✅ | ✅ | coding-tuned |
| llama-3-3-70b | Llama-3.3-70B-Instruct @ 9 | Microsoft Foundry | Chat Completions | 131,072 | ✅ | ✅ | none |
| llama-4-maverick | Llama-4-Maverick-17B-128E-Instruct-FP8 | Microsoft Foundry | Chat Completions | 1,048,576 | ✅ | ✅ | none; multimodal |
| mistral-large-3 | Mistral-Large-3 | Microsoft Foundry | Chat Completions | 262,144 ⚠️ | ✅ | ✅ | none |
| mistral-medium-3-5 | mistral-medium-3-5 | Microsoft Foundry | Chat Completions | 131,072 | ✅ | ✅ | none |
| command-a | cohere-command-a | Microsoft Foundry | Chat Completions | 256,000 | ✅ | ✅ | none |
| command-a-plus | Cohere-command-a-plus-05-2026 | Microsoft Foundry | Chat Completions | 256,000 ⚠️ | ✅ | ✅ | |
| mai-thinking-1 | MAI-Thinking-1 | Microsoft Foundry | Chat Completions | ⚠️ | ✅ | ⚠️ | probe before registering |
| phi-4-mini | Phi-4-mini-instruct | Microsoft Foundry | Chat Completions | 131,072 | ✅ | ✅ | small; fine for cheap chat, weak for agent mode |
| phi-4-mm | Phi-4-multimodal-instruct | Microsoft Foundry | Chat Completions | 131,072 | ✅ | ✅ | |
| qwen3-32b ⚠️ | Qwen/qwen3-32b (if present) | Microsoft Foundry | Chat Completions | 131,072 | ✅ | ✅ | hybrid thinking; `/no_think` |
| coder (managed) | Qwen3-Coder-Next (HF, managed compute) | OpenAI-compatible | Chat Completions | 262,144 | ✅ | ✅ | base URL `…/openai/v1`; vLLM `qwen3_coder` tool parser |

Not registrable in Copilot (no tool calling): DeepSeek-V3.2-Speciale, Phi-4, Phi-4-reasoning, Phi-4-mini-reasoning,
codex-mini, computer-use-preview; and every image/audio/embedding/rerank model.

### Rules that apply to every registration

- **Model id = deployment name.** If the deployment is `chat-fallback`, the Copilot model id is `chat-fallback`,
  regardless of which catalog model backs it.
- **Base URL is the account host only** for the Microsoft Foundry / `azure` provider. Putting the deployment path in
  it doubles the path and returns 404.
- **Max context window** ≤ the model's real window. Too high → truncation errors; too low → Copilot trims context
  unnecessarily. When unsure, enter 128,000.
- **Thinking models** in agent mode: prefer the non-reasoning variant where the publisher offers one
  (`grok-*-non-reasoning`, `DeepSeek-V3.2` over `-Speciale`); otherwise expect occasional empty responses when
  reasoning consumes the token budget.
- **Responses-only models** work through enterprise BYOK (OpenAI-compatible provider with Responses enabled) but
  not through VS Code's local BYOK `azure` provider, which only speaks Chat Completions.
- BYOK usage is billed by Azure and does not consume Copilot AI Credits.

---

## 5. Regenerate from the live catalog

```bash
#!/usr/bin/env bash
# foundry-tools-table.sh — live eastus2 catalog → markdown, with an empirical tool-call probe per model.
set -euo pipefail
LOC=eastus2; RG=rg-coolgit-copilot; ACCT=ais-coolgit-copilot-prod
EP="$(az cognitiveservices account show -g "$RG" -n "$ACCT" --query properties.endpoint -o tsv)"; EP="${EP%/}"
KEY="$(az cognitiveservices account keys list -g "$RG" -n "$ACCT" --query key1 -o tsv)"

az cognitiveservices model list -l "$LOC" -o json \
| jq -r '[.[] | select(.model.capabilities.chatCompletion=="true")
          | {format:.model.format, name:.model.name, version:.model.version,
             skus:([.model.skus[]?.name]|unique|join(" ")),
             deprecation:(.model.deprecation.inference // "")}]
         | sort_by(.format,.name,.version)[]
         | "\(.format)|\(.name)|\(.version)|\(.skus)|\(.deprecation)"' > /tmp/rows.txt

probe() { local fmt="$1" name="$2" ver="$3"
  local d="probe-$(tr '[:upper:]' '[:lower:]' <<<"$name" | tr -c 'a-z0-9\n' '-' | cut -c1-40)"
  az cognitiveservices account deployment create -g "$RG" -n "$ACCT" --deployment-name "$d" \
    --model-format "$fmt" --model-name "$name" --model-version "$ver" \
    --sku-name GlobalStandard --sku-capacity 1 -o none 2>/dev/null || { echo "n/a"; return; }
  for _ in $(seq 1 30); do
    [[ "$(az cognitiveservices account deployment show -g "$RG" -n "$ACCT" --deployment-name "$d" --query properties.provisioningState -o tsv)" == Succeeded ]] && break; sleep 10
  done
  r="$(curl -sS "$EP/openai/deployments/$d/chat/completions?api-version=2024-10-21" -H "api-key: $KEY" -H 'Content-Type: application/json' -d '{
    "messages":[{"role":"user","content":"Weather in Boston? Use the tool. /no_think"}],"max_tokens":128,"tool_choice":"auto",
    "tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]}')"
  jq -r 'if .choices[0].message.tool_calls[0].function.name=="get_weather" then "✅" else "❌" end' <<<"$r" 2>/dev/null || echo "❌"
  az cognitiveservices account deployment delete -g "$RG" -n "$ACCT" --deployment-name "$d" -o none
}

echo "| model-name | model-format | version | SKUs | deprecation | tool calling (probed) |"
echo "|---|---|---|---|---|---|"
while IFS='|' read -r fmt name ver skus dep; do
  echo "| $name | $fmt | $ver | $skus | $dep | $(probe "$fmt" "$name" "$ver") |"
done < /tmp/rows.txt
```

Each probe is one capacity-1 deployment and one request (cents), ~2–3 minutes per model. Run once, commit the
output next to `models/registry.yml`, re-run when the catalog-refresh workflow reports drift. The `deprecation`
column is what turns a working deployment into a 404 months later — read it.
