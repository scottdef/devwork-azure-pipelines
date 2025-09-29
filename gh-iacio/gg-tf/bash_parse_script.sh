#!/bin/bash
# parse-yamls.sh - YAML parsing and validation script
# True Unix style: do one thing well

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

die() {
    echo -e "${RED}Error: $*${NC}" >&2
    exit 1
}

info() {
    echo -e "${GREEN}$*${NC}"
}

warn() {
    echo -e "${YELLOW}$*${NC}"
}

# Check dependencies
command -v yq >/dev/null 2>&1 || die "yq is required but not installed"

# Parse command line
ACTION=${1:-validate}
TARGET=${2:-.}

validate_yaml() {
    local file=$1
    
    if ! yq eval '.' "$file" >/dev/null 2>&1; then
        warn "Invalid YAML: $file"
        return 1
    fi
    
    return 0
}

validate_repo_yaml() {
    local file=$1
    
    # Check required fields
    if ! yq eval '.repos' "$file" >/dev/null 2>&1; then
        warn "$file: missing 'repos' key"
        return 1
    fi
    
    # Check each repo has a name
    local count=$(yq eval '.repos | length' "$file")
    for ((i=0; i<count; i++)); do
        local name=$(yq eval ".repos[$i].name" "$file")
        if [ "$name" = "null" ] || [ -z "$name" ]; then
            warn "$file: repo at index $i missing 'name'"
            return 1
        fi
    done
    
    info "✓ $file"
    return 0
}

validate_team_yaml() {
    local file=$1
    
    # Check required fields
    if ! yq eval '.teams' "$file" >/dev/null 2>&1; then
        warn "$file: missing 'teams' key"
        return 1
    fi
    
    # Check each team has a name
    local count=$(yq eval '.teams | length' "$file")
    for ((i=0; i<count; i++)); do
        local name=$(yq eval ".teams[$i].name" "$file")
        if [ "$name" = "null" ] || [ -z "$name" ]; then
            warn "$file: team at index $i missing 'name'"
            return 1
        fi
    done
    
    info "✓ $file"
    return 0
}

list_repos() {
    find "${TARGET}/repo-yamls" -name "*.yml" -o -name "*.yaml" | while read -r file; do
        yq eval '.repos[].name' "$file" 2>/dev/null | sed "s/^/  /"
    done
}

list_teams() {
    find "${TARGET}/team-yamls" -name "*.yml" -o -name "*.yaml" | while read -r file; do
        yq eval '.teams[].name' "$file" 2>/dev/null | sed "s/^/  /"
    done
}

case "$ACTION" in
    validate)
        info "Validating repository YAMLs..."
        failed=0
        
        if [ -d "${TARGET}/repo-yamls" ]; then
            for file in "${TARGET}"/repo-yamls/*.{yml,yaml}; do
                [ -f "$file" ] || continue
                validate_yaml "$file" && validate_repo_yaml "$file" || ((failed++))
            done
        fi
        
        if [ -d "${TARGET}/team-yamls" ]; then
            info "Validating team YAMLs..."
            for file in "${TARGET}"/team-yamls/*.{yml,yaml}; do
                [ -f "$file" ] || continue
                validate_yaml "$file" && validate_team_yaml "$file" || ((failed++))
            done
        fi
        
        if [ $failed -eq 0 ]; then
            info "All YAML files are valid"
            exit 0
        else
            die "$failed validation(s) failed"
        fi
        ;;
        
    list-repos)
        info "Repositories:"
        list_repos
        ;;
        
    list-teams)
        info "Teams:"
        list_teams
        ;;
        
    *)
        cat <<EOF
Usage: $0 <action> [target_dir]

Actions:
    validate     Validate all YAML files (default)
    list-repos   List all repositories defined in YAMLs
    list-teams   List all teams defined in YAMLs

Examples:
    $0 validate
    $0 list-repos .
    $0 list-teams
EOF
        exit 1
        ;;
esac
