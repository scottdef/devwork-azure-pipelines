# Cross-Repository Branch Sync

Automatically pushes branches from the dev repo to environment-specific repos when PRs merge. Uses a GitHub App with org admin permissions to bypass branch protection rules on the target repositories.

## Flow

```
synapse-repo-dev                 synapse-repo-test       synapse-repo-prod
════════════════                 ═════════════════       ═════════════════

feature/* ──PR──► dev
                   │
                   │ PR merge
                   ▼
                  test ─────────────push──────────► test
                   │                               (branch protection
                   │ PR merge                       bypassed via app)
                   ▼
                  prod ─────────────push──────────────────────► prod
```

All development happens in the dev repo. The test and prod repos receive exact copies of their corresponding branches when PRs merge in the dev repo.

## How it works

When a PR merges to the `test` branch in `synapse-repo-dev`, the `sync-test-repo.yml` workflow fires. It checks out the `test` branch with full history, generates a short-lived GitHub App installation token scoped to `synapse-repo-test`, then pushes the branch to the target repo using `x-access-token` authentication. The same process applies for the `prod` branch and `synapse-repo-prod`.

The push uses `--force` to ensure the target branch matches the source exactly. This is safe because the dev repo is the single source of truth — the target repos never have independent commits on these branches.

## GitHub App setup

The GitHub App needs:

| Permission | Scope | Purpose |
|---|---|---|
| **Contents** | Read & Write | Push to target repos |
| **Administration** | Read & Write | Bypass branch protection |

### Installation

1. Create a GitHub App at the organization level
2. Grant the permissions above
3. Install the app on all three repositories (`synapse-repo-dev`, `synapse-repo-test`, `synapse-repo-prod`)
4. Store the app credentials as secrets in `synapse-repo-dev`:

| Secret | Description |
|---|---|
| `GH_APP_ID` | The GitHub App's numeric ID |
| `GH_APP_PRIVATE_KEY` | The app's PEM private key |

The workflow uses `actions/create-github-app-token@v1` to generate a short-lived installation token at runtime. This is more secure than storing a long-lived PAT — the token expires after 1 hour and is scoped to the specific target repository.

### Branch protection bypass

For the app token to bypass branch protection, the app must be added to the "Allow specified actors to bypass required pull requests" list in the target repo's branch protection settings:

1. Go to `synapse-repo-test` → Settings → Branches → Branch protection rules
2. Edit the rule for `test`
3. Under "Allow specified actors to bypass required pull requests", add the GitHub App
4. Repeat for `synapse-repo-prod` and its `prod` branch

Alternatively, if the app has **org admin** permissions, it bypasses all branch protection implicitly.

## Alternative: pre-stored token

If you prefer not to use the runtime token generation, store a pre-generated token as `GH_APP_TOKEN` and replace the token step in both workflows:

```yaml
- name: Set token
  id: token
  run: echo "token=${{ secrets.GH_APP_TOKEN }}" >> $GITHUB_OUTPUT
```

## Files

```
synapse-repo-dev/
├── .github/workflows/
│   ├── sync-test-repo.yml    # PR merge to test → push to synapse-repo-test
│   └── sync-prod-repo.yml    # PR merge to prod → push to synapse-repo-prod
└── scripts/
    └── sync-branch-to-repo.sh  # Core push logic
```

Both workflows live in the dev repo only. The target repos don't need any sync workflows — they receive pushes passively.

## What happens in the target repos after sync

The push to the target repo triggers any workflows configured on the `push` event for that branch. For example, if `synapse-repo-test` has a deploy workflow triggered by pushes to `test`:

```yaml
# In synapse-repo-test
on:
  push:
    branches: [test]
```

That workflow fires automatically after the sync push, deploying the new content to the test workspace. This chains the sync with the deployment without any additional configuration.
