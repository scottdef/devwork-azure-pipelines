# C. Microsoft Foundry model deployment (`foundry-model-deployment`)

This request deploys, or resizes, an **allow-listed model** on an **approved Foundry resource** for use by agentic workflows. The deployment is done with Bicep, from GitHub Actions through Azure OIDC. The `az` pre-flight checks model and SKU availability, quota, and name collisions before anything changes.

- Form: [.github/ISSUE_TEMPLATE/foundry-model-deployment.yml](../../.github/ISSUE_TEMPLATE/foundry-model-deployment.yml)
- Handler: [internal/foundry](../../internal/foundry/foundry.go)
- Template: [deploy/bicep/model-deployment.bicep](../../deploy/bicep/model-deployment.bicep)
- Workflow: [execute-foundry-deploy.yml](../../.github/workflows/execute-foundry-deploy.yml)
- Examples: [examples/foundry-model-deployment](../../examples/foundry-model-deployment/)

## Form fields

| Field id | Notes |
|---|---|
| `foundry_account` | One of the registry `accounts`. Each maps to an environment (dev, uat, prod), resource group and region |
| `foundry_model`, `foundry_model_version`, `foundry_model_format` | Must match an allow-listed model: name, versions, publisher format |
| `foundry_sku` | `GlobalStandard`, `DataZoneStandard`, `Standard`, `GlobalProvisionedManaged`, `DataZoneProvisionedManaged`, `ProvisionedManaged`, `GlobalBatch`, `DataZoneBatch` (must be allowed for the model) |
| `foundry_capacity` | Thousands of TPM for standard SKUs (1 = 1K TPM); PTUs for provisioned SKUs. Capped per model and environment |
| `foundry_deployment_name` | Optional. Defaults to `<model>-<environment>`. Pattern `^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$` |
| `foundry_version_upgrade` | `OnceNewDefaultVersionAvailable`, `OnceCurrentVersionExpired`, `NoAutoUpgrade` |
| `foundry_use_case`, `foundry_consumers` | Intended agentic use and consuming repositories or services (checked for secrets) |
| `foundry_cost_center`, `foundry_review_date` | Chargeback, plus a review date within 12 months |

## Approval policy (default)

| Rule | When | Who |
|---|---|---|
| *(auto-approve)* | `$environment == dev` **and** SKU in {GlobalStandard, DataZoneStandard} **and** capacity ≤ 10 | nobody; `.submit` executes |
| Platform AI team | otherwise | `@CoolEngOrg/platform-ai` |
| AI governance (security) | environment `prod` | `@CoolEngOrg/ai-governance` **or** org role `security_manager` |
| FinOps approvers | SKU ends with `ProvisionedManaged` (PTU) | `@CoolEngOrg/finops-approvers` |

The execution environment is `foundry-<environment>`. Each has its own Entra app, federated credential and reviewers.

## Execution

1. **Gate:** digest-bound approval check; posts `executing`.
2. **Deploy job**, in environment `foundry-<env>`, with `id-token: write`:
   1. `issueops foundry plan --expect-digest D` writes `plan.json` and `parameters.json`. The ARM parameters are produced with `encoding/json`.
   2. `azure/login` with OIDC, using the environment's `AZURE_CLIENT_ID`.
   3. Pre-flight:
      - `az cognitiveservices account list-models`
      - `az cognitiveservices usage list -l <region>`
      - `az cognitiveservices account deployment list`
      - then `issueops foundry preflight`

      It fails when:
      - the model, version and SKU aren't offered on the account
      - quota is insufficient. Standard quota is per model (`OpenAI.<SKU>.<model>`). PTU quota is model-independent (`OpenAI.<SKU>`). An existing deployment's capacity is credited on resize.
      - an existing deployment with that name uses another model or SKU

      With `quota_strict`, a missing quota entry fails standard OpenAI SKUs. It only warns for PTU and non-OpenAI models.
   4. `az deployment group what-if`, then `az deployment group create` with the Bicep template (deployment name `issueops-<issue>-<run>`).
   5. `az cognitiveservices account deployment show` and `account show`, then `issueops foundry result`. It requires `provisioningState == Succeeded` and records the endpoint.
3. **Report:** completion comment with the endpoint, keyless (Entra ID) usage snippet and quota. On failure it shows the pre-flight errors; an approver can `.retry` after fixing quota ([prod PTU example](../../examples/foundry-model-deployment/output/scenario-prod-ptu/transcript.md)).

## Azure permissions

| Identity | Role | Scope |
|---|---|---|
| `issueops-gh-foundry-<env>` (GitHub OIDC) | Cognitive Services Contributor | Foundry resource group of that environment |
| same | IssueOps Foundry Quota Reader (custom: `locations/usages/read`, `locations/models/read`, `accounts/models/read`) | Subscription |
| Consumers (managed identities, agents) | Cognitive Services OpenAI User | Foundry account (granted separately) |

## Settings

| Key | Meaning |
|---|---|
| `accounts` | Approved Foundry accounts → `environment`, `resource_group`, `region` |
| `models[]` | `name`, `format`, `versions`, `skus`, `environments`, `max_capacity` per environment |
| `default_rai_policy` | Content filter applied to every deployment (`Microsoft.DefaultV2`) |
| `deployment_name_pattern` | Deployment name regex |
| `quota_strict` | Fail (instead of warn) when no matching quota entry is found |

Confirm each allow-list entry with `az cognitiveservices account list-models -n <account> -g <rg>` before enabling it. Model availability varies by region and changes over time.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `No matching federated identity record found` | The job didn't run in the environment the credential names, or the subject differs in case. Check `repo:CoolEngOrg/issueops:environment:foundry-<env>` |
| `AuthorizationFailed` on deployment | Assign Cognitive Services Contributor on the resource group; custom quota role missing |
| "not offered on … (list-models)" | Wrong version or SKU for that region; update the request or the allow-list |
| Insufficient quota | Request a quota increase, or lower capacity. After the increase, an approver comments `.retry` |
| `--sku-capacity` ignored (manual CLI use) | Use Azure CLI 2.60 or later |
