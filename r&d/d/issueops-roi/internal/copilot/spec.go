// Package copilot implements the copilot-usage-report type: it ingests the
// Copilot usage-metrics reports (daily per-user, user-teams, repository and
// organization NDJSON reports), joins them with pull-request output data and
// AI-credit cost, and renders an ROI report (Markdown summary, interactive
// go-echarts HTML, CSV, JSON and Prometheus text for Grafana).
package copilot

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/request"
)

// Settings is the registry settings block.
type Settings struct {
	MaxDays           int    `json:"max_days"`
	MaxTeams          int    `json:"max_teams"`
	MaxRepositories   int    `json:"max_repositories"`
	DataLagDays       int    `json:"data_lag_days"`
	EarliestReportDay string `json:"earliest_report_day"`
	MaxSearchUsers    int    `json:"max_search_users"`
	EngagedDefinition string `json:"engaged_definition"`
	AssetsHost        string `json:"assets_host"`
}

func (s *Settings) defaults() {
	if s.MaxDays == 0 {
		s.MaxDays = 90
	}
	if s.MaxTeams == 0 {
		s.MaxTeams = 20
	}
	if s.MaxRepositories == 0 {
		s.MaxRepositories = 25
	}
	if s.DataLagDays == 0 {
		s.DataLagDays = 2
	}
	if s.EarliestReportDay == "" {
		s.EarliestReportDay = "2025-10-10"
	}
	if s.MaxSearchUsers == 0 {
		s.MaxSearchUsers = 300
	}
	if s.AssetsHost == "" {
		s.AssetsHost = "https://go-echarts.github.io/go-echarts-assets/assets/"
	}
}

// Form option labels.
const (
	OptHTML       = "Interactive HTML report"
	OptCSV        = "CSV exports"
	OptGrafana    = "Publish metrics to Grafana"
	OptUserDetail = "Include a per-user breakdown (requires Copilot admin approval)"
	CostSeats     = "AI credits and seats"
)

// Spec is a validated report request.
type Spec struct {
	Scope        string    `json:"scope"`
	Teams        []string  `json:"teams,omitempty"`
	Repos        []string  `json:"repositories,omitempty"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Days         int       `json:"days"`
	IncludeSeats bool      `json:"include_seats"`
	HTML         bool      `json:"html"`
	CSV          bool      `json:"csv"`
	Grafana      bool      `json:"grafana"`
	UserDetail   bool      `json:"user_detail"`
	Purpose      string    `json:"purpose"`
	Period       string    `json:"period"`
}

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,99}$`)
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	periodRe = regexp.MustCompile(`^Last (\d+) days$`)
)

func day(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }

// BuildSpec validates the parsed request.
func BuildSpec(c *request.Context) (*Spec, *Settings, []string, []string) {
	var s Settings
	var errs, warns []string
	if err := c.Type.DecodeSettings(&s); err != nil {
		return nil, nil, []string{err.Error()}, nil
	}
	s.defaults()
	v := c.Values
	sp := &Spec{
		Scope:        v.String("report_scope"),
		IncludeSeats: v.String("report_cost_basis") == CostSeats,
		HTML:         v.IsSelected("report_outputs", OptHTML),
		CSV:          v.IsSelected("report_outputs", OptCSV),
		Grafana:      v.IsSelected("report_outputs", OptGrafana),
		UserDetail:   v.IsSelected("report_user_detail", OptUserDetail),
		Purpose:      v.String("report_purpose"),
		Period:       v.String("report_period"),
	}
	org := strings.ToLower(c.Registry.Organization)
	for _, t := range issueform.SplitCSV(v.String("report_teams")) {
		t = strings.ToLower(strings.TrimPrefix(t, "@"))
		t = strings.TrimPrefix(t, org+"/")
		if !slugRe.MatchString(t) {
			errs = append(errs, fmt.Sprintf("Team `%s` is not a valid team slug.", t))
			continue
		}
		sp.Teams = append(sp.Teams, t)
	}
	for _, r := range issueform.SplitCSV(v.String("report_repositories")) {
		if o, n, ok := strings.Cut(r, "/"); ok {
			if !strings.EqualFold(o, c.Registry.Organization) {
				errs = append(errs, fmt.Sprintf("Repository `%s` is outside %s.", r, c.Registry.Organization))
				continue
			}
			r = n
		}
		if !repoRe.MatchString(r) {
			errs = append(errs, fmt.Sprintf("Repository `%s` is not a valid name.", r))
			continue
		}
		sp.Repos = append(sp.Repos, r)
	}
	switch sp.Scope {
	case "organization":
	case "teams":
		if len(sp.Teams) == 0 {
			errs = append(errs, "The teams scope needs at least one team slug.")
		}
		if len(sp.Teams) > s.MaxTeams {
			errs = append(errs, fmt.Sprintf("At most %d teams per report.", s.MaxTeams))
		}
	case "repositories":
		if len(sp.Repos) == 0 {
			errs = append(errs, "The repositories scope needs at least one repository.")
		}
		if len(sp.Repos) > s.MaxRepositories {
			errs = append(errs, fmt.Sprintf("At most %d repositories per report.", s.MaxRepositories))
		}
	default:
		errs = append(errs, fmt.Sprintf("Unknown report scope %q.", sp.Scope))
	}

	latest := day(c.Now).AddDate(0, 0, -s.DataLagDays)
	earliest, _ := time.Parse("2006-01-02", s.EarliestReportDay)
	if oneYear := day(c.Now).AddDate(-1, 0, 0); oneYear.After(earliest) {
		earliest = oneYear
	}
	if m := periodRe.FindStringSubmatch(sp.Period); m != nil {
		n, _ := strconv.Atoi(m[1])
		sp.End = latest
		sp.Start = latest.AddDate(0, 0, -(n - 1))
	} else if sp.Period == "Custom range" {
		st, err1 := time.Parse("2006-01-02", v.String("report_start_date"))
		en, err2 := time.Parse("2006-01-02", v.String("report_end_date"))
		if err1 != nil || err2 != nil {
			errs = append(errs, "A custom range needs start and end dates in YYYY-MM-DD format.")
		} else {
			sp.Start, sp.End = st, en
			if en.After(latest) {
				warns = append(warns, fmt.Sprintf("End date moved to %s: usage metrics are usually available with a %d-day delay.", latest.Format("2006-01-02"), s.DataLagDays))
				sp.End = latest
			}
			if sp.End.Before(sp.Start) {
				errs = append(errs, "The custom end date is before the start date.")
			}
		}
	} else {
		errs = append(errs, fmt.Sprintf("Unknown reporting period %q.", sp.Period))
	}
	if !sp.Start.IsZero() {
		if sp.Start.Before(earliest) {
			warns = append(warns, fmt.Sprintf("Start date moved to %s: usage-metrics reports are only available from then (and for one year).", earliest.Format("2006-01-02")))
			sp.Start = earliest
		}
		sp.Days = int(sp.End.Sub(sp.Start).Hours()/24) + 1
		if sp.Days > s.MaxDays {
			errs = append(errs, fmt.Sprintf("The period is %d days; the maximum is %d.", sp.Days, s.MaxDays))
		}
		if sp.Days < 1 {
			errs = append(errs, "The period contains no days with available data.")
		}
	}
	if sp.Purpose == "" {
		errs = append(errs, "Describe the purpose of the report.")
	}
	return sp, &s, errs, warns
}

// Handler implements request.Handler.
type Handler struct{}

// Facts implements request.Handler.
func (Handler) Facts(c *request.Context) (map[string]any, error) {
	sp, _, _, _ := BuildSpec(c)
	f := map[string]any{"$type": c.Type.ID}
	if sp != nil {
		f["$days"] = float64(sp.Days)
		f["$user_detail"] = sp.UserDetail
	}
	return f, nil
}

// Validate implements request.Handler.
func (Handler) Validate(ctx context.Context, c *request.Context) ([]string, []string) {
	sp, _, errs, warns := BuildSpec(c)
	if sp == nil || c.GH == nil {
		return errs, warns
	}
	for _, t := range sp.Teams {
		ok, err := c.GH.TeamExists(ctx, c.Registry.Organization, t)
		if err != nil {
			warns = append(warns, "Could not verify team "+t+": "+err.Error())
		} else if !ok {
			errs = append(errs, fmt.Sprintf("Team `%s/%s` does not exist.", c.Registry.Organization, t))
		}
	}
	for _, r := range sp.Repos {
		repo, err := c.GH.GetRepo(ctx, c.Registry.Organization, r)
		if err != nil {
			warns = append(warns, "Could not verify repository "+r+": "+err.Error())
		} else if repo == nil {
			errs = append(errs, fmt.Sprintf("Repository `%s/%s` does not exist.", c.Registry.Organization, r))
		}
	}
	return errs, warns
}

// Summary implements request.Handler.
func (Handler) Summary(c *request.Context) []request.Row {
	sp, _, _, _ := BuildSpec(c)
	if sp == nil {
		return nil
	}
	target := c.Registry.Organization
	switch sp.Scope {
	case "teams":
		target = strings.Join(sp.Teams, ", ")
	case "repositories":
		target = strings.Join(sp.Repos, ", ")
	}
	var outs []string
	outs = append(outs, "summary comment")
	if sp.HTML {
		outs = append(outs, "HTML report")
	}
	if sp.CSV {
		outs = append(outs, "CSV")
	}
	if sp.Grafana {
		outs = append(outs, "Grafana metrics")
	}
	period := sp.Period
	if !sp.Start.IsZero() {
		period = fmt.Sprintf("%s to %s (%d days)", sp.Start.Format("2006-01-02"), sp.End.Format("2006-01-02"), sp.Days)
	}
	cost := "AI credits only"
	if sp.IncludeSeats {
		cost = "AI credits and prorated seats"
	}
	detail := "no (aggregates only)"
	if sp.UserDetail {
		detail = "yes (needs Copilot admin approval)"
	}
	return []request.Row{
		{Label: "Scope", Value: sp.Scope + ": " + target},
		{Label: "Period", Value: period},
		{Label: "Cost basis", Value: cost},
		{Label: "Outputs", Value: strings.Join(outs, ", ")},
		{Label: "Per-user detail", Value: detail},
	}
}

// Subject implements request.Handler.
func (Handler) Subject(*request.Context) (string, map[string]string) { return "", nil }
