# Microsoft Foundry, and the Entra ID application apps call the platform as.
#
# Per environment:
#
#   <env>-eus2-coolai-fdry-001   a Foundry (AI Services) account with the
#                                model deployments in config/platform.json.
#                                Key access is off: callers sign in.
#   cool-<env>                   an Entra ID application. Its identifier is
#                                the audience of tokens for this
#                                environment's API Management, and its app
#                                roles are the AI token budgets.
#
# Only API Management calls Foundry. Apps call the cool-ai API there with
# their own identity; the gateway reads the budget from the token's roles.
# 70-addons assigns a role to an app when a cool-addon request is merged.

terraform {
  required_version = ">= 1.9"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.40"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "~> 3.0"
    }
  }
  backend "azurerm" {
    resource_group_name  = "prd-eus2-coolops-rg-001"
    storage_account_name = "prdeus2coolopsst001"
    container_name       = "tfstate"
    key                  = "65-ai.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  region   = local.platform.region
  envs     = toset(local.platform.ai.environments)

  deployments = merge([
    for env in local.envs : {
      for d in local.platform.ai.deployments : "${env}-${d.name}" => merge(d, { environment = env })
    }
  ]...)

  budgets = {
    "AI.Small"  = "Small AI token budget"
    "AI.Medium" = "Medium AI token budget"
    "AI.Large"  = "Large AI token budget"
  }
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

provider "azuread" {
  tenant_id = local.platform.tenant_id
}

resource "azurerm_resource_group" "ai" {
  for_each = local.envs
  name     = "${each.key}-${local.region}-coolai-rg-001"
  location = local.platform.location
  tags     = merge(local.platform.tags, { "cool/env" = each.key })
}

resource "azurerm_cognitive_account" "foundry" {
  for_each              = local.envs
  name                  = "${each.key}-${local.region}-coolai-fdry-001"
  resource_group_name   = azurerm_resource_group.ai[each.key].name
  location              = azurerm_resource_group.ai[each.key].location
  kind                  = "AIServices"
  sku_name              = "S0"
  custom_subdomain_name = "${each.key}-${local.region}-coolai-fdry-001"
  local_auth_enabled    = false
  tags                  = merge(local.platform.tags, { "cool/env" = each.key })

  identity {
    type = "SystemAssigned"
  }
}

resource "azurerm_cognitive_deployment" "model" {
  for_each             = local.deployments
  name                 = each.value.name
  cognitive_account_id = azurerm_cognitive_account.foundry[each.value.environment].id

  model {
    format  = "OpenAI"
    name    = each.value.model
    version = each.value.version
  }

  sku {
    name     = each.value.sku
    capacity = each.value.capacity
  }
}

# API Management signs in to Foundry with its own identity. The instances
# are created by cool-apim; their principal IDs come from config.
resource "azurerm_role_assignment" "apim_calls_foundry" {
  for_each             = { for env, id in local.platform.apim_principals : env => id if id != "" && contains(local.envs, env) }
  scope                = azurerm_cognitive_account.foundry[each.key].id
  role_definition_name = "Cognitive Services OpenAI User"
  principal_id         = each.value
  principal_type       = "ServicePrincipal"
}

# The application whose tokens an environment's API Management accepts.
resource "azuread_application" "cool" {
  for_each         = local.envs
  display_name     = "cool-${each.key}"
  sign_in_audience = "AzureADMyOrg"
  identifier_uris  = ["api://${local.platform.tenant_id}/cool-${each.key}"]

  dynamic "app_role" {
    for_each = local.budgets
    content {
      # A role's ID must not change between applies; derive it from its name.
      id                   = uuidv5("url", "https://coolapps.example/roles/${each.key}/${app_role.key}")
      value                = app_role.key
      display_name         = app_role.value
      description          = "${app_role.value} on the CoolDeploy AI gateway in ${each.key}."
      allowed_member_types = ["Application"]
      enabled              = true
    }
  }
}

resource "azuread_service_principal" "cool" {
  for_each  = local.envs
  client_id = azuread_application.cool[each.key].client_id
}

output "audiences" {
  description = "Token audience per environment: cool-api-audience in cool-apim/instances/<env>.yaml."
  value       = { for env, app in azuread_application.cool : env => one(app.identifier_uris) }
}

output "foundry_endpoints" {
  description = "Backend URL per environment: backends.cool-foundry in cool-apim/instances/<env>.yaml."
  value       = { for env, a in azurerm_cognitive_account.foundry : env => "${a.endpoint}openai" }
}
