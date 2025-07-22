package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Version and build info
const (
	AppName    = "ado-cli"
	AppVersion = "1.0.0"
)

// Configuration holds all Azure DevOps settings
type Config struct {
	Organization string
	Project      string
	PAT          string // Personal Access Token
	APIVersion   string
}

// Environment represents an Azure DevOps environment
type Environment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// EnvironmentResponse represents the API response for environments
type EnvironmentResponse struct {
	Count int           `json:"count"`
	Value []Environment `json:"value"`
}

// DeploymentRecord represents a deployment record from Azure DevOps
type DeploymentRecord struct {
	ID            int    `json:"id"`
	EnvironmentID int    `json:"environmentId"`
	Result        string `json:"result"`
	FinishTime    string `json:"finishTime"`
	Definition    struct {
		Name string `json:"name"`
	} `json:"definition"`
}

// DeploymentResponse represents the API response for deployment records
type DeploymentResponse struct {
	Count int                `json:"count"`
	Value []DeploymentRecord `json:"value"`
}

// DeploymentStats holds deployment statistics
type DeploymentStats struct {
	TotalDeployments int `json:"total_deployments"`
	TotalSucceeded   int `json:"total_succeeded"`
	TotalFailed      int `json:"total_failed"`
}

// SummaryRecord represents a deployment summary for a pipeline
type SummaryRecord struct {
	PipelineRepo     string  `json:"pipeline-repo"`
	DeploymentStats  `json:",inline"`
	PercentSucceeded float64 `json:"percent_succeeded"`
	Latest           string  `json:"latest"`
}

// CLI Commands
type Command struct {
	Name        string
	Description string
	Handler     func(args []string) error
}

// getConfig returns the application configuration
func getConfig() Config {
	return Config{
		Organization: getEnvOrDefault("ADO_ORGANIZATION", "myorga"),
		Project:      getEnvOrDefault("ADO_PROJECT", "project001"),
		PAT:          os.Getenv("AZURE_DEVOPS_PAT"),
		APIVersion:   "7.2-preview",
	}
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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

// createHTTPClient creates an HTTP client with timeout
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

// getAuthHeader creates the authorization header for Azure DevOps API
func getAuthHeader(pat string) string {
	auth := base64.StdEncoding.EncodeToString([]byte(pat + ":"))
	return "Basic " + auth
}

// callAPI makes a generic API call to Azure DevOps
func callAPI(config Config, endpoint string) ([]byte, error) {
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/%s?api-version=%s",
		config.Organization, config.Project, endpoint, config.APIVersion)

	client := createHTTPClient()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

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

	return io.ReadAll(resp.Body)
}

// getEnvironments fetches all environments from Azure DevOps
func getEnvironments(config Config) (*EnvironmentResponse, error) {
	data, err := callAPI(config, "distributedtask/environments")
	if err != nil {
		return nil, err
	}

	var response EnvironmentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("error parsing environments response: %v", err)
	}

	return &response, nil
}

// getDeploymentRecords fetches deployment records for a specific environment
func getDeploymentRecords(config Config, envID int) (*DeploymentResponse, error) {
	endpoint := fmt.Sprintf("distributedtask/environments/%d/environmentdeploymentrecords", envID)
	data, err := callAPI(config, endpoint)
	if err != nil {
		return nil, err
	}

	var response DeploymentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("error parsing deployment records response: %v", err)
	}

	return &response, nil
}

// saveToFile saves data to a JSON file with pretty formatting
func saveToFile(data interface{}, filename string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling data: %v", err)
	}

	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("error writing file: %v", err)
	}

	return nil
}

// readEnvironmentLevels reads environment levels from JSON file
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

// processEnvironmentLevel processes environments for a specific level
func processEnvironmentLevel(envLevel string, environments []Environment) (map[string]interface{}, error) {
	outputData := make(map[string]interface{})
	var idList []int

	suffix := "-" + envLevel

	for _, env := range environments {
		if strings.HasSuffix(env.Name, suffix) {
			key := strings.TrimSuffix(env.Name, suffix)
			outputData[key] = env.ID
			idList = append(idList, env.ID)
		}
	}

	outputData["idList"] = idList
	return outputData, nil
}

// generateDeploymentSummary generates deployment summary from records
func generateDeploymentSummary(records []DeploymentRecord, monthly bool) []SummaryRecord {
	var recordsToProcess []DeploymentRecord

	if monthly {
		for _, record := range records {
			if isWithinLast30Days(record.FinishTime) {
				recordsToProcess = append(recordsToProcess, record)
			}
		}
	} else {
		recordsToProcess = records
	}

	// Group by pipeline repo
	groups := make(map[string][]DeploymentRecord)
	for _, record := range recordsToProcess {
		groups[record.Definition.Name] = append(groups[record.Definition.Name], record)
	}

	var summaries []SummaryRecord
	for pipelineRepo, repoRecords := range groups {
		stats := DeploymentStats{
			TotalDeployments: len(repoRecords),
		}

		var latestTime time.Time
		var latestTimeStr string

		for _, record := range repoRecords {
			switch record.Result {
			case "succeeded":
				stats.TotalSucceeded++
			case "failed":
				stats.TotalFailed++
			}

			if record.FinishTime != "" {
				if finishTime, err := time.Parse(time.RFC3339, record.FinishTime); err == nil {
					if latestTime.IsZero() || finishTime.After(latestTime) {
						latestTime = finishTime
						latestTimeStr = record.FinishTime
					}
				}
			}
		}

		var percentSucceeded float64
		if stats.TotalDeployments > 0 {
			percentSucceeded = math.Round(float64(stats.TotalSucceeded)/float64(stats.TotalDeployments)*100*100) / 100
		}

		summary := SummaryRecord{
			PipelineRepo:     pipelineRepo,
			DeploymentStats:  stats,
			PercentSucceeded: percentSucceeded,
			Latest:           latestTimeStr,
		}

		summaries = append(summaries, summary)
	}

	// Sort by pipeline repo name
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].PipelineRepo < summaries[j].PipelineRepo
	})

	return summaries
}

// isWithinLast30Days checks if a timestamp is within the last 30 days
func isWithinLast30Days(timeStr string) bool {
	if timeStr == "" {
		return false
	}

	parsedTime, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return false
	}

	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	return parsedTime.After(thirtyDaysAgo)
}

// Command handlers

// handleEnvironments processes environments and organizes by levels
func handleEnvironments(args []string) error {
	config := getConfig()
	if err := validateConfig(config); err != nil {
		return err
	}

	// Parse flags
	fs := flag.NewFlagSet("environments", flag.ExitOnError)
	levelsFile := fs.String("levels", "environment-levels.json", "Environment levels JSON file")
	fs.Parse(args)

	fmt.Println("🔍 Fetching environments from Azure DevOps...")

	// Get environments
	response, err := getEnvironments(config)
	if err != nil {
		return fmt.Errorf("failed to get environments: %v", err)
	}

	fmt.Printf("✅ Retrieved %d environments\n", response.Count)

	// Read environment levels
	envLevels, err := readEnvironmentLevels(*levelsFile)
	if err != nil {
		return fmt.Errorf("failed to read environment levels: %v", err)
	}

	fmt.Printf("📋 Processing %d environment levels: %v\n", len(envLevels), envLevels)

	// Process each level
	successCount := 0
	for _, envLevel := range envLevels {
		outputData, err := processEnvironmentLevel(envLevel, response.Value)
		if err != nil {
			fmt.Printf("❌ Error processing level %s: %v\n", envLevel, err)
			continue
		}

		filename := fmt.Sprintf("%s-ids.json", envLevel)
		if err := saveToFile(outputData, filename); err != nil {
			fmt.Printf("❌ Error saving %s: %v\n", filename, err)
			continue
		}

		idList := outputData["idList"].([]int)
		fmt.Printf("✅ Created %s with %d environments\n", filename, len(idList))
		successCount++
	}

	fmt.Printf("\n📊 Summary: %d/%d levels processed successfully\n", successCount, len(envLevels))
	return nil
}

// handleDeployments fetches deployment records for environments
func handleDeployments(args []string) error {
	config := getConfig()
	if err := validateConfig(config); err != nil {
		return err
	}

	// Parse flags
	fs := flag.NewFlagSet("deployments", flag.ExitOnError)
	envIDs := fs.String("envs", "", "Comma-separated environment IDs (required)")
	fs.Parse(args)

	if *envIDs == "" {
		return fmt.Errorf("environment IDs are required (use -envs flag)")
	}

	// Parse environment IDs
	idStrings := strings.Split(*envIDs, ",")
	var envIDList []int
	for _, idStr := range idStrings {
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil {
			return fmt.Errorf("invalid environment ID: %s", idStr)
		}
		envIDList = append(envIDList, id)
	}

	fmt.Printf("🚀 Fetching deployment records for %d environments\n", len(envIDList))

	successCount := 0
	for _, envID := range envIDList {
		fmt.Printf("\n🔍 Processing environment ID: %d\n", envID)

		response, err := getDeploymentRecords(config, envID)
		if err != nil {
			fmt.Printf("❌ Failed to get deployment records for env %d: %v\n", envID, err)
			continue
		}

		filename := fmt.Sprintf("env-dep-rec-res-%d.json", envID)
		if err := saveToFile(response, filename); err != nil {
			fmt.Printf("❌ Failed to save deployment records for env %d: %v\n", envID, err)
			continue
		}

		fmt.Printf("✅ Saved %d deployment records to %s\n", response.Count, filename)
		successCount++
	}

	fmt.Printf("\n📊 Summary: %d/%d environments processed successfully\n", successCount, len(envIDList))
	return nil
}

// handleSummary generates deployment summaries
func handleSummary(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	inputFile := fs.String("input", "", "Input deployment records JSON file (required)")
	monthly := fs.Bool("monthly", false, "Generate monthly summary (last 30 days)")
	envID := fs.Int("env", 0, "Environment ID for output filename")
	fs.Parse(args)

	if *inputFile == "" {
		return fmt.Errorf("input file is required (use -input flag)")
	}

	fmt.Printf("📊 Generating deployment summary from: %s\n", *inputFile)

	// Read deployment records
	data, err := os.ReadFile(*inputFile)
	if err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}

	var response DeploymentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("error parsing deployment records: %v", err)
	}

	// Generate summary
	summaries := generateDeploymentSummary(response.Value, *monthly)

	// Create output filename
	var outputFilename string
	if *envID != 0 {
		if *monthly {
			outputFilename = fmt.Sprintf("deploy-summary-%d-monthly.json", *envID)
		} else {
			outputFilename = fmt.Sprintf("deploy-summary-%d.json", *envID)
		}
	} else {
		if *monthly {
			outputFilename = "deploy-summary-monthly.json"
		} else {
			outputFilename = "deploy-summary.json"
		}
	}

	// Save summary
	if err := saveToFile(summaries, outputFilename); err != nil {
		return fmt.Errorf("error saving summary: %v", err)
	}

	timeRange := "all time"
	if *monthly {
		timeRange = "last 30 days"
	}

	fmt.Printf("✅ Summary written to %s\n", outputFilename)
	fmt.Printf("📈 Processed %d deployment records (%s) across %d pipeline repos\n",
		len(response.Value), timeRange, len(summaries))

	// Show preview
	if len(summaries) > 0 {
		fmt.Println("\n🎯 Top deployments:")
		for i, summary := range summaries {
			if i >= 3 {
				break
			}
			fmt.Printf("  %s: %d deployments, %.1f%% success\n",
				summary.PipelineRepo, summary.TotalDeployments, summary.PercentSucceeded)
		}
	}

	return nil
}

// handleFull runs the complete workflow
func handleFull(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("full", flag.ExitOnError)
	levelsFile := fs.String("levels", "environment-levels.json", "Environment levels JSON file")
	monthly := fs.Bool("monthly", false, "Generate monthly summaries")
	fs.Parse(args)

	fmt.Println("🚀 Starting full Azure DevOps processing workflow...")

	config := getConfig()
	if err := validateConfig(config); err != nil {
		return err
	}

	// Step 1: Get environments and organize by levels
	fmt.Println("\n📋 Step 1: Processing environments...")
	if err := handleEnvironments([]string{"-levels", *levelsFile}); err != nil {
		return fmt.Errorf("failed to process environments: %v", err)
	}

	// Step 2: Read environment levels and get all environment IDs
	envLevels, err := readEnvironmentLevels(*levelsFile)
	if err != nil {
		return fmt.Errorf("failed to read environment levels: %v", err)
	}

	var allEnvIDs []int
	for _, level := range envLevels {
		filename := fmt.Sprintf("%s-ids.json", level)
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Printf("⚠️ Warning: Could not read %s: %v\n", filename, err)
			continue
		}

		var levelData map[string]interface{}
		if err := json.Unmarshal(data, &levelData); err != nil {
			fmt.Printf("⚠️ Warning: Could not parse %s: %v\n", filename, err)
			continue
		}

		if idList, ok := levelData["idList"].([]interface{}); ok {
			for _, id := range idList {
				if envID, ok := id.(float64); ok {
					allEnvIDs = append(allEnvIDs, int(envID))
				}
			}
		}
	}

	if len(allEnvIDs) == 0 {
		return fmt.Errorf("no environment IDs found from level files")
	}

	// Step 3: Get deployment records
	fmt.Printf("\n📊 Step 2: Fetching deployment records for %d environments...\n", len(allEnvIDs))
	envIDsStr := strings.Trim(strings.Replace(fmt.Sprint(allEnvIDs), " ", ",", -1), "[]")
	if err := handleDeployments([]string{"-envs", envIDsStr}); err != nil {
		return fmt.Errorf("failed to get deployment records: %v", err)
	}

	// Step 4: Generate summaries
	fmt.Println("\n📈 Step 3: Generating deployment summaries...")
	for _, envID := range allEnvIDs {
		inputFile := fmt.Sprintf("env-dep-rec-res-%d.json", envID)
		if _, err := os.Stat(inputFile); os.IsNotExist(err) {
			continue
		}

		// Generate all-time summary
		summaryArgs := []string{"-input", inputFile, "-env", fmt.Sprintf("%d", envID)}
		if err := handleSummary(summaryArgs); err != nil {
			fmt.Printf("⚠️ Warning: Failed to generate summary for env %d: %v\n", envID, err)
		}

		// Generate monthly summary if requested
		if *monthly {
			monthlyArgs := []string{"-input", inputFile, "-env", fmt.Sprintf("%d", envID), "-monthly"}
			if err := handleSummary(monthlyArgs); err != nil {
				fmt.Printf("⚠️ Warning: Failed to generate monthly summary for env %d: %v\n", envID, err)
			}
		}
	}

	fmt.Println("\n🎉 Full workflow completed successfully!")
	return nil
}

// showHelp displays help information
func showHelp() {
	fmt.Printf(`%s v%s - Azure DevOps Environment and Deployment CLI

USAGE:
    %s <command> [flags]

COMMANDS:
    environments    List and organize environments by levels
    deployments     Fetch deployment records for environments  
    summary         Generate deployment summaries from records
    full            Run complete workflow (environments + deployments + summaries)
    help            Show this help message

GLOBAL ENVIRONMENT VARIABLES:
    AZURE_DEVOPS_PAT     Personal Access Token (required)
    ADO_ORGANIZATION     Azure DevOps organization (default: myorga)
    ADO_PROJECT          Azure DevOps project (default: project001)

EXAMPLES:
    # Set up authentication
    export AZURE_DEVOPS_PAT="your-pat-token-here"
    export ADO_ORGANIZATION="your-org"
    export ADO_PROJECT="your-project"

    # Process environments by levels
    %s environments -levels environment-levels.json

    # Get deployment records for specific environments
    %s deployments -envs "8,10,15,20"

    # Generate deployment summary
    %s summary -input env-dep-rec-res-8.json -env 8

    # Run full workflow
    %s full -levels environment-levels.json -monthly

For detailed help on a command, run: %s <command> -h

`, AppName, AppVersion, AppName, AppName, AppName, AppName, AppName, AppName)
}

func main() {
	if len(os.Args) < 2 {
		showHelp()
		os.Exit(1)
	}

	// Define available commands
	commands := map[string]Command{
		"environments": {
			Name:        "environments",
			Description: "Process environments and organize by levels",
			Handler:     handleEnvironments,
		},
		"deployments": {
			Name:        "deployments", 
			Description: "Fetch deployment records for environments",
			Handler:     handleDeployments,
		},
		"summary": {
			Name:        "summary",
			Description: "Generate deployment summaries",
			Handler:     handleSummary,
		},
		"full": {
			Name:        "full",
			Description: "Run complete workflow",
			Handler:     handleFull,
		},
		"help": {
			Name:        "help",
			Description: "Show help information",
			Handler: func(args []string) error {
				showHelp()
				return nil
			},
		},
	}

	commandName := os.Args[1]
	command, exists := commands[commandName]

	if !exists {
		fmt.Printf("Unknown command: %s\n\n", commandName)
		showHelp()
		os.Exit(1)
	}

	// Execute command
	if err := command.Handler(os.Args[2:]); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
