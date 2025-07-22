# ADO CLI Docker Quick Start 🐳

Get the Azure DevOps CLI running in Docker in minutes!

## 📋 Prerequisites

- Docker installed (20.10+)
- Docker Compose (optional, for easier management)
- Azure DevOps Personal Access Token

## ⚡ Quick Start (30 seconds)

```bash
# 1. Create project directory
mkdir ado-cli-docker && cd ado-cli-docker

# 2. Download the Go source (save as ado-cli.go)
# Copy the complete CLI code from the previous artifact

# 3. Create Dockerfile (copy from Dockerfile artifact)

# 4. Set your Azure DevOps credentials
export AZURE_DEVOPS_PAT="your-pat-token-here"
export ADO_ORGANIZATION="your-org-name"  
export ADO_PROJECT="your-project-name"

# 5. Build and run
docker build -t ado-cli .
docker run --rm -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" ado-cli help
```

## 🚀 Full Setup

### 1. Project Structure

Create this directory structure:

```
ado-cli-docker/
├── ado-cli.go              # Main CLI source code
├── Dockerfile              # Docker build configuration
├── docker-compose.yml      # Docker Compose configuration  
├── .dockerignore          # Files to exclude from build
├── build.sh               # Build automation script
├── data/                  # Output files (created automatically)
└── config/                # Configuration files
    └── environment-levels.json
```

### 2. Download Files

Save these artifacts to your project directory:
- **ado-cli.go** - The complete CLI source code
- **Dockerfile** - Multi-stage Docker build
- **docker-compose.yml** - Docker Compose configuration
- **.dockerignore** - Build exclusions
- **build.sh** - Build automation script

### 3. Create Configuration

```bash
# Create directories
mkdir -p data config

# Create environment levels configuration
cat > config/environment-levels.json << 'EOF'
["prod", "stg", "uat", "qa", "dev"]
EOF
```

### 4. Set Environment Variables

```bash
# Required: Azure DevOps PAT token
export AZURE_DEVOPS_PAT="your-pat-token-here"

# Optional: Override defaults
export ADO_ORGANIZATION="your-org-name"     # default: myorga
export ADO_PROJECT="your-project-name"      # default: project001
```

## 🛠️ Build Options

### Option A: Using Build Script (Recommended)

```bash
# Make executable and run
chmod +x build.sh
./build.sh

# Build with specific version
BUILD_VERSION=1.0.0 ./build.sh v1.0.0
```

### Option B: Using Docker Compose

```bash
# Build with compose
docker-compose build

# Build with custom version
BUILD_VERSION=1.0.0 docker-compose build
```

### Option C: Manual Docker Build

```bash
docker build \
    --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
    --build-arg BUILD_VERSION=1.0.0 \
    -t ado-cli:latest .
```

## 🎯 Usage Examples

### Basic Commands

```bash
# Show help
docker run --rm ado-cli:latest help

# List environments and organize by levels
docker run --rm \
    -v $(pwd)/data:/data \
    -v $(pwd)/config:/config:ro \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest environments -levels /config/environment-levels.json

# Get deployment records for specific environments
docker run --rm \
    -v $(pwd)/data:/data \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest deployments -envs "8,10,15"

# Generate deployment summaries
docker run --rm \
    -v $(pwd)/data:/data \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest summary -input env-dep-rec-res-8.json -env 8 -monthly
```

### Complete Workflow

```bash
# Run full workflow with monthly summaries
docker run --rm \
    -v $(pwd)/data:/data \
    -v $(pwd)/config:/config:ro \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    -e ADO_ORGANIZATION="$ADO_ORGANIZATION" \
    -e ADO_PROJECT="$ADO_PROJECT" \
    ado-cli:latest full -levels /config/environment-levels.json -monthly
```

### Docker Compose Usage

```bash
# Create .env file for easier management
cat > .env << EOF
AZURE_DEVOPS_PAT=your-pat-token
ADO_ORGANIZATION=your-org
ADO_PROJECT=your-project
BUILD_VERSION=1.0.0
EOF

# Run full workflow
docker-compose run --rm ado-cli full -levels /config/environment-levels.json -monthly

# Interactive development mode
docker-compose up -d ado-cli-dev
docker-compose exec ado-cli-dev sh

# Run scheduled job (24-hour intervals)
docker-compose --profile scheduler up -d ado-cli-scheduler
```

## 📊 Output Files

After running commands, check the `./data/` directory:

```bash
# List generated files
ls -la data/

# Example output files:
# prod-ids.json              - Production environment mappings
# dev-ids.json               - Development environment mappings  
# env-dep-rec-res-8.json     - Raw deployment records for env 8
# deploy-summary-8.json      - All-time deployment statistics
# deploy-summary-8-monthly.json - Last 30 days statistics
```

## 🔧 Advanced Usage

### Persistent Storage

```bash
# Create named volume for persistent data
docker volume create ado-cli-data

# Use volume in run command
docker run --rm \
    -v ado-cli-data:/data \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest full
```

### Custom Configuration

```bash
# Use custom environment levels
cat > config/my-levels.json << 'EOF'
["production", "staging", "development"]
EOF

# Run with custom config
docker run --rm \
    -v $(pwd)/data:/data \
    -v $(pwd)/config:/config:ro \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest environments -levels /config/my-levels.json
```

### Multi-Organization Setup

```bash
# Process multiple organizations
for org in "org1" "org2" "org3"; do
    echo "Processing organization: $org"
    docker run --rm \
        -v $(pwd)/data/$org:/data \
        -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
        -e ADO_ORGANIZATION="$org" \
        -e ADO_PROJECT="main-project" \
        ado-cli:latest full -monthly
done
```

### CI/CD Integration

```bash
# Example Jenkins pipeline step
docker run --rm \
    -v "${WORKSPACE}/ado-reports:/data" \
    -e AZURE_DEVOPS_PAT="${AZURE_PAT}" \
    -e ADO_ORGANIZATION="${ADO_ORG}" \
    -e ADO_PROJECT="${ADO_PROJ}" \
    ado-cli:latest full -monthly

# Archive artifacts
archiveArtifacts artifacts: 'ado-reports/*.json', fingerprint: true
```

## 🐛 Troubleshooting

### Build Issues

```bash
# Check if source file exists
ls -la ado-cli.go

# Build without cache
docker build --no-cache -t ado-cli .

# Check build logs
docker build --progress=plain -t ado-cli . 2>&1 | tee build.log
```

### Runtime Issues

```bash
# Test authentication
docker run --rm \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest sh -c "curl -s -H \"Authorization: Basic \$(echo -n \$AZURE_DEVOPS_PAT: | base64)\" https://dev.azure.com"

# Debug with shell access
docker run --rm -it --entrypoint sh ado-cli:latest

# Check environment variables
docker run --rm -e AZURE_DEVOPS_PAT="test" ado-cli:latest env | grep -E "(PAT|ADO_)"
```

### Permission Issues

```bash
# Check data directory permissions
ls -la data/
sudo chown -R $USER:$USER data/

# Run with specific user ID
docker run --rm --user $(id -u):$(id -g) \
    -v $(pwd)/data:/data \
    ado-cli:latest help
```

## 🔐 Security Notes

- Never embed PAT tokens in Docker images
- Use environment variables or external secrets
- The container runs as non-root user (UID 1000)
- Config directory mounted read-only
- Minimal Alpine Linux base image

## 📈 Performance Tips

- Use multi-stage build for smaller images (~20MB vs ~400MB)
- Mount specific directories instead of entire filesystem
- Use Docker BuildKit for faster builds
- Clean up unused images: `docker image prune`

## 🎉 Success!

You should now have:
- ✅ A containerized Azure DevOps CLI
- ✅ Automated build and run scripts
- ✅ Environment organization by levels
- ✅ Deployment analytics and summaries
- ✅ Persistent data storage
- ✅ Production-ready Docker setup

Happy DevOps! 🚀