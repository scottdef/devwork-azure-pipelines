# Synapse Delta Deployment

Incremental CI/CD for Azure Synapse Analytics. Deploys only changed artifacts instead of the full workspace template. Each promotion stage deploys the exact delta since the last successful deployment to that environment. Linked services are excluded entirely and managed through Synapse Studio UI.

## Flow

```
feature/* ──PR──► dev ──PR──► test ──PR──► prod
               │           │            │
          deploy delta  deploy delta  deploy delta
          (PR changes)  (since tag)   (since tag)
```

**PR to dev:** generates an ARM template containing only the artifacts changed in the PR, validates, and deploys them to the dev workspace.

**PR from dev to test:** diffs the dev branch against the `promoted/test` tag to find everything merged since the last test promotion. Generates an ARM template containing only that delta, validates, and deploys to test. Moves the `promoted/test` tag forward on success.

**PR from test to prod:** same pattern — diffs against `promoted/prod`, deploys the delta, moves the tag.

## How the four-phase pipeline works

Every deployment runs the same sequence:

1. **Validate** — the `Azure/synapse-workspace-deployment` action runs `operation: validate` on the full repo. This generates a complete ARM template in `ExportedArtifacts/`. We need the full context because the validate operation resolves cross-references between artifacts.

2. **Filter** — `filter-and-strip.sh` takes the complete template and a list of changed files (from `git diff`). It keeps only the resources matching changed artifact names. It then strips all linked services (UI-managed), strips all `dependsOn` entries (prevents dangling-reference errors), and cleans orphaned parameters.

3. **Deploy** — the action runs `operation: deploy` with the filtered template and `DeleteArtifactsNotInTemplate: false`. Only the changed resources are created or updated. Everything else in the workspace is untouched.

4. **Tag** — for promotions, the `promoted/<env>` tag is moved to the current commit. A permanent release tag (`release/<env>/<timestamp>`) is also created for rollback.

## Why dependsOn is stripped entirely

In a delta template, most `dependsOn` entries reference resources that aren't in the template — they were filtered out because they didn't change. The deployment action's dependency resolver fails on any dangling reference with "Could not figure out full dependency model."

Since we deploy with `DeleteArtifactsNotInTemplate: false`, the workspace already contains all the dependencies. Deployment ordering is irrelevant — we're updating a subset of an existing workspace, not creating one from scratch. Stripping all `dependsOn` is safe and eliminates the class of errors entirely.

## Promotion tracking

| Tag | Branch | Meaning |
|---|---|---|
| `promoted/test` | dev | Last dev commit deployed to test |
| `promoted/prod` | test | Last test commit deployed to prod |
| `release/test/<ts>` | dev | Permanent rollback point for test |
| `release/prod/<ts>` | test | Permanent rollback point for prod |

The `promoted/*` tags are force-updated after each deployment. On first run (no tag), all artifacts are included.

## Repository layout

```
synapse-repo/
├── .github/workflows/
│   ├── deploy-dev.yml       # PR to dev → delta deploy
│   ├── promote-test.yml     # PR dev→test → delta promotion
│   └── promote-prod.yml     # PR test→prod → delta promotion
├── scripts/
│   ├── filter-and-strip.sh  # Filter + strip linked svcs + clean dependsOn
│   └── toggle-triggers.sh   # Stop/start triggers
├── parameters/
│   ├── dev.parameters.yaml
│   ├── test.parameters.yaml
│   └── prod.parameters.yaml
├── pipeline/
├── dataset/
├── linkedService/
├── notebook/
├── trigger/
├── dataflow/
└── sqlscript/
```

## Secrets per environment

| Secret | Description |
|---|---|
| `AZURE_SUBSCRIPTION_ID` | Azure subscription |
| `AZURE_TENANT_ID` | Azure AD tenant |
| `SYNAPSE_SPN_ID` | Service principal client ID |
| `SYNAPSE_SPN_SECRET` | Service principal client secret |

## Limitations

**No automatic artifact deletion.** With `DeleteArtifactsNotInTemplate: false`, removing an artifact from the repo does not remove it from the workspace. Clean up orphaned artifacts manually through Synapse Studio.

**Linked services are UI-only.** Each workspace maintains its own linked services configured through Synapse Studio. Create new linked services in all workspaces before merging pipelines that reference them.

**First promotion deploys everything.** When no `promoted/*` tag exists, the workflow treats all artifacts as changed. Subsequent promotions deploy only the delta.
