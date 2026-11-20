# cool-app

The one chart every CoolDeploy app is installed with. Teams never edit it and
never write templates: `cool-api` writes a `values.json` per app and
environment into `cool-gitops`, and Argo CD installs this chart with it.

## What it renders

| Object | When |
| --- | --- |
| `Deployment` `<app>-<release>` | one per release that takes traffic or stands by |
| `HorizontalPodAutoscaler`, `PodDisruptionBudget` | one per release that takes traffic; the budget only when `scale.min` > 1 |
| `Service`, `ServiceAccount` | always |
| `ConfigMap` `<app>-env`, `<app>-flags` | always |
| `SecretProviderClass` `<app>-secrets` | when `secrets` is set and the cluster's provider is `keyvault` |
| `DestinationRule`, `VirtualService` | when the mesh is on and a release takes traffic |
| `VirtualService` `<app>-<release>` | one per retained release, on the internal gateway |
| `CronJob` `<app>-cron-<n>` | one per entry in `crons`; none in a preview |

## Values

App values come from `cool.json` through `cool-api`. See
`coolctl/internal/release` for the Go type that writes them.

| Key | Meaning |
| --- | --- |
| `team`, `app`, `environment` | identity of the install; the namespace is `<team>-<app>` |
| `preview` | pull-request number; above 0 the install is a preview: one replica, internal only, no crons |
| `identity` | client ID of the app's managed identity; needed to read Key Vault |
| `expose` | `internal` or `public`; public adds the external gateway |
| `port`, `health` | container port and probe path |
| `scale`, `resources` | autoscaler bounds and per-replica requests |
| `env`, `overrides` | environment variables; `overrides` is set per environment by IssueOps and wins |
| `secrets` | names of Key Vault secrets to expose as environment variables |
| `routes`, `crons`, `flags`, `domains` | routing rules, schedules, feature flags, custom hosts |
| `releases` | newest first: `id`, `digest`, `weight`, `standby` |

Platform values come last, from `cool-gitops/clusters/<cluster>/platform-values.json`,
so an app cannot change them.

| Key | Meaning |
| --- | --- |
| `platform.registry` | the only registry images are pulled from |
| `platform.domain` | DNS zone of app hosts |
| `platform.namePrefix` | `<env>-<region>`; names the team Key Vault |
| `platform.mesh.enabled` | `false` renders plain Services for a cluster with no Istio |
| `platform.secrets.provider` | `keyvault`, or `none` to expect a hand-made Secret |

## Two rules for editing templates

1. Emit structured values with `toJson`, never `toYaml`. JSON is YAML, the
   indentation cannot go wrong, and `coolctl render chart` can render the
   chart without Helm.
2. Pass numbers through `int` before comparing or printing them. Helm reads
   every number as a float.

## Render it

```sh
coolctl render chart -chart charts/cool-app -namespace payments-checkout \
  -f ../cool-gitops/apps/payments/checkout/dev/values.json \
  -f ../cool-gitops/clusters/dev-eus2-appdev-aks-001/platform-values.json

helm template checkout charts/cool-app -n payments-checkout \
  -f ../cool-gitops/apps/payments/checkout/dev/values.json \
  -f ../cool-gitops/clusters/dev-eus2-appdev-aks-001/platform-values.json
```
