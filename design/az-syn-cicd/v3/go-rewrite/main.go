// rewrite-arm-templates.go — Rewrite Synapse ARM templates for cross-environment deployment.
//
// Reads a pattern file of match=replace pairs and applies every substitution
// to every string value in the ARM template JSON tree.  Handles resource names,
// referenceName fields, connection strings, URLs, parameter keys — anything
// that is a string gets rewritten if it contains a matching substring.
//
// This runs BEFORE the Synapse deployment action to transform a dev-sourced
// template into one that matches the target workspace's naming conventions.
//
// Build:
//
//	go build -o rewrite-arm-templates .
//
// Usage:
//
//	rewrite-arm-templates \
//	  -patterns test-workspace-patterns.txt \
//	  -template ./synapse-workspace-dev/TemplateForWorkspace.json \
//	  -params   ./synapse-workspace-dev/TemplateParametersForWorkspace.json \
//	  -out-dir  ./rewritten
//
//	rewrite-arm-templates \
//	  -patterns prod-workspace-patterns.txt \
//	  -template TemplateForWorkspace.json \
//	  -dry-run
//
// Pattern file format:
//
//	# Lines starting with # are comments
//	# Empty lines are ignored
//	# Format:  match=replace  (first = is the delimiter)
//	dev-db-sql01=tst-db-sql01
//	link-svc-dev-kv=link-svc-tst-kv
//	devdatalake=testdatalake
//	synapse-workspace-dev=synapse-workspace-test
//
// Requires: Go 1.21+, standard library only.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ─── Pattern loading ───────────────────────────────────────────────────────

// Pattern is a single match → replace rule.
type Pattern struct {
	Match   string
	Replace string
	Line    int // line number in the pattern file (for diagnostics)
}

// parsePatterns reads a pattern file and returns the list of substitutions.
// Lines starting with # are comments.  Empty lines are skipped.  The first
// = on each line separates match from replace.
func parsePatterns(path string) ([]Pattern, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open patterns: %w", err)
	}
	defer f.Close()

	var patterns []Pattern
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first = only
		idx := strings.Index(line, "=")
		if idx < 0 {
			return nil, fmt.Errorf("line %d: no '=' delimiter: %q", lineNum, line)
		}

		match := line[:idx]
		replace := line[idx+1:]

		if match == "" {
			return nil, fmt.Errorf("line %d: empty match pattern", lineNum)
		}

		patterns = append(patterns, Pattern{
			Match:   match,
			Replace: replace,
			Line:    lineNum,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read patterns: %w", err)
	}

	return patterns, nil
}

// ─── JSON rewriting ────────────────────────────────────────────────────────

// Replacement records a single substitution that was applied.
type Replacement struct {
	Path     string // JSON path (e.g. "resources[3].name")
	OldValue string
	NewValue string
	Pattern  Pattern
}

// rewriteString applies all patterns to a single string value.
// Returns the rewritten string and any replacements that fired.
func rewriteString(val string, patterns []Pattern, jsonPath string) (string, []Replacement) {
	var replacements []Replacement
	result := val

	for _, p := range patterns {
		if strings.Contains(result, p.Match) {
			newVal := strings.ReplaceAll(result, p.Match, p.Replace)
			replacements = append(replacements, Replacement{
				Path:     jsonPath,
				OldValue: result,
				NewValue: newVal,
				Pattern:  p,
			})
			result = newVal
		}
	}

	return result, replacements
}

// rewriteValue recursively walks a JSON value and applies pattern
// substitutions to every string it encounters.  It also rewrites
// map keys (critical for TemplateParametersForWorkspace.json where
// parameter names contain linked service names).
func rewriteValue(val any, patterns []Pattern, path string) (any, []Replacement) {
	var allReplacements []Replacement

	switch v := val.(type) {

	case string:
		newVal, reps := rewriteString(v, patterns, path)
		return newVal, reps

	case map[string]any:
		newMap := make(map[string]any, len(v))
		for key, child := range v {
			childPath := path + "." + key

			// Rewrite the map key itself
			newKey, keyReps := rewriteString(key, patterns, childPath+"<key>")
			allReplacements = append(allReplacements, keyReps...)

			// Rewrite the value recursively
			newChild, childReps := rewriteValue(child, patterns, childPath)
			allReplacements = append(allReplacements, childReps...)

			newMap[newKey] = newChild
		}
		return newMap, allReplacements

	case []any:
		newSlice := make([]any, len(v))
		for i, child := range v {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			newChild, childReps := rewriteValue(child, patterns, childPath)
			allReplacements = append(allReplacements, childReps...)
			newSlice[i] = newChild
		}
		return newSlice, allReplacements

	default:
		// numbers, bools, null — pass through unchanged
		return val, nil
	}
}

// ─── File processing ───────────────────────────────────────────────────────

// processFile loads a JSON file, applies all patterns, and returns the
// rewritten content and a list of replacements.
func processFile(inputPath string, patterns []Pattern) ([]byte, []Replacement, error) {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", inputPath, err)
	}

	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", inputPath, err)
	}

	baseName := filepath.Base(inputPath)
	rewritten, replacements := rewriteValue(root, patterns, baseName)

	out, err := json.MarshalIndent(rewritten, "", "    ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal %s: %w", inputPath, err)
	}

	// Append trailing newline (match typical file conventions)
	out = append(out, '\n')

	return out, replacements, nil
}

// ─── Reporting ─────────────────────────────────────────────────────────────

func printReport(fileResults map[string][]Replacement, dryRun bool) {
	fmt.Println()
	fmt.Println(strings.Repeat("═", 74))
	fmt.Println("  ARM TEMPLATE REWRITE REPORT")
	if dryRun {
		fmt.Println("  MODE: DRY RUN — files were not modified")
	}
	fmt.Println(strings.Repeat("═", 74))

	totalReplacements := 0

	for file, reps := range fileResults {
		fmt.Printf("\n  %s  (%d substitution(s))\n", file, len(reps))
		fmt.Printf("  %s\n", strings.Repeat("─", 60))

		if len(reps) == 0 {
			fmt.Println("    No matches found")
			continue
		}

		for _, r := range reps {
			// Truncate long values for display
			oldDisplay := truncate(r.OldValue, 60)
			newDisplay := truncate(r.NewValue, 60)
			fmt.Printf("    %s\n", r.Path)
			fmt.Printf("      pattern:  %q → %q\n", r.Pattern.Match, r.Pattern.Replace)
			fmt.Printf("      before:   %s\n", oldDisplay)
			fmt.Printf("      after:    %s\n", newDisplay)
			fmt.Println()
		}

		totalReplacements += len(reps)
	}

	fmt.Println(strings.Repeat("─", 74))
	fmt.Printf("  Total: %d substitution(s) across %d file(s)\n", totalReplacements, len(fileResults))
	fmt.Println(strings.Repeat("═", 74))

	// ── GitHub Actions outputs ──
	if ghOut := os.Getenv("GITHUB_OUTPUT"); ghOut != "" {
		f, err := os.OpenFile(ghOut, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			fmt.Fprintf(f, "total_replacements=%d\n", totalReplacements)
			status := "clean"
			if totalReplacements > 0 {
				status = "rewritten"
			}
			fmt.Fprintf(f, "rewrite_status=%s\n", status)
			f.Close()
		}
	}

	// ── GitHub Actions step summary ──
	if ghSummary := os.Getenv("GITHUB_STEP_SUMMARY"); ghSummary != "" {
		f, err := os.OpenFile(ghSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			fmt.Fprintln(f, "## ARM Template Rewrite Report\n")
			for file, reps := range fileResults {
				fmt.Fprintf(f, "### %s — %d substitution(s)\n", file, len(reps))
				if len(reps) > 0 {
					fmt.Fprintln(f, "| Path | Pattern | Before → After |")
					fmt.Fprintln(f, "|---|---|---|")
					for _, r := range reps {
						fmt.Fprintf(f, "| `%s` | `%s`→`%s` | `%s` → `%s` |\n",
							truncate(r.Path, 40),
							r.Pattern.Match, r.Pattern.Replace,
							truncate(r.OldValue, 30), truncate(r.NewValue, 30))
					}
				}
				fmt.Fprintln(f)
			}
			fmt.Fprintf(f, "**Total**: %d substitution(s)\n", totalReplacements)
			f.Close()
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ─── Main ──────────────────────────────────────────────────────────────────

func usage() {
	fmt.Fprintf(os.Stderr, `rewrite-arm-templates — Rewrite Synapse ARM templates for cross-environment deployment

Applies match/replace patterns to every string value (and map key) in the
ARM template JSON tree.  Rewrites resource names, referenceName fields,
connection strings, URLs, parameter keys — anything containing a matching
substring.

Usage:
  rewrite-arm-templates [flags]

Flags:
  -patterns  string   Path to pattern file (required)
  -template  string   Path to TemplateForWorkspace.json (required)
  -params    string   Path to TemplateParametersForWorkspace.json (optional)
  -out-dir   string   Output directory for rewritten files (default: overwrite in place)
  -dry-run            Show what would change without writing files
  -verbose            Enable debug logging

Pattern file format:
  # Comment
  match_string=replacement_string
  dev-db-sql01=tst-db-sql01
  link-svc-dev-kv=link-svc-tst-kv
  devdatalake=testdatalake

Examples:
  # Preview changes
  rewrite-arm-templates \
    -patterns patterns/test.patterns.txt \
    -template ./synapse-workspace-dev/TemplateForWorkspace.json \
    -params   ./synapse-workspace-dev/TemplateParametersForWorkspace.json \
    -dry-run

  # Rewrite in place
  rewrite-arm-templates \
    -patterns patterns/test.patterns.txt \
    -template ./TemplateForWorkspace.json \
    -params   ./TemplateParametersForWorkspace.json

  # Rewrite to a separate directory
  rewrite-arm-templates \
    -patterns patterns/prod.patterns.txt \
    -template ./TemplateForWorkspace.json \
    -out-dir  ./rewritten
`)
}

func main() {
	var (
		patternsPath string
		templatePath string
		paramsPath   string
		outDir       string
		dryRun       bool
		verbose      bool
	)

	flag.StringVar(&patternsPath, "patterns", "", "Path to pattern file (required)")
	flag.StringVar(&templatePath, "template", "", "Path to TemplateForWorkspace.json (required)")
	flag.StringVar(&paramsPath, "params", "", "Path to TemplateParametersForWorkspace.json (optional)")
	flag.StringVar(&outDir, "out-dir", "", "Output directory (default: overwrite in place)")
	flag.BoolVar(&dryRun, "dry-run", false, "Show changes without writing files")
	flag.BoolVar(&verbose, "verbose", false, "Enable debug logging")

	flag.Usage = usage
	flag.Parse()

	// ── Validate ──
	if patternsPath == "" || templatePath == "" {
		fmt.Fprintln(os.Stderr, "error: -patterns and -template are required")
		flag.Usage()
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// ── Load patterns ──
	patterns, err := parsePatterns(patternsPath)
	if err != nil {
		slog.Error("failed to load patterns", "err", err)
		os.Exit(1)
	}
	slog.Info(fmt.Sprintf("Loaded %d pattern(s) from %s", len(patterns), patternsPath))

	for _, p := range patterns {
		slog.Debug("pattern", "line", p.Line, "match", p.Match, "replace", p.Replace)
	}

	// ── Create output directory if needed ──
	if outDir != "" && !dryRun {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			slog.Error("failed to create output directory", "err", err)
			os.Exit(1)
		}
	}

	// ── Process files ──
	filesToProcess := []string{templatePath}
	if paramsPath != "" {
		filesToProcess = append(filesToProcess, paramsPath)
	}

	fileResults := make(map[string][]Replacement)

	for _, inputPath := range filesToProcess {
		baseName := filepath.Base(inputPath)
		slog.Info(fmt.Sprintf("Processing %s", baseName))

		rewritten, replacements, err := processFile(inputPath, patterns)
		if err != nil {
			slog.Error("failed to process file", "file", inputPath, "err", err)
			os.Exit(1)
		}

		fileResults[baseName] = replacements

		if len(replacements) == 0 {
			slog.Info(fmt.Sprintf("  No matches in %s", baseName))
			continue
		}

		slog.Info(fmt.Sprintf("  %d substitution(s) in %s", len(replacements), baseName))

		// ── Write output ──
		if !dryRun {
			outputPath := inputPath
			if outDir != "" {
				outputPath = filepath.Join(outDir, baseName)
			}

			if err := os.WriteFile(outputPath, rewritten, 0644); err != nil {
				slog.Error("failed to write file", "path", outputPath, "err", err)
				os.Exit(1)
			}
			slog.Info(fmt.Sprintf("  Wrote %s (%d bytes)", outputPath, len(rewritten)))
		}
	}

	// ── Report ──
	printReport(fileResults, dryRun)

	// ── Exit code ──
	totalReplacements := 0
	for _, reps := range fileResults {
		totalReplacements += len(reps)
	}

	if dryRun && totalReplacements > 0 {
		os.Exit(2) // signal that changes would be made
	}
}
