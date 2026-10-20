package item

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/issueops-library/internal/funcs"
)

// Header is the optional first line of a template file:
//
//	#!tmpl ext=yml delims=[[,]] missingkey=zero
//
// It travels with the template so a new template needs no code change.
// The line is removed before parsing.
type Header struct {
	Ext        string // artifact extension; default "txt"
	Left       string // left delimiter; default "{{"
	Right      string // right delimiter; default "}}"
	MissingKey string // text/template missingkey option; default "zero"
}

// ParseHeader splits src into header and body.
func ParseHeader(src string) (Header, string, error) {
	h := Header{Ext: "txt", Left: "{{", Right: "}}", MissingKey: "zero"}
	first, rest, _ := strings.Cut(src, "\n")
	if !strings.HasPrefix(first, "#!tmpl") {
		return h, src, nil
	}
	for _, kv := range strings.Fields(strings.TrimPrefix(first, "#!tmpl")) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return h, "", fmt.Errorf("template header: %q is not key=value", kv)
		}
		switch k {
		case "ext":
			if !reExt.MatchString(v) {
				return h, "", fmt.Errorf("template header: bad ext %q", v)
			}
			h.Ext = v
		case "delims":
			l, r, ok := strings.Cut(v, ",")
			if !ok || l == "" || r == "" {
				return h, "", fmt.Errorf("template header: delims must be left,right")
			}
			h.Left, h.Right = l, r
		case "missingkey":
			switch v {
			case "zero", "error", "default", "invalid":
			default:
				return h, "", fmt.Errorf("template header: bad missingkey %q", v)
			}
			h.MissingKey = v
		default:
			return h, "", fmt.Errorf("template header: unknown key %q", k)
		}
	}
	return h, rest, nil
}

// LoadTemplate reads and parses a template file with its header.
func LoadTemplate(path string) (*template.Template, Header, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, Header{}, err
	}
	return ParseTemplate(path, string(b))
}

// ParseTemplate parses src, honouring its header.
func ParseTemplate(name, src string) (*template.Template, Header, error) {
	h, body, err := ParseHeader(src)
	if err != nil {
		return nil, h, fmt.Errorf("%s: %w", name, err)
	}
	t, err := template.New(name).
		Delims(h.Left, h.Right).
		Funcs(funcs.Map()).
		Option("missingkey=" + h.MissingKey).
		Parse(body)
	if err != nil {
		return nil, h, err
	}
	return t, h, nil
}

// reExt allows "yml" and "nomad.hcl" but nothing that could leave the
// item directory or hide a file.
var reExt = regexp.MustCompile(`^[a-z0-9]{1,12}(\.[a-z0-9]{1,12})?$`)

// noValue is what text/template prints for a missing map key. Seeing it
// in an artifact means a template used a key the payload did not set and
// forgot a default; publishing that would ship a broken config.
const noValue = "<no value>"

// Execute runs t and refuses output containing "<no value>".
func Execute(t *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	if i := bytes.Index(buf.Bytes(), []byte(noValue)); i >= 0 {
		line := bytes.Count(buf.Bytes()[:i], []byte("\n")) + 1
		return nil, fmt.Errorf("%s: output line %d prints %s: a payload key is missing and the template has no default", t.Name(), line, noValue)
	}
	return buf.Bytes(), nil
}
