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
echo ""
echo "💡 Tip: Use the run.sh script for easier management"