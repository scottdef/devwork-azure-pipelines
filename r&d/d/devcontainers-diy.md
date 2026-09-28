# Devcontainers for VS Code on Windows 11 / Azure Virtual Desktop: Reference, Runbooks, Examples, and a Go 1.21 Companion Tool (September 2026)

Standardize on one devcontainer.json-driven platform. Build org-owned base images in GitHub Actions, pin them by digest, and scan and sign them before release. Engineers consume those images locally in VS Code only where nested virtualization is confirmed on their AVD session hosts. Everyone else uses GitHub Codespaces or remote Docker/AKS-hosted containers. CI runs every build, test and deploy (Spacelift, Databricks bundles, AKS admin) inside the same image through `devcontainers/ci`. Two of your pinned versions are past vendor support: AKS 1.30 and Go 1.21. You can still run them, but they are risk items with a compensating plan, not a steady state.

## TL;DR
- **Build once, run everywhere:** one devcontainer spec, org images in GHCR/ACR pinned by `@sha256` digest, prebuilt by `devcontainers/ci@v0.3`, and reused in VS Code, Codespaces, Copilot cloud agent (`copilot-setup-steps.yml`) and Actions. The companion Go tool (`devc`) lints configs against org policy, inventories repos over the GitHub REST API, reports drift, and exposes Prometheus metrics for Grafana 12.
- **The host is the constraint:** Docker Desktop on AVD needs a nested-virtualization-capable VM size with Standard (not Trusted Launch) security on older sizes. It also needs a Docker Business subscription if you are over 250 employees or $10M revenue. Where either condition fails, use Codespaces or a remote Docker/AKS engine; don't fight the VDI.
- **Versions pinned by the project are past vendor support:** AKS 1.30 LTS reached end of life in July 2026, and kubectl 1.30 only supports API servers 1.29–1.31. Go 1.21 is six feature releases behind current Go 1.27, which the official Go blog dates to 19 August 2026. Grafana OSS 12 has been followed by Grafana 13, which Grafana Labs unveiled at GrafanaCON 2026 on April 21, 2026; 13.2.2 (15 September 2026) is the current release. The docs below honor the pins but flag each one with a mitigation.

## Key Findings

| Area | Finding | Implication |
|---|---|---|
| Spec | `capAdd`, `securityOpt`, `privileged`, `mounts`, `init`, `remoteUser`, and lifecycle hooks are cross-orchestrator properties, and many can live in the `devcontainer.metadata` image label\[1\] | Put security-relevant defaults in the image label so repos inherit them; lint repos for overrides |
| Lifecycle | `onCreateCommand`/`updateContentCommand` run at prebuild time without user secrets; `postCreateCommand` runs after assignment to a user. If one lifecycle script fails, subsequent ones are skipped\[1\] | Never fetch user secrets in `onCreate`; make scripts idempotent and fail-fast |
| Dependabot | `devcontainers` ecosystem GA since Jan 2024; updates Features (and lockfiles), not base images; no security updates\[2\]\[3\] | Use `docker` ecosystem for Dockerfiles plus `devc drift` for `image:` digests |
| AVD | Docker Docs: "Support for running Docker Desktop on a virtual desktop is available to Docker Business customers, on VMware ESXi or Azure VMs only," provided nested virtualization is enabled; "D4s_v5 machines were used for internal testing" | Standardize AVD host pools on nested-virt SKUs, or go remote |
| AKS | 1.30 community EOL Aug 22, 2025; LTS EOL Jul 2026; kubectl supports ±1 minor\[4\] | kubectl 1.30 is valid only against 1.29–1.31 clusters; 1.31 LTS ends Nov 2026 |
| Copilot | `copilot-setup-steps.yml` must contain a single `copilot-setup-steps` job, run from the default branch, `timeout-minutes` ≤ 59; integrated firewall is incompatible with self-hosted runners | Agent sandboxes on ARC need your own egress control |\[5\]
| Databricks | "Databricks Asset Bundles" renamed "Declarative Automation Bundles" on Mar 16, 2026 — non-breaking\[6\]\[7\] | Keep `databricks bundle` commands; use `DATABRICKS_AUTH_TYPE=github-oidc` |
| Spacelift | `spacelift-io/setup-spacectl@v2` + `spacectl stack deploy --id`; OIDC exchange supported\[8\]\[9\] | Remove long-lived Spacelift API secrets |

---

## Part 1 — Reference Guides

### 1.1 devcontainer.json: the properties that matter

The spec defines `devcontainer.json` as metadata describing "how to enrich a container for the purposes of development rather than acting as a multi-container orchestrator format". Three scenario families exist: `image`, `build` (Dockerfile), and `dockerComposeFile` + `service`.\[1\]

**General properties (🏷️ = can be baked into the `devcontainer.metadata` image label):**

| Property | Use in this platform |
|---|---|
| `name` | Profile name shown in VS Code |
| `image` / `build.dockerfile` / `build.context` / `build.args` / `build.target` / `build.cacheFrom` | Always reference an org image by digest; `cacheFrom` points at the prebuilt image in GHCR/ACR |
| `features`, `overrideFeatureInstallOrder` | Feature IDs from OCI (e.g., `ghcr.io/devcontainers/features/...`), pinned by major version or digest |
| `containerEnv` 🏷️ vs `remoteEnv` 🏷️ | `containerEnv` is static for the container's life (rebuild to change) and visible to all processes; `remoteEnv` is client-scoped and can change without rebuild |
| `remoteUser` 🏷️ / `containerUser` 🏷️ / `updateRemoteUserUID` 🏷️ | Run as non-root `vscode`/`dev`; UID/GID remapping to the local user defaults to `true` on Linux (prevents bind-mount permission problems) |
| `privileged` 🏷️ | Default `false`; required for Docker-in-Docker but has security implications — forbidden by org policy except in the approved DinD profile |
| `capAdd` 🏷️ | Default `[]`; e.g., `SYS_PTRACE` for Go/delve debugging; `NET_ADMIN`/`NET_RAW` only for nettools and the agent-firewall profile |
| `securityOpt` 🏷️ | e.g., `seccomp=unconfined` — forbidden by policy unless a waiver exists |
| `init` 🏷️ | `true` to run tini and reap zombie processes (important for agent sandboxes spawning subprocesses) |
| `mounts` 🏷️ | Docker `--mount` syntax; never mount `/var/run/docker.sock` or `~/.ssh` into agent sandboxes |
| `runArgs` | Raw `docker run` flags (array only). Policy scans for `--privileged`, `--cap-add`, `--network=host`, `--pid=host` |
| `forwardPorts` / `portsAttributes` / `otherPortsAttributes` | Prefer `forwardPorts` (localhost semantics) over `appPort` (requires listening on 0.0.0.0) |
| `hostRequirements` 🏷️ (`cpus`, `memory`, `storage`, `gpu`) | Cloud services pick a matching machine; locally you only get a warning |
| `customizations.vscode` 🏷️ | `extensions`, `settings`, and (current VS Code) `mcp` server definitions |
| `workspaceMount` + `workspaceFolder` | Needed for clone-in-volume and monorepo sub-folder setups |
| `shutdownAction`, `overrideCommand`, `userEnvProbe` | `overrideCommand` defaults true for image/Dockerfile and false for Compose |

**Lifecycle hooks, in order:**

| Hook | Runs where / when | Secrets available? | Platform rule |
|---|---|---|---|
| `initializeCommand` | On the **host** (for Codespaces that is the cloud VM), on create and on each start | Host env | Keep trivial; on AVD this runs in Windows/WSL context |
| `onCreateCommand` 🏷️ | Inside container, first start; used by prebuilds | No user-scoped secrets | Toolchain warm-up (`go mod download`) |
| `updateContentCommand` 🏷️ | After `onCreate` whenever new content exists; re-run by prebuild refresh | Repo/org scoped only | Dependency sync |
| `postCreateCommand` 🏷️ | After the container is assigned to a user | User secrets available | `az login` hints, git config |
| `postStartCommand` 🏷️ | Each container start | Yes | Firewall init for agent sandbox |
| `postAttachCommand` 🏷️ | Each tool attach | Yes | Banner / health check |
| `waitFor` 🏷️ | Default `updateContentCommand` | — | Leave default |

String values run in `/bin/sh`; array values bypass the shell; object values run commands in parallel. **If one lifecycle script fails, subsequent scripts are not executed**,\[1\] so a failing `postCreateCommand` silently skips your `postStartCommand` firewall. Make security-critical steps fail loudly and verify them in `postAttachCommand`.

### 1.2 Features and Templates: authoring and publishing to GHCR (OCI)

- **Feature layout:** `src/<id>/devcontainer-feature.json` (id, version, options, `installsAfter`, `capAdd`, `containerEnv`, etc.) plus `install.sh`, which runs as root at build time. Feature metadata can itself declare `capAdd`/`privileged`/`securityOpt`/`mounts`. **Admins must lint Feature metadata too, not just repo configs.** The spec merges arrays as unions and booleans with logical OR,\[10\] so a single Feature with `privileged: true` elevates the whole container.
- **Template layout:** `src/<id>/devcontainer-template.json` plus `.devcontainer/` contents with `${templateOption:...}` placeholders.
- **Publishing:** use the `devcontainers/action` (or `devcontainer features publish` / `devcontainer templates publish` from `@devcontainers/cli`) to push to `ghcr.io/<org>/<collection>/<id>:<semver>`. Make the GHCR packages internal-visibility and grant read to the org.
- **Versioning:** semantic versions; consumers pin `:1` (auto minor) in dev and a full digest in regulated repos. Dependabot's `devcontainers` ecosystem "ensures Features are pinned to the latest major version" and updates the lockfile when present.\[11\]

### 1.3 Docker Compose-based devcontainers

Use `dockerComposeFile` (string or ordered array; later files override earlier ones), `service` (the container VS Code attaches to), `runServices`, and `workspaceFolder` (default `/`). Compose is the right choice when the devcontainer needs sidecars: a local Postgres for Go integration tests, a Prometheus + Grafana 12 pair for dashboard development, or an egress proxy container for agent sandboxes. `shutdownAction` defaults to `stopCompose`.

### 1.4 Prebuilds, pinning, and hermeticity

**Prebuild chain (canonical):** base Dockerfile → `devcontainers/ci` builds image with Features applied → Trivy scan → SBOM + provenance attestations → cosign sign → push by digest → PR bumps `image:` digest in consuming repos (`devc drift` or Renovate/Dependabot `docker` ecosystem).

- `devcontainers/ci` uses Docker BuildKit to store layer cache metadata with the image. BuildKit is installed on hosted runners; on self-hosted runners add `docker/setup-buildx-action`.\[12\]
- **Pin by digest:** `image: ghcr.io/contoso/devc/go@sha256:<64-hex>`. A tag is a mutable pointer, and a digest is the content. `devc lint` enforces this.
- **Hermeticity checklist:** pin the base image digest; pin apt packages with snapshot repos or at least record them in the SBOM; pin Go with `go 1.21.x` + `toolchain go1.21.x` in go.mod; set `GOTOOLCHAIN=local` so the Go 1.21 toolchain never auto-downloads a newer one (Go 1.21 introduced toolchain management and treats the `go` line as a strict minimum);\[13\] vendor or proxy modules (`GOFLAGS=-mod=readonly`, `GOPROXY` pointing at your artifact proxy).
- **Supply chain:** generate SBOM (`anchore/sbom-action` or `docker buildx --sbom=true`), provenance (`actions/attest-build-provenance`), and sign with `cosign sign --yes <image>@<digest>` using keyless OIDC. Verify at consumption time in CI (`cosign verify --certificate-identity-regexp ... --certificate-oidc-issuer https://token.actions.githubusercontent.com`).
- **Dependabot:** add both ecosystems:

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: "devcontainers"   # Features in devcontainer.json (+ lockfile)
    directory: "/"
    schedule: { interval: weekly }
  - package-ecosystem: "docker"          # FROM lines in .devcontainer/Dockerfile
    directory: "/.devcontainer"
    schedule: { interval: weekly }
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule: { interval: weekly }
```

Dependabot security updates are not supported for devcontainers,\[3\] so vulnerability-driven rebuilds must come from your image pipeline, not from Dependabot.

---

## Part 2 — Host Platform Guidance: Windows 11 on Azure Virtual Desktop

### 2.1 Decision tree

1. **Is the AVD session host a nested-virtualization-capable size with Standard security type?** Use Dv5/Ev5-class or newer sizes whose size page lists nested virtualization.\[14\] Microsoft Q&A guidance: "For VM sizes less than v5 you need to select Standard because Nested Virtualization isn't supported with Trusted Launch."\[15\] Docker Docs ("Run Docker Desktop for Windows in a VM or VDI environment") state: "D4s_v5 machines were used for internal testing. Use this specification or above for optimal performance of Docker Desktop."
   - **Yes → local engine is possible.** Go to step 2.
   - **No → remote engine** (§2.5).
2. **Is the host pool personal (persistent) or pooled multi-session?** Docker supports persistent VDI; Docker "does not support running multiple instances of Docker Desktop on the same machine in a VM or VDI environment".\[16\]\[17\] **Pooled multi-session hosts therefore cannot give each user Docker Desktop.** Use personal host pools for container-heavy engineers, or go remote.
3. **Licensing:** Docker Desktop requires a paid per-user subscription for organizations with more than 250 employees or more than $10M annual revenue.\[18\] Support in Azure VMs is limited to **Docker Business** customers.\[16\]
4. **Engine choice:**

| Engine on AVD | License | Notes |
|---|---|---|
| Docker Desktop (WSL2 backend) | Docker Business required at your scale | Supported on Azure VMs with nested virt; admin controls (Settings Management, Enhanced Container Isolation) |
| Docker Engine (docker-ce) inside your own WSL2 distro | Apache 2.0, free | No GUI; you manage updates; VS Code Dev Containers can use it via "Execute in WSL" |
| Podman (WSL machine) / Podman Desktop | Apache 2.0 | Set `dev.containers.dockerPath: podman`; rootless UID mapping differs (see §8.5) |
| Rancher Desktop | Apache 2.0 | moby or containerd; also nested-virt dependent |

**Known issue (September 2026):** a public report on `docker/desktop-feedback` #137 describes Docker Desktop 4.62.0 (WSL2 backend) crashing the kernel at first engine start on AVD hosts with AMD EPYC 9V45 (Turin, Dasv7) at 4+ vCPUs. The standard WSL2 kernel was unaffected. The reporter's workarounds were to initialize on 2 vCPU then resize, or use `.wslconfig` with `processors=2`.\[19\] This is a single user report, not a Docker advisory. Pilot new SKUs before a fleet rollout.

### 2.2 Enabling WSL2 on AVD session hosts

```powershell
# In the session-host image build (Azure Image Builder / Packer), as admin:
wsl.exe --install --no-distribution          # installs WSL + Virtual Machine Platform
# Reboot, then per user:
wsl --install -d Ubuntu-24.04
wsl --version                                  # confirm WSL 2.x from the Store/MSI package
```

The error `Wsl/Service/CreateVm/HCS/HCS_E_HYPERV_NOT_INSTALLED` or "enable virtualization in the BIOS" on an AVD host almost always means a non-nested SKU or Trusted Launch.\[20\]\[21\] Fix the VM size or security type; nothing inside the guest can fix it.

### 2.3 Performance

- **Keep sources in the Linux filesystem** (`\\wsl$\Ubuntu\home\<user>\src` or a Docker volume), not `C:\` bind mounts. Cross-OS 9P mounts are the main cause of slow `go build` and `git status`.
- **Clone Repository in Container Volume** (VS Code command) puts the repo in a named volume. This is the fastest option and avoids CRLF and permission problems entirely.
- Size WSL in `%UserProfile%\.wslconfig` (`memory=`, `processors=`). Size the AVD VM for ~4 vCPU/16 GB per concurrent container-heavy user.
- FSLogix profile containers: exclude `%LocalAppData%\Docker\wsl` and the WSL `ext4.vhdx` from roaming if hosts are personal, or they will bloat profiles.

### 2.4 Proxy and corporate TLS CA

Microsoft's enterprise WSL guidance recommends these `.wslconfig` settings: `networkingMode=mirrored` (improves compatibility with VPNs and complex networks), `dnsTunneling=true` (resolves DNS through a virtualization channel rather than packets), and `autoProxy=true` (propagates the Windows HTTP proxy).\[22\]\[23\]

```ini
# %UserProfile%\.wslconfig  (deploy via Intune/GPO)
[wsl2]
networkingMode=mirrored
dnsTunneling=true
autoProxy=true
firewall=true
memory=12GB
processors=4
```

Inside images, bake the corporate root CA at build time rather than per-user:

```dockerfile
COPY certs/contoso-root-ca.crt /usr/local/share/ca-certificates/contoso-root-ca.crt
RUN update-ca-certificates
ENV NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt \
    REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
```

Pass proxy variables via `containerEnv` using `${localEnv:HTTPS_PROXY}` so they aren't hard-coded in repos. Go honors `HTTPS_PROXY`/`NO_PROXY` natively.

### 2.5 When nested virtualization is not available

| Option | How | Best for |
|---|---|---|
| **GitHub Codespaces** | Same `devcontainer.json`; `hostRequirements` selects the machine; org policies restrict images, machine types, port visibility | Default fallback for all developers |
| **Remote Docker host** (Linux VM in the same VNet) | VS Code `docker.host`/Docker context over SSH; Dev Containers builds and runs there | Nettools and AKS admin work needing VNet line-of-sight to private clusters |
| **Devcontainer on AKS** | A dedicated "devbox" node pool; pod from the devcontainer image; connect with Kubernetes extension "Attach Visual Studio Code" or run `code tunnel` in the pod | Platform teams, GPU, or regulated data locality |
| **Docker Offload / Docker Cloud** | Docker's recommendation when nested virt is unsupported (was beta; contact Docker)\[17\] | Orgs already on Docker Business |
| **DevPod-style** | Open-source clients that create devcontainers on SSH/K8s/cloud providers from the same spec | Teams wanting provider flexibility |

---

## Part 3 — Security and Governance for Admins

### 3.1 GitHub Enterprise Cloud controls

- **Codespaces policies (org → Settings → Codespaces → Policies):** restrict base images ("Restricting the base image"), machine types, max codespaces per user, idle timeout, retention, and forwarded-port visibility.\[24\] The image policy applies at codespace creation. It does not apply to the default image or to the recovery image.\[25\] If a Dockerfile's base does not match, the codespace is created in **recovery mode**.\[26\] Pair it with `devc lint` in CI so rebuilds are covered too.
- **Secrets:** Codespaces secrets (user/repo/org) are injected at `postCreate` time. Actions secrets go per environment with required reviewers for prod. Copilot agent secrets use the Agents secret store with the `COPILOT_MCP_` prefix for MCP use.
- **Rulesets:** require the `devc-lint` status check on changes to `.devcontainer/**` and `.github/workflows/copilot-setup-steps.yml`; add CODEOWNERS for the platform team on those paths.

### 3.2 Least-privilege capability matrix (enforced by `devc`)

| Profile | `privileged` | `capAdd` allowed | Other |
|---|---|---|---|
| go-dev, iac, databricks | false | `SYS_PTRACE` | no host network, no docker.sock |
| aks-admin | false | none | kubeconfig via `az aks get-credentials` + kubelogin; no mounted `~/.kube` from Windows |
| nettools | false | `NET_ADMIN`, `NET_RAW` | `--network=host` only with waiver; authorized-targets file required |
| agent-sandbox | false | `NET_ADMIN`, `NET_RAW` (firewall init only) | agent runs as non-root without sudo; no docker.sock; `init: true` |
| dind (exception) | true | — | waiver + expiry date |

### 3.3 Image scanning and registries

- **Trivy** in the image pipeline (`aquasecurity/trivy-action`, fail on CRITICAL/HIGH with fix available). **Microsoft Defender for Containers** scans ACR continuously; mirror approved devcontainer images to ACR if you want Defender coverage and AKS artifact streaming.
- **GHCR auth:** `GITHUB_TOKEN` with `packages: write` in the publishing repo; consumers use `packages: read`. Locally, `gh auth token | docker login ghcr.io -u <user> --password-stdin`.
- **ACR auth:** from Actions via OIDC (`azure/login` → `az acr login`); from AKS via kubelet managed identity `AcrPull`; locally via `az acr login -n <acr>` (Entra ID).

### 3.4 Entra ID, workload identity, OIDC

- **GitHub Actions → Azure:** federated credential on an Entra app or user-assigned managed identity. Use subject `repo:<org>/<repo>:environment:<env>`; job needs `permissions: id-token: write`; `azure/login` with `client-id`, `tenant-id`, `subscription-id` only — no secrets.
- **GitHub → Databricks:** workload identity federation policy on the Databricks service principal; set `DATABRICKS_AUTH_TYPE=github-oidc`.\[27\] Databricks notes `id-token: write` must be set "on the calling workflow, not the reusable workflow."\[28\]
- **GitHub → Spacelift:** OIDC-enabled API key. The spacelift-io/spacectl README explains: "The OIDC token GitHub issues is short-lived (~5 minutes), so exchange it once for a Spacelift session token (valid up to ~10 hours) and reuse that for the rest of the job."
- **Pods on AKS (ARC runners, devbox pods):** AKS Workload Identity (service account annotated with the managed identity client ID) rather than node identity.

---

## Part 4 — CI/CD Orchestration with GitHub Actions

### 4.1 Reusable workflow: build image, scan, sign, push, then run CI inside it

```yaml
# .github/workflows/devcontainer-image.yml  (reusable)
name: devcontainer-image
on:
  workflow_call:
    inputs:
      image: { type: string, required: true }      # ghcr.io/contoso/devc/go
      subfolder: { type: string, default: "." }
      runCmd: { type: string, default: "make ci" }
      runner: { type: string, default: "ubuntu-latest" }
    outputs:
      digest: { value: "${{ jobs.build.outputs.digest }}" }
permissions: { contents: read, packages: write, id-token: write, attestations: write }
jobs:
  build:
    runs-on: ${{ inputs.runner }}
    outputs: { digest: "${{ steps.digest.outputs.digest }}" }
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ghcr.io, username: "${{ github.actor }}", password: "${{ secrets.GITHUB_TOKEN }}" }
      - name: Build and run CI inside the devcontainer
        uses: devcontainers/ci@v0.3
        with:
          subFolder: ${{ inputs.subfolder }}
          imageName: ${{ inputs.image }}
          cacheFrom: ${{ inputs.image }}
          push: filter
          refFilterForPush: refs/heads/main
          eventFilterForPush: push
          runCmd: ${{ inputs.runCmd }}
      - id: digest
        if: github.ref == 'refs/heads/main' && github.event_name == 'push'
        run: echo "digest=$(docker buildx imagetools inspect ${{ inputs.image }}:latest --format '{{json .Manifest.Digest}}' | tr -d '\"')" >> "$GITHUB_OUTPUT"
      - name: Scan
        if: steps.digest.outputs.digest != ''
        uses: aquasecurity/trivy-action@0.28.0
        with: { image-ref: "${{ inputs.image }}@${{ steps.digest.outputs.digest }}", severity: "CRITICAL,HIGH", ignore-unfixed: true, exit-code: "1" }
      - uses: sigstore/cosign-installer@v3
        if: steps.digest.outputs.digest != ''
      - name: Sign
        if: steps.digest.outputs.digest != ''
        run: cosign sign --yes "${{ inputs.image }}@${{ steps.digest.outputs.digest }}"
      - uses: actions/attest-build-provenance@v2
        if: steps.digest.outputs.digest != ''
        with: { subject-name: "${{ inputs.image }}", subject-digest: "${{ steps.digest.outputs.digest }}", push-to-registry: true }
```

Pin third-party actions by commit SHA in production. Tags are shown here for readability.

### 4.2 Matrix builds and caching

Use `strategy.matrix.profile: [go, aks-admin, nettools, agent-sandbox, iac, databricks]` with `subfolder: images/${{ matrix.profile }}`. Registry cache (`cacheFrom`) is the primary cache. For Go module caches inside `runCmd`, mount a named volume or use `actions/cache` on a host path bind-mounted through `devcontainer.json` `mounts` in a CI-only override.

### 4.3 Hosted vs self-hosted runners (ARC on AKS)

| | GitHub-hosted | ARC on AKS (self-hosted) |
|---|---|---|
| Docker for `devcontainers/ci` | Built in | Needs `containerMode: dind` (privileged sidecar) or a rootless builder; isolate on a dedicated node pool with taints |
| Private network (AKS API, Databricks private link) | Only via Azure private networking for larger runners | Native VNet |
| Copilot cloud agent firewall | Supported | **Incompatible — must be disabled**;\[29\] you supply egress control (Azure Firewall/NSG + proxy) |\[30\]
| Recommendation | Image builds, public-dependency CI | Deploy jobs needing VNet; agent jobs with your own egress |

GitHub recommends ephemeral, single-use runners for Copilot cloud agent ("Most customers set this up using ARC").\[5\] The same principle applies to every job that executes devcontainer images. ARC scale sets should be ephemeral by default.

### 4.4 Triggering Spacelift and Databricks bundles from devcontainer-based jobs

```yaml
# .github/workflows/deploy.yml
name: deploy
on: { push: { branches: [main] } }
permissions: { contents: read, id-token: write, packages: read }
jobs:
  iac:
    runs-on: [self-hosted, arc-vnet]
    environment: prod
    steps:
      - uses: actions/checkout@v4
      - uses: devcontainers/ci@v0.3
        env:
          SPACELIFT_API_KEY_ENDPOINT: ${{ vars.SPACELIFT_ENDPOINT }}
          SPACELIFT_API_KEY_ID: ${{ secrets.SPACELIFT_API_KEY_ID }}
          SPACELIFT_API_KEY_SECRET: ${{ secrets.SPACELIFT_API_KEY_SECRET }}
        with:
          subFolder: .devcontainer/iac
          cacheFrom: ghcr.io/contoso/devc/iac
          push: never
          env: |
            SPACELIFT_API_KEY_ENDPOINT
            SPACELIFT_API_KEY_ID
            SPACELIFT_API_KEY_SECRET
          runCmd: |
            tofu fmt -check -recursive && tofu validate
            spacectl stack deploy --id platform-network-prod --tail
  bundle:
    runs-on: ubuntu-latest
    environment: prod
    steps:
      - uses: actions/checkout@v4
      - name: Fetch GitHub OIDC token for Databricks inside the container
        id: oidc
        run: |
          echo "::add-mask::placeholder"
      - uses: devcontainers/ci@v0.3
        env:
          DATABRICKS_HOST: ${{ vars.DATABRICKS_HOST }}
          DATABRICKS_CLIENT_ID: ${{ vars.DATABRICKS_SP_APP_ID }}
          DATABRICKS_AUTH_TYPE: github-oidc
          ACTIONS_ID_TOKEN_REQUEST_URL: ${{ env.ACTIONS_ID_TOKEN_REQUEST_URL }}
          ACTIONS_ID_TOKEN_REQUEST_TOKEN: ${{ env.ACTIONS_ID_TOKEN_REQUEST_TOKEN }}
        with:
          subFolder: .devcontainer/databricks
          cacheFrom: ghcr.io/contoso/devc/databricks
          push: never
          env: |
            DATABRICKS_HOST
            DATABRICKS_CLIENT_ID
            DATABRICKS_AUTH_TYPE
            ACTIONS_ID_TOKEN_REQUEST_URL
            ACTIONS_ID_TOKEN_REQUEST_TOKEN
          runCmd: |
            databricks bundle validate -t prod
            databricks bundle deploy -t prod
```

Notes: the Databricks CLI's `github-oidc` auth fetches the token using the `ACTIONS_ID_TOKEN_REQUEST_*` variables, so they must be forwarded into the container (the `env:` input of `devcontainers/ci` lists variable names to pass through). Prefer the Spacelift OIDC exchange over the API-key secrets shown once your Spacelift account has an OIDC-enabled key. Spacelift's own GitHub App push policies remain the primary trigger. Use `spacectl` for orchestrated cross-stack deploys.

---

## Part 5 — AKS Admin Devcontainer and CKA-Level Runbooks

### 5.1 Version reality check (read before using kubectl 1.30)

| Fact (AKS supported-versions page, updated Sep 2026) | Consequence |
|---|---|
| 1.30: AKS GA Jul 2024, community EOL **Aug 22, 2025**, LTS EOL **Jul 2026**\[4\] | Any 1.30 cluster is now unsupported, even on Premium/LTS |
| 1.31 LTS EOL **Nov 2026**; 1.32 LTS Mar 2027\[4\] | kubectl 1.30 skew covers 1.29–1.31 only |
| Standard support now 1.34–1.37 (1.37 GA Oct 2026)\[4\] | kubectl 1.30 is **out of skew** against every standard-support cluster |

**Recommendation:** keep kubectl 1.30 as the pinned default only for clusters on 1.31 LTS (Premium tier) and plan their upgrade before November 2026. Ship `kubectl-1.34`+ side-by-side in the same image (`KUBECTL_ALT`) for standard-tier clusters, and have the project owner formally re-baseline the pin. `az aks install-cli` downloads the latest by default; the Dockerfile pins the binary instead.

### 5.2 Dockerfile (AKS admin)

```dockerfile
# images/aks-admin/Dockerfile
FROM mcr.microsoft.com/devcontainers/base:bookworm@sha256:<pin-me>
ARG KUBECTL_VERSION=v1.30.14
ARG HELM_VERSION=v3.16.4
ARG KUBELOGIN_VERSION=v0.1.6
ARG K9S_VERSION=v0.32.7
ARG FLUX_VERSION=2.4.0
ARG ARGOCD_VERSION=v2.13.3
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl jq unzip bash-completion dnsutils iproute2 tcpdump \
    && rm -rf /var/lib/apt/lists/*
# kubectl pinned + checksum
RUN curl -fsSLo /usr/local/bin/kubectl https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl \
 && curl -fsSLo /tmp/kubectl.sha256 https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl.sha256 \
 && echo "$(cat /tmp/kubectl.sha256)  /usr/local/bin/kubectl" | sha256sum -c - \
 && chmod +x /usr/local/bin/kubectl
RUN curl -fsSL https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz | tar -xz -C /tmp \
 && mv /tmp/linux-amd64/helm /usr/local/bin/helm
RUN curl -fsSLo /tmp/kl.zip https://github.com/Azure/kubelogin/releases/download/${KUBELOGIN_VERSION}/kubelogin-linux-amd64.zip \
 && unzip /tmp/kl.zip -d /tmp/kl && mv /tmp/kl/bin/linux_amd64/kubelogin /usr/local/bin/
RUN curl -fsSL https://github.com/derailed/k9s/releases/download/${K9S_VERSION}/k9s_Linux_amd64.tar.gz | tar -xz -C /usr/local/bin k9s
RUN curl -fsSL https://github.com/fluxcd/flux2/releases/download/v${FLUX_VERSION}/flux_${FLUX_VERSION}_linux_amd64.tar.gz | tar -xz -C /usr/local/bin flux
RUN curl -fsSLo /usr/local/bin/argocd https://github.com/argoproj/argo-cd/releases/download/${ARGOCD_VERSION}/argocd-linux-amd64 && chmod +x /usr/local/bin/argocd
USER vscode
```

Versions are illustrative pins. Verify each release and checksum when you build, and let Dependabot/Renovate `regex` managers bump the `ARG`s. Azure CLI comes from the `ghcr.io/devcontainers/features/azure-cli` Feature in devcontainer.json.

### 5.3 Runbooks (CKA level)

**RB-AKS-01 Node NotReady**
1. `kubectl get nodes -o wide`; `kubectl describe node <n>`. Check Conditions (MemoryPressure, DiskPressure, PIDPressure, Ready reason) and recent events.
2. `kubectl get --raw /api/v1/nodes/<n>/proxy/healthz`. If it times out, look at the node from Azure: `az vmss list-instances`, `az aks nodepool show`, and the node's boot diagnostics.
3. Shell onto the node: `kubectl debug node/<n> -it --image=mcr.microsoft.com/cbl-mariner/busybox:2.0` then `chroot /host`. Run `systemctl status kubelet containerd`, `journalctl -u kubelet --since -30m`, `crictl ps -a`, `df -h /var/lib/containerd`.
4. Remediate: `kubectl cordon` → `kubectl drain <n> --ignore-daemonsets --delete-emptydir-data`, then `az aks nodepool upgrade --node-image-only` or reimage the VMSS instance. Uncordon only after Ready plus workloads are healthy.
5. Clean up the debugger pod (`kubectl delete pod node-debugger-...`).

**RB-AKS-02 Pod network/DNS failure**
1. Ephemeral container in the failing pod's namespaces: `kubectl debug -it <pod> --image=nicolaka/netshoot --target=<container> -- bash`. Mirror netshoot into ACR and pin its digest.
2. Inside, run `dig kubernetes.default.svc.cluster.local`, `dig @<coredns-svc-ip> <fqdn>`, `curl -v <svc>:<port>`, `mtr -rwc 20 <dest>`, and `ss -tanp`.
3. Cluster side: `kubectl -n kube-system get pods -l k8s-app=kube-dns`, `kubectl -n kube-system logs -l k8s-app=kube-dns`, `kubectl get networkpolicies -A`, and CNI agent health (Azure CNI/Cilium: `kubectl -n kube-system get pods -l k8s-app=cilium`).
4. For egress, check the UDR/Azure Firewall, NAT gateway SNAT port exhaustion (Azure Monitor metrics), and private DNS zone links.

**RB-AKS-03 Packet capture on a Linux node** (Microsoft procedure)
1. `kubectl debug node/<n> -it --image=<acr>/netshoot@sha256:...` → `chroot /host`.
2. Capture with filters: `tcpdump -s 0 -vvv -w /capture.cap host 10.224.0.15 and port 443`. Reproduce, then Ctrl+C.
3. From a second shell: `kubectl cp node-debugger-<n>-xxxxx:/host/capture.cap ./capture.cap`. Microsoft notes that if `chroot /host` was used, you must prefix the source with `/host`.
4. Analyze in Wireshark (install it on AVD, or run `tshark` in the nettools container). Delete captures afterward, because they may contain credentials.

**RB-AKS-04 Pod-level capture:** `kubectl exec <pod> -it -- tcpdump -s 0 -vvv -w /capture.cap`, then `kubectl cp <pod>:/capture.cap capture.cap`. If the image lacks tcpdump, prefer an ephemeral netshoot container over installing packages into a production pod.

**RB-AKS-05 Access (Entra ID):** `az aks get-credentials -g <rg> -n <aks> --overwrite-existing`, then `kubelogin convert-kubeconfig -l azurecli` (or `-l workloadidentity` in CI). Diagnose 401s with `kubectl auth whoami` and 403s with `kubectl auth can-i --list`.

### 5.4 Observability with Grafana OSS 12

- Pin `grafana/grafana-oss:12.4.x` by digest. 12.x is the project pin; Grafana Labs unveiled Grafana 13 at GrafanaCON 2026 on April 21, 2026.
- Grafana 12 introduced Git Sync and a new dashboard schema v2. Both are **experimental in OSS 12**, and Grafana warns the new schema "should not be used in production environments, as it might result in an irreversible loss of data." Use classic file provisioning for OSS 12. Grafana's 13.1 release blog says Git Sync "reached general availability with the release of Grafana 13," so revisit this choice when you upgrade.

```yaml
# provisioning/datasources/datasources.yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    uid: prom
    access: proxy
    url: http://prometheus:9090
    isDefault: true
  - name: Azure Monitor
    type: grafana-azure-monitor-datasource
    uid: azmon
    jsonData:
      azureAuthType: workloadidentity   # or msi / clientsecret
      subscriptionId: ${AZ_SUBSCRIPTION_ID}
---
# provisioning/dashboards/dashboards.yaml
apiVersion: 1
providers:
  - name: devc
    folder: Devcontainers
    type: file
    allowUiUpdates: false
    options: { path: /var/lib/grafana/dashboards, foldersFromFilesStructure: true }
```

For Azure Monitor managed Prometheus, point a Prometheus-type data source at the Azure Monitor workspace query endpoint with Entra auth. The companion tool's `/metrics` is scraped by whichever Prometheus you run.

---

## Part 6 — Network Tools Devcontainer

**Capabilities:** raw sockets (hping3, nmap SYN scans, mtr ICMP, ping) need `NET_RAW`. Setting promiscuous mode, changing interfaces or routes, and `tc netem` need `NET_ADMIN`. tcpdump on the container's own interface needs `NET_RAW` (and `NET_ADMIN` for some interface operations). Never grant `SYS_ADMIN` or `privileged` for these.

**WSL2/Docker host-networking considerations:**
- The container sees only its own bridge network by default. Captures show container traffic, not the AVD host's.
- `--network=host` under Docker Desktop attaches to the Docker VM's namespace, not Windows. Under WSL mirrored mode, a docker-ce engine inside WSL sees mirrored Windows interfaces. Either way, it is not a supported way to capture AVD session traffic. Use Windows `pktmon` for that.
- iperf3 throughput measured from an AVD session through nested virtualization understates real VNet capability. Run authoritative performance tests from a remote Linux VM or an AKS debug pod.

**Authorized use:** scanning, flooding (hping3), and capture are only allowed against assets listed in `/workspaces/<repo>/.nettools/authorized-targets.txt` with a change ticket. The container's `postAttachCommand` prints this rule. Log `nmap`/`hping3` invocations to the shell history file that is shipped to your SIEM. Captured pcaps are sensitive data. Store them in an approved, access-controlled storage account and delete them after the incident closes.

---

## Part 7 — Agentic Sandbox Devcontainers

### 7.1 Copilot cloud agent environment (`copilot-setup-steps.yml`)

GitHub Docs rules:
- The file lives at `.github/workflows/copilot-setup-steps.yml`. It "must contain a single `copilot-setup-steps` job" and "won't trigger unless it's present on your default branch."\[5\]\[31\]
- Only `steps`, `permissions`, `runs-on`, `services`, `snapshot`, and `timeout-minutes` (max **59**) are honored. Other settings are ignored.\[5\]
- If a setup step fails, "Copilot will skip the remaining setup steps and begin working with the current state of its development environment".\[5\] A failing step therefore degrades the agent's environment silently rather than stopping it. Add a final verification step.
- Supported runners: Ubuntu x64 and Windows 64-bit. For ARC, set `runs-on: <scale-set-name>` and **disable the integrated firewall** ("The firewall is not compatible with self-hosted runners"). Self-hosted egress must allow `uploads.github.com`, `user-images.githubusercontent.com`, and `api.business.githubcopilot.com` (Copilot Business) plus standard runner hosts. Proxies are set with `https_proxy`, `http_proxy`, `no_proxy`, `ssl_cert_file`, and `node_extra_ca_certs`.\[5\]
- Since July 2026, Copilot code review can use its own `copilot-code-review.yml` and otherwise falls back to `copilot-setup-steps.yml`.\[5\]\[32\]

Run the agent's steps inside **your** devcontainer image so the agent's environment matches developers' environments:

```yaml
# .github/workflows/copilot-setup-steps.yml
name: "Copilot Setup Steps"
on:
  workflow_dispatch:
  push: { paths: [.github/workflows/copilot-setup-steps.yml] }
  pull_request: { paths: [.github/workflows/copilot-setup-steps.yml] }
jobs:
  copilot-setup-steps:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    permissions: { contents: read, packages: read }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.21.13", cache: true }
      - name: Install org CLI tools from the devcontainer image
        run: |
          echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u "${{ github.actor }}" --password-stdin
          id=$(docker create ghcr.io/contoso/devc/go@sha256:<pin-me>)
          docker cp "$id":/usr/local/bin/devc /usr/local/bin/devc
          docker rm "$id"
      - run: go mod download && devc version
```

### 7.2 Firewall behavior (hosted)

- Hosted agents get a firewall with a recommended allowlist (OS package repos, container registries including ACR, language registries) enabled by default. Custom domain or URL rules are added under repo Settings → Copilot → cloud agent → Custom allowlist. Org owners can enforce firewall settings and block repository custom rules.\[33\]
- **Limitation to design around:** the firewall "only applies to processes started by the agent via its Bash tool. It does not apply to Model Context Protocol (MCP) servers or processes started in configured Copilot setup steps." GitHub also warns that "sophisticated attacks may bypass the firewall."\[34\] Treat MCP servers as unfirewalled egress and grant them least-privilege tokens.

### 7.3 MCP, custom agents, custom instructions

| Artifact | Location | Notes |
|---|---|---|
| Repo instructions | `.github/copilot-instructions.md` | Repo-wide |
| Path instructions | `.github/instructions/*.instructions.md` with `applyTo` globs; optional `excludeAgent` | On GitHub.com supported for cloud agent and code review |
| AGENTS.md | Anywhere; nearest file wins | `CLAUDE.md`/`GEMINI.md` at root also accepted |
| Custom agents | `.github/agents/<name>.agent.md` (repo); `/agents/` in org `.github`/`.github-private` | Markdown + YAML frontmatter; prompt ≤ 30,000 chars |\[35\]\[36\]
| Cloud agent MCP | Repo Settings → Copilot → MCP servers (JSON `mcpServers`, each with `type` and `tools`) | Secrets must be Agents secrets prefixed `COPILOT_MCP_`; tools only (no resources/prompts); no OAuth remote servers; tools run without approval |\[37\]
| VS Code MCP (local) | `.vscode/mcp.json` (`servers` object) or `customizations.vscode.mcp` in devcontainer.json | VS Code writes devcontainer MCP config into the remote mcp.json on create |\[38\]\[39\]

### 7.4 Local agent sandbox pattern

- The container runs as the non-root `agent` user with no sudo. `init: true` is set, and there are no docker.sock or `~/.ssh` mounts.
- `postStartCommand` runs `sudo /usr/local/bin/init-firewall.sh` through a narrowly scoped sudoers entry for that one script. The script sets default-deny iptables egress with an ipset of allowed domains resolved at start: GitHub, `api.business.githubcopilot.com`, your artifact proxy, and the MCP backends. Because the agent user has no other sudo rights, the agent cannot undo the rules.
- `postAttachCommand` verifies the firewall (`curl -m 5 https://example.com` must fail and `curl https://api.github.com` must succeed). The container refuses to continue otherwise.
- Secrets: short-lived tokens only (`gh auth token` scoped fine-grained PAT, or GitHub App installation token minted by a helper), injected via `remoteEnv` from `${localEnv:...}`. Never bake them into images.

### 7.5 Promotion path (local → Actions agentic workflow)

1. Develop the agent/MCP server in the `agent-sandbox` devcontainer with the same egress allowlist it will have in CI.
2. Package the agent as a Go CLI bot in the image; tag and sign it.
3. Add a workflow job with `container: ghcr.io/contoso/devc/agent-sandbox@sha256:...` (or `devcontainers/ci` `runCmd`), least `permissions:`, and an `environment:` with reviewers for write actions.
4. For Copilot cloud agent, register MCP servers in repo settings with `COPILOT_MCP_*` secrets, add `.github/agents/<name>.agent.md`, and mirror tool installs in `copilot-setup-steps.yml`.
5. Gate with rulesets: agent-authored PRs require human review, and `devc lint` must pass.

---

## Part 8 — Runbooks

**RB-01 Onboard a new engineer (target: < 1 hour)**
1. Confirm their AVD host pool: personal with a nested-virt SKU, or Codespaces-only. Assign the Docker Business seat if local.
2. Intune pushes WSL, `.wslconfig`, the corporate CA, VS Code, and the Dev Containers + GitHub Copilot extensions.
3. Engineer runs `wsl --install -d Ubuntu-24.04`, then `gh auth login` (SSO), then `az login`.
4. Clone a starter repo with **Dev Containers: Clone Repository in Container Volume**, or open it in Codespaces.
5. Verify: `devc doctor` (checks engine, WSL mode, proxy, CA, GHCR pull) and `go version` → `go1.21.x`.

**RB-02 Create a new devcontainer from template**
1. `devc scaffold --profile go --image ghcr.io/contoso/devc/go@sha256:<digest> --out .devcontainer/devcontainer.json`
2. `devc lint --policy policy.json .devcontainer/devcontainer.json`
3. Open a PR. CI runs `devcontainers/ci` with `runCmd: make ci`, and CODEOWNERS (platform) approves.

**RB-03 Update base images (monthly + on critical CVE)**
1. Dependabot/Renovate bumps `FROM` digests in `images/*`, and the image pipeline rebuilds, scans, signs, and pushes.
2. Update `policy.json` `approvedImages` with the new digest and `approvedAt` date.
3. `devc inventory --org contoso` then `devc drift`. This opens PRs (or issues) in repos still on old digests. Grafana alerts when `devc_image_age_days > 45`.

**RB-04 Rotate secrets**
1. Prefer eliminating secrets: OIDC for Azure, Databricks, and Spacelift.
2. For remaining secrets (Spacelift API key, MCP backends): create the new one, update the Actions environment secret, Codespaces org secret, and `COPILOT_MCP_*` Agents secret, re-run the smoke workflow, then revoke the old one. Record the change in the change log.
3. Codespaces users must **rebuild or restart** to pick up new values. `containerEnv` values baked at create time need a rebuild.

**RB-05 Troubleshooting matrix**

| Symptom | Likely cause | Fix |
|---|---|---|
| `WSL2 is not supported ... enable virtualization in BIOS` / `HCS_E_HYPERV_NOT_INSTALLED` | Non-nested SKU or Trusted Launch on pre-v5 size | Resize/redeploy host (Standard security); else go remote |
| Docker Desktop "engine starting" forever | WSL distro corrupt, VPN DNS | `wsl --shutdown`; enable `dnsTunneling`; reset Docker Desktop data |
| Rebuild fails in a Feature `install.sh` | Upstream Feature changed / proxy | Pin Feature by digest; bake CA; check `HTTPS_PROXY` in `build.args` |
| Permission denied on bind-mounted files | UID mismatch; Podman rootless | Keep `updateRemoteUserUID: true`; with Podman add `"runArgs": ["--userns=keep-id"]` and `containerUser` |
| `bash\r: No such file or directory` | CRLF checkout on Windows | `.gitattributes`: `* text=auto eol=lf` and `*.sh text eol=lf`; `git config --global core.autocrlf input`; best: clone in volume |
| TLS `x509: certificate signed by unknown authority` | TLS-inspecting proxy | Bake corporate CA (§2.4); never set `GOINSECURE`/`NODE_TLS_REJECT_UNAUTHORIZED=0` |
| `postStartCommand` never ran | Earlier lifecycle script failed | Fix earlier hook; lifecycle chain stops on first failure |
| Codespace opens in recovery mode | Base image not allowed by org policy | Use an approved image/digest |
| Go downloads a new toolchain | `go`/`toolchain` line > local | `GOTOOLCHAIN=local`; keep `go 1.21.x` |

**RB-06 Incident response: compromised devcontainer image or Feature**
1. **Contain:** delete or make private the offending tag/digest in GHCR/ACR. Add the digest to `policy.json` `deniedDigests` so `devc lint` fails everywhere. Add a Codespaces base-image policy exclusion. Disable affected workflows.
2. **Scope:** `devc inventory` plus `devc drift --digest <bad>` lists every repo referencing it. Query the Actions audit log for runs that used it, and Codespaces lists for live codespaces. Check whether the image ran on ARC nodes (node pool identity exposure).
3. **Eradicate:** rebuild from a known-good base. Verify cosign signatures and provenance for all current digests, and rotate every credential the image could have seen: OIDC-federated identities (review sign-ins), `COPILOT_MCP_*`, and Spacelift/Databricks keys.
4. **Recover:** push the fixed digest, run automated PRs, and delete/recreate affected codespaces and ARC runner pods.
5. **Learn:** add a policy rule (e.g., require signatures from a specific identity) and write up the incident.

---

## Part 9 — Examples

Replace every `@sha256:<pin-me>` with a real digest from your pipeline. `devc lint` fails on the placeholder.

**9.1 Go 1.21 dev**

```jsonc
{
  "name": "go-1.21",
  "image": "ghcr.io/contoso/devc/go@sha256:<pin-me>",
  "features": { "ghcr.io/devcontainers/features/github-cli:1": {} },
  "capAdd": ["SYS_PTRACE"],
  "remoteUser": "vscode",
  "containerEnv": { "GOTOOLCHAIN": "local", "GOFLAGS": "-mod=readonly", "HTTPS_PROXY": "${localEnv:HTTPS_PROXY}", "NO_PROXY": "${localEnv:NO_PROXY}" },
  "onCreateCommand": "go mod download",
  "postAttachCommand": "go version",
  "hostRequirements": { "cpus": 4, "memory": "8gb" },
  "customizations": { "vscode": { "extensions": ["golang.go", "GitHub.copilot-chat"], "settings": { "go.toolsManagement.autoUpdate": false } } }
}
```

```dockerfile
# images/go/Dockerfile
FROM mcr.microsoft.com/devcontainers/go:1.21-bookworm@sha256:<pin-me>
COPY certs/contoso-root-ca.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates
ENV GOTOOLCHAIN=local
```

**9.2 AKS admin** (image from §5.2)

```jsonc
{
  "name": "aks-admin",
  "image": "ghcr.io/contoso/devc/aks-admin@sha256:<pin-me>",
  "features": { "ghcr.io/devcontainers/features/azure-cli:1": {} },
  "remoteUser": "vscode",
  "containerEnv": { "KUBECONFIG": "/home/vscode/.kube/config", "AZURE_CONFIG_DIR": "/home/vscode/.azure" },
  "mounts": [{ "source": "aks-admin-azure", "target": "/home/vscode/.azure", "type": "volume" }],
  "postCreateCommand": "kubectl version --client && kubelogin --version",
  "customizations": { "vscode": { "extensions": ["ms-kubernetes-tools.vscode-kubernetes-tools", "ms-azuretools.vscode-azure-github-copilot"] } }
}
```

**9.3 Network tools**

```dockerfile
# images/nettools/Dockerfile
FROM mcr.microsoft.com/devcontainers/base:bookworm@sha256:<pin-me>
RUN apt-get update && apt-get install -y --no-install-recommends \
    iperf3 hping3 tcpdump tshark nmap mtr-tiny dnsutils iproute2 iputils-ping \
    traceroute netcat-openbsd socat curl ethtool conntrack \
 && rm -rf /var/lib/apt/lists/*
```

```jsonc
{
  "name": "nettools",
  "image": "ghcr.io/contoso/devc/nettools@sha256:<pin-me>",
  "capAdd": ["NET_ADMIN", "NET_RAW"],
  "remoteUser": "vscode",
  "postAttachCommand": "echo 'Authorized targets only: see .nettools/authorized-targets.txt'",
  "customizations": { "contoso": { "profile": "nettools" } }
}
```

Running tcpdump/hping3 as non-root requires file capabilities on the binaries (`setcap cap_net_raw,cap_net_admin+eip /usr/bin/tcpdump`) in the Dockerfile, or `sudo` in the container. The container's bounding set (via `capAdd`) must still include them.

**9.4 Agent sandbox**

```jsonc
{
  "name": "agent-sandbox",
  "build": { "dockerfile": "Dockerfile" },
  "init": true,
  "capAdd": ["NET_ADMIN", "NET_RAW"],
  "remoteUser": "agent",
  "containerEnv": { "ALLOWED_DOMAINS": "github.com api.github.com api.business.githubcopilot.com goproxy.contoso.corp" },
  "remoteEnv": { "GH_TOKEN": "${localEnv:AGENT_GH_TOKEN}" },
  "postStartCommand": "sudo /usr/local/bin/init-firewall.sh",
  "postAttachCommand": "/usr/local/bin/verify-firewall.sh",
  "customizations": {
    "contoso": { "profile": "agent-sandbox" },
    "vscode": {
      "extensions": ["GitHub.copilot-chat"],
      "mcp": { "servers": { "devc": { "type": "stdio", "command": "/usr/local/bin/devc", "args": ["mcp"] } } }
    }
  }
}
```

```bash
#!/usr/bin/env bash
# /usr/local/bin/init-firewall.sh  (root-owned, 0755; sudoers: agent ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh)
set -euo pipefail
ipset create allowed hash:ip -exist; ipset flush allowed
for d in ${ALLOWED_DOMAINS}; do for ip in $(dig +short A "$d" | grep -E '^[0-9.]+$'); do ipset add allowed "$ip" -exist; done; done
iptables -F OUTPUT
iptables -A OUTPUT -o lo -j ACCEPT
iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A OUTPUT -m set --match-set allowed dst -j ACCEPT
iptables -P OUTPUT DROP
```

```bash
#!/usr/bin/env bash
# /usr/local/bin/verify-firewall.sh
set -eu
if curl -s -m 5 https://example.com >/dev/null; then echo "FIREWALL NOT ENFORCED" >&2; exit 1; fi
curl -s -m 5 https://api.github.com >/dev/null && echo "egress allowlist OK"
```

Allowing DNS to any resolver leaves a DNS-exfiltration channel. In higher-risk sandboxes, restrict port 53 to the Docker embedded resolver or your corporate resolver IP.

**9.5 IaC (Spacelift / Terraform / OpenTofu)**

```jsonc
{
  "name": "iac",
  "image": "ghcr.io/contoso/devc/iac@sha256:<pin-me>",
  "features": {
    "ghcr.io/devcontainers/features/terraform:1": { "version": "1.9.8" },
    "ghcr.io/devcontainers/features/azure-cli:1": {}
  },
  "remoteUser": "vscode",
  "containerEnv": { "TF_PLUGIN_CACHE_DIR": "/home/vscode/.terraform.d/plugin-cache", "SPACELIFT_API_KEY_ENDPOINT": "https://contoso.app.spacelift.io" },
  "postCreateCommand": "tofu version && spacectl version",
  "customizations": { "vscode": { "extensions": ["hashicorp.terraform", "opentofu.vscode-opentofu"] } }
}
```

Bake OpenTofu, `spacectl`, tflint, and checkov into the image. Locally, `spacectl profile login` handles interactive auth, and CI uses env vars/OIDC.

**9.6 Databricks bundles**

```jsonc
{
  "name": "databricks",
  "image": "ghcr.io/contoso/devc/databricks@sha256:<pin-me>",
  "features": { "ghcr.io/devcontainers/features/python:1": { "version": "3.11" } },
  "remoteUser": "vscode",
  "containerEnv": { "DATABRICKS_HOST": "${localEnv:DATABRICKS_HOST}", "DATABRICKS_AUTH_TYPE": "databricks-cli" },
  "postCreateCommand": "databricks --version && databricks bundle validate || true",
  "customizations": { "vscode": { "extensions": ["databricks.databricks", "ms-python.python"] } }
}
```

Locally, run `databricks auth login --host $DATABRICKS_HOST` (U2M OAuth, token cache in the container volume). In CI, use `DATABRICKS_AUTH_TYPE=github-oidc` (§4.4). The bundle commands are unchanged by the rename to Declarative Automation Bundles.

---

## Part 10 — Companion Go App: `devc`

### 10.1 Design

| Command | Purpose |
|---|---|
| `devc scaffold --profile <p> --image <ref>` | Emit a devcontainer.json from built-in profiles |
| `devc lint [--policy f] <files...>` | Validate configs (JSONC) against org policy; exit 1 on errors |
| `devc inventory --org <o>` | List org repos via GitHub REST (`net/http`), fetch `.devcontainer/devcontainer.json` or `.devcontainer.json`, lint each, write `inventory.json` |
| `devc report --in inventory.json --out report.html` | go-echarts HTML dashboard (compliance, violations by rule, image age) |
| `devc serve --org <o> --addr :8080` | Periodic inventory; serves `/`, `/metrics` (Prometheus text), `/healthz` |
| `devc version` | Build info |

Constraints honored: Go 1.21, stdlib only plus `github.com/go-echarts/go-echarts/v2`. The go-echarts v2 module declares `go 1.18`, so it builds with Go 1.21, and its only requirement (testify) is test-only.\[40\] Set `GOTOOLCHAIN=local` so a newer dependency can never silently require a newer toolchain.

### 10.2 Layout

```
devc/
├── go.mod
├── Makefile
├── Dockerfile
├── policy.example.json
├── cmd/devc/main.go
├── internal/jsonc/jsonc.go
├── internal/jsonc/jsonc_test.go
├── internal/policy/policy.go
├── internal/policy/policy_test.go
├── internal/scaffold/scaffold.go
├── internal/inventory/github.go
├── internal/report/report.go
├── internal/metrics/metrics.go
├── grafana/devc-dashboard.json
└── .github/workflows/release.yml
```

### 10.3 go.mod

```go
module github.com/contoso/devc

go 1.21.13

toolchain go1.21.13

require github.com/go-echarts/go-echarts/v2 v2.6.5
```

Run `go mod tidy` to generate `go.sum`. If a future go-echarts tag raises its `go` directive above 1.21, stay on the last compatible tag.

### 10.4 internal/jsonc/jsonc.go

```go
// Package jsonc converts JSON-with-comments (devcontainer.json) to strict JSON.
package jsonc

// Strip removes // and /* */ comments and trailing commas outside strings.
func Strip(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inStr, esc := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(in) && in[i+1] == '/' {
			for i < len(in) && in[i] != '\n' {
				i++
			}
			if i < len(in) {
				out = append(out, '\n')
			}
			continue
		}
		if c == '/' && i+1 < len(in) && in[i+1] == '*' {
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			i++
			continue
		}
		out = append(out, c)
	}
	return removeTrailingCommas(out)
}

func removeTrailingCommas(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inStr, esc := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inStr {
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
		}
		if c == ',' {
			j := i + 1
			for j < len(in) && (in[j] == ' ' || in[j] == '\t' || in[j] == '\n' || in[j] == '\r') {
				j++
			}
			if j < len(in) && (in[j] == '}' || in[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}
```

### 10.5 internal/jsonc/jsonc_test.go

```go
package jsonc

import (
	"encoding/json"
	"testing"
)

func TestStrip(t *testing.T) {
	in := []byte(`{
  // comment
  "image": "ghcr.io/x/y@sha256:abc", /* block */
  "url": "http://not-a-comment//here",
  "capAdd": ["SYS_PTRACE",],
}`)
	var m map[string]any
	if err := json.Unmarshal(Strip(in), &m); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, Strip(in))
	}
	if m["url"] != "http://not-a-comment//here" {
		t.Fatalf("string mangled: %v", m["url"])
	}
}
```

### 10.6 internal/policy/policy.go

```go
// Package policy lints devcontainer.json documents against org policy.
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/contoso/devc/internal/jsonc"
)

type ApprovedImage struct {
	Digest     string    `json:"digest"`
	ApprovedAt time.Time `json:"approvedAt"`
}

type Policy struct {
	AllowedRegistries []string                 `json:"allowedRegistries"`
	RequireDigest     bool                     `json:"requireDigest"`
	ProfileCaps       map[string][]string      `json:"profileCaps"` // profile -> allowed caps
	DefaultCaps       []string                 `json:"defaultCaps"`
	AllowPrivileged   bool                     `json:"allowPrivileged"`
	ForbidRootUser    bool                     `json:"forbidRootUser"`
	DeniedDigests     []string                 `json:"deniedDigests"`
	ApprovedImages    map[string]ApprovedImage `json:"approvedImages"` // repo (no tag) -> current approved digest
	MaxImageAgeDays   int                      `json:"maxImageAgeDays"`
}

type Config struct {
	Image          string          `json:"image"`
	Build          *struct{}       `json:"build"`
	Features       map[string]any  `json:"features"`
	CapAdd         []string        `json:"capAdd"`
	Privileged     bool            `json:"privileged"`
	SecurityOpt    []string        `json:"securityOpt"`
	RunArgs        []string        `json:"runArgs"`
	Mounts         json.RawMessage `json:"mounts"`
	RemoteUser     string          `json:"remoteUser"`
	ContainerUser  string          `json:"containerUser"`
	Customizations struct {
		Contoso struct {
			Profile string `json:"profile"`
		} `json:"contoso"`
	} `json:"customizations"`
}

type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"` // error | warn
	Message  string `json:"message"`
}

type Result struct {
	Source   string    `json:"source"`
	Image    string    `json:"image"`
	Drifted  bool      `json:"drifted"`
	AgeDays  int       `json:"ageDays"`
	Findings []Finding `json:"findings"`
}

func (r Result) Compliant() bool {
	for _, f := range r.Findings {
		if f.Severity == "error" {
			return false
		}
	}
	return true
}

var digestRe = regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)

func Load(path string) (*Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Policy
	if err := json.Unmarshal(jsonc.Strip(b), &p); err != nil {
		return nil, fmt.Errorf("policy %s: %w", path, err)
	}
	return &p, nil
}

// Parse decodes a JSONC devcontainer document.
func Parse(b []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(jsonc.Strip(b), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func splitRepo(ref string) (repo, digest string) {
	if i := strings.Index(ref, "@"); i >= 0 {
		repo, digest = ref[:i], ref[i+1:]
	} else {
		repo = ref
	}
	// strip tag (a ':' after the last '/')
	if slash := strings.LastIndex(repo, "/"); slash >= 0 {
		if colon := strings.LastIndex(repo[slash:], ":"); colon >= 0 {
			repo = repo[:slash+colon]
		}
	}
	return repo, digest
}

// Check evaluates a config; now is injected for testability.
func (p *Policy) Check(source string, c *Config, now time.Time) Result {
	r := Result{Source: source, Image: c.Image}
	add := func(rule, sev, format string, a ...any) {
		r.Findings = append(r.Findings, Finding{rule, sev, fmt.Sprintf(format, a...)})
	}
	profile := c.Customizations.Contoso.Profile

	if c.Image != "" {
		okReg := len(p.AllowedRegistries) == 0
		for _, reg := range p.AllowedRegistries {
			if strings.HasPrefix(c.Image, reg+"/") {
				okReg = true
			}
		}
		if !okReg {
			add("allowed-registry", "error", "image %q not from an allowed registry", c.Image)
		}
		if p.RequireDigest && !digestRe.MatchString(c.Image) {
			add("image-digest", "error", "image %q must be pinned by @sha256 digest", c.Image)
		}
		repo, dg := splitRepo(c.Image)
		for _, d := range p.DeniedDigests {
			if dg == d {
				add("denied-digest", "error", "image digest %s is denied (incident)", d)
			}
		}
		if ai, ok := p.ApprovedImages[repo]; ok {
			if dg != "" && dg != ai.Digest {
				r.Drifted = true
				add("image-drift", "warn", "digest differs from approved %s", ai.Digest)
			}
			r.AgeDays = int(now.Sub(ai.ApprovedAt).Hours() / 24)
			if r.Drifted {
				r.AgeDays = -1 // unknown age of an unapproved digest
			}
			if p.MaxImageAgeDays > 0 && r.AgeDays > p.MaxImageAgeDays {
				add("image-age", "warn", "approved image is %d days old", r.AgeDays)
			}
		}
	} else if c.Build == nil {
		add("image-source", "warn", "no image or build defined (compose?) — lint compose separately")
	}

	allowed := map[string]bool{}
	for _, cp := range p.DefaultCaps {
		allowed[strings.ToUpper(cp)] = true
	}
	for _, cp := range p.ProfileCaps[profile] {
		allowed[strings.ToUpper(cp)] = true
	}
	for _, cp := range c.CapAdd {
		if !allowed[strings.ToUpper(strings.TrimPrefix(cp, "CAP_"))] {
			add("capability", "error", "capability %s not allowed for profile %q", cp, profile)
		}
	}
	if c.Privileged && !p.AllowPrivileged {
		add("privileged", "error", "privileged containers are forbidden")
	}
	for _, s := range c.SecurityOpt {
		if strings.Contains(s, "unconfined") {
			add("security-opt", "error", "securityOpt %q disables confinement", s)
		}
	}
	for i, a := range c.RunArgs {
		switch {
		case a == "--privileged":
			add("runargs", "error", "runArgs --privileged is forbidden")
		case strings.HasPrefix(a, "--cap-add"):
			add("runargs", "error", "use capAdd instead of runArgs %q so policy can evaluate it", a)
		case a == "--network=host" || a == "--net=host" || (a == "--network" && i+1 < len(c.RunArgs) && c.RunArgs[i+1] == "host"):
			add("runargs", "error", "host networking requires a waiver")
		case a == "--pid=host":
			add("runargs", "error", "host PID namespace is forbidden")
		}
	}
	if strings.Contains(string(c.Mounts), "docker.sock") {
		add("mounts", "error", "mounting the Docker socket is forbidden")
	}
	if strings.Contains(string(c.Mounts), ".ssh") && profile == "agent-sandbox" {
		add("mounts", "error", "agent sandboxes must not mount ~/.ssh")
	}
	if p.ForbidRootUser && (c.RemoteUser == "root" || c.ContainerUser == "root") {
		add("root-user", "error", "remoteUser/containerUser must not be root")
	}
	for id := range c.Features {
		if !strings.Contains(id[strings.LastIndex(id, "/")+1:], ":") && !strings.Contains(id, "@sha256:") {
			add("feature-pin", "warn", "feature %s is unpinned (use :major or @sha256)", id)
		}
	}
	return r
}
```

### 10.7 internal/policy/policy_test.go

```go
package policy

import (
	"testing"
	"time"
)

func testPolicy() *Policy {
	return &Policy{
		AllowedRegistries: []string{"ghcr.io/contoso", "contoso.azurecr.io"},
		RequireDigest:     true,
		DefaultCaps:       []string{"SYS_PTRACE"},
		ProfileCaps:       map[string][]string{"nettools": {"NET_ADMIN", "NET_RAW"}},
		ForbidRootUser:    true,
		ApprovedImages: map[string]ApprovedImage{
			"ghcr.io/contoso/devc/go": {Digest: "sha256:" + repeat("a"), ApprovedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		},
		MaxImageAgeDays: 45,
	}
}

func repeat(s string) string {
	out := ""
	for i := 0; i < 64; i++ {
		out += s
	}
	return out
}

func TestCompliantGo(t *testing.T) {
	c, err := Parse([]byte(`{"image":"ghcr.io/contoso/devc/go@sha256:` + repeat("a") + `","capAdd":["SYS_PTRACE"],"remoteUser":"vscode"}`))
	if err != nil {
		t.Fatal(err)
	}
	r := testPolicy().Check("x", c, time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC))
	if !r.Compliant() || r.Drifted || r.AgeDays != 26 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestViolations(t *testing.T) {
	c, _ := Parse([]byte(`{
	  "image": "docker.io/library/ubuntu:latest", // tag, wrong registry
	  "capAdd": ["NET_ADMIN"],
	  "runArgs": ["--network", "host"],
	  "mounts": ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind"],
	  "remoteUser": "root",
	}`))
	r := testPolicy().Check("y", c, time.Now())
	want := map[string]bool{"allowed-registry": true, "image-digest": true, "capability": true, "runargs": true, "mounts": true, "root-user": true}
	for _, f := range r.Findings {
		delete(want, f.Rule)
	}
	if len(want) != 0 {
		t.Fatalf("missing findings: %v (got %+v)", want, r.Findings)
	}
}

func TestNettoolsCaps(t *testing.T) {
	c, _ := Parse([]byte(`{"image":"ghcr.io/contoso/devc/nettools@sha256:` + repeat("b") + `","capAdd":["NET_ADMIN","NET_RAW"],"customizations":{"contoso":{"profile":"nettools"}}}`))
	if r := testPolicy().Check("z", c, time.Now()); !r.Compliant() {
		t.Fatalf("nettools should be compliant: %+v", r.Findings)
	}
}
```

### 10.8 internal/scaffold/scaffold.go

```go
// Package scaffold renders devcontainer.json from built-in profiles.
package scaffold

import (
	"encoding/json"
	"fmt"
	"sort"
)

var profiles = map[string]map[string]any{
	"go": {
		"capAdd": []string{"SYS_PTRACE"}, "remoteUser": "vscode",
		"containerEnv":    map[string]string{"GOTOOLCHAIN": "local", "GOFLAGS": "-mod=readonly"},
		"onCreateCommand": "go mod download",
		"customizations":  map[string]any{"vscode": map[string]any{"extensions": []string{"golang.go"}}, "contoso": map[string]string{"profile": "go"}},
	},
	"aks-admin": {
		"remoteUser": "vscode",
		"features":   map[string]any{"ghcr.io/devcontainers/features/azure-cli:1": map[string]any{}},
		"customizations": map[string]any{"vscode": map[string]any{"extensions": []string{"ms-kubernetes-tools.vscode-kubernetes-tools"}}, "contoso": map[string]string{"profile": "aks-admin"}},
	},
	"nettools": {
		"capAdd": []string{"NET_ADMIN", "NET_RAW"}, "remoteUser": "vscode",
		"customizations": map[string]any{"contoso": map[string]string{"profile": "nettools"}},
	},
	"agent-sandbox": {
		"init": true, "capAdd": []string{"NET_ADMIN", "NET_RAW"}, "remoteUser": "agent",
		"postStartCommand":  "sudo /usr/local/bin/init-firewall.sh",
		"postAttachCommand": "/usr/local/bin/verify-firewall.sh",
		"customizations":    map[string]any{"contoso": map[string]string{"profile": "agent-sandbox"}},
	},
	"iac":        {"remoteUser": "vscode", "customizations": map[string]any{"contoso": map[string]string{"profile": "iac"}}},
	"databricks": {"remoteUser": "vscode", "customizations": map[string]any{"contoso": map[string]string{"profile": "databricks"}}},
}

func Profiles() []string {
	out := make([]string, 0, len(profiles))
	for k := range profiles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func Render(profile, name, image string) ([]byte, error) {
	base, ok := profiles[profile]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q (have %v)", profile, Profiles())
	}
	doc := map[string]any{"name": name, "image": image}
	for k, v := range base {
		doc[k] = v
	}
	return json.MarshalIndent(doc, "", "  ")
}
```

### 10.9 internal/inventory/github.go

```go
// Package inventory discovers devcontainer configs across an org via the GitHub REST API.
package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

type Client struct {
	BaseURL string // https://api.github.com or https://api.<subdomain>.ghe.com
	Token   string
	HTTP    *http.Client
}

type Repo struct {
	FullName string `json:"full_name"`
	Archived bool   `json:"archived"`
}

type File struct {
	Repo string
	Path string
	Body []byte
}

var nextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func (c *Client) do(ctx context.Context, url, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for attempt := 0; ; attempt++ {
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		if (resp.StatusCode == 403 || resp.StatusCode == 429) && attempt < 3 {
			resp.Body.Close()
			time.Sleep(time.Duration(attempt+1) * 10 * time.Second) // secondary rate limit backoff
			continue
		}
		return resp, nil
	}
}

func (c *Client) ListRepos(ctx context.Context, org string) ([]Repo, error) {
	var all []Repo
	url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100&type=all", c.BaseURL, org)
	for url != "" {
		resp, err := c.do(ctx, url, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return nil, fmt.Errorf("list repos: %s: %s", resp.Status, b)
		}
		var page []Repo
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		url = ""
		if m := nextRe.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			url = m[1]
		}
	}
	return all, nil
}

var candidatePaths = []string{".devcontainer/devcontainer.json", ".devcontainer.json"}

// FetchConfigs returns the first devcontainer config found per repo (default branch).
func (c *Client) FetchConfigs(ctx context.Context, repos []Repo) ([]File, error) {
	var files []File
	for _, r := range repos {
		if r.Archived {
			continue
		}
		for _, p := range candidatePaths {
			resp, err := c.do(ctx, fmt.Sprintf("%s/repos/%s/contents/%s", c.BaseURL, r.FullName, p), "application/vnd.github.raw+json")
			if err != nil {
				return files, err
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				files = append(files, File{Repo: r.FullName, Path: p, Body: body})
				break
			}
		}
	}
	return files, nil
}
```

Profile folders (`.devcontainer/<name>/devcontainer.json`) can be added by listing `/repos/{r}/contents/.devcontainer`. The token needs `contents:read` and `metadata:read` (fine-grained PAT or GitHub App installation token).

### 10.10 internal/report/report.go (go-echarts)

```go
// Package report renders an HTML dashboard with go-echarts.
package report

import (
	"io"
	"sort"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"

	"github.com/contoso/devc/internal/policy"
)

func Render(w io.Writer, results []policy.Result) error {
	compliant, non := 0, 0
	byRule := map[string]int{}
	var names []string
	var ages []opts.BarData
	for _, r := range results {
		if r.Compliant() {
			compliant++
		} else {
			non++
		}
		for _, f := range r.Findings {
			byRule[f.Rule]++
		}
		names = append(names, r.Source)
		ages = append(ages, opts.BarData{Value: r.AgeDays})
	}

	pie := charts.NewPie()
	pie.SetGlobalOptions(charts.WithTitleOpts(opts.Title{Title: "Policy compliance"}))
	pie.AddSeries("repos", []opts.PieData{{Name: "compliant", Value: compliant}, {Name: "non-compliant", Value: non}})

	rules := make([]string, 0, len(byRule))
	for k := range byRule {
		rules = append(rules, k)
	}
	sort.Strings(rules)
	var ruleData []opts.BarData
	for _, k := range rules {
		ruleData = append(ruleData, opts.BarData{Value: byRule[k]})
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{Title: "Findings by rule"}))
	bar.SetXAxis(rules).AddSeries("findings", ruleData)

	age := charts.NewBar()
	age.SetGlobalOptions(charts.WithTitleOpts(opts.Title{Title: "Approved image age (days, -1 = drifted)"}))
	age.SetXAxis(names).AddSeries("age", ages)

	page := components.NewPage()
	page.PageTitle = "devc dashboard"
	page.AddCharts(pie, bar, age)
	return page.Render(w)
}
```

### 10.11 internal/metrics/metrics.go

```go
// Package metrics writes Prometheus text exposition format using only the stdlib.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/contoso/devc/internal/policy"
)

type Store struct {
	mu      sync.RWMutex
	results []policy.Result
	last    time.Time
}

func (s *Store) Set(r []policy.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results, s.last = r, time.Now()
}

func (s *Store) Get() []policy.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]policy.Result(nil), s.results...)
}

func esc(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}

func (s *Store) Write(w io.Writer) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fmt.Fprintln(w, "# HELP devc_configs_total Devcontainer configs discovered.")
	fmt.Fprintln(w, "# TYPE devc_configs_total gauge")
	fmt.Fprintf(w, "devc_configs_total %d\n", len(s.results))
	fmt.Fprintln(w, "# HELP devc_config_compliant 1 if the repo config passes all error rules.")
	fmt.Fprintln(w, "# TYPE devc_config_compliant gauge")
	rules := map[string]int{}
	for _, r := range s.results {
		v := 0
		if r.Compliant() {
			v = 1
		}
		fmt.Fprintf(w, "devc_config_compliant{repo=\"%s\"} %d\n", esc(r.Source), v)
		for _, f := range r.Findings {
			rules[f.Rule+"|"+f.Severity]++
		}
	}
	fmt.Fprintln(w, "# HELP devc_image_age_days Age of the approved image digest in use (-1 drifted).")
	fmt.Fprintln(w, "# TYPE devc_image_age_days gauge")
	for _, r := range s.results {
		fmt.Fprintf(w, "devc_image_age_days{repo=\"%s\",image=\"%s\"} %d\n", esc(r.Source), esc(r.Image), r.AgeDays)
	}
	fmt.Fprintln(w, "# HELP devc_findings_total Findings by rule and severity.")
	fmt.Fprintln(w, "# TYPE devc_findings_total gauge")
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := strings.SplitN(k, "|", 2)
		fmt.Fprintf(w, "devc_findings_total{rule=\"%s\",severity=\"%s\"} %d\n", p[0], p[1], rules[k])
	}
	fmt.Fprintln(w, "# TYPE devc_last_scan_timestamp_seconds gauge")
	fmt.Fprintf(w, "devc_last_scan_timestamp_seconds %d\n", s.last.Unix())
}
```

### 10.12 cmd/devc/main.go

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/contoso/devc/internal/inventory"
	"github.com/contoso/devc/internal/metrics"
	"github.com/contoso/devc/internal/policy"
	"github.com/contoso/devc/internal/report"
	"github.com/contoso/devc/internal/scaffold"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "scaffold":
		err = cmdScaffold(os.Args[2:])
	case "lint":
		err = cmdLint(os.Args[2:])
	case "inventory":
		err = cmdInventory(os.Args[2:])
	case "report":
		err = cmdReport(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "version":
		fmt.Println("devc", version)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: devc scaffold|lint|inventory|report|serve|version [flags]")
	os.Exit(2)
}

func cmdScaffold(args []string) error {
	fs := flag.NewFlagSet("scaffold", flag.ExitOnError)
	prof := fs.String("profile", "go", "profile")
	img := fs.String("image", "", "image ref pinned by digest")
	name := fs.String("name", "", "display name (default: profile)")
	out := fs.String("out", "", "output file (default stdout)")
	fs.Parse(args)
	if *name == "" {
		*name = *prof
	}
	b, err := scaffold.Render(*prof, *name, *img)
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o644)
}

func cmdLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	pf := fs.String("policy", "policy.json", "policy file")
	fs.Parse(args)
	p, err := policy.Load(*pf)
	if err != nil {
		return err
	}
	failed := false
	for _, f := range fs.Args() {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		c, err := policy.Parse(b)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		r := p.Check(f, c, time.Now())
		for _, fd := range r.Findings {
			fmt.Printf("%s: %s [%s] %s\n", f, fd.Severity, fd.Rule, fd.Message)
		}
		if !r.Compliant() {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("policy violations found")
	}
	return nil
}

func scan(ctx context.Context, org, pf string) ([]policy.Result, error) {
	p, err := policy.Load(pf)
	if err != nil {
		return nil, err
	}
	base := os.Getenv("GITHUB_API_URL")
	if base == "" {
		base = "https://api.github.com"
	}
	cl := &inventory.Client{BaseURL: base, Token: os.Getenv("GITHUB_TOKEN"), HTTP: &http.Client{Timeout: 30 * time.Second}}
	repos, err := cl.ListRepos(ctx, org)
	if err != nil {
		return nil, err
	}
	files, err := cl.FetchConfigs(ctx, repos)
	if err != nil {
		return nil, err
	}
	var results []policy.Result
	for _, f := range files {
		src := f.Repo + ":" + f.Path
		c, err := policy.Parse(f.Body)
		if err != nil {
			results = append(results, policy.Result{Source: src, Findings: []policy.Finding{{Rule: "parse", Severity: "error", Message: err.Error()}}})
			continue
		}
		results = append(results, p.Check(src, c, time.Now()))
	}
	return results, nil
}

func cmdInventory(args []string) error {
	fs := flag.NewFlagSet("inventory", flag.ExitOnError)
	org := fs.String("org", "", "GitHub org")
	pf := fs.String("policy", "policy.json", "policy file")
	out := fs.String("out", "inventory.json", "output")
	fs.Parse(args)
	res, err := scan(context.Background(), *org, *pf)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	return os.WriteFile(*out, b, 0o644)
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	in := fs.String("in", "inventory.json", "inventory file")
	out := fs.String("out", "report.html", "html output")
	fs.Parse(args)
	b, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var res []policy.Result
	if err := json.Unmarshal(b, &res); err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	return report.Render(f, res)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	org := fs.String("org", "", "GitHub org")
	pf := fs.String("policy", "policy.json", "policy file")
	addr := fs.String("addr", ":8080", "listen address")
	every := fs.Duration("interval", 30*time.Minute, "scan interval")
	fs.Parse(args)

	store := &metrics.Store{}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		for {
			if res, err := scan(ctx, *org, *pf); err != nil {
				slog.Error("scan failed", "err", err)
			} else {
				store.Set(res)
				slog.Info("scan complete", "configs", len(res))
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(*every):
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		store.Write(w)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := report.Render(w, store.Get()); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); srv.Shutdown(context.Background()) }()
	slog.Info("listening", "addr", *addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
```

(`log/slog` entered the standard library in Go 1.21, so it is allowed here.) The `/` dashboard pulls ECharts JavaScript from go-echarts' default CDN host. Behind a TLS-inspecting proxy or in air-gapped environments, set a local asset host on the page (`page.AssetsHost`) and serve the echarts JS file yourself.

### 10.13 policy.example.json

```json
{
  "allowedRegistries": ["ghcr.io/contoso", "contoso.azurecr.io", "mcr.microsoft.com/devcontainers"],
  "requireDigest": true,
  "defaultCaps": ["SYS_PTRACE"],
  "profileCaps": { "nettools": ["NET_ADMIN", "NET_RAW"], "agent-sandbox": ["NET_ADMIN", "NET_RAW"] },
  "allowPrivileged": false,
  "forbidRootUser": true,
  "deniedDigests": [],
  "approvedImages": {
    "ghcr.io/contoso/devc/go": { "digest": "sha256:<digest>", "approvedAt": "2026-09-01T00:00:00Z" }
  },
  "maxImageAgeDays": 45
}
```

### 10.14 Makefile

```makefile
GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.version=$(VERSION)
export GOTOOLCHAIN=local
export CGO_ENABLED=0

.PHONY: all test vet build cross lint-self clean
all: vet test build
vet:   ; $(GO) vet ./...
test:  ; $(GO) test -race -count=1 ./...
build: ; $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/devc ./cmd/devc
cross:
	GOOS=linux   GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/devc-linux-amd64 ./cmd/devc
	GOOS=linux   GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/devc-linux-arm64 ./cmd/devc
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/devc-windows-amd64.exe ./cmd/devc
lint-self: build ; ./bin/devc lint --policy policy.example.json ../.devcontainer/devcontainer.json
clean: ; rm -rf bin dist
```

`-race` needs cgo. Run `make test CGO_ENABLED=1` where a C toolchain exists (the Go devcontainer has one), or drop `-race` on minimal runners.

### 10.15 Dockerfile

```dockerfile
FROM golang:1.21.13-bookworm@sha256:<pin-me> AS build
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/devc ./cmd/devc

FROM gcr.io/distroless/static-debian12:nonroot@sha256:<pin-me>
COPY --from=build /out/devc /usr/local/bin/devc
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/devc"]
CMD ["serve", "--addr", ":8080", "--policy", "/etc/devc/policy.json"]
```

Deploy on AKS as a Deployment with a ConfigMap-mounted `policy.json`, the token from a Key Vault-backed secret (Secrets Store CSI driver), and a `ServiceMonitor`/`PodMonitor` for scraping. On AKS 1.37+, Azure Monitor managed Prometheus uses namespace-scoped secret access for monitors with basicAuth.\[4\]

### 10.16 Release workflow

```yaml
# .github/workflows/release.yml
name: release
on:
  push: { tags: ["v*"] }
  pull_request:
permissions: { contents: write, packages: write, id-token: write, attestations: write }
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.21.13", cache: true }
      - run: make vet test
        env: { CGO_ENABLED: "1" }
  release:
    if: startsWith(github.ref, 'refs/tags/v')
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.21.13" }
      - run: make cross VERSION=${GITHUB_REF_NAME}
      - uses: actions/attest-build-provenance@v2
        with: { subject-path: "dist/*" }
      - run: gh release create "$GITHUB_REF_NAME" dist/* --generate-notes
        env: { GH_TOKEN: "${{ github.token }}" }
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ghcr.io, username: "${{ github.actor }}", password: "${{ secrets.GITHUB_TOKEN }}" }
      - id: push
        uses: docker/build-push-action@v6
        with:
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.ref_name }}
          build-args: VERSION=${{ github.ref_name }}
          sbom: true
          provenance: mode=max
      - uses: sigstore/cosign-installer@v3
      - run: cosign sign --yes ghcr.io/${{ github.repository }}@${{ steps.push.outputs.digest }}
```

### 10.17 Sample Grafana 12 dashboard (classic schema, file-provisioned)

```json
{
  "uid": "devc-overview",
  "title": "Devcontainer Governance",
  "schemaVersion": 41,
  "time": { "from": "now-7d", "to": "now" },
  "templating": { "list": [] },
  "panels": [
    { "id": 1, "type": "stat", "title": "Compliant configs (%)",
      "gridPos": { "x": 0, "y": 0, "w": 6, "h": 5 },
      "datasource": { "type": "prometheus", "uid": "prom" },
      "fieldConfig": { "defaults": { "unit": "percent", "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "yellow", "value": 80 }, { "color": "green", "value": 95 } ] } } },
      "targets": [ { "refId": "A", "expr": "100 * sum(devc_config_compliant) / sum(devc_configs_total)" } ] },
    { "id": 2, "type": "barchart", "title": "Findings by rule",
      "gridPos": { "x": 6, "y": 0, "w": 10, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "prom" },
      "targets": [ { "refId": "A", "expr": "sum by (rule) (devc_findings_total)", "instant": true, "format": "table" } ] },
    { "id": 3, "type": "table", "title": "Stale or drifted images",
      "gridPos": { "x": 0, "y": 8, "w": 16, "h": 8 },
      "datasource": { "type": "prometheus", "uid": "prom" },
      "targets": [ { "refId": "A", "expr": "devc_image_age_days > 45 or devc_image_age_days == -1", "instant": true, "format": "table" } ] },
    { "id": 4, "type": "stat", "title": "Last scan age",
      "gridPos": { "x": 16, "y": 0, "w": 4, "h": 5 },
      "datasource": { "type": "prometheus", "uid": "prom" },
      "fieldConfig": { "defaults": { "unit": "s" } },
      "targets": [ { "refId": "A", "expr": "time() - devc_last_scan_timestamp_seconds" } ] }
  ]
}
```

Add a Grafana-managed alert rule on `time() - devc_last_scan_timestamp_seconds > 7200` (scanner stalled) and on `sum(devc_findings_total{severity="error"}) > 0` for repos with required status checks bypassed.

---

## Recommendations (prioritized)

1. **This week:** publish `policy.json` and run `devc lint` as a required check on `.devcontainer/**` and `copilot-setup-steps.yml`. Turn on the Codespaces base-image policy.
2. **This month:** stand up the image pipeline (§4.1) for the six profiles with Trivy, SBOM, provenance and cosign. Move all repos to digest-pinned org images and enable both Dependabot ecosystems.
3. **AVD:** move container-heavy engineers to personal host pools on nested-virt v5+ sizes with Standard security, and pilot Dasv7/Turin hosts before rollout. Everyone else defaults to Codespaces.
4. **Secrets:** replace Azure, Databricks, and Spacelift secrets with OIDC federation, and restrict `COPILOT_MCP_*` secrets to least-privilege tokens.
5. **Agents:** run Copilot cloud agent on hosted runners with the firewall (org-enforced) unless you need VNet access. On ARC, give agent runners a dedicated, ephemeral, egress-restricted node pool.
6. **Re-baseline versions by Q4 2026:** move clusters off 1.31 LTS before November 2026 and adopt a kubectl matching the cluster ±1. Move Go to a supported release (keep `go 1.21` in go.mod only if you need it as a compatibility floor). Plan Grafana 12 → 13.

## Caveats

- **Version pins versus support windows:** AKS 1.30 LTS ended July 2026, and kubectl 1.30 is out of skew for standard-support clusters (1.34–1.37). Go 1.21 is six feature releases behind current Go 1.27, which the official Go blog dates to 19 August 2026; go1.27.1 followed on 2026-09-01. Grafana's current release line is 13.2.x.\[41\] These guides honor the constraints but should not be read as endorsing them long term.
- **Tool versions in Dockerfiles** (helm, kubelogin, k9s, flux, argocd, Terraform, go-echarts v2.6.5) are illustrative pins. Verify each release, checksum, and `go` directive compatibility when you build.
- **Docker Desktop on AMD Turin AVD hosts:** the crash report is a single user issue, not a vendor advisory.
- **Copilot docs are mid-reorganization:** settings paths for the firewall and MCP differ between older ("coding agent") and newer ("cloud agent") pages.\[33\]\[34\] Confirm click paths against your tenant. Custom-agent file-size limits and AGENTS.md behavior come from current GitHub Docs but change frequently.
- **Grafana 12 Git Sync and dashboard schema v2 are experimental in OSS.** This guide deliberately uses classic file provisioning. Grafana says Git Sync reached general availability with Grafana 13, so revisit this when you upgrade.
- **The Databricks OIDC-inside-container pattern** depends on forwarding `ACTIONS_ID_TOKEN_REQUEST_*` into the container. Validate it in a non-prod environment first, or run `databricks bundle deploy` on the runner with the same pinned CLI version as the image.
- **The agent firewall script** is a defense-in-depth control, not a guarantee. DNS remains an exfiltration channel unless you restrict resolvers, and GitHub itself notes that its firewall does not cover MCP servers or setup steps.\[33\]

## Sources

1. [Dev Container metadata reference](https://containers.dev/implementors/json_reference/)
2. [General Availability of Dependabot Integration](https://containers.dev/guide/dependabot)
3. [Dependabot Version Updates Support devcontainers - GitHub Changelog](https://github.blog/changelog/2024-01-24-dependabot-version-updates-support-devcontainers/)
4. [Supported Kubernetes Versions in Azure Kubernetes Service (AKS) - Azure Kubernetes Service | Microsoft Learn](https://learn.microsoft.com/en-us/azure/aks/supported-kubernetes-versions)
5. [Configure the development environment - GitHub Docs](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/customize-the-agent-environment)
6. [Pular para o conteúdo principal](https://docs.databricks.com/gcp/pt/release-notes/dev-tools/bundles)
7. [docs.databricks.com](https://docs.databricks.com/aws/en/dev-tools/bundles/faqs)
8. [GitHub - spacelift-io/spacectl: Spacelift client and CLI · GitHub](https://github.com/spacelift-io/spacectl)
9. [setup-spacectl/README.md at main · spacelift-io/setup-spacectl](https://github.com/spacelift-io/setup-spacectl/blob/main/README.md)
10. [Dev Container JSON Reference | devcontainers/spec | DeepWiki](https://deepwiki.com/devcontainers/spec/2.1-dev-container-json-reference)
11. [Dependabot supported ecosystems and repositories - GitHub Docs](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)
12. [ci/docs/github-action.md at main · devcontainers/ci](https://github.com/devcontainers/ci/blob/main/docs/github-action.md)
13. [Forward Compatibility and Toolchain Management in Go 1.21 - The Go Programming Language](https://go.dev/blog/toolchain)
14. [How to Enable Nested Virtualization on an Azure Virtual Machine](https://oneuptime.com/blog/post/2026-02-16-how-to-enable-nested-virtualization-on-an-azure-virtual-machine/view)
15. [Please provide virtualization enabled azure vm list - Microsoft Q&A](https://learn.microsoft.com/en-us/answers/questions/1335188/please-provide-virtualization-enabled-azure-vm-lis)
16. [Run Docker Desktop for Windows in a VM or VDI environment](https://docs.docker.com/desktop/setup/vm-vdi.md)
17. [github.com](https://github.com/docker/docs/pull/22696/files)
18. [Docker FAQs | Docker](https://docker.p2hp.com/pricing/faq/index.html)
19. [Docker Desktop WSL2 Engine Startup Causes Kernel Crash on Azure Nested Virtualization (4+ vCPUs, AMD EPYC) · Issue #137 · docker/desktop-feedback](https://github.com/docker/desktop-feedback/issues/137)
20. [Unable to setup nestedVirtualization in Windows VM (Standard\_D4s\_v3) - Microsoft Q&A](https://learn.microsoft.com/en-us/answers/questions/2143749/unable-to-setup-nestedvirtualization-in-windows-vm)
21. [How do I know what size Azure VM supports nested virtualization? - Microsoft Q&A](https://learn.microsoft.com/en-us/answers/questions/813416/how-do-i-know-what-size-azure-vm-supports-nested-v)
22. [Set up Windows Subsystem for Linux for your company | Microsoft Learn](https://learn.microsoft.com/en-us/windows/wsl/enterprise)
23. [Networking | MicrosoftDocs/WSL | DeepWiki](https://deepwiki.com/MicrosoftDocs/WSL/5-networking)
24. [Managing GitHub Codespaces for your organization](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization)
25. [Codespaces now offers an organizational policy to restrict container images - GitHub Changelog](https://github.blog/changelog/2022-10-20-codespaces-now-offers-an-organizational-policy-to-restrict-container-images/)
26. [Restricting the base image for codespaces - GitHub Enterprise Cloud Docs](https://docs.github.com/en/enterprise-cloud@latest/codespaces/managing-codespaces-for-your-organization/restricting-the-base-image-for-codespaces)
27. [CI/CD for Databricks Apps with GitHub Actions | Databricks on AWS](https://docs.databricks.com/aws/en/dev-tools/databricks-apps/cicd-github-actions)
28. [Enable workload identity federation for GitHub Actions | Databricks on AWS](https://docs.databricks.com/aws/en/dev-tools/auth/provider-github)
29. [Customizing the development environment for GitHub Copilot cloud agent - GitHub Enterprise Cloud Docs](https://docs.github.com/en/enterprise-cloud@latest/copilot/how-tos/agents/copilot-coding-agent/customizing-the-development-environment-for-copilot-coding-agent)
30. [Skills](https://skills.lc/pubgo/agentdocs/pubgo-agentdocs-skills-customizing-copilot-cloud-agents-environment-skill-md)
31. [About customizing Copilot coding agent's development environment](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/customize-the-agent-environment)
32. [Copilot code review: Customization and configurability improvements - GitHub Changelog](https://github.blog/changelog/2026-07-17-copilot-code-review-customization-and-configurability-improvements/)
33. [Customizing or disabling the firewall for GitHub Copilot cloud agent - GitHub Docs](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/customize-the-agent-firewall)
34. [Customizing or disabling the firewall for GitHub Copilot coding agent - GitHub Enterprise Cloud Docs](https://docs.github.com/en/enterprise-cloud@latest/copilot/how-tos/use-copilot-agents/coding-agent/customize-the-agent-firewall)
35. [Custom agents configuration - GitHub Docs](https://docs.github.com/en/copilot/reference/custom-agents-configuration)
36. [About custom agents - GitHub Docs](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/about-custom-agents)
37. [Configure MCP servers for your repository - GitHub Docs](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/configure-mcp-servers)
38. [Add and manage MCP servers in VS Code](https://code.visualstudio.com/docs/agent-customization/mcp-servers)
39. [MCP configuration reference](https://code.visualstudio.com/docs/agents/reference/mcp-configuration)
40. [go-echarts/go.mod at master · go-echarts/go-echarts](https://github.com/go-echarts/go-echarts/blob/master/go.mod)
41. [Releases · grafana/grafana](https://github.com/grafana/grafana/releases)
