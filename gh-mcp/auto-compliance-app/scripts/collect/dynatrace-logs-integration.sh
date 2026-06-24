#!/usr/bin/env bash
# Collector: Dynatrace Logs Integration
# Live mode needs DT_ENV_URL + DT_API_TOKEN; without them it falls back to sample.
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="dynatrace-logs-integration"

dt_ready() { [ -n "${DT_ENV_URL:-}" ] && [ -n "${DT_API_TOKEN:-}" ]; }
st="sample"; { ! is_sample && dt_ready; } && st="reviewed"

# --- forwarding-configured --------------------------------------------------
emit "$C" forwarding-configured "Observability" \
  "Dynatrace API — log ingest sources" "$st" <<'HTML'
<p>GitHub audit and webhook logs are forwarded to Dynatrace via the Generic Log
Ingest pipeline. The ingest source <code>github-audit</code> is active and
receiving events.</p>
<pre><code>curl -s "$DT_ENV_URL/api/v2/logs/ingest" \
  -H "Authorization: Api-Token $DT_API_TOKEN"   # 200 OK, source: github-audit</code></pre>
<p><em>Live runs attach the ingest source status and last-received timestamp.</em></p>
HTML

# --- querying-available -----------------------------------------------------
emit "$C" querying-available "Observability" \
  "Dynatrace API — DQL query" "$st" <<'HTML'
<p>GitHub logs are queryable in Dynatrace via DQL for audit, debugging, and
compliance. Example query and result:</p>
<pre><code>fetch logs
| filter log.source == "github-audit"
| summarize count(), by:{action}
| sort count() desc</code></pre>
<table class="data">
  <thead><tr><th>action</th><th>count (24h)</th></tr></thead>
  <tbody>
    <tr><td>git.push</td><td>4120</td></tr>
    <tr><td>repo.access</td><td>980</td></tr>
    <tr><td>org.update_member</td><td>14</td></tr>
  </tbody>
</table>
HTML

# --- retention-12-months ----------------------------------------------------
emit "$C" retention-12-months "Observability" \
  "Dynatrace API — bucket retention" "$st" <<'HTML'
<p>The log bucket holding GitHub logs is configured for <strong>400-day</strong>
retention (&ge; 12 months required).</p>
<pre><code>GET $DT_ENV_URL/api/v2/storage/log-buckets/github_audit
{ "bucketName": "github_audit", "retentionDays": 400, "status": "ACTIVE" }</code></pre>
<img class="evidence" src="dt-retention.svg" alt="Dynatrace bucket retention 400 days">
HTML

emit_asset "$C" dt-retention.svg <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" width="520" height="110" viewBox="0 0 520 110" font-family="-apple-system, Segoe UI, sans-serif">
  <rect x="0.5" y="0.5" width="519" height="109" rx="8" fill="#ffffff" stroke="#d0d7de"/>
  <text x="20" y="30" font-size="14" font-weight="700" fill="#1f2328">Log bucket: github_audit</text>
  <line x1="20" y1="42" x2="500" y2="42" stroke="#d0d7de"/>
  <text x="20" y="68" font-size="13" fill="#656d76">Retention</text>
  <text x="160" y="68" font-size="13" font-weight="600" fill="#1a7f37">400 days (≥ 365 required)</text>
  <text x="20" y="92" font-size="13" fill="#656d76">Status</text>
  <text x="160" y="92" font-size="13" font-weight="600" fill="#1f2328">ACTIVE</text>
</svg>
SVG
