#!/bin/bash
# test-heredoc-generation.sh - Test HEREDOC file generation locally
# Run this before using workflows to verify your HEREDOC scripts work

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Counters
PASSED=0
FAILED=0

# Functions
info() { echo -e "${BLUE}ℹ${NC} $*"; }
success() { echo -e "${GREEN}✓${NC} $*"; ((PASSED++)); }
fail() { echo -e "${RED}✗${NC} $*"; ((FAILED++)); }
warn() { echo -e "${YELLOW}⚠${NC} $*"; }
section() { echo -e "\n${BOLD}${BLUE}▶ $*${NC}\n"; }

# Setup
OUTPUT_DIR="${1:-./here-files-test}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

cleanup() {
    if [ -d "$OUTPUT_DIR" ]; then
        rm -rf "$OUTPUT_DIR"
        info "Cleaned up test directory"
    fi
}

trap cleanup EXIT

# Main
main() {
    echo ""
    echo -e "${BOLD}════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  HEREDOC Generation Test Suite${NC}"
    echo -e "${BOLD}════════════════════════════════════════════${NC}"
    echo ""
    
    info "Output directory: $OUTPUT_DIR"
    info "Timestamp: $TIMESTAMP"
    echo ""
    
    # Create output directory
    mkdir -p "$OUTPUT_DIR"
    cd "$OUTPUT_DIR"
    
    # Run tests
    test_basic_heredoc
    test_heredoc_with_expansion
    test_json_generation
    test_yaml_generation
    test_script_generation
    test_makefile_generation
    test_markdown_generation
    test_config_files
    test_multi_file_generation
    test_indented_heredoc
    
    # Summary
    echo ""
    echo -e "${BOLD}════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  Test Results${NC}"
    echo -e "${BOLD}════════════════════════════════════════════${NC}"
    echo ""
    echo -e "${GREEN}Passed: $PASSED${NC}"
    echo -e "${RED}Failed: $FAILED${NC}"
    echo ""
    
    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        echo ""
        info "Generated files are in: $OUTPUT_DIR"
        info "Review them before using in workflows"
        echo ""
        return 0
    else
        echo -e "${RED}✗ Some tests failed${NC}"
        echo ""
        return 1
    fi
}

# Test 1: Basic HEREDOC
test_basic_heredoc() {
    section "Test 1: Basic HEREDOC (no expansion)"
    
    cat > basic.txt <<'EOF'
This is a basic HEREDOC.
Variables like $HOME are not expanded.
Special characters: $ @ * ? are literal.
EOF
    
    if [ -f basic.txt ]; then
        success "Created basic.txt"
        
        if grep -q "\$HOME" basic.txt; then
            success "Variables not expanded (correct)"
        else
            fail "Variables were expanded (incorrect)"
        fi
    else
        fail "Failed to create basic.txt"
    fi
}

# Test 2: HEREDOC with expansion
test_heredoc_with_expansion() {
    section "Test 2: HEREDOC with Variable Expansion"
    
    cat > expanded.txt <<EOF
Current directory: $(pwd)
User: ${USER:-unknown}
Date: $(date)
Shell: $SHELL
EOF
    
    if [ -f expanded.txt ]; then
        success "Created expanded.txt"
        
        if grep -q "$(pwd)" expanded.txt; then
            success "Variables expanded (correct)"
        else
            fail "Variables not expanded (incorrect)"
        fi
    else
        fail "Failed to create expanded.txt"
    fi
}

# Test 3: JSON generation
test_json_generation() {
    section "Test 3: JSON File Generation"
    
    cat > config.json <<'EOF'
{
  "name": "test-app",
  "version": "1.0.0",
  "description": "Generated JSON config",
  "settings": {
    "debug": false,
    "timeout": 30,
    "retries": 3
  },
  "features": {
    "feature_a": true,
    "feature_b": false
  },
  "metadata": {
    "generated": true,
    "format": "json"
  }
}
EOF
    
    if [ -f config.json ]; then
        success "Created config.json"
        
        # Validate JSON
        if command -v jq >/dev/null 2>&1; then
            if jq empty config.json 2>/dev/null; then
                success "Valid JSON syntax"
            else
                fail "Invalid JSON syntax"
            fi
        else
            warn "jq not installed, skipping JSON validation"
        fi
    else
        fail "Failed to create config.json"
    fi
}

# Test 4: YAML generation
test_yaml_generation() {
    section "Test 4: YAML File Generation"
    
    cat > config.yml <<'EOF'
version: 1.0
metadata:
  name: test-config
  description: Generated YAML config
  
settings:
  environment: production
  debug: false
  
services:
  - name: api
    port: 3000
    replicas: 3
    
  - name: worker
    port: 3001
    replicas: 2
    
resources:
  cpu: 2
  memory: 4096
  storage: 100
EOF
    
    if [ -f config.yml ]; then
        success "Created config.yml"
        
        # Basic YAML check
        if grep -q "version: 1.0" config.yml; then
            success "YAML content present"
        else
            fail "YAML content missing"
        fi
    else
        fail "Failed to create config.yml"
    fi
}

# Test 5: Script generation
test_script_generation() {
    section "Test 5: Executable Script Generation"
    
    cat > test-script.sh <<'EOF'
#!/bin/bash
# Generated test script

set -euo pipefail

echo "This is a generated script"
echo "Arguments: $@"
echo "Script path: $0"

for arg in "$@"; do
    echo "Processing: $arg"
done

exit 0
EOF
    
    chmod +x test-script.sh
    
    if [ -f test-script.sh ]; then
        success "Created test-script.sh"
        
        if [ -x test-script.sh ]; then
            success "Script is executable"
            
            # Test execution
            if ./test-script.sh test 2>/dev/null | grep -q "Processing: test"; then
                success "Script executes correctly"
            else
                fail "Script execution failed"
            fi
        else
            fail "Script is not executable"
        fi
    else
        fail "Failed to create test-script.sh"
    fi
}

# Test 6: Makefile generation
test_makefile_generation() {
    section "Test 6: Makefile Generation"
    
    cat > Makefile <<'EOF'
# Generated Makefile

.PHONY: help all build test clean

help:
	@echo "Available targets:"
	@echo "  build - Build project"
	@echo "  test  - Run tests"
	@echo "  clean - Clean artifacts"

build:
	@echo "Building..."

test:
	@echo "Testing..."

clean:
	@echo "Cleaning..."

.DEFAULT_GOAL := help
EOF
    
    if [ -f Makefile ]; then
        success "Created Makefile"
        
        # Test make
        if command -v make >/dev/null 2>&1; then
            if make help 2>/dev/null | grep -q "Available targets"; then
                success "Makefile is valid"
            else
                fail "Makefile validation failed"
            fi
        else
            warn "make not installed, skipping validation"
        fi
    else
        fail "Failed to create Makefile"
    fi
}

# Test 7: Markdown generation
test_markdown_generation() {
    section "Test 7: Markdown Documentation Generation"
    
    cat > README.md <<EOF
# Generated Documentation

Generated at: $(date)

## Overview

This is a test of markdown generation using HEREDOC.

## Features

- Feature 1
- Feature 2
- Feature 3

## Usage

\`\`\`bash
./script.sh --help
\`\`\`

## Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| timeout | 30 | Request timeout |
| retries | 3 | Retry attempts |

## Links

- [Documentation](https://docs.example.com)
- [Repository](https://github.com/example/repo)

Generated by: Test Script
Timestamp: $TIMESTAMP
EOF
    
    if [ -f README.md ]; then
        success "Created README.md"
        
        if grep -q "# Generated Documentation" README.md; then
            success "Markdown content present"
        fi
    else
        fail "Failed to create README.md"
    fi
}

# Test 8: Multiple config files
test_config_files() {
    section "Test 8: Multiple Configuration Files"
    
    mkdir -p configs
    
    # .env template
    cat > configs/.env.template <<'EOF'
# Environment Variables Template

# Application
APP_NAME=myapp
APP_ENV=production
APP_DEBUG=false

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=
DB_USER=
DB_PASS=

# API Keys
API_KEY=
SECRET_KEY=
EOF
    
    # App config
    cat > configs/app.conf <<'EOF'
[server]
host = 0.0.0.0
port = 3000
workers = 4

[database]
pool_size = 10
timeout = 5000

[cache]
enabled = true
ttl = 3600
EOF
    
    # Logging config
    cat > configs/logging.conf <<'EOF'
[loggers]
keys=root,app

[handlers]
keys=console,file

[formatters]
keys=simple,detailed

[logger_root]
level=WARNING
handlers=console

[logger_app]
level=INFO
handlers=console,file
qualname=app
EOF
    
    if [ -d configs ] && [ -f configs/.env.template ]; then
        success "Created multiple config files"
        
        local count
        count=$(find configs -type f | wc -l)
        success "Generated $count configuration files"
    else
        fail "Failed to create config files"
    fi
}

# Test 9: Multi-file generation loop
test_multi_file_generation() {
    section "Test 9: Loop-based Multi-File Generation"
    
    mkdir -p envs
    
    for env in dev staging prod; do
        cat > "envs/${env}.env" <<EOF
# Environment: $env
ENV_NAME=$env
TIMESTAMP=$(date)
DEBUG=$([ "$env" = "dev" ] && echo "true" || echo "false")
LOG_LEVEL=$([ "$env" = "prod" ] && echo "error" || echo "info")
EOF
    done
    
    if [ -d envs ]; then
        local count
        count=$(find envs -type f | wc -l)
        
        if [ "$count" -eq 3 ]; then
            success "Generated $count environment files"
        else
            fail "Expected 3 files, got $count"
        fi
    else
        fail "Failed to create environment files"
    fi
}

# Test 10: Indented HEREDOC
test_indented_heredoc() {
    section "Test 10: Indented HEREDOC (<<-)"
    
    cat > indented.txt <<-'EOF'
		This is indented with tabs.
		The leading tabs will be stripped.
		But the relative indentation is preserved.
			This line has extra indentation.
		Back to normal.
	EOF
    
    if [ -f indented.txt ]; then
        success "Created indented.txt"
        
        # Check if tabs were stripped
        if ! grep -P '^\t' indented.txt >/dev/null 2>&1; then
            success "Leading tabs stripped (correct)"
        else
            warn "Leading tabs present (may be correct depending on shell)"
        fi
    else
        fail "Failed to create indented.txt"
    fi
}

# Run main
main "$@"
