package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/state"
)

// opsIssue is an issue plus (optionally) its comments, for ops metrics.
type opsIssue struct {
	Issue    ghapi.Issue     `json:"issue"`
	Comments []ghapi.Comment `json:"comments,omitempty"`
}

// cmdOpsMetrics exports operational metrics about the IssueOps platform
// itself (request volume, phases, lead time and approval latency) as
// Prometheus text, optionally pushed to a Pushgateway for Grafana.
//
//	issueops ops-metrics --days 90 [--deep] [--push]
//	issueops ops-metrics --issues-file examples/copilot-budget-request/output/scenario/timeline.json
func cmdOpsMetrics(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ops-metrics", flag.ExitOnError)
	var c common
	c.register(fs)
	days := fs.Int("days", 90, "look-back window (issues updated within it)")
	deep := fs.Bool("deep", false, "read comments to compute approval latency from markers (1 API call per issue)")
	issuesFile := fs.String("issues-file", "", "offline: JSON array of {issue, comments}")
	push := fs.Bool("push", false, "push to the Pushgateway after writing")
	gateway := fs.String("pushgateway", os.Getenv("PUSHGATEWAY_URL"), "Prometheus Pushgateway base URL")
	_ = fs.Parse(args)
	if *issuesFile != "" {
		c.offline = true
	}
	a, err := c.load()
	if err != nil {
		return err
	}
	var items []opsIssue
	switch {
	case *issuesFile != "":
		if err := readJSON(*issuesFile, &items); err != nil {
			return err
		}
	case a.gh != nil:
		since := a.now.AddDate(0, 0, -*days)
		list, err := a.gh.ListIssues(ctx, a.reg.Organization, a.reg.Repository, a.reg.PlatformLabel, since)
		if err != nil {
			return err
		}
		for _, is := range list {
			it := opsIssue{Issue: is}
			if *deep {
				if it.Comments, err = a.gh.ListComments(ctx, a.reg.Organization, a.reg.Repository, is.Number); err != nil {
					return err
				}
			}
			items = append(items, it)
		}
	default:
		return errors.New("ops-metrics needs the API or --issues-file")
	}
	text := opsPrometheus(a, items)
	p := filepath.Join(a.outDir, "ops.prom")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d requests)\n", p, len(items))
	if *push {
		return pushMetrics(ctx, *gateway, "issueops_ops", "org="+a.reg.Organization, p)
	}
	return nil
}

type opsAgg struct {
	byPhase      map[string]int
	opened       int
	lead         []float64 // created -> closed (completed only), hours
	approval     []float64 // submitted -> approved, hours (deep only)
	openAgeMax   float64
	escalated    int
	autoApproved int
}

func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64{}, v...)
	sort.Float64s(s)
	idx := q * float64(len(s)-1)
	lo, hi := int(math.Floor(idx)), int(math.Ceil(idx))
	return s[lo] + (s[hi]-s[lo])*(idx-float64(lo))
}

// phaseFromLabels reads the projected state label (labels are only a
// projection, but they are good enough for fleet-level counts).
func phaseFromLabels(is *ghapi.Issue) string {
	for _, l := range is.LabelNames() {
		for p, sl := range state.StateLabels {
			if l == sl {
				return p
			}
		}
	}
	return state.PhaseNew
}

func opsPrometheus(a *app, items []opsIssue) string {
	byType := map[string]*opsAgg{}
	for _, rt := range a.reg.RequestTypes {
		byType[rt.ID] = &opsAgg{byPhase: map[string]int{}}
	}
	for i := range items {
		is := &items[i].Issue
		rt, err := a.reg.ResolveLabels(is.LabelNames())
		if err != nil {
			continue
		}
		ag := byType[rt.ID]
		phase := phaseFromLabels(is)
		var snap *state.Snapshot
		if len(items[i].Comments) > 0 && a.botLogin != "" {
			// Deep mode: the timeline is authoritative (the digest is not
			// needed for latency, so the last submitted digest is reused).
			snap = state.Build(rt.ID, is, items[i].Comments, a.botLogin)
			if last := snap.LastMarker(state.EventSubmitted); last != nil {
				snap.Finalize(last.Str("digest"))
			} else {
				snap.Finalize("")
			}
			phase = snap.Phase
		}
		ag.byPhase[phase]++
		if is.CreatedAt.After(a.now.AddDate(0, 0, -30)) {
			ag.opened++
		}
		for _, l := range is.LabelNames() {
			if l == state.EscalatedLabel {
				ag.escalated++
			}
		}
		if phase == state.PhaseCompleted {
			// Agentic tasks stay open after completion (follow-up on the issue):
			// use the completed marker time when the issue is not closed.
			end := is.ClosedAt
			if snap != nil {
				if m := snap.LastMarker(state.EventCompleted); m != nil {
					end = m.At
				}
			}
			if !end.IsZero() {
				ag.lead = append(ag.lead, end.Sub(is.CreatedAt).Hours())
			}
		}
		if is.State == "open" && !state.IsTerminal(phase) {
			if h := a.now.Sub(is.CreatedAt).Hours(); h > ag.openAgeMax {
				ag.openAgeMax = h
			}
		}
		if snap != nil {
			var submitted time.Time
			for _, m := range snap.Markers {
				switch m.Event {
				case state.EventSubmitted:
					submitted = m.At
				case state.EventApproved:
					if m.Data["auto"] == true {
						ag.autoApproved++
					} else if !submitted.IsZero() {
						ag.approval = append(ag.approval, m.At.Sub(submitted).Hours())
					}
					submitted = time.Time{}
				}
			}
		}
	}

	var b strings.Builder
	org := promLabel(a.reg.Organization)
	w := func(name, help, typ string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
	}
	ids := make([]string, 0, len(byType))
	for id := range byType {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	phases := []string{state.PhaseNew, state.PhaseInvalid, state.PhaseAwaiting, state.PhaseValidated, state.PhaseSubmitted,
		state.PhaseApproved, state.PhaseExecuting, state.PhaseCompleted, state.PhaseFailed, state.PhaseDenied, state.PhaseCancelled}

	w("issueops_requests", "IssueOps requests updated in the window, by type and phase.", "gauge")
	for _, id := range ids {
		for _, p := range phases {
			fmt.Fprintf(&b, "issueops_requests{org=%q,type=%q,phase=%q} %d\n", org, id, p, byType[id].byPhase[p])
		}
	}
	w("issueops_requests_opened_30d", "Requests opened in the last 30 days.", "gauge")
	for _, id := range ids {
		fmt.Fprintf(&b, "issueops_requests_opened_30d{org=%q,type=%q} %d\n", org, id, byType[id].opened)
	}
	w("issueops_requests_escalated", "Requests carrying the escalation flag.", "gauge")
	for _, id := range ids {
		fmt.Fprintf(&b, "issueops_requests_escalated{org=%q,type=%q} %d\n", org, id, byType[id].escalated)
	}
	w("issueops_open_request_age_hours_max", "Age of the oldest open request.", "gauge")
	for _, id := range ids {
		fmt.Fprintf(&b, "issueops_open_request_age_hours_max{org=%q,type=%q} %.2f\n", org, id, byType[id].openAgeMax)
	}
	w("issueops_lead_time_hours", "Open-to-close time of completed requests.", "summary")
	for _, id := range ids {
		ag := byType[id]
		for _, q := range []float64{0.5, 0.9} {
			if v := quantile(ag.lead, q); !math.IsNaN(v) {
				fmt.Fprintf(&b, "issueops_lead_time_hours{org=%q,type=%q,quantile=\"%g\"} %.2f\n", org, id, q, v)
			}
		}
		fmt.Fprintf(&b, "issueops_lead_time_hours_count{org=%q,type=%q} %d\n", org, id, len(ag.lead))
	}
	w("issueops_approval_latency_hours", "Time from .submit to approval (deep mode).", "summary")
	for _, id := range ids {
		ag := byType[id]
		for _, q := range []float64{0.5, 0.9} {
			if v := quantile(ag.approval, q); !math.IsNaN(v) {
				fmt.Fprintf(&b, "issueops_approval_latency_hours{org=%q,type=%q,quantile=\"%g\"} %.2f\n", org, id, q, v)
			}
		}
		fmt.Fprintf(&b, "issueops_approval_latency_hours_count{org=%q,type=%q} %d\n", org, id, len(ag.approval))
	}
	w("issueops_auto_approved", "Requests auto-approved by policy (deep mode).", "gauge")
	for _, id := range ids {
		fmt.Fprintf(&b, "issueops_auto_approved{org=%q,type=%q} %d\n", org, id, byType[id].autoApproved)
	}
	w("issueops_ops_generated_timestamp_seconds", "When these metrics were generated.", "gauge")
	fmt.Fprintf(&b, "issueops_ops_generated_timestamp_seconds{org=%q} %d\n", org, a.now.Unix())
	return b.String()
}

// promLabel keeps label values printable; %q adds Go escaping, which matches
// the Prometheus text format for \, " and newline.
func promLabel(s string) string { return strings.ReplaceAll(s, "\n", " ") }
