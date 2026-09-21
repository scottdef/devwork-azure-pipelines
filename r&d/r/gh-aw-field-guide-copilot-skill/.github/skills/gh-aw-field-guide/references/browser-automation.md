# Computer use / browser automation (workflow_dispatch only)

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.


Playwright is the supported browser tool. CLI mode (`tools.playwright.mode: cli`) is recommended — token-efficient, no Docker overhead, reaches `localhost` directly; it installs `@playwright/cli` globally and the agent calls `playwright-cli <command>` from bash. MCP mode is deprecated (emits a compile-time warning). Domain access is controlled by `network:`; by default Playwright reaches only `localhost`/`127.0.0.1`. Add `playwright` (enables browser downloads) plus explicit domains. Chromium/Firefox/WebKit are available.

Complete example (`CoolGitOrg/platform-automation/.github/workflows/browser-check.md`):
````markdown
---
name: "Manual Browser Check"
on:
  workflow_dispatch:
    inputs:
      url:
        description: "URL to inspect (must be under an allowed domain)"
        required: true
        default: "https://status.coolgitorg.example.com"
      viewport:
        description: "Viewport WxH"
        required: false
        default: "1440x900"
permissions:
  contents: read
engine: copilot
timeout_minutes: 15
tools:
  playwright:
    mode: cli
    version: "0.1.13"
  bash:
    - "playwright-cli:*"
network:
  allowed:
    - defaults
    - playwright
    - "status.coolgitorg.example.com"
    - "*.coolgitorg.example.com"
safe-outputs:
  upload-artifact:
    max-uploads: 1
    retention-days: 7
  create-issue:
    title-prefix: "[browser-check] "
    labels: [automation, ops]
    max: 1
---
# Manual Browser Check
Only inspect the domains in the network allow-list. Navigate to `${{ github.event.inputs.url }}`,
resize to `${{ github.event.inputs.viewport }}`, capture a full-page screenshot to
`/tmp/gh-aw/agent/screenshot.png`, and check for HTTP errors, console errors, and broken images.
```bash
playwright-cli browser_navigate --url "${{ github.event.inputs.url }}"
playwright-cli browser_take_screenshot --filename /tmp/gh-aw/agent/screenshot.png --full-page true
```
Upload the screenshot as an artifact and, if you find problems, open one issue summarizing them with the screenshot referenced. Otherwise call `noop`.
````
Hardening: `workflow_dispatch`-only (no automatic triggers); an explicit `network.allowed` list (Playwright cannot reach anything else — the AWF firewall drops it); `bash` restricted to `playwright-cli:*`; a `timeout_minutes` cap; screenshots via `upload-artifact` (auto-expiring, preferred over `upload-asset`); read-only permissions with writes only through safe-outputs. Pin `version:` to avoid browser-engine baseline drift.

**go-surf.** There is **no gh-aw "go-surf" tool and no verifiable project named exactly "go-surf."** The closest real Go project is **headzoo/surf** (`gopkg.in/headzoo/surf.v1`), a stateful programmatic virtual browser (cookies, history, form submission, goquery CSS selection) — not a headless-Chromium driver. Other Go browser options are chromedp and rod (DevTools Protocol) and playwright-go. gh-aw has no built-in Go browser tool, so a Go-based browser would be wired in either as (a) a bash step invoking a compiled Go binary within the AWF network allow-list, or (b) a custom MCP server (`mcp-servers:` entry running a container that exposes the Go tool). Treat that as a custom, unvetted integration: pin it, restrict `allowed:` tools, and constrain `network:`.

