# Deploy with Helm
param([string]$ValuesFile = ".\helm\values-prod.yaml")
$Namespace = "kubernetes-dashboard"
helm repo add kubernetes-dashboard https://kubernetes.github.io/dashboard/
helm repo update
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard `
    --namespace $Namespace --create-namespace --version 7.14.0 `
    --values $ValuesFile --wait --timeout 10m --atomic
$token = kubectl create token dashboard-admin -n $Namespace --duration=24h
Write-Host "Token: $token" -ForegroundColor Yellow
$token | Out-File dashboard-token.txt
