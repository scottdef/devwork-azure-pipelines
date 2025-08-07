package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Configuration represents the template configuration
type Configuration struct {
	BundleName         string            `json:"bundle_name"`
	Environment        string            `json:"environment"`
	OutputDir          string            `json:"output_dir"`
	Variables          map[string]string `json:"variables"`
	DatabricksWorkspace string           `json:"databricks_workspace"`
	DocumentIntelligence DocumentIntelligenceConfig `json:"document_intelligence"`
}

// DocumentIntelligenceConfig represents Document Intelligence configuration
type DocumentIntelligenceConfig struct {
	Endpoint   string   `json:"endpoint"`
	APIVersion string   `json:"api_version"`
	Models     []string `json:"models"`
}

// TemplateProcessor handles processing of Go templates
type TemplateProcessor struct {
	config *Configuration
}

// NewTemplateProcessor creates a new template processor
func NewTemplateProcessor(config *Configuration) *TemplateProcessor {
	return &TemplateProcessor{config: config}
}

// ProcessTemplates processes all template files in the current directory
func (tp *TemplateProcessor) ProcessTemplates() error {
	return filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-template files
		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		return tp.processFile(path)
	})
}

// processFile processes a single template file
func (tp *TemplateProcessor) processFile(templatePath string) error {
	log.Printf("Processing template: %s", templatePath)

	// Read template file
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}

	// Parse template
	tmpl, err := template.New(filepath.Base(templatePath)).
		Funcs(tp.getFuncMap()).
		Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}

	// Determine output path
	outputPath := tp.getOutputPath(templatePath)
	outputDir := filepath.Dir(outputPath)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	defer outputFile.Close()

	// Execute template
	if err := tmpl.Execute(outputFile, tp.config); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}

	log.Printf("Generated: %s", outputPath)
	return nil
}

// getFuncMap returns template functions
func (tp *TemplateProcessor) getFuncMap() template.FuncMap {
	return template.FuncMap{
		"toUpper": strings.ToUpper,
		"toLower": strings.ToLower,
		"replace": strings.ReplaceAll,
		"contains": strings.Contains,
		"join": func(sep string, elems []string) string {
			return strings.Join(elems, sep)
		},
		"env": os.Getenv,
		"default": func(defaultValue, value string) string {
			if value == "" {
				return defaultValue
			}
			return value
		},
		"toJSON": func(v interface{}) string {
			b, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return ""
			}
			return string(b)
		},
	}
}

// getOutputPath determines the output path for a template file
func (tp *TemplateProcessor) getOutputPath(templatePath string) string {
	// Remove .tmpl extension
	outputPath := strings.TrimSuffix(templatePath, ".tmpl")
	
	// If output directory is specified, prepend it
	if tp.config.OutputDir != "" {
		// Get relative path from current directory
		relPath, err := filepath.Rel(".", outputPath)
		if err != nil {
			relPath = outputPath
		}
		outputPath = filepath.Join(tp.config.OutputDir, relPath)
	}
	
	return outputPath
}

// loadConfigurationFromFlags loads configuration from command line flags
func loadConfigurationFromFlags() (*Configuration, error) {
	var (
		bundleName  = flag.String("bundle-name", "document-intelligence-bundle", "Name of the Databricks Asset Bundle")
		environment = flag.String("environment", "nonprod", "Target environment")
		outputDir   = flag.String("output-dir", "", "Output directory for processed templates")
		configFile  = flag.String("config", "", "Path to configuration JSON file")
	)
	flag.Parse()

	config := &Configuration{
		BundleName:  *bundleName,
		Environment: *environment,
		OutputDir:   *outputDir,
		Variables:   make(map[string]string),
	}

	// Load from config file if provided
	if *configFile != "" {
		configData, err := os.ReadFile(*configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := json.Unmarshal(configData, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with command line flags
	if *bundleName != "document-intelligence-bundle" {
		config.BundleName = *bundleName
	}
	if *environment != "nonprod" {
		config.Environment = *environment
	}
	if *outputDir != "" {
		config.OutputDir = *outputDir
	}

	// Load environment variables
	if config.Variables == nil {
		config.Variables = make(map[string]string)
	}
	
	// Add common environment variables
	envVars := []string{
		"DATABRICKS_WORKSPACE_URL",
		"DOCUMENT_INTELLIGENCE_ENDPOINT",
		"AZURE_RESOURCE_GROUP",
		"AZURE_SUBSCRIPTION_ID",
		"AZURE_TENANT_ID",
	}
	
	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			config.Variables[envVar] = value
		}
	}

	return config, nil
}

// generateSampleTemplates creates sample template files for demonstration
func generateSampleTemplates() error {
	templates := map[string]string{
		"databricks.yml.tmpl": `bundle:
  name: {{ .BundleName }}

workspace:
  host: {{ .Variables.DATABRICKS_WORKSPACE_URL | default "https://your-workspace.databricks.com" }}

targets:
  {{ .Environment }}:
    mode: {{ if eq .Environment "prod" }}production{{ else }}development{{ end }}
    workspace:
      file_path: /Workspace/Users/{{ env "USER" | default "deploy-user" }}/{{ .BundleName }}
    variables:
      environment: {{ .Environment }}
      document_intelligence_endpoint: {{ .Variables.DOCUMENT_INTELLIGENCE_ENDPOINT }}

resources:
  jobs:
    document_processing_job:
      name: {{ .BundleName | toUpper }} - Document Processing - {{ .Environment | toUpper }}
      job_clusters:
        - job_cluster_key: main_cluster
          new_cluster:
            spark_version: "13.3.x-scala2.12"
            node_type_id: {{ if eq .Environment "prod" }}"Standard_DS4_v2"{{ else }}"Standard_DS3_v2"{{ end }}
            num_workers: {{ if eq .Environment "prod" }}4{{ else }}2{{ end }}
            custom_tags:
              Environment: {{ .Environment }}
              BundleName: {{ .BundleName }}
              ResourceClass: {{ if eq .Environment "prod" }}"Production"{{ else }}"Development"{{ end }}
      tasks:
        - task_key: process_documents
          job_cluster_key: main_cluster
          python_wheel_task:
            package_name: {{ .BundleName | replace "-" "_" }}
            entry_point: main
          libraries:
            - pypi:
                package: azure-ai-documentintelligence
            - pypi:
                package: azure-identity
            - pypi:
                package: pandas
            - pypi:
                package: numpy

  experiments:
    document_intelligence_experiment:
      name: {{ .BundleName }}/{{ .Environment }}/document-intelligence-experiment
      artifact_location: /Experiments/{{ .BundleName }}/{{ .Environment }}
`,

		"src/document_processor.py.tmpl": `"""
Document Intelligence processor for {{ .BundleName }}
Environment: {{ .Environment }}
Generated from template
"""

import os
import json
from azure.ai.documentintelligence import DocumentIntelligenceClient
from azure.core.credentials import AzureKeyCredential
from databricks.sdk.runtime import *
import pandas as pd

class DocumentIntelligenceProcessor:
    def __init__(self):
        self.endpoint = "{{ .Variables.DOCUMENT_INTELLIGENCE_ENDPOINT }}"
        self.api_key = dbutils.secrets.get(scope="document-intelligence", key="api-key")
        self.client = DocumentIntelligenceClient(
            endpoint=self.endpoint,
            credential=AzureKeyCredential(self.api_key)
        )
        self.environment = "{{ .Environment }}"
        
    def process_document(self, document_url: str, model_id: str = "prebuilt-document") -> dict:
        """Process a document using Azure Document Intelligence"""
        try:
            poller = self.client.begin_analyze_document(
                model_id=model_id,
                analyze_request={"url_source": document_url}
            )
            result = poller.result()
            
            return {
                "status": "success",
                "content": result.content,
                "pages": len(result.pages) if result.pages else 0,
                "tables": len(result.tables) if result.tables else 0,
                "key_value_pairs": len(result.key_value_pairs) if result.key_value_pairs else 0,
                "environment": self.environment
            }
        except Exception as e:
            return {
                "status": "error",
                "error": str(e),
                "environment": self.environment
            }

def main():
    """Main entry point for the document processor"""
    processor = DocumentIntelligenceProcessor()
    
    # Example usage - replace with actual document URLs
    test_documents = [
        "https://example.com/sample-document.pdf"
    ]
    
    results = []
    for doc_url in test_documents:
        print(f"Processing document: {doc_url}")
        result = processor.process_document(doc_url)
        results.append(result)
        print(f"Result: {result['status']}")
    
    # Save results to Delta table
    df = pd.DataFrame(results)
    spark_df = spark.createDataFrame(df)
    
    table_name = f"{{ .BundleName | replace "-" "_" }}.document_processing_results"
    spark_df.write.mode("append").saveAsTable(table_name)
    
    print(f"Processed {len(results)} documents and saved to {table_name}")

if __name__ == "__main__":
    main()
`,

		"config/{{ .Environment }}.json.tmpl": `{
  "environment": "{{ .Environment }}",
  "bundle_name": "{{ .BundleName }}",
  "databricks": {
    "workspace_url": "{{ .Variables.DATABRICKS_WORKSPACE_URL }}",
    "profile": "{{ .Environment }}"
  },
  "document_intelligence": {
    "endpoint": "{{ .Variables.DOCUMENT_INTELLIGENCE_ENDPOINT }}",
    "models": {
      "prebuilt": [
        "prebuilt-read",
        "prebuilt-document",
        "prebuilt-invoice",
        "prebuilt-receipt"
      ],
      "custom": []
    },
    "api_version": "2023-07-31"
  },
  "processing": {
    "batch_size": {{ if eq .Environment "prod" }}100{{ else }}50{{ end }},
    "max_retries": 3,
    "timeout_seconds": 300
  },
  "storage": {
    "input_path": "/mnt/{{ .Environment }}/input/documents/",
    "output_path": "/mnt/{{ .Environment }}/output/processed/",
    "error_path": "/mnt/{{ .Environment }}/errors/"
  }
}
`,
	}

	for filename, content := range templates {
		dir := filepath.Dir(filename)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write template %s: %w", filename, err)
		}

		log.Printf("Created sample template: %s", filename)
	}

	return nil
}

func main() {
	var generateSamples bool
	flag.BoolVar(&generateSamples, "generate-samples", false, "Generate sample template files")
	
	config, err := loadConfigurationFromFlags()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if generateSamples {
		log.Println("Generating sample templates...")
		if err := generateSampleTemplates(); err != nil {
			log.Fatalf("Failed to generate sample templates: %v", err)
		}
		log.Println("Sample templates generated successfully")
		return
	}

	log.Printf("Processing templates with configuration:")
	log.Printf("  Bundle Name: %s", config.BundleName)
	log.Printf("  Environment: %s", config.Environment)
	log.Printf("  Output Directory: %s", config.OutputDir)

	processor := NewTemplateProcessor(config)
	if err := processor.ProcessTemplates(); err != nil {
		log.Fatalf("Failed to process templates: %v", err)
	}

	log.Println("Template processing completed successfully")
}