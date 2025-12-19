#!/bin/bash
# EasyTrade Deployment Verification Script

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
NAMESPACE="${NAMESPACE:-easytrade}"

echo -e "${BLUE}=== EasyTrade Deployment Verification ===${NC}"
echo ""

# Check namespace exists
echo -e "${BLUE}Checking namespace...${NC}"
if kubectl get namespace "$NAMESPACE" &>/dev/null; then
    echo -e "${GREEN}✓ Namespace exists: ${NAMESPACE}${NC}"
else
    echo -e "${RED}✗ Namespace not found: ${NAMESPACE}${NC}"
    exit 1
fi
echo ""

# Check all pods are running
echo -e "${BLUE}Checking pod status...${NC}"
TOTAL_PODS=$(kubectl -n "$NAMESPACE" get pods --no-headers 2>/dev/null | wc -l)
RUNNING_PODS=$(kubectl -n "$NAMESPACE" get pods --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)

echo -e "Total pods: ${TOTAL_PODS}"
echo -e "Running pods: ${RUNNING_PODS}"

if [ "$RUNNING_PODS" -eq "$TOTAL_PODS" ] && [ "$TOTAL_PODS" -gt 0 ]; then
    echo -e "${GREEN}✓ All pods are running${NC}"
else
    echo -e "${YELLOW}⚠ Not all pods are running${NC}"
    echo ""
    kubectl -n "$NAMESPACE" get pods
fi
echo ""

# Check deployments
echo -e "${BLUE}Checking deployments...${NC}"
DEPLOYMENTS=$(kubectl -n "$NAMESPACE" get deployments -o jsonpath='{.items[*].metadata.name}')
ALL_READY=true

for DEPLOY in $DEPLOYMENTS; do
    DESIRED=$(kubectl -n "$NAMESPACE" get deployment "$DEPLOY" -o jsonpath='{.spec.replicas}')
    READY=$(kubectl -n "$NAMESPACE" get deployment "$DEPLOY" -o jsonpath='{.status.readyReplicas}')
    READY=${READY:-0}
    
    if [ "$READY" -eq "$DESIRED" ]; then
        echo -e "${GREEN}✓ ${DEPLOY}: ${READY}/${DESIRED}${NC}"
    else
        echo -e "${RED}✗ ${DEPLOY}: ${READY}/${DESIRED}${NC}"
        ALL_READY=false
    fi
done
echo ""

# Check services
echo -e "${BLUE}Checking services...${NC}"
kubectl -n "$NAMESPACE" get svc
echo ""

# Check database connectivity
echo -e "${BLUE}Checking database connectivity...${NC}"
DB_POD=$(kubectl -n "$NAMESPACE" get pods -l app=db -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -n "$DB_POD" ]; then
    if kubectl -n "$NAMESPACE" exec "$DB_POD" -- /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P 'Dynatrace123!' -Q "SELECT 1" &>/dev/null; then
        echo -e "${GREEN}✓ Database is accessible${NC}"
    else
        echo -e "${YELLOW}⚠ Database connectivity issue${NC}"
    fi
else
    echo -e "${RED}✗ Database pod not found${NC}"
fi
echo ""

# Check frontend accessibility via port-forward
echo -e "${BLUE}Checking frontend accessibility...${NC}"
kubectl -n "$NAMESPACE" port-forward svc/frontendreverseproxy 8080:80 &>/dev/null &
PF_PID=$!
sleep 3

if curl -s -o /dev/null -w "%{http_code}" http://localhost:8080 | grep -q "200\|301\|302"; then
    echo -e "${GREEN}✓ Frontend is accessible via port-forward${NC}"
else
    echo -e "${YELLOW}⚠ Frontend may not be ready yet${NC}"
fi

# Cleanup port-forward
kill $PF_PID 2>/dev/null || true
echo ""

# Check for recent errors in logs
echo -e "${BLUE}Checking for recent errors...${NC}"
ERROR_COUNT=$(kubectl -n "$NAMESPACE" logs --tail=100 --all-containers=true -l app.kubernetes.io/name=easytrade 2>/dev/null | grep -i "error\|exception\|fatal" | wc -l)

if [ "$ERROR_COUNT" -eq 0 ]; then
    echo -e "${GREEN}✓ No recent errors detected${NC}"
else
    echo -e "${YELLOW}⚠ Found ${ERROR_COUNT} error messages in recent logs${NC}"
    echo -e "${YELLOW}  Run 'make logs-all' to investigate${NC}"
fi
echo ""

# Check resource usage
echo -e "${BLUE}Checking resource usage...${NC}"
if kubectl top nodes &>/dev/null; then
    echo -e "${GREEN}Node metrics:${NC}"
    kubectl top nodes
    echo ""
    echo -e "${GREEN}Pod metrics (top 10):${NC}"
    kubectl -n "$NAMESPACE" top pods --sort-by=cpu | head -11
else
    echo -e "${YELLOW}⚠ Metrics not available (metrics-server may not be installed)${NC}"
fi
echo ""

# Recent events
echo -e "${BLUE}Recent events (last 10):${NC}"
kubectl -n "$NAMESPACE" get events --sort-by='.lastTimestamp' | tail -10
echo ""

# Final summary
echo -e "${BLUE}=== Verification Summary ===${NC}"

if [ "$ALL_READY" = true ] && [ "$RUNNING_PODS" -eq "$TOTAL_PODS" ]; then
    echo -e "${GREEN}✓ Deployment is healthy${NC}"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo -e "  1. Access the application:"
    echo -e "     make port-forward"
    echo -e "     # Open http://localhost:8080"
    echo ""
    echo -e "  2. Enable problem patterns:"
    echo -e "     make problem-list"
    echo -e "     make problem-enable PATTERN=high_cpu_usage"
    echo ""
    echo -e "  3. Deploy private LoadBalancer:"
    echo -e "     make deploy-private-lb"
    exit 0
else
    echo -e "${YELLOW}⚠ Deployment has issues${NC}"
    echo ""
    echo -e "${YELLOW}Troubleshooting:${NC}"
    echo -e "  1. Check pod status:      kubectl -n ${NAMESPACE} get pods"
    echo -e "  2. View pod logs:         make logs SERVICE=<service-name>"
    echo -e "  3. Describe problem pod:  kubectl -n ${NAMESPACE} describe pod <pod-name>"
    echo -e "  4. Run full diagnostics:  make diagnose"
    exit 1
fi
