#!/usr/bin/env python3
"""build.py: write train.jsonl, the seed set that teaches Gemma 4 the house
runbook format. Deterministic; replace RUNBOOKS with your own and re-run.

Every answer has the same three sections, so the effect of fine-tuning is
easy to see: ask the base model and the tuned model the same question.
"""

import json
from pathlib import Path

SYSTEM = ("You are the CoolGitOrg platform runbook assistant. Answer with three sections, "
          "Summary, Steps and Rollback, in that order. Steps are numbered shell commands with one "
          "comment each. Never invent flags. If a change needs approval, say which IssueOps label.")

RUNBOOKS = [
    ("rotate the issueops-autoadmin-app private key",
     "Generate a new key, store it, verify, then revoke the old one; the app keeps working throughout.",
     ["gh api /apps/issueops-autoadmin-app  # confirm the app id", "# In the app settings, Generate a private key; download the .pem",
      "gh secret set AUTOADMIN_APP_PRIVATE_KEY --org CoolGitOrg < key.pem  # replace the org secret",
      "gh workflow run app-token-smoke.yml --repo CoolGitOrg/platform  # prove a token can be minted"],
     "Re-set AUTOADMIN_APP_PRIVATE_KEY from the previous .pem and delete the new key in the app settings."),
    ("archive a repository that has been inactive for a year",
     "Archiving is reversible and keeps history; it needs the repo-lifecycle approval label.",
     ["gh repo view CoolGitOrg/$REPO --json pushedAt,isArchived  # check last push",
      "# Open an IssueOps request with label approval:repo-lifecycle",
      "gh repo archive CoolGitOrg/$REPO --yes  # archive after approval"],
     "gh repo unarchive CoolGitOrg/$REPO --yes"),
    ("add a team to a repository with write access",
     "Team access is managed in team-yamls; change the YAML, never the UI, so Terraform does not revert it.",
     ["git switch -c access/$TEAM-$REPO  # branch", "# Add the repo under repos: in team-yamls/$TEAM.yml with permission: push",
      "make validate  # schema and naming checks", "gh pr create --fill --label approval:access  # plan runs on the PR"],
     "Revert the PR; the next apply removes the access."),
    ("fix Terraform drift on organization settings",
     "Drift detection opened an issue; re-apply from main so the code wins, then find who changed the UI.",
     ["gh run list --workflow drift-detect.yml --limit 1  # find the drift run",
      "gh workflow run terraform-apply.yml -f target=org-settings  # re-apply from main",
      "gh api /orgs/CoolGitOrg/audit-log -X GET -f phrase=action:org.update_member_repository_creation_permission  # who changed it"],
     "If the UI change was intended, codify it in YAML and apply; do not leave drift."),
    ("revoke a leaked personal access token",
     "Revoke first, investigate second; enterprise-owned PATs can be revoked by an org owner.",
     ["gh api /orgs/CoolGitOrg/personal-access-tokens --paginate -q '.[] | select(.owner.login==\"$USER\")'  # list the user's fine-grained tokens",
      "gh api -X POST /orgs/CoolGitOrg/personal-access-tokens -f action=revoke -F 'pat_ids[]=$ID'  # revoke",
      "gh api /orgs/CoolGitOrg/audit-log -X GET -f phrase=actor:$USER  # review what it touched"],
     "None: a revoked token cannot be restored. The owner creates a new one."),
    ("enable push protection on every repository",
     "Push protection is part of secret scanning; turn it on through the security configuration, not repo by repo.",
     ["gh api /orgs/CoolGitOrg/code-security/configurations  # find the enforced configuration",
      "# Set secret_scanning_push_protection: enabled in the configuration YAML",
      "gh workflow run terraform-apply.yml -f target=security  # apply"],
     "Set the field back to disabled and apply; existing alerts are kept."),
    ("create a new repository from the gold standard",
     "New repositories come from repo-yamls through a pull request; the module applies rulesets and CODEOWNERS.",
     ["cp repo-yamls/_template.yml repo-yamls/$REPO.yml  # start from the template",
      "make validate  # naming, visibility and team checks", "gh pr create --fill --label approval:repo-create"],
     "Close the PR before merge; after apply, archive rather than delete."),
    ("give an outside collaborator temporary read access",
     "Outside collaborators need an expiry; the access YAML takes an expires field and a nightly job removes them.",
     ["# Add the user under outside_collaborators: in repo-yamls/$REPO.yml with expires: YYYY-MM-DD",
      "gh pr create --fill --label approval:outside-collaborator"],
     "Delete the entry and apply; the nightly job also removes expired entries."),
    ("check Copilot seat usage for a team",
     "Seat and usage data come from the Copilot NDJSON reports, not the legacy metrics endpoints.",
     ["gh api /orgs/CoolGitOrg/copilot/billing/seats --paginate -q '.seats[].assignee.login' > seats.txt  # seats",
      "gh api /orgs/CoolGitOrg/teams/$TEAM/members --paginate -q '.[].login' | sort > team.txt  # team",
      "comm -12 <(sort seats.txt) team.txt  # members with seats"],
     "Read-only; nothing to roll back."),
    ("roll back a bad ruleset change",
     "Rulesets are Terraform-managed; revert the commit and apply, then confirm with the API.",
     ["git revert $SHA  # revert the ruleset change", "gh pr create --fill --label approval:ruleset",
      "gh api /orgs/CoolGitOrg/rulesets  # confirm after apply"],
     "Re-apply the reverted commit."),
    ("deploy a new Foundry model version",
     "Model deployments go through the foundry-custom-models pipeline; dispatch it with a reason and review the plan first.",
     ["gh workflow run ex1-gemma4.yml -f action=plan -f reason='new runbook data'  # plan only",
      "gh run watch  # read the plan in the job summary",
      "gh workflow run ex1-gemma4.yml -f action=deploy -f dry_run=false -f reason='new runbook data'  # apply after review"],
     "ftm rollback gemma4  # traffic returns to the previous deployment"),
    ("find which workflow committed a change to records",
     "Every record commit carries a signoff and a run URL; git log is the audit trail.",
     ["git log --format='%h %an %s' -- records/  # who and what", "git show $SHA | grep run_url  # the workflow run"],
     "Read-only; nothing to roll back."),
]

ASKS = ["How do I {}?", "Runbook: {}.", "What is the procedure to {}?"]


def step(s):
    if s.startswith("# "):  # a manual step, not a command
        return s[2:]
    cmd, _, note = s.partition("  # ")
    return f"`{cmd}`" + (f"  # {note}" if note else "")


def answer(summary, steps, rollback):
    body = "\n".join(f"{i}. {step(s)}" for i, s in enumerate(steps, 1))
    return f"**Summary**\n{summary}\n\n**Steps**\n{body}\n\n**Rollback**\n{rollback}"


def main():
    rows = []
    for i, (task, summary, steps, rollback) in enumerate(RUNBOOKS):
        for j in range(2):  # two phrasings per runbook
            ask = ASKS[(i + j) % len(ASKS)].format(task)
            rows.append({"messages": [
                {"role": "system", "content": SYSTEM},
                {"role": "user", "content": ask[0].upper() + ask[1:]},
                {"role": "assistant", "content": answer(summary, steps, rollback)},
            ]})
    out = Path(__file__).with_name("train.jsonl")
    out.write_text("".join(json.dumps(r, ensure_ascii=False) + "\n" for r in rows))
    print(f"{out}: {len(rows)} examples")


if __name__ == "__main__":
    main()
