# HEREDOC Workflow Guide

Complete guide for GitHub Actions workflows that generate files using bash HEREDOC scripts.

## Overview

Three workflows provided:

1. **generate-files.yml** - Full-featured with target repo support
2. **generate-files-simple.yml** - Simple version for current repo
3. **cross-repo-generate.yml** - Advanced cross-repository generation

## Workflow 1: Full-Featured Generation

**File**: `.github/workflows/generate-files.yml`

### Features

- ✅ Clone any target repository
- ✅ Create `here-files/` directory
- ✅ Generate 7 different file types
- ✅ Add, commit, push to branch
- ✅ Create pull request automatically
- ✅ Update existing PR if branch exists

### Usage

```bash
# Via GitHub UI
Actions → Generate Files with HEREDOC → Run workflow

# Or via gh CLI
gh workflow run generate-files.yml \
  -f target_repo="your-org/your-repo" \
  -f branch_name="generated-files" \
  -f pr_title="Add generated files"
```

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| target_repo | Yes | - | Repository to target (owner/repo) |
| branch_name | No | generated-files | Branch name for changes |
| pr_title | No | Add generated files | Title for pull request |

### Generated Files

```
here-files/
├── README.md          # Documentation
├── config.json        # JSON configuration
├── script.sh          # Bash script (executable)
├── template.yml       # YAML template
├── Makefile           # Build automation
├── GENERATED.md       # Generation docs
└── VERSION            # Version info
```

### Required Secrets

- `GH_ADMIN_TOKEN` - GitHub Personal Access Token with:
  - `repo` - Full control of repositories
  - `workflow` - Update workflows
  
Or use default `GITHUB_TOKEN` for same-org repos.

### How It Works

```
1. Checkout target repo with token
2. Create branch (force push if exists)
3. Create here-files/ directory
4. Generate files using HEREDOC:
   - cat > file <<'EOF'
     content here
     EOF
5. Add files: git add here-files/
6. Commit: git commit -m "..."
7. Push: git push origin branch
8. Create PR via GitHub API
```

## Workflow 2: Simple Generation

**File**: `.github/workflows/generate-files-simple.yml`

### Features

- ✅ Generates files in current repository
- ✅ Commits directly to main
- ✅ Runs on workflow file changes
- ✅ No PR creation (direct push)

### Usage

Automatic on push to workflow file, or:

```bash
gh workflow run generate-files-simple.yml
```

### When to Use

- Quick file generation in current repo
- No need for review process
- Simple configuration files
- Testing HEREDOC scripts

### Generated Files

```
here-files/
├── config.sh          # Shell configuration
├── data.txt           # Text data file
└── template.html      # HTML template
```

## Workflow 3: Cross-Repo Advanced

**File**: `.github/workflows/cross-repo-generate.yml`

### Features

- ✅ Advanced configuration generation
- ✅ Multiple file categories
- ✅ Complete deployment scripts
- ✅ Comprehensive documentation
- ✅ Uses gh CLI for PR creation

### Usage

```bash
gh workflow run cross-repo-generate.yml \
  -f owner="your-org" \
  -f repo="target-repo" \
  -f branch="auto-generated-files"
```

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| owner | Yes | - | Repository owner |
| repo | Yes | - | Repository name |
| branch | No | auto-generated-files | Branch name |

### Generated Structure

```
here-files/
├── configs/
│   ├── .env.template       # Environment variables
│   ├── app-config.json     # App configuration
│   └── logging.yml         # Logging config
├── scripts/
│   ├── deploy.sh           # Deployment script
│   ├── backup.sh           # Backup utility
│   └── health-check.sh     # Health checker
└── docs/
    ├── README.md           # Overview
    └── USAGE.md            # Usage guide
```

## HEREDOC Syntax Reference

### Basic HEREDOC

```yaml
- name: Generate File
  run: |
    cat > output.txt <<'EOF'
    This is the file content.
    Variables like $HOME are NOT expanded.
    EOF
```

### HEREDOC with Variable Expansion

```yaml
- name: Generate File with Variables
  run: |
    cat > output.txt <<EOF
    Current directory: $(pwd)
    User: $USER
    Date: $(date)
    GitHub Actor: ${{ github.actor }}
    EOF
```

### Multi-File Generation

```yaml
- name: Generate Multiple Files
  run: |
    # File 1
    cat > file1.txt <<'EOF'
    Content for file 1
    EOF
    
    # File 2
    cat > file2.txt <<'EOF'
    Content for file 2
    EOF
    
    # File 3
    cat > file3.txt <<'EOF'
    Content for file 3
    EOF
```

### Executable Scripts

```yaml
- name: Generate Script
  run: |
    cat > script.sh <<'EOF'
    #!/bin/bash
    echo "This is executable"
    EOF
    
    chmod +x script.sh
```

### JSON Generation

```yaml
- name: Generate JSON
  run: |
    cat > config.json <<'EOF'
    {
      "key": "value",
      "array": [1, 2, 3],
      "nested": {
        "item": true
      }
    }
    EOF
```

### YAML Generation

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
      - name: item2
        value: 200
    EOF
```

## Common Patterns

### Pattern 1: Template with Variables

```yaml
- name: Generate Config
  run: |
    cat > config.sh <<EOF
    # Generated: $(date)
    # Workflow: ${{ github.workflow }}
    # Run ID: ${{ github.run_id }}
    
    export APP_VERSION="1.0.0"
    export BUILD_DATE="$(date -u +%Y-%m-%d)"
    export COMMIT_SHA="${{ github.sha }}"
    EOF
```

### Pattern 2: Conditional Generation

```yaml
- name: Generate Environment Specific Config
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

### Pattern 3: Loop Generation

```yaml
- name: Generate Multiple Configs
  run: |
    for env in dev staging prod; do
      cat > "config-${env}.yml" <<EOF
    environment: $env
    timestamp: $(date)
    EOF
    done
```

### Pattern 4: Read and Transform

```yaml
- name: Transform File
  run: |
    # Read existing file
    content=$(cat existing.txt)
    
    # Generate new file with transformation
    cat > output.txt <<EOF
    Original content:
    $content
    
    Transformed: $(echo "$content" | tr 'a-z' 'A-Z')
    EOF
```

## Git Operations

### Basic Commit Flow

```yaml
- name: Commit Changes
  run: |
    git config user.name "github-actions[bot]"
    git config user.email "github-actions[bot]@users.noreply.github.com"
    
    git add .
    git commit -m "Add generated files"
    git push
```

### Branch and PR Flow

```yaml
- name: Create Branch and PR
  run: |
    # Create branch
    git checkout -b feature-branch
    
    # Generate files
    cat > file.txt <<'EOF'
    content
    EOF
    
    # Commit
    git add file.txt
    git commit -m "Add file"
    
    # Push
    git push origin feature-branch
    
    # Create PR
    gh pr create \
      --title "Add generated file" \
      --body "Automated file generation" \
      --base main
```

### Force Push Pattern

```yaml
- name: Force Push Branch
  run: |
    # Delete remote branch
    git push origin --delete my-branch || true
    
    # Create new branch
    git checkout -b my-branch
    
    # Generate and commit
    # ...
    
    # Force push
    git push origin my-branch --force
```

## Error Handling

### Check for Changes

```yaml
- name: Commit if Changed
  run: |
    git add .
    
    if git diff --staged --quiet; then
      echo "No changes to commit"
      exit 0
    fi
    
    git commit -m "Update files"
    git push
```

### Validate Generated Files

```yaml
- name: Generate and Validate
  run: |
    # Generate
    cat > config.json <<'EOF'
    {"key": "value"}
    EOF
    
    # Validate JSON
    if ! jq empty config.json 2>/dev/null; then
      echo "Invalid JSON generated"
      exit 1
    fi
    
    echo "✓ Valid JSON"
```

### Rollback on Error

```yaml
- name: Generate with Rollback
  run: |
    # Backup existing files
    cp -r here-files/ here-files.backup/
    
    # Try to generate
    if ! ./generate-script.sh; then
      echo "Generation failed, rolling back"
      rm -rf here-files/
      mv here-files.backup/ here-files/
      exit 1
    fi
    
    # Clean up backup
    rm -rf here-files.backup/
```

## Advanced Techniques

### Template Engine

```yaml
- name: Advanced Template
  run: |
    # Define variables
    APP_NAME="MyApp"
    VERSION="1.0.0"
    AUTHOR="${{ github.actor }}"
    
    # Generate with substitution
    cat > output.txt <<EOF
    Application: $APP_NAME
    Version: $VERSION
    Author: $AUTHOR
    Generated: $(date)
    Commit: ${{ github.sha }}
    EOF
```

### Include External Data

```yaml
- name: Generate from API
  run: |
    # Fetch data
    DATA=$(curl -s https://api.example.com/data)
    
    # Generate file with data
    cat > data-file.json <<EOF
    {
      "fetched_at": "$(date)",
      "data": $DATA
    }
    EOF
```

### Multi-Stage Generation

```yaml
- name: Multi-Stage Generation
  run: |
    # Stage 1: Base config
    cat > base.yml <<'EOF'
    base: true
    EOF
    
    # Stage 2: Read base and extend
    BASE=$(cat base.yml)
    cat > extended.yml <<EOF
    $BASE
    extended: true
    timestamp: $(date)
    EOF
    
    # Stage 3: Final processing
    cat > final.yml <<EOF
    $(cat extended.yml)
    finalized: true
    EOF
```

## Troubleshooting

### Issue: Variables Not Expanding

**Problem**: Using single quotes prevents expansion

```yaml
# Wrong
cat > file.txt <<'EOF'
User: $USER  # Will be literal $USER
EOF

# Right
cat > file.txt <<EOF
User: $USER  # Will expand to actual user
EOF
```

### Issue: Indentation Problems

**Problem**: HEREDOC content has wrong indentation

```yaml
# Solution: Use <<-EOF to strip leading tabs
- name: Generate
  run: |
    cat > file.txt <<-EOF
    	This line has leading tab
    	It will be stripped
    EOF
```

### Issue: Special Characters

**Problem**: Special characters causing issues

```yaml
# Solution: Use different delimiter
- name: Generate
  run: |
    cat > file.txt <<'DELIMITER'
    This can contain 'EOF' and other special chars
    DELIMITER
```

### Issue: File Permissions

**Problem**: Script not executable after generation

```yaml
# Solution: Always chmod after generating scripts
- name: Generate Script
  run: |
    cat > script.sh <<'EOF'
    #!/bin/bash
    echo "hello"
    EOF
    
    chmod +x script.sh  # Don't forget this!
```

## Best Practices

1. **Use Single Quotes** for literal content: `<<'EOF'`
2. **Use Double Quotes** when you need variable expansion: `<<EOF`
3. **Always chmod +x** after generating shell scripts
4. **Validate JSON/YAML** after generation
5. **Check for changes** before committing
6. **Use meaningful branch names** for PRs
7. **Add descriptive commit messages**
8. **Clean up** old branches after merge
9. **Test locally** before running in CI
10. **Document** what files are generated and why

## Testing Locally

Test your HEREDOC scripts locally before adding to workflow:

```bash
# Create test directory
mkdir test-heredoc
cd test-heredoc

# Test your HEREDOC
cat > test-file.txt <<'EOF'
Your content here
EOF

# Verify
cat test-file.txt
ls -la
```

## Security Considerations

1. **Never include secrets** in generated files
2. **Use templates** for sensitive data
3. **Validate input** before using in HEREDOC
4. **Check file permissions** after generation
5. **Review PRs** before merging generated content
6. **Use .gitignore** for sensitive generated files

## Examples

See the three workflow files for complete examples:
- `generate-files.yml` - Full-featured
- `generate-files-simple.yml` - Simple
- `cross-repo-generate.yml` - Advanced

---

**Built with the Unix philosophy**: Simple tools, composable patterns, text as the universal interface.
