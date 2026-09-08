# Azure DevOps Process GitOps

A GitOps system for managing Azure DevOps Work Item Processes as code. Process definitions are stored in a GitHub repository, validated on every pull request, and promoted through Dev → UAT → Prod environments via branch-based pipelines.

## Architecture Overview

```
                           GitHub Repository
                    ┌─────────────────────────────┐
                    │  processes/base/             │
                    │    process.json              │
                    │    work-item-types/*.json    │
                    │  processes/dev/overrides.json│
                    │  processes/uat/overrides.json│
                    │  processes/prod/overrides.json│
                    │  environments.json           │
                    └──────────┬──────────────────┘
                               │
            ┌──────────────────┼──────────────────┐
            │                  │                   │
       push to dev        push to uat        push to main
            │                  │                   │
            ▼                  ▼                   ▼
    ┌───────────────┐ ┌───────────────┐  ┌────────────────┐
    │ GitHub Action  │ │ GitHub Action  │  │ GitHub Action   │
    │ deploy-dev.yml │ │ promote-uat   │  │ promote-prod    │
    │                │ │               │  │ (requires       │
    │ validate →     │ │ validate →    │  │  approval gate) │
    │ dry-run →      │ │ diff →        │  │ validate →      │
    │ sync           │ │ sync          │  │ diff → dry-run  │
    └───────┬───────┘ └───────┬───────┘  │ → sync          │
            │                  │          └────────┬────────┘
            ▼                  ▼                   ▼
    ┌───────────────┐ ┌───────────────┐  ┌────────────────┐
    │  Azure DevOps  │ │  Azure DevOps  │  │  Azure DevOps   │
    │  Organization: CoolADO                                │
    │                                                       │
    │  ┌─────────┐   ┌─────────┐   ┌──────────┐           │
    │  │ Project: │   │ Project: │   │ Project:  │           │
    │  │  Dev     │   │  UAT     │   │  Prod     │           │
    │  │         │   │         │   │           │           │
    │  │CoolADO  │   │CoolADO  │   │CoolADO    │           │
    │  │Agile-Dev│   │Agile-UAT│   │Agile-Prod │           │
    │  └─────────┘   └─────────┘   └──────────┘           │
    └───────────────────────────────────────────────────────┘
```

## Prerequisites

- **Go 1.21** or later
- **Azure DevOps** Personal Access Token with **Process (Read & Write)** scope
- **GitHub** repository with Actions enabled
- **Azure DevOps Organization** "CoolADO" with three projects: Dev, UAT, Prod

### ADO PAT Permissions

The PAT must have these scopes at minimum:

| Scope | Access | Purpose |
|---|---|---|
| Work Items | Read & Write | Manage processes, WITs, fields, states, rules |
| Project and Team | Read | Enumerate projects for process assignment |

## Quick Start

### 1. Clone and Build

```bash
git clone https://github.com/CoolADO/ado-process-gitops.git
cd ado-process-gitops
go build -o bin/adoproc ./cmd/adoproc/
```

### 2. Set Environment Variables

```bash
export ADO_PAT="your-personal-access-token"
export LOG_LEVEL="info"   # debug | info | warn | error
```

### 3. Initialize (first time only)

```bash
./bin/adoproc init
```

This creates the directory scaffold and a template `environments.json`.

### 4. Validate Configuration

```bash
# Validate all environments
./bin/adoproc validate

# Validate a specific environment
./bin/adoproc validate --env dev
```

### 5. Check Drift

```bash
./bin/adoproc diff --env dev
```

### 6. Apply Changes

```bash
# Dry-run first
./bin/adoproc sync --env dev --dry-run

# Apply for real
./bin/adoproc sync --env dev
```

### 7. Generate Report

```bash
./bin/adoproc report --output reports/drift-report.html
```

This produces an interactive HTML file with bar charts, pie charts, heatmaps, and radar charts powered by go-echarts.

## Repository Structure

```
ado-process-gitops/
├── cmd/adoproc/
│   └── main.go                   # CLI entrypoint with subcommands
├── internal/
│   ├── models/types.go           # All data structures
│   ├── client/ado.go             # Azure DevOps REST API client
│   ├── config/loader.go          # Config file reader + merge logic
│   ├── diff/diff.go              # Desired-vs-actual diff engine
│   ├── sync/sync.go              # Apply changes to ADO
│   ├── export/export.go          # Export live ADO process to files
│   ├── validate/validate.go      # Config validation
│   └── report/report.go          # go-echarts HTML report generator
├── processes/
│   ├── base/                     # Canonical process definition
│   │   ├── process.json          # Process name, parent, description
│   │   └── work-item-types/      # One JSON per WIT
│   │       ├── epic.json
│   │       ├── feature.json
│   │       ├── user-story.json
│   │       ├── task.json
│   │       └── bug.json
│   ├── dev/overrides.json        # Dev-specific additions
│   ├── uat/overrides.json        # UAT-specific additions
│   └── prod/overrides.json       # Prod-specific additions
├── .github/workflows/
│   ├── validate.yml              # PR validation + drift comment
│   ├── deploy-dev.yml            # Auto-deploy on push to dev
│   ├── promote-uat.yml           # Deploy on push to uat
│   └── promote-prod.yml          # Deploy on push to main (gated)
├── environments.json             # Maps env names → ADO targets
├── go.mod
├── Makefile
└── README.md
```

## Configuration Reference

### environments.json

Maps each environment to an ADO project and process name:

```json
{
  "organization": "CoolADO",
  "environments": [
    {
      "name": "dev",
      "org": "CoolADO",
      "project": "Dev",
      "processName": "CoolADOAgile-Dev"
    }
  ]
}
```

### processes/base/process.json

Defines the canonical process identity. The `parentProcessTypeId` is the GUID of the system process this inherits from (Agile = `adcc42ab-9882-485e-a3ed-7678f01f66bc`):

```json
{
  "name": "CoolADOAgile",
  "description": "CoolADO custom Agile process",
  "parentProcessTypeId": "adcc42ab-9882-485e-a3ed-7678f01f66bc",
  "referenceName": "CoolADO.CoolADOAgile"
}
```

### Work Item Type JSON

Each file under `processes/base/work-item-types/` defines one WIT with its custom fields, states, and rules:

```json
{
  "referenceName": "CoolADO.UserStory",
  "name": "User Story",
  "description": "...",
  "color": "009CCC",
  "icon": "icon_book",
  "inherits": "Microsoft.VSTS.WorkItemTypes.UserStory",
  "fields": [ ... ],
  "states": [ ... ],
  "rules": [ ... ]
}
```

**Fields** support these types: `string`, `integer`, `double`, `dateTime`, `html`, `plainText`, `boolean`, `treePath`, `identity`.

**States** require a valid `stateCategory`: `Proposed`, `InProgress`, `Resolved`, `Completed`, or `Removed`.

**Rules** consist of conditions and actions. Supported condition types include `whenCreated`, `whenChanged`, `whenValueEquals`, `whenValueNotEquals`, `whenNot`. Action types include `makeRequired`, `makeReadOnly`, `setDefaultValue`, `copyValue`, `hideTargetField`.

### Environment Overrides

Each `processes/<env>/overrides.json` can modify the base process for that environment:

| Key | Purpose |
|---|---|
| `processDescription` | Override the process description |
| `workItemTypes` | Per-WIT property changes (description, color, isDisabled) |
| `disabledWorkItemTypes` | List of WIT reference names to disable |
| `additionalFields` | Extra fields per WIT for this environment |
| `additionalStates` | Extra states per WIT for this environment |
| `additionalRules` | Extra rules per WIT for this environment |

## CLI Reference

```
adoproc <command> [flags]

Commands:
  export      Export a live ADO process to local JSON files
  validate    Validate local configuration files
  diff        Show differences between local config and remote ADO
  sync        Apply local config to remote ADO (supports --dry-run)
  report      Generate an HTML drift report (go-echarts)
  init        Scaffold a new process config directory
  version     Print build version
```

### Common Flags

| Flag | Default | Description |
|---|---|---|
| `--env` | (varies) | Target environment name |
| `--root` | `.` | Repository root directory |
| `--dry-run` | `false` | Preview without applying (sync only) |
| `--output` | (varies) | Output file or directory |
| `--process` | | Process name for export |
| `--org` | `CoolADO` | ADO organization for export |

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Error (validation failure, API error, etc.) |
| 2 | Drift detected (diff command only) |

## Branching Strategy & Promotion Flow

```
  feature/add-risk-field
         │
         ▼
    ┌─────────┐    PR validates + diff comment
    │   dev   │◄── merge triggers deploy-dev.yml
    └────┬────┘
         │
         ▼
    ┌─────────┐    PR validates + diff comment
    │   uat   │◄── merge triggers promote-uat.yml
    └────┬────┘
         │
         ▼
    ┌─────────┐    PR validates + diff comment
    │  main   │◄── merge triggers promote-prod.yml
    └─────────┘    (requires approval via GitHub Environment)
```

1. Create a feature branch from `dev`
2. Edit process configs in `processes/`
3. Open PR to `dev` → validation runs, drift comment posted
4. Merge to `dev` → auto-deploys to Dev environment
5. Open PR from `dev` to `uat` → validation + drift check
6. Merge to `uat` → auto-deploys to UAT environment
7. Open PR from `uat` to `main` → validation + drift check
8. Merge to `main` → deploys to Prod (after approval)

## GitHub Repository Setup

### 1. Secrets

Add these repository secrets in GitHub Settings → Secrets and Variables → Actions:

| Secret | Value |
|---|---|
| `ADO_PAT` | Azure DevOps Personal Access Token |

### 2. Environments

Create three GitHub Environments in Settings → Environments:

| Environment | Protection Rules |
|---|---|
| `dev` | None (auto-deploy) |
| `uat` | Optional: required reviewers |
| `production` | Required reviewers, deployment branches: `main` only |

### 3. Branch Protection

| Branch | Rules |
|---|---|
| `dev` | Require PR, require status checks (validate job) |
| `uat` | Require PR from dev, require status checks |
| `main` | Require PR from uat, require 1+ approvals, require status checks |

## Exporting an Existing Process

If you already have a process configured manually in ADO, you can export it as a starting point:

```bash
export ADO_PAT="your-pat"

# Export the Dev process
./bin/adoproc export --process "CoolADOAgile-Dev" --output processes-export

# Review the exported files
tree processes-export/

# Copy what you need into processes/base/
cp processes-export/base/process.json processes/base/
cp processes-export/base/work-item-types/*.json processes/base/work-item-types/
```

## ADO REST API Reference

The CLI uses the Azure DevOps Work Item Tracking – Process API (api-version 7.1-preview):

| Resource | Endpoint |
|---|---|
| List processes | `GET /_apis/work/processes` |
| Create process | `POST /_apis/work/processes` |
| Work item types | `GET/POST/PATCH /_apis/work/processes/{id}/workItemTypes` |
| Fields | `GET/POST/PATCH /_apis/work/processes/{id}/workItemTypes/{wit}/fields` |
| States | `GET/POST/PUT /_apis/work/processes/{id}/workItemTypes/{wit}/states` |
| Rules | `GET/POST/PUT/DELETE /_apis/work/processes/{id}/workItemTypes/{wit}/rules` |
| Layout | `GET /_apis/work/processes/{id}/workItemTypes/{wit}/layout` |
| Behaviors | `GET /_apis/work/processes/{id}/behaviors` |

All requests use Basic authentication with the PAT (username is empty, password is the token).

## go-echarts Report

The `report` command generates an interactive HTML file containing four visualizations:

1. **Drift Summary Bar Chart** – WIT counts by status (added/modified/removed/unchanged) per environment
2. **Change Distribution Pie** – aggregate breakdown across all environments
3. **WIT Status Heatmap** – matrix of environment × WIT showing drift intensity
4. **Change Category Radar** – field, state, rule, and WIT changes per environment

Open the generated `.html` file in any browser – no server required.

## Makefile Targets

```bash
make build          # Build the binary
make test           # Run Go tests
make validate       # Validate all environments
make diff ENV=dev   # Show drift for an environment
make sync ENV=dev   # Apply changes
make sync-dry ENV=dev  # Dry-run
make report         # Generate drift report
make export PROCESS=CoolADOAgile-Dev  # Export a process
make help           # Show all targets
```

## Design Decisions

**Single base + per-environment overrides** keeps the canonical process definition DRY while allowing legitimate differences (Dev gets experimental fields, Prod gets compliance rules).

**Branch-per-environment** provides a clear promotion path and audit trail. GitHub's native environment protection rules supply the approval gate for production without requiring an external tool.

**Standard library HTTP client** avoids unnecessary dependencies. The only external package is go-echarts for HTML report generation.

**Exit code 2 for drift** lets CI pipelines distinguish "drift detected" from "error", enabling policies like "block UAT promotion if Dev has unapplied drift."

**Process-per-environment naming** (`CoolADOAgile-Dev`, `CoolADOAgile-UAT`, `CoolADOAgile-Prod`) isolates each environment so changes can be tested on the Dev process without affecting UAT or Prod. Each ADO project is assigned its own version of the inherited process.

## Troubleshooting

**"process not found"** – The process doesn't exist yet in ADO. Run `adoproc sync --env <env>` to create it, or verify the `processName` in `environments.json`.

**"401 Unauthorized"** – Check that `ADO_PAT` is set and the token has Work Items Read & Write scope.

**"inherited states cannot be deleted"** – ADO doesn't allow deleting states inherited from the parent process. Use `HideState` instead (the sync engine handles this automatically for inherited states).

**Validation warnings about unknown WIT references** – An override file references a `referenceName` that doesn't match any file in `work-item-types/`. Check for typos.

## License

Internal tool – CoolADO organization.
