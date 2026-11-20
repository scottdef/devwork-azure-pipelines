# The platform registry: every image a cluster runs comes from here.
#
# Images are built inside it with `az acr build`, so no runner needs Docker
# or a registry password, and admission policy on the clusters refuses any
# app image from elsewhere. Base images are mirrored into base/ by
# scripts/import-base-images.sh.

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
    key                  = "20-registry.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

data "azurerm_resource_group" "coolops" {
  name = "prd-${local.platform.region}-coolops-rg-001"
}

resource "azurerm_container_registry" "platform" {
  name                    = local.platform.registry
  resource_group_name     = data.azurerm_resource_group.coolops.name
  location                = data.azurerm_resource_group.coolops.location
  sku                     = "Premium"
  admin_enabled           = false
  anonymous_pull_enabled  = false
  zone_redundancy_enabled = true
  tags                    = local.platform.tags

  # Untagged manifests are what a rolled-back release points at for a while;
  # keep them a month.
  retention_policy_in_days = 30
}

output "registry_id" {
  value = azurerm_container_registry.platform.id
}

output "login_server" {
  value = azurerm_container_registry.platform.login_server
}
