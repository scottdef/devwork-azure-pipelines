#!/bin/bash
# test-issueops.sh - Test IssueOps setup
# Validates that all components are configured correctly

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

passed=0
failed=0

info() {
    echo -e "${BLUE}ℹ $*${NC}"
}

success() {
    echo -e "${GREEN}✓ $*${NC}"
    ((passed++))
}

fail() {
    echo -e "${RED}✗ $*${NC}"
    ((failed++))
}

warn() {
    echo -e "${YELLOW}⚠ $*${NC}"
}

check_file() {
    local file=$1
    local desc=$2
    
    if [ -f "$file" ]; then
        success "$desc exists"
        return 0
    else
        fail "$desc missing: $file"
        return 1
    fi
}

check_workflow() {
    local workflow=$1
    local desc=$2
    
    if check_file ".github/workflows/$workflow" "$desc workflow"; then
        # Check if workflow has required triggers
        if grep -q "on:" ".github/workflows/$workflow"; then
            success "$desc has triggers"
        else
            fail "$desc missing triggers"
        fi
    fi
}

echo "================================================"
echo "IssueOps Setup Validation"
echo "================================================"
echo ""

# Check directory structure
info "Checking directory structure..."
check_file ".github/ISSUE_TEMPLATE/create-repo.yml" "Create repo issue template"
check_file ".github/workflows/issueops-approval.yml" "Approval workflow"
check_file ".github/workflows/issueops-create-repo.yml" "Create repo workflow"
check_file ".github/issueops-config.yml" "IssueOps config"
check_file "scripts/yaml-helpers.py" "YAML helpers script"
check_file "ISSUEOPS.md" "IssueOps documentation"
check_file "ISSUEOPS-QUICKSTART.md" "Quick start guide"
echo ""

# Check issue template syntax
info "Validating issue template..."
if [ -f ".github/ISSUE_TEMPLATE/create-repo.yml" ]; then
    if python3 -c "import yaml; yaml.safe_load(open('.github/ISSUE_TEMPLATE/create-repo.yml'))" 2>/dev/null; then
        success "Issue template YAML is valid"
    else
        fail "Issue template YAML is invalid"
    fi
    
    # Check for required fields
    if grep -q "id: repo_name" ".github/ISSUE_TEMPLATE/create-repo.yml"; then
        success "Issue template has repo_name field"
    else
        fail "Issue template missing repo_name field"
    fi
    
    if grep -q "id: team" ".github/ISSUE_TEMPLATE/create-repo.yml"; then
        success "Issue template has team field"
    else
        fail "Issue template missing team field"
    fi
fi
echo ""

# Check workflows
info "Validating workflows..."
check_workflow "issueops-approval.yml" "Approval"
check_workflow "issueops-create-repo.yml" "Create repo"

# Check workflow triggers
if [ -f ".github/workflows/issueops-approval.yml" ]; then
    if grep -q "issue_comment:" ".github/workflows/issueops-approval.yml"; then
        success "Approval workflow triggers on issue_comment"
    else
        fail "Approval workflow missing issue_comment trigger"
    fi
fi

if [ -f ".github/workflows/issueops-create-repo.yml" ]; then
    if grep -q "workflow_dispatch:" ".github/workflows/issueops-create-repo.yml"; then
        success "Create repo workflow has workflow_dispatch"
    else
        fail "Create repo workflow missing workflow_dispatch"
    fi
fi
echo ""

# Check scripts
info "Validating scripts..."
if [ -f "scripts/yaml-helpers.py" ]; then
    if python3 -m py_compile scripts/yaml-helpers.py 2>/dev/null; then
        success "yaml-helpers.py compiles"
    else
        fail "yaml-helpers.py has syntax errors"
    fi
    
    if [ -x "scripts/yaml-helpers.py" ]; then
        success "yaml-helpers.py is executable"
    else
        warn "yaml-helpers.py is not executable (run: chmod +x scripts/yaml-helpers.py)"
    fi
fi
echo ""

# Check for required secrets (can't verify values, just document)
info "Required secrets (verify manually in GitHub Settings → Secrets):"
echo "  • GH_ADMIN_TOKEN - GitHub PAT with repo, admin:org, workflow scopes"
echo "  • GITHUB_TOKEN - (automatic, built-in)"
echo ""

# Check Python dependencies
info "Checking Python dependencies..."
if python3 -c "import yaml" 2>/dev/null; then
    success "PyYAML installed"
else
    fail "PyYAML not installed (run: pip install pyyaml)"
fi
echo ""

# Check gh CLI
info "Checking gh CLI..."
if command -v gh >/dev/null 2>&1; then
    success "gh CLI installed"
    
    if gh auth status >/dev/null 2>&1; then
        success "gh CLI authenticated"
    else
        warn "gh CLI not authenticated (run: gh auth login)"
    fi
else
    warn "gh CLI not installed (optional, but recommended)"
fi
echo ""

# Check if platform-team exists (if gh is available)
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    info "Checking GitHub teams..."
    
    if gh api "orgs/$(gh repo view --json owner -q .owner.login)/teams/platform-team" >/dev/null 2>&1; then
        success "platform-team exists"
    else
        fail "platform-team does not exist (create it or update workflows)"
    fi
fi
echo ""

# Check config file
info "Validating config..."
if [ -f ".github/issueops-config.yml" ]; then
    if python3 -c "import yaml; yaml.safe_load(open('.github/issueops-config.yml'))" 2>/dev/null; then
        success "IssueOps config is valid YAML"
        
        # Check required fields
        config_check=$(python3 << 'PYTHON'
import yaml
with open('.github/issueops-config.yml') as f:
    config = yaml.safe_load(f)
    
required = ['approver_teams', 'approvals_required', 'available_teams']
missing = [k for k in required if k not in config]

if missing:
    print(f"Missing: {', '.join(missing)}")
    exit(1)
else:
    print("OK")
PYTHON
)
        if [ "$config_check" = "OK" ]; then
            success "IssueOps config has required fields"
        else
            fail "IssueOps config $config_check"
        fi
    else
        fail "IssueOps config YAML is invalid"
    fi
fi
echo ""

# Test yaml-helpers script
info "Testing yaml-helpers script..."
if [ -f "scripts/yaml-helpers.py" ]; then
    if python3 scripts/yaml-helpers.py list-repos >/dev/null 2>&1; then
        success "yaml-helpers.py list-repos works"
    else
        fail "yaml-helpers.py list-repos failed"
    fi
fi
echo ""

# Summary
echo "================================================"
echo "Test Summary"
echo "================================================"
echo -e "${GREEN}Passed: $passed${NC}"
echo -e "${RED}Failed: $failed${NC}"
echo ""

if [ $failed -eq 0 ]; then
    echo -e "${GREEN}✓ All checks passed! IssueOps is ready.${NC}"
    echo ""
    echo "Next steps:"
    echo "1. Verify secrets in GitHub Settings → Secrets"
    echo "2. Test by creating an issue with the 'Create Repository' template"
    echo "3. Have 2 platform-team members approve it"
    echo "4. Watch the automation run"
    exit 0
else
    echo -e "${RED}✗ Some checks failed. Fix the issues above.${NC}"
    exit 1
fi
