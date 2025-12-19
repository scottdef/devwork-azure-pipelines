#!/bin/bash
# Port-Forward Helper Script for EasyTrade

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
NAMESPACE="${NAMESPACE:-easytrade}"
LOCAL_PORT="${LOCAL_PORT:-8080}"

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Stopping port-forward...${NC}"
    pkill -f "kubectl.*port-forward" 2>/dev/null || true
    exit 0
}

trap cleanup INT TERM

# Display menu
show_menu() {
    echo -e "${BLUE}=== EasyTrade Port-Forward ===${NC}"
    echo ""
    echo "Select service to forward:"
    echo "  1) Frontend (recommended) - http://localhost:${LOCAL_PORT}"
    echo "  2) Feature Flag Service - http://localhost:8081"
    echo "  3) Broker Service - http://localhost:8082"
    echo "  4) All Services (multiple ports)"
    echo "  q) Quit"
    echo ""
    read -p "Enter choice: " CHOICE
}

# Forward frontend
forward_frontend() {
    echo -e "${GREEN}Starting port-forward to frontend...${NC}"
    echo -e "${GREEN}Access EasyTrade at: http://localhost:${LOCAL_PORT}${NC}"
    echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
    echo ""
    kubectl -n "$NAMESPACE" port-forward svc/frontendreverseproxy "${LOCAL_PORT}:80"
}

# Forward feature flag service
forward_feature_flags() {
    echo -e "${GREEN}Starting port-forward to feature flag service...${NC}"
    echo -e "${GREEN}API available at: http://localhost:8081/feature-flag-service/v1/flags/${NC}"
    echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
    echo ""
    kubectl -n "$NAMESPACE" port-forward svc/feature-flag-service 8081:8080
}

# Forward broker service
forward_broker() {
    echo -e "${GREEN}Starting port-forward to broker service...${NC}"
    echo -e "${GREEN}API available at: http://localhost:8082${NC}"
    echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
    echo ""
    kubectl -n "$NAMESPACE" port-forward svc/broker-service 8082:8080
}

# Forward all services
forward_all() {
    echo -e "${GREEN}Starting port-forward for multiple services...${NC}"
    echo ""
    echo -e "${GREEN}Services:${NC}"
    echo -e "  Frontend:           http://localhost:8080"
    echo -e "  Feature Flags:      http://localhost:8081"
    echo -e "  Broker Service:     http://localhost:8082"
    echo -e "  Offer Service:      http://localhost:8087"
    echo -e "  Account Service:    http://localhost:8089"
    echo ""
    echo -e "${YELLOW}Press Ctrl+C to stop all forwards${NC}"
    echo ""
    
    kubectl -n "$NAMESPACE" port-forward svc/frontendreverseproxy 8080:80 &
    kubectl -n "$NAMESPACE" port-forward svc/feature-flag-service 8081:8080 &
    kubectl -n "$NAMESPACE" port-forward svc/broker-service 8082:8080 &
    kubectl -n "$NAMESPACE" port-forward svc/offerservice 8087:8087 &
    kubectl -n "$NAMESPACE" port-forward svc/accountservice 8089:8089 &
    
    wait
}

# Main logic
if [ $# -eq 0 ]; then
    show_menu
    
    case $CHOICE in
        1) forward_frontend ;;
        2) forward_feature_flags ;;
        3) forward_broker ;;
        4) forward_all ;;
        q|Q) exit 0 ;;
        *) echo -e "${RED}Invalid choice${NC}"; exit 1 ;;
    esac
else
    # Command-line argument handling
    case $1 in
        frontend|fe) forward_frontend ;;
        flags|ff) forward_feature_flags ;;
        broker|br) forward_broker ;;
        all) forward_all ;;
        *)
            echo "Usage: $0 [frontend|flags|broker|all]"
            exit 1
            ;;
    esac
fi
