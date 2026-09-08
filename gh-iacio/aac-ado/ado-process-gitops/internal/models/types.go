package models

import "time"

// ---------------------------------------------------------------------------
// Environment + top-level configuration
// ---------------------------------------------------------------------------

// Environment represents a target ADO project that receives a process version.
type Environment struct {
	Name        string `json:"name"`        // dev | uat | prod
	Org         string `json:"org"`         // ADO organization (CoolADO)
	Project     string `json:"project"`     // ADO project name
	ProcessName string `json:"processName"` // e.g. CoolADOAgile-Dev
}

// EnvironmentsConfig is the top-level file that maps env names to ADO targets.
type EnvironmentsConfig struct {
	Organization string        `json:"organization"`
	Environments []Environment `json:"environments"`
}

// ---------------------------------------------------------------------------
// Azure DevOps Process API models
// ---------------------------------------------------------------------------

// Process represents an inherited work-item process in ADO.
type Process struct {
	TypeID              string `json:"typeId,omitempty"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	ParentProcessTypeID string `json:"parentProcessTypeId,omitempty"`
	ReferenceName       string `json:"referenceName,omitempty"`
	IsEnabled           bool   `json:"isEnabled,omitempty"`
	IsDefault           bool   `json:"isDefault,omitempty"`
	CustomizationType   string `json:"customizationType,omitempty"`
}

// ProcessList wraps the ADO list-processes response.
type ProcessList struct {
	Count int       `json:"count"`
	Value []Process `json:"value"`
}

// WorkItemType represents a work-item type inside a process.
type WorkItemType struct {
	ReferenceName string `json:"referenceName"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Color         string `json:"color,omitempty"`
	Icon          string `json:"icon,omitempty"`
	IsDisabled    bool   `json:"isDisabled,omitempty"`
	Inherits      string `json:"inherits,omitempty"`
	IsCustomType  bool   `json:"isCustomType,omitempty"`
}

// WorkItemTypeList wraps the ADO list-work-item-types response.
type WorkItemTypeList struct {
	Count int            `json:"count"`
	Value []WorkItemType `json:"value"`
}

// Field represents a field attached to a work-item type.
type Field struct {
	ReferenceName string   `json:"referenceName"`
	Name          string   `json:"name"`
	Type          string   `json:"type,omitempty"`
	Description   string   `json:"description,omitempty"`
	Required      bool     `json:"required"`
	DefaultValue  string   `json:"defaultValue,omitempty"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	ReadOnly      bool     `json:"readOnly,omitempty"`
}

// FieldList wraps the ADO list-fields response.
type FieldList struct {
	Count int     `json:"count"`
	Value []Field `json:"value"`
}

// State represents a workflow state for a work-item type.
type State struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Color         string `json:"color"`
	StateCategory string `json:"stateCategory"`
	Order         int    `json:"order,omitempty"`
	Hidden        bool   `json:"hidden,omitempty"`
}

// StateList wraps the ADO list-states response.
type StateList struct {
	Count int     `json:"count"`
	Value []State `json:"value"`
}

// RuleCondition is a single condition inside a work-item rule.
type RuleCondition struct {
	ConditionType string `json:"conditionType"`
	Field         string `json:"field,omitempty"`
	Value         string `json:"value,omitempty"`
}

// RuleAction is a single action inside a work-item rule.
type RuleAction struct {
	ActionType   string `json:"actionType"`
	TargetField  string `json:"targetField,omitempty"`
	Value        string `json:"value,omitempty"`
}

// Rule represents an automation rule on a work-item type.
type Rule struct {
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name"`
	Conditions []RuleCondition `json:"conditions"`
	Actions    []RuleAction    `json:"actions"`
	IsDisabled bool            `json:"isDisabled,omitempty"`
}

// RuleList wraps the ADO list-rules response.
type RuleList struct {
	Count int    `json:"count"`
	Value []Rule `json:"value"`
}

// Page represents a form layout page on a work-item type.
type Page struct {
	ID         string    `json:"id,omitempty"`
	Label      string    `json:"label"`
	Order      int       `json:"order,omitempty"`
	Visible    bool      `json:"visible"`
	PageType   string    `json:"pageType,omitempty"`
	Sections   []Section `json:"sections,omitempty"`
}

// Section is a column within a layout page.
type Section struct {
	ID     string  `json:"id,omitempty"`
	Groups []Group `json:"groups,omitempty"`
}

// Group is a card/grouping of controls within a section.
type Group struct {
	ID       string    `json:"id,omitempty"`
	Label    string    `json:"label,omitempty"`
	Controls []Control `json:"controls,omitempty"`
	Visible  bool      `json:"visible,omitempty"`
	Order    int       `json:"order,omitempty"`
}

// Control represents a single control (field binding) on a form layout.
type Control struct {
	ID        string `json:"id,omitempty"`
	Label     string `json:"label,omitempty"`
	FieldName string `json:"fieldName,omitempty"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
	Visible   bool   `json:"visible,omitempty"`
	Order     int    `json:"order,omitempty"`
}

// LayoutList wraps the ADO layout/pages response.
type LayoutList struct {
	Pages []Page `json:"pages"`
}

// Behavior represents a backlog level / portfolio behavior.
type Behavior struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
	Rank        int    `json:"rank,omitempty"`
	Inherits    string `json:"inherits,omitempty"`
}

// BehaviorList wraps the ADO list-behaviors response.
type BehaviorList struct {
	Count int        `json:"count"`
	Value []Behavior `json:"value"`
}

// ---------------------------------------------------------------------------
// Local configuration models  (what lives in the repo)
// ---------------------------------------------------------------------------

// ProcessConfig is the top-level JSON that defines a process in the repo.
type ProcessConfig struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	ParentProcessTypeID string `json:"parentProcessTypeId"`
	ReferenceName       string `json:"referenceName"`
}

// WorkItemTypeConfig is the per-WIT JSON stored under work-item-types/.
type WorkItemTypeConfig struct {
	ReferenceName string  `json:"referenceName"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	Color         string  `json:"color,omitempty"`
	Icon          string  `json:"icon,omitempty"`
	IsDisabled    bool    `json:"isDisabled,omitempty"`
	Inherits      string  `json:"inherits,omitempty"`
	Fields        []Field `json:"fields,omitempty"`
	States        []State `json:"states,omitempty"`
	Rules         []Rule  `json:"rules,omitempty"`
}

// EnvOverrides lets each environment add or change fields/states per WIT.
type EnvOverrides struct {
	ProcessDescription string                       `json:"processDescription,omitempty"`
	WorkItemTypes      map[string]WITOverride        `json:"workItemTypes,omitempty"`
	DisabledWITs       []string                      `json:"disabledWorkItemTypes,omitempty"`
	AdditionalFields   map[string][]Field            `json:"additionalFields,omitempty"`
	AdditionalStates   map[string][]State            `json:"additionalStates,omitempty"`
	AdditionalRules    map[string][]Rule             `json:"additionalRules,omitempty"`
}

// WITOverride allows per-environment changes to a work-item type.
type WITOverride struct {
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	IsDisabled  bool   `json:"isDisabled,omitempty"`
}

// ---------------------------------------------------------------------------
// Diff / sync result models
// ---------------------------------------------------------------------------

// DiffResult captures differences between local config and remote ADO state.
type DiffResult struct {
	Environment string          `json:"environment"`
	Timestamp   time.Time       `json:"timestamp"`
	InSync      bool            `json:"inSync"`
	Process     ProcessDiff     `json:"process"`
	WorkItems   []WorkItemDiff  `json:"workItems"`
	Summary     DiffSummary     `json:"summary"`
}

// ProcessDiff captures top-level process differences.
type ProcessDiff struct {
	Changed     bool     `json:"changed"`
	FieldDiffs  []string `json:"fieldDiffs,omitempty"`
}

// WorkItemDiff captures differences for a single work-item type.
type WorkItemDiff struct {
	ReferenceName string      `json:"referenceName"`
	Name          string      `json:"name"`
	Status        string      `json:"status"` // unchanged | modified | added | removed
	FieldDiffs    []FieldDiff `json:"fieldDiffs,omitempty"`
	StateDiffs    []StateDiff `json:"stateDiffs,omitempty"`
	RuleDiffs     []RuleDiff  `json:"ruleDiffs,omitempty"`
}

// FieldDiff captures a single field difference.
type FieldDiff struct {
	ReferenceName string `json:"referenceName"`
	Action        string `json:"action"` // add | remove | modify
	LocalValue    string `json:"localValue,omitempty"`
	RemoteValue   string `json:"remoteValue,omitempty"`
}

// StateDiff captures a single state difference.
type StateDiff struct {
	Name       string `json:"name"`
	Action     string `json:"action"`
	LocalValue string `json:"localValue,omitempty"`
	RemoteValue string `json:"remoteValue,omitempty"`
}

// RuleDiff captures a single rule difference.
type RuleDiff struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// DiffSummary provides aggregate counts.
type DiffSummary struct {
	TotalWorkItemTypes int `json:"totalWorkItemTypes"`
	Added              int `json:"added"`
	Removed            int `json:"removed"`
	Modified           int `json:"modified"`
	Unchanged          int `json:"unchanged"`
	FieldChanges       int `json:"fieldChanges"`
	StateChanges       int `json:"stateChanges"`
	RuleChanges        int `json:"ruleChanges"`
}

// SyncResult captures the outcome of applying local config to ADO.
type SyncResult struct {
	Environment string       `json:"environment"`
	Timestamp   time.Time    `json:"timestamp"`
	DryRun      bool         `json:"dryRun"`
	Success     bool         `json:"success"`
	Actions     []SyncAction `json:"actions"`
	Errors      []string     `json:"errors,omitempty"`
}

// SyncAction records a single change applied to ADO.
type SyncAction struct {
	Resource  string `json:"resource"`  // process | workItemType | field | state | rule
	Name      string `json:"name"`
	Action    string `json:"action"`    // create | update | delete | disable
	Status    string `json:"status"`    // success | failed | skipped
	Detail    string `json:"detail,omitempty"`
}

// ---------------------------------------------------------------------------
// Report models
// ---------------------------------------------------------------------------

// ReportData aggregates information for the go-echarts HTML report.
type ReportData struct {
	GeneratedAt  time.Time              `json:"generatedAt"`
	Environments []string               `json:"environments"`
	Diffs        map[string]*DiffResult `json:"diffs"`
}
