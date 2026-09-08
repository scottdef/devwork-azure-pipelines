// Package sync applies the local desired-state configuration to Azure DevOps,
// creating or updating processes, work-item types, fields, states, and rules.
package sync

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/CoolADO/ado-process-gitops/internal/client"
	"github.com/CoolADO/ado-process-gitops/internal/config"
	"github.com/CoolADO/ado-process-gitops/internal/models"
)

// Engine applies configuration to ADO.
type Engine struct {
	ado    *client.Client
	loader *config.Loader
	logger *slog.Logger
}

// NewEngine creates a sync engine.
func NewEngine(ado *client.Client, loader *config.Loader, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{ado: ado, loader: loader, logger: logger}
}

// Run applies the desired state for the given environment.
// When dryRun is true, actions are computed but not executed.
func (e *Engine) Run(env *models.Environment, dryRun bool) (*models.SyncResult, error) {
	e.logger.Info("sync starting",
		"env", env.Name,
		"process", env.ProcessName,
		"dryRun", dryRun,
	)

	desired, err := e.loader.BuildDesiredState(env)
	if err != nil {
		return nil, fmt.Errorf("build desired state: %w", err)
	}

	result := &models.SyncResult{
		Environment: env.Name,
		Timestamp:   time.Now().UTC(),
		DryRun:      dryRun,
		Success:     true,
	}

	// ---- Ensure the process exists ----
	processID, err := e.ensureProcess(desired, dryRun, result)
	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, err.Error())
		return result, nil
	}

	if processID == "" && dryRun {
		// Process would be created – we can still enumerate planned actions.
		for _, wit := range desired.WorkItemTypes {
			result.Actions = append(result.Actions, models.SyncAction{
				Resource: "workItemType",
				Name:     wit.Name,
				Action:   "create",
				Status:   "skipped",
				Detail:   "dry-run: process not yet created",
			})
		}
		return result, nil
	}

	// ---- Sync work-item types ----
	if err := e.syncWorkItemTypes(processID, desired, dryRun, result); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, err.Error())
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Process-level sync
// ---------------------------------------------------------------------------

func (e *Engine) ensureProcess(
	desired *config.DesiredState,
	dryRun bool,
	result *models.SyncResult,
) (string, error) {
	proc, err := e.ado.FindProcessByName(desired.ProcessName)
	if err != nil {
		return "", fmt.Errorf("lookup process: %w", err)
	}

	if proc != nil {
		// Process exists – update description if needed.
		if proc.Description != desired.ProcessDescription {
			action := models.SyncAction{
				Resource: "process",
				Name:     desired.ProcessName,
				Action:   "update",
				Detail:   "update description",
			}
			if dryRun {
				action.Status = "skipped"
				result.Actions = append(result.Actions, action)
			} else {
				_, err := e.ado.UpdateProcess(proc.TypeID, &models.Process{
					Description: desired.ProcessDescription,
				})
				if err != nil {
					action.Status = "failed"
					action.Detail = err.Error()
					result.Actions = append(result.Actions, action)
					return proc.TypeID, nil
				}
				action.Status = "success"
				result.Actions = append(result.Actions, action)
			}
		}
		return proc.TypeID, nil
	}

	// Process does not exist – create it.
	action := models.SyncAction{
		Resource: "process",
		Name:     desired.ProcessName,
		Action:   "create",
	}

	if dryRun {
		action.Status = "skipped"
		action.Detail = "dry-run: would create process"
		result.Actions = append(result.Actions, action)
		return "", nil
	}

	created, err := e.ado.CreateProcess(&models.Process{
		Name:                desired.ProcessName,
		Description:         desired.ProcessDescription,
		ParentProcessTypeID: desired.ParentProcessID,
		ReferenceName:       desired.ReferenceName,
	})
	if err != nil {
		action.Status = "failed"
		action.Detail = err.Error()
		result.Actions = append(result.Actions, action)
		return "", fmt.Errorf("create process: %w", err)
	}
	action.Status = "success"
	action.Detail = fmt.Sprintf("typeId=%s", created.TypeID)
	result.Actions = append(result.Actions, action)
	return created.TypeID, nil
}

// ---------------------------------------------------------------------------
// WIT-level sync
// ---------------------------------------------------------------------------

func (e *Engine) syncWorkItemTypes(
	processID string,
	desired *config.DesiredState,
	dryRun bool,
	result *models.SyncResult,
) error {
	remoteList, err := e.ado.ListWorkItemTypes(processID)
	if err != nil {
		return fmt.Errorf("list remote WITs: %w", err)
	}

	remoteMap := make(map[string]models.WorkItemType, len(remoteList.Value))
	for _, w := range remoteList.Value {
		remoteMap[w.ReferenceName] = w
	}

	for _, local := range desired.WorkItemTypes {
		remote, exists := remoteMap[local.ReferenceName]

		if !exists {
			// Create the WIT.
			e.createWIT(processID, local, dryRun, result)
			continue
		}

		// Update the WIT if needed.
		e.updateWIT(processID, local, remote, dryRun, result)

		// Sync sub-resources: fields, states, rules.
		e.syncFields(processID, local, dryRun, result)
		e.syncStates(processID, local, dryRun, result)
		e.syncRules(processID, local, dryRun, result)
	}

	return nil
}

func (e *Engine) createWIT(
	processID string,
	local models.WorkItemTypeConfig,
	dryRun bool,
	result *models.SyncResult,
) {
	action := models.SyncAction{
		Resource: "workItemType",
		Name:     local.Name,
		Action:   "create",
	}

	if dryRun {
		action.Status = "skipped"
		action.Detail = "dry-run"
		result.Actions = append(result.Actions, action)
		return
	}

	_, err := e.ado.CreateWorkItemType(processID, &models.WorkItemType{
		Name:          local.Name,
		ReferenceName: local.ReferenceName,
		Description:   local.Description,
		Color:         local.Color,
		Icon:          local.Icon,
		Inherits:      local.Inherits,
	})
	if err != nil {
		action.Status = "failed"
		action.Detail = err.Error()
	} else {
		action.Status = "success"
	}
	result.Actions = append(result.Actions, action)

	// After creation, sync sub-resources.
	if err == nil {
		e.syncFields(processID, local, dryRun, result)
		e.syncStates(processID, local, dryRun, result)
		e.syncRules(processID, local, dryRun, result)
	}
}

func (e *Engine) updateWIT(
	processID string,
	local models.WorkItemTypeConfig,
	remote models.WorkItemType,
	dryRun bool,
	result *models.SyncResult,
) {
	needsUpdate := false
	if local.Description != remote.Description {
		needsUpdate = true
	}
	if local.Color != "" && local.Color != remote.Color {
		needsUpdate = true
	}
	if local.IsDisabled != remote.IsDisabled {
		needsUpdate = true
	}

	if !needsUpdate {
		return
	}

	action := models.SyncAction{
		Resource: "workItemType",
		Name:     local.Name,
		Action:   "update",
	}

	if dryRun {
		action.Status = "skipped"
		action.Detail = "dry-run"
		result.Actions = append(result.Actions, action)
		return
	}

	_, err := e.ado.UpdateWorkItemType(processID, local.ReferenceName, &models.WorkItemType{
		Description: local.Description,
		Color:       local.Color,
		IsDisabled:  local.IsDisabled,
	})
	if err != nil {
		action.Status = "failed"
		action.Detail = err.Error()
	} else {
		action.Status = "success"
	}
	result.Actions = append(result.Actions, action)
}

// ---------------------------------------------------------------------------
// Field sync
// ---------------------------------------------------------------------------

func (e *Engine) syncFields(
	processID string,
	local models.WorkItemTypeConfig,
	dryRun bool,
	result *models.SyncResult,
) {
	if len(local.Fields) == 0 {
		return
	}

	remoteFields, err := e.ado.ListFields(processID, local.ReferenceName)
	if err != nil {
		e.logger.Warn("could not list remote fields", "wit", local.ReferenceName, "err", err)
		return
	}

	remoteSet := make(map[string]models.Field, len(remoteFields.Value))
	for _, f := range remoteFields.Value {
		remoteSet[f.ReferenceName] = f
	}

	for _, lf := range local.Fields {
		rf, exists := remoteSet[lf.ReferenceName]
		if !exists {
			action := models.SyncAction{
				Resource: "field",
				Name:     fmt.Sprintf("%s/%s", local.ReferenceName, lf.ReferenceName),
				Action:   "create",
			}
			if dryRun {
				action.Status = "skipped"
			} else {
				_, err := e.ado.AddField(processID, local.ReferenceName, &lf)
				if err != nil {
					action.Status = "failed"
					action.Detail = err.Error()
				} else {
					action.Status = "success"
				}
			}
			result.Actions = append(result.Actions, action)
			continue
		}

		// Update field if properties differ.
		if lf.Required != rf.Required || lf.DefaultValue != rf.DefaultValue {
			action := models.SyncAction{
				Resource: "field",
				Name:     fmt.Sprintf("%s/%s", local.ReferenceName, lf.ReferenceName),
				Action:   "update",
			}
			if dryRun {
				action.Status = "skipped"
			} else {
				_, err := e.ado.UpdateField(processID, local.ReferenceName, lf.ReferenceName, &lf)
				if err != nil {
					action.Status = "failed"
					action.Detail = err.Error()
				} else {
					action.Status = "success"
				}
			}
			result.Actions = append(result.Actions, action)
		}
	}
}

// ---------------------------------------------------------------------------
// State sync
// ---------------------------------------------------------------------------

func (e *Engine) syncStates(
	processID string,
	local models.WorkItemTypeConfig,
	dryRun bool,
	result *models.SyncResult,
) {
	if len(local.States) == 0 {
		return
	}

	remoteStates, err := e.ado.ListStates(processID, local.ReferenceName)
	if err != nil {
		e.logger.Warn("could not list remote states", "wit", local.ReferenceName, "err", err)
		return
	}

	remoteSet := make(map[string]models.State, len(remoteStates.Value))
	for _, s := range remoteStates.Value {
		remoteSet[s.Name] = s
	}

	for _, ls := range local.States {
		rs, exists := remoteSet[ls.Name]
		if !exists {
			action := models.SyncAction{
				Resource: "state",
				Name:     fmt.Sprintf("%s/%s", local.ReferenceName, ls.Name),
				Action:   "create",
			}
			if dryRun {
				action.Status = "skipped"
			} else {
				_, err := e.ado.CreateState(processID, local.ReferenceName, &ls)
				if err != nil {
					action.Status = "failed"
					action.Detail = err.Error()
				} else {
					action.Status = "success"
				}
			}
			result.Actions = append(result.Actions, action)
			continue
		}

		// Update state if color or category changed.
		if ls.Color != rs.Color || ls.StateCategory != rs.StateCategory {
			action := models.SyncAction{
				Resource: "state",
				Name:     fmt.Sprintf("%s/%s", local.ReferenceName, ls.Name),
				Action:   "update",
			}
			if dryRun {
				action.Status = "skipped"
			} else {
				_, err := e.ado.UpdateState(processID, local.ReferenceName, rs.ID, &ls)
				if err != nil {
					action.Status = "failed"
					action.Detail = err.Error()
				} else {
					action.Status = "success"
				}
			}
			result.Actions = append(result.Actions, action)
		}
	}
}

// ---------------------------------------------------------------------------
// Rule sync
// ---------------------------------------------------------------------------

func (e *Engine) syncRules(
	processID string,
	local models.WorkItemTypeConfig,
	dryRun bool,
	result *models.SyncResult,
) {
	if len(local.Rules) == 0 {
		return
	}

	remoteRules, err := e.ado.ListRules(processID, local.ReferenceName)
	if err != nil {
		e.logger.Warn("could not list remote rules", "wit", local.ReferenceName, "err", err)
		return
	}

	remoteSet := make(map[string]models.Rule, len(remoteRules.Value))
	for _, r := range remoteRules.Value {
		remoteSet[r.Name] = r
	}

	for _, lr := range local.Rules {
		if _, exists := remoteSet[lr.Name]; !exists {
			action := models.SyncAction{
				Resource: "rule",
				Name:     fmt.Sprintf("%s/%s", local.ReferenceName, lr.Name),
				Action:   "create",
			}
			if dryRun {
				action.Status = "skipped"
			} else {
				_, err := e.ado.CreateRule(processID, local.ReferenceName, &lr)
				if err != nil {
					action.Status = "failed"
					action.Detail = err.Error()
				} else {
					action.Status = "success"
				}
			}
			result.Actions = append(result.Actions, action)
		}
	}
}
