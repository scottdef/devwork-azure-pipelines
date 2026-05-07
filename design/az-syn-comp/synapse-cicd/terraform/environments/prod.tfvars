github_org      = "YOUR_ORG"
tenant_id       = "YOUR_TENANT_ID"
subscription_id = "YOUR_SUBSCRIPTION_ID"

environments = {
  prod = {
    resource_group_name           = "rg-synapse-prod"
    location                      = "eastus2"
    workspace_name                = "synapse-workspace-prod"
    storage_account_name          = "synapseproddatalake"
    filesystem_name               = "synapsefs"
    sql_admin_login               = "sqladmin"
    sql_admin_password            = "REPLACE_VIA_SECRET"
    spark_pool_name               = "sparkpoolprod"
    spark_pool_size               = "Medium"
    spark_min_nodes               = 3
    spark_max_nodes               = 20
    spark_version                 = "3.4"
    sql_pool_name                 = "sqldwprod"
    sql_pool_sku                  = "DW200c"
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
  prod = { client_id = "SYNAPSE_PROD_SPN_ID", client_secret = "SYNAPSE_PROD_SPN_SECRET" }
}
