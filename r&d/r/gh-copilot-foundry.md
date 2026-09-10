# Integrating GitHub Copilot with Azure AI Foundry Model Deployments (Qwen) for GitHub Enterprise Cloud

## TL;DR
- **Yes, this works and is officially supported.** GitHub Copilot Enterprise BYOK ("custom models") lets the `CoolGitEnterprise` owner register a Microsoft Foundry API key so Qwen models appear in the Copilot model picker for `CoolGitOrg` users in Copilot Chat on github.com, VS Code, JetBrains, and Copilot CLI — and BYOK usage is billed by Azure, **not** against Copilot's AI Credit allotment, making it a valid emergency fallback.
- **The exact model names you asked for do not exist.** As of September 2026 the Foundry catalog has **no** `qwen3.7` or `qwen3.8-flash`. Map to what is real: `Qwen3-Coder-Next` (80B-total/3B-active MoE, managed compute) and `qwen3-32b` (Direct-from-Azure, serverless pay-as-you-go) are the practical choices; `Qwen3-Coder-480B-A35B-Instruct` is available via other clouds/OpenRouter but not confirmed as a Direct-from-Azure serverless SKU.
- **Config is a UI + IaC hybrid.** BYOK key registration itself is **UI-only** (no REST API); everything below it — Foundry deployment, key rotation, cost monitoring, org secret distribution — is fully automatable with Terraform, `az`, `gh`, and GitHub Actions with Azure OIDC.

---

## Table of Contents
1. Verified facts vs. assumptions
2. Architecture overview
3. Qwen model availability in Azure AI Foundry (Sept 2026)
4. GitHub Copilot BYOK / custom models — surfaces, providers, plans
5. github.com web chat BYOK support
6. GitHub Enterprise/org policy setup + automation APIs
7. Copilot billing model (AI Credits) and the fallback rationale
8. Terraform: Foundry + Qwen deployment
9. Bash / `az` CLI equivalents + endpoint validation
10. Cost monitoring (budgets, metrics, KQL, daily report workflow)
11. End-user guide (VS Code, CLI, github.com)
12. Automation glue (OIDC, key rotation, docs commit, downstream trigger)
13. Security considerations
14. Makefile
15. Known gotchas appendix

---

## 1. Verified facts vs. assumptions

**Confirmed from official sources:**
- **Enterprise BYOK is real and in public preview** since Nov 20, 2025 (GitHub Changelog). Supported providers include Anthropic, Microsoft Foundry, OpenAI, and xAI; per the GitHub Changelog (2026-01-15): "You can now connect API keys from AWS Bedrock, Google AI Studio, and any OpenAI‑compatible provider. These options join Anthropic, Microsoft Foundry, OpenAI, and xAI." That same update added Responses-API support and a configurable max context window.
- **BYOK usage is billed by the provider and does not count against Copilot request/credit quotas** (GitHub Changelog, GitHub Docs BYOK page).
- **VS Code BYOK for Business/Enterprise went GA April 22, 2026.** Per GitHub Changelog 2026-04-22: "Copilot Business and Enterprise users can now use bring your own language model key (BYOK) in Visual Studio Code… The policy is enabled by default." It is governed by the "Bring Your Own Language Model Key in VS Code" policy.
- **Copilot moved from Premium Request Units to GitHub AI Credits on June 1, 2026.** Per GitHub Docs: "Starting June 1, 2026, Copilot usage consumes AI Credits instead of premium request units… 1 AI Credit equal to $0.01 USD." Individual allotments are Pro 1,500 / Pro+ 7,000 / Max 20,000 credits.
- **Copilot CLI BYOK** uses `COPILOT_PROVIDER_BASE_URL`, `COPILOT_PROVIDER_TYPE` (`openai`/`azure`/`anthropic`), `COPILOT_PROVIDER_API_KEY`, `COPILOT_MODEL` (GitHub Docs).
- **Foundry catalog Qwen models** confirmed present: `qwen3-32b` (Direct-from-Azure), `Qwen3-Coder-Next`, `qwen-qwen3-8b`, the Qwen3.5 medium series, Qwen3.6 (via the Hugging Face collection / managed compute) — from ai.azure.com catalog pages and Microsoft Community Hub.
- **Foundry token metrics**: `TokenTransaction`, `ProcessedPromptTokens`, `GeneratedTokens` on `Microsoft.CognitiveServices/accounts` (Microsoft Learn).
- **Enterprise policy REST endpoints require classic PATs** with `manage_billing:copilot` or `admin:enterprise` — fine-grained PATs are explicitly unsupported (GitHub REST docs).

**Could NOT be verified / negative findings:**
- **`qwen3.7` and `qwen3.8-flash` do not exist** in the Foundry catalog under those names. (There is a `Qwen/Qwen3.8-27B` referenced on third-party serverless hosts like Runpod, but not as a Foundry Direct-from-Azure SKU.)
- **No REST API exists for BYOK/custom-model key registration or the "Enable custom models" policy** — it is UI-only (AI controls). Confirmed via a dedicated review of GitHub's REST reference.
- **Exact Foundry pay-as-you-go per-token pricing for Qwen** is not publicly listed as a stable Direct-from-Azure price for the coder models; third-party prices ($0.22/$1.80 per 1M in/out for Qwen3-Coder-480B on Bedrock/OpenRouter) are indicative only.

---

## 2. Architecture overview

```
                     CoolGitEnterprise (GHEC)
                     └── AI controls → Copilot → Custom models (BYOK)  [UI only]
                            │  registers Foundry API key + selects Qwen deployment
                            ▼
   Developer surfaces                         Azure subscription
   ┌───────────────────┐                      ┌─────────────────────────────┐
   │ VS Code Chat      │   OpenAI-compatible  │ AI Foundry (Cognitive Acct, │
   │ github.com Chat   │───/chat/completions─▶│  kind=AIServices)           │
   │ Copilot CLI       │      + api-key       │   ├─ deployment: qwen3-32b   │
   └───────────────────┘                      │   └─ deployment: qwen3-coder │
            ▲                                  │ Key Vault (endpoint keys)   │
            │ model picker                     │ Log Analytics + Budgets     │
   ┌────────┴─────────┐                        └──────────────┬──────────────┘
   │ GitHub Actions   │  OIDC (no secrets)                    │ metrics/cost
   │ terraform apply  │───────────────────────────────────────┘
   │ rotate-key → gh secret set → repository_dispatch → Slack  │
   └──────────────────┘
```

Text is the interface throughout: Terraform emits JSON outputs, bash consumes them, `gh` and `az` speak over stdout, the Makefile is the single front door. Each component does one thing; they compose.

---

## 3. Qwen model availability in Azure AI Foundry (September 2026)

Foundry splits into two collections with **different deployment mechanics**, which determines everything downstream:

| Collection | Deployment type | Billing | OpenAI-compatible endpoint | Terraform resource |
|---|---|---|---|---|
| **Foundry Models sold by Azure** ("Direct from Azure") | Serverless API (Global/DataZone Standard, pay-per-token) | Azure subscription, SLA, no Marketplace step | Yes — `/openai/v1/` or `/models` route | `azurerm_cognitive_deployment` on a `kind=AIServices` account |
| **Foundry Models from partners & community** (Hugging Face Qwen) | Managed compute (dedicated GPU) or serverless where offered | Per-hour GPU (managed compute) or per-token; some need Azure Marketplace subscription | Yes — vLLM OpenAI-compatible; route `/managed-deployments/<name>/` or `/openai/v1/` | `azapi_resource` (managed compute deployment) |

**Mapping your requested names to reality:**

- **`qwen3-coder`** → **`Qwen3-Coder-Next`**: an 80B-total / 3B-active MoE (512 experts, 10 activated + 1 shared) with native 262,144-token context and the `qwen3_coder` tool-call parser. Per Qwen's Hugging Face card: "With only 3B activated parameters (80B total parameters), it achieves performance comparable to models with 10–20x more active parameters." It lives in the Hugging Face collection → **managed compute** (H100/A100 dedicated). This is the correct coding model for Copilot agent/CLI use. `Qwen3-Coder-480B-A35B-Instruct` exists broadly across clouds but is **not confirmed** as a Direct-from-Azure serverless SKU; treat it as a managed-compute option if present on the HF registry.
- **`qwen3.7`** → **does not exist.** Closest real: **`qwen3-32b`** (Direct-from-Azure, serverless). Per the Microsoft Foundry catalog: "It features 32.8 billion parameters, a 131K token context window, support for 100+ languages, and hybrid thinking modes." It is the recommended serverless fallback because it needs no GPU reservation. The Qwen3.5 medium series also exists but they are all Vision Language Models (262K native context, 201-language support, ranging from a 27B dense model to a 122B sparse MoE with 10B active, Apache 2.0), so they are not general chat-coding drop-ins.
- **`qwen3.8-flash`** → **does not exist** in Foundry. There is no "flash" Qwen SKU sold by Azure. Closest low-latency option: `Qwen3-Coder-Next` (a low-latency coding agent) or the smaller `qwen-qwen3-8b` (managed compute) for a cheap fast tier.

**Recommendation:** Use **`qwen3-32b` (serverless, Direct-from-Azure)** as the always-on fallback model (no idle GPU cost) and **`Qwen3-Coder-Next` (managed compute)** only when you need agentic coding quality and can tolerate per-hour GPU billing. Both expose OpenAI-compatible chat completions, which is the hard requirement for Copilot BYOK.

**Region choice:** Direct-from-Azure serverless Qwen availability follows the "Region availability for Foundry Models sold by Azure" matrix (Microsoft Learn). Managed-compute HF Qwen requires a region with H100/A100 quota (commonly East US, East US 2, Sweden Central, West US 3). Pick a region that has **both** your Direct-from-Azure SKU and GPU quota; East US 2 and Sweden Central are safe defaults as of Sept 2026 — verify against the live matrix before applying.

---

## 4. GitHub Copilot BYOK / custom models — surfaces, providers, plans

**Two distinct mechanisms (do not conflate them):**

1. **Local BYOK** — a user configures a key on their own machine (VS Code "Manage Models", Copilot CLI env vars, JetBrains, Eclipse, Xcode, the GitHub Copilot app). Keys are client-side only; models are not shared. Can be disabled for Business/Enterprise by the "Bring Your Own Language Model Key in VS Code" policy.
2. **Enterprise/Org BYOK ("custom models")** — the enterprise or org owner registers a key server-side; the models are served to all licensed users via the Copilot API and appear in the model picker under the enterprise/org name. This is the mechanism you want for `CoolGitOrg`.

**Supported providers (Sept 2026):** Anthropic, Microsoft Foundry, OpenAI, xAI, AWS Bedrock, Google AI Studio, and any OpenAI-compatible endpoint. **Microsoft Foundry is a first-class named provider.**

**Plan availability:** Enterprise/org BYOK is public preview for **GitHub Enterprise and Business**. Local VS Code BYOK became available to Business/Enterprise on April 22, 2026 (previously Individual/Pro only). BYOK applies to **chat and agent** features (Ask/Edit/Agent/Plan modes), **not** code completions.

**Constraints that matter for Qwen:**
- Models must support **tool calling (function calling) and streaming** or Copilot CLI errors out.
- Recommended context window ≥ 128k (Qwen3-Coder-Next's 262k and qwen3-32b's 131k both qualify).
- Enterprise BYOK originally required the **OpenAI Completions API** (the OpenAI Responses API was unsupported); the 2026-01-15 changelog added Responses-API support for some BYOK paths — verify per surface.
- **The `reasoning_content` 400 gotcha:** For OpenAI-compatible providers that emit `reasoning_content` on thinking-mode responses without an `id` field (documented for DeepSeek; Qwen thinking models behave similarly), the `openai`/`azure` provider path in Copilot CLI/VS Code can fail on multi-turn tool calling with `400 "The reasoning_content in the thinking mode must be passed back to the API."` The documented workaround is to use an **Anthropic-Messages-compatible endpoint** (`COPILOT_PROVIDER_TYPE=anthropic`) or to run the Qwen model in **non-thinking mode**. For Qwen3-Coder-Next (a non-reasoning coder model) this is largely a non-issue; for Qwen "thinking" variants it is a real risk.

---

## 5. github.com web chat BYOK support

**Confirmed: yes.** The GitHub Changelog states BYOK models "can be used in Copilot Chat on GitHub.com and in supported IDEs." Enterprise/org custom models appear in the github.com Copilot Chat model picker under the org/enterprise name, same as in IDEs. This is **server-side enterprise BYOK**, not local BYOK — local client-side keys (the VS Code "Manage Models" flow) do **not** propagate to github.com web chat. So for web-chat coverage you must use the enterprise/org custom-models registration, not per-developer local keys.

---

## 6. GitHub Enterprise/org policy setup + automation APIs

### 6.1 Manual setup (owner, UI — the only supported path for BYOK)
As `CoolGitEnterprise` owner:
1. Enterprise → **AI controls** → **Copilot** → **Configure custom models** → **Add API key**.
2. Provider = **Microsoft Foundry** (or "OpenAI-compatible"/"Azure"); Name = shown in picker (e.g., `Foundry-Qwen`); paste the Foundry key; set base URL/deployment.
3. Under **Available models**, select the Qwen deployment(s); set max context window.
4. To let `CoolGitOrg` owners manage their own keys, enable the **Enable custom models** policy at the enterprise, then org owners repeat under Org → Settings → Copilot → Models → Custom models.
5. Set access per organization on the model's **Access** tab (model must be **Enabled** first).

### 6.2 What you CAN automate via REST (`gh api`)
BYOK key registration and the "Enable custom models" toggle are **UI-only — no REST endpoint exists.** But surrounding governance and reporting is scriptable. Enterprise-scope endpoints require a **classic PAT** (`manage_billing:copilot` or `admin:enterprise`); fine-grained PATs are explicitly rejected.

| Purpose | Method + path | Token |
|---|---|---|
| Set enterprise coding-agent policy | `PUT /enterprises/{enterprise}/copilot/policies/coding_agent` | classic PAT `manage_billing:copilot`/`admin:enterprise` |
| Add/remove orgs to that policy | `POST`/`DELETE /enterprises/{enterprise}/copilot/policies/coding_agent/organizations` | same |
| Org Copilot settings + seats (read) | `GET /orgs/{org}/copilot/billing` | classic `manage_billing:copilot`/`read:org` |
| Copilot usage metrics (enterprise) | `GET /enterprises/{enterprise}/copilot/metrics/reports/{report_type}/{day}` | classic `manage_billing:copilot`/`read:enterprise` |
| Copilot usage metrics (org) | `GET /orgs/{org}/copilot/metrics/reports/...` | classic `manage_billing:copilot`/`read:org` |
| Enterprise AI credit usage | `GET /enterprises/{enterprise}/settings/billing/ai_credit/usage` | enterprise admin/billing mgr (classic) |
| Enterprise premium request usage | `GET /enterprises/{enterprise}/settings/billing/premium_request/usage` | classic; not fine-grained/App |
| Org AI credit / premium usage | `GET /organizations/{org}/settings/billing/ai_credit/usage` (and `/premium_request/usage`) | org admin |

⚠️ The **legacy** `GET /orgs/{org}/copilot/metrics` and `GET /enterprises/{enterprise}/copilot/metrics` endpoints were **closed down April 2, 2026** — use the `.../metrics/reports/...` usage-metrics endpoints.

```bash
# Read AI-credit consumption for the enterprise (classic PAT with manage_billing:copilot)
gh api \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2026-03-10" \
  /enterprises/CoolGitEnterprise/settings/billing/ai_credit/usage

# Pull org-level Copilot settings/seat policies (read-only)
gh api /orgs/CoolGitOrg/copilot/billing
```

---

## 7. Copilot billing model (AI Credits) and the fallback rationale

- Since **June 1, 2026**, all Copilot plans use **usage-based billing** metered in **GitHub AI Credits** (1 credit = $0.01), replacing Premium Request Units. Cost = input + output + cached tokens × published per-model API rate. Model multipliers are gone.
- **Included monthly credits.** Individual: Pro 1,500 / Pro+ 7,000 / Max 20,000 credits. For organizations, per GitHub Docs: "Business and Enterprise receive pooled monthly allowances of 1,900 and 3,900 credits per user, with temporary promotional allowances of 3,000 and 7,000 from June 1 to September 1, 2026." After Sept 1, 2026 the Business/Enterprise allowances revert to the standard 1,900 / 3,900. Balances are **pooled at the billing entity**, not isolated per user.
- **Code completions and Next Edit Suggestions remain free** (not credit-metered). Chat, agent sessions, code review, and PR summaries consume credits.
- **When the allotment is exhausted:** there is **no automatic downgrade to a cheaper model**. Usage either continues as **paid additional usage** (if the enterprise/org owner allows overage via a budget) or the request is **blocked**. Owners control this with **budgets** (Settings → Billing → Budgets) — set the Copilot budget to $0 to hard-cap, or set a positive cap to allow controlled overage.
- **Why BYOK is the fallback:** BYOK/custom-model usage is **billed by Azure and does not consume AI Credits or count against Copilot quotas.** So when the pooled credit balance is gone and overage is capped, developers can switch the Copilot model picker to the Foundry Qwen model and keep working — spend moves to the Azure subscription (which you monitor with the budgets/metrics in §10). This is the core operational value: **decouple "out of Copilot credits" from "can't use Copilot."**

---

## 8. Terraform: Foundry + Qwen deployment

Provider versions: pin `azurerm ~> 4.x` and `azapi ~> 2.x` (latest stable as of Sept 2026). `azurerm_ai_foundry`/`azurerm_ai_foundry_project` are **hub-based (legacy)**; Microsoft now recommends the **new Foundry resource provisioned via `azurerm_cognitive_account`** (kind `AIServices`). Direct-from-Azure Qwen deploys with `azurerm_cognitive_deployment`; Hugging Face managed compute needs `azapi_resource`.

### providers.tf
```hcl
terraform {
  required_version = ">= 1.9.0"
  required_providers {
    azurerm = { source = "hashicorp/azurerm", version = "~> 4.40" }
    azapi   = { source = "Azure/azapi",       version = "~> 2.5"  }
    random  = { source = "hashicorp/random",  version = "~> 3.6"  }
  }
  backend "azurerm" {
    resource_group_name  = "rg-tfstate"
    storage_account_name = "sttfstatecoolgit"
    container_name       = "tfstate"
    key                  = "copilot-foundry-qwen.tfstate"
    use_oidc             = true            # ARM_USE_OIDC=true in CI
  }
}

provider "azurerm" {
  features {
    cognitive_account { purge_soft_delete_on_destroy = true }
    key_vault         { purge_soft_delete_on_destroy = false }
  }
  use_oidc = true
}

provider "azapi" {
  use_oidc = true
}
```

### variables.tf
```hcl
variable "location"        { type = string  default = "eastus2" }
variable "resource_prefix" { type = string  default = "coolgit-copilot" }
variable "tenant_id"       { type = string }
variable "admin_object_id" { type = string  description = "Entra objectId of platform admin group" }

# Direct-from-Azure (serverless) Qwen deployments
variable "serverless_deployments" {
  type = list(object({
    name         = string
    model_name   = string
    model_format = string   # "OpenAI" for Azure-native; provider name otherwise
    model_version= string
    sku_name     = string   # e.g. GlobalStandard | DataZoneStandard
    sku_capacity = number   # TPM in thousands
  }))
  default = [{
    name          = "qwen3-32b"
    model_name    = "qwen3-32b"
    model_format  = "Qwen"
    model_version = "1"
    sku_name      = "GlobalStandard"
    sku_capacity  = 50
  }]
}

variable "budget_amount" { type = number default = 2000 }
variable "alert_emails"  { type = list(string) default = ["finops@coolgit.example"] }
variable "tags" {
  type = map(string)
  default = { project = "copilot-foundry-qwen", managed_by = "terraform", cost_center = "platform" }
}
```

### main.tf
```hcl
data "azurerm_subscription" "current" {}

resource "azurerm_resource_group" "ai" {
  name     = "rg-${var.resource_prefix}"
  location = var.location
  tags     = var.tags
}

resource "random_string" "suffix" {
  length  = 6
  special = false
  upper   = false
}

# Foundry / AI Services account (new Foundry model, NOT legacy hub)
resource "azurerm_cognitive_account" "foundry" {
  name                  = "ais-${var.resource_prefix}-${random_string.suffix.result}"
  location              = azurerm_resource_group.ai.location
  resource_group_name   = azurerm_resource_group.ai.name
  kind                  = "AIServices"
  sku_name              = "S0"
  custom_subdomain_name = "ais-${var.resource_prefix}-${random_string.suffix.result}"
  local_auth_enabled    = true   # key-based auth needed for Copilot BYOK; see §13
  identity { type = "SystemAssigned" }
  tags = var.tags
}

# Log Analytics + diagnostic settings for token metrics
resource "azurerm_log_analytics_workspace" "law" {
  name                = "law-${var.resource_prefix}"
  location            = azurerm_resource_group.ai.location
  resource_group_name = azurerm_resource_group.ai.name
  sku                 = "PerGB2018"
  retention_in_days   = 90
  tags                = var.tags
}

resource "azurerm_monitor_diagnostic_setting" "foundry" {
  name                       = "diag-foundry"
  target_resource_id         = azurerm_cognitive_account.foundry.id
  log_analytics_workspace_id = azurerm_log_analytics_workspace.law.id
  enabled_metric { category = "AllMetrics" }
  enabled_log    { category_group = "allLogs" }
}

# Key Vault to store endpoint keys
resource "azurerm_key_vault" "kv" {
  name                      = "kv-${var.resource_prefix}-${random_string.suffix.result}"
  location                  = azurerm_resource_group.ai.location
  resource_group_name       = azurerm_resource_group.ai.name
  tenant_id                 = var.tenant_id
  sku_name                  = "standard"
  enable_rbac_authorization = true
  purge_protection_enabled  = true
  tags                      = var.tags
}

resource "azurerm_role_assignment" "kv_admin" {
  scope                = azurerm_key_vault.kv.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = var.admin_object_id
}

resource "azurerm_key_vault_secret" "foundry_key" {
  name         = "foundry-primary-key"
  value        = azurerm_cognitive_account.foundry.primary_access_key
  key_vault_id = azurerm_key_vault.kv.id
  depends_on   = [azurerm_role_assignment.kv_admin]
}

# RBAC: allow developers/CI to call the endpoint with Entra ID (preferred over shared key)
resource "azurerm_role_assignment" "cog_user" {
  scope                = azurerm_cognitive_account.foundry.id
  role_definition_name = "Cognitive Services User"
  principal_id         = var.admin_object_id
}
```

### deployments.tf
```hcl
# Direct-from-Azure (serverless, pay-per-token) Qwen deployments
resource "azurerm_cognitive_deployment" "serverless" {
  for_each             = { for d in var.serverless_deployments : d.name => d }
  name                 = each.value.name
  cognitive_account_id = azurerm_cognitive_account.foundry.id

  model {
    format  = each.value.model_format
    name    = each.value.model_name
    version = each.value.model_version
  }
  sku {
    name     = each.value.sku_name
    capacity = each.value.sku_capacity  # TPM
  }

  # Optional RAI / content-filter policy binding
  rai_policy_name = "Microsoft.DefaultV2"

  lifecycle { ignore_changes = [model[0].version] }  # let Foundry auto-upgrade
}

# Hugging Face managed-compute deployment (Qwen3-Coder-Next) via azapi.
# Direct-from-Azure resources cannot express managed compute; use the
# Microsoft.CognitiveServices managed deployment shape.
resource "azapi_resource" "qwen_coder_next" {
  type      = "Microsoft.CognitiveServices/accounts/deployments@2025-06-01"
  name      = "qwen3-coder-next"
  parent_id = azurerm_cognitive_account.foundry.id
  body = {
    sku = { name = "GlobalManagedCompute", capacity = 1 }
    properties = {
      model = {
        format  = "HuggingFace"
        name    = "azureml://registries/azure-huggingface/models/qwen--qwen3-coder-next/versions/1"
      }
      # deployment template pins runtime/accelerator; H100 single-accelerator
      deploymentTemplate = "azureml://registries/azure-huggingface/deploymenttemplates/qwen--qwen3-coder-next--256k-nvidia-h100/labels/latest"
      acceleratorType    = "H100_80GB"
    }
  }
  schema_validation_enabled = false
  tags = var.tags
}
```

> Note: The managed-compute `azapi` body shape mirrors the `managed_compute_deployments` REST/SDK surface Microsoft documents for Hugging Face models; template IDs and API versions change frequently — confirm the current `deploymenttemplates` ID and `@version` against `az cognitiveservices model list` / the HF-on-Foundry docs before applying.

### outputs.tf
```hcl
output "foundry_endpoint" { value = azurerm_cognitive_account.foundry.endpoint }
output "foundry_account_name" { value = azurerm_cognitive_account.foundry.name }
output "resource_group_name" { value = azurerm_resource_group.ai.name }
output "key_vault_name" { value = azurerm_key_vault.kv.name }
output "serverless_deployment_names" {
  value = [for d in azurerm_cognitive_deployment.serverless : d.name]
}
output "log_analytics_workspace_id" { value = azurerm_log_analytics_workspace.law.id }
```

### budgets.tf (see §10)
```hcl
resource "azurerm_monitor_action_group" "cost" {
  name                = "ag-copilot-cost"
  resource_group_name = azurerm_resource_group.ai.name
  short_name          = "cpltcost"
  email_receiver {
    name          = "finops"
    email_address = var.alert_emails[0]
  }
  # Slack via webhook (Actions relays; see §12) or direct:
  # webhook_receiver { name = "slack" service_uri = var.slack_webhook }
}

resource "azurerm_consumption_budget_resource_group" "ai" {
  name              = "budget-${var.resource_prefix}"
  resource_group_id = azurerm_resource_group.ai.id
  amount            = var.budget_amount
  time_grain        = "Monthly"
  time_period {
    start_date = formatdate("YYYY-MM-01'T'00:00:00Z", timestamp())
  }
  notification {
    enabled        = true
    threshold      = 80
    operator       = "GreaterThanOrEqualTo"
    threshold_type = "Actual"
    contact_emails = var.alert_emails
    contact_groups = [azurerm_monitor_action_group.cost.id]
  }
  notification {
    enabled        = true
    threshold      = 100
    operator       = "GreaterThanOrEqualTo"
    threshold_type = "Forecasted"
    contact_groups = [azurerm_monitor_action_group.cost.id]
  }
  lifecycle { ignore_changes = [time_period] }
}
```

---

## 9. Bash / `az` CLI equivalents + endpoint validation

### deploy-models.sh
```bash
#!/usr/bin/env bash
set -euo pipefail

RG="rg-coolgit-copilot"
ACCT="$(terraform -chdir=infra output -raw foundry_account_name)"

# Direct-from-Azure serverless deployment (pay-per-token)
az cognitiveservices account deployment create \
  --resource-group "$RG" \
  --name "$ACCT" \
  --deployment-name "qwen3-32b" \
  --model-name "qwen3-32b" \
  --model-version "1" \
  --model-format "Qwen" \
  --sku-name "GlobalStandard" \
  --sku-capacity 50

# List what models/versions are actually deployable in this region (authoritative)
az cognitiveservices account list-models \
  --resource-group "$RG" --name "$ACCT" \
  --query "[?contains(name,'qwen')].{name:name,format:format,version:version}" -o table
```

Managed-compute (Qwen3-Coder-Next) is best driven via `az rest`/SDK against the `managed_compute_deployments` surface, or via the `azapi` resource in §8; the classic `az ml` serverless-endpoint path applies only to partner models that publish a serverless SKU and may require an **Azure Marketplace subscription** first (`az ml` / portal "Subscribe and deploy").

### validate.sh — curl the OpenAI-compatible endpoint
```bash
#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="$(terraform -chdir=infra output -raw foundry_endpoint)"   # https://<sub>.cognitiveservices.azure.com/ or .openai.azure.com
KEY="$(az keyvault secret show --vault-name "$(terraform -chdir=infra output -raw key_vault_name)" \
        --name foundry-primary-key --query value -o tsv)"
DEPLOY="qwen3-32b"
API_VERSION="2024-10-21"   # Azure OpenAI-style route; verify current GA version

# Shape A: Azure OpenAI-style route (deployment in path)
curl -sS "${ENDPOINT}openai/deployments/${DEPLOY}/chat/completions?api-version=${API_VERSION}" \
  -H "Content-Type: application/json" \
  -H "api-key: ${KEY}" \
  -d '{"messages":[{"role":"user","content":"Reply with the single word: OK"}],"max_tokens":8}' \
  | tee /tmp/qwen_resp.json | jq -r '.choices[0].message.content'

# Shape B: Foundry unified /models route (model in body) — for models-as-a-service
curl -sS "${ENDPOINT}models/chat/completions?api-version=2024-05-01-preview" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d "{\"model\":\"${DEPLOY}\",\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"max_tokens\":8}" \
  | jq -r '.choices[0].message.content'

# Entra-ID (keyless) variant — preferred for CI:
TOKEN="$(az account get-access-token --resource https://cognitiveservices.azure.com --query accessToken -o tsv)"
curl -sS "${ENDPOINT}openai/deployments/${DEPLOY}/chat/completions?api-version=${API_VERSION}" \
  -H "Content-Type: application/json" -H "Authorization: Bearer ${TOKEN}" \
  -d '{"messages":[{"role":"user","content":"ping"}],"max_tokens":8}' | jq .
```

> URL shape depends on account kind: Azure OpenAI-style accounts use `/openai/deployments/<name>/chat/completions?api-version=`; the newer Foundry unified inference uses `/models/chat/completions?api-version=` with the model in the body, and managed-compute uses `/managed-deployments/<name>/`. Validate both shapes and lock the one your account returns 200 on. Always test a streamed response **and** a follow-up tool-call turn — a plain-text ping only proves auth + text generation, not the tool-calling path Copilot depends on.

---

## 10. Cost monitoring

### 10.1 Metrics + KQL
Foundry token metrics on `Microsoft.CognitiveServices/accounts`, grouped by the `ModelDeploymentName` dimension:
- `ProcessedPromptTokens` — input tokens
- `GeneratedTokens` — output/completion tokens
- `TokenTransaction` — total processed inference tokens
- `AzureOpenAIRequests` — request count

KQL against Log Analytics (diagnostic setting from §8):
```kusto
// Daily token usage per Qwen deployment, last 30 days
AzureMetrics
| where ResourceProvider == "MICROSOFT.COGNITIVESERVICES"
| where MetricName in ("ProcessedPromptTokens", "GeneratedTokens", "TokenTransaction")
| where TimeGenerated > ago(30d)
| summarize Tokens = sum(Total)
    by bin(TimeGenerated, 1d), MetricName, Resource
| order by TimeGenerated asc
| render timechart
```
```kusto
// Estimated cost per day (plug your negotiated per-1M rates)
let inRate = 0.00000022;   // $/input token  (indicative Qwen3-Coder rate)
let outRate = 0.0000018;   // $/output token
AzureMetrics
| where MetricName in ("ProcessedPromptTokens","GeneratedTokens")
| where TimeGenerated > ago(30d)
| summarize toks = sum(Total) by MetricName, Day = bin(TimeGenerated, 1d)
| extend cost = iff(MetricName=="ProcessedPromptTokens", toks*inRate, toks*outRate)
| summarize DailyUSD = round(sum(cost),2) by Day
| order by Day asc
```

### 10.2 Daily cost report → GitHub Issue / Slack (workflow)
```yaml
name: foundry-cost-report
on:
  schedule: [{ cron: "0 6 * * *" }]   # 06:00 UTC daily
  workflow_dispatch:
permissions:
  id-token: write      # OIDC to Azure
  contents: read
  issues: write        # post the report to an issue
jobs:
  cost:
    runs-on: ubuntu-latest
    steps:
      - uses: azure/login@v2
        with:
          client-id: ${{ vars.AZURE_CLIENT_ID }}
          tenant-id: ${{ vars.AZURE_TENANT_ID }}
          subscription-id: ${{ vars.AZURE_SUBSCRIPTION_ID }}
      - name: Query yesterday's cost for the RG
        id: q
        run: |
          set -euo pipefail
          RG="rg-coolgit-copilot"
          SCOPE="/subscriptions/${{ vars.AZURE_SUBSCRIPTION_ID }}/resourceGroups/${RG}"
          FROM=$(date -u -d 'yesterday' +%Y-%m-%dT00:00:00Z)
          TO=$(date -u -d 'yesterday' +%Y-%m-%dT23:59:59Z)
          az costmanagement query \
            --type ActualCost --timeframe Custom \
            --time-period from="$FROM" to="$TO" \
            --scope "$SCOPE" \
            --dataset-granularity Daily \
            --dataset-aggregation '{"total":{"name":"Cost","function":"Sum"}}' \
            --query "properties.rows" -o json > rows.json
          echo "usd=$(jq -r '[.[0][0]] | add // 0' rows.json)" >> "$GITHUB_OUTPUT"
      - name: Post to issue
        env: { GH_TOKEN: ${{ secrets.GITHUB_TOKEN }} }
        run: |
          gh issue comment ${{ vars.COST_ISSUE_NUMBER }} \
            --repo CoolGitOrg/copilot-foundry \
            --body "Foundry Qwen spend (yesterday): \$${{ steps.q.outputs.usd }}"
      - name: Slack
        if: ${{ vars.SLACK_WEBHOOK != '' }}
        run: |
          curl -sS -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"Foundry Qwen spend yesterday: \$${{ steps.q.outputs.usd }}\"}" \
            "${{ secrets.SLACK_WEBHOOK }}"
```
> Cost Management data refreshes every ~4 hours; GitHub's own guidance is to call cost APIs no more than once/day. Use `vars` for non-secret IDs and `secrets` only for the Slack webhook.

### 10.3 Cost export
For durable analysis, add a scheduled **Cost Management export** to a storage account (`az costmanagement export create --scope ... --storage-account-id ... --recurrence Daily`) and point Log Analytics/Power BI at the blob.

---

## 11. End-user guide (developers in CoolGitOrg)

Prerequisite: the enterprise/org owner has registered the Foundry key and enabled the Qwen model (§6). Then it "just appears."

### (a) VS Code Copilot Chat
1. Open Chat (`Ctrl/Cmd+Alt+I`) → click the model dropdown → the org/enterprise Qwen model appears under the org name. Select it. Done.
2. *Local BYOK alternative (personal key):* model dropdown → **Manage Models…** → **Add Models** → provider **Azure** → paste endpoint `https://<resource>.openai.azure.com/`, deployment name (e.g. `qwen3-32b`), and API key → the model appears in the picker. Works in Ask/Edit/Agent/Plan; not completions.

### (b) Copilot CLI
Set env vars before launching `copilot`:
```bash
# Azure OpenAI-style route (deployment in the base URL, ending in /v1)
export COPILOT_PROVIDER_TYPE=azure
export COPILOT_PROVIDER_BASE_URL="https://<resource>.openai.azure.com/openai/deployments/qwen3-32b/v1"
export COPILOT_PROVIDER_API_KEY="$FOUNDRY_KEY"
export COPILOT_MODEL="qwen3-32b"          # deployment name, NOT the catalog model name
copilot
# switch model at runtime:
/model qwen3-coder-next
```
If a Qwen **thinking** model returns `400 reasoning_content...` on multi-turn tool calls, switch to an Anthropic-compatible gateway and `COPILOT_PROVIDER_TYPE=anthropic`, or use the non-thinking coder model. Run `copilot help providers` for the exact variables your CLI version supports.

### (c) github.com Copilot Chat
Only enterprise/org-registered custom models appear here (local keys do not). Open Copilot Chat on github.com → model picker → select the Qwen model listed under the org/enterprise name.

### Switching back / detecting the cap / decision flow
- **Switch back:** reselect a GitHub-hosted model in the same picker, or unset the `COPILOT_*` env vars (CLI).
- **Detect you've hit the cap:** watch for the "additional usage / budget" notice in the IDE or on the github.com Billing → Usage page; owners can also read `/organizations/CoolGitOrg/settings/billing/ai_credit/usage`.
- **Decision flow — when to use the Qwen fallback:**
  1. Routine completions → stay on GitHub-hosted (completions are free anyway).
  2. Chat/agent work AND credits remaining → GitHub-hosted premium model.
  3. Credits exhausted OR overage blocked/capped → **switch picker to Foundry Qwen** (cost → Azure).
  4. Data-residency-sensitive code that must stay in your Azure tenant → Foundry Qwen (managed compute, private network) regardless of credits.

---

## 12. Automation glue

### terraform.yml — plan/apply with Azure OIDC (no stored secrets)
```yaml
name: foundry-iac
on:
  push: { branches: [main], paths: ["infra/**"] }
  pull_request: { paths: ["infra/**"] }
  workflow_dispatch:
permissions:
  id-token: write        # OIDC
  contents: read
  pull-requests: write   # plan comment
env:
  ARM_USE_OIDC: "true"
  ARM_CLIENT_ID: ${{ vars.AZURE_CLIENT_ID }}
  ARM_TENANT_ID: ${{ vars.AZURE_TENANT_ID }}
  ARM_SUBSCRIPTION_ID: ${{ vars.AZURE_SUBSCRIPTION_ID }}
jobs:
  plan-apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/login@v2
        with:
          client-id: ${{ vars.AZURE_CLIENT_ID }}
          tenant-id: ${{ vars.AZURE_TENANT_ID }}
          subscription-id: ${{ vars.AZURE_SUBSCRIPTION_ID }}
      - uses: hashicorp/setup-terraform@v3
      - run: terraform -chdir=infra init
      - run: terraform -chdir=infra plan -out=tf.plan
      - if: github.ref == 'refs/heads/main' && github.event_name != 'pull_request'
        run: terraform -chdir=infra apply -auto-approve tf.plan
```
Azure side (one-time): create an Entra app + service principal with a **federated credential** for `repo:CoolGitOrg/copilot-foundry:ref:refs/heads/main` (and `:pull_request`), audience `api://AzureADTokenExchange`, issuer `https://token.actions.githubusercontent.com`; grant it Contributor on the RG and Cognitive Services Contributor. Set `ARM_USE_OIDC=true` so the AzureRM provider and the azurerm backend both use the workflow token.

### rotate-key.yml — rotate Foundry key + update org secret + commit docs + trigger downstream
```yaml
name: rotate-foundry-key
on:
  schedule: [{ cron: "0 3 1 * *" }]   # monthly
  workflow_dispatch:
permissions:
  id-token: write
  contents: write        # commit regenerated docs
jobs:
  rotate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/login@v2
        with:
          client-id: ${{ vars.AZURE_CLIENT_ID }}
          tenant-id: ${{ vars.AZURE_TENANT_ID }}
          subscription-id: ${{ vars.AZURE_SUBSCRIPTION_ID }}
      - name: Regenerate key2, promote, store in Key Vault
        id: rot
        run: |
          set -euo pipefail
          RG=rg-coolgit-copilot; ACCT=$(terraform -chdir=infra output -raw foundry_account_name)
          KV=$(terraform -chdir=infra output -raw key_vault_name)
          az cognitiveservices account keys regenerate -g "$RG" -n "$ACCT" --key-name key2 >/dev/null
          NEWKEY=$(az cognitiveservices account keys list -g "$RG" -n "$ACCT" --query key2 -o tsv)
          az keyvault secret set --vault-name "$KV" --name foundry-primary-key --value "$NEWKEY" >/dev/null
          echo "::add-mask::$NEWKEY"; echo "key=$NEWKEY" >> "$GITHUB_OUTPUT"
      - name: Update org-level Actions/Codespaces secret
        env: { GH_TOKEN: ${{ secrets.GH_ADMIN_PAT }} }   # classic PAT, admin:org
        run: |
          gh secret set FOUNDRY_KEY --org CoolGitOrg --visibility all \
            --body "${{ steps.rot.outputs.key }}"
          gh secret set FOUNDRY_KEY --org CoolGitOrg --app codespaces --visibility all \
            --body "${{ steps.rot.outputs.key }}"
      - name: Regenerate + commit user-config docs
        run: |
          make user-config > docs/user-config.md
          git config user.name  "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add docs/user-config.md
          git commit -m "chore: refresh Foundry user config after key rotation" || echo "no changes"
          git push
      - name: Trigger downstream notify workflow
        env: { GH_TOKEN: ${{ secrets.GH_ADMIN_PAT }} }   # PAT, not GITHUB_TOKEN — see gotcha
        run: |
          gh workflow run notify.yml --repo CoolGitOrg/copilot-foundry \
            -f message="Foundry key rotated $(date -u +%FT%TZ)"
```
> **Gotcha (critical):** events triggered by the built-in `GITHUB_TOKEN` do **not** trigger further workflow runs. To fire `notify.yml` (or a `repository_dispatch`), you must use a **PAT or GitHub App token**, as shown. The BYOK key registration itself cannot be updated via API, so after rotation an owner must paste the new key into AI controls (or rely on the org secret path for CI consumers) — automate the reminder, not the UI action.

### notify.yml — downstream webhook
```yaml
name: notify
on:
  workflow_dispatch: { inputs: { message: { required: true } } }
  repository_dispatch: { types: [foundry-rotated] }
permissions: { contents: read }
jobs:
  ping:
    runs-on: ubuntu-latest
    steps:
      - run: |
          curl -sS -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"${{ github.event.inputs.message || github.event.client_payload.message }}\"}" \
            "${{ secrets.SLACK_WEBHOOK }}"
```

---

## 13. Security considerations

- **Auth model — prefer Entra ID over shared keys.** Foundry supports keyless Entra ID (`Authorization: Bearer <token>`) with the **Cognitive Services User** role for callers and **Azure AI Developer** on the resource group for deployers. Use keyless for CI (`az account get-access-token`). Copilot **enterprise BYOK requires an API key** (it stores a key server-side), so you cannot make the BYOK path fully keyless — mitigate by (a) storing the key only in Key Vault + org secret, (b) rotating monthly (§12), (c) scoping the key to a single Foundry account.
- **Key distribution to developers:** do **not** hand out the shared key. For enterprise BYOK, no developer needs the key at all (it's server-side). For local-BYOK power users, distribute via **Codespaces org secret** / dev-container `remoteEnv` pulling from Key Vault, never in `settings.json` committed to a repo.
- **Data residency / privacy:** With **Foundry Models sold by Azure**, prompts/code are processed in **your Azure tenant/region** under Azure's data-handling terms — data stored at rest remains in the designated Azure geography and is not used to train the models. With **Models from partners & community** on managed compute, weights run on **Microsoft-managed GPUs in your region** with no egress to Hugging Face Hub (weights are pre-staged and validated). This is a stronger residency posture than GitHub-hosted Copilot models for regulated code. Contrast: GitHub-hosted Copilot routes to GitHub/model-provider infrastructure under GitHub's terms.
- **Content filtering / RAI:** bind a Responsible AI content-filter policy (`rai_policy_name`) to each serverless deployment; `Microsoft.DefaultV2` is the baseline. Managed-compute deployments can front-end with Azure AI Content Safety.
- **Content exclusion** still applies via Copilot's `content_exclusion` API to keep sensitive paths out of context regardless of model.
- **Least privilege:** the classic PAT for enterprise policy/billing reads should carry only `manage_billing:copilot` (not full `admin:enterprise`) where possible; store it as an org secret with `admin:org` only where `gh secret set --org` demands it.
- **Marketplace/partner terms:** partner Qwen models deployed from Azure Marketplace carry the publisher's terms in addition to Azure's; review before enabling org-wide.

---

## 14. Makefile

```makefile
SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:
TFDIR := infra
KV := $(shell terraform -chdir=$(TFDIR) output -raw key_vault_name 2>/dev/null)
ACCT := $(shell terraform -chdir=$(TFDIR) output -raw foundry_account_name 2>/dev/null)
RG := rg-coolgit-copilot

.PHONY: init plan apply deploy-models validate cost-report rotate-key user-config
init:
	terraform -chdir=$(TFDIR) init
plan:
	terraform -chdir=$(TFDIR) plan -out=tf.plan
apply:
	terraform -chdir=$(TFDIR) apply -auto-approve tf.plan
deploy-models:
	./scripts/deploy-models.sh
validate:
	./scripts/validate.sh
cost-report:
	az costmanagement query --type ActualCost --timeframe MonthToDate \
	  --scope "/subscriptions/$$AZURE_SUBSCRIPTION_ID/resourceGroups/$(RG)" \
	  --dataset-granularity Daily -o table
rotate-key:
	az cognitiveservices account keys regenerate -g $(RG) -n $(ACCT) --key-name key2
	NEWKEY=$$(az cognitiveservices account keys list -g $(RG) -n $(ACCT) --query key2 -o tsv); \
	az keyvault secret set --vault-name $(KV) --name foundry-primary-key --value "$$NEWKEY" >/dev/null; \
	gh secret set FOUNDRY_KEY --org CoolGitOrg --visibility all --body "$$NEWKEY"
user-config:
	@ENDPOINT=$$(terraform -chdir=$(TFDIR) output -raw foundry_endpoint); \
	cat <<-EOF
	# Copilot CLI (bash)
	export COPILOT_PROVIDER_TYPE=azure
	export COPILOT_PROVIDER_BASE_URL="$${ENDPOINT}openai/deployments/qwen3-32b/v1"
	export COPILOT_PROVIDER_API_KEY="\$$FOUNDRY_KEY"
	export COPILOT_MODEL="qwen3-32b"

	# VS Code settings.json (Manage Models → Azure)
	{ "endpoint": "$${ENDPOINT}", "deployment": "qwen3-32b" }
	EOF
```

---

## 15. Known gotchas appendix

1. **`GITHUB_TOKEN` does not trigger downstream workflows.** Use a PAT or GitHub App token for `gh workflow run` / `repository_dispatch`.
2. **BYOK key registration is UI-only** — no REST API; automate everything around it, not the AI-controls form itself.
3. **`reasoning_content` 400 on multi-turn tool calls** with thinking models via the `openai`/`azure` provider path — use an Anthropic-compatible endpoint or non-thinking model.
4. **`COPILOT_MODEL` must be the deployment name, not the catalog model name**; the Azure base URL must be the full deployment path ending in `/v1`, **not** the Foundry project URL.
5. **`qwen3.7` / `qwen3.8-flash` do not exist** in Foundry — map to `qwen3-32b` / `Qwen3-Coder-Next`.
6. **Legacy Copilot metrics endpoints closed April 2, 2026** — use `.../copilot/metrics/reports/...`.
7. **Enterprise Copilot policy/billing endpoints reject fine-grained PATs** — classic PAT with `manage_billing:copilot`/`admin:enterprise` only.
8. **`azurerm_ai_foundry` is legacy (hub-based)** — use `azurerm_cognitive_account` kind `AIServices` for the new Foundry resource.
9. **Managed-compute deployments bill per-GPU-hour even when idle** — scale instances to 0/destroy when not needed; the serverless `qwen3-32b` has no idle cost.
10. **`azurerm_consumption_budget_*` action-group binding** has had provider drift (forces replacement) — pin the provider and confirm the alert fires in the portal.
11. **Terraform model auto-upgrade:** `ignore_changes = [model[0].version]` prevents churn when Foundry rotates the default version.
12. **BYOK is chat/agent only, never completions** — completions always use GitHub-hosted models (and are free), so BYOK cannot offload completion cost.
13. **No auto-fallback to cheaper models when credits run out** (that behavior was removed with the AI Credits transition) — the manual model-switch to Qwen is the fallback.
14. **Cost Management API refresh is ~4h; call ≤ once/day.**
15. **Foundry endpoint URL shape varies** (`/openai/deployments/...` vs `/models/...` vs `/managed-deployments/...`) — validate both before wiring Copilot.

---

## Recommendations (staged, with thresholds)

**Stage 0 — Prove the path (day 1–2).**
Deploy a single **`qwen3-32b` serverless** deployment via `make init plan apply deploy-models`, run `make validate`, and confirm a 200 on both URL shapes plus a tool-call turn. *Threshold to proceed:* validation returns a streamed tool-call response without a 400.

**Stage 1 — Register BYOK at the enterprise (day 2–3).**
In AI controls, register the Foundry key as provider **Microsoft Foundry**, enable `qwen3-32b`, and grant access to `CoolGitOrg`. Confirm it appears in the VS Code, CLI, and github.com pickers. *Threshold:* the model is selectable in all three surfaces for a test user.

**Stage 2 — Wire governance + cost (week 1).**
Stand up OIDC, the `terraform.yml` pipeline, budgets (`azurerm_consumption_budget_resource_group` at your monthly cap), the daily cost workflow, and the KQL dashboard. Set the **Copilot budget** in GitHub to your desired overage posture ($0 to hard-cap credits and force the Qwen fallback; positive to allow controlled overage). *Threshold:* a daily cost figure lands in the issue/Slack and budget alerts fire in a test.

**Stage 3 — Add the coding tier only if justified (week 2+).**
Deploy **`Qwen3-Coder-Next` managed compute** only if agentic coding quality on `qwen3-32b` is insufficient. Because it bills per-GPU-hour, gate it behind a schedule that scales to zero off-hours. *Threshold to keep it:* measured developer value exceeds the idle-GPU burn shown in `make cost-report`.

**Stage 4 — Operationalize.**
Turn on monthly key rotation (`rotate-key.yml`), document the fallback decision flow (§11) for developers, and review the Foundry region matrix quarterly for new Qwen SKUs (watch for a genuine `qwen3-coder` Direct-from-Azure serverless listing, which would let you retire the managed-compute tier and its idle cost).

**What would change the recommendation:** If GitHub ships a REST API for BYOK key registration, fold Stage 1 into Terraform/`gh`. If a Qwen coder model appears as **Direct-from-Azure serverless**, prefer it over managed compute immediately (no idle cost, Microsoft SLA). If your credit pool consistently covers demand, keep BYOK purely as the capped-overage safety net rather than a primary roster item.

---

## Caveats
- **Public preview churn:** Enterprise/org BYOK and Foundry managed compute are both in public preview; UI flows, provider names, and API versions will change. Re-verify the AI-controls steps and the `azapi` template IDs at implementation time.
- **Model names/versions:** `qwen3.7` and `qwen3.8-flash` are not real Foundry SKUs; `qwen3-32b` and `Qwen3-Coder-Next` are the verified substitutes. Model versions rotate — use `az cognitiveservices account list-models` as the authoritative source for your region rather than hardcoding versions.
- **Pricing:** No stable public Direct-from-Azure per-token price for the Qwen coder models was found; the KQL cost estimate uses indicative third-party rates ($0.22 in / $1.80 out per 1M for Qwen3-Coder-480B on Bedrock/OpenRouter). Confirm your actual negotiated Azure rate before relying on the cost math.
- **Credit allotments:** The 1,900 / 3,900 (promo 3,000 / 7,000 through Sept 1, 2026) Business/Enterprise figures come from GitHub Docs; GitHub still directs enterprises to sales for contracted terms, so treat these as the documented defaults, not a guarantee for your contract.
- **`reasoning_content` behavior** was verified against DeepSeek's documented case and VS Code issue #318920; the identical failure on Qwen thinking models is inferred from the same request-shape mechanism, not from a Qwen-specific bug report. Test before relying on a Qwen thinking variant in agent mode.
- **Endpoint URL shape** could not be pinned to a single canonical form because it depends on whether your account resolves as Azure-OpenAI-style or the newer unified Foundry inference route — hence the dual-shape validation script.