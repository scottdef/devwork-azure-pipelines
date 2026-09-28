package ghapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// User is a GitHub account reference.
type User struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Type  string `json:"type"`
}

// Label on an issue.
type Label struct {
	Name string `json:"name"`
}

// Issue is the subset of an issue the platform uses.
type Issue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	User        User      `json:"user"`
	Labels      []Label   `json:"labels"`
	HTMLURL     string    `json:"html_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    time.Time `json:"closed_at"`
	PullRequest *struct {
		MergedAt *time.Time `json:"merged_at"`
		HTMLURL  string     `json:"html_url"`
	} `json:"pull_request,omitempty"`
	RepositoryURL string `json:"repository_url"`
}

// LabelNames returns label names.
func (i *Issue) LabelNames() []string {
	out := make([]string, len(i.Labels))
	for k, l := range i.Labels {
		out[k] = l.Name
	}
	return out
}

// Comment is an issue comment.
type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HTMLURL   string    `json:"html_url"`
}

// GetIssue fetches an issue.
func (c *Client) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	var is Issue
	_, err := c.JSON(ctx, Request{Path: fmt.Sprintf("repos/%s/%s/issues/%d", owner, repo, number)}, &is)
	return &is, err
}

// ListComments returns every comment on an issue, oldest first.
func (c *Client) ListComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	var out []Comment
	err := c.PaginateArray(ctx, Request{Path: fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repo, number)}, func(items []json.RawMessage) error {
		for _, raw := range items {
			var cm Comment
			if err := json.Unmarshal(raw, &cm); err != nil {
				return err
			}
			out = append(out, cm)
		}
		return nil
	})
	return out, err
}

// CreateComment posts a comment and returns it.
func (c *Client) CreateComment(ctx context.Context, owner, repo string, number int, body string) (*Comment, error) {
	var cm Comment
	_, err := c.JSON(ctx, Request{Method: http.MethodPost, Path: fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repo, number), Body: map[string]string{"body": body}}, &cm)
	return &cm, err
}

// CloseIssue closes an issue with a reason ("completed" or "not_planned").
func (c *Client) CloseIssue(ctx context.Context, owner, repo string, number int, reason string) error {
	_, err := c.Do(ctx, Request{Method: http.MethodPatch, Path: fmt.Sprintf("repos/%s/%s/issues/%d", owner, repo, number), Body: map[string]string{"state": "closed", "state_reason": reason}})
	return err
}

// ListIssues lists issues (not PRs) carrying a label, in any state, updated since.
func (c *Client) ListIssues(ctx context.Context, owner, repo, label string, since time.Time) ([]Issue, error) {
	q := url.Values{"state": {"all"}, "labels": {label}}
	if !since.IsZero() {
		q.Set("since", since.UTC().Format(time.RFC3339))
	}
	var out []Issue
	err := c.PaginateArray(ctx, Request{Path: fmt.Sprintf("repos/%s/%s/issues", owner, repo), Query: q}, func(items []json.RawMessage) error {
		for _, raw := range items {
			var is Issue
			if err := json.Unmarshal(raw, &is); err != nil {
				return err
			}
			if is.PullRequest == nil {
				out = append(out, is)
			}
		}
		return nil
	})
	return out, err
}

// AgentAssignment customizes a Copilot cloud agent task started by assigning
// an issue to copilot-swe-agent[bot] (public preview API).
type AgentAssignment struct {
	TargetRepo         string `json:"target_repo,omitempty"`
	BaseBranch         string `json:"base_branch,omitempty"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
	CustomAgent        string `json:"custom_agent,omitempty"`
	Model              string `json:"model,omitempty"`
}

// NewIssue is the create-issue payload.
type NewIssue struct {
	Title           string           `json:"title"`
	Body            string           `json:"body"`
	Labels          []string         `json:"labels,omitempty"`
	Assignees       []string         `json:"assignees,omitempty"`
	AgentAssignment *AgentAssignment `json:"agent_assignment,omitempty"`
}

// CreateIssue creates an issue (optionally assigned to the Copilot cloud agent).
// Assigning Copilot requires a user-to-server token (PAT or GitHub App user
// token); installation tokens are rejected by GitHub for this operation.
func (c *Client) CreateIssue(ctx context.Context, owner, repo string, in NewIssue) (*Issue, error) {
	var is Issue
	_, err := c.JSON(ctx, Request{Method: http.MethodPost, Path: fmt.Sprintf("repos/%s/%s/issues", owner, repo), Body: in}, &is)
	return &is, err
}
