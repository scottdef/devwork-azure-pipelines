# Teams creation
resource "github_team" "teams" {
  for_each = local.all_teams

  name        = each.value.name
  description = try(each.value.description, "Managed by Terraform")
  privacy     = try(each.value.privacy, "closed")
  
  # Handle parent team relationships
  parent_team_id = try(
    github_team.teams[each.value.parent_name].id,
    try(each.value.parent_id, null)
  )

  lifecycle {
    create_before_destroy = true
  }
}

# Team memberships from YAML
resource "github_team_membership" "members" {
  for_each = {
    for item in flatten([
      for team_name, team in local.teams : [
        for member in try(team.members, []) : {
          team   = team_name
          member = member.username
          role   = try(member.role, "member")
          key    = "${team_name}-${member.username}"
        }
      ]
    ]) : item.key => item
  }

  team_id  = github_team.teams[each.value.team].id
  username = each.value.member
  role     = each.value.role

  depends_on = [github_team.teams]
}

# Subteam memberships for admins
resource "github_team_membership" "admin_subteam_members" {
  for_each = {
    for item in flatten([
      for team_name, team in local.teams : [
        for member in try(team.members, []) : {
          team   = "${team_name}-admins"
          member = member.username
          role   = "maintainer"
          key    = "${team_name}-admins-${member.username}"
        }
        if try(member.role, "member") == "maintainer" && try(team.ad_group, null) != null
      ]
    ]) : item.key => item
  }

  team_id  = github_team.teams[each.value.team].id
  username = each.value.member
  role     = each.value.role

  depends_on = [github_team.teams]
}

# Organization-level team settings
resource "github_team_settings" "team_settings" {
  for_each = {
    for name, team in local.teams : name => team
    if try(team.review_request_delegation, null) != null
  }

  team_id = github_team.teams[each.key].id

  review_request_delegation {
    algorithm    = try(each.value.review_request_delegation.algorithm, "ROUND_ROBIN")
    member_count = try(each.value.review_request_delegation.member_count, 1)
    notify       = try(each.value.review_request_delegation.notify, true)
  }
}

# Output team information
output "teams" {
  description = "Map of all teams created"
  value = {
    for name, team in github_team.teams : name => {
      id   = team.id
      slug = team.slug
      name = team.name
    }
  }
}

output "team_subteams" {
  description = "Map of teams to their subteams"
  value = {
    for team_name, team in local.teams : team_name => [
      for st in local.subteams : st.name
      if st.parent_name == team_name
    ]
  }
}
