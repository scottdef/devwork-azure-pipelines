###############################################################################
# synapse.tf — Synapse workspace data source + Spark pool resources
#
# The workspace itself is assumed to already exist (3 workspaces pre-created).
# We reference it as a data source and manage only the mutable child resources
# that Terraform handles cleanly:  Spark pools, linked services, managed
# private endpoints.
#
# Excluded per requirements:
#   - azurerm_synapse_firewall_rule
#   - azurerm_synapse_role_assignment
#   - azurerm_synapse_sql_pool
###############################################################################

# ── Data: existing workspace ────────────────────────────────────────────────
data "azurerm_synapse_workspace" "this" {
  name                = var.synapse_workspace_name
  resource_group_name = var.resource_group_name
}

# ── Data: existing storage account (primary ADLS Gen2) ──────────────────────
data "azurerm_storage_account" "adls" {
  name                = var.storage_account_name
  resource_group_name = var.resource_group_name
}

# ── Spark pools ─────────────────────────────────────────────────────────────
resource "azurerm_synapse_spark_pool" "pools" {
  for_each = var.spark_pools

  name                 = each.key
  synapse_workspace_id = data.azurerm_synapse_workspace.this.id
  node_size_family     = "MemoryOptimized"
  node_size            = each.value.node_size
  node_count           = each.value.auto_pause_delay > 0 ? null : each.value.node_count
  spark_version        = each.value.spark_version

  auto_scale {
    min_node_count = each.value.min_node_count
    max_node_count = each.value.max_node_count
  }

  auto_pause {
    delay_in_minutes = each.value.auto_pause_delay
  }

  tags = merge(var.tags, {
    environment = var.environment
    managed_by  = "terraform"
  })
}

# ── Managed private endpoint (Key Vault) ────────────────────────────────────
resource "azurerm_synapse_managed_private_endpoint" "keyvault" {
  count = var.key_vault_url != "" ? 1 : 0

  name                 = "pe-keyvault-${var.environment}"
  synapse_workspace_id = data.azurerm_synapse_workspace.this.id
  target_resource_id   = data.azurerm_key_vault.this[0].id
  subresource_name     = "vault"
}

data "azurerm_key_vault" "this" {
  count = var.key_vault_url != "" ? 1 : 0

  # Extract KV name from URL: https://<name>.vault.azure.net/
  name                = regex("https://([^.]+)\\.vault\\.azure\\.net", var.key_vault_url)[0]
  resource_group_name = var.resource_group_name
}
