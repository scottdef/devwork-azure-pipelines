#!/bin/bash

# =============================================================================
# SIMPLE JSON MERGE ONE-LINERS - NO JQ REQUIRED
# Copy and paste these commands directly
# =============================================================================

# -----------------------------------------------------------------------------
# MOST COMMON: Merge JSON arrays into single array
# Input:  [{"id":1}] + [{"id":2}] 
# Output: [{"id":1},{"id":2}]
# -----------------------------------------------------------------------------

# Method 1: Using sed (recommended for most cases)
{ echo "["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > merged.json

# Method 2: More readable version
echo "[" > merged.json
first=true
for file in *.json; do
    if [[ "$first" != "true" ]]; then
        echo "," >> merged.json
    fi
    sed '1d; $d' "$file" >> merged.json
    first=false
done
echo "]" >> merged.json

# Method 3: Specific file pattern (e.g., builds_*.json)
{ echo "["; first=true; for f in builds_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > merged_builds.json

# -----------------------------------------------------------------------------
# WITH VALIDATION: Only merge valid files
# -----------------------------------------------------------------------------

# Check file exists and has matching brackets before merging
{ 
    echo "["
    first=true
    for f in *.json; do 
        if [[ -f "$f" ]] && [[ $(grep -c '\[' "$f") -eq $(grep -c '\]' "$f") ]]; then
            [[ "$first" != "true" ]] && echo ","
            sed '1d; $d' "$f"
            first=false
        fi
    done
    echo "]"
} > validated_merge.json

# -----------------------------------------------------------------------------
# ADD METADATA: Include merge info
# -----------------------------------------------------------------------------

# Merge with timestamp and file count
{
    echo "{"
    echo "  \"merged_at\": \"$(date -Iseconds)\","
    echo "  \"file_count\": $(ls *.json | wc -l),"
    echo "  \"data\": ["
    first=true
    for f in *.json; do
        [[ "$first" != "true" ]] && echo ","
        sed '1d; $d' "$f"
        first=false
    done
    echo "  ]"
    echo "}"
} > merged_with_metadata.json

# -----------------------------------------------------------------------------
# AZURE DEVOPS SPECIFIC: Process builds/releases
# -----------------------------------------------------------------------------

# Merge Azure DevOps builds
{ echo "["; first=true; for f in builds_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > all_builds.json

# Merge Azure DevOps releases  
{ echo "["; first=true; for f in releases_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > all_releases.json

# Merge deployments
{ echo "["; first=true; for f in deployments_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > all_deployments.json

# -----------------------------------------------------------------------------
# EXTRACT DATA FROM NESTED OBJECTS: Get arrays from objects
# -----------------------------------------------------------------------------

# Extract "data" array from objects like {"data": [...]}
{
    echo "["
    first=true
    for f in *.json; do
        # Extract content between "data":[ and ]
        content=$(sed -n '/"data":\[/,/\]/p' "$f" | sed '1s/.*\[//; $s/\].*//' | grep -v '^[[:space:]]*$')
        if [[ -n "$content" ]]; then
            [[ "$first" != "true" ]] && echo ","
            echo "$content"
            first=false
        fi
    done
    echo "]"
} > extracted_data.json

# -----------------------------------------------------------------------------
# BATCH PROCESSING: Handle large numbers of files
# -----------------------------------------------------------------------------

# Process files in groups of 10 to avoid command line limits
process_in_batches() {
    local batch_size=10
    local batch_num=0
    local files=(*.json)
    
    for ((i=0; i<${#files[@]}; i+=batch_size)); do
        batch_files=("${files[@]:$i:$batch_size}")
        { 
            echo "["
            first=true
            for f in "${batch_files[@]}"; do
                [[ "$first" != "true" ]] && echo ","
                sed '1d; $d' "$f"
                first=false
            done
            echo "]"
        } > "batch_${batch_num}.json"
        ((batch_num++))
    done
    
    # Merge all batches
    { 
        echo "["
        first=true
        for f in batch_*.json; do
            [[ "$first" != "true" ]] && echo ","
            sed '1d; $d' "$f"
            first=false
        done
        echo "]"
    } > final_merged.json
    
    # Cleanup batch files
    rm batch_*.json
}

# Call the function
# process_in_batches

# -----------------------------------------------------------------------------
# ERROR HANDLING: Check if merge was successful
# -----------------------------------------------------------------------------

# Merge with error checking
merge_with_check() {
    local output="merged_checked.json"
    local file_count=$(ls *.json 2>/dev/null | wc -l)
    
    if [[ $file_count -eq 0 ]]; then
        echo "Error: No JSON files found"
        return 1
    fi
    
    echo "Merging $file_count JSON files..."
    
    { 
        echo "["
        first=true
        for f in *.json; do
            if [[ -f "$f" ]]; then
                [[ "$first" != "true" ]] && echo ","
                sed '1d; $d' "$f"
                first=false
                echo "Processed: $f" >&2
            fi
        done
        echo "]"
    } > "$output" 2>merge.log
    
    # Verify output
    if [[ -f "$output" ]] && [[ $(wc -l < "$output") -gt 2 ]]; then
        echo "✅ Success: Merged $file_count files into $output"
        echo "📊 Output size: $(wc -l < "$output") lines"
    else
        echo "❌ Error: Merge failed"
        cat merge.log
        return 1
    fi
}

# -----------------------------------------------------------------------------
# QUICK ALIASES for common operations
# -----------------------------------------------------------------------------

# Create convenient aliases
alias merge_all='{ echo "["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; sed "1d; \$d" "$f"; first=false; done; echo "]"; } > merged_all.json'

alias merge_builds='{ echo "["; first=true; for f in builds_*.json; do [[ "$first" != "true" ]] && echo ","; sed "1d; \$d" "$f"; first=false; done; echo "]"; } > merged_builds.json'

alias merge_releases='{ echo "["; first=true; for f in releases_*.json; do [[ "$first" != "true" ]] && echo ","; sed "1d; \$d" "$f"; first=false; done; echo "]"; } > merged_releases.json'

# -----------------------------------------------------------------------------
# PRACTICAL EXAMPLES for Azure DevOps container
# -----------------------------------------------------------------------------

echo "=== COPY-PASTE READY COMMANDS ==="

cat << 'EXAMPLES'

# 1. BASIC MERGE - All JSON files in current directory
{ echo "["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > merged.json

# 2. MERGE SPECIFIC PATTERN - Only build files
{ echo "["; first=true; for f in builds_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > all_builds.json

# 3. MERGE WITH VALIDATION - Skip invalid files
{ echo "["; first=true; for f in *.json; do if [[ -f "$f" ]] && [[ $(grep -c '\[' "$f") -eq $(grep -c '\]' "$f") ]]; then [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; fi; done; echo "]"; } > validated.json

# 4. MERGE WITH METADATA - Include timestamp and count
{ echo "{\"merged_at\":\"$(date -Iseconds)\",\"file_count\":$(ls *.json | wc -l),\"data\":["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]}"; } > merge_meta.json

# 5. AZURE DEVOPS - Merge builds, releases, deployments
{ echo "["; first=true; for f in builds_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > all_builds.json
{ echo "["; first=true; for f in releases_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > all_releases.json

# 6. CHECK RESULT - Verify merge worked
if [[ -f "merged.json" ]] && [[ $(wc -l < "merged.json") -gt 2 ]]; then echo "✅ Merge successful"; else echo "❌ Merge failed"; fi

EXAMPLES

echo "=== USAGE IN AZURE DEVOPS CONTAINER ==="

cat << 'CONTAINER_USAGE'

# In your Azure DevOps container:

# 1. Navigate to data directory
cd /data

# 2. Merge all build files
{ echo "["; first=true; for f in builds_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > /output/merged_builds.json

# 3. Merge all release files  
{ echo "["; first=true; for f in releases_*.json; do [[ "$first" != "true" ]] && echo ","; sed '1d; $d' "$f"; first=false; done; echo "]"; } > /output/merged_releases.json

# 4. Check results
echo "Build records: $(grep -o '{' /output/merged_builds.json | wc -l)"
echo "Release records: $(grep -o '{' /output/merged_releases.json | wc -l)"

# 5. Create summary report
{
    echo "{"
    echo "  \"report_date\": \"$(date -Iseconds)\","
    echo "  \"builds_count\": $(grep -o '{' /output/merged_builds.json | wc -l),"
    echo "  \"releases_count\": $(grep -o '{' /output/merged_releases.json | wc -l),"
    echo "  \"source_files\": $(ls *.json | wc -l)"
    echo "}"
} > /output/summary.json

CONTAINER_USAGE