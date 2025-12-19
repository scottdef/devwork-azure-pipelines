#!/bin/bash
# scripts/health-check.sh
# Comprehensive health check for EasyTrade deployment

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
NAMESPACE="${EASYTRADE_NAMESPACE:-easytrade}"
VERBOSE="${1:-false}"

echo -e "${GREEN}=== EasyTrade Health Check ===${NC}"
echo -e "Namespace: ${CYAN}${NAMESPACE}${NC}\n"

# Track health status
HEALTHY=true

# Check namespace exists
echo -e "${YELLOW}Checking namespace...${NC}"
if kubectl get namespace ${NAMESPACE} &>/dev/null; then
    echo -e "${GREEN}✓ Namespace '${NAMESPACE}' exists${NC}"
else
    echo -e "${RED}✗ Namespace '${NAMESPACE}' not found${NC}"
    HEALTHY=false
    exit 1
fi

# Check pod status
echo -e "\n${YELLOW}Checking pod status...${NC}"
TOTAL_PODS=$(kubectl -n ${NAMESPACE} get pods --no-headers 2>/dev/null | wc -l)
RUNNING_PODS=$(kubectl -n ${NAMESPACE} get pods --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
PENDING_PODS=$(kubectl -n ${NAMESPACE} get pods --field-selector=status.phase=Pending --no-headers 2>/dev/null | wc -l)
FAILED_PODS=$(kubectl -n ${NAMESPACE} get pods --field-selector=status.phase=Failed --no-headers 2>/dev/null | wc -l)

echo -e "Total Pods: ${CYAN}${TOTAL_PODS}${NC}"
echo -e "Running: ${GREEN}${RUNNING_PODS}${NC}"
echo -e "Pending: ${YELLOW}${PENDING_PODS}${NC}"
echo -e "Failed: ${RED}${FAILED_PODS}${NC}"

if [ ${RUNNING_PODS} -ne ${TOTAL_PODS} ] || [ ${FAILED_PODS} -gt 0 ]; then
    echo -e "${RED}✗ Not all pods are running${NC}"
    HEALTHY=false
    
    if [ "${VERBOSE}" == "--verbose" ]; then
        echo -e "\n${YELLOW}Non-running pods:${NC}"
        kubectl -n ${NAMESPACE} get pods | grep -v "Running"
    fi
else
    echo -e "${GREEN}✓ All pods are running${NC}"
fi

# Check deployments
echo -e "\n${YELLOW}Checking deployments...${NC}"
DEPLOYMENTS_READY=true
while IFS= read -r line; do
    NAME=$(echo $line | awk '{print $1}')
    READY=$(echo $line | awk '{print $2}')
    
    if [[ ! $READY =~ ^[0-9]+/[0-9]+$ ]]; then
        continue
    fi
    
    READY_COUNT=$(echo $READY | cut -d'/' -f1)
    DESIRED_COUNT=$(echo $READY | cut -d'/' -f2)
    
    if [ ${READY_COUNT} -eq ${DESIRED_COUNT} ]; then
        echo -e "  ${GREEN}✓${NC} ${NAME}: ${READY}"
    else
        echo -e "  ${RED}✗${NC} ${NAME}: ${READY}"
        DEPLOYMENTS_READY=false
        HEALTHY=false
    fi
done < <(kubectl -n ${NAMESPACE} get deployments --no-headers 2>/dev/null)

if [ "${DEPLOYMENTS_READY}" == "true" ]; then
    echo -e "${GREEN}✓ All deployments are ready${NC}"
fi

# Check services
echo -e "\n${YELLOW}Checking services...${NC}"
SERVICES=$(kubectl -n ${NAMESPACE} get svc --no-headers 2>/dev/null | wc -l)
echo -e "Total Services: ${CYAN}${SERVICES}${NC}"

# Check critical services
CRITICAL_SERVICES=("frontendreverseproxy" "broker-service" "feature-flag-service" "db")
for SVC in "${CRITICAL_SERVICES[@]}"; do
    if kubectl -n ${NAMESPACE} get svc ${SVC} &>/dev/null; then
        ENDPOINTS=$(kubectl -n ${NAMESPACE} get endpoints ${SVC} -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null | wc -w)
        if [ ${ENDPOINTS} -gt 0 ]; then
            echo -e "  ${GREEN}✓${NC} ${SVC}: ${ENDPOINTS} endpoint(s)"
        else
            echo -e "  ${RED}✗${NC} ${SVC}: No endpoints"
            HEALTHY=false
        fi
    else
        echo -e "  ${RED}✗${NC} ${SVC}: Not found"
        HEALTHY=false
    fi
done

# Check database
echo -e "\n${YELLOW}Checking database...${NC}"
if kubectl -n ${NAMESPACE} get pod -l app=db &>/dev/null; then
    DB_POD=$(kubectl -n ${NAMESPACE} get pod -l app=db -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    
    if [ -n "${DB_POD}" ]; then
        DB_STATUS=$(kubectl -n ${NAMESPACE} get pod ${DB_POD} -o jsonpath='{.status.phase}')
        
        if [ "${DB_STATUS}" == "Running" ]; then
            echo -e "${GREEN}✓ Database pod is running${NC}"
            
            # Test database connection
            if kubectl -n ${NAMESPACE} exec ${DB_POD} -- /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P "${DB_SA_PASSWORD:-yourStrong(!)Password}" -Q "SELECT 1" &>/dev/null; then
                echo -e "${GREEN}✓ Database connection successful${NC}"
            else
                echo -e "${YELLOW}⚠ Database connection test failed${NC}"
            fi
        else
            echo -e "${RED}✗ Database pod status: ${DB_STATUS}${NC}"
            HEALTHY=false
        fi
    fi
else
    echo -e "${RED}✗ Database pod not found${NC}"
    HEALTHY=false
fi

# Check feature flag service
echo -e "\n${YELLOW}Checking Feature Flag Service...${NC}"
if kubectl -n ${NAMESPACE} get svc frontendreverseproxy &>/dev/null; then
    # Try to access feature flag service
    kubectl -n ${NAMESPACE} port-forward svc/frontendreverseproxy 8080:80 &>/dev/null &
    PF_PID=$!
    sleep 2
    
    if curl -s -f http://localhost:8080/feature-flag-service/v1/flags &>/dev/null; then
        echo -e "${GREEN}✓ Feature Flag Service is accessible${NC}"
        FLAG_COUNT=$(curl -s http://localhost:8080/feature-flag-service/v1/flags | jq '. | length')
        echo -e "  Available flags: ${CYAN}${FLAG_COUNT}${NC}"
    else
        echo -e "${YELLOW}⚠ Feature Flag Service not accessible (may require LoadBalancer)${NC}"
    fi
    
    kill ${PF_PID} 2>/dev/null || true
else
    echo -e "${RED}✗ Frontend reverse proxy service not found${NC}"
    HEALTHY=false
fi

# Check resource usage
echo -e "\n${YELLOW}Checking resource usage...${NC}"
if kubectl top nodes &>/dev/null; then
    echo -e "${GREEN}✓ Metrics server available${NC}"
    
    if [ "${VERBOSE}" == "--verbose" ]; then
        echo -e "\n${CYAN}Node resources:${NC}"
        kubectl top nodes
        
        echo -e "\n${CYAN}Pod resources (top 10):${NC}"
        kubectl -n ${NAMESPACE} top pods --sort-by=memory | head -11
    fi
else
    echo -e "${YELLOW}⚠ Metrics server not available${NC}"
fi

# Check recent events
echo -e "\n${YELLOW}Checking recent events...${NC}"
ERROR_EVENTS=$(kubectl -n ${NAMESPACE} get events --field-selector type=Warning --no-headers 2>/dev/null | wc -l)

if [ ${ERROR_EVENTS} -gt 0 ]; then
    echo -e "${YELLOW}⚠ Found ${ERROR_EVENTS} warning events${NC}"
    
    if [ "${VERBOSE}" == "--verbose" ]; then
        echo -e "\n${CYAN}Recent warnings:${NC}"
        kubectl -n ${NAMESPACE} get events --field-selector type=Warning --sort-by='.lastTimestamp' | tail -5
    fi
else
    echo -e "${GREEN}✓ No warning events${NC}"
fi

# Summary
echo -e "\n${GREEN}=== Health Check Summary ===${NC}"

if [ "${HEALTHY}" == "true" ]; then
    echo -e "${GREEN}✓ EasyTrade is HEALTHY${NC}"
    echo -e "\nAccess methods:"
    echo -e "  Port-forward: kubectl -n ${NAMESPACE} port-forward svc/frontendreverseproxy 8080:80"
    
    LB_IP=$(kubectl -n ${NAMESPACE} get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
    if [ -n "${LB_IP}" ]; then
        echo -e "  LoadBalancer: http://${LB_IP}"
    fi
    
    exit 0
else
    echo -e "${RED}✗ EasyTrade has ISSUES${NC}"
    echo -e "\nTroubleshooting steps:"
    echo -e "  1. Check pod logs: kubectl logs -n ${NAMESPACE} <pod-name>"
    echo -e "  2. Describe pods: kubectl describe pod -n ${NAMESPACE} <pod-name>"
    echo -e "  3. Check events: kubectl get events -n ${NAMESPACE} --sort-by='.lastTimestamp'"
    echo -e "  4. Restart deployments: kubectl rollout restart deployment -n ${NAMESPACE}"
    
    exit 1
fi
