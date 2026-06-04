#!/bin/bash
# Generate per-control documentation pages
# These ship alongside the trust center as static HTML

set -euo pipefail

DOCS_DIR="trust-center/docs"
mkdir -p "$DOCS_DIR"

generate_doc() {
    local slug="$1"
    local title="$2"
    local category="$3"
    local schedule="$4"
    local description="$5"
    local checks="$6"
    local criteria="$7"
    local remediation="$8"
    local references="$9"

    cat > "${DOCS_DIR}/${slug}.html" << EOF
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${title} — Kickbutt Compliance</title>
    <link rel="stylesheet" href="../style.css">
    <style>
        .doc-container { max-width: 860px; margin: 0 auto; padding: 2rem 1.5rem 4rem; }
        .breadcrumb { margin-bottom: 2rem; }
        .breadcrumb a { color: var(--info); text-decoration: none; }
        .breadcrumb a:hover { text-decoration: underline; }
        .breadcrumb span { color: var(--text-secondary); }
        .doc-header { margin-bottom: 2.5rem; }
        .doc-header h1 { font-size: 2rem; margin-bottom: 0.5rem; }
        .doc-meta { display: flex; gap: 1.5rem; flex-wrap: wrap; color: var(--text-secondary); font-size: 0.9rem; margin-top: 0.75rem; }
        .doc-meta span { display: inline-flex; align-items: center; gap: 0.35rem; }
        .doc-badge { display: inline-block; padding: 0.2rem 0.6rem; border-radius: 4px; font-size: 0.8rem; font-weight: 600; background: rgba(88,166,255,0.12); color: var(--info); }
        .doc-section { margin-bottom: 2.5rem; }
        .doc-section h2 { font-size: 1.35rem; margin-bottom: 1rem; padding-bottom: 0.5rem; border-bottom: 1px solid var(--border-color); }
        .doc-section p, .doc-section li { color: var(--text-secondary); line-height: 1.75; }
        .doc-section ul, .doc-section ol { padding-left: 1.5rem; margin-top: 0.5rem; }
        .doc-section li { margin-bottom: 0.5rem; }
        .doc-section code { background: var(--secondary-bg); padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.9em; color: var(--text-primary); }
        .check-item { background: var(--card-bg); border: 1px solid var(--border-color); border-radius: 6px; padding: 1rem 1.25rem; margin-bottom: 0.75rem; }
        .check-item strong { color: var(--text-primary); }
        .ref-link { color: var(--info); text-decoration: none; display: block; margin-bottom: 0.5rem; }
        .ref-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <header>
        <div class="container">
            <h1>🛡️ Kickbutt Compliance</h1>
            <p class="subtitle">Control Documentation</p>
        </div>
    </header>
    <div class="doc-container">
        <nav class="breadcrumb">
            <a href="../index.html">Trust Center</a>
            <span> / </span>
            <span>${title}</span>
        </nav>
        <div class="doc-header">
            <h1>${title}</h1>
            <p style="color: var(--text-secondary); font-size: 1.05rem; margin-top: 0.5rem;">${description}</p>
            <div class="doc-meta">
                <span><span class="doc-badge">${category}</span></span>
                <span>⏱ Schedule: ${schedule}</span>
                <span>📄 Workflow: <code>${slug}.yml</code></span>
            </div>
        </div>

        <section class="doc-section">
            <h2>What This Control Checks</h2>
            ${checks}
        </section>

        <section class="doc-section">
            <h2>Pass / Fail Criteria</h2>
            ${criteria}
        </section>

        <section class="doc-section">
            <h2>Remediation Steps</h2>
            ${remediation}
        </section>

        <section class="doc-section">
            <h2>References</h2>
            ${references}
        </section>
    </div>
    <footer>
        <div class="container">
            <p>Powered by GitHub Actions | Built with ❤️ by DevOps</p>
        </div>
    </footer>
</body>
</html>
EOF

    echo "  ✅ Generated: ${DOCS_DIR}/${slug}.html"
}

echo "Generating control documentation pages..."

# === control-org-settings ===
generate_doc \
    "control-org-settings" \
    "Organization Settings Control" \
    "Access Control" \
    "Every 6 hours" \
    "Validates organization-level security settings including two-factor authentication enforcement, default repository permissions, and member privilege boundaries." \
    '<div class="check-item"><strong>Two-Factor Authentication</strong><p>Verifies that 2FA is required for all organization members. This prevents credential-based account takeover.</p></div>
<div class="check-item"><strong>Default Repository Permissions</strong><p>Ensures the default permission granted to new organization members is <code>read</code> or <code>none</code>, not <code>write</code> or <code>admin</code>.</p></div>
<div class="check-item"><strong>Member Repository Creation</strong><p>Checks whether members can create public repositories. Public creation should be restricted to prevent accidental data exposure.</p></div>' \
    '<ul>
<li><strong>PASS</strong>: 2FA is enforced <em>and</em> default permissions are <code>read</code> or <code>none</code></li>
<li><strong>FAIL</strong>: 2FA is not enforced, <em>or</em> default permissions are <code>write</code> / <code>admin</code></li>
<li><strong>WARNING</strong>: Members can create public repositories (non-blocking)</li>
</ul>' \
    '<ol>
<li>Navigate to <strong>Organization Settings → Member privileges</strong></li>
<li>Under "Two-factor authentication", click <strong>Require two-factor authentication</strong></li>
<li>Under "Default repository permission", select <strong>Read</strong> or <strong>No permission</strong></li>
<li>Under "Repository creation", uncheck <strong>Allow members to create public repositories</strong></li>
<li>Click <strong>Save</strong></li>
</ol>
<p>You can also enforce these via the GitHub API or Terraform <code>github_organization_settings</code> resource.</p>' \
    '<a class="ref-link" href="https://docs.github.com/en/organizations/keeping-your-organization-secure/requiring-two-factor-authentication-in-your-organization" target="_blank">GitHub Docs: Requiring 2FA</a>
<a class="ref-link" href="https://docs.github.com/en/organizations/managing-user-access-to-your-organizations-repositories/managing-repository-roles/setting-base-permissions-for-an-organization" target="_blank">GitHub Docs: Base Permissions</a>
<a class="ref-link" href="https://cis.cisecurity.org/" target="_blank">CIS Benchmarks</a>'

# === control-repo-visibility ===
generate_doc \
    "control-repo-visibility" \
    "Repository Visibility Control" \
    "Data Protection" \
    "Every 6 hours" \
    "Monitors repository visibility policies ensuring sensitive repositories remain private, public exposure stays below threshold, and naming conventions are enforced." \
    '<div class="check-item"><strong>Public Repository Ratio</strong><p>Counts public vs. private repositories and checks that public repositories do not exceed 10% of total. Excessive public exposure increases attack surface.</p></div>
<div class="check-item"><strong>Sensitive Repository Detection</strong><p>Scans for repositories matching sensitive name patterns (<code>secret</code>, <code>config</code>, <code>infrastructure</code>, <code>terraform</code>, <code>vault</code>, <code>credentials</code>) and ensures none are publicly accessible.</p></div>' \
    '<ul>
<li><strong>PASS</strong>: Public repos ≤ 10% of total <em>and</em> no sensitive repos are public</li>
<li><strong>FAIL</strong>: Public repos &gt; 10% <em>or</em> any sensitive-named repos are public</li>
</ul>' \
    '<ol>
<li>Navigate to the public repository in question on GitHub</li>
<li>Go to <strong>Settings → General → Danger Zone</strong></li>
<li>Click <strong>Change repository visibility</strong> and switch to <strong>Private</strong></li>
<li>For bulk changes, use the GitHub API: <code>gh api -X PATCH /repos/{owner}/{repo} -f visibility=private</code></li>
<li>Consider creating an organization ruleset that prevents changing repos to public</li>
</ol>' \
    '<a class="ref-link" href="https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/managing-repository-settings/setting-repository-visibility" target="_blank">GitHub Docs: Repository Visibility</a>
<a class="ref-link" href="https://docs.github.com/en/organizations/managing-organization-settings/restricting-repository-visibility-changes-in-your-organization" target="_blank">GitHub Docs: Restricting Visibility Changes</a>'

# === control-repo-rulesets ===
generate_doc \
    "control-repo-rulesets" \
    "Repository Rulesets Control" \
    "Change Management" \
    "Every 6 hours" \
    "Verifies branch protection rules, organization-level rulesets, required status checks, and pull request review requirements on critical repositories." \
    '<div class="check-item"><strong>Organization-Level Rulesets</strong><p>Checks that at least one active organization-level ruleset is configured. Org rulesets provide a baseline of protection that cannot be overridden at the repo level.</p></div>
<div class="check-item"><strong>Branch Protection on Critical Repos</strong><p>Verifies that critical repositories (matching patterns like <code>infrastructure</code>, <code>terraform</code>, <code>config</code>, <code>*-prod</code>) have branch protection enabled on their default branch.</p></div>
<div class="check-item"><strong>Required Pull Request Reviews</strong><p>Checks that protected branches require at least one approving review before merging, preventing unreviewed code from reaching production.</p></div>' \
    '<ul>
<li><strong>PASS</strong>: Organization rulesets exist <em>and</em> all critical repos have branch protection with required reviews</li>
<li><strong>FAIL</strong>: Any critical repository lacks branch protection</li>
<li><strong>WARNING</strong>: Branch protection exists but no required reviews configured (non-blocking)</li>
</ul>' \
    '<ol>
<li>Navigate to <strong>Organization Settings → Repository → Rulesets</strong></li>
<li>Click <strong>New ruleset → New branch ruleset</strong></li>
<li>Target: <strong>All repositories</strong> (or specific repos)</li>
<li>Branch targeting: <strong>Default branch</strong></li>
<li>Enable: <strong>Require a pull request before merging</strong> → set required approvals to 1+</li>
<li>Enable: <strong>Require status checks to pass</strong></li>
<li>Set enforcement to <strong>Active</strong></li>
<li>Click <strong>Create</strong></li>
</ol>' \
    '<a class="ref-link" href="https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets" target="_blank">GitHub Docs: About Rulesets</a>
<a class="ref-link" href="https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches" target="_blank">GitHub Docs: Branch Protection</a>
<a class="ref-link" href="https://registry.terraform.io/providers/integrations/github/latest/docs/resources/organization_ruleset" target="_blank">Terraform: github_organization_ruleset</a>'

# === control-org-custom-role ===
generate_doc \
    "control-org-custom-role" \
    "Organization Custom Roles Control" \
    "Access Control" \
    "Every 6 hours" \
    "Audits custom organization role definitions, validates least-privilege assignments, and checks for overly permissive role configurations." \
    '<div class="check-item"><strong>Custom Role Inventory</strong><p>Enumerates all custom organization roles and validates that recommended roles (like <code>security-engineer</code>, <code>release-manager</code>) are defined for proper RBAC.</p></div>
<div class="check-item"><strong>Admin Permission Audit</strong><p>Checks that no custom role grants <code>admin</code>-level permissions, which would bypass branch protection and other safeguards.</p></div>
<div class="check-item"><strong>Team Assignment Verification</strong><p>Validates that teams are configured in the organization, enabling group-based access control rather than individual permission grants.</p></div>' \
    '<ul>
<li><strong>PASS</strong>: Custom roles (if any) have no admin-level permissions <em>and</em> teams are configured</li>
<li><strong>FAIL</strong>: Custom roles with admin permissions are found</li>
<li><strong>INFO</strong>: No custom roles defined (acceptable for smaller orgs; uses default roles only)</li>
</ul>' \
    '<ol>
<li>Navigate to <strong>Organization Settings → Roles</strong></li>
<li>Review each custom role and click <strong>Edit</strong></li>
<li>Remove any <code>admin</code> or overly broad permissions</li>
<li>Create recommended roles:
    <ul>
    <li><strong>security-engineer</strong>: <code>read</code> + <code>manage security advisories</code> + <code>manage code scanning</code></li>
    <li><strong>release-manager</strong>: <code>write</code> + <code>manage releases</code> + <code>bypass branch protection</code></li>
    </ul>
</li>
<li>Assign teams to roles via <strong>Organization Settings → Teams</strong></li>
</ol>' \
    '<a class="ref-link" href="https://docs.github.com/en/enterprise-cloud@latest/organizations/managing-peoples-access-to-your-organization-with-roles/managing-custom-organization-roles" target="_blank">GitHub Docs: Custom Organization Roles</a>
<a class="ref-link" href="https://docs.github.com/en/enterprise-cloud@latest/organizations/managing-peoples-access-to-your-organization-with-roles/about-custom-organization-roles" target="_blank">GitHub Docs: About Custom Roles</a>'

echo ""
echo "✅ All documentation pages generated in ${DOCS_DIR}/"
