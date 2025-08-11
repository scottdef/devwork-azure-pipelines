I'll provide you with bash commands to append multiple JSON files with identical structure into a single JSON file. Here are different approaches depending on your JSON structure:Now let me create a practical implementation script that you can use directly in your Azure DevOps API container:Here are the most commonly used commands for merging JSON files:

## Quick Reference Commands:

**1. Simple Array Merge (Most Common)**
```bash
# Merge all JSON files containing arrays
jq -s 'add' *.json > merged.json

# Merge specific pattern files
jq -s 'add' builds_*.json > all_builds.json
```

**2. Merge with Validation**
```bash
# Only merge valid JSON files
for file in *.json; do
    if jq empty "$file" 2>/dev/null; then
        echo "$file" >> valid_files.txt
    fi
done
jq -s 'add' $(cat valid_files.txt) > merged.json
```

**3. Add Metadata During Merge**
```bash
# Include merge timestamp and file count
jq -s --argjson count "$(ls *.json | wc -l)" '
{
    "merged_at": now | strftime("%Y-%m-%d %H:%M:%S"),
    "file_count": $count,
    "total_records": (add | length),
    "data": add
}' *.json > merged_with_metadata.json
```

**4. Azure DevOps Specific Example**
```bash
# Merge Azure DevOps builds with processing
jq -s 'add | map({
    id: .id,
    buildNumber: .buildNumber,
    pipeline: .definition.name,
    status: .status,
    result: .result,
    queueTime: .queueTime,
    duration: (if .finishTime and .startTime then
        (((.finishTime | fromdateiso8601) - (.startTime | fromdateiso8601)) / 60 | floor)
        else null end)
}) | sort_by(.queueTime)' builds_*.json > processed_builds.json
```

The comprehensive script I created above (`azdo_json_merger`) provides a production-ready solution specifically for Azure DevOps API data that includes error handling, validation, CSV export, and detailed reporting.

To use it in your container:
```bash
# Make the script executable
chmod +x /scripts/azdo_json_merger.sh

# Run different operations
docker exec -it azdo-container /scripts/azdo_json_merger.sh all
docker exec -it azdo-container /scripts/azdo_json_merger.sh builds
docker exec -it azdo-container /scripts/azdo_json_merger.sh csv
```
