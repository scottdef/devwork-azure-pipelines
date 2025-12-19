#!/bin/bash
# Deploy using Helm
set -euo pipefail
NAMESPACE="kubernetes-dashboard"
VALUES_FILE="${1:-./helm/values-prod.yaml}"
helm repo add kubernetes-dashboard https://kubernetes.github.io/dashboard/
helm repo update
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
    --namespace $NAMESPACE --create-namespace --version 7.14.0 \
    --values "$VALUES_FILE" --wait --timeout 10m --atomic
TOKEN=$(kubectl create token dashboard-admin -n $NAMESPACE --duration=24h 2>/dev/null || \
       kubectl -n $NAMESPACE create sa dashboard-admin && \
       kubectl create clusterrolebinding dashboard-admin --clusterrole=cluster-admin --serviceaccount=$NAMESPACE:dashboard-admin && \
       kubectl create token dashboard-admin -n $NAMESPACE --duration=24h)
echo "Admin Token: $TOKEN" | tee dashboard-token.txt
