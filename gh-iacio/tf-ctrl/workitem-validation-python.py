#!/usr/bin/env python3
"""
Work Item Prefix Pattern Validation Script (Python)
====================================================

Tests the regex pattern: ^#AB\\d{6,10}.*$
Used for Azure Boards work item enforcement in GitHub rulesets

Usage:
    python3 validate_workitem_pattern.py
"""

import re
import sys
from dataclasses import dataclass
from typing import List, Tuple

# ANSI color codes
class Colors:
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    NC = '\033[0m'  # No Color

# Pattern to test
PATTERN = r'^#AB\d{6,10}.*$'

@dataclass
class TestResult:
    passed: bool
    test_string: str
    expected: str
    actual: str
    description: str

class PatternValidator:
    def __init__(self):
        self.pattern = re.compile(PATTERN)
        self.total_tests = 0
        self.passed_tests = 0
        self.failed_tests = 0
        self.results: List[TestResult] = []
    
    def test_pattern(self, test_string: str, expected_result: str, description: str) -> bool:
        """Test a string against the pattern."""
        self.total_tests += 1
        
        matches = bool(self.pattern.match(test_string))
        actual_result = "pass" if matches else "fail"
        
        passed = (actual_result == expected_result)
        
        result = TestResult(
            passed=passed,
            test_string=test_string,
            expected=expected_result,
            actual=actual_result,
            description=description
        )
        self.results.append(result)
        
        if passed:
            self.passed_tests += 1
            print(f"{Colors.GREEN}✓ PASS{Colors.NC} | {description}")
        else:
            self.failed_tests += 1
            print(f"{Colors.RED}✗ FAIL{Colors.NC} | {description}")
        
        print(f"         String: \"{test_string}\"")
        print(f"         Expected: {expected_result} | Got: {actual_result}")
        print()
        
        return passed
    
    def print_header(self, text: str):
        """Print a section header."""
        print()
        print(f"{Colors.BLUE}{'=' * 60}{Colors.NC}")
        print(f"{Colors.BLUE}{text}{Colors.NC}")
        print(f"{Colors.BLUE}{'=' * 60}{Colors.NC}")
        print()
    
    def print_section(self, text: str):
        """Print a subsection header."""
        print()
        print(f"{Colors.YELLOW}--- {text} ---{Colors.NC}")
        print()
    
    def print_summary(self):
        """Print test results summary."""
        self.print_header("Test Results Summary")
        
        print(f"Total Tests:  {self.total_tests}")
        print(f"Passed:       {Colors.GREEN}{self.passed_tests}{Colors.NC}")
        print(f"Failed:       {Colors.RED}{self.failed_tests}{Colors.NC}")
        print()
        
        if self.failed_tests == 0:
            print(f"{Colors.GREEN}{'=' * 60}{Colors.NC}")
            print(f"{Colors.GREEN}ALL TESTS PASSED! ✓{Colors.NC}")
            print(f"{Colors.GREEN}{'=' * 60}{Colors.NC}")
            print()
            print("The regex pattern is working correctly for all test cases.")
            print("You can safely deploy the Terraform ruleset.")
            print()
            return True
        else:
            print(f"{Colors.RED}{'=' * 60}{Colors.NC}")
            print(f"{Colors.RED}SOME TESTS FAILED! ✗{Colors.NC}")
            print(f"{Colors.RED}{'=' * 60}{Colors.NC}")
            print()
            print("Review the failed tests above and adjust the pattern if needed.")
            print()
            return False

def run_tests():
    """Run all validation tests."""
    validator = PatternValidator()
    
    validator.print_header("Work Item Pattern Validation")
    
    print(f"Testing pattern: {PATTERN}")
    print("Pattern breakdown:")
    print("  ^           = Start of string")
    print("  #AB         = Literal '#AB'")
    print("  \\d{6,10}   = 6 to 10 digits")
    print("  .*          = Any characters after")
    print("  $           = End of string")
    print()
    
    # =========================================================================
    # Valid Commit Messages (Should PASS)
    # =========================================================================
    
    validator.print_section("Valid Commit Messages (Should be ALLOWED)")
    
    validator.test_pattern(
        "#AB123975 fix really bad bug",
        "pass",
        "Example commit A from requirements"
    )
    validator.test_pattern(
        "#AB123456 implement new feature",
        "pass",
        "6 digits with message"
    )
    validator.test_pattern(
        "#AB1234567 seven digit work item",
        "pass",
        "7 digits"
    )
    validator.test_pattern(
        "#AB12345678 eight digit work item",
        "pass",
        "8 digits"
    )
    validator.test_pattern(
        "#AB123456789 nine digit work item",
        "pass",
        "9 digits"
    )
    validator.test_pattern(
        "#AB1234567890 ten digit work item",
        "pass",
        "10 digits (maximum)"
    )
    validator.test_pattern(
        "#AB123456",
        "pass",
        "6 digits only, no message"
    )
    validator.test_pattern(
        "#AB999999 edge case all nines",
        "pass",
        "Edge case: all 9s"
    )
    validator.test_pattern(
        "#AB100000 edge case minimum 6 digits",
        "pass",
        "Edge case: minimum valid"
    )
    validator.test_pattern(
        "#AB9999999999 edge case maximum 10 digits",
        "pass",
        "Edge case: maximum valid"
    )
    
    # =========================================================================
    # Invalid Commit Messages (Should FAIL)
    # =========================================================================
    
    validator.print_section("Invalid Commit Messages (Should be BLOCKED)")
    
    validator.test_pattern(
        "#AB75 fix really bad bug",
        "fail",
        "Example commit B from requirements (only 2 digits)"
    )
    validator.test_pattern(
        "#fix really bad bug",
        "fail",
        "Example commit C from requirements (no work item)"
    )
    validator.test_pattern(
        "fix really bad bug",
        "fail",
        "No prefix at all"
    )
    validator.test_pattern(
        "#AB12345 short work item",
        "fail",
        "Only 5 digits (needs 6-10)"
    )
    validator.test_pattern(
        "#AB1 too short",
        "fail",
        "Only 1 digit"
    )
    validator.test_pattern(
        "#AB12 too short",
        "fail",
        "Only 2 digits"
    )
    validator.test_pattern(
        "#AB123 too short",
        "fail",
        "Only 3 digits"
    )
    validator.test_pattern(
        "#AB1234 too short",
        "fail",
        "Only 4 digits"
    )
    validator.test_pattern(
        "#AB12345 still too short",
        "fail",
        "Only 5 digits"
    )
    validator.test_pattern(
        "#AB12345678901 too long",
        "fail",
        "11 digits (exceeds maximum of 10)"
    )
    validator.test_pattern(
        "#AB123456789012 way too long",
        "fail",
        "12 digits"
    )
    validator.test_pattern(
        "AB123456 no hash symbol",
        "fail",
        "Missing # symbol"
    )
    validator.test_pattern(
        "#ab123456 lowercase prefix",
        "fail",
        "Lowercase 'ab' instead of 'AB'"
    )
    validator.test_pattern(
        "#Ab123456 mixed case",
        "fail",
        "Mixed case 'Ab'"
    )
    validator.test_pattern(
        "#aB123456 mixed case variant",
        "fail",
        "Mixed case 'aB'"
    )
    validator.test_pattern(
        " #AB123456 leading space",
        "fail",
        "Leading space"
    )
    validator.test_pattern(
        "#AB123456 ",
        "pass",
        "Trailing space (should pass - after digits)"
    )
    validator.test_pattern(
        "#AC123456 wrong prefix",
        "fail",
        "Different prefix (AC instead of AB)"
    )
    validator.test_pattern(
        "#AB123456a has letter",
        "fail",
        "Letter in digit sequence"
    )
    validator.test_pattern(
        "#AB12-3456 has dash",
        "fail",
        "Non-digit in digit sequence"
    )
    
    # =========================================================================
    # Valid Branch Names (Should PASS)
    # =========================================================================
    
    validator.print_section("Valid Branch Names (Should be ALLOWED)")
    
    validator.test_pattern(
        "#AB123975-good-branch",
        "pass",
        "Example branch A from requirements"
    )
    validator.test_pattern(
        "#AB123456-feature-branch",
        "pass",
        "6 digits with branch name"
    )
    validator.test_pattern(
        "#AB1234567-feature",
        "pass",
        "7 digits"
    )
    validator.test_pattern(
        "#AB12345678-bugfix",
        "pass",
        "8 digits"
    )
    validator.test_pattern(
        "#AB123456789-hotfix",
        "pass",
        "9 digits"
    )
    validator.test_pattern(
        "#AB1234567890-release",
        "pass",
        "10 digits (maximum)"
    )
    validator.test_pattern(
        "#AB123456",
        "pass",
        "6 digits only"
    )
    validator.test_pattern(
        "#AB123456-feature/sub-branch",
        "pass",
        "With slash (git allows)"
    )
    validator.test_pattern(
        "#AB123456_underscore_branch",
        "pass",
        "With underscores"
    )
    validator.test_pattern(
        "#AB123456.dot.branch",
        "pass",
        "With dots"
    )
    
    # =========================================================================
    # Invalid Branch Names (Should FAIL)
    # =========================================================================
    
    validator.print_section("Invalid Branch Names (Should be BLOCKED)")
    
    validator.test_pattern(
        "#AB75-really-bad-branch",
        "fail",
        "Example branch B from requirements (only 2 digits)"
    )
    validator.test_pattern(
        "#bad-branch",
        "fail",
        "Example branch C from requirements (no work item)"
    )
    validator.test_pattern(
        "feature-branch",
        "fail",
        "No prefix at all"
    )
    validator.test_pattern(
        "#AB12345-short",
        "fail",
        "Only 5 digits"
    )
    validator.test_pattern(
        "#AB12345678901-toolong",
        "fail",
        "11 digits (exceeds maximum)"
    )
    validator.test_pattern(
        "AB123456-no-hash",
        "fail",
        "Missing # symbol"
    )
    validator.test_pattern(
        "#ab123456-lowercase",
        "fail",
        "Lowercase prefix"
    )
    validator.test_pattern(
        "#AB1234a6-has-letter",
        "fail",
        "Letter in digit sequence"
    )
    validator.test_pattern(
        " #AB123456-space",
        "fail",
        "Leading space"
    )
    
    # =========================================================================
    # Edge Cases
    # =========================================================================
    
    validator.print_section("Edge Cases and Special Scenarios")
    
    validator.test_pattern(
        "#AB000000 zeros only",
        "pass",
        "All zeros (valid work item ID)"
    )
    validator.test_pattern(
        "#AB000001 mostly zeros",
        "pass",
        "Leading zeros"
    )
    validator.test_pattern(
        "#AB123456#AB789012 multiple work items",
        "pass",
        "Second work item in message (first one counts)"
    )
    validator.test_pattern(
        "#AB123456-URGENT-FIX",
        "pass",
        "Uppercase branch name"
    )
    validator.test_pattern(
        "#AB123456-mixed-Case-Branch",
        "pass",
        "Mixed case in branch suffix"
    )
    validator.test_pattern(
        "#AB123456!",
        "pass",
        "Exclamation after digits"
    )
    validator.test_pattern(
        "#AB123456:",
        "pass",
        "Colon after digits"
    )
    validator.test_pattern(
        "#AB123456;",
        "pass",
        "Semicolon after digits"
    )
    
    # Print summary and return exit code
    success = validator.print_summary()
    return 0 if success else 1

if __name__ == "__main__":
    sys.exit(run_tests())
