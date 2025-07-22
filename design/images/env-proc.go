package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Environment represents an Azure DevOps environment
type Environment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ADOResponse represents the API response structure
type ADOResponse struct {
	Count int           `json:"count"`
	Value []Environment `json:"value"`
}

// Configuration holds the Azure DevOps settings
type Config struct {
	Organization string
	Project      string
	PAT          string // Personal Access Token
	APIVersion   string
}

// getConfig returns the configuration settings
func getConfig() Config {
	return Config{
		Organization: "myorga",         // Update with your organization
		Project:      "project001",     // Update with your project
		PAT:          os.Getenv("AZURE_DEVOPS_PAT"), // Set this environment variable
		APIVersion:   "7.2-preview",
	}
}

// createHTTPClient creates an HTTP client with timeout
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

// getAuthHeader creates the authorization header for Azure DevOps API
func getAuthHeader(pat string) string {
	// Azure DevOps uses basic auth with PAT as username and empty password
	auth := base64.StdEncoding.EncodeToString([]byte(pat + ":"))
	return "Basic " + auth
}

// callAzureDevOpsAPI calls the Azure DevOps environments API
func callAzureDevOpsAPI(config Config) (*ADOResponse, error) {
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/distributedtask/environments?api-version=%s",
		config.Organization, config.Project, config.APIVersion)

	fmt.Printf("Calling Azure DevOps API: %s\n", url)

	client := createHTTPClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	// Set authentication header
	req.Header.Set("Authorization", getAuthHeader(config.PAT))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making API call: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API call failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var adoResponse ADOResponse
	if err := json.Unmarshal(body, &adoResponse); err != nil {
		return nil, fmt.Errorf("error parsing JSON response: %v", err)
	}

	return &adoResponse, nil
}

// readEnvironmentLevels reads the environment levels from the JSON file
func readEnvironmentLevels(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", filename, err)
	}

	var envLevels []string
	if err := json.Unmarshal(data, &envLevels); err != nil {
		return nil, fmt.Errorf("error parsing JSON from %s: %v", filename, err)
	}

	return envLevels, nil
}

// processEnvironmentLevel processes a single environment level
func processEnvironmentLevel(envLevel string, environments []Environment) error {
	outputData := make(map[string]interface{})
	var idList []int

	suffix := "-" + envLevel

	fmt.Printf("\nProcessing environment level: %s\n", envLevel)

	// Find environments that match the pattern
	for _, env := range environments {
		if strings.HasSuffix(env.Name, suffix) {
			// Remove the suffix to get the key name
			key := strings.TrimSuffix(env.Name, suffix)
			outputData[key] = env.ID
			idList = append(idList, env.ID)
			fmt.Printf("  Found: %s (ID: %d) -> key: %s\n", env.Name, env.ID, key)
		}
	}

	// Add idList to output
	outputData["idList"] = idList

	// Create output filename
	outputFileName := fmt.Sprintf("%s-ids.json", envLevel)

	// Marshal to JSON with proper formatting
	outputJSON, err := json.MarshalIndent(outputData, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling JSON for %s: %v", envLevel, err)
	}

	// Write to file
	if err := os.WriteFile(outputFileName, outputJSON, 0644); err != nil {
		return fmt.Errorf("error writing file %s: %v", outputFileName, err)
	}

	fmt.Printf("  Created %s with %d environments\n", outputFileName, len(idList))
	return nil
}

// validateConfig validates the configuration
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

func main() {
	fmt.Println("Azure DevOps Environment Processor")
	fmt.Println("==================================")

	// Check command line arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run script.go <environment-levels.json>")
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  go run azure-devops-env-processor.go environment-levels.json")
		fmt.Println("")
		fmt.Println("Before running, set your Azure DevOps Personal Access Token:")
		fmt.Println("  export AZURE_DEVOPS_PAT=\"your-pat-token-here\"")
		os.Exit(1)
	}

	envLevelsFile := os.Args[1]

	// Get configuration
	config := getConfig()
	if err := validateConfig(config); err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		fmt.Println("\nTo get a Personal Access Token:")
		fmt.Println("  1. Go to Azure DevOps → User Settings → Personal Access Tokens")
		fmt.Println("  2. Create token with 'Environment (Read)' scope")
		fmt.Println("  3. Set environment variable: export AZURE_DEVOPS_PAT=\"your-token\"")
		os.Exit(1)
	}

	// Read environment levels from file
	fmt.Printf("Reading environment levels from: %s\n", envLevelsFile)
	envLevels, err := readEnvironmentLevels(envLevelsFile)
	if err != nil {
		fmt.Printf("Error reading environment levels: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d environment levels: %v\n", len(envLevels), envLevels)

	// Call Azure DevOps API
	adoResponse, err := callAzureDevOpsAPI(config)
	if err != nil {
		fmt.Printf("Error calling Azure DevOps API: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Retrieved %d environments from Azure DevOps\n", adoResponse.Count)

	// Process each environment level
	successCount := 0
	for _, envLevel := range envLevels {
		if err := processEnvironmentLevel(envLevel, adoResponse.Value); err != nil {
			fmt.Printf("Error processing level %s: %v\n", envLevel, err)
		} else {
			successCount++
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total environment levels processed: %d\n", len(envLevels))
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", len(envLevels)-successCount)
	fmt.Printf("\nOutput files created:\n")
	for _, envLevel := range envLevels {
		outputFile := fmt.Sprintf("%s-ids.json", envLevel)
		if _, err := os.Stat(outputFile); err == nil {
			fmt.Printf("  ✓ %s\n", outputFile)
		}
	}
}
