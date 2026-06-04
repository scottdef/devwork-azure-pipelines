#!/usr/bin/env python3
"""
compare-flags.py — Compare app-feature-flags.json against live Azure App Configuration.

Exits 0 on success (even if differences found — differences are informational).
Exits 1 on fatal errors (missing files, az cli failures).

Usage:
    python3 compare-flags.py \
        --json-file ../app-feature-flags.json \
        --app-config-name myappconfig \
        --resource-group myrg \
        [--label dev]
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path


# ── ANSI colors (gracefully degrade in non-tty) ──────────────────────────
class C:
    RED = "\033[0;31m"
    GREEN = "\033[0;32m"
    YELLOW = "\033[0;33m"
    CYAN = "\033[0;36m"
    BOLD = "\033[1m"
    NC = "\033[0m"


def az_list_feature_flags(
    app_config_name: str,
    resource_group: str,
    label: str | None = None,
) -> list[dict]:
    """Fetch feature flags from Azure App Configuration via az cli."""
    cmd = [
        "az", "appconfig", "feature", "list",
        "--name", app_config_name,
        "--resource-group", resource_group,
        "--output", "json",
    ]
    if label:
        cmd.extend(["--label", label])

    result = subprocess.run(cmd, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        print(f"{C.RED}ERROR:{C.NC} az appconfig feature list failed:", file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        sys.exit(1)

    return json.loads(result.stdout)


def load_json_flags(path: Path) -> dict[str, bool]:
    """Load the intent file into a {name: enabled} map."""
    if not path.is_file():
        print(f"{C.RED}ERROR:{C.NC} {path} not found", file=sys.stderr)
        sys.exit(1)

    with open(path) as f:
        raw = json.load(f)

    return {entry["name"]: entry["enabled"] for entry in raw}


def compare(
    json_flags: dict[str, bool],
    live_flags: list[dict],
) -> None:
    """Compare and print a diff report."""
    # Build live map: name → enabled (bool)
    live_map: dict[str, bool] = {}
    for flag in live_flags:
        # az appconfig feature list returns "name" and "state" (on/off) or "enabled" (bool)
        name = flag.get("name", "")
        # Handle both v1 ("state": "on"/"off") and v2 ("enabled": true/false) schemas
        if "enabled" in flag:
            enabled = flag["enabled"]
        elif "state" in flag:
            enabled = flag["state"].lower() == "on"
        else:
            enabled = False
        live_map[name] = enabled

    # ── Flags in live App Config but NOT in JSON ──────────────────────
    orphaned = set(live_map.keys()) - set(json_flags.keys())
    if orphaned:
        print(f"\n{C.YELLOW}{'─' * 60}{C.NC}")
        print(f"{C.YELLOW}Flags present in App Configuration but NOT in app-feature-flags.json:{C.NC}")
        print(f"{C.YELLOW}{'─' * 60}{C.NC}")
        for name in sorted(orphaned):
            state = "enabled" if live_map[name] else "disabled"
            print(f"  {C.CYAN}•{C.NC} {C.BOLD}{name}{C.NC} (currently {state}) — {C.YELLOW}NOT PRESENT{C.NC} in JSON")

    # ── Flags in JSON — check for drift ───────────────────────────────
    updates: list[tuple[str, bool, bool]] = []
    creates: list[tuple[str, bool]] = []

    for name, desired in sorted(json_flags.items()):
        if name not in live_map:
            creates.append((name, desired))
        elif live_map[name] != desired:
            updates.append((name, live_map[name], desired))

    if creates:
        print(f"\n{C.GREEN}{'─' * 60}{C.NC}")
        print(f"{C.GREEN}Flags to CREATE (new in App Configuration):{C.NC}")
        print(f"{C.GREEN}{'─' * 60}{C.NC}")
        for name, desired in creates:
            state = "enabled" if desired else "disabled"
            print(f"  {C.GREEN}+{C.NC} {C.BOLD}{name}{C.NC} → {state}")

    if updates:
        print(f"\n{C.CYAN}{'─' * 60}{C.NC}")
        print(f"{C.CYAN}Flags to UPDATE (value differs):{C.NC}")
        print(f"{C.CYAN}{'─' * 60}{C.NC}")
        for name, current, desired in updates:
            cur = "enabled" if current else "disabled"
            des = "enabled" if desired else "disabled"
            print(f"  {C.CYAN}~{C.NC} {C.BOLD}{name}{C.NC}: {C.RED}{cur}{C.NC} → {C.GREEN}{des}{C.NC}")

    # ── Summary ───────────────────────────────────────────────────────
    unchanged = len(json_flags) - len(creates) - len(updates)
    print(f"\n{C.BOLD}Summary:{C.NC} "
          f"{len(creates)} create, "
          f"{len(updates)} update, "
          f"{unchanged} unchanged, "
          f"{len(orphaned)} not in JSON")


def main() -> None:
    parser = argparse.ArgumentParser(description="Compare feature flags: JSON vs App Configuration")
    parser.add_argument("--json-file", required=True, help="Path to app-feature-flags.json")
    parser.add_argument("--app-config-name", required=True, help="Azure App Configuration instance name")
    parser.add_argument("--resource-group", required=True, help="Azure resource group")
    parser.add_argument("--label", default=None, help="Optional label filter")
    args = parser.parse_args()

    print(f"{C.BOLD}Comparing feature flags...{C.NC}")
    print(f"  JSON file:    {args.json_file}")
    print(f"  App Config:   {args.app_config_name}")
    print(f"  Label:        {args.label or '(none)'}")

    json_flags = load_json_flags(Path(args.json_file))
    live_flags = az_list_feature_flags(args.app_config_name, args.resource_group, args.label)

    compare(json_flags, live_flags)


if __name__ == "__main__":
    main()
