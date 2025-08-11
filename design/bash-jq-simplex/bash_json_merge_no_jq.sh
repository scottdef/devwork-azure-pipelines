#!/bin/bash

# =============================================================================
# JSON FILE MERGING USING ONLY BASH AND STANDARD UNIX TOOLS
# No jq or additional packages required
# =============================================================================

# -----------------------------------------------------------------------------
# SCENARIO 1: Merge JSON files containing arrays
# Example: [{"id":1}] + [{"id":2}] = [{"id":1},{"id":2}]
# -----------------------------------------------------------------------------

echo "=== Array Concatenation Methods ==="

# Method 1: Simple array merge using sed
merge_json_arrays_sed() {
    local output_file="${1:-merged.json}"
    shift
    local input_files=("$@")
    
    if [[ ${#input_files[@]} -eq 0 ]]; then
        echo "Usage: merge_json_arrays_sed output.json file1.json file2.json ..."
        return 1
    fi
    
    # Start with first file (remove trailing ])
    sed 's/]$//' "${input_files[0]}" > "$output_file"
    
    # Process remaining files
    for ((i=1; i<${#input_files[@]}; i++)); do
        file="${input_files[$i]}"
        if [[ -f "$file" ]]; then
            # Add comma, then content without [ and ]
            echo "," >> "$output_file"
            sed '1s/^\[//; $s/\]$//' "$file" >> "$output_file"
        fi
    done
    
    # Close the array
    echo "]" >> "$output_file"
    
    echo "Merged ${#input_files[@]} files into $output_file"
}

# Usage: merge_json_arrays_sed merged.json file1.json file2.json file3.json

# Method 2: Array merge using awk
merge_json_arrays_awk() {
    local output_file="${1:-merged.json}"
    shift
    local input_files=("$@")
    
    awk '
    BEGIN { 
        print "["
        first_file = 1
    }
    FNR == 1 && NR > 1 { 
        if (!first_file) print ","
        first_file = 0
    }
    /^\[/ { 
        if (FNR == 1) next  # Skip opening bracket
    }
    /^\]$/ && FNR == NF { 
        next  # Skip closing bracket if its the last line
    }
    /^\]$/ {
        next  # Skip standalone closing brackets
    }
    { 
        # Remove trailing comma from last element of each file
        line = $0
        if (line ~ /,$/ && NR == total_lines[FILENAME]) {
            gsub(/,$/, "", line)
        }
        print line
    }
    END { 
        print "]"
    }
    ' "${input_files[@]}" > "$output_file"
    
    echo "Merged ${#input_files[@]} files into $output_file using awk"
}

# Method 3: Simple concatenation with text processing
merge_json_simple() {
    local output_file="${1:-merged.json}"
    
    # Create empty array
    echo "[" > "$output_file"
    
    local first=true
    for file in *.json; do
        if [[ -f "$file" && "$file" != "$output_file" ]]; then
            if [[ "$first" != true ]]; then
                echo "," >> "$output_file"
            fi
            
            # Extract content between [ and ]
            sed -n '1s/^\[//p; 2,$ {/^\]$/! p}' "$file" | sed '$s/,$//' >> "$output_file"
            first=false
        fi
    done
    
    echo "]" >> "$output_file"
    echo "Simple merge completed: $output_file"
}

# -----------------------------------------------------------------------------
# SCENARIO 2: Merge JSON files containing objects
# Example: {"data":[...]} + {"data":[...]} = {"merged":[...]}
# -----------------------------------------------------------------------------

echo "=== Object Merging Methods ==="

# Method 1: Extract and merge specific keys using grep and sed
merge_json_objects_grep() {
    local output_file="${1:-merged.json}"
    local key_name="${2:-data}"
    
    echo "{" > "$output_file"
    echo "  \"merged_$key_name\": [" >> "$output_file"
    
    local first=true
    for file in *.json; do
        if [[ -f "$file" && "$file" != "$output_file" ]]; then
            # Extract array content from specific key
            content=$(grep -A 1000 "\"$key_name\":" "$file" | \
                     sed -n '/\[/,/\]/p' | \
                     sed '1d; $d' | \
                     sed 's/^[[:space:]]*//')
            
            if [[ -n "$content" ]]; then
                if [[ "$first" != true ]]; then
                    echo "," >> "$output_file"
                fi
                echo "$content" >> "$output_file"
                first=false
            fi
        fi
    done
    
    echo "  ]" >> "$output_file"
    echo "}" >> "$output_file"
    
    echo "Object merge completed: $output_file"
}

# Method 2: Full object merge using awk
merge_objects_awk() {
    local output_file="${1:-merged.json}"
    
    awk '
    BEGIN {
        print "{"
        print "  \"merged_data\": ["
        first_array = 1
    }
    
    # Look for array start within objects
    /".*":\s*\[/ {
        in_array = 1
        if (!first_array) print ","
        first_array = 0
        next
    }
    
    # Process array content
    in_array && /^\s*\]/ {
        in_array = 0
        next
    }
    
    in_array {
        print "    " $0
    }
    
    END {
        print "  ]"
        print "}"
    }
    ' *.json > "$output_file"
    
    echo "Object merge with awk completed: $output_file"
}

# -----------------------------------------------------------------------------
# SCENARIO 3: Advanced merging with validation
# -----------------------------------------------------------------------------

echo "=== Advanced Merging with Validation ==="

# Function to validate JSON structure
validate_json_structure() {
    local file="$1"
    
    if [[ ! -f "$file" ]]; then
        echo "false"
        return
    fi
    
    # Basic JSON validation using bracket counting
    local open_brackets=$(grep -o '\[' "$file" | wc -l)
    local close_brackets=$(grep -o '\]' "$file" | wc -l)
    local open_braces=$(grep -o '{' "$file" | wc -l)
    local close_braces=$(grep -o '}' "$file" | wc -l)
    
    if [[ $open_brackets -eq $close_brackets && $open_braces -eq $close_braces ]]; then
        echo "true"
    else
        echo "false"
    fi
}

# Merge with validation
merge_with_validation() {
    local output_file="${1:-validated_merge.json}"
    local file_pattern="${2:-*.json}"
    
    echo "Validating JSON files matching pattern: $file_pattern"
    
    local valid_files=()
    local invalid_files=()
    
    for file in $file_pattern; do
        if [[ -f "$file" && "$file" != "$output_file" ]]; then
            if [[ "$(validate_json_structure "$file")" == "true" ]]; then
                valid_files+=("$file")
                echo "✓ Valid: $file"
            else
                invalid_files+=("$file")
                echo "✗ Invalid: $file"
            fi
        fi
    done
    
    if [[ ${#valid_files[@]} -eq 0 ]]; then
        echo "No valid JSON files found"
        return 1
    fi
    
    echo "Merging ${#valid_files[@]} valid files..."
    
    # Merge valid files
    echo "[" > "$output_file"
    local first=true
    
    for file in "${valid_files[@]}"; do
        if [[ "$first" != true ]]; then
            echo "," >> "$output_file"
        fi
        
        # Extract array content (assuming array-based JSON)
        sed '1d; $d' "$file" >> "$output_file"
        first=false
    done
    
    echo "]" >> "$output_file"
    
    echo "Validation merge completed: $output_file"
    echo "Valid files: ${#valid_files[@]}, Invalid files: ${#invalid_files[@]}"
}

# -----------------------------------------------------------------------------
# SCENARIO 4: Azure DevOps specific merging without jq
# -----------------------------------------------------------------------------

echo "=== Azure DevOps Specific Merging ==="

# Merge Azure DevOps builds using text processing
merge_azdo_builds() {
    local output_file="${1:-azdo_builds_merged.json}"
    
    echo "Merging Azure DevOps build files..."
    echo "[" > "$output_file"
    
    local first=true
    for file in builds_*.json; do
        if [[ -f "$file" ]]; then
            echo "Processing $file..."
            
            if [[ "$first" != true ]]; then
                echo "," >> "$output_file"
            fi
            
            # Process each build entry
            sed -n '/\[/,/\]/p' "$file" | \
            sed '1d; $d' | \
            while IFS= read -r line; do
                echo "  $line" >> "$output_file"
            done
            
            first=false
        fi
    done
    
    echo "]" >> "$output_file"
    echo "Azure DevOps builds merged: $output_file"
}

# Extract specific fields from Azure DevOps JSON (without jq)
extract_azdo_fields() {
    local input_file="$1"
    local output_file="${2:-extracted_fields.json}"
    
    echo "Extracting key fields from $input_file..."
    echo "[" > "$output_file"
    
    # Use awk to extract specific fields
    awk '
    /"id":/ { 
        id = $0
        gsub(/.*"id":[[:space:]]*/, "", id)
        gsub(/[,}].*/, "", id)
    }
    /"buildNumber":/ {
        buildNumber = $0
        gsub(/.*"buildNumber":[[:space:]]*"/, "", buildNumber)  
        gsub(/"[,}].*/, "", buildNumber)
    }
    /"status":/ {
        status = $0
        gsub(/.*"status":[[:space:]]*"/, "", status)
        gsub(/"[,}].*/, "", status)
    }
    /"result":/ {
        result = $0
        gsub(/.*"result":[[:space:]]*"/, "", result)
        gsub(/"[,}].*/, "", result)
        
        # Print extracted object
        if (id && buildNumber && status) {
            if (NR > 1) print ","
            printf "  {\n"
            printf "    \"id\": %s,\n", id
            printf "    \"buildNumber\": \"%s\",\n", buildNumber
            printf "    \"status\": \"%s\",\n", status
            printf "    \"result\": \"%s\"\n", result
            printf "  }"
            
            # Reset variables
            id = ""; buildNumber = ""; status = ""; result = ""
        }
    }
    END {
        print "\n]"
    }
    ' "$input_file" >> "$output_file"
    
    echo "Field extraction completed: $output_file"
}

# -----------------------------------------------------------------------------
# SCENARIO 5: Batch processing functions
# -----------------------------------------------------------------------------

echo "=== Batch Processing Functions ==="

# Process files in chunks to avoid command line limits
batch_merge() {
    local batch_size="${1:-10}"
    local output_file="${2:-batch_merged.json}"
    local file_pattern="${3:-*.json}"
    
    echo "Processing files in batches of $batch_size..."
    
    local files=($(ls $file_pattern 2>/dev/null | head -100))  # Limit to 100 files
    local batch_count=0
    local temp_dir="/tmp/batch_merge_$$"
    
    mkdir -p "$temp_dir"
    
    # Process files in batches
    for ((i=0; i<${#files[@]}; i+=batch_size)); do
        batch_files=("${files[@]:$i:$batch_size}")
        local batch_output="$temp_dir/batch_$batch_count.json"
        
        echo "Processing batch $batch_count with ${#batch_files[@]} files..."
        merge_json_arrays_sed "$batch_output" "${batch_files[@]}"
        ((batch_count++))
    done
    
    # Merge all batches
    if [[ $batch_count -gt 1 ]]; then
        echo "Merging $batch_count batches..."
        merge_json_arrays_sed "$output_file" "$temp_dir"/batch_*.json
    else
        mv "$temp_dir/batch_0.json" "$output_file"
    fi
    
    # Cleanup
    rm -rf "$temp_dir"
    
    echo "Batch processing completed: $output_file"
    echo "Processed ${#files[@]} files in $batch_count batches"
}

# -----------------------------------------------------------------------------
# SCENARIO 6: One-liner commands for common patterns
# -----------------------------------------------------------------------------

echo "=== Quick One-Liner Commands ==="

# Simple array merge one-liner
alias quick_merge='{ echo "["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; sed "1d; \$d" "$f"; first=false; done; echo "]"; } > merged.json'

# Merge with file info
alias merge_with_info='{ echo "{\"merged_at\":\"$(date)\",\"files\":["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; echo "\"$f\""; first=false; done; echo "],\"data\":["; first=true; for f in *.json; do [[ "$first" != "true" ]] && echo ","; sed "1d; \$d" "$f"; first=false; done; echo "]}" } > merged_with_info.json'

# -----------------------------------------------------------------------------
# SCENARIO 7: Streaming merge for large files
# -----------------------------------------------------------------------------

echo "=== Streaming Merge for Large Files ==="

stream_merge_large() {
    local output_file="${1:-streamed.json}"
    
    echo "Starting streaming merge..."
    exec 3>"$output_file"  # Open file descriptor
    
    echo "[" >&3
    
    local first=true
    for file in *.json; do
        if [[ -f "$file" && "$file" != "$output_file" ]]; then
            echo "Processing $file..."
            
            if [[ "$first" != true ]]; then
                echo "," >&3
            fi
            
            # Stream content without loading entire file in memory
            sed '1d; $d' "$file" >&3
            first=false
        fi
    done
    
    echo "]" >&3
    exec 3>&-  # Close file descriptor
    
    echo "Streaming merge completed: $output_file"
}

# -----------------------------------------------------------------------------
# SCENARIO 8: Main execution function
# -----------------------------------------------------------------------------

# Main function to demonstrate usage
demo_merge_operations() {
    echo "=== JSON Merge Operations Demo ==="
    
    # Create sample data if none exists
    if [[ ! -f "sample1.json" ]]; then
        echo '[{"id":1,"name":"test1"}]' > sample1.json
        echo '[{"id":2,"name":"test2"}]' > sample2.json
        echo '[{"id":3,"name":"test3"}]' > sample3.json
        echo "Created sample JSON files for testing"
    fi
    
    echo ""
    echo "Available merge functions:"
    echo "1. merge_json_arrays_sed output.json file1.json file2.json ..."
    echo "2. merge_json_arrays_awk output.json file1.json file2.json ..."
    echo "3. merge_json_simple output.json"
    echo "4. merge_with_validation output.json 'pattern*.json'"
    echo "5. batch_merge 10 output.json '*.json'"
    echo "6. stream_merge_large output.json"
    echo ""
    echo "Example usage:"
    echo "merge_json_arrays_sed merged_output.json sample1.json sample2.json sample3.json"
    echo ""
    echo "Quick test:"
    merge_json_arrays_sed test_output.json sample1.json sample2.json sample3.json
    echo "Test merge result:"
    cat test_output.json
}

# Execute demo if script is run directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    demo_merge_operations
fi

echo ""
echo "JSON merge functions loaded!"
echo "Run 'demo_merge_operations' to see examples"