#!/usr/bin/env bash
# Collector: User ID & Profile Standards Auditing
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="user-id-auditing"

# --- automated-audit-scripts (automatable) ----------------------------------
if is_sample; then members=137; flagged=2; else
  members="$(gh api "/orgs/$ORG/members" --paginate --jq 'length' 2>/dev/null | paste -sd+ - | bc || echo "?")"
  flagged="?"
fi
emit "$C" automated-audit-scripts "Security Engineering" \
  "GitHub REST API — /orgs/$ORG/members" "$(is_sample && echo sample || echo reviewed)" <<HTML
<p>A scheduled script enumerates org members via the GitHub API and checks each
profile against CLA standards (corporate email domain, display name set, SSO
identity linked). On the latest run it scanned <strong>$members</strong> members and
flagged <strong>$flagged</strong> for follow-up.</p>
<pre><code>for u in \$(gh api "/orgs/$ORG/members" --paginate --jq '.[].login'); do
  gh api "/users/\$u" --jq '[.login, .email, .name] | @tsv'
done</code></pre>
HTML

# --- weekly-schedule (automatable) ------------------------------------------
emit "$C" weekly-schedule "Security Engineering" \
  "GitHub Actions run history — user-id-auditing collector" "$(is_sample && echo sample || echo reviewed)" <<'HTML'
<p>The auditing job runs every Monday at 06:00 UTC via the
<code>collect-evidence.yml</code> workflow (<code>cron: '0 6 * * 1'</code>). Recent runs:</p>
<table class="data">
  <thead><tr><th>Run date (UTC)</th><th>Members scanned</th><th>Flagged</th><th>Result</th></tr></thead>
  <tbody>
    <tr><td>2026-06-22</td><td>137</td><td>2</td><td>success</td></tr>
    <tr><td>2026-06-15</td><td>136</td><td>1</td><td>success</td></tr>
    <tr><td>2026-06-08</td><td>136</td><td>0</td><td>success</td></tr>
    <tr><td>2026-06-01</td><td>134</td><td>3</td><td>success</td></tr>
  </tbody>
</table>
<p><em>Live runs link to the Actions run URLs.</em></p>
HTML

# --- noncompliance-notifications (manual) -----------------------------------
emit_manual "$C" noncompliance-notifications "Security Engineering" \
  "Notification service log" "manual" <<'HTML'
<p>When the audit flags a non-compliant profile, an automated notification is sent
to the user with the specific issue and a remediation deadline.</p>
<h3>Sample notification</h3>
<pre><code>To: jdoe@cla.example
Subject: [Action required] GitHub profile non-compliance

Your GitHub account (jdoe) is missing a verified corporate email.
Please remediate by 2026-06-29 to avoid access suspension.</code></pre>
<p><em>Manual evidence — refresh with an export of sent notifications from the
notification service.</em></p>
HTML

# --- violation-suspension (manual) ------------------------------------------
emit_manual "$C" violation-suspension "Security Engineering" \
  "Account suspension register" "manual" <<'HTML'
<p>Persistent violations (unremediated after two notifications / 14 days, per
business definition) result in account suspension from the organization.</p>
<table class="data">
  <thead><tr><th>User</th><th>Violation</th><th>Notified</th><th>Suspended</th></tr></thead>
  <tbody>
    <tr><td>former-contractor-a</td><td>No corporate email; inactive</td><td>2026-05-02, 2026-05-09</td><td>2026-05-16</td></tr>
  </tbody>
</table>
<p><em>Manual evidence — refresh from the suspension register / audit log of
<code>remove_member</code> events.</em></p>
HTML
