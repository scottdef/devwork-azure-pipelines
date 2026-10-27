package github

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// Actor is a GitHub user.
type Actor struct {
	Login string `json:"login"`
}

// Run is the subset of a workflow run that ghlog2dt uses.
type Run struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	DisplayTitle string     `json:"display_title,omitempty"`
	Path         string     `json:"path,omitempty"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	Event        string     `json:"event"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	WorkflowID   int64      `json:"workflow_id"`
	RunNumber    int        `json:"run_number"`
	RunAttempt   int        `json:"run_attempt"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RunStartedAt *time.Time `json:"run_started_at,omitempty"`
	Actor        Actor      `json:"actor"`
	HTMLURL      string     `json:"html_url"`
}

// Step is one step of a job.
type Step struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	Number      int        `json:"number"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// Job is one job (of one attempt) of a run.
type Job struct {
	ID           int64      `json:"id"`
	RunID        int64      `json:"run_id"`
	RunAttempt   int        `json:"run_attempt"`
	Name         string     `json:"name"`
	WorkflowName string     `json:"workflow_name,omitempty"`
	HeadBranch   string     `json:"head_branch,omitempty"`
	HeadSHA      string     `json:"head_sha"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	HTMLURL      string     `json:"html_url"`
	RunnerName   string     `json:"runner_name"`
	Labels       []string   `json:"labels,omitempty"`
	Steps        []Step     `json:"steps"`
}

// MaxFilteredResults is GitHub's cap on filtered list-runs queries.
const MaxFilteredResults = 1000

const createdLayout = "2006-01-02T15:04:05Z"

// ListRuns returns every run created in [from, to], oldest first. Any
// window that reports >= 1000 results is halved recursively, because
// GitHub silently stops at 1000 when "created" is used.
func (c *Client) ListRuns(ctx context.Context, repo string, from, to time.Time) ([]Run, error) {
	from, to = from.UTC().Truncate(time.Second), to.UTC().Truncate(time.Second)
	seen := map[int64]bool{}
	var out []Run
	if err := c.listRuns(ctx, repo, from, to, seen, &out); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

type runsPage struct {
	TotalCount   int   `json:"total_count"`
	WorkflowRuns []Run `json:"workflow_runs"`
}

func (c *Client) listRuns(ctx context.Context, repo string, from, to time.Time, seen map[int64]bool, out *[]Run) error {
	q := url.Values{
		"per_page":              {"100"},
		"exclude_pull_requests": {"true"},
		"created":               {from.Format(createdLayout) + ".." + to.Format(createdLayout)},
	}
	var page runsPage
	next, err := c.getJSON(ctx, request{path: "/repos/" + repo + "/actions/runs", query: q}, &page)
	if err != nil {
		return err
	}
	if page.TotalCount >= MaxFilteredResults && to.Sub(from) > 2*time.Second {
		mid := from.Add(to.Sub(from) / 2).Truncate(time.Second)
		c.Log.Info("splitting run window", "repo", repo, "total", page.TotalCount, "from", from, "mid", mid, "to", to)
		if err := c.listRuns(ctx, repo, from, mid, seen, out); err != nil {
			return err
		}
		return c.listRuns(ctx, repo, mid.Add(time.Second), to, seen, out)
	}
	if page.TotalCount >= MaxFilteredResults {
		c.Log.Warn("more than 1000 runs in a 2s window; some will be missed", "repo", repo, "from", from)
	}
	for {
		for _, r := range page.WorkflowRuns {
			if !seen[r.ID] {
				seen[r.ID] = true
				*out = append(*out, r)
			}
		}
		if next == "" {
			return nil
		}
		page = runsPage{}
		if next, err = c.getJSON(ctx, request{path: next}, &page); err != nil {
			return err
		}
	}
}

// GetRun fetches one run.
func (c *Client) GetRun(ctx context.Context, repo string, id int64) (Run, error) {
	var r Run
	_, err := c.getJSON(ctx, request{path: "/repos/" + repo + "/actions/runs/" + strconv.FormatInt(id, 10)}, &r)
	return r, err
}

// ListJobs returns the jobs of every attempt of a run (filter=all),
// ordered by attempt then job ID.
func (c *Client) ListJobs(ctx context.Context, repo string, runID int64) ([]Job, error) {
	type jobsPage struct {
		TotalCount int   `json:"total_count"`
		Jobs       []Job `json:"jobs"`
	}
	r := request{
		path:  "/repos/" + repo + "/actions/runs/" + strconv.FormatInt(runID, 10) + "/jobs",
		query: url.Values{"filter": {"all"}, "per_page": {"100"}},
	}
	var out []Job
	for {
		var page jobsPage
		next, err := c.getJSON(ctx, r, &page)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Jobs...)
		if next == "" {
			break
		}
		r = request{path: next}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RunAttempt != out[j].RunAttempt {
			return out[i].RunAttempt < out[j].RunAttempt
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// JobLog opens a job's plain-text log. GitHub answers with a 302 to a
// signed URL valid for about a minute, so read it promptly. A fresh 404
// is retried: logs can trail job completion by a few seconds.
func (c *Client) JobLog(ctx context.Context, repo string, jobID int64) (io.ReadCloser, error) {
	resp, err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/repos/" + repo + "/actions/jobs/" + strconv.FormatInt(jobID, 10) + "/logs",
		retry404: 3,
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// InstallationRepos lists the repositories an installation token can
// see, skipping archived ones.
func (c *Client) InstallationRepos(ctx context.Context) ([]string, error) {
	type page struct {
		Repositories []struct {
			FullName string `json:"full_name"`
			Archived bool   `json:"archived"`
			Disabled bool   `json:"disabled"`
		} `json:"repositories"`
	}
	r := request{path: "/installation/repositories", query: url.Values{"per_page": {"100"}}}
	var out []string
	for {
		var p page
		next, err := c.getJSON(ctx, r, &p)
		if err != nil {
			return nil, err
		}
		for _, x := range p.Repositories {
			if !x.Archived && !x.Disabled {
				out = append(out, x.FullName)
			}
		}
		if next == "" {
			break
		}
		r = request{path: next}
	}
	sort.Strings(out)
	return out, nil
}
