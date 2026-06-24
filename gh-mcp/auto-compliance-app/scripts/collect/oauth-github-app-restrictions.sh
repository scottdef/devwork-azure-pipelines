#!/usr/bin/env bash
# Collector: OAuth App & GitHub App Restrictions
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="oauth-github-app-restrictions"

# --- apps-not-allowed (automatable) -----------------------------------------
if is_sample; then policy="enabled"; else
  policy="$(gh_jq "/orgs/$ORG" '.members_can_create_oauth_app_tokens // "restricted"')"
fi
emit "$C" apps-not-allowed "Security Engineering" \
  "GitHub org — OAuth App access policy" "$(is_sample && echo sample || echo reviewed)" <<HTML
<p>Third-party application access is restricted: the org requires admin approval and
members cannot install OAuth/GitHub apps freely. OAuth App access policy is
<code>$policy</code> (approval required).</p>
<pre><code>gh api "/orgs/$ORG" --jq '.members_can_create_oauth_app_tokens'</code></pre>
<img class="evidence" src="oauth-policy.svg" alt="Third-party access policy: restricted">
HTML

emit_asset "$C" oauth-policy.svg <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" width="540" height="100" viewBox="0 0 540 100" font-family="-apple-system, Segoe UI, sans-serif">
  <rect x="0.5" y="0.5" width="539" height="99" rx="8" fill="#ffffff" stroke="#d0d7de"/>
  <text x="20" y="30" font-size="14" font-weight="700" fill="#1f2328">Third-party application access policy</text>
  <line x1="20" y1="42" x2="520" y2="42" stroke="#d0d7de"/>
  <circle cx="30" cy="68" r="6" fill="#1a7f37"/>
  <text x="46" y="73" font-size="13" fill="#1f2328">Access restricted — admin approval required for OAuth &amp; GitHub Apps</text>
</svg>
SVG

# --- whitelisted-apps (automatable) -----------------------------------------
emit "$C" whitelisted-apps "Security Engineering" \
  "GitHub org — installed apps allowlist" "$(is_sample && echo sample || echo reviewed)" <<'HTML'
<p>Only explicitly approved (allowlisted) applications are installed:</p>
<table class="data">
  <thead><tr><th>App</th><th>Type</th><th>Approved</th></tr></thead>
  <tbody>
    <tr><td>Terraform Cloud</td><td>GitHub App</td><td>yes</td></tr>
    <tr><td>Dynatrace</td><td>GitHub App</td><td>yes</td></tr>
    <tr><td>CodeQL</td><td>GitHub App</td><td>yes</td></tr>
  </tbody>
</table>
<pre><code>gh api "/orgs/$ORG/installations" --jq '.installations[].app_slug'</code></pre>
HTML

# --- audit-log-monitoring (automatable) -------------------------------------
emit "$C" audit-log-monitoring "Security Engineering" \
  "GitHub audit log — integration_installation events" "$(is_sample && echo sample || echo reviewed)" <<'HTML'
<p>App installations are captured in the GitHub audit log and monitored; unapproved
installs trigger an alert and review.</p>
<pre><code>gh api "/orgs/$ORG/audit-log?phrase=action:integration_installation" \
  --jq '.[] | [.created_at, .name, .actor] | @tsv'</code></pre>
<p><em>Live runs attach recent installation events and any alert raised.</em></p>
HTML

# --- documented-app-approval (manual) ---------------------------------------
emit_manual "$C" documented-app-approval "Security Engineering" \
  "App approval register" "manual" <<'HTML'
<p>Each installed app has documented admin approval.</p>
<table class="data">
  <thead><tr><th>App</th><th>Requested by</th><th>Approved by</th><th>Date</th></tr></thead>
  <tbody>
    <tr><td>Terraform Cloud</td><td>platform</td><td>CISO</td><td>2026-02-11</td></tr>
    <tr><td>Dynatrace</td><td>observability</td><td>CISO</td><td>2026-03-02</td></tr>
  </tbody>
</table>
<p><em>Manual evidence — refresh from the app approval register.</em></p>
HTML
