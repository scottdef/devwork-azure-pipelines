#!/usr/bin/env bash
# arm.sh: arm or disarm the chief of staff.
#
# While /run/cos/armed exists, gated orders are really performed. Without it
# every action is described and logged but nothing is sent to a target.
#
#   scripts/arm.sh on 15m     arm for 15 minutes, then disarm automatically
#   scripts/arm.sh off        disarm now (also what the kill switch does)
#   scripts/arm.sh status
#
# A physical switch can own the same file instead: run this script from
# whatever watches the switch (for example `gpiomon`), "on" when it closes
# and "off" when it opens. Stopping or restarting cos.service always disarms.
set -euo pipefail

FILE="${COS_ARMED_FILE:-/run/cos/armed}"
case "${1:-status}" in
  on)
    dur="${2:-15m}"
    [[ $dur =~ ^[0-9]+[smh]$ ]] || { echo "duration like 90s, 15m or 2h" >&2; exit 2; }
    touch "$FILE"
    systemctl stop cos-disarm.timer 2>/dev/null || true
    systemd-run --quiet --unit=cos-disarm --on-active="$dur" rm -f "$FILE"
    echo "ARMED for $dur"
    ;;
  off)
    rm -f "$FILE"
    systemctl stop cos-disarm.timer 2>/dev/null || true
    echo "disarmed"
    ;;
  status)
    [[ -e $FILE ]] && echo "ARMED" || echo "disarmed (dry run)"
    ;;
  *) echo "usage: arm.sh on [DURATION] | off | status" >&2; exit 2 ;;
esac
