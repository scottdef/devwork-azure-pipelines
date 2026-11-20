# Palantir Foundry and AIP: an outline

Low priority, and deliberately only an outline: nothing here is published.
`api.yaml` lists no environments, so the sync skips this directory.

## The idea

Apps on CoolDeploy do not hold Palantir credentials and do not call Palantir
directly. They call an API in API Management, the same way they call the AI
gateway, and the gateway holds the one credential:

```
app ──Entra ID token──▶ API Management ──Foundry token──▶ <stack>.palantirfoundry.com
        + subscription key      │
                                └─ token from Key Vault, as named value cool-palantir-token
```

That gives one place to rotate the token, one place to rate-limit, and a
record of which app called what.

## What is here

| File | Holds |
| --- | --- |
| `api.yaml` | the API's name, path `partners/palantir`, product `partners`, subscription-key auth |
| `openapi.yaml` | two operations as a starting point: list objects of an ontology, and ask an AIP agent one question |
| `policy.xml` | swap the caller's credentials for the Foundry token; pass nothing else through |

## To make it real

1. In Foundry, create a service user (a third-party application with the
   client-credentials grant) scoped to the ontologies and agents apps need.
   A long-lived user token works for a first trial and is the wrong thing
   to keep: move to client credentials and have the policy fetch and cache
   the access token.
2. Put the credential in the environment's Key Vault as `cool-palantir-token`.
   `global/named-values.yaml` already declares the named value that reads it.
3. Add a backend `cool-palantir` to `global/backends.yaml`, and its URL,
   `https://<stack>.palantirfoundry.com/api`, to each `instances/<env>.yaml`.
4. Check `openapi.yaml` against Palantir's current API reference. The paths
   here follow the v2 Ontologies API and the AIP Agents API as documented in
   2026; the agents API was in preview then and its paths may have moved.
5. Add environments to `api.yaml`, one at a time, starting with `dev`.

## Not decided

- Whether apps should reach Foundry on behalf of a user (OAuth authorization
  code, per-user Foundry permissions) or as a service. The outline assumes a
  service. Per-user access changes the policy and needs a Foundry-side
  application registration per environment.
- Whether Foundry should call back into the platform (webhooks, or Foundry
  functions calling app APIs). That direction would be an ordinary published
  API with `auth: subscription`, and needs no special support.
