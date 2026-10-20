package funcs

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"text/template"
)

func exec(t *testing.T, fm template.FuncMap, src string, data any) (string, error) {
	t.Helper()
	tt, err := template.New("t").Funcs(fm).Parse(src)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	err = tt.Execute(&b, data)
	return b.String(), err
}

func TestFuncs(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1760832000") // 2025-10-19T00:00:00Z
	t.Setenv("TMPLFORGE_TEST_ENV", "glenda")
	dir := t.TempDir()
	fp := filepath.Join(dir, "v")
	os.WriteFile(fp, []byte("1.0.0"), 0o644)
	data := map[string]any{
		"empty": "", "zero": 0, "name": "rob", "n": 2, "one": 1,
		"m": map[string]any{"b": 1, "a": 2}, "l": []any{"x", "y"}, "file": fp,
		"nested": map[string]any{"s": []any{1, map[string]any{"k": "v"}}, "t": true},
	}
	tests := []struct {
		name, src, want string
	}{
		{"dict", `{{ (dict "a" 1 "b" "two").b }}`, "two"},
		{"list", `{{ list 1 "a" true }}`, "[1 a true]"},
		{"list-empty", `{{ len (list) }}`, "0"},
		{"default-empty", `{{ .empty | default "x" }}`, "x"},
		{"default-zero", `{{ .zero | default 9 }}`, "9"},
		{"default-missing", `{{ .missing | default "m" }}`, "m"},
		{"default-set", `{{ .name | default "x" }}`, "rob"},
		{"default-empty-list", `{{ list | default "none" }}`, "none"},
		{"coalesce", `{{ coalesce .empty .zero .name "z" }}`, "rob"},
		{"coalesce-none", `{{ coalesce .empty .zero }}`, "<no value>"},
		{"ternary-true", `{{ true | ternary "y" "n" }}`, "y"},
		{"ternary-false", `{{ ternary "y" "n" false }}`, "n"},
		{"join", `{{ .l | join "," }}`, "x,y"},
		{"join-strings", `{{ split "," "a,b,c" | join "+" }}`, "a+b+c"},
		{"join-ints", `{{ list 1 2 | join "-" }}`, "1-2"},
		{"split", `{{ index (split ":" "a:b") 1 }}`, "b"},
		{"upper", `{{ upper "go" }}`, "GO"},
		{"lower", `{{ lower "GO" }}`, "go"},
		{"trim", `[{{ trim "  x  " }}]`, "[x]"},
		{"replace", `{{ "a.b.c" | replace "." "_" }}`, "a_b_c"},
		{"repeat", `{{ "ab" | repeat 3 }}`, "ababab"},
		{"indent", `{{ "a\nb\n\nc" | indent 2 }}`, "  a\n  b\n\n  c"},
		{"nindent", `x:{{ "a" | nindent 4 }}`, "x:\n    a"},
		{"plural-one", `{{ .one | plural "file" "files" }}`, "file"},
		{"plural-many", `{{ .n | plural "file" "files" }}`, "files"},
		{"add", `{{ add 1 2 }}`, "3"},
		{"add-string", `{{ add "4" 1 }}`, "5"},
		{"sub", `{{ sub 5 7 }}`, "-2"},
		{"mul", `{{ mul .n 21 }}`, "42"},
		{"div", `{{ div 7 2 }}`, "3"},
		{"mod", `{{ mod 7 3 }}`, "1"},
		{"seq", `{{ range seq 3 }}{{ . }}{{ end }}`, "123"},
		{"seq-range", `{{ range seq 3 5 }}{{ . }}{{ end }}`, "345"},
		{"seq-empty", `{{ range seq 0 }}x{{ else }}none{{ end }}`, "none"},
		{"seq-var", `{{ range $v := seq 2 }}{{ $v }} {{ end }}`, "1 2 "},
		{"sortedKeys", `{{ sortedKeys .m }}`, "[a b]"},
		{"toJSON", `{{ toJSON .m }}`, `{"a":2,"b":1}`},
		{"toPrettyJSON", `{{ toPrettyJSON (list 1) }}`, "[\n  1\n]"},
		{"fromJSON", `{{ (fromJSON "{\"n\": 8080}").n }}`, "8080"},
		{"fromJSON-gt", `{{ if gt (fromJSON "{\"n\": 8080}").n 1024 }}hi{{ end }}`, "hi"},
		{"toYAML", `{{ toYAML .nested }}`, "s:\n  - 1\n  - k: v\nt: true"},
		{"toYAML-scalar-quote", `{{ toYAML (dict "a" "yes" "b" "1.5" "c" "") }}`, "a: \"yes\"\nb: \"1.5\"\nc: \"\""},
		{"b64enc", `{{ b64enc "hi" }}`, "aGk="},
		{"b64dec", `{{ b64dec "aGk=" }}`, "hi"},
		{"sha256sum", `{{ sha256sum "" }}`, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"now", `{{ now.Year }}`, "2025"},
		{"dateFmt-time", `{{ dateFmt "2006-01-02" now }}`, "2025-10-19"},
		{"dateFmt-string", `{{ dateFmt "Jan 2" "2026-10-19" }}`, "Oct 19"},
		{"dateFmt-rfc3339", `{{ dateFmt "15:04" "2026-10-19T21:04:00Z" }}`, "21:04"},
		{"dateFmt-unix", `{{ dateFmt "2006" 0 }}`, "1970"},
		{"regexMatch", `{{ regexMatch "^v[0-9]+$" "v12" }}`, "true"},
		{"regexReplaceAll", `{{ regexReplaceAll "(o+)" "<$1>" "foo" }}`, "f<oo>"},
		{"shq", `{{ shq "it's" }}`, `'it'\''s'`},
		{"env", `{{ env "TMPLFORGE_TEST_ENV" }}`, "glenda"},
		{"file", `{{ file .file }}`, "1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := exec(t, All(), tc.src, data)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			if got != tc.want {
				t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestFuncErrors(t *testing.T) {
	for _, src := range []string{
		`{{ dict "a" }}`,
		`{{ dict 1 2 }}`,
		`{{ div 1 0 }}`,
		`{{ mod 1 0 }}`,
		`{{ add "x" 1 }}`,
		`{{ add 1.5 1 }}`,
		`{{ repeat -1 "x" }}`,
		`{{ repeat 2000000 "x" }}`,
		`{{ indent 1000 "x" }}`,
		`{{ seq 1 2 3 }}`,
		`{{ b64dec "!!" }}`,
		`{{ fromJSON "{" }}`,
		`{{ regexMatch "(" "x" }}`,
		`{{ dateFmt "2006" "not a date" }}`,
		`{{ sortedKeys 3 }}`,
		`{{ join "," 3 }}`,
		`{{ file "/nonexistent/x" }}`,
	} {
		if _, err := exec(t, All(), src, nil); err == nil {
			t.Errorf("%s: want error", src)
		}
	}
}

func TestSafe(t *testing.T) {
	s := Safe()
	for _, k := range Unsafe {
		if _, ok := s[k]; ok {
			t.Errorf("Safe() contains %s", k)
		}
	}
	if len(s) != len(All())-len(Unsafe) {
		t.Errorf("Safe() has %d funcs, want %d", len(s), len(All())-len(Unsafe))
	}
	if _, ok := s["now"]; !ok {
		t.Error("Safe() must keep now")
	}
	if _, err := template.New("t").Funcs(s).Parse(`{{ env "HOME" }}`); err == nil {
		t.Error("env parses under Safe()")
	}
}

// TestDocsComplete keeps FUNCS.md honest: every function documented, no
// documentation for a function that does not exist, and every example
// renders to its stated result (where the result is deterministic).
func TestDocsComplete(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1792368000") // 2026-10-19
	all := All()
	seen := map[string]bool{}
	for _, d := range Docs {
		if _, ok := all[d.Name]; !ok {
			t.Errorf("Docs has %q, not in All()", d.Name)
		}
		if seen[d.Name] {
			t.Errorf("Docs lists %q twice", d.Name)
		}
		seen[d.Name] = true
		switch d.Name {
		case "env", "file", "sha256sum":
			continue // host-dependent or abbreviated
		}
		got, err := exec(t, all, d.Example, nil)
		if err != nil {
			t.Errorf("%s example %s: %v", d.Name, d.Example, err)
			continue
		}
		want := strings.ReplaceAll(d.Result, `\n`, "\n")
		if got != want {
			t.Errorf("%s example %s = %q, doc says %q", d.Name, d.Example, got, want)
		}
	}
	var missing []string
	for k := range all {
		if !seen[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("undocumented functions: %v", missing)
	}
}

// TestFuncsMDCurrent fails when docs/FUNCS.md is stale: run go generate.
func TestFuncsMDCurrent(t *testing.T) {
	want, err := Markdown()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/FUNCS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("docs/FUNCS.md is stale; run: go generate ./internal/funcs")
	}
}

func TestYAMLListOfLists(t *testing.T) {
	got, err := toYAML([]any{[]any{1, 2}, map[string]any{}, []any{}, nil})
	if err != nil {
		t.Fatal(err)
	}
	want := "- \n  - 1\n  - 2\n- {}\n- []\n- null"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
