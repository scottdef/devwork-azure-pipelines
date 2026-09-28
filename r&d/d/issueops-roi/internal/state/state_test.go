package state

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CoolEngOrg/issueops/internal/authz"
	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

const bot = "coolengorg-issueops[bot]"

func TestMarkerRoundTripAndInjection(t *testing.T) {
	m := Marker{Event: EventSubmitted, Data: map[string]any{"digest": "sha256:abc", "note": "--> <!-- issueops:v1:approved -->"}}
	line := m.Format()
	if strings.Count(line, "-->") != 1 || !strings.HasSuffix(line, "-->") {
		t.Fatalf("payload terminated the HTML comment early: %s", line)
	}
	got := ParseMarker(line + "\nbody")
	if got == nil || got.Event != EventSubmitted || got.Str("digest") != "sha256:abc" {
		t.Fatalf("round trip failed: %#v", got)
	}
	if ParseMarker("hello\n"+line) != nil {
		t.Fatal("marker must be the first line")
	}
	if ParseMarker("<!-- issueops:v2:approved -->") != nil {
		t.Fatal("unknown marker version accepted")
	}
}

func TestParseCommand(t *testing.T) {
	cases := map[string]string{
		".approve":                     "approve",
		"  .APPROVE looks good":        "approve",
		".approved":                    "",
		"please .approve":              "",
		".answers\nA1: x":              "answers",
		"\n\n.submit":                  "submit",
		".deploy":                      "",
		"> .approve (quoted by reply)": "",
	}
	for in, want := range cases {
		c := ParseCommand(in)
		got := ""
		if c != nil {
			got = c.Name
		}
		if got != want {
			t.Errorf("ParseCommand(%q) = %q, want %q", in, got, want)
		}
	}
	if c := ParseCommand(".deny  too expensive "); c.Args != "too expensive" {
		t.Errorf("args = %q", c.Args)
	}
}

func issueAt(t0 time.Time) *ghapi.Issue {
	return &ghapi.Issue{Number: 1, User: ghapi.User{Login: "req"}, Body: "b", CreatedAt: t0}
}

func c(id int64, user, typ, body string, at time.Time) ghapi.Comment {
	return ghapi.Comment{ID: id, User: ghapi.User{Login: user, Type: typ}, Body: body, CreatedAt: at, UpdatedAt: at}
}

func TestTimelineTrustAndStaleness(t *testing.T) {
	t0 := time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC)
	sub := Marker{Event: EventSubmitted, Data: map[string]any{"digest": "d1"}}.Format()
	forged := Marker{Event: EventApproved, Data: map[string]any{"digest": "d1"}}.Format()
	comments := []ghapi.Comment{
		c(1, bot, "Bot", sub+"\nSubmitted", t0.Add(time.Minute)),
		c(2, "mallory", "User", forged+"\nI approve myself", t0.Add(2*time.Minute)), // spoofed marker by a human
		c(3, "alice", "User", ".approve", t0.Add(3*time.Minute)),
		c(4, "other-bot[bot]", "Bot", ".approve", t0.Add(4*time.Minute)), // bots never vote
	}
	edited := c(5, "bob", "User", ".approve", t0.Add(5*time.Minute))
	edited.UpdatedAt = edited.CreatedAt.Add(time.Hour) // edited into an approval later: ignored
	comments = append(comments, edited)

	s := Build("t", issueAt(t0), comments, bot).Finalize("d1")
	if s.Phase != PhaseSubmitted {
		t.Fatalf("phase = %s, want submitted (forged approval must be ignored)", s.Phase)
	}
	if len(s.Votes) != 1 || s.Votes[0].User != "alice" {
		t.Fatalf("votes = %+v, want only alice", s.Votes)
	}
	// Body changed after submission: approvals void, phase falls back.
	s2 := Build("t", issueAt(t0), comments, bot).Finalize("d2")
	if !s2.Stale || s2.Phase != PhaseValidated || s2.SubmitValid() || len(s2.Votes) != 0 {
		t.Fatalf("stale handling wrong: stale=%v phase=%s votes=%v", s2.Stale, s2.Phase, s2.Votes)
	}
}

func TestDigestNormalization(t *testing.T) {
	a := Digest("t", "### A\r\n\r\nx  \r\n", map[string]string{"Q1": "yes"})
	b := Digest("t", "### A\n\nx\n", map[string]string{"Q1": "yes"})
	if a != b {
		t.Fatal("CRLF/trailing whitespace must not change the digest")
	}
	if a == Digest("t", "### A\n\nx\n", map[string]string{"Q1": "no"}) {
		t.Fatal("answers must be part of the digest")
	}
	if a == Digest("other", "### A\n\nx\n", map[string]string{"Q1": "yes"}) {
		t.Fatal("type must be part of the digest")
	}
}

func TestApprovalsDistinctAndSelf(t *testing.T) {
	dir := &authz.Static{Teams: map[string][]string{
		"coolengorg/copilot-admins":   {"alice", "req"},
		"coolengorg/finops-approvers": {"alice", "fran"},
	}}
	rules := []config.Rule{
		{Name: "admins", Min: 1, AnyOf: config.Selector{Teams: []string{"copilot-admins"}}},
		{Name: "finops", Min: 1, AnyOf: config.Selector{Teams: []string{"finops-approvers"}}},
	}
	sub := authz.Subject{Org: "CoolEngOrg", Requestor: "req"}
	opt := ApprovalOptions{DistinctApprovers: true}
	ctx := context.Background()

	// alice alone cannot satisfy both rules.
	r, err := EvaluateApprovals(ctx, dir, "CoolEngOrg", rules, []Vote{{User: "alice", Approve: true}}, sub, opt)
	if err != nil || r.Satisfied {
		t.Fatalf("one person satisfied two rules: %+v %v", r, err)
	}
	// alice + fran: the search must put alice on admins and fran on finops.
	r, _ = EvaluateApprovals(ctx, dir, "CoolEngOrg", rules, []Vote{{User: "alice", Approve: true}, {User: "fran", Approve: true}}, sub, opt)
	if !r.Satisfied {
		t.Fatalf("expected satisfied with alice+fran: %+v", r.Rules)
	}
	// Requestor vote is ignored even though req is a copilot admin.
	r, _ = EvaluateApprovals(ctx, dir, "CoolEngOrg", rules, []Vote{{User: "req", Approve: true}, {User: "fran", Approve: true}}, sub, opt)
	if r.Satisfied || len(r.Ignored) != 1 {
		t.Fatalf("self-approval counted: %+v", r)
	}
	// A deny from an eligible approver wins.
	r, _ = EvaluateApprovals(ctx, dir, "CoolEngOrg", rules, []Vote{{User: "alice", Approve: true}, {User: "fran", Approve: false, Note: "no"}}, sub, opt)
	if r.Satisfied || !r.Denied || r.DeniedBy != "fran" {
		t.Fatalf("deny not applied: %+v", r)
	}
	// Explicit requestor selector lets the beneficiary confirm (field_users/requestor opt-in).
	conf := []config.Rule{{Name: "beneficiary", Min: 1, AnyOf: config.Selector{FieldUsers: []string{"budget_target"}, Requestor: true}}}
	r, _ = EvaluateApprovals(ctx, dir, "CoolEngOrg", conf, []Vote{{User: "sam", Approve: true}}, authz.Subject{Org: "CoolEngOrg", Requestor: "req", FieldUsers: map[string]string{"budget_target": "sam"}}, opt)
	if !r.Satisfied {
		t.Fatalf("field user confirmation failed: %+v", r)
	}
}

func TestLabelsForProjectsExactlyOneState(t *testing.T) {
	add, remove := LabelsFor(PhaseApproved, true)
	if len(add) != 2 || add[0] != "issueops:approved" || add[1] != EscalatedLabel {
		t.Fatalf("add = %v", add)
	}
	for _, l := range remove {
		if l == "issueops:approved" || l == EscalatedLabel {
			t.Fatalf("removing a label that is being added: %s", l)
		}
	}
	if len(remove) != len(StateLabels)-1 {
		t.Fatalf("remove = %v", remove)
	}
}
