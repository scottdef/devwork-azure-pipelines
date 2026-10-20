# infra-net: Terraform module, phase 1 of infra-net/terraform-module/45
terraform {
  required_version = ">= 1.6"
}

variable "name" {
  type    = string
  default = "infra-net"
}

variable "cidr" {
  type    = string
  default = "10.20.0.0/16"
}

variable "tags" {
  type = map(string)
  default = {
    "source"  = "infra-net"
    "library" = "infra-net/terraform-module/45"
    "team" = "platform"
  }
}

locals {
  subnets = [cidrsubnet(var.cidr, 8, 1), cidrsubnet(var.cidr, 8, 2), cidrsubnet(var.cidr, 8, 3)]
}

output "subnets" {
  value = local.subnets
}
