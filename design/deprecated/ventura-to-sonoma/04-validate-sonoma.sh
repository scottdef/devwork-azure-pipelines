#!/usr/bin/env bash
# 04-validate-sonoma.sh
# Post-upgrade validation — run this after the machine comes back up
set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
NC='\033[0m'

FAILED=0

pass() { echo -e "${GRN}[PASS]${NC} $*"; }
warn() { echo -e "${YLW}[WARN]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; FAILED=1; }

echo "========================================"
echo "  Post-Upgrade Validation: Sonoma 14"
echo "========================================"
echo ""

# 1. OS version
echo "--- OS Version ---"
sw_vers
CURRENT=$(sw_vers -productVersion)
BUILD=$(sw_vers -buildVersion)
MAJOR=$(echo "$CURRENT" | cut -d. -f1)
MINOR=$(echo "$CURRENT" | cut -d. -f2)

if [[ "$MAJOR" -eq 14 ]]; then
    pass "macOS Sonoma 14 confirmed ($CURRENT, build $BUILD)"
else
    fail "Expected Sonoma 14.x, got $CURRENT"
fi
echo ""

# 2. SIP still intact
echo "--- System Integrity Protection ---"
SIP=$(csrutil status)
echo "  $SIP"
if echo "$SIP" | grep -q "enabled"; then
    pass "SIP enabled"
else
    warn "SIP disabled — was this intentional?"
fi
echo ""

# 3. Disk health
echo "--- Disk Verify ---"
if diskutil verifyDisk / 2>&1 | grep -q "appears to be OK\|is OK"; then
    pass "Boot disk appears healthy"
else
    warn "diskutil reported an issue — run Disk Utility to check"
fi
echo ""

# 4. Pending software updates
echo "--- Software Updates ---"
UPDATES=$(softwareupdate -l 2>&1)
if echo "$UPDATES" | grep -q "No new software available"; then
    pass "No pending updates — fully up to date"
else
    warn "Pending updates found:"
    echo "$UPDATES" | grep -v "^Software Update Tool\|^Copyright\|^Finding\|^$" || true
    echo "  Run: softwareupdate -ia --verbose"
fi
echo ""

# 5. Homebrew
echo "--- Homebrew ---"
if command -v brew &>/dev/null; then
    BREW_VER=$(brew --version | head -1)
    echo "  $BREW_VER"
    echo "  Updating Homebrew..."
    brew update --quiet && pass "Homebrew updated"
    OUTDATED=$(brew outdated 2>/dev/null | wc -l | xargs)
    if [[ "$OUTDATED" -gt 0 ]]; then
        warn "$OUTDATED outdated formula(e) — run: brew upgrade && brew cleanup"
    else
        pass "All Homebrew formulae up to date"
    fi
else
    echo "  Homebrew not installed — skipping"
fi
echo ""

# 6. Rosetta 2 (Apple Silicon only)
echo "--- Rosetta 2 ---"
ARCH=$(uname -m)
if [[ "$ARCH" == "arm64" ]]; then
    if /usr/bin/pgrep oahd &>/dev/null; then
        pass "Rosetta 2 is running (oahd present)"
    else
        warn "Rosetta 2 not detected — installing..."
        softwareupdate --install-rosetta --agree-to-license && pass "Rosetta 2 installed"
    fi
else
    echo "  Intel Mac — Rosetta 2 not applicable"
fi
echo ""

# 7. XCode CLI tools (may need re-accept after major upgrade)
echo "--- Xcode Command Line Tools ---"
if xcode-select -p &>/dev/null; then
    XCPATH=$(xcode-select -p)
    pass "Xcode CLI tools at: $XCPATH"
else
    warn "Xcode CLI tools not found — run: xcode-select --install"
fi
echo ""

# 8. System report summary
echo "--- System Summary ---"
system_profiler SPSoftwareDataType | grep -E "System Version|Kernel Version|Boot Volume|Computer Name"
echo ""

echo "========================================"
if [[ "$FAILED" -eq 0 ]]; then
    echo -e "${GRN}All validation checks passed.${NC}"
    echo "macOS Sonoma $CURRENT is installed and healthy."
else
    echo -e "${RED}One or more checks failed — review output above.${NC}"
    exit 1
fi
echo "========================================"
