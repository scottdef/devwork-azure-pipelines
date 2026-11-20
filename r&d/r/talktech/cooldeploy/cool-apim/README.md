# cool-apim

Azure API Management for CoolDeploy, managed with bash and the Azure CLI
from YAML files and OpenAPI descriptions. There is no Terraform here, on
purpose, and `cool-infra`'s CI fails if someone adds an API Management
resource there.

```
instances/<env>.yaml          one API Management instance per environment
global/named-values.yaml      named values every instance has
global/backends.yaml          backends that are not one app
global/policy.xml             the policy around every request
fragments/*.xml               policy fragments: token check, rate limit, CORS, AI budget
products/*.yaml               products
apis/<group>/<name>/          api.yaml, openapi.yaml and, when needed, policy.xml
scripts/                      what turns the files into az calls
test/                         a stand-in az, and the checks that use it
```

## Instances

| Environment | Instance | Tier |
| --- | --- | --- |
| `sbx` | `sbx-eus2-coolapi-apim-001` | Basic v2 |
| `dev` | `dev-eus2-coolapi-apim-001` | Standard v2 |
| `tst` | `tst-eus2-coolapi-apim-001` | Standard v2 |
| `prd` | `prd-eus2-coolapi-apim-001` | Standard v2, two units |

Standard v2 integrates with a virtual network for outbound traffic, which is
how an instance reaches the internal gateways of the app clusters. Basic v2
does not, so the sandbox instance is for rehearsing configuration, not for
carrying traffic to the sandbox cluster.

## Use

```sh
scripts/apim-sync.sh --env dev --dry-run     # print what a sync would apply
scripts/apim-sync.sh --env dev               # apply it
scripts/apim-sync.sh --env dev --only apis   # one part: global, products or apis
scripts/apim-instance.sh --env dev           # create or resize the instance itself
```

Needs `az` (signed in), `jq`, and `yq` or `helm` to read YAML. Either yq
works: mikefarah's or the Python one. With neither, the scripts read YAML
through a one-template Helm chart in `scripts/yaml2json`.

A sync applies, in this order, because each step needs the one before it:

1. named values, then backends, then policy fragments, then the global policy
2. products
3. APIs: import the OpenAPI file, put the API in its product, set its policy

Every step creates or replaces. A sync that stops half way is repaired by
running it again. A sync never deletes: an API that is in API Management and
not in this repository is reported and left alone.

## How the Azure CLI is used

| For | Command |
| --- | --- |
| the instance | `az rest --method put` on the service. `az apim create` does not offer the v2 tiers. |
| named values | `az apim nv create` / `update`; Key Vault references by `az rest`, which `az apim nv` cannot express |
| backends, policy fragments, policies | `az rest --method put`. The CLI has no commands for them. |
| products | `az apim product create` / `update` |
| APIs | `az apim api import --specification-format OpenApi`, then `az apim product api add` |

The management API version is `2024-05-01`; set `APIM_API_VERSION` to use
another.

## Publishing an app's API

Nobody writes the files under `apis/<team>/<app>/` by hand. A
`cool-api-publish` request in `CoolEngOrg/issueops` opens a pull request
here with two files:

```yaml
# apis/payments/checkout/api.yaml
name: payments-checkout
displayName: payments / checkout
path: payments/checkout
product: payments
auth: entra-jwt
team: payments
app: checkout
backendPath: /api
source: CoolEngOrg/payments-checkout@<commit>
environments: [dev, tst]
```

and `openapi.yaml`, copied from the app's repository at the commit of the
release that is live. Merging runs the sync for the environments listed.

`auth` decides the API's policy when the directory has no `policy.xml`:

| `auth` | Callers | Policy |
| --- | --- | --- |
| `entra-jwt` | send an Entra ID token for the environment's audience | `cool-jwt`, `cool-rate-limit` |
| `subscription` | send a subscription key of the product | `cool-rate-limit` |
| `none` | anyone who can reach the gateway | `cool-rate-limit` |

The backend of an app's API is `https://<app>.<env>.coolapps.example` plus
`backendPath`: the app's host on the internal gateway of that environment's
cluster.

## The AI gateway

`apis/platform/cool-ai` fronts the Microsoft Foundry account of each
environment with an OpenAI-compatible path. An app calls it with its own
Entra ID token and holds no model key:

- `cool-jwt` checks the token;
- `cool-ai-budget` reads the app's budget from the token's `roles` claim
  (`AI.Small`, `AI.Medium`, `AI.Large`) and applies `llm-token-limit` per
  calling identity;
- `llm-emit-token-metric` records token use by caller and budget;
- API Management signs in to Foundry with its managed identity.

The budget is an app role that `cool-infra` assigns to the app's identity
when a `cool-addon` request for `ai` is merged. The tokens-per-minute behind
each budget are named values, set per instance in `instances/<env>.yaml`.

## Policy files are XML

Policy files here are well-formed XML and are sent with `format: xml`. An
attribute whose expression contains double quotes is written in single
quotes, and `<` and `&` inside expressions are entities. That costs a little
readability and buys a check: `test/check.sh` parses every policy before
anything is sent.

## Check

```sh
make check
```

1. Every YAML, OpenAPI and policy file parses; an API names a product and a
   backend that exist; a policy includes only fragments that exist and uses
   only named values that are declared; every instance has a value for every
   named value and backend.
2. The scripts run with `test/az`, a stand-in that records calls. The test
   reads the record: a dry run changes nothing, a first sync creates in the
   right order, a second sync updates, `prd` publishes what `dev` does not,
   and the instance request carries the right tier.

The stand-in does not know whether Azure would accept a request. The first
real sync of the sandbox instance is the test of that. In particular, check
there: the outbound VNet properties of the instance body, and the policy
expressions, which only API Management can compile.

## Palantir

`apis/partners/palantir-foundry` is an outline of fronting Palantir Foundry
and AIP through the gateway. It is not published; its README says what it
would take.
