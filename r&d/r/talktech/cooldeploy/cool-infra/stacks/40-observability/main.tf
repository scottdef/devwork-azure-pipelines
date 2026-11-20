# Metrics: one Azure Monitor workspace (managed Prometheus) that every
# cluster writes to, and one Azure Managed Grafana that reads it.
#
# The clusters' metrics add-on is enabled in 30-clusters. What is here says
# where each cluster's metrics go: a data collection rule per cluster, all
# pointing at the same workspace, so one PromQL query can compare a release
# in tst with the same release in prd.

terraform {
  required_version = ">= 1.9"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.40"
    }
  }
  backend "azurerm" {
    resource_group_name  = "prd-eus2-coolops-rg-001"
    storage_account_name = "prdeus2coolopsst001"
    container_name       = "tfstate"
    key                  = "40-observability.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))
  stem     = { for name, c in local.clusters : name => "${c.environment}-${local.platform.region}-${c.workload}" }
  ops      = "prd-${local.platform.region}-coolops"
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

data "azurerm_resource_group" "coolops" {
  name = "${local.ops}-rg-001"
}

data "azurerm_kubernetes_cluster" "cluster" {
  for_each            = local.clusters
  name                = each.key
  resource_group_name = "${local.stem[each.key]}-rg-001"
}

resource "azurerm_monitor_workspace" "metrics" {
  name                = "${local.ops}-amw-001"
  resource_group_name = data.azurerm_resource_group.coolops.name
  location            = data.azurerm_resource_group.coolops.location
  tags                = local.platform.tags
}

resource "azurerm_monitor_data_collection_endpoint" "metrics" {
  name                = "${local.ops}-dce-001"
  resource_group_name = data.azurerm_resource_group.coolops.name
  location            = data.azurerm_resource_group.coolops.location
  kind                = "Linux"
  tags                = local.platform.tags
}

resource "azurerm_monitor_data_collection_rule" "metrics" {
  name                        = "${local.ops}-dcr-001"
  resource_group_name         = data.azurerm_resource_group.coolops.name
  location                    = data.azurerm_resource_group.coolops.location
  data_collection_endpoint_id = azurerm_monitor_data_collection_endpoint.metrics.id
  kind                        = "Linux"
  tags                        = local.platform.tags

  destinations {
    monitor_account {
      monitor_account_id = azurerm_monitor_workspace.metrics.id
      name               = "workspace"
    }
  }

  data_flow {
    streams      = ["Microsoft-PrometheusMetrics"]
    destinations = ["workspace"]
  }

  data_sources {
    prometheus_forwarder {
      name    = "prometheus"
      streams = ["Microsoft-PrometheusMetrics"]
    }
  }
}

resource "azurerm_monitor_data_collection_rule_association" "cluster" {
  for_each                = local.clusters
  name                    = "${each.key}-metrics"
  target_resource_id      = data.azurerm_kubernetes_cluster.cluster[each.key].id
  data_collection_rule_id = azurerm_monitor_data_collection_rule.metrics.id
}

# Grafana names may be 23 characters at most; this one is exactly that.
resource "azurerm_dashboard_grafana" "platform" {
  name                  = "${local.ops}-mg-001"
  resource_group_name   = data.azurerm_resource_group.coolops.name
  location              = data.azurerm_resource_group.coolops.location
  grafana_major_version = local.platform.grafana_major_version
  sku                   = "Standard"
  tags                  = local.platform.tags

  identity {
    type = "SystemAssigned"
  }

  azure_monitor_workspace_integrations {
    resource_id = azurerm_monitor_workspace.metrics.id
  }
}

resource "azurerm_role_assignment" "grafana_reads_metrics" {
  scope                = azurerm_monitor_workspace.metrics.id
  role_definition_name = "Monitoring Data Reader"
  principal_id         = azurerm_dashboard_grafana.platform.identity[0].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "admins_administer_grafana" {
  scope                = azurerm_dashboard_grafana.platform.id
  role_definition_name = "Grafana Admin"
  principal_id         = local.platform.platform_admins_group
  principal_type       = "Group"
}

output "prometheus_query_endpoint" {
  description = "Set as the COOL_PROM_URL variable: what `coolctl gate` asks."
  value       = azurerm_monitor_workspace.metrics.query_endpoint
}

output "workspace_id" {
  value = azurerm_monitor_workspace.metrics.id
}

output "grafana_endpoint" {
  value = azurerm_dashboard_grafana.platform.endpoint
}
