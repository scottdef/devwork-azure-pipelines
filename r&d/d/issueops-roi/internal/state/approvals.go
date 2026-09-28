package state

import (
	"context"
	"sort"
	"strings"

	"github.com/CoolEngOrg/issueops/internal/authz"
	"github.com/CoolEngOrg/issueops/internal/config"
)

// ApproverRef is an approver counted toward a rule.
type ApproverRef struct {
	User   string `json:"user"`
	Reason string `json:"reason"`
}

// RuleStatus is the progress of one approval rule.
type RuleStatus struct {
	Name      string        `json:"name"`
	Min       int           `json:"min"`
	Approvers []ApproverRef `json:"approvers"`
	Missing   int           `json:"missing"`
	Who       string        `json:"who"` // human description of eligible approvers
}

// IgnoredVote explains why a vote did not count.
type IgnoredVote struct {
	User   string `json:"user"`
	Reason string `json:"reason"`
}

// ApprovalResult is the evaluated approval state.
type ApprovalResult struct {
	Satisfied bool          `json:"satisfied"`
	Denied    bool          `json:"denied"`
	DeniedBy  string        `json:"denied_by,omitempty"`
	DenyNote  string        `json:"deny_note,omitempty"`
	Rules     []RuleStatus  `json:"rules"`
	Ignored   []IgnoredVote `json:"ignored,omitempty"`
}

// ApprovalOptions carries policy switches.
type ApprovalOptions struct {
	AllowSelfApproval bool
	DistinctApprovers bool
}

// Describe renders who can satisfy a selector, for comments.
func Describe(org string, s config.Selector) string {
	var parts []string
	for _, t := range s.Teams {
		if strings.Contains(t, "/") {
			parts = append(parts, "@"+t)
		} else {
			parts = append(parts, "@"+org+"/"+t)
		}
	}
	for _, u := range s.Users {
		parts = append(parts, "@"+strings.TrimPrefix(u, "@"))
	}
	for _, r := range s.OrgRoles {
		if r == "owner" {
			parts = append(parts, "organization owners")
		} else {
			parts = append(parts, "organization role `"+r+"`")
		}
	}
	if len(s.RepoPermissions) > 0 {
		parts = append(parts, "`"+strings.Join(s.RepoPermissions, "`/`")+"` on the IssueOps repository")
	}
	if len(s.TargetRepoPermissions) > 0 {
		parts = append(parts, "`"+strings.Join(s.TargetRepoPermissions, "`/`")+"` on the target repository")
	}
	for _, f := range s.FieldUsers {
		parts = append(parts, "the user named in `"+f+"`")
	}
	if s.Requestor {
		parts = append(parts, "the requestor")
	}
	return strings.Join(parts, " or ")
}

// selfSelector restricts a requestor's own vote to rules that explicitly
// include the requestor (or a named-user field) when self-approval is off.
func selfSelector(s config.Selector) config.Selector {
	return config.Selector{Requestor: s.Requestor, FieldUsers: s.FieldUsers}
}

// EvaluateApprovals counts votes against the plan's rules.
func EvaluateApprovals(ctx context.Context, dir authz.Directory, org string, rules []config.Rule, votes []Vote, sub authz.Subject, opt ApprovalOptions) (*ApprovalResult, error) {
	res := &ApprovalResult{}
	type elig struct {
		user    string
		reasons map[int]string // rule index -> reason
	}
	var approvers []elig
	for _, v := range votes {
		isSelf := strings.EqualFold(v.User, sub.Requestor)
		reasons := map[int]string{}
		for i, r := range rules {
			sel := r.AnyOf
			if isSelf && !opt.AllowSelfApproval {
				sel = selfSelector(sel)
			}
			m, err := authz.Evaluate(ctx, dir, sel, v.User, sub)
			if err != nil {
				return nil, err
			}
			if m.OK {
				reasons[i] = m.Reason
			}
		}
		if len(reasons) == 0 {
			why := "not an eligible approver for any required rule"
			if isSelf && !opt.AllowSelfApproval {
				why = "requestors cannot approve their own request"
			}
			res.Ignored = append(res.Ignored, IgnoredVote{User: v.User, Reason: why})
			continue
		}
		if !v.Approve {
			res.Denied, res.DeniedBy, res.DenyNote = true, v.User, v.Note
			continue
		}
		approvers = append(approvers, elig{user: v.User, reasons: reasons})
	}

	// Assign approvers to rules. With distinct approvers, each person counts
	// toward at most one rule; a small backtracking search finds a complete
	// assignment when one exists (approver sets are tiny).
	assign := make([][]int, len(rules)) // rule -> approver indexes
	if !opt.DistinctApprovers {
		for ai, a := range approvers {
			for ri := range rules {
				if _, ok := a.reasons[ri]; ok {
					assign[ri] = append(assign[ri], ai)
				}
			}
		}
	} else {
		best := make([][]int, len(rules))
		bestScore := -1
		cur := make([][]int, len(rules))
		score := func() int {
			s := 0
			for ri, r := range rules {
				s += min(len(cur[ri]), r.Min)
			}
			return s
		}
		totalMin := 0
		for _, r := range rules {
			totalMin += r.Min
		}
		var dfs func(ai int)
		dfs = func(ai int) {
			if bestScore == totalMin {
				return // complete assignment already found
			}
			if ai == len(approvers) {
				if sc := score(); sc > bestScore {
					bestScore = sc
					for ri := range cur {
						best[ri] = append([]int{}, cur[ri]...)
					}
				}
				return
			}
			for ri := range rules {
				if _, ok := approvers[ai].reasons[ri]; ok && len(cur[ri]) < rules[ri].Min {
					cur[ri] = append(cur[ri], ai)
					dfs(ai + 1)
					cur[ri] = cur[ri][:len(cur[ri])-1]
				}
			}
			dfs(ai + 1) // also try leaving this approver unassigned
		}
		if len(approvers) <= 10 {
			dfs(0)
		} else { // greedy fallback for unusually large approver sets
			for ai, a := range approvers {
				for ri := range rules {
					if _, ok := a.reasons[ri]; ok && len(best[ri]) < rules[ri].Min {
						best[ri] = append(best[ri], ai)
						break
					}
				}
			}
		}
		assign = best
	}

	res.Satisfied = !res.Denied
	for ri, r := range rules {
		st := RuleStatus{Name: r.Name, Min: r.Min, Who: Describe(org, r.AnyOf)}
		for _, ai := range assign[ri] {
			st.Approvers = append(st.Approvers, ApproverRef{User: approvers[ai].user, Reason: approvers[ai].reasons[ri]})
		}
		sort.Slice(st.Approvers, func(i, j int) bool { return st.Approvers[i].User < st.Approvers[j].User })
		st.Missing = max(0, r.Min-len(st.Approvers))
		if st.Missing > 0 {
			res.Satisfied = false
		}
		res.Rules = append(res.Rules, st)
	}
	return res, nil
}

// VoterEligible reports whether a user can satisfy (or deny against) any rule.
func VoterEligible(ctx context.Context, dir authz.Directory, rules []config.Rule, user string, sub authz.Subject, opt ApprovalOptions) (bool, string, error) {
	isSelf := strings.EqualFold(user, sub.Requestor)
	for _, r := range rules {
		sel := r.AnyOf
		if isSelf && !opt.AllowSelfApproval {
			sel = selfSelector(sel)
		}
		m, err := authz.Evaluate(ctx, dir, sel, user, sub)
		if err != nil {
			return false, "", err
		}
		if m.OK {
			return true, m.Reason, nil
		}
	}
	if isSelf && !opt.AllowSelfApproval {
		return false, "requestors cannot approve their own request", nil
	}
	return false, "not an eligible approver for this request", nil
}
