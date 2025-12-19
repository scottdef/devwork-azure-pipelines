#!/bin/bash
# scripts/feature-flags.sh
# Manage EasyTrade feature flags and problem patterns

set -euo pipefail

# Configuration
NAMESPACE="${EASYTRADE_NAMESPACE:-easytrade}"
SERVICE_NAME="frontendreverseproxy"
PORT=8080

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Get EasyTrade URL
get_easytrade_url() {
    # Try to get LoadBalancer IP
    local LB_IP=$(kubectl -n ${NAMESPACE} get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
    
    if [ -n "$LB_IP" ]; then
        echo "http://${LB_IP}"
    else
        # Use localhost with port-forward
        echo "http://localhost:${PORT}"
    fi
}

# Start port-forward if needed
ensure_portforward() {
    local EASYTRADE_URL=$(get_easytrade_url)
    
    if [[ $EASYTRADE_URL == *"localhost"* ]]; then
        echo -e "${YELLOW}Starting port-forward for feature flag access...${NC}"
        kubectl -n ${NAMESPACE} port-forward svc/${SERVICE_NAME} ${PORT}:80 > /tmp/feature-flags-pf.log 2>&1 &
        local PF_PID=$!
        echo $PF_PID > /tmp/feature-flags-pf.pid
        sleep 3
        echo -e "${GREEN}✓ Port-forward started (PID: $PF_PID)${NC}"
    fi
}

# Stop port-forward
cleanup_portforward() {
    if [ -f /tmp/feature-flags-pf.pid ]; then
        local PF_PID=$(cat /tmp/feature-flags-pf.pid)
        kill $PF_PID 2>/dev/null || true
        rm /tmp/feature-flags-pf.pid
    fi
}

# List all feature flags
list_flags() {
    local EASYTRADE_URL=$(get_easytrade_url)
    ensure_portforward
    
    echo -e "${GREEN}=== EasyTrade Feature Flags ===${NC}"
    echo -e "${CYAN}URL: ${EASYTRADE_URL}/feature-flag-service/v1/flags${NC}\n"
    
    local RESPONSE=$(curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags")
    
    if [ $? -eq 0 ]; then
        echo "$RESPONSE" | jq -r '.[] | "\(.name): \(if .enabled then "ENABLED" else "DISABLED" end)"' | while read line; do
            if [[ $line == *"ENABLED"* ]]; then
                echo -e "${RED}  $line${NC}"
            else
                echo -e "${GREEN}  $line${NC}"
            fi
        done
    else
        echo -e "${RED}Error: Could not fetch feature flags${NC}"
        echo -e "${YELLOW}Make sure EasyTrade is deployed and accessible${NC}"
        cleanup_portforward
        exit 1
    fi
    
    cleanup_portforward
}

# Enable a feature flag
enable_flag() {
    local FLAG_NAME=$1
    local EASYTRADE_URL=$(get_easytrade_url)
    ensure_portforward
    
    echo -e "${YELLOW}Enabling feature flag: ${FLAG_NAME}${NC}"
    
    local RESPONSE=$(curl -s -X PUT \
        "${EASYTRADE_URL}/feature-flag-service/v1/flags/${FLAG_NAME}/" \
        -H "Content-Type: application/json" \
        -d '{"enabled": true}')
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Feature flag '${FLAG_NAME}' enabled${NC}"
        echo "$RESPONSE" | jq .
    else
        echo -e "${RED}Error: Could not enable feature flag${NC}"
        cleanup_portforward
        exit 1
    fi
    
    cleanup_portforward
}

# Disable a feature flag
disable_flag() {
    local FLAG_NAME=$1
    local EASYTRADE_URL=$(get_easytrade_url)
    ensure_portforward
    
    echo -e "${YELLOW}Disabling feature flag: ${FLAG_NAME}${NC}"
    
    local RESPONSE=$(curl -s -X PUT \
        "${EASYTRADE_URL}/feature-flag-service/v1/flags/${FLAG_NAME}/" \
        -H "Content-Type: application/json" \
        -d '{"enabled": false}')
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Feature flag '${FLAG_NAME}' disabled${NC}"
        echo "$RESPONSE" | jq .
    else
        echo -e "${RED}Error: Could not disable feature flag${NC}"
        cleanup_portforward
        exit 1
    fi
    
    cleanup_portforward
}

# Get specific flag status
get_flag() {
    local FLAG_NAME=$1
    local EASYTRADE_URL=$(get_easytrade_url)
    ensure_portforward
    
    echo -e "${CYAN}Feature Flag: ${FLAG_NAME}${NC}"
    
    local RESPONSE=$(curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags/${FLAG_NAME}/")
    
    if [ $? -eq 0 ]; then
        echo "$RESPONSE" | jq .
    else
        echo -e "${RED}Error: Could not fetch feature flag${NC}"
        cleanup_portforward
        exit 1
    fi
    
    cleanup_portforward
}

# Display usage
usage() {
    cat << EOF
${GREEN}EasyTrade Feature Flags Manager${NC}

${CYAN}Usage:${NC}
  $0 list                      List all feature flags
  $0 get <flag-name>           Get specific flag status
  $0 enable <flag-name>        Enable a feature flag
  $0 disable <flag-name>       Disable a feature flag

${CYAN}Problem Patterns (Chaos Engineering):${NC}
  ${YELLOW}db_not_responding${NC}          Database throws errors on Trade table (~20 min)
  ${YELLOW}ergo_aggregator_slowdown${NC}   Two aggregators receive slow responses (15-30 min)
  ${YELLOW}factory_crisis${NC}             Factory stops producing credit cards (persistent)
  ${YELLOW}high_cpu_usage${NC}             Broker service CPU spike via Collatz (persistent)

${CYAN}Examples:${NC}
  $0 list
  $0 enable db_not_responding
  $0 disable high_cpu_usage
  $0 get ergo_aggregator_slowdown

${CYAN}Environment Variables:${NC}
  EASYTRADE_NAMESPACE         Kubernetes namespace (default: easytrade)
  
EOF
}

# Main
case "${1:-}" in
    list)
        list_flags
        ;;
    get)
        if [ -z "${2:-}" ]; then
            echo -e "${RED}Error: Flag name required${NC}"
            usage
            exit 1
        fi
        get_flag "$2"
        ;;
    enable)
        if [ -z "${2:-}" ]; then
            echo -e "${RED}Error: Flag name required${NC}"
            usage
            exit 1
        fi
        enable_flag "$2"
        ;;
    disable)
        if [ -z "${2:-}" ]; then
            echo -e "${RED}Error: Flag name required${NC}"
            usage
            exit 1
        fi
        disable_flag "$2"
        ;;
    *)
        usage
        exit 0
        ;;
esac
