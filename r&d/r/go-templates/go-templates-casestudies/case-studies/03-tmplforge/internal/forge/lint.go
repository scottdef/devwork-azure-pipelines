package forge

import (
	"fmt"
	"sort"
)

// Severity of a lint finding.
type Severity string

// Severities.
const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
)

// Finding is one lint result, printed as file:line: severity: message.
type Finding struct {
	Line int
	Sev  Severity
	Msg  string
}

// Format prints f in the file:line: form editors and acme understand.
func (f Finding) Format(file string) string {
	if f.Sev == SevError {
		return fmt.Sprintf("%s:%d: %s", file, f.Line, f.Msg)
	}
	return fmt.Sprintf("%s:%d: %s: %s", file, f.Line, f.Sev, f.Msg)
}

// LintOptions configure Lint.
type LintOptions struct {
	Options                 // engine, delimiters and FuncMap as for Render
	Declared []string       // declared top-level variables; nil disables the cross-check
	Fixture  map[string]any // when set, top-level fields missing from it are warned about
}

// Lint parses src with the real engine and FuncMap (so unknown functions
// and html/template's stricter grammar are caught), then cross-checks the
// fields the template references against the declared variables.
// Errors stop the lint; warnings do not affect the exit status.
func Lint(src string, o LintOptions) []Finding {
	var out []Finding
	if _, err := Compile(src, o.Options); err != nil {
		line := ErrorLine(err)
		if line == 0 {
			line = 1
		}
		msg := StripErrorPrefix(err)
		return append(out, Finding{Line: line, Sev: SevError, Msg: msg})
	}
	v, err := Vars(src, o.Left, o.Right)
	if err != nil {
		return append(out, Finding{Line: ErrorLine(err), Sev: SevError, Msg: err.Error()})
	}
	names := make([]string, 0, len(v.Top))
	for k := range v.Top {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if v.Top[names[i]] != v.Top[names[j]] {
			return v.Top[names[i]] < v.Top[names[j]]
		}
		return names[i] < names[j]
	})
	if o.Declared != nil {
		decl := map[string]bool{}
		for _, d := range o.Declared {
			decl[d] = true
		}
		for _, n := range names {
			if !decl[n] {
				out = append(out, Finding{Line: v.Top[n], Sev: SevWarning, Msg: fmt.Sprintf(".%s referenced but not declared", n)})
			}
		}
		for _, d := range o.Declared {
			if _, ok := v.Top[d]; !ok {
				out = append(out, Finding{Line: 1, Sev: SevWarning, Msg: fmt.Sprintf("variable %q declared but never referenced", d)})
			}
		}
	}
	if o.Fixture != nil {
		for _, n := range names {
			if _, ok := o.Fixture[n]; !ok {
				out = append(out, Finding{Line: v.Top[n], Sev: SevWarning, Msg: fmt.Sprintf(".%s missing from fixture", n)})
			}
		}
	}
	return out
}

// HasError reports whether any finding is an error.
func HasError(fs []Finding) bool {
	for _, f := range fs {
		if f.Sev == SevError {
			return true
		}
	}
	return false
}
