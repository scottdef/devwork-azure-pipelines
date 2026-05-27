#!/usr/bin/env python3
"""
sync-workspace-artifacts.py — Synchronize Synapse artifact definitions between workspaces.

Compares a source workspace against one or more target workspaces.  For every
artifact that exists in the source but is missing from a target, the script
exports the full JSON definition from the source and creates an identical copy
in the target.  The copy is structurally identical — same name, same type, same
configuration — even if its connection targets are unreachable in that
environment.  The goal is workspace homogeneity so ARM templates deploy cleanly
by changing only the target workspace name.

Usage:
    python sync-workspace-artifacts.py \\
        --source synapse-workspace-dev \\
        --targets synapse-workspace-test synapse-workspace-prod \\
        --artifact-types linked-service \\
        --dry-run

    python sync-workspace-artifacts.py \\
        --source synapse-workspace-dev \\
        --targets synapse-workspace-test \\
        --artifact-types linked-service dataset \\
        --direction both

Requires:
    - Azure CLI authenticated (az login)
    - Python 3.8+
    - Synapse Administrator RBAC on all workspaces
"""

import argparse
import json
import logging
import os
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

# ─── Logging ────────────────────────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s  %(levelname)-8s  %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("sync")

# ─── Supported artifact types and their az CLI subcommands ──────────────────

ARTIFACT_TYPES = {
    "linked-service": {
        "list_cmd": "az synapse linked-service list",
        "show_cmd": "az synapse linked-service show",
        "create_cmd": "az synapse linked-service create",
        "name_field": "name",
        "file_flag": "--file",
    },
    "dataset": {
        "list_cmd": "az synapse dataset list",
        "show_cmd": "az synapse dataset show",
        "create_cmd": "az synapse dataset create",
        "name_field": "name",
        "file_flag": "--file",
    },
    "pipeline": {
        "list_cmd": "az synapse pipeline list",
        "show_cmd": "az synapse pipeline show",
        "create_cmd": "az synapse pipeline create",
        "name_field": "name",
        "file_flag": "--file",
    },
    "notebook": {
        "list_cmd": "az synapse notebook list",
        "show_cmd": "az synapse notebook show",
        "create_cmd": "az synapse notebook create",
        "name_field": "name",
        "file_flag": "--file",
    },
    "trigger": {
        "list_cmd": "az synapse trigger list",
        "show_cmd": "az synapse trigger show",
        "create_cmd": "az synapse trigger create",
        "name_field": "name",
        "file_flag": "--file",
    },
    "data-flow": {
        "list_cmd": "az synapse data-flow list",
        "show_cmd": "az synapse data-flow show",
        "create_cmd": "az synapse data-flow create",
        "name_field": "name",
        "file_flag": "--file",
    },
    "sql-script": {
        "list_cmd": "az synapse sql-script list",
        "show_cmd": "az synapse sql-script show",
        "create_cmd": "az synapse sql-script create",
        "name_field": "name",
        "file_flag": "--file",
    },
}


# ─── Data structures ───────────────────────────────────────────────────────

@dataclass
class SyncResult:
    """Tracks what happened during sync."""

    source: str
    target: str
    artifact_type: str
    source_count: int = 0
    target_count: int = 0
    missing_in_target: list = field(default_factory=list)
    missing_in_source: list = field(default_factory=list)
    created: list = field(default_factory=list)
    failed: list = field(default_factory=list)
    skipped: list = field(default_factory=list)


# ─── Azure CLI wrapper ─────────────────────────────────────────────────────

def az(cmd: str, check: bool = True) -> Optional[str]:
    """Run an az CLI command and return stdout as string."""
    log.debug(f"  az: {cmd}")
    result = subprocess.run(
        cmd, shell=True, capture_output=True, text=True, timeout=120
    )
    if result.returncode != 0:
        if check:
            log.error(f"  az failed: {result.stderr.strip()}")
            raise RuntimeError(f"az command failed: {cmd}\n{result.stderr}")
        return None
    return result.stdout.strip()


def az_json(cmd: str, check: bool = True) -> Optional[list | dict]:
    """Run an az CLI command and parse JSON output."""
    raw = az(cmd + " -o json", check=check)
    if raw is None:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError as e:
        log.error(f"  Failed to parse JSON: {e}")
        if check:
            raise
        return None


# ─── Core functions ─────────────────────────────────────────────────────────

def list_artifacts(workspace: str, artifact_type: str) -> dict[str, dict]:
    """List all artifacts of a given type in a workspace.  Returns {name: definition}."""
    cfg = ARTIFACT_TYPES[artifact_type]
    cmd = f'{cfg["list_cmd"]} --workspace-name {workspace}'
    items = az_json(cmd, check=False)

    if items is None:
        log.warning(f"  Could not list {artifact_type}s in {workspace}")
        return {}

    result = {}
    for item in items:
        name = item.get("name") or item.get("Name")
        if name:
            result[name] = item
    return result


def export_artifact(workspace: str, name: str, artifact_type: str) -> Optional[dict]:
    """Export a single artifact definition from a workspace."""
    cfg = ARTIFACT_TYPES[artifact_type]
    cmd = f'{cfg["show_cmd"]} --workspace-name {workspace} --name "{name}"'
    definition = az_json(cmd, check=False)

    if definition is None:
        log.warning(f"  Could not export {artifact_type} '{name}' from {workspace}")
        return None

    return definition


def sanitize_definition(definition: dict, artifact_type: str) -> dict:
    """Strip workspace-specific fields that prevent cross-workspace creation.

    The az CLI returns full resource metadata including id, etag, and type
    fields that are specific to the source workspace.  These must be removed
    before creating the artifact in a different workspace.
    """
    # Fields to strip at the top level
    strip_keys = {"id", "etag", "type", "resourceGroup"}

    sanitized = {k: v for k, v in definition.items() if k not in strip_keys}

    # For linked services, strip the properties.connectVia if it references
    # an integration runtime that might not exist in the target.  We keep
    # the reference but log a warning.
    if artifact_type == "linked-service":
        props = sanitized.get("properties", {})
        connect_via = props.get("connectVia", {})
        if connect_via and connect_via.get("referenceName"):
            ir_name = connect_via["referenceName"]
            log.info(f"    Note: references integration runtime '{ir_name}'")
            # We keep the IR reference — if it doesn't exist in the target,
            # the linked service will be non-functional but that's expected
            # per the homogeneity goal.

    return sanitized


def create_artifact(
    workspace: str, name: str, definition: dict, artifact_type: str
) -> bool:
    """Create an artifact in the target workspace from a sanitized definition."""
    cfg = ARTIFACT_TYPES[artifact_type]

    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".json", delete=False, prefix=f"synapse_{name}_"
    ) as f:
        json.dump(definition, f, indent=2)
        tmp_path = f.name

    try:
        cmd = (
            f'{cfg["create_cmd"]} --workspace-name {workspace} '
            f'--name "{name}" {cfg["file_flag"]} @"{tmp_path}"'
        )
        az(cmd)
        return True
    except RuntimeError:
        return False
    finally:
        os.unlink(tmp_path)


def sync_artifacts(
    source: str,
    target: str,
    artifact_type: str,
    direction: str = "source-to-target",
    dry_run: bool = False,
) -> SyncResult:
    """Compare and synchronize artifacts between two workspaces.

    Args:
        source:         Source workspace name.
        target:         Target workspace name.
        artifact_type:  One of the keys in ARTIFACT_TYPES.
        direction:      'source-to-target' — only copy missing items TO target.
                        'target-to-source' — only copy missing items TO source.
                        'both' — bidirectional sync (full homogeneity).
        dry_run:        If True, report differences without creating anything.

    Returns:
        SyncResult with counts and lists of what was/would be created.
    """
    result = SyncResult(
        source=source, target=target, artifact_type=artifact_type
    )

    log.info(f"{'─' * 60}")
    log.info(f"Syncing {artifact_type}:  {source}  →  {target}")
    log.info(f"{'─' * 60}")

    # ── List both sides ──
    log.info(f"  Listing {artifact_type}s in {source}...")
    source_artifacts = list_artifacts(source, artifact_type)
    result.source_count = len(source_artifacts)
    log.info(f"  Found {result.source_count} in source")

    log.info(f"  Listing {artifact_type}s in {target}...")
    target_artifacts = list_artifacts(target, artifact_type)
    result.target_count = len(target_artifacts)
    log.info(f"  Found {result.target_count} in target")

    # ── Compute diffs ──
    source_names = set(source_artifacts.keys())
    target_names = set(target_artifacts.keys())

    missing_in_target = sorted(source_names - target_names)
    missing_in_source = sorted(target_names - source_names)
    common = sorted(source_names & target_names)

    result.missing_in_target = missing_in_target
    result.missing_in_source = missing_in_source

    log.info(f"  Common:            {len(common)}")
    log.info(f"  In source only:    {len(missing_in_target)}")
    log.info(f"  In target only:    {len(missing_in_source)}")

    # ── Source → Target ──
    if direction in ("source-to-target", "both") and missing_in_target:
        log.info("")
        log.info(f"  Creating {len(missing_in_target)} {artifact_type}(s) in {target}:")
        for name in missing_in_target:
            log.info(f"    → {name}")
            if dry_run:
                result.skipped.append(name)
                continue

            definition = export_artifact(source, name, artifact_type)
            if definition is None:
                result.failed.append(name)
                continue

            sanitized = sanitize_definition(definition, artifact_type)
            ok = create_artifact(target, name, sanitized, artifact_type)
            if ok:
                result.created.append(name)
                log.info(f"      ✓ created")
            else:
                result.failed.append(name)
                log.error(f"      ✗ failed")

    # ── Target → Source (bidirectional mode) ──
    if direction in ("target-to-source", "both") and missing_in_source:
        log.info("")
        log.info(f"  Creating {len(missing_in_source)} {artifact_type}(s) in {source}:")
        for name in missing_in_source:
            log.info(f"    → {name}")
            if dry_run:
                result.skipped.append(name)
                continue

            definition = export_artifact(target, name, artifact_type)
            if definition is None:
                result.failed.append(name)
                continue

            sanitized = sanitize_definition(definition, artifact_type)
            ok = create_artifact(source, name, sanitized, artifact_type)
            if ok:
                result.created.append(name)
                log.info(f"      ✓ created")
            else:
                result.failed.append(name)
                log.error(f"      ✗ failed")

    return result


# ─── Reporting ──────────────────────────────────────────────────────────────

def print_report(results: list[SyncResult], dry_run: bool):
    """Print a summary report of all sync operations."""
    print("\n" + "═" * 70)
    print("  WORKSPACE SYNC REPORT")
    if dry_run:
        print("  MODE: DRY RUN — no changes were made")
    print("═" * 70)

    for r in results:
        print(f"\n  {r.artifact_type}:  {r.source}  →  {r.target}")
        print(f"  {'─' * 50}")
        print(f"    Source count:       {r.source_count}")
        print(f"    Target count:       {r.target_count}")
        print(f"    Missing in target:  {len(r.missing_in_target)}")
        print(f"    Missing in source:  {len(r.missing_in_source)}")
        if dry_run:
            print(f"    Would create:       {len(r.skipped)}")
        else:
            print(f"    Created:            {len(r.created)}")
            print(f"    Failed:             {len(r.failed)}")

        if r.missing_in_target:
            label = "Would create" if dry_run else "Created/attempted"
            print(f"\n    {label} in {r.target}:")
            for name in r.missing_in_target:
                status = "●" if name in r.created else ("○" if dry_run else "✗")
                print(f"      {status} {name}")

        if r.missing_in_source:
            print(f"\n    Exist only in {r.target} (not in source):")
            for name in r.missing_in_source:
                print(f"      ◇ {name}")

    print("\n" + "═" * 70)

    # ── GitHub Actions output ──
    if os.environ.get("GITHUB_OUTPUT"):
        total_created = sum(len(r.created) for r in results)
        total_failed = sum(len(r.failed) for r in results)
        total_missing = sum(len(r.missing_in_target) for r in results)
        with open(os.environ["GITHUB_OUTPUT"], "a") as f:
            f.write(f"total_created={total_created}\n")
            f.write(f"total_failed={total_failed}\n")
            f.write(f"total_missing={total_missing}\n")
            f.write(f"sync_status={'success' if total_failed == 0 else 'partial'}\n")

    # ── GitHub Actions summary ──
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as f:
            f.write("## Workspace Sync Report\n\n")
            for r in results:
                f.write(f"### {r.artifact_type}: {r.source} → {r.target}\n")
                f.write(f"| Metric | Count |\n|---|---|\n")
                f.write(f"| Source count | {r.source_count} |\n")
                f.write(f"| Target count | {r.target_count} |\n")
                f.write(f"| Missing in target | {len(r.missing_in_target)} |\n")
                if dry_run:
                    f.write(f"| Would create | {len(r.skipped)} |\n")
                else:
                    f.write(f"| Created | {len(r.created)} |\n")
                    f.write(f"| Failed | {len(r.failed)} |\n")
                if r.missing_in_target:
                    f.write(f"\nArtifacts:\n")
                    for name in r.missing_in_target:
                        marker = "✅" if name in r.created else ("⏸️" if dry_run else "❌")
                        f.write(f"- {marker} `{name}`\n")
                f.write("\n")


# ─── Main ───────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Synchronize Synapse workspace artifacts for deployment homogeneity.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Dry run — see what would be synced
  %(prog)s --source synapse-workspace-dev \\
           --targets synapse-workspace-test synapse-workspace-prod \\
           --artifact-types linked-service --dry-run

  # Sync linked services from dev to test and prod
  %(prog)s --source synapse-workspace-dev \\
           --targets synapse-workspace-test synapse-workspace-prod \\
           --artifact-types linked-service

  # Full bidirectional sync of all artifact types
  %(prog)s --source synapse-workspace-dev \\
           --targets synapse-workspace-test \\
           --artifact-types linked-service dataset pipeline \\
           --direction both

  # Compare only (alias for --dry-run)
  %(prog)s --source synapse-workspace-dev \\
           --targets synapse-workspace-test \\
           --artifact-types linked-service --compare
        """,
    )

    parser.add_argument(
        "--source", required=True, help="Source workspace name"
    )
    parser.add_argument(
        "--targets", required=True, nargs="+", help="Target workspace name(s)"
    )
    parser.add_argument(
        "--artifact-types",
        required=True,
        nargs="+",
        choices=list(ARTIFACT_TYPES.keys()),
        help="Artifact types to synchronize",
    )
    parser.add_argument(
        "--direction",
        choices=["source-to-target", "target-to-source", "both"],
        default="source-to-target",
        help="Sync direction (default: source-to-target)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Report differences without creating artifacts",
    )
    parser.add_argument(
        "--compare",
        action="store_true",
        help="Alias for --dry-run",
    )
    parser.add_argument(
        "--verbose", "-v", action="store_true", help="Enable debug logging"
    )

    args = parser.parse_args()

    if args.verbose:
        log.setLevel(logging.DEBUG)

    dry_run = args.dry_run or args.compare

    if dry_run:
        log.info("DRY RUN — no artifacts will be created")

    # ── Verify az CLI is authenticated ──
    try:
        account = az_json("az account show")
        log.info(f"Azure account: {account.get('name', 'unknown')}")
    except (RuntimeError, TypeError):
        log.error("az CLI is not authenticated.  Run 'az login' first.")
        sys.exit(1)

    # ── Run sync for each target × artifact type ──
    all_results = []
    for target in args.targets:
        for artifact_type in args.artifact_types:
            result = sync_artifacts(
                source=args.source,
                target=target,
                artifact_type=artifact_type,
                direction=args.direction,
                dry_run=dry_run,
            )
            all_results.append(result)

    # ── Report ──
    print_report(all_results, dry_run)

    # ── Exit code ──
    total_failed = sum(len(r.failed) for r in all_results)
    if total_failed > 0:
        log.error(f"{total_failed} artifact(s) failed to create")
        sys.exit(1)

    total_missing = sum(len(r.missing_in_target) for r in all_results)
    if dry_run and total_missing > 0:
        log.info(f"{total_missing} artifact(s) need syncing")
        sys.exit(2)  # non-zero so CI can detect drift


if __name__ == "__main__":
    main()
