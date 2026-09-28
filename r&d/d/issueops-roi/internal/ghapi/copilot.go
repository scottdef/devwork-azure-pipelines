package ghapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ReportLinks is the response of the Copilot usage-metrics report endpoints.
type ReportLinks struct {
	DownloadLinks  []string `json:"download_links"`
	ReportDay      string   `json:"report_day,omitempty"`
	ReportStartDay string   `json:"report_start_day,omitempty"`
	ReportEndDay   string   `json:"report_end_day,omitempty"`
}

// CopilotReport names an organization report endpoint.
type CopilotReport string

const (
	ReportOrg1Day       CopilotReport = "organization-1-day"
	ReportOrg28Day      CopilotReport = "organization-28-day/latest"
	ReportUsers1Day     CopilotReport = "users-1-day"
	ReportUsers28Day    CopilotReport = "users-28-day/latest"
	ReportUserTeams1Day CopilotReport = "user-teams-1-day"
	ReportRepos1Day     CopilotReport = "repos-1-day"
)

// CopilotReportLinks fetches signed download links for an organization
// report. day is required for *-1-day reports. A 204 (no data for the day)
// returns (nil, nil).
func (c *Client) CopilotReportLinks(ctx context.Context, org string, report CopilotReport, day time.Time) (*ReportLinks, error) {
	q := url.Values{}
	if strings.HasSuffix(string(report), "-1-day") {
		q.Set("day", day.Format("2006-01-02"))
	}
	var out ReportLinks
	resp, err := c.JSON(ctx, Request{Path: fmt.Sprintf("orgs/%s/copilot/metrics/reports/%s", org, report), Query: q, Version: VersionLatest, OK: []int{http.StatusNoContent}}, &out)
	if err != nil {
		return nil, err
	}
	if resp.Status == http.StatusNoContent {
		return nil, nil
	}
	return &out, nil
}

// DownloadNDJSON fetches every signed link and calls fn for each JSON line.
// The signed URLs are pre-authorized storage URLs: the GitHub token is NOT
// sent to them.
func (c *Client) DownloadNDJSON(ctx context.Context, links []string, fn func(json.RawMessage) error) error {
	for _, link := range links {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("ghapi: download report: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ghapi: download report: HTTP %d (signed links expire quickly; re-request them)", resp.StatusCode)
		}
		if err := ScanNDJSON(bytes.NewReader(body), fn); err != nil {
			return err
		}
	}
	return nil
}

// ScanNDJSON splits NDJSON (or a single JSON array) into records.
func ScanNDJSON(r io.Reader, fn func(json.RawMessage) error) error {
	br := bufio.NewReader(r)
	peek, _ := br.Peek(1)
	if len(peek) == 1 && peek[0] == '[' {
		var arr []json.RawMessage
		if err := json.NewDecoder(br).Decode(&arr); err != nil {
			return fmt.Errorf("ghapi: decode JSON array report: %w", err)
		}
		for _, raw := range arr {
			if err := fn(raw); err != nil {
				return err
			}
		}
		return nil
	}
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	line := 0
	for sc.Scan() {
		line++
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		if !json.Valid(cp) {
			return fmt.Errorf("ghapi: invalid NDJSON at line %d", line)
		}
		if err := fn(cp); err != nil {
			return err
		}
	}
	return sc.Err()
}

// SearchResult is a page of issue/PR search results.
type SearchResult struct {
	TotalCount        int     `json:"total_count"`
	IncompleteResults bool    `json:"incomplete_results"`
	Items             []Issue `json:"items"`
}

// SearchIssues runs an issue/PR search. The search API allows 30 requests per
// minute for authenticated callers; callers should pace themselves.
func (c *Client) SearchIssues(ctx context.Context, query string, perPage, page int) (*SearchResult, error) {
	q := url.Values{"q": {query}, "per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}}
	var out SearchResult
	_, err := c.JSON(ctx, Request{Path: "search/issues", Query: q}, &out)
	return &out, err
}

// SearchAll pages through up to 1,000 results (the search API ceiling).
func (c *Client) SearchAll(ctx context.Context, query string, pace time.Duration) ([]Issue, int, error) {
	var all []Issue
	total := 0
	for page := 1; page <= 10; page++ {
		res, err := c.SearchIssues(ctx, query, 100, page)
		if err != nil {
			return all, total, err
		}
		total = res.TotalCount
		all = append(all, res.Items...)
		if len(res.Items) < 100 || len(all) >= total {
			break
		}
		if pace > 0 && c.Sleep != nil {
			c.Sleep(pace)
		}
	}
	return all, total, nil
}

// CopilotSeatCount returns the organization's assigned Copilot seats
// (GET /orgs/{org}/copilot/billing, seat_breakdown.total). Requires the
// "Organization Copilot seat management: read" permission.
func (c *Client) CopilotSeatCount(ctx context.Context, org string) (int, error) {
	var out struct {
		SeatBreakdown struct {
			Total int `json:"total"`
		} `json:"seat_breakdown"`
	}
	_, err := c.JSON(ctx, Request{Path: fmt.Sprintf("orgs/%s/copilot/billing", org)}, &out)
	return out.SeatBreakdown.Total, err
}
