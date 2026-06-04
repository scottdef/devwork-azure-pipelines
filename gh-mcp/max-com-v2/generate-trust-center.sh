#!/bin/bash
# Trust Center Status Generator v2
# Queries GitHub Actions API — fetches last 5 runs per control,
# calculates pass rates, trends, and generates enriched status.json
#
# "A little copying is better than a little dependency." — Go Proverbs

set -euo pipefail

# Configuration
GITHUB_ORG="${GITHUB_REPOSITORY_OWNER}"
GITHUB_REPO="${GITHUB_REPOSITORY#*/}"
FULL_REPO="${GITHUB_ORG}/${GITHUB_REPO}"
HISTORY_DEPTH=5

# Control workflows to check
CONTROL_WORKFLOWS=(
    "control-org-settings.yml"
    "control-repo-visibility.yml"
    "control-repo-rulesets.yml"
    "control-org-custom-role.yml"
)

# Human-readable descriptions for each control
declare -A CONTROL_DESCRIPTIONS
CONTROL_DESCRIPTIONS=(
    ["control-org-settings.yml"]="Validates organization-level security settings including two-factor authentication enforcement, default repository permissions, and member privilege boundaries."
    ["control-repo-visibility.yml"]="Monitors repository visibility policies ensuring sensitive repositories remain private, public exposure stays below threshold, and naming conventions are enforced."
    ["control-repo-rulesets.yml"]="Verifies branch protection rules, organization-level rulesets, required status checks, and pull request review requirements on critical repositories."
    ["control-org-custom-role.yml"]="Audits custom organization role definitions, validates least-privilege assignments, and checks for overly permissive role configurations."
)

# Control categories
declare -A CONTROL_CATEGORIES
CONTROL_CATEGORIES=(
    ["control-org-settings.yml"]="Access Control"
    ["control-repo-visibility.yml"]="Data Protection"
    ["control-repo-rulesets.yml"]="Change Management"
    ["control-org-custom-role.yml"]="Access Control"
)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()   { echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $*" >&2; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }

# Get the last N workflow runs for a specific workflow file
get_workflow_status() {
    local workflow_file="$1"
    local workflow_id

    # Resolve workflow ID from filename
    workflow_id=$(gh api \
        "/repos/${FULL_REPO}/actions/workflows" \
        --jq ".workflows[] | select(.path | contains(\"${workflow_file}\")) | .id" \
        2>/dev/null || echo "")

    if [[ -z "$workflow_id" ]]; then
        warn "Workflow not found: ${workflow_file}"
        jq -n \
            --arg name "$workflow_file" \
            --arg desc "${CONTROL_DESCRIPTIONS[$workflow_file]:-No description available.}" \
            --arg cat "${CONTROL_CATEGORIES[$workflow_file]:-General}" \
            '{
                name: $name,
                description: $desc,
                category: $cat,
                status: "unknown",
                last_run: null,
                run_url: null,
                all_runs_url: null,
                docs_url: null,
                pass_rate: null,
                trend: "unknown",
                history: []
            }'
        return
    fi

    # Fetch last N runs
    local runs_json
    runs_json=$(gh api \
        "/repos/${FULL_REPO}/actions/workflows/${workflow_id}/runs?per_page=${HISTORY_DEPTH}" \
        --jq '.workflow_runs' \
        2>/dev/null || echo "[]")

    local run_count
    run_count=$(echo "$runs_json" | jq 'length')

    if [[ "$run_count" -eq 0 ]]; then
        warn "No runs found for: ${workflow_file}"
        jq -n \
            --arg name "$workflow_file" \
            --arg desc "${CONTROL_DESCRIPTIONS[$workflow_file]:-No description available.}" \
            --arg cat "${CONTROL_CATEGORIES[$workflow_file]:-General}" \
            --arg all_runs "https://github.com/${FULL_REPO}/actions/workflows/${workflow_file}" \
            --arg docs "docs/${workflow_file%.yml}.html" \
            '{
                name: $name,
                description: $desc,
                category: $cat,
                status: "unknown",
                last_run: null,
                run_url: null,
                all_runs_url: $all_runs,
                docs_url: $docs,
                pass_rate: null,
                trend: "unknown",
                history: []
            }'
        return
    fi

    # Build history array (last N runs, newest first)
    local history
    history=$(echo "$runs_json" | jq '[
        .[] | {
            status: (if .status == "completed" then (.conclusion // "unknown") else .status end),
            created_at: .created_at,
            run_url: .html_url,
            run_number: .run_number,
            duration_seconds: (
                if .updated_at and .created_at then
                    (((.updated_at | fromdateiso8601) - (.created_at | fromdateiso8601)) | if . < 0 then 0 else . end)
                else 0 end
            )
        }
    ]')

    # Extract latest run info
    local latest_status latest_run latest_url
    latest_status=$(echo "$history" | jq -r '.[0].status')
    latest_run=$(echo "$history" | jq -r '.[0].created_at')
    latest_url=$(echo "$history" | jq -r '.[0].run_url')

    # Calculate pass rate over the history window
    local total_completed pass_count pass_rate
    total_completed=$(echo "$history" | jq '[.[] | select(.status == "success" or .status == "failure")] | length')
    pass_count=$(echo "$history" | jq '[.[] | select(.status == "success")] | length')

    if [[ "$total_completed" -gt 0 ]]; then
        pass_rate=$(echo "scale=0; ($pass_count * 100) / $total_completed" | bc)
    else
        pass_rate="null"
    fi

    # Calculate trend: compare first half vs second half of history
    # "improving" = more recent runs have higher pass rate
    # "degrading" = more recent runs have lower pass rate
    # "stable"    = same or too few runs to tell
    local trend="stable"
    if [[ "$run_count" -ge 3 ]]; then
        local recent_pass older_pass midpoint
        midpoint=$(( run_count / 2 ))

        recent_pass=$(echo "$history" | jq --argjson mid "$midpoint" \
            '[.[:$mid] | .[] | select(.status == "success")] | length')
        older_pass=$(echo "$history" | jq --argjson mid "$midpoint" \
            '[.[$mid:] | .[] | select(.status == "success")] | length')

        if [[ "$recent_pass" -gt "$older_pass" ]]; then
            trend="improving"
        elif [[ "$recent_pass" -lt "$older_pass" ]]; then
            trend="degrading"
        fi
    fi

    # All-runs URL and docs URL
    local all_runs_url="https://github.com/${FULL_REPO}/actions/workflows/${workflow_file}"
    local docs_url="docs/${workflow_file%.yml}.html"

    # Build final JSON object for this control
    jq -n \
        --arg name "$workflow_file" \
        --arg desc "${CONTROL_DESCRIPTIONS[$workflow_file]:-No description available.}" \
        --arg cat "${CONTROL_CATEGORIES[$workflow_file]:-General}" \
        --arg status "$latest_status" \
        --arg last_run "$latest_run" \
        --arg run_url "$latest_url" \
        --arg all_runs "$all_runs_url" \
        --arg docs "$docs_url" \
        --argjson pass_rate "${pass_rate:-null}" \
        --arg trend "$trend" \
        --argjson history "$history" \
        '{
            name: $name,
            description: $desc,
            category: $cat,
            status: $status,
            last_run: $last_run,
            run_url: $run_url,
            all_runs_url: $all_runs,
            docs_url: $docs,
            pass_rate: $pass_rate,
            trend: $trend,
            history: $history
        }'
}

# Determine overall status
calculate_overall_status() {
    local controls_json="$1"
    local failed degraded
    failed=$(echo "$controls_json"  | jq '[.[] | select(.status == "failure")]  | length')
    degraded=$(echo "$controls_json" | jq '[.[] | select(.status == "unknown" or .status == "pending" or .status == "in_progress")] | length')

    if   [[ "$failed" -gt 0 ]]; then echo "failed"
    elif [[ "$degraded" -gt 0 ]]; then echo "degraded"
    else echo "operational"
    fi
}

# Calculate aggregate summary statistics
calculate_summary() {
    local controls_json="$1"
    echo "$controls_json" | jq '{
        total_controls: length,
        passing: [.[] | select(.status == "success")] | length,
        failing: [.[] | select(.status == "failure")] | length,
        unknown: [.[] | select(.status == "unknown" or .status == "pending" or .status == "in_progress")] | length,
        average_pass_rate: (
            [.[] | select(.pass_rate != null) | .pass_rate] |
            if length > 0 then (add / length | floor) else null end
        ),
        categories: (group_by(.category) | map({
            name: .[0].category,
            total: length,
            passing: [.[] | select(.status == "success")] | length
        }))
    }'
}

main() {
    log "Starting trust center status generation (v2)..."
    log "Repository: ${FULL_REPO}"
    log "History depth: ${HISTORY_DEPTH} runs"

    for cmd in gh jq bc; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            error "$cmd not found. Please install it first."
            exit 1
        fi
    done

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
        [[ "$first" == true ]] && first=false || controls_json+=","
        workflow_status=$(get_workflow_status "$workflow")
        controls_json+="$workflow_status"
    done

    controls_json+="]"

    # Calculate overall status and summary
    overall_status=$(calculate_overall_status "$controls_json")
    summary=$(calculate_summary "$controls_json")
    generated_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    log "Overall status: ${overall_status}"

    # Build final JSON
    final_json=$(jq -n \
        --arg overall "$overall_status" \
        --arg generated "$generated_at" \
        --arg repo "$FULL_REPO" \
        --argjson controls "$controls_json" \
        --argjson summary "$summary" \
        '{
            overall_status: $overall,
            generated_at: $generated,
            repository: $repo,
            summary: $summary,
            controls: $controls
        }')

    output_dir="trust-center"
    mkdir -p "$output_dir"
    echo "$final_json" | jq '.' > "${output_dir}/status.json"

    log "Status file generated: ${output_dir}/status.json"

    # Summary
    echo ""
    echo "═══════════════════════════════════════"
    echo " Trust Center Status Summary"
    echo "═══════════════════════════════════════"
    echo " Overall: ${overall_status}"
    echo " Controls: $(echo "$summary" | jq -r '.total_controls')"
    echo " Passing:  $(echo "$summary" | jq -r '.passing')"
    echo " Failing:  $(echo "$summary" | jq -r '.failing')"
    echo " Avg Pass Rate: $(echo "$summary" | jq -r '.average_pass_rate // "N/A"')%"
    echo "═══════════════════════════════════════"
    echo "$controls_json" | jq -r '.[] | "  \(.status | if . == "success" then "✅" elif . == "failure" then "❌" else "❓" end) \(.name)  [\(.pass_rate // "?")% pass rate]  \(.trend)"'
    echo "═══════════════════════════════════════"
    echo ""
}

main "$@"
