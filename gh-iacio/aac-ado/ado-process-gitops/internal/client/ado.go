// Package client provides a thin HTTP wrapper around the Azure DevOps
// Work Item Tracking – Process REST API (api-version 7.1-preview).
//
// All methods return deserialised response structs and a plain error.
// The caller is responsible for choosing the right process type-ID,
// which can be obtained from ListProcesses.
package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/CoolADO/ado-process-gitops/internal/models"
)

const apiVersion = "7.1-preview"

// Client talks to a single Azure DevOps organization.
type Client struct {
	org        string
	pat        string
	httpClient *http.Client
	logger     *slog.Logger
}

// New creates a Client for the given organization and PAT.
func New(org, pat string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		org: org,
		pat: pat,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// ---------------------------------------------------------------------------
// internal HTTP helpers
// ---------------------------------------------------------------------------

func (c *Client) authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+c.pat))
}

// processURL builds the base URL for the process API (org-level, no project).
func (c *Client) processURL(path string) string {
	return fmt.Sprintf(
		"https://dev.azure.com/%s/_apis/work/processes/%s?api-version=%s",
		c.org, path, apiVersion,
	)
}

func (c *Client) do(method, url string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("ADO API request", "method", method, "url", url)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"ADO API %s %s returned %d: %s",
			method, url, resp.StatusCode, string(data),
		)
	}
	return data, nil
}

func (c *Client) get(url string) ([]byte, error) {
	return c.do(http.MethodGet, url, nil)
}

func (c *Client) post(url string, body any) ([]byte, error) {
	return c.do(http.MethodPost, url, body)
}

func (c *Client) patch(url string, body any) ([]byte, error) {
	return c.do(http.MethodPatch, url, body)
}

func (c *Client) put(url string, body any) ([]byte, error) {
	return c.do(http.MethodPut, url, body)
}

func (c *Client) delete(url string) ([]byte, error) {
	return c.do(http.MethodDelete, url, nil)
}

// ---------------------------------------------------------------------------
// Process CRUD
// ---------------------------------------------------------------------------

// ListProcesses returns every process in the organization.
func (c *Client) ListProcesses() (*models.ProcessList, error) {
	data, err := c.get(c.processURL(""))
	if err != nil {
		return nil, err
	}
	var list models.ProcessList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode process list: %w", err)
	}
	return &list, nil
}

// GetProcess returns a single process by its type ID.
func (c *Client) GetProcess(typeID string) (*models.Process, error) {
	data, err := c.get(c.processURL(typeID))
	if err != nil {
		return nil, err
	}
	var p models.Process
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode process: %w", err)
	}
	return &p, nil
}

// CreateProcess creates a new inherited process.
func (c *Client) CreateProcess(p *models.Process) (*models.Process, error) {
	data, err := c.post(c.processURL(""), p)
	if err != nil {
		return nil, err
	}
	var created models.Process
	if err := json.Unmarshal(data, &created); err != nil {
		return nil, fmt.Errorf("decode created process: %w", err)
	}
	return &created, nil
}

// UpdateProcess patches an existing process.
func (c *Client) UpdateProcess(typeID string, p *models.Process) (*models.Process, error) {
	data, err := c.patch(c.processURL(typeID), p)
	if err != nil {
		return nil, err
	}
	var updated models.Process
	if err := json.Unmarshal(data, &updated); err != nil {
		return nil, fmt.Errorf("decode updated process: %w", err)
	}
	return &updated, nil
}

// FindProcessByName looks up a process by its display name.
// Returns nil, nil when the process does not exist.
func (c *Client) FindProcessByName(name string) (*models.Process, error) {
	list, err := c.ListProcesses()
	if err != nil {
		return nil, err
	}
	for _, p := range list.Value {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Work Item Types
// ---------------------------------------------------------------------------

func (c *Client) witURL(processID, suffix string) string {
	base := fmt.Sprintf("%s/workItemTypes", processID)
	if suffix != "" {
		base += "/" + suffix
	}
	return c.processURL(base)
}

// ListWorkItemTypes returns all WITs for a process.
func (c *Client) ListWorkItemTypes(processID string) (*models.WorkItemTypeList, error) {
	data, err := c.get(c.witURL(processID, ""))
	if err != nil {
		return nil, err
	}
	var list models.WorkItemTypeList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode WIT list: %w", err)
	}
	return &list, nil
}

// GetWorkItemType fetches one WIT by reference name.
func (c *Client) GetWorkItemType(processID, witRef string) (*models.WorkItemType, error) {
	data, err := c.get(c.witURL(processID, witRef))
	if err != nil {
		return nil, err
	}
	var wit models.WorkItemType
	if err := json.Unmarshal(data, &wit); err != nil {
		return nil, fmt.Errorf("decode WIT: %w", err)
	}
	return &wit, nil
}

// CreateWorkItemType creates a new WIT in the process.
func (c *Client) CreateWorkItemType(processID string, wit *models.WorkItemType) (*models.WorkItemType, error) {
	data, err := c.post(c.witURL(processID, ""), wit)
	if err != nil {
		return nil, err
	}
	var created models.WorkItemType
	if err := json.Unmarshal(data, &created); err != nil {
		return nil, fmt.Errorf("decode created WIT: %w", err)
	}
	return &created, nil
}

// UpdateWorkItemType patches a WIT.
func (c *Client) UpdateWorkItemType(processID, witRef string, wit *models.WorkItemType) (*models.WorkItemType, error) {
	data, err := c.patch(c.witURL(processID, witRef), wit)
	if err != nil {
		return nil, err
	}
	var updated models.WorkItemType
	if err := json.Unmarshal(data, &updated); err != nil {
		return nil, fmt.Errorf("decode updated WIT: %w", err)
	}
	return &updated, nil
}

// DisableWorkItemType sets isDisabled=true on a WIT (soft delete).
func (c *Client) DisableWorkItemType(processID, witRef string) error {
	body := map[string]bool{"isDisabled": true}
	_, err := c.patch(c.witURL(processID, witRef), body)
	return err
}

// ---------------------------------------------------------------------------
// Fields on a WIT
// ---------------------------------------------------------------------------

func (c *Client) fieldURL(processID, witRef, suffix string) string {
	base := fmt.Sprintf("%s/workItemTypes/%s/fields", processID, witRef)
	if suffix != "" {
		base += "/" + suffix
	}
	return c.processURL(base)
}

// ListFields returns fields for a work-item type.
func (c *Client) ListFields(processID, witRef string) (*models.FieldList, error) {
	data, err := c.get(c.fieldURL(processID, witRef, ""))
	if err != nil {
		return nil, err
	}
	var list models.FieldList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode field list: %w", err)
	}
	return &list, nil
}

// AddField adds a field to a WIT.
func (c *Client) AddField(processID, witRef string, f *models.Field) (*models.Field, error) {
	data, err := c.post(c.fieldURL(processID, witRef, ""), f)
	if err != nil {
		return nil, err
	}
	var created models.Field
	if err := json.Unmarshal(data, &created); err != nil {
		return nil, fmt.Errorf("decode added field: %w", err)
	}
	return &created, nil
}

// UpdateField patches a field on a WIT.
func (c *Client) UpdateField(processID, witRef, fieldRef string, f *models.Field) (*models.Field, error) {
	data, err := c.patch(c.fieldURL(processID, witRef, fieldRef), f)
	if err != nil {
		return nil, err
	}
	var updated models.Field
	if err := json.Unmarshal(data, &updated); err != nil {
		return nil, fmt.Errorf("decode updated field: %w", err)
	}
	return &updated, nil
}

// RemoveField removes a field from a WIT.
func (c *Client) RemoveField(processID, witRef, fieldRef string) error {
	_, err := c.delete(c.fieldURL(processID, witRef, fieldRef))
	return err
}

// ---------------------------------------------------------------------------
// States on a WIT
// ---------------------------------------------------------------------------

func (c *Client) stateURL(processID, witRef, suffix string) string {
	base := fmt.Sprintf("%s/workItemTypes/%s/states", processID, witRef)
	if suffix != "" {
		base += "/" + suffix
	}
	return c.processURL(base)
}

// ListStates returns states for a work-item type.
func (c *Client) ListStates(processID, witRef string) (*models.StateList, error) {
	data, err := c.get(c.stateURL(processID, witRef, ""))
	if err != nil {
		return nil, err
	}
	var list models.StateList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode state list: %w", err)
	}
	return &list, nil
}

// CreateState creates a new state on a WIT.
func (c *Client) CreateState(processID, witRef string, s *models.State) (*models.State, error) {
	data, err := c.post(c.stateURL(processID, witRef, ""), s)
	if err != nil {
		return nil, err
	}
	var created models.State
	if err := json.Unmarshal(data, &created); err != nil {
		return nil, fmt.Errorf("decode created state: %w", err)
	}
	return &created, nil
}

// UpdateState updates a state by its ID.
func (c *Client) UpdateState(processID, witRef, stateID string, s *models.State) (*models.State, error) {
	data, err := c.put(c.stateURL(processID, witRef, stateID), s)
	if err != nil {
		return nil, err
	}
	var updated models.State
	if err := json.Unmarshal(data, &updated); err != nil {
		return nil, fmt.Errorf("decode updated state: %w", err)
	}
	return &updated, nil
}

// HideState hides a state (inherited states cannot be deleted).
func (c *Client) HideState(processID, witRef, stateID string) error {
	body := map[string]bool{"hidden": true}
	_, err := c.put(c.stateURL(processID, witRef, stateID), body)
	return err
}

// DeleteState removes a custom state.
func (c *Client) DeleteState(processID, witRef, stateID string) error {
	_, err := c.delete(c.stateURL(processID, witRef, stateID))
	return err
}

// ---------------------------------------------------------------------------
// Rules on a WIT
// ---------------------------------------------------------------------------

func (c *Client) ruleURL(processID, witRef, suffix string) string {
	base := fmt.Sprintf("%s/workItemTypes/%s/rules", processID, witRef)
	if suffix != "" {
		base += "/" + suffix
	}
	return c.processURL(base)
}

// ListRules returns rules for a work-item type.
func (c *Client) ListRules(processID, witRef string) (*models.RuleList, error) {
	data, err := c.get(c.ruleURL(processID, witRef, ""))
	if err != nil {
		return nil, err
	}
	var list models.RuleList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode rule list: %w", err)
	}
	return &list, nil
}

// CreateRule adds a new rule to a WIT.
func (c *Client) CreateRule(processID, witRef string, r *models.Rule) (*models.Rule, error) {
	data, err := c.post(c.ruleURL(processID, witRef, ""), r)
	if err != nil {
		return nil, err
	}
	var created models.Rule
	if err := json.Unmarshal(data, &created); err != nil {
		return nil, fmt.Errorf("decode created rule: %w", err)
	}
	return &created, nil
}

// UpdateRule patches a rule by its ID.
func (c *Client) UpdateRule(processID, witRef, ruleID string, r *models.Rule) (*models.Rule, error) {
	data, err := c.put(c.ruleURL(processID, witRef, ruleID), r)
	if err != nil {
		return nil, err
	}
	var updated models.Rule
	if err := json.Unmarshal(data, &updated); err != nil {
		return nil, fmt.Errorf("decode updated rule: %w", err)
	}
	return &updated, nil
}

// DeleteRule removes a rule by its ID.
func (c *Client) DeleteRule(processID, witRef, ruleID string) error {
	_, err := c.delete(c.ruleURL(processID, witRef, ruleID))
	return err
}

// ---------------------------------------------------------------------------
// Layout (pages / groups / controls)
// ---------------------------------------------------------------------------

// GetLayout returns the form layout for a WIT.
func (c *Client) GetLayout(processID, witRef string) (*models.LayoutList, error) {
	url := c.processURL(fmt.Sprintf(
		"%s/workItemTypes/%s/layout", processID, witRef,
	))
	data, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var layout models.LayoutList
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, fmt.Errorf("decode layout: %w", err)
	}
	return &layout, nil
}

// ---------------------------------------------------------------------------
// Behaviors (backlog levels)
// ---------------------------------------------------------------------------

// ListBehaviors returns behaviors for a process.
func (c *Client) ListBehaviors(processID string) (*models.BehaviorList, error) {
	url := c.processURL(fmt.Sprintf("%s/behaviors", processID))
	data, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var list models.BehaviorList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode behavior list: %w", err)
	}
	return &list, nil
}
