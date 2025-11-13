# GitHub Audit Log Events: Scheduled Workflow Orchestration Guide

> *"The Unix philosophy: Write programs that do one thing and do it well. Write programs to work together."* - Doug McIlroy  
> Applied here: Query once. Parse well. Trigger precisely. Audit everything.

## Table of Contents

1. [Audit Log API Fundamentals](#audit-log-api-fundamentals)
2. [Event Types Deep Dive](#event-types-deep-dive)
3. [Scheduled Query Workflows](#scheduled-query-workflows)
4. [Query Parameterization Patterns](#query-parameterization-patterns)
5. [Conditional Workflow Triggering](#conditional-workflow-triggering)
6. [Event-Driven Issue Creation](#event-driven-issue-creation)
7. [Advanced Orchestration Patterns](#advanced-orchestration-patterns)
8. [Complete Reference Implementation](#complete-reference-implementation)

---

## Audit Log API Fundamentals

The GitHub Enterprise Audit Log API provides programmatic access to organization and enterprise audit events. **Critical distinction**: This is Enterprise-only functionality - GitHub Free/Team organizations cannot access audit logs via API.

### API Endpoint Structure

```bash
# Organization audit log
GET /orgs/{org}/audit-log

# Enterprise audit log (GHEC/GHES only)
GET /enterprises/{enterprise}/audit-log
```

### Authentication Requirements

**Required Token Scopes**:
- `read:audit_log` (Fine-grained PAT)
- `admin:org` (Classic PAT)
- GitHub App with `organization_administration: read` permission

```bash
# Using gh CLI with token
export GITHUB_TOKEN=ghp_your_token_with_audit_permissions
gh api /orgs/YOUR_ORG/audit-log

# Using curl
curl -L \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer $GITHUB_TOKEN" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  https://api.github.com/orgs/YOUR_ORG/audit-log
```

### Response Structure

Every audit log entry follows this schema:

```json
{
  "@timestamp": 1699564800000,
  "action": "repo.create",
  "actor": "octocat",
  "actor_id": 1234567,
  "actor_location": {
    "country_code": "US"
  },
  "created_at": 1699564800000,
  "org": "my-org",
  "org_id": 7654321,
  "repo": "my-org/new-repo",
  "user": "octocat",
  "user_id": 1234567,
  "_document_id": "abc123def456"
}
```

---

## Event Types Deep Dive

### Organization Events (`org.*`)

Track organization-level administrative actions.

**Key Event Actions**:
```
org.add_member              # Member added to organization
org.remove_member           # Member removed from organization
org.invite_member           # Member invited
org.update_member           # Member role changed
org.disable_two_factor_requirement
org.enable_two_factor_requirement
org.advanced_security_enabled_for_new_repositories
org.audit_log_exported      # Audit log export initiated
```

**Query Example**:
```bash
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:org.add_member' \
  --field per_page=100
```

**Workflow Use Case**: Automatically create onboarding issues when new members are added.

---

### Credential Authorization Events (`org_credential_authorization.*`)

Monitor OAuth app and GitHub App authorizations.

**Key Event Actions**:
```
org_credential_authorization.grant      # OAuth/PAT authorized
org_credential_authorization.revoke     # Authorization revoked
org_credential_authorization.deauthorized
```

**Critical Fields**:
- `credential_type`: "SSH_KEY", "PERSONAL_ACCESS_TOKEN", "OAUTH_ACCESS"
- `credential_authorized_app`: Application name
- `credential_id`: Unique credential identifier

**Query Example**:
```bash
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:org_credential_authorization.grant created:>=2025-01-01' \
  --field per_page=100 \
  --jq '.[] | select(.credential_type == "PERSONAL_ACCESS_TOKEN")'
```

**Workflow Use Case**: Alert security team when new PATs are created, especially classic PATs with broad scope.

---

### Personal Access Token Events (`personal_access_token.*`)

Fine-grained PAT lifecycle events (introduced 2022).

**Key Event Actions**:
```
personal_access_token.create
personal_access_token.credential_regenerated
personal_access_token.credential_authorized
personal_access_token.access_granted
personal_access_token.access_revoked
```

**Critical Fields**:
- `token_scopes`: Array of granted scopes
- `token_id`: Unique token identifier
- `token_name`: User-defined token name

**Query Example**:
```bash
# Find all PAT creations in last 7 days
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:personal_access_token.create created:>=$(date -d "7 days ago" +%Y-%m-%d)' \
  --field per_page=100 \
  --jq '[.[] | {actor, created_at, token_scopes}]'
```

**Workflow Use Case**: Enforce policy requiring fine-grained PATs by detecting classic PAT creation and auto-creating policy violation issues.

---

### Repository Events (`repo.*`)

Comprehensive repository lifecycle tracking.

**Key Event Actions**:
```
repo.create                 # Repository created
repo.destroy                # Repository deleted
repo.rename                 # Repository renamed
repo.transfer               # Repository transferred
repo.archived               # Repository archived
repo.unarchived             # Repository unarchived
repo.access                 # Repository access changed
repo.add_member             # Collaborator added
repo.remove_member          # Collaborator removed
repo.config                 # Repository settings changed
```

**Query Example**:
```bash
# Find all repo creations by specific actor
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:repo.create actor:octocat' \
  --field per_page=100 \
  --jq '[.[] | {repo, created_at, visibility}]'
```

**Workflow Use Case**: Validate new repositories against naming conventions and security policies, auto-apply branch protection rules.

---

### Repository Visibility Events (`repository_visibility_change.*`)

Track repository visibility modifications - **critical for security compliance**.

**Key Event Actions**:
```
repository_visibility_change.disable  # Visibility changes disabled
repository_visibility_change.enable   # Visibility changes enabled
```

**Note**: Actual visibility changes appear as `repo.access` events with visibility field changes.

**Query Example**:
```bash
# Find repos that changed from private to public
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:repo.access' \
  --field per_page=100 \
  --jq '.[] | select(.visibility_before == "private" and .visibility_after == "public")'
```

**Workflow Use Case**: Immediate alert + automatic revert when repository goes from private to public without approval.

---

### Required Status Check Events (`required_status_check.*`)

Branch protection status check modifications.

**Key Event Actions**:
```
required_status_check.create   # Status check added to branch protection
required_status_check.destroy  # Status check removed from branch protection
```

**Critical Fields**:
- `pattern`: Branch pattern (e.g., "main", "release/*")
- `required_status_checks`: Array of check names

**Query Example**:
```bash
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:required_status_check.destroy' \
  --field per_page=100 \
  --jq '[.[] | {repo, pattern, actor, removed_checks: .required_status_checks}]'
```

**Workflow Use Case**: Prevent branch protection weakening by auto-restoring removed status checks.

---

### Workflow Events (`workflows.*`)

GitHub Actions workflow execution and configuration events.

**Key Event Actions**:
```
workflows.created_workflow_run         # Workflow run started
workflows.completed_workflow_run       # Workflow run completed
workflows.approved_workflow_run        # Manual approval granted
workflows.rejected_workflow_run        # Manual approval rejected
workflows.workflow_dispatch            # workflow_dispatch triggered
```

**Critical Fields**:
- `workflow_id`: Workflow file identifier
- `workflow_run_id`: Specific run identifier
- `head_branch`: Branch that triggered run
- `head_sha`: Commit SHA
- `event`: Trigger event type

**Query Example**:
```bash
# Find all workflow_dispatch events
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:workflows.workflow_dispatch' \
  --field per_page=100 \
  --jq '[.[] | {repo, workflow_id, actor, head_branch, created_at}]'
```

**Workflow Use Case**: Audit manual workflow triggers, create reports of after-hours production deployments.

---

## Scheduled Query Workflows

### Basic Scheduled Audit Query

```yaml
# .github/workflows/audit-log-monitor.yml
name: Audit Log Monitor

on:
  schedule:
    - cron: '0 * * * *'  # Every hour
  workflow_dispatch:      # Manual trigger for testing

permissions:
  contents: read
  issues: write

jobs:
  query-audit-log:
    runs-on: ubuntu-latest
    steps:
      - name: Query Audit Log
        id: query
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          # Query last hour of audit events
          SINCE=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
          
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > audit-events.json
          
          echo "event_count=$(jq 'length' audit-events.json)" >> $GITHUB_OUTPUT
      
      - name: Upload Audit Events
        uses: actions/upload-artifact@v4
        with:
          name: audit-events-${{ github.run_id }}
          path: audit-events.json
          retention-days: 90
```

### Advanced Multi-Query Pattern

```yaml
# .github/workflows/audit-security-events.yml
name: Security-Critical Audit Monitor

on:
  schedule:
    - cron: '*/15 * * * *'  # Every 15 minutes
  workflow_dispatch:

permissions:
  contents: read
  issues: write
  security-events: write

jobs:
  query-security-events:
    runs-on: ubuntu-latest
    outputs:
      has_cred_events: ${{ steps.check.outputs.has_cred_events }}
      has_pat_events: ${{ steps.check.outputs.has_pat_events }}
      has_visibility_events: ${{ steps.check.outputs.has_visibility_events }}
    steps:
      - name: Query Multiple Event Types
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '15 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
          
          # Credential authorization events
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="action:org_credential_authorization.* created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > cred-events.json
          
          # PAT events
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="action:personal_access_token.* created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > pat-events.json
          
          # Visibility change events
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="action:repo.access created:>=${SINCE}" \
            --field per_page=100 \
            --paginate \
            | jq '[.[] | select(.visibility_before != .visibility_after)]' > visibility-events.json
      
      - name: Check Event Presence
        id: check
        run: |
          echo "has_cred_events=$(jq 'length > 0' cred-events.json)" >> $GITHUB_OUTPUT
          echo "has_pat_events=$(jq 'length > 0' pat-events.json)" >> $GITHUB_OUTPUT
          echo "has_visibility_events=$(jq 'length > 0' visibility-events.json)" >> $GITHUB_OUTPUT
      
      - name: Upload Event Artifacts
        uses: actions/upload-artifact@v4
        with:
          name: security-events-${{ github.run_id }}
          path: |
            cred-events.json
            pat-events.json
            visibility-events.json
          retention-days: 90

  process-credential-events:
    needs: query-security-events
    if: needs.query-security-events.outputs.has_cred_events == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Download Events
        uses: actions/download-artifact@v4
        with:
          name: security-events-${{ github.run_id }}
      
      - name: Process Credential Events
        run: |
          echo "## Credential Authorization Events" > report.md
          jq -r '.[] | "- **\(.action)** by \(.actor) at \(.created_at) - App: \(.credential_authorized_app // "N/A")"' cred-events.json >> report.md
      
      - name: Create Issue
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const report = fs.readFileSync('report.md', 'utf8');
            
            await github.rest.issues.create({
              owner: context.repo.owner,
              repo: context.repo.repo,
              title: `🔐 Credential Authorization Events Detected`,
              body: report,
              labels: ['security', 'audit-log', 'credentials']
            });
```

---

## Query Parameterization Patterns

### Date Range Queries

```bash
# Last 24 hours
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase="created:>=$(date -u -d '1 day ago' +%Y-%m-%d)"

# Specific date range
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='created:2025-01-01..2025-01-31'

# Events after specific timestamp
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='created:>=2025-01-15T10:00:00Z'
```

### Actor-Based Queries

```bash
# Events by specific user
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='actor:octocat'

# Events by multiple users (OR logic)
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='actor:alice actor:bob'

# Exclude specific actors
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='-actor:service-account'
```

### Action Pattern Queries

```bash
# All repo events
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:repo.*'

# Multiple specific actions (OR logic)
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:repo.create action:repo.destroy'

# Combine action with other filters
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:repo.access visibility_after:public created:>=2025-01-01'
```

### Repository-Scoped Queries

```bash
# Events for specific repository
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='repo:my-org/my-repo'

# Events across multiple repos (pattern matching)
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='repo:my-org/prod-*'
```

### Complex Combined Queries

```bash
# High-risk events: repo deletions and visibility changes to public
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='(action:repo.destroy OR (action:repo.access visibility_after:public)) created:>=2025-01-01'

# PAT creations by non-admin users in last week
gh api /orgs/YOUR_ORG/audit-log \
  --field phrase='action:personal_access_token.create -actor:admin-user created:>=$(date -d "7 days ago" +%Y-%m-%d)'
```

### Parameterized Workflow Query Function

```yaml
jobs:
  parameterized-query:
    runs-on: ubuntu-latest
    steps:
      - name: Query with Parameters
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
          QUERY_ACTIONS: ${{ inputs.actions || 'repo.* org.*' }}
          QUERY_ACTOR: ${{ inputs.actor || '' }}
          QUERY_SINCE: ${{ inputs.since || '1 day ago' }}
        run: |
          # Build query dynamically
          PHRASE="action:${QUERY_ACTIONS} created:>=$(date -u -d "${QUERY_SINCE}" +%Y-%m-%d)"
          
          if [ -n "$QUERY_ACTOR" ]; then
            PHRASE="${PHRASE} actor:${QUERY_ACTOR}"
          fi
          
          echo "Query: $PHRASE"
          
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="${PHRASE}" \
            --field per_page=100 \
            --paginate > results.json
          
          jq '.' results.json
```

---

## Conditional Workflow Triggering

### Repository Event Handler

```yaml
# .github/workflows/repo-event-handler.yml
name: Repository Event Handler

on:
  schedule:
    - cron: '*/10 * * * *'
  workflow_dispatch:

permissions:
  contents: write
  issues: write

jobs:
  detect-repo-events:
    runs-on: ubuntu-latest
    outputs:
      has_new_repos: ${{ steps.check.outputs.has_new_repos }}
      has_deleted_repos: ${{ steps.check.outputs.has_deleted_repos }}
      has_renamed_repos: ${{ steps.check.outputs.has_renamed_repos }}
      new_repo_list: ${{ steps.parse.outputs.new_repos }}
    steps:
      - name: Query Repository Events
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '10 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
          
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="action:repo.create action:repo.destroy action:repo.rename created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > repo-events.json
      
      - name: Check Event Types
        id: check
        run: |
          echo "has_new_repos=$(jq '[.[] | select(.action == "repo.create")] | length > 0' repo-events.json)" >> $GITHUB_OUTPUT
          echo "has_deleted_repos=$(jq '[.[] | select(.action == "repo.destroy")] | length > 0' repo-events.json)" >> $GITHUB_OUTPUT
          echo "has_renamed_repos=$(jq '[.[] | select(.action == "repo.rename")] | length > 0' repo-events.json)" >> $GITHUB_OUTPUT
      
      - name: Parse New Repositories
        id: parse
        if: steps.check.outputs.has_new_repos == 'true'
        run: |
          NEW_REPOS=$(jq -c '[.[] | select(.action == "repo.create") | {repo, actor, created_at, visibility}]' repo-events.json)
          echo "new_repos=${NEW_REPOS}" >> $GITHUB_OUTPUT
      
      - name: Upload Events
        uses: actions/upload-artifact@v4
        with:
          name: repo-events-${{ github.run_id }}
          path: repo-events.json

  handle-new-repositories:
    needs: detect-repo-events
    if: needs.detect-repo-events.outputs.has_new_repos == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Terraform Config
        uses: actions/checkout@v4
        with:
          repository: ${{ github.repository_owner }}/gh-iac
          token: ${{ secrets.IaC_PAT }}
          path: iac
      
      - name: Validate Against Naming Convention
        env:
          NEW_REPOS: ${{ needs.detect-repo-events.outputs.new_repo_list }}
        run: |
          echo "$NEW_REPOS" | jq -r '.[] | .repo' | while read REPO_FULL; do
            REPO_NAME=$(basename "$REPO_FULL")
            
            # Check naming convention: lowercase, hyphens only, max 50 chars
            if [[ ! "$REPO_NAME" =~ ^[a-z0-9-]{1,50}$ ]]; then
              echo "::error::Repository $REPO_NAME violates naming convention"
              echo "REPO_NAME=${REPO_NAME}" >> naming-violations.txt
            fi
          done
      
      - name: Create Policy Violation Issues
        if: hashFiles('naming-violations.txt') != ''
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            if (!fs.existsSync('naming-violations.txt')) return;
            
            const violations = fs.readFileSync('naming-violations.txt', 'utf8')
              .split('\n')
              .filter(line => line.startsWith('REPO_NAME='))
              .map(line => line.replace('REPO_NAME=', ''));
            
            for (const repoName of violations) {
              await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `🚨 Repository Naming Violation: ${repoName}`,
                body: `Repository \`${repoName}\` was created but violates naming conventions.\n\n` +
                      `**Requirements:**\n` +
                      `- Lowercase letters only\n` +
                      `- Hyphens for word separation\n` +
                      `- Maximum 50 characters\n` +
                      `- No underscores or special characters\n\n` +
                      `**Action Required:** Rename or delete this repository.`,
                labels: ['policy-violation', 'naming-convention', 'needs-action'],
                assignees: ['security-team']
              });
            }
      
      - name: Apply Branch Protection
        env:
          GH_TOKEN: ${{ secrets.ORG_ADMIN_PAT }}
          NEW_REPOS: ${{ needs.detect-repo-events.outputs.new_repo_list }}
        run: |
          echo "$NEW_REPOS" | jq -r '.[] | .repo' | while read REPO_FULL; do
            echo "Applying branch protection to $REPO_FULL"
            
            # Check if main branch exists
            if gh api "/repos/${REPO_FULL}/branches/main" 2>/dev/null; then
              # Apply branch protection
              gh api "/repos/${REPO_FULL}/branches/main/protection" \
                -X PUT \
                -f required_status_checks='{"strict":true,"contexts":["ci-tests"]}' \
                -f enforce_admins=true \
                -f required_pull_request_reviews='{"required_approving_review_count":1,"dismiss_stale_reviews":true}' \
                -f restrictions=null
              
              echo "✅ Branch protection applied to $REPO_FULL"
            else
              echo "⚠️  No main branch in $REPO_FULL yet"
            fi
          done

  handle-deleted-repositories:
    needs: detect-repo-events
    if: needs.detect-repo-events.outputs.has_deleted_repos == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Download Events
        uses: actions/download-artifact@v4
        with:
          name: repo-events-${{ github.run_id }}
      
      - name: Alert on Deletions
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const events = JSON.parse(fs.readFileSync('repo-events.json', 'utf8'));
            const deletions = events.filter(e => e.action === 'repo.destroy');
            
            for (const event of deletions) {
              await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `⚠️ Repository Deleted: ${event.repo}`,
                body: `**Repository:** ${event.repo}\n` +
                      `**Deleted by:** ${event.actor}\n` +
                      `**Timestamp:** ${event.created_at}\n\n` +
                      `Please verify this deletion was intentional and properly documented.`,
                labels: ['deletion-alert', 'audit', 'requires-review']
              });
            }
```

### Security Event Dispatcher

```yaml
# .github/workflows/security-event-dispatcher.yml
name: Security Event Dispatcher

on:
  schedule:
    - cron: '*/5 * * * *'
  workflow_dispatch:

permissions:
  contents: read

jobs:
  query-and-dispatch:
    runs-on: ubuntu-latest
    steps:
      - name: Query Security Events
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '5 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
          
          # Query all security-relevant events
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="(action:org_credential_authorization.* OR action:personal_access_token.* OR action:repo.access OR action:required_status_check.destroy) created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > events.json
      
      - name: Categorize Events
        id: categorize
        run: |
          # Categorize by severity and type
          jq '[.[] | select(.action | startswith("org_credential_authorization"))] | length > 0' events.json > has_cred.txt
          jq '[.[] | select(.action | startswith("personal_access_token"))] | length > 0' events.json > has_pat.txt
          jq '[.[] | select(.action == "repo.access" and .visibility_after == "public")] | length > 0' events.json > has_public.txt
          jq '[.[] | select(.action == "required_status_check.destroy")] | length > 0' events.json > has_check_removal.txt
          
          echo "has_credential_events=$(cat has_cred.txt)" >> $GITHUB_OUTPUT
          echo "has_pat_events=$(cat has_pat.txt)" >> $GITHUB_OUTPUT
          echo "has_public_visibility=$(cat has_public.txt)" >> $GITHUB_OUTPUT
          echo "has_check_removals=$(cat has_check_removal.txt)" >> $GITHUB_OUTPUT
      
      - name: Trigger Credential Handler
        if: steps.categorize.outputs.has_credential_events == 'true'
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.WORKFLOW_TRIGGER_PAT }}
          script: |
            await github.rest.actions.createWorkflowDispatch({
              owner: context.repo.owner,
              repo: context.repo.repo,
              workflow_id: 'credential-event-handler.yml',
              ref: 'main',
              inputs: {
                run_id: `${context.runId}`,
                event_file: 'events.json'
              }
            });
      
      - name: Trigger PAT Handler
        if: steps.categorize.outputs.has_pat_events == 'true'
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.WORKFLOW_TRIGGER_PAT }}
          script: |
            await github.rest.actions.createWorkflowDispatch({
              owner: context.repo.owner,
              repo: context.repo.repo,
              workflow_id: 'pat-event-handler.yml',
              ref: 'main'
            });
      
      - name: Trigger CRITICAL Visibility Handler
        if: steps.categorize.outputs.has_public_visibility == 'true'
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.WORKFLOW_TRIGGER_PAT }}
          script: |
            await github.rest.actions.createWorkflowDispatch({
              owner: context.repo.owner,
              repo: context.repo.repo,
              workflow_id: 'visibility-revert-handler.yml',
              ref: 'main'
            });
      
      - name: Upload Events
        uses: actions/upload-artifact@v4
        with:
          name: security-events-${{ github.run_id }}
          path: events.json
          retention-days: 365  # Long retention for security events
```

---

## Event-Driven Issue Creation

### PAT Creation Issue Template

```yaml
# .github/workflows/pat-event-handler.yml
name: PAT Event Handler

on:
  workflow_dispatch:
    inputs:
      run_id:
        description: 'Source workflow run ID'
        required: false

permissions:
  issues: write
  contents: read

jobs:
  create-pat-issues:
    runs-on: ubuntu-latest
    steps:
      - name: Download or Query Events
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          if [ -n "${{ inputs.run_id }}" ]; then
            # Download from artifact
            gh api "/repos/${GITHUB_REPOSITORY}/actions/runs/${{ inputs.run_id }}/artifacts" \
              --jq '.artifacts[] | select(.name | contains("security-events")) | .archive_download_url' \
              | xargs -I {} gh api {} > artifact.zip
            unzip -p artifact.zip events.json > pat-events.json
          else
            # Fresh query
            SINCE=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
            gh api "/orgs/${ORG_NAME}/audit-log" \
              --field phrase="action:personal_access_token.create created:>=${SINCE}" \
              --field per_page=100 \
              --paginate > pat-events.json
          fi
          
          # Filter for classic PATs (higher risk)
          jq '[.[] | select(.token_scopes | contains(["repo"]) or contains(["admin:org"]))]' pat-events.json > high-risk-pats.json
      
      - name: Create Issues for High-Risk PATs
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const events = JSON.parse(fs.readFileSync('high-risk-pats.json', 'utf8'));
            
            for (const event of events) {
              const scopes = event.token_scopes ? event.token_scopes.join(', ') : 'N/A';
              const tokenName = event.token_name || 'Unnamed Token';
              
              const issueBody = `## 🔐 High-Risk Personal Access Token Created
              
**Actor:** ${event.actor}
**Token Name:** ${tokenName}
**Scopes:** \`${scopes}\`
**Created:** ${event.created_at}
**Event ID:** ${event._document_id}

### Risk Assessment

This PAT was granted broad permissions that may pose security risks:

${event.token_scopes?.includes('repo') ? '- ✅ Full repository access granted\n' : ''}
${event.token_scopes?.includes('admin:org') ? '- ✅ Organization admin access granted\n' : ''}
${event.token_scopes?.includes('delete_repo') ? '- ⚠️ Repository deletion capability\n' : ''}

### Recommended Actions

1. **Verify Necessity:** Confirm these permissions are actually required
2. **Consider Fine-Grained PAT:** Recommend migration to fine-grained PAT with limited scope
3. **Set Expiration:** Ensure token has reasonable expiration date
4. **Document Use Case:** Update token description with intended use

### Policy Compliance

- [ ] Use case documented
- [ ] Security team notified
- [ ] Token expiration set (max 90 days)
- [ ] Alternative fine-grained PAT considered

---
*Automated issue created from audit log monitoring*`;

              const labels = ['security', 'pat-created', 'needs-review'];
              if (event.token_scopes?.includes('admin:org')) {
                labels.push('high-risk');
              }

              await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `🔐 PAT Created: ${tokenName} by ${event.actor}`,
                body: issueBody,
                labels: labels,
                assignees: ['security-team']
              });
              
              console.log(`Created issue for PAT: ${tokenName}`);
            }
```

### Repository Visibility Change Alert

```yaml
# .github/workflows/visibility-revert-handler.yml
name: Repository Visibility Change Handler

on:
  workflow_dispatch:
  schedule:
    - cron: '*/2 * * * *'  # Every 2 minutes for critical security

permissions:
  issues: write
  contents: write

jobs:
  handle-visibility-changes:
    runs-on: ubuntu-latest
    steps:
      - name: Query Visibility Changes
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '2 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
          
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="action:repo.access created:>=${SINCE}" \
            --field per_page=100 \
            --paginate \
            | jq '[.[] | select(.visibility_before != .visibility_after)]' > visibility-changes.json
          
          # Extract repos that went public
          jq '[.[] | select(.visibility_after == "public")]' visibility-changes.json > public-changes.json
      
      - name: Create CRITICAL Issues
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const changes = JSON.parse(fs.readFileSync('public-changes.json', 'utf8'));
            
            for (const change of changes) {
              const issueBody = `## 🚨 CRITICAL: Repository Made Public
              
**Repository:** ${change.repo}
**Changed By:** ${change.actor}
**Previous Visibility:** ${change.visibility_before}
**Current Visibility:** ${change.visibility_after}
**Timestamp:** ${change.created_at}
**Event ID:** ${change._document_id}

### Immediate Actions Required

⚠️ **This is a CRITICAL security event requiring immediate attention.**

1. **Verify Intentionality:** Confirm this change was intentional and approved
2. **Review Contents:** Audit repository for sensitive data exposure
3. **Assess Impact:** Determine if secrets, credentials, or proprietary code were exposed
4. **Consider Reversion:** If unauthorized, repository will be automatically reverted to private

### Auto-Reversion Status

If this change was NOT authorized with a pre-approved ticket, this repository will be automatically reverted to private status in 5 minutes.

To prevent auto-reversion, add label \`visibility-change-approved\` to this issue.

---
*Automated alert from audit log monitoring - Immediate action required*`;

              const issue = await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `🚨 CRITICAL: ${change.repo} Made Public`,
                body: issueBody,
                labels: ['critical', 'security', 'visibility-change', 'requires-immediate-action'],
                assignees: ['security-team', 'ciso']
              });
              
              // Store issue number for reversion check
              fs.appendFileSync('issue-mapping.txt', `${change.repo}|${issue.data.number}\n`);
            }
      
      - name: Wait for Approval or Auto-Revert
        if: hashFiles('public-changes.json') != '' && hashFiles('issue-mapping.txt') != ''
        run: sleep 300  # 5 minute grace period
      
      - name: Check for Approvals
        id: check-approval
        if: hashFiles('issue-mapping.txt') != ''
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const mappings = fs.readFileSync('issue-mapping.txt', 'utf8')
              .trim()
              .split('\n')
              .map(line => {
                const [repo, issueNum] = line.split('|');
                return { repo, issueNumber: parseInt(issueNum) };
              });
            
            const unapprovedRepos = [];
            
            for (const mapping of mappings) {
              const issue = await github.rest.issues.get({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: mapping.issueNumber
              });
              
              const hasApproval = issue.data.labels.some(l => l.name === 'visibility-change-approved');
              
              if (!hasApproval) {
                unapprovedRepos.push(mapping.repo);
              }
            }
            
            fs.writeFileSync('unapproved-repos.txt', unapprovedRepos.join('\n'));
            core.setOutput('has_unapproved', unapprovedRepos.length > 0);
      
      - name: Auto-Revert Unapproved Changes
        if: steps.check-approval.outputs.has_unapproved == 'true'
        env:
          GH_TOKEN: ${{ secrets.ORG_ADMIN_PAT }}
        run: |
          cat unapproved-repos.txt | while read REPO; do
            echo "Reverting $REPO to private..."
            
            gh api "/repos/${REPO}" \
              -X PATCH \
              -f visibility='private'
            
            echo "✅ Reverted $REPO to private"
          done
      
      - name: Update Issues with Reversion Status
        if: steps.check-approval.outputs.has_unapproved == 'true'
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const unapproved = fs.readFileSync('unapproved-repos.txt', 'utf8').trim().split('\n');
            const mappings = fs.readFileSync('issue-mapping.txt', 'utf8')
              .trim()
              .split('\n')
              .map(line => {
                const [repo, issueNum] = line.split('|');
                return { repo, issueNumber: parseInt(issueNum) };
              })
              .filter(m => unapproved.includes(m.repo));
            
            for (const mapping of mappings) {
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: mapping.issueNumber,
                body: `## ✅ Auto-Reversion Completed\n\n` +
                      `Repository has been automatically reverted to **private** visibility.\n\n` +
                      `If you require public visibility, please:\n` +
                      `1. Create a change request ticket\n` +
                      `2. Obtain security team approval\n` +
                      `3. Re-apply visibility change with proper documentation`
              });
              
              await github.rest.issues.addLabels({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: mapping.issueNumber,
                labels: ['auto-reverted']
              });
            }
```

### Branch Protection Weakening Detector

```yaml
# .github/workflows/branch-protection-monitor.yml
name: Branch Protection Monitor

on:
  schedule:
    - cron: '*/10 * * * *'
  workflow_dispatch:

permissions:
  issues: write
  contents: write

jobs:
  monitor-protection-changes:
    runs-on: ubuntu-latest
    steps:
      - name: Query Status Check Removals
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '10 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
          
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="action:required_status_check.destroy created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > check-removals.json
      
      - name: Create Remediation Issues
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const events = JSON.parse(fs.readFileSync('check-removals.json', 'utf8'));
            
            for (const event of events) {
              const checks = event.required_status_checks ? event.required_status_checks.join(', ') : 'Unknown';
              
              const issueBody = `## ⚠️ Branch Protection Weakened
              
**Repository:** ${event.repo}
**Branch Pattern:** ${event.pattern || 'main'}
**Removed By:** ${event.actor}
**Removed Checks:** ${checks}
**Timestamp:** ${event.created_at}
**Event ID:** ${event._document_id}

### Security Impact

Required status checks were removed from branch protection rules. This weakens the security posture by allowing merges without validation.

### Automatic Remediation

Branch protection rules will be automatically restored in 10 minutes unless this issue is labeled with \`protection-change-approved\`.

### Approval Process

If this change is intentional:
1. Document the business justification in this issue
2. Add label \`protection-change-approved\`
3. Update security policies to reflect the change

---
*Automated security monitoring - Branch protection enforcement*`;

              const issue = await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `⚠️ Branch Protection Weakened: ${event.repo}`,
                body: issueBody,
                labels: ['security', 'branch-protection', 'needs-review'],
                assignees: ['platform-team']
              });
              
              // Store for remediation
              fs.appendFileSync('remediation-map.json', JSON.stringify({
                repo: event.repo,
                pattern: event.pattern || 'main',
                checks: event.required_status_checks || [],
                issueNumber: issue.data.number
              }) + '\n');
            }
      
      - name: Wait for Approval
        if: hashFiles('check-removals.json') != ''
        run: sleep 600  # 10 minute grace period
      
      - name: Restore Protection Rules
        if: hashFiles('remediation-map.json') != ''
        env:
          GH_TOKEN: ${{ secrets.ORG_ADMIN_PAT }}
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const remediations = fs.readFileSync('remediation-map.json', 'utf8')
              .trim()
              .split('\n')
              .map(line => JSON.parse(line));
            
            for (const item of remediations) {
              // Check if approved
              const issue = await github.rest.issues.get({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: item.issueNumber
              });
              
              const isApproved = issue.data.labels.some(l => l.name === 'protection-change-approved');
              
              if (!isApproved && item.checks.length > 0) {
                // Restore the checks
                const [owner, repo] = item.repo.split('/');
                
                await github.rest.repos.updateBranchProtection({
                  owner,
                  repo,
                  branch: item.pattern,
                  required_status_checks: {
                    strict: true,
                    contexts: item.checks
                  },
                  enforce_admins: true,
                  required_pull_request_reviews: {
                    required_approving_review_count: 1,
                    dismiss_stale_reviews: true
                  },
                  restrictions: null
                });
                
                await github.rest.issues.createComment({
                  owner: context.repo.owner,
                  repo: context.repo.repo,
                  issue_number: item.issueNumber,
                  body: `## ✅ Branch Protection Restored\n\n` +
                        `Required status checks have been automatically restored:\n` +
                        `${item.checks.map(c => `- ${c}`).join('\n')}`
                });
                
                await github.rest.issues.addLabels({
                  owner: context.repo.owner,
                  repo: context.repo.repo,
                  issue_number: item.issueNumber,
                  labels: ['auto-remediated']
                });
              }
            }
```

---

## Advanced Orchestration Patterns

### Workflow Dispatch Trigger Chain

```yaml
# .github/workflows/audit-event-orchestrator.yml
name: Audit Event Orchestrator

on:
  schedule:
    - cron: '0 * * * *'  # Hourly orchestration
  workflow_dispatch:

permissions:
  contents: read
  actions: write

jobs:
  orchestrate:
    runs-on: ubuntu-latest
    steps:
      - name: Query All Event Categories
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
          
          # Query each event category
          for category in "org.*" "repo.*" "personal_access_token.*" "workflows.*"; do
            SAFE_NAME=$(echo "$category" | tr '.' '_' | tr '*' 'all')
            
            gh api "/orgs/${ORG_NAME}/audit-log" \
              --field phrase="action:${category} created:>=${SINCE}" \
              --field per_page=100 \
              --paginate > "${SAFE_NAME}-events.json"
            
            EVENT_COUNT=$(jq 'length' "${SAFE_NAME}-events.json")
            echo "${SAFE_NAME}_count=${EVENT_COUNT}" >> $GITHUB_ENV
          done
      
      - name: Trigger Repository Handler
        if: env.repo_all_count > 0
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.WORKFLOW_TRIGGER_PAT }}
          script: |
            const fs = require('fs');
            const events = JSON.parse(fs.readFileSync('repo_all-events.json', 'utf8'));
            
            // Pass event summary as input
            await github.rest.actions.createWorkflowDispatch({
              owner: context.repo.owner,
              repo: context.repo.repo,
              workflow_id: 'repo-event-handler.yml',
              ref: 'main',
              inputs: {
                event_count: `${events.length}`,
                source_run_id: `${context.runId}`
              }
            });
      
      - name: Trigger PAT Handler
        if: env.personal_access_token_all_count > 0
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.WORKFLOW_TRIGGER_PAT }}
          script: |
            await github.rest.actions.createWorkflowDispatch({
              owner: context.repo.owner,
              repo: context.repo.repo,
              workflow_id: 'pat-event-handler.yml',
              ref: 'main',
              inputs: {
                source_run_id: `${context.runId}`
              }
            });
      
      - name: Trigger Workflow Audit
        if: env.workflows_all_count > 0
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.WORKFLOW_TRIGGER_PAT }}
          script: |
            await github.rest.actions.createWorkflowDispatch({
              owner: context.repo.owner,
              repo: context.repo.repo,
              workflow_id: 'workflow-audit-handler.yml',
              ref: 'main',
              inputs: {
                source_run_id: `${context.runId}`
              }
            });
      
      - name: Upload All Events
        uses: actions/upload-artifact@v4
        with:
          name: audit-events-${{ github.run_id }}
          path: |
            *-events.json
          retention-days: 90
```

### Repository Dispatch Webhook Pattern

```yaml
# .github/workflows/audit-webhook-forwarder.yml
name: Audit Event Webhook Forwarder

on:
  schedule:
    - cron: '*/5 * * * *'
  workflow_dispatch:

jobs:
  forward-to-external-systems:
    runs-on: ubuntu-latest
    steps:
      - name: Query Recent Events
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
        run: |
          SINCE=$(date -u -d '5 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
          
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > events.json
      
      - name: Forward to SIEM
        env:
          SPLUNK_HEC_URL: ${{ secrets.SPLUNK_HEC_URL }}
          SPLUNK_HEC_TOKEN: ${{ secrets.SPLUNK_HEC_TOKEN }}
        run: |
          jq -c '.[]' events.json | while read event; do
            curl -k -X POST "${SPLUNK_HEC_URL}" \
              -H "Authorization: Splunk ${SPLUNK_HEC_TOKEN}" \
              -H "Content-Type: application/json" \
              -d "{\"event\": ${event}, \"sourcetype\": \"github:audit\"}"
          done
      
      - name: Forward to Slack
        env:
          SLACK_WEBHOOK: ${{ secrets.SLACK_SECURITY_WEBHOOK }}
        run: |
          HIGH_RISK=$(jq '[.[] | select(.action | contains("destroy") or contains("delete") or (.action == "repo.access" and .visibility_after == "public"))]' events.json)
          
          if [ "$(echo "$HIGH_RISK" | jq 'length')" -gt 0 ]; then
            MESSAGE=$(echo "$HIGH_RISK" | jq -r '
              "⚠️ *High-Risk Audit Events Detected*\n\n" + 
              (.[] | "• *\(.action)* by \(.actor) - \(.repo // .org)\n") + 
              "\n<https://github.com/orgs/'"$ORG_NAME"'/audit-log|View Full Audit Log>"
            ')
            
            curl -X POST "${SLACK_WEBHOOK}" \
              -H "Content-Type: application/json" \
              -d "{\"text\": \"${MESSAGE}\"}"
          fi
      
      - name: Trigger Repository Dispatch
        uses: actions/github-script@v7
        with:
          github-token: ${{ secrets.REPO_DISPATCH_PAT }}
          script: |
            const fs = require('fs');
            const events = JSON.parse(fs.readFileSync('events.json', 'utf8'));
            
            if (events.length > 0) {
              // Trigger external repo to process events
              await github.rest.repos.createDispatchEvent({
                owner: 'my-org',
                repo: 'security-automation',
                event_type: 'audit-events-received',
                client_payload: {
                  event_count: events.length,
                  source: 'github-audit-log',
                  timestamp: new Date().toISOString(),
                  events: events.slice(0, 10)  // Send first 10 as sample
                }
              });
            }
```

### Stateful Event Tracking with Issues

```yaml
# .github/workflows/stateful-audit-tracker.yml
name: Stateful Audit Event Tracker

on:
  schedule:
    - cron: '0 */6 * * *'  # Every 6 hours
  workflow_dispatch:

permissions:
  issues: write
  contents: read

jobs:
  track-cumulative-events:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Tracking Repo
        uses: actions/checkout@v4
        with:
          repository: ${{ github.repository_owner }}/audit-tracking
          token: ${{ secrets.TRACKING_PAT }}
      
      - name: Load Last Processed Timestamp
        id: load-state
        run: |
          if [ -f last-processed.txt ]; then
            LAST=$(cat last-processed.txt)
            echo "last_timestamp=${LAST}" >> $GITHUB_OUTPUT
          else
            # Default to 6 hours ago
            LAST=$(date -u -d '6 hours ago' +%Y-%m-%dT%H:%M:%SZ)
            echo "last_timestamp=${LAST}" >> $GITHUB_OUTPUT
          fi
      
      - name: Query Events Since Last Run
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
          ORG_NAME: ${{ github.repository_owner }}
          SINCE: ${{ steps.load-state.outputs.last_timestamp }}
        run: |
          gh api "/orgs/${ORG_NAME}/audit-log" \
            --field phrase="created:>=${SINCE}" \
            --field per_page=100 \
            --paginate > new-events.json
          
          EVENT_COUNT=$(jq 'length' new-events.json)
          echo "Found ${EVENT_COUNT} new events"
      
      - name: Update Tracking Issue
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const events = JSON.parse(fs.readFileSync('new-events.json', 'utf8'));
            
            if (events.length === 0) {
              console.log('No new events to track');
              return;
            }
            
            // Find or create tracking issue
            const issues = await github.rest.issues.listForRepo({
              owner: context.repo.owner,
              repo: context.repo.repo,
              labels: 'audit-tracker',
              state: 'open'
            });
            
            let trackingIssue;
            if (issues.data.length > 0) {
              trackingIssue = issues.data[0];
            } else {
              trackingIssue = await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: '📊 Audit Event Tracking Dashboard',
                body: '# Audit Event Statistics\n\nTracking cumulative audit events...',
                labels: ['audit-tracker', 'dashboard']
              });
              trackingIssue = trackingIssue.data;
            }
            
            // Aggregate statistics
            const actionCounts = {};
            events.forEach(e => {
              actionCounts[e.action] = (actionCounts[e.action] || 0) + 1;
            });
            
            const updateBody = `## Update: ${new Date().toISOString()}
            
**New Events:** ${events.length}
**Time Range:** ${events[events.length - 1]?.created_at} to ${events[0]?.created_at}

### Event Breakdown
${Object.entries(actionCounts)
  .sort((a, b) => b[1] - a[1])
  .slice(0, 10)
  .map(([action, count]) => `- ${action}: ${count}`)
  .join('\n')}

### Top Actors
${Object.entries(events.reduce((acc, e) => {
  acc[e.actor] = (acc[e.actor] || 0) + 1;
  return acc;
}, {}))
  .sort((a, b) => b[1] - a[1])
  .slice(0, 5)
  .map(([actor, count]) => `- ${actor}: ${count} events`)
  .join('\n')}

---
*Last updated: ${new Date().toISOString()}*`;

            await github.rest.issues.createComment({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: trackingIssue.number,
              body: updateBody
            });
      
      - name: Update State
        run: |
          date -u +%Y-%m-%dT%H:%M:%SZ > last-processed.txt
          git config user.name "Audit Bot"
          git config user.email "audit-bot@example.com"
          git add last-processed.txt
          git commit -m "Update last processed timestamp"
          git push
```

---

## Complete Reference Implementation

### Project Structure

```
audit-log-automation/
├── .github/
│   ├── workflows/
│   │   ├── audit-orchestrator.yml           # Main scheduler
│   │   ├── repo-event-handler.yml          # Repository events
│   │   ├── pat-event-handler.yml           # PAT events
│   │   ├── visibility-revert-handler.yml   # Critical security
│   │   ├── branch-protection-monitor.yml   # Protection rules
│   │   ├── workflow-audit-handler.yml      # Workflow events
│   │   └── audit-dashboard-update.yml      # Reporting
│   └── scripts/
│       ├── query-audit-log.sh              # Reusable query script
│       ├── parse-events.py                 # Event parser
│       └── create-issue.sh                 # Issue creator
├── configs/
│   ├── event-policies.yaml                 # Event handling policies
│   └── notification-rules.yaml             # Alert routing
└── README.md
```

### Reusable Query Script

```bash
#!/bin/bash
# scripts/query-audit-log.sh

set -euo pipefail

# Configuration
ORG_NAME="${GH_ORG:-${GITHUB_REPOSITORY_OWNER}}"
GITHUB_TOKEN="${GH_TOKEN:-${GITHUB_TOKEN}}"
OUTPUT_FILE="${1:-audit-events.json}"
SINCE="${2:-1 hour ago}"
ACTIONS="${3:-*}"

# Convert relative time to ISO8601
SINCE_ISO=$(date -u -d "${SINCE}" +%Y-%m-%dT%H:%M:%SZ)

# Build query
PHRASE="created:>=${SINCE_ISO}"
if [ "$ACTIONS" != "*" ]; then
  PHRASE="${PHRASE} action:${ACTIONS}"
fi

echo "Querying audit log for: ${PHRASE}"

# Execute query with pagination
gh api "/orgs/${ORG_NAME}/audit-log" \
  --field phrase="${PHRASE}" \
  --field per_page=100 \
  --paginate > "${OUTPUT_FILE}"

EVENT_COUNT=$(jq 'length' "${OUTPUT_FILE}")
echo "Retrieved ${EVENT_COUNT} events"
echo "event_count=${EVENT_COUNT}" >> "${GITHUB_OUTPUT:-/dev/null}"
```

### Event Parser Python Script

```python
#!/usr/bin/env python3
# scripts/parse-events.py

import json
import sys
from typing import Dict, List
from datetime import datetime

def parse_events(events: List[Dict]) -> Dict:
    """Parse and categorize audit events."""
    
    categories = {
        'org': [],
        'repo': [],
        'credentials': [],
        'workflows': [],
        'security': [],
        'other': []
    }
    
    for event in events:
        action = event.get('action', '')
        
        if action.startswith('org.'):
            categories['org'].append(event)
        elif action.startswith('repo.'):
            categories['repo'].append(event)
        elif action.startswith('personal_access_token.') or \
             action.startswith('org_credential_authorization.'):
            categories['credentials'].append(event)
        elif action.startswith('workflows.'):
            categories['workflows'].append(event)
        elif action in ['repo.access'] and \
             event.get('visibility_after') == 'public':
            categories['security'].append(event)
        else:
            categories['other'].append(event)
    
    return categories

def generate_summary(categories: Dict) -> str:
    """Generate markdown summary of events."""
    
    summary = "# Audit Log Event Summary\n\n"
    summary += f"**Generated:** {datetime.utcnow().isoformat()}\n\n"
    
    for category, events in categories.items():
        if not events:
            continue
        
        summary += f"## {category.capitalize()} Events ({len(events)})\n\n"
        
        # Top actions
        action_counts = {}
        for event in events:
            action = event.get('action', 'unknown')
            action_counts[action] = action_counts.get(action, 0) + 1
        
        summary += "### Top Actions\n"
        for action, count in sorted(action_counts.items(), 
                                   key=lambda x: x[1], 
                                   reverse=True)[:5]:
            summary += f"- `{action}`: {count}\n"
        
        summary += "\n"
    
    return summary

if __name__ == '__main__':
    input_file = sys.argv[1] if len(sys.argv) > 1 else 'audit-events.json'
    output_file = sys.argv[2] if len(sys.argv) > 2 else 'summary.md'
    
    with open(input_file, 'r') as f:
        events = json.load(f)
    
    categories = parse_events(events)
    summary = generate_summary(categories)
    
    with open(output_file, 'w') as f:
        f.write(summary)
    
    print(f"Parsed {len(events)} events into {output_file}")
```

### Master Orchestration Workflow

```yaml
# .github/workflows/audit-orchestrator.yml
name: Audit Log Orchestrator

on:
  schedule:
    - cron: '0 * * * *'  # Hourly
  workflow_dispatch:
    inputs:
      timerange:
        description: 'Time range to query'
        required: false
        default: '1 hour ago'
      force_all:
        description: 'Force trigger all handlers'
        type: boolean
        default: false

permissions:
  contents: read
  issues: write
  actions: write

env:
  ORG_NAME: ${{ github.repository_owner }}
  TIMERANGE: ${{ inputs.timerange || '1 hour ago' }}

jobs:
  query-and-categorize:
    runs-on: ubuntu-latest
    outputs:
      has_org_events: ${{ steps.categorize.outputs.has_org_events }}
      has_repo_events: ${{ steps.categorize.outputs.has_repo_events }}
      has_cred_events: ${{ steps.categorize.outputs.has_cred_events }}
      has_workflow_events: ${{ steps.categorize.outputs.has_workflow_events }}
      has_security_events: ${{ steps.categorize.outputs.has_security_events }}
      run_id: ${{ github.run_id }}
    
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      
      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'
      
      - name: Query Audit Log
        env:
          GH_TOKEN: ${{ secrets.AUDIT_LOG_PAT }}
        run: |
          chmod +x .github/scripts/query-audit-log.sh
          .github/scripts/query-audit-log.sh audit-events.json "$TIMERANGE"
      
      - name: Parse and Categorize
        run: |
          python .github/scripts/parse-events.py audit-events.json summary.md
          
          # Split into category files
          jq '[.[] | select(.action | startswith("org."))]' audit-events.json > org-events.json
          jq '[.[] | select(.action | startswith("repo."))]' audit-events.json > repo-events.json
          jq '[.[] | select(.action | startswith("personal_access_token.") or startswith("org_credential_authorization."))]' audit-events.json > cred-events.json
          jq '[.[] | select(.action | startswith("workflows."))]' audit-events.json > workflow-events.json
          jq '[.[] | select(.action == "repo.access" and .visibility_after == "public")]' audit-events.json > security-events.json
      
      - name: Check Categories
        id: categorize
        run: |
          echo "has_org_events=$(jq 'length > 0' org-events.json)" >> $GITHUB_OUTPUT
          echo "has_repo_events=$(jq 'length > 0' repo-events.json)" >> $GITHUB_OUTPUT
          echo "has_cred_events=$(jq 'length > 0' cred-events.json)" >> $GITHUB_OUTPUT
          echo "has_workflow_events=$(jq 'length > 0' workflow-events.json)" >> $GITHUB_OUTPUT
          echo "has_security_events=$(jq 'length > 0' security-events.json)" >> $GITHUB_OUTPUT
      
      - name: Upload Artifacts
        uses: actions/upload-artifact@v4
        with:
          name: audit-events-${{ github.run_id }}
          path: |
            *.json
            summary.md
          retention-days: 90

  trigger-repo-handler:
    needs: query-and-categorize
    if: needs.query-and-categorize.outputs.has_repo_events == 'true' || inputs.force_all
    uses: ./.github/workflows/repo-event-handler.yml
    secrets: inherit
    with:
      source_run_id: ${{ needs.query-and-categorize.outputs.run_id }}

  trigger-cred-handler:
    needs: query-and-categorize
    if: needs.query-and-categorize.outputs.has_cred_events == 'true' || inputs.force_all
    uses: ./.github/workflows/pat-event-handler.yml
    secrets: inherit
    with:
      source_run_id: ${{ needs.query-and-categorize.outputs.run_id }}

  trigger-security-handler:
    needs: query-and-categorize
    if: needs.query-and-categorize.outputs.has_security_events == 'true' || inputs.force_all
    uses: ./.github/workflows/visibility-revert-handler.yml
    secrets: inherit
    with:
      source_run_id: ${{ needs.query-and-categorize.outputs.run_id }}

  trigger-workflow-handler:
    needs: query-and-categorize
    if: needs.query-and-categorize.outputs.has_workflow_events == 'true' || inputs.force_all
    uses: ./.github/workflows/workflow-audit-handler.yml
    secrets: inherit
    with:
      source_run_id: ${{ needs.query-and-categorize.outputs.run_id }}

  update-dashboard:
    needs: query-and-categorize
    runs-on: ubuntu-latest
    steps:
      - name: Download Artifacts
        uses: actions/download-artifact@v4
        with:
          name: audit-events-${{ needs.query-and-categorize.outputs.run_id }}
      
      - name: Update Dashboard Issue
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const summary = fs.readFileSync('summary.md', 'utf8');
            
            // Find dashboard issue
            const issues = await github.rest.issues.listForRepo({
              owner: context.repo.owner,
              repo: context.repo.repo,
              labels: 'audit-dashboard',
              state: 'open'
            });
            
            let dashboardIssue;
            if (issues.data.length > 0) {
              dashboardIssue = issues.data[0];
              
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: dashboardIssue.number,
                body: summary
              });
            } else {
              await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: '📊 Audit Log Dashboard',
                body: summary,
                labels: ['audit-dashboard']
              });
            }
```

---

## Best Practices and Recommendations

### 1. Token Security

- **Use fine-grained PATs** for audit log access with minimal scope
- **Rotate tokens** every 90 days
- **Store in GitHub Secrets** with descriptive names (`AUDIT_LOG_PAT`)
- **Audit token usage** through audit logs themselves

### 2. Query Efficiency

- **Use time-based filters** to limit result set size
- **Implement pagination** for large organizations
- **Cache results** in artifacts for reuse across jobs
- **Rate limit handling**: Implement exponential backoff

### 3. Event Processing

- **Idempotency**: Ensure handlers can run multiple times safely
- **Deduplication**: Track processed events to avoid duplicate actions
- **Error handling**: Always use `continue-on-error` for non-critical steps
- **Retry logic**: Implement retries for API failures

### 4. Issue Management

- **Use labels** for categorization and filtering
- **Set assignees** programmatically based on event type
- **Close stale issues** automatically after resolution
- **Link related issues** for audit trail

### 5. Security Considerations

- **Minimize approval windows** for critical security events (2-5 minutes)
- **Implement auto-revert** for unauthorized changes
- **Alert multiple channels** (Slack, PagerDuty, email)
- **Log all automated actions** for compliance

### 6. Compliance and Auditing

- **Retain artifacts** for required compliance period (typically 90-365 days)
- **Export to SIEM** for long-term storage and analysis
- **Document all workflows** with clear descriptions
- **Regular reviews** of automation effectiveness

---

## Troubleshooting

### Common Issues

**Issue**: "Resource not accessible by integration"
- **Solution**: Verify token has `read:audit_log` or `admin:org` scope

**Issue**: Empty results despite recent events
- **Solution**: Check time zone formatting; use UTC timestamps

**Issue**: Workflow not triggering
- **Solution**: Verify workflow file name matches dispatch reference

**Issue**: Rate limiting errors
- **Solution**: Implement pagination with delays between requests

---

## References

- [GitHub Enterprise Audit Log API Documentation](https://docs.github.com/en/rest/orgs/orgs#get-the-audit-log-for-an-organization)
- [GitHub Actions Workflow Syntax](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions)
- [GitHub Script Action](https://github.com/actions/github-script)

---

*"Perfection is achieved, not when there is nothing more to add, but when there is nothing left to take away." - Antoine de Saint-Exupéry*

**This guide represents the minimal sufficient complexity** for enterprise-scale audit log orchestration. Start simple, iterate based on real needs.
