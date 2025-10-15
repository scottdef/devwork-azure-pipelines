#!/usr/bin/env bash
# ==============================================================================
# Work Item Prefix Pattern Validation Script
# ==============================================================================
#
# Tests the regex pattern: ^#AB\d{6,10}.*$
# Used for Azure Boards work item enforcement in GitHub rulesets
#
# Usage:
#   chmod +x validate_workitem_pattern.sh
#   ./validate_workitem_pattern.sh
#
# ==============================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Pattern to test
PATTERN='^#AB[0-9]{6,10}.*$'

# Counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# ==============================================================================
# Helper Functions
# ==============================================================================

print_header() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
}

print_section() {
    echo ""
    echo -e "${YELLOW}--- $1 ---${NC}"
    echo ""
}

test_pattern() {
    local test_string="$1"
    local expected_result="$2"  # "pass" or "fail"
    local description="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if echo "$test_string" | grep -qE "$PATTERN"; then
        actual_result="pass"
    else
        actual_result="fail"
    fi
    
    if [ "$actual_result" == "$expected_result" ]; then
        echo -e "${GREEN}✓ PASS${NC} | $description"
        echo "         String: \"$test_string\""
        echo "         Expected: $expected_result | Got: $actual_result"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}✗ FAIL${NC} | $description"
        echo "         String: \"$test_string\""
        echo "         Expected: $expected_result | Got: $actual_result"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    echo ""
}

# ==============================================================================
# Main Test Suite
# ==============================================================================

print_header "Work Item Pattern Validation"

echo "Testing pattern: $PATTERN"
echo "Pattern breakdown:"
echo "  ^       = Start of string"
echo "  #AB     = Literal '#AB'"
echo "  [0-9]{6,10} = 6 to 10 digits"
echo "  .*      = Any characters after"
echo "  $       = End of string"
echo ""

# ==============================================================================
# Test Suite: Valid Commit Messages (Should PASS)
# ==============================================================================

print_section "Valid Commit Messages (Should be ALLOWED by ruleset)"

test_pattern "#AB123975 fix really bad bug" "pass" "Example commit A from requirements"
test_pattern "#AB123456 implement new feature" "pass" "6 digits with message"
test_pattern "#AB1234567 seven digit work item" "pass" "7 digits"
test_pattern "#AB12345678 eight digit work item" "pass" "8 digits"
test_pattern "#AB123456789 nine digit work item" "pass" "9 digits"
test_pattern "#AB1234567890 ten digit work item" "pass" "10 digits (maximum)"
test_pattern "#AB123456" "pass" "6 digits only, no message"
test_pattern "#AB999999 edge case all nines" "pass" "Edge case: all 9s"
test_pattern "#AB100000 edge case minimum 6 digits" "pass" "Edge case: minimum valid"
test_pattern "#AB9999999999 edge case maximum 10 digits" "pass" "Edge case: maximum valid"

# ==============================================================================
# Test Suite: Invalid Commit Messages (Should FAIL)
# ==============================================================================

print_section "Invalid Commit Messages (Should be BLOCKED by ruleset)"

test_pattern "#AB75 fix really bad bug" "fail" "Example commit B from requirements (only 2 digits)"
test_pattern "#fix really bad bug" "fail" "Example commit C from requirements (no work item)"
test_pattern "fix really bad bug" "fail" "No prefix at all"
test_pattern "#AB12345 short work item" "fail" "Only 5 digits (needs 6-10)"
test_pattern "#AB1 too short" "fail" "Only 1 digit"
test_pattern "#AB12 too short" "fail" "Only 2 digits"
test_pattern "#AB123 too short" "fail" "Only 3 digits"
test_pattern "#AB1234 too short" "fail" "Only 4 digits"
test_pattern "#AB12345 still too short" "fail" "Only 5 digits"
test_pattern "#AB12345678901 too long" "fail" "11 digits (exceeds maximum of 10)"
test_pattern "#AB123456789012 way too long" "fail" "12 digits"
test_pattern "AB123456 no hash symbol" "fail" "Missing # symbol"
test_pattern "#ab123456 lowercase prefix" "fail" "Lowercase 'ab' instead of 'AB'"
test_pattern "#Ab123456 mixed case" "fail" "Mixed case 'Ab'"
test_pattern "#aB123456 mixed case variant" "fail" "Mixed case 'aB'"
test_pattern " #AB123456 leading space" "fail" "Leading space"
test_pattern "#AB123456 " "pass" "Trailing space (should pass - after digits)"
test_pattern "#AC123456 wrong prefix" "fail" "Different prefix (AC instead of AB)"
test_pattern "#AB123456a has letter" "fail" "Letter in digit sequence"
test_pattern "#AB12-3456 has dash" "fail" "Non-digit in digit sequence"

# ==============================================================================
# Test Suite: Valid Branch Names (Should PASS)
# ==============================================================================

print_section "Valid Branch Names (Should be ALLOWED by ruleset)"

test_pattern "#AB123975-good-branch" "pass" "Example branch A from requirements"
test_pattern "#AB123456-feature-branch" "pass" "6 digits with branch name"
test_pattern "#AB1234567-feature" "pass" "7 digits"
test_pattern "#AB12345678-bugfix" "pass" "8 digits"
test_pattern "#AB123456789-hotfix" "pass" "9 digits"
test_pattern "#AB1234567890-release" "pass" "10 digits (maximum)"
test_pattern "#AB123456" "pass" "6 digits only"
test_pattern "#AB123456-feature/sub-branch" "pass" "With slash (git allows)"
test_pattern "#AB123456_underscore_branch" "pass" "With underscores"
test_pattern "#AB123456.dot.branch" "pass" "With dots"

# ==============================================================================
# Test Suite: Invalid Branch Names (Should FAIL)
# ==============================================================================

print_section "Invalid Branch Names (Should be BLOCKED by ruleset)"

test_pattern "#AB75-really-bad-branch" "fail" "Example branch B from requirements (only 2 digits)"
test_pattern "#bad-branch" "fail" "Example branch C from requirements (no work item)"
test_pattern "feature-branch" "fail" "No prefix at all"
test_pattern "#AB12345-short" "fail" "Only 5 digits"
test_pattern "#AB12345678901-toolong" "fail" "11 digits (exceeds maximum)"
test_pattern "AB123456-no-hash" "fail" "Missing # symbol"
test_pattern "#ab123456-lowercase" "fail" "Lowercase prefix"
test_pattern "#AB1234a6-has-letter" "fail" "Letter in digit sequence"
test_pattern " #AB123456-space" "fail" "Leading space"

# ==============================================================================
# Edge Cases
# ==============================================================================

print_section "Edge Cases and Special Scenarios"

test_pattern "#AB000000 zeros only" "pass" "All zeros (valid work item ID)"
test_pattern "#AB000001 mostly zeros" "pass" "Leading zeros"
test_pattern "#AB123456#AB789012 multiple work items" "pass" "Second work item in message (first one counts)"
test_pattern "#AB123456\nNewline after" "pass" "Newline in commit message"
test_pattern "#AB123456-URGENT-FIX" "pass" "Uppercase branch name"
test_pattern "#AB123456-mixed-Case-Branch" "pass" "Mixed case in branch suffix"
test_pattern "#AB123456!" "pass" "Exclamation after digits"
test_pattern "#AB123456:" "pass" "Colon after digits"
test_pattern "#AB123456;" "pass" "Semicolon after digits"

# ==============================================================================
# Multi-line Commit Messages
# ==============================================================================

print_section "Multi-line Commit Messages"

test_pattern "#AB123456 First line
Second line
Third line" "pass" "Multi-line commit (first line has pattern)"

test_pattern "First line without pattern
#AB123456 Second line" "fail" "Pattern on second line (should fail)"

# ==============================================================================
# Results Summary
# ==============================================================================

print_header "Test Results Summary"

echo "Total Tests:  $TOTAL_TESTS"
echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}ALL TESTS PASSED! ✓${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "The regex pattern is working correctly for all test cases."
    echo "You can safely deploy the Terraform ruleset."
    echo ""
    exit 0
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}SOME TESTS FAILED! ✗${NC}"
    echo -e "${RED}========================================${NC}"
    echo ""
    echo "Review the failed tests above and adjust the pattern if needed."
    echo ""
    exit 1
fi

# ==============================================================================
# Usage Examples for Reference
# ==============================================================================

: <<'USAGE_EXAMPLES'

Example git commands that will work with this ruleset:

# Valid branch creation
git checkout -b "#AB123456-feature-authentication"
git checkout -b "#AB987654-bugfix-login"
git checkout -b "#AB1234567890-release-v2"

# Valid commits
git commit -m "#AB123456 Add user authentication"
git commit -m "#AB987654 Fix login bug"
git commit -m "#AB555555 Implement password reset"

# Invalid branch creation (will be blocked)
git checkout -b "feature-authentication"
git checkout -b "#AB123-short"
git checkout -b "#AB12345678901-toolong"

# Invalid commits (will be blocked)
git commit -m "Add user authentication"
git commit -m "#AB123 Fix bug"
git commit -m "Fix bug #AB123456"  # Pattern must be at start

USAGE_EXAMPLES
