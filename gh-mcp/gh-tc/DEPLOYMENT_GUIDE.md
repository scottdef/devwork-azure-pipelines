# GitHub Audit Log Monitoring with Dynatrace
## Complete Deployment Guide

### Overview
This solution monitors critical GitHub audit log events and provides real-time alerting through Dynatrace. When specific security events are detected, metrics are extracted via OpenPipeline processors, alerts are triggered, and problems are created that require manual investigation and closure.

### Monitored Events
1. **org_credential_authorization.grant** - Critical (P1)
2. **personal_access_token.request_created** - Warning (P2)
3. **personal_access_token.access_granted** - Critical (P1)

### Architecture

```
GitHub Audit Logs
       ↓
Dynatrace Log Ingestion
       ↓
OpenPipeline Processor
  - Parse events
  - Extract metrics
       ↓
Metric Storage
  - github.audit.credential_grant.count
  - github.audit.token_request.count
  - github.audit.token_grant.count
       ↓
Alert Evaluation
       ↓
Problem Creation (Manual Closure Required)
       ↓
Notifications
  - PagerDuty (on-call)
  - Slack (#audit-response-team)
  - Email (audit-response-oncall@company.com)
  - SIEM Webhook
```

---

## Prerequisites

### 1. Dynatrace Environment
- Dynatrace SaaS or Managed environment
- API access with following permissions:
  - `logs.ingest` - Log ingestion
  - `settings.write` - Configuration management
  - `metrics.write` - Metric creation
  - `problems.write` - Problem creation

### 2. GitHub Audit Log Integration
- GitHub Enterprise or GitHub.com organization
- Audit log streaming enabled
- Webhook or log forwarder configured to send logs to Dynatrace

### 3. Notification Channels
- **PagerDuty**: Service key for audit-response-team
- **Slack**: Webhook URL for audit channel
- **Email**: SMTP configuration
- **SIEM**: Webhook endpoint (optional)

### 4. Software Versions
- Go 1.21 (for validation tools)
- kubectl 1.30 (for AKS deployment)
- Dynatrace OneAgent or ActiveGate

---

## Deployment Steps

### Step 1: Configure GitHub Audit Log Streaming

#### Option A: Using GitHub Webhook
```bash
# Configure GitHub to send audit logs to Dynatrace
# Settings → Audit log → Stream audit logs

Endpoint: https://{your-environment}.live.dynatrace.com/api/v2/logs/ingest
Headers:
  Authorization: Api-Token {your-api-token}
  Content-Type: application/json
Format: JSON
```

#### Option B: Using Log Forwarder (Fluentd/Logstash)
```yaml
# fluentd.conf example
<source>
  @type http
  port 9880
  bind 0.0.0.0
  body_size_limit 32m
  keepalive_timeout 10s
</source>

<match github.audit>
  @type http
  endpoint https://{your-environment}.live.dynatrace.com/api/v2/logs/ingest
  headers {"Authorization":"Api-Token #{ENV['DYNATRACE_API_TOKEN']}"}
  json_array true
  <buffer>
    flush_interval 1s
  </buffer>
</match>
```

### Step 2: Deploy OpenPipeline Processor

1. **Navigate to Dynatrace UI**
   ```
   Settings → Log Monitoring → Log processing rules
   ```

2. **Create New Processing Rule**
   - Name: `GitHub Audit Security Events`
   - Matcher: `log.source == "github_audit_log"`
   - Priority: `High`

3. **Add DQL Processor**
   ```dql
   fetch logs
   | filter log.source == "github_audit_log"
   | parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
   | filter event_action == "org_credential_authorization.grant"
       or event_action == "personal_access_token.request_created"
       or event_action == "personal_access_token.access_granted"
   ```

4. **Configure Metric Extraction**
   - Upload `openpipeline-config.json`
   - Or manually create extraction rules:
     ```json
     {
       "metricKey": "github.audit.credential_grant.count",
       "condition": "event_action == 'org_credential_authorization.grant'",
       "value": 1,
       "dimensions": ["organization", "actor"]
     }
     ```

5. **Save and Activate**

### Step 3: Configure Alerting Rules

1. **Navigate to Alerting**
   ```
   Settings → Anomaly detection → Metric events
   ```

2. **Create Alert for Credential Grants**
   ```
   Name: GitHub Credential Authorization Grant
   Metric: github.audit.credential_grant.count
   Threshold: >= 1
   Evaluation window: 1 minute
   Alert severity: CRITICAL
   ```

3. **Create Alert for Token Grants**
   ```
   Name: GitHub Personal Access Token Granted
   Metric: github.audit.token_grant.count
   Threshold: >= 1
   Evaluation window: 1 minute
   Alert severity: CRITICAL
   ```

4. **Create Alert for Token Requests**
   ```
   Name: GitHub Personal Access Token Request
   Metric: github.audit.token_request.count
   Threshold: >= 1
   Evaluation window: 5 minutes
   Alert severity: WARNING
   ```

### Step 4: Configure Problem Creation

1. **Navigate to Problem Settings**
   ```
   Settings → Anomaly detection → Problem alerting profiles
   ```

2. **Create Profile: GitHub Audit Security**
   ```json
   {
     "name": "github-audit-security",
     "severity": "AVAILABILITY",
     "autoClose": false,
     "requireManualClosure": true,
     "tags": ["github:security", "team:audit-response"]
   }
   ```

3. **Configure Problem Fields**
   - Investigation Status (text)
   - Resolution Notes (required, textarea)
   - Event Authorized (required, boolean)
   - Assigned Investigator (text)

### Step 5: Configure Notifications

#### PagerDuty Integration
```bash
# Navigate to Settings → Integration → PagerDuty
Service Key: <your-pagerduty-service-key>
Escalation Policy: audit-response-oncall
Alert Filter: tag:team:audit-response
```

#### Slack Integration
```bash
# Navigate to Settings → Integration → Slack
Webhook URL: <your-slack-webhook-url>
Channel: #audit-response-team
Alert Filter: tag:team:audit-response

# Message template provided in alerting-config.json
```

#### Email Notifications
```bash
# Navigate to Settings → Integration → Email
Recipients:
  - audit-response-oncall@company.com
  - security-team@company.com
Subject: [URGENT] GitHub Security Event: {event_type}
Alert Filter: severity:critical AND team:audit-response
```

#### SIEM Integration (Optional)
```bash
# Navigate to Settings → Integration → Custom webhook
URL: <your-siem-webhook-url>
Method: POST
Headers:
  Content-Type: application/json
  Authorization: Bearer <siem-api-token>
```

### Step 6: Testing and Validation

#### Test 1: Verify Log Ingestion
```dql
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| limit 10
```

Expected: GitHub audit logs should appear

#### Test 2: Verify Event Parsing
```dql
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter isNotNull(event_action)
| summarize count(), by: {event_action}
```

Expected: Event actions should be parsed correctly

#### Test 3: Verify Metric Extraction
```dql
fetch metrics
| filter metricKey == "github.audit.credential_grant.count"
| limit 10
```

Expected: Metric data points should exist (if events occurred)

#### Test 4: Simulate Event (if possible)
```bash
# In GitHub, perform a test action that generates audit log
# For example: Request a personal access token

# Then verify in Dynatrace:
fetch logs, from: now()-5m
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action == "personal_access_token.request_created"
```

#### Test 5: Verify Alert Firing
```bash
# Navigate to Problems in Dynatrace
# Filter: tag:team:audit-response
# Verify problem was created for test event
```

#### Test 6: Verify Notifications
```bash
# Check PagerDuty incident created
# Check Slack message in #audit-response-team
# Check email received
# Check SIEM webhook received (if configured)
```

---

## Monitoring and Maintenance

### Daily Checks
1. Review active problems: `tag:team:audit-response AND problemStatus:OPEN`
2. Check metric ingestion rate: `github.audit.security_events.total`
3. Verify notification delivery

### Weekly Reviews
1. Analyze event trends by organization
2. Review top actors performing sensitive actions
3. Update alert thresholds if needed
4. Review false positive rate

### Monthly Tasks
1. Audit resolution compliance
2. Review and update DQL queries
3. Update documentation
4. Test disaster recovery procedures

### Quarterly Activities
1. Security review of all closed problems
2. Update notification contact lists
3. Review and optimize metric retention
4. Conduct tabletop exercise for incident response

---

## Troubleshooting

### Issue: Logs not appearing
**Symptoms**: No logs with `log.source == "github_audit_log"`

**Resolution**:
1. Verify GitHub webhook is configured correctly
2. Check API token permissions
3. Verify log ingest endpoint URL
4. Check Dynatrace ingestion statistics
5. Review GitHub webhook delivery logs

### Issue: Events not being parsed
**Symptoms**: `event_action` field is null

**Resolution**:
1. Check JSON format of incoming logs
2. Verify DQL parse statement matches log format
3. Test parse statement in Notebooks
4. Check for special characters or encoding issues

### Issue: Metrics not appearing
**Symptoms**: No data for `github.audit.*.count` metrics

**Resolution**:
1. Verify processing rule is active
2. Check metric extraction configuration
3. Review processing rule statistics
4. Verify events are being parsed correctly
5. Check metric retention settings

### Issue: Alerts not firing
**Symptoms**: Events occur but no alerts generated

**Resolution**:
1. Verify alert rules are enabled
2. Check metric selector syntax
3. Review threshold configuration
4. Check alert evaluation window
5. Verify problem creation is enabled

### Issue: Notifications not sent
**Symptoms**: Problems created but team not notified

**Resolution**:
1. Verify integration credentials
2. Check notification channel configuration
3. Review alert filters
4. Test notification channels manually
5. Check integration logs

### Issue: Problems auto-closing
**Symptoms**: Problems close automatically

**Resolution**:
1. Verify auto-close is disabled in problem settings
2. Check problem alerting profile configuration
3. Review problem lifecycle settings

---

## Performance Optimization

### Reduce Processing Load
```dql
# Use pre-filtering to reduce data processed
fetch logs
| filter log.source == "github_audit_log"
| filter matchesPhrase(content, "*org_credential_authorization*")
    or matchesPhrase(content, "*personal_access_token*")
```

### Optimize Metric Cardinality
```json
{
  "dimensions": {
    "organization": "{organization}",
    "event_type": "{event_action}"
    // Avoid high-cardinality dimensions like timestamps
  }
}
```

### Set Appropriate Aggregation Windows
- Real-time monitoring: 1 minute
- Standard monitoring: 5 minutes
- Trending analysis: 1 hour

---

## Security Considerations

### Access Control
- Restrict API token access to minimum required scopes
- Use separate tokens for different integrations
- Rotate tokens quarterly
- Store tokens in secrets management system

### Data Privacy
- Ensure audit logs don't contain PII
- Configure data masking if needed
- Set appropriate retention periods
- Comply with data residency requirements

### Audit Trail
- All problems require manual closure
- Resolution notes are mandatory
- Track investigator assignments
- Maintain 90-day problem retention

---

## Compliance and Reporting

### Audit Reports
```dql
# Monthly report: All credential grants
fetch logs, from: -30d
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action == "org_credential_authorization.grant"
| parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
| parse content, "\"org\"" SPACE? ":" SPACE? "\"" LD:organization "\""
| summarize count(), by: {organization, actor}
```

### Problem Resolution Metrics
```dql
# Average time to resolution
fetch problems
| filter hasTag("team:audit-response")
| fieldsAdd resolution_time = endTime - startTime
| summarize avg(resolution_time), by: {problemSeverity}
```

### SLA Compliance
- P1 alerts: 15-minute response time
- P2 alerts: 1-hour response time
- Problem closure: Within 24 hours
- Monthly compliance report required

---

## API Integration Examples

### Create Problem via API
```bash
curl -X POST "https://{your-environment}.live.dynatrace.com/api/v2/problems" \
  -H "Authorization: Api-Token ${DT_API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "GitHub Security: Manual Test",
    "severity": "AVAILABILITY",
    "entityId": "CUSTOM_DEVICE-1234567890",
    "tags": ["github:security", "team:audit-response"]
  }'
```

### Query Metrics via API
```bash
curl "https://{your-environment}.live.dynatrace.com/api/v2/metrics/query?metricSelector=github.audit.credential_grant.count" \
  -H "Authorization: Api-Token ${DT_API_TOKEN}"
```

---

## Additional Resources

- **Dynatrace DQL Documentation**: https://docs.dynatrace.com/docs/platform/grail/dynatrace-query-language
- **OpenPipeline Documentation**: https://docs.dynatrace.com/docs/platform-modules/automations/workflows/data-enrichment/openpipeline
- **GitHub Audit Log Events**: https://docs.github.com/en/organizations/keeping-your-organization-secure/reviewing-the-audit-log-for-your-organization
- **Dynatrace API Reference**: https://www.dynatrace.com/support/help/dynatrace-api

---

## Support Contacts

- **Audit Response Team**: audit-response-oncall@company.com
- **Dynatrace Support**: support.dynatrace.com
- **GitHub Support**: support.github.com
