#!/bin/bash
# scripts/test-deployment.sh
# Comprehensive testing script for EasyTrade deployment

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
NAMESPACE="${EASYTRADE_NAMESPACE:-easytrade}"
TIMEOUT=300
VERBOSE="${1:-false}"

# Test results
TESTS_PASSED=0
TESTS_FAILED=0
FAILED_TESTS=()

echo -e "${GREEN}=== EasyTrade Deployment Test Suite ===${NC}"
echo -e "Namespace: ${CYAN}${NAMESPACE}${NC}"
echo -e "Timeout: ${TIMEOUT}s\n"

# Function to run test
run_test() {
    local test_name=$1
    local test_command=$2
    
    echo -ne "${YELLOW}Testing: ${test_name}...${NC} "
    
    if eval "$test_command" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ PASS${NC}"
        ((TESTS_PASSED++))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC}"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

# Function to run test with output
run_test_verbose() {
    local test_name=$1
    local test_command=$2
    
    echo -e "${YELLOW}Testing: ${test_name}${NC}"
    
    if eval "$test_command"; then
        echo -e "${GREEN}✓ PASS${NC}\n"
        ((TESTS_PASSED++))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC}\n"
        ((TESTS_FAILED++))
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

# Test 1: Namespace exists
run_test "Namespace exists" \
    "kubectl get namespace ${NAMESPACE}"

# Test 2: All pods are running
run_test "All pods running" \
    "[ \$(kubectl -n ${NAMESPACE} get pods --field-selector=status.phase!=Running --no-headers 2>/dev/null | wc -l) -eq 0 ]"

# Test 3: All deployments ready
run_test "All deployments ready" \
    "kubectl -n ${NAMESPACE} wait --for=condition=available --timeout=${TIMEOUT}s deployment --all"

# Test 4: Database pod ready
run_test "Database pod ready" \
    "kubectl -n ${NAMESPACE} wait --for=condition=ready --timeout=${TIMEOUT}s pod -l app=db"

# Test 5: Database connection
if kubectl -n ${NAMESPACE} get pod -l app=db &>/dev/null; then
    DB_POD=$(kubectl -n ${NAMESPACE} get pod -l app=db -o jsonpath='{.items[0].metadata.name}')
    run_test "Database connection" \
        "kubectl -n ${NAMESPACE} exec ${DB_POD} -- /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P 'yourStrong(!)Password' -Q 'SELECT 1' 2>/dev/null"
fi

# Test 6: All services have endpoints
run_test "All services have endpoints" \
    "[ \$(kubectl -n ${NAMESPACE} get endpoints -o json | jq '[.items[] | select(.subsets == null or .subsets == [])] | length') -eq 0 ]"

# Test 7: Frontend service accessible
run_test "Frontend service exists" \
    "kubectl -n ${NAMESPACE} get svc frontendreverseproxy"

# Test 8: Feature Flag Service accessible
if [ "${VERBOSE}" == "--verbose" ]; then
    run_test_verbose "Feature Flag Service API" \
        "kubectl -n ${NAMESPACE} port-forward svc/frontendreverseproxy 8080:80 &
        PF_PID=\$!
        sleep 3
        curl -sf http://localhost:8080/feature-flag-service/v1/flags
        kill \$PF_PID"
else
    run_test "Feature Flag Service API" \
        "kubectl -n ${NAMESPACE} port-forward svc/frontendreverseproxy 8080:80 &
        PF_PID=\$!
        sleep 3
        curl -sf http://localhost:8080/feature-flag-service/v1/flags > /dev/null
        RESULT=\$?
        kill \$PF_PID 2>/dev/null
        exit \$RESULT"
fi

# Test 9: No error events in last 5 minutes
ERROR_COUNT=$(kubectl -n ${NAMESPACE} get events --field-selector type=Warning 2>/dev/null | grep -v "LAST SEEN" | wc -l)
run_test "No recent error events" \
    "[ ${ERROR_COUNT} -eq 0 ]"

# Test 10: Resource quotas in place
run_test "Resource quota exists" \
    "kubectl -n ${NAMESPACE} get resourcequota"

# Test 11: Persistent volume claims bound
run_test "PVCs bound" \
    "[ \$(kubectl -n ${NAMESPACE} get pvc --field-selector=status.phase!=Bound --no-headers 2>/dev/null | wc -l) -eq 0 ]"

# Test 12: ConfigMap exists
run_test "ConfigMap exists" \
    "kubectl -n ${NAMESPACE} get configmap easytrade-config"

# Test 13: Secret exists
run_test "Secret exists" \
    "kubectl -n ${NAMESPACE} get secret easytrade-db-secret"

# Test 14: Istio sidecar injection (if enabled)
if kubectl get namespace ${NAMESPACE} -o jsonpath='{.metadata.labels.istio-injection}' 2>/dev/null | grep -q "enabled"; then
    run_test "Istio sidecar injection enabled" \
        "[ \$(kubectl -n ${NAMESPACE} get pods -o json | jq '[.items[] | select(.spec.containers | length > 1)] | length') -gt 0 ]"
fi

# Test 15: LoadBalancer IP assigned (if exists)
if kubectl -n ${NAMESPACE} get svc easytrade-lb &>/dev/null; then
    run_test "LoadBalancer IP assigned" \
        "kubectl -n ${NAMESPACE} get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' | grep -q '[0-9]'"
fi

# Summary
echo -e "\n${GREEN}=== Test Summary ===${NC}"
echo -e "Tests Passed: ${GREEN}${TESTS_PASSED}${NC}"
echo -e "Tests Failed: ${RED}${TESTS_FAILED}${NC}"

if [ ${TESTS_FAILED} -gt 0 ]; then
    echo -e "\n${RED}Failed Tests:${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  ${RED}✗${NC} $test"
    done
    
    echo -e "\n${YELLOW}Troubleshooting:${NC}"
    echo -e "  1. Check pod logs: kubectl logs -n ${NAMESPACE} <pod-name>"
    echo -e "  2. Describe resources: kubectl describe -n ${NAMESPACE} <resource-type> <name>"
    echo -e "  3. Check events: kubectl get events -n ${NAMESPACE} --sort-by='.lastTimestamp'"
    echo -e "  4. Run health check: ./scripts/health-check.sh --verbose"
    
    exit 1
else
    echo -e "\n${GREEN}✓ All tests passed!${NC}"
    echo -e "\n${CYAN}Next steps:${NC}"
    echo -e "  1. Access EasyTrade: make setup-portforward"
    echo -e "  2. Test feature flags: ./scripts/feature-flags.sh list"
    echo -e "  3. Monitor: kubectl top pods -n ${NAMESPACE}"
    
    exit 0
fi
