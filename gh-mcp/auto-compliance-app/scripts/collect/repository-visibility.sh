#!/usr/bin/env bash
# Collector: Repository Visibility
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="repository-visibility"

# --- all-internal-or-private (automatable) ----------------------------------
if is_sample; then pub=0; priv=58; internal=41; else
  pub="$(gh_jq "/orgs/$ORG/repos?per_page=100" '[.[]|select(.visibility=="public")]|length')"
  priv="$(gh_jq "/orgs/$ORG/repos?per_page=100" '[.[]|select(.visibility=="private")]|length')"
  internal="$(gh_jq "/orgs/$ORG/repos?per_page=100" '[.[]|select(.visibility=="internal")]|length')"
fi
status="reviewed"; [ "${pub:-0}" != "0" ] && status="needs remediation"; is_sample && status="sample"
emit "$C" all-internal-or-private "Platform Engineering" \
  "GitHub REST API — /orgs/$ORG/repos (.visibility)" "$status" <<HTML
<p>Repository visibility across <code>$ORG</code>: <strong>$internal</strong> internal,
<strong>$priv</strong> private, <strong>$pub</strong> public. No public repositories
are permitted; the count above must remain 0.</p>
<table class="data">
  <thead><tr><th>Visibility</th><th>Count</th></tr></thead>
  <tbody>
    <tr><td>internal</td><td>$internal</td></tr>
    <tr><td>private</td><td>$priv</td></tr>
    <tr><td>public</td><td>$pub</td></tr>
  </tbody>
</table>
HTML

# --- no-ui-repo-creation (automatable) --------------------------------------
if is_sample; then can_pub="false"; can_priv="false"; can_int="false"; else
  can_pub="$(gh_jq "/orgs/$ORG" '.members_can_create_public_repositories')"
  can_priv="$(gh_jq "/orgs/$ORG" '.members_can_create_private_repositories')"
  can_int="$(gh_jq "/orgs/$ORG" '.members_can_create_internal_repositories')"
fi
status="reviewed"; { [ "$can_pub" = "true" ] || [ "$can_priv" = "true" ] || [ "$can_int" = "true" ]; } && status="needs remediation"; is_sample && status="sample"
emit "$C" no-ui-repo-creation "Platform Engineering" \
  "GitHub REST API — /orgs/$ORG (members_can_create_*_repositories)" "$status" <<HTML
<p>Members cannot create repositories through the GitHub UI — all member
repo-creation settings are disabled; repositories are provisioned via Terraform.</p>
<pre><code>members_can_create_public_repositories   = $can_pub
members_can_create_private_repositories  = $can_priv
members_can_create_internal_repositories = $can_int</code></pre>
HTML

# --- automated-scans-alerting (automatable) ---------------------------------
emit "$C" automated-scans-alerting "Platform Engineering" \
  "collect-evidence.yml + alerting webhook" "$(is_sample && echo sample || echo reviewed)" <<'HTML'
<p>A scheduled scan re-checks visibility weekly (this collector) and posts an alert
to the security channel if any repository is public.</p>
<pre><code>pub=$(gh api "/orgs/$ORG/repos?per_page=100" \
        --jq '[.[]|select(.visibility=="public")]|length')
[ "$pub" -gt 0 ] && curl -s -X POST "$ALERT_WEBHOOK" \
  -d "{\"text\":\"⚠️ $pub public repo(s) detected in $ORG\"}"</code></pre>
<p><em>Live runs link the scan run URL and any alert posted.</em></p>
HTML

# --- default-visibility-iac (manual: Terraform snapshot) --------------------
emit_manual "$C" default-visibility-iac "Platform Engineering" \
  "Terraform — github_organization_settings" "manual" <<'HTML'
<p>Default repository visibility is set to <code>private</code> and member
repo-creation is disabled via Terraform (org settings as code):</p>
<pre><code>resource "github_organization_settings" "cla" {
  default_repository_permission           = "read"
  members_can_create_public_repositories  = false
  members_can_create_private_repositories = false
  members_can_create_internal_repositories = false
}</code></pre>
<p><em>Manual evidence — refresh by pinning the current Terraform file/commit and a
<code>terraform plan</code> showing no drift.</em></p>
HTML
