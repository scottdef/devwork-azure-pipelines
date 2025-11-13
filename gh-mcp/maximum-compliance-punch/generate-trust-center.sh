#!/bin/bash
# Trust Center Status Generator
# Queries GitHub Actions API and generates status.json
# Simple, composable, Unix-style

set -euo pipefail

# Configuration
GITHUB_ORG="${GITHUB_REPOSITORY_OWNER}"
GITHUB_REPO="${GITHUB_REPOSITORY#*/}"
API_BASE="https://api.github.com"

# Control workflows to check
CONTROL_WORKFLOWS=(
    "control-org-settings.yml"
    "control-repo-visibility.yml"
    "control-repo-rulesets.yml"
    "control-org-custom-role.yml"
)

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

# Get the latest workflow run for a specific workflow file
get_workflow_status() {
    local workflow_file="$1"
    local workflow_id
    
    # Get workflow ID from filename
    workflow_id=$(gh api \
        "/repos/${GITHUB_ORG}/${GITHUB_REPO}/actions/workflows" \
        --jq ".workflows[] | select(.path | contains(\"${workflow_file}\")) | .id" \
        2>/dev/null || echo "")
    
    if [[ -z "$workflow_id" ]]; then
        warn "Workflow not found: ${workflow_file}"
        echo "{\"name\":\"${workflow_file}\",\"status\":\"unknown\",\"last_run\":null,\"run_url\":null}"
        return
    fi
    
    # Get the latest run for this workflow
    local run_data
    run_data=$(gh api \
        "/repos/${GITHUB_ORG}/${GITHUB_REPO}/actions/workflows/${workflow_id}/runs?per_page=1" \
        --jq '.workflow_runs[0]' \
        2>/dev/null || echo "{}")
    
    if [[ "$run_data" == "{}" ]] || [[ -z "$run_data" ]]; then
        warn "No runs found for: ${workflow_file}"
        echo "{\"name\":\"${workflow_file}\",\"status\":\"unknown\",\"last_run\":null,\"run_url\":null}"
        return
    fi
    
    # Extract relevant fields
    local conclusion status created_at html_url
    conclusion=$(echo "$run_data" | jq -r '.conclusion // "unknown"')
    status=$(echo "$run_data" | jq -r '.status // "unknown"')
    created_at=$(echo "$run_data" | jq -r '.created_at // null')
    html_url=$(echo "$run_data" | jq -r '.html_url // null')
    
    # Determine final status
    local final_status
    if [[ "$status" == "completed" ]]; then
        final_status="$conclusion"
    else
        final_status="$status"
    fi
    
    # Build JSON object
    jq -n \
        --arg name "$workflow_file" \
        --arg status "$final_status" \
        --arg last_run "$created_at" \
        --arg run_url "$html_url" \
        '{
            name: $name,
            status: $status,
            last_run: $last_run,
            run_url: $run_url
        }'
}

# Determine overall status based on individual controls
calculate_overall_status() {
    local controls_json="$1"
    local failed degraded
    
    failed=$(echo "$controls_json" | jq '[.[] | select(.status == "failure")] | length')
    degraded=$(echo "$controls_json" | jq '[.[] | select(.status == "unknown" or .status == "pending" or .status == "in_progress")] | length')
    
    if [[ "$failed" -gt 0 ]]; then
        echo "failed"
    elif [[ "$degraded" -gt 0 ]]; then
        echo "degraded"
    else
        echo "operational"
    fi
}

# Main execution
main() {
    log "Starting trust center status generation..."
    
    # Check for required tools
    if ! command -v gh >/dev/null 2>&1; then
        error "GitHub CLI (gh) not found. Please install it first."
        exit 1
    fi
    
    if ! command -v jq >/dev/null 2>&1; then
        error "jq not found. Please install it first."
        exit 1
    fi
    
    # Verify authentication
    if ! gh auth status >/dev/null 2>&1; then
        error "GitHub CLI not authenticated. Run: gh auth login"
        exit 1
    fi
    
    log "Checking ${#CONTROL_WORKFLOWS[@]} control workflows..."
    
    # Collect status for all controls
    controls_json="["
    first=true
    
    for workflow in "${CONTROL_WORKFLOWS[@]}"; do
        log "Querying: ${workflow}"
        
        if [[ "$first" == true ]]; then
            first=false
        else
            controls_json+=","
        fi
        
        workflow_status=$(get_workflow_status "$workflow")
        controls_json+="$workflow_status"
    done
    
    controls_json+="]"
    
    # Calculate overall status
    overall_status=$(calculate_overall_status "$controls_json")
    log "Overall status: ${overall_status}"
    
    # Generate final JSON
    generated_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    final_json=$(jq -n \
        --arg overall "$overall_status" \
        --arg generated "$generated_at" \
        --argjson controls "$controls_json" \
        '{
            overall_status: $overall,
            generated_at: $generated,
            controls: $controls
        }')
    
    # Write to file
    output_dir="trust-center"
    mkdir -p "$output_dir"
    
    echo "$final_json" | jq '.' > "${output_dir}/status.json"
    
    log "Status file generated: ${output_dir}/status.json"
    log "Overall status: ${overall_status}"
    
    # Pretty print summary
    echo ""
    echo "Control Status Summary:"
    echo "$controls_json" | jq -r '.[] | "  - \(.name): \(.status)"'
    echo ""
}

# Run main function
main "$@"
