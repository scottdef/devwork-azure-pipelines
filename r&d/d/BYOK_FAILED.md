# `byok_upstream_provider_failed` — diagnosis and resolution

Applies to: GitHub Copilot Chat (VS Code, JetBrains, github.com) using a custom model registered under
**CoolGitEnterprise → AI controls → Copilot → Custom models**, backed by an Azure AI Foundry deployment on
`ais-coolgit-copilot-prod` (`rg-coolgit-copilot`, `eastus2`).

---

## 1. What the error means

`byok_upstream_provider_failed` is emitted by **GitHub's Copilot API**, not by Azure. The request took the
enterprise BYOK path:

```
VS Code / github.com  ─►  api.githubcopilot.com  ─►  GitHub BYOK proxy  ─►  https://<acct>.openai.azure.com/...
                                                                             ▲ this hop failed
```

GitHub's proxy sent the request to the endpoint registered in AI controls using the key registered there, and got
back something it could not use: a non-2xx status, a body it could not parse, or a stream that closed early. GitHub
collapses all of those into this one code and **discards the upstream response body**, so a 401, a 404, a 400 and a
429 from Foundry all look identical to the user.

Two facts follow and drive everything below:

1. **Local tests do not exercise the failing hop.** `curl` from a laptop, the deploy workflow's validation step, and
   Copilot CLI with `COPILOT_PROVIDER_*` all go *laptop/runner → Foundry* with a key you fetched yourself. The failing
   hop is *GitHub's servers → Foundry* with the key and URL **typed into AI controls**. Either can differ.
2. **Azure has the real status code.** GitHub throws it away; Foundry logs it. Getting that number is step one.

---

## 2. Triage in ten minutes

### Step 1 — Is the request reaching Foundry at all?

Log Analytics (diagnostic settings from the Terraform stack):

```kusto
AzureDiagnostics
| where ResourceProvider == "MICROSOFT.COGNITIVESERVICES" and Category == "RequestResponse"
| where TimeGenerated > ago(30m)
| summarize count() by ResultSignature, OperationName, bin(TimeGenerated, 1m), CallerIPAddress
| order by TimeGenerated desc
```

No Log Analytics? Use the metric, split by status code:

```bash
ACCT_ID=$(az cognitiveservices account show -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --query id -o tsv)
az monitor metrics list --resource "$ACCT_ID" --metric AzureOpenAIRequests \
  --dimension StatusCode --interval PT1M --start-time "$(date -u -d '-30 min' +%FT%TZ)" \
  --query "value[0].timeseries[].{status:metadatavalues[0].value, count:sum(data[].total)}" -o table
```

| What you see at the minute you clicked | Meaning | Go to |
|---|---|---|
| Nothing at all | Request never arrived — network or wrong host | §3 causes 1, 2 |
| 401 | Key | cause 4 |
| 403 | Network ACL, local-auth disabled, RAI, Marketplace | causes 1, 6 |
| 404 | Deployment / path | causes 2, 3, 9 |
| 400 | Request body rejected | causes 5, 8 |
| 429 | Quota | cause 7 |
| 200 only | Foundry is happy; GitHub can't parse the reply | cause 8 |

### Step 2 — Reproduce GitHub's hop, not yours

From a **GitHub-hosted runner** (public egress, like GitHub's proxy), using the **values copied from AI controls**
(base URL, model id, key) — not from `az`:

```bash
curl -s -o body.json -w 'HTTP %{http_code}\n' \
  "$BASE_URL/openai/deployments/$MODEL_ID/chat/completions?api-version=2024-10-21" \
  -H "api-key: $KEY" -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"hi"}],"max_tokens":5,"temperature":0.1,"stream":false}'
jq . body.json
```

`temperature: 0.1` is deliberate — Copilot pins it and you cannot remove it (cause 5).

| Result | Conclusion |
|---|---|
| non-200 | that status is your cause; body names it |
| 200 here, error in Copilot | cause 1 (an allow-list that includes runners but not GitHub's proxy) or cause 8 |

### Step 3 — Isolate GitHub vs. client

Try the same custom model in **github.com Copilot Chat**. Both fail → upstream (this document). github.com works and
only the IDE fails → not this error path; check the IDE's model/policy config instead.

---

## 3. Causes and solutions

Ordered by how often each turns out to be the answer.

### 1. Network ACL blocks GitHub's proxy  (Azure logs: nothing)
Your laptop or VPN is on the account's IP allow-list; GitHub's egress isn't, and there is no published range narrow
enough to allow-list.
```bash
az cognitiveservices account show -g rg-coolgit-copilot -n ais-coolgit-copilot-prod \
  --query '{pna:properties.publicNetworkAccess, default:properties.networkAcls.defaultAction, ips:properties.networkAcls.ipRules[].value}'
```
**Fix:** enterprise BYOK requires `publicNetworkAccess = Enabled` and `networkAcls.defaultAction = Allow`. In
Terraform: `public_network_access_enabled = true`, drop `network_acls`. If policy forbids a public account, enterprise
BYOK is not viable for that account — use a separate BYOK-only account, or local BYOK/CLI from inside the network.

### 2. Base URL contains a path  (Azure logs: 404, or nothing if the host is wrong)
The Microsoft Foundry / Azure provider appends `/openai/deployments/<model id>/chat/completions?api-version=…` itself.
A base URL of `https://<acct>.openai.azure.com/openai/deployments/x` doubles the path; the Foundry *project* URL
(`…/api/projects/<name>`) has no chat route at all.

**Fix:** base URL = `https://<acct>.openai.azure.com` (or `https://<acct>.services.ai.azure.com` for partner
models) — host only, no trailing slash. Copy it from the deploy workflow's issue (`Endpoint` row), not from the portal
project page.

### 3. Model id is the catalog name, not the deployment name  (Azure logs: 404 `DeploymentNotFound`)
```bash
az cognitiveservices account deployment list -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --query "[].name" -o tsv
```
**Fix:** the id under *Available models* must be one of those names exactly (`chat-fallback`, `coder`,
`mistral-large-3`…), case-sensitive.

### 4. Key rotated, or key from the other account  (Azure logs: 401)
Monthly rotation ran and AI controls still holds the previous `key1`; or the dev key was pasted against the prod host.
```bash
az cognitiveservices account keys list -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --query key1 -o tsv | cut -c1-6
```
**Fix:** re-paste `key1`. Rotation order that avoids this: regenerate `key2` → update AI controls → regenerate `key1`.

### 5. Copilot pins request fields a reasoning model rejects  (Azure logs: 400 on every request)
Copilot sends `temperature: 0.1` (and other fixed fields) in every BYOK request; the BYOK config has no way to omit
them. Reasoning models — gpt-5 family, o-series, `*-reasoning` Grok/DeepSeek variants — return
`400 Unsupported parameter: 'temperature'`.

**Fix:** back the slot with a non-reasoning model (gpt-4.1 family, gpt-4o, Mistral-Large-3, Llama, Cohere,
`grok-*-non-reasoning`, DeepSeek-V3.2), or put a gateway (Azure APIM policy, LiteLLM) in front that strips the field.

### 6. Content filter or local-auth/Marketplace policy  (Azure logs: 403, or 200 with `content_filter`)
- `Key based authentication is not permitted` → `disableLocalAuth = true`; BYOK needs keys. Terraform
  `local_auth_enabled = true`.
- `ResponsibleAIPolicyViolation` / `content_filter` → the prompt or completion tripped the RAI policy. Prompt-dependent
  ("hello" works, one file fails). Assign a custom RAI policy to the deployment.
- Claude only: `marketplace` / `not eligible` → terms not accepted or billing region ineligible.

### 7. Quota  (Azure logs: 429)
Agent-mode turns carry large contexts; a 50K-TPM deployment saturates on one user.
```bash
az cognitiveservices usage list -l eastus2 --query "[?contains(name.value,'GlobalStandard')].{q:name.value,cur:currentValue,lim:limit}" -o table
```
**Fix:** `az cognitiveservices account deployment create … --sku-capacity 150` (same command is an in-place update),
after confirming subscription quota headroom.

### 8. Foundry returned 200 but GitHub could not parse it  (Azure logs: 200 only)
The one cause where Azure is satisfied. Known shapes:
- **Cohere** returns `tool_calls[].function.arguments` as an object on the `/models` route; register against the
  `/openai/deployments` route (Microsoft Foundry provider) which stringifies it.
- **Claude** registered under an OpenAI-style provider: Anthropic SSE events reach an OpenAI parser. Provider must
  be Anthropic with base URL `https://<acct>.services.ai.azure.com/anthropic`.
- Partner model streams that end without `data: [DONE]`. Verify with `curl -N … | tail -c 200`.
- Thinking models spending the whole `max_tokens` on `reasoning_content` → `finish_reason: length`, no content.
  Use the non-thinking variant.

### 9. Deployment removed or retired  (Azure logs: 404)
Decommissioned in Azure, or the model version hit its retirement date, while AI controls still lists it.
```bash
az cognitiveservices account deployment show -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --deployment-name <id> --query 'properties.{state:provisioningState, model:model.name, ver:model.version}'
```
**Fix:** redeploy under the same name (users' `COPILOT_MODEL` stays valid) or remove the entry from AI controls.
Check `models/catalog/eastus2.json` deprecation dates before choosing a replacement version.

---

## 4. Resolution checklist

Work down; stop at the first box that was wrong, fix it, retest with Step 2, then in Copilot.

- [ ] Azure logs/metrics show requests arriving from GitHub at click time (else → cause 1 / 2 host)
- [ ] `publicNetworkAccess = Enabled`, `networkAcls.defaultAction = Allow`
- [ ] AI controls base URL is host-only, no path, no trailing slash, correct account
- [ ] AI controls model id ∈ `deployment list --query [].name`
- [ ] AI controls key prefix matches current `key1`
- [ ] Backing model is non-reasoning, or a gateway strips `temperature`
- [ ] `disableLocalAuth` false; RAI policy not blocking the failing prompt
- [ ] TPM quota not saturated (no 429s)
- [ ] Provider type matches the wire API (Foundry/Azure ↔ Chat Completions, Anthropic ↔ Messages)
- [ ] Deployment exists, `Succeeded`, model version not retired
- [ ] Same model works on github.com Copilot Chat

---

## 5. Prevent recurrence

- **Validate the registered values, not fresh ones.** Add a nightly workflow that reads the base URL and model id
  from `deployments/<account>/<name>.json` and probes with the current `key1` from a public runner, alerting on
  non-200. It won't catch a stale key in AI controls (no API exposes it) — which is why the next item exists.
- **Rotation runbook**: `key2` → AI controls → `key1`. Never regenerate both keys in one change.
- **Prefer non-reasoning models for BYOK slots.** Reasoning models fight Copilot's pinned parameters and spend
  budget on hidden tokens; keep them for the CLI where `COPILOT_PROVIDER_*` gives more control.
- **Stable deployment names** (`chat-fallback`, `coder`): swap the backing model without touching AI controls.
- **Watch retirement dates.** The catalog-refresh workflow already flags models that leave the region; extend the
  drift check to `model.deprecation.inference` within 60 days.
- **Keep the account public.** Enterprise BYOK cannot work through a private endpoint or IP allow-list; if that
  is unacceptable, this feature is the wrong tool and per-user CLI BYOK from inside the network is the fallback.

---

## 6. Escalation

Evidence for GitHub support (Copilot): the exact error string, timestamp (UTC), enterprise/org name, model id as
registered, and proof from Step 2 that the endpoint answers 200 to the same request from a public runner.
Evidence for Azure support: `apim-request-id` / `x-ms-request-id` from the Step 2 response headers, the deployment
JSON, and the Log Analytics rows. GitHub cannot see Foundry's body; Azure cannot see GitHub's proxy. Bring both
halves to whichever side the status code points at.
