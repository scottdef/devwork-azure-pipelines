# HEREDOC Workflow Cheat Sheet

Quick reference for GitHub Actions workflows with HEREDOC file generation.

## Basic Syntax

### No Variable Expansion (Literal)

```yaml
- name: Generate File
  run: |
    cat > output.txt <<'EOF'
    Literal content
    $HOME will not expand
    $(date) will not execute
    EOF
```

### With Variable Expansion

```yaml
- name: Generate File
  run: |
    cat > output.txt <<EOF
    Current directory: $(pwd)
    User: $USER
    Date: $(date)
    Workflow: ${{ github.workflow }}
    EOF
```

### Indented HEREDOC (Strip Leading Tabs)

```yaml
- name: Generate File
  run: |
    cat > output.txt <<-EOF
    	This line has a leading tab
    	It will be stripped
    EOF
```

## Common File Types

### JSON

```yaml
- name: Generate JSON
  run: |
    cat > config.json <<'EOF'
    {
      "key": "value",
      "number": 123,
      "array": [1, 2, 3],
      "nested": {
        "item": true
      }
    }
    EOF
    
    # Validate
    jq empty config.json
```

### YAML

```yaml
- name: Generate YAML
  run: |
    cat > config.yml <<'EOF'
    version: 1.0
    settings:
      enabled: true
      timeout: 30
    items:
      - name: item1
        value: 100
    EOF
```

### Bash Script

```yaml
- name: Generate Script
  run: |
    cat > script.sh <<'EOF'
    #!/bin/bash
    set -euo pipefail
    
    echo "Hello, World!"
    EOF
    
    chmod +x script.sh
```

### Markdown

```yaml
- name: Generate Markdown
  run: |
    cat > README.md <<EOF
    # Project Name
    
    Generated: $(date)
    
    ## Quick Start
    
    \`\`\`bash
    npm install
    \`\`\`
    EOF
```

### Makefile

```yaml
- name: Generate Makefile
  run: |
    cat > Makefile <<'EOF'
    .PHONY: help build test
    
    help:
    	@echo "Available targets:"
    
    build:
    	@echo "Building..."
    
    test:
    	@echo "Testing..."
    EOF
```

### Dockerfile

```yaml
- name: Generate Dockerfile
  run: |
    cat > Dockerfile <<'EOF'
    FROM node:20-alpine
    
    WORKDIR /app
    
    COPY package*.json ./
    RUN npm ci
    
    COPY . .
    
    EXPOSE 3000
    CMD ["node", "index.js"]
    EOF
```

### Environment File

```yaml
- name: Generate .env
  run: |
    cat > .env.template <<'EOF'
    # Application Settings
    APP_NAME=myapp
    APP_ENV=production
    
    # Database
    DB_HOST=localhost
    DB_PORT=5432
    DB_NAME=
    DB_USER=
    DB_PASS=
    EOF
```

## Git Operations

### Commit & Push

```yaml
- name: Commit and Push
  run: |
    git config user.name "github-actions[bot]"
    git config user.email "github-actions[bot]@users.noreply.github.com"
    
    git add .
    git commit -m "Add generated files"
    git push
```

### Create Branch & Push

```yaml
- name: Create Branch
  run: |
    git checkout -b feature-branch
    # ... generate files ...
    git add .
    git commit -m "Add files"
    git push origin feature-branch
```

### Create PR

```yaml
- name: Create PR
  run: |
    gh pr create \
      --title "Add generated files" \
      --body "Automated file generation" \
      --base main \
      --head feature-branch
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Force Push (Replace Branch)

```yaml
- name: Force Push
  run: |
    git push origin --delete branch-name || true
    git checkout -b branch-name
    # ... generate files ...
    git push origin branch-name --force
```

## Patterns

### Multiple Files Loop

```yaml
- name: Generate Multiple Files
  run: |
    for i in 1 2 3; do
      cat > "file${i}.txt" <<EOF
    File number $i
    Generated: $(date)
    EOF
    done
```

### Conditional Generation

```yaml
- name: Generate Based on Condition
  run: |
    if [ "${{ github.ref }}" = "refs/heads/main" ]; then
      cat > config.env <<'EOF'
    ENV=production
    DEBUG=false
    EOF
    else
      cat > config.env <<'EOF'
    ENV=development
    DEBUG=true
    EOF
    fi
```

### Environment-Specific Files

```yaml
- name: Generate Per Environment
  run: |
    for env in dev staging prod; do
      cat > "config-${env}.yml" <<EOF
    environment: $env
    debug: $([ "$env" = "dev" ] && echo true || echo false)
    log_level: $([ "$env" = "prod" ] && echo error || echo info)
    EOF
    done
```

### With Template Variables

```yaml
- name: Generate from Template
  run: |
    APP_NAME="MyApp"
    VERSION="1.0.0"
    
    cat > config.txt <<EOF
    Application: $APP_NAME
    Version: $VERSION
    Built: $(date)
    Commit: ${{ github.sha }}
    EOF
```

### Read and Transform

```yaml
- name: Transform Existing File
  run: |
    CONTENT=$(cat existing.txt)
    
    cat > transformed.txt <<EOF
    Original:
    $CONTENT
    
    Uppercase: $(echo "$CONTENT" | tr 'a-z' 'A-Z')
    EOF
```

## Validation

### Check for Changes

```yaml
- name: Commit if Changed
  run: |
    git add .
    
    if git diff --staged --quiet; then
      echo "No changes"
      exit 0
    fi
    
    git commit -m "Update"
```

### Validate JSON

```yaml
- name: Validate JSON
  run: |
    for f in *.json; do
      if ! jq empty "$f"; then
        echo "Invalid JSON: $f"
        exit 1
      fi
    done
```

### Validate YAML

```yaml
- name: Validate YAML
  run: |
    pip install pyyaml
    
    python3 <<'PY'
    import yaml
    import sys
    
    try:
        with open('config.yml') as f:
            yaml.safe_load(f)
        print("✓ Valid YAML")
    except:
        print("✗ Invalid YAML")
        sys.exit(1)
    PY
```

### Check File Exists

```yaml
- name: Verify Files
  run: |
    for f in file1.txt file2.json; do
      if [ ! -f "$f" ]; then
        echo "Missing: $f"
        exit 1
      fi
    done
```

## Error Handling

### Continue on Error

```yaml
- name: Generate
  run: |
    cat > file.txt <<'EOF'
    content
    EOF
  continue-on-error: true
```

### With Rollback

```yaml
- name: Generate with Rollback
  run: |
    cp -r files/ files.backup/
    
    if ! ./generate.sh; then
      echo "Failed, rolling back"
      rm -rf files/
      mv files.backup/ files/
      exit 1
    fi
    
    rm -rf files.backup/
```

### Cleanup on Failure

```yaml
- name: Generate
  run: |
    # generation code
  
- name: Cleanup on Failure
  if: failure()
  run: |
    rm -rf temp-files/
```

## Workflow Triggers

### Manual Dispatch

```yaml
on:
  workflow_dispatch:
    inputs:
      target_repo:
        description: 'Target repository'
        required: true
```

### On Push

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'src/**'
      - '.github/workflows/*.yml'
```

### Scheduled

```yaml
on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly
```

### On PR

```yaml
on:
  pull_request:
    types: [opened, synchronize]
```

## Secrets & Variables

### Use Secrets

```yaml
- name: Use Secret
  run: |
    cat > config.txt <<EOF
    API_KEY=${{ secrets.API_KEY }}
    EOF
```

### Use Repository Variables

```yaml
- name: Use Variables
  run: |
    cat > config.txt <<EOF
    APP_NAME=${{ vars.APP_NAME }}
    ENVIRONMENT=${{ vars.ENVIRONMENT }}
    EOF
```

### Environment Variables

```yaml
env:
  APP_NAME: myapp
  VERSION: 1.0.0

jobs:
  build:
    steps:
      - name: Generate
        run: |
          cat > config.txt <<EOF
    App: $APP_NAME
    Version: $VERSION
    EOF
```

## Quick Commands

### Run Workflow

```bash
# Via gh CLI
gh workflow run workflow-name.yml

# With inputs
gh workflow run workflow-name.yml \
  -f input1=value1 \
  -f input2=value2
```

### List Runs

```bash
gh run list --workflow workflow-name.yml
```

### View Run

```bash
gh run view <run-id> --log
```

### Watch Run

```bash
gh run watch
```

## Common Gotchas

### ❌ Variables Not Expanding

```yaml
# Wrong
cat > file.txt <<'EOF'
User: $USER  # Literal $USER
EOF

# Right
cat > file.txt <<EOF
User: $USER  # Expands
EOF
```

### ❌ Forgot chmod +x

```yaml
# Wrong
cat > script.sh <<'EOF'
#!/bin/bash
echo "test"
EOF

# Right
cat > script.sh <<'EOF'
#!/bin/bash
echo "test"
EOF
chmod +x script.sh
```

### ❌ Indentation Issues

```yaml
# Be careful with YAML indentation
- name: Generate
  run: |
    cat > file.txt <<'EOF'
    Content must be aligned properly
    EOF
```

### ❌ Special Characters

```yaml
# Use different delimiter if content has 'EOF'
cat > file.txt <<'DELIMITER'
This content contains 'EOF' in it
DELIMITER
```

## Best Practices

1. ✅ Use `<<'EOF'` for literal content
2. ✅ Use `<<EOF` when you need variable expansion
3. ✅ Always `chmod +x` scripts
4. ✅ Validate generated files (JSON, YAML)
5. ✅ Check for changes before committing
6. ✅ Use meaningful commit messages
7. ✅ Add descriptive PR bodies
8. ✅ Test locally before CI
9. ✅ Clean up branches after merge
10. ✅ Document what files are generated

## Example Workflow Template

```yaml
name: Generate Files

on:
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  generate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Generate
        run: |
          mkdir -p output
          
          cat > output/file.txt <<'EOF'
          Generated content
          EOF
      
      - name: Commit
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add output/
          git commit -m "Generate files"
          git push
```

---

**Pro Tip**: Test HEREDOC locally before adding to workflows:

```bash
cat > test.txt <<'EOF'
Your content
EOF

cat test.txt  # Verify it worked
```
