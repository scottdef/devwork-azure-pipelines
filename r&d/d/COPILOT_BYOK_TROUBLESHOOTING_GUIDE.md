# GitHub Copilot BYOK on Azure AI Foundry — testing and troubleshooting guide

Models covered: **Mistral-Large-3**, **Cohere-command-a-plus-05-2026** (both Foundry Models sold by Azure,
serverless GlobalStandard) and **Claude Sonnet 4.6** (Anthropic on Foundry, serverless GlobalStandard, Azure
Marketplace-billed). Account: `ais-coolgit-copilot-prod` in `rg-coolgit-copilot`, region `eastus2`.
Identity for automation: UAMI with **Foundry Owner** on the account.

The three models differ in one way that drives every test below:

| | Mistral-Large-3 | Cohere-command-a-plus | Claude Sonnet 4.6 |
|---|---|---|---|
| `--model-format` | `Mistral AI` | `Cohere` | `Anthropic` |
| Wire API on Foundry | OpenAI Chat Completions | OpenAI Chat Completions | **Anthropic Messages** (`/anthropic/v1/messages`) |
| Base URL for tests | `https://<acct>.openai.azure.com` | same | `https://<acct>.services.ai.azure.com/anthropic` |
| Auth header | `api-key:` (or `Authorization: Bearer <key>`) | same | `x-api-key:` / `api-key:` (both accepted) + `anthropic-version: 2023-06-01` |
| Commercial gate | none | none | Azure Marketplace terms + `modelProviderData` attestation on the deployment |
| Copilot provider | Microsoft Foundry / `azure` | Microsoft Foundry / `azure` | **Anthropic** with custom base URL — see §5 |
| Thinking | none | none | optional extended thinking (off by default) |

Region note: Claude on Foundry is offered in `eastus2` and `swedencentral` (all families) and `westus2`
(sonnet/opus only). If your account is elsewhere, Claude is a no-go before you start.

---

## 1. Pre-flight — five commands, five yes/no answers

```bash
RG=rg-coolgit-copilot; ACCT=ais-coolgit-copilot-prod
az cognitiveservices account show -g $RG -n $ACCT \
  --query '{kind:kind, location:location, endpoint:properties.endpoint, subdomain:properties.customSubDomainName,
            localAuth:(properties.disableLocalAuth==null||properties.disableLocalAuth==`false`),
            publicNet:properties.publicNetworkAccess}' -o table
```
Expect `kind=AIServices`, `location=eastus2`, a non-empty `endpoint` and `subdomain`, `localAuth=true`,
`publicNet=Enabled`. Any other answer is a blocker; fix it in Terraform before touching Copilot.

```bash
# catalog has the three models in this region (exact names + formats)
az cognitiveservices model list -l eastus2 \
  --query "[?model.name=='Mistral-Large-3' || model.name=='Cohere-command-a-plus-05-2026' || starts_with(model.name,'claude-sonnet-4-6')]
           .{format:model.format,name:model.name,version:model.version,skus:join(',',model.skus[].name)}" -o table
# quota headroom for GlobalStandard (thousands TPM)
az cognitiveservices usage list -l eastus2 --query "[?contains(name.value,'GlobalStandard')] | [?contains(name.value,'Mistral') || contains(name.value,'Cohere') || contains(name.value,'Anthropic') || contains(name.value,'claude')].{q:name.value,cur:currentValue,lim:limit}" -o table
# the role that will do the work
az role assignment list --scope "$(az cognitiveservices account show -g $RG -n $ACCT --query id -o tsv)" --query "[].{who:principalName,role:roleDefinitionName}" -o table
```

Claude only — Marketplace terms must be accepted for the subscription once, and the deployment must carry the
attestation block. Foundry Owner does **not** include `Microsoft.MarketplaceOrdering/*`; a subscription
Contributor does this step:
```bash
az term show --publisher anthropic --product anthropic-claude-sonnet-4-6-offer --plan "$(az term show --publisher anthropic --product anthropic-claude-sonnet-4-6-offer --query '[0].plan' -o tsv 2>/dev/null || echo claude-sonnet-4-6)" --query accepted
az term accept --publisher anthropic --product anthropic-claude-sonnet-4-6-offer --plan <plan-from-above>
```

---

## 2. Deploy

Mistral and Cohere are plain deployments (this is what the chat-fallback workflow does):
```bash
az cognitiveservices account deployment create -g $RG -n $ACCT --deployment-name mistral-large-3 \
  --model-format "Mistral AI" --model-name Mistral-Large-3 --model-version 1 \
  --sku-name GlobalStandard --sku-capacity 50 --rai-policy-name Microsoft.DefaultV2
az cognitiveservices account deployment create -g $RG -n $ACCT --deployment-name command-a-plus \
  --model-format Cohere --model-name Cohere-command-a-plus-05-2026 --model-version 1 \
  --sku-name GlobalStandard --sku-capacity 50 --rai-policy-name Microsoft.DefaultV2
```

Claude needs `modelProviderData` (organization, country, industry — sent to Anthropic with every request and must
be true). `deployment create` has no flag for it, so use the REST PUT:
```bash
ACCT_ID=$(az cognitiveservices account show -g $RG -n $ACCT --query id -o tsv)
VER=$(az cognitiveservices model list -l eastus2 --query "[?model.format=='Anthropic' && model.name=='claude-sonnet-4-6'] | [-1].model.version" -o tsv)
az rest --method put --url "https://management.azure.com$ACCT_ID/deployments/claude-sonnet-4-6?api-version=2025-06-01" --body "$(jq -n --arg v "$VER" '{
  sku:{name:"GlobalStandard",capacity:50},
  properties:{ model:{format:"Anthropic",name:"claude-sonnet-4-6",version:$v},
    modelProviderData:{anthropicPublisherData:{organizationName:"CoolGit Inc",countryCode:"US",industry:"Software"}} } }')"
```
Poll all three to `Succeeded`:
```bash
for d in mistral-large-3 command-a-plus claude-sonnet-4-6; do
  printf '%-18s %s\n' $d "$(az cognitiveservices account deployment show -g $RG -n $ACCT --deployment-name $d --query 'properties.provisioningState' -o tsv)"; done
```

---

## 3. Test ladder

Run in order. Each rung isolates one layer; stop at the first failure and go to §6 with the rung number.

```bash
EP="$(az cognitiveservices account show -g $RG -n $ACCT --query properties.endpoint -o tsv)"; EP="${EP%/}"   # https://<acct>.openai.azure.com or .cognitiveservices.azure.com
AEP="https://${ACCT}.services.ai.azure.com/anthropic"
KEY="$(az cognitiveservices account keys list -g $RG -n $ACCT --query key1 -o tsv)"
```

### T1 — control plane sees the deployment
```bash
az cognitiveservices account deployment show -g $RG -n $ACCT --deployment-name mistral-large-3 --query '{state:properties.provisioningState,model:properties.model.name,sku:sku.name,cap:sku.capacity}'
```
Fail → deployment doesn't exist or is in another account. Not a Copilot problem.

### T2 — key authenticates (expect 200; 401 = key, 403 = policy, 404 = URL)
```bash
# OpenAI-shape models
for d in mistral-large-3 command-a-plus; do
  printf '%-18s %s\n' $d "$(curl -s -o /dev/null -w '%{http_code}' "$EP/openai/deployments/$d/chat/completions?api-version=2024-10-21" \
    -H "api-key: $KEY" -H 'Content-Type: application/json' -d '{"messages":[{"role":"user","content":"hi"}],"max_tokens":5}')"; done
# Claude — Messages API, x-api-key, anthropic-version required, max_tokens required
printf '%-18s %s\n' claude-sonnet-4-6 "$(curl -s -o /dev/null -w '%{http_code}' "$AEP/v1/messages" \
  -H "x-api-key: $KEY" -H 'anthropic-version: 2023-06-01' -H 'Content-Type: application/json' \
  -d '{"model":"claude-sonnet-4-6","max_tokens":5,"messages":[{"role":"user","content":"hi"}]}')"
```

### T3 — chat returns text
```bash
curl -s "$EP/openai/deployments/mistral-large-3/chat/completions?api-version=2024-10-21" -H "api-key: $KEY" -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"Reply with exactly: OK"}],"max_tokens":8}' | jq -r '.choices[0].message.content, .choices[0].finish_reason'
curl -s "$AEP/v1/messages" -H "x-api-key: $KEY" -H 'anthropic-version: 2023-06-01' -H 'Content-Type: application/json' \
  -d '{"model":"claude-sonnet-4-6","max_tokens":8,"messages":[{"role":"user","content":"Reply with exactly: OK"}]}' | jq -r '.content[0].text, .stop_reason'
```
Expect `OK` and `stop`/`end_turn`. Empty content with `length` → `max_tokens` too small for this test (raise to 64).

### T4 — streaming (Copilot requires SSE; count events)
```bash
curl -sN "$EP/openai/deployments/command-a-plus/chat/completions?api-version=2024-10-21" -H "api-key: $KEY" -H 'Content-Type: application/json' \
  -d '{"stream":true,"messages":[{"role":"user","content":"Count to five."}],"max_tokens":40}' | grep -c '^data:'
curl -sN "$AEP/v1/messages" -H "x-api-key: $KEY" -H 'anthropic-version: 2023-06-01' -H 'Content-Type: application/json' \
  -d '{"model":"claude-sonnet-4-6","stream":true,"max_tokens":40,"messages":[{"role":"user","content":"Count to five."}]}' | grep -c '^event: content_block_delta'
```
Expect > 1 in both. OpenAI-shape streams must end with `data: [DONE]`; Anthropic streams with `event: message_stop`.

### T5 — single tool call (the Copilot minimum)
```bash
TOOLS_OAI='[{"type":"function","function":{"name":"get_weather","description":"Get weather for a city","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]'
for d in mistral-large-3 command-a-plus; do
  curl -s "$EP/openai/deployments/$d/chat/completions?api-version=2024-10-21" -H "api-key: $KEY" -H 'Content-Type: application/json' \
    -d "{\"messages\":[{\"role\":\"user\",\"content\":\"What is the weather in Boston? Use the tool.\"}],\"tools\":$TOOLS_OAI,\"tool_choice\":\"auto\",\"max_tokens\":128}" \
    | jq -c "{d:\"$d\", fn:.choices[0].message.tool_calls[0].function.name, args:.choices[0].message.tool_calls[0].function.arguments, finish:.choices[0].finish_reason}"; done
curl -s "$AEP/v1/messages" -H "x-api-key: $KEY" -H 'anthropic-version: 2023-06-01' -H 'Content-Type: application/json' \
  -d '{"model":"claude-sonnet-4-6","max_tokens":256,"messages":[{"role":"user","content":"What is the weather in Boston? Use the tool."}],
       "tools":[{"name":"get_weather","description":"Get weather for a city","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}' \
  | jq -c '{fn:(.content[]|select(.type=="tool_use")|.name), input:(.content[]|select(.type=="tool_use")|.input), stop:.stop_reason}'
```
Expect `get_weather`, arguments containing `Boston`, `finish_reason: tool_calls` / `stop_reason: tool_use`.
Arguments must be a **JSON string** for OpenAI-shape (Cohere sometimes returns an object — see §6).

### T6 — multi-turn tool round-trip (what agent mode actually does)
This is where most "works in curl, fails in Copilot" cases live: the assistant turn with `tool_calls` and the
`tool` result must be echoed back and the model must continue.
```bash
d=mistral-large-3
r1=$(curl -s "$EP/openai/deployments/$d/chat/completions?api-version=2024-10-21" -H "api-key: $KEY" -H 'Content-Type: application/json' \
  -d "{\"messages\":[{\"role\":\"user\",\"content\":\"Weather in Boston? Use the tool.\"}],\"tools\":$TOOLS_OAI,\"max_tokens\":128}")
asst=$(jq -c '.choices[0].message' <<<"$r1"); id=$(jq -r '.choices[0].message.tool_calls[0].id' <<<"$r1")
curl -s "$EP/openai/deployments/$d/chat/completions?api-version=2024-10-21" -H "api-key: $KEY" -H 'Content-Type: application/json' \
  -d "{\"messages\":[{\"role\":\"user\",\"content\":\"Weather in Boston? Use the tool.\"},$asst,{\"role\":\"tool\",\"tool_call_id\":\"$id\",\"content\":\"{\\\"temp_c\\\":18,\\\"sky\\\":\\\"clear\\\"}\"}],\"tools\":$TOOLS_OAI,\"max_tokens\":128}" \
  | jq -r '.choices[0].message.content // .error'
```
Expect a sentence mentioning 18 °C / clear. A 400 here (not on T5) is a message-format incompatibility — §6.
Claude equivalent: append the assistant `content` array as-is, then
`{"role":"user","content":[{"type":"tool_result","tool_use_id":"<id>","content":"{\"temp_c\":18}"}]}`.

### T7 — Copilot CLI smoke test (first Copilot surface; fastest to debug)
```bash
export COPILOT_PROVIDER_API_KEY="$KEY"
# Mistral / Cohere
export COPILOT_PROVIDER_TYPE=azure COPILOT_PROVIDER_BASE_URL="$EP/openai/deployments/mistral-large-3/v1" COPILOT_MODEL=mistral-large-3
copilot --log-level debug -p "list the files in this directory and summarize them"   # forces a tool call
# Claude
export COPILOT_PROVIDER_TYPE=anthropic COPILOT_PROVIDER_BASE_URL="$AEP" COPILOT_MODEL=claude-sonnet-4-6
copilot --log-level debug -p "list the files in this directory and summarize them"
```
Log: `~/.copilot/logs/`. A successful run shows the tool invocation and a final answer. If the CLI works and VS
Code doesn't, the problem is the VS Code/enterprise registration, not the endpoint.

### T8 — VS Code, then github.com
1. Enterprise BYOK registered (§5) → VS Code Copilot Chat → model picker → org-labelled model → Ask mode:
   "what does this file do" (no tools) → Agent mode: "find every TODO in the repo" (tools).
2. github.com Copilot Chat → same model → same two prompts. Web chat only sees **enterprise** BYOK models.

Turn on `Developer: Show Chat Debug View` (or Output → *GitHub Copilot Chat*, log level `trace`) before the
agent-mode prompt; it shows the exact request and where the stream stopped.

---

## 4. One-shot matrix script

`scripts/byok-test.sh` runs T1–T6 for all three and prints a pass/fail grid. Drop it next to the workflows.

```bash
#!/usr/bin/env bash
set -uo pipefail
RG=${RG:-rg-coolgit-copilot}; ACCT=${ACCT:-ais-coolgit-copilot-prod}
EP="$(az cognitiveservices account show -g $RG -n $ACCT --query properties.endpoint -o tsv)"; EP="${EP%/}"
AEP="https://${ACCT}.services.ai.azure.com/anthropic"
KEY="$(az cognitiveservices account keys list -g $RG -n $ACCT --query key1 -o tsv)"
TOOLS='[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]'
ok(){ [[ "$1" == "$2" ]] && echo "✅" || echo "❌($1)"; }

oai(){ local d=$1 url="$EP/openai/deployments/$d/chat/completions?api-version=2024-10-21" H=(-H "api-key: $KEY" -H 'Content-Type: application/json')
  t1=$(az cognitiveservices account deployment show -g $RG -n $ACCT --deployment-name $d --query properties.provisioningState -o tsv 2>/dev/null)
  t2=$(curl -s -o /tmp/r -w '%{http_code}' "$url" "${H[@]}" -d '{"messages":[{"role":"user","content":"Reply with exactly: OK"}],"max_tokens":8}')
  t3=$(jq -r '.choices[0].message.content' /tmp/r 2>/dev/null | tr -d ' \n.')
  t4=$(curl -sN "$url" "${H[@]}" -d '{"stream":true,"messages":[{"role":"user","content":"Count to five."}],"max_tokens":40}' | grep -c '^data:')
  r5=$(curl -s "$url" "${H[@]}" -d "{\"messages\":[{\"role\":\"user\",\"content\":\"Weather in Boston? Use the tool.\"}],\"tools\":$TOOLS,\"max_tokens\":128}")
  t5=$(jq -r '.choices[0].message.tool_calls[0].function.name' <<<"$r5" 2>/dev/null)
  asst=$(jq -c '.choices[0].message' <<<"$r5"); id=$(jq -r '.choices[0].message.tool_calls[0].id' <<<"$r5")
  t6=$(curl -s -o /tmp/r6 -w '%{http_code}' "$url" "${H[@]}" -d "{\"messages\":[{\"role\":\"user\",\"content\":\"Weather in Boston? Use the tool.\"},$asst,{\"role\":\"tool\",\"tool_call_id\":\"$id\",\"content\":\"{\\\"temp_c\\\":18}\"}],\"tools\":$TOOLS,\"max_tokens\":128}")
  printf '| %-18s | %s | %s | %s | %s | %s | %s |\n' "$d" "$(ok "$t1" Succeeded)" "$(ok "$t2" 200)" "$(ok "$t3" OK)" "$([[ ${t4:-0} -gt 1 ]] && echo ✅ || echo "❌($t4)")" "$(ok "$t5" get_weather)" "$(ok "$t6" 200)"; }

claude(){ local d=$1 url="$AEP/v1/messages" H=(-H "x-api-key: $KEY" -H 'anthropic-version: 2023-06-01' -H 'Content-Type: application/json')
  t1=$(az cognitiveservices account deployment show -g $RG -n $ACCT --deployment-name $d --query properties.provisioningState -o tsv 2>/dev/null)
  t2=$(curl -s -o /tmp/r -w '%{http_code}' "$url" "${H[@]}" -d "{\"model\":\"$d\",\"max_tokens\":8,\"messages\":[{\"role\":\"user\",\"content\":\"Reply with exactly: OK\"}]}")
  t3=$(jq -r '.content[0].text' /tmp/r 2>/dev/null | tr -d ' \n.')
  t4=$(curl -sN "$url" "${H[@]}" -d "{\"model\":\"$d\",\"stream\":true,\"max_tokens\":40,\"messages\":[{\"role\":\"user\",\"content\":\"Count to five.\"}]}" | grep -c '^event: content_block_delta')
  r5=$(curl -s "$url" "${H[@]}" -d "{\"model\":\"$d\",\"max_tokens\":256,\"messages\":[{\"role\":\"user\",\"content\":\"Weather in Boston? Use the tool.\"}],\"tools\":[{\"name\":\"get_weather\",\"input_schema\":{\"type\":\"object\",\"properties\":{\"city\":{\"type\":\"string\"}},\"required\":[\"city\"]}}]}")
  t5=$(jq -r '.content[]|select(.type=="tool_use")|.name' <<<"$r5" 2>/dev/null); id=$(jq -r '.content[]|select(.type=="tool_use")|.id' <<<"$r5"); asst=$(jq -c '.content' <<<"$r5")
  t6=$(curl -s -o /tmp/r6 -w '%{http_code}' "$url" "${H[@]}" -d "{\"model\":\"$d\",\"max_tokens\":128,\"messages\":[{\"role\":\"user\",\"content\":\"Weather in Boston? Use the tool.\"},{\"role\":\"assistant\",\"content\":$asst},{\"role\":\"user\",\"content\":[{\"type\":\"tool_result\",\"tool_use_id\":\"$id\",\"content\":\"{\\\"temp_c\\\":18}\"}]}],\"tools\":[{\"name\":\"get_weather\",\"input_schema\":{\"type\":\"object\",\"properties\":{\"city\":{\"type\":\"string\"}}}}]}")
  printf '| %-18s | %s | %s | %s | %s | %s | %s |\n' "$d" "$(ok "$t1" Succeeded)" "$(ok "$t2" 200)" "$(ok "$t3" OK)" "$([[ ${t4:-0} -gt 1 ]] && echo ✅ || echo "❌($t4)")" "$(ok "$t5" get_weather)" "$(ok "$t6" 200)"; }

echo "| deployment | T1 state | T2 auth | T3 chat | T4 stream | T5 tool | T6 round-trip |"; echo "|---|---|---|---|---|---|---|"
oai mistral-large-3; oai command-a-plus; claude claude-sonnet-4-6
```

---

## 5. Registering in Copilot — what to type, per model

### Enterprise (CoolGitEnterprise → AI controls → Copilot → Custom models → Add API key)

| Field | Mistral-Large-3 | Cohere-command-a-plus | Claude Sonnet 4.6 |
|---|---|---|---|
| Provider | Microsoft Foundry | Microsoft Foundry | **Anthropic** |
| Base URL | `https://<acct>.openai.azure.com` (host only) | same | `https://<acct>.services.ai.azure.com/anthropic` |
| API key | account `key1` | same key | same key |
| Model id | `mistral-large-3` (deployment name) | `command-a-plus` | `claude-sonnet-4-6` (deployment name) |
| Max context window | 262144 | 256000 | 200000 (1M if the 1M beta is enabled on the deployment) |
| Access | Enable → grant CoolGitOrg | same | same |

**Claude caveat.** The Anthropic provider in AI controls was built for `api.anthropic.com`. If the form for
provider *Anthropic* shows **no base-URL field**, Claude-on-Foundry cannot be registered enterprise-wide directly;
your options are (a) register it under **OpenAI-compatible** *only if* your Foundry account answers on
`…/openai/deployments/claude-sonnet-4-6/chat/completions` (test it — many don't), (b) put an OpenAI-compatible
gateway (LiteLLM, Azure APIM with the Anthropic→OpenAI policy) in front and register that, or (c) use Claude via
Copilot CLI only, where `COPILOT_PROVIDER_TYPE=anthropic` + base URL is documented and works. Establish which
applies to your tenant with T7 before promising Claude in the VS Code picker.

### VS Code local (Manage Models → Add Models)
- Mistral / Cohere: provider **Azure** → endpoint `https://<acct>.openai.azure.com`, deployment name, key.
- Claude: provider **Anthropic** accepts a key only (fixed endpoint) in current builds → not usable for Foundry
  Claude locally; use the enterprise path or the CLI.

### Copilot CLI
See T7. `COPILOT_MODEL` is always the **deployment name**.

---

## 6. Troubleshooting by symptom

Read the error **body**, not just the status. Azure, Anthropic-on-Foundry and GitHub all return JSON with a
`code`/`type`; the body tells you which of the three layers refused.

### HTTP status → layer

| Status | Rung | Meaning | First check |
|---|---|---|---|
| 401 | T2 | key wrong for this account, or rotated after registration | `az cognitiveservices account keys list` → compare first 6 chars with what's in AI controls |
| 403 `Public access is disabled` | T2 | network ACL | `publicNetworkAccess`, `networkAcls.defaultAction` |
| 403 `Key based authentication is not permitted` | T2 | `disableLocalAuth` | Terraform `local_auth_enabled = true` |
| 403 `content_filter` / `ResponsibleAIPolicyViolation` | T3–T6 | RAI policy on this deployment | prompt-dependent; custom RAI policy |
| 403 Claude only: `marketplace` / `subscription not eligible` | T2 | terms not accepted, or billing country not eligible | `az term show … --query accepted`; subscription billing region |
| 404 `Resource not found` | T2 | URL ≠ deployment; project URL used; no custom subdomain; Claude called on `/openai/…` | `deployment list --query [].name`; use `$AEP/v1/messages` for Claude |
| 400 (see per-model table) | T5/T6 | request shape the model rejects | body `message` |
| 408 / 504 | T3 | gateway timeout, usually a long thinking turn | lower `max_tokens`; disable thinking |
| 429 | any | TPM quota; Claude: Marketplace throughput cap | `usage list`; raise `sku.capacity` |
| 200 + empty content | T3/T4 | `finish_reason: length` (tokens spent on reasoning) or stream shape not parsed | see "Sorry, no response" below |

### Per-model 400s and quirks

**Mistral-Large-3**
- `tool_choice` must be `auto`, `none`, `any`, or a specific function; Mistral rejects `required` → Copilot sends
  `auto`, curl tests should too.
- Tool-call ids are 9-char alphanumerics; if you construct a T6 payload by hand with a different id format you
  get `400 Invalid tool_call_id`. Always echo the id the model produced.
- Function names must match `^[a-zA-Z0-9_-]{1,64}$`. Copilot's built-in tool names comply; user-defined MCP tools
  with dots do not → 400 in agent mode only. Rename the MCP tool.
- A `system` message after a `user` message is rejected; Copilot puts system first, so this only bites custom
  gateways.

**Cohere-command-a-plus**
- `tool_calls[].function.arguments` can come back as an **object** rather than a JSON string on the `/models`
  route; the `/openai/deployments/…` route stringifies it. Register Copilot against the `/openai/deployments`
  base (the Microsoft Foundry provider does) — if you used OpenAI-compatible with the `/models` base, agent mode
  fails to parse arguments and shows an empty response.
- Requires `tool_choice: "REQUIRED"` (its own spelling) to force a call on the native API; on the OpenAI route
  `auto` is honoured. Don't set `tool_choice: "required"`.
- Strict JSON-schema mode (`response_format: json_schema`) is not supported; Copilot doesn't use it, but MCP tools
  that request it will error.
- 256K context but throughput at GlobalStandard cap 50 is modest; large-repo agent runs hit 429 first. Raise
  capacity to 150 for team use.

**Claude Sonnet 4.6 on Foundry**
- `max_tokens` is **required**; omitting it is a 400 `max_tokens: Field required`. Copilot sets it; hand-written
  tests must too.
- `anthropic-version` header is required; missing → 400 `anthropic-version header is required`. Use `2023-06-01`.
- System prompt is a top-level `system` field, not a message. A `{"role":"system"}` message → 400. This is why an
  OpenAI-compatible provider pointed at `/anthropic/v1/messages` fails on every request — the shapes differ.
- Tool results go back as a `user` message containing `tool_result` blocks whose `tool_use_id` matches the
  `tool_use` block id. Reordered or missing → 400 `tool_use ids were found without tool_result blocks`. Copilot's
  `anthropic` provider does this correctly; the `openai`/`azure` provider cannot.
- Extended thinking is off unless requested; if you enable it on a gateway, `budget_tokens` counts against
  `max_tokens` and you'll reproduce the "no response after 30 s" symptom. Leave it off for Copilot.
- Region-locked: `eastus2` / `swedencentral` / `westus2` only. Global Standard means prompts may be processed in
  any region where the model runs — note this for data-residency reviews; Data Zone (US) is available for the
  Hosted-on-Azure sonnet SKU if that matters.
- Billing is Marketplace (Claude Consumption Units), so it does **not** appear under Cognitive Services in Cost
  Management — filter on the Marketplace meter for Anthropic when reconciling.

### "Sorry, no response was returned" (VS Code) — 200 but nothing renderable
1. Mistral/Cohere: `finish_reason: length` with empty content → Copilot's `max_tokens` too small for the
   prompt; usually an MCP tool with a huge schema in the request. Remove the tool, retry.
2. Cohere: arguments-as-object (above). Switch base to `/openai/deployments`.
3. Claude via wrong provider type: Anthropic stream events (`event: content_block_delta`) fed to an OpenAI parser
   → nothing rendered. Provider must be `anthropic`.
4. Any: stream closed without `[DONE]` / `message_stop`; run T4 and look at the tail.

### "works in curl, fails in Copilot agent mode" → almost always T6
Re-run T6 with the assistant turn copied verbatim from the model's own T5 response. If T6 passes in curl and fails
in Copilot, capture the request from the Chat Debug View and diff the `tool` / `tool_result` message against what
you sent by hand — id format, ordering, or a `system` message placement are the usual differences.

### Works in VS Code, not on github.com
Web chat only lists **enterprise** BYOK models. A model added via VS Code *Manage Models* is local. Register in
AI controls and grant CoolGitOrg.

### Worked yesterday, 401 today
Key rotated (`make rotate-key` / monthly workflow) but AI controls not updated. Rotate `key2`, update AI controls,
then rotate `key1` — never both in one go.

### Copilot says the model is unavailable / greyed out
Enterprise policy *Bring Your Own Language Model Key in VS Code* disabled (local BYOK), or the custom model is
Enabled but CoolGitOrg isn't on its Access tab (enterprise BYOK). Ask an owner; nothing on the Azure side.

---

## 7. Evidence to attach when escalating

- Output of §1 pre-flight and the §4 matrix.
- For a failing rung: the exact curl, the HTTP status, and the response body (redact the key).
- Azure request id from response headers: `apim-request-id` / `x-ms-request-id` (OpenAI-shape) or
  `request-id` (Anthropic-shape) — Microsoft support asks for these first.
- For Copilot-side failures: the request/response pair from VS Code's Chat Debug View or `~/.copilot/logs/`.
- `az cognitiveservices account deployment show … -o json` for the deployment (model version, sku, RAI policy).

Escalation routes: Azure support for anything reproducible in curl; GitHub support (Copilot) for anything that
passes T1–T6 but fails T7/T8; Anthropic isn't a support path for Foundry-hosted Claude — Microsoft is.
