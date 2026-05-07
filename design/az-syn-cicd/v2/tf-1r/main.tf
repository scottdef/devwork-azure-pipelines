###############################################################################
# main.tf — Synapse Infrastructure-as-Code
#
# State strategy: local backend, persisted as a GitHub Actions artifact.
# Each workflow downloads the previous state artifact before init, and
# uploads the updated state after apply.  This avoids any remote backend
# dependency (no Azure Storage, no S3, no TF Cloud) while keeping state
# durable across workflow runs.
#
# Ignored by design (per requirements):
#   - azurerm_synapse_firewall_rule
#   - azurerm_synapse_role_assignment (RBAC)
#   - azurerm_synapse_sql_pool
###############################################################################

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.100"
    }
  }

  # Local backend — state file lives on the runner filesystem.
  # Workflows restore it from a GitHub artifact before init and
  # upload it after apply.  See scripts/tf-state-artifact.sh.
  backend "local" {
    path = "terraform.tfstate"
  }
}
