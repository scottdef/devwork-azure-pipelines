package copilot

import (
	"encoding/json"
	"time"
)

// UserDay is one row of the users-1-day report (fields from the documented
// example schema; unknown fields are ignored so schema additions are safe).
type UserDay struct {
	Day             string  `json:"day"`
	UserID          int64   `json:"user_id"`
	UserLogin       string  `json:"user_login"`
	AICreditsUsed   float64 `json:"ai_credits_used"`
	CodeAcceptance  int     `json:"code_acceptance_activity_count"`
	CodeGeneration  int     `json:"code_generation_activity_count"`
	Interactions    int     `json:"user_initiated_interaction_count"`
	LocAdded        int     `json:"loc_added_sum"`
	LocDeleted      int     `json:"loc_deleted_sum"`
	LocSuggestedAdd int     `json:"loc_suggested_to_add_sum"`
	UsedAgent       bool    `json:"used_agent"`
	UsedChat        bool    `json:"used_chat"`
	UsedCLI         bool    `json:"used_cli"`
	UsedCopilotApp  bool    `json:"used_copilot_app"`
	UsedCloudAgent  bool    `json:"used_copilot_cloud_agent"`
	UsedCodingAgent bool    `json:"used_copilot_coding_agent"`
	UsedReviewAct   *bool   `json:"used_copilot_code_review_active"`
	Phase           struct {
		Phase       string `json:"phase"`
		PhaseNumber int    `json:"phase_number"`
	} `json:"ai_adoption_phase"`
	TotalsByLanguageFeature []struct {
		Language       string `json:"language"`
		Feature        string `json:"feature"`
		CodeGeneration int    `json:"code_generation_activity_count"`
		CodeAcceptance int    `json:"code_acceptance_activity_count"`
		LocAdded       int    `json:"loc_added_sum"`
	} `json:"totals_by_language_feature"`
}

// Active reports whether the user used Copilot that day.
func (u *UserDay) Active() bool {
	return u.Interactions > 0 || u.CodeGeneration > 0 || u.CodeAcceptance > 0 ||
		u.UsedAgent || u.UsedChat || u.UsedCLI || u.UsedCopilotApp || u.UsedCloudAgent || u.UsedCodingAgent ||
		(u.UsedReviewAct != nil && *u.UsedReviewAct)
}

// Engaged: accepted code or used an agentic surface (agent mode, CLI,
// Copilot app, or cloud agent) that day. Chat-only or passive use is active
// but not engaged.
func (u *UserDay) Engaged() bool {
	return u.CodeAcceptance > 0 || u.UsedAgent || u.UsedCLI || u.UsedCopilotApp || u.UsedCloudAgent || u.UsedCodingAgent
}

// Agentic reports use of an agentic surface.
func (u *UserDay) Agentic() bool {
	return u.UsedAgent || u.UsedCLI || u.UsedCopilotApp || u.UsedCloudAgent || u.UsedCodingAgent
}

// UserTeam is one row of the user-teams-1-day report.
type UserTeam struct {
	UserID    int64  `json:"user_id"`
	UserLogin string `json:"user_login"`
	Day       string `json:"day"`
	TeamID    int64  `json:"team_id"`
	Slug      string `json:"slug"`
}

// PullRequests is the pull_requests block in organization and repository reports.
type PullRequests struct {
	TotalCreated                 int     `json:"total_created"`
	TotalMerged                  int     `json:"total_merged"`
	TotalReviewed                int     `json:"total_reviewed"`
	MedianMinutesToMerge         float64 `json:"median_minutes_to_merge"`
	MedianMinutesCopilotAuthored float64 `json:"median_minutes_to_merge_copilot_authored"`
	TotalCreatedByCopilot        int     `json:"total_created_by_copilot"`
	TotalMergedCreatedByCopilot  int     `json:"total_merged_created_by_copilot"`
	TotalReviewedByCopilot       int     `json:"total_reviewed_by_copilot"`
	TotalMergedReviewedByCopilot int     `json:"total_merged_reviewed_by_copilot"`
	TotalCopilotSuggestions      int     `json:"total_copilot_suggestions"`
	TotalCopilotApplied          int     `json:"total_copilot_applied_suggestions"`
}

// RepoDay is one row of the repos-1-day report.
type RepoDay struct {
	Day          string       `json:"day"`
	RepoID       int64        `json:"repo_id"`
	RepoOwner    string       `json:"repo_owner_name"`
	RepoName     string       `json:"repo_name"`
	Visibility   string       `json:"repo_visibility"`
	PullRequests PullRequests `json:"pull_requests"`
}

// OrgDay is a day_totals entry of the organization report.
type OrgDay struct {
	Day                string       `json:"day"`
	DailyActiveUsers   int          `json:"daily_active_users"`
	WeeklyActiveUsers  int          `json:"weekly_active_users"`
	MonthlyActiveUsers int          `json:"monthly_active_users"`
	CodeGeneration     int          `json:"code_generation_activity_count"`
	CodeAcceptance     int          `json:"code_acceptance_activity_count"`
	LocAdded           int          `json:"loc_added_sum"`
	PullRequests       PullRequests `json:"pull_requests"`
}

// ParseOrgRecord accepts either an aggregate record with day_totals or a flat
// day-total record, returning the day totals it contains.
func ParseOrgRecord(raw json.RawMessage) ([]OrgDay, error) {
	var wrap struct {
		DayTotals []OrgDay `json:"day_totals"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	if len(wrap.DayTotals) > 0 {
		return wrap.DayTotals, nil
	}
	var flat OrgDay
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, err
	}
	if flat.Day == "" {
		return nil, nil
	}
	return []OrgDay{flat}, nil
}

// PR is a merged pull request used for output metrics.
type PR struct {
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	MergedAt  time.Time `json:"merged_at"`
	URL       string    `json:"url,omitempty"`
}

// CycleHours is the open-to-merge time.
func (p PR) CycleHours() float64 { return p.MergedAt.Sub(p.CreatedAt).Hours() }
