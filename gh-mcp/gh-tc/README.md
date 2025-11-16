# GitHub Audit Log Monitoring with Dynatrace

## 🎯 Overview

This solution provides comprehensive monitoring and alerting for critical GitHub audit log security events using Dynatrace Query Language (DQL), OpenPipeline processors, and automated problem creation.

### Monitored Events

| Event | Severity | Priority | Alert Action |
|-------|----------|----------|--------------|
| `org_credential_authorization.grant` | **CRITICAL** | P1 | Immediate notification + Problem creation |
| `personal_access_token.access_granted` | **CRITICAL** | P1 | Immediate notification + Problem creation |
| `personal_access_token.request_created` | **WARNING** | P2 | Notification + Problem creation |

### Key Features

✅ **Real-time Event Detection** - DQL queries filter audit logs in real-time  
✅ **Metric Extraction** - OpenPipeline processors extract metrics for each event type  
✅ **Automated Alerting** - Alerts triggered immediately when events are detected  
✅ **Problem Management** - Problems created with manual closure requirement  
✅ **Multi-channel Notifications** - PagerDuty, Slack, Email, and SIEM integration  
✅ **Visual Dashboards** - Go-based dashboard generation using go-echarts  
✅ **Validation Tools** - Go tools for testing and validating configuration  

---

## 📁 Repository Structure

```
.
├── README.md                          # This file
├── DEPLOYMENT_GUIDE.md                # Complete deployment instructions
├── github-audit-monitoring.dql        # DQL queries for event detection
├── openpipeline-config.json           # OpenPipeline processor configuration
├── alerting-config.json               # Alert and notification configuration
├── validate.go                        # Validation tool (Go 1.21)
├── dashboard.go                       # Dashboard generator (Go 1.21, go-echarts)
├── go.mod                             # Go module dependencies
└── go.sum                             # Go dependency checksums
```

---

## 🚀 Quick Start

### Prerequisites

- **Dynatrace Environment** (SaaS or Managed)
- **GitHub Enterprise** or GitHub.com organization
- **Go 1.21** (for validation and dashboard tools)
- **kubectl 1.30** (if deploying monitoring agents to AKS)
- **Dynatrace API Token** with permissions:
  - `logs.ingest`
  - `settings.write`
  - `metrics.write`
  - `problems.write`

### Step 1: Configure Environment Variables

```bash
export DYNATRACE_URL="https://your-environment.live.dynatrace.com"
export DYNATRACE_API_TOKEN="dt0c01.YOUR_TOKEN_HERE"
export PAGERDUTY_SERVICE_KEY="your-pagerduty-service-key"
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
export TEST_MODE="false"  # Set to "true" for testing
```

### Step 2: Configure GitHub Audit Log Streaming

#### Option A: Direct Webhook
```bash
# GitHub Settings → Audit log → Stream audit logs
Endpoint: ${DYNATRACE_URL}/api/v2/logs/ingest
Headers:
  Authorization: Api-Token ${DYNATRACE_API_TOKEN}
  Content-Type: application/json
```

#### Option B: Using Fluentd/Logstash
See `DEPLOYMENT_GUIDE.md` for complete configuration examples.

### Step 3: Deploy OpenPipeline Processor

1. Navigate to: **Dynatrace UI → Settings → Log Monitoring → Log processing rules**
2. Create new rule: **"GitHub Audit Security Events"**
3. Set matcher: `log.source == "github_audit_log"`
4. Upload or copy configuration from `openpipeline-config.json`
5. Enable the processing rule

### Step 4: Configure Alerts

1. Navigate to: **Settings → Anomaly detection → Metric events**
2. Import alert rules from `alerting-config.json`
3. Configure notification channels:
   - PagerDuty integration
   - Slack webhook
   - Email settings
   - SIEM webhook (optional)

### Step 5: Validate Configuration

```bash
# Install Go dependencies
go mod download

# Run validation tool
go run validate.go

# Expected output:
# [PASS] 1. Test Log Ingestion - Test completed successfully (1.2s)
# [PASS] 2. Test DQL Query Execution - Test completed successfully (0.8s)
# [PASS] 3. Test Event Parsing - Test completed successfully (0.9s)
# [PASS] 4. Test Metric Extraction - Test completed successfully (1.1s)
# [PASS] 5. Test Alert Configuration - Test completed successfully (0.3s)
```

### Step 6: Generate Dashboard

```bash
# Generate visual dashboard
go run dashboard.go

# Output: github-audit-dashboard.html
# Open in browser to view metrics and trends
```

---

## 📊 DQL Queries

### Query 1: Main Event Detection
```dql
fetch logs
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action == "org_credential_authorization.grant"
    or event_action == "personal_access_token.request_created"
    or event_action == "personal_access_token.access_granted"
| parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
| parse content, "\"org\"" SPACE? ":" SPACE? "\"" LD:organization "\""
| fields timestamp, event_action, actor, organization
```

### Query 2: Event Summary (Last 1 Hour)
```dql
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action == "org_credential_authorization.grant"
    or event_action == "personal_access_token.request_created"
    or event_action == "personal_access_token.access_granted"
| summarize event_count = count(), by: {event_action}
```

### Query 3: Top Actors (Last 24 Hours)
```dql
fetch logs, from: -24h
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
| filter event_action in ("org_credential_authorization.grant", 
    "personal_access_token.request_created",
    "personal_access_token.access_granted")
| summarize total_events = count(), by: {actor}
| sort total_events desc
| limit 10
```

See `github-audit-monitoring.dql` for all 5 production queries.

---

## 🔔 Alerting Workflow

```
Event Detected
    ↓
Metric Extracted (via OpenPipeline)
    ↓
Alert Threshold Exceeded
    ↓
Problem Created (Manual Closure Required)
    ↓
Notifications Sent:
├── PagerDuty (on-call team)
├── Slack (#audit-response-team)
├── Email (audit-response-oncall@company.com)
└── SIEM (webhook)
    ↓
Investigation by audit-response-team
    ↓
Manual Problem Closure with Resolution Notes
```

---

## 📈 Metrics

### Extracted Metrics

| Metric Key | Type | Dimensions | Description |
|------------|------|------------|-------------|
| `github.audit.credential_grant.count` | Counter | organization, actor, severity | Credential authorization grants |
| `github.audit.token_request.count` | Counter | organization, actor, severity | Token creation requests |
| `github.audit.token_grant.count` | Counter | organization, actor, severity | Token access grants |
| `github.audit.security_events.total` | Counter | event_category | Total security events |

### Metric Retention
- **Default**: 35 days
- **Recommended**: 90 days for compliance
- **Maximum**: 365 days for long-term audit trail

---

## 🛠️ Validation Tools

### validate.go
Comprehensive validation tool that tests:
- Log ingestion capability
- DQL query execution
- Event parsing logic
- Metric extraction
- Alert configuration

**Usage:**
```bash
export DYNATRACE_URL="https://your-env.live.dynatrace.com"
export DYNATRACE_API_TOKEN="your-token"
go run validate.go
```

### dashboard.go
Visual dashboard generator using go-echarts that creates:
- Event count timeline (24-hour view)
- Event distribution pie chart
- Top actors bar chart
- Organization activity heatmap
- Current threat level gauge

**Usage:**
```bash
go run dashboard.go
# Open github-audit-dashboard.html in browser
```

---

## 🔍 Troubleshooting

### Logs Not Appearing
```bash
# Check if logs are being ingested
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| limit 10
```

**If empty:**
- Verify GitHub webhook configuration
- Check API token permissions
- Review Dynatrace ingestion statistics
- Check GitHub webhook delivery logs

### Events Not Parsed
```bash
# Test parsing logic
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter isNotNull(event_action)
| limit 10
```

**If event_action is null:**
- Verify JSON format of logs
- Check DQL parse statement
- Test with sample log in Notebooks

### Metrics Not Extracted
```bash
# Check metric existence
fetch metrics
| filter metricKey == "github.audit.credential_grant.count"
| limit 10
```

**If empty:**
- Verify OpenPipeline processor is enabled
- Check metric extraction configuration
- Review processing rule statistics
- Verify events are being parsed correctly

### Alerts Not Firing
- Check alert rules are enabled
- Verify metric selector syntax
- Review threshold values
- Check alert evaluation window
- Verify problem creation is enabled

---

## 📖 Documentation

- **Deployment Guide**: `DEPLOYMENT_GUIDE.md` - Complete step-by-step deployment instructions
- **DQL Queries**: `github-audit-monitoring.dql` - All DQL queries with comments
- **OpenPipeline Config**: `openpipeline-config.json` - Processor configuration
- **Alert Config**: `alerting-config.json` - Complete alerting setup

---

## 🔐 Security Considerations

### Access Control
- Use minimum required API token scopes
- Rotate tokens quarterly
- Store tokens in secrets management system (HashiCorp Vault, Azure Key Vault, etc.)

### Data Privacy
- Ensure logs don't contain PII
- Configure data masking if needed
- Set appropriate retention periods
- Comply with data residency requirements

### Audit Trail
- All problems require manual closure
- Resolution notes are mandatory
- Track investigator assignments
- Maintain 90-day problem retention

---

## 📝 Compliance and Reporting

### Monthly Audit Report
```dql
fetch logs, from: -30d
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action == "org_credential_authorization.grant"
| parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
| parse content, "\"org\"" SPACE? ":" SPACE? "\"" LD:organization "\""
| summarize count(), by: {organization, actor}
```

### SLA Metrics
- **P1 alerts**: 15-minute response time
- **P2 alerts**: 1-hour response time
- **Problem closure**: Within 24 hours
- **Monthly compliance report**: Required

---

## 🤝 Support

### Contact Information
- **Audit Response Team**: audit-response-oncall@company.com
- **Dynatrace Support**: https://support.dynatrace.com
- **GitHub Support**: https://support.github.com

### Additional Resources
- [Dynatrace DQL Documentation](https://docs.dynatrace.com/docs/platform/grail/dynatrace-query-language)
- [OpenPipeline Documentation](https://docs.dynatrace.com/docs/platform-modules/automations/workflows/data-enrichment/openpipeline)
- [GitHub Audit Log Events](https://docs.github.com/en/organizations/keeping-your-organization-secure/reviewing-the-audit-log-for-your-organization)

---

## 📦 Dependencies

### Go Modules
```go
module github-audit-monitoring

go 1.21

require (
    github.com/go-echarts/go-echarts/v2 v2.3.3
)
```

**Note**: Only `go-echarts` is used as an external package per project requirements.

---

## 📄 License

This solution is provided as-is for use with Dynatrace and GitHub Enterprise.

---

## ✅ Checklist

Before going to production:

- [ ] GitHub audit log streaming configured
- [ ] OpenPipeline processor deployed and tested
- [ ] Alert rules configured and tested
- [ ] PagerDuty integration configured
- [ ] Slack notifications configured
- [ ] Email notifications configured
- [ ] SIEM integration configured (optional)
- [ ] Problem closure workflow documented
- [ ] Team training completed
- [ ] Runbook created for on-call team
- [ ] Validation tools tested
- [ ] Dashboard generated and reviewed
- [ ] Compliance requirements verified
- [ ] SLA metrics defined
- [ ] Monthly reporting scheduled

---

**Version**: 1.0  
**Last Updated**: 2024-01-01  
**Maintained By**: Audit Response Team
