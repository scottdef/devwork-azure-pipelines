# The Divergent Name Problem in Azure Synapse CI/CD

When linked services, datasets, and other Synapse artifacts have different naming conventions across environments — `dev-db-sql01` in dev, `tst-db-sql01` in test, `prd-db-sql01` in prod — the CI/CD pipeline breaks. This guide documents the problem, compares seven solutions, and provides decision criteria for choosing the right one.

---

## The Problem

Azure Synapse CI/CD works by generating ARM-style templates from the dev workspace and deploying them to downstream environments with parameter overrides. The parameter system can change property values (connection strings, Key Vault URLs, storage endpoints) but it cannot change resource names. A linked service named `dev-db-sql01` in the ARM template will be created as `dev-db-sql01` in every workspace it deploys to — the name is baked into the template structure.

This creates two failure modes. First, pipelines in the target workspace that reference the environment-specific name (`tst-db-sql01`) break because that resource doesn't exist — the template deployed `dev-db-sql01` instead. Second, if `DeleteArtifactsNotInTemplate` is enabled, the deployment deletes `tst-db-sql01` from the target workspace because it isn't in the dev-sourced template, then creates `dev-db-sql01` in its place. Every pipeline reference to the old name becomes a dangling pointer.

The divergent name problem extends beyond linked services. Any Synapse artifact whose name contains an environment identifier creates the same issue: datasets named `DS_dev_customers`, notebooks with environment-prefixed Spark pool references, integration runtime names, and even trigger names. The root cause is always the same: the ARM template carries literal names from dev, and the downstream workspace expects different literal names.

---

## Method Comparison Table

| # | Method | Intervention Point | Rewrites Names | Rewrites Values | Tooling Required | Scope of Change | Ongoing Effort | Best For |
|---|---|---|---|---|---|---|---|---|
| 1 | Environment-agnostic naming | Design-time | N/A (no divergence) | Via parameters | None | One-time migration | None | New projects, clean starts |
| 2 | Parameterize values only | Deploy-time | No | Yes | Parameter YAML | Config only | Low | Quick unblock, cosmetic mismatch OK |
| 3 | Expression-based linked services | Design-time + Runtime | N/A (dynamic) | At runtime | Synapse expressions | Per-linked service | Low | Key Vault, storage connectors |
| 4 | Terraform templatefile() | Build-time | Yes | Yes | Terraform, .tpl files | All templated artifacts | Medium | Teams already using Terraform for artifacts |
| 5 | Terraform azurerm_synapse_linked_service | Build-time | Yes | Yes | Terraform, jq pre-step | Linked services only | Medium | Terraform-managed infrastructure teams |
| 6 | Workspace sync (Go tool) | Pre-deploy | No (copies dev names) | No | Go binary, az CLI | Target workspace | Low | Forcing homogeneity, legacy environments |
| 7 | ARM template rewrite (Go tool) | Pre-deploy | Yes | Yes | Go binary, pattern file | Template JSON | Low | Retrofitting existing divergent names |

---

## Method 1: Environment-Agnostic Naming

Remove environment identifiers from all artifact names. Use `link-svc-db-sql01` everywhere instead of `dev-db-sql01`, `tst-db-sql01`, `prd-db-sql01`. The name stays constant across all workspaces; only the connection properties change per environment via parameter overrides.

This is the approach the parameter system was designed for. Synapse's `template-parameters-definition.json` exposes properties like `connectionString`, `baseUrl`, and `accountName` for environment-specific overrides. The resource name is not a parameter — it's structural. When the name is identical everywhere, the ARM template deploys to any workspace without modification.

The trade-off is a one-time migration cost. Every pipeline, dataset, and data flow that references the old name must be updated. In Synapse Studio, renaming a linked service does not cascade — you must find and update every `referenceName` manually. For a workspace with 40 pipelines referencing `dev-db-sql01`, this means editing 40 pipeline definitions.

When to use: new projects with no existing naming conventions, or teams willing to invest in a one-time rename. This is the only solution with zero ongoing maintenance cost.

---

## Method 2: Parameterize Values Only

Keep the dev names everywhere. Deploy `dev-db-sql01` to test and prod. Override the connection properties so it connects to the correct environment's resources, even though the name carries the `dev-` prefix.

In `test.parameters.yaml`:

```yaml
dev-db-sql01_properties_typeProperties_connectionString: "Server=test-server.database.windows.net;Database=testdb"
```

The linked service is named `dev-db-sql01` on the test workspace but connects to the test database. Every pipeline reference in the template already points to `dev-db-sql01`, so the references resolve. The workspace is functionally correct but cosmetically misleading — a developer opening Synapse Studio on the test workspace sees `dev-db-sql01` and may not realize it connects to test resources.

When to use: immediate unblock while planning a longer-term fix. Acceptable when the team understands the naming mismatch and can tolerate it.

---

## Method 3: Expression-Based Dynamic Linked Services

Parameterize the linked service definition itself so environment-specific values are resolved at runtime rather than at deploy time. The linked service name is environment-agnostic; the target resource is determined by a parameter passed at pipeline execution time or defaulted per environment.

```json
{
    "name": "LS_KeyVault",
    "properties": {
        "parameters": {
            "KeyVaultName": {
                "type": "string",
                "defaultValue": "kv-synapse-dev"
            }
        },
        "type": "AzureKeyVault",
        "typeProperties": {
            "baseUrl": {
                "value": "@{concat('https://', linkedService().KeyVaultName, '.vault.azure.net/')}",
                "type": "Expression"
            }
        }
    }
}
```

The linked service name is `LS_KeyVault` in all environments. The `KeyVaultName` parameter defaults to the dev value but is overridden per environment via the parameter YAML at deploy time. This works well for Key Vault and Azure Blob Storage linked services where the only difference is a name or URL component. It does not work for all connector types — some `typeProperties` fields do not support expression syntax.

When to use: Key Vault linked services, storage accounts, and any connector where the only environment difference is a single URL component or account name.

---

## Method 4: Terraform templatefile()

Define Synapse artifact JSON files as Terraform templates (`.tpl`) with variable interpolation. Terraform renders the templates per environment, writing the output to the filesystem with environment-correct names. The Synapse deployment action then deploys the rendered files instead of the raw dev artifacts.

A linked service template:

```hcl
# templates/linkedService/LS_Database.json.tpl
{
    "name": "${ls_name}",
    "properties": {
        "type": "AzureSqlDatabase",
        "typeProperties": {
            "connectionString": "${connection_string}"
        }
    }
}
```

Terraform renders it per environment via `terraform.tfvars`:

```hcl
linked_services = {
  LS_Database = {
    name              = "link-svc-tst-db-sql01"
    connection_string = "Server=test-server.database.windows.net"
  }
}
```

The critical caveat: every artifact that contains a `referenceName` to a divergently-named linked service must also be a template. If 40 pipelines reference the linked service, all 40 become `.tpl` files. The template tree grows proportionally to the number of cross-references.

When to use: teams already managing Synapse infrastructure through Terraform who want a single rendering engine for all environment-specific content.

---

## Method 5: Terraform azurerm_synapse_linked_service

Manage linked services directly as Terraform resources instead of through the Synapse deployment action. Terraform creates linked services with environment-correct names natively. The Synapse deployment action handles everything else (pipelines, notebooks, datasets, triggers).

```hcl
resource "azurerm_synapse_linked_service" "database" {
  name                 = "link-svc-${var.environment}-db-sql01"
  synapse_workspace_id = data.azurerm_synapse_workspace.this.id
  type                 = "AzureSqlDatabase"
  type_properties_json = jsonencode({
    connectionString = var.db_connection_string
  })
}
```

This creates a conflict: both Terraform and the deployment action try to manage linked services. The solution is to strip linked services from the ARM template before deploying with a `jq` pre-step, and to rewrite pipeline `referenceName` fields with `sed` to match the Terraform-managed names. This dual-ownership model works but requires careful coordination between the two systems.

When to use: teams that want Terraform to own the linked service lifecycle and are comfortable with the `jq` + `sed` pre-processing step.

---

## Method 6: Workspace Sync (Go Tool)

Instead of making the ARM template match the target workspace, make the target workspace match the ARM template. Before deployment, compare the source and target workspaces and create copies of any missing artifacts in the target. The copies are structurally identical — same name, same configuration — even if their connection targets are unreachable in that environment. The ARM template then deploys cleanly because every name it references already exists.

```bash
./sync-workspace-artifacts \
  -source synapse-workspace-dev \
  -targets synapse-workspace-test,synapse-workspace-prod \
  -types linked-service
```

The sync tool uses `az synapse linked-service list` to enumerate both sides, computes the set difference, exports the full JSON definition from the source, strips workspace-specific metadata (`id`, `etag`), and creates the copy in the target via `az synapse linked-service create`. The copied linked service is non-functional (it points at dev resources), but the subsequent ARM template deployment overwrites its properties via parameter overrides.

Over time, as all workspaces converge to the same set of artifact names, the ARM templates become fully portable. This is a convergent approach — it doesn't solve the problem instantly but eliminates it incrementally.

When to use: legacy environments with established divergent naming where renaming is politically or operationally expensive. The tool creates homogeneity without requiring anyone to rename anything in Synapse Studio.

---

## Method 7: ARM Template Rewrite (Go Tool)

Transform the ARM template itself before deployment. A pattern file of match/replace pairs drives substring substitution across every string value and map key in the JSON tree. Resource names, `referenceName` fields, connection strings, parameter keys — anything containing a matching substring gets rewritten.

Pattern file (`test-workspace-patterns.txt`):

```
dev-db-sql01=tst-db-sql01
link-svc-dev-kv=link-svc-tst-kv
devdatalake=testdatalake
synapse-workspace-dev=synapse-workspace-test
```

```bash
./rewrite-arm-templates \
  -patterns patterns/test-workspace-patterns.txt \
  -template TemplateForWorkspace.json \
  -params   TemplateParametersForWorkspace.json
```

The tool parses the JSON into a generic tree, walks every node recursively, and applies `strings.ReplaceAll` for each pattern. It also rewrites map keys, which matters for `TemplateParametersForWorkspace.json` where parameter names encode linked service names (e.g. `dev-db-sql01_properties_typeProperties_connectionString`).

Pattern ordering matters: longer, more specific patterns should precede shorter, broader ones to avoid partial corruption. Each environment maintains its own pattern file, version-controlled alongside the deployment configuration.

When to use: retrofitting existing divergent names without modifying Synapse Studio, without creating copies in target workspaces, and without any Azure API calls. The rewrite operates entirely on local files before deployment.

---

## Decision Matrix

The right method depends on where you are in the project lifecycle and what constraints you operate under.

**Can you rename artifacts in Synapse Studio?**

If yes, use Method 1 (environment-agnostic naming). It has zero ongoing cost and eliminates the problem at its source. Every other method is a workaround for not doing this.

**Do you need a solution today without touching Synapse Studio?**

If the team needs to ship deployments immediately, Method 2 (parameterize values only) gets you unblocked in minutes. The `dev-` prefix on the test workspace is cosmetically wrong but functionally correct. Pair this with a planned migration to Method 1 when capacity allows.

**Is the naming divergence limited to Key Vault and storage linked services?**

Method 3 (expression-based) handles these connector types cleanly. The linked service name is environment-agnostic; only the target URL varies at runtime. This doesn't help with SQL Server linked services, custom connectors, or anything where the expression syntax isn't supported.

**Does your team manage Synapse through Terraform?**

Methods 4 and 5 integrate the name transformation into the Terraform workflow. Method 4 (templatefile) is more comprehensive — it rewrites everything, including pipeline references. Method 5 (TF resource) is simpler but requires a `jq` pre-step to prevent Terraform and the deployment action from fighting over linked service ownership.

**Do you want the workspaces themselves to become homogeneous?**

Method 6 (workspace sync) creates copies of missing artifacts so all workspaces contain the same set of names. The copies are non-functional but make the ARM template portable. This is convergent — run it repeatedly and the workspaces align. The sync tool also serves as a drift detection mechanism.

**Do you want to transform the template without touching the workspaces?**

Method 7 (ARM template rewrite) is purely local. It rewrites the JSON files on the runner's filesystem before the deployment action sees them. No Azure API calls, no workspace modifications, no Terraform. Just a pattern file and a Go binary. This is the lowest-friction solution for teams that can't or won't modify the workspaces directly.

---

## Combined Approaches

These methods are not mutually exclusive. Common combinations:

Method 2 + Method 1 migration: use parameterize-only as an immediate fix, then systematically rename artifacts over sprint cycles until all names are environment-agnostic.

Method 7 + Method 2: rewrite resource names in the template (Method 7) and override connection values via parameter YAML (Method 2). The rewrite handles names; the parameters handle properties. Together they cover both dimensions of the divergence.

Method 6 + Method 7: sync workspaces to create missing artifacts (Method 6), then rewrite the template to use the target's naming conventions (Method 7). Belt and suspenders — the workspace has all the names, and the template uses the right ones.

Method 5 + Method 7: let Terraform own linked services with correct per-environment names (Method 5), rewrite pipeline references in the ARM template to match (Method 7), and let the deployment action handle everything else.

---

## Recommendations by Scenario

**Starting a new Synapse project**: Method 1. Establish environment-agnostic naming conventions from day one. There is no reason to encode environment tokens in artifact names.

**Inheriting an existing project with divergent names**: Method 7 (ARM template rewrite) for immediate deployability, combined with a gradual migration to Method 1. The rewrite tool handles the divergence today; the rename migration eliminates it permanently.

**Enterprise with strict change control**: Method 6 (workspace sync) to create homogeneity without modifying existing artifacts, then Method 2 (parameterize only) to handle connection properties. No artifact is renamed, no template is rewritten — the workspaces simply gain additional (non-functional) copies of the artifacts they're missing.

**Terraform-native team**: Method 4 (templatefile) for teams that want Terraform to be the single source of truth for all artifact content. Accept the template tree growth as the cost of centralized control.
