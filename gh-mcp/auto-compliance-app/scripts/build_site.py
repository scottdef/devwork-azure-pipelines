#!/usr/bin/env python3
"""
Build the Compliance Evidence Center static site.

Produces a pure-static, no-JavaScript site that works fully offline. The site is
an Apache/FTP-style directory browser over compliance controls and their
evidence:

    index.html                          home / control index
    controls/index.html                 directory listing of all controls
    controls/<control>/index.html       control report (lists evidence items)
    controls/<control>/evidence/        directory listing + one HTML doc per
                                        evidence requirement (templated)
    controls/<control>.zip              downloadable bundle of the control folder

Every page is self-contained (CSS is inlined) so any single file or any
downloaded/extracted zip renders correctly with no network access.

The control + evidence model is derived directly from comp-cons.md. Edit CONTROLS
below to change what the site contains, then re-run this script.

Usage:
    python3 scripts/build_site.py [--out _site]
"""

from __future__ import annotations

import argparse
import datetime as _dt
import html
import os
import shutil
import zipfile
from dataclasses import dataclass, field

# --------------------------------------------------------------------------- #
# Data model (source of truth: comp-cons.md)
# --------------------------------------------------------------------------- #


@dataclass
class Evidence:
    slug: str
    title: str
    requirement: str  # the verbatim requirement text from comp-cons.md


@dataclass
class Control:
    slug: str
    title: str
    summary: str
    evidence: list[Evidence] = field(default_factory=list)


CONTROLS: list[Control] = [
    Control(
        slug="user-id-auditing",
        title="User ID & Profile Standards Auditing",
        summary=(
            "Automated, scheduled auditing of GitHub user identities and profile "
            "standards via the GitHub API, including notification and enforcement "
            "for non-compliant accounts."
        ),
        evidence=[
            Evidence(
                "automated-audit-scripts",
                "Automated auditing scripts (GitHub API)",
                "Evidence of the automated auditing scripts via GitHub API.",
            ),
            Evidence(
                "weekly-schedule",
                "Weekly automated execution",
                "Evidence the scripts have been running on an automated basis every week.",
            ),
            Evidence(
                "noncompliance-notifications",
                "Automatic non-compliance notifications",
                "Evidence of the automatic notifications sent to users when "
                "non-compliance is identified.",
            ),
            Evidence(
                "violation-suspension",
                "Suspension for persistent violations",
                "Evidence that persistent violations (as defined by the business) "
                "resulted in suspension of the account.",
            ),
        ],
    ),
    Control(
        slug="multi-factor-authentication",
        title="Multi-Factor Authentication",
        summary=(
            "Mandatory two-factor authentication for all accounts with access to "
            "CLA's GitHub, enforced so it cannot be disabled and applied to new "
            "users by default."
        ),
        evidence=[
            Evidence(
                "all-accounts-2fa",
                "All accounts have 2FA configured",
                "Evidence demonstrating all accounts with access to CLA's GitHub "
                "have 2FA configured for their account.",
            ),
            Evidence(
                "cannot-disable",
                "Users cannot disable 2FA",
                "Evidence demonstrating that users cannot turn this setting off.",
            ),
            Evidence(
                "new-user-default",
                "2FA enforced for new users",
                "Evidence demonstrating that new users have this setting "
                "automatically set, and/or process documentation showing this is a "
                "requirement for setting up new users.",
            ),
        ],
    ),
    Control(
        slug="repository-visibility",
        title="Repository Visibility",
        summary=(
            "All repositories restricted to internal or private, with default "
            "visibility enforced via Terraform IaC, UI-based repo creation "
            "disabled, and automated visibility scanning with alerting."
        ),
        evidence=[
            Evidence(
                "all-internal-or-private",
                "All repos internal or private",
                'Evidence that all repos are set to "internal" or "private".',
            ),
            Evidence(
                "default-visibility-iac",
                "Default visibility via Terraform IaC",
                "Evidence that default visibility for CLA repos is set to Internal "
                "or Private via Terraform IaC.",
            ),
            Evidence(
                "no-ui-repo-creation",
                "Users cannot create repos via UI",
                "Evidence showing individual users are not able to create repos via "
                "the GitHub UI.",
            ),
            Evidence(
                "automated-scans-alerting",
                "Automated visibility scans & alerting",
                "Evidence of automated scans for visibility across all CLA repos and "
                "associated alerting / monitoring for non-compliance.",
            ),
        ],
    ),
    Control(
        slug="repository-permissions",
        title="Repository Permissions",
        summary=(
            "Periodic (quarterly) audits of repository permissions, including "
            "remediation changes made as a result of each audit."
        ),
        evidence=[
            Evidence(
                "quarterly-audits",
                "Quarterly permission audits",
                "Evidence of quarterly audits of permissions, including any changes "
                "that were made as a result of the quarterly audit.",
            ),
        ],
    ),
    Control(
        slug="dynatrace-logs-integration",
        title="Dynatrace Logs Integration",
        summary=(
            "GitHub logs forwarded to Dynatrace, queryable for audit/debug/"
            "compliance purposes, with retention of at least 12 months."
        ),
        evidence=[
            Evidence(
                "forwarding-configured",
                "Log forwarding configured",
                "Evidence that logs are configured to be forwarded from GitHub to "
                "Dynatrace.",
            ),
            Evidence(
                "querying-available",
                "Logs queryable in Dynatrace",
                "Evidence that querying is available for the GitHub logs in Dynatrace "
                "for auditing, debugging and compliance.",
            ),
            Evidence(
                "retention-12-months",
                "Retention >= 12 months",
                "Evidence that retention policies are set for >= 12 months for GitHub "
                "logs in Dynatrace.",
            ),
        ],
    ),
    Control(
        slug="compensating-control-settings",
        title="Audit Compensating Control Settings",
        summary=(
            "Compliance-as-code auditing of compensating control settings, with "
            "out-of-range values reported and remediated, run on a weekly schedule."
        ),
        evidence=[
            Evidence(
                "audit-script",
                "Compliance-as-code audit script",
                'Evidence of the script set to audit the compensating control '
                'settings to establish "Compliance as Code".',
            ),
            Evidence(
                "out-of-range-reporting",
                "Out-of-range reporting & remediation",
                "Evidence that out-of-expected values are reported as a result of the "
                "automated script and remediated as needed.",
            ),
            Evidence(
                "weekly-runs",
                "Weekly automated execution",
                "Evidence showing the automated script runs weekly.",
            ),
        ],
    ),
    Control(
        slug="pat-lifetime-scope",
        title="PAT Lifetime & Scope",
        summary=(
            "Personal access tokens constrained by policy: a 30-day maximum lifetime "
            "that users cannot override, documented approval for admin and "
            "fine-grained scopes, Terraform-managed org configuration, expiration "
            "reminders, and automated auditing of expired tokens."
        ),
        evidence=[
            Evidence(
                "max-lifetime-30-days",
                "30-day max lifetime, not user-changeable",
                "Evidence that PAT configuration is set to max lifetime of 30 days and "
                "this config can't be changed by individual users.",
            ),
            Evidence(
                "admin-scope-justification",
                "Admin-scope PATs justified & approved",
                "Evidence that any PATs with admin scopes have documented "
                "justification and approval.",
            ),
            Evidence(
                "fine-grained-admin-approval",
                "Fine-grained PATs admin-approved",
                "Evidence that all fine-grained PATs have documented admin approval.",
            ),
            Evidence(
                "terraform-org-config",
                "PATs configured via Terraform at org level",
                "Evidence that PATs must be configured via Terraform at the "
                "organization level.",
            ),
            Evidence(
                "expiration-reminders",
                "Expiration reminders",
                "Evidence of expiration reminders, including the automated script and "
                "the reminder sent to individuals.",
            ),
            Evidence(
                "expired-pat-audits",
                "Audits of expired PATs",
                "Evidence of audits of the expired PATs.",
            ),
            Evidence(
                "automated-weekly-scan",
                "Weekly automated expired-PAT scan",
                "Evidence of the automated weekly script running to identify expired "
                "PATs and review of output, with any changes identified.",
            ),
        ],
    ),
    Control(
        slug="oauth-github-app-restrictions",
        title="OAuth App & GitHub App Restrictions",
        summary=(
            "OAuth and GitHub App installations restricted to an approved allowlist "
            "with documented admin approval, and installations captured in audit logs "
            "and monitored for non-compliance."
        ),
        evidence=[
            Evidence(
                "apps-not-allowed",
                "App installation restricted",
                "Evidence that users are not allowed to install and use OAuth and/or "
                "GitHub apps.",
            ),
            Evidence(
                "documented-app-approval",
                "Installed apps have admin approval",
                "Evidence that, if apps are installed and used, they have documented "
                "approval from an admin.",
            ),
            Evidence(
                "whitelisted-apps",
                "Allowlisted applications only",
                'Evidence of the "whitelisted" applications and configuration to allow '
                "only those.",
            ),
            Evidence(
                "audit-log-monitoring",
                "App installs logged & monitored",
                "Evidence that (1) GitHub audit logs capture app installations and "
                "(2) these are monitored and addressed when non-compliance arises.",
            ),
        ],
    ),
    Control(
        slug="ssh-keys",
        title="SSH Keys",
        summary=(
            "SSH key usage surfaced in GitHub logs, alerted on, and remediated by "
            "notifying users and removing their keys."
        ),
        evidence=[
            Evidence(
                "logs-ssh-key-use",
                "Logs showing SSH key use",
                "Evidence of GitHub logs showing the use of SSH keys.",
            ),
            Evidence(
                "alerting-ssh-key-use",
                "Alerting on SSH key use",
                "Evidence of the alerting when SSH key use is identified.",
            ),
            Evidence(
                "notify-and-remove",
                "Users notified & keys removed",
                "Evidence showing users leveraging SSH keys were notified and removed "
                "keys.",
            ),
        ],
    ),
]

SITE_TITLE = "Compliance Evidence Center"
SITE_SUBTITLE = "CLA GitHub Security & Compliance — control evidence repository"

# Real evidence content lives here (kept out of generated output). For each
# evidence item the build looks for an optional HTML body fragment and an
# optional `.meta` sidecar; if absent it renders the TODO placeholder.
#
#   evidence/<control-slug>/<evidence-slug>.html   body fragment (injected)
#   evidence/<control-slug>/<evidence-slug>.meta   key: value provenance
#   evidence/<control-slug>/<anything-else>        assets, copied verbatim
#
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
EVIDENCE_SRC = os.path.join(REPO_ROOT, "evidence")

# --------------------------------------------------------------------------- #
# Rendering
# --------------------------------------------------------------------------- #

CSS = """
:root {
  --bg: #f6f8fa; --panel: #ffffff; --ink: #1f2328; --muted: #656d76;
  --line: #d0d7de; --accent: #0969da; --accent-bg: #ddf4ff; --code: #f6f8fa;
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--bg); color: var(--ink);
  font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
}
.wrap { max-width: 960px; margin: 0 auto; padding: 24px 20px 64px; }
header.site { border-bottom: 1px solid var(--line); margin-bottom: 20px; padding-bottom: 14px; }
header.site h1 { margin: 0 0 2px; font-size: 20px; }
header.site .sub { color: var(--muted); font-size: 13px; }
nav.crumbs { font-size: 13px; margin-bottom: 18px; color: var(--muted); word-break: break-word; }
nav.crumbs a { color: var(--accent); text-decoration: none; }
nav.crumbs a:hover { text-decoration: underline; }
h2 { font-size: 17px; margin: 26px 0 10px; }
p.lede { color: var(--muted); margin-top: 4px; }
.panel { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
table.listing { width: 100%; border-collapse: collapse; font-size: 14px; }
table.listing th, table.listing td { text-align: left; padding: 9px 14px; border-bottom: 1px solid var(--line); }
table.listing th { background: var(--code); color: var(--muted); font-weight: 600; font-size: 12px; text-transform: uppercase; letter-spacing: .03em; }
table.listing tr:last-child td { border-bottom: none; }
table.listing td.name a { color: var(--accent); text-decoration: none; font-weight: 500; }
table.listing td.name a:hover { text-decoration: underline; }
table.listing td.meta { color: var(--muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
.ico { display: inline-block; width: 1.3em; }
.actions a {
  display: inline-block; font-size: 13px; padding: 2px 8px; margin-right: 6px;
  border: 1px solid var(--line); border-radius: 6px; color: var(--accent);
  text-decoration: none; background: var(--panel);
}
.actions a:hover { background: var(--accent-bg); }
.btn {
  display: inline-block; background: var(--accent); color: #fff !important; border-radius: 6px;
  padding: 7px 14px; text-decoration: none; font-size: 14px; font-weight: 500;
}
.btn:hover { filter: brightness(1.08); }
.btn.secondary { background: var(--panel); color: var(--accent) !important; border: 1px solid var(--line); }
.req { background: var(--code); border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 6px; padding: 12px 14px; margin: 14px 0; }
.req .label { font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: var(--muted); }
dl.fields { display: grid; grid-template-columns: 180px 1fr; gap: 0; margin: 0; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
dl.fields dt { background: var(--code); padding: 9px 14px; font-weight: 600; font-size: 13px; border-bottom: 1px solid var(--line); }
dl.fields dd { margin: 0; padding: 9px 14px; border-bottom: 1px solid var(--line); color: var(--muted); }
dl.fields dt:last-of-type, dl.fields dd:last-of-type { border-bottom: none; }
.todo { border: 1px dashed var(--line); border-radius: 8px; padding: 18px; text-align: center; color: var(--muted); background: var(--code); margin: 14px 0; }
.todo strong { color: var(--ink); }
.pill { display: inline-block; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .04em; padding: 2px 8px; border-radius: 999px; vertical-align: middle; }
.pill.ok { background: #dafbe1; color: #1a7f37; }
.pill.pending { background: #fff1cc; color: #9a6700; }
pre { background: var(--code); border: 1px solid var(--line); border-radius: 6px; padding: 12px 14px; overflow-x: auto; font-size: 13px; line-height: 1.45; }
pre code { background: none; padding: 0; }
table.data { width: 100%; border-collapse: collapse; font-size: 14px; margin: 12px 0; }
table.data th, table.data td { border: 1px solid var(--line); padding: 6px 10px; text-align: left; }
table.data th { background: var(--code); }
img.evidence { max-width: 100%; border: 1px solid var(--line); border-radius: 6px; }
footer.site { margin-top: 40px; padding-top: 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 12px; }
code { background: var(--code); padding: 1px 5px; border-radius: 4px; font-size: 13px; }
@media (max-width: 600px) {
  dl.fields { grid-template-columns: 1fr; }
  table.listing th.hide, table.listing td.hide { display: none; }
}
""".strip()


def page(title: str, crumbs_html: str, body: str, *, generated: str) -> str:
    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{html.escape(title)}</title>
<style>{CSS}</style>
</head>
<body>
<div class="wrap">
<header class="site">
  <h1>{html.escape(SITE_TITLE)}</h1>
  <div class="sub">{html.escape(SITE_SUBTITLE)}</div>
</header>
<nav class="crumbs">{crumbs_html}</nav>
{body}
<footer class="site">
  Static evidence repository &middot; built {generated} &middot;
  served via GitHub Pages &middot; works offline once downloaded.
</footer>
</div>
</body>
</html>
"""


def crumbs(*parts: tuple[str, str | None]) -> str:
    out = []
    for label, href in parts:
        if href:
            out.append(f'<a href="{href}">{html.escape(label)}</a>')
        else:
            out.append(html.escape(label))
    return " / ".join(out)


def fmt_size(n: int) -> str:
    if n < 1024:
        return f"{n} B"
    if n < 1024 * 1024:
        return f"{n / 1024:.1f} KB"
    return f"{n / (1024 * 1024):.1f} MB"


# --------------------------------------------------------------------------- #
# Evidence source loading (real content) + document rendering
# --------------------------------------------------------------------------- #


def parse_meta(path: str) -> dict[str, str]:
    """Parse a simple `key: value` provenance sidecar (blank/`#` lines ignored)."""
    meta: dict[str, str] = {}
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#") or ":" not in line:
                continue
            key, val = line.split(":", 1)
            meta[key.strip().lower()] = val.strip()
    return meta


def load_evidence_source(control: Control, ev: Evidence) -> tuple[str | None, dict[str, str]]:
    """Return (body_fragment_html_or_None, meta_dict) for an evidence item."""
    base = os.path.join(EVIDENCE_SRC, control.slug)
    frag = os.path.join(base, ev.slug + ".html")
    metap = os.path.join(base, ev.slug + ".meta")
    body = None
    if os.path.isfile(frag):
        with open(frag, encoding="utf-8") as fh:
            body = fh.read()
    meta = parse_meta(metap) if os.path.isfile(metap) else {}
    return body, meta


def _prov(meta: dict[str, str], key: str, default: str) -> str:
    return html.escape(meta.get(key, default))


def ev_documented(control: Control, ev: Evidence) -> bool:
    return os.path.isfile(os.path.join(EVIDENCE_SRC, control.slug, ev.slug + ".html"))


def render_evidence(
    control: Control,
    ev: Evidence,
    generated: str,
    source_html: str | None = None,
    meta: dict[str, str] | None = None,
) -> str:
    meta = meta or {}
    documented = source_html is not None
    crumbs_html = crumbs(
        ("Home", "../../../index.html"),
        ("Controls", "../../index.html"),
        (control.title, "../index.html"),
        ("Evidence", "index.html"),
        (ev.title, None),
    )

    if documented:
        pill = '<span class="pill ok">documented</span>'
        content = f"""
<h2>Evidence</h2>
{source_html}
"""
        default_status = meta.get("status", "documented")
    else:
        pill = '<span class="pill pending">pending</span>'
        content = """
<h2>Evidence summary</h2>
<div class="todo">
  <strong>TODO — attach evidence.</strong><br>
  Replace this section with the evidence that satisfies the requirement above:
  narrative description, links, and the supporting artifact(s).
</div>

<h2>Supporting artifact</h2>
<div class="todo">
  <strong>TODO — embed screenshot / data export.</strong><br>
  Drop a screenshot (<code>&lt;img&gt;</code>), an exported table, or paste the raw
  API/script output here. Keep artifacts inside this folder so the evidence
  remains self-contained and downloadable for offline review.
</div>
"""
        default_status = "pending"

    body = f"""
<h2>{html.escape(ev.title)} {pill}</h2>
<p class="lede">{html.escape(control.title)}</p>

<div class="req">
  <div class="label">Requirement</div>
  {html.escape(ev.requirement)}
</div>
{content}
<h2>Provenance</h2>
<dl class="fields">
  <dt>Control</dt><dd>{html.escape(control.title)}</dd>
  <dt>Evidence ID</dt><dd><code>{html.escape(control.slug)}/{html.escape(ev.slug)}</code></dd>
  <dt>Collection date</dt><dd>{_prov(meta, "date", "TODO")}</dd>
  <dt>Collected by</dt><dd>{_prov(meta, "by", "TODO")}</dd>
  <dt>Source / system</dt><dd>{_prov(meta, "source", "TODO (e.g. GitHub API, Terraform plan, Dynatrace query)")}</dd>
  <dt>Review status</dt><dd>{_prov(meta, "status", default_status)}</dd>
</dl>
"""
    return page(f"{ev.title} — {control.title}", crumbs_html, body, generated=generated)


# --------------------------------------------------------------------------- #
# Listing helpers (Apache/FTP-style directory tables)
# --------------------------------------------------------------------------- #


def listing_row(icon: str, name: str, href: str, size: str, modified: str, actions: str = "") -> str:
    return (
        f'<tr>'
        f'<td class="name"><span class="ico">{icon}</span>'
        f'<a href="{href}">{html.escape(name)}</a></td>'
        f'<td class="meta hide">{size}</td>'
        f'<td class="meta hide">{modified}</td>'
        f'<td class="actions">{actions}</td>'
        f'</tr>'
    )


def listing_table(rows: list[str]) -> str:
    return f"""
<div class="panel">
<table class="listing">
  <thead><tr>
    <th>Name</th><th class="hide">Size</th><th class="hide">Modified</th><th>Actions</th>
  </tr></thead>
  <tbody>
    {''.join(rows)}
  </tbody>
</table>
</div>
"""


# --------------------------------------------------------------------------- #
# Build
# --------------------------------------------------------------------------- #


def build(out_dir: str) -> None:
    now = _dt.datetime.now(_dt.timezone.utc)
    generated = now.strftime("%Y-%m-%d %H:%M UTC")
    date_only = now.strftime("%Y-%m-%d")

    if os.path.exists(out_dir):
        shutil.rmtree(out_dir)
    os.makedirs(out_dir)

    # Tell GitHub Pages not to run Jekyll (keeps folders/files served verbatim).
    with open(os.path.join(out_dir, ".nojekyll"), "w") as fh:
        fh.write("")

    controls_dir = os.path.join(out_dir, "controls")
    os.makedirs(controls_dir)

    # ---- per-control pages + evidence docs -------------------------------- #
    documented_counts: dict[str, int] = {}
    for control in CONTROLS:
        cdir = os.path.join(controls_dir, control.slug)
        edir = os.path.join(cdir, "evidence")
        os.makedirs(edir)

        # copy any source assets (screenshots, exports, etc.) into the output so
        # injected fragments can reference them relatively and they ship in the zip.
        src_ctrl_dir = os.path.join(EVIDENCE_SRC, control.slug)
        consumed = {f"{ev.slug}.html" for ev in control.evidence}
        consumed |= {f"{ev.slug}.meta" for ev in control.evidence}
        if os.path.isdir(src_ctrl_dir):
            for entry in os.listdir(src_ctrl_dir):
                if entry in consumed:
                    continue
                s = os.path.join(src_ctrl_dir, entry)
                d = os.path.join(edir, entry)
                if os.path.isdir(s):
                    shutil.copytree(s, d, dirs_exist_ok=True)
                else:
                    shutil.copy2(s, d)

        # evidence documents (inject real source content when present)
        documented = 0
        for i, ev in enumerate(control.evidence, 1):
            source_html, meta = load_evidence_source(control, ev)
            if source_html is not None:
                documented += 1
            fname = f"{i:02d}-{ev.slug}.html"
            with open(os.path.join(edir, fname), "w") as fh:
                fh.write(render_evidence(control, ev, generated, source_html, meta))
        documented_counts[control.slug] = documented

        # evidence directory listing
        ev_rows = [
            listing_row(
                "✅" if ev_documented(control, ev) else "📄",
                f"{i:02d}-{ev.slug}.html",
                f"{i:02d}-{ev.slug}.html",
                fmt_size(os.path.getsize(os.path.join(edir, f"{i:02d}-{ev.slug}.html"))),
                date_only,
                f'<a href="{i:02d}-{ev.slug}.html">view</a>'
                f'<a href="{i:02d}-{ev.slug}.html" download>download</a>',
            )
            for i, ev in enumerate(control.evidence, 1)
        ]
        ev_listing = listing_table(ev_rows)
        ev_crumbs = crumbs(
            ("Home", "../../../index.html"),
            ("Controls", "../../index.html"),
            (control.title, "../index.html"),
            ("Evidence", None),
        )
        with open(os.path.join(edir, "index.html"), "w") as fh:
            fh.write(
                page(
                    f"Evidence — {control.title}",
                    ev_crumbs,
                    f'<h2>Evidence files</h2><p class="lede">{html.escape(control.title)}</p>{ev_listing}',
                    generated=generated,
                )
            )

        # control report (index of the control folder)
        report_rows = []
        for i, ev in enumerate(control.evidence, 1):
            href = f"evidence/{i:02d}-{ev.slug}.html"
            report_rows.append(
                listing_row(
                    "✅" if ev_documented(control, ev) else "📄",
                    ev.title,
                    href,
                    fmt_size(os.path.getsize(os.path.join(edir, f"{i:02d}-{ev.slug}.html"))),
                    date_only,
                    f'<a href="{href}">view</a><a href="{href}" download>download</a>',
                )
            )
        report_crumbs = crumbs(
            ("Home", "../../index.html"),
            ("Controls", "../index.html"),
            (control.title, None),
        )
        report_body = f"""
<h2>{html.escape(control.title)}</h2>
<p class="lede">{html.escape(control.summary)}</p>
<p>
  <a class="btn" href="../{control.slug}.zip" download>⬇ Download all evidence (.zip)</a>
  <a class="btn secondary" href="evidence/index.html">Browse evidence folder →</a>
</p>
<h2>Required evidence ({documented_counts[control.slug]}/{len(control.evidence)} documented)</h2>
{listing_table(report_rows)}
"""
        with open(os.path.join(cdir, "index.html"), "w") as fh:
            fh.write(page(control.title, report_crumbs, report_body, generated=generated))

    # ---- per-control zip bundles ------------------------------------------ #
    for control in CONTROLS:
        cdir = os.path.join(controls_dir, control.slug)
        zip_path = os.path.join(controls_dir, f"{control.slug}.zip")
        with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
            for root, _dirs, files in os.walk(cdir):
                for f in files:
                    full = os.path.join(root, f)
                    arc = os.path.join(control.slug, os.path.relpath(full, cdir))
                    zf.write(full, arc)

    # ---- controls directory listing --------------------------------------- #
    ctrl_rows = []
    for control in CONTROLS:
        zip_size = fmt_size(os.path.getsize(os.path.join(controls_dir, f"{control.slug}.zip")))
        ctrl_rows.append(
            listing_row(
                "📁",
                control.title,
                f"{control.slug}/index.html",
                f"{documented_counts[control.slug]}/{len(control.evidence)} documented",
                date_only,
                f'<a href="{control.slug}/index.html">open</a>'
                f'<a href="{control.slug}.zip" download>zip ({zip_size})</a>',
            )
        )
    controls_listing = listing_table(ctrl_rows)
    with open(os.path.join(controls_dir, "index.html"), "w") as fh:
        fh.write(
            page(
                "Controls",
                crumbs(("Home", "../index.html"), ("Controls", None)),
                f"<h2>Compliance controls</h2>{controls_listing}",
                generated=generated,
            )
        )

    # ---- home page -------------------------------------------------------- #
    total_ev = sum(len(c.evidence) for c in CONTROLS)
    # Links on the home page are one level up from the controls listing, so they
    # need a "controls/" prefix.
    home_rows = []
    for control in CONTROLS:
        zip_size = fmt_size(os.path.getsize(os.path.join(controls_dir, f"{control.slug}.zip")))
        home_rows.append(
            listing_row(
                "📁",
                control.title,
                f"controls/{control.slug}/index.html",
                f"{documented_counts[control.slug]}/{len(control.evidence)} documented",
                date_only,
                f'<a href="controls/{control.slug}/index.html">open</a>'
                f'<a href="controls/{control.slug}.zip" download>zip ({zip_size})</a>',
            )
        )
    home_body = f"""
<p class="lede">
  Browse, view, and download compliance evidence for each control. Every report
  is a self-contained folder of HTML evidence files; download a control's
  <code>.zip</code> for a complete offline copy.
</p>
<h2>Controls ({len(CONTROLS)}) &middot; {sum(documented_counts.values())}/{total_ev} evidence items documented</h2>
{listing_table(home_rows)}
<p style="margin-top:18px"><a class="btn secondary" href="controls/index.html">Open full directory listing →</a></p>
"""
    with open(os.path.join(out_dir, "index.html"), "w") as fh:
        fh.write(
            page(
                SITE_TITLE,
                crumbs(("Home", None)),
                home_body,
                generated=generated,
            )
        )

    print(f"Built {len(CONTROLS)} controls / {total_ev} evidence files into {out_dir}/")


def main() -> None:
    ap = argparse.ArgumentParser(description="Build the Compliance Evidence Center site.")
    ap.add_argument("--out", default="_site", help="output directory (default: _site)")
    args = ap.parse_args()
    here = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    out = args.out if os.path.isabs(args.out) else os.path.join(here, args.out)
    build(out)


if __name__ == "__main__":
    main()
