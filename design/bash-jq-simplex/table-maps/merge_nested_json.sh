#!/bin/bash

# Merge JSON files and nest by environmentId using prod-envIds.json metadata
# prod-envIds.json maps: environment_name -> environmentId

echo "=== Merging deployment files with environment nesting ==="

# Method 1: Merge deployment records (env-rec-dep-res files) and nest by environment name
echo "Processing deployment records..."
jq -s --slurpfile envs prod-envIds.json '
  # Create reverse mapping: environmentId -> environment_name
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  
  # Process deployment files
  map(select(has("value")) | .value) | 
  flatten | 
  unique_by(.id) |
  
  # Group by environmentId and nest under environment names
  group_by(.environmentId) |
  reduce .[] as $group ({};
    .[$id_to_name[$group[0].environmentId | tostring] // "unknown-\($group[0].environmentId)"] = {
      environmentId: $group[0].environmentId,
      environmentName: ($id_to_name[$group[0].environmentId | tostring] // "unknown"),
      deployments: {
        count: ($group | length),
        records: ($group | sort_by(.finishTime) | reverse)
      },
      summary: {
        total_deployments: ($group | length),
        succeeded: ($group | map(select(.result == "succeeded")) | length),
        failed: ($group | map(select(.result != "succeeded")) | length),
        success_rate: (($group | map(select(.result == "succeeded")) | length) / ($group | length) * 100 | floor),
        latest_deployment: ($group | map(.finishTime) | max),
        unique_definitions: ($group | map(.definition.name) | unique | sort)
      }
    }
  ) |
  
  # Add metadata
  {
    metadata: {
      generated: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
      source_type: "deployment_records",
      total_environments: (keys | length),
      total_deployments: ([.[] | .deployments.count] | add)
    },
    environments: .
  }
' env-rec-dep-res-*.json > deployment_records_by_env.json

echo "Created: deployment_records_by_env.json"

# Method 2: Merge pipeline summaries (deploy-summary files) and nest by environment name
echo "Processing pipeline summaries..."
jq -s --slurpfile envs prod-envIds.json '
  # Create reverse mapping: environmentId -> environment_name  
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  
  # Process summary files
  flatten |
  
  # Group by environmentId and nest under environment names
  group_by(.environmentId) |
  reduce .[] as $group ({};
    .[$id_to_name[$group[0].environmentId | tostring] // "unknown-\($group[0].environmentId)"] = {
      environmentId: $group[0].environmentId,
      environmentName: ($id_to_name[$group[0].environmentId | tostring] // "unknown"),
      pipelines: {
        count: ($group | length),
        records: ($group | sort_by(.latest) | reverse)
      },
      summary: {
        total_repositories: ($group | length),
        total_deployments: ($group | map(.total_deployments) | add),
        total_succeeded: ($group | map(.total_succeeded) | add),
        total_failed: ($group | map(.total_failed) | add),
        overall_success_rate: (($group | map(.total_succeeded) | add) / ($group | map(.total_deployments) | add) * 100 | floor),
        latest_activity: ($group | map(.latest) | max),
        repositories_with_failures: ($group | map(select(.total_failed > 0)) | length),
        pipeline_names: ($group | map(.["pipeline-repo"]) | sort)
      }
    }
  ) |
  
  # Add metadata
  {
    metadata: {
      generated: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
      source_type: "pipeline_summaries", 
      total_environments: (keys | length),
      total_repositories: ([.[] | .pipelines.count] | add)
    },
    environments: .
  }
' deploy-summary-*.json > pipeline_summaries_by_env.json

echo "Created: pipeline_summaries_by_env.json"

# Method 3: Comprehensive merge - both deployment records and pipeline summaries
echo "Creating comprehensive merge..."
jq -s --slurpfile envs prod-envIds.json '
  # Create reverse mapping: environmentId -> environment_name
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  
  # Separate file types
  [.[] | select(has("value"))] as $deployment_files |
  [.[] | select(type == "array")] as $summary_files |
  
  # Process deployment records
  ($deployment_files | map(.value) | flatten | unique_by(.id) | group_by(.environmentId)) as $deployments |
  
  # Process summary records  
  ($summary_files | flatten | group_by(.environmentId)) as $summaries |
  
  # Get all unique environment IDs from both data sources
  (($deployments + $summaries) | flatten | map(.environmentId // .[0].environmentId) | unique) as $all_env_ids |
  
  # Create comprehensive structure for each environment
  $all_env_ids | map(. as $env_id |
    {
      environmentId: $env_id,
      environmentName: ($id_to_name[$env_id | tostring] // "unknown-\($env_id)"),
      deployment_records: (
        ($deployments | map(select(.[0].environmentId == $env_id)) | flatten) as $env_deployments |
        if ($env_deployments | length) > 0 then {
          count: ($env_deployments | length),
          records: ($env_deployments | sort_by(.finishTime) | reverse),
          latest: ($env_deployments | map(.finishTime) | max),
          success_rate: (($env_deployments | map(select(.result == "succeeded")) | length) / ($env_deployments | length) * 100 | floor),
          definitions: ($env_deployments | map(.definition.name) | unique | sort)
        } else null end
      ),
      pipeline_summaries: (
        ($summaries | map(select(.[0].environmentId == $env_id)) | flatten) as $env_summaries |
        if ($env_summaries | length) > 0 then {
          count: ($env_summaries | length),
          records: ($env_summaries | sort_by(.latest) | reverse),
          total_deployments: ($env_summaries | map(.total_deployments) | add),
          overall_success_rate: (($env_summaries | map(.total_succeeded) | add) / ($env_summaries | map(.total_deployments) | add) * 100 | floor),
          latest_activity: ($env_summaries | map(.latest) | max),
          pipeline_names: ($env_summaries | map(.["pipeline-repo"]) | sort)
        } else null end
      )
    }
  ) |
  
  # Convert to keyed structure using environment names
  reduce .[] as $env ({};
    .[$env.environmentName] = $env
  ) |
  
  # Add comprehensive metadata
  {
    metadata: {
      generated: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
      source_types: ["deployment_records", "pipeline_summaries"],
      total_environments: (keys | length),
      environments_with_deployments: ([.[] | select(.deployment_records != null)] | length),
      environments_with_summaries: ([.[] | select(.pipeline_summaries != null)] | length),
      environment_mapping: ($id_to_name)
    },
    environments: .
  }
' env-rec-dep-res-*.json deploy-summary-*.json > comprehensive_deployment_data.json

echo "Created: comprehensive_deployment_data.json"

# Method 4: Create environment-specific files
echo "Creating environment-specific files..."
jq -r '.environments | keys[]' comprehensive_deployment_data.json | while read env_name; do
    echo "Extracting environment: $env_name"
    jq --arg env "$env_name" '.environments[$env] | {environment: $env, data: .}' comprehensive_deployment_data.json > "environment_${env_name}.json"
done

# Method 5: Validation and environment mapping verification
echo "Generating validation report..."
jq -s --slurpfile envs prod-envIds.json '
  # Create reverse mapping for validation
  ($envs[0] | to_entries | map(select(.key != "idList")) | map({key: (.value | tostring), value: .key}) | from_entries) as $id_to_name |
  
  # Get all environment IDs from data files
  [.[] | if has("value") then .value[].environmentId else .[].environmentId end] | unique as $data_env_ids |
  
  # Get all environment IDs from metadata
  $envs[0].idList as $metadata_env_ids |
  
  {
    validation: {
      total_environments_in_data: ($data_env_ids | length),
      total_environments_in_metadata: ($metadata_env_ids | length),
      environment_ids_in_data: $data_env_ids,
      environment_ids_in_metadata: ($metadata_env_ids | sort),
      mapped_environments: ($data_env_ids | map({environmentId: ., name: $id_to_name[. | tostring]})),
      missing_in_metadata: ($data_env_ids - $metadata_env_ids),
      missing_in_data: ($metadata_env_ids - $data_env_ids)
    },
    environment_mapping: $id_to_name,
    available_environments: ($envs[0] | to_entries | map(select(.key != "idList")) | map({name: .key, id: .value}) | sort_by(.name))
  }
' *.json > environment_validation.json

echo "Created: environment_validation.json"

# Method 6: Quick statistics
echo "Generating statistics..."
jq '
  {
    summary: {
      total_environments: (.environments | length),
      environments_with_deployment_records: ([.environments[] | select(.deployment_records != null)] | length),
      environments_with_pipeline_summaries: ([.environments[] | select(.pipeline_summaries != null)] | length),
      total_deployment_records: ([.environments[] | select(.deployment_records != null) | .deployment_records.count] | add // 0),
      total_pipeline_records: ([.environments[] | select(.pipeline_summaries != null) | .pipeline_summaries.count] | add // 0)
    },
    environment_list: (.environments | keys | sort),
    deployment_success_rates: (.environments | to_entries | map(select(.value.deployment_records != null) | {environment: .key, success_rate: .value.deployment_records.success_rate})),
    pipeline_success_rates: (.environments | to_entries | map(select(.value.pipeline_summaries != null) | {environment: .key, success_rate: .value.pipeline_summaries.overall_success_rate}))
  }
' comprehensive_deployment_data.json > deployment_statistics.json

echo "Created: deployment_statistics.json"

echo ""
echo "=== Summary of created files ==="
echo "- deployment_records_by_env.json: Deployment records nested by environment name"
echo "- pipeline_summaries_by_env.json: Pipeline summaries nested by environment name"  
echo "- comprehensive_deployment_data.json: Combined view of all deployment data"
echo "- environment_*.json: Individual environment files"
echo "- environment_validation.json: Validation report and environment mapping"
echo "- deployment_statistics.json: Statistics summary"
echo ""

# Display key statistics
echo "=== Key Statistics ==="
echo "Environments found in data: $(jq -r '.validation.total_environments_in_data' environment_validation.json)"
echo "Environments in metadata: $(jq -r '.validation.total_environments_in_metadata' environment_validation.json)" 
echo "Environment names: $(jq -r '.environment_list | join(", ")' deployment_statistics.json)"
echo "Total deployment records: $(jq -r '.summary.total_deployment_records' deployment_statistics.json)"
echo "Total pipeline summaries: $(jq -r '.summary.total_pipeline_records' deployment_statistics.json)"