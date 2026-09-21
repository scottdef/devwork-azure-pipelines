#!/usr/bin/env python3
"""render.py - report-data.json in, one self-contained HTML file out.

usage: render.py REPORT_DATA.json OUT.html [INSIGHTS.json]

No network, no fonts, no libraries, no build step. Charts are SVG drawn here, so the
page is complete with JavaScript off; script only sorts tables and exports CSV.
INSIGHTS.json is {"headline": str, "findings": str, "recommendations": str}, written by
the agent. It is escaped before anything else happens to it.
"""
import html
import json
import re
import sys

TIERS = ("power", "heavy", "medium", "light", "inactive")
esc = html.escape


def n(v, nd=0):
    return "n/a" if v is None else f"{v:,.{nd}f}"


def usd(v):
    return "n/a" if v is None else f"${v:,.2f}"


def pct(v):
    return "n/a" if v is None else f"{v * 100:.0f}%"


def md_lite(text):
    """Escape first, then allow paragraphs, '- ' lists, **bold**, `code`. Nothing else."""
    out, items = [], []

    def inline(s):
        s = re.sub(r"\*\*(.+?)\*\*", r"<strong>\1</strong>", esc(s))
        return re.sub(r"`([^`]+)`", r"<code>\1</code>", s)

    def flush():
        if items:
            out.append("<ul>" + "".join(f"<li>{i}</li>" for i in items) + "</ul>")
            items.clear()

    for line in (text or "").strip().splitlines():
        line = line.strip()
        if line.startswith(("- ", "* ")):
            items.append(inline(line[2:]))
        else:
            flush()
            if line:
                out.append(f"<p>{inline(line.lstrip('# '))}</p>")
    flush()
    return "\n".join(out)


# ---------------------------------------------------------------- charts

def day_bars(daily, key, fmt, label):
    """One row of vertical bars per day. Weekends and gaps stay visible as absences."""
    if not daily:
        return ""
    w, h, pad, base = 1000, 150, 46, 118
    top = max(d[key] for d in daily) or 1
    step = (w - pad - 8) / len(daily)
    bars, ticks = [], []
    for i, d in enumerate(daily):
        bh = (d[key] / top) * (base - 14)
        x = pad + i * step
        bars.append(f'<rect x="{x + step * .15:.1f}" y="{base - bh:.1f}" width="{step * .7:.1f}" height="{bh:.1f}">'
                    f'<title>{esc(d["day"])}: {fmt(d[key])}</title></rect>')
        if i % max(1, len(daily) // 7) == 0:
            ticks.append(f'<text x="{x + step / 2:.1f}" y="{base + 16}" text-anchor="middle">{esc(d["day"][5:])}</text>')
    return (f'<svg viewBox="0 0 {w} {h}" role="img" aria-label="{esc(label)}" class="chart daybars">'
            f'<text x="0" y="12" class="axis-title">{esc(label)}</text>'
            f'<text x="{pad - 6}" y="26" text-anchor="end">{fmt(top)}</text><text x="{pad - 6}" y="{base}" text-anchor="end">0</text>'
            f'<line x1="{pad}" y1="{base}" x2="{w}" y2="{base}" class="rule"/>{"".join(bars)}{"".join(ticks)}</svg>')


def hbars(rows, label):
    """rows: (name, [(value, css_class, tooltip)], right_text). Segments stack left to right."""
    if not rows:
        return ""
    w, rh, left, right = 1000, 26, 200, 130
    top = max(sum(v for v, _, _ in segs) for _, segs, _ in rows) or 1
    out = []
    for i, (name, segs, tail) in enumerate(rows):
        y, x = i * rh + 4, left
        out.append(f'<text x="{left - 8}" y="{y + 14}" text-anchor="end">{esc(name)}</text>')
        for v, cls, tip in segs:
            bw = v / top * (w - left - right)
            out.append(f'<rect x="{x:.1f}" y="{y}" width="{bw:.1f}" height="{rh - 9}" class="{cls}"><title>{esc(tip)}</title></rect>')
            x += bw
        out.append(f'<text x="{x + 8:.1f}" y="{y + 14}">{esc(tail)}</text>')
    return (f'<svg viewBox="0 0 {w} {len(rows) * rh + 6}" role="img" aria-label="{esc(label)}" class="chart">'
            + "".join(out) + "</svg>")


# ---------------------------------------------------------------- tables

def table(caption, cols, rows, ident):
    """cols: (title, align, help). rows: list of cells; a cell is text or (text, sort_value, html)."""
    head = "".join(f'<th scope="col" class="{a}"><button type="button" title="{esc(h)}">{esc(t)}</button></th>' for t, a, h in cols)
    body = []
    for r in rows:
        tds = []
        for (t, a, _), c in zip(cols, r):
            text, sortv, raw = (c + (None,) * 3)[:3] if isinstance(c, tuple) else (c, c, None)
            tds.append(f'<td class="{a}" data-v="{esc(str(sortv if sortv is not None else ""))}">{raw if raw else esc(str(text))}</td>')
        body.append("<tr>" + "".join(tds) + "</tr>")
    return (f'<div class="scroll"><table id="{ident}"><caption>{esc(caption)}</caption>'
            f'<thead><tr>{head}</tr></thead><tbody>{"".join(body)}</tbody></table></div>')


def rate_cell(v):
    if v is None:
        return ("n/a", -1, '<span class="muted">n/a</span>')
    return (pct(v), v, f'<span class="rate"><i style="width:{v * 100:.0f}%"></i></span> {pct(v)}')


def tier_cell(t):
    return (t, TIERS.index(t), f'<span class="sw t-{t}"></span>{t}')


# ---------------------------------------------------------------- page

CSS = """
:root{--ground:#eef1f4;--sheet:#fbfcfd;--ink:#16202b;--soft:#51606f;--rule:#c5cdd6;--money:#8c2f39;
--power:#0b4f6c;--heavy:#2e7f98;--medium:#7fb5c4;--light:#c4dee5;--inactive:#d9d3c7;--no:#d5dbe2}
@media (prefers-color-scheme:dark){:root{--ground:#10161d;--sheet:#161e27;--ink:#e6ebf0;--soft:#9aa8b6;--rule:#2c3946;
--money:#e59aa2;--power:#5cc0e0;--heavy:#3b93b0;--medium:#2a6a80;--light:#214a59;--inactive:#4a463f;--no:#2a3440}}
*{box-sizing:border-box}html{background:var(--ground)}
body{margin:0 auto;max-width:1120px;padding:2.5rem 1.5rem 4rem;color:var(--ink);
font:16px/1.55 Seravek,"Gill Sans Nova",Ubuntu,Calibri,"DejaVu Sans",source-sans-pro,sans-serif;font-variant-numeric:tabular-nums}
h1,h2{font-family:Charter,"Bitstream Charter","Sitka Text",Cambria,serif;font-weight:600;line-height:1.15;margin:0}
h1{font-size:2.6rem;letter-spacing:-.01em}h2{font-size:1.45rem;margin:3.2rem 0 .8rem;padding-top:.9rem;border-top:2px solid var(--ink)}
header p,.muted,caption,.defs{color:var(--soft)}header p{margin:.5rem 0 0;max-width:70ch}
p,li{max-width:74ch}code{font-size:.92em;background:var(--ground);padding:0 .25em;border-radius:3px}
.band{display:flex;height:64px;margin:2.2rem 0 .6rem;border-radius:4px;overflow:hidden}
.band div{min-width:3px;display:flex;align-items:flex-end;padding:.35rem .55rem;font-size:.95rem;overflow:hidden;white-space:nowrap}
.t-power{background:var(--power);color:#fff}.t-heavy{background:var(--heavy);color:#fff}.t-medium{background:var(--medium);color:#0d1b24}
.t-light{background:var(--light);color:#0d1b24}.t-inactive{background:var(--inactive);color:#2b2922}
.who{display:grid;grid-template-columns:7.5rem 1fr;gap:.15rem 1rem;margin:0;font-size:.95rem}
.who dt{font-weight:600}.who dd{margin:0;color:var(--soft)}
.sw{display:inline-block;width:.7em;height:.7em;margin-right:.45em;border-radius:2px;vertical-align:baseline}
.econ{font-family:Charter,"Bitstream Charter","Sitka Text",Cambria,serif;font-size:1.5rem;line-height:1.4;max-width:34em;margin:2.4rem 0 .6rem}
.econ b{color:var(--money);font-weight:600;white-space:nowrap}
.analysis{background:var(--sheet);border-left:4px solid var(--heavy);padding:1rem 1.4rem;margin-top:1rem}
.analysis h3{margin:.9rem 0 .2rem;font-size:1rem}.analysis .lede{font-size:1.15rem;margin:.2rem 0 .6rem}
.chart{width:100%;height:auto;display:block;margin:.8rem 0 1.4rem}.chart text{font-size:12px;fill:var(--soft)}
.chart .axis-title{fill:var(--ink);font-size:13px}.chart .rule{stroke:var(--rule)}.daybars rect{fill:var(--heavy)}
.chart rect.t-power{fill:var(--power)}.chart rect.t-heavy{fill:var(--heavy)}.chart rect.t-medium{fill:var(--medium)}
.chart rect.t-light{fill:var(--light)}.chart rect.t-inactive{fill:var(--inactive)}
.chart rect.yes{fill:var(--power)}.chart rect.no{fill:var(--no)}.chart rect.agent{fill:var(--money);opacity:.75}
.key{font-size:.9rem;color:var(--soft);margin:0}.key span{margin-right:1.2rem}
.scroll{overflow-x:auto;background:var(--sheet);border:1px solid var(--rule);border-radius:4px}
table{border-collapse:collapse;width:100%;font-size:.9rem}caption{text-align:left;padding:.7rem .8rem .3rem;font-size:.9rem}
th,td{padding:.42rem .65rem;border-top:1px solid var(--rule);white-space:nowrap}th{border-top:0;text-align:left;vertical-align:bottom}
.r{text-align:right}th button{all:unset;cursor:pointer;font-weight:600}th button:focus-visible{outline:2px solid var(--heavy);outline-offset:3px}
th[aria-sort=ascending] button::after{content:" \\2191"}th[aria-sort=descending] button::after{content:" \\2193"}
.rate{display:inline-block;width:56px;height:7px;background:var(--no);border-radius:2px;margin-right:.5em;vertical-align:middle}
.rate i{display:block;height:100%;background:var(--power);border-radius:2px}.cost{color:var(--money)}
.tools{margin:.6rem 0 0}.tools button{font:inherit;padding:.35rem .8rem;border:1px solid var(--ink);background:none;color:inherit;border-radius:4px;cursor:pointer}
.defs{font-size:.92rem}.defs dt{font-weight:600;color:var(--ink);margin-top:.6rem}.defs dd{margin:0;max-width:74ch}
footer{margin-top:3rem;font-size:.85rem;color:var(--soft)}
@media (prefers-color-scheme:dark){.band .t-power{color:#06222e}.band .t-medium,.band .t-light,.band .t-inactive{color:#e6ebf0}}
@media (max-width:640px){h1{font-size:1.9rem}.econ{font-size:1.2rem}.who{grid-template-columns:1fr}}
@media print{html,.scroll,.analysis{background:#fff}body{max-width:none;padding:0}.tools{display:none}.scroll{overflow:visible;border:0}h2{break-after:avoid}tr{break-inside:avoid}}
"""

JS = """
document.querySelectorAll('th button').forEach(function(b){b.addEventListener('click',function(){
var th=b.parentNode,tb=th.closest('table').tBodies[0],i=Array.prototype.indexOf.call(th.parentNode.children,th);
var up=th.getAttribute('aria-sort')!=='ascending';th.parentNode.querySelectorAll('th').forEach(function(x){x.removeAttribute('aria-sort')});
th.setAttribute('aria-sort',up?'ascending':'descending');
Array.prototype.slice.call(tb.rows).sort(function(a,c){var x=a.cells[i].dataset.v,y=c.cells[i].dataset.v,p=parseFloat(x),q=parseFloat(y);
var r=(isNaN(p)||isNaN(q))?x.localeCompare(y):p-q;return up?r:-r}).forEach(function(r){tb.appendChild(r)})})});
var ex=document.getElementById('csv');if(ex)ex.addEventListener('click',function(){
var d=JSON.parse(document.getElementById('data').textContent).users,k=Object.keys(d[0]||{});
var s=[k.join(',')].concat(d.map(function(u){return k.map(function(f){var v=u[f];v=Array.isArray(v)?v.join(' '):(v==null?'':String(v));
return /^[=+@-]/.test(v)?"'"+v:v}).map(function(v){return '"'+v.replace(/"/g,'""')+'"'}).join(',')})).join('\\n');
var a=document.createElement('a');a.href=URL.createObjectURL(new Blob([s],{type:'text/csv'}));a.download='copilot-members.csv';a.click()});
"""


def page(d, ins):
    m, t, tiers, users = d["meta"], d["totals"], d["tiers"], d["users"]
    team = m.get("team_name") or m.get("team") or "team"
    total = max(t["members"], 1)

    band = "".join(f'<div class="t-{k}" style="flex:{tiers[k]}" title="{k}: {tiers[k]}">{tiers[k]} {k}</div>' for k in TIERS if tiers[k])
    who = "".join(f'<dt><span class="sw t-{k}"></span>{k}, {tiers[k]}</dt><dd>{esc(", ".join(u["login"] for u in users if u["tier"] == k)) or "none"}</dd>' for k in TIERS)
    th = m["thresholds"]

    econ = (f'Over {m.get("days")} days, {t["active_users"]} of {t["members"]} members drew <b>{n(t["credits"])} AI credits</b>, '
            f'worth <b>{usd(t["cost_usd"])}</b> at list value. ')
    if t["cost_per_merged_pr"] is not None:
        econ += f'That is <b>{usd(t["cost_per_merged_pr"])}</b> for each of {n(t["merged_prs_protected"])} pull requests merged into a protected branch'
        econ += (f', and <b>{usd(t["cost_per_prod_deployment"])}</b> for each of {n(t["prod_deployments"])} production deployments.'
                 if t["cost_per_prod_deployment"] is not None else '. No production deployment succeeded in the window.')
    else:
        econ += 'Nothing merged into a protected branch in the window, so there is no unit cost to state.'
    if t.get("seat_cost_usd"):
        econ += f' Seats for the same period cost {usd(t["seat_cost_usd"])}; they fund the included credit pool, so the two figures overlap and are not summed.'

    if ins and any(ins.get(k) for k in ("headline", "findings", "recommendations")):
        analysis = (f'<div class="analysis"><p class="lede">{esc(ins.get("headline") or "")}</p>'
                    f'<h3>What the numbers show</h3>{md_lite(ins.get("findings"))}'
                    f'<h3>What to do about it</h3>{md_lite(ins.get("recommendations"))}'
                    '<p class="muted">Written by an AI agent from the figures in this report. It had no other input and cannot change them.</p></div>')
    else:
        analysis = '<p class="muted">This copy carries no written analysis. The figures below are complete without it.</p>'

    ucols = [("Member", "", "GitHub login"), ("Tier", "", f"events per day: power {th['power']:g}+, heavy {th['heavy']:g}+, medium {th['medium']:g}+"),
             ("Cost", "r", "credits at list value"), ("Credits", "r", "AI credits used"),
             ("Events/day", "r", "interactions plus generations, per calendar day"), ("Active days", "r", "days with any Copilot activity"),
             ("Acceptance", "", "accepted over shown, agent features excluded"), ("Accepted", "r", "suggestions taken"),
             ("Not accepted", "r", "suggestions shown and not taken; GitHub reports no explicit reject"),
             ("Agent gens", "r", "agent-mode generations; no accept step"), ("Agent LoC", "r", "lines added by agent edits"),
             ("Merged PRs", "r", "authored, merged into a protected branch"), ("Uses", "", "surfaces seen in the window"),
             ("Seat", "", "holds a Copilot seat in this org"), ("Top model", "", "")]
    urows = [[u["login"], tier_cell(u["tier"]),
              (usd(u["cost_usd"]), u["cost_usd"], f'<span class="cost">{usd(u["cost_usd"])}</span>'), (n(u["credits"]), u["credits"]),
              (n(u["events_per_day"], 1), u["events_per_day"]),
              ("n/a" if u["active_days"] is None else u["active_days"], u["active_days"] or 0), rate_cell(u["acceptance_rate"]),
              (n(u["accepted"]), u["accepted"]), (n(u["not_accepted"]), u["not_accepted"]),
              (n(u["agent_generations"]), u["agent_generations"]), (n(u["agent_loc_added"]), u["agent_loc_added"]),
              (u["merged_prs"], u["merged_prs"]), " ".join(u["uses"]).replace("_", " ") or "none",
              {True: "yes", False: "no", None: "?"}[u["seat"]], u["top_model"] or ""] for u in users]

    fcols = [("Feature", "", ""), ("Interactions", "r", ""), ("Generations", "r", ""), ("Accepted", "r", ""), ("Not accepted", "r", ""),
             ("Acceptance", "", "n/a for agent features: they have no accept step"), ("LoC added", "r", "")]
    frows = [[f["feature"], (n(f["interactions"]), f["interactions"]), (n(f["generations"]), f["generations"]),
              (n(f["accepted"]), f["accepted"]), ("n/a" if f["agent"] else n(f["not_accepted"]), -1 if f["agent"] else f["not_accepted"]),
              rate_cell(f["acceptance_rate"]), (n(f["loc_added"]), f["loc_added"])] for f in d["features"]]
    fchart = hbars([(f["feature"], [(f["generations"], "agent", f'{n(f["generations"])} agent generations')] if f["agent"] else
                    [(f["accepted"], "yes", f'{n(f["accepted"])} accepted'), (f["not_accepted"], "no", f'{n(f["not_accepted"])} not accepted')],
                    "no accept step" if f["agent"] else pct(f["acceptance_rate"])) for f in d["features"][:10]], "Accepted and not accepted, by feature")

    cchart = hbars([(u["login"], [(u["cost_usd"], f't-{u["tier"]}', usd(u["cost_usd"]))], usd(u["cost_usd"])) for u in users[:15] if u["cost_usd"] > 0],
                   "Cost by member")

    rcols = [("Repository", "", ""), ("Protected merges", "r", "PRs by team members or Copilot, merged into a protected branch"),
             ("by members", "r", ""), ("by Copilot", "r", "authored by the Copilot cloud agent"), ("All merged", "r", "including unprotected bases"),
             ("Prod deploys", "r", "successful, environment matches the production pattern"), ("Copilot opened", "r", "from GitHub's repository report"),
             ("Copilot merged", "r", ""), ("Copilot reviews", "r", ""), ("Suggested", "r", ""), ("Applied", "r", "")]
    rrows = [[r["repo"]] + [(n(r[k]), r[k]) for k in ("merged_protected", "by_members", "by_copilot", "merged_prs", "prod_deployments",
             "copilot_created", "copilot_merged", "copilot_reviewed", "review_suggestions", "review_applied")] for r in d["repos"]]

    notes = "".join(f"<li>{esc(x)}</li>" for x in m.get("notes") or [])
    blob = json.dumps(d, separators=(",", ":")).replace("</", "<\\/")
    return f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex"><title>Copilot usage: {esc(team)}</title><style>{CSS}</style></head><body>
<header><h1>Copilot usage, {esc(team)}</h1>
<p>{esc(m.get("org") or "")}/{esc(m.get("team") or "")}, {esc(m.get("since") or "?")} to {esc(m.get("until") or "?")}.
Generated {esc(m["generated_at"])}{" for @" + esc(m["requested_by"]) if m.get("requested_by") else ""}. This file is self-contained and works offline.</p></header>

<div class="band" role="img" aria-label="Members by usage tier">{band}</div>
<dl class="who">{who}</dl>
<p class="econ">{econ}</p>
{analysis}

<h2>Day by day</h2>
{day_bars(d["daily"], "active_users", lambda v: n(v), "Members active per day")}
{day_bars(d["daily"], "credits", lambda v: n(v), "AI credits used per day")}

<h2>Members</h2>
{table(f"{total} members, costliest first. Select a heading to sort.", ucols, urows, "members")}
<p class="tools"><button type="button" id="csv">Save members as CSV</button></p>

<h2>What gets accepted</h2>
<p>Completions and chat suggest code and wait for a decision. Agents edit files directly, so they have no acceptance rate; they are drawn separately.
Across suggest-and-accept features the team took {pct(t["acceptance_rate"])} of what it was shown.
{t["agent_users"]} members used an agent, producing {n(t["agent_generations"])} generations and {n(t["agent_loc_added"])} added lines; {t["cloud_agent_users"]} used the cloud agent.</p>
{fchart}
<p class="key"><span><i class="sw" style="background:var(--power)"></i>accepted</span><span><i class="sw" style="background:var(--no)"></i>not accepted</span><span><i class="sw" style="background:var(--money);opacity:.75"></i>agent generations</span></p>
{table("By feature", fcols, frows, "features")}

<h2>Where the credits went</h2>
{cchart or '<p class="muted">No member used credits in the window.</p>'}

<h2>Repositories</h2>
<p>Repositories the team can reach. Pull requests count when a team member or the Copilot cloud agent wrote them.
{n(t["merged_prs_by_copilot"])} of the {n(t["merged_prs_protected"])} protected-branch merges were written by Copilot.</p>
{table(f"{len(d['repos'])} repositories with activity", rcols, rrows, "repos") if rrows else '<p class="muted">No merged pull requests or production deployments were found.</p>'}

<h2>How to read this</h2>
<dl class="defs">
<dt>Tiers</dt><dd>Events per calendar day over the window. An event is one user-initiated interaction or one code generation. Power {th["power"]:g} or more, heavy {th["heavy"]:g}, medium {th["medium"]:g}, light anything above zero, inactive none.</dd>
<dt>Not accepted</dt><dd>Generations minus acceptances. GitHub records what was shown and what was taken; it does not record a rejection, so a suggestion ignored counts the same as one dismissed.</dd>
<dt>Cost</dt><dd>AI credits at ${m["credit_usd"]:g} each. Credits come first from the pool your seats include; this is consumption at list value, not an invoice line.</dd>
<dt>Cost per merged PR, per deployment</dt><dd>The team's credit cost divided by the count. All Copilot use is in the numerator, whether or not it ended in a merge. Read it as a trend across reports, not as the price of a pull request.</dd>
</dl>
{f'<h2>Caveats from this run</h2><ul>{notes}</ul>' if notes else ''}
<footer>Per-person usage data. Share it as you would a performance document.</footer>
<script type="application/json" id="data">{blob}</script><script>{JS}</script></body></html>"""


if __name__ == "__main__":
    if len(sys.argv) not in (3, 4):
        sys.exit(__doc__.split("\n\n")[1])
    data = json.load(open(sys.argv[1], encoding="utf-8"))
    insights = None
    if len(sys.argv) == 4:
        try:
            insights = json.load(open(sys.argv[3], encoding="utf-8"))
        except (OSError, ValueError) as e:
            print(f"warning: insights unreadable ({e}); rendering without", file=sys.stderr)
    with open(sys.argv[2], "w", encoding="utf-8") as f:
        f.write(page(data, insights))
    print(f"wrote {sys.argv[2]}")
