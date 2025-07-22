package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Base structs for embedding
type DeploymentStats struct {
	TotalDeployments int `json:"total_deployments"`
	TotalSucceeded   int `json:"total_succeeded"`
	TotalFailed      int `json:"total_failed"`
}

// Simplified structs to handle the JSON parsing issues
type Definition struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Owner struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type DeploymentRecord struct {
	ID            int        `json:"id"`
	EnvironmentID int        `json:"environmentId"`
	Definition    Definition `json:"definition"`
	Owner         Owner      `json:"owner"`
	Result        string     `json:"result"`
	QueueTime     string     `json:"queueTime"`
	StartTime     string     `json:"startTime"`
	FinishTime    string     `json:"finishTime"`
}

type InputData struct {
	Count int                `json:"count"`
	Value []DeploymentRecord `json:"value"`
}

type SummaryRecord struct {
	PipelineRepo string `json:"pipeline-repo"`
	DeploymentStats
	PercentSucceeded float64 `json:"percent_succeeded"`
	Latest          string  `json:"latest"`
}

// Helper function to check if a finish time is within the last 30 days
func isWithinLast30Days(finishTimeStr string) bool {
	if finishTimeStr == "" {
		return false
	}
	
	finishTime, err := time.Parse(time.RFC3339, finishTimeStr)
	if err != nil {
		return false
	}
	
	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	
	return finishTime.After(thirtyDaysAgo) && finishTime.Before(now.Add(time.Hour)) // Add small buffer for current time
}

// Helper function to filter records based on finish time
func filterRecordsForMonthly(records []DeploymentRecord) []DeploymentRecord {
	var filtered []DeploymentRecord
	for _, record := range records {
		if isWithinLast30Days(record.FinishTime) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

// Helper function to clean and fix JSON formatting issues
func fixJSON(jsonStr string) string {
	// Fix incomplete URLs that are missing closing quotes
	re := regexp.MustCompile(`"href":\s*"https://dev\.azure\.com/redacted[^"]*\n`)
	fixed := re.ReplaceAllString(jsonStr, `"href": "https://dev.azure.com/redacted"`)
	
	// Remove any other potential line break issues in string values
	re2 := regexp.MustCompile(`"([^"]*)\n([^"]*)"`)
	fixed = re2.ReplaceAllString(fixed, `"$1$2"`)
	
	return fixed
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	// Parse command line arguments
	var monthly bool
	var inputFile string
	flag.BoolVar(&monthly, "monthly", false, "Filter records to last 30 days only")
	flag.StringVar(&inputFile, "input", "env-dep-rec-res.json", "Input JSON file path")
	flag.Parse()

	// Read input file
	data, err := os.ReadFile(inputFile)
	if err != nil {
		log.Fatal("Error reading file:", err)
	}

	// Fix JSON formatting issues
	jsonStr := string(data)
	fixedJSON := fixJSON(jsonStr)

	// Parse JSON
	var input InputData
	if err := json.Unmarshal([]byte(fixedJSON), &input); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		
		// Try to find where the error occurs
		lines := strings.Split(fixedJSON, "\n")
		fmt.Printf("JSON has %d lines\n", len(lines))
		
		// Show first few lines for debugging
		for i, line := range lines[:min(10, len(lines))] {
			fmt.Printf("Line %d: %s\n", i+1, line)
		}
		
		log.Fatal("JSON parsing failed")
	}

	fmt.Printf("Processing input file: %s\n", inputFile)
	fmt.Printf("Successfully parsed %d deployment records\n", len(input.Value))

	// Apply monthly filter if requested
	var recordsToProcess []DeploymentRecord
	if monthly {
		recordsToProcess = filterRecordsForMonthly(input.Value)
		fmt.Printf("Filtered to %d records from the last 30 days\n", len(recordsToProcess))
		
		if len(recordsToProcess) == 0 {
			fmt.Println("No records found in the last 30 days")
			return
		}
	} else {
		recordsToProcess = input.Value
	}

	// Group by pipeline repo
	groups := make(map[string][]DeploymentRecord)
	var environmentID int
	
	for _, record := range recordsToProcess {
		environmentID = record.EnvironmentID
		groups[record.Definition.Name] = append(groups[record.Definition.Name], record)
	}

	// Calculate summaries
	var summaries []SummaryRecord
	
	for pipelineRepo, records := range groups {
		stats := DeploymentStats{
			TotalDeployments: len(records),
		}
		
		var latestTime time.Time
		var latestTimeStr string
		
		for _, record := range records {
			switch record.Result {
			case "succeeded":
				stats.TotalSucceeded++
			case "failed":
				stats.TotalFailed++
			}
			
			// Parse finish time and find latest
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
			percentSucceeded = math.Round(float64(stats.TotalSucceeded) / float64(stats.TotalDeployments) * 100 * 100) / 100
		}
		
		summary := SummaryRecord{
			PipelineRepo:     pipelineRepo,
			DeploymentStats:  stats,
			PercentSucceeded: percentSucceeded,
			Latest:          latestTimeStr,
		}
		
		summaries = append(summaries, summary)
	}
	
	// Sort by pipeline repo name for consistent output
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].PipelineRepo < summaries[j].PipelineRepo
	})
	
	// Write output
	outputData, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		log.Fatal("Error marshaling output:", err)
	}
	
	// Create output filename with monthly indicator if applicable
	var outputFilename string
	if monthly {
		outputFilename = fmt.Sprintf("deploy-summary-%d-monthly.json", environmentID)
	} else {
		outputFilename = fmt.Sprintf("deploy-summary-%d.json", environmentID)
	}
	
	if err := os.WriteFile(outputFilename, outputData, 0644); err != nil {
		log.Fatal("Error writing output file:", err)
	}
	
	fmt.Printf("Summary written to %s\n", outputFilename)
	
	if monthly {
		fmt.Printf("Processed %d deployment records (last 30 days) across %d pipeline repos\n", len(recordsToProcess), len(summaries))
	} else {
		fmt.Printf("Processed %d deployment records (all time) across %d pipeline repos\n", len(recordsToProcess), len(summaries))
	}
	
	// Print a preview of the summary
	if monthly {
		fmt.Printf("\nSample summary data (last 30 days):\n")
	} else {
		fmt.Printf("\nSample summary data (all time):\n")
	}
	
	for i, summary := range summaries {
		if i >= 3 { // Show only first 3 for preview
			break
		}
		fmt.Printf("  %s: %d deployments, %.1f%% success, latest: %s\n", 
			summary.PipelineRepo, summary.TotalDeployments, summary.PercentSucceeded, summary.Latest)
	}
	
	if monthly {
		thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
		fmt.Printf("\nTime range: %s to %s\n", 
			thirtyDaysAgo.Format("2006-01-02"), time.Now().Format("2006-01-02"))
	}
}