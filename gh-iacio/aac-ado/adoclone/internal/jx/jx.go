// Package jx has helpers for editing decoded JSON (map[string]any) and for
// remapping source IDs to target IDs inside it.
package jx

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// M is a decoded JSON object.
type M = map[string]any

// Decode decodes a JSON object, keeping numbers as float64.
func Decode(b []byte) (M, error) {
	var m M
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Copy deep-copies a JSON value.
func Copy[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}

// Del removes keys from m.
func Del(m M, keys ...string) {
	for _, k := range keys {
		delete(m, k)
	}
}

// Get walks object keys and returns the value found, or nil.
func Get(v any, path ...string) any {
	for _, p := range path {
		m, ok := v.(M)
		if !ok {
			return nil
		}
		v = m[p]
	}
	return v
}

// Obj returns the object at path, or nil.
func Obj(v any, path ...string) M {
	m, _ := Get(v, path...).(M)
	return m
}

// Arr returns the array at path, or nil.
func Arr(v any, path ...string) []any {
	a, _ := Get(v, path...).([]any)
	return a
}

// Str returns the string at path, or "".
func Str(v any, path ...string) string {
	switch s := Get(v, path...).(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	}
	return ""
}

// Int returns the integer at path. Numeric strings count.
func Int(v any, path ...string) (int, bool) {
	switch n := Get(v, path...).(type) {
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}

// Set stores value at path, creating objects as needed.
func Set(m M, value any, path ...string) {
	for i, p := range path {
		if i == len(path)-1 {
			m[p] = value
			return
		}
		next, ok := m[p].(M)
		if !ok {
			next = M{}
			m[p] = next
		}
		m = next
	}
}

// Walk calls fn for every object member, depth first. When fn returns
// (replacement, true) the member is replaced and not descended into.
func Walk(v any, fn func(parent M, key string, val any) (any, bool)) {
	switch t := v.(type) {
	case M:
		for k, val := range t {
			if nv, ok := fn(t, k, val); ok {
				t[k] = nv
				continue
			}
			Walk(val, fn)
		}
	case []any:
		for _, e := range t {
			Walk(e, fn)
		}
	}
}

var guidRE = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// GUIDs returns every GUID in s.
func GUIDs(s string) []string { return guidRE.FindAllString(s, -1) }

// RemapGUIDs replaces every GUID in s that has an entry in m (keys compared
// case-insensitively) with its mapped value.
func RemapGUIDs(s string, m map[string]string) string {
	if len(m) == 0 {
		return s
	}
	return guidRE.ReplaceAllStringFunc(s, func(g string) string {
		if t, ok := m[strings.ToLower(g)]; ok {
			return t
		}
		return g
	})
}

// RemapJSON returns a copy of v with every mapped GUID replaced, anywhere in
// keys or string values.
func RemapJSON[T any](v T, m map[string]string) T {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return v
	}
	var out T
	if err := json.Unmarshal([]byte(RemapGUIDs(buf.String(), m)), &out); err != nil {
		return v
	}
	return out
}

// RebasePath rewrites an area or iteration path that starts with the project
// name from (for example "Src\\Team") so it starts with to ("Tgt\\Team").
func RebasePath(s, from, to string) string {
	switch {
	case strings.EqualFold(s, from):
		return to
	case len(s) > len(from) && strings.EqualFold(s[:len(from)], from) && s[len(from)] == '\\':
		return to + s[len(from):]
	}
	return s
}

// ReplaceFold replaces every case-insensitive occurrence of old with repl.
func ReplaceFold(s, old, repl string) string {
	if old == "" {
		return s
	}
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(old))
	return re.ReplaceAllLiteralString(s, repl)
}
