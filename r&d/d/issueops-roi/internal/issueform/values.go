package issueform

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Values is a parsed issue body. It accepts both the Go parser's typed values
// and the generic JSON decoded from issue-ops/parser (map[string]any, []any).
type Values map[string]any

// LoadValues reads parser JSON from a file ("-" for stdin is handled by callers).
func LoadValues(path string) (Values, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeValues(b)
}

// DecodeValues decodes parser JSON.
func DecodeValues(b []byte) (Values, error) {
	var v Values
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("issueform: decode parsed body: %w", err)
	}
	if v == nil {
		v = Values{}
	}
	return v, nil
}

// String returns a trimmed string value ("" if absent or not a string).
// A single-element dropdown is returned as its element.
func (v Values) String(key string) string {
	switch x := v[key].(type) {
	case string:
		return strings.TrimSpace(x)
	case []string:
		if len(x) == 1 {
			return strings.TrimSpace(x[0])
		}
	case []any:
		if len(x) == 1 {
			if s, ok := x[0].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

// List returns a dropdown value, or splits a comma-separated input.
func (v Values) List(key string) []string {
	switch x := v[key].(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return SplitCSV(x)
	}
	return nil
}

// Selected returns the selected labels of a checkboxes value.
func (v Values) Selected(key string) []string {
	switch x := v[key].(type) {
	case Checkboxes:
		return x.Selected
	case *Checkboxes:
		return x.Selected
	case map[string]any:
		var out []string
		if arr, ok := x["selected"].([]any); ok {
			for _, e := range arr {
				if s, ok := e.(string); ok {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

// Unselected returns the unselected labels of a checkboxes value.
func (v Values) Unselected(key string) []string {
	switch x := v[key].(type) {
	case Checkboxes:
		return x.Unselected
	case map[string]any:
		var out []string
		if arr, ok := x["unselected"].([]any); ok {
			for _, e := range arr {
				if s, ok := e.(string); ok {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

// IsSelected reports whether a checkbox label is ticked.
func (v Values) IsSelected(key, label string) bool {
	for _, s := range v.Selected(key) {
		if s == label {
			return true
		}
	}
	return false
}

// Int parses an integer value.
func (v Values) Int(key string) (int, error) {
	s := strings.ReplaceAll(strings.TrimPrefix(v.String(key), "$"), ",", "")
	if s == "" {
		return 0, fmt.Errorf("%s is empty", key)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number, got %q", key, v.String(key))
	}
	return n, nil
}

// Keys returns sorted keys.
func (v Values) Keys() []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Display renders any value as a single human-readable string.
func (v Values) Display(key string) string {
	switch v[key].(type) {
	case Checkboxes, *Checkboxes, map[string]any:
		sel := v.Selected(key)
		if len(sel) == 0 {
			return "(none selected)"
		}
		return strings.Join(sel, "; ")
	case []string, []any:
		return strings.Join(v.List(key), ", ")
	}
	return v.String(key)
}

// SplitCSV splits "a, b,c" into trimmed, non-empty items.
func SplitCSV(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
