// Package issueform reads the body GitHub writes when someone submits an
// issue form: a "### Label" heading per field, then the value.
//
// The text is untrusted. Parse only splits it; every caller must still
// validate each field before using it.
package issueform

import (
	"strings"
	"unicode"
)

// Parse returns the fields of an issue-form body, keyed by the slug of each
// heading: "Target environment" becomes target_environment. Checked boxes
// are joined with commas. An unanswered field is the empty string.
func Parse(body string) map[string]string {
	fields := map[string]string{}
	var key string
	var lines []string
	fenced := false
	flush := func() {
		if key != "" {
			fields[key] = value(lines)
		}
		lines = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "### ") {
			flush()
			key = Slug(line[4:])
			continue
		}
		lines = append(lines, line)
	}
	flush()
	return fields
}

// Slug lower-cases s and replaces each run of other characters with one
// underscore.
func Slug(s string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if gap && b.Len() > 0 {
				b.WriteByte('_')
			}
			gap = false
			b.WriteRune(r)
		} else {
			gap = true
		}
	}
	return b.String()
}

func value(lines []string) string {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "_No response_" || text == "None" {
		return ""
	}
	// A checkbox group: keep the labels of the checked boxes.
	if strings.HasPrefix(text, "- [") {
		var checked []string
		for _, l := range strings.Split(text, "\n") {
			l = strings.TrimSpace(l)
			if rest, ok := strings.CutPrefix(l, "- [x] "); ok {
				checked = append(checked, strings.TrimSpace(rest))
			} else if rest, ok := strings.CutPrefix(l, "- [X] "); ok {
				checked = append(checked, strings.TrimSpace(rest))
			}
		}
		return strings.Join(checked, ",")
	}
	// A field rendered as a code block: drop the fences.
	if strings.HasPrefix(text, "```") && strings.HasSuffix(text, "```") && strings.Count(text, "\n") >= 1 {
		text = text[strings.Index(text, "\n")+1:]
		text = strings.TrimSuffix(text, "```")
		return strings.TrimSpace(text)
	}
	return text
}
