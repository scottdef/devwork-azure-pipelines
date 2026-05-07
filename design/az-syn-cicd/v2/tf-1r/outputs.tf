###############################################################################
# outputs.tf — Exported values for downstream workflows
###############################################################################

output "workspace_id" {
  description = "Synapse workspace resource ID"
  value       = data.azurerm_synapse_workspace.this.id
}

output "workspace_name" {
  description = "Synapse workspace name"
  value       = data.azurerm_synapse_workspace.this.name
}

output "connectivity_endpoints" {
  description = "Synapse connectivity endpoints"
  value       = data.azurerm_synapse_workspace.this.connectivity_endpoints
}

output "spark_pool_ids" {
  description = "Map of Spark pool name → resource ID"
  value = {
    for k, v in azurerm_synapse_spark_pool.pools : k => v.id
  }
}

output "environment" {
  description = "Current deployment environment"
  value       = var.environment
}
