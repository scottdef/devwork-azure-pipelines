# One AKS cluster, the way CoolDeploy runs them:
#
#   - Entra ID sign-in with Azure RBAC, and no local accounts
#   - workload identity, so pods get Azure tokens without secrets
#   - Azure CNI overlay with Cilium, which enforces NetworkPolicy
#   - the Key Vault secrets provider and managed Prometheus add-ons
#   - the Istio add-on with both ingress gateways, on app clusters
#
# Kubernetes 1.30; kubectl 1.30 is what the platform's tooling is pinned to.

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.40"
    }
  }
}

resource "azurerm_kubernetes_cluster" "this" {
  name                = var.name
  resource_group_name = var.resource_group_name
  location            = var.location
  dns_prefix          = var.name
  kubernetes_version  = var.kubernetes_version
  sku_tier            = var.environment == "prd" ? "Standard" : "Free"
  tags                = var.tags

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = true
  image_cleaner_enabled     = true
  automatic_upgrade_channel = "patch"
  node_os_upgrade_channel   = "NodeImage"

  identity {
    type = "SystemAssigned"
  }

  azure_active_directory_role_based_access_control {
    tenant_id          = var.tenant_id
    azure_rbac_enabled = true
  }

  default_node_pool {
    name                 = "system"
    vm_size              = var.node_size
    vnet_subnet_id       = var.subnet_id
    zones                = ["1", "2", "3"]
    os_sku               = "AzureLinux"
    auto_scaling_enabled = true
    min_count            = var.node_min
    max_count            = var.node_max

    upgrade_settings {
      max_surge = "33%"
    }
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_data_plane  = "cilium"
    network_policy      = "cilium"
    load_balancer_sku   = "standard"
  }

  key_vault_secrets_provider {
    secret_rotation_enabled = true
  }

  # Managed Prometheus. The data collection rule that says where the metrics
  # go is in the 40-observability stack.
  monitor_metrics {}

  dynamic "service_mesh_profile" {
    for_each = var.mesh ? [1] : []
    content {
      mode                             = "Istio"
      revisions                        = [var.istio_revision]
      internal_ingress_gateway_enabled = true
      external_ingress_gateway_enabled = true
    }
  }

  maintenance_window_auto_upgrade {
    frequency   = "Weekly"
    interval    = 1
    day_of_week = var.environment == "prd" ? "Sunday" : "Wednesday"
    start_time  = "02:00"
    utc_offset  = "+00:00"
    duration    = 4
  }

  lifecycle {
    # The autoscaler changes the node count; an upgrade changes the version.
    ignore_changes = [default_node_pool[0].node_count, kubernetes_version]
  }
}

# The platform admins administer every cluster.
resource "azurerm_role_assignment" "admins" {
  scope                = azurerm_kubernetes_cluster.this.id
  role_definition_name = "Azure Kubernetes Service RBAC Cluster Admin"
  principal_id         = var.admins_group
  principal_type       = "Group"
}

# Nodes pull from the platform registry with the kubelet's own identity.
resource "azurerm_role_assignment" "pull" {
  scope                = var.registry_id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_kubernetes_cluster.this.kubelet_identity[0].object_id
  principal_type       = "ServicePrincipal"
}
