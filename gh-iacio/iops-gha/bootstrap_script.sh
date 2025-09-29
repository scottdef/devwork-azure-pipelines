#!/bin/bash
# bootstrap-issueops.sh - Bootstrap IssueOps system
# One command to rule them all

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info() { echo -e "${BLUE}ℹ${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; }
error() { echo -e "${RED}✗${NC} $*"; }
warn() { echo -e "${YELLOW}⚠${NC} $*"; }
section() { echo -e "\n${BOLD}${BLUE}▶ $*${NC}\n"; }

die() {
    error "$*"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    section "Checking Prerequisites"
    
    local missing=()
    
    command -v git >/dev/null 2>&1 || missing+=("git")
    command -v gh >/dev/null 2>&1 || missing+=("gh")
    command -v python3 >/dev/null 2>&1 || missing+=("python3")
    
    if [ ${#missing[@]} -gt 0 ]; then
        error "Missing required tools: ${missing[*]}"
        echo ""
        echo "Install them:"
        echo "  macOS:  brew install ${missing[*]}"
        echo "  Ubuntu: apt install ${missing[*]}"
        exit 1
    fi
    
    success "All required tools installed"
    
    # Check Python packages
    if ! python3 -c "import yaml" 2>/dev/null; then
        warn "PyYAML not installed"
        info "Installing PyYAML..."
        pip3 install pyyaml || python3 -m pip install pyyaml || {
            error "Failed to install PyYAML"
            echo "Try: pip3 install pyyaml"
            exit 1
        }
        success "PyYAML installed"
    else
        success "PyYAML installed"
    fi
    
    # Check gh auth
    if ! gh auth status >/dev/null 2>&1; then
        warn "GitHub CLI not authenticated"
        info "Running: gh auth login"
        gh auth login || die "GitHub CLI authentication failed"
        success "GitHub CLI authenticated"
    else
        success "GitHub CLI authenticated"
    fi
}

# Make scripts executable
setup_scripts() {
    section "Setting Up Scripts"
    
    chmod +x scripts/*.sh scripts/*.py 2>/dev/null || true
    success "Scripts made executable"
}

# Verify directory structure
verify_structure() {
    section "Verifying Directory Structure"
    
    local dirs=(
        ".github/ISSUE_TEMPLATE"
        ".github/workflows"
        "repo-yamls"
        "team-yamls"
        "scripts"
        "terraform"
        "templates"
    )
    
    for dir in "${dirs[@]}"; do
        if [ ! -d "$dir" ]; then
            warn "Creating missing directory: $dir"
            mkdir -p "$dir"
        fi
        success "$dir exists"
    done
}

# Check for platform-team
check_platform_team() {
    section "Checking Platform Team"
    
    local org
    org=$(gh repo view --json owner -q .owner.login 2>/dev/null) || {
        warn "Cannot determine organization"
        return
    }
    
    if gh api "orgs/$org/teams/platform-team" >/dev/null 2>&1; then
        success "platform-team exists"
    else
        warn "platform-team does not exist"
        echo ""
        echo "Create it:"
        echo "  gh api orgs/$org/teams -f name=platform-team -f privacy=closed"
        echo ""
    fi
}

# Configure secrets
configure_secrets() {
    section "Configuring Secrets"
    
    info "Checking for required secrets..."
    
    local secrets=(
        "GH_ADMIN_TOKEN"
        "GITHUB_ORG"
    )
    
    for secret in "${secrets[@]}"; do
        if gh secret list | grep -q "^$secret"; then
            success "$secret is configured"
        else
            warn "$secret is not configured"
            echo "  Set it: gh secret set $secret"
        fi
    done
    
    echo ""
    info "To set secrets:"
    echo "  gh secret set GH_ADMIN_TOKEN --body 'ghp_your_token_here'"
    echo "  gh secret set GITHUB_ORG --body 'your-org-name'"
    echo ""
}

# Run tests
run_tests() {
    section "Running Validation Tests"
    
    if [ -f "scripts/test-issueops.sh" ]; then
        ./scripts/test-issueops.sh
    else
        warn "Test script not found, skipping tests"
    fi
}

# Create example files
create_examples() {
    section "Creating Example Files"
    
    # Example team if none exist
    if [ ! -f "team-yamls/platform-team.yml" ]; then
        cat > "team-yamls/platform-team.yml" <<'EOF'
teams:
  - name: platform-team
    description: "Platform Engineering Team"
    privacy: closed
    ad_group: "engineering-platform"
    members:
      - username: admin
        role: maintainer
EOF
        success "Created team-yamls/platform-team.yml"
    fi
    
    # Example repo if none exist
    if [ ! -f "repo-yamls/platform-repos.yml" ]; then
        cat > "repo-yamls/platform-repos.yml" <<'EOF'
repos:
  - name: example-repo
    description: "Example repository created during bootstrap"
    team: platform-team
    visibility: private
    has_issues: true
EOF
        success "Created repo-yamls/platform-repos.yml"
    fi
}

# Print next steps
print_next_steps() {
    section "Bootstrap Complete!"
    
    echo ""
    echo -e "${BOLD}Next Steps:${NC}"
    echo ""
    echo "1. Configure secrets (if not done):"
    echo "   ${BLUE}gh secret set GH_ADMIN_TOKEN${NC}"
    echo "   ${BLUE}gh secret set GITHUB_ORG${NC}"
    echo ""
    echo "2. Ensure platform-team exists in GitHub"
    echo ""
    echo "3. Test IssueOps:"
    echo "   ${BLUE}# Create test issue${NC}"
    echo "   Go to Issues → New Issue → 'Create Repository'"
    echo ""
    echo "4. Approve issue:"
    echo "   ${BLUE}# React with 👍 or comment /approve${NC}"
    echo "   Need 2 approvals from platform-team members"
    echo ""
    echo "5. Watch automation:"
    echo "   ${BLUE}# Check Actions tab${NC}"
    echo "   Workflow will create PR automatically"
    echo ""
    echo -e "${BOLD}Documentation:${NC}"
    echo "  • ISSUEOPS.md - Full documentation"
    echo "  • ISSUEOPS-QUICKSTART.md - Quick start"
    echo "  • ISSUEOPS-INTEGRATION.md - Integration guide"
    echo ""
    echo -e "${GREEN}✓ IssueOps is ready to use!${NC}"
    echo ""
}

# Main
main() {
    echo ""
    echo -e "${BOLD}═══════════════════════════════════════════${NC}"
    echo -e "${BOLD}  IssueOps Bootstrap${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════${NC}"
    echo ""
    
    check_prerequisites
    setup_scripts
    verify_structure
    check_platform_team
    configure_secrets
    create_examples
    run_tests || warn "Some tests failed, but continuing..."
    print_next_steps
}

main "$@"
