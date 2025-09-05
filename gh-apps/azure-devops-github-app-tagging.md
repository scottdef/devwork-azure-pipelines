# Azure DevOps Pipeline: GitHub App Authentication for Git Tagging

This guide shows how to configure Azure DevOps pipelines to use GitHub Apps for creating git tags on GitHub repositories. This approach provides better security and attribution compared to Personal Access Tokens.

## Prerequisites Setup

### 1. GitHub App Configuration

Ensure your GitHub App has these permissions:
- **Contents**: Read & Write (required for tagging)
- **Metadata**: Read (always required)
- **Actions**: Write (if triggering workflows)

### 2. Azure DevOps Variable Groups

Create a variable group to store GitHub App credentials securely:

1. Go to **Azure DevOps** → **Pipelines** → **Library** → **Variable groups**
2. Create new variable group: `github-app-secrets`
3. Add these variables:
   - `GITHUB_APP_ID`: Your GitHub App ID (not secret)
   - `GITHUB_INSTALLATION_ID`: Installation ID (not secret)
   - `GITHUB_APP_PRIVATE_KEY`: Private key content (mark as secret)

### 3. Service Connection (Alternative)

Alternatively, create a GitHub service connection:
1. **Project Settings** → **Service connections** → **New service connection**
2. Choose **GitHub** → **GitHub App**
3. Configure with your GitHub App details

## Azure DevOps Pipeline Implementation

### Complete Pipeline YAML

```yaml
# azure-pipelines-github-tagging.yml
trigger:
  branches:
    include:
    - main
    - release/*

variables:
- group: github-app-secrets
- name: GITHUB_REPO_OWNER
  value: 'your-org'
- name: GITHUB_REPO_NAME
  value: 'your-repo'
- name: PYTHON_VERSION
  value: '3.9'

pool:
  vmImage: 'ubuntu-latest'

stages:
- stage: Build
  displayName: 'Build and Test'
  jobs:
  - job: BuildJob
    displayName: 'Build Application'
    steps:
    - checkout: self
      persistCredentials: true
      fetchDepth: 0  # Full history for proper tagging
    
    - task: UsePythonVersion@0
      inputs:
        versionSpec: '$(PYTHON_VERSION)'
      displayName: 'Setup Python'
    
    - script: |
        python -m pip install --upgrade pip
        pip install PyJWT cryptography requests
      displayName: 'Install Dependencies'
    
    # Your build steps here
    - script: |
        echo "Building application..."
        # Add your build commands
      displayName: 'Build Application'
    
    - script: |
        echo "Running tests..."
        # Add your test commands
      displayName: 'Run Tests'

- stage: Tag
  displayName: 'Create Git Tag'
  dependsOn: Build
  condition: and(succeeded(), or(eq(variables['Build.SourceBranch'], 'refs/heads/main'), startsWith(variables['Build.SourceBranch'], 'refs/heads/release/')))
  jobs:
  - job: CreateTag
    displayName: 'Create Git Tag'
    steps:
    - checkout: none  # We'll clone manually with GitHub App auth
    
    - task: UsePythonVersion@0
      inputs:
        versionSpec: '$(PYTHON_VERSION)'
      displayName: 'Setup Python'
    
    - script: |
        python -m pip install --upgrade pip
        pip install PyJWT cryptography requests
      displayName: 'Install Python Dependencies'
    
    - task: PythonScript@0
      name: GitHubAppAuth
      displayName: 'Authenticate with GitHub App'
      inputs:
        scriptSource: 'inline'
        script: |
          import jwt
          import time
          import requests
          import os
          import sys
          
          def generate_jwt(app_id, private_key):
              payload = {
                  'iat': int(time.time()) - 60,
                  'exp': int(time.time()) + 600,
                  'iss': app_id
              }
              return jwt.encode(payload, private_key, algorithm='RS256')
          
          def get_installation_token(jwt_token, installation_id):
              url = f"https://api.github.com/app/installations/{installation_id}/access_tokens"
              headers = {
                  'Authorization': f'Bearer {jwt_token}',
                  'Accept': 'application/vnd.github+json',
                  'X-GitHub-Api-Version': '2022-11-28'
              }
              
              response = requests.post(url, headers=headers)
              if response.status_code == 201:
                  return response.json()['token']
              else:
                  print(f"Failed to get access token: {response.status_code}")
                  print(response.text)
                  sys.exit(1)
          
          # Get configuration
          app_id = os.environ['GITHUB_APP_ID']
          installation_id = os.environ['GITHUB_INSTALLATION_ID']
          private_key = os.environ['GITHUB_APP_PRIVATE_KEY']
          
          print("Generating JWT token...")
          jwt_token = generate_jwt(app_id, private_key)
          
          print("Getting installation access token...")
          access_token = get_installation_token(jwt_token, installation_id)
          
          # Set output variable for use in subsequent steps
          print(f"##vso[task.setvariable variable=GITHUB_ACCESS_TOKEN;isOutput=true;issecret=true]{access_token}")
          print("✅ GitHub App authentication successful")
    
    - script: |
        # Configure git with GitHub App token
        ACCESS_TOKEN="$(GitHubAppAuth.GITHUB_ACCESS_TOKEN)"
        
        git config --global user.name "Azure DevOps Pipeline"
        git config --global user.email "azure-devops@$(GITHUB_REPO_OWNER).com"
        git config --global credential.helper store
        
        # Create git credentials file
        echo "https://x-access-token:${ACCESS_TOKEN}@github.com" > ~/.git-credentials
        
        echo "✅ Git authentication configured"
      displayName: 'Configure Git Authentication'
      env:
        GITHUB_ACCESS_TOKEN: $(GitHubAppAuth.GITHUB_ACCESS_TOKEN)
    
    - script: |
        # Clone repository with authentication
        ACCESS_TOKEN="$(GitHubAppAuth.GITHUB_ACCESS_TOKEN)"
        REPO_URL="https://x-access-token:${ACCESS_TOKEN}@github.com/$(GITHUB_REPO_OWNER)/$(GITHUB_REPO_NAME).git"
        
        echo "Cloning repository..."
        git clone "$REPO_URL" repo
        cd repo
        
        # Get current commit SHA
        COMMIT_SHA=$(git rev-parse HEAD)
        echo "Current commit: $COMMIT_SHA"
        
        # Generate version tag
        if [[ "$(Build.SourceBranch)" == "refs/heads/main" ]]; then
            # For main branch, create release tag
            TIMESTAMP=$(date +%Y%m%d-%H%M%S)
            TAG_NAME="v1.0.${TIMESTAMP}"
            TAG_MESSAGE="Release build from main branch"
        elif [[ "$(Build.SourceBranch)" == refs/heads/release/* ]]; then
            # For release branches, use branch name
            BRANCH_NAME=$(echo "$(Build.SourceBranch)" | sed 's/refs\/heads\/release\///')
            TIMESTAMP=$(date +%Y%m%d-%H%M%S)
            TAG_NAME="release-${BRANCH_NAME}-${TIMESTAMP}"
            TAG_MESSAGE="Release candidate from branch: release/$BRANCH_NAME"
        else
            # Fallback for other branches
            BRANCH_NAME=$(echo "$(Build.SourceBranch)" | sed 's/refs\/heads\///')
            TIMESTAMP=$(date +%Y%m%d-%H%M%S)
            TAG_NAME="build-${BRANCH_NAME}-${TIMESTAMP}"
            TAG_MESSAGE="Build from branch: $BRANCH_NAME"
        fi
        
        echo "Creating tag: $TAG_NAME"
        echo "Tag message: $TAG_MESSAGE"
        
        # Create annotated tag
        git tag -a "$TAG_NAME" -m "$TAG_MESSAGE"
        
        # Push tag to remote
        git push origin "$TAG_NAME"
        
        echo "✅ Tag $TAG_NAME created and pushed successfully"
        
        # Set output variables
        echo "##vso[task.setvariable variable=TAG_NAME;isOutput=true]$TAG_NAME"
        echo "##vso[task.setvariable variable=COMMIT_SHA;isOutput=true]$COMMIT_SHA"
      name: CreateGitTag
      displayName: 'Create and Push Git Tag'
      env:
        GITHUB_ACCESS_TOKEN: $(GitHubAppAuth.GITHUB_ACCESS_TOKEN)
    
    - script: |
        echo "✅ Git tag created successfully!"
        echo "Tag Name: $(CreateGitTag.TAG_NAME)"
        echo "Commit SHA: $(CreateGitTag.COMMIT_SHA)"
        echo "Repository: $(GITHUB_REPO_OWNER)/$(GITHUB_REPO_NAME)"
      displayName: 'Display Results'

- stage: Notify
  displayName: 'Notification'
  dependsOn: Tag
  condition: always()
  jobs:
  - job: NotifyResults
    displayName: 'Notify Tag Results'
    variables:
      TAG_NAME: $[ stageDependencies.Tag.CreateTag.outputs['CreateGitTag.TAG_NAME'] ]
    steps:
    - script: |
        if [ -n "$(TAG_NAME)" ]; then
            echo "🎉 Successfully created tag: $(TAG_NAME)"
            echo "View at: https://github.com/$(GITHUB_REPO_OWNER)/$(GITHUB_REPO_NAME)/tags"
        else
            echo "❌ Tag creation failed or was skipped"
        fi
      displayName: 'Show Final Status'
```

### Alternative: Bash Script Approach

For more complex tagging logic, use external bash scripts:

```yaml
# Simplified pipeline using external script
- stage: Tag
  jobs:
  - job: CreateTag
    steps:
    - checkout: self
      persistCredentials: false
    
    - task: DownloadSecureFile@1
      name: GitHubAppKey
      displayName: 'Download GitHub App Private Key'
      inputs:
        secureFile: 'github-app-private-key.pem'
    
    - script: |
        chmod +x scripts/create-tag.sh
        ./scripts/create-tag.sh
      displayName: 'Create Git Tag'
      env:
        GITHUB_APP_ID: $(GITHUB_APP_ID)
        GITHUB_INSTALLATION_ID: $(GITHUB_INSTALLATION_ID)
        GITHUB_APP_PRIVATE_KEY_PATH: $(GitHubAppKey.secureFilePath)
        GITHUB_REPO_OWNER: $(GITHUB_REPO_OWNER)
        GITHUB_REPO_NAME: $(GITHUB_REPO_NAME)
        BUILD_SOURCE_BRANCH: $(Build.SourceBranch)
        BUILD_NUMBER: $(Build.BuildNumber)
```

### External Bash Script (scripts/create-tag.sh)

```bash
#!/bin/bash
set -euo pipefail

# Configuration
GITHUB_API_URL="https://api.github.com"
REPO_URL="https://github.com/${GITHUB_REPO_OWNER}/${GITHUB_REPO_NAME}.git"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

# Generate JWT token
generate_jwt() {
    local app_id="$1"
    local private_key_path="$2"
    
    python3 -c "
import jwt
import time

with open('$private_key_path', 'r') as pem_file:
    private_key = pem_file.read()

payload = {
    'iat': int(time.time()) - 60,
    'exp': int(time.time()) + 600,
    'iss': '$app_id'
}

token = jwt.encode(payload, private_key, algorithm='RS256')
print(token)
"
}

# Get installation access token
get_installation_token() {
    local jwt_token="$1"
    local installation_id="$2"
    
    curl -s -X POST \
        -H "Authorization: Bearer $jwt_token" \
        -H "Accept: application/vnd.github+json" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        "$GITHUB_API_URL/app/installations/$installation_id/access_tokens" \
        | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    print(data['token'])
except:
    sys.exit(1)
"
}

# Determine tag name based on branch
determine_tag_name() {
    local branch="$BUILD_SOURCE_BRANCH"
    local build_number="$BUILD_NUMBER"
    local timestamp=$(date +%Y%m%d-%H%M%S)
    
    case "$branch" in
        "refs/heads/main")
            echo "v1.0.${build_number}"
            ;;
        "refs/heads/release/"*)
            local release_name=$(echo "$branch" | sed 's/refs\/heads\/release\///')
            echo "release-${release_name}-${build_number}"
            ;;
        *)
            local branch_name=$(echo "$branch" | sed 's/refs\/heads\///')
            echo "build-${branch_name}-${build_number}"
            ;;
    esac
}

# Main execution
main() {
    log "🚀 Starting GitHub App git tagging process"
    
    # Validate required environment variables
    for var in GITHUB_APP_ID GITHUB_INSTALLATION_ID GITHUB_APP_PRIVATE_KEY_PATH GITHUB_REPO_OWNER GITHUB_REPO_NAME; do
        if [ -z "${!var:-}" ]; then
            log "❌ Error: $var environment variable is not set"
            exit 1
        fi
    done
    
    # Install Python dependencies
    log "📦 Installing Python dependencies"
    pip install PyJWT cryptography
    
    # Generate JWT token
    log "🔑 Generating JWT token"
    local jwt_token=$(generate_jwt "$GITHUB_APP_ID" "$GITHUB_APP_PRIVATE_KEY_PATH")
    
    if [ -z "$jwt_token" ]; then
        log "❌ Failed to generate JWT token"
        exit 1
    fi
    
    # Get installation access token
    log "🎫 Getting installation access token"
    local access_token=$(get_installation_token "$jwt_token" "$GITHUB_INSTALLATION_ID")
    
    if [ -z "$access_token" ]; then
        log "❌ Failed to get installation access token"
        exit 1
    fi
    
    # Configure git
    log "⚙️ Configuring git authentication"
    git config --global user.name "Azure DevOps Pipeline"
    git config --global user.email "azure-devops@${GITHUB_REPO_OWNER}.com"
    git config --global credential.helper store
    echo "https://x-access-token:${access_token}@github.com" > ~/.git-credentials
    
    # Clone repository
    log "📥 Cloning repository"
    git clone "$REPO_URL" temp-repo
    cd temp-repo
    
    # Determine tag name
    local tag_name=$(determine_tag_name)
    log "🏷️ Creating tag: $tag_name"
    
    # Create tag message
    local tag_message="Automated tag created by Azure DevOps Pipeline
    
Build: $BUILD_NUMBER
Branch: $BUILD_SOURCE_BRANCH
Commit: $(git rev-parse HEAD)
Created: $(date -u '+%Y-%m-%d %H:%M:%S UTC')
Pipeline: Azure DevOps"
    
    # Create and push tag
    git tag -a "$tag_name" -m "$tag_message"
    git push origin "$tag_name"
    
    log "✅ Successfully created and pushed tag: $tag_name"
    
    # Set Azure DevOps output variables
    echo "##vso[task.setvariable variable=TAG_NAME;isOutput=true]$tag_name"
    echo "##vso[task.setvariable variable=REPO_URL;isOutput=true]https://github.com/${GITHUB_REPO_OWNER}/${GITHUB_REPO_NAME}/releases/tag/$tag_name"
    
    # Cleanup
    cd ..
    rm -rf temp-repo
    rm -f ~/.git-credentials
    
    log "🎉 Git tagging process completed successfully"
}

# Execute main function
main "$@"
```

## Advanced Configuration Options

### 1. Semantic Versioning Support

```yaml
- script: |
    # Install semantic versioning tools
    npm install -g semantic-release @semantic-release/changelog @semantic-release/git
    
    # Create semantic release configuration
    cat > .releaserc.json << 'EOF'
    {
      "branches": ["main"],
      "plugins": [
        "@semantic-release/commit-analyzer",
        "@semantic-release/release-notes-generator",
        "@semantic-release/changelog",
        "@semantic-release/git"
      ]
    }
    EOF
    
    # Run semantic release (dry-run to get version)
    npx semantic-release --dry-run --no-ci
  displayName: 'Determine Semantic Version'
```

### 2. Conditional Tagging with Validation

```yaml
- script: |
    # Only tag if tests passed and specific conditions are met
    if [ "$(Agent.JobStatus)" = "Succeeded" ] && [ "$BUILD_REASON" = "Manual" ]; then
        echo "Conditions met for tagging"
        echo "##vso[task.setvariable variable=SHOULD_TAG]true"
    else
        echo "Conditions not met for tagging"
        echo "##vso[task.setvariable variable=SHOULD_TAG]false"
    fi
  displayName: 'Check Tagging Conditions'

- script: |
    # Your tagging script here
  displayName: 'Create Git Tag'
  condition: eq(variables['SHOULD_TAG'], 'true')
```

### 3. Multi-Repository Tagging

```yaml
- script: |
    # Tag multiple related repositories
    REPOSITORIES=("repo1" "repo2" "repo3")
    
    for repo in "${REPOSITORIES[@]}"; do
        echo "Tagging repository: $repo"
        
        # Clone and tag each repository
        git clone "https://x-access-token:${ACCESS_TOKEN}@github.com/${GITHUB_REPO_OWNER}/${repo}.git" "$repo"
        cd "$repo"
        
        git tag -a "sync-${BUILD_NUMBER}" -m "Synchronized release ${BUILD_NUMBER}"
        git push origin "sync-${BUILD_NUMBER}"
        
        cd ..
        rm -rf "$repo"
    done
  displayName: 'Tag Multiple Repositories'
  env:
    ACCESS_TOKEN: $(GitHubAppAuth.GITHUB_ACCESS_TOKEN)
```

## Security Best Practices

### 1. Secure Variable Management

```yaml
# Use Azure Key Vault for sensitive data
- task: AzureKeyVault@2
  inputs:
    azureSubscription: 'your-service-connection'
    KeyVaultName: 'your-keyvault'
    SecretsFilter: 'github-app-private-key'
    RunAsPreJob: true
```

### 2. Credential Cleanup

```yaml
- script: |
    # Always clean up credentials
    rm -f ~/.git-credentials
    git config --global --unset credential.helper || true
  displayName: 'Cleanup Credentials'
  condition: always()
```

### 3. Audit Logging

```yaml
- script: |
    # Log tagging activity for audit
    echo "Tag created: $TAG_NAME" | logger -t azure-devops-tagging
    
    # Optional: Send to external logging system
    curl -X POST "https://your-logging-endpoint.com/audit" \
         -H "Content-Type: application/json" \
         -d "{\"action\":\"tag_created\",\"tag\":\"$TAG_NAME\",\"repo\":\"$GITHUB_REPO_OWNER/$GITHUB_REPO_NAME\",\"pipeline\":\"$BUILD_DEFINITIONNAME\"}"
  displayName: 'Audit Log'
  condition: succeeded()
```

This implementation provides a secure, scalable approach for creating git tags on GitHub repositories from Azure DevOps pipelines using GitHub Apps. The GitHub App approach offers better security, audit trails, and rate limits compared to Personal Access Tokens.