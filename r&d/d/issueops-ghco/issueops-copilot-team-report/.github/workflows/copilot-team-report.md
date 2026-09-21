---
name: copilot-team-report
description: Collect a team's Copilot usage by script, have an agent read the totals and write the analysis, publish one offline HTML report to the requesting issue.

on:
  workflow_dispatch:
    inputs:
      team:
        description: "Team slug in CoolGitOrg"
        required: true
        type: string
      days:
        description: "Window in days"
        required: true
        type: choice
        options: ["7", "14", "28"]
      issue:
        description: "Issue number that asked for the report"
        required: true
        type: string
      requested_by:
        description: "Login of the requester, already authorised by the front door"
        required: false
        type: string
  # The front door (report-request.yml) dispatches as the app. Humans need repo admin to bypass it.
  roles: [admin]
  bots: ["issueops-autoadmin-app[bot]"]

permissions:
  contents: read
  copilot-requests: write   # inference billed to the org; no personal COPILOT_GITHUB_TOKEN to rotate

# Two teams may be reported at once; two runs for one team queue.
concurrency:
  group: copilot-team-report-${{ github.event.inputs.team }}
  job-discriminator: ${{ github.event.inputs.team }}

engine: copilot
timeout-minutes: 15
max-ai-credits: 400

network: defaults

tools:
  bash: ["cat", "head", "wc", "grep"]

# Deterministic work with secrets happens here, in its own job, outside the agent job.
# Strict mode forbids secrets in the agent job's steps, and it is right to.
jobs:
  collect:
    needs: [activation]
    runs-on: ubuntu-latest
    timeout-minutes: 30
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v7
        with:
          persist-credentials: false
      - name: App token (org installation)
        id: app
        uses: actions/create-github-app-token@v3
        with:
          client-id: ${{ vars.ISSUEOPS_APP_CLIENT_ID }}
          private-key: ${{ secrets.ISSUEOPS_APP_PRIVATE_KEY }}
          owner: CoolGitOrg
      - name: Collect, aggregate, render baseline
        env:
          ORG_TOKEN: ${{ steps.app.outputs.token }}
          TEAM: ${{ github.event.inputs.team }}
          DAYS: ${{ github.event.inputs.days }}
          ISSUE: ${{ github.event.inputs.issue }}
          REQUESTED_BY: ${{ github.event.inputs.requested_by }}
        run: |
          set -euo pipefail
          set -a; . config/report.env; set +a
          scripts/collect.sh "$TEAM" "$DAYS" "$RUNNER_TEMP/r"
          python3 scripts/aggregate.py "$RUNNER_TEMP/r/raw" "$RUNNER_TEMP/r/out"
          python3 scripts/render.py "$RUNNER_TEMP/r/out/report-data.json" "$RUNNER_TEMP/r/html/copilot-report-$TEAM.html"
      # Aggregates only. The raw per-day rows stay on the runner and die with it.
      - name: Hand the aggregates to the later jobs
        uses: actions/upload-artifact@v7
        with:
          name: report-data
          path: ${{ runner.temp }}/r/out/
          retention-days: 1
      - name: Upload the baseline report
        id: html
        uses: actions/upload-artifact@v7
        with:
          name: copilot-report-${{ github.event.inputs.team }}-baseline
          path: ${{ runner.temp }}/r/html/
          retention-days: 14
      - name: Tell the issue
        env:
          ORG_TOKEN: ${{ steps.app.outputs.token }}
          REPO: ${{ github.repository }}
          ISSUE: ${{ github.event.inputs.issue }}
          ARTIFACT_URL: ${{ steps.html.outputs.artifact-url }}
          DATA: ${{ runner.temp }}/r/out/report-data.json
        run: scripts/publish.sh baseline
      - name: Tell the issue it failed
        if: failure()
        env:
          ORG_TOKEN: ${{ steps.app.outputs.token }}
          REPO: ${{ github.repository }}
          ISSUE: ${{ github.event.inputs.issue }}
          RUN_URL: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
        run: scripts/publish.sh failed || true

# Agent job pre-step: no secrets, just put the compact summary where the agent can read it.
steps:
  - name: Fetch the summary for the agent
    uses: actions/download-artifact@v8
    with:
      name: report-data
      path: /tmp/gh-aw/agent/report

safe-outputs:
  jobs:
    publish-report:
      description: "Publish the written analysis. Call exactly once. The numbers, tables and charts are already built; this adds your prose to the HTML report and posts it to the requesting issue."
      runs-on: ubuntu-latest
      output: "Analysis attached; the report was posted to the issue."
      inputs:
        headline:
          description: "One plain sentence, under 140 characters: the single most useful thing a team lead should know."
          required: true
          type: string
        findings:
          description: "Three to six markdown bullets. Each states one fact and quotes figures exactly as they appear in summary.json. **bold** and `code` are rendered; nothing else is."
          required: true
          type: string
        recommendations:
          description: "Two to four markdown bullets. Each is an action a team lead can take this month and names the figure that motivates it."
          required: true
          type: string
      permissions:
        contents: read
      steps:
        - uses: actions/checkout@v7
          with:
            persist-credentials: false
        - uses: actions/download-artifact@v8
          with:
            name: report-data
            path: ${{ runner.temp }}/data
        - name: Render with the analysis
          env:
            TEAM: ${{ github.event.inputs.team }}
          run: |
            set -euo pipefail
            [[ "$TEAM" =~ ^[a-z0-9][a-z0-9._-]{0,99}$ ]]
            jq '[.items[] | select(.type == "publish_report")][0] // {} | {headline, findings, recommendations}' \
              "$GH_AW_AGENT_OUTPUT" > "$RUNNER_TEMP/insights.json"
            jq -r '.headline // ""' "$RUNNER_TEMP/insights.json" | head -c 300 | tr -d '\r\n' > "$RUNNER_TEMP/headline"
            mkdir -p "$RUNNER_TEMP/html"
            python3 scripts/render.py "$RUNNER_TEMP/data/report-data.json" \
              "$RUNNER_TEMP/html/copilot-report-$TEAM.html" "$RUNNER_TEMP/insights.json"
        - name: Upload the report
          id: html
          uses: actions/upload-artifact@v7
          with:
            name: copilot-report-${{ github.event.inputs.team }}
            path: ${{ runner.temp }}/html/
            retention-days: 14
        - name: App token (org installation)
          id: app
          uses: actions/create-github-app-token@v3
          with:
            client-id: ${{ vars.ISSUEOPS_APP_CLIENT_ID }}
            private-key: ${{ secrets.ISSUEOPS_APP_PRIVATE_KEY }}
            owner: CoolGitOrg
            repositories: ${{ github.event.repository.name }}
        - name: Post to the issue
          env:
            ORG_TOKEN: ${{ steps.app.outputs.token }}
            REPO: ${{ github.repository }}
            ISSUE: ${{ github.event.inputs.issue }}
            ARTIFACT_URL: ${{ steps.html.outputs.artifact-url }}
            DATA: ${{ runner.temp }}/data/report-data.json
          run: |
            if [ "${GH_AW_SAFE_OUTPUTS_STAGED:-}" = true ]; then echo "staged: would post to #$ISSUE"; exit 0; fi
            HEADLINE=$(cat "$RUNNER_TEMP/headline") scripts/publish.sh final
---

# Copilot team report: the analysis

A script has already collected and aggregated Copilot usage for team
`${{ github.event.inputs.team }}` over the last ${{ github.event.inputs.days }} days, and
built every table and chart. Your job is the part a script cannot do: read the figures and
say what they mean to the person who leads this team.

## Input

Read `/tmp/gh-aw/agent/report/summary.json`. It is the only input. Do not look anywhere else.

- `totals`: team figures, including `cost_usd`, `cost_per_merged_pr`, `cost_per_prod_deployment`, `acceptance_rate`.
- `tiers`: member count per usage tier. `meta.thresholds` defines the tiers.
- `users`: one row per member, costliest first. `uses` lists the surfaces seen: agent, chat, cli, cloud_agent, app, code_review.
- `features`: accepted and not accepted per feature. Agent features have no accept step; their `acceptance_rate` is null by design, not a zero.
- `daily`, `repos`: the trend, and delivery per repository.
- `meta.notes`: gaps in the data. If a note says a source was unavailable, do not reason from its absence.

## What to look for

Pick what this data actually shows. Likely candidates:

- Concentration: what share of cost sits with how few people, and whether their delivery (`merged_prs`) matches.
- Seats held by inactive members, and members active without a seat.
- A feature with an acceptance rate far from the others.
- Agent adoption: who uses it, what share of added lines it accounts for, whether the cloud agent's merged PRs are a meaningful share of `merged_prs_protected`.
- Unit cost, stated once, with the caveat that it is a trend measure.

## Rules

- Every figure you quote must appear in `summary.json`, or be a ratio or share of two figures that do, rounded sensibly. Do not estimate, extrapolate, or annualise.
- Logins are lowercase as they appear in the file. Name people only when the figure is about them.
- Low usage is not a verdict on a person. Describe what the numbers show and suggest something to try; never recommend disciplining anyone or removing a named person's seat. "Two inactive seats could be reassigned" is fine.
- A null is "not available", never zero.
- Plain sentences. No preamble, no closing summary, no emoji.

## Output

Call `publish_report` exactly once with `headline`, `findings`, `recommendations`.

If `summary.json` is missing or has no `users`, the collection failed upstream: call
`report_incomplete` and say which. If the file is fine but every member is inactive, still
call `publish_report`; that is a finding. Use `noop` only if you were started with nothing to do.
