#!/usr/bin/env python3
"""make_fixtures.py OUT_DIR - deterministic synthetic raw data shaped like the collector's output.
Fourteen members across every tier, one outsider who must be filtered out, 28 days."""
import json, os, random, sys
from datetime import date, timedelta

out = sys.argv[1]; os.makedirs(out, exist_ok=True)
rnd = random.Random(52)
end = date(2026, 9, 20); days = [end - timedelta(d) for d in range(27, -1, -1)]
#           login            events/day  accept  agent  credits/day  p(active)
people = [("avasquez",        95, .34, .55, 210, .95), ("bnakamura",  70, .41, .30, 120, .90),
          ("cokafor",         34, .29, .60, 160, .85), ("dlindqvist", 28, .38, .10,  45, .80),
          ("efontaine",       24, .22, .05,  30, .80), ("fadeyemi",   11, .31, .20,  22, .70),
          ("gmoretti",         9, .45, .00,  10, .65), ("hsorensen",   7, .27, .15,  14, .60),
          ("ikowalski",        6, .19, .00,   6, .55), ("jtanaka",     2, .35, .00,   2, .30),
          ("kobrien",          1, .50, .00,   1, .20), ("lhaddad",     1, .10, .00,   1, .15),
          ("mpetrov",          0, 0, 0, 0, 0),         ("nwright",     0, 0, 0, 0, 0)]
rows = []
for login, epd, acc, agent, cr, p in people + [("outsider", 40, .3, .2, 50, .9)]:
    for d in days:
        if d.weekday() >= 5 or rnd.random() > p or not epd: continue
        ev = max(1, int(rnd.gauss(epd * 28 / 20 / max(p, .1), epd * .25)))
        a_gen = int(ev * agent * .5); comp = int(ev * (1 - agent) * .7); chat = ev - a_gen - comp
        feats = [{"feature": "code_completion", "user_initiated_interaction_count": 0, "code_generation_activity_count": comp,
                  "code_acceptance_activity_count": int(comp * acc), "loc_added_sum": int(comp * acc * 2.2)},
                 {"feature": "chat_panel_ask_mode", "user_initiated_interaction_count": chat // 2, "code_generation_activity_count": chat - chat // 2,
                  "code_acceptance_activity_count": int((chat - chat // 2) * acc * .6), "loc_added_sum": int(chat * acc * 3)}]
        if a_gen: feats.append({"feature": "chat_panel_agent_mode", "user_initiated_interaction_count": a_gen // 4,
                  "code_generation_activity_count": a_gen, "code_acceptance_activity_count": 0, "loc_added_sum": a_gen * 14})
        rows.append({"user_login": login if login != "bnakamura" else "BNakamura", "day": d.isoformat(),
            "user_initiated_interaction_count": sum(f["user_initiated_interaction_count"] for f in feats),
            "code_generation_activity_count": sum(f["code_generation_activity_count"] for f in feats),
            "code_acceptance_activity_count": sum(f["code_acceptance_activity_count"] for f in feats),
            "loc_added_sum": sum(f["loc_added_sum"] for f in feats), "loc_suggested_to_add_sum": ev * 4,
            "ai_credits_used": round(max(0, rnd.gauss(cr * 28 / 20 / max(p, .1), cr * .3)), 1),
            "used_agent": bool(a_gen), "used_chat": True, "used_cli": login in ("avasquez", "cokafor"),
            "used_copilot_cloud_agent": login in ("avasquez", "cokafor", "bnakamura") and rnd.random() < .3,
            "totals_by_feature": feats, "totals_by_ide": [{"ide": "jetbrains" if login[0] in "dg" else "vscode",
            "user_initiated_interaction_count": 1, "code_generation_activity_count": ev}],
            "totals_by_model_feature": [{"model": rnd.choice(["claude-sonnet-5", "gpt-5.4", "gpt-5-mini"]), "feature": "chat",
            "user_initiated_interaction_count": ev, "code_generation_activity_count": 0}]})
# First half NDJSON, second half a JSON array: the parser must take both.
half = len(rows) // 2
with open(f"{out}/usage.ndjson", "w") as f:
    f.write("\n".join(json.dumps(r) for r in rows[:half]) + "\n" + json.dumps(rows[half:]) + "\n")
members = [p[0] for p in people]
repos = ["payments-api", "payments-web", "ledger-core", "infra-live", "docs"]
prs = []
for i in range(64):
    a = rnd.choice(members[:9] * 3 + ["copilot-swe-agent[bot]"] * 4 + ["someone-else"])
    r = rnd.choice(repos)
    prs.append({"repo": r, "number": 100 + i, "author": a, "base": "main" if rnd.random() < .85 else "spike/x",
                "merged_at": rnd.choice(days).isoformat() + "T12:00:00Z"})
for p in prs: p["protected"] = p["base"] == "main" and p["repo"] != "docs"
deps = [{"repo": rnd.choice(repos[:3]), "id": i, "environment": rnd.choice(["production", "Prod"]),
         "created_at": rnd.choice(days).isoformat() + "T15:00:00Z", "success": rnd.random() < .9} for i in range(11)]
repo_rows = [{"day": d.isoformat(), "repo_name": r, "pull_requests": {"total_created_by_copilot": rnd.randint(0, 1),
              "total_merged_created_by_copilot": rnd.randint(0, 1), "total_reviewed_by_copilot": rnd.randint(0, 3),
              "total_copilot_suggestions": rnd.randint(0, 5), "total_copilot_applied_suggestions": rnd.randint(0, 2)}}
             for d in days[::3] for r in repos[:4]]
json.dump(members, open(f"{out}/members.json", "w"))
json.dump([{"login": m, "plan_type": "business"} for m in members if m != "nwright"], open(f"{out}/seats.json", "w"))
json.dump(prs, open(f"{out}/prs.json", "w")); json.dump(deps, open(f"{out}/deployments.json", "w"))
open(f"{out}/repo_copilot.ndjson", "w").write("\n".join(json.dumps(r) for r in repo_rows))
json.dump({"org": "CoolGitOrg", "team": "payments-platform", "team_name": "Payments Platform", "days": 28,
           "since": days[0].isoformat(), "until": end.isoformat(), "report_start_day": days[0].isoformat(),
           "report_end_day": end.isoformat(), "requested_by": "avasquez", "issue": 42,
           "notes": ["Team membership is as of report time; someone who joined last week carries their whole window with them."]},
          open(f"{out}/meta.json", "w"))
