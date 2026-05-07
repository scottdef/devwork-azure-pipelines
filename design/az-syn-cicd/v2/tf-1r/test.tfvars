# environments/test.tfvars
environment            = "test"
synapse_workspace_name = "synapse-workspace-test"
resource_group_name    = "rg-synapse-test"
location               = "eastus2"
storage_account_name   = "testdatalake"
storage_filesystem_name = "synapse"
key_vault_url          = "https://kv-synapse-test.vault.azure.net/"
storage_account_url    = "https://testdatalake.dfs.core.windows.net"

spark_pools = {
  sparkpool01 = {
    node_size        = "Medium"
    node_count       = 3
    min_node_count   = 3
    max_node_count   = 15
    spark_version    = "3.4"
    auto_pause_delay = 15
  }
}

tags = {
  project     = "synapse-analytics"
  environment = "test"
  cost_center = "data-engineering"
}
