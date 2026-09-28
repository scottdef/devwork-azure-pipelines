package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

// PRQuery selects merged pull requests by authors and/or repositories.
type PRQuery struct {
	Org     string
	Authors []string
	Repos   []string
	Start   time.Time
	End     time.Time // inclusive day
}

// Source abstracts where report data comes from (GitHub API or fixtures).
type Source interface {
	Report(ctx context.Context, kind ghapi.CopilotReport, day time.Time, fn func(json.RawMessage) error) (bool, error)
	MergedPRs(ctx context.Context, q PRQuery) ([]PR, error)
	ReviewedCount(ctx context.Context, org, user string, start, end time.Time) (int, error)
	CodeFrequency(ctx context.Context, owner, repo string) ([]ghapi.WeeklyCodeFrequency, error)
	CommitCount(ctx context.Context, owner, repo string, start, end time.Time) (int, error)
	SeatCount(ctx context.Context, org string) (int, error)
}

// APISource reads live data through the GitHub REST API.
type APISource struct {
	GH  *ghapi.Client
	Org string
	// SearchPace spaces search API calls (30 requests/minute limit).
	SearchPace time.Duration
	Log        func(string)
}

func (a *APISource) log(s string) {
	if a.Log != nil {
		a.Log(s)
	}
}

// Report implements Source.
func (a *APISource) Report(ctx context.Context, kind ghapi.CopilotReport, day time.Time, fn func(json.RawMessage) error) (bool, error) {
	links, err := a.GH.CopilotReportLinks(ctx, a.Org, kind, day)
	if err != nil {
		if ghapi.IsStatus(err, 404) {
			return false, nil
		}
		return false, err
	}
	if links == nil || len(links.DownloadLinks) == 0 {
		return false, nil
	}
	return true, a.GH.DownloadNDJSON(ctx, links.DownloadLinks, fn)
}

func dateRange(start, end time.Time) string {
	return start.Format("2006-01-02") + ".." + end.Format("2006-01-02")
}

func toPR(is ghapi.Issue) PR {
	repo := is.RepositoryURL
	if i := strings.Index(repo, "/repos/"); i >= 0 {
		repo = repo[i+len("/repos/"):]
	}
	p := PR{Repo: repo, Number: is.Number, Author: is.User.Login, CreatedAt: is.CreatedAt, MergedAt: is.ClosedAt, URL: is.HTMLURL}
	if is.PullRequest != nil && is.PullRequest.MergedAt != nil {
		p.MergedAt = *is.PullRequest.MergedAt
	}
	return p
}

// MergedPRs implements Source with one search per author or repository.
func (a *APISource) MergedPRs(ctx context.Context, q PRQuery) ([]PR, error) {
	seen := map[string]bool{}
	var out []PR
	run := func(query string) error {
		items, total, err := a.GH.SearchAll(ctx, query, a.SearchPace)
		if err != nil {
			return err
		}
		if total > len(items) {
			a.log(fmt.Sprintf("search %q returned %d of %d results (search API cap)", query, len(items), total))
		}
		for _, it := range items {
			p := toPR(it)
			k := fmt.Sprintf("%s#%d", p.Repo, p.Number)
			if !seen[k] {
				seen[k] = true
				out = append(out, p)
			}
		}
		if a.SearchPace > 0 && a.GH.Sleep != nil {
			a.GH.Sleep(a.SearchPace)
		}
		return nil
	}
	rng := dateRange(q.Start, q.End)
	for _, author := range q.Authors {
		if err := run(fmt.Sprintf("is:pr is:merged org:%s author:%s merged:%s", q.Org, author, rng)); err != nil {
			return out, err
		}
	}
	for _, repo := range q.Repos {
		if err := run(fmt.Sprintf("is:pr is:merged repo:%s/%s merged:%s", q.Org, repo, rng)); err != nil {
			return out, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MergedAt.Before(out[j].MergedAt) })
	return out, nil
}

// ReviewedCount implements Source.
func (a *APISource) ReviewedCount(ctx context.Context, org, user string, start, end time.Time) (int, error) {
	res, err := a.GH.SearchIssues(ctx, fmt.Sprintf("is:pr org:%s reviewed-by:%s -author:%s updated:%s", org, user, user, dateRange(start, end)), 1, 1)
	if err != nil {
		return 0, err
	}
	if a.SearchPace > 0 && a.GH.Sleep != nil {
		a.GH.Sleep(a.SearchPace)
	}
	return res.TotalCount, nil
}

// CodeFrequency implements Source.
func (a *APISource) CodeFrequency(ctx context.Context, owner, repo string) ([]ghapi.WeeklyCodeFrequency, error) {
	return a.GH.CodeFrequency(ctx, owner, repo)
}

// CommitCount implements Source.
func (a *APISource) CommitCount(ctx context.Context, owner, repo string, start, end time.Time) (int, error) {
	return a.GH.CommitCount(ctx, owner, repo, start, end.AddDate(0, 0, 1))
}

// SeatCount implements Source.
func (a *APISource) SeatCount(ctx context.Context, org string) (int, error) {
	return a.GH.CopilotSeatCount(ctx, org)
}

// FixtureSource reads a directory of recorded data (used for examples, tests
// and offline dry runs):
//
//	reports/<report>/<YYYY-MM-DD>.ndjson
//	prs.json                     []PR
//	reviews.json                 {"login": count}
//	commits.json                 {"repo": count}
//	code_frequency/<repo>.json   [[unix_week, additions, deletions], ...]
//	seats.json                   {"total": N}
type FixtureSource struct{ Dir string }

// Report implements Source.
func (f *FixtureSource) Report(_ context.Context, kind ghapi.CopilotReport, day time.Time, fn func(json.RawMessage) error) (bool, error) {
	name := strings.TrimSuffix(string(kind), "/latest")
	p := filepath.Join(f.Dir, "reports", name, day.Format("2006-01-02")+".ndjson")
	fh, err := os.Open(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer fh.Close()
	return true, ghapi.ScanNDJSON(fh, fn)
}

func (f *FixtureSource) readJSON(name string, v any) error {
	b, err := os.ReadFile(filepath.Join(f.Dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// MergedPRs implements Source by filtering prs.json.
func (f *FixtureSource) MergedPRs(_ context.Context, q PRQuery) ([]PR, error) {
	var all []PR
	if err := f.readJSON("prs.json", &all); err != nil {
		return nil, err
	}
	authors := map[string]bool{}
	for _, a := range q.Authors {
		authors[strings.ToLower(a)] = true
	}
	repos := map[string]bool{}
	for _, r := range q.Repos {
		repos[strings.ToLower(q.Org+"/"+r)] = true
	}
	end := q.End.AddDate(0, 0, 1)
	var out []PR
	for _, p := range all {
		if p.MergedAt.Before(q.Start) || !p.MergedAt.Before(end) {
			continue
		}
		if authors[strings.ToLower(p.Author)] || repos[strings.ToLower(p.Repo)] {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MergedAt.Before(out[j].MergedAt) })
	return out, nil
}

// ReviewedCount implements Source.
func (f *FixtureSource) ReviewedCount(_ context.Context, _, user string, _, _ time.Time) (int, error) {
	m := map[string]int{}
	err := f.readJSON("reviews.json", &m)
	return m[user], err
}

// CodeFrequency implements Source.
func (f *FixtureSource) CodeFrequency(_ context.Context, _, repo string) ([]ghapi.WeeklyCodeFrequency, error) {
	var raw [][3]int64
	if err := f.readJSON(filepath.Join("code_frequency", repo+".json"), &raw); err != nil {
		return nil, err
	}
	out := make([]ghapi.WeeklyCodeFrequency, 0, len(raw))
	for _, r := range raw {
		d := int(r[2])
		if d < 0 {
			d = -d
		}
		out = append(out, ghapi.WeeklyCodeFrequency{Week: time.Unix(r[0], 0).UTC(), Additions: int(r[1]), Deletions: d})
	}
	return out, nil
}

// CommitCount implements Source.
func (f *FixtureSource) CommitCount(_ context.Context, _, repo string, _, _ time.Time) (int, error) {
	m := map[string]int{}
	err := f.readJSON("commits.json", &m)
	return m[repo], err
}

// SeatCount implements Source.
func (f *FixtureSource) SeatCount(_ context.Context, _ string) (int, error) {
	var s struct {
		Total int `json:"total"`
	}
	err := f.readJSON("seats.json", &s)
	return s.Total, err
}
