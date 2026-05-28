# Local LLM Deployment Configs — P340 SFF (4 GB VRAM / 32 GB RAM)

Every file in this repository is tuned for a **Lenovo ThinkStation P340 SFF** with a
**Quadro P1000 (4 GB GDDR5)** and **32 GB DDR4**. The single hardest constraint is
VRAM: every config pins `num_gpu` layers, KV-cache quantization, context windows,
and parallelism to stay inside 4 GB with ~512 MB reserved for the desktop compositor.

## Directory layout

```
llm-deploy/
├── Makefile                          # one-command deploy targets
├── .env                              # shared env vars (source this)
├── ollama/
│   ├── override.conf                 # systemd drop-in for Ollama
│   ├── modelfiles/
│   │   ├── Modelfile.chat-3b         # general chat    — full GPU
│   │   ├── Modelfile.code-3b         # coding assistant — full GPU
│   │   ├── Modelfile.agent-3b        # tool-calling agent — full GPU
│   │   ├── Modelfile.heavy-7b        # 7B partial offload — hybrid
│   │   └── Modelfile.embed           # nomic-embed-text defaults
│   └── scripts/
│       └── benchmark.sh              # measure tok/s per model
├── lmstudio/
│   ├── server-config.json            # headless daemon settings
│   └── mcp.json                      # MCP server declarations
├── docker/
│   ├── docker-compose.yml            # Ollama + Open WebUI + SearXNG
│   ├── docker-compose.litellm.yml    # LiteLLM proxy overlay
│   └── searxng/
│       └── settings.yml              # SearXNG with JSON API enabled
├── agents/
│   ├── langchain/
│   │   ├── react_agent.py            # LangGraph ReAct with VRAM guards
│   │   └── research_graph.py         # multi-node research pipeline
│   ├── crewai/
│   │   └── crew.py                   # hierarchical crew with local LLMs
│   ├── autogen/
│   │   └── team.py                   # AutoGen 0.4+ with OllamaClient
│   ├── smolagents/
│   │   └── code_agent.py             # CodeAgent — best for small models
│   ├── pydantic_ai/
│   │   └── structured_agent.py       # typed outputs via schema constraint
│   └── litellm/
│       └── litellm_config.yaml       # multi-backend proxy config
├── scripts/
│   ├── vram_audit.sh                 # live VRAM usage report
│   └── preflight.sh                  # verify entire stack is healthy
└── pyproject.toml                    # uv/pip deps for the agent stack
```

## Quick start

```bash
source .env
make install          # pull models, create custom Modelfiles
make up               # docker compose up -d (Ollama + WebUI + SearXNG)
make test             # run preflight checks
make benchmark        # tok/s per model on this hardware
```
