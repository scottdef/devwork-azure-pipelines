terraform {
  required_version = ">= 1.6.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
    dynatrace = {
      source  = "dynatrace-oss/dynatrace"
      version = "~> 1.107"
    }
  }

  # State holds the Dynatrace ingest token in plain text (the provider
  # marks it sensitive but stores it). Use an encrypted, access-controlled
  # remote backend; never local state in CI. Example (Azure):
  #
  # backend "azurerm" {
  #   resource_group_name  = "prod-cus-platform-base-rg-001"
  #   storage_account_name = "coolgittfstate"
  #   container_name       = "tfstate"
  #   key                  = "gh-log-forwarder.tfstate"
  #   use_azuread_auth     = true
  # }
}
