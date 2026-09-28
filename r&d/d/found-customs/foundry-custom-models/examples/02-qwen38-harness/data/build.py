#!/usr/bin/env python3
"""build.py: write train.jsonl for the coolgit-ops adapter.

The adapter teaches Qwen3.8 one habit: review a proposed governance YAML
change and answer with a single JSON object,
    {"decision": "approve|changes|reject", "risks": [...], "checks": [...]}
which the Agent Framework workflow parses. Deterministic; replace CASES.
"""

import json
from pathlib import Path

SYSTEM = ("You are coolgit-ops, the CoolGitOrg governance reviewer. Review the proposed YAML change "
          "and reply with one JSON object and nothing else: {\"decision\": \"approve\"|\"changes\"|\"reject\", "
          "\"risks\": [strings], \"checks\": [strings]}. Base the decision on the policy: repositories are "
          "private or internal unless public is approved; admin is only for platform-admins; every repo "
          "has CODEOWNERS and a default-branch ruleset; outside collaborators need an expiry.")

CASES = [
    ("repo: data-lake-etl\nvisibility: private\nteams:\n  data-eng: push\n  platform-admins: admin\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/data-eng']",
     "approve", [], ["terraform plan shows one repository and two team grants"]),
    ("repo: marketing-site\nvisibility: public\nteams:\n  web: push\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/web']",
     "changes", ["public visibility without an approval:public-repo label"], ["add the approval label or set visibility: internal"]),
    ("repo: ml-sandbox\nvisibility: internal\nteams:\n  ml-research: admin\nrulesets: []\ncodeowners: []",
     "reject", ["admin granted to a team other than platform-admins", "no default-branch ruleset", "no CODEOWNERS"],
     ["grant ml-research push", "add rulesets: [default-branch]", "add a CODEOWNERS owner"]),
    ("repo: partner-api\nvisibility: private\noutside_collaborators:\n  vendor-dev: pull\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/api']",
     "changes", ["outside collaborator vendor-dev has no expires date"], ["add expires: YYYY-MM-DD under vendor-dev"]),
    ("repo: infra-live\nvisibility: private\nteams:\n  platform-admins: admin\n  sre: maintain\nrulesets: [default-branch, signed-commits]\ncodeowners: ['@CoolGitOrg/sre']",
     "approve", [], ["plan shows signed-commits ruleset attached"]),
    ("repo: docs-internal\nvisibility: internal\nteams:\n  everyone: push\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/docs']",
     "changes", ["push for the everyone team makes every member a writer"], ["grant everyone pull and docs push"]),
    ("repo: payments-core\nvisibility: public\nteams:\n  payments: admin\nrulesets: []\ncodeowners: []",
     "reject", ["public visibility for a payments repository", "admin to a non-platform team", "no ruleset", "no CODEOWNERS"],
     ["set visibility: private", "grant payments maintain", "add default-branch ruleset and CODEOWNERS"]),
    ("repo: mobile-app\nvisibility: private\nteams:\n  mobile: push\n  qa: triage\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/mobile']",
     "approve", [], ["plan shows two team grants"]),
    ("repo: vendor-sdk\nvisibility: private\noutside_collaborators:\n  acme-bot: push\n    expires: 2026-12-31\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/sdk']",
     "approve", ["outside collaborator with push; expiry present"], ["confirm acme-bot is a machine account with 2FA"]),
    ("repo: old-prototype\nvisibility: private\narchived: false\nlast_push: 2024-02-01\nteams: {}\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/platform']",
     "changes", ["no pushes for over two years and no team owner"], ["set archived: true or assign an owning team"]),
    ("repo: secrets-tooling\nvisibility: internal\nteams:\n  security: maintain\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/security']\nsecret_scanning_push_protection: disabled",
     "reject", ["push protection disabled on a secrets tooling repository"], ["remove the override; the org configuration enables it"]),
    ("repo: analytics-dash\nvisibility: internal\nteams:\n  analytics: push\n  platform-admins: admin\nrulesets: [default-branch]\ncodeowners: ['@CoolGitOrg/analytics']",
     "approve", [], ["plan shows one repository"]),
]

PROMPTS = ["Review this change:\n```yaml\n{}\n```", "Proposed repo-yamls entry, approve or not?\n```yaml\n{}\n```"]


def main():
    rows = []
    for yml, decision, risks, checks in CASES:
        for p in PROMPTS:
            rows.append({"messages": [
                {"role": "system", "content": SYSTEM},
                {"role": "user", "content": p.format(yml)},
                {"role": "assistant", "content": json.dumps({"decision": decision, "risks": risks, "checks": checks})},
            ]})
    out = Path(__file__).with_name("train.jsonl")
    out.write_text("".join(json.dumps(r, ensure_ascii=False) + "\n" for r in rows))
    print(f"{out}: {len(rows)} examples")


if __name__ == "__main__":
    main()
