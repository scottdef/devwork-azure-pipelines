// Package export fetches the current process configuration from Azure DevOps
// and writes it as local JSON files that match the repository layout.
package export

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoolADO/ado-process-gitops/internal/client"
	"github.com/CoolADO/ado-process-gitops/internal/models"
)

// Engine exports process configuration from ADO to disk.
type Engine struct {
	ado    *client.Client
	logger *slog.Logger
}

// NewEngine creates an export engine.
func NewEngine(ado *client.Client, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{ado: ado, logger: logger}
}

// Run exports the named process into the given output directory.
// The directory structure mirrors processes/base/.
func (e *Engine) Run(processName, outputDir string) error {
	e.logger.Info("exporting process", "name", processName, "output", outputDir)

	proc, err := e.ado.FindProcessByName(processName)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	if proc == nil {
		return fmt.Errorf("process %q not found in ADO", processName)
	}

	// Write process.json.
	baseDir := filepath.Join(outputDir, "base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create base dir: %w", err)
	}

	processCfg := models.ProcessConfig{
		Name:                proc.Name,
		Description:         proc.Description,
		ParentProcessTypeID: proc.ParentProcessTypeID,
		ReferenceName:       proc.ReferenceName,
	}
	if err := writeJSON(filepath.Join(baseDir, "process.json"), processCfg); err != nil {
		return err
	}
	e.logger.Info("wrote process.json")

	// Fetch and write work-item types.
	witDir := filepath.Join(baseDir, "work-item-types")
	if err := os.MkdirAll(witDir, 0o755); err != nil {
		return fmt.Errorf("create work-item-types dir: %w", err)
	}

	wits, err := e.ado.ListWorkItemTypes(proc.TypeID)
	if err != nil {
		return fmt.Errorf("list work-item types: %w", err)
	}

	for _, wit := range wits.Value {
		if err := e.exportWIT(proc.TypeID, wit, witDir); err != nil {
			e.logger.Warn("failed to export WIT", "wit", wit.ReferenceName, "err", err)
			continue
		}
	}

	e.logger.Info("export complete",
		"workItemTypes", len(wits.Value),
		"outputDir", outputDir,
	)
	return nil
}

// exportWIT exports a single work-item type including fields, states, rules.
func (e *Engine) exportWIT(processID string, wit models.WorkItemType, dir string) error {
	cfg := models.WorkItemTypeConfig{
		ReferenceName: wit.ReferenceName,
		Name:          wit.Name,
		Description:   wit.Description,
		Color:         wit.Color,
		Icon:          wit.Icon,
		IsDisabled:    wit.IsDisabled,
		Inherits:      wit.Inherits,
	}

	// Fields.
	fields, err := e.ado.ListFields(processID, wit.ReferenceName)
	if err != nil {
		e.logger.Warn("could not fetch fields", "wit", wit.ReferenceName, "err", err)
	} else {
		cfg.Fields = fields.Value
	}

	// States.
	states, err := e.ado.ListStates(processID, wit.ReferenceName)
	if err != nil {
		e.logger.Warn("could not fetch states", "wit", wit.ReferenceName, "err", err)
	} else {
		cfg.States = states.Value
	}

	// Rules.
	rules, err := e.ado.ListRules(processID, wit.ReferenceName)
	if err != nil {
		e.logger.Warn("could not fetch rules", "wit", wit.ReferenceName, "err", err)
	} else {
		cfg.Rules = rules.Value
	}

	// File name: last segment of reference name, lower-cased.
	parts := strings.Split(wit.ReferenceName, ".")
	slug := strings.ToLower(parts[len(parts)-1])
	filename := filepath.Join(dir, slug+".json")

	return writeJSON(filename, cfg)
}

// writeJSON marshals v to pretty JSON and writes it to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
