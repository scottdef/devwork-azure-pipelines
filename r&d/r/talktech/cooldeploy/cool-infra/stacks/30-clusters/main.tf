# The five clusters, from config/clusters.json.
#
#   prd-eus2-coolops-aks-001   hub: Argo CD, runners, cool-api
#   sbx-eus2-appsim-aks-001    sandbox: simulators, platform rehearsals
#   dev|tst|prd-eus2-appdev-aks-001   apps, one namespace per app
#
# A cluster is added, resized or given the mesh by editing the JSON file.

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
    key                  = "30-clusters.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))
  stem     = { for name, c in local.clusters : name => "${c.environment}-${local.platform.region}-${c.workload}" }
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

data "azurerm_container_registry" "platform" {
  name                = local.platform.registry
  resource_group_name = "prd-${local.platform.region}-coolops-rg-001"
}

data "azurerm_subnet" "nodes" {
  for_each             = local.clusters
  name                 = "nodes"
  virtual_network_name = "${local.stem[each.key]}-vnet-001"
  resource_group_name  = "${local.stem[each.key]}-rg-001"
}

module "cluster" {
  source   = "../../modules/aks-cluster"
  for_each = local.clusters

  name                = each.key
  environment         = each.value.environment
  resource_group_name = "${local.stem[each.key]}-rg-001"
  location            = local.platform.location
  tenant_id           = local.platform.tenant_id
  kubernetes_version  = local.platform.kubernetes_version
  subnet_id           = data.azurerm_subnet.nodes[each.key].id
  node_size           = each.value.nodes.size
  node_min            = each.value.nodes.min
  node_max            = each.value.nodes.max
  mesh                = each.value.mesh
  istio_revision      = local.platform.istio_revision
  registry_id         = data.azurerm_container_registry.platform.id
  admins_group        = local.platform.platform_admins_group
  tags                = merge(local.platform.tags, { "cool/env" = each.value.environment })
}

output "oidc_issuers" {
  description = "OIDC issuer URL of each cluster."
  value       = { for name, c in module.cluster : name => c.oidc_issuer_url }
}

output "cluster_ids" {
  value = { for name, c in module.cluster : name => c.id }
}
