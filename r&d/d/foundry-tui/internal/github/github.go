// Package github reads GitHub Actions over REST with net/http.
//
// Reads are plain Go. Writes are not here at all: they are gh commands
// built by package dispatch. The token comes from the environment or
// from "gh auth token", so there is one login to manage, gh's.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

// Client reads one repository.
type Client struct {
	Base  string // https://api.github.com
	Repo  string // OWNER/NAME
	Token string
	HTTP  *http.Client
}

// New finds a token: GH_TOKEN, GITHUB_TOKEN, then gh's keyring.
func New(ctx context.Context, r run.Runner, base, repo string) (*Client, error) {
	tok := os.Getenv("GH_TOKEN")
	if tok == "" {
		tok = os.Getenv("GITHUB_TOKEN")
	}
	if tok == "" {
		out, err := r.Run(ctx, run.Cmd{Name: "gh", Args: []string{"auth", "token"}})
		if err != nil {
			return nil, fmt.Errorf("no GitHub token: set GH_TOKEN or run 'gh auth login': %w", err)
		}
		tok = strings.TrimSpace(string(out))
	}
	return &Client{
		Base:  strings.TrimRight(base, "/"),
		Repo:  repo,
		Token: tok,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/repos/"+c.Repo+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "foundry-tui")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, e.Message)
	}
	return json.Unmarshal(body, v)
}

// Workflow is one workflow file registered with Actions.
type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// Agentic reports whether path is a gh-aw compiled workflow. gh aw
// compiles NAME.md into NAME.lock.yml; the suffix is the contract.
func Agentic(path string) bool { return strings.HasSuffix(path, ".lock.yml") }

// File is the workflow's file name, the form gh and the REST API accept.
func (w Workflow) File() string { return w.Path[strings.LastIndexByte(w.Path, '/')+1:] }

// Workflows lists up to 100 workflows.
func (c *Client) Workflows(ctx context.Context) ([]Workflow, error) {
	var page struct {
		Workflows []Workflow `json:"workflows"`
	}
	err := c.get(ctx, "/actions/workflows?per_page=100", &page)
	return page.Workflows, err
}

// Run is one workflow run.
type Run struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	DisplayTitle string    `json:"display_title"`
	Path         string    `json:"path"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	Branch       string    `json:"head_branch"`
	URL          string    `json:"html_url"`
	Number       int       `json:"run_number"`
	Attempt      int       `json:"run_attempt"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    time.Time `json:"run_started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Actor        struct {
		Login string `json:"login"`
	} `json:"triggering_actor"`
}

// State folds status and conclusion into one word.
func (r Run) State() string {
	if r.Status != "completed" {
		return r.Status
	}
	return r.Conclusion
}

// Duration is wall time for finished runs, elapsed time otherwise.
func (r Run) Duration() time.Duration {
	start := r.StartedAt
	if start.IsZero() {
		start = r.CreatedAt
	}
	end := r.UpdatedAt
	if r.Status != "completed" {
		end = time.Now()
	}
	if d := end.Sub(start); d > 0 {
		return d.Round(time.Second)
	}
	return 0
}

// Runs returns the n most recent runs (n ≤ 100). A workflow file name
// narrows the list; "" means the whole repository.
func (c *Client) Runs(ctx context.Context, workflowFile string, n int) ([]Run, error) {
	if n <= 0 || n > 100 {
		n = 100
	}
	path := fmt.Sprintf("/actions/runs?per_page=%d", n)
	if workflowFile != "" {
		path = fmt.Sprintf("/actions/workflows/%s/runs?per_page=%d", workflowFile, n)
	}
	var page struct {
		Runs []Run `json:"workflow_runs"`
	}
	err := c.get(ctx, path, &page)
	return page.Runs, err
}

// FindRun looks for the run a dispatch created. Workflows in this
// project put the request id in run-name, so a substring is enough.
func FindRun(runs []Run, requestID string) (Run, bool) {
	for _, r := range runs {
		if requestID != "" && strings.Contains(r.DisplayTitle, requestID) {
			return r, true
		}
	}
	return Run{}, false
}

// Stats summarises the recent history of one workflow.
type Stats struct {
	Workflow    string
	Path        string
	Agentic     bool
	State       string // workflow state: active, disabled_manually, ...
	Runs        int
	Succeeded   int
	Failed      int
	Active      int
	SuccessRate float64 // percent of finished runs
	AvgDuration time.Duration
	Last        Run
}

// Summarize groups runs by workflow path. Workflows with no recent
// runs still get a row, so a silent agent is visible too.
func Summarize(wfs []Workflow, runs []Run) []Stats {
	by := map[string]*Stats{}
	for _, w := range wfs {
		by[w.Path] = &Stats{Workflow: w.Name, Path: w.Path, Agentic: Agentic(w.Path), State: w.State}
	}
	var total = map[string]time.Duration{}
	for _, r := range runs {
		s := by[r.Path]
		if s == nil {
			s = &Stats{Workflow: r.Name, Path: r.Path, Agentic: Agentic(r.Path)}
			by[r.Path] = s
		}
		s.Runs++
		if s.Last.ID == 0 || r.CreatedAt.After(s.Last.CreatedAt) {
			s.Last = r
		}
		switch {
		case r.Status != "completed":
			s.Active++
		case r.Conclusion == "success":
			s.Succeeded++
			total[r.Path] += r.Duration()
		case r.Conclusion == "failure" || r.Conclusion == "timed_out" || r.Conclusion == "startup_failure":
			s.Failed++
			total[r.Path] += r.Duration()
		}
	}
	out := make([]Stats, 0, len(by))
	for p, s := range by {
		if done := s.Succeeded + s.Failed; done > 0 {
			s.SuccessRate = 100 * float64(s.Succeeded) / float64(done)
			s.AvgDuration = (total[p] / time.Duration(done)).Round(time.Second)
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
