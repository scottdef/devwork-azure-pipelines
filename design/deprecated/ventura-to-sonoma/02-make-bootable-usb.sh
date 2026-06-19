#!/usr/bin/env bash
# 02-make-bootable-usb.sh
# Burns the Sonoma installer to a USB stick for recovery / multi-machine use
# REQUIRES: 16GB+ USB drive — IT WILL BE COMPLETELY ERASED
set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
NC='\033[0m'

INSTALLER="/Applications/Install macOS Sonoma.app"
CREATEINSTALLMEDIA="$INSTALLER/Contents/Resources/createinstallmedia"

echo "========================================"
echo "  Create Bootable Sonoma 14 USB Drive"
echo "========================================"
echo ""

# Check installer exists
if [[ ! -d "$INSTALLER" ]]; then
    echo -e "${RED}ERROR:${NC} Installer not found at: $INSTALLER"
    echo "Run ./01-download-sonoma.sh first."
    exit 1
fi

# List available disks so user can identify the USB
echo "Available disks (identify your USB drive):"
echo ""
diskutil list
echo ""

read -rp "Enter USB disk identifier (e.g. disk4, NOT disk0): " DISKID

# Sanity check — refuse if user tries to nuke disk0 (internal)
if [[ "$DISKID" == "disk0" || "$DISKID" == "/dev/disk0" ]]; then
    echo -e "${RED}ERROR:${NC} Refusing to write to disk0 — that's your boot drive."
    exit 1
fi

# Normalize to /dev/diskN
DISK="/dev/${DISKID#/dev/}"

echo ""
echo -e "${YLW}WARNING:${NC} This will COMPLETELY ERASE: $DISK"
echo "         Make sure it's the USB stick, not your internal drive."
echo ""
read -rp "Type YES (all caps) to confirm: " confirm

if [[ "$confirm" != "YES" ]]; then
    echo "Aborted. Nothing was written."
    exit 0
fi

echo ""
echo "Creating bootable USB installer..."
echo "This takes 10-20 minutes. Do not unplug the drive."
echo ""

sudo "$CREATEINSTALLMEDIA" \
    --volume "$DISK" \
    --nointeraction

echo ""
echo -e "${GRN}[DONE]${NC} Bootable Sonoma installer ready on $DISK"
echo ""
echo "To use: Reboot, hold Option (⌥) at startup, select the USB drive."
echo "Next step: ./03-install-sonoma.sh  (or use the USB on any compatible Mac)"
