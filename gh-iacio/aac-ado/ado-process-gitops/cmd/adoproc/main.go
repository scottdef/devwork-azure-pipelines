// Command adoproc is the GitOps CLI for Azure DevOps Work Item Processes.
//
// Usage:
//
//	adoproc <command> [flags]
//
// Commands:
//
//	export     Export a live ADO process to local JSON files
//	validate   Validate local config files for an environment
//	diff       Show differences between local config and remote ADO state
//	sync       Apply local config to a remote ADO environment
//	report     Generate an HTML drift report using go-echarts
//	init       Scaffold a new process configuration directory
//	version    Print build version
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/CoolADO/ado-process-gitops/internal/client"
	"github.com/CoolADO/ado-process-gitops/internal/config"
	diffpkg "github.com/CoolADO/ado-process-gitops/internal/diff"
	"github.com/CoolADO/ado-process-gitops/internal/export"
	"github.com/CoolADO/ado-process-gitops/internal/report"
	syncpkg "github.com/CoolADO/ado-process-gitops/internal/sync"
	"github.com/CoolADO/ado-process-gitops/internal/validate"
)

// Build-time variables set via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel(),
	}))

	cmd := os.Args[1]
	switch cmd {
	case "export":
		runExport(os.Args[2:], logger)
	case "validate":
		runValidate(os.Args[2:], logger)
	case "diff":
		runDiff(os.Args[2:], logger)
	case "sync":
		runSync(os.Args[2:], logger)
	case "report":
		runReport(os.Args[2:], logger)
	case "init":
		runInit(os.Args[2:], logger)
	case "version":
		fmt.Printf("adoproc %s (commit=%s date=%s)\n", version, commit, date)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Common helpers
// ---------------------------------------------------------------------------

func printUsage() {
	fmt.Fprintln(os.Stderr, `adoproc — GitOps CLI for Azure DevOps Work Item Processes

Usage:
  adoproc <command> [flags]

Commands:
  export     Export a live ADO process to local JSON files
  validate   Validate local configuration files
  diff       Show differences between local config and remote ADO
  sync       Apply local config to remote ADO (supports --dry-run)
  report     Generate an HTML drift report (go-echarts)
  init       Scaffold a new process config directory
  version    Print build version

Environment Variables:
  ADO_PAT           Azure DevOps Personal Access Token (required for remote ops)
  ADO_ORG           Azure DevOps organization (overrides environments.json)
  LOG_LEVEL         Log verbosity: debug, info, warn, error (default: info)`)
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func requirePAT() string {
	pat := os.Getenv("ADO_PAT")
	if pat == "" {
		fmt.Fprintln(os.Stderr, "error: ADO_PAT environment variable is required")
		os.Exit(1)
	}
	return pat
}

func newClient(org string, logger *slog.Logger) *client.Client {
	pat := requirePAT()
	if override := os.Getenv("ADO_ORG"); override != "" {
		org = override
	}
	return client.New(org, pat, logger)
}

func newLoader(root string) *config.Loader {
	return config.NewLoader(root)
}

func exitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// export
// ---------------------------------------------------------------------------

func runExport(args []string, logger *slog.Logger) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	processName := fs.String("process", "", "Name of the ADO process to export")
	org := fs.String("org", "CoolADO", "ADO organization")
	output := fs.String("output", "processes", "Output directory")
	fs.Parse(args)

	if *processName == "" {
		fmt.Fprintln(os.Stderr, "error: --process is required")
		os.Exit(1)
	}

	ado := newClient(*org, logger)
	eng := export.NewEngine(ado, logger)
	if err := eng.Run(*processName, *output); err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "export complete")
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

func runValidate(args []string, logger *slog.Logger) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	env := fs.String("env", "", "Environment name to validate (empty = all)")
	root := fs.String("root", ".", "Repository root directory")
	fs.Parse(args)

	loader := newLoader(*root)

	if *env != "" {
		result, err := validate.Validate(loader, *env)
		if err != nil {
			fmt.Fprintf(os.Stderr, "validation error: %v\n", err)
			os.Exit(1)
		}
		exitJSON(result)
		if !result.Valid {
			os.Exit(1)
		}
		return
	}

	results, err := validate.ValidateAll(loader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validation error: %v\n", err)
		os.Exit(1)
	}
	allValid := true
	for _, r := range results {
		if !r.Valid {
			allValid = false
		}
	}
	exitJSON(results)
	if !allValid {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// diff
// ---------------------------------------------------------------------------

func runDiff(args []string, logger *slog.Logger) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	env := fs.String("env", "", "Environment name (required)")
	root := fs.String("root", ".", "Repository root directory")
	fs.Parse(args)

	if *env == "" {
		fmt.Fprintln(os.Stderr, "error: --env is required")
		os.Exit(1)
	}

	loader := newLoader(*root)
	envCfg, err := loader.FindEnvironment(*env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	envsCfg, _ := loader.LoadEnvironments()
	ado := newClient(envsCfg.Organization, logger)
	eng := diffpkg.NewEngine(ado, loader, logger)
	result, err := eng.Run(envCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diff failed: %v\n", err)
		os.Exit(1)
	}
	exitJSON(result)

	if !result.InSync {
		os.Exit(2) // Non-zero signals drift for CI/CD gating.
	}
}

// ---------------------------------------------------------------------------
// sync
// ---------------------------------------------------------------------------

func runSync(args []string, logger *slog.Logger) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	env := fs.String("env", "", "Environment name (required)")
	root := fs.String("root", ".", "Repository root directory")
	dryRun := fs.Bool("dry-run", false, "Compute actions without applying them")
	fs.Parse(args)

	if *env == "" {
		fmt.Fprintln(os.Stderr, "error: --env is required")
		os.Exit(1)
	}

	loader := newLoader(*root)
	envCfg, err := loader.FindEnvironment(*env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	envsCfg, _ := loader.LoadEnvironments()
	ado := newClient(envsCfg.Organization, logger)
	eng := syncpkg.NewEngine(ado, loader, logger)
	result, err := eng.Run(envCfg, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
		os.Exit(1)
	}
	exitJSON(result)

	if !result.Success {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// report
// ---------------------------------------------------------------------------

func runReport(args []string, logger *slog.Logger) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	root := fs.String("root", ".", "Repository root directory")
	output := fs.String("output", "reports/drift-report.html", "Output HTML file")
	fs.Parse(args)

	loader := newLoader(*root)
	envsCfg, err := loader.LoadEnvironments()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ado := newClient(envsCfg.Organization, logger)

	diffs := make(map[string]*models.DiffResult)
	for _, env := range envsCfg.Environments {
		eng := diffpkg.NewEngine(ado, loader, logger)
		d, err := eng.Run(&env)
		if err != nil {
			fmt.Fprintf(os.Stderr, "diff for %s failed: %v\n", env.Name, err)
			os.Exit(1)
		}
		diffs[env.Name] = d
	}

	gen := report.NewGenerator(logger)
	if err := gen.Generate(diffs, *output); err != nil {
		fmt.Fprintf(os.Stderr, "report generation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "report written to %s\n", *output)
}

// ---------------------------------------------------------------------------
// init  (scaffold new config directory)
// ---------------------------------------------------------------------------

func runInit(args []string, logger *slog.Logger) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	root := fs.String("root", ".", "Repository root directory")
	fs.Parse(args)

	dirs := []string{
		"processes/base/work-item-types",
		"processes/dev",
		"processes/uat",
		"processes/prod",
		"reports",
	}
	for _, d := range dirs {
		path := *root + "/" + d
		if err := os.MkdirAll(path, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating %s: %v\n", path, err)
			os.Exit(1)
		}
		logger.Info("created directory", "path", path)
	}

	// Write template environments.json if missing.
	envPath := *root + "/environments.json"
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		tmpl := models.EnvironmentsConfig{
			Organization: "CoolADO",
			Environments: []models.Environment{
				{Name: "dev", Org: "CoolADO", Project: "Dev", ProcessName: "CoolADOAgile-Dev"},
				{Name: "uat", Org: "CoolADO", Project: "UAT", ProcessName: "CoolADOAgile-UAT"},
				{Name: "prod", Org: "CoolADO", Project: "Prod", ProcessName: "CoolADOAgile-Prod"},
			},
		}
		data, _ := json.MarshalIndent(tmpl, "", "  ")
		if err := os.WriteFile(envPath, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing environments.json: %v\n", err)
			os.Exit(1)
		}
		logger.Info("created environments.json")
	}

	fmt.Fprintln(os.Stderr, "init complete – edit environments.json and add process/WIT configs")
}
