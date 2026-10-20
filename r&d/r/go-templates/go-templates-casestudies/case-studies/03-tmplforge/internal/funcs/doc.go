package funcs

//go:generate go run ./gendoc -o ../../docs/FUNCS.md

// Doc describes one template function for docs/FUNCS.md and the TUI's
// help. The table is the single source: a test fails if a function in All
// has no entry here, or an entry names no function.
type Doc struct {
	Name    string // function name as called from a template
	Sig     string // Go-ish signature, subject argument last
	Desc    string // one sentence
	Example string // template snippet
	Result  string // what Example renders to
}

// Docs lists every function in All, in the order FUNCS.md prints them.
var Docs = []Doc{
	{"dict", "dict k1 v1 [k2 v2 ...] map", "Build a map from alternating string keys and values.", `{{ (dict "a" 1).a }}`, "1"},
	{"list", "list v... []any", "Build a list from its arguments.", `{{ list 1 2 3 }}`, "[1 2 3]"},
	{"default", "default d v any", "Return v, or d when v is empty (zero, \"\", nil, empty list/map).", `{{ "" | default "x" }}`, "x"},
	{"coalesce", "coalesce v... any", "Return the first non-empty argument.", `{{ coalesce "" 0 "z" }}`, "z"},
	{"ternary", "ternary a b cond any", "Return a if cond, else b.", `{{ true | ternary "y" "n" }}`, "y"},
	{"join", "join sep list string", "Join list elements (stringified) with sep.", `{{ list "a" "b" | join "," }}`, "a,b"},
	{"split", "split sep s []string", "Split s on sep.", `{{ split "," "a,b" }}`, "[a b]"},
	{"upper", "upper s string", "Upper-case s.", `{{ "go" | upper }}`, "GO"},
	{"lower", "lower s string", "Lower-case s.", `{{ "GO" | lower }}`, "go"},
	{"trim", "trim s string", "Trim leading and trailing white space.", `{{ "  x " | trim }}`, "x"},
	{"replace", "replace old new s string", "Replace every old with new in s.", `{{ "a-b" | replace "-" "_" }}`, "a_b"},
	{"repeat", "repeat n s string", "Repeat s n times (result capped at 1 MiB).", `{{ "ab" | repeat 2 }}`, "abab"},
	{"indent", "indent n s string", "Indent every non-empty line of s by n spaces.", `{{ "a" | indent 2 }}`, "  a"},
	{"nindent", "nindent n s string", "Like indent, with a leading newline.", `{{ "a" | nindent 2 }}`, "\\n  a"},
	{"plural", "plural one many n string", "Choose one if n == 1, else many.", `{{ 2 | plural "file" "files" }}`, "files"},
	{"add", "add a b int", "Integer sum.", `{{ add 1 2 }}`, "3"},
	{"sub", "sub a b int", "Integer difference a-b.", `{{ sub 5 2 }}`, "3"},
	{"mul", "mul a b int", "Integer product.", `{{ mul 2 3 }}`, "6"},
	{"div", "div a b int", "Integer quotient; error on zero.", `{{ div 7 2 }}`, "3"},
	{"mod", "mod a b int", "Integer remainder; error on zero.", `{{ mod 7 2 }}`, "1"},
	{"seq", "seq [lo] hi iter.Seq[int]", "Lazy sequence lo..hi (lo defaults to 1) for range; one range variable only (an iter.Seq, so a huge range costs time, not memory).", `{{ range seq 3 }}{{ . }}{{ end }}`, "123"},
	{"sortedKeys", "sortedKeys m []string", "Map keys in sorted order: deterministic map iteration.", `{{ sortedKeys (dict "b" 1 "a" 2) }}`, "[a b]"},
	{"toJSON", "toJSON v string", "Compact JSON encoding.", `{{ dict "a" 1 | toJSON }}`, `{"a":1}`},
	{"toPrettyJSON", "toPrettyJSON v string", "Two-space-indented JSON encoding.", `{{ list 1 | toPrettyJSON }}`, "[\\n  1\\n]"},
	{"fromJSON", "fromJSON s any", "Decode JSON; integral numbers become int.", `{{ (fromJSON "{\"n\":2}").n }}`, "2"},
	{"toYAML", "toYAML v string", "Block-style YAML with sorted keys (stdlib emitter).", `{{ dict "a" 1 | toYAML }}`, "a: 1"},
	{"b64enc", "b64enc s string", "Standard base64 encoding.", `{{ "hi" | b64enc }}`, "aGk="},
	{"b64dec", "b64dec s string", "Standard base64 decoding.", `{{ "aGk=" | b64dec }}`, "hi"},
	{"sha256sum", "sha256sum s string", "Hex SHA-256 of s.", `{{ "" | sha256sum }}`, "e3b0c442...b855"},
	{"now", "now time.Time", "Current UTC time; honours SOURCE_DATE_EPOCH for reproducible output.", `{{ now.Year }}`, "2026"},
	{"dateFmt", "dateFmt layout t string", "Format a time, RFC 3339 / YYYY-MM-DD string or Unix seconds with a Go layout.", `{{ dateFmt "Jan 2" "2026-10-19" }}`, "Oct 19"},
	{"regexMatch", "regexMatch re s bool", "RE2 match (pattern capped at 1 KiB).", `{{ regexMatch "^v[0-9]" "v1" }}`, "true"},
	{"regexReplaceAll", "regexReplaceAll re repl s string", "RE2 replace-all; repl may use $1.", `{{ regexReplaceAll "o+" "0" "foo" }}`, "f0"},
	{"shq", "shq s string", "Single-quote s for a POSIX shell.", `{{ "it's" | shq }}`, `'it'\''s'`},
	{"env", "env name string", "Environment variable. **Not in Safe()**.", `{{ env "HOME" }}`, "/home/glenda"},
	{"file", "file path string", "Read a file (capped at 1 MiB). **Not in Safe()**.", `{{ file "VERSION" }}`, "1.0.0"},
}
