// Package validate checks local configuration files for structural
// correctness, missing references, and policy violations before
// attempting a sync with Azure DevOps.
package validate

import (
	"fmt"
	"strings"

	"github.com/CoolADO/ado-process-gitops/internal/config"
	"github.com/CoolADO/ado-process-gitops/internal/models"
)

// Result holds all validation findings.
type Result struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Validate checks the configuration for the given environment.
func Validate(loader *config.Loader, envName string) (*Result, error) {
	r := &Result{Valid: true}

	// ---- Environments file ----
	envCfg, err := loader.LoadEnvironments()
	if err != nil {
		r.addError("environments.json: %v", err)
		return r, nil
	}
	if envCfg.Organization == "" {
		r.addError("environments.json: organization is empty")
	}
	if len(envCfg.Environments) == 0 {
		r.addError("environments.json: no environments defined")
	}

	env, err := loader.FindEnvironment(envName)
	if err != nil {
		r.addError("%v", err)
		return r, nil
	}
	if env.ProcessName == "" {
		r.addError("environment %q: processName is empty", envName)
	}
	if env.Project == "" {
		r.addError("environment %q: project is empty", envName)
	}

	// ---- Base process config ----
	proc, err := loader.LoadProcessConfig()
	if err != nil {
		r.addError("process.json: %v", err)
		return r, nil
	}
	validateProcessConfig(proc, r)

	// ---- Work-item types ----
	wits, err := loader.LoadWorkItemTypes()
	if err != nil {
		r.addError("work-item-types: %v", err)
		return r, nil
	}
	if len(wits) == 0 {
		r.addWarning("no work-item type definitions found")
	}
	seenRefs := make(map[string]bool)
	for _, wit := range wits {
		validateWIT(wit, seenRefs, r)
	}

	// ---- Overrides ----
	overrides, err := loader.LoadOverrides(envName)
	if err != nil {
		r.addError("overrides for %s: %v", envName, err)
		return r, nil
	}
	validateOverrides(overrides, seenRefs, envName, r)

	return r, nil
}

// ValidateAll runs validation for every environment in environments.json.
func ValidateAll(loader *config.Loader) (map[string]*Result, error) {
	envCfg, err := loader.LoadEnvironments()
	if err != nil {
		return nil, fmt.Errorf("load environments: %w", err)
	}
	results := make(map[string]*Result, len(envCfg.Environments))
	for _, env := range envCfg.Environments {
		res, err := Validate(loader, env.Name)
		if err != nil {
			return nil, fmt.Errorf("validate %s: %w", env.Name, err)
		}
		results[env.Name] = res
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Sub-validators
// ---------------------------------------------------------------------------

func validateProcessConfig(p *models.ProcessConfig, r *Result) {
	if p.Name == "" {
		r.addError("process.json: name is empty")
	}
	if p.ParentProcessTypeID == "" {
		r.addError("process.json: parentProcessTypeId is empty (required for inherited processes)")
	}
}

var validStateCategories = map[string]bool{
	"Proposed":   true,
	"InProgress": true,
	"Resolved":   true,
	"Completed":  true,
	"Removed":    true,
}

func validateWIT(wit models.WorkItemTypeConfig, seenRefs map[string]bool, r *Result) {
	if wit.ReferenceName == "" {
		r.addError("work-item type missing referenceName")
		return
	}
	if seenRefs[wit.ReferenceName] {
		r.addError("duplicate work-item type referenceName: %s", wit.ReferenceName)
	}
	seenRefs[wit.ReferenceName] = true

	if wit.Name == "" {
		r.addError("WIT %s: name is empty", wit.ReferenceName)
	}

	// Validate color is hex (if provided).
	if wit.Color != "" && !isHexColor(wit.Color) {
		r.addWarning("WIT %s: color %q may not be a valid hex color", wit.ReferenceName, wit.Color)
	}

	// Validate states.
	seenStates := make(map[string]bool)
	for _, s := range wit.States {
		if s.Name == "" {
			r.addError("WIT %s: state with empty name", wit.ReferenceName)
		}
		if seenStates[s.Name] {
			r.addError("WIT %s: duplicate state name %q", wit.ReferenceName, s.Name)
		}
		seenStates[s.Name] = true

		if s.StateCategory != "" && !validStateCategories[s.StateCategory] {
			r.addError("WIT %s: state %q has invalid stateCategory %q",
				wit.ReferenceName, s.Name, s.StateCategory)
		}
	}

	// Validate fields.
	seenFields := make(map[string]bool)
	for _, f := range wit.Fields {
		if f.ReferenceName == "" {
			r.addError("WIT %s: field with empty referenceName", wit.ReferenceName)
		}
		if seenFields[f.ReferenceName] {
			r.addError("WIT %s: duplicate field %q", wit.ReferenceName, f.ReferenceName)
		}
		seenFields[f.ReferenceName] = true
	}

	// Validate rules.
	for i, rule := range wit.Rules {
		if rule.Name == "" {
			r.addWarning("WIT %s: rule[%d] has no name", wit.ReferenceName, i)
		}
		if len(rule.Conditions) == 0 {
			r.addWarning("WIT %s: rule %q has no conditions", wit.ReferenceName, rule.Name)
		}
		if len(rule.Actions) == 0 {
			r.addError("WIT %s: rule %q has no actions", wit.ReferenceName, rule.Name)
		}
	}
}

func validateOverrides(ov *models.EnvOverrides, knownWITs map[string]bool, envName string, r *Result) {
	for ref := range ov.WorkItemTypes {
		if !knownWITs[ref] {
			r.addWarning("overrides[%s]: WIT override references unknown %q", envName, ref)
		}
	}
	for _, ref := range ov.DisabledWITs {
		if !knownWITs[ref] {
			r.addWarning("overrides[%s]: disabledWorkItemTypes references unknown %q", envName, ref)
		}
	}
	for ref := range ov.AdditionalFields {
		if !knownWITs[ref] {
			r.addWarning("overrides[%s]: additionalFields references unknown WIT %q", envName, ref)
		}
	}
	for ref := range ov.AdditionalStates {
		if !knownWITs[ref] {
			r.addWarning("overrides[%s]: additionalStates references unknown WIT %q", envName, ref)
		}
	}
	for ref := range ov.AdditionalRules {
		if !knownWITs[ref] {
			r.addWarning("overrides[%s]: additionalRules references unknown WIT %q", envName, ref)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func isHexColor(s string) bool {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 && len(s) != 3 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (r *Result) addError(format string, args ...any) {
	r.Valid = false
	r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
}

func (r *Result) addWarning(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}
