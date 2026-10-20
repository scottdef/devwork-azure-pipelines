package funcs

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"
	"time"
)

func run(t *testing.T, src string, data any) (string, error) {
	t.Helper()
	tm, err := template.New("t").Funcs(New(DocOptions())).Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var b strings.Builder
	err = tm.Execute(&b, data)
	return b.String(), err
}

// TestEveryFuncDocumented keeps the table in docs honest: one Doc per
// function, no Doc for a function that is gone, and every example prints Want.
func TestEveryFuncDocumented(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range Docs {
		if seen[d.Name] {
			t.Errorf("duplicate doc for %s", d.Name)
		}
		seen[d.Name] = true
		if _, ok := Map()[d.Name]; !ok {
			t.Errorf("doc for unknown func %s", d.Name)
		}
		got, err := RenderDoc(d)
		if err != nil {
			t.Errorf("%s: %v", d.Name, err)
			continue
		}
		if got != d.Want {
			t.Errorf("%s: example\n got %q\nwant %q", d.Name, got, d.Want)
		}
	}
	for _, n := range Names() {
		if !seen[n] {
			t.Errorf("func %s has no Doc", n)
		}
	}
}

func TestFuncs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		data any
		want string
		err  string // substring of the error, "" for success
	}{
		{"dict ok", `{{ (dict "a" 1).a }}`, nil, "1", ""},
		{"dict empty", `{{ len (dict) }}`, nil, "0", ""},
		{"dict odd", `{{ dict "a" }}`, nil, "", "odd number of arguments"},
		{"dict key type", `{{ dict 1 2 }}`, nil, "", "want string"},
		{"list empty", `{{ len (list) }}`, nil, "0", ""},
		{"list mixed", `{{ list 1 "a" nil }}`, nil, "[1 a <nil>]", ""},
		{"sortedKeys", `{{ sortedKeys . }}`, map[string]bool{"z": true, "a": true, "m": true}, "[a m z]", ""},
		{"sortedKeys nil", `{{ len (sortedKeys .) }}`, nil, "0", ""},
		{"sortedKeys not map", `{{ sortedKeys 3 }}`, nil, "", "want map"},
		{"sortedKeys int keys", `{{ sortedKeys . }}`, map[int]int{1: 1}, "", "string keys"},
		{"seq 1", `{{ seq 3 }}`, nil, "[1 2 3]", ""},
		{"seq 0", `{{ seq 0 }}`, nil, "[]", ""},
		{"seq 2", `{{ seq 2 5 }}`, nil, "[2 3 4 5]", ""},
		{"seq down", `{{ seq 5 -2 0 }}`, nil, "[5 3 1]", ""},
		{"seq zero step", `{{ seq 1 0 5 }}`, nil, "", "step is zero"},
		{"seq no args", `{{ seq }}`, nil, "", "1 to 3 arguments"},
		{"seq huge", `{{ seq 2000000 }}`, nil, "", "more than"},
		{"seq float ok", `{{ seq . }}`, 3.0, "[1 2 3]", ""},
		{"seq float bad", `{{ seq . }}`, 2.5, "", "not an integer"},
		{"add", `{{ add 1 2 3 }}`, nil, "6", ""},
		{"add one", `{{ add 5 }}`, nil, "5", ""},
		{"add json float", `{{ add . 1 }}`, 41.0, "42", ""},
		{"add string", `{{ add "a" 1 }}`, nil, "", "not an integer"},
		{"sub", `{{ sub 1 3 }}`, nil, "-2", ""},
		{"mul", `{{ mul 2 3 4 }}`, nil, "24", ""},
		{"div", `{{ div -7 2 }}`, nil, "-3", ""},
		{"div zero", `{{ div 1 0 }}`, nil, "", "division by zero"},
		{"mod", `{{ mod 7 3 }}`, nil, "1", ""},
		{"mod negative", `{{ mod -7 3 }}`, nil, "-1", ""},
		{"mod zero", `{{ mod 1 0 }}`, nil, "", "division by zero"},
		{"mod pipe order", `{{ 3 | mod 7 }}`, nil, "1", ""}, // 7 mod 3: the pipe supplies the last argument
		{"default empty", `{{ default "d" "" }}`, nil, "d", ""},
		{"default zero", `{{ default 9 0 }}`, nil, "9", ""},
		{"default false", `{{ default "d" false }}`, nil, "d", ""},
		{"default set", `{{ default "d" "v" }}`, nil, "v", ""},
		{"default nil", `{{ default "d" .x }}`, map[string]any{}, "d", ""},
		{"default empty slice", `{{ default "d" . }}`, []int{}, "d", ""},
		{"default zero struct", `{{ default "d" . }}`, time.Time{}, "d", ""},
		{"coalesce", `{{ coalesce "" 0 nil "z" "y" }}`, nil, "z", ""},
		{"coalesce none", `{{ coalesce "" 0 }}`, nil, "<no value>", ""},
		{"ternary t", `{{ ternary 1 2 true }}`, nil, "1", ""},
		{"ternary f", `{{ ternary 1 2 false }}`, nil, "2", ""},
		{"join strings", `{{ join "-" . }}`, []string{"a", "b"}, "a-b", ""},
		{"join ints", `{{ join "," . }}`, []int{1, 2, 3}, "1,2,3", ""},
		{"join nil", `[{{ join "," . }}]`, nil, "[]", ""},
		{"join array", `{{ join "," . }}`, [2]int{4, 5}, "4,5", ""},
		{"join scalar", `{{ join "," 3 }}`, nil, "", "want slice"},
		{"split", `{{ split "," "a,b" }}`, nil, "[a b]", ""},
		{"split none", `{{ len (split "," "ab") }}`, nil, "1", ""},
		{"upper", `{{ upper "aé" }}`, nil, "AÉ", ""},
		{"lower", `{{ lower "AB" }}`, nil, "ab", ""},
		{"trim", `[{{ trim " \t a b \n" }}]`, nil, "[a b]", ""},
		{"replace", `{{ replace "ab" "X" "abab" }}`, nil, "XX", ""},
		{"replace pipe", `{{ "a.b" | replace "." "/" }}`, nil, "a/b", ""},
		{"repeat", `{{ repeat 0 "x" }}|{{ repeat 2 "x" }}`, nil, "|xx", ""},
		{"repeat negative", `{{ repeat -1 "x" }}`, nil, "", "out of range"},
		{"repeat huge", `{{ repeat 100000000 "xx" }}`, nil, "", "out of range"},
		{"indent", `{{ indent 3 "a\n\nb" }}`, nil, "   a\n\n   b", ""},
		{"indent zero", `{{ indent 0 "a" }}`, nil, "a", ""},
		{"indent negative", `{{ indent -1 "a" }}`, nil, "", "out of range"},
		{"nindent", `{{ nindent 1 "a" }}`, nil, "\n a", ""},
		{"nindent negative", `{{ nindent -1 "a" }}`, nil, "", "out of range"},
		{"plural 0", `{{ plural 0 "a" "b" }}`, nil, "b", ""},
		{"plural 1", `{{ plural 1 "a" "b" }}`, nil, "a", ""},
		{"plural float", `{{ plural . "a" "b" }}`, 1.0, "a", ""},
		{"plural bad", `{{ plural "x" "a" "b" }}`, nil, "", "not an integer"},
		{"toJSON", `{{ toJSON . }}`, map[string]any{"k": []string{"a<b"}}, `{"k":["a<b"]}`, ""},
		{"toJSON unsupported", `{{ toJSON . }}`, make(chan int), "", "unsupported type"},
		{"toPrettyJSON", `{{ toPrettyJSON . }}`, []int{1}, "[\n  1\n]", ""},
		{"fromJSON", `{{ (fromJSON "[1,2]") }}`, nil, "[1 2]", ""},
		{"fromJSON bad", `{{ fromJSON "{" }}`, nil, "", "unexpected end"},
		{"b64enc", `{{ b64enc "" }}|{{ b64enc "a" }}`, nil, "|YQ==", ""},
		{"b64dec", `{{ b64dec "YQ==" }}`, nil, "a", ""},
		{"b64dec bad", `{{ b64dec "!!" }}`, nil, "", "illegal base64"},
		{"sha256sum", `{{ sha256sum "" }}`, nil, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", ""},
		{"env set", `{{ env "APP_ENV" }}`, nil, "staging", ""},
		{"env unset", `[{{ env "NOPE" }}]`, nil, "[]", ""},
		{"file", `{{ file "hello.txt" }}`, nil, "hello, world", ""},
		{"file dotdot", `{{ file "../x" }}`, nil, "", "invalid path"},
		{"file absolute", `{{ file "/etc/passwd" }}`, nil, "", "invalid path"},
		{"file missing", `{{ file "nope" }}`, nil, "", "file does not exist"},
		{"shq plain", `{{ shq "a/b-c_1.txt" }}`, nil, "a/b-c_1.txt", ""},
		{"shq empty", `{{ shq "" }}`, nil, "''", ""},
		{"shq space", `{{ shq "a b" }}`, nil, "'a b'", ""},
		{"shq quote", `{{ shq "a'b" }}`, nil, `'a'\''b'`, ""},
		{"shq meta", `{{ shq "$(rm -rf /); ` + "`x`" + `" }}`, nil, "'$(rm -rf /); `x`'", ""},
		{"now", `{{ (now).Year }}`, nil, "2024", ""},
		{"dateFmt time", `{{ dateFmt "2006-01-02" now }}`, nil, "2024-03-05", ""},
		{"dateFmt unix", `{{ dateFmt "2006-01-02" 0 }}`, nil, "1970-01-01", ""},
		{"dateFmt rfc3339", `{{ dateFmt "Jan 2" "2021-12-25T10:00:00Z" }}`, nil, "Dec 25", ""},
		{"dateFmt bad string", `{{ dateFmt "Jan 2" "yesterday" }}`, nil, "", "cannot parse"},
		{"dateFmt bad type", `{{ dateFmt "Jan 2" true }}`, nil, "", "not an integer"},
		{"regexMatch", `{{ regexMatch "^a.c$" "abc" }}{{ regexMatch "^a.c$" "abcd" }}`, nil, "truefalse", ""},
		{"regexMatch bad", `{{ regexMatch "(" "x" }}`, nil, "", "missing closing"},
		{"regexReplaceAll", `{{ regexReplaceAll "(\\d+)" "#${1}" "a1b22" }}`, nil, "a#1b#22", ""},
		{"regexReplaceAll bad", `{{ regexReplaceAll "[" "x" "y" }}`, nil, "", "missing closing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := run(t, tc.src, tc.data)
			if tc.err != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got output %q", tc.err, got)
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("error %q does not contain %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrArgsIsWrapped(t *testing.T) {
	for _, src := range []string{`{{ dict "a" }}`, `{{ div 1 0 }}`, `{{ seq }}`, `{{ add "x" 1 }}`} {
		_, err := run(t, src, nil)
		if !errors.Is(err, ErrArgs) {
			t.Errorf("%s: error %v does not wrap ErrArgs", src, err)
		}
	}
}

func TestFileWithoutFS(t *testing.T) {
	tm := template.Must(template.New("t").Funcs(Map()).Parse(`{{ file "x" }}`))
	err := tm.Execute(&strings.Builder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no filesystem") {
		t.Fatalf("got %v", err)
	}
}

func TestFileRootedSubFS(t *testing.T) {
	// A MapFS rooted at a subdirectory must not see its parent.
	fsys := fstest.MapFS{"inner/a.txt": {Data: []byte("A")}, "secret.txt": {Data: []byte("S")}}
	sub, err := fsys.Sub("inner")
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{FS: sub})
	tm := template.Must(template.New("t").Funcs(m).Parse(`{{ file "a.txt" }}`))
	var b strings.Builder
	if err := tm.Execute(&b, nil); err != nil || b.String() != "A" {
		t.Fatalf("got %q, %v", b.String(), err)
	}
	tm = template.Must(template.New("t").Funcs(m).Parse(`{{ file "../secret.txt" }}`))
	if err := tm.Execute(&b, nil); err == nil {
		t.Fatal("escaped the root")
	}
}

func TestDefaultsEnvAndNow(t *testing.T) {
	t.Setenv("COOKBOOK_TEST_VAR", "from-process")
	tm := template.Must(template.New("t").Funcs(Map()).Parse(`{{ env "COOKBOOK_TEST_VAR" }}`))
	var b strings.Builder
	if err := tm.Execute(&b, nil); err != nil || b.String() != "from-process" {
		t.Fatalf("got %q, %v", b.String(), err)
	}
	before := time.Now().Add(-time.Second)
	if n := Map()["now"].(func() time.Time)(); n.Before(before) {
		t.Fatalf("now is %v", n)
	}
}

func TestNamesSortedAndComplete(t *testing.T) {
	want := strings.Fields(`dict list sortedKeys seq add sub mul mod default coalesce ternary join split
		upper lower trim replace repeat indent nindent plural toJSON toPrettyJSON fromJSON toYAML
		b64enc b64dec sha256sum env file shq now dateFmt regexMatch regexReplaceAll`)
	have := map[string]bool{}
	names := Names()
	for i, n := range names {
		have[n] = true
		if i > 0 && names[i-1] >= n {
			t.Errorf("Names not sorted at %s", n)
		}
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("missing %s", w)
		}
	}
}

func TestToYAML(t *testing.T) {
	type pod struct {
		Name  string            `json:"name"`
		Ports []int             `json:"ports,omitempty"`
		Env   map[string]string `json:"env,omitempty"`
	}
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"scalar string", "hello", "hello"},
		{"scalar number", 42, "42"},
		{"scalar bool", true, "true"},
		{"nil", nil, "null"},
		{"empty map", map[string]any{}, "{}"},
		{"empty slice", []int{}, "[]"},
		{"sorted keys", map[string]int{"b": 2, "a": 1}, "a: 1\nb: 2"},
		{"nested map", map[string]any{"a": map[string]any{"b": "c"}}, "a:\n  b: c"},
		{"slice of scalars", []string{"x", "z"}, "- x\n- z"},
		{"slice under key", map[string]any{"k": []int{1, 2}}, "k:\n  - 1\n  - 2"},
		{"slice of maps", []map[string]int{{"a": 1, "b": 2}, {"a": 3}}, "- a: 1\n  b: 2\n- a: 3"},
		{"slice of slices", [][]int{{1, 2}, {3}}, "- - 1\n  - 2\n- - 3"},
		{"empty inside", map[string]any{"m": map[string]any{}, "s": []any{}}, "m: {}\ns: []"},
		{"struct tags", pod{Name: "web", Ports: []int{80}, Env: map[string]string{"B": "2", "A": "1"}},
			"env:\n  A: \"1\"\n  B: \"2\"\nname: web\nports:\n  - 80"},
		{"quote number-like", []string{"8080", "1.5", "0755", "0x1f", "1e3", "1:30", "2024-03-05"},
			"- \"8080\"\n- \"1.5\"\n- \"0755\"\n- \"0x1f\"\n- \"1e3\"\n- \"1:30\"\n- \"2024-03-05\""},
		{"quote reserved", []string{"true", "No", "NULL", "off", "~", "y", "n", "yes"},
			"- \"true\"\n- \"No\"\n- \"NULL\"\n- \"off\"\n- \"~\"\n- \"y\"\n- \"n\"\n- \"yes\""},
		{"quote indicators", []string{"- a", "-", "?", ": x", "[a]", "{a}", "#c", "&a", "*a", "!t", "|", ">", "'q'", "\"q\"", "%d", "@a", "`b"},
			"- \"- a\"\n- \"-\"\n- \"?\"\n- \": x\"\n- \"[a]\"\n- \"{a}\"\n- \"#c\"\n- \"&a\"\n- \"*a\"\n- \"!t\"\n- \"|\"\n- \">\"\n- \"'q'\"\n- \"\\\"q\\\"\"\n- \"%d\"\n- \"@a\"\n- \"`b\""},
		{"plain dashes", []string{"--flag", "-x", "a-b", "a:b", "a#b", "http://x.y/z"},
			"- --flag\n- -x\n- a-b\n- a:b\n- a#b\n- http://x.y/z"},
		{"quote inner", []string{"a: b", "a #b", "end:", " lead", "trail ", ""},
			"- \"a: b\"\n- \"a #b\"\n- \"end:\"\n- \" lead\"\n- \"trail \"\n- \"\""},
		{"quote control", "a\tb\x00", "\"a\\tb\\u0000\""},
		{"unicode plain", "héllo ✓", "héllo ✓"},
		{"html not escaped", "a<b>&c", "a<b>&c"},
		{"key quoting", map[string]int{"on": 1, "8080": 2, "ok": 3, "a b": 4}, "\"8080\": 2\na b: 4\nok: 3\n\"on\": 1"},
		{"literal block", map[string]string{"s": "a\nb"}, "s: |-\n  a\n  b"},
		{"literal block keep one", map[string]string{"s": "a\nb\n"}, "s: |\n  a\n  b"},
		{"literal block keep all", map[string]string{"s": "a\n\n\n"}, "s: |+\n  a\n\n"},
		{"literal blank inside", map[string]string{"s": "a\n\nb"}, "s: |-\n  a\n\n  b"},
		{"literal in list", []string{"a\nb"}, "- |-\n  a\n  b"},
		{"literal top level", "a\nb", "|-\na\nb"},
		{"literal leading space falls back", map[string]string{"s": " a\nb"}, `s: " a\nb"`},
		{"literal ws-only line falls back", map[string]string{"s": "a\n \nb"}, `s: "a\n \nb"`},
		{"literal control falls back", map[string]string{"s": "a\r\nb"}, `s: "a\r\nb"`},
		{"deep", map[string]any{"a": []any{map[string]any{"b": []any{1, map[string]any{"c": nil}}}}},
			"a:\n  - b:\n      - 1\n      - c: null"},
		{"float", 1.5, "1.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToYAML(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestToYAMLErrors(t *testing.T) {
	if _, err := ToYAML(make(chan int)); err == nil {
		t.Error("channel: want error")
	}
	if _, err := ToYAML(func() {}); err == nil {
		t.Error("func: want error")
	}
}

func FuzzToYAMLNeverPanics(f *testing.F) {
	for _, s := range []string{"", "a", "a: b", "- x", "1e3", "a\nb", " \n", "\x00", "é"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if _, err := ToYAML(map[string]any{s: []any{s, map[string]any{s: s}}}); err != nil {
			t.Fatal(err)
		}
	})
}
