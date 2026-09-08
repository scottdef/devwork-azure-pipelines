// Package diff compares the local desired-state configuration against
// the live state in Azure DevOps, producing a structured DiffResult.
package diff

import (
	"fmt"
	"log/slog"

	"github.com/CoolADO/ado-process-gitops/internal/client"
	"github.com/CoolADO/ado-process-gitops/internal/config"
	"github.com/CoolADO/ado-process-gitops/internal/models"
	"time"
)

// Engine performs diff operations.
type Engine struct {
	ado    *client.Client
	loader *config.Loader
	logger *slog.Logger
}

// NewEngine creates a diff engine.
func NewEngine(ado *client.Client, loader *config.Loader, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{ado: ado, loader: loader, logger: logger}
}

// Run computes differences for the given environment.
func (e *Engine) Run(env *models.Environment) (*models.DiffResult, error) {
	e.logger.Info("computing diff", "env", env.Name, "process", env.ProcessName)

	desired, err := e.loader.BuildDesiredState(env)
	if err != nil {
		return nil, fmt.Errorf("build desired state: %w", err)
	}

	// Locate the process in ADO.
	proc, err := e.ado.FindProcessByName(env.ProcessName)
	if err != nil {
		return nil, fmt.Errorf("find process %q: %w", env.ProcessName, err)
	}

	result := &models.DiffResult{
		Environment: env.Name,
		Timestamp:   time.Now().UTC(),
		InSync:      true,
	}

	// ---- Process does not exist yet ----
	if proc == nil {
		result.InSync = false
		result.Process.Changed = true
		result.Process.FieldDiffs = append(result.Process.FieldDiffs,
			"process does not exist remotely – will be created",
		)
		// All WITs are additions.
		for _, wit := range desired.WorkItemTypes {
			result.WorkItems = append(result.WorkItems, models.WorkItemDiff{
				ReferenceName: wit.ReferenceName,
				Name:          wit.Name,
				Status:        "added",
			})
			result.Summary.Added++
		}
		result.Summary.TotalWorkItemTypes = len(desired.WorkItemTypes)
		return result, nil
	}

	// ---- Process exists – compare description ----
	if proc.Description != desired.ProcessDescription {
		result.InSync = false
		result.Process.Changed = true
		result.Process.FieldDiffs = append(result.Process.FieldDiffs,
			fmt.Sprintf("description: remote=%q local=%q", proc.Description, desired.ProcessDescription),
		)
	}

	// ---- Compare work-item types ----
	remoteWITs, err := e.ado.ListWorkItemTypes(proc.TypeID)
	if err != nil {
		return nil, fmt.Errorf("list remote WITs: %w", err)
	}

	remoteMap := make(map[string]models.WorkItemType, len(remoteWITs.Value))
	for _, w := range remoteWITs.Value {
		remoteMap[w.ReferenceName] = w
	}

	localMap := make(map[string]models.WorkItemTypeConfig, len(desired.WorkItemTypes))
	for _, w := range desired.WorkItemTypes {
		localMap[w.ReferenceName] = w
	}

	// WITs in local but not remote → added.
	for _, local := range desired.WorkItemTypes {
		remote, exists := remoteMap[local.ReferenceName]
		if !exists {
			result.WorkItems = append(result.WorkItems, models.WorkItemDiff{
				ReferenceName: local.ReferenceName,
				Name:          local.Name,
				Status:        "added",
			})
			result.Summary.Added++
			result.InSync = false
			continue
		}

		// Both exist – compare.
		wd := e.compareWIT(local, remote, proc.TypeID)
		result.WorkItems = append(result.WorkItems, wd)
		switch wd.Status {
		case "modified":
			result.Summary.Modified++
			result.InSync = false
		case "unchanged":
			result.Summary.Unchanged++
		}
	}

	// WITs in remote but not local → removed.
	for ref, remote := range remoteMap {
		if _, exists := localMap[ref]; !exists {
			result.WorkItems = append(result.WorkItems, models.WorkItemDiff{
				ReferenceName: ref,
				Name:          remote.Name,
				Status:        "removed",
			})
			result.Summary.Removed++
			result.InSync = false
		}
	}

	result.Summary.TotalWorkItemTypes = len(result.WorkItems)
	return result, nil
}

// compareWIT compares a single work-item type between local and remote.
func (e *Engine) compareWIT(
	local models.WorkItemTypeConfig,
	remote models.WorkItemType,
	processID string,
) models.WorkItemDiff {
	wd := models.WorkItemDiff{
		ReferenceName: local.ReferenceName,
		Name:          local.Name,
		Status:        "unchanged",
	}

	// Basic property diffs.
	if local.Color != "" && local.Color != remote.Color {
		wd.Status = "modified"
		wd.FieldDiffs = append(wd.FieldDiffs, models.FieldDiff{
			ReferenceName: "color",
			Action:        "modify",
			LocalValue:    local.Color,
			RemoteValue:   remote.Color,
		})
	}
	if local.Description != remote.Description {
		wd.Status = "modified"
		wd.FieldDiffs = append(wd.FieldDiffs, models.FieldDiff{
			ReferenceName: "description",
			Action:        "modify",
			LocalValue:    local.Description,
			RemoteValue:   remote.Description,
		})
	}
	if local.IsDisabled != remote.IsDisabled {
		wd.Status = "modified"
		wd.FieldDiffs = append(wd.FieldDiffs, models.FieldDiff{
			ReferenceName: "isDisabled",
			Action:        "modify",
			LocalValue:    fmt.Sprintf("%t", local.IsDisabled),
			RemoteValue:   fmt.Sprintf("%t", remote.IsDisabled),
		})
	}

	// Compare fields.
	fieldDiffs := e.compareFields(local.Fields, processID, local.ReferenceName)
	wd.FieldDiffs = append(wd.FieldDiffs, fieldDiffs...)
	if len(fieldDiffs) > 0 {
		wd.Status = "modified"
	}

	// Compare states.
	stateDiffs := e.compareStates(local.States, processID, local.ReferenceName)
	wd.StateDiffs = stateDiffs
	if len(stateDiffs) > 0 {
		wd.Status = "modified"
	}

	// Compare rules.
	ruleDiffs := e.compareRules(local.Rules, processID, local.ReferenceName)
	wd.RuleDiffs = ruleDiffs
	if len(ruleDiffs) > 0 {
		wd.Status = "modified"
	}

	return wd
}

// compareFields fetches remote fields and compares against local.
func (e *Engine) compareFields(
	localFields []models.Field,
	processID, witRef string,
) []models.FieldDiff {
	if len(localFields) == 0 {
		return nil
	}

	remoteFields, err := e.ado.ListFields(processID, witRef)
	if err != nil {
		e.logger.Warn("could not fetch remote fields", "wit", witRef, "err", err)
		return nil
	}

	remoteSet := make(map[string]models.Field, len(remoteFields.Value))
	for _, f := range remoteFields.Value {
		remoteSet[f.ReferenceName] = f
	}

	var diffs []models.FieldDiff
	for _, lf := range localFields {
		rf, exists := remoteSet[lf.ReferenceName]
		if !exists {
			diffs = append(diffs, models.FieldDiff{
				ReferenceName: lf.ReferenceName,
				Action:        "add",
				LocalValue:    lf.Name,
			})
			continue
		}
		if lf.Required != rf.Required || lf.DefaultValue != rf.DefaultValue {
			diffs = append(diffs, models.FieldDiff{
				ReferenceName: lf.ReferenceName,
				Action:        "modify",
				LocalValue:    fmt.Sprintf("required=%t default=%s", lf.Required, lf.DefaultValue),
				RemoteValue:   fmt.Sprintf("required=%t default=%s", rf.Required, rf.DefaultValue),
			})
		}
	}
	return diffs
}

// compareStates fetches remote states and compares against local.
func (e *Engine) compareStates(
	localStates []models.State,
	processID, witRef string,
) []models.StateDiff {
	if len(localStates) == 0 {
		return nil
	}

	remoteStates, err := e.ado.ListStates(processID, witRef)
	if err != nil {
		e.logger.Warn("could not fetch remote states", "wit", witRef, "err", err)
		return nil
	}

	remoteSet := make(map[string]models.State, len(remoteStates.Value))
	for _, s := range remoteStates.Value {
		remoteSet[s.Name] = s
	}

	var diffs []models.StateDiff
	for _, ls := range localStates {
		rs, exists := remoteSet[ls.Name]
		if !exists {
			diffs = append(diffs, models.StateDiff{
				Name:   ls.Name,
				Action: "add",
			})
			continue
		}
		if ls.Color != rs.Color || ls.StateCategory != rs.StateCategory {
			diffs = append(diffs, models.StateDiff{
				Name:        ls.Name,
				Action:      "modify",
				LocalValue:  fmt.Sprintf("color=%s cat=%s", ls.Color, ls.StateCategory),
				RemoteValue: fmt.Sprintf("color=%s cat=%s", rs.Color, rs.StateCategory),
			})
		}
	}
	return diffs
}

// compareRules fetches remote rules and compares against local.
func (e *Engine) compareRules(
	localRules []models.Rule,
	processID, witRef string,
) []models.RuleDiff {
	if len(localRules) == 0 {
		return nil
	}

	remoteRules, err := e.ado.ListRules(processID, witRef)
	if err != nil {
		e.logger.Warn("could not fetch remote rules", "wit", witRef, "err", err)
		return nil
	}

	remoteSet := make(map[string]models.Rule, len(remoteRules.Value))
	for _, r := range remoteRules.Value {
		remoteSet[r.Name] = r
	}

	var diffs []models.RuleDiff
	for _, lr := range localRules {
		if _, exists := remoteSet[lr.Name]; !exists {
			diffs = append(diffs, models.RuleDiff{
				Name:   lr.Name,
				Action: "add",
			})
		}
		// Rule modification comparison would require deep struct comparison.
		// For MVP we detect add/remove; modify detection is a follow-up.
	}
	return diffs
}
