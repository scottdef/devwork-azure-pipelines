# ──────────────────────────────────────────────────────────────────────
#  Makefile — deploy, build, test the local LLM stack
#  Hardware: P340 SFF · Quadro P1000 4 GB · 32 GB RAM
# ──────────────────────────────────────────────────────────────────────

SHELL     := /bin/bash
ENVFILE   := .env
COMPOSE   := docker compose -f docker/docker-compose.yml
OLLAMA    := ollama

include $(ENVFILE)
export

# ── Phony targets ────────────────────────────────────────────────────
.PHONY: help install pull models up down restart logs test benchmark \
        vram preflight clean litellm

help: ## show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# ── Install & build ──────────────────────────────────────────────────

install: pull models ## pull base models + create custom Modelfiles
	@echo "✓ All models installed and custom Modelfiles created."

pull: ## pull upstream base models from ollama.com
	$(OLLAMA) pull qwen2.5:3b
	$(OLLAMA) pull qwen2.5-coder:3b
	$(OLLAMA) pull qwen2.5:7b
	$(OLLAMA) pull llama3.2:3b
	$(OLLAMA) pull nomic-embed-text
	$(OLLAMA) pull llava-phi3

models: ## create hardware-tuned Modelfiles
	$(OLLAMA) create $(MODEL_CHAT)  -f ollama/modelfiles/Modelfile.chat-3b
	$(OLLAMA) create $(MODEL_CODE)  -f ollama/modelfiles/Modelfile.code-3b
	$(OLLAMA) create $(MODEL_AGENT) -f ollama/modelfiles/Modelfile.agent-3b
	$(OLLAMA) create $(MODEL_HEAVY) -f ollama/modelfiles/Modelfile.heavy-7b
	@echo "✓ Custom models: $(MODEL_CHAT) $(MODEL_CODE) $(MODEL_AGENT) $(MODEL_HEAVY)"

# ── Docker compose ───────────────────────────────────────────────────

up: ## start Ollama + Open WebUI + SearXNG
	$(COMPOSE) up -d
	@echo "✓ Open WebUI → http://localhost:$(WEBUI_PORT)"
	@echo "✓ SearXNG    → http://localhost:$(SEARXNG_PORT)"

down: ## stop stack
	$(COMPOSE) down

restart: down up ## restart stack

logs: ## tail all container logs
	$(COMPOSE) logs -f --tail=100

litellm: ## start LiteLLM proxy overlay
	docker compose -f docker/docker-compose.yml \
	               -f docker/docker-compose.litellm.yml up -d litellm

# ── Diagnostics ──────────────────────────────────────────────────────

test: preflight ## run full preflight check
preflight: ## verify driver, CUDA, Ollama, Docker, GPU
	@bash scripts/preflight.sh

vram: ## live VRAM audit
	@bash scripts/vram_audit.sh

benchmark: ## tok/s benchmark per model
	@bash ollama/scripts/benchmark.sh

# ── Cleanup ──────────────────────────────────────────────────────────

clean: ## remove custom models (keeps base models)
	-$(OLLAMA) rm $(MODEL_CHAT)
	-$(OLLAMA) rm $(MODEL_CODE)
	-$(OLLAMA) rm $(MODEL_AGENT)
	-$(OLLAMA) rm $(MODEL_HEAVY)
