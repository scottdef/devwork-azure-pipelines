#!/bin/bash
# =============================================================================
# Azure DevOps JSON Data Merger Script
# Specifically designed for merging Azure DevOps API extraction results
# =============================================================================

set -e  # Exit on any error

# Configuration
DATA_DIR="/data"
OUTPUT_DIR="/output"
TEMP_DIR="/tmp/azdo_merge"
LOG_FILE="/data/merge.log"

# Ensure directories exist
mkdir -p "$OUTPUT_DIR" "$TEMP_DIR"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# Function to validate JSON files
validate_json() {
    local file="$1"
    if [[ ! -f "$file" ]]; then
        echo "false"
        return
    fi
    
    if jq empty "$file" 2>/dev/null; then
        echo "true"
    else
        echo "false"
    fi
}

# Function to merge Azure DevOps builds
merge_builds() {
    log "Starting Azure DevOps builds merge..."
    
    local build_files=("$DATA_DIR"/builds_*.json)
    local valid_files=()
    
    # Validate build files
    for file in "${build_files[@]}"; do
        if [[ $(validate_json "$file") == "true" ]]; then
            valid_files+=("$file")
            log "✓ Valid build file: $(basename "$file")"
        else
            log "✗ Invalid build file: $(basename "$file")"
        fi
    done
    
    if [[ ${#valid_files[@]} -eq 0 ]]; then
        log "No valid build files found"
        return 1
    fi
    
    # Merge and process builds
    jq -s '
        add | 
        map({
            build_id: .id,
            build_number: .buildNumber,
            definition_name: .definition.name,
            definition_id: .definition.id,
            source_branch: .sourceBranch // "unknown",
            source_version: .sourceVersion // "unknown",
            status: .status,
            result: .result,
            queue_time: .queueTime,
            start_time: .startTime,
            finish_time: .finishTime,
            duration_seconds: (
                if .finishTime and .startTime then
                    (.finishTime | fromdateiso8601) - (.startTime | fromdateiso8601)
                else null end
            ),
            duration_minutes: (
                if .finishTime and .startTime then
                    (((.finishTime | fromdateiso8601) - (.startTime | fromdateiso8601)) / 60 | floor)
                else null end
            ),
            requested_by: .requestedBy.displayName // "System",
            requested_for: .requestedFor.displayName // "System",
            repository: .repository.name // "unknown",
            reason: .reason // "unknown",
            tags: (.tags // []),
            validation_results: [.validationResults[]? | {
                result: .result,
                message: .message
            }],
            demands: (.demands // [])
        }) | 
        sort_by(.queue_time)
    ' "${valid_files[@]}" > "$OUTPUT_DIR/merged_builds.json"
    
    # Create summary
    local total_builds=$(jq length "$OUTPUT_DIR/merged_builds.json")
    local successful_builds=$(jq '[.[] | select(.result == "succeeded")] | length' "$OUTPUT_DIR/merged_builds.json")
    local failed_builds=$(jq '[.[] | select(.result == "failed")] | length' "$OUTPUT_DIR/merged_builds.json")
    
    jq -n \
        --argjson total "$total_builds" \
        --argjson successful "$successful_builds" \
        --argjson failed "$failed_builds" \
        --arg timestamp "$(date -Iseconds)" \
        '{
            merged_at: $timestamp,
            source_files: $ARGS.positional | length,
            statistics: {
                total_builds: $total,
                successful_builds: $successful,
                failed_builds: $failed,
                success_rate: (if $total > 0 then ($successful / $total * 100 | round) else 0 end)
            }
        }' \
        --args "${valid_files[@]}" > "$OUTPUT_DIR/builds_summary.json"
    
    log "✅ Merged $total_builds builds from ${#valid_files[@]} files"
    log "   Success rate: $(jq -r '.statistics.success_rate' "$OUTPUT_DIR/builds_summary.json")%"
}

# Function to merge Azure DevOps releases
merge_releases() {
    log "Starting Azure DevOps releases merge..."
    
    local release_files=("$DATA_DIR"/releases_*.json)
    local valid_files=()
    
    # Validate release files
    for file in "${release_files[@]}"; do
        if [[ $(validate_json "$file") == "true" ]]; then
            valid_files+=("$file")
            log "✓ Valid release file: $(basename "$file")"
        else
            log "✗ Invalid release file: $(basename "$file")"
        fi
    done
    
    if [[ ${#valid_files[@]} -eq 0 ]]; then
        log "No valid release files found"
        return 1
    fi
    
    # Merge and process releases
    jq -s '
        add | 
        map({
            release_id: .id,
            release_name: .name,
            description: .description // "",
            definition_name: .releaseDefinition.name,
            definition_id: .releaseDefinition.id,
            status: .status,
            created_on: .createdOn,
            created_by: .createdBy.displayName // "System",
            modified_on: .modifiedOn,
            modified_by: .modifiedBy.displayName // "System",
            environments: [.environments[] | {
                id: .id,
                name: .name,
                status: .status,
                rank: .rank,
                deployment_status: (
                    if (.deploySteps | length) > 0 then
                        .deploySteps[-1].status
                    else "not-deployed" 
                end),
                last_deployment: (
                    if (.deploySteps | length) > 0 then {
                        started_on: .deploySteps[-1].startedOn,
                        completed_on: .deploySteps[-1].completedOn,
                        deployed_by: .deploySteps[-1].requestedBy.displayName
                    }
                    else null
                    end
                ),
                pre_deployment_approvals: [.preDeployApprovals[]? | {
                    status: .status,
                    approver: .approver.displayName,
                    approved_by: .approvedBy.displayName // null,
                    created_on: .createdOn
                }],
                post_deployment_approvals: [.postDeployApprovals[]? | {
                    status: .status,
                    approver: .approver.displayName,
                    approved_by: .approvedBy.displayName // null,
                    created_on: .createdOn
                }]
            }],
            artifacts: [.artifacts[] | {
                alias: .alias,
                type: .type,
                definition_reference: .definitionReference
            }],
            variables: (.variables // {}),
            tags: (.tags // [])
        }) | 
        sort_by(.created_on)
    ' "${valid_files[@]}" > "$OUTPUT_DIR/merged_releases.json"
    
    # Create summary
    local total_releases=$(jq length "$OUTPUT_DIR/merged_releases.json")
    local active_releases=$(jq '[.[] | select(.status == "active")] | length' "$OUTPUT_DIR/merged_releases.json")
    local abandoned_releases=$(jq '[.[] | select(.status == "abandoned")] | length' "$OUTPUT_DIR/merged_releases.json")
    
    jq -n \
        --argjson total "$total_releases" \
        --argjson active "$active_releases" \
        --argjson abandoned "$abandoned_releases" \
        --arg timestamp "$(date -Iseconds)" \
        '{
            merged_at: $timestamp,
            source_files: $ARGS.positional | length,
            statistics: {
                total_releases: $total,
                active_releases: $active,
                abandoned_releases: $abandoned,
                environments_summary: {}
            }
        }' \
        --args "${valid_files[@]}" > "$OUTPUT_DIR/releases_summary.json"
    
    log "✅ Merged $total_releases releases from ${#valid_files[@]} files"
}

# Function to merge deployment data
merge_deployments() {
    log "Starting deployment data merge..."
    
    local deployment_files=("$DATA_DIR"/deployments_*.json)
    local valid_files=()
    
    # Validate deployment files
    for file in "${deployment_files[@]}"; do
        if [[ $(validate_json "$file") == "true" ]]; then
            valid_files+=("$file")
            log "✓ Valid deployment file: $(basename "$file")"
        else
            log "✗ Invalid deployment file: $(basename "$file")"
        fi
    done
    
    if [[ ${#valid_files[@]} -eq 0 ]]; then
        log "No valid deployment files found"
        return 1
    fi
    
    # Merge deployments
    jq -s 'add | sort_by(.deploymentStart)' "${valid_files[@]}" > "$OUTPUT_DIR/merged_deployments.json"
    
    local total_deployments=$(jq length "$OUTPUT_DIR/merged_deployments.json")
    log "✅ Merged $total_deployments deployments from ${#valid_files[@]} files"
}

# Function to create comprehensive report
create_comprehensive_report() {
    log "Creating comprehensive Azure DevOps report..."
    
    # Check if merged files exist
    local builds_exist=false
    local releases_exist=false
    local deployments_exist=false
    
    [[ -f "$OUTPUT_DIR/merged_builds.json" ]] && builds_exist=true
    [[ -f "$OUTPUT_DIR/merged_releases.json" ]] && releases_exist=true
    [[ -f "$OUTPUT_DIR/merged_deployments.json" ]] && deployments_exist=true
    
    # Create comprehensive report
    jq -n \
        --argjson builds_exist "$builds_exist" \
        --argjson releases_exist "$releases_exist" \
        --argjson deployments_exist "$deployments_exist" \
        --arg timestamp "$(date -Iseconds)" \
        '{
            report_generated: $timestamp,
            data_sources: {
                builds: $builds_exist,
                releases: $releases_exist,
                deployments: $deployments_exist
            },
            summary: {}
        }' > "$TEMP_DIR/report_base.json"
    
    # Add builds data if available
    if [[ "$builds_exist" == "true" ]]; then
        jq --slurpfile builds "$OUTPUT_DIR/merged_builds.json" \
           '.summary.builds = {
               total_count: ($builds | length),
               by_status: ($builds | group_by(.status) | map({status: .[0].status, count: length}) | from_entries),
               by_result: ($builds | group_by(.result) | map({result: .[0].result, count: length}) | from_entries),
               success_rate: (($builds | map(select(.result == "succeeded")) | length) / ($builds | length) * 100 | round),
               average_duration_minutes: (($builds | map(select(.duration_minutes != null) | .duration_minutes) | add) / ($builds | map(select(.duration_minutes != null)) | length) | round)
           }' "$TEMP_DIR/report_base.json" > "$TEMP_DIR/report_with_builds.json"
        mv "$TEMP_DIR/report_with_builds.json" "$TEMP_DIR/report_base.json"
    fi
    
    # Add releases data if available
    if [[ "$releases_exist" == "true" ]]; then
        jq --slurpfile releases "$OUTPUT_DIR/merged_releases.json" \
           '.summary.releases = {
               total_count: ($releases | length),
               by_status: ($releases | group_by(.status) | map({status: .[0].status, count: length}) | from_entries),
               environments_count: ($releases | map(.environments | length) | add),
               average_environments_per_release: (($releases | map(.environments | length) | add) / ($releases | length) | round)
           }' "$TEMP_DIR/report_base.json" > "$TEMP_DIR/report_with_releases.json"
        mv "$TEMP_DIR/report_with_releases.json" "$TEMP_DIR/report_base.json"
    fi
    
    mv "$TEMP_DIR/report_base.json" "$OUTPUT_DIR/comprehensive_report.json"
    log "✅ Comprehensive report created"
}

# Function to export to CSV format
export_to_csv() {
    log "Exporting data to CSV format..."
    
    # Export builds to CSV
    if [[ -f "$OUTPUT_DIR/merged_builds.json" ]]; then
        jq -r '
            ["Build ID", "Build Number", "Pipeline", "Status", "Result", "Branch", "Queue Time", "Duration (min)", "Requested By"] as $header |
            $header,
            (.[] | [
                .build_id,
                .build_number,
                .definition_name,
                .status,
                .result,
                .source_branch,
                .queue_time,
                .duration_minutes,
                .requested_by
            ]) |
            @csv
        ' "$OUTPUT_DIR/merged_builds.json" > "$OUTPUT_DIR/builds.csv"
        log "✅ Builds exported to builds.csv"
    fi
    
    # Export releases to CSV
    if [[ -f "$OUTPUT_DIR/merged_releases.json" ]]; then
        jq -r '
            ["Release ID", "Release Name", "Pipeline", "Status", "Created On", "Created By", "Environment Count"] as $header |
            $header,
            (.[] | [
                .release_id,
                .release_name,
                .definition_name,
                .status,
                .created_on,
                .created_by,
                (.environments | length)
            ]) |
            @csv
        ' "$OUTPUT_DIR/merged_releases.json" > "$OUTPUT_DIR/releases.csv"
        log "✅ Releases exported to releases.csv"
    fi
}

# Main execution function
main() {
    local operation="${1:-all}"
    
    log "Starting Azure DevOps JSON merger - Operation: $operation"
    
    case "$operation" in
        "builds")
            merge_builds
            ;;
        "releases")
            merge_releases
            ;;
        "deployments")
            merge_deployments
            ;;
        "report")
            create_comprehensive_report
            ;;
        "csv")
            export_to_csv
            ;;
        "all")
            merge_builds || log "⚠️ Builds merge failed or no data"
            merge_releases || log "⚠️ Releases merge failed or no data"
            merge_deployments || log "⚠️ Deployments merge failed or no data"
            create_comprehensive_report
            export_to_csv
            ;;
        *)
            echo "Usage: $0 [builds|releases|deployments|report|csv|all]"
            echo "  builds      - Merge build data only"
            echo "  releases    - Merge release data only"
            echo "  deployments - Merge deployment data only"
            echo "  report      - Create comprehensive report"
            echo "  csv         - Export to CSV format"
            echo "  all         - Execute all operations (default)"
            exit 1
            ;;
    esac
    
    # Cleanup
    rm -rf "$TEMP_DIR"
    
    log "Azure DevOps JSON merger completed"
    echo ""
    echo "Output files generated in $OUTPUT_DIR:"
    ls -la "$OUTPUT_DIR/"
}

# Execute main function with all arguments
main "$@"