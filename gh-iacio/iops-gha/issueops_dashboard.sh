#!/bin/bash
# issueops-dashboard.sh - IssueOps monitoring dashboard
# Show status of all IssueOps requests

set -euo pipefail

BOLD='\033[1m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Check if gh is available
command -v gh >/dev/null 2>&1 || {
    echo "Error: gh CLI not found"
    echo "Install: https://cli.github.com"
    exit 1
}

header() {
    echo ""
    echo -e "${BOLD}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  $*${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════════${NC}"
    echo ""
}

section() {
    echo ""
    echo -e "${CYAN}▶ $*${NC}"
    echo ""
}

# Pending approvals
show_pending() {
    section "Pending Approvals"
    
    local pending
    pending=$(gh issue list \
        --label "pending-approval" \
        --state open \
        --json number,title,author,createdAt,labels \
        --jq '.[] | "#\(.number) - \(.title) by @\(.author.login) (\(.createdAt | fromdateiso8601 | strftime("%Y-%m-%d")))"' \
        2>/dev/null || echo "")
    
    if [ -z "$pending" ]; then
        echo -e "${GREEN}✓ No pending approvals${NC}"
    else
        echo "$pending" | while read -r line; do
            echo -e "  ${YELLOW}⏳${NC} $line"
        done
        
        local count
        count=$(echo "$pending" | wc -l | tr -d ' ')
        echo ""
        echo -e "  ${BOLD}Total: $count pending${NC}"
    fi
}

# Recently approved
show_approved() {
    section "Recently Approved (Last 7 days)"
    
    local approved
    approved=$(gh issue list \
        --label "approved" \
        --state all \
        --limit 10 \
        --json number,title,author,updatedAt,state \
        --jq '.[] | "#\(.number) - \(.title) by @\(.author.login) [\(.state)]"' \
        2>/dev/null || echo "")
    
    if [ -z "$approved" ]; then
        echo -e "${YELLOW}No recently approved requests${NC}"
    else
        echo "$approved" | while read -r line; do
            if [[ $line == *"[CLOSED]"* ]]; then
                echo -e "  ${GREEN}✓${NC} $line"
            else
                echo -e "  ${BLUE}→${NC} $line"
            fi
        done
    fi
}

# Active PRs from IssueOps
show_active_prs() {
    section "Active IssueOps PRs"
    
    local prs
    prs=$(gh pr list \
        --label "issueops" \
        --json number,title,author,createdAt,reviews \
        --jq '.[] | "\(.number)|\(.title)|\(.author.login)|\(.reviews | length)"' \
        2>/dev/null || echo "")
    
    if [ -z "$prs" ]; then
        echo -e "${GREEN}✓ No active PRs${NC}"
    else
        echo "$prs" | while IFS='|' read -r num title author reviews; do
            echo -e "  ${CYAN}#$num${NC} - $title"
            echo -e "    Author: @$author | Reviews: $reviews"
        done
        
        local count
        count=$(echo "$prs" | wc -l | tr -d ' ')
        echo ""
        echo -e "  ${BOLD}Total: $count active PRs${NC}"
    fi
}

# Errors
show_errors() {
    section "Failed Requests"
    
    local errors
    errors=$(gh issue list \
        --label "error" \
        --state open \
        --json number,title,author,updatedAt \
        --jq '.[] | "#\(.number) - \(.title) by @\(.author.login)"' \
        2>/dev/null || echo "")
    
    if [ -z "$errors" ]; then
        echo -e "${GREEN}✓ No errors${NC}"
    else
        echo "$errors" | while read -r line; do
            echo -e "  ${RED}✗${NC} $line"
        done
        
        local count
        count=$(echo "$errors" | wc -l | tr -d ' ')
        echo ""
        echo -e "  ${BOLD}${RED}Total: $count errors${NC}"
    fi
}

# Statistics
show_statistics() {
    section "Statistics (Last 30 days)"
    
    local total created deleted members
    
    # Count by type
    created=$(gh issue list \
        --label "create-repo" \
        --state all \
        --search "created:>=$(date -d '30 days ago' +%Y-%m-%d 2>/dev/null || date -v-30d +%Y-%m-%d)" \
        2>/dev/null | wc -l | tr -d ' ')
    
    deleted=$(gh issue list \
        --label "delete-repo" \
        --state all \
        --search "created:>=$(date -d '30 days ago' +%Y-%m-%d 2>/dev/null || date -v-30d +%Y-%m-%d)" \
        2>/dev/null | wc -l | tr -d ' ')
    
    members=$(gh issue list \
        --label "add-member" \
        --state all \
        --search "created:>=$(date -d '30 days ago' +%Y-%m-%d 2>/dev/null || date -v-30d +%Y-%m-%d)" \
        2>/dev/null | wc -l | tr -d ' ')
    
    total=$((created + deleted + members))
    
    echo -e "  ${BOLD}Total Requests:${NC} $total"
    echo -e "  ${GREEN}├─${NC} Create Repository: $created"
    echo -e "  ${RED}├─${NC} Delete Repository: $deleted"
    echo -e "  ${BLUE}└─${NC} Add Team Member: $members"
    
    # Success rate
    local closed
    closed=$(gh issue list \
        --label "issueops" \
        --state closed \
        --search "created:>=$(date -d '30 days ago' +%Y-%m-%d 2>/dev/null || date -v-30d +%Y-%m-%d)" \
        2>/dev/null | wc -l | tr -d ' ')
    
    if [ "$total" -gt 0 ]; then
        local success_rate
        success_rate=$((closed * 100 / total))
        echo ""
        echo -e "  ${BOLD}Completion Rate:${NC} $success_rate% ($closed/$total)"
    fi
}

# Recent workflow runs
show_workflows() {
    section "Recent Workflow Runs (Last 5)"
    
    local runs
    runs=$(gh run list \
        --workflow "issueops-create-repo.yml" \
        --limit 5 \
        --json conclusion,createdAt,displayTitle,number \
        --jq '.[] | "\(.conclusion)|\(.number)|\(.displayTitle)"' \
        2>/dev/null || echo "")
    
    if [ -z "$runs" ]; then
        echo -e "${YELLOW}No recent workflow runs${NC}"
    else
        echo "$runs" | while IFS='|' read -r status num title; do
            case "$status" in
                success)
                    echo -e "  ${GREEN}✓${NC} #$num - $title"
                    ;;
                failure)
                    echo -e "  ${RED}✗${NC} #$num - $title"
                    ;;
                *)
                    echo -e "  ${YELLOW}⏳${NC} #$num - $title"
                    ;;
            esac
        done
    fi
}

# Top requesters
show_top_requesters() {
    section "Top Requesters (Last 30 days)"
    
    gh issue list \
        --label "issueops" \
        --state all \
        --search "created:>=$(date -d '30 days ago' +%Y-%m-%d 2>/dev/null || date -v-30d +%Y-%m-%d)" \
        --json author \
        --jq -r '.[].author.login' \
        2>/dev/null | sort | uniq -c | sort -rn | head -5 | while read -r count user; do
        echo -e "  ${BOLD}$count${NC} requests - @$user"
    done || echo -e "${YELLOW}No data available${NC}"
}

# Main dashboard
main() {
    header "IssueOps Dashboard"
    
    echo -e "${BLUE}Updated:${NC} $(date)"
    echo -e "${BLUE}Repository:${NC} $(gh repo view --json nameWithOwner -q .nameWithOwner)"
    
    show_pending
    show_approved
    show_active_prs
    show_errors
    show_statistics
    show_workflows
    show_top_requesters
    
    echo ""
    echo -e "${BOLD}Quick Actions:${NC}"
    echo -e "  View pending:     ${CYAN}gh issue list --label pending-approval${NC}"
    echo -e "  Approve request:  ${CYAN}gh issue comment <number> --body '/approve'${NC}"
    echo -e "  View workflows:   ${CYAN}gh run list${NC}"
    echo ""
}

# Parse arguments
case "${1:-}" in
    pending)
        show_pending
        ;;
    approved)
        show_approved
        ;;
    prs)
        show_active_prs
        ;;
    errors)
        show_errors
        ;;
    stats)
        show_statistics
        ;;
    workflows)
        show_workflows
        ;;
    *)
        main
        ;;
esac
