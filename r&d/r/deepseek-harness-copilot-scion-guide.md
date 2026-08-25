# DeepSeek Harness with GitHub Copilot & Google Scion

## A Systems Engineer's Reference Guide

> **Status**: DeepSeek Harness is in developer preview (0.1.0-rc.x) as of August 2026.
> Scion is experimental. Both projects ship breaking changes by design.
> Pin versions. Read changelogs. Trust nothing you haven't tested.

> **Verified against**: `@deepseek-ai/dsh@0.1.0-rc.8`, Scion `main` (Aug 23, 2026),
> VS Code 1.116+, GitHub Copilot CLI (current), Node.js 22.x LTS.

---

## Table of Contents

1. [What Is DeepSeek Harness](#1-what-is-deepseek-harness)
2. [Architecture at a Glance](#2-architecture-at-a-glance)
3. [Prerequisites](#3-prerequisites)
4. [Basic Setup — dsh from Zero](#4-basic-setup--dsh-from-zero)
5. [DeepSeek × GitHub Copilot Integration](#5-deepseek--github-copilot-integration)
6. [Devcontainer Setup for VS Code](#6-devcontainer-setup-for-vs-code)
7. [DeepSeek Harness inside Google Scion](#7-deepseek-harness-inside-google-scion)
8. [Makefile-Driven Workflow](#8-makefile-driven-workflow)
9. [CI/CD — GitHub Actions Integration](#9-cicd--github-actions-integration)
10. [Security Considerations](#10-security-considerations)
11. [Troubleshooting](#11-troubleshooting)
12. [Appendix — Quick Reference](#12-appendix--quick-reference)

---

## 1. What Is DeepSeek Harness

DeepSeek assembled its Agent Harness team in March 2026 with one internal goal: benchmark
Claude Code and build a competing open-source agent runtime. The result — DeepSeek Harness
(dsh) — shipped publicly on August 13, 2026, the same day as DeepSeek V4-Pro, under the
MIT license.

The core thesis: **everything is a plugin.** The model adapter, the tool registry, the session
log, the permission system, the agent loop itself — all replaceable Cordis plugins. Where
Claude Code and Codex are finished products with fixed architectures, dsh is a composable
runtime where you own the full stack.

Key primitives:

- **Cordis** — the reactive plugin framework underneath; its design paper is titled
  *A Programming Paradigm for Spatiotemporal Composability*
- **Presets** — Standard, Code, Minimal, Creator — each a curated plugin bundle
- **Profiles** — `web` (browser UI), `headless` (single-shot CLI), `acp` (JSON-RPC stdio
  for programmatic control)
- **Sessions** — append-only logs of every system prompt, reasoning trace, tool call,
  and context injection, inspectable in the Trajectory view
- **Plugins** — npm packages with a `cordis.patch.yml` that compose through Cordis's
  patch layer; 367+ existed within 24 hours of launch

**The `dsh` in the command name has nothing to do with the older Unix distributed shell.
Know this before you search.**

---

## 2. Architecture at a Glance

```
┌──────────────────────────────────────────────────────┐
│                  DeepSeek Harness (dsh)               │
│                                                      │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │
│  │  Model    │  │  Tool    │  │  Session / Memory │  │
│  │  Provider │  │  Registry│  │  (append-only log)│  │
│  │  Plugin   │  │  Plugins │  │                   │  │
│  └─────┬────┘  └─────┬────┘  └────────┬──────────┘  │
│        │              │               │              │
│  ┌─────┴──────────────┴───────────────┴──────────┐   │
│  │              Cordis Plugin Runtime             │   │
│  │       (reactive services, events, patches)     │   │
│  └───────────────────────────────────────────────┘   │
│                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐  │
│  │  Web UI      │  │  Headless    │  │  ACP      │  │
│  │  :3080       │  │  CLI         │  │  JSON-RPC │  │
│  └──────────────┘  └──────────────┘  └───────────┘  │
└──────────────────────────────────────────────────────┘
         │                    │                │
   Browser/VS Code    Shell scripts/CI    Scion / Buzz
```

---

## 3. Prerequisites

### Minimum system requirements

```bash
# Node.js — MUST be ^22.19 or >=24 (odd releases like 23 will fail)
node --version   # v22.22.x or v24.x.x

# pnpm — required for plugin management
npm install -g pnpm

# Git
git --version

# Docker — required for devcontainers and Scion
docker --version

# Go — for Scion CLI (built from source)
go version   # 1.22+
```

### API keys you will need

| Service | Variable | Where to get it |
|---|---|---|
| DeepSeek | `DEEPSEEK_API_KEY` | https://platform.deepseek.com/api_keys |
| GitHub Copilot | `GH_TOKEN` (fine-grained PAT) | GitHub Settings → Developer Settings → Fine-grained PATs |
| Google (for Scion/Gemini) | `GEMINI_API_KEY` | https://aistudio.google.com/apikey |

---

## 4. Basic Setup — dsh from Zero

### 4.1 One-command install and launch

```bash
# Create a workspace directory — dsh scopes file access to this
mkdir -p ~/projects/dsh-workspace && cd ~/projects/dsh-workspace

# Launch with npx (downloads on first run, ~2 minutes of silence)
npx @deepseek-ai/dsh@0.1.0-rc.8 web
```

The Web UI opens at `http://127.0.0.1:3080`. This is a **loopback-only** server holding
your API key and with filesystem access. Never bind it to `0.0.0.0`.

### 4.2 Global install (preferred for repeated use)

```bash
# Global install gives you the bare `dsh` command
npm install -g @deepseek-ai/dsh@0.1.0-rc.8

# Verify
dsh --version
# 0.1.0-rc.8

# Launch the web UI
dsh web

# Or run a one-shot headless task
dsh --profile headless "Summarize this workspace and identify its main packages"
```

### 4.3 Build from source

```bash
git clone https://github.com/deepseek-ai/deepseek-harness.git
cd deepseek-harness
pnpm install
pnpm run build
pnpm dsh web
```

### 4.4 Python SDK (alternative entry point)

```bash
# Use uv, not pip
uv pip install deepseek-harness-sdk
```

The Python SDK bundles the Node.js runtime — no system Node required on the host.

### 4.5 First-run configuration

After the Web UI opens:

1. **Settings → Models** — paste your `DEEPSEEK_API_KEY`, save
2. **Choose a workspace** — the UI disables the session composer until one is selected
3. **Select a model** — `deepseek-v4-pro` or `deepseek-v4-flash` (Flash is cheaper, often sufficient for code tasks)
4. **Choose an Agent preset** — start with `Code` for development work
5. **Review the permission mode** — start restrictive, open up as you build trust

### 4.6 File layout

```
~/.dsh/                          # DSH_HOME — auto-created on first run
├── storages/                    # Session data (keep out of VCS)
├── profiles/
│   ├── web/                     # Web UI profile config
│   ├── headless/                # CLI one-shot profile
│   └── acp/                     # Programmatic (JSON-RPC) profile
└── cordis.yml                   # Global plugin configuration
```

### 4.7 Plugin management

```bash
# List installed plugins
dsh plugin --profile web list

# Install a plugin (example: filesystem search)
dsh plugin --profile web install @deepseek-ai/dsh-tool-fs-search

# Core official plugins
# @deepseek-ai/dsh-tool-fs          — filesystem read/write
# @deepseek-ai/dsh-tool-fs-search   — file search
# @deepseek-ai/dsh-tool-web         — web access
# @deepseek-ai/dsh-mcp-client       — MCP server connections
# @deepseek-ai/dsh-tool-subagent    — delegate to child agents
```

---

## 5. DeepSeek × GitHub Copilot Integration

There are three distinct integration paths. They serve different workflows.

### 5.1 Path A — DeepSeek V4 in the Copilot Chat Model Picker (VS Code)

This adds DeepSeek V4 Pro and Flash as selectable models inside GitHub Copilot Chat.
You keep Copilot's agent mode, tool calling, skills, and MCP — powered by DeepSeek's models.

**Install the extension:**

Search VS Code Marketplace for `DeepSeek V4 for Copilot Chat` (publisher: Vizards),
or install from the repo at `github.com/Vizards/deepseek-v4-for-copilot`.

**Requirements:**
- VS Code 1.116+
- GitHub Copilot subscription (Free tier works)
- DeepSeek API key

**Configure:**

```
Cmd+Shift+P → "DeepSeek: Set API Key" → paste your sk-... key
```

The key is stored in the OS keychain, not on disk. Then open Copilot Chat, click the
model picker, and select `DeepSeek V4 Pro` or `DeepSeek V4 Flash`.

**Vision handling:** DeepSeek V4 is text-only. The extension proxies images through
another installed Copilot model (Claude, GPT-4o) to describe them, then sends the
description to DeepSeek. Flash Vision Exp handles images natively as a separate
experimental model.

**Key settings in `settings.json`:**

```jsonc
{
  // Model IDs sent to the DeepSeek API — change only for third-party endpoints
  "deepseekForCopilot.flashModelId": "deepseek-v4-flash",
  "deepseekForCopilot.proModelId": "deepseek-v4-pro",

  // Optional: custom API base URL (for self-hosted or proxy endpoints)
  "deepseekForCopilot.apiBaseUrl": "https://api.deepseek.com",

  // Max output tokens (0 = no limit)
  "deepseekForCopilot.maxTokens": 8192,

  // Thinking mode (optional extended reasoning)
  "deepseekForCopilot.enableThinking": false,

  // Diagnostic level: "minimal" | "metadata" | "verbose"
  "deepseekForCopilot.diagnosticMode": "minimal"
}
```

### 5.2 Path B — Copilot CLI with DeepSeek via BYOK

This configures the standalone `copilot` CLI (formerly `gh copilot`, now `@github/copilot`)
to use DeepSeek as its backend model. Full agent mode, tool calling, and MCP support.

**Critical:** Use `anthropic` as the provider type, not `openai`. The OpenAI provider
triggers a 400 error because DeepSeek requires `reasoning_content` to be echoed back
on subsequent requests, which Copilot CLI's OpenAI integration does not support.
The Anthropic Messages API endpoint avoids this entirely.

**Bash/Zsh:**

```bash
# Export provider configuration
export COPILOT_PROVIDER_TYPE=anthropic
export COPILOT_PROVIDER_BASE_URL=https://api.deepseek.com/anthropic
export COPILOT_PROVIDER_API_KEY=sk-your-deepseek-api-key
export COPILOT_MODEL=deepseek-v4-pro

# Since deepseek-v4-pro isn't in Copilot CLI's built-in catalog,
# configure token limits explicitly
export COPILOT_MODEL_MAX_INPUT_TOKENS=65536
export COPILOT_MODEL_MAX_OUTPUT_TOKENS=8192
```

**PowerShell:**

```powershell
$env:COPILOT_PROVIDER_TYPE="anthropic"
$env:COPILOT_PROVIDER_BASE_URL="https://api.deepseek.com/anthropic"
$env:COPILOT_PROVIDER_API_KEY="sk-your-deepseek-api-key"
$env:COPILOT_MODEL="deepseek-v4-pro"
$env:COPILOT_MODEL_MAX_INPUT_TOKENS="65536"
$env:COPILOT_MODEL_MAX_OUTPUT_TOKENS="8192"
```

**Available models:** `deepseek-v4-pro`, `deepseek-v4-flash`.
Switch by changing `COPILOT_MODEL`.

**Persistent configuration (~/.bashrc or ~/.zshrc):**

```bash
# DeepSeek BYOK for Copilot CLI
export COPILOT_PROVIDER_TYPE=anthropic
export COPILOT_PROVIDER_BASE_URL=https://api.deepseek.com/anthropic
export COPILOT_PROVIDER_API_KEY="${DEEPSEEK_API_KEY}"
export COPILOT_MODEL=deepseek-v4-flash
export COPILOT_MODEL_MAX_INPUT_TOKENS=65536
export COPILOT_MODEL_MAX_OUTPUT_TOKENS=8192
```

Run `copilot help providers` for all available environment variables.

### 5.3 Path C — OAI-Compatible Provider Extension (multi-provider)

The `OAI Compatible Provider for Copilot` extension supports multiple OpenAI-compatible
providers simultaneously, including DeepSeek. It auto-manages API keys without manual
switching and supports vision models and reasoning/thinking content display.

**VS Code settings:**

```jsonc
{
  "oaicopilot.providers": [
    {
      "name": "DeepSeek",
      "baseUrl": "https://api.deepseek.com/v1",
      "apiKey": "${env:DEEPSEEK_API_KEY}",
      "models": [
        { "id": "deepseek-v4-pro", "label": "DeepSeek V4 Pro" },
        { "id": "deepseek-v4-flash", "label": "DeepSeek V4 Flash" }
      ]
    }
  ]
}
```

### 5.4 Using dsh + Copilot side-by-side in VS Code

The DeepSeek Harness VS Code extension (multiple community implementations exist) embeds
the dsh Web UI or a native sidebar chat alongside Copilot. This gives you two independent
agent runtimes in one editor:

- **Copilot** — inline completions, quick chat, established ecosystem
- **dsh** — full agent mode with workspace-level tool use, plugin extensibility,
  session trajectory inspection

Install a dsh VS Code extension (e.g., `DeepSeek Harness for VS Code` from the
Marketplace, publisher Jager), then:

1. Open the DeepSeek Harness sidebar
2. Configure `DEEPSEEK_API_KEY` via the key button
3. Choose permission mode, model, and reasoning effort
4. Use `@dsh` as a chat participant in VS Code's native Chat panel

---

## 6. Devcontainer Setup for VS Code

A devcontainer that ships dsh pre-installed with Copilot, pinned Node.js, and a
development-ready environment.

### 6.1 Directory structure

```
.devcontainer/
├── devcontainer.json
├── Dockerfile
├── post-create.sh
└── dsh/
    └── cordis.yml          # dsh plugin presets
```

### 6.2 Dockerfile

```dockerfile
# .devcontainer/Dockerfile
# syntax=docker/dockerfile:1

FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

ARG NODE_MAJOR=22
ARG DSH_VERSION=0.1.0-rc.8

# ─── System packages ───────────────────────────────────────────
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl gnupg git jq tmux zsh \
       python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/*

# ─── Node.js 22.x LTS (dsh requires ^22.19 || >=24) ───────────
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y nodejs \
    && npm install -g pnpm@latest

# ─── DeepSeek Harness (pinned) ─────────────────────────────────
RUN npm install -g @deepseek-ai/dsh@${DSH_VERSION}

# ─── GitHub Copilot CLI ───────────────────────────────────────
# Requires Node.js 22+
RUN npm install -g @github/copilot

# ─── Go (for Scion, tooling) ──────────────────────────────────
ARG GO_VERSION=1.23.1
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/home/vscode/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# ─── Terraform ─────────────────────────────────────────────────
ARG TF_VERSION=1.10.3
RUN curl -fsSL "https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_amd64.zip" \
    | funzip > /usr/local/bin/terraform \
    && chmod +x /usr/local/bin/terraform

# ─── gh CLI ────────────────────────────────────────────────────
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# ─── uv (Python package manager, preferred over pip) ──────────
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/root/.local/bin:/home/vscode/.local/bin:${PATH}"

# ─── Non-root user setup ──────────────────────────────────────
USER vscode
WORKDIR /workspaces
```

### 6.3 devcontainer.json

```jsonc
// .devcontainer/devcontainer.json
{
  "name": "DeepSeek Harness + Copilot Dev Environment",
  "build": {
    "dockerfile": "Dockerfile",
    "args": {
      "NODE_MAJOR": "22",
      "DSH_VERSION": "0.1.0-rc.8",
      "GO_VERSION": "1.23.1",
      "TF_VERSION": "1.10.3"
    }
  },

  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2": {},
    "ghcr.io/devcontainers/features/git:1": {}
  },

  // Forward dsh web UI port
  "forwardPorts": [3080],
  "portsAttributes": {
    "3080": {
      "label": "DeepSeek Harness Web UI",
      "onAutoForward": "notify"
    }
  },

  // VS Code extensions
  "customizations": {
    "vscode": {
      "extensions": [
        "github.copilot",
        "github.copilot-chat",
        "Vizards.deepseek-v4-for-copilot",
        "Jager.dsh-vscode",
        "hashicorp.terraform",
        "golang.go",
        "ms-python.python",
        "redhat.vscode-yaml"
      ],
      "settings": {
        "terminal.integrated.defaultProfile.linux": "zsh",
        "editor.formatOnSave": true,

        // Copilot settings
        "github.copilot.enable": {
          "*": true,
          "yaml": true,
          "terraform": true,
          "go": true
        },

        // DeepSeek V4 for Copilot Chat
        "deepseekForCopilot.flashModelId": "deepseek-v4-flash",
        "deepseekForCopilot.proModelId": "deepseek-v4-pro",
        "deepseekForCopilot.maxTokens": 8192
      }
    }
  },

  // Secrets — injected as env vars, never committed
  "secrets": {
    "DEEPSEEK_API_KEY": {
      "description": "DeepSeek Platform API key (starts with sk-)"
    },
    "GH_TOKEN": {
      "description": "GitHub fine-grained PAT with Copilot Requests permission"
    }
  },

  // Post-create setup
  "postCreateCommand": "bash .devcontainer/post-create.sh",

  // Run as non-root
  "remoteUser": "vscode"
}
```

### 6.4 Post-create script

```bash
#!/usr/bin/env bash
# .devcontainer/post-create.sh
set -euo pipefail

echo "=== DeepSeek Harness + Copilot — post-create setup ==="

# ─── Verify toolchain ──────────────────────────────────────────
for cmd in node npm pnpm dsh copilot go terraform gh git; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: $cmd not found in PATH" >&2
        exit 1
    fi
done

echo "Node.js    : $(node --version)"
echo "dsh        : $(dsh --version)"
echo "Copilot CLI: $(copilot --version 2>/dev/null || echo 'installed')"
echo "Go         : $(go version)"
echo "Terraform  : $(terraform version -json | jq -r .terraform_version)"
echo "gh         : $(gh --version | head -1)"

# ─── Configure Copilot CLI for DeepSeek BYOK ──────────────────
if [ -n "${DEEPSEEK_API_KEY:-}" ]; then
    cat >> ~/.zshrc << 'COPILOT_BYOK'

# DeepSeek BYOK for Copilot CLI
export COPILOT_PROVIDER_TYPE=anthropic
export COPILOT_PROVIDER_BASE_URL=https://api.deepseek.com/anthropic
export COPILOT_PROVIDER_API_KEY="${DEEPSEEK_API_KEY}"
export COPILOT_MODEL=deepseek-v4-flash
export COPILOT_MODEL_MAX_INPUT_TOKENS=65536
export COPILOT_MODEL_MAX_OUTPUT_TOKENS=8192
COPILOT_BYOK
    echo "Copilot CLI configured for DeepSeek BYOK"
fi

# ─── Pre-install core dsh plugins ─────────────────────────────
if command -v dsh &>/dev/null; then
    echo "Installing core dsh plugins..."
    dsh plugin --profile web install @deepseek-ai/dsh-tool-fs-search 2>/dev/null || true
    dsh plugin --profile web install @deepseek-ai/dsh-mcp-client 2>/dev/null || true
fi

# ─── Copy dsh preset config if present ─────────────────────────
if [ -f ".devcontainer/dsh/cordis.yml" ]; then
    mkdir -p ~/.dsh
    cp .devcontainer/dsh/cordis.yml ~/.dsh/cordis.yml
    echo "dsh cordis.yml installed"
fi

# ─── Git config (container-local) ─────────────────────────────
git config --global init.defaultBranch main
git config --global pull.rebase true

echo "=== Setup complete ==="
echo "Run 'dsh web' to start DeepSeek Harness Web UI on port 3080"
echo "Run 'copilot' to start Copilot CLI with DeepSeek backend"
```

### 6.5 dsh Cordis config preset

```yaml
# .devcontainer/dsh/cordis.yml
# Minimal Cordis configuration for development
# See: https://github.com/deepseek-ai/deepseek-harness/blob/main/docs/

# Plugin overrides (empty = use defaults from preset)
plugins: {}

# Provider configuration (API key set via env or Web UI)
# providers:
#   deepseek:
#     type: deepseek
#     models:
#       - deepseek-v4-flash
#       - deepseek-v4-pro
```

### 6.6 Usage inside the devcontainer

```bash
# Terminal 1: Start dsh Web UI
dsh web
# → opens http://127.0.0.1:3080 (forwarded by VS Code)

# Terminal 2: Copilot CLI with DeepSeek backend
copilot "Refactor the error handling in cmd/server/main.go to use error wrapping"

# Terminal 3: Headless dsh for one-shot tasks
dsh --profile headless "Review this repository's Makefile for security issues"
```

---

## 7. DeepSeek Harness inside Google Scion

### 7.1 What Scion is

Scion is Google's open-source multi-agent orchestration testbed — a "hypervisor for agents."
It manages deep agents (Claude Code, Gemini CLI, Codex, Copilot CLI, and others) as isolated,
concurrent processes. Each agent gets its own container, git worktree, and credentials.

Key Scion concepts:

- **Grove** — top-level grouping, represented by a `.scion/` directory
- **Agent** — an isolated container running an LLM harness
- **Template** — a blueprint defining harness type, env vars, system prompt, and home directory
- **Harness** — adapter between Scion and a specific LLM tool
- **Runtime Broker** — manages container lifecycle (Docker, Podman, Apple Container, K8s)

### 7.2 Current harness support status

As of August 2026, Scion ships with these harnesses:

| Harness | Status | Default? |
|---|---|---|
| Gemini CLI | Supported | ✅ Installed by default |
| Claude Code | Supported | ❌ Opt-in bundle |
| Antigravity | Supported | ✅ Installed by default |
| OpenCode | Experimental | ❌ Opt-in |
| Codex | Supported | ❌ Opt-in |
| Copilot CLI | Supported | ❌ Opt-in |
| Hermes | Supported | ❌ Opt-in |
| Grok Build | Supported | ❌ Opt-in |
| **DeepSeek Harness** | **Not yet supported** | — |

**DeepSeek Harness is not a built-in Scion harness.** But Scion's architecture is
harness-agnostic, and every harness is now defined declaratively as a bundle under
`harnesses/<name>/` with a `config.yaml` plus a `provision.py` container-script.
This means we can build a custom harness bundle.

### 7.3 Installing Scion

```bash
# Install the Scion CLI from source
git clone https://github.com/GoogleCloudPlatform/scion.git
cd scion
go install ./cmd/scion

# Verify
scion version

# Initialize a grove in your project
cd ~/projects/my-project
scion init

# This creates .scion/ with default settings
# Add .scion/agents to .gitignore
echo ".scion/agents" >> .gitignore
```

Scion auto-detects your OS and configures the runtime:
- **Linux/Windows**: Docker
- **macOS**: Apple Container (override in `.scion/settings.yaml`)

### 7.4 Building a custom DeepSeek Harness bundle for Scion

Since dsh isn't a native Scion harness, we build one. Scion's harness-development
documentation describes the interface: a `config.yaml`, a `provision.py` that runs
inside the agent container, and communication with the orchestrator via `sciontool`.

#### 7.4.1 Custom container image

```dockerfile
# scion-dsh/Dockerfile
# OCI image for running DeepSeek Harness inside a Scion agent container

FROM node:22-bookworm-slim

ARG DSH_VERSION=0.1.0-rc.8

# ─── System dependencies ──────────────────────────────────────
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl jq tmux python3 python3-pip ca-certificates openssh-client \
    && rm -rf /var/lib/apt/lists/*

# ─── pnpm + dsh ───────────────────────────────────────────────
RUN npm install -g pnpm@latest \
    && npm install -g @deepseek-ai/dsh@${DSH_VERSION}

# ─── Scion compatibility layer ─────────────────────────────────
# sciontool is injected by Scion at /usr/local/bin/sciontool
# We create the expected home structure
RUN useradd -m -s /bin/bash scion
USER scion
WORKDIR /home/scion

# ─── Provision script entry point ──────────────────────────────
COPY provision.py /opt/scion-harness/provision.py

ENTRYPOINT ["/bin/bash"]
```

#### 7.4.2 Harness config.yaml

```yaml
# scion-dsh/config.yaml
# Scion harness bundle configuration for DeepSeek Harness

name: deepseek-harness
display_name: "DeepSeek Harness (dsh)"
description: "DeepSeek's open-source agent harness — everything is a plugin"

# Container image
image: "scion-dsh:latest"   # Build locally or push to a registry

# How the harness process is launched inside the container
command: ["dsh", "--profile", "headless"]

# Authentication
auth:
  methods:
    - type: api-key
      env_vars:
        - DEEPSEEK_API_KEY
      description: "DeepSeek Platform API key"

# Model aliases (Scion universal → DeepSeek model IDs)
model_aliases:
  small: "deepseek-v4-flash"
  medium: "deepseek-v4-flash"
  large: "deepseek-v4-pro"
  extra-large: "deepseek-v4-pro"

# Default model when none specified
default_model: "deepseek-v4-flash"

# Session resume support
supports_resume: false   # dsh doesn't yet have a --resume/--continue equivalent

# Capabilities
capabilities:
  hooks: false
  opentelemetry: false
  system_prompt_override: false   # dsh manages its own system prompts via presets
  mcp: true                       # via @deepseek-ai/dsh-mcp-client plugin
```

#### 7.4.3 Provision script

```python
#!/usr/bin/env python3
"""
scion-dsh/provision.py
Scion container-script provisioner for DeepSeek Harness.

This runs inside the agent container during startup.
It configures dsh, resolves auth, and sets up the workspace.
"""

import json
import os
import subprocess
import sys
from pathlib import Path

# ─── Scion context (populated by the orchestrator) ─────────────
WORKSPACE = os.environ.get("SCION_WORKSPACE", "/workspace")
MODEL = os.environ.get("SCION_MODEL", "")
AGENT_INSTRUCTIONS = os.environ.get("SCION_AGENT_INSTRUCTIONS", "")
SYSTEM_PROMPT = os.environ.get("SCION_SYSTEM_PROMPT", "")
DSH_HOME = Path.home() / ".dsh"

# ─── Model alias resolution ────────────────────────────────────
MODEL_ALIASES = {
    "small": "deepseek-v4-flash",
    "medium": "deepseek-v4-flash",
    "large": "deepseek-v4-pro",
    "extra-large": "deepseek-v4-pro",
    # Single-letter aliases (Scion convention)
    "S": "deepseek-v4-flash",
    "M": "deepseek-v4-flash",
    "L": "deepseek-v4-pro",
}


def resolve_model(requested: str) -> str:
    """Resolve a Scion model alias to a DeepSeek model ID."""
    if not requested:
        return "deepseek-v4-flash"
    return MODEL_ALIASES.get(requested, requested)


def setup_auth():
    """Resolve and configure the DeepSeek API key."""
    api_key = os.environ.get("DEEPSEEK_API_KEY", "")
    if not api_key:
        print("WARNING: No DEEPSEEK_API_KEY found. Agent will drop to shell.", file=sys.stderr)
        # Signal Scion that we need user input
        subprocess.run(
            ["sciontool", "status", "ask_user", "DEEPSEEK_API_KEY is not set."],
            check=False,
        )
        return False
    return True


def setup_workspace():
    """Configure dsh to use the Scion-managed workspace."""
    os.makedirs(DSH_HOME, exist_ok=True)

    # dsh expects a workspace directory — Scion provides one
    workspace_path = Path(WORKSPACE)
    if not workspace_path.exists():
        workspace_path.mkdir(parents=True, exist_ok=True)


def write_instructions():
    """Project agent instructions if provided by the Scion template."""
    if AGENT_INSTRUCTIONS or SYSTEM_PROMPT:
        instructions_dir = Path(WORKSPACE) / ".dsh-instructions"
        instructions_dir.mkdir(exist_ok=True)

        combined = ""
        if SYSTEM_PROMPT:
            combined += SYSTEM_PROMPT + "\n\n"
        if AGENT_INSTRUCTIONS:
            combined += AGENT_INSTRUCTIONS

        (instructions_dir / "INSTRUCTIONS.md").write_text(combined)
        print(f"Instructions written to {instructions_dir / 'INSTRUCTIONS.md'}")


def build_launch_command() -> list[str]:
    """Build the dsh command for this agent session."""
    model = resolve_model(MODEL)
    cmd = ["dsh", "--profile", "headless"]

    # The headless profile reads the task from the command line
    # Scion sends the task via the resume/interject mechanism
    return cmd


def main():
    print("=== DeepSeek Harness Scion Provisioner ===")
    print(f"Workspace: {WORKSPACE}")
    print(f"Model: {resolve_model(MODEL)}")

    if not setup_auth():
        sys.exit(1)

    setup_workspace()
    write_instructions()

    # Report ready status to Scion
    subprocess.run(
        ["sciontool", "status", "task_completed", "Provisioning complete."],
        check=False,
    )

    # Build and print the launch command for the harness wrapper
    cmd = build_launch_command()
    print(f"Launch command: {' '.join(cmd)}")


if __name__ == "__main__":
    main()
```

#### 7.4.4 Build and register the custom image

```bash
# Build the custom Scion-compatible dsh image
cd scion-dsh/
docker build -t scion-dsh:latest .

# For remote/Hub deployments, push to a registry
# docker tag scion-dsh:latest gcr.io/my-project/scion-dsh:latest
# docker push gcr.io/my-project/scion-dsh:latest
```

#### 7.4.5 Install the harness bundle in Scion

```bash
# Copy the harness bundle into Scion's harness-configs directory
mkdir -p ~/.scion/harness-configs/deepseek-harness/
cp scion-dsh/config.yaml ~/.scion/harness-configs/deepseek-harness/
mkdir -p ~/.scion/harness-configs/deepseek-harness/home/
```

### 7.5 Creating a Scion template for DeepSeek Harness

```yaml
# .scion/templates/dsh-coder/scion-agent.yaml
harness: deepseek-harness
image: scion-dsh:latest

# Model override (uses alias resolution from config.yaml)
model: large   # → deepseek-v4-pro

# Environment variables
environment:
  DEEPSEEK_API_KEY: "${DEEPSEEK_API_KEY}"
  # DSH_HOME is set automatically by the image

# Agent instructions (projected into the workspace by provision.py)
agent_instructions: |
  You are a senior platform engineer specializing in infrastructure automation.
  Focus on: Terraform, Go, GitHub Actions, Docker, and Kubernetes.
  Follow Unix philosophy: small, composable, single-purpose tools.
  Always include error handling with set -euo pipefail in shell scripts.
  Write idiomatic Go — run gofmt, go vet, and go test before declaring done.

# Resource limits
resources:
  memory: "4Gi"
  cpu: "2"
```

### 7.6 Running DeepSeek Harness as a Scion agent

```bash
# Start a dsh agent using the custom template
scion start dsh-coder "Refactor the Terraform modules to use for_each instead of count" --attach

# Start alongside other agents for multi-agent workflows
scion start dsh-coder "Implement the new API endpoint in Go" --attach &
scion start claude "Review the Go implementation for security issues" --attach &
scion start gemini "Write integration tests for the new endpoint" --attach &
wait

# List running agents
scion agents list

# Attach to a running agent's tmux session
scion attach <agent-id>

# Send a follow-up task to a running agent
scion interject <agent-id> "Also add OpenTelemetry tracing spans"
```

### 7.7 Multi-agent workflow example: code + review + test

```bash
#!/usr/bin/env bash
# multi-agent-workflow.sh
# Three agents collaborate on a feature using Scion
set -euo pipefail

FEATURE_BRANCH="feature/add-health-endpoint"

# Create a feature branch
git checkout -b "${FEATURE_BRANCH}"

# Agent 1: DeepSeek Harness — implement the feature
scion start dsh-coder \
  "Implement a /healthz endpoint in cmd/server/main.go that returns
   JSON with service name, version from build tags, uptime, and
   dependency status checks for postgres and redis." \
  --attach

# Agent 2: Claude Code — security review
scion start claude \
  "Review all changes on the current branch for security issues.
   Focus on: input validation, error information leakage, timing
   attacks in health checks, and dependency status exposure." \
  --attach

# Agent 3: Gemini CLI — write tests
scion start gemini \
  "Write table-driven Go tests for the /healthz endpoint.
   Cover: healthy state, postgres down, redis down, both down.
   Use httptest.NewServer. Run go test -v -race ./..." \
  --attach
```

---

## 8. Makefile-Driven Workflow

A composable Makefile that wraps dsh, Copilot, and Scion operations.

```makefile
# Makefile — DeepSeek Harness + Copilot + Scion workflow targets
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

# ─── Configuration ─────────────────────────────────────────────
DSH_VERSION    := 0.1.0-rc.8
DSH_PORT       := 3080
DSH_PROFILE    := headless
NODE_VERSION   := 22

# ─── Validation ────────────────────────────────────────────────
.PHONY: check-deps
check-deps: ## Verify all required tools are installed
	@echo "Checking dependencies..."
	@command -v node >/dev/null 2>&1 || { echo "ERROR: node not found"; exit 1; }
	@command -v dsh >/dev/null 2>&1 || { echo "ERROR: dsh not found. Run: npm install -g @deepseek-ai/dsh@$(DSH_VERSION)"; exit 1; }
	@command -v copilot >/dev/null 2>&1 || { echo "WARN: copilot CLI not found"; }
	@command -v scion >/dev/null 2>&1 || { echo "WARN: scion not found"; }
	@node -e "const [,m]=process.versions.node.split('.').map(Number); if(m<19&&parseInt(process.versions.node)<24){console.error('ERROR: Node ^22.19 or >=24 required');process.exit(1)}"
	@echo "All core dependencies OK"

# ─── DeepSeek Harness ─────────────────────────────────────────
.PHONY: dsh-web dsh-headless dsh-plugins dsh-stop
dsh-web: check-deps ## Start dsh Web UI on port $(DSH_PORT)
	dsh web --port $(DSH_PORT)

dsh-headless: check-deps ## Run a one-shot dsh task (TASK= required)
	@test -n "$(TASK)" || { echo "Usage: make dsh-headless TASK='your task here'"; exit 1; }
	dsh --profile $(DSH_PROFILE) "$(TASK)"

dsh-plugins: check-deps ## List installed dsh plugins
	dsh plugin --profile web list

dsh-stop: ## Kill any running dsh processes
	@pkill -f "dsh web" 2>/dev/null || echo "No dsh process found"

# ─── Copilot CLI ───────────────────────────────────────────────
.PHONY: copilot-deepseek copilot-review
copilot-deepseek: ## Run Copilot CLI with DeepSeek backend (TASK= required)
	@test -n "$(TASK)" || { echo "Usage: make copilot-deepseek TASK='your task here'"; exit 1; }
	COPILOT_PROVIDER_TYPE=anthropic \
	COPILOT_PROVIDER_BASE_URL=https://api.deepseek.com/anthropic \
	COPILOT_PROVIDER_API_KEY=$${DEEPSEEK_API_KEY} \
	COPILOT_MODEL=deepseek-v4-flash \
	copilot "$(TASK)"

copilot-review: ## Code review with Copilot + DeepSeek
	COPILOT_PROVIDER_TYPE=anthropic \
	COPILOT_PROVIDER_BASE_URL=https://api.deepseek.com/anthropic \
	COPILOT_PROVIDER_API_KEY=$${DEEPSEEK_API_KEY} \
	COPILOT_MODEL=deepseek-v4-pro \
	copilot "Review the staged changes (git diff --cached) for bugs, security issues, and style violations"

# ─── Scion Multi-Agent ────────────────────────────────────────
.PHONY: scion-init scion-start scion-agents scion-clean
scion-init: ## Initialize a Scion grove in the current directory
	scion init
	echo ".scion/agents" >> .gitignore

scion-start: ## Start a DeepSeek Harness agent via Scion (TASK= required)
	@test -n "$(TASK)" || { echo "Usage: make scion-start TASK='your task'"; exit 1; }
	scion start dsh-coder "$(TASK)" --attach

scion-agents: ## List running Scion agents
	scion agents list

scion-clean: ## Stop all Scion agents
	scion agents list --format json 2>/dev/null \
	  | jq -r '.[].id' \
	  | xargs -I{} scion stop {} 2>/dev/null || true

# ─── Devcontainer ─────────────────────────────────────────────
.PHONY: devcontainer-build devcontainer-up
devcontainer-build: ## Build the devcontainer image
	docker build -t dsh-copilot-dev:latest .devcontainer/

devcontainer-up: ## Start the devcontainer
	devcontainer up --workspace-folder .

# ─── Help ─────────────────────────────────────────────────────
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
```

---

## 9. CI/CD — GitHub Actions Integration

### 9.1 Running dsh headless in a workflow

```yaml
# .github/workflows/dsh-review.yml
name: DeepSeek Harness Code Review

on:
  pull_request:
    types: [opened, synchronize]

permissions:
  contents: read
  pull-requests: write

jobs:
  dsh-review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-node@v4
        with:
          node-version: '22'

      - name: Install DeepSeek Harness
        run: npm install -g @deepseek-ai/dsh@0.1.0-rc.8

      - name: Run dsh code review
        env:
          DEEPSEEK_API_KEY: ${{ secrets.DEEPSEEK_API_KEY }}
        run: |
          DIFF=$(git diff origin/main...HEAD --stat)
          dsh --profile headless \
            "Review these changes for bugs and security issues. Be concise.
             Changed files:
             ${DIFF}" \
            > review-output.txt 2>&1

      - name: Post review comment
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const review = fs.readFileSync('review-output.txt', 'utf8');
            await github.rest.issues.createComment({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: context.issue.number,
              body: `## 🔍 DeepSeek Harness Review\n\n${review}`
            });
```

### 9.2 Triggering Scion agents from a workflow

```yaml
# .github/workflows/scion-multi-agent.yml
name: Scion Multi-Agent Pipeline

on:
  workflow_dispatch:
    inputs:
      task:
        description: 'Task description for the agent swarm'
        required: true
        type: string

jobs:
  agent-swarm:
    runs-on: ubuntu-latest
    services:
      docker:
        image: docker:dind
        options: --privileged
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - uses: actions/setup-node@v4
        with:
          node-version: '22'

      - name: Install tools
        run: |
          npm install -g @deepseek-ai/dsh@0.1.0-rc.8
          go install github.com/GoogleCloudPlatform/scion/cmd/scion@latest

      - name: Initialize Scion grove
        run: scion init

      - name: Run agent swarm
        env:
          DEEPSEEK_API_KEY: ${{ secrets.DEEPSEEK_API_KEY }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          scion start dsh-coder "${{ inputs.task }}" --attach

      - name: Commit results
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add -A
          git diff --cached --quiet || git commit -m "feat: agent-generated changes

          Task: ${{ inputs.task }}
          Agent: dsh-coder (DeepSeek V4)"
          git push
```

---

## 10. Security Considerations

**dsh is a developer preview with the guard removed.** Treat it accordingly.

1. **Loopback only** — dsh web binds to `127.0.0.1:3080`. Passing `--host 0.0.0.0`
   causes the CLI to exit with an error (by design). Never proxy it to a public network
   without authentication.

2. **API key storage** — keys are write-only in the Web UI (the page receives a redacted
   descriptor after saving). In the Copilot CLI BYOK path, keys live in environment
   variables — use a secrets manager in CI, never hardcode.

3. **Workspace scoping** — dsh has filesystem access scoped to the selected workspace.
   Start with a bounded directory, not `/` or `$HOME`.

4. **Permission modes** — start restrictive. Review every requested tool call and
   shell command before approving, especially on first use.

5. **Plugin trust** — 367+ plugins appeared within 24 hours of launch. Inspect a
   package, test in a small scope, verify the result before production use.

6. **Scion isolation** — each Scion agent runs in its own container with its own
   credentials and git worktree. This is the correct security boundary for
   multi-agent workflows. Do not run multiple agents in a shared container.

7. **Session data** — `$DSH_HOME/storages` contains full session logs including
   API keys and workspace contents. Keep the entire `~/.dsh` directory out of
   version control.

---

## 11. Troubleshooting

### Node.js version mismatch

```
Error: DeepSeek Harness requires Node.js ^22.19 or >=24
```

dsh will not run on Node 23 (odd-numbered release). Install Node 22 LTS:

```bash
# Via nvm
nvm install 22
nvm use 22

# Or via nodesource
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo bash -
sudo apt-get install -y nodejs
```

### Web UI opens in Chinese

A fresh install may default to Chinese. Change the language in the Web UI settings,
or set the `LANG` environment variable before launching:

```bash
LANG=en_US.UTF-8 dsh web
```

### Copilot CLI 400 error with DeepSeek

```
400: The reasoning_content in the thinking mode must be passed back to the API
```

You used `openai` as the provider type. Switch to `anthropic`:

```bash
export COPILOT_PROVIDER_TYPE=anthropic
export COPILOT_PROVIDER_BASE_URL=https://api.deepseek.com/anthropic
```

### Scion custom harness not found

Ensure the harness bundle is in the correct directory:

```bash
ls ~/.scion/harness-configs/deepseek-harness/config.yaml
```

And that the Docker image is built and available locally:

```bash
docker images | grep scion-dsh
```

### dsh plugin install failures

Plugins require `pnpm` on your PATH:

```bash
npm install -g pnpm
```

---

## 12. Appendix — Quick Reference

### Environment variables

| Variable | Purpose | Where used |
|---|---|---|
| `DEEPSEEK_API_KEY` | DeepSeek Platform API key | dsh, Copilot BYOK, Scion |
| `COPILOT_PROVIDER_TYPE` | `anthropic` (not `openai`) | Copilot CLI |
| `COPILOT_PROVIDER_BASE_URL` | `https://api.deepseek.com/anthropic` | Copilot CLI |
| `COPILOT_PROVIDER_API_KEY` | Same as `DEEPSEEK_API_KEY` | Copilot CLI |
| `COPILOT_MODEL` | `deepseek-v4-pro` or `deepseek-v4-flash` | Copilot CLI |
| `DSH_HOME` | dsh configuration directory (default: `~/.dsh`) | dsh |
| `SCION_MODEL` | Model alias for Scion agents | Scion |
| `SCION_WORKSPACE` | Workspace path inside agent container | Scion |

### CLI cheatsheet

```bash
# ─── dsh ───────────────────────────────────────────────────────
dsh web                              # Start Web UI (:3080)
dsh web --port 4000                  # Custom port
dsh --profile headless "task"        # One-shot CLI
dsh --version                        # Version check
dsh plugin --profile web list        # List plugins
dsh plugin --profile web install PKG # Install a plugin

# ─── Copilot CLI with DeepSeek ─────────────────────────────────
copilot "task description"           # Run a task
copilot help providers               # List provider env vars

# ─── Scion ─────────────────────────────────────────────────────
scion init                           # Initialize grove
scion start TEMPLATE "task" --attach # Start agent
scion agents list                    # List agents
scion attach AGENT_ID                # Attach to agent tmux
scion interject AGENT_ID "message"   # Send follow-up to running agent
scion stop AGENT_ID                  # Stop agent
scion suspend AGENT_ID               # Suspend (resume later)
scion templates list                 # List templates
scion templates create NAME          # Create template
```

### Key URLs

| Resource | URL |
|---|---|
| DeepSeek Harness repo | https://github.com/deepseek-ai/deepseek-harness |
| DeepSeek Harness npm | https://www.npmjs.com/package/@deepseek-ai/dsh |
| DeepSeek API docs (Copilot) | https://api-docs.deepseek.com/quick_start/agent_integrations/github_copilot/ |
| DeepSeek API docs (Copilot CLI) | https://api-docs.deepseek.com/quick_start/agent_integrations/copilot_cli/ |
| DeepSeek V4 for Copilot extension | https://github.com/Vizards/deepseek-v4-for-copilot |
| Google Scion repo | https://github.com/GoogleCloudPlatform/scion |
| Scion docs | https://googlecloudplatform.github.io/scion/overview/ |
| Scion supported harnesses | https://googlecloudplatform.github.io/scion/supported-harnesses/ |
| Scion harness development | https://googlecloudplatform.github.io/scion/contributing/harness-dev/ |

---

> *"Simplicity is the ultimate sophistication."* — not actually Leonardo, but the sentiment
> applies. A good harness is a good shell: it gets out of the way and lets the tool do
> its job. dsh gets this right by making everything a plugin. Scion gets this right by
> making everything a container. The rest is plumbing — and plumbing is what we do.

---

**Document version**: 2026-08-25
**Verified against**: dsh 0.1.0-rc.8, Scion main (Aug 23, 2026), VS Code 1.116+
**License**: This guide is provided as-is. Pin your versions. Read the changelogs. Test everything.
