# The storage account that holds every other stack's state.
#
# This stack is the one exception to "state lives in Azure": it creates the
# place state lives in, so it starts with local state. Apply it once by hand,
# then uncomment the backend block and run `terraform init -migrate-state`.

terraform {
  required_version = ">= 1.9"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.40"
    }
  }

  # backend "azurerm" {
  #   resource_group_name  = "prd-eus2-coolops-rg-001"
  #   storage_account_name = "prdeus2coolopsst001"
  #   container_name       = "tfstate"
  #   key                  = "00-state.tfstate"
  #   use_azuread_auth     = true
  # }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
}

provider "azurerm" {
  features {}
  subscription_id     = local.platform.subscription_id
  storage_use_azuread = true
}

resource "azurerm_resource_group" "coolops" {
  name     = "prd-${local.platform.region}-coolops-rg-001"
  location = local.platform.location
  tags     = local.platform.tags
}

resource "azurerm_storage_account" "state" {
  name                            = local.platform.state_account
  resource_group_name             = azurerm_resource_group.coolops.name
  location                        = azurerm_resource_group.coolops.location
  account_tier                    = "Standard"
  account_replication_type        = "GZRS"
  min_tls_version                 = "TLS1_2"
  shared_access_key_enabled       = false
  allow_nested_items_to_be_public = false
  tags                            = local.platform.tags

  blob_properties {
    versioning_enabled = true
    delete_retention_policy {
      days = 30
    }
  }
}

resource "azurerm_storage_container" "tfstate" {
  name                  = "tfstate"
  storage_account_id    = azurerm_storage_account.state.id
  container_access_type = "private"
}

output "state_account" {
  value = azurerm_storage_account.state.name
}
