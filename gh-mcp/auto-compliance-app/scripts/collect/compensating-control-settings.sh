#!/usr/bin/env bash
# Collector: Audit Compensating Control Settings
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="compensating-control-settings"
st="$(is_sample && echo sample || echo reviewed)"

# --- audit-script (automatable) ---------------------------------------------
emit "$C" audit-script "Security Engineering" \
  "control-*.yml workflows (compliance-as-code)" "$st" <<'HTML'
<p>Compensating control settings are audited as code by the
<code>control-*.yml</code> workflows, each asserting an expected GitHub
configuration and failing the run when reality drifts.</p>
<pre><code>two_factor=$(gh api "/orgs/$ORG" --jq '.two_factor_requirement_enabled')
[ "$two_factor" = "true" ] || { echo "::error::2FA not enforced"; exit 1; }</code></pre>
<p>Controls: org settings, repo visibility, repo rulesets, custom roles.</p>
HTML

# --- out-of-range-reporting (automatable) -----------------------------------
emit "$C" out-of-range-reporting "Security Engineering" \
  "control-*.yml run conclusions" "$st" <<'HTML'
<p>Out-of-expected values surface as failed control runs and are remediated. Latest
cycle:</p>
<table class="data">
  <thead><tr><th>Control</th><th>Setting</th><th>Expected</th><th>Found</th><th>Status</th></tr></thead>
  <tbody>
    <tr><td>org-settings</td><td>2FA required</td><td>true</td><td>true</td><td>pass</td></tr>
    <tr><td>repo-visibility</td><td>public repos</td><td>0</td><td>0</td><td>pass</td></tr>
    <tr><td>repo-rulesets</td><td>default branch protection</td><td>on</td><td>off → fixed</td><td>remediated</td></tr>
  </tbody>
</table>
HTML

# --- weekly-runs (automatable) ----------------------------------------------
emit "$C" weekly-runs "Security Engineering" \
  "GitHub Actions schedule" "$st" <<'HTML'
<p>The compliance-as-code audit runs on a weekly schedule
(<code>cron: '0 6 * * 1'</code>) plus on demand. Recent runs all completed
successfully; see the Actions history for run URLs.</p>
HTML
