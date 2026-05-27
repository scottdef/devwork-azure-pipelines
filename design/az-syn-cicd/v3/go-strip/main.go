package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ─── Main logic ────────────────────────────────────────────────────────────

// stripLinkedServices removes all resources whose "type" field ends with
// "/linkedServices" from the ARM template.  It also collects the linked
// service names so the corresponding parameters can be removed from the
// parameters file.
//
// ARM template resource entries look like:
//
//	{
//	  "name": "[concat(parameters('workspaceName'), '/LS_AzureKeyVault')]",
//	  "type": "Microsoft.Synapse/workspaces/linkedServices",
//	  "properties": { ... }
//	}
//
// The name field contains a concat expression.  We extract the linked
// service name from the second argument after the '/'.
func stripLinkedServices(template map[string]any) (map[string]any, []string) {
	resources, ok := template["resources"].([]any)
	if !ok {
		slog.Warn("no resources array in template")
		return template, nil
	}

	var kept []any
	var removedNames []string

	for _, res := range resources {
		r, ok := res.(map[string]any)
		if !ok {
			kept = append(kept, res)
			continue
		}

		resType, _ := r["type"].(string)
		if !strings.HasSuffix(resType, "/linkedServices") {
			kept = append(kept, res)
			continue
		}

		// Extract the linked service name from the name field.
		lsName := extractLinkedServiceName(r)
		removedNames = append(removedNames, lsName)
		slog.Info("  stripped resource", "name", lsName, "type", resType)
	}

	result := make(map[string]any, len(template))
	for k, v := range template {
		result[k] = v
	}
	result["resources"] = kept

	return result, removedNames
}

// extractLinkedServiceName pulls the linked service name from the ARM
// template resource's "name" field.  The field typically looks like:
//
//	"[concat(parameters('workspaceName'), '/LS_AzureKeyVault')]"
//
// We extract "LS_AzureKeyVault" from that expression.  If the format
// doesn't match, we fall back to the raw name string.
func extractLinkedServiceName(resource map[string]any) string {
	name, _ := resource["name"].(string)
	if name == "" {
		return "<unknown>"
	}

	// Try to extract from concat expression: ...'/NAME')]
	if idx := strings.LastIndex(name, "'/"); idx >= 0 {
		tail := name[idx+2:]
		if end := strings.Index(tail, "'"); end >= 0 {
			return tail[:end]
		}
	}

	// Try to extract from "workspace/name" format
	if parts := strings.SplitN(name, "/", 2); len(parts) == 2 {
		return parts[1]
	}

	return name
}

// stripLinkedServiceParams removes parameters from the parameters file
// whose names start with any of the removed linked service names.
//
// TemplateParametersForWorkspace.json has structure:
//
//	{
//	  "$schema": "...",
//	  "contentVersion": "...",
//	  "parameters": {
//	    "workspaceName": { "value": "..." },
//	    "LS_AzureKeyVault_properties_typeProperties_baseUrl": { "value": "..." },
//	    ...
//	  }
//	}
//
// If LS_AzureKeyVault was stripped, we remove all parameters whose key
// starts with "LS_AzureKeyVault".
func stripLinkedServiceParams(params map[string]any, removedNames []string) (map[string]any, []string) {
	paramsObj, ok := params["parameters"].(map[string]any)
	if !ok {
		slog.Warn("no parameters object in params file")
		return params, nil
	}

	var removedKeys []string
	cleaned := make(map[string]any, len(paramsObj))

	for key, val := range paramsObj {
		stripped := false
		for _, lsName := range removedNames {
			if strings.HasPrefix(key, lsName+"_") || key == lsName {
				removedKeys = append(removedKeys, key)
				stripped = true
				slog.Info("  stripped parameter", "key", key, "linked_service", lsName)
				break
			}
		}
		if !stripped {
			cleaned[key] = val
		}
	}

	result := make(map[string]any, len(params))
	for k, v := range params {
		result[k] = v
	}
	result["parameters"] = cleaned

	return result, removedKeys
}

// ─── File I/O ──────────────────────────────────────────────────────────────

func readJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return obj, nil
}

func writeJSON(path string, obj map[string]any) error {
	data, err := json.MarshalIndent(obj, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ─── Reporting ─────────────────────────────────────────────────────────────

func writeGitHubOutputs(removedResources []string, removedParams []string) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "stripped_resources=%d\n", len(removedResources))
	fmt.Fprintf(f, "stripped_params=%d\n", len(removedParams))
	fmt.Fprintf(f, "stripped_names=%s\n", strings.Join(removedResources, ","))
}

func writeGitHubSummary(removedResources []string, removedParams []string, dryRun bool) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintln(f, "## Linked service strip report\n")
	if dryRun {
		fmt.Fprintln(f, "> **Dry run** — files were not modified.\n")
	}

	if len(removedResources) > 0 {
		fmt.Fprintf(f, "**Stripped %d linked service(s):**\n\n", len(removedResources))
		for _, name := range removedResources {
			fmt.Fprintf(f, "- `%s`\n", name)
		}
	} else {
		fmt.Fprintln(f, "No linked services found in template.")
	}

	if len(removedParams) > 0 {
		fmt.Fprintf(f, "\n**Stripped %d parameter(s):**\n\n", len(removedParams))
		for _, key := range removedParams {
			fmt.Fprintf(f, "- `%s`\n", key)
		}
	}
}

// ─── Main ──────────────────────────────────────────────────────────────────

func usage() {
	fmt.Fprintf(os.Stderr, `strip-linked-services — Remove linked services from Synapse ARM templates

Strips all linkedServices resources from TemplateForWorkspace.json and
their corresponding parameters from TemplateParametersForWorkspace.json.
Linked services are managed directly through Synapse Studio and excluded
from CI/CD deployment.

Usage:
  strip-linked-services [flags]

Flags:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Examples:
  # Strip and overwrite in place
  strip-linked-services \
    -template ExportedArtifacts/TemplateForWorkspace.json \
    -params   ExportedArtifacts/TemplateParametersForWorkspace.json

  # Dry run — see what would be removed
  strip-linked-services \
    -template TemplateForWorkspace.json \
    -params   TemplateParametersForWorkspace.json \
    -dry-run

Exit codes:
  0  Success (linked services stripped, or none found)
  1  Error (file not found, parse failure)
`)
}

func main() {
	var (
		templatePath string
		paramsPath   string
		dryRun       bool
		verbose      bool
	)

	flag.StringVar(&templatePath, "template", "", "Path to TemplateForWorkspace.json (required)")
	flag.StringVar(&paramsPath, "params", "", "Path to TemplateParametersForWorkspace.json (optional)")
	flag.BoolVar(&dryRun, "dry-run", false, "Report what would be stripped without modifying files")
	flag.BoolVar(&verbose, "verbose", false, "Enable debug logging")

	flag.Usage = usage
	flag.Parse()

	if templatePath == "" {
		fmt.Fprintln(os.Stderr, "error: -template is required")
		flag.Usage()
		os.Exit(1)
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if dryRun {
		slog.Info("DRY RUN — files will not be modified")
	}

	// ── Process template ──
	slog.Info("Processing template", "path", templatePath)

	template, err := readJSON(templatePath)
	if err != nil {
		slog.Error("failed to read template", "err", err)
		os.Exit(1)
	}

	// Count resources before
	resourcesBefore := 0
	if res, ok := template["resources"].([]any); ok {
		resourcesBefore = len(res)
	}

	strippedTemplate, removedNames := stripLinkedServices(template)

	resourcesAfter := 0
	if res, ok := strippedTemplate["resources"].([]any); ok {
		resourcesAfter = len(res)
	}

	slog.Info(fmt.Sprintf("Template: %d resources → %d resources (%d linked services stripped)",
		resourcesBefore, resourcesAfter, len(removedNames)))

	if !dryRun && len(removedNames) > 0 {
		if err := writeJSON(templatePath, strippedTemplate); err != nil {
			slog.Error("failed to write template", "err", err)
			os.Exit(1)
		}
		slog.Info("Template written", "path", templatePath)
	}

	// ── Process parameters ──
	var removedParams []string

	if paramsPath != "" && len(removedNames) > 0 {
		slog.Info("Processing parameters", "path", paramsPath)

		params, err := readJSON(paramsPath)
		if err != nil {
			slog.Error("failed to read params", "err", err)
			os.Exit(1)
		}

		paramsBefore := 0
		if p, ok := params["parameters"].(map[string]any); ok {
			paramsBefore = len(p)
		}

		strippedParams, removed := stripLinkedServiceParams(params, removedNames)
		removedParams = removed

		paramsAfter := 0
		if p, ok := strippedParams["parameters"].(map[string]any); ok {
			paramsAfter = len(p)
		}

		slog.Info(fmt.Sprintf("Parameters: %d → %d (%d stripped)",
			paramsBefore, paramsAfter, len(removedParams)))

		if !dryRun && len(removedParams) > 0 {
			if err := writeJSON(paramsPath, strippedParams); err != nil {
				slog.Error("failed to write params", "err", err)
				os.Exit(1)
			}
			slog.Info("Parameters written", "path", paramsPath)
		}
	}

	// ── Summary ──
	fmt.Println()
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("  LINKED SERVICE STRIP REPORT")
	if dryRun {
		fmt.Println("  MODE: DRY RUN")
	}
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  Linked services removed:  %d\n", len(removedNames))
	for _, name := range removedNames {
		fmt.Printf("    • %s\n", name)
	}
	fmt.Printf("  Parameters removed:       %d\n", len(removedParams))
	for _, key := range removedParams {
		fmt.Printf("    • %s\n", key)
	}
	fmt.Println(strings.Repeat("═", 60))

	writeGitHubOutputs(removedNames, removedParams)
	writeGitHubSummary(removedNames, removedParams, dryRun)
}
