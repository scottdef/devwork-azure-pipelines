# EasyTrade Feature Flags & Problem Patterns Guide for AKS Platform Engineers

## Overview

EasyTrade includes a **Feature Flag Service** that controls application behavior through a REST API. The primary use case for AKS platform engineers is **chaos engineering** and **observability testing** using four built-in problem patterns that simulate real-world failure scenarios.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Feature Flag Service                      │
│                    (Port 8080, REST API)                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Flags checked every 30s
                              │ (configurable via ENV)
                              ↓
┌─────────────────────────────────────────────────────────────┐
│         Broker Service, Database, Factory, etc.              │
│         (Behavior changes based on flag state)               │
└─────────────────────────────────────────────────────────────┘
```

## Problem Patterns Reference

| Pattern ID | Component Affected | Behavior | Duration | Typical Alert Time |
|------------|-------------------|----------|----------|-------------------|
| `db_not_responding` | Database (Trade table) | SQL errors on operations | Persistent until disabled | ~20 min for Dynatrace |
| `ergo_aggregator_slowdown` | Aggregator services | Slow response times (2 aggregators) | Persistent | 15-30 min |
| `factory_crisis` | Credit Card Factory | Stops producing cards | Persistent | Immediate |
| `high_cpu_usage` | Broker Service | CPU spike via Collatz calculations | Persistent | 5-10 min |

## Access Methods

### 1. Via kubectl port-forward (Development/Testing)

```bash
# Start port-forward
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80

# In another terminal, interact with Feature Flag Service
export EASYTRADE_URL="http://localhost:8080"

# List all flags
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags" | jq

# Enable a problem pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/db_not_responding/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'

# Disable a problem pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/high_cpu_usage/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'
```

### 2. Via Internal LoadBalancer (Production)

```bash
# Deploy LoadBalancer
kubectl apply -f kubernetes/ingress/loadbalancer.yaml

# Get LoadBalancer IP
LB_IP=$(kubectl -n easytrade get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

export EASYTRADE_URL="http://${LB_IP}"

# Interact with Feature Flag Service
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags" | jq
```

### 3. Via Istio Ingress (Service Mesh)

```bash
# Get Istio ingress IP
ISTIO_IP=$(kubectl -n aks-istio-ingress get svc aks-istio-ingressgateway-internal -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

export EASYTRADE_URL="http://${ISTIO_IP}"

# Access Feature Flag Service
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags" | jq
```

## Using the Management Script

The repository includes `scripts/feature-flags.sh` (Bash) for automated flag management:

```bash
# List all feature flags with status
./scripts/feature-flags.sh list

# Get specific flag details
./scripts/feature-flags.sh get db_not_responding

# Enable a problem pattern
./scripts/feature-flags.sh enable high_cpu_usage

# Disable a problem pattern
./scripts/feature-flags.sh disable factory_crisis
```

For Windows:

```powershell
# PowerShell equivalent (create as needed)
.\scripts\feature-flags.ps1 -Action List
.\scripts\feature-flags.ps1 -Action Enable -Pattern db_not_responding
.\scripts\feature-flags.ps1 -Action Disable -Pattern high_cpu_usage
```

## Problem Pattern Details

### 1. Database Not Responding (`db_not_responding`)

**What it does:**
- Simulates database failures on the `Trade` table
- Causes SQL errors and failed transactions
- Tests database resilience and error handling

**Impact:**
- Users cannot execute trades
- Database error logs increase
- Transaction failure rate spikes

**Use Cases:**
- Test APM database monitoring
- Validate error handling in services
- Test circuit breaker patterns
- Verify database failure alerts

**Expected Behavior:**
```bash
# When enabled
curl -X POST "${EASYTRADE_URL}/api/Trade" 
# Returns: 500 Internal Server Error
# Logs show: "Database connection failed"

# Dynatrace should detect:
- Increased error rate
- Database problem alert (~20 min)
- Failed transaction metrics
```

**Enable/Disable:**
```bash
# Enable
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/db_not_responding/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'

# Disable
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/db_not_responding/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'
```

### 2. Ergo Aggregator Slowdown (`ergo_aggregator_slowdown`)

**What it does:**
- Injects latency into two aggregator services
- Simulates network degradation or slow backend responses
- Tests service timeout and retry logic

**Impact:**
- Increased response times for affected services
- Potential timeout errors
- Queue buildup in dependent services

**Use Cases:**
- Test latency monitoring
- Validate timeout configurations
- Test retry and circuit breaker patterns
- Verify SLO breach detection

**Expected Behavior:**
```bash
# Normal response time: ~100ms
# With slowdown: ~2000-5000ms

# Dynatrace should detect:
- Increased response time
- SLO violations
- Alert after 15-30 min
```

**Enable/Disable:**
```bash
# Enable
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/ergo_aggregator_slowdown/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'
```

### 3. Factory Crisis (`factory_crisis`)

**What it does:**
- Credit Card Factory stops producing new credit cards
- Simulates supply chain or production failures
- Tests application behavior when dependent services fail

**Impact:**
- No new credit cards generated
- Credit card inventory depletes
- Users cannot get new cards

**Use Cases:**
- Test inventory monitoring
- Validate graceful degradation
- Test business process alerts
- Verify fallback mechanisms

**Expected Behavior:**
```bash
# Check factory status
curl "${EASYTRADE_URL}/api/factory/status"
# Returns: "Factory stopped due to crisis"

# Dynatrace should detect:
- Factory service availability issue
- Business transaction failures
- Inventory depletion
```

**Enable/Disable:**
```bash
# Enable
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/factory_crisis/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'
```

### 4. High CPU Usage (`high_cpu_usage`)

**What it does:**
- Broker Service performs intensive Collatz conjecture calculations
- Simulates CPU exhaustion scenarios
- Tests resource monitoring and autoscaling

**Impact:**
- Broker Service CPU usage spikes to 80-100%
- Increased response times due to resource contention
- Potential pod throttling or OOM kills

**Use Cases:**
- Test horizontal pod autoscaling (HPA)
- Validate CPU monitoring and alerts
- Test Kubernetes resource limits
- Verify cluster autoscaling

**Expected Behavior:**
```bash
# Check CPU usage
kubectl top pods -n easytrade -l app=broker-service

# Should show high CPU:
NAME                              CPU(cores)   MEMORY(bytes)
broker-service-7d9f8c4b5d-abc12   950m         256Mi
broker-service-7d9f8c4b5d-def34   980m         245Mi

# Dynatrace should detect:
- High CPU usage alert (5-10 min)
- Performance degradation
- Resource saturation
```

**Environment Variables (Broker Service):**
```yaml
env:
  - name: HIGH_CPU_USAGE_REQUEST_DELAY_MS
    value: "1000"  # Delay per request
  - name: HIGH_CPU_USAGE_CONCURRENCY
    value: "4"     # Concurrent calculations
```

**Enable/Disable:**
```bash
# Enable
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/high_cpu_usage/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'
```

## Operational Procedures

### Pre-Test Checklist

```bash
# 1. Verify all pods are healthy
kubectl get pods -n easytrade
# All pods should be Running

# 2. Verify Feature Flag Service is accessible
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags" | jq
# Should return all flags

# 3. Verify monitoring is active (Dynatrace, Prometheus, etc.)
# Ensure you can see current metrics

# 4. Notify team of chaos test
# Send notification to team channel
```

### Test Execution Workflow

```bash
# Step 1: Establish baseline
echo "Capturing baseline metrics..."
kubectl top pods -n easytrade > baseline.txt

# Step 2: Enable problem pattern
echo "Enabling problem pattern: db_not_responding"
./scripts/feature-flags.sh enable db_not_responding

# Step 3: Monitor for alerts
echo "Monitoring for alerts (20 min window)..."
# Watch dashboards/alerts

# Step 4: Verify expected behavior
kubectl logs -n easytrade -l app=broker-service --tail=50

# Step 5: Disable problem pattern
echo "Disabling problem pattern"
./scripts/feature-flags.sh disable db_not_responding

# Step 6: Verify recovery
echo "Verifying recovery..."
kubectl get pods -n easytrade
```

### Automated Testing with CronJobs

EasyTrade can deploy problem patterns as CronJobs for scheduled chaos testing:

```bash
# Deploy problem pattern CronJobs
kubectl apply -f kubernetes/problem-patterns/problem-patterns.yaml

# CronJobs will enable patterns daily at 8 AM UTC
# Patterns run for configured duration then auto-disable
```

## Monitoring and Observability

### Key Metrics to Track

```yaml
# Prometheus metrics (example)
- broker_service_cpu_usage
- broker_service_response_time_ms
- database_error_rate
- trade_transaction_failures
- factory_production_rate

# Kubernetes metrics
kubectl top pods -n easytrade
kubectl get hpa -n easytrade

# Application logs
kubectl logs -n easytrade -l app=broker-service --tail=100 -f
```

### Dynatrace Integration

If Dynatrace is monitoring your cluster, you should see:

1. **Problem Detection:**
   - Database connectivity issues (~20 min)
   - Response time degradation (~15-30 min)
   - CPU saturation (5-10 min)
   - Service availability issues

2. **Davis AI Analysis:**
   - Root cause identification
   - Impact analysis
   - Automatic problem correlation

3. **Smart Alerts:**
   - Configured via Alerting Profiles
   - Notify appropriate teams

## Troubleshooting

### Feature Flag Service Not Responding

```bash
# Check pod status
kubectl get pods -n easytrade -l app=feature-flag-service

# Check logs
kubectl logs -n easytrade -l app=feature-flag-service --tail=50

# Restart if needed
kubectl rollout restart deployment/feature-flag-service -n easytrade
```

### Flag Changes Not Taking Effect

```bash
# Flags are cached for 30s by default
# Check FEATURE_FLAG_CACHE_DURATION_S environment variable

kubectl get deployment broker-service -n easytrade -o yaml | grep FEATURE_FLAG_CACHE

# Wait 30s after flag change, then verify
sleep 30
curl "${EASYTRADE_URL}/api/health"
```

### Cannot Access Feature Flag Service

```bash
# Verify service exists
kubectl get svc -n easytrade | grep feature-flag-service

# Check endpoints
kubectl get endpoints feature-flag-service -n easytrade

# Test from within cluster
kubectl run -it --rm test-curl --image=curlimages/curl --restart=Never -n easytrade -- \
    curl -s http://feature-flag-service:8080/v1/flags
```

## Best Practices

1. **Always Notify Before Testing**
   - Inform teams of planned chaos tests
   - Schedule during low-traffic periods
   - Document test windows

2. **Monitor During Tests**
   - Active monitoring required
   - Watch for unexpected cascading failures
   - Have rollback plan ready

3. **Use Automation**
   - Script common test scenarios
   - Automate flag toggling
   - Integrate with CI/CD for regular testing

4. **Document Results**
   - Record metrics before/after
   - Document alert response times
   - Note any unexpected behavior

5. **Gradual Escalation**
   - Start with single pattern
   - Increase complexity over time
   - Test recovery procedures

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Chaos Engineering Test

on:
  schedule:
    - cron: '0 8 * * 1'  # Every Monday at 8 AM
  workflow_dispatch:

jobs:
  chaos-test:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup kubectl
        uses: azure/setup-kubectl@v4
      
      - name: Configure AKS
        run: |
          az aks get-credentials -g ${{ secrets.RESOURCE_GROUP }} -n ${{ secrets.CLUSTER_NAME }}
      
      - name: Enable Problem Pattern
        run: |
          ./scripts/feature-flags.sh enable db_not_responding
      
      - name: Wait for alerts
        run: sleep 1200  # 20 minutes
      
      - name: Disable Problem Pattern
        run: |
          ./scripts/feature-flags.sh disable db_not_responding
      
      - name: Verify recovery
        run: |
          kubectl get pods -n easytrade
```

## Conclusion

EasyTrade's Feature Flag Service provides platform engineers with powerful tools for chaos engineering and observability testing. By systematically enabling problem patterns, you can:

- Validate monitoring and alerting configurations
- Test application resilience
- Verify autoscaling and self-healing
- Train teams on incident response
- Build confidence in production readiness

Regular use of these patterns ensures your AKS platform and applications are prepared for real-world failure scenarios.
