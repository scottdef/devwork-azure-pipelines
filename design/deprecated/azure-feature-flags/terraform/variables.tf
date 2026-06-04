variable "app_configuration_name" {
  description = "Name of the Azure App Configuration instance"
  type        = string
}

variable "resource_group_name" {
  description = "Resource group containing the App Configuration instance"
  type        = string
}

variable "feature_flags_file" {
  description = "Path to the JSON file containing feature flag definitions"
  type        = string
  default     = "../app-feature-flags.json"
}

variable "label" {
  description = "Label to apply to feature flags (maps to environment/slice)"
  type        = string
  default     = ""
}
