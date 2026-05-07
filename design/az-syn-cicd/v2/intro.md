All three repos are available in the outputs directory with full directory structure preserved. Here's the breakdown of what changed and why:

**The fundamental shift:** instead of one repo with a matrix strategy and multi-environment jobs, each repo is a self-contained deployment unit. 

The `synapse-repo-dev` is the only repo connected to Git/Synapse Studio. 
* Test and prod repos are "deployment-only" — they hold their own Terraform configs, parameter overrides, and scripts, but get their Synapse ARM templates by checking out `synapse-repo-dev` at a specific tag.

**The cross-repo dispatch chain** is the critical wiring. 
* After dev deploys successfully, it tags the commit (`synapse-v<timestamp>`) and fires `github.rest.actions.createWorkflowDispatch()` against `synapse-repo-test/synapse-deploy.yml`, passing the tag as an input. 
* Test does the same toward prod after its own deployment succeeds. The `GH_APP_TOKEN` (from your GitHub App with org owner permissions) authenticates every cross-repo call.

**Secrets are simplified per repo.** Instead of six SPN secrets in one repo, each repo stores only `SYNAPSE_SPN_ID` and `SYNAPSE_SPN_SECRET` for its own environment, plus the shared `AZURE_SUBSCRIPTION_ID`, `AZURE_TENANT_ID`, and `GH_APP_TOKEN`. 

* Blast radius is contained — compromising the test repo's secrets gives zero access to prod.

**File counts across the three repos:**

| Repo | Workflows | Terraform | Scripts | Other | Total |
|---|---|---|---|---|---|
| `synapse-repo-dev` | 5 | 6 | 2 | 6 | 19 |
| `synapse-repo-test` | 5 | 6 | 2 | 3 | 16 |
| `synapse-repo-prod` | 5 | 6 | 2 | 3 | 16 |

* Dev has the extra Synapse Studio config files (`publish_config.json`, `template-parameters-definition.json`) and the README. 
* Terraform files (`.tf`) are identical across all three repos — the differentiation is in `terraform.tfvars` per repo. 
* Scripts are also identical copies since they're small and avoids cross-repo checkout complexity for basic operations.
