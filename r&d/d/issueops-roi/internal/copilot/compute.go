package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

// Options configures report generation.
type Options struct {
	Org         string
	Pricing     config.Pricing
	Settings    Settings
	Now         time.Time
	WithReviews bool
	Log         func(string)
}

// Totals are the headline numbers for a scope (organization, team, repo set).
type Totals struct {
	Seats           int      `json:"seats"`
	ActiveUsers     int      `json:"active_users"`
	EngagedUsers    int      `json:"engaged_users"`
	AgenticUsers    int      `json:"agentic_users"`
	ActiveDays      int      `json:"active_user_days"`
	EngagedDays     int      `json:"engaged_user_days"`
	Credits         float64  `json:"ai_credits"`
	AICostUSD       float64  `json:"ai_cost_usd"`
	SeatCostUSD     float64  `json:"seat_cost_usd"`
	TotalCostUSD    float64  `json:"total_cost_usd"`
	Interactions    int      `json:"interactions"`
	CodeGeneration  int      `json:"code_generation_activities"`
	CodeAcceptance  int      `json:"code_acceptance_activities"`
	LocAdded        int      `json:"ai_loc_added"`
	AcceptanceRate  *float64 `json:"acceptance_rate"`
	EngagedShare    *float64 `json:"engaged_share"`
	MergedPRs       *int     `json:"merged_prs"`
	MergedByCopilot *int     `json:"merged_copilot_authored"`
	CopilotShare    *float64 `json:"copilot_authored_share"`
	MedianCycleHrs  *float64 `json:"median_cycle_hours"`
	P90CycleHrs     *float64 `json:"p90_cycle_hours"`
	Reviews         *int     `json:"reviews"`
	CostPerMergedPR *float64 `json:"cost_per_merged_pr_usd"`
	AICostPerPR     *float64 `json:"ai_cost_per_merged_pr_usd"`
	LocPerDollar    *float64 `json:"ai_loc_added_per_usd"`
}

// TeamRow is a team-level result.
type TeamRow struct {
	Slug string `json:"slug"`
	Totals
}

// RepoRow is a repository-level result.
type RepoRow struct {
	Name                string   `json:"name"`
	CreatedPRs          int      `json:"created_prs"`
	MergedPRs           int      `json:"merged_prs"`
	MergedCopilot       int      `json:"merged_copilot_authored"`
	CreatedCopilot      int      `json:"created_copilot_authored"`
	ReviewedByCopilot   int      `json:"reviewed_by_copilot"`
	CopilotSuggestions  int      `json:"copilot_review_suggestions"`
	CopilotApplied      int      `json:"copilot_review_suggestions_applied"`
	MedianMinutesMerge  *float64 `json:"median_minutes_to_merge"`
	CopilotShare        *float64 `json:"copilot_authored_share"`
	Commits             *int     `json:"commits,omitempty"`
	Additions           *int     `json:"additions,omitempty"`
	Deletions           *int     `json:"deletions,omitempty"`
	Authors             int      `json:"authors,omitempty"`
	AttributedAICostUSD *float64 `json:"attributed_ai_cost_usd,omitempty"`
	CostPerMergedPR     *float64 `json:"attributed_ai_cost_per_merged_pr_usd,omitempty"`
}

// UserRow is a per-user result (only produced when approved per-user detail was requested).
type UserRow struct {
	Login          string   `json:"login"`
	Teams          []string `json:"teams"`
	ActiveDays     int      `json:"active_days"`
	EngagedDays    int      `json:"engaged_days"`
	AgenticDays    int      `json:"agentic_days"`
	Credits        float64  `json:"ai_credits"`
	AICostUSD      float64  `json:"ai_cost_usd"`
	LocAdded       int      `json:"ai_loc_added"`
	AcceptanceRate *float64 `json:"acceptance_rate"`
	MergedPRs      *int     `json:"merged_prs"`
	AdoptionPhase  string   `json:"adoption_phase"`
}

// DailyPoint is one day of the scope.
type DailyPoint struct {
	Day       string  `json:"day"`
	Active    int     `json:"active_users"`
	Engaged   int     `json:"engaged_users"`
	Credits   float64 `json:"ai_credits"`
	AICostUSD float64 `json:"ai_cost_usd"`
}

// WeekPoint is one week (Monday start) of output and cost.
type WeekPoint struct {
	Week      string  `json:"week"`
	MergedPRs int     `json:"merged_prs"`
	AICostUSD float64 `json:"ai_cost_usd"`
}

// Report is the full result.
type Report struct {
	Org         string            `json:"organization"`
	Spec        Spec              `json:"spec"`
	GeneratedAt time.Time         `json:"generated_at"`
	Pricing     config.Pricing    `json:"pricing"`
	DaysFound   int               `json:"days_with_data"`
	DaysMissing []string          `json:"days_missing,omitempty"`
	Totals      Totals            `json:"totals"`
	Daily       []DailyPoint      `json:"daily"`
	Weekly      []WeekPoint       `json:"weekly"`
	Teams       []TeamRow         `json:"teams"`
	Repos       []RepoRow         `json:"repositories"`
	Users       []UserRow         `json:"users,omitempty"`
	Findings    []string          `json:"findings"`
	Notes       []string          `json:"notes"`
	Definitions map[string]string `json:"definitions"`
}

type agg struct {
	seats, active, engaged, agentic map[int64]bool
	activeDays, engagedDays         int
	agenticDays                     int
	credits                         float64
	interactions, gen, acc, loc     int
	logins                          map[string]bool
}

func newAgg() *agg {
	return &agg{seats: map[int64]bool{}, active: map[int64]bool{}, engaged: map[int64]bool{}, agentic: map[int64]bool{}, logins: map[string]bool{}}
}

func (a *agg) add(u *UserDay) {
	a.logins[strings.ToLower(u.UserLogin)] = true
	a.credits += u.AICreditsUsed
	a.interactions += u.Interactions
	a.gen += u.CodeGeneration
	a.acc += u.CodeAcceptance
	a.loc += u.LocAdded
	if u.Active() {
		a.active[u.UserID] = true
		a.activeDays++
	}
	if u.Engaged() {
		a.engaged[u.UserID] = true
		a.engagedDays++
	}
	if u.Agentic() {
		a.agentic[u.UserID] = true
		a.agenticDays++
	}
}

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }

func ratio(n, d float64) *float64 {
	if d == 0 {
		return nil
	}
	return ptrF(n / d)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func (a *agg) totals(seats int, days int, p config.Pricing, includeSeats bool) Totals {
	t := Totals{
		Seats:          seats,
		ActiveUsers:    len(a.active),
		EngagedUsers:   len(a.engaged),
		AgenticUsers:   len(a.agentic),
		ActiveDays:     a.activeDays,
		EngagedDays:    a.engagedDays,
		Credits:        round2(a.credits),
		AICostUSD:      round2(a.credits * p.AICreditUSD),
		Interactions:   a.interactions,
		CodeGeneration: a.gen,
		CodeAcceptance: a.acc,
		LocAdded:       a.loc,
		AcceptanceRate: ratio(float64(a.acc), float64(a.gen)),
		EngagedShare:   ratio(float64(len(a.engaged)), float64(len(a.active))),
	}
	if includeSeats {
		t.SeatCostUSD = round2(float64(seats) * p.SeatMonthlyUSD * float64(days) / 30.0)
	}
	t.TotalCostUSD = round2(t.AICostUSD + t.SeatCostUSD)
	return t
}

func (t *Totals) applyOutput(prs []PR) {
	t.MergedPRs = ptrI(len(prs))
	if len(prs) > 0 {
		hrs := make([]float64, len(prs))
		for i, p := range prs {
			hrs[i] = p.CycleHours()
		}
		t.MedianCycleHrs = ptrF(round2(percentile(hrs, 50)))
		t.P90CycleHrs = ptrF(round2(percentile(hrs, 90)))
	}
	t.derive()
}

func (t *Totals) derive() {
	if t.MergedPRs != nil && *t.MergedPRs > 0 {
		t.CostPerMergedPR = ptrF(round2(t.TotalCostUSD / float64(*t.MergedPRs)))
		t.AICostPerPR = ptrF(round2(t.AICostUSD / float64(*t.MergedPRs)))
		if t.MergedByCopilot != nil {
			t.CopilotShare = ratio(float64(*t.MergedByCopilot), float64(*t.MergedPRs))
		}
	}
	if t.TotalCostUSD > 0 {
		t.LocPerDollar = ptrF(round2(float64(t.LocAdded) / t.TotalCostUSD))
	}
}

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64{}, v...)
	sort.Float64s(s)
	if len(s) == 1 {
		return s[0]
	}
	rank := p / 100 * float64(len(s)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	return s[lo] + (s[hi]-s[lo])*(rank-float64(lo))
}

func weekStart(t time.Time) time.Time {
	d := day(t)
	wd := int(d.Weekday())
	if wd == 0 {
		wd = 7
	}
	return d.AddDate(0, 0, -(wd - 1))
}

// Build collects data from src and computes the report.
func Build(ctx context.Context, src Source, sp *Spec, opt Options) (*Report, error) {
	logf := func(format string, a ...any) {
		if opt.Log != nil {
			opt.Log(fmt.Sprintf(format, a...))
		}
	}
	rep := &Report{Org: opt.Org, Spec: *sp, GeneratedAt: opt.Now.UTC(), Pricing: opt.Pricing, Definitions: definitions(opt)}
	wantTeams := map[string]bool{}
	for _, t := range sp.Teams {
		wantTeams[strings.ToLower(t)] = true
	}
	wantRepos := map[string]bool{}
	for _, r := range sp.Repos {
		wantRepos[strings.ToLower(r)] = true
	}

	type dayData struct {
		users   []UserDay
		teams   map[int64][]string // user -> team slugs
		repos   []RepoDay
		orgDays []OrgDay
	}
	var days []time.Time
	for d := sp.Start; !d.After(sp.End); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	data := map[string]*dayData{}
	needTeams := sp.Scope != "repositories" || sp.UserDetail
	needRepos := sp.Scope != "teams"
	for _, d := range days {
		key := d.Format("2006-01-02")
		dd := &dayData{teams: map[int64][]string{}}
		found, err := src.Report(ctx, ghapi.ReportUsers1Day, d, func(raw json.RawMessage) error {
			var u UserDay
			if err := json.Unmarshal(raw, &u); err != nil {
				return err
			}
			if u.Day == "" {
				u.Day = key
			}
			dd.users = append(dd.users, u)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("copilot: users report %s: %w", key, err)
		}
		if !found {
			rep.DaysMissing = append(rep.DaysMissing, key)
			continue
		}
		rep.DaysFound++
		if needTeams {
			if _, err := src.Report(ctx, ghapi.ReportUserTeams1Day, d, func(raw json.RawMessage) error {
				var ut UserTeam
				if err := json.Unmarshal(raw, &ut); err != nil {
					return err
				}
				dd.teams[ut.UserID] = append(dd.teams[ut.UserID], strings.ToLower(ut.Slug))
				return nil
			}); err != nil {
				return nil, fmt.Errorf("copilot: user-teams report %s: %w", key, err)
			}
		}
		if needRepos {
			if _, err := src.Report(ctx, ghapi.ReportRepos1Day, d, func(raw json.RawMessage) error {
				var r RepoDay
				if err := json.Unmarshal(raw, &r); err != nil {
					return err
				}
				dd.repos = append(dd.repos, r)
				return nil
			}); err != nil {
				return nil, fmt.Errorf("copilot: repository report %s: %w", key, err)
			}
		}
		if sp.Scope == "organization" {
			if _, err := src.Report(ctx, ghapi.ReportOrg1Day, d, func(raw json.RawMessage) error {
				od, err := ParseOrgRecord(raw)
				dd.orgDays = append(dd.orgDays, od...)
				return err
			}); err != nil {
				return nil, fmt.Errorf("copilot: organization report %s: %w", key, err)
			}
		}
		data[key] = dd
	}
	if rep.DaysFound == 0 {
		rep.Notes = append(rep.Notes, "No Copilot usage-metrics data was available for the requested period. Check that the Copilot usage metrics policy is enabled and that the GitHub App has Organization Copilot metrics: read.")
		return rep, nil
	}
	if len(rep.DaysMissing) > 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf("%d day(s) had no usage-metrics data and were skipped.", len(rep.DaysMissing)))
	}

	// ---- aggregate usage ------------------------------------------------
	scope := newAgg()
	teamAgg := map[string]*agg{}
	teamMembers := map[string]map[string]bool{} // slug -> logins
	userAgg := map[string]*agg{}                // login -> agg
	userTeams := map[string]map[string]bool{}
	userPhase := map[string]string{}
	loginByID := map[int64]string{}
	creditsByLogin := map[string]float64{}
	var repoDays []RepoDay
	var orgDays []OrgDay
	var daily []DailyPoint
	costByWeek := map[string]float64{}

	for _, d := range days {
		key := d.Format("2006-01-02")
		dd := data[key]
		if dd == nil {
			continue
		}
		repoDays = append(repoDays, dd.repos...)
		orgDays = append(orgDays, dd.orgDays...)
		dp := DailyPoint{Day: key}
		for i := range dd.users {
			u := &dd.users[i]
			login := strings.ToLower(u.UserLogin)
			loginByID[u.UserID] = u.UserLogin
			teams := dd.teams[u.UserID]
			inScope := sp.Scope != "teams"
			for _, t := range teams {
				if sp.Scope == "teams" && !wantTeams[t] {
					continue
				}
				if sp.Scope == "teams" {
					inScope = true
				}
				if teamAgg[t] == nil {
					teamAgg[t], teamMembers[t] = newAgg(), map[string]bool{}
				}
				teamAgg[t].add(u)
				teamAgg[t].seats[u.UserID] = true
				teamMembers[t][login] = true
				if userTeams[login] == nil {
					userTeams[login] = map[string]bool{}
				}
				userTeams[login][t] = true
			}
			creditsByLogin[login] += u.AICreditsUsed
			if userAgg[login] == nil {
				userAgg[login] = newAgg()
			}
			userAgg[login].add(u)
			if u.Phase.Phase != "" {
				userPhase[login] = u.Phase.Phase
			}
			if !inScope {
				continue
			}
			scope.add(u)
			scope.seats[u.UserID] = true
			if u.Active() {
				dp.Active++
			}
			if u.Engaged() {
				dp.Engaged++
			}
			dp.Credits += u.AICreditsUsed
		}
		// Seat-holding team members without activity still count as seats.
		for uid, teams := range dd.teams {
			for _, t := range teams {
				if teamAgg[t] != nil {
					teamAgg[t].seats[uid] = true
					if sp.Scope == "teams" && wantTeams[t] {
						scope.seats[uid] = true
					}
				}
			}
		}
		dp.Credits = round2(dp.Credits)
		dp.AICostUSD = round2(dp.Credits * opt.Pricing.AICreditUSD)
		costByWeek[weekStart(d).Format("2006-01-02")] += dp.AICostUSD
		daily = append(daily, dp)
	}
	rep.Daily = daily
	ndays := rep.DaysFound

	// ---- outputs --------------------------------------------------------
	var prs []PR
	switch sp.Scope {
	case "teams":
		authors := map[string]bool{}
		for _, t := range sp.Teams {
			for l := range teamMembers[t] {
				authors[l] = true
			}
		}
		list := sortedKeys(authors)
		if len(list) > opt.Settings.MaxSearchUsers {
			rep.Notes = append(rep.Notes, fmt.Sprintf("Output search capped at %d of %d team members.", opt.Settings.MaxSearchUsers, len(list)))
			list = list[:opt.Settings.MaxSearchUsers]
		}
		logf("searching merged pull requests for %d team members", len(list))
		var err error
		prs, err = src.MergedPRs(ctx, PRQuery{Org: opt.Org, Authors: list, Start: sp.Start, End: sp.End})
		if err != nil {
			return nil, fmt.Errorf("copilot: search merged pull requests: %w", err)
		}
	case "repositories":
		logf("searching merged pull requests in %d repositories", len(sp.Repos))
		var err error
		prs, err = src.MergedPRs(ctx, PRQuery{Org: opt.Org, Repos: sp.Repos, Start: sp.Start, End: sp.End})
		if err != nil {
			return nil, fmt.Errorf("copilot: search merged pull requests: %w", err)
		}
		// In repository scope the "scope" users are the PR authors.
		authors := map[string]bool{}
		for _, p := range prs {
			authors[strings.ToLower(p.Author)] = true
		}
		scope = newAgg()
		daily = nil
		for _, d := range days {
			key := d.Format("2006-01-02")
			dd := data[key]
			if dd == nil {
				continue
			}
			dp := DailyPoint{Day: key}
			for i := range dd.users {
				u := &dd.users[i]
				if !authors[strings.ToLower(u.UserLogin)] {
					continue
				}
				scope.add(u)
				scope.seats[u.UserID] = true
				if u.Active() {
					dp.Active++
				}
				if u.Engaged() {
					dp.Engaged++
				}
				dp.Credits += u.AICreditsUsed
			}
			dp.Credits = round2(dp.Credits)
			dp.AICostUSD = round2(dp.Credits * opt.Pricing.AICreditUSD)
			daily = append(daily, dp)
		}
		rep.Daily = daily
		costByWeek = map[string]float64{}
		for _, dp := range daily {
			t, _ := time.Parse("2006-01-02", dp.Day)
			costByWeek[weekStart(t).Format("2006-01-02")] += dp.AICostUSD
		}
	}

	// ---- scope totals ---------------------------------------------------
	seats := len(scope.seats)
	if sp.Scope == "organization" {
		if n, err := src.SeatCount(ctx, opt.Org); err == nil && n > 0 {
			seats = n
		} else {
			rep.Notes = append(rep.Notes, "Seat count taken from users with usage rows (seat API unavailable); seat cost may be understated.")
		}
	}
	rep.Totals = scope.totals(seats, ndays, opt.Pricing, sp.IncludeSeats)
	if sp.Scope == "organization" {
		merged, byCopilot, weighted, wsum := 0, 0, 0.0, 0
		for _, od := range orgDays {
			pr := od.PullRequests
			merged += pr.TotalMerged
			byCopilot += pr.TotalMergedCreatedByCopilot
			if pr.TotalMerged > 0 && pr.MedianMinutesToMerge > 0 {
				weighted += pr.MedianMinutesToMerge * float64(pr.TotalMerged)
				wsum += pr.TotalMerged
			}
		}
		if len(orgDays) > 0 {
			rep.Totals.MergedPRs, rep.Totals.MergedByCopilot = ptrI(merged), ptrI(byCopilot)
			if wsum > 0 {
				rep.Totals.MedianCycleHrs = ptrF(round2(weighted / float64(wsum) / 60))
			}
			rep.Totals.derive()
		} else {
			rep.Notes = append(rep.Notes, "Organization pull-request totals were not present in the organization report.")
		}
	} else {
		rep.Totals.applyOutput(prs)
	}

	// ---- teams ----------------------------------------------------------
	teamSlugs := sortedKeys(func() map[string]bool {
		m := map[string]bool{}
		for t := range teamAgg {
			m[t] = true
		}
		return m
	}())
	if sp.Scope == "teams" {
		for _, t := range sp.Teams {
			if teamAgg[t] == nil {
				rep.Notes = append(rep.Notes, fmt.Sprintf("Team `%s` has no rows in the user-teams report (fewer than 5 Copilot-seated members, or no seats).", t))
			}
		}
	}
	for _, t := range teamSlugs {
		a := teamAgg[t]
		row := TeamRow{Slug: t, Totals: a.totals(len(a.seats), ndays, opt.Pricing, sp.IncludeSeats)}
		if sp.Scope == "teams" {
			var tp []PR
			for _, p := range prs {
				if teamMembers[t][strings.ToLower(p.Author)] {
					tp = append(tp, p)
				}
			}
			row.applyOutput(tp)
			if opt.WithReviews {
				total := 0
				for _, l := range sortedKeys(teamMembers[t]) {
					n, err := src.ReviewedCount(ctx, opt.Org, loginFor(l, loginByID), sp.Start, sp.End)
					if err != nil {
						logf("review count for %s: %v", l, err)
						continue
					}
					total += n
				}
				row.Reviews = ptrI(total)
			}
		}
		rep.Teams = append(rep.Teams, row)
	}
	sort.SliceStable(rep.Teams, func(i, j int) bool { return rep.Teams[i].TotalCostUSD > rep.Teams[j].TotalCostUSD })

	// ---- repositories ---------------------------------------------------
	repoAgg := map[string]*RepoRow{}
	repoWeighted := map[string][2]float64{}
	for _, rd := range repoDays {
		name := rd.RepoName
		if sp.Scope == "repositories" && !wantRepos[strings.ToLower(name)] {
			continue
		}
		r := repoAgg[name]
		if r == nil {
			r = &RepoRow{Name: name}
			repoAgg[name] = r
		}
		pr := rd.PullRequests
		r.CreatedPRs += pr.TotalCreated
		r.MergedPRs += pr.TotalMerged
		r.MergedCopilot += pr.TotalMergedCreatedByCopilot
		r.CreatedCopilot += pr.TotalCreatedByCopilot
		r.ReviewedByCopilot += pr.TotalReviewedByCopilot
		r.CopilotSuggestions += pr.TotalCopilotSuggestions
		r.CopilotApplied += pr.TotalCopilotApplied
		if pr.TotalMerged > 0 && pr.MedianMinutesToMerge > 0 {
			w := repoWeighted[name]
			repoWeighted[name] = [2]float64{w[0] + pr.MedianMinutesToMerge*float64(pr.TotalMerged), w[1] + float64(pr.TotalMerged)}
		}
	}
	if sp.Scope == "repositories" {
		for _, r := range sp.Repos {
			if repoAgg[r] == nil {
				repoAgg[r] = &RepoRow{Name: r}
			}
		}
		// Attribute AI cost to repositories by each author's share of merged PRs.
		perAuthor := map[string]int{}
		perAuthorRepo := map[string]map[string]int{}
		for _, p := range prs {
			a := strings.ToLower(p.Author)
			repo := p.Repo
			if i := strings.LastIndex(repo, "/"); i >= 0 {
				repo = repo[i+1:]
			}
			perAuthor[a]++
			if perAuthorRepo[repo] == nil {
				perAuthorRepo[repo] = map[string]int{}
			}
			perAuthorRepo[repo][a]++
		}
		for name, r := range repoAgg {
			cost := 0.0
			for a, n := range perAuthorRepo[name] {
				cost += creditsByLogin[a] * opt.Pricing.AICreditUSD * float64(n) / float64(perAuthor[a])
			}
			r.Authors = len(perAuthorRepo[name])
			r.AttributedAICostUSD = ptrF(round2(cost))
			if r.MergedPRs > 0 {
				r.CostPerMergedPR = ptrF(round2(cost / float64(r.MergedPRs)))
			}
			if n, err := src.CommitCount(ctx, opt.Org, name, sp.Start, sp.End); err == nil {
				r.Commits = ptrI(n)
			} else {
				logf("commit count %s: %v", name, err)
			}
			if weeks, err := src.CodeFrequency(ctx, opt.Org, name); err == nil && weeks != nil {
				add, del := 0, 0
				for _, w := range weeks {
					if !w.Week.Before(weekStart(sp.Start)) && !w.Week.After(sp.End) {
						add += w.Additions
						del += w.Deletions
					}
				}
				r.Additions, r.Deletions = ptrI(add), ptrI(del)
			}
		}
		rep.Notes = append(rep.Notes, "Repository AI cost is attributed from each author's AI-credit spend in proportion to their merged pull requests across the requested repositories (an approximation: usage is not recorded per repository).")
		// Repository scope totals use repository-report PR counts when present.
		merged, byCop := 0, 0
		for _, r := range repoAgg {
			merged += r.MergedPRs
			byCop += r.MergedCopilot
		}
		if merged > 0 {
			rep.Totals.MergedPRs, rep.Totals.MergedByCopilot = ptrI(merged), ptrI(byCop)
			rep.Totals.derive()
		}
	}
	for name, r := range repoAgg {
		if w := repoWeighted[name]; w[1] > 0 {
			r.MedianMinutesMerge = ptrF(round2(w[0] / w[1]))
		}
		r.CopilotShare = ratio(float64(r.MergedCopilot), float64(r.MergedPRs))
		rep.Repos = append(rep.Repos, *r)
	}
	sort.SliceStable(rep.Repos, func(i, j int) bool {
		if rep.Repos[i].MergedPRs == rep.Repos[j].MergedPRs {
			return rep.Repos[i].Name < rep.Repos[j].Name
		}
		return rep.Repos[i].MergedPRs > rep.Repos[j].MergedPRs
	})
	if sp.Scope == "organization" && len(rep.Repos) > 25 {
		rep.Notes = append(rep.Notes, fmt.Sprintf("Showing the 25 repositories with the most merged pull requests of %d with activity.", len(rep.Repos)))
		rep.Repos = rep.Repos[:25]
	}

	// ---- weekly ---------------------------------------------------------
	mergedByWeek := map[string]int{}
	for _, p := range prs {
		mergedByWeek[weekStart(p.MergedAt).Format("2006-01-02")]++
	}
	weeks := map[string]bool{}
	for k := range costByWeek {
		weeks[k] = true
	}
	for k := range mergedByWeek {
		weeks[k] = true
	}
	for _, w := range sortedKeys(weeks) {
		rep.Weekly = append(rep.Weekly, WeekPoint{Week: w, MergedPRs: mergedByWeek[w], AICostUSD: round2(costByWeek[w])})
	}

	// ---- users (approved detail only) -----------------------------------
	if sp.UserDetail {
		prByAuthor := map[string]int{}
		for _, p := range prs {
			prByAuthor[strings.ToLower(p.Author)]++
		}
		for login, a := range userAgg {
			if sp.Scope == "teams" && len(userTeams[login]) == 0 {
				continue
			}
			if sp.Scope == "repositories" && !scope.logins[login] {
				continue
			}
			ur := UserRow{Login: loginFor(login, loginByID), Teams: sortedKeys(userTeams[login]), ActiveDays: a.activeDays, EngagedDays: a.engagedDays,
				AgenticDays: a.agenticDays, Credits: round2(a.credits), AICostUSD: round2(a.credits * opt.Pricing.AICreditUSD), LocAdded: a.loc,
				AcceptanceRate: ratio(float64(a.acc), float64(a.gen)), AdoptionPhase: userPhase[login]}
			if sp.Scope != "organization" {
				ur.MergedPRs = ptrI(prByAuthor[login])
			}
			rep.Users = append(rep.Users, ur)
		}
		sort.Slice(rep.Users, func(i, j int) bool { return rep.Users[i].AICostUSD > rep.Users[j].AICostUSD })
	}

	rep.Findings = findings(rep, opt)
	rep.Notes = append(rep.Notes,
		"Teams with fewer than five Copilot-seated users are omitted from GitHub's user-teams report; users on several teams count toward each of their teams, so team rows do not sum to the scope total.",
		"AI-assisted lines of code (loc_added_sum) are directional: they measure Copilot-suggested lines accepted or applied, not total output.")
	return rep, nil
}

func loginFor(lower string, byID map[int64]string) string {
	for _, l := range byID {
		if strings.EqualFold(l, lower) {
			return l
		}
	}
	return lower
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func definitions(opt Options) map[string]string {
	return map[string]string{
		"active user":        "a Copilot-seated user with any interaction, generation, acceptance or agent use on a day",
		"engaged user":       opt.Settings.EngagedDefinition,
		"AI cost":            fmt.Sprintf("ai_credits_used x $%.4f per credit (config pricing.ai_credit_usd)", opt.Pricing.AICreditUSD),
		"seat cost":          fmt.Sprintf("seats x $%.2f per month x days/30 (%s)", opt.Pricing.SeatMonthlyUSD, opt.Pricing.SeatPlan),
		"cost per merged PR": "total cost in scope / merged pull requests in scope",
		"cycle time":         "pull request created -> merged, in hours",
	}
}

func pct(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

func findings(r *Report, opt Options) []string {
	var out []string
	t := r.Totals
	if t.ActiveUsers > 0 && t.EngagedShare != nil {
		out = append(out, fmt.Sprintf("%d of %d active Copilot users (%s) were engaged: they %s.", t.EngagedUsers, t.ActiveUsers, pct(*t.EngagedShare), opt.Settings.EngagedDefinition))
	}
	if t.AgenticUsers > 0 && t.ActiveUsers > 0 {
		out = append(out, fmt.Sprintf("%s of active users used an agentic surface (agent mode, CLI, Copilot app or cloud agent) at least once.", pct(float64(t.AgenticUsers)/float64(t.ActiveUsers))))
	}
	if t.CopilotShare != nil && t.MergedPRs != nil && *t.MergedPRs > 0 {
		out = append(out, fmt.Sprintf("Copilot-authored pull requests were %s of %d merged pull requests.", pct(*t.CopilotShare), *t.MergedPRs))
	}
	if t.CostPerMergedPR != nil {
		out = append(out, fmt.Sprintf("Total cost per merged pull request: $%.2f (AI credits alone: $%.2f).", *t.CostPerMergedPR, *t.AICostPerPR))
	}
	var ranked []TeamRow
	for _, tr := range r.Teams {
		if tr.CostPerMergedPR != nil && tr.MergedPRs != nil && *tr.MergedPRs >= 5 {
			ranked = append(ranked, tr)
		}
	}
	if len(ranked) >= 2 {
		sort.Slice(ranked, func(i, j int) bool { return *ranked[i].CostPerMergedPR < *ranked[j].CostPerMergedPR })
		lo, hi := ranked[0], ranked[len(ranked)-1]
		out = append(out, fmt.Sprintf("Lowest cost per merged PR: %s ($%.2f); highest: %s ($%.2f).", lo.Slug, *lo.CostPerMergedPR, hi.Slug, *hi.CostPerMergedPR))
	}
	for _, tr := range r.Teams {
		if tr.EngagedShare != nil && tr.ActiveUsers >= 5 && *tr.EngagedShare < 0.4 {
			out = append(out, fmt.Sprintf("Team %s: only %s of active users are engaged; enablement may help before raising its budget.", tr.Slug, pct(*tr.EngagedShare)))
		}
	}
	if n := len(r.Daily); n >= 14 {
		first, second := 0.0, 0.0
		for i, d := range r.Daily {
			if i < n/2 {
				first += d.AICostUSD
			} else {
				second += d.AICostUSD
			}
		}
		if first > 0 {
			out = append(out, fmt.Sprintf("AI-credit spend in the second half of the period changed %+.0f%% versus the first half.", (second-first)/first*100))
		}
	}
	if len(out) == 0 {
		out = append(out, "Not enough activity in the period to draw conclusions.")
	}
	return out
}
