// Package event decodes the repository_dispatch event that GitHub hands a
// workflow as ${{ toJSON(github.event) }}. Everything in client_payload
// came from an issue body and is treated as untrusted: every field is
// checked here, once, and the rest of the program sees only clean values.
package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Actions this library handles.
const (
	Publish = "library-publish"
	Borrow  = "library-borrow"
	Return  = "library-return"
)

// Templates is the closed set of item templates. Adding one means adding
// templates/items/<id>.tmpl and listing it here and in the issue form.
var Templates = []string{"caddyfile", "workflow", "issue-form", "terraform-module", "nomad-job", "readme"}

var (
	reSource = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,46}[a-z0-9]$`)
	reLogin  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})(?:\[bot\])?$`)
	reRepo   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// MaxPayload mirrors the intake's limit.
const MaxPayload = 32 << 10

// Event is a validated dispatch.
type Event struct {
	Action     string         // event_type
	Issue      int            // intake issue number
	Repo       string         // intake owner/name
	Requester  string         // issue author
	SourceID   string         // publish
	TemplateID string         // publish
	Phase      int            // publish
	Payload    map[string]any // publish, decoded
	PayloadRaw string         // publish, as sent
	SHA256     string         // publish
	ItemID     string         // borrow/return (and derived for publish)
	Days       int            // borrow
	Reason     string         // borrow
}

type wire struct {
	Action        string         `json:"action"`
	ClientPayload map[string]any `json:"client_payload"`
}

// Decode reads a GitHub event document.
func Decode(r io.Reader) (*Event, error) {
	var w wire
	dec := json.NewDecoder(io.LimitReader(r, 1<<20))
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("event: %w", err)
	}
	p := w.ClientPayload
	if p == nil {
		return nil, fmt.Errorf("event: no client_payload")
	}
	e := &Event{Action: w.Action}
	var err error
	if e.Issue, err = intField(p, "issue"); err != nil {
		return nil, err
	}
	if e.Issue <= 0 {
		return nil, fmt.Errorf("event: issue must be positive")
	}
	e.Repo = str(p, "repo")
	if !reRepo.MatchString(e.Repo) {
		return nil, fmt.Errorf("event: bad repo %q", e.Repo)
	}
	e.Requester = str(p, "requester")
	if !reLogin.MatchString(e.Requester) {
		return nil, fmt.Errorf("event: bad requester %q", e.Requester)
	}
	switch e.Action {
	case Publish:
		return e, e.publish(p)
	case Borrow:
		if e.ItemID, err = CleanItemID(str(p, "item_id")); err != nil {
			return nil, err
		}
		d, err := intField(p, "days")
		if err != nil {
			return nil, err
		}
		if d != 7 && d != 14 {
			return nil, fmt.Errorf("event: days must be 7 or 14, got %d", d)
		}
		e.Days = d
		e.Reason = str(p, "reason")
		if len(e.Reason) > 1024 {
			e.Reason = e.Reason[:1024]
		}
	case Return:
		if e.ItemID, err = CleanItemID(str(p, "item_id")); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("event: unknown action %q", e.Action)
	}
	return e, nil
}

func (e *Event) publish(p map[string]any) error {
	e.SourceID = str(p, "source_id")
	if !reSource.MatchString(e.SourceID) {
		return fmt.Errorf("event: bad source_id %q", e.SourceID)
	}
	e.TemplateID = str(p, "template_id")
	if !KnownTemplate(e.TemplateID) {
		return fmt.Errorf("event: unknown template_id %q", e.TemplateID)
	}
	ph, err := intField(p, "phase")
	if err != nil {
		return err
	}
	if ph < 1 || ph > 3 {
		return fmt.Errorf("event: phase must be 1-3, got %d", ph)
	}
	e.Phase = ph
	e.PayloadRaw = str(p, "payload")
	if len(e.PayloadRaw) > MaxPayload {
		return fmt.Errorf("event: payload is %d bytes; limit %d", len(e.PayloadRaw), MaxPayload)
	}
	sum := sha256.Sum256([]byte(e.PayloadRaw))
	e.SHA256 = hex.EncodeToString(sum[:])
	if want := str(p, "sha256"); want != e.SHA256 {
		return fmt.Errorf("event: payload sha256 %s does not match sent %q", e.SHA256, want)
	}
	dec := json.NewDecoder(strings.NewReader(e.PayloadRaw))
	dec.UseNumber() // keep 8443 as 8443, not 8443.0 or 8.443e+03
	if err := dec.Decode(&e.Payload); err != nil {
		return fmt.Errorf("event: payload: %w", err)
	}
	if e.Payload == nil {
		return fmt.Errorf("event: payload must be a JSON object")
	}
	e.ItemID = fmt.Sprintf("%s/%s/%d", e.SourceID, e.TemplateID, e.Issue)
	return nil
}

// KnownTemplate reports whether id is in Templates.
func KnownTemplate(id string) bool {
	for _, t := range Templates {
		if t == id {
			return true
		}
	}
	return false
}

// CleanItemID validates "source/template/issue". It is the only way an
// item id becomes a path, so it must reject anything that could escape
// items/: dots, slashes beyond the two separators, empty parts.
func CleanItemID(id string) (string, error) {
	parts := strings.Split(strings.TrimSpace(id), "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("event: item id %q is not source/template/issue", id)
	}
	if !reSource.MatchString(parts[0]) || !KnownTemplate(parts[1]) {
		return "", fmt.Errorf("event: bad item id %q", id)
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n <= 0 || strconv.Itoa(n) != parts[2] {
		return "", fmt.Errorf("event: bad issue number in item id %q", id)
	}
	return strings.Join(parts, "/"), nil
}

func str(p map[string]any, k string) string {
	s, _ := p[k].(string)
	return strings.TrimSpace(s)
}

// intField accepts a JSON number or a decimal string: the intake sends
// phase and days as strings because issue forms do.
func intField(p map[string]any, k string) (int, error) {
	switch v := p[k].(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("event: %s is not an integer", k)
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("event: %s: %w", k, err)
		}
		return n, nil
	case nil:
		return 0, fmt.Errorf("event: missing %s", k)
	}
	return 0, fmt.Errorf("event: %s has type %T", k, p[k])
}
