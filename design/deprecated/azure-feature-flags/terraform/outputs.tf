output "managed_feature_flags" {
  description = "Feature flags managed by this Terraform configuration"
  value = {
    for name, flag in azurerm_app_configuration_feature.this : name => {
      name    = flag.name
      enabled = flag.enabled
      label   = flag.label
    }
  }
}

output "app_configuration_id" {
  description = "ID of the App Configuration instance"
  value       = data.azurerm_app_configuration.this.id
}

output "app_configuration_endpoint" {
  description = "Endpoint of the App Configuration instance"
  value       = data.azurerm_app_configuration.this.endpoint
}
