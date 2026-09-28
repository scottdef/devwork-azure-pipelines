package state

import (
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

// Phases of a request.
const (
	PhaseNew       = "new"
	PhaseInvalid   = "invalid"
	PhaseAwaiting  = "awaiting-answers"
	PhaseValidated = "validated"
	PhaseSubmitted = "submitted"
	PhaseApproved  = "approved"
	PhaseExecuting = "executing"
	PhaseCompleted = "completed"
	PhaseFailed    = "failed"
	PhaseDenied    = "denied"
	PhaseCancelled = "cancelled"
)

// StateLabels maps phases to their label. Exactly one is applied at a time.
var StateLabels = map[string]string{
	PhaseInvalid:   "issueops:invalid",
	PhaseAwaiting:  "issueops:awaiting-answers",
	PhaseValidated: "issueops:validated",
	PhaseSubmitted: "issueops:submitted",
	PhaseApproved:  "issueops:approved",
	PhaseExecuting: "issueops:executing",
	PhaseCompleted: "issueops:completed",
	PhaseFailed:    "issueops:failed",
	PhaseDenied:    "issueops:denied",
	PhaseCancelled: "issueops:cancelled",
}

// EscalatedLabel flags requests with policy escalations.
const EscalatedLabel = "issueops:escalated"

// LabelsFor returns labels to add and remove to project a phase.
func LabelsFor(phase string, escalated bool) (add, remove []string) {
	want := StateLabels[phase]
	for p, l := range StateLabels {
		if p != phase {
			remove = append(remove, l)
		}
	}
	if want != "" {
		add = append(add, want)
	}
	if escalated {
		add = append(add, EscalatedLabel)
	} else {
		remove = append(remove, EscalatedLabel)
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove
}

// EditTolerance is how long after creation a command comment may be updated
// (GitHub sometimes stamps updated_at a moment after created_at).
const EditTolerance = 5 * time.Second

// IsTerminal reports whether a phase is final.
func IsTerminal(phase string) bool {
	return phase == PhaseDenied || phase == PhaseCancelled || phase == PhaseCompleted
}

// MarkerEvent is a trusted marker with its comment metadata.
type MarkerEvent struct {
	*Marker
	CommentID int64
	At        time.Time
}

// CommandEvent is a human command with its comment metadata.
type CommandEvent struct {
	*Command
	User      string
	CommentID int64
	At        time.Time
}

// Vote is an approver's latest .approve/.deny after the current submission.
type Vote struct {
	User      string
	Approve   bool
	Note      string
	CommentID int64
	At        time.Time
}

// Snapshot is the reconstructed state of a request.
type Snapshot struct {
	TypeID        string
	Requestor     string
	BotLogin      string
	CurrentDigest string
	Phase         string
	// Stale is true when the request was submitted but the body/answers changed
	// since; approvals are void and .submit is required again.
	Stale        bool
	LastSubmit   *MarkerEvent
	LastApproved *MarkerEvent
	Markers      []MarkerEvent
	Commands     []CommandEvent
	Votes        []Vote
	// AnswerComments are the requestor's .answers commands, oldest first.
	AnswerComments []CommandEvent
}

// Build parses the timeline. Call Finalize with the current digest before
// reading Phase or Votes: for Q&A requests the digest depends on answers and
// on dynamic questions that are themselves carried by markers, so digest
// computation happens between the two steps.
func Build(typeID string, issue *ghapi.Issue, comments []ghapi.Comment, botLogin string) *Snapshot {
	s := &Snapshot{TypeID: typeID, Requestor: issue.User.Login, BotLogin: botLogin, Phase: PhaseNew}
	sorted := append([]ghapi.Comment{}, comments...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})
	for _, c := range sorted {
		if botLogin != "" && strings.EqualFold(c.User.Login, botLogin) {
			if m := ParseMarker(c.Body); m != nil {
				s.Markers = append(s.Markers, MarkerEvent{Marker: m, CommentID: c.ID, At: c.CreatedAt})
			}
			continue
		}
		if strings.EqualFold(c.User.Type, "Bot") {
			continue // other bots can never issue commands
		}
		if cmd := ParseCommand(c.Body); cmd != nil {
			// Decision commands must be unedited: editing an old comment into
			// ".approve" (or changing a vote) is ignored. Answers may be edited
			// because they feed the digest, which voids approvals anyway.
			if cmd.Name != "answers" && !c.UpdatedAt.IsZero() && c.UpdatedAt.Sub(c.CreatedAt) > EditTolerance {
				continue
			}
			ev := CommandEvent{Command: cmd, User: c.User.Login, CommentID: c.ID, At: c.CreatedAt}
			s.Commands = append(s.Commands, ev)
			if cmd.Name == "answers" && strings.EqualFold(c.User.Login, s.Requestor) {
				s.AnswerComments = append(s.AnswerComments, ev)
			}
		}
	}
	return s
}

// Finalize computes phase, staleness and votes for the given current digest.
func (s *Snapshot) Finalize(currentDigest string) *Snapshot {
	s.CurrentDigest = currentDigest
	s.Phase, s.Stale, s.LastSubmit, s.LastApproved, s.Votes = PhaseNew, false, nil, nil, nil
	s.applyMarkers()
	s.collectVotes()
	return s
}

func (s *Snapshot) applyMarkers() {
	for i := range s.Markers {
		m := s.Markers[i]
		terminal := IsTerminal(s.Phase)
		switch m.Event {
		case EventReset:
			s.Phase = PhaseValidated
			if m.Str("phase") != "" {
				s.Phase = m.Str("phase")
			}
			s.LastSubmit, s.LastApproved = nil, nil
		case EventSummary:
			if terminal || s.Phase == PhaseSubmitted || s.Phase == PhaseApproved || s.Phase == PhaseExecuting || s.Phase == PhaseFailed {
				continue // summaries are informational once submitted
			}
			switch {
			case m.Data["valid"] != true:
				s.Phase = PhaseInvalid
			case m.Data["qa_complete"] == false:
				s.Phase = PhaseAwaiting
			default:
				s.Phase = PhaseValidated
			}
		case EventInvalid:
			if !terminal {
				s.Phase = PhaseInvalid
			}
		case EventQuestions:
			if !terminal || s.Phase == PhaseCompleted {
				s.Phase = PhaseAwaiting
				s.LastSubmit, s.LastApproved = nil, nil
			}
		case EventSpec:
			if !terminal && s.Phase != PhaseSubmitted && s.Phase != PhaseApproved && s.Phase != PhaseExecuting {
				s.Phase = PhaseValidated
			}
		case EventSubmitted:
			if !terminal {
				ev := m
				s.LastSubmit, s.LastApproved = &ev, nil
				s.Phase = PhaseSubmitted
			}
		case EventApproved:
			if !terminal {
				ev := m
				s.LastApproved = &ev
				if m.Data["auto"] == true {
					// Auto-approval (no approval rules) is its own submission.
					s.LastSubmit = &ev
				}
				s.Phase = PhaseApproved
			}
		case EventExecuting:
			if !terminal {
				s.Phase = PhaseExecuting
			}
		case EventCompleted:
			if !terminal {
				s.Phase = PhaseCompleted
			}
		case EventFailed:
			if !terminal {
				s.Phase = PhaseFailed
			}
		case EventDenied:
			if !terminal {
				s.Phase = PhaseDenied
			}
		case EventCancelled:
			if !terminal {
				s.Phase = PhaseCancelled
			}
		}
	}
	if s.LastSubmit != nil && s.LastSubmit.Str("digest") != s.CurrentDigest {
		switch s.Phase {
		case PhaseSubmitted, PhaseApproved, PhaseFailed:
			s.Stale = true
			s.Phase = PhaseValidated
		}
	}
}

// SubmitValid reports whether there is a submission for the current content.
func (s *Snapshot) SubmitValid() bool {
	return s.LastSubmit != nil && !s.Stale && s.LastSubmit.Str("digest") == s.CurrentDigest
}

// ApprovedValid reports whether the last approval marker matches the current content.
func (s *Snapshot) ApprovedValid() bool {
	return s.SubmitValid() && s.LastApproved != nil && s.LastApproved.Str("digest") == s.CurrentDigest
}

func (s *Snapshot) collectVotes() {
	if !s.SubmitValid() {
		return
	}
	latest := map[string]Vote{}
	order := []string{}
	for _, c := range s.Commands {
		if c.Name != "approve" && c.Name != "deny" {
			continue
		}
		// Votes must come strictly after the submission marker comment.
		if c.At.Before(s.LastSubmit.At) || (c.At.Equal(s.LastSubmit.At) && c.CommentID < s.LastSubmit.CommentID) {
			continue
		}
		key := strings.ToLower(c.User)
		if _, ok := latest[key]; !ok {
			order = append(order, key)
		}
		latest[key] = Vote{User: c.User, Approve: c.Name == "approve", Note: c.Args, CommentID: c.CommentID, At: c.At}
	}
	for _, k := range order {
		s.Votes = append(s.Votes, latest[k])
	}
}

// LastMarker returns the most recent marker of an event type.
func (s *Snapshot) LastMarker(event string) *MarkerEvent {
	for i := len(s.Markers) - 1; i >= 0; i-- {
		if s.Markers[i].Event == event {
			return &s.Markers[i]
		}
	}
	return nil
}

// MarkersOf returns all markers of an event type, oldest first.
func (s *Snapshot) MarkersOf(event string) []MarkerEvent {
	var out []MarkerEvent
	for _, m := range s.Markers {
		if m.Event == event {
			out = append(out, m)
		}
	}
	return out
}
