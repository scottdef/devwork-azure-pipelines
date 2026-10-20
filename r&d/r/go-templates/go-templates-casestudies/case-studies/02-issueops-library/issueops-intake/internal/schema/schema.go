// Package schema checks a parsed issue (field id -> value) against a small
// rules file. It is deliberately not JSON Schema: five rules cover every
// field an issue form can produce, and each rule fits on one line of Go.
//
// Rules file:
//
//	{
//	  "additional": false,
//	  "fields": {
//	    "source-id": {"required": true, "pattern": "^[a-z0-9-]+$", "maxLength": 48},
//	    "phase":     {"required": true, "enum": ["1", "2", "3"]},
//	    "payload":   {"required": true, "type": "json", "jsonType": "object", "maxLength": 32768}
//	  }
//	}
package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"unicode/utf8"
)

// Rule constrains one field.
type Rule struct {
	Required  bool     `json:"required,omitempty"`
	Type      string   `json:"type,omitempty"`     // "string" (default), "json", "list"
	JSONType  string   `json:"jsonType,omitempty"` // for type json: object, array, any
	Pattern   string   `json:"pattern,omitempty"`  // RE2, applied to string values
	Enum      []string `json:"enum,omitempty"`
	MinLength int      `json:"minLength,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"` // bytes for json, runes otherwise
	Title     string   `json:"title,omitempty"`     // human name used in messages

	re *regexp.Regexp
}

// Schema is a rules file.
type Schema struct {
	Additional bool             `json:"additional"`
	Fields     map[string]*Rule `json:"fields"`
}

// Error is one violation. The JSON form is what bin/validate prints and
// what templates/comment-invalid.md.tmpl ranges over.
type Error struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Load reads a rules file and compiles its patterns.
func Load(path string) (*Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes and compiles a rules file.
func Parse(b []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	if len(s.Fields) == 0 {
		return nil, fmt.Errorf("schema: no fields")
	}
	for id, r := range s.Fields {
		if r == nil {
			return nil, fmt.Errorf("schema: field %q: null rule", id)
		}
		switch r.Type {
		case "", "string", "json", "list":
		default:
			return nil, fmt.Errorf("schema: field %q: unknown type %q", id, r.Type)
		}
		if r.Pattern != "" {
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return nil, fmt.Errorf("schema: field %q: %w", id, err)
			}
			r.re = re
		}
	}
	return &s, nil
}

// IDs returns the field ids in sorted order.
func (s *Schema) IDs() []string {
	ids := make([]string, 0, len(s.Fields))
	for id := range s.Fields {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Check validates doc and returns every violation, sorted by field.
func (s *Schema) Check(doc map[string]any) []Error {
	var errs []Error
	add := func(f, rule, format string, a ...any) {
		errs = append(errs, Error{Field: f, Rule: rule, Message: fmt.Sprintf(format, a...)})
	}
	for _, id := range s.IDs() {
		r := s.Fields[id]
		name := r.Title
		if name == "" {
			name = id
		}
		v, ok := doc[id]
		if !ok || v == nil || v == "" {
			if r.Required {
				add(id, "required", "%s is required", name)
			}
			continue
		}
		if r.Type == "list" {
			l, ok := v.([]any)
			if !ok {
				add(id, "type", "%s must be a list", name)
				continue
			}
			for _, e := range l {
				es, _ := e.(string)
				if len(r.Enum) > 0 && !in(es, r.Enum) {
					add(id, "enum", "%s: %q is not one of %v", name, es, r.Enum)
				}
			}
			continue
		}
		sv, ok := v.(string)
		if !ok {
			add(id, "type", "%s must be text", name)
			continue
		}
		n := utf8.RuneCountInString(sv)
		if r.Type == "json" {
			n = len(sv)
		}
		if r.MaxLength > 0 && n > r.MaxLength {
			add(id, "maxLength", "%s is %d long; the limit is %d", name, n, r.MaxLength)
		}
		if r.MinLength > 0 && n < r.MinLength {
			add(id, "minLength", "%s is %d long; the minimum is %d", name, n, r.MinLength)
		}
		if r.re != nil && !r.re.MatchString(sv) {
			add(id, "pattern", "%s %q does not match %s", name, clip(sv, 60), r.Pattern)
		}
		if len(r.Enum) > 0 && !in(sv, r.Enum) {
			add(id, "enum", "%s %q is not one of %v", name, clip(sv, 60), r.Enum)
		}
		if r.Type == "json" {
			var x any
			if err := json.Unmarshal([]byte(sv), &x); err != nil {
				add(id, "json", "%s is not valid JSON: %v", name, err)
				continue
			}
			switch r.JSONType {
			case "object":
				if _, ok := x.(map[string]any); !ok {
					add(id, "jsonType", "%s must be a JSON object", name)
				}
			case "array":
				if _, ok := x.([]any); !ok {
					add(id, "jsonType", "%s must be a JSON array", name)
				}
			}
		}
	}
	if !s.Additional {
		var extra []string
		for k := range doc {
			if _, ok := s.Fields[k]; !ok {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, k := range extra {
			add(k, "additional", "%s is not a known field", k)
		}
	}
	return errs
}

func in(s string, set []string) bool {
	for _, e := range set {
		if s == e {
			return true
		}
	}
	return false
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "..."
}
