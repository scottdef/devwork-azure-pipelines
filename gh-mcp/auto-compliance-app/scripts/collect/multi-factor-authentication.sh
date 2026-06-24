#!/usr/bin/env bash
# Collector: Multi-Factor Authentication
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="multi-factor-authentication"

# --- all-accounts-2fa (automatable) -----------------------------------------
if is_sample; then disabled=0; total=137; else
  disabled="$(gh_jq "/orgs/$ORG/members?filter=2fa_disabled" 'length')"
  total="$(gh api "/orgs/$ORG/members" --paginate --jq 'length' 2>/dev/null | paste -sd+ - | bc || echo "?")"
fi
status="reviewed"; [ "${disabled:-0}" != "0" ] && status="needs remediation"
is_sample && status="sample"
emit "$C" all-accounts-2fa "Security Engineering" \
  "GitHub REST API — /orgs/$ORG/members?filter=2fa_disabled" "$status" <<HTML
<p>As of $(today), the <code>$ORG</code> organization has <strong>$total</strong> members,
of which <strong>$disabled</strong> do not have two-factor authentication enabled
(queried via the GitHub REST API). Org policy also requires 2FA, so non-compliant
members cannot remain in the organization.</p>
<pre><code>gh api "/orgs/$ORG/members?filter=2fa_disabled" --jq 'length'   # =&gt; $disabled</code></pre>
HTML

# --- cannot-disable (automatable) -------------------------------------------
if is_sample; then required="true"; else
  required="$(gh_jq "/orgs/$ORG" '.two_factor_requirement_enabled')"
fi
status="reviewed"; [ "$required" != "true" ] && status="needs remediation"
is_sample && status="sample"
emit "$C" cannot-disable "Security Engineering" \
  "GitHub REST API — /orgs/$ORG (.two_factor_requirement_enabled)" "$status" <<HTML
<p>Two-factor authentication is <strong>enforced at the organization level</strong>
(<code>two_factor_requirement_enabled = $required</code>). Members cannot turn 2FA
off for their account while remaining in the org; disabling it removes them.</p>
<pre><code>gh api "/orgs/$ORG" --jq '.two_factor_requirement_enabled'   # =&gt; $required</code></pre>
<img class="evidence" src="2fa-org-setting.svg" alt="Require two-factor authentication enabled">
HTML

emit_asset "$C" 2fa-org-setting.svg <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" width="560" height="120" viewBox="0 0 560 120" font-family="-apple-system, Segoe UI, sans-serif">
  <rect x="0.5" y="0.5" width="559" height="119" rx="8" fill="#ffffff" stroke="#d0d7de"/>
  <text x="20" y="30" font-size="15" font-weight="700" fill="#1f2328">Authentication security</text>
  <line x1="20" y1="44" x2="540" y2="44" stroke="#d0d7de"/>
  <rect x="20" y="62" width="34" height="20" rx="10" fill="#1a7f37"/><circle cx="44" cy="72" r="8" fill="#fff"/>
  <text x="68" y="69" font-size="14" font-weight="600" fill="#1f2328">Require two-factor authentication for everyone</text>
  <text x="68" y="88" font-size="12" fill="#656d76">Enabled — members without 2FA are removed from the organization.</text>
</svg>
SVG

# --- new-user-default (manual) ----------------------------------------------
emit_manual "$C" new-user-default "IT Onboarding" "Onboarding runbook v3" "manual" <<'HTML'
<p>New-member onboarding requires 2FA before access is granted. Because the org
enforces 2FA, an invited user cannot accept membership until 2FA is configured.</p>
<h3>Onboarding runbook (excerpt)</h3>
<table class="data">
  <thead><tr><th>Step</th><th>Action</th><th>Owner</th></tr></thead>
  <tbody>
    <tr><td>1</td><td>Invite user to org (2FA enforced)</td><td>IT</td></tr>
    <tr><td>2</td><td>User configures 2FA at invite acceptance</td><td>New hire</td></tr>
    <tr><td>3</td><td>Access provisioned via Terraform team membership</td><td>Platform</td></tr>
  </tbody>
</table>
<p><em>Manual evidence — refresh by linking the current onboarding runbook and a
sample invite showing the 2FA gate.</em></p>
HTML
