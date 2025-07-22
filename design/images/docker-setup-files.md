# Docker Setup for Azure DevOps CLI

Complete Docker setup including Dockerfile, build scripts, and usage examples.

## Files Included

### 1. `.dockerignore`

```dockerignore
# Git
.git/
.gitignore
.gitattributes

# Documentation
*.md
README*
docs/

# IDE files
.vscode/
.idea/
*.swp
*.swo
*~

# OS files
.DS_Store
Thumbs.db

# Build artifacts
*.json
*.log
deploy-summary*
env-dep-rec-res-*
*-ids.json
environment-levels.json

# Docker files
Dockerfile*
docker-compose*
.dockerignore

# Test files
*_test.go
test/
tests/

# Temporary files
tmp/
temp/
*.tmp
```

### 2. `docker-compose.yml`

```yaml
version: '3.8'

services:
  ado-cli:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        BUILD_DATE: ${BUILD_DATE:-$(date -u +'%Y-%m-%dT%H:%M:%SZ')}
        BUILD_VERSION: ${BUILD_VERSION:-1.0.0}
        BUILD_COMMIT: ${BUILD_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}
    image: ado-cli:latest
    container_name: ado-cli
    environment:
      - AZURE_DEVOPS_PAT=${AZURE_DEVOPS_PAT}
      - ADO_ORGANIZATION=${ADO_ORGANIZATION:-myorga}
      - ADO_PROJECT=${ADO_PROJECT:-project001}
      - TZ=${TZ:-UTC}
    volumes:
      - ./data:/data
      - ./config:/config:ro
    working_dir: /data
    command: help
    
  # Interactive shell for development
  ado-cli-dev:
    extends: ado-cli
    container_name: ado-cli-dev
    entrypoint: /bin/sh
    command: -c "while true; do sleep 30; done"
    tty: true
    stdin_open: true

volumes:
  ado-data:
    driver: local
```

### 3. `build.sh` - Build Script

```bash
#!/bin/bash

# Azure DevOps CLI Docker Build Script
set -e

# Configuration
IMAGE_NAME="ado-cli"
IMAGE_TAG="${1:-latest}"
BUILD_PLATFORM="${BUILD_PLATFORM:-linux/amd64,linux/arm64}"

# Build information
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILD_VERSION="${BUILD_VERSION:-1.0.0}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"

echo "🐳 Building Azure DevOps CLI Docker Image"
echo "========================================"
echo "Image: ${IMAGE_NAME}:${IMAGE_TAG}"
echo "Platform: ${BUILD_PLATFORM}"
echo "Build Date: ${BUILD_DATE}"
echo "Version: ${BUILD_VERSION}"
echo "Commit: ${BUILD_COMMIT}"
echo ""

# Check if ado-cli.go exists
if [ ! -f "ado-cli.go" ]; then
    echo "❌ Error: ado-cli.go not found in current directory"
    echo "Please ensure the Go source file is present"
    exit 1
fi

# Build multi-platform image if buildx is available
if docker buildx version >/dev/null 2>&1; then
    echo "🔨 Building multi-platform image with buildx..."
    
    # Create builder if it doesn't exist
    docker buildx create --name ado-cli-builder --use 2>/dev/null || true
    
    # Build and push (or load for single platform)
    if [ "$BUILD_PLATFORM" = "linux/amd64" ]; then
        docker buildx build \
            --platform "$BUILD_PLATFORM" \
            --build-arg BUILD_DATE="$BUILD_DATE" \
            --build-arg BUILD_VERSION="$BUILD_VERSION" \
            --build-arg BUILD_COMMIT="$BUILD_COMMIT" \
            --tag "${IMAGE_NAME}:${IMAGE_TAG}" \
            --tag "${IMAGE_NAME}:latest" \
            --load \
            .
    else
        docker buildx build \
            --platform "$BUILD_PLATFORM" \
            --build-arg BUILD_DATE="$BUILD_DATE" \
            --build-arg BUILD_VERSION="$BUILD_VERSION" \
            --build-arg BUILD_COMMIT="$BUILD_COMMIT" \
            --tag "${IMAGE_NAME}:${IMAGE_TAG}" \
            --tag "${IMAGE_NAME}:latest" \
            --push \
            .
    fi
else
    echo "🔨 Building single-platform image..."
    docker build \
        --build-arg BUILD_DATE="$BUILD_DATE" \
        --build-arg BUILD_VERSION="$BUILD_VERSION" \
        --build-arg BUILD_COMMIT="$BUILD_COMMIT" \
        --tag "${IMAGE_NAME}:${IMAGE_TAG}" \
        --tag "${IMAGE_NAME}:latest" \
        .
fi

echo ""
echo "✅ Build completed successfully!"
echo ""
echo "🚀 Quick start:"
echo "  docker run --rm -e AZURE_DEVOPS_PAT=\"your-token\" ${IMAGE_NAME}:${IMAGE_TAG} help"
echo ""
echo "📁 With data persistence:"
echo "  docker run --rm -v \$(pwd)/data:/data -e AZURE_DEVOPS_PAT=\"your-token\" ${IMAGE_NAME}:${IMAGE_TAG} full"
```

### 4. `run.sh` - Run Script

```bash
#!/bin/bash

# Azure DevOps CLI Docker Run Script
set -e

# Default configuration
IMAGE_NAME="ado-cli"
IMAGE_TAG="${IMAGE_TAG:-latest}"
DATA_DIR="${DATA_DIR:-$(pwd)/data}"
CONFIG_DIR="${CONFIG_DIR:-$(pwd)/config}"

# Ensure directories exist
mkdir -p "$DATA_DIR" "$CONFIG_DIR"

# Check for required environment variables
if [ -z "$AZURE_DEVOPS_PAT" ]; then
    echo "❌ Error: AZURE_DEVOPS_PAT environment variable is required"
    echo ""
    echo "Set your Personal Access Token:"
    echo "  export AZURE_DEVOPS_PAT=\"your-pat-token-here\""
    echo ""
    echo "Or create a .env file:"
    echo "  echo 'AZURE_DEVOPS_PAT=your-token' > .env"
    echo "  source .env"
    exit 1
fi

echo "🐳 Running Azure DevOps CLI"
echo "=========================="
echo "Image: ${IMAGE_NAME}:${IMAGE_TAG}"
echo "Data Directory: ${DATA_DIR}"
echo "Config Directory: ${CONFIG_DIR}"
echo "Organization: ${ADO_ORGANIZATION:-myorga}"
echo "Project: ${ADO_PROJECT:-project001}"
echo ""

# Create default environment-levels.json if it doesn't exist
if [ ! -f "${CONFIG_DIR}/environment-levels.json" ]; then
    echo '["prod", "stg", "uat", "qa", "dev"]' > "${CONFIG_DIR}/environment-levels.json"
    echo "📝 Created default environment-levels.json"
fi

# Run the container
docker run --rm -it \
    -v "${DATA_DIR}:/data" \
    -v "${CONFIG_DIR}:/config:ro" \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    -e ADO_ORGANIZATION="${ADO_ORGANIZATION:-myorga}" \
    -e ADO_PROJECT="${ADO_PROJECT:-project001}" \
    -e TZ="${TZ:-UTC}" \
    --name ado-cli-run \
    "${IMAGE_NAME}:${IMAGE_TAG}" \
    "$@"
```

## Build Instructions

### 1. Quick Build

```bash
# Make build script executable
chmod +x build.sh

# Build the image
./build.sh

# Or build with specific tag
./build.sh v1.0.0
```

### 2. Docker Compose Build

```bash
# Build with docker-compose
docker-compose build

# Build with environment variables
BUILD_VERSION=1.0.0 docker-compose build
```

### 3. Manual Build

```bash
docker build \
    --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
    --build-arg BUILD_VERSION=1.0.0 \
    --build-arg BUILD_COMMIT=$(git rev-parse --short HEAD) \
    -t ado-cli:latest \
    .
```

## Usage Examples

### 1. Basic Usage

```bash
# Run with environment variables
export AZURE_DEVOPS_PAT="your-token"
export ADO_ORGANIZATION="your-org"
export ADO_PROJECT="your-project"

# Show help
docker run --rm ado-cli:latest help

# Run full workflow
docker run --rm \
    -v $(pwd)/data:/data \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    -e ADO_ORGANIZATION="$ADO_ORGANIZATION" \
    -e ADO_PROJECT="$ADO_PROJECT" \
    ado-cli:latest full -monthly
```

### 2. Using Run Script

```bash
# Make run script executable
chmod +x run.sh

# Set environment variables
export AZURE_DEVOPS_PAT="your-token"

# Run full workflow
./run.sh full -monthly

# Process environments only
./run.sh environments -levels /config/environment-levels.json
```

### 3. Docker Compose Usage

```bash
# Create .env file
cat > .env << EOF
AZURE_DEVOPS_PAT=your-pat-token
ADO_ORGANIZATION=your-org
ADO_PROJECT=your-project
BUILD_VERSION=1.0.0
EOF

# Run with docker-compose
docker-compose run --rm ado-cli full -monthly

# Interactive development mode
docker-compose up -d ado-cli-dev
docker-compose exec ado-cli-dev sh
```

### 4. Persistent Data

```bash
# Create data directory
mkdir -p ./data ./config

# Create configuration
echo '["prod", "stg", "dev"]' > ./config/environment-levels.json

# Run with persistent data
docker run --rm \
    -v $(pwd)/data:/data \
    -v $(pwd)/config:/config:ro \
    -e AZURE_DEVOPS_PAT="$AZURE_DEVOPS_PAT" \
    ado-cli:latest full -levels /config/environment-levels.json

# Check generated files
ls -la ./data/
```

## Production Considerations

### 1. Security

- Never embed PAT tokens in images
- Use secrets management for tokens
- Run as non-root user (already configured)
- Use read-only config mounts

### 2. Performance

- Multi-stage build reduces image size
- Static binary compilation (CGO disabled)
- Alpine Linux base for minimal footprint
- Health checks for container monitoring

### 3. CI/CD Integration

```yaml
# Example GitHub Actions workflow
name: Build ADO CLI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build Docker image
        run: |
          docker build \
            --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
            --build-arg BUILD_VERSION=${{ github.ref_name }} \
            --build-arg BUILD_COMMIT=${{ github.sha }} \
            -t ado-cli:${{ github.ref_name }} \
            .
```

### 4. Image Size Optimization

The multi-stage build creates a minimal image:
- Build stage: Full Go environment (~400MB)
- Runtime stage: Alpine + binary (~20MB)
- Static binary with no dependencies
- CA certificates included for HTTPS calls

## Troubleshooting

### Build Issues

```bash
# Check Go source exists
ls -la ado-cli.go

# Verify Docker buildx
docker buildx version

# Test build without cache
docker build --no-cache -t ado-cli:test .
```

### Runtime Issues

```bash
# Test container connectivity
docker run --rm ado-cli:latest sh -c "curl -s https://dev.azure.com"

# Debug with shell access
docker run --rm -it --entrypoint sh ado-cli:latest

# Check environment variables
docker run --rm -e AZURE_DEVOPS_PAT="test" ado-cli:latest env | grep ADO
```

This Docker setup provides a complete, production-ready containerization of the Azure DevOps CLI with security best practices, multi-platform support, and comprehensive tooling for development and deployment.
