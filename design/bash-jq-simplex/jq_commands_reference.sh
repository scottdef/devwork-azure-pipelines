# Essential JQ Commands for Merging JSON Files

# 1. BASIC MERGE (removes duplicates by ID)
jq -s 'map(.value) | flatten | unique_by(.id) | sort_by(.id) | {count: length, value: .}' *.json

# 2. MERGE WITH TIME SORTING (most recent first)
jq -s 'map(.value) | flatten | unique_by(.id) | sort_by(.finishTime) | reverse | {count: length, value: .}' *.json

# 3. MERGE SPECIFIC ENVIRONMENT ONLY
jq -s 'map(.value) | flatten | map(select(.environmentId == 64)) | unique_by(.id) | {count: length, value: .}' *.json

# 4. MERGE WITH FILTERING (only successful deployments)
jq -s 'map(.value) | flatten | unique_by(.id) | map(select(.result == "succeeded")) | {count: length, value: .}' *.json

# 5. MERGE WITH DATE RANGE FILTERING (last 30 days)
jq -s --arg date_cutoff "$(date -d '30 days ago' -u +%Y-%m-%dT%H:%M:%SZ)" '
  map(.value) | 
  flatten | 
  unique_by(.id) | 
  map(select(.finishTime >= $date_cutoff)) | 
  sort_by(.finishTime) | 
  {count: length, value: .}
' *.json

# 6. COUNT RECORDS BY DEFINITION NAME
jq -s 'map(.value) | flatten | unique_by(.id) | group_by(.definition.name) | map({name: .[0].definition.name, count: length})' *.json

# 7. MERGE AND EXTRACT ONLY SPECIFIC FIELDS
jq -s 'map(.value) | flatten | unique_by(.id) | map({id, result, name: .definition.name, finishTime}) | {count: length, value: .}' *.json

# 8. VALIDATE AND MERGE (skip invalid JSON files)
find . -name "*.json" -exec jq -e . {} \; 2>/dev/null | jq -s 'map(select(has("value"))) | map(.value) | flatten | unique_by(.id) | {count: length, value: .}'

# 9. MERGE WITH ERROR HANDLING
jq -s 'map(select(type == "object" and has("value"))) | map(.value) | flatten | unique_by(.id) | {count: length, value: .}' *.json 2>/dev/null || echo "Error: Invalid JSON format in one or more files"

# 10. OUTPUT TO DIFFERENT FORMATS

# Output as CSV
jq -s -r 'map(.value) | flatten | unique_by(.id) | sort_by(.id) | ["id","environmentId","result","finishTime","definitionName"], (.[] | [.id, .environmentId, .result, .finishTime, .definition.name]) | @csv' *.json

# Output as Tab-separated
jq -s -r 'map(.value) | flatten | unique_by(.id) | sort_by(.id) | .[] | [.id, .environmentId, .result, .finishTime, .definition.name] | @tsv' *.json

# 11. MERGE WITH STATISTICS
jq -s '{
  merged_data: (map(.value) | flatten | unique_by(.id) | sort_by(.finishTime)),
  statistics: {
    total_files: length,
    unique_deployments: (map(.value) | flatten | unique_by(.id) | length),
    environments: (map(.value[].environmentId) | flatten | unique | sort),
    success_rate: (map(.value) | flatten | unique_by(.id) | [group_by(.result)[] | {key: .[0].result, value: length}] | from_entries)
  }
} | .count = (.merged_data | length)' *.json