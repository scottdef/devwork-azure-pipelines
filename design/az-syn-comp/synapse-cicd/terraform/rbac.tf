# ============================================================================
# RBAC — Azure control plane + Synapse data plane
# ============================================================================

data "azuread_service_principal" "spn" {
  for_each  = var.spn_configs
  client_id = each.value.client_id
}

resource "azurerm_role_assignment" "spn_contributor" {
  for_each             = var.environments
  scope                = azurerm_resource_group.synapse[each.key].id
  role_definition_name = "Contributor"
  principal_id         = data.azuread_service_principal.spn[each.key].object_id
}

resource "azurerm_role_assignment" "spn_storage_blob" {
  for_each             = var.environments
  scope                = azurerm_storage_account.synapse_datalake[each.key].id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = data.azuread_service_principal.spn[each.key].object_id
}

resource "azurerm_synapse_role_assignment" "spn_synapse_admin" {
  for_each             = var.environments
  synapse_workspace_id = azurerm_synapse_workspace.this[each.key].id
  role_name            = "Synapse Administrator"
  principal_id         = data.azuread_service_principal.spn[each.key].object_id
  depends_on           = [azurerm_synapse_firewall_rule.allow_azure_services]
}

resource "azurerm_role_assignment" "workspace_msi_storage" {
  for_each             = var.environments
  scope                = azurerm_storage_account.synapse_datalake[each.key].id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_synapse_workspace.this[each.key].identity[0].principal_id
}
