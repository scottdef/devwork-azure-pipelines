// Package issuebody turns the Markdown body GitHub writes for an issue form
// back into values keyed by field id.
//
// GitHub renders every form field as
//
//	### <label>
//
//	<value>
//
// with "_No response_" for empty fields, "- [x] option" lines for
// checkboxes, and a fenced block for textareas that set render:. The body
// is untrusted: users edit issues after opening them, so the parser is
// lenient about whitespace, case and CRLF, never splits on a heading that
// sits inside a code fence, and drops sections under headings the form
// does not know.
package issuebody

import (
	"errors"
	"strings"

	"github.com/CoolGitOrg/issueops-intake/internal/form"
)

// NoResponse is the placeholder GitHub writes for an empty field.
const NoResponse = "_No response_"

// Checkboxes is the value of a checkboxes field, shaped like
// issue-ops/parser's output.
type Checkboxes struct {
	Selected   []string `json:"selected"`
	Unselected []string `json:"unselected"`
}

// ErrNoFields means no heading in the body matched a form field.
var ErrNoFields = errors.New("issuebody: no form fields found in body")

// Parse returns field id -> value. Values are string (input, textarea,
// single dropdown), []string (multiple dropdown) or Checkboxes.
// Empty inputs and textareas are omitted, as issue-ops/parser omits them.
func Parse(f *form.Form, body string) (map[string]any, error) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	byLabel := map[string]form.Field{}
	for _, fd := range f.Inputs() {
		byLabel[norm(fd.Label)] = fd
	}

	sections := map[string][]string{}
	var order []string
	cur := ""
	var fence string // active fence marker, "" when outside one
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if fence == "" {
			if m := fenceMarker(t); m != "" {
				fence = m
			} else if strings.HasPrefix(t, "### ") || t == "###" {
				label := norm(strings.TrimPrefix(t, "###"))
				if fd, ok := byLabel[label]; ok {
					cur = fd.ID
					if _, dup := sections[cur]; !dup {
						order = append(order, cur)
					}
					sections[cur] = nil // a repeated heading wins: last edit counts
					continue
				}
				// A heading the form does not know is a user addition:
				// it ends the current field and its text is dropped.
				cur = ""
				continue
			}
		} else if closesFence(t, fence) {
			fence = ""
		}
		if cur != "" {
			sections[cur] = append(sections[cur], ln)
		}
	}
	if len(order) == 0 {
		return nil, ErrNoFields
	}

	out := map[string]any{}
	for _, id := range order {
		fd, _ := f.Field(id)
		raw := trimBlank(sections[id])
		text := strings.Join(raw, "\n")
		switch fd.Type {
		case "checkboxes":
			out[id] = checkboxes(fd, raw)
		case "dropdown":
			if text == "" || strings.TrimSpace(text) == NoResponse {
				if fd.Multiple {
					out[id] = []string{}
				} else {
					out[id] = ""
				}
				continue
			}
			text = strings.TrimSpace(text)
			if fd.Multiple {
				var vals []string
				for _, v := range strings.Split(text, ",") {
					if v = strings.TrimSpace(v); v != "" {
						vals = append(vals, v)
					}
				}
				out[id] = vals
			} else {
				out[id] = text
			}
		default:
			if text == "" || strings.TrimSpace(text) == NoResponse {
				continue
			}
			if fd.Type == "textarea" {
				text = unfence(text)
			} else {
				text = strings.TrimSpace(text)
			}
			out[id] = text
		}
	}
	return out, nil
}

func norm(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "*") // some clients mark required fields
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// fenceMarker returns the run of ` or ~ that opens a fence on line t.
func fenceMarker(t string) string {
	for _, c := range []byte{'`', '~'} {
		n := 0
		for n < len(t) && t[n] == c {
			n++
		}
		if n >= 3 {
			if c == '`' && strings.ContainsRune(t[n:], '`') {
				return "" // inline code, not a fence
			}
			return t[:n]
		}
	}
	return ""
}

func closesFence(t, open string) bool {
	if !strings.HasPrefix(t, open) {
		return false
	}
	return strings.Trim(t, open[:1]) == ""
}

// unfence strips one surrounding code fence, if the value is exactly one
// fenced block. Text around a fence (a user edit) leaves the value alone.
func unfence(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	m := fenceMarker(strings.TrimSpace(lines[0]))
	if m == "" || !closesFence(strings.TrimSpace(lines[len(lines)-1]), m) {
		return text
	}
	for _, l := range lines[1 : len(lines)-1] {
		if closesFence(strings.TrimSpace(l), m) {
			return text // more than one block
		}
	}
	return strings.Join(lines[1:len(lines)-1], "\n")
}

func checkboxes(fd form.Field, lines []string) Checkboxes {
	cb := Checkboxes{Selected: []string{}, Unselected: []string{}}
	checked := map[string]bool{}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		t = strings.TrimLeft(t, "-*+ ")
		switch {
		case strings.HasPrefix(t, "[x]"), strings.HasPrefix(t, "[X]"):
			checked[norm(t[3:])] = true
		}
	}
	for _, o := range fd.Options {
		if checked[norm(o)] {
			cb.Selected = append(cb.Selected, o)
		} else {
			cb.Unselected = append(cb.Unselected, o)
		}
	}
	return cb
}
