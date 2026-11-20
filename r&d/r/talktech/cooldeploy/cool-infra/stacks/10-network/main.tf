# One virtual network per cluster, each peered with the hub.
#
# The hub (prd-eus2-coolops-aks-001) runs Argo CD, the runners and cool-api.
# Argo CD reaches the app clusters' API servers over the peerings, and the
# runners reach cool-api inside the hub.
#
# Each app network also has a small subnet delegated to API Management, for
# the outbound integration of that environment's instance. The instance
# itself is not here: cool-apim creates it, with scripts.

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
    key                  = "10-network.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))

  # prd-eus2-appdev-aks-001 -> prd-eus2-appdev
  stem  = { for name, c in local.clusters : name => "${c.environment}-${local.platform.region}-${c.workload}" }
  hub   = one([for name, c in local.clusters : name if c.role == "hub"])
  spoke = { for name, c in local.clusters : name => c if c.role != "hub" }
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

resource "azurerm_resource_group" "cluster" {
  for_each = { for name, c in local.clusters : name => c if name != local.hub }
  name     = "${local.stem[each.key]}-rg-001"
  location = local.platform.location
  tags     = merge(local.platform.tags, { "cool/env" = each.value.environment })
}

# The hub's resource group is created by 00-state, with the state account.
data "azurerm_resource_group" "hub" {
  name = "${local.stem[local.hub]}-rg-001"
}

locals {
  resource_group = merge(
    { for name, rg in azurerm_resource_group.cluster : name => rg.name },
    { (local.hub) = data.azurerm_resource_group.hub.name },
  )
}

resource "azurerm_virtual_network" "cluster" {
  for_each            = local.clusters
  name                = "${local.stem[each.key]}-vnet-001"
  resource_group_name = local.resource_group[each.key]
  location            = local.platform.location
  address_space       = [each.value.vnet_cidr]
  tags                = merge(local.platform.tags, { "cool/env" = each.value.environment })
}

resource "azurerm_subnet" "nodes" {
  for_each             = local.clusters
  name                 = "nodes"
  resource_group_name  = local.resource_group[each.key]
  virtual_network_name = azurerm_virtual_network.cluster[each.key].name
  address_prefixes     = [each.value.nodes_cidr]
}

resource "azurerm_subnet" "apim_outbound" {
  for_each             = { for name, c in local.clusters : name => c if c.apim_cidr != "" }
  name                 = "apim-outbound"
  resource_group_name  = local.resource_group[each.key]
  virtual_network_name = azurerm_virtual_network.cluster[each.key].name
  address_prefixes     = [each.value.apim_cidr]

  delegation {
    name = "apim"
    service_delegation {
      name    = "Microsoft.Web/serverFarms"
      actions = ["Microsoft.Network/virtualNetworks/subnets/action"]
    }
  }
}

resource "azurerm_virtual_network_peering" "hub_to_spoke" {
  for_each                  = local.spoke
  name                      = "to-${local.stem[each.key]}"
  resource_group_name       = local.resource_group[local.hub]
  virtual_network_name      = azurerm_virtual_network.cluster[local.hub].name
  remote_virtual_network_id = azurerm_virtual_network.cluster[each.key].id
  allow_forwarded_traffic   = false
}

resource "azurerm_virtual_network_peering" "spoke_to_hub" {
  for_each                  = local.spoke
  name                      = "to-${local.stem[local.hub]}"
  resource_group_name       = local.resource_group[each.key]
  virtual_network_name      = azurerm_virtual_network.cluster[each.key].name
  remote_virtual_network_id = azurerm_virtual_network.cluster[local.hub].id
  allow_forwarded_traffic   = false
}

output "node_subnets" {
  description = "Subnet ID of each cluster's nodes."
  value       = { for name, s in azurerm_subnet.nodes : name => s.id }
}

output "apim_outbound_subnets" {
  description = "Subnet ID for each environment's API Management instance; copy into cool-apim/instances/<env>.yaml."
  value       = { for name, s in azurerm_subnet.apim_outbound : local.clusters[name].environment => s.id }
}
