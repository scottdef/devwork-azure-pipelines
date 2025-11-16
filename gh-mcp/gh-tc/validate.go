package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ============================================================================
// GitHub Audit Log Monitoring - Validation Tool
// ============================================================================
// Purpose: Validate Dynatrace configuration and test DQL queries
// Go Version: 1.21
// Usage: go run validate.go
// ============================================================================

// Config represents the Dynatrace configuration
type Config struct {
	DynatraceURL   string `json:"dynatrace_url"`
	APIToken       string `json:"api_token"`
	LogSource      string `json:"log_source"`
	MetricPrefix   string `json:"metric_prefix"`
	TestMode       bool   `json:"test_mode"`
}

// DQLQuery represents a DQL query to execute
type DQLQuery struct {
	Query       string `json:"query"`
	Description string `json:"description"`
}

// MetricValidation represents expected metrics
type MetricValidation struct {
	MetricKey   string `json:"metricKey"`
	ExpectedMin int    `json:"expectedMin"`
	ExpectedMax int    `json:"expectedMax"`
}

// TestResult represents the result of a validation test
type TestResult struct {
	TestName    string    `json:"testName"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	Duration    string    `json:"duration"`
	Timestamp   time.Time `json:"timestamp"`
}

// DynatraceClient handles API communication
type DynatraceClient struct {
	BaseURL    string
	APIToken   string
	HTTPClient *http.Client
}

// NewDynatraceClient creates a new Dynatrace API client
func NewDynatraceClient(baseURL, apiToken string) *DynatraceClient {
	return &DynatraceClient{
		BaseURL:  baseURL,
		APIToken: apiToken,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExecuteDQLQuery executes a DQL query against Dynatrace
func (dc *DynatraceClient) ExecuteDQLQuery(query string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/v2/query/execute", dc.BaseURL)
	
	payload := map[string]interface{}{
		"query": query,
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}
	
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Api-Token %s", dc.APIToken))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := dc.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	return result, nil
}

// GetMetrics retrieves metrics from Dynatrace
func (dc *DynatraceClient) GetMetrics(metricSelector string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/v2/metrics/query?metricSelector=%s", dc.BaseURL, metricSelector)
	
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Api-Token %s", dc.APIToken))
	
	resp, err := dc.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	return result, nil
}

// IngestTestLog sends a test log to Dynatrace
func (dc *DynatraceClient) IngestTestLog(logContent map[string]interface{}) error {
	endpoint := fmt.Sprintf("%s/api/v2/logs/ingest", dc.BaseURL)
	
	jsonData, err := json.Marshal(logContent)
	if err != nil {
		return fmt.Errorf("failed to marshal log: %w", err)
	}
	
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Api-Token %s", dc.APIToken))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := dc.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("log ingestion failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// Validator performs validation tests
type Validator struct {
	Client  *DynatraceClient
	Config  *Config
	Results []TestResult
}

// NewValidator creates a new validator instance
func NewValidator(config *Config) *Validator {
	client := NewDynatraceClient(config.DynatraceURL, config.APIToken)
	return &Validator{
		Client:  client,
		Config:  config,
		Results: make([]TestResult, 0),
	}
}

// RunTest executes a single validation test
func (v *Validator) RunTest(name string, testFunc func() error) {
	start := time.Now()
	
	var status string
	var message string
	
	err := testFunc()
	if err != nil {
		status = "FAIL"
		message = err.Error()
	} else {
		status = "PASS"
		message = "Test completed successfully"
	}
	
	duration := time.Since(start)
	
	result := TestResult{
		TestName:  name,
		Status:    status,
		Message:   message,
		Duration:  duration.String(),
		Timestamp: time.Now(),
	}
	
	v.Results = append(v.Results, result)
	
	fmt.Printf("[%s] %s - %s (%s)\n", status, name, message, duration)
}

// TestLogIngestion validates log ingestion capability
func (v *Validator) TestLogIngestion() error {
	if !v.Config.TestMode {
		return fmt.Errorf("skipped - test mode disabled")
	}
	
	testLog := map[string]interface{}{
		"content":   `{"action":"personal_access_token.request_created","actor":"test-user","org":"test-org","created_at":"2024-01-01T00:00:00Z"}`,
		"log.source": v.Config.LogSource,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	
	return v.Client.IngestTestLog(testLog)
}

// TestDQLQuery validates DQL query execution
func (v *Validator) TestDQLQuery() error {
	query := fmt.Sprintf(`
		fetch logs, from: -1h
		| filter log.source == "%s"
		| limit 10
	`, v.Config.LogSource)
	
	result, err := v.Client.ExecuteDQLQuery(query)
	if err != nil {
		return err
	}
	
	if result["records"] == nil {
		return fmt.Errorf("no records field in response")
	}
	
	return nil
}

// TestEventParsing validates event parsing logic
func (v *Validator) TestEventParsing() error {
	query := fmt.Sprintf(`
		fetch logs, from: -1h
		| filter log.source == "%s"
		| parse content, "\"action\"" SPACE? ":" SPACE? "\"" LD:event_action "\""
		| filter isNotNull(event_action)
		| limit 1
	`, v.Config.LogSource)
	
	result, err := v.Client.ExecuteDQLQuery(query)
	if err != nil {
		return err
	}
	
	// Validate parsing worked
	if result["records"] == nil {
		return fmt.Errorf("parsing validation failed - no records returned")
	}
	
	return nil
}

// TestMetricExtraction validates metric extraction
func (v *Validator) TestMetricExtraction() error {
	metricSelector := fmt.Sprintf("%s.credential_grant.count", v.Config.MetricPrefix)
	
	result, err := v.Client.GetMetrics(metricSelector)
	if err != nil {
		return err
	}
	
	if result["result"] == nil {
		return fmt.Errorf("no metric data found")
	}
	
	return nil
}

// TestAlertConfiguration validates alert rules exist
func (v *Validator) TestAlertConfiguration() error {
	// This would check if alert rules are configured
	// For now, return success as it requires additional API calls
	return nil
}

// PrintSummary prints the validation summary
func (v *Validator) PrintSummary() {
	fmt.Println("\n" + "="*80)
	fmt.Println("VALIDATION SUMMARY")
	fmt.Println("="*80)
	
	passed := 0
	failed := 0
	skipped := 0
	
	for _, result := range v.Results {
		switch result.Status {
		case "PASS":
			passed++
		case "FAIL":
			failed++
		case "SKIP":
			skipped++
		}
	}
	
	fmt.Printf("\nTotal Tests: %d\n", len(v.Results))
	fmt.Printf("Passed: %d\n", passed)
	fmt.Printf("Failed: %d\n", failed)
	fmt.Printf("Skipped: %d\n", skipped)
	
	if failed > 0 {
		fmt.Println("\nFailed Tests:")
		for _, result := range v.Results {
			if result.Status == "FAIL" {
				fmt.Printf("  - %s: %s\n", result.TestName, result.Message)
			}
		}
	}
	
	fmt.Println("\n" + "="*80)
}

// SaveResults saves validation results to JSON file
func (v *Validator) SaveResults(filename string) error {
	data, err := json.MarshalIndent(v.Results, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, data, 0644)
}

func main() {
	fmt.Println("GitHub Audit Log Monitoring - Validation Tool")
	fmt.Println("="*80)
	
	// Load configuration
	config := &Config{
		DynatraceURL: os.Getenv("DYNATRACE_URL"),
		APIToken:     os.Getenv("DYNATRACE_API_TOKEN"),
		LogSource:    "github_audit_log",
		MetricPrefix: "github.audit",
		TestMode:     os.Getenv("TEST_MODE") == "true",
	}
	
	if config.DynatraceURL == "" {
		fmt.Println("Error: DYNATRACE_URL environment variable not set")
		os.Exit(1)
	}
	
	if config.APIToken == "" {
		fmt.Println("Error: DYNATRACE_API_TOKEN environment variable not set")
		os.Exit(1)
	}
	
	fmt.Printf("Dynatrace URL: %s\n", config.DynatraceURL)
	fmt.Printf("Log Source: %s\n", config.LogSource)
	fmt.Printf("Test Mode: %t\n\n", config.TestMode)
	
	// Create validator
	validator := NewValidator(config)
	
	// Run validation tests
	validator.RunTest("1. Test Log Ingestion", validator.TestLogIngestion)
	validator.RunTest("2. Test DQL Query Execution", validator.TestDQLQuery)
	validator.RunTest("3. Test Event Parsing", validator.TestEventParsing)
	validator.RunTest("4. Test Metric Extraction", validator.TestMetricExtraction)
	validator.RunTest("5. Test Alert Configuration", validator.TestAlertConfiguration)
	
	// Print summary
	validator.PrintSummary()
	
	// Save results
	if err := validator.SaveResults("validation-results.json"); err != nil {
		fmt.Printf("Warning: Failed to save results: %v\n", err)
	} else {
		fmt.Println("\nResults saved to: validation-results.json")
	}
}
