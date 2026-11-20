# Names inside the network.
#
# An app's host, <app>.<env>.coolapps.example, has to resolve for three
# callers: API Management, cron pods, and people on the internal network.
# All three get the address of that environment's internal ingress gateway,
# from a private DNS zone per environment with two wildcard records:
#
#   *.<env>.coolapps.example           apps
#   *.preview.<env>.coolapps.example   pull-request previews
#
# The gateway's address exists only once the cluster's Istio add-on has
# created its load balancer, so it comes from config/platform.json:
#
#   kubectl -n aks-istio-ingress get service aks-istio-ingressgateway-internal \
#     -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
#
# An environment with no address yet gets its zone and no records.
#
# Public DNS and certificates are not here. The wildcard certificate of each
# environment is imported into its platform Key Vault by whoever issues
# certificates in your organization, under the name the cool-platform chart
# expects: wildcard-<env>-coolapps.

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
    key                  = "35-dns.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))
  region   = local.platform.region
  ops_rg   = "prd-${local.region}-coolops-rg-001"

  stem = { for name, c in local.clusters : name => "${c.environment}-${local.region}-${c.workload}" }
  hub  = one([for name, c in local.clusters : name if c.role == "hub"])
  # environment -> the cluster its apps run on
  app_cluster = { for name, c in local.clusters : c.environment => name if c.role == "apps" }

  gateway_ips = { for env, ip in local.platform.internal_gateway_ips : env => ip if ip != "" }
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

data "azurerm_virtual_network" "cluster" {
  for_each            = local.clusters
  name                = "${local.stem[each.key]}-vnet-001"
  resource_group_name = "${local.stem[each.key]}-rg-001"
}

# --- App hosts ------------------------------------------------------------------

resource "azurerm_private_dns_zone" "apps" {
  for_each            = local.app_cluster
  name                = "${each.key}.${local.platform.domain}"
  resource_group_name = local.ops_rg
  tags                = merge(local.platform.tags, { "cool/env" = each.key })
}

# The environment's own network resolves its apps, and so does the hub,
# where the runners and Argo CD are.
resource "azurerm_private_dns_zone_virtual_network_link" "apps_env" {
  for_each              = local.app_cluster
  name                  = each.value
  resource_group_name   = local.ops_rg
  private_dns_zone_name = azurerm_private_dns_zone.apps[each.key].name
  virtual_network_id    = data.azurerm_virtual_network.cluster[each.value].id
}

resource "azurerm_private_dns_zone_virtual_network_link" "apps_hub" {
  for_each              = local.app_cluster
  name                  = local.hub
  resource_group_name   = local.ops_rg
  private_dns_zone_name = azurerm_private_dns_zone.apps[each.key].name
  virtual_network_id    = data.azurerm_virtual_network.cluster[local.hub].id
}

resource "azurerm_private_dns_a_record" "apps" {
  for_each            = local.gateway_ips
  name                = "*"
  zone_name           = azurerm_private_dns_zone.apps[each.key].name
  resource_group_name = local.ops_rg
  ttl                 = 300
  records             = [each.value]
}

resource "azurerm_private_dns_a_record" "previews" {
  for_each            = local.gateway_ips
  name                = "*.preview"
  zone_name           = azurerm_private_dns_zone.apps[each.key].name
  resource_group_name = local.ops_rg
  ttl                 = 300
  records             = [each.value]
}

# --- The control plane ------------------------------------------------------------

# cool-api.prd.coolops.example: where API Management finds cool-api, which
# listens on an internal load balancer of the hub cluster.
resource "azurerm_private_dns_zone" "ops" {
  name                = "prd.${local.platform.ops_domain}"
  resource_group_name = local.ops_rg
  tags                = local.platform.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "ops" {
  for_each              = local.clusters
  name                  = each.key
  resource_group_name   = local.ops_rg
  private_dns_zone_name = azurerm_private_dns_zone.ops.name
  virtual_network_id    = data.azurerm_virtual_network.cluster[each.key].id
}

resource "azurerm_private_dns_a_record" "cool_api" {
  count               = local.platform.cool_api_ip != "" ? 1 : 0
  name                = "cool-api"
  zone_name           = azurerm_private_dns_zone.ops.name
  resource_group_name = local.ops_rg
  ttl                 = 300
  records             = [local.platform.cool_api_ip]
}

output "zones" {
  value = { for env, z in azurerm_private_dns_zone.apps : env => z.name }
}
