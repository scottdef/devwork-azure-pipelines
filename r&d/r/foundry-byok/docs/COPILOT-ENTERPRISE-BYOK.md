# Copilot custom models for an enterprise or organization (BYOK)

This is the path that puts a Foundry deployment into the model picker of
**Copilot Chat, Copilot CLI and the IDEs** for your users. Primary source:
<https://docs.github.com/en/enterprise-cloud@latest/copilot/how-tos/administer-copilot/manage-for-enterprise/enable-custom-models>.
The feature is in public preview.

## Why the tool generates a runbook and not an API call

GitHub documents this feature as a settings form. No REST endpoint or
configuration file for it is documented, so there is nothing to automate
against. What can be automated is getting the values right, grouping them the
way the form demands, and keeping a reviewable record. `generate` therefore
writes:

- `copilot-enterprise/custom-models-runbook.md`: what to type, in order.
- `copilot-enterprise/custom-models.json`: the same as data. Commit it; a pull
  request that changes it is your change record for a setting that otherwise
  has no history outside the audit log.

If GitHub ships an API for this later, `custom-models.json` is already the
desired state to feed it.

## The form

Enterprise: **Enterprise → AI controls → Copilot → Configure custom models →
Add API key** (the Enterprise Cloud edition of the docs words the entry point
as *Configure allowed models → Custom models tab*).
Organization: **Settings → Copilot → Models → Custom models → Add API key**.

| Field | What the tool supplies |
|---|---|
| Provider | `Microsoft Foundry` |
| Name | `enterprise_key_name`, default `foundry-{env}-{resource}`. Shown in the model picker |
| API key | not supplied; the runbook names the secret (`api_key_secret`) whose value you paste |
| Deployment URL | per `enterprise_url_style`, see below |
| Available models → Model ID | the **deployment name**, one per deployment sharing the URL |

After saving: **Added models** tab → **Configure** → set **Enabled** → **Access**
tab → all organizations or selected ones. Models then appear at the bottom of
the picker under the enterprise name.

## One Deployment URL per key entry

GitHub's rule: models with different deployment URLs cannot be added to the
same API key; create a separate key entry per URL. The tool enforces that when
it builds the runbook:

- Deployments are grouped by key name. With the default `v1` style every
  OpenAI-compatible deployment on a resource shares `{origin}/openai/v1`, so
  one entry carries all of them as Model IDs.
- Claude deployments use a different URL, so they get their own entry
  (`…-anthropic`).
- With the `deployment` style every deployment has its own URL, so the default
  key name gains the deployment name.
- If you set the same `enterprise_key_name` on deployments whose URLs differ,
  or that use different key secrets, `validate` fails and tells you which.

## The Deployment URL format is not documented

GitHub's page names the field and shows a screenshot, but does not say what
the URL must look like. The tool makes the choice explicit and reversible
instead of guessing silently.

| `enterprise_url_style` | Value | Reasoning |
|---|---|---|
| `v1` (default) | `{origin}/openai/v1` | The only Foundry URL form GitHub documents for any Copilot BYOK surface (gh-aw, SDK). One URL serves many models, which fits a form that takes several Model IDs per URL |
| `deployment` | `{origin}/openai/deployments/{deployment}/chat/completions?api-version=…` | The "Target URI" shown for many deployments in the Foundry portal, and the form reported for VS Code's personal Azure BYOK |
| `root` | `{origin}` | What the Copilot SDK documents for provider type `azure` |

For Claude the three values are the messages URL, the `/anthropic` base, and
the origin.

Procedure: try the primary URL; if the form rejects it, try the two fallbacks
the runbook lists, in order. Once one works, set `enterprise_url_style` in the
inventory `defaults`, regenerate, and commit, so the record matches reality.
If you learn the authoritative format from GitHub support, the same setting
captures it.

## Constraints to plan for

**Hostname validation.** A community report (discussion 198472, in which the
author says GitHub Support confirmed it as a product limitation) states that
the Microsoft Foundry provider checks the hostname before sending any request
and only accepts `*.openai.azure.com` and `*.services.ai.azure.com`. The tool
encodes this: an `endpoint` pointing at APIM, a private name or a sovereign
cloud, and `host_kind: cognitiveservices`, are marked **Blocked** in the
runbook and produce an `enterprise`-scoped warning. gh-aw and direct API output
for the same deployment are unaffected.

**API key only.** The form has no Entra option, so the Azure resource must
allow key auth (`disableLocalAuth=false`). `auth: entra` deployments are listed
under "Not registered". To serve both worlds from one resource, keep Entra as
the resource default and override one deployment with `auth: api-key` for Chat.

**Reachability.** This is an inference from the architecture, not a documented
statement: Copilot Chat requests to a custom model are made by GitHub's
service, not from your network, so a resource with public network access
disabled cannot serve it, while the same resource can serve gh-aw from a
self-hosted runner inside your VNet. If policy forbids public access, plan for
a separate, key-enabled resource for Chat and keep the locked-down one for
workflows and applications.

**Key scope.** A resource key authorizes every deployment on the resource,
whether or not you listed it as a Model ID. GitHub recommends least privilege
for these keys; with Azure that means deciding per *resource* what may be
exposed.

**Model requirements.** GitHub's Copilot CLI BYOK page says models must support
tool calling and streaming, and recommends a context window of at least 128k
tokens; expect the same to matter for enterprise custom models, which also
feed the CLI. The enterprise page adds that fine-tuned models are supported
but quality varies, so test before rollout.

## Key rotation

1. Regenerate **key2** in Azure; update the GitHub Actions secret and the
   custom-models key entry to key2.
2. Run `foundry-byok verify --only-env <env> --probe` and send one Chat prompt.
3. Regenerate **key1**. Next rotation, swap roles.

Nothing generated by this tool changes during rotation, because it only ever
records the secret's name.

## Personal BYOK is a different feature

Individual users can also add their own keys in VS Code or the Copilot CLI.
That is configured on the workstation, not here. `copilot-cli.env` covers the
CLI case and is mainly useful for reproducing a gh-aw problem locally with the
same variables the workflow uses.
