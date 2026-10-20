package funcs

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// toYAML emits v as block-style YAML 1.2. The standard library has no YAML
// package, and the subset needed to print a fixture (maps, lists, scalars)
// is small enough that a dependency would cost more than it saves.
// Map keys are sorted so output is deterministic.
func toYAML(v any) (string, error) {
	var b strings.Builder
	if err := emitYAML(&b, reflect.ValueOf(v), 0, 0, false); err != nil {
		return "", fmt.Errorf("toYAML: %w", err)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

const maxYAMLDepth = 64

func deref(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// emitYAML writes v at the given indent. Collections write their own
// trailing newline; the caller has already written any "key:" or "- ".
// inline means the current line already holds "- ", so a map's first key
// goes on it without padding.
func emitYAML(b *strings.Builder, v reflect.Value, ind, depth int, inline bool) error {
	if depth > maxYAMLDepth {
		return fmt.Errorf("nesting deeper than %d", maxYAMLDepth)
	}
	v = deref(v)
	pad := strings.Repeat("  ", ind)
	switch {
	case !v.IsValid():
		b.WriteString("null\n")
	case v.Kind() == reflect.Map:
		if v.Len() == 0 {
			b.WriteString("{}\n")
			return nil
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		for i, k := range keys {
			if i > 0 || !inline {
				b.WriteString(pad)
			}
			b.WriteString(yamlScalar(fmt.Sprint(k.Interface())))
			b.WriteString(":")
			if err := emitChild(b, v.MapIndex(k), ind, depth); err != nil {
				return err
			}
		}
	case (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && v.Type().Elem().Kind() != reflect.Uint8:
		if v.Len() == 0 {
			b.WriteString("[]\n")
			return nil
		}
		for i := 0; i < v.Len(); i++ {
			b.WriteString(pad)
			b.WriteString("- ")
			e := deref(v.Index(i))
			if isCollection(e) && e.Len() > 0 {
				if e.Kind() == reflect.Map {
					// first key shares the dash line; the rest indent one level
					if err := emitYAML(b, e, ind+1, depth+1, true); err != nil {
						return err
					}
					continue
				}
				b.WriteString("\n")
				if err := emitYAML(b, e, ind+1, depth+1, false); err != nil {
					return err
				}
				continue
			}
			if err := emitYAML(b, e, ind+1, depth+1, false); err != nil {
				return err
			}
		}
	default:
		b.WriteString(scalarOf(v))
		b.WriteString("\n")
	}
	return nil
}

func emitChild(b *strings.Builder, v reflect.Value, ind, depth int) error {
	e := deref(v)
	if isCollection(e) && e.Len() > 0 {
		b.WriteString("\n")
		return emitYAML(b, e, ind+1, depth+1, false)
	}
	b.WriteString(" ")
	return emitYAML(b, e, ind+1, depth+1, false)
}

func isCollection(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Map:
		return true
	case reflect.Slice, reflect.Array:
		return v.Type().Elem().Kind() != reflect.Uint8
	}
	return false
}

func scalarOf(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case reflect.String:
		return yamlScalar(v.String())
	}
	return yamlScalar(fmt.Sprint(v.Interface()))
}

// yamlScalar returns s bare when YAML would read it back as the same
// string, otherwise double-quoted with Go escapes (a valid YAML subset).
func yamlScalar(s string) string {
	if needsQuote(s) {
		return strconv.Quote(s)
	}
	return s
}

func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~", "y", "n":
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	if strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@` ") {
		return true
	}
	if strings.HasSuffix(s, " ") || strings.HasSuffix(s, ":") {
		return true
	}
	if strings.Contains(s, ": ") || strings.Contains(s, " #") {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
