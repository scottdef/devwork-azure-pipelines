# EasyTrade Feature Flags and Problem Patterns Guide for Platform Engineers

## Overview

EasyTrade includes **four built-in problem patterns** controlled via the Feature Flag Service API. These patterns simulate real-world failure scenarios for testing observability platforms, incident response procedures, and chaos engineering experiments.

**Target Audience**: AKS Platform Engineers, SRE Teams, Observability Engineers

## Architecture

The Feature Flag Service exposes a REST API that controls behavior across multiple services:

```
┌─────────────────────────────────────────────────────┐
│         Feature Flag Service API                     │
│         /feature-flag-service/v1/flags/              │
└────────────┬────────────────────────────────────────┘
             │
             ├─────> Broker Service (high_cpu_usage)
             ├─────> Database (db_not_responding)
             ├─────> Factory (factory_crisis)
             └─────> Offer Service (ergo_aggregator_slowdown)
```

## Problem Patterns Reference

### 1. db_not_responding

**Effect**: Database throws errors when accessing the Trade table  
**Duration**: ~20 minutes (designed to trigger Dynatrace alerts)  
**Services Affected**: All services making database queries  
**Use Case**: Database failure simulation, database monitoring validation

**Symptoms**:
- HTTP 500 errors on trade-related endpoints
- Failed database transactions
- Application logs show SQL connection errors

**Enable**:
```bash
make problem-enable PATTERN=db_not_responding
```

**Verify**:
```bash
# Check broker-service logs for database errors
kubectl -n easytrade logs -l app=broker-service --tail=50

# Expected log entries:
# ERROR: Database connection failed
# SQLException: Cannot access Trade table
```

**Use Cases**:
- Test database monitoring and alerting
- Validate application error handling
- Practice database failover procedures
- Test circuit breaker patterns

---

### 2. ergo_aggregator_slowdown

**Effect**: Two aggregators (offer-service instances) receive slow responses  
**Duration**: 15-30 minutes  
**Services Affected**: Offer Service  
**Use Case**: Service degradation, partial failure scenarios

**Symptoms**:
- Increased response times on `/offer` endpoints
- Half of the offer-service pods show high latency
- Client-side timeouts may occur

**Enable**:
```bash
make problem-enable PATTERN=ergo_aggregator_slowdown
```

**Verify**:
```bash
# Monitor offer-service response times
kubectl -n easytrade exec -it deploy/broker-service -- \
    curl -w "@curl-format.txt" -o /dev/null -s \
    http://offerservice:8087/api/offers

# Check logs
kubectl -n easytrade logs -l app=offerservice --tail=50
```

**Use Cases**:
- Test partial service degradation detection
- Validate load balancing behavior
- Practice performance troubleshooting
- Test adaptive timeout strategies

---

### 3. factory_crisis

**Effect**: Factory service stops producing credit cards  
**Duration**: Persistent until manually disabled  
**Services Affected**: Factory, Credit Card Order Service  
**Use Case**: Supply chain failure, dependency breakdown

**Symptoms**:
- Credit card creation requests fail
- Factory service returns empty responses
- Credit card inventory depletes

**Enable**:
```bash
make problem-enable PATTERN=factory_crisis
```

**Verify**:
```bash
# Check factory logs
kubectl -n easytrade logs -l app=factory --tail=50

# Test factory endpoint
kubectl -n easytrade exec -it deploy/credit-card-order-service -- \
    curl http://factory:8081/api/factory/creditcard
```

**Disable** (Required - pattern persists):
```bash
make problem-disable PATTERN=factory_crisis
```

**Use Cases**:
- Test business process monitoring
- Validate inventory alerting
- Practice supply chain disruption handling
- Test graceful degradation strategies

---

### 4. high_cpu_usage

**Effect**: Broker service performs intensive Collatz conjecture calculations  
**Duration**: Persistent until manually disabled  
**Services Affected**: Broker Service  
**Use Case**: Resource exhaustion, CPU saturation

**Symptoms**:
- Broker service CPU usage spikes to 100%
- Increased pod temperature (if monitored)
- Slower response times across all broker endpoints
- Potential pod evictions if resource limits exceeded

**Enable**:
```bash
make problem-enable PATTERN=high_cpu_usage
```

**Verify**:
```bash
# Monitor CPU usage
kubectl -n easytrade top pod -l app=broker-service

# Check HPA behavior (if enabled)
kubectl -n easytrade get hpa

# Watch pod resource consumption
kubectl -n easytrade describe pod -l app=broker-service | grep -A5 "Limits"
```

**Disable** (Required - pattern persists):
```bash
make problem-disable PATTERN=high_cpu_usage
```

**Use Cases**:
- Test CPU monitoring and alerting
- Validate Horizontal Pod Autoscaler (HPA)
- Practice resource limit tuning
- Test throttling mechanisms

---

## Operational Procedures

### Accessing the Feature Flag Service

Three methods for accessing the API:

#### Method 1: kubectl port-forward (Development)

```bash
# Start port-forward
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80

# Use API via localhost
curl http://localhost:8080/feature-flag-service/v1/flags/
```

#### Method 2: Private LoadBalancer (Internal Network)

```bash
# Get LoadBalancer IP
LB_IP=$(kubectl -n easytrade get svc easytrade-private-lb \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Use API via internal IP
curl http://${LB_IP}/feature-flag-service/v1/flags/
```

#### Method 3: Istio Gateway (Service Mesh)

```bash
# Get Istio gateway IP
GATEWAY_IP=$(kubectl -n aks-istio-ingress get svc aks-istio-ingressgateway-internal \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Use API via gateway
curl http://${GATEWAY_IP}/feature-flag-service/v1/flags/
```

### Manual API Operations

```bash
# List all flags
curl http://localhost:8080/feature-flag-service/v1/flags/ | jq

# Get specific flag status
curl http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/ | jq

# Enable a flag
curl -X PUT http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/ \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'

# Disable a flag
curl -X PUT http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/ \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'
```

### Automated Management via Makefile

```bash
# List all available patterns
make problem-list

# Check status of all patterns
./scripts/feature-flags.sh status

# Enable a pattern
make problem-enable PATTERN=high_cpu_usage

# Disable a pattern
make problem-disable PATTERN=high_cpu_usage

# Disable all patterns
make problem-disable-all
```

---

## Testing Scenarios

### Scenario 1: Dynatrace Alert Validation

**Objective**: Verify Dynatrace detects and alerts on database failures

```bash
# 1. Ensure monitoring is active
kubectl -n easytrace get pods  # Check OneAgent pods

# 2. Enable database failure pattern
make problem-enable PATTERN=db_not_responding

# 3. Generate traffic
kubectl -n easytrade port-forward svc/headless-loadgen 8080:80 &

# 4. Wait for alert (typically 5-10 minutes)

# 5. Verify in Dynatrace UI:
#    - Problem detection
#    - Root cause analysis
#    - Service impact

# 6. Disable pattern
make problem-disable PATTERN=db_not_responding
```

### Scenario 2: Horizontal Pod Autoscaler Testing

**Objective**: Verify HPA scales pods under CPU load

```bash
# 1. Deploy HPA for broker-service
kubectl -n easytrade autoscale deployment broker-service \
    --cpu-percent=50 --min=2 --max=10

# 2. Enable CPU spike
make problem-enable PATTERN=high_cpu_usage

# 3. Monitor HPA scaling
watch -n 2 'kubectl -n easytrade get hpa'

# 4. Verify new pods are created
kubectl -n easytrade get pods -l app=broker-service

# 5. Disable pattern and watch scale-down
make problem-disable PATTERN=high_cpu_usage
```

### Scenario 3: Incident Response Drill

**Objective**: Practice incident response procedures

```bash
# 1. Enable multiple failures simultaneously
make problem-enable PATTERN=db_not_responding
make problem-enable PATTERN=high_cpu_usage

# 2. Incident team investigates:
#    - Check pod status
kubectl -n easytrade get pods

#    - Review logs
kubectl -n easytrade logs -l app=broker-service --tail=100

#    - Check metrics
kubectl -n easytrade top pods

# 3. Triage and resolution:
#    - Identify affected services
#    - Disable problematic patterns
make problem-disable-all

# 4. Verify recovery
./scripts/verify-deployment.sh
```

### Scenario 4: Performance Degradation Investigation

**Objective**: Debug partial service slowdown

```bash
# 1. Enable aggregator slowdown
make problem-enable PATTERN=ergo_aggregator_slowdown

# 2. Measure response times
for i in {1..20}; do
    kubectl -n easytrade exec -it deploy/broker-service -- \
        curl -w "Time: %{time_total}s\n" -o /dev/null -s \
        http://offerservice:8087/api/offers
done

# 3. Identify slow instances
kubectl -n easytrade top pods -l app=offerservice

# 4. Investigate logs
kubectl -n easytrade logs -l app=offerservice --tail=200 | grep -i slow

# 5. Disable pattern
make problem-disable PATTERN=ergo_aggregator_slowdown
```

---

## Integration with Monitoring Platforms

### Dynatrace Integration

Problem patterns are designed to trigger specific Dynatrace features:

- **db_not_responding**: Triggers Davis AI problem detection
- **ergo_aggregator_slowdown**: Shows in service flow analysis
- **factory_crisis**: Appears in business transaction monitoring
- **high_cpu_usage**: Triggers resource saturation alerts

### Prometheus/Grafana Integration

Query examples for monitoring problem patterns:

```promql
# High CPU usage detection
rate(container_cpu_usage_seconds_total{pod=~"broker-service.*"}[5m]) > 0.8

# Database error rate
rate(http_requests_total{job="broker-service",status="500"}[5m]) > 0.1

# Slow response times
histogram_quantile(0.95, 
    rate(http_request_duration_seconds_bucket{job="offerservice"}[5m])
) > 2.0
```

---

## Troubleshooting

### Problem Pattern Not Taking Effect

**Symptom**: Pattern enabled but no observable changes

**Solutions**:

1. Verify feature flag service is running:
```bash
kubectl -n easytrade get pods -l app=feature-flag-service
kubectl -n easytrade logs -l app=feature-flag-service
```

2. Check feature flag cache duration:
```bash
# Services cache flags for 30s by default
# Wait 30-60 seconds after enabling
```

3. Verify API response:
```bash
curl http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/ | jq
# Should show: "enabled": true
```

### Cannot Access Feature Flag API

**Symptom**: Connection refused or timeout errors

**Solutions**:

1. Verify port-forward is running:
```bash
ps aux | grep "port-forward"
```

2. Restart port-forward:
```bash
pkill -f "port-forward"
make port-forward
```

3. Check service is healthy:
```bash
kubectl -n easytrade get svc frontendreverseproxy
kubectl -n easytrade get endpoints frontendreverseproxy
```

### Pattern Won't Disable

**Symptom**: Pattern remains active after disable command

**Solutions**:

1. Force disable via API:
```bash
curl -X PUT http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/ \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'
```

2. Restart affected service:
```bash
kubectl -n easytrade rollout restart deployment/broker-service
```

3. Check feature flag service logs:
```bash
kubectl -n easytrade logs -l app=feature-flag-service --tail=100
```

---

## Best Practices

### For Platform Engineers

1. **Always disable persistent patterns** (factory_crisis, high_cpu_usage) after testing
2. **Document pattern usage** in runbooks and incident reports
3. **Use patterns in non-production environments first**
4. **Combine patterns with monitoring validation** to verify detection
5. **Schedule pattern testing** during maintenance windows when possible

### For SRE Teams

1. **Create automated runbooks** for each pattern scenario
2. **Integrate pattern management** into incident response playbooks
3. **Use patterns for game day exercises** and chaos engineering
4. **Monitor pattern impact** on dependent services
5. **Track MTTR (Mean Time To Recovery)** during drills

### For Observability Engineers

1. **Validate alert configurations** with each pattern
2. **Test anomaly detection** using gradual pattern activation
3. **Verify distributed tracing** captures error propagation
4. **Use patterns to tune alert thresholds** and reduce noise
5. **Document expected metrics** for each pattern scenario

---

## Advanced Usage

### Scheduled Problem Pattern Activation

EasyTrade includes CronJob manifests for scheduled pattern activation:

```bash
# Deploy problem pattern CronJobs
kubectl -n easytrade apply -f manifests/problem-patterns/

# CronJobs run daily at 8 AM UTC by default
# Patterns automatically disable after 1 hour
```

### CI/CD Integration

Example GitHub Actions workflow for automated chaos testing:

```yaml
name: Chaos Engineering Test

on:
  schedule:
    - cron: '0 2 * * 1'  # Monday 2 AM

jobs:
  chaos-test:
    runs-on: ubuntu-24.04
    steps:
      - name: Enable problem pattern
        run: make problem-enable PATTERN=high_cpu_usage
      
      - name: Run load test
        run: ./scripts/load-test.sh
      
      - name: Verify monitoring alerts
        run: ./scripts/check-dynatrace-alerts.sh
      
      - name: Disable pattern
        run: make problem-disable PATTERN=high_cpu_usage
```

---

## Reference

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/feature-flag-service/v1/flags/` | GET | List all flags |
| `/feature-flag-service/v1/flags/{pattern}/` | GET | Get flag status |
| `/feature-flag-service/v1/flags/{pattern}/` | PUT | Update flag status |

### Configuration

Feature flag cache duration can be adjusted via environment variable:

```yaml
env:
  - name: FEATURE_FLAG_CACHE_DURATION_S
    value: "30"  # Seconds
```

### Support

For issues or questions:
- Check deployment logs: `kubectl -n easytrade logs -l app=feature-flag-service`
- Run diagnostics: `make diagnose`
- Review Makefile targets: `make help`
