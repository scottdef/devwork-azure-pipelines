#!/usr/bin/env python3
"""aggregate.py - raw collector output in, report-data.json and summary.json out.

usage: aggregate.py RAW_DIR OUT_DIR

Standard library only. Every number in the report is computed here, once.
The renderer draws them and the agent talks about them; neither does arithmetic.

Definitions worth knowing before trusting a number:

  event          user_initiated_interaction_count + code_generation_activity_count
  tier           events per calendar day over the window, against TIER_* thresholds
  accepted       code_acceptance_activity_count, over suggest-then-accept features only
  not accepted   generations - acceptances over those same features. GitHub reports no
                 "reject" signal; this is shown-but-not-taken, which includes ignored.
  agent          features whose name contains "agent". Agents edit files without an
                 accept step, so they are excluded from acceptance and counted apart.
  cost           ai_credits_used x CREDIT_USD. This is consumption at list value, drawn
                 first from the pool the seats already paid for. It is not an invoice.
"""
import json
import os
import sys
from collections import defaultdict
from datetime import date, datetime, timedelta, timezone

TIERS = ("power", "heavy", "medium", "light", "inactive")
SUMS = ("user_initiated_interaction_count", "code_generation_activity_count",
        "code_acceptance_activity_count", "loc_suggested_to_add_sum", "loc_added_sum",
        "loc_deleted_sum", "ai_credits_used")
FLAGS = {"used_agent": "agent", "used_chat": "chat", "used_cli": "cli",
         "used_copilot_app": "app", "used_copilot_cloud_agent": "cloud_agent",
         "used_copilot_code_review_active": "code_review"}


def env(name, default, cast=float):
    try:
        return cast(os.environ.get(name, default))
    except ValueError:
        sys.exit(f"error: {name} is not a valid {cast.__name__}")


def load(path, default):
    try:
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return default


def ndjson(path):
    """NDJSON, a JSON array, or a mix. GitHub's docs show both shapes."""
    try:
        text = open(path, encoding="utf-8").read()
    except FileNotFoundError:
        return []
    dec, i, out = json.JSONDecoder(), 0, []
    while i < len(text):
        while i < len(text) and text[i].isspace():
            i += 1
        if i >= len(text):
            break
        val, i = dec.raw_decode(text, i)
        out.extend(val if isinstance(val, list) else [val])
    return out


def ratio(a, b, nd=4):
    return round(a / b, nd) if b else None


def is_agent(feature):
    return "agent" in (feature or "").lower()


def span(meta, daily):
    """Every day of the window, so a quiet weekend shows as a gap and not as nothing."""
    try:
        a, b = date.fromisoformat(meta["since"]), date.fromisoformat(meta["until"])
        return [(a + timedelta(i)).isoformat() for i in range((b - a).days + 1)]
    except (KeyError, TypeError, ValueError):
        return sorted(daily)


def tier_of(events_per_day, th):
    if events_per_day <= 0:
        return "inactive"
    for name in ("power", "heavy", "medium"):
        if events_per_day >= th[name]:
            return name
    return "light"


def main(raw, out):
    meta = load(f"{raw}/meta.json", {})
    members = [m.lower() for m in load(f"{raw}/members.json", [])]
    seats = {s["login"].lower(): s for s in load(f"{raw}/seats.json", [])}
    prs = load(f"{raw}/prs.json", [])
    deploys = load(f"{raw}/deployments.json", [])
    rows = [r for r in ndjson(f"{raw}/usage.ndjson") if (r.get("user_login") or "").lower() in members]
    repo_rows = ndjson(f"{raw}/repo_copilot.ndjson")
    bots = {b.lower() for b in os.environ.get("COPILOT_BOTS", "copilot-swe-agent[bot] Copilot").split()}
    days = int(meta.get("days", 28))
    credit_usd = env("CREDIT_USD", "0.01")
    seat_usd = env("SEAT_USD_MONTH", "0")
    th = {"power": env("TIER_POWER", "50"), "heavy": env("TIER_HEAVY", "20"), "medium": env("TIER_MEDIUM", "5")}
    notes = list(meta.get("notes", []))

    # ---- per user
    u = {m: {"login": m, **{k: 0 for k in SUMS}, "days": set(), "flags": set(), "feat": defaultdict(int),
             "ide": defaultdict(int), "model": defaultdict(int), "acc_gen": 0, "acc": 0,
             "agent_gen": 0, "agent_loc": 0, "merged_prs": 0} for m in members}
    feat = defaultdict(lambda: defaultdict(int))
    daily = defaultdict(lambda: {"users": set(), "credits": 0.0, "events": 0, "accepted": 0})
    dated = True
    for r in rows:
        x = u[r["user_login"].lower()]
        for k in SUMS:
            x[k] += r.get(k) or 0
        ev = (r.get("user_initiated_interaction_count") or 0) + (r.get("code_generation_activity_count") or 0)
        day = r.get("day")
        dated = dated and bool(day)
        if day and (ev or r.get("ai_credits_used")):
            x["days"].add(day)
            d = daily[day]
            d["users"].add(x["login"])
            d["credits"] += r.get("ai_credits_used") or 0
            d["events"] += ev
            d["accepted"] += r.get("code_acceptance_activity_count") or 0
        for k, name in FLAGS.items():
            if r.get(k):
                x["flags"].add(name)
        fs = r.get("totals_by_feature") or []
        if not fs:  # no breakdown: treat the row as one suggest-then-accept feature
            fs = [{"feature": "unspecified", **{k: r.get(k) or 0 for k in SUMS}}]
        for f in fs:
            name = f.get("feature") or "unspecified"
            gen = f.get("code_generation_activity_count") or 0
            acc = f.get("code_acceptance_activity_count") or 0
            inter = f.get("user_initiated_interaction_count") or 0
            x["feat"][name] += inter + gen
            a = feat[name]
            a["interactions"] += inter
            a["generations"] += gen
            a["acceptances"] += acc
            a["loc_added"] += f.get("loc_added_sum") or 0
            if is_agent(name):
                x["agent_gen"] += gen
                x["agent_loc"] += f.get("loc_added_sum") or 0
            else:
                x["acc_gen"] += gen
                x["acc"] += acc
        for i in r.get("totals_by_ide") or []:
            x["ide"][i.get("ide") or "unknown"] += (i.get("user_initiated_interaction_count") or 0) + \
                (i.get("code_generation_activity_count") or 0)
        for m in r.get("totals_by_model_feature") or []:
            x["model"][m.get("model") or "unknown"] += (m.get("user_initiated_interaction_count") or 0) + \
                (m.get("code_generation_activity_count") or 0)
    if rows and not dated:
        notes.append("Usage rows carry no day field; the window could not be cut and active days are unknown.")

    # ---- delivery: merged PRs into protected branches, production deployments
    repos = defaultdict(lambda: defaultdict(int))
    for p in prs:
        a = (p.get("author") or "").lower()
        mine, bot = a in u, a in bots
        if not (mine or bot):
            continue
        r = repos[p["repo"]]
        r["merged_prs"] += 1
        if p.get("protected"):
            r["merged_protected"] += 1
            r["by_copilot" if bot else "by_members"] += 1
            if mine:
                u[a]["merged_prs"] += 1
    for d in deploys:
        if d.get("success"):
            repos[d["repo"]]["prod_deployments"] += 1
    for r in repo_rows:
        pr = r.get("pull_requests") or {}
        x = repos[r.get("repo_name")]
        for src, dst in (("total_created_by_copilot", "copilot_created"), ("total_merged_created_by_copilot", "copilot_merged"),
                         ("total_reviewed_by_copilot", "copilot_reviewed"), ("total_copilot_suggestions", "review_suggestions"),
                         ("total_copilot_applied_suggestions", "review_applied")):
            x[dst] += pr.get(src) or 0

    # ---- assemble
    users = []
    for x in u.values():
        ev = x["user_initiated_interaction_count"] + x["code_generation_activity_count"]
        epd = round(ev / days, 2) if days else 0
        seat = seats.get(x["login"])
        top = lambda d: max(d, key=d.get) if d else None  # noqa: E731
        users.append({
            "login": x["login"], "tier": tier_of(epd, th), "seat": (True if seat else (False if seats else None)),
            "active_days": len(x["days"]) if dated else None, "events": ev, "events_per_day": epd,
            "interactions": x["user_initiated_interaction_count"], "generations": x["code_generation_activity_count"],
            "accepted": x["acc"], "not_accepted": max(x["acc_gen"] - x["acc"], 0),
            "acceptance_rate": ratio(x["acc"], x["acc_gen"]),
            "loc_added": x["loc_added_sum"], "agent_generations": x["agent_gen"], "agent_loc_added": x["agent_loc"],
            "uses": sorted(x["flags"]), "credits": round(x["ai_credits_used"], 2),
            "cost_usd": round(x["ai_credits_used"] * credit_usd, 2), "merged_prs": x["merged_prs"],
            "top_feature": top(x["feat"]), "top_ide": top(x["ide"]), "top_model": top(x["model"])})
    users.sort(key=lambda r: (-r["cost_usd"], -r["events"], r["login"]))

    features = [{"feature": k, "agent": is_agent(k), "interactions": v["interactions"], "generations": v["generations"],
                 "accepted": v["acceptances"], "not_accepted": max(v["generations"] - v["acceptances"], 0),
                 "acceptance_rate": None if is_agent(k) else ratio(v["acceptances"], v["generations"]),
                 "loc_added": v["loc_added"]} for k, v in feat.items()]
    features.sort(key=lambda f: -(f["interactions"] + f["generations"]))

    repo_list = [{"repo": k, **{f: v.get(f, 0) for f in ("merged_prs", "merged_protected", "by_members", "by_copilot",
                  "prod_deployments", "copilot_created", "copilot_merged", "copilot_reviewed",
                  "review_suggestions", "review_applied")}} for k, v in repos.items() if k]
    repo_list.sort(key=lambda r: (-r["merged_protected"], -r["prod_deployments"], r["repo"]))

    credits = sum(x["credits"] for x in users)
    cost = round(credits * credit_usd, 2)
    merged = sum(r["merged_protected"] for r in repo_list)
    deployed = sum(r["prod_deployments"] for r in repo_list)
    acc = sum(x["accepted"] for x in users)
    nacc = sum(x["not_accepted"] for x in users)
    tiers = {t: sum(1 for x in users if x["tier"] == t) for t in TIERS}
    totals = {
        "members": len(users), "seats": (sum(1 for x in users if x["seat"]) if seats else None),
        "active_users": len(users) - tiers["inactive"], "credits": round(credits, 2), "cost_usd": cost,
        "seat_cost_usd": round(seat_usd * (sum(1 for x in users if x["seat"]) if seats else len(users)) * days / 30, 2) or None,
        "events": sum(x["events"] for x in users), "accepted": acc, "not_accepted": nacc,
        "acceptance_rate": ratio(acc, acc + nacc), "loc_added": sum(x["loc_added"] for x in users),
        "agent_users": sum(1 for x in users if "agent" in x["uses"] or x["agent_generations"]),
        "cloud_agent_users": sum(1 for x in users if "cloud_agent" in x["uses"]),
        "agent_generations": sum(x["agent_generations"] for x in users),
        "agent_loc_added": sum(x["agent_loc_added"] for x in users),
        "merged_prs_protected": merged, "merged_prs_by_copilot": sum(r["by_copilot"] for r in repo_list),
        "prod_deployments": deployed,
        "cost_per_merged_pr": ratio(cost, merged, 2), "cost_per_prod_deployment": ratio(cost, deployed, 2),
        "cost_per_active_user": ratio(cost, len(users) - tiers["inactive"], 2)}
    if not merged:
        notes.append("No PR by a team member or Copilot merged into a protected branch in the window; cost per merged PR is undefined.")
    if not deployed:
        notes.append("No successful deployment to a production environment in the team's repositories; cost per deployment is undefined.")
    if not rows:
        notes.append("The usage report held no rows for these members. Check that the Copilot usage metrics policy is enabled.")

    data = {
        "meta": {**{k: meta.get(k) for k in ("org", "team", "team_name", "days", "since", "until",
                                              "report_start_day", "report_end_day", "requested_by", "issue")},
                 "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC"),
                 "thresholds": th, "credit_usd": credit_usd, "notes": notes},
        "totals": totals, "tiers": tiers, "users": users, "features": features,
        "daily": [{"day": d, "active_users": len(daily[d]["users"]), "credits": round(daily[d]["credits"], 2),
                   "events": daily[d]["events"], "accepted": daily[d]["accepted"]} for d in span(meta, daily)],
        "repos": repo_list}
    os.makedirs(out, exist_ok=True)
    with open(f"{out}/report-data.json", "w", encoding="utf-8") as f:
        json.dump(data, f, indent=1)
    # The agent's copy: same numbers, long tails cut. It reads this and nothing else.
    brief = {**data, "users": users[:60], "repos": repo_list[:25],
             "truncated": {"users": max(len(users) - 60, 0), "repos": max(len(repo_list) - 25, 0)}}
    with open(f"{out}/summary.json", "w", encoding="utf-8") as f:
        json.dump(brief, f, separators=(",", ":"))
    print(f"members={len(users)} rows={len(rows)} cost=${cost} merged={merged} deployed={deployed}")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__.split("\n\n")[1])
    main(sys.argv[1], sys.argv[2])
