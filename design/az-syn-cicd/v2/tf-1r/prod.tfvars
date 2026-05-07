# environments/prod.tfvars
environment            = "prod"
synapse_workspace_name = "synapse-workspace-prod"
resource_group_name    = "rg-synapse-prod"
location               = "eastus2"
storage_account_name   = "proddatalake"
storage_filesystem_name = "synapse"
key_vault_url          = "https://kv-synapse-prod.vault.azure.net/"
storage_account_url    = "https://proddatalake.dfs.core.windows.net"

spark_pools = {
  sparkpool01 = {
    node_size        = "Large"
    node_count       = 5
    min_node_count   = 5
    max_node_count   = 20
    spark_version    = "3.4"
    auto_pause_delay = 0  # no auto-pause in prod
  }
}

tags = {
  project     = "synapse-analytics"
  environment = "prod"
  cost_center = "data-engineering"
}
