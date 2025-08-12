# JQ Commands for Environment-Based JSON Merging with prod-envIds.json
# prod-envIds.json structure: {"environment_name": environmentId, "idList": [ids...]}

# 1. CREATE REVERSE MAPPING (environmentId -> environment_name)
jq --slurpfile envs prod-envIds.json '
  $envs[0] | to_entries | map(select(.key != "idList")) | 
  map({key: (.value | tostring), value: .key}) | from_entries
' < /dev/null

# 2. BASIC MERGE WITH ENVIRONMENT NESTING (deployment records)
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  map(select(has("value")) | .value) | flatten | unique_by(.id) |
  group_by(.environmentId) |
  reduce .[] as $group ({};
    .[$id_to_name[$group[0].environmentId | tostring] // "unknown-\($group[0].environmentId)"] = {
      environmentId: $group[0].environmentId,
      count: ($group | length),
      deployments: $group
    }
  )
' env-rec-dep-res-*.json

# 3. MERGE PIPELINE SUMMARIES BY ENVIRONMENT NAME
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  flatten | group_by(.environmentId) |
  reduce .[] as $group ({};
    .[$id_to_name[$group[0].environmentId | tostring] // "unknown-\($group[0].environmentId)"] = {
      environmentId: $group[0].environmentId,
      pipelines: $group,
      stats: {
        total_deployments: ($group | map(.total_deployments) | add),
        success_rate: (($group | map(.total_succeeded) | add) / ($group | map(.total_deployments) | add) * 100 | floor)
      }
    }
  )
' deploy-summary-*.json

# 4. COMPREHENSIVE MERGE (both file types with environment names)
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  [.[] | select(has("value"))] as $deploy_files |
  [.[] | select(type == "array")] as $summary_files |
  
  ($deploy_files | map(.value) | flatten | unique_by(.id) | group_by(.environmentId)) as $deployments |
  ($summary_files | flatten | group_by(.environmentId)) as $summaries |
  
  (($deployments + $summaries) | flatten | map(.environmentId // .[0].environmentId) | unique) |
  reduce .[] as $env_id ({};
    .[$id_to_name[$env_id | tostring] // "env-\($env_id)"] = {
      environmentId: $env_id,
      deployments: ($deployments | map(select(.[0].environmentId == $env_id)) | flatten),
      summaries: ($summaries | map(select(.[0].environmentId == $env_id)) | flatten)
    }
  )
' env-rec-dep-res-*.json deploy-summary-*.json

# 5. VALIDATE ENVIRONMENT MAPPING
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  [.[] | if has("value") then .value[].environmentId else .[].environmentId end] | unique |
  map({
    environmentId: .,
    environment_name: ($id_to_name[. | tostring] // "unmapped"),
    has_mapping: ($id_to_name | has(. | tostring))
  })
' *.json

# 6. FILTER BY SPECIFIC ENVIRONMENTS (by name)
jq -s --slurpfile envs prod-envIds.json --argjson target_names '["clienthq", "camunda"]' '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  # Get target environment IDs
  ($envs[0] | to_entries | map(select($target_names | contains([.key]))) | map(.value)) as $target_ids |
  
  map(select(has("value")) | .value) | flatten | unique_by(.id) |
  map(select(.environmentId as $id | $target_ids | contains([$id]))) |
  group_by(.environmentId) |
  reduce .[] as $group ({};
    .[$id_to_name[$group[0].environmentId | tostring]] = $group
  )
' env-rec-dep-res-*.json

# 7. STATISTICS BY ENVIRONMENT NAME
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  flatten | group_by(.environmentId) |
  map({
    environment_name: ($id_to_name[.[0].environmentId | tostring] // "unmapped"),
    environmentId: .[0].environmentId,
    total_pipelines: length,
    total_deployments: (map(.total_deployments) | add),
    success_rate: ((map(.total_succeeded) | add) / (map(.total_deployments) | add) * 100 | round),
    failed_pipelines: (map(select(.total_failed > 0)) | length)
  }) | sort_by(.environment_name)
' deploy-summary-*.json

# 8. CREATE GRAFANA-FRIENDLY TIME SERIES DATA
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  map(select(has("value")) | .value) | flatten | unique_by(.id) |
  map({
    timestamp: (.finishTime | fromdateiso8601),
    environment_name: ($id_to_name[.environmentId | tostring] // "unmapped"),
    environmentId: .environmentId,
    pipeline: .definition.name,
    result: .result,
    success: (if .result == "succeeded" then 1 else 0 end)
  }) | sort_by(.timestamp)
' env-rec-dep-res-*.json

# 9. EXTRACT ENVIRONMENT MAPPINGS ONLY
jq --slurpfile envs prod-envIds.json '
  $envs[0] | to_entries | map(select(.key != "idList")) | 
  map({environment_name: .key, environmentId: .value}) | 
  sort_by(.environment_name)
' < /dev/null

# 10. FIND UNMAPPED ENVIRONMENTS IN DATA
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map(.value)) as $known_ids |
  [.[] | if has("value") then .value[].environmentId else .[].environmentId end] | unique |
  map(select(. as $id | $known_ids | contains([$id]) | not)) |
  map({environmentId: ., status: "unmapped"})
' *.json

# 11. MERGE WITH SUCCESS RATE CALCULATIONS PER ENVIRONMENT
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  flatten | group_by(.environmentId) |
  reduce .[] as $group ({};
    .[$id_to_name[$group[0].environmentId | tostring] // "env-\($group[0].environmentId)"] = {
      environment: {
        name: ($id_to_name[$group[0].environmentId | tostring] // "unmapped"),
        id: $group[0].environmentId
      },
      metrics: {
        total_pipelines: ($group | length),
        total_deployments: ($group | map(.total_deployments) | add),
        total_succeeded: ($group | map(.total_succeeded) | add),
        total_failed: ($group | map(.total_failed) | add),
        success_rate: (($group | map(.total_succeeded) | add) / ($group | map(.total_deployments) | add) * 100 | floor),
        latest_activity: ($group | map(.latest) | max)
      },
      pipelines: $group
    }
  )
' deploy-summary-*.json

# 12. ONE-LINER FOR QUICK ENVIRONMENT GROUPING BY NAME
jq -s --slurpfile envs prod-envIds.json '($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name | flatten | group_by(.environmentId) | reduce .[] as $g ({}; .[$id_to_name[$g[0].environmentId|tostring]//"unknown"] = $g)' deploy-summary-*.json

# 13. EXTRACT SPECIFIC ENVIRONMENT DATA BY NAME
jq -s --slurpfile envs prod-envIds.json --arg env_name "clienthq" '
  ($envs[0][$env_name]) as $target_id |
  if $target_id then
    flatten | map(select(.environmentId == $target_id))
  else
    error("Environment \($env_name) not found in prod-envIds.json")
  end
' deploy-summary-*.json

# 14. CREATE ENVIRONMENT HEALTH DASHBOARD DATA
jq -s --slurpfile envs prod-envIds.json '
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  [.[] | select(has("value"))] as $deploy_files |
  [.[] | select(type == "array")] as $summary_files |
  
  # Create dashboard structure
  {
    dashboard: {
      title: "Environment Deployment Health",
      generated: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
      environments: (
        ($summary_files | flatten | group_by(.environmentId)) |
        map({
          name: ($id_to_name[.[0].environmentId | tostring] // "unknown"),
          id: .[0].environmentId,
          health_score: ((map(.total_succeeded) | add) / (map(.total_deployments) | add) * 100 | floor),
          total_deployments: (map(.total_deployments) | add),
          failed_deployments: (map(.total_failed) | add),
          pipeline_count: length,
          latest_activity: (map(.latest) | max),
          status: (
            if ((map(.total_succeeded) | add) / (map(.total_deployments) | add) * 100) >= 90 then "healthy"
            elif ((map(.total_succeeded) | add) / (map(.total_deployments) | add) * 100) >= 70 then "warning"
            else "critical" end
          )
        }) | sort_by(.health_score) | reverse
      )
    }
  }
' env-rec-dep-res-*.json deploy-summary-*.json