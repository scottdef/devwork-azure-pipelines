terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }

  backend "azurerm" {
    # Configured via -backend-config in CI:
    #   resource_group_name  = "tfstate-rg"
    #   storage_account_name = "tfstateXXXXX"
    #   container_name       = "tfstate"
    #   key                  = "feature-flags.tfstate"
    #   use_oidc             = true
  }
}

provider "azurerm" {
  features {}
  use_oidc = true
}
