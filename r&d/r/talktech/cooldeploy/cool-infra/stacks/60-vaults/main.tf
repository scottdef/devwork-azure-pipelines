# Key Vaults, and who may read them.
#
#   <env>-eus2-coolops-kv-001     the platform's vault for an environment:
#                                 the wildcard certificate of the gateways,
#                                 secrets API Management refers to, and in
#                                 prd the GitHub App keys
#   <env>-eus2-<team>-kv-001      a team's vault for an environment: its apps'
#                                 secrets, named <app>-<NAME>
#
# All vaults use Azure RBAC. No access policies, no keys in Terraform state:
# this stack creates vaults and role assignments, never secret values.

terraform {
  required_version = ">= 1.9"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.40"
    }
  }
  backend "azurerm" {
    resource_group_name  = "prd-eus2-coolops-rg-001"
    storage_account_name = "prdeus2coolopsst001"
    container_name       = "tfstate"
    key                  = "60-vaults.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))
  teams    = jsondecode(file("${path.module}/../../config/teams.json"))
  region   = local.platform.region

  # environment -> the cluster its apps run on
  app_cluster = { for name, c in local.clusters : c.environment => name if c.role == "apps" }
  # environment -> the resource group of that cluster
  app_rg = { for name, c in local.clusters : c.environment => "${c.environment}-${local.region}-${c.workload}-rg-001" if c.role == "apps" }
  # The prd platform vault sits with the hub; the others with their cluster.
  ops_rg = merge(local.app_rg, { prd = "prd-${local.region}-coolops-rg-001" })

  team_vaults = merge([
    for team, t in local.teams : {
      for env in t.environments : "${env}-${team}" => { team = team, environment = env, group = t.group }
    }
  ]...)
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

data "azurerm_kubernetes_cluster" "apps" {
  for_each            = local.app_cluster
  name                = each.value
  resource_group_name = local.app_rg[each.key]
}

data "azurerm_user_assigned_identity" "coolapi" {
  name                = "prd-${local.region}-coolapi-id-001"
  resource_group_name = "prd-${local.region}-coolops-rg-001"
}

data "azurerm_user_assigned_identity" "coolapply" {
  name                = "prd-${local.region}-coolapply-id-001"
  resource_group_name = "prd-${local.region}-coolops-rg-001"
}

# --- Platform vaults ------------------------------------------------------------

resource "azurerm_key_vault" "ops" {
  for_each                   = local.app_cluster
  name                       = "${each.key}-${local.region}-coolops-kv-001"
  resource_group_name        = local.ops_rg[each.key]
  location                   = local.platform.location
  tenant_id                  = local.platform.tenant_id
  sku_name                   = "standard"
  enable_rbac_authorization  = true
  purge_protection_enabled   = true
  soft_delete_retention_days = 90
  tags                       = merge(local.platform.tags, { "cool/env" = each.key })
}

resource "azurerm_role_assignment" "admins_manage_ops" {
  for_each             = azurerm_key_vault.ops
  scope                = each.value.id
  role_definition_name = "Key Vault Administrator"
  principal_id         = local.platform.platform_admins_group
  principal_type       = "Group"
}

# The pod that keeps the gateways' certificate in sync reads it from here.
resource "azurerm_user_assigned_identity" "tls" {
  for_each            = local.app_cluster
  name                = "${each.key}-${local.region}-cooltls-id-001"
  resource_group_name = local.app_rg[each.key]
  location            = local.platform.location
  tags                = merge(local.platform.tags, { "cool/env" = each.key })
}

resource "azurerm_federated_identity_credential" "tls" {
  for_each            = local.app_cluster
  name                = each.value
  resource_group_name = local.app_rg[each.key]
  parent_id           = azurerm_user_assigned_identity.tls[each.key].id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = data.azurerm_kubernetes_cluster.apps[each.key].oidc_issuer_url
  subject             = "system:serviceaccount:aks-istio-ingress:cool-tls-sync"
}

resource "azurerm_role_assignment" "tls_reads_ops" {
  for_each             = local.app_cluster
  scope                = azurerm_key_vault.ops[each.key].id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.tls[each.key].principal_id
  principal_type       = "ServicePrincipal"
}

# cool-api reads the CoolDeploy GitHub App's private key.
resource "azurerm_role_assignment" "coolapi_reads_ops" {
  scope                = azurerm_key_vault.ops["prd"].id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = data.azurerm_user_assigned_identity.coolapi.principal_id
  principal_type       = "ServicePrincipal"
}

# API Management reads the secrets its named values refer to. The instances
# are created by cool-apim; once one exists, put its principal ID in
# config/platform.json:
#   az apim show -n <env>-eus2-coolapi-apim-001 -g <env>-eus2-coolapi-rg-001 --query identity.principalId -o tsv
resource "azurerm_role_assignment" "apim_reads_ops" {
  for_each             = { for env, id in local.platform.apim_principals : env => id if id != "" }
  scope                = azurerm_key_vault.ops[each.key].id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = each.value
  principal_type       = "ServicePrincipal"
}

# --- Team vaults ----------------------------------------------------------------

# A vault name may be 24 characters. <env>-<region>-<team>-kv-001 with a team
# slug of 8 is exactly that, which is where the 8 comes from.
resource "azurerm_key_vault" "team" {
  for_each                   = local.team_vaults
  name                       = "${each.value.environment}-${local.region}-${each.value.team}-kv-001"
  resource_group_name        = local.app_rg[each.value.environment]
  location                   = local.platform.location
  tenant_id                  = local.platform.tenant_id
  sku_name                   = "standard"
  enable_rbac_authorization  = true
  purge_protection_enabled   = true
  soft_delete_retention_days = 90
  tags                       = merge(local.platform.tags, { "cool/env" = each.value.environment, "cool/team" = each.value.team })
}

# The team sets its own secrets. Apps read them through their own identity,
# which the cool-app-create request grants.
resource "azurerm_role_assignment" "team_manages_secrets" {
  for_each             = local.team_vaults
  scope                = azurerm_key_vault.team[each.key].id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = each.value.group
  principal_type       = "Group"
}

# The 70-addons stack writes connection strings into team vaults.
resource "azurerm_role_assignment" "terraform_writes_addon_secrets" {
  for_each             = local.team_vaults
  scope                = azurerm_key_vault.team[each.key].id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_user_assigned_identity.coolapply.principal_id
  principal_type       = "ServicePrincipal"
}

output "tls_client_ids" {
  description = "Client ID of each environment's certificate-sync identity: platform.tls.identity in cool-gitops."
  value       = { for env, id in azurerm_user_assigned_identity.tls : env => id.client_id }
}

output "ops_vaults" {
  value = { for env, v in azurerm_key_vault.ops : env => v.name }
}
