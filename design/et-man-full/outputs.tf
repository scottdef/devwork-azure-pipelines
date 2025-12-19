# terraform/outputs.tf
# Outputs for EasyTrade Terraform deployment

output "namespace_name" {
  description = "Name of the created namespace"
  value       = kubernetes_namespace.easytrade.metadata[0].name
}

output "cluster_name" {
  description = "AKS cluster name"
  value       = data.azurerm_kubernetes_cluster.aks.name
}

output "cluster_fqdn" {
  description = "AKS cluster FQDN"
  value       = data.azurerm_kubernetes_cluster.aks.fqdn
}

output "acr_login_server" {
  description = "ACR login server"
  value       = data.azurerm_container_registry.acr.login_server
}

output "database_secret_name" {
  description = "Name of the database credentials secret"
  value       = kubernetes_secret.database.metadata[0].name
}

output "configmap_name" {
  description = "Name of the application ConfigMap"
  value       = kubernetes_config_map.easytrade.metadata[0].name
}

output "helm_release_name" {
  description = "Helm release name (if deployed via Helm)"
  value       = var.deploy_via_helm ? helm_release.easytrade[0].name : "Not deployed via Helm"
}

output "helm_release_status" {
  description = "Helm release status (if deployed via Helm)"
  value       = var.deploy_via_helm ? helm_release.easytrade[0].status : "Not deployed via Helm"
}

output "kubectl_config_command" {
  description = "Command to configure kubectl"
  value       = "az aks get-credentials --resource-group ${var.resource_group_name} --name ${var.cluster_name}"
}

output "port_forward_command" {
  description = "Command to port-forward to EasyTrade"
  value       = "kubectl -n ${kubernetes_namespace.easytrade.metadata[0].name} port-forward svc/frontendreverseproxy 8080:80"
}

output "access_url" {
  description = "EasyTrade access URL (via port-forward)"
  value       = "http://localhost:8080"
}

output "default_credentials" {
  description = "Default login credentials"
  value = {
    user1 = "demouser / demopass"
    user2 = "specialuser / specialpass"
  }
  sensitive = true
}
