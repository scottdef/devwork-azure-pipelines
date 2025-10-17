# GoogleCloudPlatform/Terraformer: Comprehensive CLI Guide

Terraformer reverse-engineers existing cloud infrastructure into Terraform code, generating both configuration files and state files from live resources. This guide provides complete coverage of terraformer usage, with detailed focus on GitHub Actions automation workflows for continuous infrastructure import and drift detection.

## Table of Contents

1. [Overview and Core Concepts](#overview-and-core-concepts)
2. [Command Reference Cheatsheet](#command-reference-cheatsheet)
3. [Installation and Setup Guide](#installation-and-setup-guide)
4. [Docker Containerization](#docker-containerization)
5. [GitHub Provider Specifics](#github-provider-specifics)
6. [GitHub Actions Workflows](#github-actions-workflows)
7. [Terraform State Management and Drift Detection](#terraform-state-management-and-drift-detection)
8. [Common Usage Patterns](#common-usage-patterns)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

---

## Overview and Core Concepts

### What Terraformer does

Terraformer solves the "infrastructure to code" challenge by **automatically generating Terraform configurations from existing cloud resources**. Unlike traditional `terraform import` which only creates state entries, terraformer generates both `.tf` configuration files and `.tfstate` files from live infrastructure.

**Core workflow**: Terraformer authenticates with cloud provider APIs, retrieves resource information, calls Terraform providers to refresh complete state, then generates HCL/JSON configuration files with corresponding state files. The tool uses actual Terraform providers rather than custom templating, ensuring accuracy and automatic support for new provider features.

### Supported providers

Terraformer supports **40+ providers** including AWS, GCP, Azure, GitHub, GitLab, Kubernetes, Datadog, PagerDuty, Okta, and more. Each provider requires appropriate authentication and read-only permissions.

### Key advantages

**Direct provider integration** ensures generated code matches current Terraform provider schemas. **Automatic relationship mapping** creates terraform_remote_state connections between resources. **Flexible filtering** enables selective imports by resource type, ID, tags, or attributes. **Read-only operation** requires no write permissions to infrastructure.

---

## Command Reference Cheatsheet

### Essential commands

```bash
# List available resources for a provider
terraformer import [provider] list

# Import specific resources
terraformer import aws --resources=vpc,subnet --regions=us-east-1

# Import all resources
terraformer import azure --resources=*

# Import with filtering by ID
terraformer import github --owner=myorg --resources=repositories --filter=repository=repo1:repo2:repo3

# Import with attribute filtering
terraformer import aws --resources=ec2_instance --filter="Name=tags.Environment;Value=Production"

# Generate plan before importing
terraformer plan gcp --resources=networks,firewall --projects=my-project

# Execute import from plan
terraformer import plan generated/google/my-project/terraformer/plan.json
```

### Common flags

| Flag | Description | Example |
|------|-------------|---------|
| `--resources` | Resource types to import (comma-separated or *) | `--resources=vpc,subnet` |
| `--regions` | Cloud regions to scan | `--regions=us-east-1,us-west-2` |
| `--filter` | Filter by ID or attributes | `--filter=vpc=vpc-12345` |
| `--excludes` | Exclude specific resources | `--excludes=firewalls,networks` |
| `--path-output` | Output directory | `--path-output=./generated` |
| `--path-pattern` | Folder structure pattern | `--path-pattern="{output}/{provider}/"` |
| `--compact` | Combine resources in single file | `--compact` |
| `--output` | Format (hcl or json) | `--output=json` |
| `-v, --verbose` | Verbose output | `-v` |

### Provider-specific examples

```bash
# AWS with profile and filtering
terraformer import aws --profile=production \
  --resources=vpc,subnet,sg \
  --filter Type=sg;Name=vpc_id;Value=vpc-12345 \
  --regions=us-east-1

# GCP with multiple projects
terraformer import google \
  --resources=networks,firewall,compute \
  --projects=project-1,project-2 \
  --regions=us-central1

# GitHub organization export
terraformer import github \
  --owner=myorg \
  --resources=repositories,teams,members \
  --token=$GITHUB_TOKEN

# GitHub Enterprise
terraformer import github \
  --owner=enterprise-org \
  --base-url=https://github.enterprise.com \
  --resources=repositories

# Azure with compact output
terraformer import azure \
  --resources=resource_group,network \
  --compact
```

---

## Installation and Setup Guide

### Prerequisites

**1. Terraform CLI** (version 0.13+ required)
```bash
# Download from https://www.terraform.io/downloads
# Verify installation
terraform version
```

**2. Provider plugin initialization**

Create a working directory and initialize the Terraform provider:

```hcl
# versions.tf
terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 5.0"
    }
  }
  required_version = ">= 0.13"
}
```

```bash
terraform init
```

### Installing Terraformer

**Package managers** (recommended):

```bash
# Homebrew (macOS/Linux)
brew install terraformer

# MacPorts (macOS)
sudo port install terraformer

# Chocolatey (Windows)
choco install terraformer
```

**Binary downloads**:

```bash
# Linux
export PROVIDER=all  # Options: all, google, aws, azure, github, kubernetes
curl -LO "https://github.com/GoogleCloudPlatform/terraformer/releases/download/$(curl -s https://api.github.com/repos/GoogleCloudPlatform/terraformer/releases/latest | grep tag_name | cut -d '"' -f 4)/terraformer-${PROVIDER}-linux-amd64"
chmod +x terraformer-${PROVIDER}-linux-amd64
sudo mv terraformer-${PROVIDER}-linux-amd64 /usr/local/bin/terraformer

# macOS
export PROVIDER=all
curl -LO "https://github.com/GoogleCloudPlatform/terraformer/releases/download/$(curl -s https://api.github.com/repos/GoogleCloudPlatform/terraformer/releases/latest | grep tag_name | cut -d '"' -f 4)/terraformer-${PROVIDER}-darwin-amd64"
chmod +x terraformer-${PROVIDER}-darwin-amd64
sudo mv terraformer-${PROVIDER}-darwin-amd64 /usr/local/bin/terraformer
```

**Build from source**:

```bash
git clone https://github.com/GoogleCloudPlatform/terraformer.git
cd terraformer/
go mod download
go build -v  # Builds for all providers
# Or build specific provider: go run build/main.go github
```

### Authentication configuration

**GitHub** (focus of automation examples):
```bash
# Environment variable (recommended)
export GITHUB_TOKEN=ghp_your_personal_access_token

# Or pass via command line
terraformer import github --token=ghp_xxx --owner=myorg --resources=repositories
```

**Required GitHub token scopes**:
- `repo` - Full control of repositories
- `read:org` - Read organization data
- `admin:org` - Full organization control (for complete exports)
- `admin:repo_hook` - Repository webhooks

**AWS**:
```bash
# Use AWS credentials file or environment variables
export AWS_PROFILE=production
export AWS_REGION=us-east-1
```

**Azure**:
```bash
# Azure CLI authentication
az login

# Or environment variables
export ARM_CLIENT_ID="xxx"
export ARM_CLIENT_SECRET="xxx"
export ARM_SUBSCRIPTION_ID="xxx"
export ARM_TENANT_ID="xxx"
```

**GCP**:
```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

---

## Docker Containerization

### Building custom terraformer images with helper tools

The following Dockerfile creates a production-ready image with terraformer, Terraform CLI, and **all required helper tools** (jq, bash, curl, GitHub CLI, Azure CLI, diff) optimized for CI/CD workflows.

```dockerfile
# syntax=docker/dockerfile:1

##################################################
# Build Stage - Download binaries
##################################################
FROM alpine:3.21 AS builder

ARG TERRAFORM_VERSION=1.7.2
ARG TERRAFORMER_VERSION=0.8.30
ARG TERRAFORMER_PROVIDER=all

WORKDIR /tmp/downloads

RUN apk add --no-cache curl wget unzip ca-certificates

# Download Terraform
RUN wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    chmod +x terraform

# Download Terraformer
RUN wget https://github.com/GoogleCloudPlatform/terraformer/releases/download/${TERRAFORMER_VERSION}/terraformer-${TERRAFORMER_PROVIDER}-linux-amd64 && \
    chmod +x terraformer-${TERRAFORMER_PROVIDER}-linux-amd64

##################################################
# Runtime Stage - Minimal production image
##################################################
FROM alpine:3.21

LABEL maintainer="Your Team"
LABEL description="Terraformer with jq, bash, curl, GitHub CLI, Azure CLI, diff"

# Install core utilities and GitHub CLI
RUN apk add --no-cache \
    bash \
    curl \
    jq \
    diffutils \
    git \
    openssh-client \
    ca-certificates \
    py3-pip \
    python3 \
    github-cli --repository=https://dl-cdn.alpinelinux.org/alpine/edge/community && \
    rm -rf /var/cache/apk/*

# Install Azure CLI
RUN apk add --no-cache --virtual=build-deps \
    gcc musl-dev python3-dev libffi-dev openssl-dev cargo make && \
    pip3 install --no-cache-dir --prefer-binary azure-cli && \
    apk del build-deps && \
    rm -rf /var/cache/apk/*

# Copy binaries from builder
COPY --from=builder /tmp/downloads/terraform /usr/local/bin/terraform
COPY --from=builder /tmp/downloads/terraformer-*-linux-amd64 /usr/local/bin/terraformer

WORKDIR /workspace

RUN mkdir -p /root/.terraform.d/plugins && \
    mkdir -p /workspace/generated

# Environment variables for automation
ENV TF_IN_AUTOMATION=true
ENV TF_INPUT=false
ENV TF_CLI_ARGS="-no-color"

# Verify installations
RUN terraform version && \
    terraformer version || terraformer --help && \
    jq --version && \
    bash --version && \
    curl --version && \
    gh --version && \
    az version && \
    diff --version

CMD ["/bin/bash"]
```

### Building and optimizing the image

**Build with arguments**:
```bash
docker build \
  --build-arg TERRAFORM_VERSION=1.7.2 \
  --build-arg TERRAFORMER_VERSION=0.8.30 \
  --build-arg TERRAFORMER_PROVIDER=all \
  -t terraformer:latest \
  .
```

**Multi-stage optimization** reduces image size from ~1.2GB to **200-250MB** by separating build dependencies from runtime components.

### Pushing to Azure Container Registry

**Setup and authentication**:
```bash
# Create ACR
az acr create \
  --resource-group myResourceGroup \
  --name myacrregistry \
  --sku Standard

# Login to ACR
az acr login --name myacrregistry

# Get login server
ACR_LOGIN_SERVER=$(az acr show --name myacrregistry --query loginServer --output tsv)
echo $ACR_LOGIN_SERVER  # myacrregistry.azurecr.io
```

**Tag and push**:
```bash
# Tag image with ACR registry
docker tag terraformer:latest ${ACR_LOGIN_SERVER}/terraformer:latest
docker tag terraformer:latest ${ACR_LOGIN_SERVER}/terraformer:1.0
docker tag terraformer:latest ${ACR_LOGIN_SERVER}/terraformer:github-v1

# Push to ACR
docker push ${ACR_LOGIN_SERVER}/terraformer:latest
docker push ${ACR_LOGIN_SERVER}/terraformer:1.0

# Verify
az acr repository list --name myacrregistry --output table
az acr repository show-tags --name myacrregistry --repository terraformer --output table
```

**Service principal authentication** (for CI/CD):
```bash
# Create service principal
az ad sp create-for-rbac \
  --name myServicePrincipal \
  --role acrpush \
  --scopes /subscriptions/SUBSCRIPTION_ID/resourceGroups/myResourceGroup/providers/Microsoft.ContainerRegistry/registries/myacrregistry

# Login with service principal
docker login myacrregistry.azurecr.io \
  --username APP_ID \
  --password PASSWORD
```

### Running the container

**Basic execution**:
```bash
# Interactive shell
docker run -it --rm terraformer:latest bash

# Run terraformer command directly
docker run --rm \
  -e GITHUB_TOKEN=$GITHUB_TOKEN \
  -v $(pwd):/workspace \
  myacrregistry.azurecr.io/terraformer:latest \
  terraformer import github --owner=myorg --resources=repositories
```

**With volume mounts and credentials**:
```bash
docker run -it --rm \
  -v $(pwd):/workspace \
  -v ~/.azure:/root/.azure:ro \
  -v ~/.config/gh:/root/.config/gh:ro \
  -e GITHUB_TOKEN=$GITHUB_TOKEN \
  -e ARM_CLIENT_ID=$ARM_CLIENT_ID \
  -e ARM_CLIENT_SECRET=$ARM_CLIENT_SECRET \
  -e ARM_SUBSCRIPTION_ID=$ARM_SUBSCRIPTION_ID \
  -e ARM_TENANT_ID=$ARM_TENANT_ID \
  -w /workspace \
  terraformer:latest \
  bash
```

**Docker Compose example**:
```yaml
version: '3.8'

services:
  terraformer:
    image: myacrregistry.azurecr.io/terraformer:latest
    working_dir: /workspace
    environment:
      - GITHUB_TOKEN
      - ARM_CLIENT_ID
      - ARM_CLIENT_SECRET
      - ARM_SUBSCRIPTION_ID
      - ARM_TENANT_ID
    volumes:
      - ./terraform:/workspace
      - ./generated:/workspace/generated
      - ~/.azure:/root/.azure:ro
      - ~/.config/gh:/root/.config/gh:ro
    command: bash
    stdin_open: true
    tty: true
```

---

## GitHub Provider Specifics

### Supported GitHub resources

Terraformer's GitHub provider supports **organizational resources only** (personal repositories not supported):

**Available resources**:
- `github_repository` - Repository configurations
- `github_branch_protection` - Branch protection rules
- `github_repository_collaborator` - Repository collaborators
- `github_repository_deploy_key` - Deploy keys
- `github_repository_webhook` - Repository webhooks
- `github_team` - Team definitions
- `github_team_membership` - Team memberships
- `github_membership` - Organization memberships
- `github_organization_project` - Organization projects
- `github_organization_webhook` - Organization webhooks
- `github_organization_block` - Blocked users

### Critical limitation: Unsupported newer GitHub features

**NOT SUPPORTED** (these require manual Terraform creation):
- ❌ `github_repository_ruleset` - Repository-level rulesets
- ❌ `github_organization_ruleset` - Organization-level rulesets
- ❌ `github_organization_settings` - Organization settings
- ❌ `github_organization_custom_role` - Custom organization roles

These newer GitHub features exist in the Terraform provider but have not been implemented in terraformer's import logic. For complete infrastructure management, manually create these resources in Terraform after initial import.

### GitHub authentication

**Personal Access Token (PAT)** - only documented authentication method:

```bash
# Environment variable (recommended)
export GITHUB_TOKEN=ghp_your_token_here
terraformer import github --owner=myorg --resources=repositories

# Or command-line flag
terraformer import github --owner=myorg --resources=repositories --token=ghp_xxx
```

**Required token permissions** for comprehensive organizational export:
- `repo` (full) - All repository access
- `admin:org` (full) - Organization administration
- `read:org` - Organization metadata
- `admin:repo_hook` - Repository webhooks

### GitHub Enterprise support

Terraformer added GitHub Enterprise support in **version 0.8.13**:

```bash
# GitHub Enterprise export
export GITHUB_TOKEN=ghp_enterprise_token
terraformer import github \
  --owner=enterprise-org \
  --base-url=https://github.enterprise.com \
  --resources=repositories,teams,members \
  --verbose
```

The `--base-url` flag specifies your GitHub Enterprise instance. Earlier versions had issues with environment variable configuration defaulting to public GitHub.

### Exporting all GitHub resources from organization

**Complete organizational export**:
```bash
export GITHUB_TOKEN=ghp_xxx

# Export all supported resources
terraformer import github \
  --owner=myorg \
  --resources=* \
  --path-output=./github-export \
  --compact \
  --verbose

# Or specify explicit list
terraformer import github \
  --owner=myorg \
  --resources=repositories,teams,members,organization_webhooks,organization_projects \
  --compact
```

**Selective repository export with filtering**:
```bash
# Import specific repositories only
terraformer import github \
  --owner=myorg \
  --resources=repositories \
  --filter=repository=frontend:backend:api-service:infrastructure
```

### Known GitHub provider limitations

**Webhook secrets cannot be retrieved** - The GitHub API doesn't return webhook secrets for security reasons. After import, `terraform plan` may show configuration differences. Use `lifecycle { ignore_changes = [configuration] }` blocks or manage secrets separately.

**Organization-only support** - Cannot import personal/user repositories. The `--owner` flag must specify an organization.

**Rate limiting** - GitHub API has rate limits (5000 requests/hour authenticated). For large organizations, use `--filter` to import in batches.

---

## GitHub Actions Workflows

This section provides detailed coverage of **all requested automation use cases** for running terraformer in GitHub Actions.

### Workflow permissions and authentication

**GitHub Actions automatic token** (GITHUB_TOKEN):

```yaml
name: GitHub Resource Export
on: [push]

permissions:
  contents: read      # Read repository content
  issues: write       # Create drift detection issues
  pull-requests: write

jobs:
  export:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Export GitHub resources
        run: |
          terraformer import github \
            --owner=${{ github.repository_owner }} \
            --resources=repositories
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**Personal Access Token** for extended permissions:

```yaml
jobs:
  export:
    runs-on: ubuntu-latest
    steps:
      - name: Export with PAT
        run: |
          terraformer import github \
            --owner=myorg \
            --resources=* \
            --compact
        env:
          GITHUB_TOKEN: ${{ secrets.PAT_TOKEN }}
```

Store PAT in **Settings → Secrets and variables → Actions → Repository secrets**.

**Required PAT scopes** for comprehensive export: `repo`, `admin:org`, `read:org`, `admin:repo_hook`.

**GitHub App authentication** (not directly supported by terraformer, but can generate tokens):

```yaml
- name: Generate token from GitHub App
  id: generate_token
  uses: actions/create-github-app-token@v1
  with:
    app-id: ${{ secrets.APP_ID }}
    private-key: ${{ secrets.APP_PRIVATE_KEY }}
    owner: ${{ github.repository_owner }}

- name: Export with App token
  run: terraformer import github --owner=myorg --resources=repositories
  env:
    GITHUB_TOKEN: ${{ steps.generate_token.outputs.token }}
```

### Exporting ALL GitHub resources for GitHub Enterprise

**Complete GitHub Enterprise organizational export workflow**:

```yaml
name: GitHub Enterprise - Complete Export

on:
  workflow_dispatch:
  schedule:
    - cron: '0 2 * * 0'  # Weekly Sunday 2 AM UTC

permissions:
  contents: write
  pull-requests: write

jobs:
  export-github-enterprise:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Terraform
        uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: 1.7.2
          
      - name: Create provider configuration
        run: |
          cat > versions.tf <<EOF
          terraform {
            required_providers {
              github = {
                source  = "integrations/github"
                version = "~> 5.0"
              }
            }
          }
          
          provider "github" {
            owner = "${{ vars.GITHUB_ORG }}"
            base_url = "${{ vars.GITHUB_ENTERPRISE_URL }}"
          }
          EOF
          
      - name: Initialize Terraform provider
        run: terraform init
        
      - name: Install Terraformer
        run: |
          export PROVIDER=github
          curl -LO "https://github.com/GoogleCloudPlatform/terraformer/releases/download/0.8.30/terraformer-${PROVIDER}-linux-amd64"
          chmod +x terraformer-${PROVIDER}-linux-amd64
          sudo mv terraformer-${PROVIDER}-linux-amd64 /usr/local/bin/terraformer
          
      - name: Export ALL GitHub Enterprise resources
        run: |
          terraformer import github \
            --owner=${{ vars.GITHUB_ORG }} \
            --base-url=${{ vars.GITHUB_ENTERPRISE_URL }} \
            --resources=* \
            --path-output=./github-enterprise-export \
            --compact \
            --verbose
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_ENTERPRISE_PAT }}
          
      - name: Format generated Terraform
        run: |
          cd github-enterprise-export
          terraform fmt -recursive
          
      - name: Create Pull Request with exports
        uses: peter-evans/create-pull-request@v5
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          commit-message: 'GitHub Enterprise export - ${{ github.run_number }}'
          title: 'GitHub Enterprise Infrastructure Export'
          body: |
            ## GitHub Enterprise Resource Export
            
            Automated export of all GitHub Enterprise resources for organization: ${{ vars.GITHUB_ORG }}
            
            **Exported resources:**
            - Repositories
            - Teams and memberships
            - Organization webhooks
            - Organization projects
            - Branch protections
            - Deploy keys
            
            **Note:** Unsupported resources require manual creation:
            - Organization rulesets
            - Repository rulesets
            - Organization settings
            - Custom roles
            
            Review and merge to update infrastructure state.
          branch: github-export-${{ github.run_number }}
          labels: infrastructure,automated
```

**Configuration variables** (Settings → Variables):
- `GITHUB_ORG` - Your GitHub Enterprise organization name
- `GITHUB_ENTERPRISE_URL` - Base URL (e.g., `https://github.enterprise.com`)

**Secrets**:
- `GITHUB_ENTERPRISE_PAT` - Personal access token with required scopes

### Building custom Docker image in GitHub Actions workflow

**Workflow to build Docker image with helper tools and push to ACR**:

```yaml
name: Build Terraformer Docker Image

on:
  push:
    branches: [main]
    paths:
      - 'Dockerfile'
      - '.github/workflows/build-docker.yml'
  workflow_dispatch:

permissions:
  id-token: write
  contents: read

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
        
      - name: Azure Login with OIDC
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
          
      - name: Login to Azure Container Registry
        run: az acr login --name ${{ vars.ACR_NAME }}
        
      - name: Build Docker image with all tools
        run: |
          docker build \
            --build-arg TERRAFORM_VERSION=1.7.2 \
            --build-arg TERRAFORMER_VERSION=0.8.30 \
            --build-arg TERRAFORMER_PROVIDER=all \
            --tag ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest \
            --tag ${{ vars.ACR_NAME }}.azurecr.io/terraformer:${{ github.run_number }} \
            --tag ${{ vars.ACR_NAME }}.azurecr.io/terraformer:${{ github.sha }} \
            .
            
      - name: Verify helper tools in image
        run: |
          docker run --rm ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest bash -c "
            echo 'Terraform:' && terraform version
            echo 'Terraformer:' && terraformer version || terraformer --help
            echo 'jq:' && jq --version
            echo 'bash:' && bash --version | head -1
            echo 'curl:' && curl --version | head -1
            echo 'GitHub CLI:' && gh --version
            echo 'Azure CLI:' && az version --output json | jq -r '.\"azure-cli\"'
            echo 'diff:' && diff --version | head -1
          "
          
      - name: Push images to ACR
        run: |
          docker push ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest
          docker push ${{ vars.ACR_NAME }}.azurecr.io/terraformer:${{ github.run_number }}
          docker push ${{ vars.ACR_NAME }}.azurecr.io/terraformer:${{ github.sha }}
          
      - name: List pushed images
        run: |
          az acr repository show-tags \
            --name ${{ vars.ACR_NAME }} \
            --repository terraformer \
            --output table \
            --orderby time_desc \
            --top 10
```

**Dockerfile** (as shown in Docker Containerization section) should include:
- ✅ jq
- ✅ bash
- ✅ curl
- ✅ GitHub CLI (gh)
- ✅ diff
- ✅ Azure CLI (az)
- ✅ Terraform CLI
- ✅ Terraformer CLI

### Running terraformer as service container

**Service container pattern** - Container runs alongside job:

```yaml
name: Terraformer Service Container

on:
  workflow_dispatch:
  schedule:
    - cron: '0 */6 * * *'  # Every 6 hours

jobs:
  import-with-service:
    runs-on: ubuntu-latest
    
    services:
      terraformer:
        image: myacrregistry.azurecr.io/terraformer:latest
        credentials:
          username: ${{ secrets.ACR_USERNAME }}
          password: ${{ secrets.ACR_PASSWORD }}
        volumes:
          - /workspace:/workspace
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          
    steps:
      - uses: actions/checkout@v4
        with:
          path: /workspace
          
      - name: Access service container
        run: |
          # Service containers are accessible via Docker network
          # Note: Service containers are designed for supporting services like databases
          # For terraformer, docker exec pattern is more appropriate
          echo "Service container running"
```

**Note**: Service containers are better suited for supporting services (databases, caches). For terraformer CLI operations, the **docker exec pattern** (next section) or **direct container execution** is more appropriate.

### Running terraformer with docker exec in GitHub Actions

**Docker exec pattern** - Start container, execute commands via docker exec:

```yaml
name: Terraformer with Docker Exec

on:
  workflow_dispatch:
  schedule:
    - cron: '0 */1 * * *'  # Hourly

permissions:
  contents: write
  issues: write

jobs:
  import-via-docker-exec:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Azure Login
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
          
      - name: Login to ACR
        run: az acr login --name ${{ vars.ACR_NAME }}
        
      - name: Pull terraformer image
        run: docker pull ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest
        
      - name: Start terraformer container
        run: |
          docker run -d \
            --name terraformer-container \
            --volume ${{ github.workspace }}:/workspace \
            --workdir /workspace \
            --env GITHUB_TOKEN=${{ secrets.GITHUB_TOKEN }} \
            ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest \
            tail -f /dev/null
            
      - name: Initialize Terraform provider via docker exec
        run: |
          docker exec terraformer-container bash -c '
            cat > versions.tf <<EOF
          terraform {
            required_providers {
              github = {
                source  = "integrations/github"
                version = "~> 5.0"
              }
            }
          }
          EOF
          '
          docker exec terraformer-container terraform init
          
      - name: Run terraformer import via docker exec
        run: |
          docker exec terraformer-container bash -c "
            terraformer import github \
              --owner=${{ vars.GITHUB_ORG }} \
              --resources=repositories \
              --path-output=/workspace/generated \
              --compact \
              --verbose
          "
          
      - name: Format Terraform code via docker exec
        run: |
          docker exec terraformer-container terraform fmt -recursive /workspace/generated
          
      - name: Run terraform plan via docker exec
        id: plan
        run: |
          docker exec -w /workspace/generated/github terraformer-container bash -c "
            terraform init
            terraform plan -detailed-exitcode -no-color | tee /workspace/plan.txt
          " || echo "exitcode=$?" >> $GITHUB_OUTPUT
        continue-on-error: true
          
      - name: Stop and remove container
        if: always()
        run: docker rm -f terraformer-container
        
      - name: Upload generated files
        uses: actions/upload-artifact@v4
        with:
          name: github-terraform-${{ github.run_number }}
          path: generated/
          retention-days: 30
```

**Key advantages of docker exec pattern**:
- **Isolated environment** with all tools pre-installed
- **Consistent execution** across different runners
- **Volume mounting** enables direct file access on runner
- **Multiple commands** can be executed in same container context
- **Container cleanup** ensures no state leakage between runs

### Hourly cron schedule to export specific GitHub resources and detect drift

**Complete workflow for hourly drift detection**:

```yaml
name: GitHub Resource Drift Detection

on:
  schedule:
    - cron: '0 * * * *'  # Every hour at minute 0
  workflow_dispatch:

permissions:
  contents: write
  issues: write
  pull-requests: write

jobs:
  export-and-detect-drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Azure Login
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
          
      - name: Login to ACR
        run: az acr login --name ${{ vars.ACR_NAME }}
        
      - name: Download previous export
        id: download_previous
        uses: actions/download-artifact@v4
        with:
          name: github-resources-state
          path: previous
        continue-on-error: true
        
      - name: Export specific GitHub resources
        run: |
          docker run --rm \
            -v ${{ github.workspace }}:/workspace \
            -e GITHUB_TOKEN=${{ secrets.GITHUB_TOKEN }} \
            -w /workspace \
            ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest \
            bash -c "
              # Create provider configuration
              cat > versions.tf <<'EOF'
          terraform {
            required_providers {
              github = {
                source  = \"integrations/github\"
                version = \"~> 5.0\"
              }
            }
          }
          provider \"github\" {
            owner = \"${{ vars.GITHUB_ORG }}\"
          }
          EOF
              
              # Initialize provider
              terraform init
              
              # Export ONLY specified resources
              terraformer import github \
                --owner=${{ vars.GITHUB_ORG }} \
                --resources=repositories \
                --path-output=/workspace/current \
                --compact \
                --verbose
              
              # Note: Unsupported resources must be managed separately:
              # - github_repository_ruleset
              # - github_organization_ruleset
              # - github_organization_settings
              # - github_organization_custom_role
            "
            
      - name: Run Terraform plan to detect changes
        id: plan
        run: |
          docker run --rm \
            -v ${{ github.workspace }}:/workspace \
            -e GITHUB_TOKEN=${{ secrets.GITHUB_TOKEN }} \
            -w /workspace/current/github \
            ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest \
            bash -c "
              terraform init
              terraform plan -detailed-exitcode -no-color -out=tfplan | tee /workspace/plan.txt
            " || echo "exitcode=$?" >> $GITHUB_OUTPUT
        continue-on-error: true
        
      - name: Compare with previous export
        if: steps.download_previous.outcome == 'success'
        id: compare
        run: |
          docker run --rm \
            -v ${{ github.workspace }}:/workspace \
            ${{ vars.ACR_NAME }}.azurecr.io/terraformer:latest \
            bash -c "
              if [ -d /workspace/previous ]; then
                # Compare generated Terraform files
                diff -r /workspace/previous /workspace/current > /workspace/diff.txt || true
                
                # Check if differences exist
                if [ -s /workspace/diff.txt ]; then
                  echo 'changes_detected=true' >> \$GITHUB_OUTPUT
                  
                  # Count lines changed
                  LINES_CHANGED=\$(wc -l < /workspace/diff.txt)
                  echo \"lines_changed=\$LINES_CHANGED\" >> \$GITHUB_OUTPUT
                  
                  # Use jq to analyze tfstate if available
                  if [ -f /workspace/current/github/terraform.tfstate ]; then
                    RESOURCE_COUNT=\$(cat /workspace/current/github/terraform.tfstate | jq '.resources | length')
                    echo \"resource_count=\$RESOURCE_COUNT\" >> \$GITHUB_OUTPUT
                  fi
                else
                  echo 'changes_detected=false' >> \$GITHUB_OUTPUT
                fi
              fi
            "
            
      - name: Upload current export as artifact
        uses: actions/upload-artifact@v4
        with:
          name: github-resources-state
          path: current/
          retention-days: 7
          
      - name: Create drift detection issue
        if: steps.plan.outputs.exitcode == '2' || steps.compare.outputs.changes_detected == 'true'
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const planOutput = fs.readFileSync('plan.txt', 'utf8');
            
            // Check if issue already exists
            const issues = await github.rest.issues.listForRepo({
              owner: context.repo.owner,
              repo: context.repo.repo,
              labels: 'github-drift',
              state: 'open'
            });
            
            const existingIssue = issues.data.find(issue => 
              issue.title.includes('GitHub Infrastructure Drift')
            );
            
            const issueBody = `## 🚨 GitHub Infrastructure Drift Detected
            
            **Detection Time:** ${new Date().toISOString()}
            **Workflow Run:** [#${{ github.run_number }}](${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }})
            
            ### Resources Monitored
            - ✅ github_repositories
            - ⚠️ github_repository_ruleset (unsupported - manual monitoring required)
            - ⚠️ github_organization_ruleset (unsupported - manual monitoring required)
            - ⚠️ github_organization_settings (unsupported - manual monitoring required)
            - ⚠️ github_organization_custom_role (unsupported - manual monitoring required)
            
            ### Detected Changes
            
            \`\`\`terraform
            ${planOutput.substring(0, 50000)}
            \`\`\`
            
            ### Action Required
            1. Review the changes above
            2. Determine if changes were intentional
            3. Either:
               - Update Terraform code to match current state
               - Apply Terraform to revert unauthorized changes
            4. Close this issue after resolution
            
            ### Statistics
            - Lines changed: ${{ steps.compare.outputs.lines_changed || 'N/A' }}
            - Resource count: ${{ steps.compare.outputs.resource_count || 'N/A' }}
            `;
            
            if (existingIssue) {
              // Update existing issue
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: existingIssue.number,
                body: issueBody
              });
            } else {
              // Create new issue
              await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `GitHub Infrastructure Drift Detected - ${new Date().toISOString().split('T')[0]}`,
                body: issueBody,
                labels: ['github-drift', 'infrastructure', 'automated']
              });
            }
            
      - name: Send Slack notification on drift
        if: steps.plan.outputs.exitcode == '2'
        run: |
          curl -X POST ${{ secrets.SLACK_WEBHOOK_URL }} \
            -H 'Content-Type: application/json' \
            -d '{
              "text": "🚨 GitHub Infrastructure Drift Detected",
              "blocks": [
                {
                  "type": "section",
                  "text": {
                    "type": "mrkdwn",
                    "text": "*GitHub Infrastructure Drift Detected*\n\nChanges detected in GitHub organization infrastructure. Review workflow run for details."
                  }
                },
                {
                  "type": "section",
                  "fields": [
                    {
                      "type": "mrkdwn",
                      "text": "*Organization:*\n${{ vars.GITHUB_ORG }}"
                    },
                    {
                      "type": "mrkdwn",
                      "text": "*Workflow:*\n<${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}|View Run>"
                    }
                  ]
                }
              ]
            }'
```

**Key features of this workflow**:
- ✅ **Runs every hour** via cron schedule
- ✅ **Exports specific resources** (repositories only, as rulesets/settings unsupported)
- ✅ **Uses custom Docker image** from ACR with all helper tools
- ✅ **Compares with previous run** using diff command
- ✅ **Detects drift** via terraform plan with detailed exit codes
- ✅ **Creates GitHub issues** for detected drift
- ✅ **Stores artifacts** for historical comparison
- ✅ **Sends notifications** (Slack integration example)

**Cron schedule variations**:
```yaml
# Every hour
- cron: '0 * * * *'

# Every 2 hours
- cron: '0 */2 * * *'

# Every 6 hours
- cron: '0 */6 * * *'

# Every hour during business hours (9-17 UTC) on weekdays
- cron: '0 9-17 * * 1-5'

# Multiple schedules
schedule:
  - cron: '0 */1 * * 1-5'  # Hourly on weekdays
  - cron: '0 */6 * * 0,6'  # Every 6 hours on weekends
```

**Important notes**:
- Cron times are **UTC** - adjust for your timezone
- Minimum interval is **5 minutes**
- GitHub may **delay execution** during high load (especially at top of hour)
- Only runs on **default branch**

---

## Terraform State Management and Drift Detection

### How Terraformer generates state files

Terraformer's state generation process:
1. Queries cloud provider APIs to list resources
2. Extracts resource IDs
3. Calls Terraform provider refresh methods to retrieve complete resource state
4. Converts data to Go structs
5. Generates both `.tf` configuration files and `.tfstate` files

**Output structure**:
```
generated/
└── github/
    ├── repositories/
    │   ├── repository1.tf
    │   ├── repository2.tf
    │   └── terraform.tfstate
    └── teams/
        ├── team1.tf
        └── terraform.tfstate
```

### Detecting infrastructure drift

**Drift** occurs when actual infrastructure state diverges from Terraform's recorded state due to manual changes, external automation, or emergency modifications.

**Primary detection method - terraform plan**:
```bash
# Basic drift check
terraform plan -detailed-exitcode

# Exit codes:
# 0 = No changes (no drift)
# 1 = Error occurred
# 2 = Changes present (drift detected)

# Refresh-only mode shows drift without modifying state
terraform plan -refresh-only
```

**Automated drift detection in CI/CD**:
```yaml
- name: Check for drift
  id: plan
  run: terraform plan -detailed-exitcode -no-color | tee plan.txt
  continue-on-error: true

- name: Handle drift detection
  if: steps.plan.outputs.exitcode == '2'
  run: |
    echo "Drift detected!"
    # Create issue, send notification, etc.
```

### Comparing terraform state between runs

**Method 1: Plan-based comparison**
```bash
# Generate plans at different times
terraform plan -out=plan1.tfplan
# ... changes occur ...
terraform plan -out=plan2.tfplan

# Convert to JSON and compare
terraform show -json plan1.tfplan > plan1.json
terraform show -json plan2.tfplan > plan2.json

# Use jq to compare resource changes
diff <(jq '.resource_changes | sort_by(.address)' plan1.json) \
     <(jq '.resource_changes | sort_by(.address)' plan2.json)
```

**Method 2: State file comparison**
```bash
# Pull state at different times
terraform state pull > state1.json
# ... changes occur ...
terraform state pull > state2.json

# Compare states
diff state1.json state2.json

# Or use jq for structured comparison
diff <(jq '.resources | sort_by(.type + .name)' state1.json) \
     <(jq '.resources | sort_by(.type + .name)' state2.json)
```

**Method 3: Artifact-based comparison in GitHub Actions**
```yaml
- name: Download previous state
  uses: actions/download-artifact@v4
  with:
    name: terraform-state
    path: previous
  continue-on-error: true

- name: Generate current state
  run: terraform show -json > current-state.json

- name: Compare states
  run: |
    if [ -f previous/state.json ]; then
      diff previous/state.json current-state.json > diff.txt || true
      if [ -s diff.txt ]; then
        echo "changes_detected=true" >> $GITHUB_ENV
      fi
    fi

- name: Upload current state
  uses: actions/upload-artifact@v4
  with:
    name: terraform-state
    path: current-state.json
    retention-days: 30
```

### State management best practices

**Remote state storage**:
```hcl
terraform {
  backend "azurerm" {
    resource_group_name  = "terraform-rg"
    storage_account_name = "terraformstate"
    container_name       = "tfstate"
    key                 = "prod.terraform.tfstate"
  }
}
```

**State locking** prevents concurrent modifications:
```hcl
terraform {
  backend "s3" {
    bucket         = "terraform-state"
    key            = "prod/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "terraform-locks"  # Enables state locking
  }
}
```

**Never commit state files to version control**:
```gitignore
# .gitignore
*.tfstate
*.tfstate.*
*.tfstate.backup
.terraform/
.terraform.lock.hcl
```

---

## Common Usage Patterns

### Initial infrastructure import

**1. Prepare working directory**:
```bash
mkdir terraform-import
cd terraform-import

# Create provider configuration
cat > versions.tf <<EOF
terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 5.0"
    }
  }
}

provider "github" {
  owner = "myorg"
}
EOF

terraform init
```

**2. List available resources**:
```bash
terraformer import github list
```

**3. Import specific resources**:
```bash
export GITHUB_TOKEN=ghp_xxx

# Start with repositories
terraformer import github \
  --owner=myorg \
  --resources=repositories \
  --compact

# Verify generated files
ls -la generated/github/
```

**4. Validate with Terraform**:
```bash
cd generated/github
terraform init
terraform plan  # Should show no changes if import was successful
```

### Incremental adoption

**Phase 1: Import critical infrastructure**:
```bash
terraformer import github \
  --owner=myorg \
  --resources=repositories \
  --filter=repository=prod-api:prod-web:prod-database
```

**Phase 2: Add teams and memberships**:
```bash
terraformer import github \
  --owner=myorg \
  --resources=teams,members
```

**Phase 3: Add webhooks and projects**:
```bash
terraformer import github \
  --owner=myorg \
  --resources=organization_webhooks,organization_projects
```

### Migrating to Terraform management

**After import**:

1. **Review generated code** - Check for hardcoded values that should be variables
2. **Extract variables**:
```hcl
# variables.tf
variable "repository_visibility" {
  default = "private"
}

# Use in configuration
resource "github_repository" "example" {
  name       = "example"
  visibility = var.repository_visibility
}
```

3. **Refactor structure** - Organize into logical modules
4. **Add remote state** - Configure backend for team collaboration
5. **Set up CI/CD** - Automate plan and apply operations
6. **Enable drift detection** - Schedule regular checks

### Handling unsupported resources

For GitHub resources not supported by terraformer (rulesets, organization settings, custom roles):

```hcl
# Manually create in Terraform after import
resource "github_organization_ruleset" "main" {
  name        = "main-ruleset"
  target      = "branch"
  enforcement = "active"
  
  rules {
    required_status_checks {
      strict_status_check_policy {
        strict   = true
        contexts = ["ci"]
      }
    }
  }
}
```

---

## Best Practices

### Security and access control

**Use read-only credentials** for import operations:
```bash
# GitHub - ensure token has minimum required scopes
# AWS - use IAM policy with read-only permissions
# Azure - assign Reader role to service principal
```

**Store credentials securely**:
```yaml
# GitHub Actions - use encrypted secrets
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  ARM_CLIENT_SECRET: ${{ secrets.AZURE_CLIENT_SECRET }}
```

**Implement least privilege**:
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "ec2:Describe*",
      "s3:GetObject",
      "s3:ListBucket"
    ],
    "Resource": "*"
  }]
}
```

### Organization and structure

**Separate state by environment**:
```
terraform/
├── production/
│   ├── main.tf
│   └── terraform.tfstate
├── staging/
│   ├── main.tf
│   └── terraform.tfstate
└── development/
    ├── main.tf
    └── terraform.tfstate
```

**Use consistent naming**:
```bash
# Pattern: {environment}-{service}-{resource}
terraformer import aws \
  --resources=vpc \
  --filter="Name=tags.Environment;Value=production"
```

**Organize by service**:
```
terraform/
├── networking/
│   ├── vpc.tf
│   └── subnets.tf
├── compute/
│   └── instances.tf
└── storage/
    └── buckets.tf
```

### Performance optimization

**Import in batches** for large infrastructures:
```bash
# Import by region
for region in us-east-1 us-west-2 eu-west-1; do
  terraformer import aws \
    --resources=vpc,subnet \
    --regions=$region \
    --path-output=./terraform-$region
done
```

**Use filtering** to limit scope:
```bash
# Filter by tags
terraformer import aws \
  --resources=ec2_instance \
  --filter="Name=tags.Team;Value=platform" \
  --regions=us-east-1
```

**Handle rate limits**:
```bash
# Add retry flags
terraformer import github \
  --owner=myorg \
  --resources=repositories \
  --retry-number=5 \
  --retry-sleep-ms=1000
```

### CI/CD integration patterns

**Plan on pull request, apply on merge**:
```yaml
name: Terraform Workflow

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  plan:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: terraform init
      - run: terraform plan -no-color | tee plan.txt
      - uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const plan = fs.readFileSync('plan.txt', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '```terraform\n' + plan + '\n```'
            });

  apply:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: terraform init
      - run: terraform apply -auto-approve
```

**Scheduled drift detection with remediation**:
```yaml
on:
  schedule:
    - cron: '0 */6 * * *'

jobs:
  drift-check:
    runs-on: ubuntu-latest
    steps:
      - run: terraform plan -detailed-exitcode
        id: plan
        continue-on-error: true
      
      - if: steps.plan.outputs.exitcode == '2'
        run: |
          # Notify team
          # Optionally: terraform apply -auto-approve (if policy allows)
```

### Drift prevention strategies

**1. Implement access controls** - Restrict manual changes via IAM policies
**2. Enable audit logging** - Track all infrastructure modifications
**3. Use policy as code** - Enforce compliance before apply (OPA, Sentinel)
**4. Require PR reviews** - All changes through version control
**5. Schedule regular scans** - Hourly or daily drift detection
**6. Automate documentation** - Keep infrastructure documentation current

### Handling drift when detected

**Intentional changes** (import into Terraform):
```bash
# Import manually created resource
terraform import aws_instance.new_server i-1234567890abcdef0

# Update configuration to match
```

**Unintentional changes** (revert to desired state):
```bash
# Apply Terraform to restore configuration
terraform apply

# Investigate root cause
# Update access controls to prevent recurrence
```

**Resource no longer managed**:
```bash
# Remove from Terraform state without destroying
terraform state rm aws_instance.decommissioned
```

### Documentation and maintenance

**Document import process**:
```markdown
# Infrastructure Import Procedure

## Prerequisites
- Terraform 1.7.2+
- Terraformer 0.8.30+
- AWS credentials with read access

## Steps
1. `terraform init`
2. `terraformer import aws --resources=vpc --regions=us-east-1`
3. Review generated files in `generated/aws/`
4. `terraform plan` to verify
5. Commit to version control
```

**Maintain version compatibility**:
```hcl
# versions.tf
terraform {
  required_version = ">= 1.7.0"
  
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 5.0"
    }
  }
}
```

**Regular updates**:
```bash
# Update Terraformer
brew upgrade terraformer

# Update Terraform
brew upgrade terraform

# Update providers
terraform init -upgrade
```

---

## Troubleshooting

### Common errors and solutions

**Error: "terraform provider not found"**
```bash
# Solution: Initialize Terraform provider first
terraform init
```

**Error: "401 Unauthorized" (GitHub)**
```bash
# Solution: Verify token is valid and has required scopes
curl -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/user

# Ensure token has: repo, admin:org, read:org scopes
```

**Error: "404 Not Found" (GitHub)**
```bash
# Solution: Verify organization name and token access
# Check: --owner flag matches organization name
# Check: Token has access to organization
```

**Error: Webhook secrets showing as changed**
```bash
# This is expected - GitHub API doesn't return secrets
# Solution: Use lifecycle block to ignore
lifecycle {
  ignore_changes = [configuration]
}
```

**Rate limiting errors**
```bash
# Solution: Add retry flags
terraformer import github \
  --owner=myorg \
  --resources=repositories \
  --retry-number=5 \
  --retry-sleep-ms=2000
```

### GitHub Enterprise issues

**Base URL not respected (pre-v0.8.13)**:
```bash
# Solution: Upgrade to v0.8.30+
# Use --base-url flag instead of environment variable
terraformer import github \
  --base-url=https://github.enterprise.com \
  --owner=org \
  --resources=repositories
```

**SSL certificate errors**:
```bash
# For testing only - do not use in production
export GIT_SSL_NO_VERIFY=true
```

### Docker container issues

**Container exits immediately**:
```bash
# Solution: Use tail -f to keep container running for docker exec
docker run -d \
  --name terraformer \
  terraformer:latest \
  tail -f /dev/null
```

**Permission denied on mounted volumes**:
```bash
# Solution: Ensure correct volume mount syntax
docker run -v $(pwd):/workspace -w /workspace terraformer:latest
```

**Azure CLI not authenticated in container**:
```bash
# Solution: Mount Azure credentials or use environment variables
docker run \
  -v ~/.azure:/root/.azure:ro \
  -e ARM_CLIENT_ID=$ARM_CLIENT_ID \
  terraformer:latest
```

### State management issues

**State locked**:
```bash
# Check who has lock (DynamoDB/Azure Storage)
# Force unlock (use with caution)
terraform force-unlock LOCK_ID
```

**State corruption**:
```bash
# Restore from backup
# S3: Use versioning to retrieve previous version
# Azure: Use blob versioning
# Local: Use .backup file
```

**Plan shows changes after import**:
```bash
# Solution: Some computed fields may differ
# Review differences - may need lifecycle ignore_changes
# Re-run terraform apply to sync state
```

### GitHub Actions workflow debugging

**Enable debug logging**:
```yaml
env:
  ACTIONS_STEP_DEBUG: true
  ACTIONS_RUNNER_DEBUG: true
```

**Inspect container logs**:
```yaml
- name: Show container logs
  if: failure()
  run: docker logs terraformer-container
```

**Verify environment variables**:
```yaml
- name: Debug environment
  run: |
    echo "GitHub Token length: ${#GITHUB_TOKEN}"
    echo "Organization: ${{ vars.GITHUB_ORG }}"
    env | grep -v TOKEN | sort
```

---

## Summary

Terraformer provides powerful infrastructure-to-code capabilities by reverse-engineering existing cloud resources into Terraform configurations. **For GitHub Enterprise organizations**, terraformer enables comprehensive exports of repositories, teams, memberships, webhooks, and projects, though newer features like rulesets and custom roles require manual Terraform management.

**GitHub Actions automation** enables continuous import and drift detection workflows. Using **custom Docker images** containing terraformer with helper tools (jq, bash, curl, GitHub CLI, Azure CLI, diff) and pushing to **Azure Container Registry** creates reproducible, isolated execution environments. **Service container and docker exec patterns** provide flexible approaches for running terraformer in CI/CD pipelines.

**Hourly cron schedules** enable continuous monitoring of GitHub organizational resources, automatically detecting drift through terraform plan comparisons and creating issues for team notification. This workflow combines terraformer's import capabilities with Terraform's state management to maintain infrastructure consistency and prevent unauthorized changes.

The key to successful terraformer adoption lies in **incremental implementation**, starting with critical resources, establishing drift detection early, and gradually expanding coverage while maintaining clear documentation of unsupported resources requiring manual management.