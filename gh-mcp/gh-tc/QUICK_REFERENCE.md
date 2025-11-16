# GitHub Audit Monitoring - Quick Reference Card

## 🚨 Emergency Response

### When Alert Fires
1. **Acknowledge** in PagerDuty/Slack immediately
2. **Open** Dynatrace problem link from alert
3. **Execute** investigation DQL query:
   ```dql
   fetch logs, from: -1h
   | filter log.source == "github_audit_log"
   | parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
   | parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
   | parse content, "\"org\"" SPACE? ":" SPACE? "\"" LD:organization "\""
   | filter event_action == "{EVENT_FROM_ALERT}"
   ```
4. **Check** GitHub audit log for full details:
   `https://github.com/organizations/{ORG}/settings/audit-log`
5. **Determine** if event is authorized
6. **Document** findings in problem resolution notes
7. **Close** problem with required fields

---

## 📊 Common DQL Queries

### Check Recent Events (Last Hour)
```dql
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action in ("org_credential_authorization.grant",
    "personal_access_token.request_created",
    "personal_access_token.access_granted")
| summarize count(), by: {event_action}
```

### Top Actors Today
```dql
fetch logs, from: -24h
| filter log.source == "github_audit_log"
| parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action in ("org_credential_authorization.grant",
    "personal_access_token.request_created",
    "personal_access_token.access_granted")
| summarize total = count(), by: {actor}
| sort total desc
| limit 10
```

### Events by Organization
```dql
fetch logs, from: -24h
| filter log.source == "github_audit_log"
| parse content, "\"org\"" SPACE? ":" SPACE? "\"" LD:organization "\""
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter event_action in ("org_credential_authorization.grant",
    "personal_access_token.request_created",
    "personal_access_token.access_granted")
| summarize total = count(), by: {organization}
| sort total desc
```

### Specific Actor Investigation
```dql
fetch logs, from: -7d
| filter log.source == "github_audit_log"
| parse content, "\"actor\"" SPACE? ":" SPACE? "\"" LD:actor "\""
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| parse content, "\"actor_ip\"" SPACE? ":" SPACE? "\"" LD:source_ip "\""
| filter actor == "SUSPICIOUS_USER"
| fields timestamp, event_action, source_ip, content
| sort timestamp desc
```

---

## 📈 Metric Queries

### Check Credential Grant Count
```dql
fetch metrics
| filter metricKey == "github.audit.credential_grant.count"
| summarize value = sum(value), by: {organization, actor}
```

### Check Token Activity
```dql
fetch metrics
| filter metricKey in ("github.audit.token_request.count",
    "github.audit.token_grant.count")
| summarize value = sum(value), by: {metricKey, organization}
```

### Total Security Events
```dql
fetch metrics
| filter metricKey == "github.audit.security_events.total"
| summarize total = sum(value)
```

---

## 🔍 Validation Commands

### Test Log Ingestion
```bash
export DYNATRACE_URL="https://your-env.live.dynatrace.com"
export DYNATRACE_API_TOKEN="your-token"
go run validate.go
```

### Generate Dashboard
```bash
go run dashboard.go
# Open github-audit-dashboard.html
```

### Check Processing Rule Status
Navigate to: `Settings → Log Monitoring → Log processing rules`
- Verify "GitHub Audit Security Events" is enabled
- Check "Processing statistics" for throughput

---

## 🔧 Troubleshooting

### No Logs Appearing
```dql
# Test 1: Check any logs
fetch logs, from: -1h | limit 10

# Test 2: Check GitHub logs specifically
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| limit 10
```
**If empty**: Check GitHub webhook configuration

### Events Not Parsed
```dql
# Test parsing
fetch logs, from: -1h
| filter log.source == "github_audit_log"
| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
| filter isNotNull(event_action)
| limit 10
```
**If event_action is null**: Check log format

### Metrics Missing
```dql
# Check metric exists
fetch metrics
| filter metricKey == "github.audit.credential_grant.count"
| limit 10
```
**If empty**: Verify OpenPipeline processor is active

### Alerts Not Firing
1. Check alert rule is enabled: `Settings → Anomaly detection → Metric events`
2. Verify threshold: Should be `>= 1`
3. Check evaluation window: Should be `1-5 minutes`
4. Verify metric selector syntax

---

## 🎯 Problem Closure Workflow

### Required Information
1. **Investigation Status**: Text description of investigation
2. **Resolution Notes**: Required - What was found and what action was taken
3. **Event Authorized**: Boolean - Was this event legitimate?
4. **Assigned Investigator**: Who performed the investigation

### Example Resolution Note
```
Event Type: org_credential_authorization.grant
Actor: john.doe
Organization: acme-corp
Timestamp: 2024-01-15 14:23:45 UTC

Investigation:
- Verified with actor via Slack
- Credential grant was for new CI/CD pipeline integration
- Approved by Security team on 2024-01-15
- Ticket reference: SEC-12345

Conclusion: AUTHORIZED
Action Taken: No action required - legitimate use case
Investigator: jane.smith
Date Closed: 2024-01-15 14:45:00 UTC
```

---

## 📞 Contact Information

### Alerts & Problems
- **PagerDuty**: Check on-call rotation
- **Slack**: #audit-response-team
- **Email**: audit-response-oncall@company.com

### Configuration Issues
- **Platform Team**: platform-engineering@company.com
- **Dynatrace Support**: support.dynatrace.com

### Security Concerns
- **Security Team**: security-team@company.com
- **CISO Office**: ciso@company.com

---

## 🔐 Access & Credentials

### Dynatrace
- **URL**: (Set in DYNATRACE_URL)
- **API Token**: Stored in secrets manager
- **Permissions**: logs.ingest, settings.write, metrics.write, problems.write

### GitHub
- **Audit Log**: `https://github.com/organizations/{ORG}/settings/audit-log`
- **Webhook Config**: `https://github.com/organizations/{ORG}/settings/hooks`

### PagerDuty
- **Service**: audit-response-team
- **Escalation**: audit-response-oncall
- **Integration Key**: Stored in secrets manager

---

## 📋 Monthly Checklist

### Week 1
- [ ] Review all closed problems from previous month
- [ ] Generate compliance report
- [ ] Check for false positives

### Week 2
- [ ] Review alert thresholds
- [ ] Update team contact information
- [ ] Test backup notification channels

### Week 3
- [ ] Review top actors/organizations
- [ ] Check for anomalies or trends
- [ ] Update documentation if needed

### Week 4
- [ ] Quarterly security review (if applicable)
- [ ] Team training refresh
- [ ] Test disaster recovery procedures

---

## ⚡ Quick Actions

### Temporarily Disable Alerts (Emergency Only)
```
Settings → Anomaly detection → Metric events
→ Find "GitHub Credential Authorization Grant"
→ Toggle "Enabled" to OFF
```
**Important**: Document why and re-enable ASAP

### Add New Organization to Monitoring
Already automatic - all organizations monitored by default

### Adjust Alert Threshold
```
Settings → Anomaly detection → Metric events
→ Select alert rule
→ Modify "Threshold" value
→ Save
```

### Export Problem History
```dql
fetch problems
| filter hasTag("team:audit-response")
| filter startTime >= toTimestamp("2024-01-01")
| fields problemId, title, severity, status, startTime, endTime
| sort startTime desc
```

---

## 📖 Documentation Links

- **Main README**: `README.md`
- **Deployment Guide**: `DEPLOYMENT_GUIDE.md`
- **Executive Summary**: `EXECUTIVE_SUMMARY.md`
- **DQL Queries**: `github-audit-monitoring.dql`
- **Alert Config**: `alerting-config.json`

---

## 🎓 Training Resources

- **Dynatrace DQL**: https://docs.dynatrace.com/docs/platform/grail/dynatrace-query-language
- **GitHub Audit Events**: https://docs.github.com/en/organizations/keeping-your-organization-secure/reviewing-the-audit-log-for-your-organization
- **OpenPipeline**: https://docs.dynatrace.com/docs/platform-modules/automations/workflows/data-enrichment/openpipeline

---

**Print this card and keep it accessible for on-call team**  
**Version 1.0 | Last Updated: November 2024**
