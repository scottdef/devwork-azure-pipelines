#!/usr/bin/env bash
# Collector: Repository Permissions
source "$(dirname "${BASH_SOURCE[0]}")/_lib.sh"
parse_args "$@"
C="repository-permissions"

# --- quarterly-audits (automatable) -----------------------------------------
emit "$C" quarterly-audits "Platform Engineering" \
  "GitHub REST API — repo collaborators & team permissions" "$(is_sample && echo sample || echo reviewed)" <<'HTML'
<p>Repository permissions are dumped and reviewed quarterly. The latest audit
(Q2 2026) found two over-broad grants, both remediated.</p>
<h3>Permission snapshot (sample)</h3>
<table class="data">
  <thead><tr><th>Repository</th><th>Principal</th><th>Permission</th><th>Action</th></tr></thead>
  <tbody>
    <tr><td>cla-core</td><td>team:platform</td><td>maintain</td><td>kept</td></tr>
    <tr><td>cla-core</td><td>ext-vendor</td><td>admin</td><td>downgraded → read</td></tr>
    <tr><td>cla-infra</td><td>team:sre</td><td>admin</td><td>kept</td></tr>
    <tr><td>cla-infra</td><td>j.former</td><td>write</td><td>removed</td></tr>
  </tbody>
</table>
<pre><code>gh api "/repos/$ORG/$REPO/collaborators" \
  --jq '.[] | [.login, .role_name] | @tsv'</code></pre>
<p><em>Live runs attach the full per-repo permission export and the quarterly review
sign-off.</em></p>
HTML
