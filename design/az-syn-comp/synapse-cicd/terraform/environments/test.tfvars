github_org      = "YOUR_ORG"
tenant_id       = "YOUR_TENANT_ID"
subscription_id = "YOUR_SUBSCRIPTION_ID"

environments = {
  test = {
    resource_group_name           = "rg-synapse-test"
    location                      = "eastus2"
    workspace_name                = "synapse-workspace-test"
    storage_account_name          = "synapsetestdatalake"
    filesystem_name               = "synapsefs"
    sql_admin_login               = "sqladmin"
    sql_admin_password            = "REPLACE_VIA_SECRET"
    spark_pool_name               = "sparkpooltest"
    spark_pool_size               = "Medium"
    spark_min_nodes               = 3
    spark_max_nodes               = 10
    spark_version                 = "3.4"
    sql_pool_name                 = ""
    sql_pool_sku                  = "DW100c"
    firewall_rules                = []
    managed_vnet_enabled          = true
    public_network_access_enabled = false
    git_repo_name                 = ""
    github_account_name           = ""
    collaboration_branch          = "main"
    root_folder                   = "/"
  }
}

spn_configs = {
  test = { client_id = "SYNAPSE_TEST_SPN_ID", client_secret = "SYNAPSE_TEST_SPN_SECRET" }
}
