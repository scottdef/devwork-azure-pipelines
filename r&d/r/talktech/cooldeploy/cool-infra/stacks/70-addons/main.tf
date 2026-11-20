# Add-ons: the backing services apps ask for with a cool-addon request.
#
# The list is addons.json. An approved request opens a pull request that adds
# a line to it; merging runs this stack. Nobody edits the list by hand.
#
#   redis      an Azure Managed Redis for the app, reached over a private
#              endpoint, with Entra ID sign-in and no access keys
#   postgres   a database for the app on the team's PostgreSQL server for
#              the environment, reached over a private endpoint
#   storage    a blob container for the app in the team's storage account
#              for the environment; the app's identity is granted access
#   ai         a token budget on the AI gateway: an app role on the app's
#              identity, which API Management reads from the caller's token
#
# Whatever an app needs to connect is written to the team's Key Vault as
# <app>-<NAME>, where the app's "secrets" in cool.json pick it up.
#
# The app's managed identity, <env>-<region>-<team>-<app>-id-001, is created
# by the cool-app-create request and only read here.

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
    azapi = {
      source  = "azure/azapi"
      version = "~> 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
  backend "azurerm" {
    resource_group_name  = "prd-eus2-coolops-rg-001"
    storage_account_name = "prdeus2coolopsst001"
    container_name       = "tfstate"
    key                  = "70-addons.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))
  addons   = jsondecode(file("${path.module}/addons.json"))
  region   = local.platform.region

  app_cluster = { for name, c in local.clusters : c.environment => c if c.role == "apps" }
  app_rg      = { for env, c in local.app_cluster : env => "${env}-${local.region}-${c.workload}-rg-001" }
  app_vnet    = { for env, c in local.app_cluster : env => "${env}-${local.region}-${c.workload}-vnet-001" }

  # One entry per add-on, keyed <env>-<team>-<app>.
  by_kind = {
    for kind in ["redis", "postgres", "storage", "ai"] : kind => {
      for a in local.addons : "${a.environment}-${a.team}-${a.app}" => a if a.addon == kind
    }
  }
  # Every app that has an add-on, for looking up its identity and vault.
  apps = { for a in local.addons : "${a.environment}-${a.team}-${a.app}" => a... }
  # Team-and-environment pairs that share a server or an account.
  postgres_teams = { for k, a in local.by_kind.postgres : "${a.environment}-${a.team}" => a... }
  storage_teams  = { for k, a in local.by_kind.storage : "${a.environment}-${a.team}" => a... }
  # Environments that need a private DNS zone for a kind of add-on.
  redis_envs    = toset([for a in local.by_kind.redis : a.environment])
  postgres_envs = toset([for a in local.by_kind.postgres : a.environment])

  redis_sku = { small = "Balanced_B0", medium = "Balanced_B3", large = "Balanced_B10" }
  postgres_sku = {
    small  = { sku = "B_Standard_B1ms", storage_mb = 32768 }
    medium = { sku = "GP_Standard_D2ds_v5", storage_mb = 65536 }
    large  = { sku = "GP_Standard_D4ds_v5", storage_mb = 131072 }
  }
  ai_role = { small = "AI.Small", medium = "AI.Medium", large = "AI.Large" }
}

provider "azurerm" {
  features {}
  subscription_id     = local.platform.subscription_id
  storage_use_azuread = true
}

provider "azuread" {
  tenant_id = local.platform.tenant_id
}

provider "azapi" {
  subscription_id = local.platform.subscription_id
}

data "azurerm_user_assigned_identity" "app" {
  for_each            = local.apps
  name                = "${each.value[0].environment}-${local.region}-${each.value[0].team}-${each.value[0].app}-id-001"
  resource_group_name = local.app_rg[each.value[0].environment]
}

data "azurerm_key_vault" "team" {
  for_each            = local.apps
  name                = "${each.value[0].environment}-${local.region}-${each.value[0].team}-kv-001"
  resource_group_name = local.app_rg[each.value[0].environment]
}

data "azurerm_resource_group" "apps" {
  for_each = local.app_rg
  name     = each.value
}

data "azurerm_subnet" "nodes" {
  for_each             = local.app_vnet
  name                 = "nodes"
  virtual_network_name = each.value
  resource_group_name  = local.app_rg[each.key]
}

data "azurerm_virtual_network" "apps" {
  for_each            = local.app_vnet
  name                = each.value
  resource_group_name = local.app_rg[each.key]
}

# --- redis ----------------------------------------------------------------------

# Azure Managed Redis, through the ARM API: the resource is new enough that
# its shape there is the stable one. Access keys are off; the app signs in
# with its managed identity.
resource "azapi_resource" "redis" {
  for_each                  = local.by_kind.redis
  type                      = "Microsoft.Cache/redisEnterprise@2025-04-01"
  name                      = "${each.value.environment}-${local.region}-${each.value.team}-${each.value.app}-redis-001"
  parent_id                 = data.azurerm_resource_group.apps[each.value.environment].id
  location                  = local.platform.location
  schema_validation_enabled = false
  tags                      = merge(local.platform.tags, { "cool/env" = each.value.environment, "cool/team" = each.value.team, "cool/app" = each.value.app })

  body = {
    sku = { name = local.redis_sku[each.value.size] }
    properties = {
      minimumTlsVersion = "1.2"
      highAvailability  = each.value.environment == "prd" ? "Enabled" : "Disabled"
    }
  }
  response_export_values = ["properties.hostName"]
}

resource "azapi_resource" "redis_database" {
  for_each                  = local.by_kind.redis
  type                      = "Microsoft.Cache/redisEnterprise/databases@2025-04-01"
  name                      = "default"
  parent_id                 = azapi_resource.redis[each.key].id
  schema_validation_enabled = false

  body = {
    properties = {
      clientProtocol           = "Encrypted"
      port                     = 10000
      clusteringPolicy         = "OSSCluster"
      evictionPolicy           = "VolatileLRU"
      accessKeysAuthentication = "Disabled"
    }
  }
}

resource "azapi_resource" "redis_access" {
  for_each                  = local.by_kind.redis
  type                      = "Microsoft.Cache/redisEnterprise/databases/accessPolicyAssignments@2025-04-01"
  name                      = "app"
  parent_id                 = azapi_resource.redis_database[each.key].id
  schema_validation_enabled = false

  body = {
    properties = {
      accessPolicyName = "default"
      user             = { objectId = data.azurerm_user_assigned_identity.app[each.key].principal_id }
    }
  }
}

resource "azurerm_private_dns_zone" "redis" {
  for_each            = local.redis_envs
  name                = "privatelink.redis.azure.net"
  resource_group_name = local.app_rg[each.key]
}

resource "azurerm_private_dns_zone_virtual_network_link" "redis" {
  for_each              = local.redis_envs
  name                  = "apps"
  resource_group_name   = local.app_rg[each.key]
  private_dns_zone_name = azurerm_private_dns_zone.redis[each.key].name
  virtual_network_id    = data.azurerm_virtual_network.apps[each.key].id
}

resource "azurerm_private_endpoint" "redis" {
  for_each            = local.by_kind.redis
  name                = "${azapi_resource.redis[each.key].name}-pe"
  resource_group_name = local.app_rg[each.value.environment]
  location            = local.platform.location
  subnet_id           = data.azurerm_subnet.nodes[each.value.environment].id

  private_service_connection {
    name                           = "redis"
    private_connection_resource_id = azapi_resource.redis[each.key].id
    subresource_names              = ["redisEnterprise"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "redis"
    private_dns_zone_ids = [azurerm_private_dns_zone.redis[each.value.environment].id]
  }
}

resource "azurerm_key_vault_secret" "redis_url" {
  for_each     = local.by_kind.redis
  name         = "${each.value.app}-REDIS-URL"
  key_vault_id = data.azurerm_key_vault.team[each.key].id
  # No password: the app signs in with its identity's token.
  value = "rediss://${azapi_resource.redis[each.key].output.properties.hostName}:10000"
}

# --- postgres -------------------------------------------------------------------

resource "random_password" "postgres" {
  for_each = local.postgres_teams
  length   = 32
  special  = false
}

# One server per team and environment, sized for the largest request.
resource "azurerm_postgresql_flexible_server" "team" {
  for_each                      = local.postgres_teams
  name                          = "${each.key}-${local.region}-pg-001"
  resource_group_name           = local.app_rg[each.value[0].environment]
  location                      = local.platform.location
  version                       = "16"
  administrator_login           = "cool"
  administrator_password        = random_password.postgres[each.key].result
  public_network_access_enabled = false
  backup_retention_days         = each.value[0].environment == "prd" ? 35 : 7
  sku_name = local.postgres_sku[
    contains([for a in each.value : a.size], "large") ? "large" :
    contains([for a in each.value : a.size], "medium") ? "medium" : "small"
  ].sku
  storage_mb = local.postgres_sku[
    contains([for a in each.value : a.size], "large") ? "large" :
    contains([for a in each.value : a.size], "medium") ? "medium" : "small"
  ].storage_mb
  tags = merge(local.platform.tags, { "cool/env" = each.value[0].environment, "cool/team" = each.value[0].team })

  lifecycle {
    ignore_changes = [zone]
  }
}

resource "azurerm_postgresql_flexible_server_database" "app" {
  for_each  = local.by_kind.postgres
  name      = replace(each.value.app, "-", "_")
  server_id = azurerm_postgresql_flexible_server.team["${each.value.environment}-${each.value.team}"].id
  charset   = "UTF8"
  collation = "en_US.utf8"
}

resource "azurerm_private_dns_zone" "postgres" {
  for_each            = local.postgres_envs
  name                = "privatelink.postgres.database.azure.com"
  resource_group_name = local.app_rg[each.key]
}

resource "azurerm_private_dns_zone_virtual_network_link" "postgres" {
  for_each              = local.postgres_envs
  name                  = "apps"
  resource_group_name   = local.app_rg[each.key]
  private_dns_zone_name = azurerm_private_dns_zone.postgres[each.key].name
  virtual_network_id    = data.azurerm_virtual_network.apps[each.key].id
}

resource "azurerm_private_endpoint" "postgres" {
  for_each            = local.postgres_teams
  name                = "${azurerm_postgresql_flexible_server.team[each.key].name}-pe"
  resource_group_name = local.app_rg[each.value[0].environment]
  location            = local.platform.location
  subnet_id           = data.azurerm_subnet.nodes[each.value[0].environment].id

  private_service_connection {
    name                           = "postgres"
    private_connection_resource_id = azurerm_postgresql_flexible_server.team[each.key].id
    subresource_names              = ["postgresqlServer"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "postgres"
    private_dns_zone_ids = [azurerm_private_dns_zone.postgres[each.value[0].environment].id]
  }
}

resource "azurerm_key_vault_secret" "database_url" {
  for_each     = local.by_kind.postgres
  name         = "${each.value.app}-DATABASE-URL"
  key_vault_id = data.azurerm_key_vault.team[each.key].id
  value = format("postgres://cool:%s@%s:5432/%s?sslmode=require",
    random_password.postgres["${each.value.environment}-${each.value.team}"].result,
    azurerm_postgresql_flexible_server.team["${each.value.environment}-${each.value.team}"].fqdn,
    azurerm_postgresql_flexible_server_database.app[each.key].name,
  )
}

# --- storage --------------------------------------------------------------------

resource "azurerm_storage_account" "team" {
  for_each                        = local.storage_teams
  name                            = "${each.value[0].environment}${local.region}${each.value[0].team}st001"
  resource_group_name             = local.app_rg[each.value[0].environment]
  location                        = local.platform.location
  account_tier                    = "Standard"
  account_replication_type        = each.value[0].environment == "prd" ? "ZRS" : "LRS"
  min_tls_version                 = "TLS1_2"
  shared_access_key_enabled       = false
  allow_nested_items_to_be_public = false
  tags                            = merge(local.platform.tags, { "cool/env" = each.value[0].environment, "cool/team" = each.value[0].team })
}

resource "azurerm_storage_container" "app" {
  for_each              = local.by_kind.storage
  name                  = each.value.app
  storage_account_id    = azurerm_storage_account.team["${each.value.environment}-${each.value.team}"].id
  container_access_type = "private"
}

resource "azurerm_role_assignment" "app_uses_container" {
  for_each             = local.by_kind.storage
  scope                = azurerm_storage_container.app[each.key].id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = data.azurerm_user_assigned_identity.app[each.key].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_key_vault_secret" "storage_url" {
  for_each     = local.by_kind.storage
  name         = "${each.value.app}-STORAGE-URL"
  key_vault_id = data.azurerm_key_vault.team[each.key].id
  value        = "${azurerm_storage_account.team["${each.value.environment}-${each.value.team}"].primary_blob_endpoint}${each.value.app}"
}

# --- ai -------------------------------------------------------------------------

data "azuread_application" "cool" {
  for_each     = toset([for a in local.by_kind.ai : a.environment])
  display_name = "cool-${each.key}"
}

data "azuread_service_principal" "cool" {
  for_each  = data.azuread_application.cool
  client_id = each.value.client_id
}

# The budget is an app role on the app's identity. It shows up in the roles
# claim of the tokens the app sends to API Management.
resource "azuread_app_role_assignment" "budget" {
  for_each            = local.by_kind.ai
  app_role_id         = data.azuread_application.cool[each.value.environment].app_role_ids[local.ai_role[each.value.size]]
  principal_object_id = data.azurerm_user_assigned_identity.app[each.key].principal_id
  resource_object_id  = data.azuread_service_principal.cool[each.value.environment].object_id
}
