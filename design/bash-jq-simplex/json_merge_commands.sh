#!/bin/bash

# =============================================================================
# BASH COMMANDS FOR MERGING MULTIPLE JSON FILES
# =============================================================================

# -----------------------------------------------------------------------------
# SCENARIO 1: JSON files contain arrays at root level
# Example: [{"id": 1}, {"id": 2}] + [{"id": 3}, {"id": 4}] = [{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}]
# -----------------------------------------------------------------------------

echo "=== Array Concatenation Methods ==="

# Method 1: Simple array concatenation using jq
jq -s 'add' file1.json file2.json file3.json > merged.json

# Method 2: Concatenate all JSON files in directory
jq -s 'add' *.json > merged_all.json

# Method 3: Using wildcard with specific pattern
jq -s 'add' builds_*.json > merged_builds.json

# Method 4: Pipe multiple files
cat file1.json file2.json file3.json | jq -s 'add' > merged.json

# Method 5: With error handling
{
    jq -s 'add' *.json > merged.json 2>/dev/null && 
    echo "Successfully merged $(ls *.json | wc -l) files" ||
    echo "Error: Failed to merge JSON files"
}

# -----------------------------------------------------------------------------
# SCENARIO 2: JSON files contain objects at root level
# Example: {"data": []} + {"data": []} = {"merged_data": [...]}
# -----------------------------------------------------------------------------

echo "=== Object Merging Methods ==="

# Method 1: Merge objects by combining arrays from specific key
jq -s '[.[] | .data] | add | {"merged_data": .}' file1.json file2.json > merged.json

# Method 2: Merge multiple object keys
jq -s '
    {
        "builds": [.[] | .builds // []] | add,
        "releases": [.[] | .releases // []] | add,
        "deployments": [.[] | .deployments // []] | add
    }
' *.json > merged.json

# Method 3: Generic object merge (shallow merge)
jq -s 'add' file1.json file2.json > merged.json

# -----------------------------------------------------------------------------
# SCENARIO 3: Create a new structure with metadata
# -----------------------------------------------------------------------------

echo "=== Structured Merge with Metadata ==="

# Method 1: Create wrapper with source file info
jq -n '
    {
        "merged_at": now | strftime("%Y-%m-%d %H:%M:%S"),
        "source_files": $ARGS.positional,
        "data": [inputs | arrays | .[]]
    }
' --args builds_*.json > merged_with_metadata.json

# Method 2: Include file count and timestamp
{
    FILE_COUNT=$(ls *.json | wc -l)
    jq -s --argjson count "$FILE_COUNT" '
        {
            "metadata": {
                "merged_at": now | strftime("%Y-%m-%d %H:%M:%S"),
                "file_count": $count,
                "total_records": [.[] | length] | add
            },
            "data": add
        }
    ' *.json > merged_with_stats.json
}

# -----------------------------------------------------------------------------
# SCENARIO 4: Advanced merging with filtering and transformation
# -----------------------------------------------------------------------------

echo "=== Advanced Merging Operations ==="

# Method 1: Merge with deduplication by ID
jq -s 'add | unique_by(.id)' file1.json file2.json > merged_unique.json

# Method 2: Merge and sort by timestamp
jq -s 'add | sort_by(.timestamp)' *.json > merged_sorted.json

# Method 3: Merge with filtering
jq -s 'add | map(select(.status == "completed"))' *.json > merged_completed.json

# Method 4: Merge Azure DevOps builds with specific fields
jq -s '
    add | map({
        id: .id,
        buildNumber: .buildNumber,
        status: .status,
        result: .result,
        queueTime: .queueTime,
        finishTime: .finishTime,
        definition: .definition.name,
        duration: (
            if .finishTime and .queueTime then
                (((.finishTime | fromdateiso8601) - (.queueTime | fromdateiso8601)) / 60 | floor)
            else null end
        )
    })
' builds_*.json > processed_builds.json

# -----------------------------------------------------------------------------
# SCENARIO 5: Batch processing with error handling
# -----------------------------------------------------------------------------

echo "=== Batch Processing Scripts ==="

# Method 1: Process files in batches to avoid command line length limits
process_json_batch() {
    local batch_size=${1:-10}
    local output_file=${2:-merged_output.json}
    local pattern=${3:-"*.json"}
    
    echo "Processing JSON files in batches of $batch_size..."
    
    # Create temporary directory
    temp_dir=$(mktemp -d)
    batch_count=0
    
    # Split files into batches
    files=($(ls $pattern))
    for ((i=0; i<${#files[@]}; i+=batch_size)); do
        batch_files=("${files[@]:$i:$batch_size}")
        jq -s 'add' "${batch_files[@]}" > "$temp_dir/batch_$batch_count.json"
        ((batch_count++))
    done
    
    # Merge all batches
    jq -s 'add' "$temp_dir"/batch_*.json > "$output_file"
    
    # Cleanup
    rm -rf "$temp_dir"
    
    echo "Merged $(ls $pattern | wc -l) files into $output_file"
}

# Usage: process_json_batch 5 merged_builds.json "builds_*.json"

# Method 2: Robust merge with validation
merge_with_validation() {
    local output_file=${1:-merged.json}
    shift
    local input_files=("$@")
    
    echo "Validating and merging ${#input_files[@]} JSON files..."
    
    # Validate each file first
    valid_files=()
    for file in "${input_files[@]}"; do
        if [[ -f "$file" ]] && jq empty "$file" 2>/dev/null; then
            valid_files+=("$file")
            echo "✓ Valid: $file"
        else
            echo "✗ Invalid or missing: $file"
        fi
    done
    
    if [[ ${#valid_files[@]} -eq 0 ]]; then
        echo "Error: No valid JSON files found"
        return 1
    fi
    
    # Merge valid files
    jq -s 'add' "${valid_files[@]}" > "$output_file"
    echo "Successfully merged ${#valid_files[@]} files into $output_file"
    
    # Display summary
    echo "Summary:"
    echo "- Total records: $(jq length "$output_file")"
    echo "- File size: $(ls -lh "$output_file" | awk '{print $5}')"
}

# Usage: merge_with_validation merged_output.json file1.json file2.json file3.json

# -----------------------------------------------------------------------------
# SCENARIO 6: One-liner commands for common patterns
# -----------------------------------------------------------------------------

echo "=== Quick One-Liner Commands ==="

# Merge all JSON arrays in current directory
alias merge_arrays='jq -s "add" *.json > merged.json'

# Merge Azure DevOps builds from multiple files
alias merge_builds='jq -s "add | sort_by(.queueTime)" builds_*.json > all_builds.json'

# Merge and create summary
alias merge_summary='jq -s "{total: length, data: add}" *.json > merged_with_count.json'

# Merge with timestamp
alias merge_timestamped='jq -s "{merged_at: now|strftime(\"%Y-%m-%d %H:%M:%S\"), data: add}" *.json > timestamped_merge.json'

# -----------------------------------------------------------------------------
# SCENARIO 7: Streaming merge for large files
# -----------------------------------------------------------------------------

echo "=== Streaming Merge for Large Files ==="

# Method 1: Stream processing to handle large datasets
stream_merge() {
    local output_file=${1:-streamed_merge.json}
    
    echo "Starting streaming merge..."
    echo "[" > "$output_file"
    
    first_file=true
    for file in *.json; do
        if [[ -f "$file" ]]; then
            if [[ "$first_file" == true ]]; then
                # First file: remove closing bracket
                jq -r '.[] | tostring' "$file" >> "$output_file"
                first_file=false
            else
                # Subsequent files: add comma and content
                echo "," >> "$output_file"
                jq -r '.[] | tostring' "$file" >> "$output_file"
            fi
        fi
    done
    
    echo "]" >> "$output_file"
    echo "Streaming merge completed: $output_file"
}

# -----------------------------------------------------------------------------
# SCENARIO 8: Azure DevOps specific merging
# -----------------------------------------------------------------------------

echo "=== Azure DevOps Specific Merging ==="

# Merge builds with pipeline information
merge_azdo_builds() {
    jq -s '
        add | map({
            build_id: .id,
            build_number: .buildNumber,
            pipeline: .definition.name,
            status: .status,
            result: .result,
            branch: .sourceBranch // "unknown",
            queue_time: .queueTime,
            start_time: .startTime,
            finish_time: .finishTime,
            duration_minutes: (
                if .finishTime and .startTime then
                    (((.finishTime | fromdateiso8601) - (.startTime | fromdateiso8601)) / 60 | floor)
                else null end
            ),
            triggered_by: .requestedFor.displayName // "System"
        }) | sort_by(.queue_time)
    ' builds_*.json > processed_builds.json
}

# Merge releases with deployment information
merge_azdo_releases() {
    jq -s '
        add | map({
            release_id: .id,
            release_name: .name,
            pipeline: .releaseDefinition.name,
            status: .status,
            created_on: .createdOn,
            environments: [.environments[] | {
                name: .name,
                status: .status,
                deployment_status: (
                    if (.deploySteps | length) > 0 then
                        .deploySteps[-1].status
                    else "not-deployed" end
                )
            }]
        }) | sort_by(.created_on)
    ' releases_*.json > processed_releases.json
}

echo "JSON merge commands ready!"
echo ""
echo "Usage Examples:"
echo "1. Basic array merge: jq -s 'add' *.json > merged.json"
echo "2. With validation: merge_with_validation output.json file1.json file2.json"
echo "3. Azure DevOps builds: merge_azdo_builds"
echo "4. Batch processing: process_json_batch 10 output.json 'data_*.json'"