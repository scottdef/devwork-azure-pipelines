#!/bin/bash

# Method 1: Merge all JSON files and deduplicate by 'id' field
# This combines all 'value' arrays and removes duplicates based on the 'id' field

echo "Merging JSON files and removing duplicates by ID..."
jq -s '
  map(.value) | 
  flatten | 
  unique_by(.id) | 
  sort_by(.id) | 
  { count: length, value: . }
' *.json > merged_deployments.json

echo "Merged file created: merged_deployments.json"
echo "Total unique records: $(jq '.count' merged_deployments.json)"

# Method 2: Simple concatenation of all 'value' arrays (includes duplicates)
# Use this if you want to keep all records including duplicates

echo "Creating simple concatenation (with duplicates)..."
jq -s '
  map(.value) | 
  flatten | 
  sort_by(.finishTime) | 
  { count: length, value: . }
' *.json > merged_all_deployments.json

echo "All records merged (with duplicates): merged_all_deployments.json"
echo "Total records: $(jq '.count' merged_all_deployments.json)"

# Method 3: Advanced merge with statistics
echo "Creating detailed merge with statistics..."
jq -s '
  {
    source_files: length,
    total_records: (map(.value | length) | add),
    unique_records: (map(.value) | flatten | unique_by(.id) | length),
    environments: (map(.value[].environmentId) | flatten | unique),
    date_range: {
      earliest: (map(.value[].finishTime) | flatten | min),
      latest: (map(.value[].finishTime) | flatten | max)
    },
    results_summary: (
      map(.value[].result) | 
      flatten | 
      group_by(.) | 
      map({result: .[0], count: length}) | 
      sort_by(.result)
    ),
    data: (
      map(.value) | 
      flatten | 
      unique_by(.id) | 
      sort_by(.finishTime | fromdateiso8601) |
      reverse
    )
  } |
  .count = .data | length
' *.json > detailed_merge.json

echo "Detailed merge created: detailed_merge.json"

# Method 4: One-liner for quick merge (most common use case)
echo "Quick one-liner merge (unique by ID, sorted by finish time):"
echo 'jq -s "map(.value) | flatten | unique_by(.id) | sort_by(.finishTime) | {count: length, value: .}" *.json > quick_merge.json'

# Execute the one-liner
jq -s 'map(.value) | flatten | unique_by(.id) | sort_by(.finishTime) | {count: length, value: .}' *.json > quick_merge.json

echo "Quick merge completed: quick_merge.json"