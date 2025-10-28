# Organization Settings Variables

variable "org_billing_email" {
  description = "Billing email for the organization"
  type        = string
}

variable "org_company" {
  description = "Company name"
  type        = string
  default     = null
}

variable "org_email" {
  description = "Public email for the organization"
  type        = string
  default     = null
}

variable "org_twitter" {
  description = "Twitter username (without @)"
  type        = string
  default     = null
}

variable "org_location" {
  description = "Organization location"
  type        = string
  default     = null
}

variable "org_description" {
  description = "Organization description"
  type        = string
  default     = null
}

variable "org_display_name" {
  description = "Display name for the organization"
  type        = string
  default     = null
}

variable "org_blog" {
  description = "Organization blog URL"
  type        = string
  default     = null
}
