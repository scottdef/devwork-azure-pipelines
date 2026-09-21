# Testing models

A probe asks a deployment to repeat one word, `FOUNDRY_OK`, and records
whether it did, how long it took and how many tokens it cost. It is the
cheapest request that proves the whole path: DNS, network rules, Entra, RBAC
data actions, the deployment, the model.

There are two paths, because there are two ways your users reach the models.

## Path 1: the API (`t`, or `foundry-tui test -mode api`)

| Model format | Request |
|---|---|
| anything but Anthropic | `POST https://<account>.openai.azure.com/openai/v1/chat/completions`, body `{"model": "<deployment>", "messages": […]}` |
| Anthropic | `POST https://<account>.services.ai.azure.com/anthropic/v1/messages`, header `anthropic-version: 2023-06-01`, `max_tokens: 64` |

The v1 endpoint takes no `api-version`; `model` is the **deployment name**, not
the model name. Authentication is `Authorization: Bearer <token>` where the
token comes from

```sh
az account get-access-token --resource https://cognitiveservices.azure.com
```

No key is involved, so the probe keeps working after you set
`disableLocalAuth: true` on the account, which you should. The Foundry Owner
role carries the `Microsoft.CognitiveServices/*` data actions, so the owner can
probe with no extra assignment.

Deployments that do not declare chat completion (embeddings, image, speech) are
skipped, and the result says so; a skip is not a failure.

## Path 2: Copilot CLI with your own model (`c`, or `-mode copilot`)

This answers a different question: *can a developer point Copilot CLI at this
deployment and get an answer?* The program runs

```sh
copilot -p "<prompt>" -s --no-ask-user
```

in a fresh empty temporary directory (nothing for an agent to read or change),
with this environment:

| Variable | Value |
|---|---|
| `COPILOT_PROVIDER_BASE_URL` | `https://<account>.openai.azure.com/openai/v1` (Anthropic: the `/anthropic` base) |
| `COPILOT_PROVIDER_TYPE` | `copilot.provider_type`, default `openai`; forced to `anthropic` for Anthropic-format deployments |
| `COPILOT_PROVIDER_BEARER_TOKEN` | the same short-lived Entra token; it takes precedence over any API key |
| `COPILOT_MODEL` | the deployment name |
| `COPILOT_PROVIDER_WIRE_API` | `copilot.wire_api`, default `completions`; use `responses` for models that need it |
| `COPILOT_OFFLINE` | `true` when `copilot.offline` is set: the CLI talks only to your provider |

In BYOK mode Copilot CLI needs no GitHub login. The token is passed in the
child's environment only; it is never written to disk, the audit log or a report.

Things that will bite you, so you know what a failure means:

- **Tool calling and streaming are required.** GitHub's documentation says a
  BYOK model must support both or the CLI returns an error. A model can pass
  the API probe and fail this one for that reason alone. That is a true result:
  the deployment is healthy and is not usable from Copilot CLI.
- **`openai` rather than `azure`.** Foundry's v1 endpoint is OpenAI-compatible,
  and the `openai` provider type against it has been the more reliable pairing.
  Set `copilot.provider_type` to `azure` if your CLI version prefers it; the
  program then sends the bare resource URL and `COPILOT_PROVIDER_WIRE_MODEL`.
- **DeepSeek-family deployments** have needed the `anthropic` provider type in
  this project's experience. Override per run with a second config file.
- **Empty stdout.** Some CLI versions have printed `-p` answers somewhere other
  than stdout. The probe reports `exit 0 but no marker on stdout`. Add
  `"--output-format", "json"` to `copilot.args`; the marker is searched for in
  whatever comes back.
- **Token lifetime.** The Entra token lasts about an hour. Each probe fetches a
  fresh one; a long interactive Copilot session on a bearer token will not.

`copilot.bin`, `copilot.args` and `copilot.timeout_seconds` are configuration
because this CLI changes weekly. Run `copilot help providers` for what your
version accepts.

## Path 3: in Actions (`T`, or `dispatch -kind test`)

`foundry-model-test.yml` runs both probes from a GitHub runner with the
workflow identity and commits the results. Use it to tell "my laptop cannot
reach it" from "nobody can", and put it on a schedule for a synthetic monitor.

## Where results go

| Place | What |
|---|---|
| the screen | result line in the status bar; the record in the Audit view (`6`) |
| `<state_dir>/probes.jsonl` | one JSON object per probe: `time`, `deployment`, `kind`, `ok`, `status`, `latency_ms`, `tokens_in`, `tokens_out`, `detail` |
| audit chain | `probe.api` / `probe.copilot` records |
| reports | "Model probes" section, newest first; the latest failure per deployment and path becomes a `warn` finding |

`foundry-tui test -json | jq -r 'select(.ok|not) | .deployment'` lists what is
broken. The exit status is non-zero if any probe failed.

## Testing the program itself

```sh
make test     # everything, with fakes
make race     # the same under the race detector
make sample   # render the fixture report into examples/sample-report/
```

No test touches the network. `internal/azure/azuretest` holds canned `az`, `gh`
and `copilot` output; add a fixture there when Azure changes a shape, and the
parsers, the report and the screen are all exercised against it.
