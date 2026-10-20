package forge

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template/parse"
)

// A shape is what the walker learns about one value the template touches:
// a scalar (with a type guess), a map of fields, or a list of elements.
// The template's own parse tree is the only evidence; nothing is executed.
type shape struct {
	path   string // ".user.name", ".items[]", "" for the root
	kind   kind
	leaf   string // scalar guess: "", "int", "bool"
	fields map[string]*shape
	elem   *shape
	line   int // first line the value was referenced on
}

type kind int

const (
	scalar kind = iota
	mapKind
	listKind
)

func (s *shape) field(name string, line int) *shape {
	if s.kind != listKind {
		s.kind = mapKind
	}
	if s.fields == nil {
		s.fields = map[string]*shape{}
	}
	c, ok := s.fields[name]
	if !ok {
		c = &shape{path: s.path + "." + name, line: line}
		s.fields[name] = c
	}
	return c
}

func (s *shape) list() *shape {
	s.kind = listKind
	if s.elem == nil {
		s.elem = &shape{path: s.path + "[]", line: s.line}
	}
	return s.elem
}

func (s *shape) hint(leaf string) {
	if s != nil && s.kind == scalar && s.leaf == "" {
		s.leaf = leaf
	}
}

// untyped reports whether nothing is known about s yet.
func (s *shape) untyped() bool {
	return s != nil && s.kind == scalar && s.leaf == "" && s.fields == nil
}

// asMap marks an untyped shape as a map (sortedKeys, dynamic index).
func (s *shape) asMap() {
	if s.untyped() {
		s.kind = mapKind
	}
}

// asList marks an untyped shape as a list (join's argument).
func (s *shape) asList() {
	if s.untyped() {
		s.list()
	}
}

// value turns a shape into fixture JSON: maps for field sets, a one-element
// list for ranges, typed zero values for leaves.
func (s *shape) value() any {
	switch s.kind {
	case mapKind:
		m := make(map[string]any, len(s.fields))
		for k, c := range s.fields {
			m[k] = c.value()
		}
		return m
	case listKind:
		return []any{s.elem.value()}
	}
	switch s.leaf {
	case "int":
		return 0
	case "bool":
		return false
	}
	return ""
}

func (s *shape) walk(f func(*shape)) {
	f(s)
	keys := make([]string, 0, len(s.fields))
	for k := range s.fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.fields[k].walk(f)
	}
	if s.elem != nil {
		s.elem.walk(f)
	}
}

// Ref is one data path a template references, e.g. ".items[].name".
type Ref struct {
	Path string // dotted path; [] marks a range element
	Line int    // first line referencing it
	Kind string // "scalar", "map" or "list"
	Type string // scalar guess: "string", "int" or "bool"
}

// VarsResult is the walker's verdict on a template.
type VarsResult struct {
	Skeleton map[string]any // a fixture with every referenced path filled
	Refs     []Ref          // every path, depth-first, sorted by name
	Top      map[string]int // top-level field name -> first line
}

// Functions whose arguments are numbers, and those whose arguments are
// truth values. The walker uses them to guess leaf types in the skeleton.
var (
	intFuncs  = map[string]bool{"add": true, "sub": true, "mul": true, "div": true, "mod": true, "seq": true, "repeat": true, "indent": true, "nindent": true, "plural": true, "lt": true, "le": true, "gt": true, "ge": true}
	boolFuncs = map[string]bool{"not": true, "and": true, "or": true, "ternary": true}
)

// maxTemplateDepth bounds {{template}} recursion during the walk.
const maxTemplateDepth = 16

type walker struct {
	trees map[string]*parse.Tree
	tree  *parse.Tree
	root  *shape
	depth int
	seen  map[string]bool // template name + dot path already walked
}

// Vars parses src (without checking function names, so it works on
// templates that use funcs it does not know) and walks the parse tree to
// find every data path the template reads. Dot is tracked through range,
// with, variables, {{template}} calls and parenthesised pipelines, so
// {{range .items}}{{.name}}{{end}} yields .items[].name.
func Vars(src, left, right string) (*VarsResult, error) {
	t := parse.New("tmpl")
	t.Mode = parse.SkipFuncCheck
	trees := map[string]*parse.Tree{}
	if _, err := t.Parse(src, left, right, trees); err != nil {
		return nil, err
	}
	w := &walker{trees: trees, root: &shape{}, seen: map[string]bool{}}
	w.tree = t
	if t.Root != nil {
		w.list(t.Root, w.root, scope{"$": w.root})
	}
	res := &VarsResult{Top: map[string]int{}}
	if v, ok := w.root.value().(map[string]any); ok {
		res.Skeleton = v
	} else {
		res.Skeleton = map[string]any{}
	}
	w.root.walk(func(s *shape) {
		if s == w.root {
			return
		}
		k := [...]string{"scalar", "map", "list"}[s.kind]
		typ := ""
		if s.kind == scalar {
			typ = map[string]string{"": "string", "int": "int", "bool": "bool"}[s.leaf]
		}
		res.Refs = append(res.Refs, Ref{Path: s.path, Line: s.line, Kind: k, Type: typ})
	})
	for k, s := range w.root.fields {
		res.Top[k] = s.line
	}
	return res, nil
}

// scope maps variable names to shapes. Blocks copy it, as text/template
// scopes variables to the enclosing control structure.
type scope map[string]*shape

func (s scope) clone() scope {
	c := make(scope, len(s))
	for k, v := range s {
		c[k] = v
	}
	return c
}

func (w *walker) line(n parse.Node) int {
	loc, _ := w.tree.ErrorContext(n)
	// loc is "name:line:col"
	parts := strings.Split(loc, ":")
	if len(parts) >= 3 {
		if l, err := strconv.Atoi(parts[len(parts)-2]); err == nil {
			return l
		}
	}
	return 0
}

func (w *walker) list(l *parse.ListNode, dot *shape, vars scope) {
	if l == nil {
		return
	}
	for _, n := range l.Nodes {
		w.node(n, dot, vars)
	}
}

func (w *walker) node(n parse.Node, dot *shape, vars scope) {
	switch n := n.(type) {
	case *parse.ActionNode:
		w.pipe(n.Pipe, dot, vars, "", true)
	case *parse.IfNode:
		inner := vars.clone()
		w.pipe(n.Pipe, dot, inner, "bool", true)
		w.list(n.List, dot, inner)
		w.list(n.ElseList, dot, vars.clone())
	case *parse.WithNode:
		inner := vars.clone()
		s := w.pipe(n.Pipe, dot, inner, "", true)
		if s == nil {
			s = &shape{path: "?"}
		}
		w.list(n.List, s, inner)
		w.list(n.ElseList, dot, vars.clone())
	case *parse.RangeNode:
		inner := vars.clone()
		s := w.pipe(n.Pipe, dot, inner, "", false)
		var elem *shape
		if s != nil {
			if isSeq(n.Pipe) {
				elem = &shape{path: s.path + "[]", leaf: "int"}
			} else {
				elem = s.list()
			}
		} else {
			elem = &shape{path: "?[]"}
		}
		switch len(n.Pipe.Decl) {
		case 1:
			inner[n.Pipe.Decl[0].Ident[0]] = elem
		case 2:
			inner[n.Pipe.Decl[0].Ident[0]] = &shape{leaf: "int"}
			inner[n.Pipe.Decl[1].Ident[0]] = elem
		}
		w.list(n.List, elem, inner)
		w.list(n.ElseList, dot, vars.clone())
	case *parse.TemplateNode:
		arg := dot
		if n.Pipe != nil {
			arg = w.pipe(n.Pipe, dot, vars, "", false)
		} else {
			arg = &shape{path: "?"} // {{template "x"}} passes nil
		}
		w.call(n.Name, arg)
	}
}

// isSeq reports whether a range pipeline is a call to seq: its element is
// an int, not a fixture list.
func isSeq(p *parse.PipeNode) bool {
	if p == nil || len(p.Cmds) == 0 || len(p.Cmds[0].Args) == 0 {
		return false
	}
	id, ok := p.Cmds[0].Args[0].(*parse.IdentifierNode)
	return ok && id.Ident == "seq"
}

func (w *walker) call(name string, dot *shape) {
	t, ok := w.trees[name]
	if !ok || dot == nil || w.depth >= maxTemplateDepth {
		return
	}
	key := name + "\x00" + dot.path
	if w.seen[key] {
		return
	}
	w.seen[key] = true
	saved := w.tree
	w.tree = t
	w.depth++
	w.list(t.Root, dot, scope{"$": w.root})
	w.depth--
	w.tree = saved
}

// pipe walks a pipeline and returns the shape of its result when that is
// knowable (a bare field, variable or dot), else nil. When declare is set
// it binds {{$x := ...}} declarations in vars.
func (w *walker) pipe(p *parse.PipeNode, dot *shape, vars scope, hint string, declare bool) *shape {
	if p == nil {
		return nil
	}
	var prev *shape
	for i, c := range p.Cmds {
		prev = w.cmd(c, dot, vars, prev, i > 0)
	}
	if len(p.Cmds) == 1 && len(p.Cmds[0].Args) == 1 {
		prev.hint(hint)
	}
	if declare {
		for _, d := range p.Decl {
			if prev != nil {
				vars[d.Ident[0]] = prev
			} else {
				vars[d.Ident[0]] = &shape{path: "?"}
			}
		}
	}
	if len(p.Cmds) == 1 && len(p.Cmds[0].Args) == 1 {
		return prev
	}
	return nil
}

// cmd walks one command. prev is the previous command's result, which a
// pipeline passes as this command's final argument.
func (w *walker) cmd(c *parse.CommandNode, dot *shape, vars scope, prev *shape, piped bool) *shape {
	if len(c.Args) == 0 {
		return nil
	}
	fn := ""
	if id, ok := c.Args[0].(*parse.IdentifierNode); ok {
		fn = id.Ident
	}
	args := make([]*shape, len(c.Args))
	for i, a := range c.Args {
		args[i] = w.arg(a, dot, vars)
	}
	if piped && prev != nil {
		args = append(args, prev)
	}
	switch {
	case intFuncs[fn]:
		for _, a := range args[1:] {
			a.hint("int")
		}
	case boolFuncs[fn]:
		for _, a := range args[1:] {
			a.hint("bool")
		}
	case fn == "join" && len(args) >= 3:
		args[len(args)-1].asList()
	case fn == "sortedKeys" && len(args) >= 2:
		args[1].asMap()
	case fn == "index" && len(c.Args) >= 3:
		// {{index .m "k" "j"}} reads .m.k.j when the keys are literals;
		// with a computed key all we learn is that .m is a collection.
		s := args[1]
		for _, k := range c.Args[2:] {
			str, ok := k.(*parse.StringNode)
			if !ok || s == nil {
				if _, num := k.(*parse.NumberNode); num && s != nil && s.untyped() {
					s.list()
				} else if s != nil {
					s.asMap()
				}
				return nil
			}
			s = s.field(str.Text, w.line(k))
		}
		return s
	case fn == "len" || fn == "range":
		return nil
	}
	if fn != "" {
		return nil
	}
	return args[0]
}

func (w *walker) arg(n parse.Node, dot *shape, vars scope) *shape {
	switch n := n.(type) {
	case *parse.DotNode:
		return dot
	case *parse.FieldNode:
		return w.path(dot, n.Ident, w.line(n))
	case *parse.VariableNode:
		base, ok := vars[n.Ident[0]]
		if !ok {
			return nil
		}
		return w.path(base, n.Ident[1:], w.line(n))
	case *parse.ChainNode:
		var base *shape
		switch b := n.Node.(type) {
		case *parse.PipeNode:
			base = w.pipe(b, dot, vars, "", false)
		case *parse.FieldNode, *parse.VariableNode, *parse.DotNode:
			base = w.arg(b, dot, vars)
		}
		if base == nil {
			return nil
		}
		return w.path(base, n.Field, w.line(n))
	case *parse.PipeNode:
		return w.pipe(n, dot, vars, "", false)
	}
	return nil
}

func (w *walker) path(s *shape, idents []string, line int) *shape {
	for _, id := range idents {
		if s == nil {
			return nil
		}
		s = s.field(id, line)
	}
	return s
}

// String renders a Ref for `tmplforge vars -refs` style listings.
func (r Ref) String() string {
	if r.Type != "" {
		return fmt.Sprintf("%s\t%s\t%d", r.Path, r.Type, r.Line)
	}
	return fmt.Sprintf("%s\t%s\t%d", r.Path, r.Kind, r.Line)
}
