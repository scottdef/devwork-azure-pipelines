# gh-aw-field-guide: GitHub Copilot agent skill

Converted from the Claude skill of the same name. Same Agent Skills format
(SKILL.md + resources); what changed is below.

## Install

Project scope (Copilot cloud agent, Copilot CLI, VS Code agent mode):

    unzip gh-aw-field-guide-copilot-skill.zip -d /path/to/repo
    cd /path/to/repo
    git add .github/skills/gh-aw-field-guide
    git commit -m "skills: add gh-aw-field-guide"

Personal scope (all repos, Copilot CLI):

    mkdir -p ~/.copilot/skills
    cp -r .github/skills/gh-aw-field-guide ~/.copilot/skills/

In a running Copilot CLI session: `/skills reload`, then
`/skills info gh-aw-field-guide`. Invoke explicitly with
`/gh-aw-field-guide <task>`; otherwise Copilot loads it from the description.

## What changed from the Claude skill

| | Claude | Copilot |
|---|---|---|
| Location | `gh-aw-field-guide.skill` (zip), installed via Save skill | `.github/skills/gh-aw-field-guide/` in the repo, or `~/.copilot/skills/` |
| Frontmatter | name, description, license | same three; nothing host-specific added |
| Resource pointers | backticked paths | relative Markdown links |
| Compile step | run if `gh aw` exists, else hand commands to the user | run in terminal; install the extension if missing |
| Coexistence | n/a | defers mechanics to upstream `agentic-workflows` skill/agent from `gh aw init` |
| Cloud-agent note | n/a | `copilot-setup-steps.yml` must install `gh aw`; never fake a lock file |
| evals/ | kept in source, excluded from package | dropped |

`.claude/skills/` is also read by Copilot, so the Claude skill directory works
unconverted if you would rather keep one copy. The conversion exists for the
terminal-first workflow and the `gh aw init` coexistence rules.
