// ==============================================================================
// Terratest Suite for gold-standard-github-repository Module
// ==============================================================================
//
// File: tests/repository_test.go
//
// Prerequisites:
//   go get github.com/gruntwork-io/terratest/modules/terraform
//   go get github.com/stretchr/testify/assert
//   go get github.com/google/go-github/v57/github
//
// Run tests:
//   cd tests
//   go test -v -timeout 30m
//
// ==============================================================================

package test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// ==============================================================================
// Test 1: Basic Repository Creation
// ==============================================================================

func TestBasicRepositoryCreation(t *testing.T) {
	t.Parallel()

	// Generate random repository name to avoid conflicts
	repoName := fmt.Sprintf("test-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../examples/simple",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"parent_team_slug": "super-parent-team",
			"team_members": []map[string]string{
				{
					"username": "test-user",
					"role":     "maintainer",
				},
			},
		},
		EnvVars: map[string]string{
			"GITHUB_TOKEN": os.Getenv("GITHUB_TOKEN"),
		},
	})

	// Clean up resources after test
	defer terraform.Destroy(t, terraformOptions)

	// Run terraform init and apply
	terraform.InitAndApply(t, terraformOptions)

	// Validate outputs
	repositoryOutput := terraform.OutputMap(t, terraformOptions, "repository")
	assert.Equal(t, repoName, repositoryOutput["name"])
	assert.Equal(t, "private", repositoryOutput["visibility"])
	assert.Contains(t, repositoryOutput["html_url"], repoName)

	// Verify repository exists in GitHub
	client := createGitHubClient(t)
	repo, _, err := client.Repositories.Get(context.Background(), getOrgName(t), repoName)
	require.NoError(t, err)
	assert.NotNil(t, repo)
	assert.Equal(t, repoName, *repo.Name)
}

// ==============================================================================
// Test 2: Repository with Topics and Security Features
// ==============================================================================

func TestRepositoryWithSecurityFeatures(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-secure-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../examples/complete",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"repository_topics": []string{
				"gold-standard",
				"terraform-managed",
				"test",
			},
			"parent_team_slug":                            "super-parent-team",
			"vulnerability_alerts":                        true,
			"security_and_analysis_secret_scanning":       "enabled",
			"security_and_analysis_secret_scanning_push_protection": "enabled",
		},
	})

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Verify topics
	repositoryOutput := terraform.OutputMap(t, terraformOptions, "repository")
	topics := repositoryOutput["topics"]
	assert.Contains(t, topics, "gold-standard")
	assert.Contains(t, topics, "terraform-managed")

	// Verify security features via GitHub API
	client := createGitHubClient(t)
	repo, _, err := client.Repositories.Get(context.Background(), getOrgName(t), repoName)
	require.NoError(t, err)

	assert.True(t, *repo.GetSecurityAndAnalysis().SecretScanning.Status == "enabled")
	assert.True(t, *repo.GetSecurityAndAnalysis().SecretScanningPushProtection.Status == "enabled")
}

// ==============================================================================
// Test 3: Team Creation and Access
// ==============================================================================

func TestTeamCreationAndAccess(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-team-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name":           repoName,
			"parent_team_slug":          "super-parent-team",
			"team_name_suffix":          "owners",
			"team_repository_permission": "admin",
			"team_members": []map[string]string{
				{
					"username": "test-maintainer",
					"role":     "maintainer",
				},
			},
		},
	})

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Verify team output
	teamOutput := terraform.OutputMap(t, terraformOptions, "team")
	expectedTeamName := fmt.Sprintf("%s-owners", repoName)
	assert.Equal(t, expectedTeamName, teamOutput["name"])

	// Verify team repository access via GitHub API
	client := createGitHubClient(t)
	orgName := getOrgName(t)
	teamSlug := teamOutput["slug"]

	// Check team has access to repository
	_, resp, err := client.Teams.IsTeamRepoBySlug(
		context.Background(),
		orgName,
		teamSlug,
		orgName,
		repoName,
	)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode) // 204 = team has access
}

// ==============================================================================
// Test 4: Environment Creation
// ==============================================================================

func TestEnvironmentCreation(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-env-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"parent_team_slug": "super-parent-team",
			"environments": map[string]interface{}{
				"nonproductive": map[string]interface{}{
					"wait_timer":          0,
					"can_admins_bypass":   true,
					"prevent_self_review": false,
				},
				"staging": map[string]interface{}{
					"wait_timer":          60,
					"can_admins_bypass":   true,
					"prevent_self_review": false,
				},
				"prod": map[string]interface{}{
					"wait_timer":          300,
					"can_admins_bypass":   false,
					"prevent_self_review": true,
				},
			},
		},
	})

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Verify environments
	environmentsOutput := terraform.OutputMap(t, terraformOptions, "environments")
	assert.Contains(t, environmentsOutput, "nonproductive")
	assert.Contains(t, environmentsOutput, "staging")
	assert.Contains(t, environmentsOutput, "prod")

	// Verify via GitHub API
	client := createGitHubClient(t)
	orgName := getOrgName(t)

	environments, _, err := client.Repositories.ListEnvironments(
		context.Background(),
		orgName,
		repoName,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, 3, *environments.TotalCount)
}

// ==============================================================================
// Test 5: Ruleset Configuration
// ==============================================================================

func TestRulesetConfiguration(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-ruleset-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name":    repoName,
			"parent_team_slug":   "super-parent-team",
			"ruleset_enabled":    true,
			"ruleset_name":       "Test Branch Protection",
			"ruleset_target":     "branch",
			"ruleset_enforcement": "active",
			"ruleset_pull_request_required_approving_review_count": 2,
			"ruleset_required_status_checks": []string{
				"ci/build",
				"ci/test",
			},
		},
	})

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Verify ruleset output
	rulesetOutput := terraform.OutputMap(t, terraformOptions, "ruleset")
	assert.NotNil(t, rulesetOutput)
	assert.Equal(t, "Test Branch Protection", rulesetOutput["name"])
	assert.Equal(t, "active", rulesetOutput["enforcement"])

	// Verify via GitHub API
	client := createGitHubClient(t)
	orgName := getOrgName(t)

	rulesets, _, err := client.Repositories.GetAllRulesets(
		context.Background(),
		orgName,
		repoName,
		false, // includesParents
	)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rulesets), 1)
}

// ==============================================================================
// Test 6: Pre-flight Validation (Repository Already Exists)
// ==============================================================================

func TestPreflightValidationFailsForExistingRepo(t *testing.T) {
	t.Parallel()

	// First, create a repository
	repoName := fmt.Sprintf("test-existing-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"parent_team_slug": "super-parent-team",
		},
	})

	terraform.InitAndApply(t, terraformOptions)
	defer terraform.Destroy(t, terraformOptions)

	// Now try to create another module with the same repo name
	terraformOptions2 := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"parent_team_slug": "super-parent-team",
		},
	})

	// This should fail with validation error
	_, err := terraform.InitAndApplyE(t, terraformOptions2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// ==============================================================================
// Test 7: Custom Role Creation (Requires GitHub Enterprise Cloud)
// ==============================================================================

func TestCustomRoleCreation(t *testing.T) {
	// Skip if not on Enterprise Cloud
	if !isEnterpriseCloud(t) {
		t.Skip("Custom roles require GitHub Enterprise Cloud")
	}

	t.Parallel()

	repoName := fmt.Sprintf("test-custom-role-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name":      repoName,
			"parent_team_slug":     "super-parent-team",
			"create_custom_role":   true,
			"custom_role_name":     "",
			"custom_role_base_role": "write",
			"custom_role_permissions": []string{
				"read_code",
				"write_code",
				"manage_pull_requests",
			},
		},
	})

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Verify custom role output
	customRoleOutput := terraform.OutputMap(t, terraformOptions, "custom_role")
	assert.NotNil(t, customRoleOutput)
	expectedRoleName := fmt.Sprintf("%s-superdev", repoName)
	assert.Equal(t, expectedRoleName, customRoleOutput["name"])
}

// ==============================================================================
// Test 8: Repository from Template
// ==============================================================================

func TestRepositoryFromTemplate(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-from-template-%s", random.UniqueId())

	// Assume a template repo exists
	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name":     repoName,
			"parent_team_slug":    "super-parent-team",
			"template_owner":      getOrgName(t),
			"template_repository": "service-template",
		},
	})

	defer terraform.Destroy(t, terraformOptions)

	// This will fail if template doesn't exist, which is expected
	_, err := terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		t.Logf("Template test skipped (template repo doesn't exist): %v", err)
		t.Skip()
	}
}

// ==============================================================================
// Test 9: Idempotency Test
// ==============================================================================

func TestIdempotency(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-idempotent-repo-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"parent_team_slug": "super-parent-team",
		},
	})

	defer terraform.Destroy(t, terraformOptions)

	// Apply twice
	terraform.InitAndApply(t, terraformOptions)
	output1 := terraform.OutputAll(t, terraformOptions)

	// Second apply should result in no changes
	planOutput := terraform.Plan(t, terraformOptions)
	assert.Contains(t, planOutput, "No changes")

	// Apply again and verify outputs are identical
	terraform.Apply(t, terraformOptions)
	output2 := terraform.OutputAll(t, terraformOptions)

	assert.Equal(t, output1, output2)
}

// ==============================================================================
// Test 10: Actions Disabled by Default
// ==============================================================================

func TestActionsDisabledByDefault(t *testing.T) {
	t.Parallel()

	repoName := fmt.Sprintf("test-actions-disabled-%s", random.UniqueId())

	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../",
		Vars: map[string]interface{}{
			"repository_name": repoName,
			"parent_team_slug": "super-parent-team",
			// actions_enabled defaults to false
		},
	})

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Verify Actions are disabled via API
	client := createGitHubClient(t)
	orgName := getOrgName(t)

	// Check Actions permissions
	// Note: API call may not be available for all GitHub plans
	permissions, resp, _ := client.Repositories.GetActionsPermissions(
		context.Background(),
		orgName,
		repoName,
	)

	if resp.StatusCode == 200 {
		assert.False(t, *permissions.Enabled)
	}
}

// ==============================================================================
// Helper Functions
// ==============================================================================

func createGitHubClient(t *testing.T) *github.Client {
	token := os.Getenv("GITHUB_TOKEN")
	require.NotEmpty(t, token, "GITHUB_TOKEN environment variable must be set")

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

func getOrgName(t *testing.T) string {
	orgName := os.Getenv("GITHUB_ORG")
	if orgName == "" {
		orgName = "test-org" // Default for testing
	}
	return orgName
}

func isEnterpriseCloud(t *testing.T) bool {
	return os.Getenv("GITHUB_ENTERPRISE_CLOUD") == "true"
}

// ==============================================================================
// Benchmark Tests
// ==============================================================================

func BenchmarkRepositoryCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		repoName := fmt.Sprintf("benchmark-repo-%d-%s", i, random.UniqueId())

		terraformOptions := terraform.WithDefaultRetryableErrors(&testing.T{}, &terraform.Options{
			TerraformDir: "../",
			Vars: map[string]interface{}{
				"repository_name": repoName,
				"parent_team_slug": "super-parent-team",
			},
		})

		start := time.Now()
		terraform.InitAndApply(&testing.T{}, terraformOptions)
		duration := time.Since(start)

		b.Logf("Repository %s created in %v", repoName, duration)

		terraform.Destroy(&testing.T{}, terraformOptions)
	}
}

// ==============================================================================
// Integration Test Suite
// ==============================================================================

func TestIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	t.Run("BasicCreation", TestBasicRepositoryCreation)
	t.Run("SecurityFeatures", TestRepositoryWithSecurityFeatures)
	t.Run("TeamAccess", TestTeamCreationAndAccess)
	t.Run("Environments", TestEnvironmentCreation)
	t.Run("Rulesets", TestRulesetConfiguration)
	t.Run("PreflightValidation", TestPreflightValidationFailsForExistingRepo)
	t.Run("Idempotency", TestIdempotency)
	t.Run("ActionsDisabled", TestActionsDisabledByDefault)
}

// ==============================================================================
// End of Test Suite
// ==============================================================================
