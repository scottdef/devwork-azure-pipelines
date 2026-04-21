// cmd/workflow-submitter/main.go
//
// Dynatrace Workflow Submitter — Go 1.21
//
// Reads the workflow JSON from disk (or stdin), upserts it via the
// Dynatrace Platform Automations API, and then immediately triggers
// a manual execution with the synthetic monitor parameters supplied
// via CLI flags or environment variables.
//
// Usage:
//   go run ./cmd/workflow-submitter \
//     -tenant-url  https://abc12345.live.dynatrace.com \
//     -api-token   dt0c01.XXXX \
//     -workflow    ./dynatrace-workflow-create-browser-monitor.json \
//     -name        "My App Health Check" \
//     -url         "https://app.example.com" \
//     -freq        5 \
//     -locations   GEOLOCATION-9999453BE4BDB3CD,SYNTHETIC_LOCATION-DF80ACFB688C583B \
//     -app-id      APPLICATION-4ADF0EF407C7C545 \
//     -tags        production,aks,monitoring

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ─── API Types ────────────────────────────────────────────────────────────────

// WorkflowDefinition is the top-level Dynatrace Automation workflow object.
type WorkflowDefinition struct {
	ID          string                 `json:"id,omitempty"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Owner       string                 `json:"owner,omitempty"`
	OwnerType   string                 `json:"ownerType,omitempty"`
	IsPrivate   bool                   `json:"isPrivate"`
	Trigger     map[string]interface{} `json:"trigger,omitempty"`
	Tasks       map[string]interface{} `json:"tasks,omitempty"`
}

// WorkflowListResponse wraps the paginated list from GET /platform/classic/environment-api/v2/workflows.
type WorkflowListResponse struct {
	Results      []WorkflowDefinition `json:"results"`
	TotalResults int                  `json:"totalResults"`
}

// ExecutionRequest triggers a manual workflow run.
type ExecutionRequest struct {
	Params map[string]interface{} `json:"params"`
}

// ExecutionResponse is the response body from POST /workflows/{id}/run.
type ExecutionResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ─── HTTP Client ──────────────────────────────────────────────────────────────

type dtClient struct {
	tenantURL  string
	apiToken   string
	httpClient *http.Client
}

func newDTClient(tenantURL, apiToken string) *dtClient {
	return &dtClient{
		tenantURL: strings.TrimRight(tenantURL, "/"),
		apiToken:  apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *dtClient) do(method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	url := c.tenantURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Api-Token "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return data, resp.StatusCode, nil
}

// ─── Workflow Operations ───────────────────────────────────────────────────────

const workflowsAPIPath = "/platform/classic/environment-api/v2/workflows"

// findWorkflowByTitle returns the existing workflow ID if a workflow with the
// given title already exists, or an empty string if none is found.
func (c *dtClient) findWorkflowByTitle(title string) (string, error) {
	data, code, err := c.do("GET", workflowsAPIPath, nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("GET workflows HTTP %d: %s", code, string(data))
	}

	var resp WorkflowListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("decode workflow list: %w", err)
	}

	for _, wf := range resp.Results {
		if wf.Title == title {
			return wf.ID, nil
		}
	}
	return "", nil
}

// upsertWorkflow creates or replaces the workflow definition.
func (c *dtClient) upsertWorkflow(def *WorkflowDefinition) (string, error) {
	existingID, err := c.findWorkflowByTitle(def.Title)
	if err != nil {
		return "", fmt.Errorf("find workflow: %w", err)
	}

	if existingID != "" {
		// PUT to update
		log.Printf("Updating existing workflow id=%s title=%q", existingID, def.Title)
		def.ID = existingID
		data, code, err := c.do("PUT", workflowsAPIPath+"/"+existingID, def)
		if err != nil {
			return "", err
		}
		if code != http.StatusOK && code != http.StatusNoContent {
			return "", fmt.Errorf("PUT workflow HTTP %d: %s", code, string(data))
		}
		return existingID, nil
	}

	// POST to create
	log.Printf("Creating new workflow title=%q", def.Title)
	data, code, err := c.do("POST", workflowsAPIPath, def)
	if err != nil {
		return "", err
	}
	if code != http.StatusCreated && code != http.StatusOK {
		return "", fmt.Errorf("POST workflow HTTP %d: %s", code, string(data))
	}

	var created WorkflowDefinition
	if err := json.Unmarshal(data, &created); err != nil {
		return "", fmt.Errorf("decode created workflow: %w", err)
	}
	return created.ID, nil
}

// triggerExecution fires a manual execution of the workflow with the monitor parameters.
func (c *dtClient) triggerExecution(workflowID string, params map[string]interface{}) (*ExecutionResponse, error) {
	payload := ExecutionRequest{Params: params}
	path := fmt.Sprintf("%s/%s/run", workflowsAPIPath, workflowID)

	data, code, err := c.do("POST", path, payload)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK && code != http.StatusAccepted && code != http.StatusCreated {
		return nil, fmt.Errorf("trigger execution HTTP %d: %s", code, string(data))
	}

	var exec ExecutionResponse
	if err := json.Unmarshal(data, &exec); err != nil {
		return nil, fmt.Errorf("decode execution response: %w", err)
	}
	return &exec, nil
}

// pollExecution polls until the execution reaches a terminal state.
func (c *dtClient) pollExecution(workflowID, execID string, pollInterval, timeout time.Duration) (string, error) {
	path := fmt.Sprintf("%s/%s/executions/%s", workflowsAPIPath, workflowID, execID)
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return "", errors.New("timeout waiting for execution to complete")
		}

		data, code, err := c.do("GET", path, nil)
		if err != nil {
			return "", err
		}
		if code != http.StatusOK {
			return "", fmt.Errorf("GET execution HTTP %d: %s", code, string(data))
		}

		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return "", fmt.Errorf("decode execution state: %w", err)
		}

		status, _ := result["status"].(string)
		log.Printf("Execution %s status: %s", execID, status)

		switch status {
		case "SUCCEEDED":
			return status, nil
		case "FAILED", "CANCELLED", "TIMEOUT":
			return status, fmt.Errorf("execution ended with status %s", status)
		}

		time.Sleep(pollInterval)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// loadWorkflowDef reads the workflow JSON from a file path.
func loadWorkflowDef(path string) (*WorkflowDefinition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open workflow file %q: %w", path, err)
	}
	defer f.Close()

	var def WorkflowDefinition
	if err := json.NewDecoder(f).Decode(&def); err != nil {
		return nil, fmt.Errorf("decode workflow JSON: %w", err)
	}
	return &def, nil
}

// envOrFlag returns val if non-empty, otherwise the environment variable envKey.
func envOrFlag(val, envKey string) string {
	if val != "" {
		return val
	}
	return os.Getenv(envKey)
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[workflow-submitter] ")

	// ── CLI flags ────────────────────────────────────────────────────────────
	tenantURL    := flag.String("tenant-url",  "", "Dynatrace tenant URL (env: DT_TENANT_URL)")
	apiToken     := flag.String("api-token",   "", "Dynatrace API token  (env: DT_API_TOKEN)")
	workflowFile := flag.String("workflow",    "dynatrace-workflow-create-browser-monitor.json", "Path to workflow definition JSON")
	monitorName  := flag.String("name",        "", "Synthetic monitor name (required)")
	targetURL    := flag.String("url",         "", "Target URL to monitor (required)")
	freqMin      := flag.Int("freq",           5,  "Monitor frequency in minutes (min 5)")
	locationIDs  := flag.String("locations",   "", "Comma-separated Dynatrace location entity IDs (required)")
	appID        := flag.String("app-id",      "", "Dynatrace APPLICATION entity ID (required)")
	tagsRaw      := flag.String("tags",        "", "Comma-separated tags to apply")
	deviceName   := flag.String("device",      "Desktop", "Browser device profile")
	waitFor      := flag.String("wait-for",    "page_complete", "Wait condition: page_complete|network_idle")
	loadTimeMs   := flag.Int("load-time-ms",   10000, "Loading time threshold in milliseconds")
	pollSec      := flag.Int("poll-sec",       10, "Execution poll interval in seconds")
	timeoutSec   := flag.Int("timeout-sec",    300, "Execution wait timeout in seconds")
	dryRun       := flag.Bool("dry-run",       false, "Print workflow JSON and params; do not call API")
	flag.Parse()

	// ── Resolve credentials from flags or env ────────────────────────────────
	tenant := envOrFlag(*tenantURL, "DT_TENANT_URL")
	token  := envOrFlag(*apiToken,  "DT_API_TOKEN")

	if !*dryRun {
		if tenant == "" {
			log.Fatal("Dynatrace tenant URL is required. Use -tenant-url or DT_TENANT_URL.")
		}
		if token == "" {
			log.Fatal("Dynatrace API token is required. Use -api-token or DT_API_TOKEN.")
		}
	}

	// ── Validate monitor parameters ───────────────────────────────────────────
	switch {
	case *monitorName == "":
		log.Fatal("-name is required")
	case *targetURL == "":
		log.Fatal("-url is required")
	case *locationIDs == "":
		log.Fatal("-locations is required")
	case *appID == "":
		log.Fatal("-app-id is required")
	case *freqMin < 5:
		log.Fatal("-freq must be >= 5 (Dynatrace minimum)")
	}

	locations := strings.Split(*locationIDs, ",")
	for i := range locations {
		locations[i] = strings.TrimSpace(locations[i])
	}

	var tags []string
	if *tagsRaw != "" {
		for _, t := range strings.Split(*tagsRaw, ",") {
			if s := strings.TrimSpace(t); s != "" {
				tags = append(tags, s)
			}
		}
	}

	// ── Build execution parameters (match workflow task expectations) ─────────
	execParams := map[string]interface{}{
		"monitorName":                     *monitorName,
		"targetUrl":                       *targetURL,
		"frequencyMin":                    float64(*freqMin),
		"locationIds":                     locations,
		"applicationId":                   *appID,
		"tags":                            tags,
		"enabled":                         true,
		"deviceName":                      *deviceName,
		"orientation":                     "landscape",
		"waitFor":                         *waitFor,
		"loadingTimeThresholdMs":          float64(*loadTimeMs),
		"loadActionKpm":                   "VISUALLY_COMPLETE",
		"xhrActionKpm":                    "VISUALLY_COMPLETE",
		"localOutageAffectedLocations":    float64(1),
		"localOutageConsecutiveRuns":      float64(3),
	}

	// ── Load workflow definition ──────────────────────────────────────────────
	workflowDef, err := loadWorkflowDef(*workflowFile)
	if err != nil {
		log.Fatalf("Failed to load workflow: %v", err)
	}

	// ── Dry-run: print and exit ───────────────────────────────────────────────
	if *dryRun {
		fmt.Println("=== Workflow Definition ===")
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(workflowDef)
		fmt.Println("\n=== Execution Parameters ===")
		_ = enc.Encode(execParams)
		return
	}

	// ── Submit workflow ───────────────────────────────────────────────────────
	client := newDTClient(tenant, token)

	workflowID, err := client.upsertWorkflow(workflowDef)
	if err != nil {
		log.Fatalf("Failed to upsert workflow: %v", err)
	}
	log.Printf("Workflow ready: id=%s", workflowID)

	// ── Trigger manual execution ──────────────────────────────────────────────
	execResp, err := client.triggerExecution(workflowID, execParams)
	if err != nil {
		log.Fatalf("Failed to trigger execution: %v", err)
	}
	log.Printf("Execution started: id=%s status=%s", execResp.ID, execResp.Status)

	// ── Poll until terminal ───────────────────────────────────────────────────
	pollInterval := time.Duration(*pollSec) * time.Second
	timeout      := time.Duration(*timeoutSec) * time.Second
	finalStatus, err := client.pollExecution(workflowID, execResp.ID, pollInterval, timeout)
	if err != nil {
		log.Fatalf("Execution failed or timed out: %v (status=%s)", err, finalStatus)
	}

	log.Printf("Execution completed successfully: status=%s", finalStatus)
	log.Printf("Monitor '%s' provisioned at %s", *monitorName, tenant)
}
