# Example 3: Muse, from your Hugging Face repository, for Copilot CLI and gh-aw

"Muse" names two different models. This example covers both.

| | Muse Glimmer 30B (`EX=muse`) | Microsoft Muse / WHAM (`EX=wham`) |
|---|---|---|
| Maker | Meta, August 2026 | Microsoft Research + Ninja Theory, 2025 |
| Weights | [`meta-models/Muse-Glimmer-30B`](https://huggingface.co/meta-models/Muse-Glimmer-30B) | [`microsoft/wham`](https://huggingface.co/microsoft/wham) |
| License | Apache-2.0, not gated (read `USAGE_POLICY.md`) | Microsoft Research License, research only |
| What it does | image + text in, text out; reasoning, tool calls, 128K context | game frames + controller actions in, predicted frames out |
| In Foundry's catalog | listing not verified | yes, as "Muse" |
| Serving | vLLM v0.30.0 (`muse_glimmer` parsers), 2 x H100 | its own Flask server behind `shared/containers/wham` |
| Agents / Copilot | yes | no: not a language model |

Meta's other Muse model, Muse Spark, has no downloadable weights (Meta has said
an open 1.2 is coming, with no date), so it cannot be deployed here.

## Muse Glimmer: custom weights from your own repository

Here the weights are customised *elsewhere*: a fine-tune, a merge or a
quantisation published to a private repository such as
`CoolGitOrg/muse-glimmer-30b-coolgit`. The pipeline fetches that repository
inside Azure ML using a token from Key Vault.

1. Put a read token in Key Vault as secret `hf-token`; set the repository
   variable `KEY_VAULT_URL`. `bootstrap-azure.sh` gives the cluster identity
   *Key Vault Secrets User*. Public repositories need none of this.
2. `gh workflow run ex3-muse.yml -f variant=muse -f hf_repo=CoolGitOrg/muse-glimmer-30b-coolgit -f action=plan -f reason="plan our Muse fine-tune"`
3. Same with `-f action=deploy -f dry_run=false`. The `copilot` job then runs
   Copilot CLI against the endpoint and requires the `FTM_OK` round trip.

The repository must have the base's layout: `config.json` with
`MuseGlimmerForConditionalGeneration`, safetensors, tokenizer, processor and
chat template. Anything vLLM can load from a directory works.

## Copilot CLI (bring your own key)

```sh
export FTM_BASE_URL=https://ftm-muse-coolgit.eastus2.inference.ml.azure.com
export FTM_KEY=$(az ml online-endpoint get-credentials -n ftm-muse-coolgit -g rg-coolgit-copilot -w mlw-coolgit-foundry --query primaryKey -o tsv)
examples/03-muse-glimmer/harness/copilot-byok.sh probe
examples/03-muse-glimmer/harness/copilot-byok.sh ask "Summarise shared/bin/ftm in five bullets."
examples/03-muse-glimmer/harness/copilot-byok.sh shell
```

It sets `COPILOT_PROVIDER_BASE_URL=<endpoint>/v1`, `COPILOT_PROVIDER_TYPE=openai`,
`COPILOT_PROVIDER_API_KEY`, `COPILOT_MODEL=muse-glimmer` and `COPILOT_OFFLINE=true`.
Copilot CLI requires a provider model with tool calling and streaming;
`--tool-call-parser muse_glimmer` provides the first and vLLM the second.
vLLM also accepts an echoed `reasoning_content` field (it renames it), which
avoids the 400 errors that forced `COPILOT_PROVIDER_TYPE=anthropic` with some
DeepSeek setups. If you still see them, set `COPILOT_PROVIDER_TYPE=anthropic`:
vLLM serves `/v1/messages`, and the script drops `/v1` from the base URL for you.

## GitHub agentic workflow on your weights

[`agentic/muse-issue-triage.md`](agentic/muse-issue-triage.md) is a gh-aw
workflow whose engine is `codex` with `OPENAI_BASE_URL` pointed at the
endpoint. The AWF api-proxy injects the `OPENAI_API_KEY` secret (set it to the
endpoint key) and passes `model: muse-glimmer` through unchanged. It can only
label and comment, through `safe-outputs`. Copy it to `.github/workflows/`, fix
the host name, `gh aw compile --strict`, and commit both files. This path is
experimental. Codex speaks the Responses API, which vLLM serves at
`/v1/responses`. Run `gh aw run muse-issue-triage` on a test issue before
relying on it.

## Images

```sh
pip install -r examples/03-muse-glimmer/client/requirements.txt
python examples/03-muse-glimmer/client/vision.py diagram.png "List every component and arrow."
```

## Microsoft Muse (WHAM)

`wham/model.env` fetches the code, config and 1.6B checkpoint (not the
demonstrator app) and serves them with `shared/containers/wham`: WHAM's own
`run_server.py`, plus a small stdlib shim that adds `/health` and binds
`0.0.0.0`. No Foundry connection is created because it is not a chat model. The
smoke test only checks that the server answers.

```sh
gh workflow run ex3-muse.yml -f variant=wham -f action=deploy -f dry_run=false -f reason="WHAM research sandbox"
python examples/03-muse-glimmer/wham/wham_client.py --steps steps.json --frames frames/ --predict 5
```

`wham_client.py` follows the request shape in `run_server.py` (multipart
`/new_job`, zip from `/get_job_results`). The action encoding and the exact
`/new_job` reply are documented in the model card and the demonstrator, not
here. Check them against the checkpoint you deploy. Research use only; do not
remove the output watermark.
