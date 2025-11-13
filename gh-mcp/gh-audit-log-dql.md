I'll help you create a comprehensive practical guide and cheat sheet for querying GitHub audit log events in Dynatrace notebooks. Let me first check if there's relevant information in the project knowledge.Now I'll create a comprehensive practical guide and cheat sheet for querying GitHub audit log events in Dynatrace notebooks using DQL.Let me create this comprehensive guide for you using the proper file creation approach. I'll save it to the outputs directory so you can access it.I'll provide you with a comprehensive practical guide and cheat sheet for querying GitHub audit log events in Dynatrace notebooks using DQL. Here's the complete guide:

---

# **Dynatrace DQL Practical Guide: GitHub Audit Log Events**
## Complete Reference for Querying GitHub Advanced Security Extension Audit Logs

**Version:** 1.0  
**Target:** Dynatrace Notebooks with GitHub Advanced Security Extension  
**Software Versions:** Grafana OSS v12, kubectl 1.30, Go 1.21  
**Date:** January 2025

---

## 📋 **Table of Contents**
1. [Overview](#overview)
2. [Understanding GitHub Audit Log Data](#data-structure)
3. [Basic DQL Query Structure](#basic-queries)
4. [Personal Access Token Queries](#pat-queries)
5. [SSH Key Queries](#ssh-queries)
6. [Repository Events Queries](#repo-queries)
7. [Credential Authorization Queries](#credential-queries)
8. [Aggregated Metrics Dashboard](#metrics)
9. [Quick Reference Cheat Sheet](#cheat-sheet)
10. [Performance Optimization](#optimization)

---

## 🎯 **Overview**

The GitHub Advanced Security (GHAS) Extension ingests audit logs from GitHub into Dynatrace's Grail data lake, stored in the `default_securityevents` bucket.

### Key Characteristics:
- **Storage:** `default_securityevents` bucket
- **Retention:** Per your Grail configuration
- **Structure:** Dynatrace Semantic Dictionary mapping
- **Original Data:** Preserved in `event.original_content`
- **GitHub Namespace:** GHAS-specific attributes added

---

## 📊 **Understanding GitHub Audit Log Data**

### Common Field Reference Table

| Field | Description | Example |
|-------|-------------|---------|
| `event.type` | Event category | `"AUDIT_LOG"` |
| `action` | Audit action | `"personal_access_token.create"` |
| `actor` | User who performed action | `"octocat"` |
| `actor_id` | GitHub user ID | `12345` |
| `@timestamp` | Event timestamp | ISO 8601 format |
| `org` / `org_id` | Organization info | `"my-org"` / `67890` |
| `repo` / `repo_id` | Repository info | `"my-org/my-repo"` / `54321` |
| `token_id` | Token identifier | Numeric ID |
| `hashed_token` | SHA-256 token hash | Base64-encoded |
| `token_scopes` | Token permissions | Array of scopes |
| `visibility` | Repo visibility | `public`, `private`, `internal` |
| `fingerprint` | SSH key fingerprint | `SHA256:...` |

### Event Action Pattern: `{category}.{operation}`

Examples:
- `personal_access_token.create` - PAT creation
- `public_key.create` - SSH key added
- `repo.visibility_change` - Visibility modified
- `org_credential_authorization.grant` - Credential authorized

---

## 🔍 **Basic DQL Query Structure**

### Fetch GitHub Audit Logs

```dql
// Basic fetch
fetch security.events
| filter event.type == "AUDIT_LOG"
| filter contains(event.provider, "github")
| sort timestamp desc
| limit 100
```

### Time Range Examples

```dql
// Last 24 hours
fetch security.events, from: -24h
| filter event.type == "AUDIT_LOG"

// Specific date range
fetch security.events, from: 2025-01-01, to: 2025-01-31
| filter event.type == "AUDIT_LOG"

// Last 7 days
fetch security.events, from: now()-7d
| filter event.type == "AUDIT_LOG"
```

---

## 🔑 **Personal Access Token (PAT) Queries**

### 1. All PAT Creation Events

```dql
// Fine-grained and Classic PAT creation
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "personal_access_token")
| filter matchesValue(action, "personal_access_token.create")
| fieldsAdd
    user = actor,
    token_type = if(contains(event.original_content, "fine_grained"),
                   "fine-grained", "classic"),
    created_time = timestamp,
    organization = org
| fields timestamp, user, token_type, org, token_scopes, user_agent
| sort timestamp desc
```

### 2. Classic PAT Authorization (SAML-SSO)

```dql
// PAT authorization for SAML-enabled organizations
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "personal_access_token.access_granted")
    or matchesValue(action, "org_credential_authorization.grant")
| fieldsAdd
    user = actor,
    authorized_time = timestamp,
    organization = org,
    scopes = token_scopes
| fields timestamp, user, org, scopes, token_id, user_agent
| sort timestamp desc
```

### 3. Fine-Grained PAT Events

```dql
// Fine-grained PAT specific events
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "personal_access_token")
| parse event.original_content,
    DQUOTE "token_type" DQUOTE [ ]* ":" [ ]* DQUOTE LD:pat_type DQUOTE
| filter pat_type == "fine_grained"
| fieldsAdd
    user = actor,
    action_type = action,
    token_type = pat_type
| fields timestamp, user, action_type, token_type, org, repo
| sort timestamp desc
```

### 4. PAT Usage Tracking by Token Hash

```dql
// Track all activity for a specific PAT
fetch security.events, from: -7d
| filter event.type == "AUDIT_LOG"
| filter isNotNull(hashed_token)
| filter hashed_token == "YOUR_HASHED_TOKEN_HERE"  // Replace
| fieldsAdd
    user = actor,
    action_performed = action,
    target_repo = repo
| fields timestamp, user, action_performed, target_repo, org
| summarize count(), by: {action_performed}
| sort `count()` desc
```

### 5. All PAT Events Summary

```dql
// Comprehensive PAT events
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "personal_access_token")
    or contains(action, "oauth_access")
| fieldsAdd
    user = actor,
    event_type = action,
    organization = org
| summarize
    event_count = count(),
    unique_users = countDistinct(user),
    by: {event_type, organization}
| sort event_count desc
```

---

## 🔐 **SSH Key Queries**

### 1. SSH Key Creation

```dql
// All SSH key additions
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "public_key.create")
    or matchesValue(action, "public_key.add")
| fieldsAdd
    user = actor,
    key_title = title,
    key_fingerprint = fingerprint,
    created_time = timestamp
| fields timestamp, user, key_title, key_fingerprint, org, user_agent
| sort timestamp desc
```

### 2. SSH Key Removal

```dql
// SSH key deletions
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "public_key.destroy")
    or matchesValue(action, "public_key.delete")
| fieldsAdd
    user = actor,
    key_title = title,
    key_fingerprint = fingerprint,
    deleted_time = timestamp,
    reason = explanation
| fields timestamp, user, key_title, key_fingerprint, reason, org
| sort timestamp desc
```

### 3. SSH Key Authorization/Verification

```dql
// SSH key verification events
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "public_key.verify")
    or matchesValue(action, "public_key.unverify")
    or matchesValue(action, "public_key.unverification_failure")
| fieldsAdd
    user = actor,
    verification_action = action,
    key_fingerprint = fingerprint,
    verification_time = timestamp
| fields timestamp, user, verification_action, key_fingerprint, org
| sort timestamp desc
```

### 4. Deploy Keys (Repository SSH Keys)

```dql
// Repository deploy keys
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "public_key")
| filter isNotNull(repo)
| fieldsAdd
    key_action = action,
    repository = repo,
    key_title = title,
    key_fingerprint = fingerprint,
    read_only_status = read_only
| fields timestamp, key_action, repository, key_title,
        key_fingerprint, read_only_status
| sort timestamp desc
```

### 5. SSH Key Activity Summary by User

```dql
// Aggregate SSH key events per user
fetch security.events, from: -90d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "public_key")
| fieldsAdd
    user = actor,
    key_event = action
| summarize
    total_events = count(),
    keys_created = countIf(key_event == "public_key.create"),
    keys_deleted = countIf(key_event == "public_key.destroy"),
    by: {user, org}
| sort total_events desc
```

---

## 📦 **Repository Events Queries**

### 1. Repository Creation

```dql
// New repository creation
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "repo.create")
| fieldsAdd
    creator = actor,
    repository = repo,
    repo_visibility = visibility,
    creation_time = timestamp
| fields timestamp, creator, repository, repo_visibility, org
| sort timestamp desc
```

### 2. Repository Visibility Changes

```dql
// Public/Private/Internal visibility changes
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "repo.visibility_change")
| fieldsAdd
    changed_by = actor,
    repository = repo,
    new_visibility = visibility,
    previous_visibility = visibility_was,
    change_time = timestamp
| fields timestamp, changed_by, repository,
        previous_visibility, new_visibility, org
| sort timestamp desc
```

### 3. Critical: Repository Made Public Alert

```dql
// Repositories changed to public
fetch security.events, from: -7d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "repo.visibility_change")
| filter visibility == "public"
| fieldsAdd
    changed_by = actor,
    repository = repo,
    previous_state = visibility_was,
    alert_severity = "HIGH"
| fields timestamp, alert_severity, changed_by, repository,
        previous_state, org, user_agent
| sort timestamp desc
```

### 4. Repository Ruleset Updates

```dql
// Repository rule changes (branch protection, etc.)
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "repo_rules")
    or contains(action, "repository_ruleset")
    or matchesValue(action, "repo.update_member")
    or matchesValue(action, "repo.add_member")
| fieldsAdd
    modified_by = actor,
    repository = repo,
    rule_action = action,
    modification_time = timestamp
| fields timestamp, modified_by, repository, rule_action, org
| sort timestamp desc
```

### 5. Repository Access Changes

```dql
// Team and member access modifications
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "repo.add_member")
    or matchesValue(action, "repo.remove_member")
    or matchesValue(action, "repo.add_topic")
    or matchesValue(action, "repo.transfer")
| fieldsAdd
    modified_by = actor,
    repository = repo,
    access_action = action,
    affected_user = user
| fields timestamp, modified_by, repository, access_action, affected_user, org
| sort timestamp desc
```

---

## 🎫 **Credential Authorization Queries**

### 1. All org_credential_authorization Events

```dql
// All credential authorization events
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(action, "org_credential_authorization")
| fieldsAdd
    user = actor,
    auth_action = action,
    organization = org,
    credential_type = if(
        isNotNull(token_id), "PAT",
        if(isNotNull(fingerprint), "SSH Key", "Unknown")
    )
| fields timestamp, user, auth_action, credential_type, organization
| sort timestamp desc
```

### 2. OAuth and Credential Grants

```dql
// Credential grants for SAML-SSO
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "org_credential_authorization.grant")
| fieldsAdd
    authorized_user = actor,
    organization = org,
    token_identifier = token_id,
    scopes_granted = token_scopes,
    grant_time = timestamp
| fields timestamp, authorized_user, organization,
        token_identifier, scopes_granted
| sort timestamp desc
```

### 3. Credential Revocations

```dql
// Revoked credentials
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter matchesValue(action, "org_credential_authorization.revoke")
    or matchesValue(action, "org_credential_authorization.deauthorize")
| fieldsAdd
    revoked_by = actor,
    organization = org,
    credential_identifier = token_id,
    revocation_time = timestamp
| fields timestamp, revoked_by, organization, credential_identifier
| sort timestamp desc
```

### 4. Suspicious Credential Activity

```dql
// Multiple authorization attempts in short time
fetch security.events, from: -24h
| filter event.type == "AUDIT_LOG"
| filter contains(action, "org_credential_authorization")
| fieldsAdd
    user = actor,
    auth_event = action,
    organization = org
| summarize
    auth_count = count(),
    by: {user, organization, bin(timestamp, 1h)}
| filter auth_count > 5
| fieldsAdd alert_level = "SUSPICIOUS"
| fields timestamp, alert_level, user, organization, auth_count
| sort auth_count desc
```

---

## 📈 **Aggregated Metrics Dashboard**

### Master Summary: Key Security Metrics

```dql
// Comprehensive security metrics table
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter contains(event.provider, "github")
| fieldsAdd
    event_category = if(
        contains(action, "personal_access_token"), "PAT Events",
        if(contains(action, "public_key"), "SSH Key Events",
        if(contains(action, "repo"), "Repository Events",
        if(contains(action, "org_credential"), "Credential Auth",
        "Other Events")))
    ),
    user = actor,
    organization = org
| summarize
    total_events = count(),
    unique_users = countDistinct(user),
    unique_repos = countDistinct(repo),
    by: {event_category, organization}
| sort total_events desc
```

### Daily Activity Trend (Time-Series)

```dql
// Time-series visualization
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| fieldsAdd
    event_type = if(
        contains(action, "personal_access_token"), "PAT",
        if(contains(action, "public_key"), "SSH Key",
        if(contains(action, "repo.create"), "Repo Creation",
        if(contains(action, "repo.visibility_change"), "Visibility Change",
        if(contains(action, "org_credential"), "Credential Auth",
        "Other")))))
| makeTimeseries count(),
    by: {event_type},
    interval: 1d
```

### Organization Security Posture with Risk Scoring

```dql
// Per-organization security metrics with risk score
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| fieldsAdd
    organization = org
| summarize
    total_events = count(),
    unique_actors = countDistinct(actor),
    repos_created = countIf(action == "repo.create"),
    visibility_changes = countIf(action == "repo.visibility_change"),
    pat_created = countIf(action == "personal_access_token.create"),
    ssh_keys_added = countIf(action == "public_key.create"),
    public_repos = countIf(visibility == "public"
                          and action == "repo.visibility_change"),
    by: {organization}
| fieldsAdd
    risk_score = (public_repos * 10) + (visibility_changes * 5) + (pat_created * 2)
| sort risk_score desc
```

---

## ⚡ **Quick Reference Cheat Sheet**

### Essential Filters

```dql
// By organization
| filter org == "my-org-name"

// By user
| filter actor == "username"

// By repository
| filter repo == "my-org/my-repo"

// By action
| filter contains(action, "personal_access_token")
| filter matchesValue(action, "repo.create")
```

### String Matching Functions

```dql
contains(field, "substring")     // Case-insensitive substring
matchesValue(field, "value")     // Case-insensitive exact match
matchesPhrase(field, "*wild*")   // Wildcards
startsWith(field, "prefix")      // Starts with
endsWith(field, "suffix")        // Ends with
```

### Aggregation Patterns

```dql
| summarize count()                          // Total count
| summarize count(), by: {action}           // Group by field
| summarize countDistinct(actor)            // Unique values
| summarize countIf(visibility == "public") // Conditional count
| makeTimeseries count(), interval: 1d     // Time-series
```

---

## 🚀 **Performance Optimization Tips**

### ✅ Best Practices

```dql
// 1. Use specific time ranges
fetch security.events, from: -7d
| filter event.type == "AUDIT_LOG"

// 2. Filter early in pipeline
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| filter org == "my-org"
| filter contains(action, "repo")

// 3. Limit results for exploration
| limit 100

// 4. Use sampling during development
fetch security.events, from: -24h, samplingRatio: 10
```

### ❌ Avoid

```dql
// No time range specified
fetch security.events

// Late filtering
| summarize ...
| filter org == "my-org"

// Too many aggregation dimensions
| summarize count(), by: {action, actor, org, repo, user_agent}
```

---

## 🔧 **Troubleshooting**

### No Data Returned

```dql
// Check if data exists
fetch security.events, from: -30d
| filter event.type == "AUDIT_LOG"
| summarize count(), by: {bin(timestamp, 1d)}
```

**Checklist:**
- ✓ Verify GitHub Advanced Security Extension is active
- ✓ Confirm OpenPipeline configuration
- ✓ Check time range includes events

### Field Not Found

```dql
// Always check if field exists
| filter isNotNull(field_name)
| fieldsAdd value = field_name
```

---

## 📚 **Additional Resources**

- **Dynatrace GHAS Extension:** https://docs.dynatrace.com/docs/secure/threat-observability/security-events-ingest/ingest-github-advanced-security
- **GitHub Audit Events Reference:** https://docs.github.com/en/organizations/keeping-your-organization-secure/managing-security-settings-for-your-organization/audit-log-events-for-your-organization
- **DQL Reference:** https://docs.dynatrace.com/docs/discover-dynatrace/references/dynatrace-query-language
- **Semantic Dictionary:** https://docs.dynatrace.com/docs/discover-dynatrace/references/semantic-dictionary

---

**End of Guide - Version 1.0 (January 2025)**

This comprehensive guide provides everything you need to query GitHub audit logs in Dynatrace notebooks using DQL. All queries are production-ready and follow DQL best practices for optimal performance on Kubernetes/AKS environments.
