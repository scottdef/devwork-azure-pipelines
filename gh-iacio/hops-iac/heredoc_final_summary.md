# HEREDOC Workflows - Complete Summary

GitHub Actions workflows for generating files using bash HEREDOC scripts, committing, pushing, and creating PRs.

## What You Have

### 🎯 Three Workflows

**1. Full-Featured Generation** (`generate-files.yml`)
- Target any repository
- Generate 7 example files
- Create PR automatically
- Update existing PRs

**2. Simple Generation** (`generate-files-simple.yml`)
- Works in current repo
- Direct commit to main
- No PR needed
- Quick and simple

**3. Cross-Repo Advanced** (`cross-repo-generate.yml`)
- Complete configuration suite
- Deployment scripts
- Health checks
- Documentation

### 📚 Supporting Files

**Documentation**:
- `HEREDOC-WORKFLOW-GUIDE.md` - Complete usage guide
- Examples and patterns
- Troubleshooting

**Tools**:
- `Makefile` - Command shortcuts
- `test-heredoc-generation.sh` - Local testing
- Multiple examples

## Quick Start

### Option 1: Use Full-Featured Workflow

```bash
# Via GitHub UI
Actions → Generate Files with HEREDOC → Run workflow
Fill in:
  - target_repo: your-org/your-repo
  - branch_name: generated-files
  - pr_title: Add generated files

# Via CLI
gh workflow run generate-files.yml \
  -f target_repo="your-org/your-repo" \
  -f branch_name="generated-files" \
  -f pr_title="Add generated files"
```

**Result**: PR created with 7 generated files

### Option 2: Use Simple Workflow

```bash
gh workflow run generate-files-simple.yml
```

**Result**: Files committed directly to main

### Option 3: Test Locally First

```bash
# Make script executable
chmod +x scripts/test-heredoc-generation.sh

# Run tests
./scripts/test-heredoc-generation.sh

# Review output
ls -la here-files-test/
```

**Result**: Local test files to verify HEREDOC works

### Option 4: Use Makefile

```bash
# Generate locally
make generate-local

# Generate in remote repo
make generate-remote TARGET_REPO=your-org/your-repo

# Generate with cross-repo workflow
make generate-cross OWNER=your-org REPO=your-repo

# Test HEREDOC syntax
make test-heredoc
```

## Files Generated

### Full-Featured Workflow Output

```
here-files/
├── README.md          # Documentation (Markdown)
├── config.json        # Configuration (JSON)
├── script.sh          # Example script (Bash, executable)
├── template.yml       # Template (YAML)
├── Makefile           # Build automation
├── GENERATED.md       # Generation docs
└── VERSION            # Version info
```

### Cross-Repo Workflow Output

```
here-files/
├── configs/
│   ├── .env.template       # Environment variables
│   ├── app-config.json     # App configuration
│   └── logging.yml         # Logging config
├── scripts/
│   ├── deploy.sh           # Deployment (executable)
│   ├── backup.sh           # Backup utility (executable)
│   └── health-check.sh     # Health checker (executable)
└── docs/
    ├── README.md           # Overview
    └── USAGE.md            # Usage guide
```

## How It Works

```
1. Workflow triggered
   ↓
2. Clone target repo
   ↓
3. Create/checkout branch
   ↓
4. Create here-files/ directory
   ↓
5. Generate files using HEREDOC:
   cat > file.txt <<'EOF'
   content here
   EOF
   ↓
6. Make scripts executable:
   chmod +x script.sh
   ↓
7. Add files to git:
   git add here-files/
   ↓
8. Commit changes:
   git commit -m "..."
   ↓
9. Push to branch:
   git push origin branch
   ↓
10. Create PR:
    gh pr create ...
```

## HEREDOC Patterns

### Pattern 1: No Variable Expansion

```yaml
- name: Generate File
  run: |
    cat > output.txt <<'EOF'
    Literal content: $HOME
    No expansion of: $(date)
    EOF
```

### Pattern 2: With Variable Expansion

```yaml
- name: Generate File
  run: |
    cat > output.txt <<EOF
    Current dir: $(pwd)
    Date: $(date)
    User: $USER
    Workflow: ${{ github.workflow }}
    EOF
```

### Pattern 3: Executable Script

```yaml
- name: Generate Script
  run: |
    cat > script.sh <<'EOF'
    #!/bin/bash
    echo "Hello, World!"
    EOF
    
    chmod +x script.sh
```

### Pattern 4: Multiple Files

```yaml
- name: Generate Multiple
  run: |
    for i in 1 2 3; do
      cat > "file${i}.txt" <<EOF
    This is file number $i
    Generated: $(date)
    EOF
    done
```

## Configuration

### Required Secrets

Add to repository secrets:

```
GH_ADMIN_TOKEN    # GitHub PAT with repo, workflow permissions
```

Or use default `GITHUB_TOKEN` for same-org operations.

### Workflow Permissions

Add to workflow:

```yaml
permissions:
  contents: write
  pull-requests: write
```

## Common Use Cases

### Use Case 1: Configuration Management

Generate environment-specific configs:

```yaml
- name: Generate Configs
  run: |
    for env in dev staging prod; do
      cat > "config-${env}.yml" <<EOF
    environment: $env
    debug: $([ "$env" = "dev" ] && echo true || echo false)
    log_level: $([ "$env" = "prod" ] && echo error || echo info)
    EOF
    done
```

### Use Case 2: Documentation Generation

Auto-generate docs from templates:

```yaml
- name: Generate Docs
  run: |
    cat > USAGE.md <<EOF
    # Usage Guide
    
    Generated: $(date)
    Version: ${{ github.ref_name }}
    
    ## Installation
    ...
    EOF
```

### Use Case 3: Script Distribution

Deploy scripts to multiple repos:

```yaml
- name: Generate Scripts
  run: |
    cat > deploy.sh <<'EOF'
    #!/bin/bash
    # Deployment script
    ...
    EOF
    chmod +x deploy.sh
```

### Use Case 4: CI/CD Configuration

Generate workflow files:

```yaml
- name: Generate Workflow
  run: |
    cat > .github/workflows/ci.yml <<'EOF'
    name: CI
    on: [push]
    jobs:
      build:
        runs-on: ubuntu-latest
        steps:
          - uses: actions/checkout@v4
    EOF
```

## Testing

### Local Testing

```bash
# Test HEREDOC syntax
./scripts/test-heredoc-generation.sh

# Or use Make
make test-heredoc

# Review output
ls -la here-files-test/
```

### Workflow Testing

```bash
# Dry run (use test repository)
make create-test-repo REPO_NAME=test-heredoc
make generate-remote TARGET_REPO=your-user/test-heredoc

# Check workflow
gh run list --workflow generate-files.yml

# View logs
gh run view --log
```

## Troubleshooting

### Issue: Variables Not Expanding

```yaml
# Wrong - single quotes prevent expansion
cat > file.txt <<'EOF'
User: $USER  # Literal $USER
EOF

# Right - no quotes or double quotes allow expansion
cat > file.txt <<EOF
User: $USER  # Expands to actual user
EOF
```

### Issue: Script Not Executable

```yaml
# Solution - always chmod after generating
cat > script.sh <<'EOF'
#!/bin/bash
echo "test"
EOF

chmod +x script.sh  # Don't forget!
```

### Issue: PR Already Exists

Workflow handles this automatically:
- Checks for existing PR
- Updates existing PR if found
- Creates new PR if not found

### Issue: No Changes Detected

Workflow checks before committing:

```yaml
if git diff --staged --quiet; then
  echo "No changes to commit"
  exit 0
fi
```

## Best Practices

1. **Test Locally First**
   - Use `test-heredoc-generation.sh`
   - Verify syntax before workflows

2. **Use Single Quotes** for literal content
   ```bash
   <<'EOF'  # No expansion
   ```

3. **Use Double Quotes/No Quotes** for variables
   ```bash
   <<EOF    # Variable expansion
   ```

4. **Always chmod +x** for scripts
   ```bash
   chmod +x script.sh
   ```

5. **Validate Generated Files**
   ```bash
   jq empty config.json  # Validate JSON
   ```

6. **Check for Changes**
   ```bash
   git diff --staged --quiet
   ```

7. **Use Meaningful Names**
   - Branches: `generated-configs` not `temp`
   - Files: `app-config.json` not `config.json`

8. **Document Generated Files**
   - Include README in generated folder
   - Explain what files are for

9. **Version Generated Content**
   - Include timestamp
   - Add workflow run ID
   - Track generation source

10. **Review Before Merge**
    - Always review generated PRs
    - Verify no sensitive data
    - Check file permissions

## Advanced Patterns

### Pattern 1: Template Engine

```yaml
- name: Advanced Template
  run: |
    VARS=(
      "APP_NAME:MyApp"
      "VERSION:1.0.0"
      "AUTHOR:${{ github.actor }}"
    )
    
    cat > config.txt <<EOF
    $(for var in "${VARS[@]}"; do
      IFS=':' read -r key val <<< "$var"
      echo "$key=$val"
    done)
    EOF
```

### Pattern 2: Include External Data

```yaml
- name: Generate from API
  run: |
    DATA=$(curl -s https://api.example.com/data)
    
    cat > output.json <<EOF
    {
      "fetched_at": "$(date)",
      "data": $DATA
    }
    EOF
```

### Pattern 3: Conditional Content

```yaml
- name: Environment Specific
  run: |
    if [ "${{ github.ref }}" = "refs/heads/main" ]; then
      ENV="production"
      DEBUG="false"
    else
      ENV="development"
      DEBUG="true"
    fi
    
    cat > config.env <<EOF
    ENVIRONMENT=$ENV
    DEBUG=$DEBUG
    EOF
```

## Security

**✅ Do**:
- Use templates for sensitive data
- Validate generated content
- Review PRs before merge
- Use .gitignore for secrets
- Check file permissions

**❌ Don't**:
- Include secrets in HEREDOC
- Commit actual passwords
- Skip PR review
- Use root permissions
- Ignore validation errors

## Integration

### With IssueOps

```yaml
on:
  issues:
    types: [labeled]
  
jobs:
  generate:
    if: contains(github.event.issue.labels.*.name, 'generate-files')
    # ... generate files based on issue
```

### With Terraform

```yaml
- name: Generate Terraform Config
  run: |
    cat > terraform.tfvars <<EOF
    environment = "${{ github.ref_name }}"
    region = "us-east-1"
    EOF
```

### With Other Workflows

```yaml
- name: Trigger Other Workflow
  run: |
    gh workflow run other-workflow.yml \
      -f config_file="$(cat config.json)"
```

## Monitoring

```bash
# List workflow runs
make list-workflows

# Watch workflows
make watch-workflows

# View specific run
make view-workflow RUN_ID=123456

# List generated PRs
make approve-prs
```

## Resources

**Files**:
- `.github/workflows/generate-files.yml`
- `.github/workflows/generate-files-simple.yml`
- `.github/workflows/cross-repo-generate.yml`
- `scripts/test-heredoc-generation.sh`
- `Makefile`
- `HEREDOC-WORKFLOW-GUIDE.md`

**Commands**:
```bash
make help                    # Show all commands
make generate-local          # Test locally
make generate-remote         # Trigger workflow
make test-heredoc            # Test syntax
make verify-setup            # Check setup
```

**Documentation**:
- Full guide: `HEREDOC-WORKFLOW-GUIDE.md`
- This summary: `HEREDOC-WORKFLOWS-SUMMARY.md`

## Examples

See workflow files for complete examples:
- 7 file types in `generate-files.yml`
- Simple 3-file example in `generate-files-simple.yml`
- Complete config suite in `cross-repo-generate.yml`

---

**Built in the Unix tradition**: Simple tools, composable patterns, text as interface. HEREDOC is just another pipe in the toolchain.

**Ship it.** 🚀
