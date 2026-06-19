#!/usr/bin/env bash
# 01-download-sonoma.sh
# Downloads the macOS Sonoma 14 full installer via softwareupdate CLI
# Pins to version 14 — will NOT pull Sequoia (15) or Tahoe (26)
set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
NC='\033[0m'

echo "========================================"
echo "  Download macOS Sonoma 14 Installer"
echo "========================================"
echo ""

# Verify we're still on Ventura
CURRENT=$(sw_vers -productVersion)
MAJOR=$(echo "$CURRENT" | cut -d. -f1)
if [[ "$MAJOR" -ne 13 ]]; then
    echo -e "${RED}ERROR:${NC} Expected macOS 13.x, got $CURRENT. Aborting."
    exit 1
fi

INSTALLER="/Applications/Install macOS Sonoma.app"

if [[ -d "$INSTALLER" ]]; then
    echo -e "${YLW}[INFO]${NC} Installer already present at:"
    echo "  $INSTALLER"
    echo ""
    read -rp "Re-download? (y/N): " redownload
    if [[ "${redownload,,}" != "y" ]]; then
        echo "Skipping download. Run ./02-make-bootable-usb.sh or ./03-install-sonoma.sh"
        exit 0
    fi
    echo "Removing existing installer..."
    sudo rm -rf "$INSTALLER"
fi

echo ""
echo "Fetching macOS Sonoma 14 full installer (~13GB)..."
echo "This will take a while. Go make coffee."
echo ""

# --full-installer-version 14 pins to Sonoma 14.x (latest point release)
# No GUI, no Sequoia bleed-through
softwareupdate --fetch-full-installer --full-installer-version 14

echo ""

# Verify
if [[ -d "$INSTALLER" ]]; then
    SIZE=$(du -sh "$INSTALLER" | cut -f1)
    echo -e "${GRN}[PASS]${NC} Installer present: $INSTALLER ($SIZE)"
    echo ""
    echo "Next step: run  ./02-make-bootable-usb.sh  (optional but recommended)"
    echo "      or:  ./03-install-sonoma.sh           (go straight to upgrade)"
else
    echo -e "${RED}[FAIL]${NC} Installer not found after download. Check network and try again."
    exit 1
fi
