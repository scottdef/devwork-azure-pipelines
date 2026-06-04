# ---------------------------------------------------------------------------
# Data sources
# ---------------------------------------------------------------------------

data "azurerm_resource_group" "this" {
  name = var.resource_group_name
}

data "azurerm_app_configuration" "this" {
  name                = var.app_configuration_name
  resource_group_name = data.azurerm_resource_group.this.name
}

# ---------------------------------------------------------------------------
# Locals — parse the JSON intent file into a map keyed by flag name
# ---------------------------------------------------------------------------

locals {
  feature_flags_raw = jsondecode(file(var.feature_flags_file))

  feature_flags = {
    for flag in local.feature_flags_raw : flag.name => flag.enabled
  }
}

# ---------------------------------------------------------------------------
# Feature flags — one resource per entry in app-feature-flags.json
#
# Terraform handles the diff natively:
#   • flag in JSON + not in state  → create
#   • flag in JSON + state differs → update
#   • flag removed from JSON       → destroy (from TF-managed set only)
#
# Flags that exist in App Configuration but are NOT in the JSON are
# never touched — they are outside Terraform's management scope.
# The companion compare-flags.py script reports those as informational.
# ---------------------------------------------------------------------------

resource "azurerm_app_configuration_feature" "this" {
  for_each = local.feature_flags

  configuration_store_id = data.azurerm_app_configuration.this.id
  name                   = each.key
  enabled                = each.value
  label                  = var.label != "" ? var.label : null

  lifecycle {
    # Prevent Terraform from clobbering description/filters managed outside IaC
    ignore_changes = [
      description,
      targeting_filter,
      timewindow_filter,
      percentage_filter_value,
      tags,
    ]
  }
}
