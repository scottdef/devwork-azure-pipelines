# Vercel User Guide (November 2026): Frontends, JavaScript and Go Backends, a GitHub IssueOps Platform, Observability, Data, Redis and Cloudflare

Vercel works well for JavaScript frontends and JavaScript backends. Its Go runtime is usable but still in **Beta**. An IssueOps internal developer platform (IDP) can drive everything through the Vercel REST API from GitHub Actions. One security gap remains: deploying from GitHub Actions still needs a static `VERCEL_TOKEN`, because Vercel's OIDC federation covers outbound access only.\[1\]\[2\]

## TL;DR

- **Compute and runtimes:** Fluid compute is the default model. Every plan gets a 300s default timeout. Pro and Enterprise can raise it to 800s (GA), and to 1,800s (beta) on Node.js, Bun and Python. Express, Hono, Fastify and NestJS deploy with zero configuration. Go runs in Beta and reads the version from `go.mod`, so Go 1.24 can be pinned with `go 1.24` plus a `toolchain` line. Upstream Go 1.24 support ended on 10 Feb 2026, so pin a patched toolchain.
- **IssueOps IDP:** Use GitHub issue forms, GitHub Actions and a GitHub App token, plus a small stdlib-only Go CLI that calls documented Vercel endpoints (promote, rollback, env, domains, projects). Gate production with GitHub Environments. Treat the Vercel token as a high-value, narrowly scoped secret.
- **Data, monitoring and Cloudflare:** Databases and Redis now come from the Marketplace. Vercel Postgres moved to Neon, and Vercel KV was moved to Upstash in December 2024. Use `attachDatabasePool` on Fluid compute. Export logs and traces to Dynatrace, Datadog or Grafana with Drains, which need Pro or Enterprise; trace drains accept OTLP/HTTP only. Use Cloudflare as **DNS-only** (grey cloud), not as a proxy in front of Vercel. Cloudflare R2, Turnstile and Workers-fronted D1/KV work well as HTTP services called from Vercel.

---

## Table of Contents

1. Platform fundamentals
2. JavaScript frontend use cases
3. JavaScript backends
4. Go backends
5. Reference IDP: GitHub IssueOps → Vercel
6. Monitoring and observability
7. Database integration
8. Redis
9. Cloudflare compatibility
10. Quick reference / cheat sheet
11. Caveats

---

## 1. Platform fundamentals

### 1.1 Core concepts

- **Team:** the billing, membership and RBAC boundary. A Developer role is now available on Pro.\[3\]
- **Project:** one app linked to one Git repo, or to a monorepo root directory. It holds env vars, domains, build settings and Deployment Protection.
- **Deployment:** an immutable build with a unique URL. Environments are **Production**, **Preview** (each branch/PR) and **Custom environments** such as `staging`.
- **Alias:** a hostname mapped to a deployment. Promote and rollback just move the production aliases; nothing is rebuilt.\[4\]

**Git integration.** The native GitHub integration builds every push. Branches get Preview deployments, and the production branch gets a Production deployment. It also posts commit statuses and PR comments. An alternative is to build in GitHub Actions and upload with `vercel deploy --prebuilt` (§5.5).\[2\]

### 1.2 `vercel.json` or `vercel.ts`

Vercel now supports **`vercel.ts`**, a typed configuration file. It uses the `@vercel/config` package and accepts the same properties as `vercel.json`.\[5\] A project must use only one of the two.\[6\] Support for the legacy `now.json` was removed on 31 March 2026.\[7\]

```ts
// vercel.ts
import { type VercelConfig } from '@vercel/config/v1';

export const config: VercelConfig = {
  framework: 'nextjs',
  regions: ['fra1'],
  functions: { 'app/api/**/route.ts': { maxDuration: 120 } },
  crons: [{ path: '/api/cleanup', schedule: '0 3 * * *' }],
};
```

### 1.3 Environment variables, secrets and OIDC

- Each variable is scoped to Production, Preview, Development or a custom environment. A Preview variable can also be scoped to a single branch.
- Use the **Sensitive** type for secrets. Once saved, a sensitive value can't be read back.\[8\]
- `vercel env pull` writes `.env.local`. It also downloads a development OIDC token that lasts about 12 hours.\[9\]
- **OIDC federation:** Vercel issues per-project, per-environment OIDC tokens to builds and functions. Turn it on under *Settings → Security*. Functions can then reach AWS, GCP or **Azure** without storing static keys.\[1\] For Azure, add a workload identity federation credential that trusts issuer `https://oidc.vercel.com/<team-slug>`.

### 1.4 Domains

- **Apex domain:** create an A record using the value on the project's domain card. Vercel now assigns anycast IPs per project. Many projects show `76.76.21.21`, but newer projects can show an address such as `216.198.79.1`.\[10\]
- **Subdomain:** create a CNAME to the target on the domain card. This is often a project-specific host such as `…vercel-dns-017.com`; older guides cite `cname.vercel-dns.com`.\[10\]\[11\]\[12\]\[13\]
- With a third-party DNS provider, don't add AAAA records. Vercel doesn't support IPv6 for those custom domains, and a stray AAAA record splits traffic and can stall SSL.\[12\]

### 1.5 Compute: Vercel Functions on Fluid compute

Fluid compute has been the default for new projects since **23 April 2025**.\[14\] A single instance serves many invocations concurrently, and billing uses **Active CPU** pricing.\[15\]\[16\]

| Setting | Hobby | Pro | Enterprise |
|---|---|---|---|
| Default max duration | 300s | 300s | 300s |
| Max duration (GA) | 300s | 800s | 800s |
| Extended max (beta; Node.js/Bun/Python) | – | 1,800s | 1,800s |
| Auto-scaling concurrency | 30,000 | 30,000 | 100,000+ |
| Regions | one (default `iad1`) | multiple | multiple |

How the limits work:

- Vercel's changelog "Higher defaults and limits for Vercel Functions running Fluid compute" records that the default rose to 300s from 60s on Hobby and from 90s on Pro and Enterprise. The standard machine also grew from 1 vCPU / 1.7 GB to 1 vCPU / 2 GB.
- The 30-minute extended maximum arrived in beta in June 2026. Secure Compute doesn't support durations above 800s during that beta.\[15\]\[17\]
- For work that runs longer than these limits, use Vercel Workflows (§3.5).\[17\]
- Each function runs in a microVM with a read-only filesystem, apart from up to 500 MB of writable `/tmp`.\[18\]
- If the same setting is configured in several places, code (`export const maxDuration`) wins over `vercel.json`, which wins over the dashboard, which wins over the Fluid defaults.\[17\]

| Runtime | Status (Nov 2026) | Notes |
|---|---|---|
| Node.js | GA, default | Recommended for server code and middleware |
| Bun | **Beta** | Set `"bunVersion": "1.4.x"` in `vercel.json`. Works with Next.js, Express, Hono and Nitro. If `package.json` sets `engines.node` and no Bun version, `engines.node` wins.\[19\]\[20\] |
| Edge | Legacy path | Next.js 16 `proxy.ts` runs on Node only\[21\]\[22\] |
| Go | **Beta** | §4 |

### 1.6 Routing Middleware

Routing Middleware runs **before the cache** and now defaults to the **Node.js** runtime. Bun and Python are also available.\[23\]\[24\]\[25\]\[26\]

- **Non-Next.js projects:** point `"proxy": { "entrypoint": "proxy.ts" }` in `vercel.json` at a file that exports a default function. Helpers such as `next()` come from `@vercel/functions`, and Vercel adds geo headers like `x-vercel-ip-country`.\[25\]\[27\]
- **Next.js 16+:** use `proxy.ts` at the project root. Next.js 16 renamed the `middleware.ts` convention to `proxy.ts`.\[21\]\[25\]\[28\]

### 1.7 Caching, ISR and CDN

- Static assets and ISR pages are served from Vercel's CDN. ISR reads and writes are billed separately.
- Set `Cache-Control: s-maxage=…, stale-while-revalidate=…` on a function response to cache it at the CDN.

### 1.8 Releases: Promote, Instant Rollback, Rolling Releases

- **Promote:** `POST /v10/projects/{projectId}/promote/{deploymentId}` points production at an existing deployment **without rebuilding**.\[4\]
- **Instant Rollback:** `POST /v1/projects/{projectId}/rollback/{deploymentId}` points production back at an earlier production deployment.\[29\]
- **Rolling Releases:** in automation, use `vercel rolling-release start|complete` or `POST /v1/projects/{idOrName}/rolling-release/complete`. **Don't call promote during a rollout.** It completes the rollout to 100%.\[30\]
- **Webhooks:** `deployment.promoted` and `deployment.rollback` let you mirror these events into your audit systems.\[31\]

### 1.9 Deployment Protection

- Options include Vercel Authentication (SSO), Password Protection and Trusted IPs.
- For CI access, use a Protection Bypass for Automation secret, or **Trusted Sources**. Trusted Sources accepts a short-lived GitHub Actions OIDC token in the `x-vercel-trusted-oidc-idp-token` header and matches on claims such as `repository`.\[32\]

### 1.10 Plans and pricing

| Item | Hobby | Pro | Enterprise |
|---|---|---|---|
| Price | $0, personal non-commercial only | $20/month (1 deploying seat + **$20 usage credit**); extra seats $20; viewers free | Custom\[33\] |
| Overage | Hard limits | On-demand; Active CPU from **$0.128/h**\[34\] | Contract |
| Drains | ✗ | ✓ | ✓ (+ Audit Log drains)\[35\]\[36\] |
| Max duration | 300s | 800s (1,800s beta) | same |

Set a spend limit with alerts on Pro. This matters for an IDP that lets anyone create projects.

### 1.11 Notable 2025–2026 changes

| Date | Change |
|---|---|
| Apr 2025 | Fluid compute becomes the default |
| Aug–Oct 2025 | Zero-config Hono, Express, Fastify, NestJS\[37\]\[38\] |
| Oct 2025 | Workflow Development Kit public beta\[39\] |
| 27 Feb 2026 | Vercel Queues public beta\[40\] |
| Feb–Mar 2026 | Activity log covers 100% of team/project changes\[3\] |
| 31 Mar 2026 | `now.json` support removed |
| 16 Apr 2026 | Vercel Workflows GA\[41\] |
| Jun 2026 | 30-minute max duration (beta)\[15\] |
| 2026 | Cron Jobs: 100 per project on all plans; Schedules (beta) named the successor\[42\]\[43\] |

---

## 2. JavaScript frontend use cases

### 2.1 Next.js (App Router)

Vercel is the reference platform for Next.js. Server Components, streaming, ISR, `next/image` and route handlers map directly onto Functions, the CDN and the ISR cache.

```tsx
// app/products/[id]/page.tsx — ISR + tag revalidation
export const revalidate = 300;
export default async function Product({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const p = await (await fetch(`${process.env.API_URL}/products/${id}`, { next: { tags: [`product:${id}`] } })).json();
  return <h1>{p.name}</h1>;
}
```

A CMS webhook can then call a route handler that runs `revalidateTag('product:42')`. Set `images.remotePatterns` narrowly so the image optimizer can't be used as an open proxy.

### 2.2 Other frameworks

Vercel auto-detects Vite/React SPAs, SvelteKit (`adapter-vercel`), Nuxt/Nitro, Astro (`@astrojs/vercel`) and React Router v7/Remix. Their server-side rendering also runs on Fluid compute. For SPAs, add a rewrite to `/index.html`.

### 2.3 Previews and collaboration

- Every PR gets a Preview URL.
- The Vercel Toolbar adds Comments, which sync to PR threads, plus feature-flag overrides.
- Protect previews with Vercel Authentication. Give E2E tests access through a bypass secret or Trusted Sources.

### 2.4 Monorepos

- Create one Vercel project per app and set each project's *Root Directory* (for example `apps/web`).
- Set `"ignoreCommand": "npx turbo-ignore"` so commits that don't touch an app skip its build. Use Turborepo remote caching with pnpm workspaces.

### 2.5 Feature flags, AI SDK and AI Gateway

- **Flags:** the Flags SDK (`flags/next`) reads flag values from **Edge Config**. Edge Config suits read-heavy, rarely changing data such as flags, redirects and allow-lists.
- **AI:** the AI SDK's `streamText({ model: 'provider/model', … })` routes through the Vercel AI Gateway. The Gateway emits an OpenTelemetry trace for every request, which you can export with AI Gateway Trace Drains.\[44\]

---

## 3. JavaScript backends

### 3.1 Functions and zero-configuration servers

Vercel detects **Express, Hono, Fastify and NestJS** and deploys them as Functions on Fluid compute with Active CPU pricing. You don't need `/api` folders or `vercel.json` rewrites.\[45\]\[46\]\[47\]\[48\]

```ts
// app.ts — Express, zero-config
import express from 'express';
const app = express();
app.get('/health', (_req, res) => res.json({ ok: true }));
export default app;
```

A plain function can be as small as `export default { fetch: (req: Request) => Response.json({ ok: true }) }`.

When zero-config Hono launched, community reports described broken builds with some relative-import layouts.\[49\] Run `vercel build` locally before migrating.

### 3.2 Streaming and `waitUntil`

- **Streaming:** return a `ReadableStream`, or use the AI SDK's stream helpers. The instance stays alive while it streams, up to `maxDuration`.
- **`waitUntil`:** runs work after the response is sent.\[50\]

```ts
import { waitUntil } from '@vercel/functions';
export async function POST(req: Request) {
  const order = await req.json();
  waitUntil(fetch(process.env.AUDIT_URL!, { method: 'POST', body: JSON.stringify(order) }));
  return Response.json({ accepted: true }, { status: 202 });
}
```

### 3.3 Cron Jobs (and Schedules)

| Plan | Cron jobs / project | Min interval | Precision |
|---|---|---|---|
| Hobby | 100 | once per day | per-hour (±59 min)\[51\] |
| Pro / Enterprise | 100 | once per minute | per-minute\[51\] |

How cron jobs behave:

- **Old limits are gone.** Vercel removed the per-team limits (2/40/100) and raised the per-project limit from 20 to 100 on every plan.\[43\] Many blogs still quote the old numbers.
- **Production only, no retries.** Cron jobs run only on production deployments, and Vercel doesn't retry a failed invocation.\[8\]\[50\] Make handlers idempotent.
- **`CRON_SECRET`.** Vercel sends it as `Authorization: Bearer <secret>`. Each deployment sends its own copy, so a changed secret takes effect only after the next deploy.\[8\]\[50\]
- **Legacy status.** Vercel calls Cron Jobs "a legacy feature" that will move to **Schedules**. Schedules is in public beta and can run in preview and custom environments.\[42\]\[51\]

```ts
export async function GET(req: Request) {
  if (req.headers.get('authorization') !== `Bearer ${process.env.CRON_SECRET}`) return new Response('unauthorized', { status: 401 });
  return Response.json({ ok: true });
}
```

### 3.4 Limits to design around

- Duration limits are in §1.5.
- Per Vercel's "Configuring Memory and CPU" docs, Pro and Enterprise functions on Fluid compute default to 2 GB of memory and 1 vCPU, upgradable to 4 GB and 2 vCPUs. Memory can't be set in `vercel.json`, so set it in the dashboard.
- Vercel's Functions Limits docs cap the request or response body at 4.5 MB. Larger payloads return `413: FUNCTION_PAYLOAD_TOO_LARGE`, so use Blob or R2 presigned uploads for large files.
- WebSocket connections close when an instance is recycled, so clients must reconnect.\[52\]

### 3.5 Queues and Workflows

| Need | Use | Status |
|---|---|---|
| Work after the response | `waitUntil` | GA |
| Durable delivery, fan-out, retries, delays, idempotency keys | **Vercel Queues** (`@vercel/queue`, Node 22+) | Public beta since 27 Feb 2026; billed per operation\[40\]\[53\] |
| Multi-step, resumable logic (minutes to months) | **Vercel Workflows** (`"use workflow"` / `"use step"`) | GA since 16 Apr 2026; Python SDK in beta\[41\] |

---

## 4. Go backends

### 4.1 Two modes (both Beta)

| Mode | Layout | When to use |
|---|---|---|
| **Go server** (framework preset `go`) | Root `go.mod` plus `main.go`, `cmd/api/main.go` or `cmd/server/main.go`. Must listen on `$PORT`.\[54\] | One `net/http` server; the idiomatic choice |
| **`/api` functions** | Each `api/*.go` file exports one `http.HandlerFunc`, and each becomes a separate Function\[54\] | Small endpoints next to a JS frontend |

To run a Go server alongside Next.js in one project, use **Services**, which are themselves in beta.\[41\]\[54\]

### 4.2 Is Go 1.24 supported, and how do you pin it?

Vercel's documented rules:

- Vercel reads the **`go` directive** in `go.mod`.\[54\]
- A **`toolchain` directive** takes precedence over the `go` directive.\[54\]
- `go.mod` must be at the project root.\[54\]
- If no version is declared, Vercel uses the latest supported version.\[54\]
- Vercel downloads and caches each toolchain the first time it is used.\[54\]

Vercel publishes **no explicit list of supported versions**. Because toolchains are fetched on demand, Go 1.24 should be honored, but verify it with a test deployment.

Upstream support for Go 1.24 **ended on 10 Feb 2026**; its last patch is 1.24.13.\[55\] Keep the 1.24 language level and build with a patched toolchain:

```go
// go.mod
module example.com/svc

go 1.24

toolchain go1.26.3 // illustrative; Vercel builds with the toolchain directive when present
```

### 4.3 Build options and pitfalls

- `GO_BUILD_FLAGS` sets build flags; the default is `-ldflags '-s -w'`.\[54\]
- `buildCommand` replaces `go build` and runs with `GOOS=linux`. Vercel looks for the binary in the project root or `bin/`, or at `$VERCEL_OUTPUT_FILE`.\[54\]
- A `vendor/modules.txt` file makes Vercel pass `-mod=vendor` automatically.\[54\]
- For private modules, set `GIT_CREDENTIALS=https://user:token@github.com`. \[54\]
- **Pitfall:** in `/api` mode the builder rewrites the module name. Imports of `yourmod/internal/...` can then fail Go's `internal` visibility rule.\[56\] Put shared code in `pkg/`.

### 4.4 Example (Go 1.24, standard library only)

```go
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/greet/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hello": r.PathValue("name")})
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	slog.Info("listening", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
```

In `/api` mode, write the same logic as `package handler` with `func Handler(w http.ResponseWriter, r *http.Request)`.

### 4.5 Local development, performance and limits

- **Local development:** `vercel dev` runs `/api` Go functions. For server mode, `PORT=3000 go run .` is simpler. Run `vercel build` before pushing.
- **Cold starts:** stripped static binaries start fast, and Fluid's instance reuse amortizes cold starts further.
- **Duration:** the 1,800s beta is documented only for Node.js, Bun and Python.\[57\]\[58\] **Plan for an 800s maximum on Go.**
- **Hand-off to Node.js:** Queues, Workflows, `waitUntil` and `attachDatabasePool` are Node.js-first. Delegate durable work from Go over HTTP.
- **Fallback:** for critical Go services, keep a Docker-on-AKS fallback. Vercel also publishes a Docker-based Go guide.

---

## 5. Reference IDP: GitHub IssueOps → Vercel

### 5.1 Architecture

```
Developer ─ issue form ─► repo "platform-requests"
                           │ issues: opened/labeled · issue_comment: created
                           ▼
                GitHub Actions
  1 route by label (idp:create-app|promote|rollback|env|domain)
  2 mint GitHub App installation token (1h)
  3 authZ: team membership; "/approve" by a 2nd person
  4 prod-affecting jobs bound to Environment "vercel-prod" (required reviewers)
  5 idpctl (Go 1.24 stdlib): parse → validate → Vercel REST (+ GitHub REST)
  6 comment result, label, close
        │ Bearer VERCEL_TOKEN          │ App token
        ▼                              ▼
  api.vercel.com                 api.github.com (repo from template)
        │ webhooks + Drains → SIEM / Dynatrace
```

**Design choices:**

- **Let Vercel's native Git integration do the builds.** The IDP then manages configuration and traffic: projects, env vars, domains, promote and rollback. You keep Preview URLs, PR comments and build caching.
- **Use `--prebuilt` from Actions only where it pays off,** such as signing or SBOM generation, or builds that need private network access. Vercel says plainly that it "doesn't currently support OIDC for authenticating the GitHub Actions deployment itself."\[2\]

### 5.2 Issue form (example: promote)

```yaml
# .github/ISSUE_TEMPLATE/promote.yml
name: "Promote to production"
description: "Promote an existing READY deployment (no rebuild)"
title: "[promote] <project>"
labels: ["idp:promote"]
body:
  - type: input
    id: project
    attributes: { label: Project, description: "lowercase a-z0-9-" }
    validations: { required: true }
  - type: input
    id: deployment
    attributes: { label: Deployment, description: "dpl_… or *.vercel.app host" }
    validations: { required: true }
  - type: textarea
    id: reason
    attributes: { label: Reason }
    validations: { required: true }
```

Build `create-app.yml`, `env-var.yml` (key, value, target dropdown), `domain.yml` and `rollback.yml` the same way. **Never accept secret values in issues.** Route secret requests through a vault-backed flow instead, for example Azure Key Vault → Actions → Vercel env API.

### 5.3 Workflow

```yaml
# .github/workflows/idp.yml
name: idp
on:
  issues: { types: [opened, labeled] }
permissions: { contents: read, issues: write }
concurrency: { group: "idp-${{ github.event.issue.number }}", cancel-in-progress: false }

jobs:
  route:
    runs-on: ubuntu-latest
    outputs: { kind: "${{ steps.k.outputs.kind }}", prod: "${{ steps.k.outputs.prod }}" }
    steps:
      - id: k
        env: { LABELS: "${{ join(github.event.issue.labels.*.name, ' ') }}" }
        run: |
          for l in $LABELS; do case "$l" in
            idp:promote|idp:rollback|idp:domain) echo "kind=${l#idp:}" >>"$GITHUB_OUTPUT"; echo prod=true >>"$GITHUB_OUTPUT";;
            idp:env|idp:create-app) echo "kind=${l#idp:}" >>"$GITHUB_OUTPUT"; echo prod=false >>"$GITHUB_OUTPUT";;
          esac; done

  execute:
    needs: route
    if: needs.route.outputs.kind != ''
    runs-on: ubuntu-latest
    environment: ${{ needs.route.outputs.prod == 'true' && 'vercel-prod' || 'vercel-nonprod' }}
    steps:
      - uses: actions/create-github-app-token@v2        # pin to SHA
        id: app
        with: { app-id: "${{ vars.IDP_APP_ID }}", private-key: "${{ secrets.IDP_APP_PRIVATE_KEY }}", owner: "${{ github.repository_owner }}" }
      - name: Requester must be in platform-users
        env: { GH_TOKEN: "${{ steps.app.outputs.token }}", ACTOR: "${{ github.event.issue.user.login }}", ORG: "${{ github.repository_owner }}" }
        run: |
          s=$(gh api "orgs/$ORG/teams/platform-users/memberships/$ACTOR" --jq .state 2>/dev/null || echo none)
          test "$s" = active || { echo "::error::not authorized"; exit 1; }
      - uses: actions/checkout@v5                        # pin to SHA
        with: { repository: my-org/idp-tools }
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Run idpctl
        env:   # untrusted input only via env, never interpolated into the script
          ISSUE_BODY: ${{ github.event.issue.body }}
          KIND: ${{ needs.route.outputs.kind }}
          VERCEL_TOKEN: ${{ secrets.VERCEL_TOKEN }}
          VERCEL_TEAM_ID: ${{ vars.VERCEL_TEAM_ID }}
        run: go run ./cmd/idpctl -kind "$KIND" -out result.md
      - name: Report and close
        if: always()
        env: { GH_TOKEN: "${{ github.token }}", N: "${{ github.event.issue.number }}", ST: "${{ job.status }}" }
        run: |
          test -f result.md || echo "Failed before execution." > result.md
          gh issue comment "$N" -R "$GITHUB_REPOSITORY" --body-file result.md
          [ "$ST" = success ] && gh issue close "$N" -R "$GITHUB_REPOSITORY" --reason completed || true
```

**Approvals.** There are two options:

1. **Recommended: GitHub Environment required reviewers** on `vercel-prod`, with "prevent self-review" enabled. GitHub records the approval.
2. **An `issue_comment` job** that accepts `/approve` only from members of `platform-approvers` who are not the issue author. That job adds an `approved` label using the **App token**, because events created with the default `GITHUB_TOKEN` don't trigger new workflow runs.

**Create an app repo** with `gh api -X POST repos/my-org/template-nextjs/generate -f owner=my-org -f name="$NAME" -F private=true` using the App token. Then call `POST /v11/projects` with `gitRepository: {type:"github", repo:"my-org/NAME"}`.

The GitHub App needs these permissions:

- `administration:write` (to create repos)
- `contents:read`
- `members:read` (for team checks)
- `issues:write`

### 5.4 The Vercel endpoints and `idpctl` (Go 1.24, standard library only)

| Action | Endpoint |
|---|---|
| Create project + link Git | `POST /v11/projects` |
| Env var | `POST /v10/projects/{idOrName}/env?upsert=true` |
| Add domain | `POST /v10/projects/{idOrName}/domains` (DNS records still required) |
| List / get deployments | `GET /v6/deployments?projectId=…&target=production&state=READY`, `GET /v13/deployments/{idOrUrl}`\[59\]\[60\] |
| Promote | `POST /v10/projects/{id}/promote/{deploymentId}` (body `{}`)\[59\] |
| Promote status | `GET /v1/projects/{id}/promote/aliases`\[61\] |
| Rollback | `POST /v1/projects/{id}/rollback/{deploymentId}?description=…` (body `{}`)\[29\]\[59\] |
| What prod serves | `GET /v4/aliases/{domain}` → `deploymentId`\[59\] |

Append `?teamId=team_…` to every call. `@vercel/sdk` wraps the same endpoints.

```go
// cmd/idpctl/main.go — IssueOps executor. Standard library only.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	reProject = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)
	reDeploy  = regexp.MustCompile(`^(dpl_[A-Za-z0-9]+|[a-z0-9-]+\.vercel\.app)$`)
	reKey     = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	targets   = map[string]bool{"preview": true, "production": true, "preview,production": true}
)

type vercel struct {
	token, team string
	hc          *http.Client
}

func (v *vercel) do(ctx context.Context, method, path string, in, out any) error {
	u, err := url.Parse("https://api.vercel.com" + path)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("teamId", v.team)
	u.RawQuery = q.Encode()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, u.Path, resp.Status, bytes.TrimSpace(b))
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

// parseForm reads issue-form markdown: "### Label\n\nvalue".
func parseForm(body string) map[string]string {
	m := map[string]string{}
	var key string
	var val []string
	flush := func() {
		if key != "" {
			v := strings.TrimSpace(strings.Join(val, "\n"))
			if v == "_No response_" {
				v = ""
			}
			m[strings.ToLower(key)] = v
		}
	}
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		if l := sc.Text(); strings.HasPrefix(l, "### ") {
			flush()
			key, val = strings.TrimSpace(l[4:]), nil
		} else {
			val = append(val, l)
		}
	}
	flush()
	return m
}

func need(f map[string]string, k string, re *regexp.Regexp) (string, error) {
	if v := f[k]; re.MatchString(v) {
		return v, nil
	}
	return "", fmt.Errorf("field %q is invalid", k)
}

func run(ctx context.Context, v *vercel, kind string, f map[string]string) (string, error) {
	project, err := need(f, "project", reProject)
	if err != nil {
		return "", err
	}
	switch kind {
	case "promote":
		d, err := need(f, "deployment", reDeploy)
		if err != nil {
			return "", err
		}
		var dep struct{ ID, ReadyState string }
		if err := v.do(ctx, "GET", "/v13/deployments/"+d, nil, &dep); err != nil {
			return "", err
		}
		if dep.ReadyState != "READY" {
			return "", fmt.Errorf("deployment %s is %s, not READY", dep.ID, dep.ReadyState)
		}
		if err := v.do(ctx, "POST", "/v10/projects/"+project+"/promote/"+dep.ID, struct{}{}, nil); err != nil {
			return "", err
		}
		return fmt.Sprintf("Promoted `%s` for `%s`.", dep.ID, project), nil
	case "rollback":
		var list struct {
			Deployments []struct{ UID string } `json:"deployments"`
		}
		p := "/v6/deployments?target=production&state=READY&limit=2&projectId=" + url.QueryEscape(project)
		if err := v.do(ctx, "GET", p, nil, &list); err != nil {
			return "", err
		}
		if len(list.Deployments) < 2 {
			return "", errors.New("no previous production deployment")
		}
		prev := list.Deployments[1].UID
		q := "?description=" + url.QueryEscape(f["reason"])
		if err := v.do(ctx, "POST", "/v1/projects/"+project+"/rollback/"+prev+q, struct{}{}, nil); err != nil {
			return "", err
		}
		return fmt.Sprintf("Rolled back `%s` to `%s`.", project, prev), nil
	case "env":
		key, err := need(f, "key", reKey)
		if err != nil {
			return "", err
		}
		t := f["target"]
		if !targets[t] {
			return "", errors.New("invalid target")
		}
		in := map[string]any{"key": key, "value": f["value"], "type": "encrypted", "target": strings.Split(t, ",")}
		if err := v.do(ctx, "POST", "/v10/projects/"+project+"/env?upsert=true", in, nil); err != nil {
			return "", err
		}
		return fmt.Sprintf("Set `%s` on `%s` (%s). Redeploy to apply.", key, project, t), nil
	}
	return "", fmt.Errorf("unknown kind %q", kind)
}

func main() {
	kind := flag.String("kind", "", "promote|rollback|env")
	out := flag.String("out", "result.md", "result file")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	v := &vercel{os.Getenv("VERCEL_TOKEN"), os.Getenv("VERCEL_TEAM_ID"), &http.Client{Timeout: 30 * time.Second}}
	if v.token == "" || v.team == "" {
		log.Error("missing VERCEL_TOKEN or VERCEL_TEAM_ID")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	msg, err := run(ctx, v, *kind, parseForm(os.Getenv("ISSUE_BODY")))
	if err != nil {
		log.Error("request failed", "kind", *kind, "err", err)
		msg = "❌ " + err.Error()
	} else {
		log.Info("request done", "kind", *kind)
		msg = "✅ " + msg
	}
	os.WriteFile(*out, []byte(msg+"\n"), 0o644)
	if err != nil {
		os.Exit(1)
	}
}
```

The CLI is injection-safe by construction:

- The issue body arrives only through an environment variable.
- Every field must match an allow-list regex before it reaches a URL or JSON body.
- Results go to a file, which the workflow posts with `--body-file`.

The domain and create-app cases follow the same pattern.

### 5.5 The `--prebuilt` path, and a CLI pitfall

```bash
vercel pull --yes --environment=production --token="$VERCEL_TOKEN"
vercel build --prod --token="$VERCEL_TOKEN"
vercel deploy --prebuilt --prod --token="$VERCEL_TOKEN"
```

A public bug report found that **CLI 63.1.2's `promote` and `rollback`** looked up `/v2/user` first, which failed with a team-scoped token. `vercel deploy` worked. The fix was to call the REST endpoints directly,\[59\] which supports driving promote and rollback from `idpctl`.

### 5.6 Credentials and least privilege

| Credential | Use | Hardening |
|---|---|---|
| GitHub App installation token (1h) | GitHub API | Minimal permissions; install only on the needed repos |
| `VERCEL_TOKEN` | Vercel REST/CLI | Scope to one team, set an expiry, store as an **Environment** secret, rotate it, use a dedicated machine account |
| GitHub OIDC → Trusted Sources | E2E tests against protected previews | No static bypass secret\[2\] |
| GitHub OIDC → Vercel Container Registry | `vercel/vcr-action/login` | Short-lived token, revoked after the job\[62\] |
| Vercel OIDC → Azure/AWS/GCP | Functions calling cloud APIs | No cloud keys stored\[1\] |

The New Stack's article "Vercel moved to kill static credentials. Then developers spotted its own deployment token." describes the gap. Vercel is requiring Marketplace vendors to move to OIDC by January 2027. Its CTO, Malte Ubl, cited a survey of more than 1,000 SaaS vendors in which only 8.4% supported secure authentication. Yet Fanatics engineer Yordis Prieto said he still relies on "a long-lived access token" in his CI pipeline. Treat that token as the most sensitive string in the pipeline.

**Security checklist:**

- Never put `${{ github.event.issue.* }}` inside `run:` scripts.
- Pin actions to commit SHAs.
- Keep `permissions:` minimal.
- Use `concurrency` to serialize requests.
- Require approvers who aren't the issue author.
- Ship the Vercel Activity log and webhooks to your SIEM. On Enterprise, use **Audit Log Drains** (S3, Splunk, Datadog, Panther).\[35\]

---

## 6. Monitoring and observability

### 6.1 Built-in features

| Feature | What | Plan / retention |
|---|---|---|
| Observability | Function, route and external-API metrics and errors | All plans. mindcat.ai (16 Oct 2026) reports data kept for 12 hours on Hobby (runtime logs 1 hour), 1 day on Pro and 3 days on Enterprise. |
| Observability Plus | 30-day retention, deeper querying and alerts | Pro/Enterprise add-on. Vercel's docs list usage-based pricing of $1.20 per 1 million events with no base fee, enabled by default for teams created or upgraded to Pro on or after 3 April 2026. |
| Runtime Logs | Searchable function logs | 1h / 1 day / 3 days; **30 days with Plus** (14-day query window). Limits: 256 KB per line, 1 MB and 256 lines per request.\[63\]\[64\] |
| Tracing | Automatic infrastructure spans plus `@vercel/otel` custom spans | Export with Trace Drains\[65\] |
| Web Analytics / Speed Insights | Page views and events; real-user Core Web Vitals | Drain export of Speed Insights needs Speed Insights Plus\[66\] |
| Firewall / WAF / BotID | Dashboards, custom rules, rate limits | Rules propagate in under 300ms\[67\] |
| Usage / spend management | Meters, alerts, auto-pause | Pro |

### 6.2 Drains

Drains (formerly "Log Drains") export **logs, traces, Web Analytics and Speed Insights** from Pro and Enterprise teams.\[36\]\[68\]

- **Formats:** JSON, NDJSON or Protobuf.\[68\]
- **Controls:** sampling, custom headers and signature verification.\[68\]
- **Trace drains:** **OTLP/HTTP only**, typically port 4318 at `/v1/traces`. gRPC is not supported.\[69\]
- **Correlation:** logs from traced requests carry `traceId` and `spanId`.\[68\]

```ts
// instrumentation.ts
import { registerOTel } from '@vercel/otel';
export function register() { registerOTel({ serviceName: 'storefront' }); } // = project name (Datadog requirement)
```

### 6.3 Dynatrace, Datadog, Grafana and Elastic

| Vendor | Path |
|---|---|
| Datadog | Native integration: log drain (sampling 1–100%) plus trace drain for APM\[70\] |
| Elastic | Drain to an Elastic Cloud managed endpoint, one drain per data type\[71\] |
| Grafana / Honeycomb / New Relic | Trace drain to their OTLP/HTTP endpoints\[68\] |
| **Dynatrace** | **Custom drains.** No native Vercel integration was confirmed. |

**Recommended Dynatrace design** (validate in a pilot):

1. **Traces:** send a Trace Drain to Dynatrace OTLP/HTTP ingest (`https://<env>.live.dynatrace.com/api/v2/otlp/v1/traces`) with the header `Authorization: Api-Token <openTelemetryTrace.ingest>`.
2. **Logs:** send a Log Drain to an **OpenTelemetry Collector on AKS**, with signature verification. The Collector redacts and samples, then exports OTLP to Dynatrace.
3. **Correlation:** propagate W3C `traceparent` into AKS services so Dynatrace can stitch Vercel → AKS traces end to end.

For SLO alerting, alert in Dynatrace or Datadog on the drained data. Vercel and AKS signals then share one alerting plane, and Vercel's built-in alerts act as a secondary signal.

---

## 7. Database integration

### 7.1 What exists now

| Need | Options |
|---|---|
| Postgres | **Neon** (successor to Vercel Postgres),\[72\] Supabase, Prisma Postgres, AWS options |
| MySQL / document / SQLite | PlanetScale / MongoDB Atlas / Turso |
| Objects / config | **Vercel Blob** or R2 / **Edge Config** |
| Redis | Upstash, Redis Cloud (§8) |

Marketplace integrations provision the database, inject credentials as env vars, and bill through Vercel.\[73\]\[74\]

### 7.2 Pooling on Fluid compute

```ts
import { Pool } from 'pg';
import { drizzle } from 'drizzle-orm/node-postgres';
import { attachDatabasePool } from '@vercel/functions';

const pool = new Pool({ connectionString: process.env.DATABASE_URL, max: 10, idleTimeoutMillis: 5000 });
attachDatabasePool(pool); // release idle clients before suspension
export const db = drizzle(pool);
```

- `attachDatabasePool` supports `pg`, `mysql2`, MariaDB, MongoDB, `ioredis` and Cassandra pools.\[75\]
- Vercel advises **against a pool max of 1**, because one Fluid instance serves several invocations at once.\[14\]\[76\]
- **Neon** now recommends TCP with pooling on Fluid compute rather than its HTTP driver.\[77\]
- **Prisma:** use `@prisma/adapter-pg` together with `attachDatabasePool`.\[78\]
- **Pitfall:** a Supabase thread reported Supavisor connections still growing on Fluid compute.\[79\] Monitor connection counts, and size the pooler for instances × pool max.

**Preview branches:** the Neon and Supabase integrations can create a database branch per Preview deployment. Run migrations against the **direct** (non-pooled) URL.

### 7.3 Go under a stdlib-only rule

`database/sql` is stdlib, but **every driver (`pgx`, `lib/pq`, `go-sql-driver/mysql`) is an external module** and needs approval under your rule. Stdlib-only alternatives are HTTP data APIs:

- Supabase PostgREST (`/rest/v1/{table}` with `apikey` and Bearer headers)
- Turso HTTP
- Cloudflare D1 REST, or a Worker proxy
- Upstash REST (§8.3)

### 7.4 Secure networking

- **Secure Compute** (Enterprise) provides private networking and peering.
- **Static IPs** give you egress addresses to allow-list on Azure database firewalls.
- Otherwise, use Vercel OIDC → Azure Key Vault for short-lived credentials.

---

## 8. Redis

### 8.1 Options

- **Vercel KV is gone.** Stores were moved to **Upstash Redis in December 2024**, and `@vercel/kv` is deprecated.\[74\]\[80\]
- **Upstash:** serverless and priced per request. It offers REST plus TLS TCP, and global read replicas.
- **Redis Cloud:** classic TCP Redis with provisioned capacity.

### 8.2 Connections and use cases

| Approach | Client | Best for |
|---|---|---|
| REST/HTTP | `@upstash/redis`, `fetch`, Go `net/http` | Any runtime; no socket management |
| TCP | `ioredis`/`node-redis` (+ `attachDatabasePool`) | High command rates, Lua, transactions |

**Use cases:**

- Caching
- Rate limiting with `@upstash/ratelimit`, e.g. `Ratelimit.slidingWindow(20, '10 s')` keyed by IP
- Sessions
- Idempotency keys
- Lightweight queues (QStash, or Vercel Queues)

Functions can **publish** but shouldn't hold **subscriptions**. Consume pub/sub in a long-running AKS service.

### 8.3 Go with `net/http` (Upstash REST)

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

var hc = &http.Client{Timeout: 3 * time.Second}

func cmd(r *http.Request, args ...any) (any, error) {
	b, _ := json.Marshal(args)
	req, err := http.NewRequestWithContext(r.Context(), "POST", os.Getenv("UPSTASH_REDIS_REST_URL"), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("UPSTASH_REDIS_REST_TOKEN"))
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct{ Result any }
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out.Result, err
}

func Handler(w http.ResponseWriter, r *http.Request) {
	n, err := cmd(r, "INCR", "hits")
	if err != nil {
		http.Error(w, "redis error", http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"hits": n})
}
```

Batch commands with `/pipeline`.\[81\]\[82\] Check which env var names were injected: legacy KV stores use `KV_REST_API_*`, newer ones `UPSTASH_REDIS_REST_*`.\[83\]

### 8.4 Cost and placement

**Co-locate the Redis primary with your function region**, for example `fra1` with eu-central-1. Add read replicas only for global read-heavy traffic. With per-request pricing, a chatty cache can cost more than the database it protects.

---

## 9. Cloudflare compatibility

### 9.1 Cloudflare DNS (the recommended setup)

Vercel states it does **not** recommend a reverse proxy in front of Vercel.\[84\] A proxy:

- hides client IPs and TLS fingerprints from the Firewall and BotID\[67\]
- adds latency
- breaks cache purges
- shows a "Proxy Detected" banner on the project\[85\]

| Record | Cloudflare setting |
|---|---|
| Apex `A` → value on the domain card | **DNS only (grey cloud)**\[86\] |
| Subdomain `CNAME` → target on the domain card | **DNS only**; CNAME flattening works for the apex\[87\] |
| CAA | Must allow Vercel's CA |

**If policy forces the orange cloud (proxying):**

- Set SSL/TLS to **Full (strict)**. Flexible mode causes `ERR_TOO_MANY_REDIRECTS`, and Vercel's HTTPS redirect can't be disabled.\[88\]\[89\]
- Never cache `/.well-known/vercel/*`, and keep WAF and page rules off `/.well-known/acme-challenge/*`.\[86\]\[88\] Otherwise certificates fail; community threads report 526 errors.\[90\]
- Enable **Verified Proxy Lite** so Vercel trusts `CF-Connecting-IP`.\[85\]\[88\]
- On Enterprise, you can upload a Cloudflare Origin CA wildcard certificate.\[84\]

### 9.2 Double CDN and double WAF

Each cache miss travels Cloudflare → Vercel → Cloudflare, and you pay both vendors. Vercel's comparison says Vercel WAF rules propagate in under 300ms and can reference framework routes, while Cloudflare offers paranoia-level OWASP tuning.\[67\] **Position:** use the Vercel WAF for Vercel apps, and the Cloudflare WAF for non-Vercel origins.

### 9.3 Cloudflare services from Vercel Functions

| Product | Fit | How |
|---|---|---|
| **R2** | ✅ | S3 API at `https://<ACCOUNT_ID>.r2.cloudflarestorage.com`, region `auto`, **no egress fees**. Presigned GET/HEAD/PUT/DELETE valid 1s–7 days; no POST forms; not on custom domains. EU: `<ACCOUNT_ID>.eu.r2…`.\[91\]\[92\]\[93\]\[94\]\[95\] |
| **Turnstile** | ✅ | Verify server-side via `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` |
| **Workers** | ✅ | HTTPS with a shared secret or Access service token |
| **D1 / KV** | ⚠️ | REST for admin use; a Worker HTTP facade for app traffic |
| **Durable Objects / Hyperdrive** | ⚠️ | Bindings only, so front them with a Worker |
| **Images / Stream** | ✅ | HTTP APIs. Choose one image optimizer, not both. |
| **Access / Tunnel** | ⚠️ / ✅ | Access in front of Vercel is the discouraged proxy setup; use Vercel SSO instead. Tunnel + Access service tokens let functions reach private AKS APIs. |
| **AI Gateway** | ✅ | Use either Cloudflare's or Vercel's, not both in series |

From JavaScript, use the AWS SDK v3 S3 client with `region: 'auto'` and the R2 endpoint.\[96\]\[97\] From Go under the stdlib rule, sign requests with SigV4 using `crypto/hmac` and `crypto/sha256`, with the credential scope `<date>/auto/s3/aws4_request`. Issue presigned PUT URLs so large uploads bypass Vercel's 4.5 MB function body limit, which its Functions Limits docs enforce with a `413: FUNCTION_PAYLOAD_TOO_LARGE` error.

### 9.4 Vercel vs Cloudflare Workers/Pages

| Dimension | Vercel | Cloudflare Workers |
|---|---|---|
| Compute | Fluid microVMs: Node/Bun/Python/Go, regional, 300–800s (1,800s beta) | V8 isolates at every PoP, CPU-time limits |
| Next.js | First-party | OpenNext adapter; some features lag |
| Go | Native runtime (Beta) | WASM/TinyGo or containers |
| Data | Marketplace, Blob, Edge Config | D1, KV, R2, Durable Objects, Hyperdrive (bindings) |
| Security | Vercel WAF, BotID, Deployment Protection | Cloudflare WAF, Bot Management, Zero Trust |
| Pricing | Seat + credit + meters | Request/CPU-based; R2 has no egress fees |

### 9.5 Hybrid patterns that work

1. Vercel app + Cloudflare DNS (grey cloud) + R2 + Turnstile.
2. Vercel frontend + a Cloudflare Worker "data edge" in front of D1 or Durable Objects.
3. Vercel frontend + AKS APIs through Tunnel and Access service tokens, traced end to end in Dynatrace.

---

## 10. Quick reference / cheat sheet

```bash
vercel link && vercel env pull .env.local
vercel dev
vercel build --prod && vercel deploy --prebuilt --prod
vercel deploy --target=staging
vercel promote <url-or-id>
vercel rollback [url-or-id]
vercel rolling-release start|complete
vercel logs <url>
vercel domains add example.com
```

| Purpose | Endpoint (append `?teamId=`) |
|---|---|
| Create project | `POST /v11/projects` |
| Env var | `POST /v10/projects/{id}/env?upsert=true` |
| Domain | `POST /v10/projects/{id}/domains` |
| Deployments | `GET /v6/deployments`, `GET /v13/deployments/{idOrUrl}` |
| Promote / Rollback | `POST /v10/projects/{id}/promote/{dpl}` / `POST /v1/projects/{id}/rollback/{dpl}` |
| Alias | `GET /v4/aliases/{idOrAlias}` |
| Drains | `/v1/drains` |

| Limit | Value |
|---|---|
| Duration | Default 300s. Max 300s (Hobby) / 800s (Pro, Ent). 1,800s beta. |
| `/tmp` / body | 500 MB / 4.5 MB (413 `FUNCTION_PAYLOAD_TOO_LARGE`, per Vercel Functions Limits docs) |
| Concurrency | 30,000 (Hobby/Pro) / 100,000+ (Ent) |
| Cron | 100/project; Hobby daily ±59 min |
| Runtime logs | 1h / 1d / 3d; 30d with Plus |
| Pro | $20/month incl. 1 seat + $20 credit; Active CPU from $0.128/h |

---

## 11. Caveats

- **Beta features:** the Go runtime, Bun runtime, Services, Queues, Schedules and the 1,800s duration can all change. Re-check the docs before relying on exact APIs.
- **Figures now confirmed in Vercel's own docs:**
  - Observability Plus costs $1.20 per 1 million events.
  - Drains cost $0.50 per GB on Pro, measured as the uncompressed JSON serialization of each drained record.
  - Memory defaults to 2 GB / 1 vCPU, upgradable to 4 GB / 2 vCPUs.
  - The function body limit is 4.5 MB.
  - Re-check vercel.com/pricing before budgeting, because rates change.
- **Go 1.24** has reached end of upstream support. Build with a current toolchain.
- **Dynatrace:** no first-party integration was confirmed. The OTLP/Collector design in §6.3 is a recommendation to validate in a pilot.
- **REST API versions** change over time. Prefer `@vercel/sdk`, or clients generated from the OpenAPI spec.

## Sources

1. [Replace static CI keys with OIDC federation on Vercel - Vercel](https://vercel.com/i/oidc-federation-ci-tokens)
2. [How can I use GitHub Actions with Vercel?](https://vercel.com/kb/guide/how-can-i-use-github-actions-with-vercel)
3. [News Cache (2026-03-02) - Announcements - Vercel Community](https://community.vercel.com/t/news-cache-2026-03-02/34687)
4. [Point production traffic to a given deployment](https://vercel.com/docs/rest-api/projects/point-production-traffic-to-a-given-deployment)
5. [Introducing vercel.ts: Programmatic project configuration - Vercel](https://vercel.com/changelog/vercel-ts)
6. [--- title: Programmatic Configuration with vercel.ts product: vercel](https://vercel.com/docs/project-configuration/vercel-ts.md)
7. [Project Configuration](https://academy-front.vercel.sh/docs/project-configuration)
8. <https://vercel.com/docs/cron-jobs/manage-cron-jobs>
9. [Authentication in Vercel Connect](https://vercel.com/docs/connect/concepts/authentication)
10. [Troubleshooting domains](https://vercel.com/kb/guide/a-record-and-caa-with-vercel)
11. [How to Point Your Custom Domain to Vercel (next.js)](https://www.namesilo.com/blog/en/dns/how-to-point-your-custom-domain-to-vercel-nextjs)
12. [Can I use my domain on Vercel with A records?](https://examples.vercel.com/kb/guide/a-record-and-caa-with-vercel)
13. [Cloudflare in Front of Vercel: Next.js Caching Setup and Trade-offs](https://blog.blazingcdn.com/en-us/cloudflare-vercel-workflow-deploying-nextjs-edge-caching)
14. [Vercel functions get "Connection terminated unexpectedly" from Postgres? How to fix it](https://www.prisma.io/postgres/connection-terminated-unexpectedly)
15. [Higher defaults and limits for Vercel Functions running Fluid compute - Vercel](https://vercel.com/changelog/higher-defaults-and-limits-for-vercel-functions-running-fluid-compute)
16. [Connection Pooling with Vercel Functions](https://vercel.com/kb/guide/connection-pooling-with-functions)
17. [Fluid compute](https://vercel.com/docs/fluid-compute)
18. [Runtimes](https://vercel.com/docs/runtimes)
19. [bun/docs/guides/deployment/vercel.mdx at main · oven-sh/bun · GitHub](https://github.com/oven-sh/bun/blob/main/docs/guides/deployment/vercel.mdx)
20. [Static Configuration with vercel.json](https://vercel.com/docs/project-configuration/vercel-json)
21. [Application authentication on Vercel](https://vercel.com/kb/guide/application-authentication-on-vercel)
22. [Proxy Basics](https://vercel.com/academy/nextjs-foundations/proxy-basics)
23. [routing-middleware](https://mcpservers.org/agent-skills/openai/routing-middleware)
24. [Routing Middleware API](https://vercel.com/docs/routing-middleware/api)
25. [Routing Middleware](https://vercel.com/docs/routing-middleware)
26. [Getting Started with Routing Middleware](https://vercel.com/docs/routing-middleware/getting-started)
27. [Routing Middleware: JS Reference](https://vercel.com/docs/routing-middleware/js-reference#continuing-the-routing-middleware-chain)
28. [Middleware and Routing](https://vercel.com/docs/platforms/multi-tenant-platforms/middleware-and-routing)
29. [Point production traffic to a previous production deployment by ID](https://vercel.com/docs/rest-api/projects/point-production-traffic-to-a-previous-production-deployment-by-id)
30. [Rolling Releases](https://vercel.com/docs/rolling-releases)
31. [Webhooks API Reference](https://vercel.com/docs/webhooks/webhooks-api)
32. [Trusted Sources](https://vercel.com/docs/deployment-protection/methods-to-bypass-deployment-protection/trusted-sources)
33. [Vercel Pricing 2026: Hobby, Pro, Usage and the Surprise Bill](https://axonbuild.com/blog/vercel-pricing/)
34. [Vercel Pricing 2026: Hobby Limits, Pro \$20 and Overage Rates](https://livemy.app/blog/vercel-pricing)
35. [Using Drains](https://vercel.com/docs/drains/using-drains)
36. [Logs](https://vercel.com/docs/logs)
37. [Vercel Developers on X: "Zero-configuration support for NestJS backends, now on Vercel. NestJS joins frameworks like Express, FastAPI, Flask, and Hono, all supported via framework-defined infrastructure. https://t.co/nsrLjghfwg" / X](https://x.com/vercel_dev/status/1979648839570350353)
38. [Vercel Developers on X: "You can now deploy Hono backends with zero-configuration. Hono apps deployed to Vercel benefit from Fluid compute, with Active CPU pricing, automatic cold start optimizations, background processing, and more. https://t.co/XEKqX2PKIe" / X](https://x.com/vercel_dev/status/1951297712114274531)
39. [Open source Workflow Development Kit is now in public beta - Announcements - Vercel Community](https://community.vercel.com/t/open-source-workflow-development-kit-is-now-in-public-beta/25854)
40. [plugins/plugins/vercel/skills/vercel-queues/SKILL.md at main · openai/plugins · GitHub](https://github.com/openai/plugins/blob/main/plugins/vercel/skills/vercel-queues/SKILL.md)
41. [Vercel Ship 2026: a sourced recap for builders](https://www.frankx.ai/guides/vercel-ship-2026-recap)
42. [Vercel Schedules is now in public beta - Vercel](https://vercel.com/changelog/vercel-schedules-is-now-in-public-beta)
43. [Cron jobs now support 100 per project on every plan - Vercel](https://vercel.com/changelog/cron-jobs-now-support-100-per-project-on-every-plan)
44. [AI Gateway Trace Drains](https://vercel.com/docs/ai-gateway/observability-and-spend/trace-drains)
45. [Deploy Hono backends with zero configuration - Vercel](https://vercel.com/changelog/deploy-hono-backends-with-zero-configuration)
46. [Zero-configuration Express backends - Vercel](https://vercel.com/changelog/zero-configuration-express-backends)
47. [Zero-configuration support for NestJS - Vercel](https://vercel.com/changelog/zero-configuration-support-for-nestjs)
48. [Zero-configuration support for Fastify - Vercel](https://vercel.com/changelog/zero-configuration-support-for-fastify)
49. [Introducing zero-configuration Hono backends - Announcements - Vercel Community](https://community.vercel.com/t/introducing-zero-configuration-hono-backends/17619)
50. [Running background workers on Vercel](https://vercel.com/kb/guide/background-workers-on-vercel)
51. [Usage & Pricing for Cron Jobs](https://vercel.com/docs/cron-jobs/usage-and-pricing)
52. [How Vercel Services run on Fluid compute](https://vercel.com/kb/guide/vercel-services-fluid-compute)
53. [vercel-queues](https://lobehub.com/skills/vercel-vercel-plugin-vercel-queues)
54. <https://vercel.com/docs/functions/runtimes/go.md>
55. [Go](https://endoflife.date/go)
56. [Vercel Functions with Go](https://jhartman.pl/posts/hosting-and-home-page/2026-02-07-vercel-functions-with-go/)
57. [How to stop Vercel Functions from timing out](https://vercel.com/kb/guide/what-can-i-do-about-vercel-serverless-functions-timing-out)
58. [Vercel Function Timeout Limits by Plan & Runtime](https://hyperobserve.com/limits/vercel/function-duration/)
59. [fix(deploy): promote and roll back through Vercel's API, and say what production serves (#149) by dragondad22 · Pull Request #150 · dragondad22/CrossWise · GitHub](https://github.com/dragondad22/CrossWise/pull/150)
60. [Using Vercel's instant rollback feature in your own CI/CD pipeline - DEV Community](https://dev.to/philw_/using-vercels-instant-rollback-feature-in-your-own-cicd-pipeline-57oi)
61. [List aliases](https://vercel.com/docs/rest-api/aliases/list-aliases)
62. [Push Images from GitHub Actions](https://vercel.com/docs/container-registry/github-actions)
63. [Vercel Traces: Tree and Waterfall Views for Slow Spans](https://www.digitalapplied.com/blog/vercel-traces-tree-waterfall-views-performance-2026)
64. [Runtime Logs](https://vercel.com/docs/logs/runtime)
65. [Tracing](https://vercel.com/docs/tracing)
66. [Working with Drains](https://vercel.com/docs/drains)
67. [Vercel WAF vs Cloudflare WAF](https://vercel.com/kb/guide/vercel-waf-vs-cloudflare-waf)
68. [Introducing Vercel Drains: Complete observability data, anywhere - Vercel](https://vercel.com/blog/introducing-vercel-drains)
69. [Trace Drains Reference](https://vercel.com/docs/drains/reference/traces)
70. [Vercel](https://docs.datadoghq.com/integrations/vercel/)
71. [Vercel OpenTelemetry Assets](https://www.elastic.co/docs/reference/integrations/vercel_otel)
72. [Introducing the Vercel Marketplace - Vercel](https://vercel.com/blog/introducing-the-vercel-marketplace)
73. [Integrated billing for Supabase, Redis, and EdgeDB - Vercel](https://vercel.com/changelog/integrated-billing-for-supabase-redis-and-edgedb)
74. [Redis on Vercel](https://vercel.com/docs/redis)
75. [@vercel/functions API Reference (Node.js)](https://vercel.com/docs/functions/functions-api-reference/vercel-functions-package)
76. [Hosted Postgres says "too many connections" when people open my app. How do I fix it without managing extra servers?](https://www.prisma.io/postgres/too-many-connections)
77. [Connecting to Neon from Vercel - Neon Docs](https://neon.com/docs/guides/vercel-connection-methods)
78. [Deploy to Vercel (Prisma ORM v7)](https://www.prisma.io/docs/orm/v7/prisma-client/deployment/serverless/deploy-to-vercel)
79. [Shared Pooler (Supavisor) client connections keep growing with Vercel Fluid + attachDatabasePool · supabase · Discussion #40671 · GitHub](https://github.com/orgs/supabase/discussions/40671)
80. [Vercel KV sunset: how to migrate your Redis data](https://layerbase.com/blog/vercel-kv-sunset-migration)
81. [Upstash Redis REST API - Upstash Documentation](https://upstash.com/docs/redis/features/restapi)
82. [blog/posts/2026-03-31-redis-how-to-use-upstash-rest-api-for-serverless-redis/README.md at master · OneUptime/blog · GitHub](https://github.com/OneUptime/blog/blob/master/posts/2026-03-31-redis-how-to-use-upstash-rest-api-for-serverless-redis/README.md)
83. [Migrating from Vercel KV to Layerbase](https://layerbase.com/blog/migrating-from-vercel-kv-to-layerbase)
84. [Should I use Cloudflare in front of Vercel?](https://vercel.com/docs/integrations/external-platforms/cloudflare)
85. [Using Self-hosted & Reverse Proxies with Vercel](https://examples.vercel.com/kb/guide/how-to-setup-verified-proxy)
86. [Cloudflare Proxy Issue - Help - Vercel Community](https://community.vercel.com/t/cloudflare-proxy-issue/667)
87. [CNAME vs A record: Key differences explained - Domains](https://vercel.com/i/cname-vs-a-record)
88. [Reverse Proxy Servers and Vercel](https://vercel.com/docs/security/reverse-proxy)
89. [--- title: Reverse Proxy Servers and Vercel product: vercel](https://vercel.com/docs/security/reverse-proxy.md)
90. [How to fix Vercel Invalid Configuration error with Cloudflare Proxy enabled - Discussions - Vercel Community](https://community.vercel.com/t/how-to-fix-vercel-invalid-configuration-error-with-cloudflare-proxy-enabled/37446)
91. [S3 API compatibility · Cloudflare R2 docs](https://developers.cloudflare.com/r2/api/s3/api/)
92. [R2 is now Generally Available](https://blog.cloudflare.com/r2-ga)
93. [Data location · Cloudflare R2 docs](https://developers.cloudflare.com/r2/reference/data-location/)
94. [Presigned URLs](https://developers.cloudflare.com/r2/api/s3/presigned-urls/)
95. [Cloudflare](https://developers.cloudflare.com/r2/pricing/index.md)
96. [Upload objects · Cloudflare R2 docs](https://developers.cloudflare.com/r2/objects/upload-objects/)
97. [Cloudflare R2 S3 Endpoint Setup Guide (2026)](https://www.bucketmate.app/blogs/s3-compatible-setup-cloudflare-r2)
