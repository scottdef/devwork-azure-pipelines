//go:build ignore

// generate writes deterministic, illustrative Copilot usage-metrics fixtures
// in the layout read by copilot.FixtureSource:
//
//	reports/<report>/<YYYY-MM-DD>.ndjson   users-1-day, user-teams-1-day, repos-1-day, organization-1-day
//	prs.json  reviews.json  commits.json  seats.json  code_frequency/<repo>.json
//
// Usage (from the repository root):
//
//	go run examples/copilot-usage-report/fixtures/generate.go \
//	  -out examples/copilot-usage-report/fixtures -start 2026-08-17 -end 2026-09-20
//
// All logins, teams, repositories and numbers are fictitious. Record shapes
// follow the documented usage-metrics report fields the platform reads;
// unknown fields are ignored by the parser, so schema additions are safe.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type person struct {
	id        int64
	login     string
	team      string
	teamID    int64
	intensity float64 // 0 = seat never used, 1 = heavy agentic user
	agentic   bool
	repo      string
}

var people = []person{
	{101, "ada-platform", "platform-eng", 11, 0.95, true, "platform-infra"},
	{102, "ben-sre", "platform-eng", 11, 0.70, true, "platform-infra"},
	{103, "chen-infra", "platform-eng", 11, 0.55, false, "platform-infra"},
	{104, "dev-k8s", "platform-eng", 11, 0.80, true, "platform-infra"},
	{105, "eli-ops", "platform-eng", 11, 0.0, false, "platform-infra"},
	{106, "fay-net", "platform-eng", 11, 0.25, false, "shared-libs"},
	{201, "gia-pay", "payments", 12, 0.90, true, "payments-api"},
	{202, "hal-api", "payments", 12, 0.65, false, "payments-api"},
	{203, "ivy-ledger", "payments", 12, 0.60, true, "payments-api"},
	{204, "jon-risk", "payments", 12, 0.20, false, "payments-api"},
	{205, "kai-fraud", "payments", 12, 0.45, false, "shared-libs"},
	{301, "lea-ml", "data-science", 13, 0.85, true, "ml-pipelines"},
	{302, "max-data", "data-science", 13, 0.50, false, "ml-pipelines"},
	{303, "nia-stats", "data-science", 13, 0.35, false, "ml-pipelines"},
	{304, "omar-etl", "data-science", 13, 0.15, false, "ml-pipelines"},
	{401, "pat-ios", "mobile", 14, 0.60, false, "mobile-app"},
	{402, "quinn-android", "mobile", 14, 0.40, true, "mobile-app"},
	{403, "rae-mobile", "mobile", 14, 0.10, false, "mobile-app"},
}

var repos = []struct {
	id   int64
	name string
}{{9001, "platform-infra"}, {9002, "payments-api"}, {9003, "ml-pipelines"}, {9004, "mobile-app"}, {9005, "shared-libs"}}

var languages = map[string]string{"platform-infra": "go", "payments-api": "java", "ml-pipelines": "python", "mobile-app": "kotlin", "shared-libs": "typescript"}

type pr struct {
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	MergedAt  time.Time `json:"merged_at"`
	URL       string    `json:"url"`
}

func main() {
	out := flag.String("out", "examples/copilot-usage-report/fixtures", "output directory")
	startS := flag.String("start", "2026-08-17", "first day")
	endS := flag.String("end", "2026-09-20", "last day")
	flag.Parse()
	start, _ := time.Parse("2006-01-02", *startS)
	end, _ := time.Parse("2006-01-02", *endS)
	rng := rand.New(rand.NewSource(20260921))

	for _, r := range []string{"users-1-day", "user-teams-1-day", "repos-1-day", "organization-1-day"} {
		must(os.MkdirAll(filepath.Join(*out, "reports", r), 0o755))
	}
	must(os.MkdirAll(filepath.Join(*out, "code_frequency"), 0o755))

	var prs []pr
	prNum := map[string]int{"platform-infra": 1400, "payments-api": 2210, "ml-pipelines": 640, "mobile-app": 980, "shared-libs": 310}
	reviews := map[string]int{}
	commits := map[string]int{}

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		weekend := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
		// Adoption ramps up over the period (enablement sessions mid-August).
		ramp := 0.75 + 0.25*float64(d.Sub(start).Hours()/24)/math.Max(1, end.Sub(start).Hours()/24)
		var users, teams []map[string]any
		dau, codeGen, codeAcc, loc := 0, 0, 0, 0
		repoPR := map[string]map[string]any{}
		for _, r := range repos {
			repoPR[r.name] = map[string]any{"total_created": 0, "total_merged": 0, "total_reviewed": 0,
				"median_minutes_to_merge": 0.0, "median_minutes_to_merge_copilot_authored": 0.0,
				"total_created_by_copilot": 0, "total_merged_created_by_copilot": 0, "total_reviewed_by_copilot": 0,
				"total_merged_reviewed_by_copilot": 0, "total_copilot_suggestions": 0, "total_copilot_applied_suggestions": 0}
		}
		for _, p := range people {
			teams = append(teams, map[string]any{"user_id": p.id, "user_login": p.login, "day": day, "team_id": p.teamID, "slug": p.team})
			pActive := p.intensity * ramp
			if weekend {
				pActive *= 0.15
			}
			if p.intensity == 0 || rng.Float64() > pActive+0.05 {
				continue
			}
			gen := int(float64(20+rng.Intn(60)) * p.intensity * ramp)
			acc := int(float64(gen) * (0.22 + 0.2*p.intensity + rng.Float64()*0.08))
			if p.intensity < 0.3 {
				// Light users: chat only (active but not engaged).
				gen, acc = 0, 0
			}
			interactions := 2 + int(float64(rng.Intn(25))*p.intensity)
			agentDay := p.agentic && rng.Float64() < 0.55*ramp
			cliDay := p.agentic && rng.Float64() < 0.25
			cloudDay := p.agentic && rng.Float64() < 0.12
			credits := float64(interactions)*(4+rng.Float64()*3) + float64(gen)*1.5
			if agentDay {
				credits += 600 + rng.Float64()*1200
			}
			if cloudDay {
				credits += 1500 + rng.Float64()*2500
			}
			credits = math.Round(credits*100) / 100
			added := acc*(3+rng.Intn(6)) + map[bool]int{true: 120 + rng.Intn(260), false: 0}[agentDay]
			phase, num := "exploring", 1
			switch {
			case p.agentic && p.intensity >= 0.8:
				phase, num = "agentic", 3
			case p.intensity >= 0.5:
				phase, num = "integrating", 2
			}
			review := p.intensity > 0.5 && rng.Float64() < 0.3
			users = append(users, map[string]any{
				"day": day, "user_id": p.id, "user_login": p.login, "ai_credits_used": credits,
				"code_generation_activity_count": gen, "code_acceptance_activity_count": acc,
				"user_initiated_interaction_count": interactions, "loc_added_sum": added, "loc_deleted_sum": added / 4,
				"loc_suggested_to_add_sum": gen * 9, "used_agent": agentDay, "used_chat": true, "used_cli": cliDay,
				"used_copilot_app": false, "used_copilot_cloud_agent": cloudDay, "used_copilot_coding_agent": cloudDay,
				"used_copilot_code_review_active": review,
				"ai_adoption_phase": map[string]any{"phase": phase, "phase_number": num},
				"totals_by_language_feature": []map[string]any{{"language": languages[p.repo], "feature": "code_completion",
					"code_generation_activity_count": gen, "code_acceptance_activity_count": acc, "loc_added_sum": added}},
			})
			dau++
			codeGen += gen
			codeAcc += acc
			loc += added
			// Output: merged pull requests on weekdays, more for engaged users.
			if !weekend && rng.Float64() < 0.18+0.35*p.intensity*ramp {
				created := d.Add(time.Duration(8+rng.Intn(4)) * time.Hour).Add(-time.Duration(4+rng.Intn(60)) * time.Hour)
				merged := d.Add(time.Duration(9+rng.Intn(8)) * time.Hour)
				prNum[p.repo]++
				prs = append(prs, pr{Repo: "CoolEngOrg/" + p.repo, Number: prNum[p.repo], Author: p.login, CreatedAt: created.UTC(), MergedAt: merged.UTC(),
					URL: fmt.Sprintf("https://github.com/CoolEngOrg/%s/pull/%d", p.repo, prNum[p.repo])})
				rp := repoPR[p.repo]
				rp["total_created"] = rp["total_created"].(int) + 1
				rp["total_merged"] = rp["total_merged"].(int) + 1
				rp["total_reviewed"] = rp["total_reviewed"].(int) + 1
				rp["median_minutes_to_merge"] = math.Round(merged.Sub(created).Minutes())
				if review {
					rp["total_reviewed_by_copilot"] = rp["total_reviewed_by_copilot"].(int) + 1
					rp["total_merged_reviewed_by_copilot"] = rp["total_merged_reviewed_by_copilot"].(int) + 1
					sug := 1 + rng.Intn(5)
					rp["total_copilot_suggestions"] = rp["total_copilot_suggestions"].(int) + sug
					rp["total_copilot_applied_suggestions"] = rp["total_copilot_applied_suggestions"].(int) + rng.Intn(sug+1)
				}
				commits[p.repo] += 1 + rng.Intn(4)
			}
			if !weekend && rng.Float64() < 0.4*p.intensity {
				reviews[p.login]++
			}
			if cloudDay && !weekend {
				rp := repoPR[p.repo]
				rp["total_created_by_copilot"] = rp["total_created_by_copilot"].(int) + 1
				rp["total_created"] = rp["total_created"].(int) + 1
				if rng.Float64() < 0.7 {
					rp["total_merged_created_by_copilot"] = rp["total_merged_created_by_copilot"].(int) + 1
					rp["total_merged"] = rp["total_merged"].(int) + 1
					rp["median_minutes_to_merge_copilot_authored"] = float64(90 + rng.Intn(600))
				}
			}
		}
		writeNDJSON(filepath.Join(*out, "reports", "users-1-day", day+".ndjson"), users)
		writeNDJSON(filepath.Join(*out, "reports", "user-teams-1-day", day+".ndjson"), teams)
		var rows []map[string]any
		orgPR := map[string]any{}
		for _, r := range repos {
			rows = append(rows, map[string]any{"day": day, "repo_id": r.id, "repo_owner_name": "CoolEngOrg", "repo_name": r.name,
				"repo_visibility": "internal", "pull_requests": repoPR[r.name]})
			for k, v := range repoPR[r.name] {
				if n, ok := v.(int); ok {
					if cur, ok := orgPR[k].(int); ok {
						orgPR[k] = cur + n
					} else {
						orgPR[k] = n
					}
				}
			}
		}
		writeNDJSON(filepath.Join(*out, "reports", "repos-1-day", day+".ndjson"), rows)
		writeNDJSON(filepath.Join(*out, "reports", "organization-1-day", day+".ndjson"), []map[string]any{{
			"day_totals": []map[string]any{{"day": day, "daily_active_users": dau, "weekly_active_users": 0, "monthly_active_users": 0,
				"code_generation_activity_count": codeGen, "code_acceptance_activity_count": codeAcc, "loc_added_sum": loc, "pull_requests": orgPR}},
		}})
	}
	sort.Slice(prs, func(i, j int) bool { return prs[i].MergedAt.Before(prs[j].MergedAt) })
	writeJSON(filepath.Join(*out, "prs.json"), prs)
	writeJSON(filepath.Join(*out, "reviews.json"), reviews)
	writeJSON(filepath.Join(*out, "commits.json"), commits)
	writeJSON(filepath.Join(*out, "seats.json"), map[string]int{"total": len(people)})
	for _, r := range repos {
		var weeks [][3]int64
		for w := start.AddDate(0, 0, -int(start.Weekday())); !w.After(end); w = w.AddDate(0, 0, 7) {
			add := int64(400 + rng.Intn(3000))
			weeks = append(weeks, [3]int64{w.Unix(), add, -add / 3})
		}
		writeJSON(filepath.Join(*out, "code_frequency", r.name+".json"), weeks)
	}
	fmt.Printf("fixtures written to %s (%d merged PRs)\n", *out, len(prs))
}

func writeNDJSON(p string, rows []map[string]any) {
	f, err := os.Create(p)
	must(err)
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		must(enc.Encode(r))
	}
}

func writeJSON(p string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	must(err)
	must(os.WriteFile(p, append(b, '\n'), 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
