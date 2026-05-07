output "synapse_workspace_ids" {
  value = { for k, ws in azurerm_synapse_workspace.this : k => ws.id }
}

output "synapse_endpoints" {
  value = { for k, ws in azurerm_synapse_workspace.this : k => ws.connectivity_endpoints }
}

output "synapse_msi_ids" {
  value = { for k, ws in azurerm_synapse_workspace.this : k => ws.identity[0].principal_id }
}

output "storage_dfs_endpoints" {
  value = { for k, sa in azurerm_storage_account.synapse_datalake : k => sa.primary_dfs_endpoint }
}

output "resource_group_names" {
  value = { for k, rg in azurerm_resource_group.synapse : k => rg.name }
}
