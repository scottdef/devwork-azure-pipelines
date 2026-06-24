#!/usr/bin/env bash
# Collector: PAT Lifetime & Scope
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="pat-lifetime-scope"
st="$(is_sample && echo sample || echo reviewed)"

# --- max-lifetime-30-days (automatable) -------------------------------------
emit "$C" max-lifetime-30-days "Security Engineering" \
  "GitHub org PAT policy (personal-access-token-requests settings)" "$st" <<'HTML'
<p>Organization policy caps personal access token lifetime at <strong>30 days</strong>
and members cannot raise it. Fine-grained PATs against org resources require approval.</p>
<pre><code>gh api "/orgs/$ORG/settings/personal-access-token-requests" \
  --jq '{max_lifetime_days, enforced}'
# => { "max_lifetime_days": 30, "enforced": true }</code></pre>
HTML

# --- expiration-reminders (automatable) -------------------------------------
emit "$C" expiration-reminders "Security Engineering" \
  "Reminder script + notification log" "$st" <<'HTML'
<p>A scheduled script identifies tokens expiring within 7 days and emails the owner.</p>
<pre><code># tokens expiring in <= 7 days -> notify owner
for t in $(list_org_tokens --expiring 7d); do
  notify "$(token_owner "$t")" "PAT $t expires on $(token_expiry "$t")"
done</code></pre>
<p><em>Sample reminder:</em> "Your PAT <code>ci-deploy</code> expires 2026-06-30. Rotate via Terraform."</p>
HTML

# --- expired-pat-audits (automatable) ---------------------------------------
emit "$C" expired-pat-audits "Security Engineering" \
  "Expired-token audit output" "$st" <<'HTML'
<p>Expired tokens are audited and confirmed revoked. Latest audit:</p>
<table class="data">
  <thead><tr><th>Token</th><th>Owner</th><th>Expired</th><th>Revoked</th></tr></thead>
  <tbody>
    <tr><td>old-ci-token</td><td>svc-ci</td><td>2026-05-28</td><td>yes</td></tr>
    <tr><td>laptop-cli</td><td>a.dev</td><td>2026-06-01</td><td>yes</td></tr>
  </tbody>
</table>
HTML

# --- automated-weekly-scan (automatable) ------------------------------------
emit "$C" automated-weekly-scan "Security Engineering" \
  "GitHub Actions schedule — PAT scan" "$st" <<'HTML'
<p>The expired-PAT scan runs weekly (<code>cron: '0 6 * * 1'</code>); output is
reviewed and any changes (revocations, policy updates) are recorded with the run.</p>
HTML

# --- admin-scope-justification (manual) -------------------------------------
emit_manual "$C" admin-scope-justification "Security Engineering" \
  "PAT approval register" "manual" <<'HTML'
<p>Any PAT carrying admin scopes has a documented business justification and approval.</p>
<table class="data">
  <thead><tr><th>Token</th><th>Scope</th><th>Justification</th><th>Approved by</th></tr></thead>
  <tbody>
    <tr><td>org-terraform</td><td>admin:org</td><td>IaC org management</td><td>CISO, 2026-04-10</td></tr>
  </tbody>
</table>
<p><em>Manual evidence — refresh from the PAT approval register.</em></p>
HTML

# --- fine-grained-admin-approval (manual) -----------------------------------
emit_manual "$C" fine-grained-admin-approval "Security Engineering" \
  "Fine-grained PAT approvals" "manual" <<'HTML'
<p>All fine-grained PATs targeting org resources are approved by an admin via the
organization's PAT approval flow.</p>
<pre><code>gh api "/orgs/$ORG/personal-access-tokens" \
  --jq '.[] | [.owner.login, .token_name, .access_granted_at] | @tsv'</code></pre>
<p><em>Manual evidence — refresh with the approvals export + reviewer sign-off.</em></p>
HTML

# --- terraform-org-config (manual) ------------------------------------------
emit_manual "$C" terraform-org-config "Platform Engineering" \
  "Terraform — github_organization_settings" "manual" <<'HTML'
<p>PAT policy is configured via Terraform at the org level (settings as code):</p>
<pre><code>resource "github_organization_settings" "cla" {
  # personal access tokens
  personal_access_token_max_lifetime_days = 30
  personal_access_tokens_enabled          = true
}</code></pre>
<p><em>Manual evidence — refresh by pinning the Terraform file/commit and a clean
<code>terraform plan</code>.</em></p>
HTML
