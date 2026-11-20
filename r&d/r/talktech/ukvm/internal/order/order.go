// Package order turns written or spoken orders into validated tool calls.
//
// There is exactly one action vocabulary. Written orders are parsed by the
// small grammar in Parse. Spoken orders arrive as the same Call values from
// the Needle model, which is handed the schema produced by ToolsJSON. Either
// way every call must pass Validate against the site Policy before the gate
// lets it near a target.
//
// Text that the system reads (screens, PDFs, command output) is never passed
// to Parse. Orders come from the principal; observations are data.
package order

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ukvm/internal/hid"
)

// Call is one tool invocation, in the JSON shape Needle emits.
type Call struct {
	Name string         `json:"name"`
	Args map[string]any `json:"arguments,omitempty"`
}

// Str returns a string argument, or "" when absent or not a string.
func (c Call) Str(k string) string {
	s, _ := c.Args[k].(string)
	return s
}

// Int returns an integer argument however JSON or a model spelled it.
func (c Call) Int(k string) (int, bool) {
	switch v := c.Args[k].(type) {
	case int:
		return v, true
	case float64:
		if v == math.Trunc(v) {
			return int(v), true
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	}
	return 0, false
}

func (c Call) String() string {
	if len(c.Args) == 0 {
		return c.Name
	}
	b, _ := json.Marshal(c.Args)
	return c.Name + string(b)
}

// Mode says whether a permitted call may run unattended.
type Mode string

const (
	Auto    Mode = "auto"    // runs at high confidence without asking
	Confirm Mode = "confirm" // always shown to the principal first
)

// Policy is the closed vocabulary for one site. Anything not listed here
// cannot be ordered, whoever or whatever asks.
type Policy struct {
	Targets  []string        `json:"targets"`
	Commands map[string]Mode `json:"commands"`
	Chords   map[string]Mode `json:"chords"`
}

var (
	keyRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
)

// Validate checks one call against the policy and reports how it may run.
func Validate(c Call, p Policy) (Mode, error) {
	switch c.Name {
	case "select_target":
		t := c.Str("target")
		for _, x := range p.Targets {
			if x == t {
				return Auto, nil
			}
		}
		return "", fmt.Errorf("unknown target %q", t)
	case "run_command":
		cmd := c.Str("command")
		m, ok := p.Commands[cmd]
		if !ok {
			return "", fmt.Errorf("command %q is not on the allowlist", cmd)
		}
		if _, err := hid.Type(cmd); err != nil {
			return "", err
		}
		return checkMode(m)
	case "press_keys":
		ch := strings.ToLower(c.Str("chord"))
		m, ok := p.Chords[ch]
		if !ok {
			return "", fmt.Errorf("chord %q is not on the allowlist", ch)
		}
		if _, err := hid.ParseChord(ch); err != nil {
			return "", err
		}
		return checkMode(m)
	case "type_text":
		s := c.Str("text")
		if s == "" || len(s) > 500 {
			return "", errors.New("type_text needs 1 to 500 characters")
		}
		if _, err := hid.Type(s); err != nil {
			return "", err
		}
		return Confirm, nil
	case "click":
		x, okx := c.Int("x")
		y, oky := c.Int("y")
		if !okx || !oky || x < 0 || x > 1000 || y < 0 || y > 1000 {
			return "", errors.New("click needs x and y between 0 and 1000")
		}
		return Confirm, nil
	case "click_text":
		if s := c.Str("text"); s == "" || len(s) > 80 {
			return "", errors.New("click_text needs 1 to 80 characters")
		}
		return Confirm, nil
	case "read_screen", "status":
		return Auto, nil
	case "read_pdf":
		if p := c.Str("path"); p == "" || strings.ContainsRune(p, 0) {
			return "", errors.New("read_pdf needs a path")
		}
		return Auto, nil
	case "note":
		if !keyRE.MatchString(c.Str("key")) {
			return "", fmt.Errorf("bad note key %q", c.Str("key"))
		}
		if v := c.Str("value"); v == "" || len(v) > 500 {
			return "", errors.New("note needs a value of 1 to 500 characters")
		}
		return Auto, nil
	case "recall":
		if !keyRE.MatchString(c.Str("key")) {
			return "", fmt.Errorf("bad note key %q", c.Str("key"))
		}
		return Auto, nil
	case "runbook_run":
		if !nameRE.MatchString(c.Str("name")) {
			return "", fmt.Errorf("bad runbook name %q", c.Str("name"))
		}
		return Confirm, nil
	}
	return "", fmt.Errorf("unknown tool %q", c.Name)
}

func checkMode(m Mode) (Mode, error) {
	if m != Auto && m != Confirm {
		return "", fmt.Errorf("policy mode %q is neither auto nor confirm", m)
	}
	return m, nil
}

type token struct {
	s      string
	quoted bool
	sep    bool
}

func lex(text string) ([]token, error) {
	var toks []token
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, token{s: cur.String()})
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case inQuote:
			if r == '"' {
				toks = append(toks, token{s: cur.String(), quoted: true})
				cur.Reset()
				inQuote = false
			} else {
				cur.WriteRune(r)
			}
		case r == '"':
			flush()
			inQuote = true
		case r == ';' || r == '\n':
			flush()
			toks = append(toks, token{sep: true})
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return toks, nil
}

// Parse reads the written-order language. Statements are separated by ';',
// a newline, or the word "then":
//
//	target server3; press ctrl+alt+t; run uptime
//	type "hello world"        click 512 300        click "Log in"
//	read screen               read pdf manuals/bmc.pdf
//	note bios.server3 = 2.19  recall bios.server3
//	runbook run nightly-check status    confirm    cancel
func Parse(text string) ([]Call, error) {
	toks, err := lex(text)
	if err != nil {
		return nil, err
	}
	var calls []Call
	var stmt []token
	end := func() error {
		for len(stmt) > 0 && !stmt[0].quoted && isFiller(stmt[0].s) {
			stmt = stmt[1:]
		}
		if len(stmt) == 0 {
			return nil
		}
		c, err := parseStmt(stmt)
		stmt = nil
		if err != nil {
			return err
		}
		calls = append(calls, c)
		return nil
	}
	for _, t := range toks {
		if t.sep || (!t.quoted && strings.EqualFold(t.s, "then")) {
			if err := end(); err != nil {
				return nil, err
			}
			continue
		}
		stmt = append(stmt, t)
	}
	if err := end(); err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		return nil, errors.New("no order given")
	}
	return calls, nil
}

func isFiller(s string) bool {
	switch strings.ToLower(s) {
	case "and", "please":
		return true
	}
	return false
}

func words(ts []token) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.s
	}
	return out
}

func parseStmt(ts []token) (Call, error) {
	verb := strings.ToLower(ts[0].s)
	rest := ts[1:]
	need := func(n int) error {
		if len(rest) < n {
			return fmt.Errorf("%q needs more words", verb)
		}
		return nil
	}
	call := func(name string, kv ...any) (Call, error) {
		c := Call{Name: name}
		if len(kv) > 0 {
			c.Args = map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				c.Args[kv[i].(string)] = kv[i+1]
			}
		}
		return c, nil
	}
	if ts[0].quoted {
		return Call{}, fmt.Errorf("an order cannot start with quoted text %q", ts[0].s)
	}
	switch verb {
	case "target", "select", "use", "switch":
		if verb == "switch" && len(rest) > 0 && strings.EqualFold(rest[0].s, "to") {
			rest = rest[1:]
		}
		if err := need(1); err != nil {
			return Call{}, err
		}
		return call("select_target", "target", strings.ToLower(rest[0].s))
	case "run":
		if err := need(1); err != nil {
			return Call{}, err
		}
		return call("run_command", "command", strings.Join(words(rest), " "))
	case "press":
		if err := need(1); err != nil {
			return Call{}, err
		}
		return call("press_keys", "chord", strings.ToLower(strings.Join(words(rest), "+")))
	case "type":
		if err := need(1); err != nil {
			return Call{}, err
		}
		return call("type_text", "text", strings.Join(words(rest), " "))
	case "click":
		if err := need(1); err != nil {
			return Call{}, err
		}
		if len(rest) == 2 && !rest[0].quoted && !rest[1].quoted {
			x, ex := strconv.Atoi(rest[0].s)
			y, ey := strconv.Atoi(rest[1].s)
			if ex == nil && ey == nil {
				return call("click", "x", x, "y", y)
			}
		}
		if !rest[0].quoted && strings.EqualFold(rest[0].s, "on") && len(rest) > 1 {
			rest = rest[1:]
		}
		return call("click_text", "text", strings.Join(words(rest), " "))
	case "read":
		if err := need(1); err != nil {
			return Call{}, err
		}
		switch strings.ToLower(rest[0].s) {
		case "screen":
			return call("read_screen")
		case "pdf":
			if len(rest) < 2 {
				return Call{}, errors.New("read pdf needs a path")
			}
			return call("read_pdf", "path", strings.Join(words(rest[1:]), " "))
		}
		return Call{}, fmt.Errorf("cannot read %q; try \"read screen\" or \"read pdf PATH\"", rest[0].s)
	case "screen":
		return call("read_screen")
	case "status", "report":
		return call("status")
	case "note":
		if len(rest) < 3 || rest[1].s != "=" {
			return Call{}, errors.New("use: note KEY = VALUE")
		}
		return call("note", "key", strings.ToLower(rest[0].s), "value", strings.Join(words(rest[2:]), " "))
	case "recall":
		if err := need(1); err != nil {
			return Call{}, err
		}
		return call("recall", "key", strings.ToLower(rest[0].s))
	case "runbook":
		if len(rest) > 1 && strings.EqualFold(rest[0].s, "run") {
			rest = rest[1:]
		}
		if err := need(1); err != nil {
			return Call{}, err
		}
		return call("runbook_run", "name", strings.ToLower(rest[0].s))
	case "confirm", "yes":
		return call("confirm")
	case "cancel", "abort":
		return call("cancel")
	}
	return Call{}, fmt.Errorf("unknown order %q", ts[0].s)
}

// ToolsJSON renders the policy as the tool schema given to Needle, so the
// model's decode grammar and the validator can never drift apart.
func ToolsJSON(p Policy) ([]byte, error) {
	keys := func(m map[string]Mode) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	str := map[string]any{"type": "string"}
	enum := func(v []string) map[string]any { return map[string]any{"type": "string", "enum": v} }
	num := map[string]any{"type": "integer", "minimum": 0, "maximum": 1000}
	tool := func(name, desc string, props map[string]any, required ...string) map[string]any {
		params := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			params["required"] = required
		}
		return map[string]any{"name": name, "description": desc, "parameters": params}
	}
	targets := append([]string(nil), p.Targets...)
	sort.Strings(targets)
	tools := []map[string]any{
		tool("select_target", "Switch attention to one computer", map[string]any{"target": enum(targets)}, "target"),
		tool("run_command", "Run an approved command on the selected computer", map[string]any{"command": enum(keys(p.Commands))}, "command"),
		tool("press_keys", "Press a key or key chord on the selected computer", map[string]any{"chord": enum(keys(p.Chords))}, "chord"),
		tool("type_text", "Type dictated text on the selected computer", map[string]any{"text": str}, "text"),
		tool("click", "Click at a screen position, 0 to 1000 on each axis", map[string]any{"x": num, "y": num}, "x", "y"),
		tool("click_text", "Click on visible text such as a button label", map[string]any{"text": str}, "text"),
		tool("read_screen", "Read the text shown on the selected computer's display", map[string]any{}),
		tool("read_pdf", "Convert a PDF document to text", map[string]any{"path": str}, "path"),
		tool("status", "Report on devices, jobs and anything awaiting confirmation", map[string]any{}),
		tool("note", "Record a fact to track", map[string]any{"key": str, "value": str}, "key", "value"),
		tool("recall", "Recall a recorded fact", map[string]any{"key": str}, "key"),
		tool("runbook_run", "Run a saved runbook", map[string]any{"name": str}, "name"),
		tool("confirm", "Approve the order awaiting confirmation", map[string]any{}),
		tool("cancel", "Cancel the order awaiting confirmation", map[string]any{}),
	}
	return json.MarshalIndent(tools, "", "  ")
}
