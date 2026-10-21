# GitHub Codespaces on GitHub Enterprise Cloud: Production Guide for CoolGitEnterprise (October 2026)

You can run GitHub Codespaces at enterprise scale on GHEC, but only part of it can be managed as code. Secrets, access lists and codespace lifecycle can be driven through the REST API and Terraform. Codespaces policies (machine types, idle timeout, retention, base images, port visibility, maximum codespaces per user) can only be set in the web UI. Your governance-as-code platform should therefore manage everything that has an API, and watch the audit log for policy changes it cannot apply itself.

## TL;DR

- **Prices have not changed, but GPUs are gone.** Compute is $0.18/hour for 2-core, $0.36 for 4-core, $0.72 for 8-core, $1.44 for 16-core and $2.88 for 32-core. Storage is $0.07 per GB-month. Organizations and enterprises get no included quota. GitHub retired the GPU machine type on August 29, 2025. Expect roughly $44–49 per developer per month at 4–6 active hours a day. Idle timeout and retention policies affect the bill more than machine size does.
- **There is no API for Codespaces policies.** The REST API covers access control, listing, stopping and deleting org codespaces, secrets, machines and billing usage. Terraform (`integrations/github` 6.13.0) covers Codespaces secrets only. Policy constraints exist only in the UI, so detect changes through `codespaces.policy_group_*` audit events. GitHub Apps also cannot create or rebuild a codespace for a user. "Rebuild via IssueOps" therefore means: commit the config, let the prebuild refresh, stop stale codespaces through the org API, and ask the user to rebuild.
- **Codespaces is the wrong place for always-on agents and automated incident response.** A codespace has a 240-minute maximum idle timeout and a hard 12-hour lifetime. Run autonomous agents on Copilot cloud agent (formerly "coding agent"), which runs on Actions and is configured by `copilot-setup-steps.yml`, or on self-hosted ARC runners or Azure Container Apps. Run incident aggregation in Actions with a Go binary. Use codespaces for humans: standard developer environments, onboarding, and war-room environments for responders.

## Key Findings

| Area | Current state (verified Oct 2026) | Implication |
|---|---|---|
| Compute pricing | 2/4/8/16/32-core at $0.18/$0.36/$0.72/$1.44/$2.88 per hour; included-usage multipliers 2/4/8/16/32\[1\] | Price is linear per core. A 32-core is 16× a 2-core. |
| Storage | $0.07 per GB-month, measured hourly, covering codespaces and prebuilds\[1\] | Stopped codespaces keep costing money until deleted.\[2\] |
| Included quota | Free: 120 core-hours and 15 GB-month. Pro: 180 core-hours and 20 GB-month. Personal accounts only.\[1\]\[3\] | Org-owned codespaces on GHEC are billed from the first minute.\[4\] |
| GPU | Retired by August 29, 2025 (Azure NCv3 retirement)\[5\] | Data-science GPU work has to run elsewhere. |
| Idle timeout | Default 30 min, user range 5–240 min; maximum lifetime 12 h\[6\]\[7\] | No "always-on" codespace is possible. |
| Retention | Default 30 days; users choose 0–30; org policy can lower it\[8\]\[9\] | Shorten it to cut storage costs. |
| Policies | Machine types, max codespaces per user, base images, port visibility, idle timeout, retention, all UI-only\[10\] | Terraform cannot manage them. Track drift through the audit log. |
| REST API version | `X-GitHub-Api-Version: 2026-03-10` is the latest\[11\] | Pin it in all scripts. |
| Billing API | Enhanced billing `/usage` and `/usage/summary`; CSV report-export API GA June 4, 2026; budgets API\[12\]\[13\]\[14\] | Build the cost CLI on these. Legacy billing endpoints are no longer accurate.\[15\] |
| Terraform | `integrations/github` 6.13.0 (July 8, 2026): `github_codespaces_organization_secret`, `_repositories`, `github_codespaces_secret`, `github_codespaces_user_secret`\[16\]\[17\] | No resource exists for Codespaces access settings or policies. |
| Data residency | Codespaces GA for GHEC with data residency on April 1, 2026 (Australia, EU, Japan, US); org- or enterprise-owned only\[18\] | Relevant if CoolGitEnterprise is on a `ghe.com` tenant. |
| Copilot | "Coding agent" renamed "Copilot cloud agent"; legacy Copilot metrics API closed down April 2, 2026\[19\]\[20\] | Update dashboards to the `/copilot/metrics/reports/*` endpoints and the `used_copilot_cloud_agent` field.\[21\] |

## Details

### 1. Enterprise and organization setup

#### 1.1 Ownership, billing and access

Codespaces ownership decides who pays, which policies apply and which audit log receives the events. GitHub's 2023 access-and-ownership changelog explains it this way.\[22\] With **organization ownership**, CoolGitOrg pays, CoolGitOrg's policies apply, and events go to the org audit log. With **user ownership**, the member pays, their personal settings apply, and events go to the member's security log.\[23\] For a governed enterprise, the only defensible setting is organization ownership with access granted to selected members. It is also the only option for GHEC with data residency: the April 2026 GA changelog says "user-owned codespaces aren't supported for GitHub Enterprise Cloud with data residency accounts."\[18\]

The setup order:

1. **Enterprise** (Enterprise settings → Policies → Codespaces): choose which organizations may enable Codespaces and pay for it.
2. **Organization** (CoolGitOrg → Settings → Codespaces → General): set ownership to *Organization ownership*, then set access to *Selected members* (or members and outside collaborators). Access can be automated with the REST API, which appears in the endpoint table below.
3. **Budget**: set a Codespaces budget on the enhanced billing platform. Prebuild Actions minutes are billed under the Actions budget, not the Codespaces budget. The cost-management docs say "Storage of the generated prebuilds is charged at the same rate as your codespaces, and is included in your GitHub Codespaces budget," while the prebuild workflows "may incur billable costs for GitHub Actions."\[24\]
4. **Policies** (UI only; see 1.2).
5. **Secrets**: use Terraform (see 1.4).

The access endpoint takes a visibility of `disabled`, `selected_members`, `all_members` or `all_members_and_outside_collaborators`:

```bash
#!/usr/bin/env bash
# scripts/codespaces-access.sh: set CoolGitOrg Codespaces access to selected members
set -euo pipefail
: "${GH_TOKEN:?GH_TOKEN (issueops-autoadmin-app installation token) required}"
ORG="${ORG:-CoolGitOrg}"
API_VER="2026-03-10"
command -v gh >/dev/null || { echo "gh CLI missing" >&2; exit 1; }
[[ $# -ge 1 ]] || { echo "usage: $0 user1 [user2 ...]" >&2; exit 2; }

users_json=$(printf '%s\n' "$@" | jq -R . | jq -s .)
gh api -X PUT "/orgs/${ORG}/codespaces/access" \
  -H "X-GitHub-Api-Version: ${API_VER}" \
  --input - <<<"$(jq -n --argjson u "$users_json" '{visibility:"selected_members", selected_usernames:$u}')"
echo "Codespaces access for ${ORG} set to selected_members (${#} users)"
```

**SSO, SAML and EMU.** With SAML SSO, a user needs an active SSO session (and authorized tokens) to create codespaces on org repositories. With Enterprise Managed Users, managed accounts work on org-owned repositories, so organization billing is in practice required. This document does not re-verify every EMU edge case; test with a pilot managed user.

**IP allow lists.** Codespaces run on GitHub-managed Azure hosts whose egress addresses change. If CoolGitEnterprise enforces an IP allow list, test whether `git` and API calls from inside a codespace are blocked before rolling out. The private-network options below are the supported workaround. This document could not confirm a current, published static IP range for Codespaces egress.

**GitHub Advanced Security.** Codespaces do not change GHAS coverage. Code still reaches GitHub through `git push`, so secret-scanning push protection applies to pushes from codespaces exactly as it does from laptops. Since March 2026, roles, teams and apps can be exempted from push protection.\[25\] Do not exempt `issueops-autoadmin-app` unless you have a strong reason, because generated commits should be scanned too.

#### 1.2 Organization Codespaces policies (UI only)

GitHub documents six constraint types, all under Settings → Codespaces → Policies → *Create Policy* → *Add constraint*:

| Constraint | Scope | Behaviour |
|---|---|---|
| Machine types | Org-wide or selected repos | Adding policies narrows the choice, never widens it. A repo whose `hostRequirements` exceed the allowed set cannot create codespaces.\[26\] |
| Maximum codespaces per user | **Always org-wide** | Applies only to org-billed codespaces. It cannot be combined with a repo-scoped policy.\[27\]\[28\] |
| Base images | Org-wide or selected repos | A disallowed `image`/`FROM` produces recovery mode or `HTTP 400: Base image ... is not allowed`. It does not affect existing codespaces.\[29\]\[30\] |
| Port visibility | Org-wide or selected repos | Choose from private, org and public. Set the org-wide policy to the most lenient value any repo needs.\[31\] |
| Maximum idle timeout | Org-wide or selected repos | The shortest value wins. It applies to existing codespaces on their next start.\[32\] |
| Retention period | Org-wide or selected repos | The shortest value wins. It applies only at creation, and prevents users from exempting codespaces from auto-deletion.\[8\] |

**API and Terraform support:** none. The REST "Codespaces organizations" reference lists only list, access control, per-user list, delete and stop.\[33\] No Terraform resource exists for policies. Treat policies as reviewed configuration: keep the intended policy in `policies/codespaces-policies.yaml` in the governance repo, apply it by hand, and alert on the audit events `codespaces.policy_group_created`, `codespaces.policy_group_updated` and `codespaces.policy_group_deleted` (see 2.3).

The recommended baseline for CoolGitOrg:

```yaml
# policies/codespaces-policies.yaml: source of truth; applied manually in UI, verified via audit log
org: CoolGitOrg
policies:
  - name: org-baseline
    target: all-repositories
    constraints:
      machine_types: [2-core, 4-core, 8-core]
      max_idle_timeout_minutes: 60
      retention_period_days: 7
      port_visibility: [private, org]        # public disabled org-wide
      base_images:
        - ghcr.io/coolgitorg/devbase:*
        - mcr.microsoft.com/devcontainers/*
  - name: org-max-per-user
    target: all-repositories                   # this constraint is always org-wide
    constraints:
      max_codespaces_per_user: 3
  - name: heavy-builds
    target: [CoolGitOrg/monorepo]
    constraints:
      machine_types: [8-core, 16-core]         # still capped by org-baseline unless baseline includes 16-core
  - name: onboarding
    target: [CoolGitOrg/onboarding-lab]
    constraints:
      machine_types: [2-core]
      max_idle_timeout_minutes: 30
      retention_period_days: 3
```

Note the interaction: policies only make the allowed set smaller. If `CoolGitOrg/monorepo` needs 16-core machines, the org-wide baseline must include 16-core, and every other repository must then get its own repo-scoped policy to cap it at 8-core. GitHub's docs say this directly: set the org-wide policy "to the largest choice of machine types that will be available for any repository."\[26\]

#### 1.3 Prebuilds at scale

- **Mechanism.** A GitHub-managed Actions workflow named "Codespaces Prebuilds" builds the container and runs `onCreateCommand` and `updateContentCommand`. It runs one workflow per prebuild, and Actions must be enabled on the repo.\[34\]\[35\]
- **Triggers.** Every push (the default), on configuration change, or scheduled. "On configuration change" uses fewer Actions minutes but can serve slightly stale dependencies.\[35\]\[36\] It is the right default for most repos.
- **Regions.** By default prebuilds are created in **all** available regions, and each region is charged storage separately.\[35\] Limit each repo to the regions where your developers are.
- **Template retention.** The default keeps two versions (the latest plus one previous).\[36\] Each retained version costs storage.\[35\]
- **Failure modes.** If Actions spending is exhausted, prebuild workflows fail. A config change or push that is still building temporarily removes the "Prebuild Ready" label, and codespaces are created without a prebuild until it finishes. To check whether a codespace used a prebuild, run `gh api /user/codespaces/$CODESPACE_NAME --jq .prebuild`.\[37\]
- **No direct trigger API.** This document found no public REST endpoint that starts a prebuild. Trigger one by pushing to a prebuild-enabled branch, which is exactly what the IssueOps flow below does.

#### 1.4 Secrets with Terraform

The Terraform Registry index for provider 6.13.0 lists `github_codespaces_organization_secret`, `github_codespaces_organization_secret_repositories`, `github_codespaces_secret` (repository level) and `github_codespaces_user_secret`.\[16\]\[17\] User secrets need a user token. A GitHub App installation cannot write them, so leave user secrets to developers.

```hcl
# terraform/modules/codespaces-secrets/main.tf
terraform {
  required_version = ">= 1.7.0"
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
  }
}

variable "org_secrets" {
  description = "Codespaces org secrets. Values must be pre-encrypted with the org Codespaces public key."
  type = map(object({
    encrypted_value = string
    visibility      = string           # all | private | selected
    repositories    = optional(list(string), [])
  }))
  sensitive = true
}

data "github_repository" "selected" {
  for_each  = toset(flatten([for s in values(nonsensitive(var.org_secrets)) : s.repositories]))
  full_name = "CoolGitOrg/${each.value}"
}

resource "github_codespaces_organization_secret" "this" {
  for_each        = nonsensitive(toset(keys(var.org_secrets)))
  secret_name     = each.value
  encrypted_value = var.org_secrets[each.value].encrypted_value
  visibility      = var.org_secrets[each.value].visibility
  selected_repository_ids = var.org_secrets[each.value].visibility == "selected" ? [
    for r in var.org_secrets[each.value].repositories : data.github_repository.selected[r].repo_id
  ] : null
}

# terraform/envs/coolgitorg/providers.tf
provider "github" {
  owner = "CoolGitOrg"
  app_auth {
    id              = var.app_id              # issueops-autoadmin-app
    installation_id = var.installation_id
    pem_file        = var.app_private_key
  }
}
```

Use `encrypted_value` rather than `plaintext_value`. Even so, Terraform state holds the ciphertext and metadata, so encrypt the state backend.\[38\] The app needs the **Organization codespaces secrets: write** permission.

#### 1.5 Dotfiles, signing, gh auth, registries, networking, features

- **Dotfiles.** These are a per-user setting (Settings → Codespaces → dotfiles repo) and are not an org-enforceable control. Put org standards in the image or in features. Leave dotfiles to personal taste.
- **Commit signing.** Codespaces offers GPG verification in the user's Codespaces settings for trusted repositories. Because GitHub signs those commits, signed-commit rulesets are satisfied. Enable it in onboarding docs; it is not available as an org toggle.
- **`gh` inside a codespace.** `GITHUB_TOKEN` is injected and scoped to the codespace's repository. For cross-repo access, declare `customizations.codespaces.repositories` permissions in `devcontainer.json` (shown in the full config below). Users approve these at creation.
- **Private registries.** GHCR images in CoolGitOrg can be pulled when the repo has package access. For other registries, Codespaces recognizes secrets with the `*_CONTAINER_REGISTRY_SERVER`, `*_CONTAINER_REGISTRY_USER` and `*_CONTAINER_REGISTRY_PASSWORD` naming pattern. Create them as org Codespaces secrets through Terraform.
- **Private networking.** GitHub's docs describe gateway/VPN approaches from inside the codespace. Azure VNet private networking for Codespaces was explicitly "in beta" when hosted-runner private networking went GA, and this document could not confirm that it has reached GA since. Community threads in 2025–2026 still describe direct VNet attachment for Codespaces as a gap.\[39\]\[40\] Plan on a VPN or tunnel approach, or on Actions larger runners with Azure private networking, for workloads that need private resources.
- **Inheritance across many repos.** Use three layers. (1) Publish a hardened base image `ghcr.io/coolgitorg/devbase:<semver>` built and signed in Actions. (2) Publish org dev container Features and Templates in an OCI registry (`ghcr.io/coolgitorg/features/*`). (3) Give each repo a thin `devcontainer.json` that references the image and features. Allow only those images in the base-image policy. Version bumps then spread through Dependabot (`package-ecosystem: devcontainers`) or the IssueOps flow.

### 2. Admin reference

#### 2.1 REST endpoints (API version 2026-03-10)

| Purpose | Method and path | Notes / auth |
|---|---|---|
| List org codespaces | `GET /orgs/{org}/codespaces` | Classic PAT needs `admin:org`; App: Organization codespaces (read)\[33\] |
| Set access | `PUT /orgs/{org}/codespaces/access` | App: Organization codespaces settings (write)\[41\] |
| Add users to access | `POST /orgs/{org}/codespaces/access/selected_users` | Requires visibility `selected_members`\[42\] |
| Remove users | `DELETE /orgs/{org}/codespaces/access/selected_users` | |
| List a member's codespaces | `GET /orgs/{org}/members/{username}/codespaces` | |
| Delete a member's codespace | `DELETE /orgs/{org}/members/{username}/codespaces/{codespace_name}` | Admin cleanup |
| Stop a member's codespace | `POST /orgs/{org}/members/{username}/codespaces/{codespace_name}/stop` | Forces a restart onto the new config |
| Org secrets | `GET/PUT/DELETE /orgs/{org}/codespaces/secrets[/{name}]`, `.../public-key`, `.../{name}/repositories[/{repository_id}]` | App: Organization codespaces secrets |
| Repo secrets | `GET/PUT/DELETE /repos/{owner}/{repo}/codespaces/secrets[/{name}]`, `.../public-key` | |
| User secrets | `/user/codespaces/secrets[...]` | User token with `codespace` or `codespace:secrets`\[43\] |
| Machines | `GET /repos/{owner}/{repo}/codespaces/machines`, `GET /user/codespaces/{name}/machines` | Discover machine names |
| User codespace lifecycle | `GET/POST /user/codespaces`, `GET/PATCH/DELETE /user/codespaces/{name}`, `POST .../start`, `POST .../stop`, `POST .../exports`, `GET .../exports/{export_id}`, `POST .../publish` | **User access tokens only** (UAT). Installation tokens cannot create codespaces. |
| Create for repo/PR | `POST /repos/{owner}/{repo}/codespaces`, `POST /repos/{owner}/{repo}/pulls/{n}/codespaces` | UAT |
| Org billing usage | `GET /organizations/{org}/settings/billing/usage`, `.../usage/summary` | Enhanced billing platform\[12\] |
| Enterprise billing usage | `GET /enterprises/{enterprise}/settings/billing/usage`, `.../usage/summary` | Enterprise owner or billing manager; the `hour` parameter was removed in November 2025\[14\]\[15\] |
| Enterprise CSV exports | `GET/POST /enterprises/{enterprise}/settings/billing/reports`, `GET .../reports/{report_id}` | GA June 4, 2026; classic PAT `manage_billing:enterprise`\[13\]\[44\] |
| Budgets | `/enterprises/{enterprise}/settings/billing/budgets` family | Create budgets and alerts programmatically |
| Audit log | `GET /orgs/{org}/audit-log?phrase=action:codespaces`, `GET /enterprises/{enterprise}/audit-log` | `read:audit_log` |

The "Permissions required for GitHub Apps" page lists the user-lifecycle endpoints (`POST /user/codespaces`, `POST /repos/{owner}/{repo}/codespaces`, etc.) as UAT-only.\[41\] In practice, `issueops-autoadmin-app` can **stop and delete** any org codespace, but it can **create, start or rebuild** a codespace only if it holds a user-to-server token for that user. Design around this limit.

#### 2.2 `gh codespace` reference

These subcommands and flags come from the `gh` manual. Re-check them with `gh codespace <cmd> --help` in your pre-flight step, because flags change between releases.

| Command | Key flags | Use |
|---|---|---|
| `gh codespace create` | `-R repo -b branch -m machine --devcontainer-path --idle-timeout 30m --retention-period 72h -l location --default-permissions -w -s` | Create a codespace |
| `gh codespace list` | `--org CoolGitOrg -u user --repo --json name,state,lastUsedAt,machineName,owner` | Inventory (`--org` needs admin) |
| `gh codespace stop` | `-c name --org -u user` | Admin stop |
| `gh codespace delete` | `-c name --all --days N --org -u user -r repo -f` | Bulk cleanup of stale codespaces |
| `gh codespace rebuild` | `-c name --full` | Apply new devcontainer config (user context) |
| `gh codespace ssh` | `-c name -- cmd`, `--config` | Remote commands; generates OpenSSH config |
| `gh codespace ports` | `forward 8080:8080`, `visibility 8080:org` | Port management (still subject to policy) |
| `gh codespace cp` / `logs` / `view` / `edit` / `code` / `jupyter` | `-e` for remote paths; `edit -m` changes machine | Day-to-day work |

User-set idle timeouts must be between 5 and 240 minutes, and the GitHub CLI cannot set a *default* timeout.\[6\]

#### 2.3 Audit log events

From GitHub's org and enterprise audit-event references: `codespaces.create`, `codespaces.destroy`, `codespaces.connect` (credentials refreshed), `codespaces.export_environment` (exported to a branch), `codespaces.restore`, `codespaces.suspend_environment` (stopped), `codespaces.attempted_to_create_from_prebuild`, `codespaces.trusted_repositories_access_update`, `codespaces.policy_group_created`, `codespaces.policy_group_updated` and `codespaces.policy_group_deleted`.\[45\]\[46\]\[47\] Stream the enterprise audit log to your SIEM. A scheduled workflow should also query `action:codespaces.policy_group` and open an issue whenever a policy changes outside the change process:

```yaml
# .github/workflows/codespaces-policy-drift.yml
name: codespaces-policy-drift
on:
  schedule: [{ cron: "17 * * * *" }]
  workflow_dispatch:
permissions: { contents: read, issues: write }
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - name: Query audit log
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_TOKEN }}   # classic PAT read:audit_log (org owner)
        run: |
          set -euo pipefail
          since=$(date -u -d '-65 minutes' +%Y-%m-%dT%H:%M:%SZ)
          gh api -H "X-GitHub-Api-Version: 2026-03-10" \
            "/orgs/CoolGitOrg/audit-log?phrase=action:codespaces.policy_group+created:>=${since}&per_page=100" \
            > events.json
          n=$(jq length events.json)
          [[ "$n" -eq 0 ]] && { echo "no policy changes"; exit 0; }
          body=$(jq -r '.[] | "- `\(.action)` by @\(.actor) at \(.["@timestamp"]/1000|todate)"' events.json)
          gh issue create -R CoolGitOrg/governance \
            --title "Codespaces policy changed (${n} events) – reconcile with policies/codespaces-policies.yaml" \
            --label codespaces,policy-drift --body "$body"
```

#### 2.4 Lifecycle, retention, limits

- **Lifecycle.** Create → active → stopped (by idle timeout, manual stop, or the 12-hour maximum lifetime) → auto-deleted after the retention period. Users get an email 24 hours before deletion.\[48\]
- **Export.** "Export changes to a branch" (`POST /user/codespaces/{name}/exports`) is the disaster-recovery path for unpushed work. Tell users to push often, because deletion is permanent.
- **Limits.** GitHub does not publish a fixed maximum number of codespaces per user. The docs say the limits "vary based on a number of factors" and that GitHub sets them.\[49\] Use the "Maximum codespaces per user" policy to set your own. Any specific figure you see quoted (for example "10") is unverified.
- **Troubleshooting.** Recovery mode usually means the build failed or a base image was blocked by policy (check `/workspaces/.codespaces/.persistedshare/creation.log` or `gh codespace logs`). "Machine type not available" means policy conflicts with `hostRequirements`. Missing prebuilds usually mean an Actions budget problem or a build still in progress.

#### 2.5 Usage and cost CLI (Go stdlib plus go-echarts)

The enhanced billing `/usage` endpoint returns `usageItems` with date, product, SKU, quantity, unit type, price per unit, gross/discount/net amounts, and organization and repository names. Confirm the exact field set against your own response, because GitHub has changed granularity before (the hourly breakdown was removed in November 2025).\[14\] The CLI below fetches org usage, filters to Codespaces, groups by repository and SKU, and writes CSV plus an HTML chart.

```go
// cmd/csusage/main.go – go 1.24; deps: stdlib + github.com/go-echarts/go-echarts/v2
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

type usageItem struct {
	Date           string  `json:"date"`
	Product        string  `json:"product"`
	SKU            string  `json:"sku"`
	Quantity       float64 `json:"quantity"`
	UnitType       string  `json:"unitType"`
	PricePerUnit   float64 `json:"pricePerUnit"`
	GrossAmount    float64 `json:"grossAmount"`
	NetAmount      float64 `json:"netAmount"`
	RepositoryName string  `json:"repositoryName"`
}

type usageResp struct {
	UsageItems []usageItem `json:"usageItems"`
}

func fetch(org, token string, year, month int) ([]usageItem, error) {
	url := fmt.Sprintf("https://api.github.com/organizations/%s/settings/billing/usage?year=%d&month=%d", org, year, month)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	c := &http.Client{Timeout: 60 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("billing usage: HTTP %d", res.StatusCode)
	}
	var u usageResp
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return nil, err
	}
	return u.UsageItems, nil
}

func main() {
	org := flag.String("org", "CoolGitOrg", "organization")
	ym := flag.String("month", time.Now().UTC().Format("2006-01"), "YYYY-MM")
	out := flag.String("out", "codespaces-usage", "output prefix")
	flag.Parse()
	tok := os.Getenv("GH_TOKEN")
	if tok == "" {
		fmt.Fprintln(os.Stderr, "GH_TOKEN required (org owner / billing manager)")
		os.Exit(2)
	}
	t, err := time.Parse("2006-01", *ym)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad -month:", err)
		os.Exit(2)
	}
	items, err := fetch(*org, tok, t.Year(), int(t.Month()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	byRepo := map[string]float64{}
	bySKU := map[string]float64{}
	for _, it := range items {
		if !strings.Contains(strings.ToLower(it.Product), "codespaces") {
			continue
		}
		repo := it.RepositoryName
		if repo == "" {
			repo = "(unattributed)"
		}
		byRepo[repo] += it.NetAmount
		bySKU[it.SKU] += it.NetAmount
	}
	if err := writeCSV(*out+".csv", byRepo, bySKU); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeChart(*out+".html", *org, *ym, byRepo); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sortedKeys(m map[string]float64) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Slice(k, func(i, j int) bool { return m[k[i]] > m[k[j]] })
	return k
}

func writeCSV(path string, byRepo, bySKU map[string]float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"dimension", "key", "net_usd"})
	for _, k := range sortedKeys(byRepo) {
		_ = w.Write([]string{"repository", k, fmt.Sprintf("%.2f", byRepo[k])})
	}
	for _, k := range sortedKeys(bySKU) {
		_ = w.Write([]string{"sku", k, fmt.Sprintf("%.2f", bySKU[k])})
	}
	w.Flush()
	return w.Error()
}

func writeChart(path, org, ym string, byRepo map[string]float64) error {
	keys := sortedKeys(byRepo)
	if len(keys) > 25 {
		keys = keys[:25]
	}
	data := make([]opts.BarData, 0, len(keys))
	for _, k := range keys {
		data = append(data, opts.BarData{Value: fmt.Sprintf("%.2f", byRepo[k])})
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title:    "Codespaces net cost by repository",
		Subtitle: org + " " + ym,
	}))
	bar.SetXAxis(keys).AddSeries("USD", data)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return bar.Render(f)
}
```

At enterprise scope, change the URL to `/enterprises/CoolGitEnterprise/settings/billing/usage`. Use a classic PAT owned by an enterprise owner or billing manager: community reporting on the enterprise usage endpoints says they do not work with GitHub App user access tokens.\[50\] For month-end chargeback, use the CSV report-export API (`report_type` = `detailed` covers up to 31 days; `summarized` covers up to 366 days).\[44\]

### 3. Connecting GitHub Copilot

#### 3.1 Copilot Chat in codespaces

In both VS Code web and desktop, Copilot follows the user's seat and the org/enterprise policies. Pre-install the extensions with `customizations.vscode.extensions: ["GitHub.copilot", "GitHub.copilot-chat"]`. VS Code has been folding inline completions into the Chat extension, so check whether `GitHub.copilot` is still needed separately in your VS Code channel. This document could not verify the current status. MCP servers for agent mode go in `.vscode/mcp.json` committed to the repo, for example the remote GitHub MCP server. The enterprise "MCP servers in Copilot" policy must allow them; August 2026 changelog coverage mentions new enterprise MCP governance controls.\[51\]

#### 3.2 Copilot CLI inside a codespace

Copilot CLI went GA on February 25, 2026. The GitHub Changelog post "GitHub Copilot CLI is now generally available" says it is "now generally available for all Copilot subscribers" and that "Copilot CLI is also included in the default GitHub Codespaces image and available as a Dev Container Feature." It is installed from npm as `@github/copilot` and needs a recent Node.js. The codespace's injected `GITHUB_TOKEN` is repo-scoped and is not a Copilot credential. Store a user-owned fine-grained PAT with Copilot permissions as a **Codespaces user secret** (e.g. `COPILOT_GITHUB_TOKEN`), or run `copilot` and `/login` interactively. The token-variable precedence is from memory; verify it against `copilot help`.

BYOK is documented by GitHub. Set these variables before starting `copilot`:\[52\]

```bash
# Codespaces user secrets → env vars (never commit)
export COPILOT_PROVIDER_TYPE=anthropic            # openai | azure | anthropic
export COPILOT_PROVIDER_BASE_URL=https://api.anthropic.com
export COPILOT_PROVIDER_API_KEY="${ANTHROPIC_API_KEY:?}"
export COPILOT_MODEL=claude-opus-4-5              # model name from your provider
copilot
```

The provider is locked in when the session starts, so you must restart `copilot` to switch providers.\[53\] Enterprise BYOK policy can also govern this, as described on GitHub's "use your own API keys" admin page.

#### 3.3 Copilot cloud agent (formerly coding agent) and how it relates to Codespaces

- **Not a codespace.** Copilot cloud agent runs in an ephemeral **GitHub Actions** environment. Configure that environment in `.github/workflows/copilot-setup-steps.yml`, which must contain a single `copilot-setup-steps` job and must be on the default branch. Only certain job settings can be customized.\[54\]\[55\]
- **Runners.** The default is `ubuntu-latest`. Larger runners, self-hosted ARC scale sets (supported since October 28, 2025) and Windows are supported. Only Ubuntu x64 and Windows 64-bit work. Org admins can set a default runner and decide whether repos may override it (Settings → Copilot → Cloud agent).\[54\]\[55\]\[56\]
- **Capabilities since April 1, 2026.** It can research, plan and work on a branch without opening a PR. Startup became more than 20% faster with Actions custom images (April 2026).\[57\]\[58\]
- **Billing.** It uses Actions minutes plus Copilot premium requests. The one-premium-request-per-session rule from July 2025 should be checked against your current plan.\[20\]
- **Keep both environments in sync.** Have `copilot-setup-steps.yml` install the same toolchain as `devcontainer.json`. The simplest way is to run the setup job inside your `ghcr.io/coolgitorg/devbase` image through a `container:` key, if your runner supports it. Otherwise, call the same `make bootstrap` target from both.

```yaml
# .github/workflows/copilot-setup-steps.yml
name: "Copilot Setup Steps"
on:
  workflow_dispatch:
  push:    { paths: [.github/workflows/copilot-setup-steps.yml] }
  pull_request: { paths: [.github/workflows/copilot-setup-steps.yml] }
jobs:
  copilot-setup-steps:
    runs-on: ubuntu-latest          # or a larger-runner label / ARC scale set
    permissions: { contents: read }
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v5
        with: { go-version: "1.24.x" }
      - name: Bootstrap (same target the devcontainer uses)
        run: make bootstrap
```

#### 3.4 Copilot Spaces, policies, metrics and seats

- **Copilot Spaces** now has REST endpoints (Spaces, collaborators, resources) in the 2026-03-10 API reference.\[59\] Treat Spaces as curated context (repos, docs, runbooks) and attach the incident-response runbooks space to on-call engineers.
- **Policies.** Enterprise policies override org policies. Copilot Business and Enterprise users need an admin to enable cloud agent.\[58\] Set the cloud agent runner and the MCP policy deliberately.
- **Metrics.** The legacy `GET /orgs/{org}/copilot/metrics` and enterprise equivalents were closed down on April 2, 2026. Use `/enterprises/{enterprise}/copilot/metrics/reports/{enterprise-1-day|enterprise-28-day/latest|users-1-day|users-28-day/latest|repos-1-day|user-teams-1-day}` and the matching `/orgs/{org}/copilot/metrics/reports/...`. These endpoints return signed NDJSON download links. They need the "Copilot usage metrics" policy set to *Enabled everywhere*. Classic tokens need `manage_billing:copilot` or `read:enterprise` (enterprise scope). Fine-grained PATs are not listed for the enterprise endpoints, which is consistent with your assumption that enterprise Copilot metering needs classic PATs. Adopt `used_copilot_cloud_agent`; the legacy `used_copilot_coding_agent` field was only guaranteed until August 1, 2026.\[11\]\[19\]\[21\]
- **Seats** (public preview): `GET /orgs/{org}/copilot/billing`, `GET /orgs/{org}/copilot/billing/seats`, and `POST`/`DELETE /orgs/{org}/copilot/billing/selected_users` (`selected_usernames`) and `/selected_teams`. Classic scopes are `manage_billing:copilot` or `admin:org`; the fine-grained permission is "GitHub Copilot Business" (write). A `422` error means the org's access setting is "all users" or unconfigured. Premium-request usage is at `/enterprises/{enterprise}/settings/billing/premium_request/usage`.\[59\]\[60\] Assign seats through team membership (`selected_teams`) managed by Terraform `github_team_members`, rather than user by user.

### 4. Base engineering codespace, reconfigurable through IssueOps (use case a)

**Flow.** A developer files the "Dev environment change" form in `CoolGitOrg/devenv-standard`. Workflow 1 (`devenv-request`) parses and validates it with the `devenvctl` Go binary and posts a plan comment. When a member of `@CoolGitOrg/platform-approvers` applies the `approved` label, the workflow renders the files and commits with the `issueops-autoadmin-app` token. It opens a PR (`mode=pr`) or pushes to `main` (`mode=direct`, allowed only for low-risk changes such as extensions), then sends `repository_dispatch: devenv-changed`. Workflow 2 (`devenv-rollout`) runs on push to `main` under `.devcontainer/**`. It validates the build with `@devcontainers/cli`, waits for the prebuild that the push triggered, stops stale org codespaces through the org API, and comments back. An org webhook on `workflow_run` completion feeds Slack/Teams and the governance event bus.

Commits pushed with an **App token** trigger downstream workflows. Commits pushed with `GITHUB_TOKEN` do not, except for `workflow_dispatch` and `repository_dispatch`. This is why the app token is used throughout.

#### 4.1 Repository layout

```
devenv-standard/
├── .devcontainer/{devcontainer.json,Dockerfile,scripts/*.sh}
├── .github/ISSUE_TEMPLATE/devenv-change.yml
├── .github/workflows/{devenv-request.yml,devenv-rollout.yml,release-devenvctl.yml,copilot-setup-steps.yml}
├── cmd/devenvctl/main.go
├── allowlist.json
├── Makefile
└── README.md
```

#### 4.2 Issue form

```yaml
# .github/ISSUE_TEMPLATE/devenv-change.yml
name: Dev environment change
description: Change the standard-issue codespace (machine, features, extensions, env, ports)
title: "[devenv] "
labels: [devenv-request]
body:
  - type: dropdown
    id: machine
    attributes:
      label: Machine type
      options: ["2-core", "4-core", "8-core", "16-core"]
      default: 1
    validations: { required: true }
  - type: checkboxes
    id: features
    attributes:
      label: Dev container features
      options:
        - label: go
        - label: node
        - label: python-uv
        - label: terraform
        - label: azure-cli
        - label: docker-in-docker
  - type: textarea
    id: extensions
    attributes:
      label: VS Code extensions
      description: One publisher.extension per line (must be in allowlist.json)
      placeholder: golang.go
  - type: textarea
    id: env
    attributes:
      label: Environment variables
      description: KEY=value per line. NO SECRETS – use Recommended secrets.
  - type: textarea
    id: secrets
    attributes:
      label: Recommended secrets
      description: Secret NAMES only, one per line (values are managed in Terraform/user settings)
  - type: input
    id: ports
    attributes:
      label: Forwarded ports
      placeholder: "8080,5432"
  - type: textarea
    id: apt
    attributes:
      label: APT packages
      description: One package per line (allowlisted)
  - type: dropdown
    id: mode
    attributes:
      label: Delivery mode
      options: ["pr", "direct"]
      default: 0
  - type: textarea
    id: why
    attributes:
      label: Justification
    validations: { required: true }
  - type: checkboxes
    id: ack
    attributes:
      label: Acknowledgements
      options:
        - label: I confirm this request contains no secret values
          required: true
```

#### 4.3 `devenvctl` (Go, stdlib only)

```go
// cmd/devenvctl/main.go – parse | render ; go 1.24, stdlib only
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Request struct {
	MachineType   string            `json:"machine_type"`
	Features      []string          `json:"features"`
	Extensions    []string          `json:"extensions"`
	EnvVars       map[string]string `json:"env_vars"`
	Secrets       []string          `json:"recommended_secrets"`
	Ports         []int             `json:"ports"`
	Apt           []string          `json:"apt_packages"`
	Mode          string            `json:"mode"`
	Justification string            `json:"justification"`
}

type Allowlist struct {
	Features   map[string]struct {
		Ref     string         `json:"ref"`
		Options map[string]any `json:"options"`
	} `json:"features"`
	Extensions []string `json:"extensions"`
	Apt        []string `json:"apt"`
	Machines   map[string]struct {
		CPUs    int    `json:"cpus"`
		Memory  string `json:"memory"`
		Storage string `json:"storage"`
	} `json:"machines"`
	DirectAllowed []string `json:"direct_allowed_fields"`
}

var (
	reEnvKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	reExt    = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9-]+$`)
	reApt    = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{0,62}$`)
	reSecret = regexp.MustCompile(`(?i)(ghp_|github_pat_|ghs_|xox[bp]-|sk-[a-z0-9]{10,}|-----BEGIN)`)
)

func sections(body string) map[string]string {
	out := map[string]string{}
	var cur string
	var b strings.Builder
	flush := func() {
		if cur != "" {
			v := strings.TrimSpace(b.String())
			if v == "_No response_" {
				v = ""
			}
			out[cur] = v
		}
		b.Reset()
	}
	for _, ln := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(ln, "### ") {
			flush()
			cur = strings.TrimSpace(strings.TrimPrefix(ln, "### "))
			continue
		}
		b.WriteString(ln + "\n")
	}
	flush()
	return out
}

func lines(s string) []string {
	var r []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			r = append(r, l)
		}
	}
	return r
}

func checked(s string) []string {
	var r []string
	for _, l := range lines(s) {
		if strings.HasPrefix(l, "- [X] ") || strings.HasPrefix(l, "- [x] ") {
			r = append(r, strings.TrimSpace(l[6:]))
		}
	}
	return r
}

func parse(body string) (Request, error) {
	if reSecret.MatchString(body) {
		return Request{}, errors.New("issue body appears to contain a secret value; refusing")
	}
	s := sections(body)
	req := Request{
		MachineType:   s["Machine type"],
		Features:      checked(s["Dev container features"]),
		Extensions:    lines(s["VS Code extensions"]),
		Secrets:       lines(s["Recommended secrets"]),
		Apt:           lines(s["APT packages"]),
		Mode:          s["Delivery mode"],
		Justification: s["Justification"],
		EnvVars:       map[string]string{},
	}
	for _, kv := range lines(s["Environment variables"]) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return req, fmt.Errorf("env line %q is not KEY=value", kv)
		}
		req.EnvVars[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if p := strings.TrimSpace(s["Forwarded ports"]); p != "" {
		for _, f := range strings.Split(p, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil || n < 1024 || n > 65535 {
				return req, fmt.Errorf("invalid port %q (1024-65535)", f)
			}
			req.Ports = append(req.Ports, n)
		}
	}
	return req, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func validate(r Request, a Allowlist) error {
	var errs []string
	if _, ok := a.Machines[r.MachineType]; !ok {
		errs = append(errs, "machine type not allowed: "+r.MachineType)
	}
	for _, f := range r.Features {
		if _, ok := a.Features[f]; !ok {
			errs = append(errs, "feature not allowlisted: "+f)
		}
	}
	for _, e := range r.Extensions {
		if !reExt.MatchString(e) || !contains(a.Extensions, e) {
			errs = append(errs, "extension not allowlisted: "+e)
		}
	}
	for _, p := range r.Apt {
		if !reApt.MatchString(p) || !contains(a.Apt, p) {
			errs = append(errs, "apt package not allowlisted: "+p)
		}
	}
	for k := range r.EnvVars {
		if !reEnvKey.MatchString(k) {
			errs = append(errs, "bad env key: "+k)
		}
	}
	for _, s := range r.Secrets {
		if !reEnvKey.MatchString(s) {
			errs = append(errs, "bad secret name: "+s)
		}
	}
	if r.Mode != "pr" && r.Mode != "direct" {
		errs = append(errs, "mode must be pr|direct")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func render(r Request, a Allowlist) ([]byte, error) {
	m := a.Machines[r.MachineType]
	feats := map[string]any{}
	for _, f := range r.Features {
		def := a.Features[f]
		if def.Options == nil {
			feats[def.Ref] = map[string]any{}
		} else {
			feats[def.Ref] = def.Options
		}
	}
	sort.Strings(r.Extensions)
	exts := append([]string{"GitHub.copilot", "GitHub.copilot-chat"}, r.Extensions...)
	secrets := map[string]any{}
	for _, s := range r.Secrets {
		secrets[s] = map[string]string{"description": "Requested via IssueOps; value managed outside git"}
	}
	dc := map[string]any{
		"name":  "CoolGitOrg standard-issue",
		"build": map[string]any{"dockerfile": "Dockerfile", "args": map[string]string{"APT_PACKAGES": strings.Join(r.Apt, " ")}},
		"hostRequirements": map[string]any{"cpus": m.CPUs, "memory": m.Memory, "storage": m.Storage},
		"features":          feats,
		"containerEnv":      r.EnvVars,
		"forwardPorts":      r.Ports,
		"secrets":           secrets,
		"onCreateCommand":       ".devcontainer/scripts/on-create.sh",
		"updateContentCommand":  ".devcontainer/scripts/update-content.sh",
		"postCreateCommand":     ".devcontainer/scripts/post-create.sh",
		"postStartCommand":      ".devcontainer/scripts/post-start.sh",
		"postAttachCommand":     "make --no-print-directory motd",
		"customizations": map[string]any{
			"vscode": map[string]any{"extensions": exts},
			"codespaces": map[string]any{
				"openFiles": []string{"README.md"},
				"repositories": map[string]any{
					"CoolGitOrg/governance": map[string]any{"permissions": map[string]string{"contents": "read", "issues": "write"}},
				},
			},
		},
	}
	return json.MarshalIndent(dc, "", "  ")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "devenvctl:", err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: devenvctl parse <allowlist.json> < body.md > req.json | render <allowlist.json> < req.json > devcontainer.json")
		os.Exit(2)
	}
	if len(os.Args) < 3 {
		must(errors.New("allowlist path required"))
	}
	ab, err := os.ReadFile(os.Args[2])
	must(err)
	var al Allowlist
	must(json.Unmarshal(ab, &al))
	in, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
	must(err)
	switch os.Args[1] {
	case "parse":
		r, err := parse(string(in))
		must(err)
		must(validate(r, al))
		out, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(out))
	case "render":
		var r Request
		must(json.Unmarshal(in, &r))
		must(validate(r, al))
		out, err := render(r, al)
		must(err)
		fmt.Println(string(out))
	default:
		must(fmt.Errorf("unknown subcommand %q", os.Args[1]))
	}
}
```

`allowlist.json` (excerpt). Machine names should match `gh api /repos/CoolGitOrg/devenv-standard/codespaces/machines`. `hostRequirements` sets minimums; the org policy sets the ceiling.

```json
{
  "machines": {
    "2-core":  {"cpus": 2,  "memory": "8gb",  "storage": "32gb"},
    "4-core":  {"cpus": 4,  "memory": "16gb", "storage": "32gb"},
    "8-core":  {"cpus": 8,  "memory": "32gb", "storage": "64gb"},
    "16-core": {"cpus": 16, "memory": "64gb", "storage": "128gb"}
  },
  "features": {
    "go":               {"ref": "ghcr.io/devcontainers/features/go:1", "options": {"version": "1.24"}},
    "node":             {"ref": "ghcr.io/devcontainers/features/node:1", "options": {"version": "lts"}},
    "python-uv":        {"ref": "ghcr.io/coolgitorg/features/python-uv:1"},
    "terraform":        {"ref": "ghcr.io/devcontainers/features/terraform:1"},
    "azure-cli":        {"ref": "ghcr.io/devcontainers/features/azure-cli:1"},
    "docker-in-docker": {"ref": "ghcr.io/devcontainers/features/docker-in-docker:2"}
  },
  "extensions": ["golang.go", "hashicorp.terraform", "ms-python.python", "redhat.vscode-yaml", "github.vscode-github-actions"],
  "apt": ["jq", "make", "shellcheck", "gnupg", "plan9port"],
  "direct_allowed_fields": ["extensions"]
}
```

#### 4.4 Dockerfile and lifecycle scripts

```dockerfile
# .devcontainer/Dockerfile
# Pin by digest in production; base must be allowed by the org base-image policy.
FROM ghcr.io/coolgitorg/devbase:1.6.0
ARG APT_PACKAGES=""
RUN set -eu; if [ -n "$APT_PACKAGES" ]; then \
      apt-get update && apt-get install -y --no-install-recommends $APT_PACKAGES && rm -rf /var/lib/apt/lists/*; \
    fi
# uv (never pip): verify release checksum
ARG UV_VERSION=0.8.0
ARG UV_SHA256=""
RUN set -eu; \
    curl -fsSLo /tmp/uv.tgz "https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/uv-x86_64-unknown-linux-gnu.tar.gz"; \
    if [ -n "$UV_SHA256" ]; then echo "${UV_SHA256}  /tmp/uv.tgz" | sha256sum -c -; fi; \
    tar -xzf /tmp/uv.tgz -C /usr/local/bin --strip-components=1; rm /tmp/uv.tgz
```

```bash
#!/usr/bin/env bash
# .devcontainer/scripts/on-create.sh  – runs in prebuild: heavy, cacheable work only
set -euo pipefail
cd "${CODESPACE_VSCODE_FOLDER:-/workspaces/devenv-standard}"
make bootstrap            # go mod download, uv sync --frozen, terraform init -backend=false
```

```bash
#!/usr/bin/env bash
# .devcontainer/scripts/post-start.sh – every start: cheap checks only
set -euo pipefail
command -v gh >/dev/null && gh auth status >/dev/null 2>&1 || echo "warn: gh not authenticated"
[[ -n "${COPILOT_GITHUB_TOKEN:-}" ]] || echo "info: set COPILOT_GITHUB_TOKEN user secret to use Copilot CLI"
```

Lifecycle semantics, from the dev container spec as GitHub uses it. `onCreateCommand` and `updateContentCommand` run during prebuilds. `postCreateCommand` runs after the codespace is assigned to the user, so user secrets are available. `postStartCommand` runs on every start. `postAttachCommand` runs on every attach. Put anything slow in the first two, or the prebuild saves you nothing.

#### 4.5 Workflow 1: request, approval, commit and dispatch

```yaml
# .github/workflows/devenv-request.yml
name: devenv-request
on:
  issues: { types: [opened, edited, labeled] }
permissions: { contents: read, issues: write }
concurrency: { group: "devenv-${{ github.event.issue.number }}", cancel-in-progress: false }
jobs:
  plan-or-apply:
    if: contains(github.event.issue.labels.*.name, 'devenv-request')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/create-github-app-token@v2
        id: app
        with:
          app-id: ${{ vars.ISSUEOPS_APP_ID }}
          private-key: ${{ secrets.ISSUEOPS_APP_PRIVATE_KEY }}
          owner: CoolGitOrg
      - uses: actions/checkout@v5
        with: { token: "${{ steps.app.outputs.token }}", fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: "1.24.x" }
      - name: Build devenvctl
        run: make build
      - name: Parse + validate issue
        id: parse
        env: { BODY: "${{ github.event.issue.body }}" }
        run: |
          set -euo pipefail
          printf '%s' "$BODY" | ./bin/devenvctl parse allowlist.json > req.json
          echo "mode=$(jq -r .mode req.json)" >> "$GITHUB_OUTPUT"
      - name: Comment plan / errors
        if: always()
        env: { GH_TOKEN: "${{ steps.app.outputs.token }}" }
        run: |
          set -euo pipefail
          if [[ -s req.json ]]; then
            ./bin/devenvctl render allowlist.json < req.json > /tmp/devcontainer.json
            diff -u .devcontainer/devcontainer.json /tmp/devcontainer.json > /tmp/plan.diff || true
            { echo "### Plan"; echo '```diff'; head -c 60000 /tmp/plan.diff; echo '```';
              echo "Apply requires the \`approved\` label from @CoolGitOrg/platform-approvers."; } > /tmp/c.md
          else
            echo "❌ Validation failed – see run ${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}" > /tmp/c.md
          fi
          gh issue comment "${{ github.event.issue.number }}" --body-file /tmp/c.md
      - name: Authorize approver
        if: github.event.action == 'labeled' && github.event.label.name == 'approved'
        env: { GH_TOKEN: "${{ steps.app.outputs.token }}" }
        run: |
          set -euo pipefail
          state=$(gh api "/orgs/CoolGitOrg/teams/platform-approvers/memberships/${{ github.event.sender.login }}" --jq .state 2>/dev/null || echo none)
          [[ "$state" == "active" ]] || { echo "::error::${{ github.event.sender.login }} is not an approver"; exit 1; }
      - name: Render, commit, deliver
        if: github.event.action == 'labeled' && github.event.label.name == 'approved'
        env:
          GH_TOKEN: ${{ steps.app.outputs.token }}
          N: ${{ github.event.issue.number }}
          MODE: ${{ steps.parse.outputs.mode }}
        run: |
          set -euo pipefail
          bot="issueops-autoadmin-app[bot]"
          uid=$(gh api "/users/${bot}" --jq .id)
          git config user.name "$bot"
          git config user.email "${uid}+${bot}@users.noreply.github.com"
          ./bin/devenvctl render allowlist.json < req.json > .devcontainer/devcontainer.json
          git add .devcontainer/devcontainer.json
          git diff --cached --quiet && { gh issue comment "$N" --body "No changes to apply."; exit 0; }
          if [[ "$MODE" == "direct" ]]; then
            git commit -m "devenv: apply #${N} (direct)"; git push origin HEAD:main
          else
            br="devenv/issue-${N}"
            git switch -c "$br"; git commit -m "devenv: apply #${N}"; git push -f origin "$br"
            gh pr create --base main --head "$br" --title "devenv: apply #${N}" \
              --body "Closes #${N}. Generated by issueops-autoadmin-app." || true
          fi
          gh api -X POST "/repos/${GITHUB_REPOSITORY}/dispatches" \
            -f event_type=devenv-changed -F "client_payload[issue]=${N}" -f "client_payload[mode]=${MODE}"
```

#### 4.6 Workflow 2: rollout, prebuild wait and codespace refresh

```yaml
# .github/workflows/devenv-rollout.yml
name: devenv-rollout
on:
  push: { branches: [main], paths: [".devcontainer/**"] }
  repository_dispatch: { types: [devenv-changed] }
permissions: { contents: read, issues: write, actions: read }
jobs:
  rollout:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/create-github-app-token@v2
        id: app
        with:
          app-id: ${{ vars.ISSUEOPS_APP_ID }}
          private-key: ${{ secrets.ISSUEOPS_APP_PRIVATE_KEY }}
          owner: CoolGitOrg
      - uses: actions/checkout@v5
      - name: Validate devcontainer builds
        if: github.event_name == 'push'
        run: |
          set -euo pipefail
          npx --yes @devcontainers/cli@latest build --workspace-folder . --log-level info
      - name: Wait for Codespaces prebuild of this commit
        if: github.event_name == 'push'
        env: { GH_TOKEN: "${{ steps.app.outputs.token }}" }
        run: |
          set -euo pipefail
          for i in $(seq 1 60); do
            st=$(gh run list --workflow "Codespaces Prebuilds" --commit "$GITHUB_SHA" --json status,conclusion \
                 --jq '.[0] | "\(.status)/\(.conclusion)"' 2>/dev/null || echo "none/")
            echo "prebuild: $st"
            case "$st" in completed/success) exit 0;; completed/*) echo "::error::prebuild failed"; exit 1;; esac
            sleep 30
          done
          echo "::warning::prebuild not observed within 30m"
      - name: Stop stale codespaces and notify owners
        if: github.event_name == 'push'
        env: { GH_TOKEN: "${{ steps.app.outputs.token }}" }
        run: |
          set -euo pipefail
          repo="${GITHUB_REPOSITORY#*/}"
          gh api --paginate "/orgs/CoolGitOrg/codespaces?per_page=100" \
            --jq ".codespaces[] | select(.repository.name==\"${repo}\" and .state==\"Available\") | \"\(.owner.login) \(.name)\"" |
          while read -r user cs; do
            gh api -X POST "/orgs/CoolGitOrg/members/${user}/codespaces/${cs}/stop" >/dev/null && echo "stopped $cs ($user)"
          done
      - name: Comment back
        if: always() && github.event_name == 'repository_dispatch'
        env: { GH_TOKEN: "${{ steps.app.outputs.token }}" }
        run: |
          gh issue comment "${{ github.event.client_payload.issue }}" --body \
            "Change delivered via **${{ github.event.client_payload.mode }}**. After merge, the prebuild refreshes automatically; running codespaces are stopped – run \`gh codespace rebuild\` (or *Rebuild container*) on next start."
```

The step that waits for "Codespaces Prebuilds" relies on how that GitHub-managed dynamic workflow appears in `gh run list`. Test it in a pilot repo. If it does not match, poll the repo's Actions runs filtered by `event=dynamic`.

#### 4.7 Makefile

```make
# Makefile – unified interface
SHELL := /usr/bin/env bash -euo pipefail
GO    ?= go
BIN   := bin
.PHONY: build test bootstrap lint motd release-checksums
build:
	@mkdir -p $(BIN)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w" -o $(BIN)/devenvctl ./cmd/devenvctl
test:
	$(GO) vet ./... && $(GO) test ./...
lint:
	shellcheck .devcontainer/scripts/*.sh scripts/*.sh
bootstrap:
	@command -v $(GO) >/dev/null && $(GO) mod download || true
	@[[ -f pyproject.toml ]] && uv sync --frozen || true
	@[[ -d terraform ]] && terraform -chdir=terraform init -backend=false -input=false || true
release-checksums:
	cd $(BIN) && sha256sum * > SHA256SUMS
motd:
	@echo "CoolGitOrg standard-issue | change me: open a 'Dev environment change' issue"
```

### 5. Cost estimation

#### 5.1 Price sheet (GitHub Docs billing page, verified)

| SKU | Unit | Price | Core-hour multiplier |
|---|---|---|---|
| Compute 2-core (8 GB RAM, 32 GB disk) | hour | $0.18 | 2\[1\] |
| Compute 4-core (16 GB, 32 GB) | hour | $0.36 | 4\[1\] |
| Compute 8-core (32 GB, 64 GB) | hour | $0.72 | 8\[1\] |
| Compute 16-core (64 GB, 128 GB) | hour | $1.44 | 16\[1\] |
| Compute 32-core (128 GB, 128 GB) | hour | $2.88 | 32\[1\] |
| GPU | n/a | **Retired August 29, 2025** | n/a\[5\] |
| Storage (codespaces and prebuilds) | GB-month | $0.07 | n/a\[61\] |
| Prebuild builds | Actions minutes | Your Actions runner rate | n/a\[62\] |

Included quotas apply only to personal accounts: Free gets 120 core-hours and 15 GB-month, Pro gets 180 core-hours and 20 GB-month. Team and Enterprise organizations have no included Codespaces quota.\[1\]\[4\] The examples use $0.006/min for Linux 2-core x64 runners (SKU `actions_linux`), the rate in GitHub Docs' "Actions runner pricing" reference. That rate reflects the GitHub Changelog of December 16, 2025, which announced GitHub would "reduce the price of GitHub-hosted runners by up to 39%" on January 1, 2026. Replace it with your own rate card if prebuilds run on larger runners.

#### 5.2 Worked examples (21 working days per month)

Assumptions: average stored footprint is 1.5 codespaces × 32 GB = 48 GB per developer (40 GB at 2000 developers, with 7-day retention). Each prebuild template is about 20 GB, in 2 regions, with 2 retained versions. Prebuild runs take about 15 minutes, with about 60 runs per repo per month using the "on configuration change" trigger.

| Org | Mix and usage | Compute/month | Storage | Prebuild storage | Prebuild Actions | **Total** | **Per dev** |
|---|---|---|---|---|---|---|---|
| 50 devs | 100% 4-core, 6 h/day | 50×126 h×$0.36 = $2,268 | 2,400 GB → $168 | 3 repos → 240 GB → $16.80 | ~$3.60 | **≈ $2,456** | **≈ $49** |
| 500 devs | 70% 4c / 20% 8c / 10% 2c, 5 h/day | $43.47×500 = $21,735 | 24,000 GB → $1,680 | 30 repos → $168 | ~$162 | **≈ $23,745** | **≈ $47.50** |
| 2000 devs | 60% 4c / 25% 8c / 10% 2c / 5% 16c, 4 h/day | $40.82×2000 = $81,648 | 80,000 GB → $5,600 | 100 repos → $560 | ~$540 | **≈ $88,348** | **≈ $44** |

**What this means:** compute makes up 90–92% of spend in every scenario. Prebuild costs are small. The policies that cap idle hours and machine size are the ones that move the bill.

#### 5.3 Idle timeout and retention levers

Suppose each developer walks away twice a day. Each walk-away burns the full idle timeout. For 500 developers on 4-core machines, a 120-minute timeout burns 2 × 2 h = 4 h/day per developer, and a 30-minute timeout burns 1 h/day. The difference is 3 h × $0.36 × 21 × 500 = **$11,340/month**, about 48% of the 500-developer bill. Cutting retention from 30 days to 7 days removes most "forgotten" stopped codespaces. If it lowers the average footprint from 48 GB to 32 GB, the 500-developer org saves 8,000 GB-month, or $560/month. Ranked levers: (1) max idle timeout of 30–60 min; (2) machine-type caps per repo; (3) retention of 7 days or less; (4) max codespaces per user of 2–3; (5) prebuilds in your users' regions only, with "on configuration change"; (6) a budget with alerts; (7) a weekly `gh codespace delete --org CoolGitOrg --days 14` sweep.

#### 5.4 Calculator and budget-alert workflow

Spreadsheet formula (one row per persona): `=Devs * HoursPerDay * Days * Cores * 0.09 + Devs * AvgGB * 0.07`. This works because the price is $0.09 per core-hour: $0.18 ÷ 2.

Add an `estimate` subcommand to `csusage`:

```go
// cmd/csusage/estimate.go
package main

import (
	"flag"
	"fmt"
)

const corePrice = 0.09   // USD per core-hour (2-core = $0.18/h)
const gbMonth = 0.07

func estimate(args []string) {
	fs := flag.NewFlagSet("estimate", flag.ExitOnError)
	devs := fs.Int("devs", 50, "developers")
	cores := fs.Int("cores", 4, "cores per codespace (2,4,8,16,32)")
	hours := fs.Float64("hours", 6, "active hours/day incl. idle tail")
	days := fs.Int("days", 21, "working days/month")
	gb := fs.Float64("gb", 48, "avg stored GB per dev")
	_ = fs.Parse(args)
	compute := float64(*devs) * *hours * float64(*days) * float64(*cores) * corePrice
	storage := float64(*devs) * *gb * gbMonth
	fmt.Printf("compute=$%.2f storage=$%.2f total=$%.2f per_dev=$%.2f\n",
		compute, storage, compute+storage, (compute+storage)/float64(*devs))
}
```

Wire it in with a `case "estimate": estimate(os.Args[2:]); return` before flag parsing in `main`. Next, the budget alert. It uses the org usage summary, a classic PAT owned by a billing manager, and posts to Slack:

```yaml
# .github/workflows/codespaces-budget-alert.yml
name: codespaces-budget-alert
on: { schedule: [{ cron: "0 7 * * *" }], workflow_dispatch: {} }
permissions: { contents: read }
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - env:
          GH_TOKEN: ${{ secrets.BILLING_READ_TOKEN }}
          SLACK_WEBHOOK: ${{ secrets.SLACK_WEBHOOK_URL }}
          BUDGET: "25000"
        run: |
          set -euo pipefail
          y=$(date -u +%Y); m=$(date -u +%-m)
          spent=$(gh api -H "X-GitHub-Api-Version: 2026-03-10" \
            "/organizations/CoolGitOrg/settings/billing/usage?year=${y}&month=${m}" \
            --jq '[.usageItems[] | select(.product|ascii_downcase|contains("codespaces")) | .netAmount] | add // 0')
          dom=$(date -u +%-d); dim=$(date -u -d "$(date -u +%Y-%m-01) +1 month -1 day" +%-d)
          proj=$(awk -v s="$spent" -v d="$dom" -v n="$dim" 'BEGIN{printf "%.0f", s/d*n}')
          echo "spent=$spent projected=$proj budget=$BUDGET"
          if (( proj > BUDGET )); then
            jq -n --arg t "Codespaces projected \$${proj} > budget \$${BUDGET} (MTD \$${spent})" '{text:$t}' |
              curl -fsS -X POST -H 'Content-Type: application/json' --data @- "$SLACK_WEBHOOK"
          fi
```

Use the native budgets API or UI for hard stops as well. The workflow adds month-end projection, which native budgets do not provide.

### 6. Use case b: agentic sandbox for always-on operation

**Verdict: do not use Codespaces as the always-on host.** The facts: the idle timeout maximum is 240 minutes, and "regardless of your idle timeout setting, a codespace has a maximum lifetime of 12 hours."\[6\] A running background process does not count as activity. Codespaces are also built for interactive development, so check GitHub's terms before running persistent autonomous workloads on them.

| Option | Always-on? | Isolation and secrets | Cost (24×7) | Fit |
|---|---|---|---|---|
| Codespace plus cron restarts | No (12 h cap, restart gaps) | User-owned; needs a user token to start | 4-core: 730 h × $0.36 = $262.80/mo | Poor |
| **Copilot cloud agent** (Actions) | Event-driven sessions | Ephemeral; integrated firewall; `copilot-setup-steps.yml` | Actions minutes plus premium requests | **Best for repo tasks** |
| Actions on a schedule, self-hosted ARC | Event or cron loop | Runner groups scope access; OIDC to cloud | Your infra | **Best for a custom Go agent with private access** |
| Azure Container Apps (jobs or apps) | Yes | Managed identity, Key Vault | Consumption-based | Best for true daemons |

**Recommended architecture.** A Go agent binary (`agentd`) runs as an Azure Container Apps job on a cron trigger (every 10 minutes), or as an ARC-backed Actions workflow. State lives in git: the agent writes its state file and journal to a `state/agent` branch through the GitHub App, which gives you an auditable log you can diff. Work that changes code goes to **Copilot cloud agent** by creating an issue assigned to Copilot or calling the "Agent tasks" REST API, which appears in the 2026-03-10 reference. This document did not verify the request schema for that API.

**Guardrails:**
- Run under the GitHub App (`issueops-autoadmin-app`) with least-privilege repository selection.
- Require PRs; rulesets block direct pushes to protected branches.
- Keep the cloud agent firewall enabled.
- Cap Actions budget per cost center.
- Provide a kill switch: the agent checks the `vars.AGENT_ENABLED` repository variable on every loop.

If a codespace is still required for the agent's interactive debugging window, restart it on a schedule with a user-to-server token:

```yaml
# .github/workflows/agent-codespace-recycle.yml  (only if you must)
name: agent-codespace-recycle
on: { schedule: [{ cron: "0 */11 * * *" }], workflow_dispatch: {} }   # < 12h lifetime
jobs:
  recycle:
    runs-on: ubuntu-latest
    steps:
      - env: { GH_TOKEN: "${{ secrets.AGENT_OPERATOR_USER_TOKEN }}" }   # user PAT with 'codespace' scope
        run: |
          set -euo pipefail
          cs=$(gh codespace list --repo CoolGitOrg/agent-sandbox --json name --jq '.[0].name')
          [[ -n "$cs" ]] || cs=$(gh codespace create -R CoolGitOrg/agent-sandbox -m standardLinux32gb --idle-timeout 240m)
          gh codespace stop -c "$cs" || true
          gh api -X POST "/user/codespaces/${cs}/start" >/dev/null
          gh codespace ssh -c "$cs" -- 'nohup make agent-run >/tmp/agent.log 2>&1 &'
```

The machine name `standardLinux32gb` is assumed to be the 4-core SKU. Confirm it against the machines endpoint. Expect gaps during restarts and up to 240 minutes of idle burn.

### 7. Use case c: automated incident response

**Design.** Run aggregation and communications in **Actions plus a Go binary**. Open a **war-room codespace** only for human responders. External signals reach GitHub through a small relay, because neither Azure Monitor's webhook action nor a Statuspage webhook can send GitHub's required `Authorization` header and `{event_type, client_payload}` body. Dynatrace custom integrations *can* add HTTP headers, but storing a long-lived GitHub token in Dynatrace is weaker than the relay, which mints short-lived App tokens.\[63\]

**Sources:**
- **Dynatrace.** Use a custom problem-notification integration that sends JSON with placeholders `{ProblemID}`, `{PID}`, `{ProblemTitle}`, `{State}`, `{ProblemSeverity}`, `{ProblemImpact}`, `{ProblemURL}`, `{Tags}`, `{ImpactedEntities}` and `{ProblemDetailsJSONv2}`. Enable notifications on close as well.\[64\]\[65\]
- **Azure Monitor.** Use an action group webhook with the **common alert schema** (`schemaId: azureMonitorCommonAlertSchema`, `data.essentials.{alertRule,severity,monitorCondition,firedDateTime,alertTargetIDs}`).\[66\]\[67\]\[68\] Microsoft notes that you cannot pass certificates through a webhook action, and recommends Logic Apps when the target needs a different schema.\[69\] The relay below does that job.
- **GitHub status.** Subscribe to `githubstatus.com` webhooks for incident create, update and resolve and component changes, sent to the relay. Also poll `https://www.githubstatus.com/api/v2/incidents/unresolved.json` from *outside* GitHub.\[70\] If Actions is the thing that is down, a scheduled workflow poller will not run.
- **Deploy failures.** Use `workflow_run` with `conclusion == failure` on deploy workflows.
- **Health probes.** The relay accepts `probe` events from your synthetic monitors.

#### 7.1 Relay (Go stdlib; runs on Azure Container Apps)

```go
// cmd/ir-relay/main.go – verifies source token, normalizes, sends repository_dispatch via App token
package main

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	appID       = os.Getenv("GH_APP_ID")
	instID      = os.Getenv("GH_INSTALLATION_ID")
	targetRepo  = "CoolGitOrg/incident-response"
	sharedToken = os.Getenv("RELAY_SHARED_TOKEN") // per-source secret, sent as ?token= or X-Relay-Token
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func appJWT(key *rsa.PrivateKey) (string, error) {
	now := time.Now().Unix()
	h := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	p, _ := json.Marshal(map[string]any{"iat": now - 60, "exp": now + 540, "iss": appID})
	unsigned := h + "." + b64(p)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + b64(sig), nil
}

func installationToken(key *rsa.PrivateKey) (string, error) {
	j, err := appJWT(key)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest(http.MethodPost, "https://api.github.com/app/installations/"+instID+"/access_tokens", nil)
	req.Header.Set("Authorization", "Bearer "+j)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("token: HTTP %d", res.StatusCode)
	}
	var t struct{ Token string `json:"token"` }
	return t.Token, json.NewDecoder(res.Body).Decode(&t)
}

func loadKey() (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode([]byte(os.Getenv("GH_APP_PRIVATE_KEY")))
	if blk == nil {
		return nil, errors.New("no PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not RSA")
	}
	return rk, nil
}

func handler(key *rsa.PrivateKey) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		src := r.PathValue("source") // dynatrace | azure | ghstatus | probe
		tok := r.Header.Get("X-Relay-Token")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(sharedToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
		if err != nil || !json.Valid(raw) {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		// client_payload is capped (10 top-level keys); keep it small and nest the raw event.
		body, _ := json.Marshal(map[string]any{
			"event_type":     "incident-signal",
			"client_payload": map[string]any{"source": src, "received_at": time.Now().UTC().Format(time.RFC3339), "raw": json.RawMessage(raw)},
		})
		it, err := installationToken(key)
		if err != nil {
			log.Print(err)
			http.Error(w, "upstream", http.StatusBadGateway)
			return
		}
		req, _ := http.NewRequest(http.MethodPost, "https://api.github.com/repos/"+targetRepo+"/dispatches", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+it)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != http.StatusNoContent {
			log.Printf("dispatch failed: %v", err)
			http.Error(w, "dispatch failed", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func main() {
	key, err := loadKey()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hook/{source}", handler(key)) // Go 1.22+ pattern routing
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

GitHub Docs' "Events that trigger workflows" page sets the repository_dispatch limits: "The maximum number of top-level properties in client_payload is 10. The payload can contain a maximum of 65,535 characters." The relay keeps the payload at four keys to stay well within it.

#### 7.2 Aggregation and fan-out workflow chain

```yaml
# .github/workflows/incident-signal.yml   (workflow 1: normalize + dedupe + open/update issue)
name: incident-signal
on:
  repository_dispatch: { types: [incident-signal] }
  workflow_run:
    workflows: ["deploy-production"]
    types: [completed]
permissions: { contents: read, issues: write, actions: write }
concurrency: { group: incident-signal, cancel-in-progress: false }
jobs:
  aggregate:
    if: github.event_name == 'repository_dispatch' || github.event.workflow_run.conclusion == 'failure'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - name: Fetch incidentctl release binary + verify checksum
        env: { GH_TOKEN: "${{ github.token }}" }
        run: |
          set -euo pipefail
          gh release download --repo CoolGitOrg/incident-response --pattern 'incidentctl_linux_amd64' --pattern 'SHA256SUMS'
          grep incidentctl_linux_amd64 SHA256SUMS | sha256sum -c -
          install -m 0755 incidentctl_linux_amd64 /usr/local/bin/incidentctl
      - name: Normalize
        env:
          EVENT: ${{ toJSON(github.event) }}
          GH_TOKEN: ${{ github.token }}
        run: |
          set -euo pipefail
          printf '%s' "$EVENT" | incidentctl normalize > incident.json
          incidentctl enrich --github-status --recent-runs CoolGitOrg/app-prod < incident.json > enriched.json
          incidentctl render --format issue < enriched.json > issue.md
          echo "KEY=$(jq -r .dedupe_key enriched.json)" >> "$GITHUB_ENV"
          echo "SEV=$(jq -r .severity enriched.json)" >> "$GITHUB_ENV"
      - name: Open or update incident issue (dedupe by key)
        env: { GH_TOKEN: "${{ github.token }}" }
        run: |
          set -euo pipefail
          n=$(gh issue list --label incident --state open --search "\"$KEY\" in:body" --json number --jq '.[0].number // empty')
          if [[ -n "$n" ]]; then gh issue comment "$n" --body-file issue.md
          else n=$(gh issue create --title "[$SEV] $(jq -r .title enriched.json)" --label "incident,sev:$SEV" --body-file issue.md | grep -o '[0-9]*$'); fi
          echo "ISSUE=$n" >> "$GITHUB_ENV"
      - name: Trigger notification fan-out (workflow 2)
        env: { GH_TOKEN: "${{ github.token }}" }
        run: gh workflow run incident-notify.yml -f issue="$ISSUE" -f severity="$SEV"
```

```yaml
# .github/workflows/incident-notify.yml   (workflow 2: fan-out; workflow_dispatch from GITHUB_TOKEN is allowed)
name: incident-notify
on:
  workflow_dispatch:
    inputs:
      issue:    { required: true, type: string }
      severity: { required: true, type: choice, options: [critical, major, minor, info] }
permissions: { issues: read }
jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - env:
          GH_TOKEN: ${{ github.token }}
          SLACK: ${{ secrets.SLACK_WEBHOOK_URL }}
          TEAMS: ${{ secrets.TEAMS_WORKFLOW_WEBHOOK_URL }}       # Teams Workflows "webhook request received" trigger
          PD_KEY: ${{ secrets.PAGERDUTY_ROUTING_KEY }}
        run: |
          set -euo pipefail
          url="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/issues/${{ inputs.issue }}"
          title=$(gh issue view "${{ inputs.issue }}" --json title --jq .title)
          runbook="https://github.com/CoolGitOrg/runbooks/blob/main/$(echo "$title" | tr 'A-Z ' 'a-z-' | cut -c1-40).md"
          msg="🚨 ${title}\nIssue: ${url}\nRunbook: ${runbook}\nWar room: https://codespaces.new/CoolGitOrg/incident-response?quickstart=1"
          jq -n --arg t "$(printf "$msg")" '{text:$t}' | curl -fsS -X POST -H 'Content-Type: application/json' --data @- "$SLACK"
          jq -n --arg t "$(printf "$msg")" '{type:"message",attachments:[{contentType:"application/vnd.microsoft.card.adaptive",content:{type:"AdaptiveCard",version:"1.4",body:[{type:"TextBlock",wrap:true,text:$t}]}}]}' |
            curl -fsS -X POST -H 'Content-Type: application/json' --data @- "$TEAMS"
          if [[ "${{ inputs.severity }}" == "critical" ]]; then
            jq -n --arg k "$PD_KEY" --arg s "$title" --arg u "$url" \
              '{routing_key:$k,event_action:"trigger",payload:{summary:$s,source:"github-incident-response",severity:"critical"},links:[{href:$u,text:"GitHub incident"}]}' |
              curl -fsS -X POST -H 'Content-Type: application/json' --data @- https://events.pagerduty.com/v2/enqueue
          fi
```

The third link in the chain is an **org webhook** (`issues` and `workflow_run` events) that sends to your SIEM or event bus, so every incident issue and fan-out run is mirrored outside GitHub.

Notes:
- Microsoft 365 Message Center post MC1181996 (updated April 15, 2026) gave the "final deprecation timeline" for Office 365 connectors, which include classic Teams incoming webhooks: rollout started May 18, 2026 and finished May 22, 2026. Classic connectors therefore no longer work, so use a Teams Workflows URL.
- Since October 15, 2026, GitHub's Slack and Teams apps support multiple repositories.\[71\] They are a reasonable alternative channel for subscribing to `incident`-labelled issues.

#### 7.3 `incidentctl` normalize core (Go stdlib)

```go
// cmd/incidentctl/normalize.go – maps any source to one Incident shape
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type Incident struct {
	Source    string   `json:"source"`
	Title     string   `json:"title"`
	Severity  string   `json:"severity"` // critical|major|minor|info
	State     string   `json:"state"`    // open|resolved
	Links     []string `json:"links"`
	DedupeKey string   `json:"dedupe_key"`
}

func key(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "irk-" + hex.EncodeToString(h[:8])
}

func normalize(ev map[string]any) Incident {
	cp, _ := ev["client_payload"].(map[string]any)
	if cp == nil { // workflow_run failure
		wr, _ := ev["workflow_run"].(map[string]any)
		name, _ := wr["name"].(string)
		url, _ := wr["html_url"].(string)
		sha, _ := wr["head_sha"].(string)
		return Incident{"deploy", "Deploy failed: " + name, "major", "open", []string{url}, key("deploy", name, sha)}
	}
	src, _ := cp["source"].(string)
	raw, _ := json.Marshal(cp["raw"])
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	switch src {
	case "dynatrace":
		sev := map[string]string{"AVAILABILITY": "critical", "ERROR": "major", "PERFORMANCE": "minor"}[str(m, "ProblemSeverity")]
		if sev == "" {
			sev = "minor"
		}
		state := "open"
		if str(m, "State") == "RESOLVED" {
			state = "resolved"
		}
		return Incident{"dynatrace", str(m, "ProblemTitle"), sev, state, []string{str(m, "ProblemURL")}, key("dt", str(m, "PID"))}
	case "azure":
		data, _ := m["data"].(map[string]any)
		es, _ := data["essentials"].(map[string]any)
		sev := map[string]string{"Sev0": "critical", "Sev1": "major", "Sev2": "minor"}[str(es, "severity")]
		if sev == "" {
			sev = "info"
		}
		state := "open"
		if str(es, "monitorCondition") == "Resolved" {
			state = "resolved"
		}
		return Incident{"azure-monitor", str(es, "alertRule"), sev, state, nil, key("az", str(es, "alertId"))}
	case "ghstatus":
		inc, _ := m["incident"].(map[string]any)
		sev := map[string]string{"critical": "critical", "major": "major", "minor": "minor"}[str(inc, "impact")]
		if sev == "" {
			sev = "info"
		}
		return Incident{"githubstatus", "GitHub: " + str(inc, "name"), sev, str(inc, "status"), []string{str(inc, "shortlink")}, key("gh", str(inc, "id"))}
	}
	return Incident{src, "Unclassified signal", "info", "open", nil, key(src, string(raw))}
}

func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	s, _ := m[k].(string)
	return s
}
```

Example Dynatrace custom payload (Settings → Integration → Problem notifications → Custom integration, webhook URL `https://ir-relay.<aca-domain>/hook/dynatrace`, header `X-Relay-Token`):

```json
{"PID":"{PID}","ProblemID":"{ProblemID}","ProblemTitle":"{ProblemTitle}","State":"{State}",
 "ProblemSeverity":"{ProblemSeverity}","ProblemImpact":"{ProblemImpact}","ProblemURL":"{ProblemURL}",
 "Tags":"{Tags}","ImpactedEntities":{ImpactedEntities},"Details":{ProblemDetailsJSONv2}}
```

For Azure, create an action group with a webhook receiver at `https://ir-relay.<aca-domain>/hook/azure?token=...` and `useCommonAlertSchema: true`.\[72\] Better still, use the "Secure webhook" action with Entra ID auth and validate the bearer token in the relay instead of a query token.

#### 7.4 War-room codespace

`CoolGitOrg/incident-response` ships a `devcontainer.json` with `incidentctl`, `az`, `kubectl`, the Dynatrace CLI and the runbooks repo granted through `customizations.codespaces.repositories`. A prebuild runs in the on-call region. A repo policy allows 4-core machines, a 60-minute idle timeout, 3-day retention and org-only ports. Responders open it from the link in every notification (`codespaces.new/...?...quickstart=1`). Attach a Copilot Space containing the runbooks, past incident postmortems and service ownership files, so that Copilot Chat in the war room answers from curated context.

### 8. Use case d: ephemeral onboarding and training environments

GPU codespaces are retired, so the fourth use case is **onboarding**, which also exercises the access-control API.\[5\]

**Orchestration:**
1. A manager files the "Onboard engineer" issue form with the GitHub handle, team and start date.
2. Workflow 1 adds the user to the `onboarding` team (Terraform PR or `gh api`), grants Codespaces access with `POST /orgs/CoolGitOrg/codespaces/access/selected_users`, assigns a Copilot seat through `POST /orgs/CoolGitOrg/copilot/billing/selected_teams` (team-based), and comments a one-click link: `https://codespaces.new/CoolGitOrg/onboarding-lab?quickstart=1`.
3. The trainee creates the codespace. The app cannot create it for them.
4. A nightly sweep deletes onboarding-lab codespaces older than 7 days through the org delete endpoint. On day 30, the workflow removes Codespaces access unless the user has moved to a product team.

Repo policy: 2-core only, 30-minute idle timeout, 3-day retention, no public ports.

```yaml
# onboarding-lab/.devcontainer/devcontainer.json (as JSON-compatible YAML view; commit as .json)
{
  "name": "CoolGitOrg onboarding lab",
  "image": "ghcr.io/coolgitorg/devbase:1.6.0",
  "hostRequirements": { "cpus": 2, "memory": "8gb", "storage": "32gb" },
  "features": { "ghcr.io/devcontainers/features/go:1": { "version": "1.24" } },
  "onCreateCommand": "make bootstrap",
  "postCreateCommand": "make lab-init USER=$GITHUB_USER",
  "postAttachCommand": "make --no-print-directory motd",
  "portsAttributes": { "8080": { "label": "lab-app", "onAutoForward": "notify" } },
  "customizations": {
    "codespaces": { "openFiles": ["LAB.md"] },
    "vscode": { "extensions": ["golang.go", "GitHub.copilot-chat"] }
  }
}
```

```bash
#!/usr/bin/env bash
# scripts/onboarding-sweep.sh – delete onboarding-lab codespaces older than N days
set -euo pipefail
: "${GH_TOKEN:?app installation token with Organization codespaces: write}"
DAYS="${DAYS:-7}"; ORG=CoolGitOrg; REPO=onboarding-lab
cutoff=$(date -u -d "-${DAYS} days" +%s)
gh api --paginate "/orgs/${ORG}/codespaces?per_page=100" \
  --jq ".codespaces[] | select(.repository.name==\"${REPO}\") | \"\(.owner.login) \(.name) \(.created_at)\"" |
while read -r user cs created; do
  (( $(date -u -d "$created" +%s) < cutoff )) || continue
  gh api -X DELETE "/orgs/${ORG}/members/${user}/codespaces/${cs}" && echo "deleted ${cs} (${user}, ${created})"
done
```

Cost: 30 trainees × 4 h/day × 10 days × $0.18 = $216, plus negligible storage under 3-day retention.

### 9. Security hardening checklist

- Set organization ownership and selected-member access. Use an org-wide port policy of `private, org`, with no public ports.
- Use a base-image policy limited to `ghcr.io/coolgitorg/*` and `mcr.microsoft.com/devcontainers/*`. Pin images by digest in the Dockerfile.
- Keep secrets in Codespaces org and repo secrets scoped to `selected` repositories through Terraform. Never put them in issue bodies (`devenvctl` refuses token-shaped strings) or in `containerEnv`.
- Keep push protection on. Do not exempt the automation app.
- Use `customizations.codespaces.repositories` for least-privilege cross-repo tokens.
- `gh codespace ssh` relies on GitHub auth. Discourage running custom `sshd` on public ports; the port policy should block exposure anyway.
- Watch the audit log for `codespaces.policy_group_*`, `codespaces.export_environment` and unusual `codespaces.create` volume.\[73\]
- Do disaster recovery through git. Push often, and use `gh codespace` export to a branch before forced deletions. Org-forced deletes cannot be undone.

## Recommendations

1. **This week:** switch CoolGitOrg to organization ownership with selected-member access (scripted). Apply the policy baseline: 2/4/8-core, 60-minute idle, 7-day retention, no public ports, 3 codespaces per user. Turn on the drift-detection workflow.
2. **Next sprint:** move all Codespaces secrets into the Terraform module using `encrypted_value`. Publish `ghcr.io/coolgitorg/devbase` and org features. Switch prebuilds to "on configuration change" in 1–2 regions.
3. **IssueOps:** ship `devenv-standard` with `devenvctl`. Keep `direct` mode limited to extension-only changes. Everything else goes through a PR.
4. **Cost:** build `csusage` as a release binary (with SHA256SUMS). Run the daily budget-alert projection. Review the top-25 repo chart monthly. Expect $44–49 per developer per month. If you see more than $60, the cause is almost always idle timeout or machine size.
5. **Agents:** use Copilot cloud agent for repository work and ACA or ARC for daemons. Do not build always-on agents on Codespaces.
6. **Incidents:** deploy `ir-relay` on Azure Container Apps. Connect Dynatrace, Azure Monitor and githubstatus webhooks to it. Keep a status poller outside GitHub.
7. **Copilot telemetry:** move every dashboard to `/copilot/metrics/reports/*` and `used_copilot_cloud_agent`. Manage seats through `selected_teams` tied to Terraform-managed teams.

## Caveats

- **Unverified or from memory:**
  - `gh codespace` flag details
  - Codespaces machine SKU names (e.g. `standardLinux32gb`)
  - Copilot CLI token-variable precedence
  - Whether `GitHub.copilot` is still a separate extension
  - The current status of Azure VNet private networking for Codespaces (last confirmed as beta)

  Validate each in a pilot before production.
- **Inferred, not stated by GitHub:** "no API for policies" comes from the absence of policy endpoints in the REST reference, not from an explicit statement. "No prebuild trigger API" was likewise not found rather than confirmed absent.
- **Secondary sources:** the Copilot CLI BYOK release date (April 7, 2026) comes from secondary coverage. Copilot CLI GA on February 25, 2026 is a primary-source fact, confirmed by the GitHub Changelog of that date and by the github/copilot-cli changelog entry for v0.0.418 ("Copilot CLI is now generally available"). The BYOK environment variables themselves are confirmed in GitHub Docs.\[52\]
- **Illustrative cost figures:** the cost tables use the stated assumptions. Your actual hours, footprints and runner rates will differ, so use the calculator with your own telemetry.
- **Data residency:** if CoolGitEnterprise runs on GHEC with data residency (`*.ghe.com`), API hostnames differ, and user-owned codespaces are not supported.\[18\]

## Sources

1. [About GitHub Codespaces pricing](https://docs.github.com/pt/billing/managing-billing-for-your-products/managing-billing-for-github-codespaces)
2. [Codespaces vs Gitpod vs Cloud Workstations: \$199 Gap \[2026\]](https://tech-insider.org/codespaces-vs-gitpod-vs-cloud-workstations-2026/)
3. [GitHub Codespaces · Cloud Development Environment · GitHub · GitHub](https://github.com/features/codespaces)
4. [GitHub Codespaces Review: Cloud IDE Pricing, Dev Containers, Features, and Alternatives](https://aiidelist.com/ide/github-codespaces)
5. [Upcoming deprecation of GPU machine type in Codespaces - GitHub Changelog](https://github.blog/changelog/2025-08-01-upcoming-deprecation-of-gpu-machine-type-in-codespaces/)
6. [Setting your timeout period for GitHub Codespaces - GitHub Docs](https://docs.github.com/en/codespaces/setting-your-user-preferences/setting-your-timeout-period-for-github-codespaces)
7. [troubleshooting included usage](https://docs.github.com/en/codespaces/troubleshooting/troubleshooting-included-usage)
8. [Restricting the retention period for codespaces - GitHub Docs](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization/restricting-the-retention-period-for-codespaces)
9. [Configuring automatic deletion of your codespaces - GitHub Docs](https://docs.github.com/en/codespaces/setting-your-user-preferences/configuring-automatic-deletion-of-your-codespaces)
10. [Managing GitHub Codespaces for your organization](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization)
11. [REST API endpoints for Copilot usage metrics - GitHub Docs](https://docs.github.com/en/rest/copilot/copilot-usage-metrics)
12. [Billing usage - GitHub Docs](https://docs.github.com/en/rest/billing/usage)
13. [API access to billing usage reports now generally available - GitHub Changelog](https://github.blog/changelog/2026-06-04-api-access-to-billing-usage-reports-now-generally-available/)
14. [Manage budgets and track usage with new billing API updates - GitHub Changelog](https://github.blog/changelog/2025-11-03-manage-budgets-and-track-usage-with-new-billing-api-updates/)
15. [automating usage reporting](https://docs.github.com/en/enterprise-cloud@latest/billing/managing-your-billing/automating-usage-reporting)
16. [integrations/github](https://registry.terraform.io/providers/integrations/github/latest)
17. [github\_actions\_environment\_secret](https://registry.terraform.io/providers/integrations/github/latest/docs/resources/actions_environment_secret)
18. [Codespaces is now generally available for GitHub Enterprise with data residency - GitHub Changelog](https://github.blog/changelog/2026-04-01-codespaces-is-now-generally-available-for-github-enterprise-with-data-residency/)
19. [REST API endpoints for Copilot metrics - GitHub Docs](https://docs.github.com/en/rest/copilot/copilot-metrics?apiVersion=2026-03-10)
20. [Research, plan, and code with Copilot cloud agent 🚀 · community · Discussion #190573 · GitHub](https://github.com/orgs/community/discussions/190573)
21. [Copilot cloud agent fields added to usage metrics - GitHub Changelog](https://github.blog/changelog/2026-04-23-copilot-cloud-agent-fields-added-to-usage-metrics/)
22. [Codespaces Access and Ownership Settings - GitHub Changelog](https://github.blog/changelog/2023-07-25-codespaces-access-and-ownership-settings/)
23. [Upcoming Changes to Codespaces Access, Ownership, and Billing Settings - GitHub Changelog](https://github.blog/changelog/2023-06-09-upcoming-changes-to-codespaces-access-ownership-and-billing-settings/)
24. [Managing the cost of GitHub Codespaces in your organization - GitHub Docs](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization/managing-the-cost-of-github-codespaces-in-your-organization)
25. [03/2026 - GitHub Changelog](https://github.blog/changelog/month/03-2026/page/3/)
26. [Restricting access to machine types - GitHub Docs](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization/restricting-access-to-machine-types)
27. [Maximum Codespaces per User Policy - GitHub Changelog](https://github.blog/changelog/2023-06-15-maximum-codespaces-per-user-policy/)
28. [restricting the number of organization billed codespaces a user can create](https://docs.github.com/en/enterprise-cloud@latest/codespaces/managing-codespaces-for-your-organization/restricting-the-number-of-organization-billed-codespaces-a-user-can-create)
29. [Introduction to dev containers - GitHub Docs](https://docs.github.com/en/codespaces/setting-up-your-project-for-codespaces/adding-a-dev-container-configuration/introduction-to-dev-containers)
30. [restricting the base image for codespaces](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization/restricting-the-base-image-for-codespaces)
31. [restricting the visibility of forwarded ports](https://docs.github.com/en/enterprise-cloud@latest/codespaces/managing-codespaces-for-your-organization/restricting-the-visibility-of-forwarded-ports)
32. [Restricting the idle timeout period - GitHub Docs](https://docs.github.com/en/codespaces/managing-codespaces-for-your-organization/restricting-the-idle-timeout-period)
33. [REST API endpoints for Codespaces organizations - GitHub Docs](https://docs.github.com/en/rest/codespaces/organizations)
34. [What Is GitHub Codespaces? A Beginner's Guide](https://tms-outsource.com/blog/posts/what-is-github-codespaces/)
35. [configuring prebuilds](https://docs.github.com/en/codespaces/prebuilding-your-codespaces/configuring-prebuilds)
36. [Prebuilding codespaces is generally available - The GitHub Blog](https://github.blog/news-insights/product-news/prebuilding-codespaces-is-generally-available/)
37. [Troubleshooting prebuilds - GitHub Docs](https://docs.github.com/en/codespaces/troubleshooting/troubleshooting-prebuilds)
38. [How to Create GitHub Actions Secrets with Terraform](https://oneuptime.com/blog/post/2026-02-23-how-to-create-github-actions-secrets-with-terraform/view)
39. [Can Codespaces support Vnet? · community · Discussion #10477 · GitHub](https://github.com/orgs/community/discussions/10477)
40. [Bringing enterprise-level security and even more power to GitHub-hosted runners - The GitHub Blog](https://github.blog/news-insights/product-news/bringing-enterprise-level-security-and-even-more-power-to-github-hosted-runners/)
41. [Permissions required for GitHub Apps - GitHub Docs](https://docs.github.com/en/rest/authentication/permissions-required-for-github-apps)
42. [GitHub.Codespaces — GitHub REST API Client v0.3.3](https://hexdocs.pm/oapi_github/GitHub.Codespaces.html)
43. [REST API endpoints for Codespaces user secrets - GitHub Docs](https://docs.github.com/en/rest/codespaces/secrets)
44. [Public preview of API for .CSV usage reports · community · Discussion #186162 · GitHub](https://github.com/orgs/community/discussions/186162)
45. [Audit log events for your organization - GitHub Docs](https://docs.github.com/en/organizations/keeping-your-organization-secure/managing-security-settings-for-your-organization/audit-log-events-for-your-organization)
46. [Audit log events for your enterprise - GitHub Enterprise Cloud Docs](https://docs.github.com/en/enterprise-cloud@latest/admin/monitoring-activity-in-your-enterprise/reviewing-audit-logs-for-your-enterprise/audit-log-events-for-your-enterprise)
47. [audit log events for your organization](https://docs.github.com/es/enterprise-server@3.8/organizations/keeping-your-organization-secure/managing-security-settings-for-your-organization/audit-log-events-for-your-organization)
48. [What’s new in Codespaces for Organizations - The GitHub Blog](https://github.blog/news-insights/product-news/whats-new-in-codespaces-for-organizations/)
49. [Understanding the codespace lifecycle - GitHub Docs](https://docs.github.com/en/codespaces/about-codespaces/understanding-the-codespace-lifecycle)
50. [Manage budgets and track usage with new billing API updates · community · Discussion #178723 · GitHub](https://github.com/orgs/community/discussions/178723)
51. [GitHub Copilot Agent Mode: What It Does in 2026](https://www.refontelearning.com/blog/github-copilot-agent-mode-what-it-does)
52. [Adding LLM models to GitHub Copilot CLI - GitHub Docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-byok-models)
53. [GitHub Copilot CLI: Seamless LLM Provider Switching & Your Software Project Plan](https://devactivity.com/insights/streamlining-github-copilot-cli-efficient-llm-provider-switching-for-your-software-project-plan/)
54. [Configure the development environment - GitHub Docs](https://docs.github.com/copilot/how-tos/use-copilot-agents/coding-agent/customize-the-agent-environment)
55. [Copilot coding agent now supports self-hosted runners - GitHub Changelog](https://github.blog/changelog/2025-10-28-copilot-coding-agent-now-supports-self-hosted-runners/)
56. [Configuring runners for GitHub Copilot cloud agent in your organization - GitHub Docs](https://docs.github.com/en/copilot/how-tos/administer-copilot/manage-for-organization/configure-runner-for-coding-agent)
57. [04/2026 - GitHub Changelog](https://github.blog/changelog/month/04-2026/)
58. [Research, plan, and code with Copilot cloud agent - GitHub Changelog](https://github.blog/changelog/2026-04-01-research-plan-and-code-with-copilot-cloud-agent/)
59. [REST API endpoints for Copilot user management - GitHub Docs](https://docs.github.com/en/rest/copilot/copilot-user-management)
60. [docs.github.com](https://docs.github.com/en/enterprise-cloud@latest/rest/billing/usage)
61. [GitHub Codespaces billing - GitHub Docs](https://docs.github.com/billing/managing-billing-for-github-codespaces/about-billing-for-github-codespaces)
62. [about github codespaces prebuilds](https://docs.github.com/en/codespaces/prebuilding-your-codespaces/about-github-codespaces-prebuilds)
63. [Dynatrace integration - APEX AIOps - Moogsoft](https://docs.moogsoft.com/moogsoft-cloud/en/dynatrace-integration.html)
64. [dynatrace\_notification](https://registry.terraform.io/providers/dynatrace-oss/dynatrace/latest/docs/resources/notification)
65. [Dynatrace Automation with Native Tools: The DIY Integration Guide](https://cloudthinker.io/blogs/dynatrace-automation-native-tools)
66. [Common alert schema for Azure Monitor alerts - Azure Monitor](https://learn.microsoft.com/en-us/azure/azure-monitor/alerts/alerts-common-schema)
67. [How to Create and Manage Action Groups in Azure Monitor for Alert Notifications](https://oneuptime.com/blog/post/2026-02-16-how-to-create-and-manage-action-groups-in-azure-monitor-for-alert-notifications/view)
68. [Automating Incident Response: Configuring Azure Monitor Action Groups with Webhooks and Logic Apps](https://www.c-sharpcorner.com/article/automating-incident-response-configuring-azure-monitor-action-groups-with-webhooks-and-logic-apps)
69. [action groups](https://learn.microsoft.com/pt-br/Azure/azure-monitor/alerts/action-groups)
70. [GitHub Status - API](https://www.githubstatus.com/api/)
71. [GitHub Changelog](https://github.blog/changelog/)
72. [Create and manage action groups in Azure Monitor - Azure Monitor](https://learn.microsoft.com/en-us/azure/azure-monitor/alerts/action-groups)
73. [GitHub Codespaces Security Best Practices](https://www.legitsecurity.com/blog/github-codespaces-security-best-practices)
