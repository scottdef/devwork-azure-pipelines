package ghapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// WeeklyCodeFrequency is one [week_unix, additions, deletions] triple.
type WeeklyCodeFrequency struct {
	Week      time.Time
	Additions int
	Deletions int
}

// CodeFrequency returns weekly additions/deletions for the default branch.
// GitHub computes statistics asynchronously and answers 202 until ready; the
// call polls a few times. Repositories with 10,000+ commits return 422.
func (c *Client) CodeFrequency(ctx context.Context, owner, repo string) ([]WeeklyCodeFrequency, error) {
	for attempt := 0; attempt < 6; attempt++ {
		resp, err := c.Do(ctx, Request{Path: fmt.Sprintf("repos/%s/%s/stats/code_frequency", owner, repo), OK: []int{http.StatusAccepted, http.StatusNoContent, http.StatusUnprocessableEntity}})
		if err != nil {
			return nil, err
		}
		switch resp.Status {
		case http.StatusAccepted:
			if c.Sleep != nil {
				c.Sleep(time.Duration(2+attempt*2) * time.Second)
			}
			continue
		case http.StatusNoContent, http.StatusUnprocessableEntity:
			return nil, nil
		}
		var raw [][3]int64
		if err := json.Unmarshal(resp.Body, &raw); err != nil {
			return nil, fmt.Errorf("ghapi: code_frequency: %w", err)
		}
		out := make([]WeeklyCodeFrequency, 0, len(raw))
		for _, r := range raw {
			del := int(r[2])
			if del < 0 {
				del = -del
			}
			out = append(out, WeeklyCodeFrequency{Week: time.Unix(r[0], 0).UTC(), Additions: int(r[1]), Deletions: del})
		}
		return out, nil
	}
	return nil, nil
}

// CommitCount counts default-branch commits in [since, until) using the
// per_page=1 + rel="last" technique (one request).
func (c *Client) CommitCount(ctx context.Context, owner, repo string, since, until time.Time) (int, error) {
	q := url.Values{"per_page": {"1"}, "since": {since.UTC().Format(time.RFC3339)}, "until": {until.UTC().Format(time.RFC3339)}}
	resp, err := c.Do(ctx, Request{Path: fmt.Sprintf("repos/%s/%s/commits", owner, repo), Query: q, OK: []int{http.StatusConflict}})
	if err != nil {
		return 0, err
	}
	if resp.Status == http.StatusConflict { // empty repository
		return 0, nil
	}
	if n := LastPage(resp.Header); n > 0 {
		return n, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(resp.Body, &items); err != nil {
		return 0, err
	}
	return len(items), nil
}

// DispatchWorkflow triggers a workflow_dispatch event.
func (c *Client) DispatchWorkflow(ctx context.Context, owner, repo, workflow, ref string, inputs map[string]string) error {
	body := map[string]any{"ref": ref}
	if len(inputs) > 0 {
		body["inputs"] = inputs
	}
	_, err := c.Do(ctx, Request{Method: http.MethodPost, Path: fmt.Sprintf("repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, url.PathEscape(workflow)), Body: body})
	return err
}

// AgentTask is a Copilot cloud agent task (agent tasks API, public preview).
type AgentTask struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url,omitempty"`
}

// CreateAgentTask starts a Copilot cloud agent task. Requires a user-to-server
// token; installation tokens are not supported by this API.
func (c *Client) CreateAgentTask(ctx context.Context, owner, repo, prompt, baseRef, model string, createPR bool) (*AgentTask, error) {
	body := map[string]any{"prompt": prompt, "create_pull_request": createPR}
	if baseRef != "" {
		body["base_ref"] = baseRef
	}
	if model != "" {
		body["model"] = model
	}
	var t AgentTask
	_, err := c.JSON(ctx, Request{Method: http.MethodPost, Path: fmt.Sprintf("agents/repos/%s/%s/tasks", owner, repo), Body: body}, &t)
	return &t, err
}
