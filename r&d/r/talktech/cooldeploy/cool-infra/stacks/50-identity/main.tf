# The identities automation runs as, and what each may do.
#
# Nothing on the platform signs in with a stored secret. A workflow job or a
# pod presents a token that says what it is, and Azure exchanges it for an
# Azure token if a federated credential here says that subject is trusted.
#
#   identity     trusted subject                                    may
#   coolbuild    jobs defined by cool-actions' cool-build.yml@v1    build images in the registry
#   coolargo     Argo CD's service accounts on the hub              administer the app clusters
#   coolapi      cool-api's service account on the hub              read its GitHub App key
#   coolprov     IssueOps jobs in the environment cool-dev          create app identities
#   coolmon      IssueOps jobs in cool-tst, cool-prd, cool-sbx      read metrics, read image digests
#   coolplan     cool-infra pull requests                           read, for terraform plan
#   coolapply    cool-infra jobs in the environment cool-infra      apply this repository
#   coolapimr    cool-apim pull requests and main                   read API Management
#   coolapimw    cool-apim jobs in cool-apim-<env>                  configure API Management
#
# The first subject is the unusual one. It names a workflow file, not a
# repository: any app repository's pipeline may build, because the steps it
# runs are the platform's and not the app's. That works only for repositories
# whose OIDC subject includes job_workflow_ref, which the cool-app-create
# request sets.
#
# This stack creates the identity that applies it. The first apply is by a
# person with Owner on the subscription.

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
    key                  = "50-identity.tfstate"
    use_azuread_auth     = true
  }
}

locals {
  platform = jsondecode(file("${path.module}/../../config/platform.json"))
  clusters = jsondecode(file("${path.module}/../../config/clusters.json"))
  stem     = { for name, c in local.clusters : name => "${c.environment}-${local.platform.region}-${c.workload}" }
  ops      = "prd-${local.platform.region}-coolops"
  hub      = one([for name, c in local.clusters : name if c.role == "hub"])
  apps     = { for name, c in local.clusters : name => c if c.role == "apps" }
  org      = local.platform.github_org
  envs     = ["sbx", "dev", "tst", "prd"]

  github = "https://token.actions.githubusercontent.com"

  # identity -> the GitHub subjects it trusts
  github_subjects = {
    # The build job is defined in cool-build.yml and reached through
    # cool-pipeline.yml. Both are trusted: whichever of the two GitHub names
    # in the token, the steps that run are the platform's.
    coolbuild = [
      "job_workflow_ref:${local.org}/cool-actions/.github/workflows/cool-build.yml@refs/tags/v1",
      "job_workflow_ref:${local.org}/cool-actions/.github/workflows/cool-pipeline.yml@refs/tags/v1",
    ]
    coolprov  = ["repo:${local.org}/issueops:environment:cool-dev"]
    coolmon   = [for e in ["tst", "prd", "sbx"] : "repo:${local.org}/issueops:environment:cool-${e}"]
    coolplan  = ["repo:${local.org}/cool-infra:pull_request", "repo:${local.org}/cool-infra:ref:refs/heads/main"]
    coolapply = ["repo:${local.org}/cool-infra:environment:cool-infra"]
    coolapimr = ["repo:${local.org}/cool-apim:pull_request", "repo:${local.org}/cool-apim:ref:refs/heads/main"]
    coolapimw = [for e in local.envs : "repo:${local.org}/cool-apim:environment:cool-apim-${e}"]
  }

  # identity -> the service accounts on the hub cluster it trusts
  hub_subjects = {
    coolargo = [
      "system:serviceaccount:argocd:argocd-application-controller",
      "system:serviceaccount:argocd:argocd-server",
      "system:serviceaccount:argocd:argocd-applicationset-controller",
    ]
    coolapi = ["system:serviceaccount:cool-system:cool-api"]
  }

  names = toset(concat(keys(local.github_subjects), keys(local.hub_subjects)))

  github_credentials = merge([
    for id, subjects in local.github_subjects : {
      for i, s in subjects : "${id}-${i}" => { identity = id, subject = s }
    }
  ]...)
  hub_credentials = merge([
    for id, subjects in local.hub_subjects : {
      for i, s in subjects : "${id}-${i}" => { identity = id, subject = s }
    }
  ]...)
}

provider "azurerm" {
  features {}
  subscription_id = local.platform.subscription_id
}

data "azurerm_subscription" "current" {}

data "azurerm_resource_group" "coolops" {
  name = "${local.ops}-rg-001"
}

data "azurerm_kubernetes_cluster" "cluster" {
  for_each            = local.clusters
  name                = each.key
  resource_group_name = "${local.stem[each.key]}-rg-001"
}

data "azurerm_container_registry" "platform" {
  name                = local.platform.registry
  resource_group_name = data.azurerm_resource_group.coolops.name
}

data "azurerm_monitor_workspace" "metrics" {
  name                = "${local.ops}-amw-001"
  resource_group_name = data.azurerm_resource_group.coolops.name
}

resource "azurerm_user_assigned_identity" "this" {
  for_each            = local.names
  name                = "prd-${local.platform.region}-${each.key}-id-001"
  resource_group_name = data.azurerm_resource_group.coolops.name
  location            = data.azurerm_resource_group.coolops.location
  tags                = local.platform.tags
}

resource "azurerm_federated_identity_credential" "github" {
  for_each            = local.github_credentials
  name                = "github-${each.key}"
  resource_group_name = data.azurerm_resource_group.coolops.name
  parent_id           = azurerm_user_assigned_identity.this[each.value.identity].id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = local.github
  subject             = each.value.subject
}

resource "azurerm_federated_identity_credential" "hub" {
  for_each            = local.hub_credentials
  name                = "hub-${each.key}"
  resource_group_name = data.azurerm_resource_group.coolops.name
  parent_id           = azurerm_user_assigned_identity.this[each.value.identity].id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = data.azurerm_kubernetes_cluster.cluster[local.hub].oidc_issuer_url
  subject             = each.value.subject
}

# --- What each may do ---------------------------------------------------------

# `az acr build` schedules a run in the registry and uploads its source.
resource "azurerm_role_assignment" "build" {
  scope                = data.azurerm_container_registry.platform.id
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.this["coolbuild"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "argo" {
  for_each             = local.apps
  scope                = data.azurerm_kubernetes_cluster.cluster[each.key].id
  role_definition_name = "Azure Kubernetes Service RBAC Cluster Admin"
  principal_id         = azurerm_user_assigned_identity.this["coolargo"].principal_id
  principal_type       = "ServicePrincipal"
}

# Creating an app means an identity per environment, federated with the app
# cluster, with a role on the team's Key Vault.
resource "azurerm_role_assignment" "provision_identities" {
  for_each             = { for name, c in local.apps : name => c if c.environment != "sbx" }
  scope                = "${data.azurerm_subscription.current.id}/resourceGroups/${local.stem[each.key]}-rg-001"
  role_definition_name = "Managed Identity Contributor"
  principal_id         = azurerm_user_assigned_identity.this["coolprov"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "provision_roles" {
  for_each             = { for name, c in local.apps : name => c if c.environment != "sbx" }
  scope                = "${data.azurerm_subscription.current.id}/resourceGroups/${local.stem[each.key]}-rg-001"
  role_definition_name = "Role Based Access Control Administrator"
  principal_id         = azurerm_user_assigned_identity.this["coolprov"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "provision_reads" {
  scope                = data.azurerm_subscription.current.id
  role_definition_name = "Reader"
  principal_id         = azurerm_user_assigned_identity.this["coolprov"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "monitor_metrics" {
  scope                = data.azurerm_monitor_workspace.metrics.id
  role_definition_name = "Monitoring Data Reader"
  principal_id         = azurerm_user_assigned_identity.this["coolmon"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "monitor_registry" {
  scope                = data.azurerm_container_registry.platform.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.this["coolmon"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "plan" {
  scope                = data.azurerm_subscription.current.id
  role_definition_name = "Reader"
  principal_id         = azurerm_user_assigned_identity.this["coolplan"].principal_id
  principal_type       = "ServicePrincipal"
}

# Applying this repository means creating resources and assigning roles.
resource "azurerm_role_assignment" "apply" {
  for_each             = toset(["Contributor", "Role Based Access Control Administrator"])
  scope                = data.azurerm_subscription.current.id
  role_definition_name = each.key
  principal_id         = azurerm_user_assigned_identity.this["coolapply"].principal_id
  principal_type       = "ServicePrincipal"
}

# The resource groups API Management lives in. The instances are not here.
resource "azurerm_resource_group" "coolapi" {
  for_each = toset(local.envs)
  name     = "${each.key}-${local.platform.region}-coolapi-rg-001"
  location = local.platform.location
  tags     = merge(local.platform.tags, { "cool/env" = each.key, "cool/managed-by" = "cool-apim" })
}

resource "azurerm_role_assignment" "apim_read" {
  for_each             = azurerm_resource_group.coolapi
  scope                = each.value.id
  role_definition_name = "API Management Service Reader Role"
  principal_id         = azurerm_user_assigned_identity.this["coolapimr"].principal_id
  principal_type       = "ServicePrincipal"
}

resource "azurerm_role_assignment" "apim_write" {
  for_each             = azurerm_resource_group.coolapi
  scope                = each.value.id
  role_definition_name = "API Management Service Contributor"
  principal_id         = azurerm_user_assigned_identity.this["coolapimw"].principal_id
  principal_type       = "ServicePrincipal"
}

output "client_ids" {
  description = "Client ID of each identity; they become GitHub variables and chart values."
  value       = { for name, id in azurerm_user_assigned_identity.this : name => id.client_id }
}

output "argocd_client_id" {
  value = azurerm_user_assigned_identity.this["coolargo"].client_id
}
