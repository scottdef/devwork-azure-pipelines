# GitHub Actions Quick Setup Guide

Get your workflows running in 15 minutes.

## 🎯 Goal

Deploy Kubernetes Dashboard to AKS using GitHub Actions with zero manual intervention.

---

## Step 1: Fork/Clone Repository (2 min)

```bash
# Clone the repository
git clone https://github.com/your-org/kubernetes-dashboard-aks.git
cd kubernetes-dashboard-aks

# Or fork on GitHub
# Click "Fork" button on GitHub
```

---

## Step 2: Azure Setup (5 min)

### Create Service Principal

```bash
# Set variables
SUBSCRIPTION_ID="your-subscription-id"
RG="prod-cus-platform-base-rg-001"
APP_NAME="github-actions-dashboard"

# Create service principal
az ad sp create-for-rbac \
  --name "$APP_NAME" \
  --role "Contributor" \
  --scopes "/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RG" \
  --sdk-auth

# Output will look like:
# {
#   "clientId": "xxx",
#   "clientSecret": "xxx",
#   "subscriptionId": "xxx",
#   "tenantId": "xxx",
#   ...
# }
```

### Grant AKS Permissions

```bash
# Get service principal ID
SP_ID=$(az ad sp list --display-name "$APP_NAME" --query "[0].id" -o tsv)

# Grant AKS cluster user role
az role assignment create \
  --assignee $SP_ID \
  --role "Azure Kubernetes Service Cluster User Role" \
  --scope "/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RG/providers/Microsoft.ContainerService/managedClusters/prod-cus-aks-sre-lab-003"
```

### Optional: Setup OIDC (Recommended)

```bash
# Get application object ID
APP_OBJECT_ID=$(az ad app list --display-name "$APP_NAME" --query "[0].id" -o tsv)

# Create federated credential
cat > credential.json <<EOF
{
  "name": "github-actions-federated",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:your-org/kubernetes-dashboard-aks:ref:refs/heads/main",
  "description": "GitHub Actions Federated Identity",
  "audiences": ["api://AzureADTokenExchange"]
}
EOF

az ad app federated-credential create \
  --id $APP_OBJECT_ID \
  --parameters credential.json
```

---

## Step 3: GitHub Secrets (3 min)

### Navigate to Repository Settings

```
GitHub Repository → Settings → Secrets and variables → Actions
```

### Add Repository Secrets

Click "New repository secret" for each:

```
Name: AZURE_CLIENT_ID
Value: (clientId from Step 2)

Name: AZURE_TENANT_ID
Value: (tenantId from Step 2)

Name: AZURE_SUBSCRIPTION_ID
Value: (subscriptionId from Step 2)

Name: AZURE_CLIENT_SECRET (if not using OIDC)
Value: (clientSecret from Step 2)
```

**Pro Tip:** Copy-paste from the JSON output in Step 2.

---

## Step 4: GitHub Environments (3 min)

### Create Environments

```
GitHub Repository → Settings → Environments → New environment
```

### Create Three Environments:

**1. dev**
```
Name: dev
Protection rules: None
```

**2. staging**
```
Name: staging
Protection rules: Optional
  ☐ Required reviewers (optional)
  Deployment branches: Selected branches → main
```

**3. prod**
```
Name: prod
Protection rules:
  ☑ Required reviewers → Add reviewers (minimum 2)
  ☑ Wait timer → 5 minutes (optional)
  Deployment branches: Selected branches → main
```

---

## Step 5: Test First Workflow (2 min)

### Run Simple Deployment

```
1. Go to Actions tab
2. Click "Quick Deploy Dashboard"
3. Click "Run workflow"
4. Select:
   - Environment: dev
   - Method: helm
5. Click "Run workflow"
```

### Monitor Progress

```
1. Click on running workflow
2. Watch logs in real-time
3. Wait for completion (~3-5 minutes)
```

### Expected Output

```
✅ Azure Login
✅ Setup Tools
✅ Connect to AKS
✅ Deploy with Helm
✅ Create Admin User & Generate Token
✅ Verify Deployment
✅ Display Access Info
```

---

## Step 6: Access Dashboard

### From Workflow Summary

```
1. Click on completed workflow run
2. Find "Display Access Info" step
3. Copy port-forward command:
   kubectl port-forward -n kubernetes-dashboard svc/kubernetes-dashboard-kong-proxy 8443:443
```

### Or Generate Token Manually

```bash
# Connect to AKS
az login
az aks get-credentials --resource-group prod-cus-platform-base-rg-001 --name prod-cus-aks-sre-lab-003
kubelogin convert-kubeconfig -l azurecli

# Port-forward
kubectl port-forward -n kubernetes-dashboard svc/kubernetes-dashboard-kong-proxy 8443:443

# In another terminal, generate token
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

### Access Dashboard

```
1. Open browser: https://localhost:8443
2. Select "Token" authentication
3. Paste token from workflow or manual generation
4. Click "Sign In"
```

---

## ✅ Verification Checklist

- [ ] Service principal created
- [ ] AKS permissions granted
- [ ] GitHub secrets configured
- [ ] GitHub environments created
- [ ] First workflow completed successfully
- [ ] Can access dashboard UI
- [ ] Token authentication works

---

## 🎉 Success!

You now have automated deployments! Here's what you can do:

### Daily Development

```bash
# Make changes
git checkout -b feature/my-change
git commit -am "Update config"
git push origin feature/my-change

# Create PR → Workflows validate automatically
# Merge → Auto-deploys to dev
```

### Production Deployment

```
Actions → Terraform Deploy (Optimized) → Run workflow
  Action: plan
  Environment: prod
→ Review plan
→ Approve in environment
→ Auto-applies
```

### Emergency Rollback

```
Actions → Rollback Dashboard → Run workflow
  Environment: prod
  Rollback type: helm-history
  Confirm: ROLLBACK
```

---

## 📚 Next Steps

1. **Read Workflows README**: `.github/workflows/README.md`
2. **Setup Terraform Backend**: Follow [Configuration Guide](.github/workflows/README.md#terraform-backend)
3. **Configure Monitoring**: Setup workflow failure notifications
4. **Schedule Token Rotation**: security-token-rotation.yml runs daily automatically
5. **Backup Production**: Run operations.yml backup before major changes

---

## 🆘 Troubleshooting

### Workflow Fails at Azure Login

**Error:** `AADSTS700016: Application not found`

**Solution:**
```bash
# Verify service principal
az ad sp show --id {client-id}

# Recreate if needed
az ad sp create-for-rbac --name "github-actions-dashboard-new"
```

### Workflow Fails at kubectl Connection

**Error:** `Unable to connect to cluster`

**Solution:**
```bash
# Check cluster is running
az aks show -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003

# Verify permissions
az role assignment list --assignee {client-id}
```

### Token Not Working

**Error:** `Unauthorized`

**Solution:**
```bash
# Check service account exists
kubectl get sa dashboard-admin -n kubernetes-dashboard

# Check RBAC binding
kubectl get clusterrolebindings | grep dashboard-admin

# Regenerate token
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

### For More Help

- Check [Workflows README](.github/workflows/README.md#troubleshooting)
- Review workflow logs in Actions tab
- Check [Main README](../README.md)

---

## 🔒 Security Notes

1. **Never commit secrets** to git
2. **Use OIDC** instead of client secrets when possible
3. **Rotate tokens** every 24 hours (automated by security-token-rotation.yml)
4. **Review approvers** for production environment
5. **Enable audit logging** on AKS cluster
6. **Restrict service principal** to minimum permissions

---

## Time to Deploy: ~15 minutes ⏱️

**Breakdown:**
- Azure setup: 5 min
- GitHub configuration: 5 min
- First deployment: 3 min
- Verification: 2 min

You're now ready for production deployments! 🚀
