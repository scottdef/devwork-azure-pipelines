# Validation Script Output Example

## Running the Script

```bash
# Make executable and run bash version
chmod +x validate_workitem_pattern.sh
./validate_workitem_pattern.sh

# Or run Python version
python3 validate_workitem_pattern.py
```

## Expected Output

```
========================================
Work Item Pattern Validation
========================================

Testing pattern: ^#AB[0-9]{6,10}.*$
Pattern breakdown:
  ^       = Start of string
  #AB     = Literal '#AB'
  [0-9]{6,10} = 6 to 10 digits
  .*      = Any characters after
  $       = End of string


--- Valid Commit Messages (Should be ALLOWED by ruleset) ---

✓ PASS | Example commit A from requirements
         String: "#AB123975 fix really bad bug"
         Expected: pass | Got: pass

✓ PASS | 6 digits with message
         String: "#AB123456 implement new feature"
         Expected: pass | Got: pass

✓ PASS | 7 digits
         String: "#AB1234567 seven digit work item"
         Expected: pass | Got: pass

✓ PASS | 8 digits
         String: "#AB12345678 eight digit work item"
         Expected: pass | Got: pass

✓ PASS | 9 digits
         String: "#AB123456789 nine digit work item"
         Expected: pass | Got: pass

✓ PASS | 10 digits (maximum)
         String: "#AB1234567890 ten digit work item"
         Expected: pass | Got: pass

✓ PASS | 6 digits only, no message
         String: "#AB123456"
         Expected: pass | Got: pass

✓ PASS | Edge case: all 9s
         String: "#AB999999 edge case all nines"
         Expected: pass | Got: pass

✓ PASS | Edge case: minimum valid
         String: "#AB100000 edge case minimum 6 digits"
         Expected: pass | Got: pass

✓ PASS | Edge case: maximum valid
         String: "#AB9999999999 edge case maximum 10 digits"
         Expected: pass | Got: pass


--- Invalid Commit Messages (Should be BLOCKED by ruleset) ---

✓ PASS | Example commit B from requirements (only 2 digits)
         String: "#AB75 fix really bad bug"
         Expected: fail | Got: fail

✓ PASS | Example commit C from requirements (no work item)
         String: "#fix really bad bug"
         Expected: fail | Got: fail

✓ PASS | No prefix at all
         String: "fix really bad bug"
         Expected: fail | Got: fail

✓ PASS | Only 5 digits (needs 6-10)
         String: "#AB12345 short work item"
         Expected: fail | Got: fail

✓ PASS | Only 1 digit
         String: "#AB1 too short"
         Expected: fail | Got: fail

✓ PASS | 11 digits (exceeds maximum of 10)
         String: "#AB12345678901 too long"
         Expected: fail | Got: fail

✓ PASS | 12 digits
         String: "#AB123456789012 way too long"
         Expected: fail | Got: fail

✓ PASS | Missing # symbol
         String: "AB123456 no hash symbol"
         Expected: fail | Got: fail

✓ PASS | Lowercase 'ab' instead of 'AB'
         String: "#ab123456 lowercase prefix"
         Expected: fail | Got: fail

✓ PASS | Mixed case 'Ab'
         String: "#Ab123456 mixed case"
         Expected: fail | Got: fail


--- Valid Branch Names (Should be ALLOWED by ruleset) ---

✓ PASS | Example branch A from requirements
         String: "#AB123975-good-branch"
         Expected: pass | Got: pass

✓ PASS | 6 digits with branch name
         String: "#AB123456-feature-branch"
         Expected: pass | Got: pass

✓ PASS | 10 digits (maximum)
         String: "#AB1234567890-release"
         Expected: pass | Got: pass

✓ PASS | With slash (git allows)
         String: "#AB123456-feature/sub-branch"
         Expected: pass | Got: pass


--- Invalid Branch Names (Should be BLOCKED by ruleset) ---

✓ PASS | Example branch B from requirements (only 2 digits)
         String: "#AB75-really-bad-branch"
         Expected: fail | Got: fail

✓ PASS | Example branch C from requirements (no work item)
         String: "#bad-branch"
         Expected: fail | Got: fail

✓ PASS | No prefix at all
         String: "feature-branch"
         Expected: fail | Got: fail

✓ PASS | Only 5 digits
         String: "#AB12345-short"
         Expected: fail | Got: fail

✓ PASS | 11 digits (exceeds maximum)
         String: "#AB12345678901-toolong"
         Expected: fail | Got: fail


--- Edge Cases and Special Scenarios ---

✓ PASS | All zeros (valid work item ID)
         String: "#AB000000 zeros only"
         Expected: pass | Got: pass

✓ PASS | Leading zeros
         String: "#AB000001 mostly zeros"
         Expected: pass | Got: pass

✓ PASS | Second work item in message (first one counts)
         String: "#AB123456#AB789012 multiple work items"
         Expected: pass | Got: pass

✓ PASS | Uppercase branch name
         String: "#AB123456-URGENT-FIX"
         Expected: pass | Got: pass


========================================
Test Results Summary
========================================

Total Tests:  67
Passed:       67
Failed:       0

========================================
ALL TESTS PASSED! ✓
========================================

The regex pattern is working correctly for all test cases.
You can safely deploy the Terraform ruleset.
```

## Quick Test Commands

Once validated, test the actual ruleset behavior:

```bash
# Test with real git commands (after deploying the ruleset)

# These should WORK:
git checkout -b "#AB123456-feature"
git commit -m "#AB123456 add feature"
git push

# These should be BLOCKED:
git checkout -b "feature"
git commit -m "add feature"
git push
# Error: Branch name does not match required pattern

git checkout -b "#AB12-short"
git commit -m "#AB12 add feature"
git push
# Error: Branch name does not match required pattern
```

## Manual Regex Testing

If you want to test the regex manually:

```bash
# Bash one-liner test
echo "#AB123456 test" | grep -qE '^#AB[0-9]{6,10}.*$' && echo "MATCH" || echo "NO MATCH"

# Python one-liner test
python3 -c "import re; print('MATCH' if re.match(r'^#AB\d{6,10}.*$', '#AB123456 test') else 'NO MATCH')"
```

## Integration with CI/CD

Add to your GitHub Actions workflow:

```yaml
name: Validate Work Item Pattern

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Validate pattern
        run: |
          chmod +x validate_workitem_pattern.sh
          ./validate_workitem_pattern.sh
```

## Pattern Summary

| Requirement | Pattern Component | Example |
|-------------|------------------|---------|
| Must start with | `^#AB` | #AB |
| Digit count | `\d{6,10}` | 123456 to 1234567890 |
| Anything after | `.*` | -feature-branch |
| End of string | `$` | |

**Full Pattern**: `^#AB\d{6,10}.*$`

## Common Mistakes to Avoid

❌ **Too few digits**
```
#AB12345 add feature  ← Only 5 digits (needs 6-10)
```

❌ **Too many digits**
```
#AB12345678901 add feature  ← 11 digits (max is 10)
```

❌ **Wrong prefix**
```
#ab123456 add feature  ← Lowercase (must be uppercase AB)
AB123456 add feature   ← Missing # symbol
```

❌ **Pattern not at start**
```
add feature #AB123456  ← Pattern must be at beginning
```

✅ **Correct format**
```
#AB123456 add feature
#AB1234567890-release-branch
```
