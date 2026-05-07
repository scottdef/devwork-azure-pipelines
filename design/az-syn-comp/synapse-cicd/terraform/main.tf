# ============================================================================
# Azure Synapse CI/CD — Terraform Root Module
# ============================================================================
# State backend: local file on the GitHub runner.
# The workflow downloads previous state from a GitHub artifact before
# plan/apply, and uploads the new state afterward.
# ============================================================================

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.100"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "~> 2.47"
    }
  }

  backend "local" {
    path = "terraform.tfstate"
  }
}

# ── Data Sources ─────────────────────────────────────────────────────────────

data "azurerm_client_config" "current" {}
data "azurerm_subscription" "current" {}

# ── Resource Groups ──────────────────────────────────────────────────────────

resource "azurerm_resource_group" "synapse" {
  for_each = var.environments

  name     = each.value.resource_group_name
  location = each.value.location

  tags = merge(var.common_tags, {
    environment = each.key
    managed_by  = "terraform"
  })
}

# ── Storage Accounts (ADLS Gen2 — Synapse primary storage) ──────────────────

resource "azurerm_storage_account" "synapse_datalake" {
  for_each = var.environments

  name                     = each.value.storage_account_name
  resource_group_name      = azurerm_resource_group.synapse[each.key].name
  location                 = azurerm_resource_group.synapse[each.key].location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"
  is_hns_enabled           = true

  tags = merge(var.common_tags, {
    environment = each.key
  })
}

resource "azurerm_storage_data_lake_gen2_filesystem" "synapse_fs" {
  for_each = var.environments

  name               = each.value.filesystem_name
  storage_account_id = azurerm_storage_account.synapse_datalake[each.key].id
}
