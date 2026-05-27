// sync-workspace-artifacts.go — Synchronize Synapse artifact definitions
// between workspaces for deployment homogeneity.
//
// Compares a source workspace against one or more targets.  For every
// artifact that exists in the source but is missing from a target, exports
// the full JSON definition and creates an identical copy in the target.
//
// Build:
//
//	go build -o sync-workspace-artifacts ./sync-workspace-artifacts.go
//
// Usage:
//
//	sync-workspace-artifacts \
//	  -source synapse-workspace-dev \
//	  -targets synapse-workspace-test,synapse-workspace-prod \
//	  -types linked-service \
//	  -dry-run
//
// Requires: az CLI authenticated, Go 1.21+, standard library only.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// ─── Artifact type registry ────────────────────────────────────────────────

// ArtifactType maps a human-readable type name to the az CLI subcommands
// that list, show, and create artifacts of that type.
type ArtifactType struct {
	ListCmd   string // e.g. "az synapse linked-service list"
	ShowCmd   string
	CreateCmd string
	FileFlag  string // usually "--file"
}

var artifactTypes = map[string]ArtifactType{
	"linked-service": {
		ListCmd:   "az synapse linked-service list",
		ShowCmd:   "az synapse linked-service show",
		CreateCmd: "az synapse linked-service create",
		FileFlag:  "--file",
	},
	"dataset": {
		ListCmd:   "az synapse dataset list",
		ShowCmd:   "az synapse dataset show",
		CreateCmd: "az synapse dataset create",
		FileFlag:  "--file",
	},
	"pipeline": {
		ListCmd:   "az synapse pipeline list",
		ShowCmd:   "az synapse pipeline show",
		CreateCmd: "az synapse pipeline create",
		FileFlag:  "--file",
	},
	"notebook": {
		ListCmd:   "az synapse notebook list",
		ShowCmd:   "az synapse notebook show",
		CreateCmd: "az synapse notebook create",
		FileFlag:  "--file",
	},
	"trigger": {
		ListCmd:   "az synapse trigger list",
		ShowCmd:   "az synapse trigger show",
		CreateCmd: "az synapse trigger create",
		FileFlag:  "--file",
	},
	"data-flow": {
		ListCmd:   "az synapse data-flow list",
		ShowCmd:   "az synapse data-flow show",
		CreateCmd: "az synapse data-flow create",
		FileFlag:  "--file",
	},
	"sql-script": {
		ListCmd:   "az synapse sql-script list",
		ShowCmd:   "az synapse sql-script show",
		CreateCmd: "az synapse sql-script create",
		FileFlag:  "--file",
	},
}

// validTypeNames returns a sorted list of known artifact type keys.
func validTypeNames() []string {
	names := make([]string, 0, len(artifactTypes))
	for k := range artifactTypes {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ─── Data structures ───────────────────────────────────────────────────────

// SyncResult tracks everything that happened during a single
// source→target sync for one artifact type.
type SyncResult struct {
	Source          string
	Target          string
	ArtifactType    string
	SourceCount     int
	TargetCount     int
	MissingInTarget []string
	MissingInSource []string
	Created         []string
	Failed          []string
	Skipped         []string
}

// ─── Azure CLI wrapper ─────────────────────────────────────────────────────

// az runs an az CLI command and returns stdout.  If the command fails and
// check is true, it returns an error.  If check is false, it returns nil
// output without an error (soft failure).
func az(cmdLine string, check bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := strings.Fields(cmdLine)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("az", "cmd", cmdLine)

	if err := cmd.Run(); err != nil {
		if check {
			slog.Error("az failed", "cmd", cmdLine, "stderr", stderr.String())
			return nil, fmt.Errorf("az command failed: %s\n%s", cmdLine, stderr.String())
		}
		return nil, nil // soft failure
	}
	return bytes.TrimSpace(stdout.Bytes()), nil
}

// azJSON runs an az CLI command with "-o json" appended and parses the
// result into the value pointed to by dest.  dest should be a pointer to
// a slice or map.
func azJSON(cmdLine string, check bool, dest any) error {
	raw, err := az(cmdLine+" -o json", check)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil // soft failure
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		slog.Error("json parse failed", "err", err)
		if check {
			return fmt.Errorf("json parse: %w", err)
		}
		return nil
	}
	return nil
}

// ─── Core functions ────────────────────────────────────────────────────────

// listArtifacts returns a map of artifact name → raw JSON object for every
// artifact of the given type in the workspace.
func listArtifacts(workspace, artifactType string) map[string]map[string]any {
	cfg := artifactTypes[artifactType]
	cmd := fmt.Sprintf("%s --workspace-name %s", cfg.ListCmd, workspace)

	var items []map[string]any
	if err := azJSON(cmd, false, &items); err != nil || items == nil {
		slog.Warn("could not list artifacts", "type", artifactType, "workspace", workspace)
		return nil
	}

	result := make(map[string]map[string]any, len(items))
	for _, item := range items {
		name, _ := item["name"].(string)
		if name == "" {
			name, _ = item["Name"].(string) // some commands capitalise
		}
		if name != "" {
			result[name] = item
		}
	}
	return result
}

// exportArtifact fetches the full JSON definition of a single artifact.
func exportArtifact(workspace, name, artifactType string) (map[string]any, error) {
	cfg := artifactTypes[artifactType]
	cmd := fmt.Sprintf(`%s --workspace-name %s --name "%s"`, cfg.ShowCmd, workspace, name)

	var def map[string]any
	if err := azJSON(cmd, false, &def); err != nil || def == nil {
		return nil, fmt.Errorf("could not export %s '%s' from %s", artifactType, name, workspace)
	}
	return def, nil
}

// sanitizeDefinition strips workspace-specific metadata that prevents
// cross-workspace creation (id, etag, type, resourceGroup).
func sanitizeDefinition(def map[string]any, artifactType string) map[string]any {
	stripKeys := map[string]bool{
		"id":            true,
		"etag":          true,
		"type":          true,
		"resourceGroup": true,
	}

	sanitized := make(map[string]any, len(def))
	for k, v := range def {
		if !stripKeys[k] {
			sanitized[k] = v
		}
	}

	// Log integration runtime references on linked services.
	if artifactType == "linked-service" {
		if props, ok := sanitized["properties"].(map[string]any); ok {
			if cv, ok := props["connectVia"].(map[string]any); ok {
				if irName, ok := cv["referenceName"].(string); ok && irName != "" {
					slog.Info("    references integration runtime", "ir", irName)
				}
			}
		}
	}

	return sanitized
}

// createArtifact writes the definition to a temp file and runs the az
// create command against the target workspace.
func createArtifact(workspace, name string, def map[string]any, artifactType string) error {
	cfg := artifactTypes[artifactType]

	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("synapse_%s_%d.json", name, time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	defer os.Remove(tmpFile)

	cmd := fmt.Sprintf(`%s --workspace-name %s --name "%s" %s @"%s"`,
		cfg.CreateCmd, workspace, name, cfg.FileFlag, tmpFile)

	if _, err := az(cmd, true); err != nil {
		return err
	}
	return nil
}

// setDiff returns elements in a that are not in b, sorted.
func setDiff(a, b map[string]map[string]any) []string {
	var diff []string
	for k := range a {
		if _, exists := b[k]; !exists {
			diff = append(diff, k)
		}
	}
	sort.Strings(diff)
	return diff
}

// setIntersection returns elements common to both a and b.
func setIntersection(a, b map[string]map[string]any) []string {
	var common []string
	for k := range a {
		if _, exists := b[k]; exists {
			common = append(common, k)
		}
	}
	sort.Strings(common)
	return common
}

// copyMissing exports each named artifact from srcWorkspace and creates it
// in dstWorkspace.  Respects dry-run mode.
func copyMissing(
	names []string,
	srcWorkspace, dstWorkspace, artifactType string,
	dryRun bool,
	result *SyncResult,
) {
	if len(names) == 0 {
		return
	}

	slog.Info(fmt.Sprintf("  Creating %d %s(s) in %s:", len(names), artifactType, dstWorkspace))

	for _, name := range names {
		slog.Info("    → " + name)

		if dryRun {
			result.Skipped = append(result.Skipped, name)
			continue
		}

		def, err := exportArtifact(srcWorkspace, name, artifactType)
		if err != nil {
			slog.Warn("      export failed", "err", err)
			result.Failed = append(result.Failed, name)
			continue
		}

		sanitized := sanitizeDefinition(def, artifactType)

		if err := createArtifact(dstWorkspace, name, sanitized, artifactType); err != nil {
			slog.Error("      create failed", "err", err)
			result.Failed = append(result.Failed, name)
		} else {
			slog.Info("      created")
			result.Created = append(result.Created, name)
		}
	}
}

// syncArtifacts compares source and target, then copies missing artifacts
// in the configured direction.
func syncArtifacts(source, target, artifactType, direction string, dryRun bool) SyncResult {
	result := SyncResult{
		Source:       source,
		Target:       target,
		ArtifactType: artifactType,
	}

	slog.Info(strings.Repeat("─", 60))
	slog.Info(fmt.Sprintf("Syncing %s:  %s  →  %s", artifactType, source, target))
	slog.Info(strings.Repeat("─", 60))

	// ── List both sides ──
	slog.Info(fmt.Sprintf("  Listing %ss in %s...", artifactType, source))
	sourceArtifacts := listArtifacts(source, artifactType)
	if sourceArtifacts == nil {
		sourceArtifacts = make(map[string]map[string]any)
	}
	result.SourceCount = len(sourceArtifacts)
	slog.Info(fmt.Sprintf("  Found %d in source", result.SourceCount))

	slog.Info(fmt.Sprintf("  Listing %ss in %s...", artifactType, target))
	targetArtifacts := listArtifacts(target, artifactType)
	if targetArtifacts == nil {
		targetArtifacts = make(map[string]map[string]any)
	}
	result.TargetCount = len(targetArtifacts)
	slog.Info(fmt.Sprintf("  Found %d in target", result.TargetCount))

	// ── Compute diffs ──
	missingInTarget := setDiff(sourceArtifacts, targetArtifacts)
	missingInSource := setDiff(targetArtifacts, sourceArtifacts)
	common := setIntersection(sourceArtifacts, targetArtifacts)

	result.MissingInTarget = missingInTarget
	result.MissingInSource = missingInSource

	slog.Info(fmt.Sprintf("  Common:            %d", len(common)))
	slog.Info(fmt.Sprintf("  In source only:    %d", len(missingInTarget)))
	slog.Info(fmt.Sprintf("  In target only:    %d", len(missingInSource)))

	// ── Source → Target ──
	if direction == "source-to-target" || direction == "both" {
		copyMissing(missingInTarget, source, target, artifactType, dryRun, &result)
	}

	// ── Target → Source (bidirectional) ──
	if direction == "target-to-source" || direction == "both" {
		copyMissing(missingInSource, target, source, artifactType, dryRun, &result)
	}

	return result
}

// ─── Reporting ─────────────────────────────────────────────────────────────

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

func printReport(results []SyncResult, dryRun bool) {
	fmt.Println()
	fmt.Println(strings.Repeat("═", 70))
	fmt.Println("  WORKSPACE SYNC REPORT")
	if dryRun {
		fmt.Println("  MODE: DRY RUN — no changes were made")
	}
	fmt.Println(strings.Repeat("═", 70))

	for _, r := range results {
		fmt.Printf("\n  %s:  %s  →  %s\n", r.ArtifactType, r.Source, r.Target)
		fmt.Printf("  %s\n", strings.Repeat("─", 50))
		fmt.Printf("    Source count:       %d\n", r.SourceCount)
		fmt.Printf("    Target count:       %d\n", r.TargetCount)
		fmt.Printf("    Missing in target:  %d\n", len(r.MissingInTarget))
		fmt.Printf("    Missing in source:  %d\n", len(r.MissingInSource))

		if dryRun {
			fmt.Printf("    Would create:       %d\n", len(r.Skipped))
		} else {
			fmt.Printf("    Created:            %d\n", len(r.Created))
			fmt.Printf("    Failed:             %d\n", len(r.Failed))
		}

		if len(r.MissingInTarget) > 0 {
			label := "Created/attempted"
			if dryRun {
				label = "Would create"
			}
			fmt.Printf("\n    %s in %s:\n", label, r.Target)
			for _, name := range r.MissingInTarget {
				marker := "✗"
				if contains(r.Created, name) {
					marker = "●"
				} else if dryRun {
					marker = "○"
				}
				fmt.Printf("      %s %s\n", marker, name)
			}
		}

		if len(r.MissingInSource) > 0 {
			fmt.Printf("\n    Exist only in %s (not in source):\n", r.Target)
			for _, name := range r.MissingInSource {
				fmt.Printf("      ◇ %s\n", name)
			}
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("═", 70))

	// ── GitHub Actions outputs ──
	writeGitHubOutputs(results, dryRun)
	writeGitHubSummary(results, dryRun)
}

func writeGitHubOutputs(results []SyncResult, dryRun bool) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}

	var totalCreated, totalFailed, totalMissing int
	for _, r := range results {
		totalCreated += len(r.Created)
		totalFailed += len(r.Failed)
		totalMissing += len(r.MissingInTarget)
	}

	status := "success"
	if totalFailed > 0 {
		status = "partial"
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "total_created=%d\n", totalCreated)
	fmt.Fprintf(f, "total_failed=%d\n", totalFailed)
	fmt.Fprintf(f, "total_missing=%d\n", totalMissing)
	fmt.Fprintf(f, "sync_status=%s\n", status)
}

func writeGitHubSummary(results []SyncResult, dryRun bool) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintln(f, "## Workspace Sync Report\n")

	for _, r := range results {
		fmt.Fprintf(f, "### %s: %s → %s\n", r.ArtifactType, r.Source, r.Target)
		fmt.Fprintln(f, "| Metric | Count |")
		fmt.Fprintln(f, "|---|---|")
		fmt.Fprintf(f, "| Source count | %d |\n", r.SourceCount)
		fmt.Fprintf(f, "| Target count | %d |\n", r.TargetCount)
		fmt.Fprintf(f, "| Missing in target | %d |\n", len(r.MissingInTarget))

		if dryRun {
			fmt.Fprintf(f, "| Would create | %d |\n", len(r.Skipped))
		} else {
			fmt.Fprintf(f, "| Created | %d |\n", len(r.Created))
			fmt.Fprintf(f, "| Failed | %d |\n", len(r.Failed))
		}

		if len(r.MissingInTarget) > 0 {
			fmt.Fprintln(f, "\nArtifacts:")
			for _, name := range r.MissingInTarget {
				marker := "❌"
				if contains(r.Created, name) {
					marker = "✅"
				} else if dryRun {
					marker = "⏸️"
				}
				fmt.Fprintf(f, "- %s `%s`\n", marker, name)
			}
		}
		fmt.Fprintln(f)
	}
}

// ─── CLI flag helpers ──────────────────────────────────────────────────────

// stringList implements flag.Value for comma-separated string lists.
type stringList []string

func (s *stringList) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringList) Set(val string) error {
	for _, v := range strings.Split(val, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			*s = append(*s, v)
		}
	}
	return nil
}

// ─── Main ──────────────────────────────────────────────────────────────────

func usage() {
	fmt.Fprintf(os.Stderr, `sync-workspace-artifacts — Synapse workspace homogeneity tool

Synchronize artifact definitions between Azure Synapse workspaces so ARM
templates deploy cleanly across environments without name mismatches.

Usage:
  sync-workspace-artifacts [flags]

Flags:
  -source     string   Source workspace name (required)
  -targets    string   Comma-separated target workspace names (required)
  -types      string   Comma-separated artifact types (default: linked-service)
  -direction  string   source-to-target | target-to-source | both
  -dry-run             Report differences without creating artifacts
  -compare             Alias for -dry-run
  -verbose             Enable debug logging

Valid artifact types:
  %s

Examples:
  # Dry run — see what would be synced
  sync-workspace-artifacts \
    -source synapse-workspace-dev \
    -targets synapse-workspace-test,synapse-workspace-prod \
    -types linked-service \
    -dry-run

  # Sync linked services from dev to test and prod
  sync-workspace-artifacts \
    -source synapse-workspace-dev \
    -targets synapse-workspace-test,synapse-workspace-prod \
    -types linked-service

  # Full bidirectional sync
  sync-workspace-artifacts \
    -source synapse-workspace-dev \
    -targets synapse-workspace-test \
    -types linked-service,dataset,pipeline \
    -direction both
`, strings.Join(validTypeNames(), ", "))
}

func main() {
	var (
		source    string
		targets   stringList
		types     stringList
		direction string
		dryRun    bool
		compare   bool
		verbose   bool
	)

	flag.StringVar(&source, "source", "", "Source workspace name")
	flag.Var(&targets, "targets", "Comma-separated target workspace names")
	flag.Var(&types, "types", "Comma-separated artifact types (default: linked-service)")
	flag.StringVar(&direction, "direction", "source-to-target",
		"Sync direction: source-to-target, target-to-source, both")
	flag.BoolVar(&dryRun, "dry-run", false, "Report differences without creating artifacts")
	flag.BoolVar(&compare, "compare", false, "Alias for -dry-run")
	flag.BoolVar(&verbose, "verbose", false, "Enable debug logging")

	flag.Usage = usage
	flag.Parse()

	// ── Validate inputs ──
	if source == "" {
		fmt.Fprintln(os.Stderr, "error: -source is required")
		flag.Usage()
		os.Exit(1)
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "error: -targets is required")
		flag.Usage()
		os.Exit(1)
	}
	if len(types) == 0 {
		types = stringList{"linked-service"}
	}

	validDirs := map[string]bool{
		"source-to-target": true,
		"target-to-source": true,
		"both":             true,
	}
	if !validDirs[direction] {
		fmt.Fprintf(os.Stderr, "error: invalid -direction %q\n", direction)
		os.Exit(1)
	}

	for _, t := range types {
		if _, ok := artifactTypes[t]; !ok {
			fmt.Fprintf(os.Stderr, "error: unknown artifact type %q\nvalid types: %s\n",
				t, strings.Join(validTypeNames(), ", "))
			os.Exit(1)
		}
	}

	// ── Logging ──
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	dryRun = dryRun || compare

	if dryRun {
		slog.Info("DRY RUN — no artifacts will be created")
	}

	// ── Verify az CLI authentication ──
	var account map[string]any
	if err := azJSON("az account show", true, &account); err != nil {
		slog.Error("az CLI is not authenticated — run 'az login' first")
		os.Exit(1)
	}
	if name, ok := account["name"].(string); ok {
		slog.Info("Azure account", "name", name)
	}

	// ── Run sync for each target × artifact type ──
	var allResults []SyncResult

	for _, target := range targets {
		for _, artifactType := range types {
			result := syncArtifacts(source, target, artifactType, direction, dryRun)
			allResults = append(allResults, result)
		}
	}

	// ── Report ──
	printReport(allResults, dryRun)

	// ── Exit code ──
	var totalFailed, totalMissing int
	for _, r := range allResults {
		totalFailed += len(r.Failed)
		totalMissing += len(r.MissingInTarget)
	}

	if totalFailed > 0 {
		slog.Error(fmt.Sprintf("%d artifact(s) failed to create", totalFailed))
		os.Exit(1)
	}
	if dryRun && totalMissing > 0 {
		slog.Info(fmt.Sprintf("%d artifact(s) need syncing", totalMissing))
		os.Exit(2) // non-zero so CI can detect drift
	}
}
