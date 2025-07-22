/*
Azure DevOps Deployment Records Processor

This script automates the process of:
1. Fetching deployment records from Azure DevOps API for multiple environments
2. Generating deployment summaries for each environment (all-time and monthly)

Prerequisites:
1. Set AZURE_DEVOPS_PAT environment variable with your Personal Access Token
2. Update the organization and project names in getConfig()
3. Update the envIds list with your environment IDs
4. Ensure the deploy-summary.go script is in the same directory

Usage:
  export AZURE_DEVOPS_PAT="your-pat-token-here"
  go run azure-devops-processor.go

Output files per environment ID (e.g., envId=8):
  - env-dep-rec-res-8.json (raw API response)
  - deploy-summary-8.json (all-time summary)
  - deploy-summary-8-monthly.json (last 30 days summary)
*/

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Configuration struct
type Config struct {
	Organization string
	Project      string
	PAT          string // Personal Access Token
	APIVersion   string
}

// Environment deployment record response structure
type APIResponse struct {
	Count int             `json:"count"`
	Value []interface{}   `json:"value"`
}

// Default configuration
func getConfig() Config {
	return Config{
		Organization: "myorga",
		Project:      "project001",
		PAT:          os.Getenv("AZURE_DEVOPS_PAT"), // Set this environment variable
		APIVersion:   "7.2-preview",
	}
}

// Create HTTP client with authentication
func createHTTPClient(pat string) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

// Get authentication header value
func getAuthHeader(pat string) string {
	// Azure DevOps uses basic auth with PAT as username and empty password
	auth := base64.StdEncoding.EncodeToString([]byte(pat + ":"))
	return "Basic " + auth
}

// Call Azure DevOps API for environment deployment records
func getEnvironmentDeploymentRecords(config Config, envId int) ([]byte, error) {
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/distributedtask/environments/%d/environmentdeploymentrecords?api-version=%s",
		config.Organization, config.Project, envId, config.APIVersion)
	
	fmt.Printf("Calling API: %s\n", url)
	
	client := createHTTPClient(config.PAT)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	
	// Set authentication header
	req.Header.Set("Authorization", getAuthHeader(config.PAT))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}
	
	return body, nil
}

// Save API response to file
func saveResponseToFile(data []byte, envId int) (string, error) {
	filename := fmt.Sprintf("env-dep-rec-res-%d.json", envId)
	
	// Pretty print the JSON
	var response interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("error parsing JSON response: %v", err)
	}
	
	prettyJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error formatting JSON: %v", err)
	}
	
	if err := os.WriteFile(filename, prettyJSON, 0644); err != nil {
		return "", fmt.Errorf("error writing file: %v", err)
	}
	
	fmt.Printf("Saved API response to: %s\n", filename)
	return filename, nil
}

// Execute the deployment summary script
func executeSummaryScript(inputFile string, monthly bool) error {
	scriptPath := "./deploy-summary"
	
	// Check if the compiled binary exists, if not try to compile it
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		fmt.Println("Compiling deployment summary script...")
		cmd := exec.Command("go", "build", "-o", "deploy-summary", "deploy-summary.go")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error compiling deploy-summary script: %v", err)
		}
	}
	
	// Prepare command arguments
	var args []string
	args = append(args, "-input", inputFile)
	if monthly {
		args = append(args, "-monthly")
	}
	
	// Execute the script
	cmd := exec.Command(scriptPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if monthly {
		fmt.Printf("Executing deployment summary script (monthly) for: %s\n", inputFile)
	} else {
		fmt.Printf("Executing deployment summary script (all-time) for: %s\n", inputFile)
	}
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error executing deploy-summary script: %v", err)
	}
	
	return nil
}

// Process a single environment ID
func processEnvironment(config Config, envId int) error {
	fmt.Printf("\n=== Processing Environment ID: %d ===\n", envId)
	
	// Step 1: Call Azure DevOps API
	data, err := getEnvironmentDeploymentRecords(config, envId)
	if err != nil {
		return fmt.Errorf("failed to get deployment records for envId %d: %v", envId, err)
	}
	
	// Step 2: Save response to file
	inputFile, err := saveResponseToFile(data, envId)
	if err != nil {
		return fmt.Errorf("failed to save response for envId %d: %v", envId, err)
	}
	
	// Step 3: Execute summary script for all records
	if err := executeSummaryScript(inputFile, false); err != nil {
		return fmt.Errorf("failed to generate all-time summary for envId %d: %v", envId, err)
	}
	
	// Step 4: Execute summary script for monthly records
	if err := executeSummaryScript(inputFile, true); err != nil {
		return fmt.Errorf("failed to generate monthly summary for envId %d: %v", envId, err)
	}
	
	fmt.Printf("✓ Successfully processed environment ID: %d\n", envId)
	return nil
}

// Validate configuration
func validateConfig(config Config) error {
	if config.PAT == "" {
		return fmt.Errorf("AZURE_DEVOPS_PAT environment variable is required")
	}
	if config.Organization == "" {
		return fmt.Errorf("organization is required")
	}
	if config.Project == "" {
		return fmt.Errorf("project is required")
	}
	return nil
}

// Main function
func main() {
	// Configuration
	config := getConfig()
	
	// Validate configuration
	if err := validateConfig(config); err != nil {
		log.Fatal("Configuration error:", err)
	}
	
	// List of environment IDs to process
	// You can modify this list or make it configurable via command line args
	envIds := []int{8, 10, 15, 20} // Example environment IDs
	
	fmt.Printf("Azure DevOps Deployment Records Processor\n")
	fmt.Printf("Organization: %s\n", config.Organization)
	fmt.Printf("Project: %s\n", config.Project)
	fmt.Printf("Processing %d environment(s): %v\n", len(envIds), envIds)
	
	// Track success/failure
	var successCount, failureCount int
	var failedEnvs []int
	
	// Process each environment ID
	for _, envId := range envIds {
		if err := processEnvironment(config, envId); err != nil {
			fmt.Printf("❌ Error processing environment %d: %v\n", envId, err)
			failureCount++
			failedEnvs = append(failedEnvs, envId)
		} else {
			successCount++
		}
	}
	
	// Summary
	fmt.Printf("\n=== Processing Summary ===\n")
	fmt.Printf("Total environments processed: %d\n", len(envIds))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", failureCount)
	
	if len(failedEnvs) > 0 {
		fmt.Printf("Failed environment IDs: %v\n", failedEnvs)
	}
	
	// List generated files
	fmt.Printf("\n=== Generated Files ===\n")
	files, err := filepath.Glob("deploy-summary-*.json")
	if err == nil {
		for _, file := range files {
			fmt.Printf("  %s\n", file)
		}
	}
	
	// List raw API response files
	apiFiles, err := filepath.Glob("env-dep-rec-res-*.json")
	if err == nil && len(apiFiles) > 0 {
		fmt.Printf("\nAPI Response Files:\n")
		for _, file := range apiFiles {
			fmt.Printf("  %s\n", file)
		}
	}
	
	if failureCount > 0 {
		os.Exit(1)
	}
}
