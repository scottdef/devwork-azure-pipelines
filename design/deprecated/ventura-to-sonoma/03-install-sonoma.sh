#!/usr/bin/env bash
# 03-install-sonoma.sh
# Kicks off the macOS Sonoma 14 upgrade from CLI
# Machine WILL reboot automatically — save all work before running
set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
NC='\033[0m'

INSTALLER="/Applications/Install macOS Sonoma.app"
STARTOSINSTALL="$INSTALLER/Contents/Resources/startosinstall"

echo "========================================"
echo "  Install macOS Sonoma 14"
echo "========================================"
echo ""

# 1. Verify we're on Ventura
CURRENT=$(sw_vers -productVersion)
MAJOR=$(echo "$CURRENT" | cut -d. -f1)
if [[ "$MAJOR" -ne 13 ]]; then
    echo -e "${RED}ERROR:${NC} Expected macOS 13.x Ventura, got $CURRENT"
    echo "This script is for Ventura -> Sonoma only."
    exit 1
fi
echo -e "${GRN}[OK]${NC} Confirmed: running macOS $CURRENT (Ventura)"

# 2. Verify installer present
if [[ ! -f "$STARTOSINSTALL" ]]; then
    echo -e "${RED}ERROR:${NC} startosinstall not found at: $STARTOSINSTALL"
    echo "Run ./01-download-sonoma.sh first."
    exit 1
fi
echo -e "${GRN}[OK]${NC} Installer found: $INSTALLER"

# 3. Disk space check
FREE_GB=$(df -g / | awk 'NR==2 {print $4}')
if [[ "$FREE_GB" -lt 35 ]]; then
    echo -e "${RED}ERROR:${NC} Only ${FREE_GB}GB free. Need at least 35GB."
    exit 1
fi
echo -e "${GRN}[OK]${NC} Disk space: ${FREE_GB}GB free"

# 4. Power check
if pmset -g batt 2>/dev/null | grep -q "Battery Power"; then
    PCTG=$(pmset -g batt | grep -Eo '[0-9]+%' | head -1 | tr -d '%')
    if [[ "${PCTG:-0}" -lt 50 ]]; then
        echo -e "${RED}ERROR:${NC} On battery at ${PCTG}%. Plug in AC power before upgrading."
        exit 1
    else
        echo -e "${YLW}[WARN]${NC} On battery at ${PCTG}% — plug in AC power if possible"
    fi
else
    echo -e "${GRN}[OK]${NC} On AC power"
fi

echo ""
echo -e "${YLW}IMPORTANT:${NC}"
echo "  - Save ALL open work NOW — the system will reboot automatically"
echo "  - Upgrade takes 30-60 minutes depending on hardware"
echo "  - The machine will reboot 2-3 times — this is normal"
echo "  - Do NOT power off during upgrade"
echo ""
read -rp "Ready to begin? Type YES to start the upgrade: " confirm

if [[ "$confirm" != "YES" ]]; then
    echo "Aborted. Nothing was changed."
    exit 0
fi

echo ""
echo "Launching Sonoma installer..."
echo "The system will reboot shortly."
echo ""

# --agreetolicense    : accepts EULA non-interactively
# --nointeraction     : no prompts during install
sudo "$STARTOSINSTALL" \
    --agreetolicense \
    --nointeraction

# This line is only reached if startosinstall exits without rebooting (rare)
echo ""
echo "Installer exited. Check for errors above."
echo "If the machine did not reboot, run this script again."
