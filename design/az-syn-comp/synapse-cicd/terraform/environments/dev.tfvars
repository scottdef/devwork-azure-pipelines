github_org      = "YOUR_ORG"
tenant_id       = "YOUR_TENANT_ID"
subscription_id = "YOUR_SUBSCRIPTION_ID"

environments = {
  dev = {
    resource_group_name           = "rg-synapse-dev"
    location                      = "eastus2"
    workspace_name                = "synapse-workspace-dev"
    storage_account_name          = "synapsedevdatalake"
    filesystem_name               = "synapsefs"
    sql_admin_login               = "sqladmin"
    sql_admin_password            = "REPLACE_VIA_SECRET"
    spark_pool_name               = "sparkpooldev"
    spark_pool_size               = "Small"
    spark_min_nodes               = 3
    spark_max_nodes               = 10
    spark_version                 = "3.4"
    sql_pool_name                 = ""
    sql_pool_sku                  = "DW100c"
    firewall_rules                = [{ name = "AllowGitHubRunners", start_ip = "0.0.0.0", end_ip = "255.255.255.255" }]
    managed_vnet_enabled          = true
    public_network_access_enabled = true
    git_repo_name                 = "synapse-repo-dev"
    github_account_name           = "YOUR_ORG"
    collaboration_branch          = "main"
    root_folder                   = "/"
  }
}

spn_configs = {
  dev = { client_id = "SYNAPSE_DEV_SPN_ID", client_secret = "SYNAPSE_DEV_SPN_SECRET" }
}
