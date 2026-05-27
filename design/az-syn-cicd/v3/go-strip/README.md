# Three-Phase Synapse Deployment: Validate → Strip → Deploy

A deployment strategy that excludes linked services from CI/CD entirely. Linked services are created and managed exclusively through Synapse Studio UI. The pipeline deploys everything else — pipelines, datasets, notebooks, triggers, data flows, SQL scripts — without ever touching linked service definitions.

This eliminates the divergent name problem for linked services completely. Each workspace maintains its own linked services with environment-appropriate names and connection strings. The ARM template never carries linked service resources, so there's nothing to mismatch.

## How it works

```
Phase 1: Validate
  ┌─────────────────┐         ┌────────────────────────────────────┐
  │ Collaboration    │ ──────► │ Azure/synapse-workspace-deployment │
  │ branch artifacts │  (JSON) │ operation: "validate"              │
  │ pipeline/        │         └──────────────┬─────────────────────┘
  │ dataset/         │                        │
  │ notebook/        │                        ▼
  │ trigger/         │         ┌────────────────────────────────────┐
  │ linkedService/   │         │ ExportedArtifacts/                 │
  └─────────────────┘         │   TemplateForWorkspace.json        │
                              │   TemplateParametersForWorkspace   │
                              └──────────────┬─────────────────────┘
                                             │
Phase 2: Strip                               ▼
                              ┌────────────────────────────────────┐
                              │ strip-linked-services              │
                              │   removes /linkedServices resources│
                              │   removes LS parameter keys        │
                              └──────────────┬─────────────────────┘
                                             │
Phase 3: Deploy                              ▼
                              ┌────────────────────────────────────┐
                              │ Azure/synapse-workspace-deployment │
                              │ operation: "deploy"                │
                              │ OverrideArmParameters: env.yaml    │
                              │ DeleteArtifactsNotInTemplate: true │
                              └────────────────────────────────────┘
```

`validate` reads the individual artifact JSON files from the collaboration branch and generates consolidated ARM templates in `ExportedArtifacts/`. This is the same template generation that Synapse Studio does when you click Publish, but it runs in CI instead.

`strip-linked-services` then removes all resources whose type ends with `/linkedServices` from `TemplateForWorkspace.json`, and removes any parameter keys in `TemplateParametersForWorkspace.json` that belong to those linked services. The linked service JSON files in `linkedService/` are still in the repo (Synapse Studio needs them for Git sync), but they never reach the deployment.

`deploy` applies the stripped template to the target workspace. Because linked services aren't in the template, the deployment action never creates, updates, or deletes them. They remain exactly as configured in each workspace's Synapse Studio UI.

## Why validate then deploy instead of validateDeploy

`validateDeploy` is a single-step operation — it validates the artifacts and deploys in one pass. There's no intermediate template you can transform. By splitting into `validate` (generate templates) and `deploy` (apply templates), we get a window between the two where the strip tool can modify the templates before deployment.

This is the same pattern as compiling code: validate is the build step, strip is a post-build transform, deploy is the release step. Separating build from release lets you insert any transformation.

## What DeleteArtifactsNotInTemplate does with stripped templates

When `DeleteArtifactsNotInTemplate: true` is set and linked services are stripped from the template, the deployment action treats linked services as outside its scope. It only manages artifact types that appear in the template. Since no linked services are in the template, the action doesn't create, update, or delete any linked services — they're invisible to the deployment.

This is the critical behavior that makes the strategy work. Pipelines, datasets, notebooks, and triggers in the template are deployed with full lifecycle management (create, update, delete). Linked services are completely untouched.

## Repository layout

Add the Go tool to each repo under `tools/strip-linked-services/`:

```
synapse-repo-<env>/
├── tools/
│   └── strip-linked-services/
│       ├── main.go
│       └── go.mod
├── scripts/
│   ├── strip-linked-services.sh    # bash/jq alternative (optional)
│   └── toggle-triggers.sh
├── parameters/
│   └── <env>.parameters.yaml
├── .github/workflows/
│   └── synapse-deploy.yml          # three-phase workflow
└── ...
```

## Tools

### Go tool (recommended)

Build:

```bash
cd tools/strip-linked-services
go build -o strip-linked-services .
```

Usage:

```bash
# Strip in place
./strip-linked-services \
  -template ExportedArtifacts/TemplateForWorkspace.json \
  -params   ExportedArtifacts/TemplateParametersForWorkspace.json

# Dry run
./strip-linked-services \
  -template ExportedArtifacts/TemplateForWorkspace.json \
  -params   ExportedArtifacts/TemplateParametersForWorkspace.json \
  -dry-run
```

### Bash/jq alternative

No compilation required. Uses `jq` (pre-installed on GitHub runners):

```bash
./scripts/strip-linked-services.sh \
  ExportedArtifacts/TemplateForWorkspace.json \
  ExportedArtifacts/TemplateParametersForWorkspace.json
```

Both tools produce identical results. The Go tool provides structured logging, GitHub Actions integration (outputs + step summary), and a dry-run mode. The bash script is simpler for ad-hoc use.

## Workflows

Three workflow files are provided, one per repo:

| File | Repository | Trigger |
|---|---|---|
| `dev-synapse-deploy.yml` | `synapse-repo-dev` | Push to main (artifact paths), manual dispatch |
| `test-synapse-deploy.yml` | `synapse-repo-test` | `workflow_dispatch` from dev repo |
| `prod-synapse-deploy.yml` | `synapse-repo-prod` | `workflow_dispatch` from test repo |

Each workflow runs the same three-phase pipeline independently. Test and prod check out `synapse-repo-dev` at the source tag, regenerate the ARM templates via `validate`, strip linked services, then deploy with their own environment's parameter overrides.

## Parameter files

Since linked services are excluded from the template, the parameter YAML files no longer need linked service overrides. They only contain parameters for the remaining artifact types:

```yaml
# parameters/test.parameters.yaml
workspaceName: synapse-workspace-test
```

Linked service connection strings, Key Vault URLs, and storage endpoints are configured directly in each workspace's Synapse Studio. They never appear in the CI/CD pipeline.

## Managing linked services across environments

With linked services excluded from CI/CD, each environment's linked services are independent:

1. Create linked services in each workspace through Synapse Studio UI
2. Name them according to each environment's conventions (divergent names are fine)
3. Configure connection strings, Key Vault references, and storage endpoints per environment
4. Pipelines reference linked services by name — the names must match within each workspace but can differ across workspaces

This is the trade-off: you gain complete freedom from the divergent name problem, but you lose automated linked service deployment. Creating a new linked service requires manual action in each workspace. For teams where linked services change infrequently (the common case — most changes are to pipelines and notebooks), this is a net win.

## Limitations

Linked services referenced by pipelines must exist in the target workspace before deployment. If a pipeline references `LS_NewDatabase` and that linked service doesn't exist in the test workspace, the deployment will succeed (the template doesn't validate references) but the pipeline will fail at runtime. Coordinate new linked service creation across workspaces before merging pipeline changes that reference them.

The `validate` operation requires that `linkedService/` JSON files exist in the collaboration branch even though they won't be deployed. Synapse Studio needs them for Git sync and the validate operation reads them to generate a complete template. After stripping, the linked service resources are removed from the template, but the files must be present for validation to succeed.
