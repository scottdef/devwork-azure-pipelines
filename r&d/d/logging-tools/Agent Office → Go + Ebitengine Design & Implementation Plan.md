# Agent Office → Go + Ebitengine: Design & Implementation Plan

Oct 22, 2026 · @crafty

## Executive summary

Agent Office becomes one static Go binary (Go 1.24, Ebitengine v2.9) that runs the office server, a 2D pixel-art desktop client, and a WebAssembly build of that same client served at `/`. The port keeps the existing JSON-over-WebSocket protocol and the `.agent-office/` on-disk layout byte-compatible, so the Go server can take over a running office under the current TypeScript client first (a strangler migration), and the Ebitengine client lands second against a server that is already proven.

What is being ported: about 100,600 lines of TypeScript in 608 files (server 22,800 lines, three.js client 24,100 lines, shared layout/protocol 8,100 lines, 83 test files), plus five native dependencies the Go build replaces: `@lydell/node-pty`, `@xterm/headless` + `addon-serialize`, `ws`, `selfsigned`, and Excalidraw.

The six decisions that shape everything else:

1. **Protocol and files first, pixels second.** Every `t:` message type, every `.agent-office/*.json` file, the hook server, the `office-workers`/`office-queue` CLIs and the MCP server are reproduced exactly. Milestone 1 is the existing browser client running unchanged against the Go server.
2. **Top-down 3/4 pixel art at 32 px per meter.** The world already lives in meters in `shared/layout.ts` (a 36 m x 26 m floor, every desk, board and seat positioned). The renderer projects those coordinates; an isometric projector is a drop-in alternative behind one interface, not a second world.
3. **One client, two targets.** The Ebitengine client compiles natively (Windows, macOS, Linux) and to Wasm. The server embeds and serves the Wasm build, so the browser deployment, the SSH-tunnel and Tailscale stories survive. The phone `/lite` view becomes a list-mode scene inside the same client.
4. **Terminals stay real.** The detached PTY host becomes a subcommand of the same binary (Unix socket, same adoption dance), `charmbracelet/x/vt` replaces the headless xterm mirror, and the in-game terminal renders the same per-row `Run` diffs with a bitmap monospace font. Workers keep surviving server restarts.
5. **Browser-only features use the browser.** In the Wasm build, voice, screen sharing and dictation call the browser's WebRTC and Web Speech APIs through `syscall/js`. Native voice (pion/webrtc) is a later phase; native dictation is out of scope.
6. **Replace, do not emulate, the unportable.** Excalidraw becomes a native stroke whiteboard with the same reconcile rule; web-page tabs open the system browser; the synthesized drum-and-bass set and the driveable scenic loop are last.

The plan runs in five phases over about 20 weeks for two to three engineers: Go server under the TS client (week 6), walk-hire-watch-type in Ebitengine (week 10), daily-work parity (week 14), toys and the castle and station maps (week 18), then voice, cars and the rooftop. Each phase has an exit test, and the ported golden tests gate every phase.

## What Agent Office is today

Agent Office is a Node 20+ server plus a three.js browser client: a shared office where AI coding agents (Claude Code, Codex, OpenCode, Grok, Muse, DeepSeek Harness, Pi, Cursor) run in PTYs at desks, each GitHub repository is a floor, and people walk around, read the issue and PR boards, open live terminals and hand out work. The code splits into `src/shared` (pure types and geometry both sides use), `src/server` (Node) and `src/client` (Vite + three.js), with registries so a feature is a folder plus one line in each list it joins.

| Subsystem | Where | Lines | What it owns |
| --- | --- | --- | --- |
| Shared layout and geometry | `src/shared/layout.ts`, `nav.ts`, `floorplan.ts`, `maps/` | 8,100 | A 36 x 26 m floor in meters: 16 desks + 4 back-office desks, 12 bean bags, 3 board-agent kiosks, 5 meeting chairs, boards, elevator, loft, balcony, rooftop; a half-meter `NavGrid` with A\*; `MapConfig` to `MapPlan` for castle and station maps |
| Wire protocol | `src/shared/protocol/*.ts` | 1,600 | 13 domains, one JSON object per WebSocket frame with a `t` discriminator; about 95 client and 60 server message types |
| Server core | `src/server/server.ts`, `office/`, `ws/`, `http/`, `auth.ts`, `accounts.ts` | 4,000 | Composition root, per-floor and building-wide broadcast, 26 HTTP routes, HMAC-signed session cookies, scrypt accounts, invites, rate limits |
| Workers and terminals | `src/server/workers/`, `ptyhost.ts`, `ptys.ts`, `screen.ts`, `providers/` | 6,500 | One detached PTY host per floor over a Unix socket, headless xterm mirror sending per-row diffs, worker state machine, 8 provider adapters, hooks, worktrees, PRs, usage |
| Floor services | `github.ts`, `queue.ts`, `meetings.ts`, `changes.ts`, `dog.ts`, `services.ts`, `docs.ts`, `decor.ts`, `whiteboard.ts`, `jukebox.ts`, `cabinet.ts`, `court.ts`, `garage.ts` | 5,500 | Boards via `gh` (90 s refresh, 10 min idle), task queue, meeting patterns, diff watcher, dog AI, port scanner and service relay, bookshelf, pictures, whiteboard, arcade, basketball, cars |
| Building services | `building.ts`, `clone.ts`, `sky.ts`, `theme.ts`, `maps.ts`, `machine.ts`, `usage.ts`, `limits.ts`, `signins.ts`, `webhook.ts`, `upgrade.ts`, `tunnel/` | 5,000 | Floors and clones, weather and sun, holidays, map pick, CPU/memory monitor, spend ledger, Claude plan limits, per-account sign-ins, Slack/Discord webhook, self-upgrade, service tunnel |
| Client world | `src/client/world/`, `features/*/world.ts` | 9,000 | Toon-shaded three.js office, castle, station, tower, city, scenic loop, characters and workers with poses, dog, cars, confetti, sky |
| Client features and UI | `src/client/features/`, `ui/`, `state/`, `core/`, `player/`, `input/`, `sound/` | 15,000 | 38 feature folders, 60 UI modules (terminal with xterm.js, boards, changes, queue, meeting, settings, palette), store slices, frame loop registries, synthesized sound |

State is files. The building's `.agent-office/` holds `config.json` (scrypt verifier, salt, secret), `accounts.json`, `floors.json`, `cloning.json`, `chat.jsonl`, `arcade.json`, `usage.json`, `theme.json`, `map.json`, `maps/*.json`, `prompts.json`, `leave-on-merge.json`, `machine.json`, `webhook.json`, `sky-place.json`, `sky-clock.json`, `hook-port`, and `homes/<account>/` with that person's Claude, gh and git config. Each floor's checkout gets its own `.agent-office/` with `workers.json`, `scrollback/<worker>.ansi`, `queue.json`, `decor.json`, `floorplan.json`, `dog.json`, `jail.json`, `jukebox.json`, `whiteboard/`, `meetings.json`, `worktrees/`, `drops/`, `bin/` (the `office-workers` and `office-queue` commands), the PTY socket and `pty-host.json`, and the generated hook files for each provider.

External processes do the real work: `git` and `gh` for clones, boards, worktrees, PRs and merges; `claude` for plan limits and task-card summaries; `ss` or `lsof` for the services board; `ps` for process attribution; open-meteo over HTTPS for weather. The agent CLIs themselves run inside the PTY host and report back through a loopback hook server with a per-worker bearer token.

The parts that do not port one-to-one are all in the browser: three.js rendering, xterm.js, Excalidraw, WebRTC voice and screen share, the Web Speech dictation, `<iframe>` web-page tabs, and Web Audio synthesis of every sound including a 172 BPM drum-and-bass set on the roof.

## Target architecture

One Go binary, `agent-office`, replaces Node, the Vite bundle, the `bin/*.js` helpers and the PTY host. Which role it plays is a subcommand; the roles talk to each other exactly as the Node pieces do today, over HTTPS/WSS, a loopback hook port and a Unix socket.

&#91;embedded content: target architecture: one binary, four roles\]

Clients reach one server process over HTTPS and WebSocket; the server owns the floors, spawns a detached PTY host per floor over a Unix socket, the agents in those PTYs report back through the loopback hook server, and git, gh and JSON files under `.agent-office/` are the only state.

| Subcommand | Replaces | Runs where |
| --- | --- | --- |
| `agent-office` (no args) | `agent-office` + browser open | Starts `serve` and the native client against it; prints the one-time sign-in link as today |
| `agent-office serve` | `node bin/agent-office.js` | The office machine, under systemd or a container |
| `agent-office client --url https://…` | the browser tab | Any desktop; connects to a remote office with the same cookie flow |
| `agent-office ptyhost <sock> <info>` | `agent-office-ptys` (`ptyhost.js`) | Spawned detached by `serve`, one per floor |
| `agent-office tunnel [office@host]` | `agent-office tunnel` | A teammate's computer |
| `agent-office mcp` | `office-workers mcp` | Inside an agent's session (stdio MCP) |
| `office-workers`, `office-queue` | `bin/office-workers.js`, `bin/office-queue.js` | Hard links or copies of the binary in each floor's `.agent-office/bin/`, dispatched on `argv[0]` |
| \`agent-office accounts | setup | prune |

The Wasm client is produced by `GOOS=js GOARCH=wasm` from the same `client` package and embedded in the server with `embed`, served at `/` with `wasm_exec.js` and a 40-line loader page; `/lite` serves the same Wasm with a `?lite=1` query that starts the client in list mode. Nothing in the deploy scripts (`deploy/*.sh`, the Dockerfile, SSH tunnels, Tailscale Serve, Caddy) changes except the install line, which downloads one binary instead of running `npm install`.

Concurrency replaces the Node event loop with one mutex per floor and one for the building. The rule is the one the Node code already follows implicitly: no I/O under the lock. PTY output, hook requests, `gh` results and timers arrive on goroutines, take the floor lock only to mutate state and compute the broadcast, then release it before writing to sockets. Every WebSocket connection has one writer goroutine fed by a bounded channel; a full channel is the Go form of the current `bufferedAmount` checks (8 MiB slow-client and 4 MiB droppable thresholds).
