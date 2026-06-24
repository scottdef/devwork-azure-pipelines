#!/usr/bin/env bash
# Collector: SSH Keys
# Live mode uses the enterprise/org audit log (requires audit-log read scope).
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="ssh-keys"
st="$(is_sample && echo sample || echo reviewed)"

# --- logs-ssh-key-use (automatable) -----------------------------------------
emit "$C" logs-ssh-key-use "Security Engineering" \
  "GitHub audit log — git.clone/git.push over SSH" "$st" <<'HTML'
<p>SSH-based Git operations are visible in the GitHub audit log (and forwarded to
Dynatrace). Policy is HTTPS + short-lived tokens; SSH key usage is flagged.</p>
<pre><code>gh api "/orgs/$ORG/audit-log?phrase=transport_protocol:ssh" \
  --jq '.[] | [.created_at, .actor, .repo, .action] | @tsv'</code></pre>
<table class="data">
  <thead><tr><th>When</th><th>Actor</th><th>Repo</th><th>Action</th></tr></thead>
  <tbody>
    <tr><td>2026-06-19T14:02Z</td><td>a.dev</td><td>cla-core</td><td>git.clone (ssh)</td></tr>
  </tbody>
</table>
HTML

# --- alerting-ssh-key-use (automatable) -------------------------------------
emit "$C" alerting-ssh-key-use "Security Engineering" \
  "Dynatrace alerting rule" "$st" <<'HTML'
<p>A Dynatrace alerting rule fires when SSH transport is observed in GitHub logs,
notifying the security channel.</p>
<pre><code>fetch logs
| filter log.source == "github-audit" and transport_protocol == "ssh"
| // alert: notify #security when count() > 0</code></pre>
<p><em>Live runs link the alert configuration and most recent firing.</em></p>
HTML

# --- notify-and-remove (manual) ---------------------------------------------
emit_manual "$C" notify-and-remove "Security Engineering" \
  "Remediation log" "manual" <<'HTML'
<p>Users found using SSH keys are notified and their keys removed.</p>
<table class="data">
  <thead><tr><th>User</th><th>Detected</th><th>Notified</th><th>Key removed</th></tr></thead>
  <tbody>
    <tr><td>a.dev</td><td>2026-06-19</td><td>2026-06-19</td><td>2026-06-20</td></tr>
  </tbody>
</table>
<pre><code>gh api -X DELETE "/user/keys/$KEY_ID"   # after user notification</code></pre>
<p><em>Manual evidence — refresh from the SSH-key remediation log.</em></p>
HTML
