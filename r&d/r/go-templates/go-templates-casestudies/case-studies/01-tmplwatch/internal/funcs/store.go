package funcs

import (
	"sort"
	"strings"
)

// MapStore is a Store over a plain map. It records missing keys; it is
// what tpl uses and what tests render against.
type MapStore struct {
	Data    map[string]string
	Missed  []string
	Touched []string // every key or prefix read, in call order
}

// Get implements Store.
func (m *MapStore) Get(k string) (string, bool) {
	m.Touched = append(m.Touched, k)
	v, ok := m.Data[k]
	return v, ok
}

// Missing implements Store.
func (m *MapStore) Missing(k string) { m.Missed = append(m.Missed, k) }

// List implements Store.
func (m *MapStore) List(prefix string, recursive bool) []KVPair {
	m.Touched = append(m.Touched, prefix+"/")
	return List(m.Data, prefix, recursive)
}

// List returns the pairs of data under prefix, sorted by key. Keys are
// relative to prefix. Without recursive, only immediate children that
// hold values are returned, as consul-template's ls does.
func List(data map[string]string, prefix string, recursive bool) []KVPair {
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	var out []KVPair
	for k, v := range data {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rel := k[len(prefix):]
		if rel == "" || (!recursive && strings.Contains(rel, "/")) {
			continue
		}
		out = append(out, KVPair{Key: rel, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
