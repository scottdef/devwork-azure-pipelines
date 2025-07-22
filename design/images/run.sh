#!/bin/bash

# Azure DevOps Deployment Records Processor - Quick Start Script

set -e  # Exit on any error

echo "Azure DevOps Deployment Records Processor"
echo "=========================================="

# Check if PAT is set
if [ -z "$AZURE_DEVOPS_PAT" ]; then
    echo "❌ Error: AZURE_DEVOPS_PAT environment variable is not set"
    echo ""
    echo "Please set your Personal Access Token:"
    echo "  export AZURE_DEVOPS_PAT=\"your-pat-token-here\""
    echo ""
    echo "To get a PAT token:"
    echo "  1. Go to Azure DevOps → User Settings → Personal Access Tokens"
    echo "  2. Create token with 'Environment (Read)' scope"
    echo "  3. Copy the token value"
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed or not in PATH"
    echo "Please install Go 1.21 or later from https://golang.org/dl/"
    exit 1
fi

echo "✅ Go version: $(go version)"
echo "✅ Azure DevOps PAT is set"
echo ""

# Check if source files exist
if [ ! -f "azure-devops-processor.go" ]; then
    echo "❌ Error: azure-devops-processor.go not found"
    echo "Please ensure all script files are in the current directory"
    exit 1
fi

if [ ! -f "deploy-summary.go" ]; then
    echo "❌ Error: deploy-summary.go not found"
    echo "Please ensure all script files are in the current directory"
    exit 1
fi

echo "✅ Source files found"
echo ""

# Clean up previous runs
echo "🧹 Cleaning up previous output files..."
rm -f deploy-summary env-dep-rec-res-*.json deploy-summary-*.json
echo ""

# Run the processor
echo "🚀 Starting Azure DevOps Deployment Records Processor..."
echo ""

if go run azure-devops-processor.go; then
    echo ""
    echo "🎉 Processing completed successfully!"
    echo ""

    # Show generated files
    echo "📄 Generated files:"
    ls -la deploy-summary-*.json 2>/dev/null || echo "  No summary files found"
    ls -la env-dep-rec-res-*.json 2>/dev/null || echo "  No API response files found"

else
    echo ""
    echo "❌ Processing failed. Check the error messages above."
    exit 1
fi

echo ""
echo "✨ Done! Check the generated JSON files for your deployment summaries."
