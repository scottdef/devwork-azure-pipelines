// Package state reconstructs the lifecycle of an IssueOps request from the
// issue timeline (event sourcing). Labels are only a projection for humans:
// every decision is recomputed from
//
//   - bot "marker" comments, trusted ONLY when authored by the IssueOps GitHub
//     App bot, and only when the marker is the first line of the comment;
//   - human commands (.submit, .approve, .deny, .cancel, .answers, .retry,
//     .status, .help); and
//   - a digest of the current request body (plus Q&A answers), which binds
//     approvals to exactly what was approved (TOCTOU protection).
package state

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MarkerVersion is embedded in every marker.
const MarkerVersion = "v1"

// Marker events.
const (
	EventSummary   = "summary"
	EventInvalid   = "invalid"
	EventQuestions = "questions"
	EventQAStatus  = "qa-status"
	EventSpec      = "spec"
	EventSubmitted = "submitted"
	EventStatus    = "approval-status"
	EventApproved  = "approved"
	EventDenied    = "denied"
	EventCancelled = "cancelled"
	EventReset     = "reset"
	EventExecuting = "executing"
	EventCompleted = "completed"
	EventFailed    = "failed"
	EventNotice    = "notice"
)

// Marker is machine state embedded in a bot comment's first line:
//
//	<!-- issueops:v1:submitted {"digest":"sha256:...","actor":"alice"} -->
type Marker struct {
	Event string         `json:"-"`
	Data  map[string]any `json:"-"`
}

var markerRe = regexp.MustCompile(`^<!-- issueops:(v[0-9]+):([a-z-]+)(?: (\{.*\}))? -->$`)

// Format renders the marker line. encoding/json escapes <, > and & so the JSON
// payload can never terminate the HTML comment early.
func (m Marker) Format() string {
	if len(m.Data) == 0 {
		return fmt.Sprintf("<!-- issueops:%s:%s -->", MarkerVersion, m.Event)
	}
	b, err := json.Marshal(m.Data)
	if err != nil {
		b = []byte(`{}`)
	}
	return fmt.Sprintf("<!-- issueops:%s:%s %s -->", MarkerVersion, m.Event, b)
}

// Prefix is the stable prefix used by peter-evans/find-comment to locate an
// updatable comment of this event type.
func Prefix(event string) string {
	return fmt.Sprintf("<!-- issueops:%s:%s", MarkerVersion, event)
}

// ParseMarker parses the first line of a comment body. It returns nil when the
// first line is not a marker.
func ParseMarker(body string) *Marker {
	first, _, _ := strings.Cut(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	m := markerRe.FindStringSubmatch(strings.TrimSpace(first))
	if m == nil || m[1] != MarkerVersion {
		return nil
	}
	mk := &Marker{Event: m[2], Data: map[string]any{}}
	if m[3] != "" {
		if err := json.Unmarshal([]byte(m[3]), &mk.Data); err != nil {
			return nil
		}
	}
	return mk
}

// Str returns a string field from marker data.
func (m *Marker) Str(key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m.Data[key].(string); ok {
		return s
	}
	return ""
}

// Command is a parsed human command.
type Command struct {
	Name string // "submit", "approve", ...
	Args string // text after the command on the first line
	Body string // remaining lines (used by .answers)
}

// Commands recognized by the router. Privileged commands are approver commands.
var knownCommands = map[string]bool{
	"submit": true, "approve": true, "deny": true, "cancel": true,
	"answers": true, "retry": true, "status": true, "help": true,
}

// PrivilegedCommands require approver eligibility.
var PrivilegedCommands = map[string]bool{"approve": true, "deny": true, "retry": true}

// ParseCommand recognizes ".command [args]" as the exact first token of the
// first non-empty line. ".approved" or "please .approve" are NOT commands.
func ParseCommand(body string) *Command {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimLeft(body, " \t\n")
	first, rest, _ := strings.Cut(body, "\n")
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, ".") {
		return nil
	}
	tok, args, _ := strings.Cut(first, " ")
	name := strings.ToLower(strings.TrimPrefix(tok, "."))
	if !knownCommands[name] {
		return nil
	}
	return &Command{Name: name, Args: strings.TrimSpace(args), Body: rest}
}

// Trigger returns the literal command string (for github/command's `command` input).
func (c *Command) Trigger() string { return "." + c.Name }
