# Synapse Workspace Backup & Restore

Database-style backup and restore for Azure Synapse workspaces. The Git branch is the backup. Snapshots are tagged recovery points. Restore deploys a ref to the workspace with full artifact lifecycle management.

## Concepts

| Database term | Synapse equivalent |
|---|---|
| Database | Synapse workspace |
| Backup file | Git branch (dev/test/prod) |
| Point-in-time backup | Snapshot tag (`snapshot/<env>/<timestamp>`) |
| Pre-restore backup | Auto-created tag (`pre-restore/<env>/<timestamp>`) |
| Restore | Full deployment with `DeleteArtifactsNotInTemplate: true` |
| Recovery point | Any tag, branch, or commit SHA |

## Operations

### Snapshot (backup)

Creates a tagged recovery point from the current branch HEAD without deploying anything. Use this before risky changes or on a schedule as a safety net.

```bash
gh workflow run workspace-restore-dev.yml \
  --repo <org>/synapse-repo-dev \
  -f operation=snapshot
```

This creates `snapshot/dev/20260604.143022` pointing at the current `dev` branch HEAD. To restore to this point later, use it as the `restore_ref`.

### Restore

Deploys a branch, tag, or commit to the workspace. Automatically creates a pre-restore snapshot first so the operation is reversible.

```bash
# Restore dev workspace to current dev branch state
gh workflow run workspace-restore-dev.yml \
  --repo <org>/synapse-repo-dev \
  -f operation=restore \
  -f confirm=RESTORE

# Restore dev workspace to a specific snapshot
gh workflow run workspace-restore-dev.yml \
  --repo <org>/synapse-repo-dev \
  -f operation=restore \
  -f restore_ref=snapshot/dev/20260604.143022 \
  -f confirm=RESTORE

# Restore test workspace to its current branch
gh workflow run workspace-restore-test.yml \
  --repo <org>/synapse-repo-test \
  -f operation=restore \
  -f confirm=RESTORE

# Restore prod workspace to a specific release
gh workflow run workspace-restore-prod.yml \
  --repo <org>/synapse-repo-prod \
  -f operation=restore \
  -f restore_ref=release/prod/20260604.100000 \
  -f confirm=RESTORE
```

### Undo a restore

Every restore creates a `pre-restore/<env>/<timestamp>` tag before deploying. To undo:

```bash
# Find the pre-restore snapshot
gh api repos/<org>/synapse-repo-dev/tags --jq '.[].name' | grep pre-restore

# Restore to the pre-restore point
gh workflow run workspace-restore-dev.yml \
  --repo <org>/synapse-repo-dev \
  -f operation=restore \
  -f restore_ref=pre-restore/dev/20260604.150000 \
  -f confirm=RESTORE
```

## What restore deploys

Restore uses the three-phase pipeline (validate → strip → deploy) with `DeleteArtifactsNotInTemplate: true`:

- Every non-linked-service artifact in the workspace is replaced with the version at the restore ref
- Artifacts that existed at the ref but were deleted from the workspace are recreated
- Artifacts in the workspace that don't exist at the ref are deleted
- Linked services are untouched (stripped from the template, managed via UI)
- Triggers are stopped before deployment and restarted after

The workspace matches the restore ref exactly, minus linked services.

## Workflow placement

Each workflow lives in its corresponding repo:

```
synapse-repo-dev/                     synapse-repo-test/
├── .github/workflows/                ├── .github/workflows/
│   └── workspace-restore-dev.yml     │   └── workspace-restore-test.yml
├── scripts/                          ├── scripts/
│   ├── strip-for-rollback.sh         │   ├── strip-for-rollback.sh
│   └── toggle-triggers.sh            │   └── toggle-triggers.sh
└── parameters/                       └── parameters/
    └── dev.parameters.yaml               └── test.parameters.yaml

synapse-repo-prod/
├── .github/workflows/
│   └── workspace-restore-prod.yml
├── scripts/
│   ├── strip-for-rollback.sh
│   └── toggle-triggers.sh
└── parameters/
    └── prod.parameters.yaml
```

## Tag namespace

| Pattern | Created by | Purpose |
|---|---|---|
| `snapshot/<env>/<ts>` | Manual snapshot operation | Point-in-time recovery |
| `pre-restore/<env>/<ts>` | Auto before every restore | Undo safety net |
| `release/<env>/<ts>` | Promotion workflows | Deployment history |
| `promoted/<env>` | Promotion workflows | Last-promoted tracking |

All four tag types are valid `restore_ref` values.

## Scheduled snapshots

For automated backups, add a scheduled trigger to the workflow or create a separate schedule workflow:

```yaml
name: "Scheduled Snapshot"
on:
  schedule:
    - cron: "0 0 * * *"  # daily at midnight UTC
jobs:
  snapshot:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: dev
          fetch-depth: 0
      - run: |
          TAG="snapshot/dev/$(date +'%Y%m%d.%H%M%S')"
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git tag -a "$TAG" -m "Scheduled snapshot"
          git push origin "$TAG"
```

## Listing recovery points

```bash
# All snapshots for dev
gh api repos/<org>/synapse-repo-dev/tags --paginate --jq '.[].name' \
  | grep -E '^(snapshot|pre-restore|release)/' | sort -r | head -20

# Just manual snapshots
gh api repos/<org>/synapse-repo-dev/tags --paginate --jq '.[].name' \
  | grep '^snapshot/' | sort -r

# Inspect a specific tag
git log -1 --format='%h %ci %s' snapshot/dev/20260604.143022
```
