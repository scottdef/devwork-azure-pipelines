#!/usr/bin/env bash
# 00-pre-flight.sh
# Pre-flight checks before upgrading Ventura 13.7.6 -> Sonoma 14
set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GRN}[PASS]${NC} $*"; }
warn() { echo -e "${YLW}[WARN]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; FAILED=1; }

FAILED=0

echo "========================================"
echo "  Ventura -> Sonoma Pre-Flight Checks"
echo "========================================"
echo ""

# 1. OS Version
echo "--- OS Version ---"
sw_vers
CURRENT=$(sw_vers -productVersion)
MAJOR=$(echo "$CURRENT" | cut -d. -f1)
if [[ "$MAJOR" -eq 13 ]]; then
    pass "Running macOS 13.x Ventura ($CURRENT) — correct baseline"
else
    fail "Expected macOS 13.x, got $CURRENT — wrong baseline"
fi
echo ""

# 2. Hardware compatibility (Sonoma requires 2018+ Intel or any Apple Silicon)
echo "--- Hardware ---"
MODEL=$(system_profiler SPHardwareDataType | grep "Model Identifier" | awk -F': ' '{print $2}' | xargs)
CHIP=$(system_profiler SPHardwareDataType | grep -E "Chip|Processor Name" | head -1 | awk -F': ' '{print $2}' | xargs)
echo "  Model      : $MODEL"
echo "  Chip/CPU   : $CHIP"
pass "Hardware detected — verify Sonoma compatibility at: https://support.apple.com/en-us/111893"
echo ""

# 3. Disk space (need >= 35GB free on /)
echo "--- Disk Space ---"
FREE_GB=$(df -g / | awk 'NR==2 {print $4}')
echo "  Free space on /: ${FREE_GB}GB"
if [[ "$FREE_GB" -ge 35 ]]; then
    pass "Sufficient disk space (${FREE_GB}GB free, 35GB required)"
else
    fail "Insufficient disk space (${FREE_GB}GB free, need 35GB)"
fi
echo ""

# 4. Time Machine backup
echo "--- Time Machine ---"
LAST_BACKUP=$(tmutil latestbackup 2>/dev/null || echo "NONE")
if [[ "$LAST_BACKUP" == "NONE" ]]; then
    warn "No Time Machine backup found — STRONGLY recommend backing up before upgrade"
else
    echo "  Last backup: $LAST_BACKUP"
    pass "Time Machine backup exists"
fi
echo ""

# 5. Power source (laptop must be on AC)
echo "--- Power Source ---"
POWER=$(pmset -g batt | head -2)
echo "$POWER"
if echo "$POWER" | grep -q "AC Power"; then
    pass "On AC power"
elif echo "$POWER" | grep -q "100%"; then
    warn "On battery but fully charged — plug in before starting upgrade"
else
    warn "Not on AC power — plug in before starting upgrade"
fi
echo ""

# 6. SIP status
echo "--- System Integrity Protection ---"
SIP=$(csrutil status)
echo "  $SIP"
if echo "$SIP" | grep -q "enabled"; then
    pass "SIP is enabled (correct)"
else
    warn "SIP is disabled — upgrade will still work but this is unusual"
fi
echo ""

# 7. FileVault
echo "--- FileVault ---"
FV=$(fdesetup status)
echo "  $FV"
if echo "$FV" | grep -q "On"; then
    pass "FileVault is on — upgrade supported, may take longer"
else
    pass "FileVault is off"
fi
echo ""

# 8. Available network
echo "--- Network ---"
if ping -c 1 -W 2 swcdn.apple.com &>/dev/null 2>&1; then
    pass "Apple CDN reachable (swcdn.apple.com)"
else
    warn "Cannot reach Apple CDN — check network before downloading installer"
fi
echo ""

echo "========================================"
if [[ "$FAILED" -eq 0 ]]; then
    echo -e "${GRN}All checks passed. Ready to proceed.${NC}"
    echo "Next step: run  ./01-download-sonoma.sh"
else
    echo -e "${RED}One or more checks FAILED. Fix before proceeding.${NC}"
    exit 1
fi
echo "========================================"
