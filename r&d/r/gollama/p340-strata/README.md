# p340-strata

Companion to the guide "Local LLM Operations on a ThinkStation P340: Unsloth
Studio to Strata". It holds the production plane's glue: a gateway, a voting
council, a tool-loop agent, the systemd units, and the simulator behind the
guide's section 8 and Appendix A.

Go 1.24, standard library only. No Python, no containers, no network access at
build time.

## What is here

| Path | What it is |
| --- | --- |
| `cmd/gate` | Alias gateway on `127.0.0.1:4000`. Routes by the `model` field, checks one key, tries the next upstream on a connection error or a 5xx, streams through |
| `cmd/council` | Asks several seats the same question and prints the majority answer. One alias named k times is self-consistency; different aliases make a mixed council |
| `cmd/agent` | Tool loop with `web_search` (SearXNG) and `fetch_url`. Step budget, size budget, and a fetch that refuses private addresses |
| `cmd/p340sim` | First-order model of memory fit and turn time. Prints the tables in the guide |
| `etc/routes.json` | Alias table for `gate` |
| `etc/env.example` | Keys and paths. Copy to `~/.config/p340/env`, `chmod 600` |
| `systemd/` | User units: `p340-gate`, `p340-brain-llama`, `p340-brain-strata`, `p340-embed` |
| `scripts/preflight.sh` | Read-only check of the machine. Installs and starts nothing |
| `scripts/strata-plan.sh` | Runs `strata launch --plan` three ways and saves the output |
| `sim/results.md`, `sim/results.json` | Simulator output as shipped |
| `SHA256SUMS` | Checksums of every file above |

## Quick start

Unpack into your home directory. The units expect `~/p340-strata`.

```bash
tar -xzf p340-strata.tar.gz -C ~ && cd ~/p340-strata
sha256sum -c SHA256SUMS
make build test          # compile into ./bin, run the unit tests
make preflight           # what this machine has and lacks
make help                # every target
```

Then follow section 10 of the guide. In short:

```bash
mkdir -p ~/.config/p340 && cp etc/env.example ~/.config/p340/env && chmod 600 ~/.config/p340/env
$EDITOR ~/.config/p340/env
make install-units
make prod-llama          # production on llama-server; works on the stock P340
make plan                # ask Strata whether it accepts this machine
make prod-strata         # production on Strata, if it does
make proto               # stop the brain, free memory for Unsloth Studio
```

## What was tested, and what was not

Run where this package was built (Go 1.24.7, Linux x86-64, no GPU):

- `go vet ./...` and `go test ./...` pass.
- `gate`: alias rewrite, key check (401), unknown alias (404), failover to the
  second upstream, 502 when every upstream is down, `/v1/models` listing.
- `council`: answer extraction, case- and punctuation-insensitive majority,
  split vote with and without a judge.
- `agent`: search then answer against mock servers, HTML to text, refusal of
  loopback and `file:` URLs, tools withdrawn when the size budget is spent.
- `gate` and `council` end to end against a mock model server.
- `p340sim`: output in `sim/`.

Not run anywhere: any of this against a real model, Strata, Unsloth Studio, a
Quadro P1000 or a ThinkStation P340. Specifically untested:

- `systemd/p340-brain-strata.service` is a sketch. Strata documents its own
  startup recipe in `docs/DETAILS.md`; prefer that if the two disagree. The
  unit assumes `strata` is at `~/.local/bin/strata`; check with
  `command -v strata` and edit the path if not. Because `strata launch` exits
  0 even when it starts nothing, the unit waits for the endpoint to answer.
- `scripts/strata-plan.sh` probes a Flash-Next Coder pack by a catalog name
  that was not confirmed. Strata answers an unknown name with the closest
  match.
- The llama-server flags in `p340-brain-llama.service` follow llama.cpp's
  documentation; the VRAM they use on a P1000 is modelled, not measured.

## llama-server for the P1000

The llama lane needs a llama-server built for Pascal (compute capability 6.1)
with CUDA 12.9. CUDA 13 cannot target this card.

```bash
uv tool install cmake            # Focal ships 3.16; CMAKE_CUDA_ARCHITECTURES needs 3.18
git clone https://github.com/ggml-org/llama.cpp ~/llama.cpp && cd ~/llama.cpp
cmake -B build -DCMAKE_BUILD_TYPE=Release -DGGML_CUDA=ON \
  -DCMAKE_CUDA_ARCHITECTURES=61 \
  -DCMAKE_CUDA_COMPILER=/usr/local/cuda-12.9/bin/nvcc
cmake --build build -j 10 --target llama-server
```

Put the resulting `build/bin/llama-server` path in `LLAMA_SERVER` in the env
file.

## The gateway

```bash
GATE_KEY=... bin/gate -listen 127.0.0.1:4000 -routes etc/routes.json
curl -s http://127.0.0.1:4000/healthz
curl -s http://127.0.0.1:4000/v1/models -H "Authorization: Bearer $GATE_KEY"
```

- A request's `model` is an alias. Each upstream may rewrite it (`"model"` in
  `routes.json`); empty keeps the alias.
- Upstream keys are read from the environment variable named in `key_env` and
  sent as both `Authorization: Bearer` and `x-api-key`.
- The response header `X-Gate-Upstream` names the server that answered.
- `gate` refuses to listen on anything but loopback unless `GATE_KEY` is set.
- It has no budgets, per-user keys or usage accounting. Use LiteLLM in its
  place if you need those.

## The council

```bash
bin/council -seats brain,brain,brain -temp 0.8 "What is 17 * 23?"
bin/council -seats brain,second,brain -judge brain "..."
```

Exit status 0 on a majority, 2 on a split vote with no judge, 1 on error.

## The agent

```bash
bin/agent "What changed in the latest llama.cpp release?"
bin/agent -steps 1 "Summarize: $(cat notes.txt)"      # no tools, one call
```

`-budget` caps tool output in characters across the run (default 16,000, about
4,000 tokens) and `-page` caps one fetched page (default 6,000). When the
budget is spent the next model call offers no tools. `fetch_url` refuses
loopback, private and link-local addresses at dial time; `-allow-private`
turns that off.

## The simulator

```bash
make sim                          # rewrites sim/results.md and sim/results.json
bin/p340sim -format csv
```

It is arithmetic, not a benchmark. Decode time is bytes read per token divided
by effective memory bandwidth; prompt reading is 2 FLOP per active parameter
per token. Every constant in `cmd/p340sim/main.go` is a published figure, a
calibration point or a labelled assumption. Replace them with your own
measurements and rerun.

Its third table checks the model against five speeds Strata published for
other machines: within 12% where a 12 GB card shares the work with the CPU,
25 to 32% too fast where a whole model sits on one fast card.

## Security notes

- Everything binds to 127.0.0.1 by default. Each hop carries a key.
- `~/.config/p340/env` holds keys. Keep it at mode 600 and out of git.
- The agent's tools run on this machine with your permissions. Read
  `cmd/agent/main.go` before widening what they can reach.
- Verify downloads: `sha256sum -c SHA256SUMS` here, and the `SHA256SUMS.txt`
  that Strata publishes with each release before running its installer.
