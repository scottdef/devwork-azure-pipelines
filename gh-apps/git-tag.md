# GitHub Enterprise Automated Git Tagging Workflows: Complete Implementation Guide

This comprehensive guide provides production-ready configurations for setting up automated git tagging workflows on GitHub Enterprise with multiple authentication methods, trigger types, and Azure Kubernetes Service integration.

## Overview and architecture

Automated git tagging workflows enable consistent version control and deployment automation in enterprise environments. This guide covers two primary trigger patterns: manual workflow dispatch with input validation and automated triggering on branch merges, with full integration support for Azure Kubernetes Service environments.

**Key workflow components:**
- **Manual dispatch workflows** with comprehensive input validation and parameter handling
- **Automated merge-triggered workflows** using conventional commit analysis for semantic versioning
- **Four authentication patterns** supporting different enterprise security models
- **GitHub Enterprise Server API integration** for programmatic workflow management
- **AKS deployment integration** with kubectl 1.30 compatibility

## 1. GitHub Actions Workflow YAML Configurations

### Manual workflow dispatch with input parameters

This production-ready workflow supports comprehensive input validation, environment targeting, and enterprise security controls:

```yaml
name: Enterprise Manual Tagging
run-name: "Manual Tag: ${{ github.event.inputs.version }} → ${{ github.event.inputs.environment }}"

on:
  workflow_dispatch:
    inputs:
      version:
        description: 'Release version (e.g., v1.2.3)'
        required: true
        type: string
      environment:
        description: 'Target environment'
        required: true
        type: environment
      tag_message:
        description: 'Tag annotation message'
        required: false
        type: string
        default: 'Manual release'
      create_release:
        description: 'Create GitHub release'
        type: boolean
        default: true
      force_overwrite:
        description: 'Force overwrite existing tag'
        type: boolean
        default: false

permissions:
  contents: write
  pull-requests: read
  actions: read

env:
  TAG_PREFIX: "release"
  ENTERPRISE_DOMAIN: "github.company.com"

jobs:
  validate-and-tag:
    name: Validate & Create Tag
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment }}
    outputs:
      final_tag: ${{ steps.create_tag.outputs.tag_name }}
      release_url: ${{ steps.create_release.outputs.html_url }}
    
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Validate Version & Environment
        id: validate
        run: |
          VERSION="${{ github.event.inputs.version }}"
          ENV="${{ github.event.inputs.environment }}"
          
          # Semantic version validation
          if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+(\.a-zA-Z0-9]+)*)?$ ]]; then
            echo "❌ Invalid version format. Use: v1.2.3 or v1.2.3-alpha.1"
            exit 1
          fi
          
          # Environment-specific validation
          case $ENV in
            production)
              if [[ ! "${{ github.actor }}" =~ ^(admin|release-manager|devops-team)$ ]]; then
                echo "❌ Production releases require elevated permissions"
                exit 1
              fi
              ;;
            staging|development)
              echo "✅ Non-production environment approved"
              ;;
            *)
              echo "❌ Unknown environment: $ENV"
              exit 1
              ;;
          esac
          
          echo "validated_version=$VERSION" >> $GITHUB_OUTPUT
          echo "✅ Validation completed successfully"

      - name: Configure Git for Enterprise
        run: |
          git config --local user.email "github-actions@company.com"
          git config --local user.name "GitHub Enterprise Actions"
          git config --local tag.gpgSign false

      - name: Handle Tag Conflicts
        id: handle_conflicts
        run: |
          TAG_NAME="${{ steps.validate.outputs.validated_version }}"
          
          # Check for existing tags
          if git tag -l | grep -q "^$TAG_NAME$"; then
            echo "Local tag $TAG_NAME exists"
            LOCAL_TAG_EXISTS=true
          else
            LOCAL_TAG_EXISTS=false
          fi
          
          if git ls-remote --tags origin | grep -q "refs/tags/$TAG_NAME$"; then
            echo "Remote tag $TAG_NAME exists"
            REMOTE_TAG_EXISTS=true
          else
            REMOTE_TAG_EXISTS=false
          fi
          
          # Handle conflicts based on force flag
          if [[ "$REMOTE_TAG_EXISTS" == "true" ]]; then
            if [[ "${{ github.event.inputs.force_overwrite }}" == "true" ]]; then
              echo "🔄 Force overwrite enabled, deleting existing tag"
              git push --delete origin "$TAG_NAME" 2>/dev/null || true
              git tag -d "$TAG_NAME" 2>/dev/null || true
            else
              echo "❌ Tag $TAG_NAME already exists. Use force_overwrite to replace."
              exit 1
            fi
          fi
          
          echo "conflict_resolved=true" >> $GITHUB_OUTPUT

      - name: Create Enterprise Tag
        id: create_tag
        run: |
          TAG_NAME="${{ steps.validate.outputs.validated_version }}"
          ENV="${{ github.event.inputs.environment }}"
          MESSAGE="${{ github.event.inputs.tag_message }}"
          
          # Create comprehensive tag annotation
          cat > tag_message.txt <<EOF
          $MESSAGE
          
          Release Information:
          - Version: $TAG_NAME
          - Environment: $ENV
          - Created by: ${{ github.actor }}
          - Workflow: ${{ github.workflow }}
          - Run ID: ${{ github.run_id }}
          - Commit SHA: $(git rev-parse HEAD)
          - Created: $(date -u +%Y-%m-%dT%H:%M:%SZ)
          
          Recent Changes:
          $(git log --oneline --max-count=10)
          
          Enterprise Approval:
          - Environment: $ENV
          - Approver: ${{ github.actor }}
          - Approval Time: $(date -u)
          EOF
          
          # Create annotated tag
          git tag -a "$TAG_NAME" -F tag_message.txt
          
          # Push to remote
          git push origin "$TAG_NAME"
          
          echo "tag_name=$TAG_NAME" >> $GITHUB_OUTPUT
          echo "✅ Tag $TAG_NAME created and pushed successfully"

      - name: Create GitHub Release
        id: create_release
        if: github.event.inputs.create_release == 'true'
        uses: actions/create-release@v1
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        with:
          tag_name: ${{ steps.create_tag.outputs.tag_name }}
          release_name: "Enterprise Release ${{ steps.create_tag.outputs.tag_name }}"
          body: |
            ## 🚀 Enterprise Release ${{ steps.create_tag.outputs.tag_name }}
            
            **Environment:** ${{ github.event.inputs.environment }}
            **Created by:** ${{ github.actor }}
            **Message:** ${{ github.event.inputs.tag_message }}
            
            ### 📋 Release Details
            - **Commit:** ${{ github.sha }}
            - **Workflow Run:** [${{ github.run_id }}](https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }})
            - **Created:** $(date -u +%Y-%m-%dT%H:%M:%SZ)
            
            ### 🔄 Recent Changes
            $(git log --pretty=format:"- %s (%an)" $PREVIOUS_TAG..HEAD || echo "- Initial release")
            
            ---
            *This release was created automatically via GitHub Enterprise Actions*
          draft: false
          prerelease: ${{ contains(steps.create_tag.outputs.tag_name, '-') }}
```

### Automated triggering on branch merges

This workflow automatically creates semantic version tags based on conventional commit messages:

```yaml
name: Automated Git Tagging on Merge
run-name: "Auto-tag for merge to ${{ github.ref_name }} by ${{ github.actor }}"

on:
  push:
    branches:
      - main
      - master
  pull_request:
    types:
      - closed
    branches:
      - main
      - master

permissions:
  contents: write
  pull-requests: read

jobs:
  auto-tag:
    name: Create Automatic Tag
    runs-on: ubuntu-latest
    if: |
      (github.event_name == 'push') || 
      (github.event_name == 'pull_request' && github.event.pull_request.merged == true)
    steps:
      - name: Checkout Repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Configure Git
        run: |
          git config --local user.email "github-actions[bot]@users.noreply.github.com"
          git config --local user.name "github-actions[bot]"

      - name: Determine Version Bump
        id: version_bump
        run: |
          # Get the latest tag
          LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
          echo "Latest tag: $LATEST_TAG"
          
          # Extract version numbers
          VERSION=${LATEST_TAG#v}
          IFS='.' read -ra VERSION_PARTS <<< "$VERSION"
          MAJOR=${VERSION_PARTS[0]:-0}
          MINOR=${VERSION_PARTS[1]:-0}
          PATCH=${VERSION_PARTS[2]:-0}
          
          # Analyze commit messages to determine bump type
          COMMIT_MESSAGES=$(git log --pretty=format:"%s" $LATEST_TAG..HEAD)
          
          if echo "$COMMIT_MESSAGES" | grep -qiE "^feat(\(.+\))?!:|^fix(\(.+\))?!:|BREAKING CHANGE"; then
            # Major version bump for breaking changes
            NEW_VERSION="v$((MAJOR + 1)).0.0"
            BUMP_TYPE="major"
          elif echo "$COMMIT_MESSAGES" | grep -qiE "^feat(\(.+\))?:"; then
            # Minor version bump for features
            NEW_VERSION="v$MAJOR.$((MINOR + 1)).0"
            BUMP_TYPE="minor"
          else
            # Patch version bump for everything else
            NEW_VERSION="v$MAJOR.$MINOR.$((PATCH + 1))"
            BUMP_TYPE="patch"
          fi
          
          echo "new_version=$NEW_VERSION" >> $GITHUB_OUTPUT
          echo "bump_type=$BUMP_TYPE" >> $GITHUB_OUTPUT
          echo "latest_tag=$LATEST_TAG" >> $GITHUB_OUTPUT
          
          echo "Version bump: $LATEST_TAG → $NEW_VERSION ($BUMP_TYPE)"

      - name: Create Automatic Tag
        run: |
          NEW_VERSION="${{ steps.version_bump.outputs.new_version }}"
          
          # Generate changelog
          CHANGELOG=$(git log --pretty=format:"- %s (%an)" ${{ steps.version_bump.outputs.latest_tag }}..HEAD)
          
          # Create annotated tag
          git tag -a "$NEW_VERSION" -m "Automatic ${{ steps.version_bump.outputs.bump_type }} release $NEW_VERSION

          Changes since ${{ steps.version_bump.outputs.latest_tag }}:
          $CHANGELOG

          Created automatically on merge to ${{ github.ref_name }}
          Triggered by: ${{ github.event_name }}
          Actor: ${{ github.actor }}
          Workflow Run: ${{ github.run_id }}"
          
          # Push tag
          git push origin "$NEW_VERSION"
          
          echo "✅ Created automatic tag: $NEW_VERSION"
```

## 2. Authentication Setup and Configuration

### Git global config setup for authentication

**Basic enterprise configuration:**
```bash
# Set global user identity
git config --global user.name "Your Name"
git config --global user.email "your.email@company.com"

# Configure credential helper for HTTPS authentication
git config --global credential.helper store

# Configure context-specific credentials for GitHub Enterprise
git config --global credential.https://github.company.com.username your-username

# For GitHub Enterprise Server instances
git config --global url."https://github.company.com/".insteadOf "https://github.com/"

# Set up GitHub CLI for Enterprise
gh auth login --hostname github.company.com
gh auth setup-git --hostname github.company.com
```

**Complete .gitconfig example:**
```ini
[user]
    name = Your Name
    email = your.email@company.com
[credential]
    helper = store
[credential "https://github.company.com"]
    username = your-username
[url "https://github.company.com/"]
    insteadOf = https://github.com/
```

### Personal Access Token (PAT) authentication

**Fine-grained personal access tokens (recommended):**

Creation steps for GitHub Enterprise:
1. Navigate to Settings > Developer settings > Personal access tokens > Fine-grained tokens
2. Click "Generate new token"
3. Configure token with minimal required permissions:
   - **Repository access**: Choose specific repositories
   - **Permissions**: Actions (write), Contents (write), Metadata (read)
   - **Expiration**: Set shortest practical lifetime

**Code implementation:**
```bash
# Clone with fine-grained PAT
git clone https://username:github_pat_11ABCDEFGHIJK123456789_abcdefghijklmnopqrstuvwxyz1234567890@github.company.com/org/repo.git

# Use with GitHub CLI
export GH_TOKEN=github_pat_11ABCDEFGHIJK123456789_abcdefghijklmnopqrstuvwxyz1234567890
gh repo list --limit 10

# Store in credential helper
echo "https://username:ghp_1234567890abcdefghijklmnopqrstuvwxyz12@github.company.com" >> ~/.git-credentials
```

### Service account authentication

**Service account setup process:**
```bash
# Generate SSH keys for service account
ssh-keygen -t ed25519 -C "service-account@company.com" -f ~/.ssh/id_service_account

# Configure local SSH config
cat >> ~/.ssh/config <<EOF
Host github.company.com
    HostName github.company.com
    User git
    IdentityFile ~/.ssh/id_service_account
    IdentitiesOnly yes
EOF

# Create long-lived PAT for service account with minimal scopes
# Store securely in CI/CD system or credential manager

# Example CI/CD usage
export GITHUB_TOKEN=${SERVICE_ACCOUNT_TOKEN}
git clone https://x-access-token:${GITHUB_TOKEN}@github.company.com/org/repo.git
```

**Service account best practices:**
- Use dedicated service accounts for automation
- Enable 2FA with TOTP stored in organizational password manager
- Grant minimum required repository and organization permissions
- Implement credential rotation procedures every 90 days
- Monitor service account activity through audit logs

### GitHub App authentication

**Workflow authentication example:**
```yaml
name: GitHub App Authentication
on: workflow_dispatch

jobs:
  app-auth:
    runs-on: ubuntu-latest
    steps:
      - name: Generate token
        id: generate-token
        uses: actions/create-github-app-token@v1
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_PRIVATE_KEY }}
          
      - name: Use app token
        run: |
          curl -H "Authorization: Bearer ${{ steps.generate-token.outputs.token }}" \
               https://github.company.com/api/v3/user
```

## 3. API Endpoints and Request Formats

### Core workflow dispatch endpoint

**GitHub Enterprise Server format:**
```
POST https://HOSTNAME/api/v3/repos/OWNER/REPO/actions/workflows/WORKFLOW_ID/dispatches
```

**Complete API request with input parameters:**
```bash
curl -L \
  -X POST \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer <YOUR-TOKEN>" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  https://your-enterprise-server.com/api/v3/repos/owner/repo/actions/workflows/workflow.yml/dispatches \
  -d '{
    "ref": "develop",
    "inputs": {
      "environment": "production",
      "version": "v1.2.3",
      "deploy_region": "us-west-2",
      "enable_logging": "true",
      "notification_email": "admin@company.com"
    }
  }'
```

### Python implementation for programmatic triggering

```python
import requests
import json

def trigger_workflow(hostname, owner, repo, workflow_id, token, branch, inputs=None):
    url = f"https://{hostname}/api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches"
    
    headers = {
        "Accept": "application/vnd.github+json",
        "Authorization": f"Bearer {token}",
        "X-GitHub-Api-Version": "2022-11-28"
    }
    
    payload = {"ref": branch}
    if inputs:
        payload["inputs"] = inputs
    
    response = requests.post(url, headers=headers, json=payload)
    
    if response.status_code == 204:
        print("Workflow triggered successfully")
    else:
        print(f"Error: {response.status_code} - {response.text}")
    
    return response

# Usage example
inputs = {
    "environment": "production",
    "version": "v2.1.0",
    "enable_notifications": "true"
}

response = trigger_workflow(
    hostname="github.company.com",
    owner="myorg", 
    repo="myrepo",
    workflow_id="deploy.yml",
    token="ghp_xxxxxxxxxxxxxxxxxxxx",
    branch="main",
    inputs=inputs
)
```

## 4. Input Parameter Handling and Validation

### Comprehensive input validation patterns

```yaml
inputs:
  # Version with pattern validation
  version:
    description: 'Version (semantic versioning: x.y.z or vx.y.z)'
    required: true
    type: string
    default: 'v1.0.0'
  
  # Environment with predefined choices
  environment:
    description: 'Target deployment environment'
    required: true
    type: choice
    options:
      - development
      - staging
      - production
    default: 'development'
  
  # Boolean with clear description
  force_overwrite:
    description: 'Force overwrite existing tag (dangerous)'
    required: false
    type: boolean
    default: false

jobs:
  validate:
    name: Validate Parameters
    runs-on: ubuntu-latest
    outputs:
      validated_version: ${{ steps.validate.outputs.version }}
      validated_branch: ${{ steps.validate.outputs.branch }}
    steps:
      - name: Comprehensive Input Validation
        id: validate
        run: |
          set -e
          
          # Version validation
          VERSION="${{ github.event.inputs.version }}"
          if [[ ! $VERSION =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+(\\.[a-zA-Z0-9]+)*)?(\\+[a-zA-Z0-9]+(\\.[a-zA-Z0-9]+)*)?$ ]]; then
            echo "❌ Invalid version format: $VERSION"
            echo "Expected: v1.2.3, 1.2.3, or v1.2.3-alpha.1+build.123"
            exit 1
          fi
          
          # Environment validation
          ENV="${{ github.event.inputs.environment }}"
          if [[ ! "$ENV" =~ ^(development|staging|production)$ ]]; then
            echo "❌ Invalid environment: $ENV"
            exit 1
          fi
          
          # Additional security checks for production
          if [[ "$ENV" == "production" && "${{ github.actor }}" != "admin-user" ]]; then
            echo "❌ Production releases require admin approval"
            exit 1
          fi
          
          # Set validated outputs
          echo "version=${VERSION#v}" >> $GITHUB_OUTPUT
          echo "environment=$ENV" >> $GITHUB_OUTPUT
          
          echo "✅ All parameters validated successfully"
```

## 5. Git Tagging Commands and Best Practices

### Standardized git tagging operations

```yaml
- name: Create Annotated Tag
  run: |
    # Always use annotated tags for releases
    git tag -a "$TAG_NAME" -m "Release $TAG_NAME
    
    Environment: ${{ github.event.inputs.environment }}
    Created: $(date -u +%Y-%m-%dT%H:%M:%SZ)
    Author: ${{ github.actor }}
    Workflow: ${{ github.workflow }} #${{ github.run_number }}
    Commit: $(git rev-parse HEAD)
    
    $(git log --oneline --max-count=5)"

- name: Push Tag with Verification
  run: |
    # Push tag and verify
    git push origin "$TAG_NAME"
    
    # Verify tag was pushed successfully
    if git ls-remote --tags origin | grep -q "refs/tags/$TAG_NAME"; then
      echo "✅ Tag $TAG_NAME pushed successfully"
    else
      echo "❌ Failed to push tag $TAG_NAME"
      exit 1
    fi
```

### Tag naming conventions and conflict handling

```yaml
- name: Generate Standard Tag Name
  id: tag_name
  run: |
    VERSION="${{ github.event.inputs.version }}"
    ENV="${{ github.event.inputs.environment }}"
    
    # Ensure version starts with 'v'
    if [[ ! $VERSION =~ ^v ]]; then
      VERSION="v$VERSION"
    fi
    
    # Add environment suffix for non-production
    if [[ "$ENV" != "production" ]]; then
      TAG_NAME="$VERSION-$ENV"
    else
      TAG_NAME="$VERSION"
    fi
    
    echo "tag_name=$TAG_NAME" >> $GITHUB_OUTPUT
    echo "Generated tag name: $TAG_NAME"
```

## 6. Service Account Setup and Permissions

### Enterprise managed users configuration

**Authentication setup for OIDC:**
- Configure GitHub Enterprise Managed User (OIDC) in Microsoft Entra ID
- Set up provisioning with SCIM 2.0
- Configure conditional access policies for enhanced security
- Default session lifetime: 1 hour (configurable)

**Permission models for enterprise:**
```yaml
# Organization-level roles
enterprise_owners: # Ultimate control
  - devops-admin@company.com
  - security-admin@company.com

billing_managers: # Billing only
  - finance-admin@company.com

members: # Default access
  - developers@company.com

# Repository-level permissions
repository_permissions:
  admin: # Full control
  maintain: # Manage without destructive actions  
  push: # Read/write access
  triage: # Manage issues/PRs
  pull: # Read-only access
```

## 7. Security Considerations and Secret Management

### GitHub Secrets management implementation

**Current enterprise features (2025):**
- Push protection blocks secrets in real-time with 75% precision rate
- AI-powered secret detection for unstructured secrets
- Organization-wide secret risk assessment
- Enhanced OIDC support for cloud provider authentication

**Restrictive permissions configuration:**
```yaml
# Recommended for new repositories
permissions:
  contents: read
  issues: write
  pull-requests: write
  actions: read
  # Explicitly define only required permissions
```

**OIDC authentication setup:**
```yaml
permissions:
  id-token: write
  contents: read

steps:
- name: Azure login
  uses: azure/login@v2
  with:
    client-id: ${{ secrets.AZURE_CLIENT_ID }}
    tenant-id: ${{ secrets.AZURE_TENANT_ID }}
    subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
```

### External secrets integration

```yaml
# AWS Secrets Manager Integration
- name: Configure AWS credentials
  uses: aws-actions/configure-aws-credentials@v4
  with:
    role-to-assume: arn:aws:iam::123456789012:role/GitHubActions
    aws-region: us-east-1
    role-session-name: GitHubActions

# Azure Key Vault Integration  
- name: Get secrets from Azure Key Vault
  uses: Azure/get-keyvault-secrets@v1
  with:
    keyvault: "myKeyVault"
    secrets: 'mySecret, mySecret2'
  id: myGetSecretAction
```

## 8. Error Handling and Logging

### Comprehensive error handling patterns

```yaml
name: Production Deployment
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    continue-on-error: false # Fail fast for critical jobs
    steps:
    - name: Test deployment
      id: test
      run: |
        # Test commands here
        if [[ $? -ne 0 ]]; then
          echo "::error::Deployment test failed"
          exit 1
        fi
      continue-on-error: true
      
    - name: Handle test failure
      if: steps.test.outcome == 'failure'
      run: |
        echo "::warning::Tests failed, initiating rollback"
        # Rollback logic here
        
    - name: Deploy to production
      if: steps.test.outcome == 'success'
      run: |
        # Deployment commands

    - name: Notify on failure
      if: failure()
      uses: actions/github-script@v7
      with:
        script: |
          github.rest.issues.createComment({
            issue_number: context.issue.number,
            owner: context.repo.owner,
            repo: context.repo.repo,
            body: `🚨 Deployment failed at step: ${{ github.job }}\\n\\nError details: ${{ steps.test.outputs.error }}`
          })
```

### Advanced logging and debugging

```yaml
- name: Setup logging
  run: |
    # Enable debug logging
    echo "::debug::Starting deployment process"
    echo "::notice::Environment: ${{ github.ref_name }}"
    echo "::warning::This is a critical deployment"
    
    # Group related logs
    echo "::group::Dependency Installation"
    npm install
    echo "::endgroup::"
    
    # Mask sensitive data
    echo "::add-mask::${{ secrets.DATABASE_PASSWORD }}"

# Repository secrets/variables for enhanced logging
ACTIONS_RUNNER_DEBUG: true  # Runner diagnostic logs
ACTIONS_STEP_DEBUG: true    # Verbose step logs
```

## 9. Integration Patterns with AKS/Kubernetes

### Complete AKS deployment workflow

```yaml
name: Deploy to AKS with Git Tagging
on:
  push:
    tags: ['v*']
  workflow_dispatch:
    inputs:
      environment:
        description: 'Deployment environment'
        required: true
        default: 'staging'
        type: choice
        options:
        - staging
        - production

permissions:
  id-token: write
  contents: read

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    environment: ${{ github.event.inputs.environment || 'production' }}
    
    steps:
    - name: Checkout
      uses: actions/checkout@v4
    
    - name: Extract metadata
      id: meta
      uses: docker/metadata-action@v5
      with:
        images: ${{ secrets.ACR_REGISTRY }}/myapp
        tags: |
          type=ref,event=tag
          type=sha,prefix={{branch}}-
          type=raw,value=latest,enable={{is_default_branch}}
    
    - name: Azure Login
      uses: azure/login@v2
      with:
        client-id: ${{ secrets.AZURE_CLIENT_ID }}
        tenant-id: ${{ secrets.AZURE_TENANT_ID }}
        subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
    
    - name: Build and push to ACR
      run: |
        az acr login --name ${{ secrets.ACR_REGISTRY }}
        docker build -t ${{ steps.meta.outputs.tags }} .
        docker push ${{ steps.meta.outputs.tags }}
    
    - name: Set AKS context
      uses: azure/aks-set-context@v4
      with:
        resource-group: ${{ secrets.AKS_RESOURCE_GROUP }}
        cluster-name: ${{ secrets.AKS_CLUSTER_NAME }}
    
    - name: Setup kubectl
      uses: azure/setup-kubectl@v4
      with:
        version: 'v1.30.0'
    
    - name: Deploy application
      uses: Azure/k8s-deploy@v5
      with:
        action: deploy
        namespace: ${{ github.event.inputs.environment || 'production' }}
        manifests: |
          k8s/deployment.yaml
          k8s/service.yaml
          k8s/ingress.yaml
        images: |
          ${{ steps.meta.outputs.tags }}
        strategy: canary
        percentage: 25
```

### kubectl 1.30 compatibility and installation

**Version compatibility requirements:**
- kubectl supports ±1 minor version skew with kube-apiserver
- kubectl 1.30 compatible with AKS clusters running Kubernetes 1.29-1.31
- If using kubectl 1.30.5, use workaround for download availability issues

**Installation and usage:**
```yaml
- name: Setup kubectl
  uses: azure/setup-kubectl@v4
  with:
    version: 'v1.30.0'

- name: Cluster diagnostics
  run: |
    kubectl cluster-info
    kubectl get nodes -o wide
    kubectl get pods --all-namespaces

- name: Deploy with kubectl
  run: |
    kubectl apply -f k8s/
    kubectl rollout status deployment/myapp -n production
    kubectl get services -n production
```

### OIDC authentication for AKS

**Workload Identity Federation setup:**
```bash
# Create managed identity
az identity create --name github-actions-identity --resource-group myRG

# Get identity details
IDENTITY_CLIENT_ID=$(az identity show --name github-actions-identity --resource-group myRG --query clientId -o tsv)
IDENTITY_OBJECT_ID=$(az identity show --name github-actions-identity --resource-group myRG --query principalId -o tsv)

# Create federated credential
az identity federated-credential create \
  --name github-actions-fedcred \
  --identity-name github-actions-identity \
  --resource-group myRG \
  --issuer https://token.actions.githubusercontent.com \
  --subject repo:organization/repository:ref:refs/heads/main \
  --audience api://AzureADTokenExchange

# Grant permissions to AKS cluster
az role assignment create \
  --assignee $IDENTITY_OBJECT_ID \
  --role "Azure Kubernetes Service Cluster User Role" \
  --scope /subscriptions/$SUBSCRIPTION_ID/resourceGroups/$AKS_RG/providers/Microsoft.ContainerService/managedClusters/$AKS_NAME
```

## 10. Testing and Troubleshooting Approaches

### Comprehensive testing framework

**Local testing with 'act':**
```bash
# Install act for local testing
curl -s https://raw.githubusercontent.com/nektos/act/master/install.sh | bash

# Test workflow locally
act -j test-workflow --secret-file .secrets

# Test specific events
act push -e .github/workflows/test-event.json
```

**Automated validation workflow:**
```yaml
name: Tag Release Testing
on:
  push:
    tags: ['v*']

jobs:
  validate-tag:
    runs-on: ubuntu-latest
    steps:
    - name: Validate semantic versioning
      run: |
        TAG=${GITHUB_REF#refs/tags/}
        if [[ ! $TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
          echo "::error::Invalid tag format: $TAG"
          exit 1
        fi
        echo "Valid tag: $TAG"
        
    - name: Security scan on tagged release
      uses: github/super-linter@v4
      env:
        DEFAULT_BRANCH: main
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        VALIDATE_ALL_CODEBASE: true
```

### Advanced debugging techniques

**Interactive debugging with SSH:**
```yaml
- name: Debug with SSH
  if: failure()
  uses: mxschmitt/action-tmate@v3
  timeout-minutes: 15
  with:
    limit-access-to-actor: true
```

**Debug information collection:**
```yaml
- name: Debug Information
  if: always()
  run: |
    echo "Runner OS: ${{ runner.os }}"
    echo "Runner Architecture: ${{ runner.arch }}"
    echo "GitHub Event: ${{ github.event_name }}"
    echo "Workflow: ${{ github.workflow }}"
    echo "Job: ${{ github.job }}"
    echo "Run ID: ${{ github.run_id }}"
    echo "Actor: ${{ github.actor }}"
    
    # Environment inspection
    env | sort
    
    # File system inspection  
    ls -la
    pwd
```

## Implementation checklist and best practices

**Security priorities:**
- ✅ Enable push protection for secrets across all repositories
- ✅ Use fine-grained PATs with minimal required permissions
- ✅ Implement OIDC for cloud authentication instead of long-lived secrets
- ✅ Configure restrictive GITHUB_TOKEN permissions as default
- ✅ Pin actions to specific commit SHAs for security

**Operational excellence:**
- ✅ Implement comprehensive input validation for all workflow parameters
- ✅ Use annotated tags with detailed metadata for enterprise tracking
- ✅ Configure environment protection rules for production deployments
- ✅ Implement automated rollback procedures for failed deployments
- ✅ Monitor workflow success rates and performance metrics

**Enterprise integration:**
- ✅ Configure service accounts with appropriate RBAC permissions
- ✅ Integrate with Microsoft Entra ID for centralized authentication
- ✅ Implement conditional access policies for enhanced security
- ✅ Use self-hosted runners for sensitive workloads and compliance requirements
- ✅ Enable comprehensive audit logging for security and compliance

This guide provides production-ready configurations that can be implemented directly in GitHub Enterprise environments. Regular reviews should be conducted quarterly to align with GitHub's feature development and emerging security best practices.