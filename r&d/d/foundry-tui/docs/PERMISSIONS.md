# Permissions: the Foundry Owner boundary

The brief: reports must be "available to Foundry Owner permissions". So the
role is the specification. This file says what it contains, what follows, and
how to widen it when you choose to.

## The role

| | |
|---|---|
| Name | **Foundry Owner** (called *Azure AI Owner* before the 2026 rename) |
| Role definition id | `c883944f-8b7b-4483-af10-35834be79c4a` |
| Used by | you, at the terminal; and `uami-foundry-deploy`, in Actions |

Microsoft's guidance during the rename is to refer to these roles by id, not
by name. Every script, workflow and Go file here does.

`foundry-tui perms` prints the matrix below from the **live** definition when
`az` can read it, and from the snapshot embedded in `internal/perms` (taken
2026-07-16) when it cannot. The report header says which was used.

| Capability | Azure action | Plane | In role |
|---|---|---|---|
| Account settings | `Microsoft.CognitiveServices/accounts/read` | control | yes |
| Deployments and authorship (`systemData`) | `…/accounts/deployments/read` | control | yes |
| Catalog and retirement dates | `…/accounts/models/read` | control | yes |
| Regional quota | `…/locations/usages/read` | control | yes |
| Role assignments | `Microsoft.Authorization/roleAssignments/read` | control | yes |
| Resource Health | `Microsoft.ResourceHealth/availabilityStatuses/read` | control | yes |
| Fired alerts | `Microsoft.AlertsManagement/alerts/read` | control | yes |
| Create / scale / delete deployments | `…/accounts/deployments/write`, `/delete` | control | yes |
| Chat completions (probes) | `Microsoft.CognitiveServices/*` data action | data | yes |
| **Azure Monitor metrics** | `Microsoft.Insights/metrics/read` | control | **no** |
| **Activity log** | `Microsoft.Insights/eventtypes/values/read` | control | **no** |
| Diagnostic settings, Log Analytics queries | `Microsoft.Insights/diagnosticSettings/*`, `Microsoft.OperationalInsights/*` | control | no |

The role does include `Microsoft.Insights/metricalerts/*`,
`activityLogAlerts/*` and `scheduledqueryrules/*`: an owner may *define* alert
rules and *see them fire* without being able to chart the metric underneath.

## What "monitoring" therefore means here

With Foundry Owner and nothing else, the program monitors with:

1. **Provisioning state** of the account and every deployment.
2. **Resource Health** availability state and summary.
3. **Fired alerts** in the last 30 days. Put your 429-rate, latency and
   token-burn thresholds in alert rules; the owner sees the result.
4. **Quota** used against limit, per quota line.
5. **Retirement dates** from the catalog joined to what is deployed.
6. **Its own probes**: success, latency and token counts from a real request,
   through the API and through Copilot CLI, kept as history.
7. **GitHub**: every run of the deploy, report and test workflows, and the
   success rate of agentic workflows.

Authorship comes from each deployment's `systemData` (`createdBy`,
`lastModifiedBy`, timestamps), from the workflow run history, from the
committed `records/deployments/…json` files and from the local audit chain:
four independent witnesses, none of which needs the activity log.

## Turning metrics on

Microsoft's role table says Foundry Owner may complete role assignments
conditionally: Foundry User, ACR and **monitoring roles**. So an owner can
usually grant this without asking anyone:

```sh
scope=$(az cognitiveservices account show -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --query id -o tsv)
az role assignment create --role "Monitoring Reader" --scope "$scope" \
   --assignee-object-id "$(az ad signed-in-user show --query id -o tsv)" --assignee-principal-type User
```

Check the condition on the live definition first
(`az role definition list --name c883944f-8b7b-4483-af10-35834be79c4a --query '[0].permissions[0].condition'`);
the list of assignable roles is Microsoft's to change.

The program decides from the *Foundry Owner definition*, not from your effective
access, so that records stay comparable between owners. To make it read
metrics, say so explicitly: the matcher treats the role set as data, and
`internal/perms` has one obvious place (`Requirements`) where the `metrics`
capability names its action. Add Monitoring Reader's action to the loaded set
in a fork, or wait for the role to gain it; on the day Microsoft adds
`Microsoft.Insights/metrics/read` to Foundry Owner the live definition will say
so and the metrics section will appear with no code change.

## The workflow identity

`scripts/bootstrap-azure.sh` assigns Foundry Owner to the managed identity **at
the scope of the one Foundry account**, not the resource group or subscription.
It gets no secret: three federated credentials bind it to

- `repo:CoolGitOrg/foundry-ops:environment:foundry-plan`
- `repo:CoolGitOrg/foundry-ops:environment:foundry-prod`
- `repo:CoolGitOrg/foundry-ops:ref:refs/heads/main`

A fork, a pull request or another branch cannot obtain its token. Real changes
run in `foundry-prod`; add required reviewers to that environment and an
approval stands between every dispatch and Azure.

On the GitHub side a person needs `actions: write` on the ops repository to
dispatch, and nothing in Azure beyond read. The person who asks and the
identity that acts are different principals, and the record names both.
