---
name: "REPLACE: Human Name"
on:
  workflow_dispatch:          # start manual; widen the trigger once it behaves
permissions:
  contents: read              # agent job is read-only, always
engine: copilot               # copilot | claude | codex | gemini | pi
timeout_minutes: 15
network: defaults             # add ecosystems/domains one at a time
tools:
  github:
    toolsets: [repos, issues]
safe-outputs:                 # the only writes this workflow can make
  create-issue:
    title-prefix: "[REPLACE] "
    labels: [automation]
    max: 1
---
# REPLACE: Human Name

State the task in plain sentences. Name inputs, outputs, and limits.

For event-triggered workflows, read user text only from:
"${{ needs.activation.outputs.text }}"

If no action is warranted, call the `noop` tool with a reason.
