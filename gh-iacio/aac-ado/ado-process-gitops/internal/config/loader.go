// Package config reads the on-disk process definition files and merges
// per-environment overrides to produce the desired state for a target.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoolADO/ado-process-gitops/internal/models"
)

// Loader reads configuration from a repository root.
type Loader struct {
	root string // repository root directory
}

// NewLoader creates a Loader anchored at the given directory.
func NewLoader(root string) *Loader {
	return &Loader{root: root}
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// LoadEnvironments reads environments.json from the repo root.
func (l *Loader) LoadEnvironments() (*models.EnvironmentsConfig, error) {
	data, err := os.ReadFile(filepath.Join(l.root, "environments.json"))
	if err != nil {
		return nil, fmt.Errorf("read environments.json: %w", err)
	}
	var cfg models.EnvironmentsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse environments.json: %w", err)
	}
	return &cfg, nil
}

// FindEnvironment returns the Environment entry for the given name.
func (l *Loader) FindEnvironment(name string) (*models.Environment, error) {
	cfg, err := l.LoadEnvironments()
	if err != nil {
		return nil, err
	}
	for _, e := range cfg.Environments {
		if strings.EqualFold(e.Name, name) {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("environment %q not found in environments.json", name)
}

// LoadProcessConfig reads processes/base/process.json.
func (l *Loader) LoadProcessConfig() (*models.ProcessConfig, error) {
	data, err := os.ReadFile(filepath.Join(l.root, "processes", "base", "process.json"))
	if err != nil {
		return nil, fmt.Errorf("read process.json: %w", err)
	}
	var cfg models.ProcessConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse process.json: %w", err)
	}
	return &cfg, nil
}

// LoadWorkItemTypes reads every JSON file under processes/base/work-item-types/.
func (l *Loader) LoadWorkItemTypes() ([]models.WorkItemTypeConfig, error) {
	dir := filepath.Join(l.root, "processes", "base", "work-item-types")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read work-item-types directory: %w", err)
	}

	var wits []models.WorkItemTypeConfig
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		var wit models.WorkItemTypeConfig
		if err := json.Unmarshal(data, &wit); err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		wits = append(wits, wit)
	}
	return wits, nil
}

// LoadOverrides reads processes/<env>/overrides.json.
// Returns an empty override set (not an error) when the file is absent.
func (l *Loader) LoadOverrides(env string) (*models.EnvOverrides, error) {
	path := filepath.Join(l.root, "processes", env, "overrides.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.EnvOverrides{}, nil
		}
		return nil, fmt.Errorf("read overrides for %s: %w", env, err)
	}
	var ov models.EnvOverrides
	if err := json.Unmarshal(data, &ov); err != nil {
		return nil, fmt.Errorf("parse overrides for %s: %w", env, err)
	}
	return &ov, nil
}

// ---------------------------------------------------------------------------
// Merge logic – produces the desired state for an environment.
// ---------------------------------------------------------------------------

// DesiredState is the fully resolved configuration for one environment.
type DesiredState struct {
	ProcessName        string
	ProcessDescription string
	ParentProcessID    string
	ReferenceName      string
	WorkItemTypes      []models.WorkItemTypeConfig
}

// BuildDesiredState merges base + overrides for the given environment.
func (l *Loader) BuildDesiredState(env *models.Environment) (*DesiredState, error) {
	proc, err := l.LoadProcessConfig()
	if err != nil {
		return nil, err
	}
	wits, err := l.LoadWorkItemTypes()
	if err != nil {
		return nil, err
	}
	overrides, err := l.LoadOverrides(env.Name)
	if err != nil {
		return nil, err
	}

	ds := &DesiredState{
		ProcessName:        env.ProcessName,
		ProcessDescription: proc.Description,
		ParentProcessID:    proc.ParentProcessTypeID,
		ReferenceName:      proc.ReferenceName,
	}

	// Apply process-level overrides.
	if overrides.ProcessDescription != "" {
		ds.ProcessDescription = overrides.ProcessDescription
	}

	// Index WITs by reference name for override lookup.
	witMap := make(map[string]*models.WorkItemTypeConfig, len(wits))
	for i := range wits {
		witMap[wits[i].ReferenceName] = &wits[i]
	}

	// Apply WIT-level overrides.
	for ref, ov := range overrides.WorkItemTypes {
		wit, ok := witMap[ref]
		if !ok {
			continue
		}
		if ov.Description != "" {
			wit.Description = ov.Description
		}
		if ov.Color != "" {
			wit.Color = ov.Color
		}
		wit.IsDisabled = ov.IsDisabled
	}

	// Disable WITs listed in overrides.
	for _, ref := range overrides.DisabledWITs {
		if wit, ok := witMap[ref]; ok {
			wit.IsDisabled = true
		}
	}

	// Merge additional fields.
	for ref, fields := range overrides.AdditionalFields {
		if wit, ok := witMap[ref]; ok {
			wit.Fields = append(wit.Fields, fields...)
		}
	}

	// Merge additional states.
	for ref, states := range overrides.AdditionalStates {
		if wit, ok := witMap[ref]; ok {
			wit.States = append(wit.States, states...)
		}
	}

	// Merge additional rules.
	for ref, rules := range overrides.AdditionalRules {
		if wit, ok := witMap[ref]; ok {
			wit.Rules = append(wit.Rules, rules...)
		}
	}

	// Collect final list.
	for _, wit := range wits {
		ds.WorkItemTypes = append(ds.WorkItemTypes, wit)
	}
	return ds, nil
}
