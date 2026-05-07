# environments/dev.tfvars
environment            = "dev"
synapse_workspace_name = "synapse-workspace-dev"
resource_group_name    = "rg-synapse-dev"
location               = "eastus2"
storage_account_name   = "devdatalake"
storage_filesystem_name = "synapse"
key_vault_url          = "https://kv-synapse-dev.vault.azure.net/"
storage_account_url    = "https://devdatalake.dfs.core.windows.net"

spark_pools = {
  sparkpool01 = {
    node_size        = "Small"
    node_count       = 3
    min_node_count   = 3
    max_node_count   = 10
    spark_version    = "3.4"
    auto_pause_delay = 15
  }
}

tags = {
  project     = "synapse-analytics"
  environment = "dev"
  cost_center = "data-engineering"
}
