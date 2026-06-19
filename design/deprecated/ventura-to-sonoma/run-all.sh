#!/usr/bin/env bash
# run-all.sh
# Master orchestrator: runs the full Ventura -> Sonoma upgrade sequence
# Each phase must pass before the next begins
set -euo pipefail

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
BLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

header() {
    echo ""
    echo -e "${BLD}════════════════════════════════════════${NC}"
    echo -e "${BLD}  $*${NC}"
    echo -e "${BLD}════════════════════════════════════════${NC}"
    echo ""
}

run_phase() {
    local script="$1"
    local label="$2"
    header "$label"
    bash "$SCRIPT_DIR/$script"
}

echo ""
echo -e "${BLD}  macOS Ventura 13.7.6 → Sonoma 14${NC}"
echo -e "${BLD}  Full Upgrade Sequence${NC}"
echo ""
echo "  Scripts will run in order:"
echo "    00-pre-flight.sh"
echo "    01-download-sonoma.sh"
echo "    03-install-sonoma.sh     (reboots machine)"
echo ""
echo -e "${YLW}  NOTE:${NC} After reboot, log back in and run:"
echo "    ./04-validate-sonoma.sh"
echo ""
read -rp "Start sequence? (y/N): " go
[[ "${go,,}" == "y" ]] || { echo "Aborted."; exit 0; }

run_phase "00-pre-flight.sh"     "Phase 0: Pre-Flight"
run_phase "01-download-sonoma.sh" "Phase 1: Download Installer"

echo ""
echo -e "${YLW}[OPTIONAL]${NC} Create bootable USB before installing?"
read -rp "(y/N): " usb
if [[ "${usb,,}" == "y" ]]; then
    run_phase "02-make-bootable-usb.sh" "Phase 2: Bootable USB"
fi

run_phase "03-install-sonoma.sh" "Phase 3: Install Sonoma 14"

# If we somehow get here (startosinstall can exit before reboot in some cases)
echo ""
echo -e "${YLW}After the machine reboots and you log back in, run:${NC}"
echo "  cd $(pwd) && ./04-validate-sonoma.sh"
