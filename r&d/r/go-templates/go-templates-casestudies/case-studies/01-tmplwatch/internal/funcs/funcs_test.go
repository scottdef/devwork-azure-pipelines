package funcs

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"text/template"
	"time"
)

var fixedNow = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

func exec(t *testing.T, src string, data map[string]string) (string, *MapStore, error) {
	t.Helper()
	st := &MapStore{Data: data}
	fm := Map(Options{Store: st, Dir: os.DirFS("."), Now: fixedNow, Env: func(k string) string {
		if k == "TMPLWATCH_EXAMPLE" {
			return "example"
		}
		return ""
	}})
	tp, err := template.New("t").Funcs(fm).Parse(src)
	if err != nil {
		return "", st, err
	}
	var b bytes.Buffer
	err = tp.Execute(&b, nil)
	return b.String(), st, err
}

// TestDocExamples runs every documented example: FUNCS.md cannot lie.
func TestDocExamples(t *testing.T) {
	fm := Map(Options{})
	seen := map[string]bool{}
	for _, d := range Docs {
		if seen[d.Name] {
			t.Errorf("%s documented twice", d.Name)
		}
		seen[d.Name] = true
		if _, ok := fm[d.Name]; !ok {
			t.Errorf("%s documented but not in Map", d.Name)
		}
		got, _, err := exec(t, d.Example, ExampleData)
		if d.HostDeps {
			// Interfaces vary; lo may be absent (Plan 9, containers).
			if err != nil && !strings.Contains(err.Error(), "GetInterfaceIP") {
				t.Errorf("%s: %v", d.Name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", d.Name, err)
			continue
		}
		if got != d.Output {
			t.Errorf("%s: %s\n got %q\nwant %q", d.Name, d.Example, got, d.Output)
		}
	}
	for name := range fm {
		if !seen[name] {
			t.Errorf("%s in Map but not documented", name)
		}
	}
}

// TestFuncsDocUpToDate fails when docs/FUNCS.md was not regenerated.
func TestFuncsDocUpToDate(t *testing.T) {
	want, err := os.ReadFile("../../docs/FUNCS.md")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := WriteDoc(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Error("docs/FUNCS.md is stale: run `make docs`")
	}
}

func TestFuncs(t *testing.T) {
	data := map[string]string{"a": "1", "d/x": "X", "d/y": "Y", "d/z/deep": "D", "n": "42"}
	for _, tc := range []struct{ src, want, err string }{
		{`{{ key "a" }}`, "1", ""},
		{`{{ keyOrDefault "missing" "dflt" }}`, "dflt", ""},
		{`{{ range ls "d" }}{{ .Key }}{{ end }}`, "xy", ""},
		{`{{ range ls "d/" }}{{ .Key }}{{ end }}`, "xy", ""},
		{`{{ range tree "d" }}{{ .Key }},{{ end }}`, "x,y,z/deep,", ""},
		{`{{ len (ls "nope") }}`, "0", ""},
		{`{{ timestamp "unix" }}`, "1767323045", ""},
		{`{{ timestamp "a" "b" }}`, "", "at most one"},
		{`{{ parseJSON "{\"port\": 51820}" | toJSON }}`, `{"port":51820}`, ""},
		{`{{ parseJSON "{" }}`, "", "parseJSON"},
		{`{{ parseJSON "1 2" }}`, "", "trailing"},
		{`{{ b64dec "!!" }}`, "", "illegal"},
		{`{{ "a\n\nb" | indent 4 }}`, "    a\n\n    b", ""},
		{`{{ regexMatch "(" "x" }}`, "", "error parsing regexp"},
		{`{{ add "x" 1 }}`, "", "add"},
		{`{{ add (key "n") 8 }}`, "50", ""},
		{`{{ add 1.5 1 }}`, "", "not an integer"},
		{`{{ mod 1 0 }}`, "", "division by zero"},
		{`{{ dict "a" }}`, "", "odd"},
		{`{{ dict 1 2 }}`, "", "not string"},
		{`{{ join "," 3 }}`, "", "cannot use int"},
		{`{{ sortedKeys 3 }}`, "", "cannot use int"},
		{`{{ shq "" }}`, "''", ""},
		{`{{ shq "a b'c" }}`, `'a b'\''c'`, ""},
		{`{{ contains "x" (dict "x" 1) }}`, "true", ""},
		{`{{ contains "q" (list "a" "q") }}`, "true", ""},
		{`{{ 0 | default 7 }}`, "7", ""},
		{`{{ list | default "e" }}`, "e", ""},
		{`{{ plural "a" "b" "1" }}`, "a", ""},
		{`{{ file "../../go.mod" }}`, "", "relative to the template"},
		{`{{ file "/etc/passwd" }}`, "", "relative to the template"},
		{`{{ sockaddr "GetSomething" }}`, "", "unsupported"},
		{`{{ sockaddr "GetInterfaceIP lo" }}`, "", "quoted"},
		{`{{ toYAML (dict) }}`, "{}", ""},
		{`{{ toYAML (list) }}`, "[]", ""},
		{`{{ toYAML "plain" }}`, "plain", ""},
		{`{{ fail "boom" }}`, "", "boom"},
		{`{{ values 3 }}`, "", "cannot use int"},
		{`{{ values (dict "b" 2 "a" 1) | toJSON }}`, "[1,2]", ""},
		{`{{ ls "nope" | values | toJSON }}`, "[]", ""},
	} {
		got, _, err := exec(t, tc.src, data)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("%s: err %v, want %q", tc.src, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q %v, want %q", tc.src, got, err, tc.want)
		}
	}
}

func TestMissingRecorded(t *testing.T) {
	_, st, err := exec(t, `{{ key "gone" }}{{ keyOrDefault "soft" "" }}{{ keyExists "probe" }}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st.Missed, []string{"gone"}) {
		t.Errorf("missed %v", st.Missed)
	}
	if !reflect.DeepEqual(st.Touched, []string{"gone", "soft", "probe"}) {
		t.Errorf("touched %v", st.Touched)
	}
}

func TestYAMLQuoting(t *testing.T) {
	for in, want := range map[string]string{
		"plain": "plain", "yes": `"yes"`, "No": `"No"`, "0755": `"0755"`, "1e3": `"1e3"`,
		"0x1F": `"0x1F"`, "": `""`, " pad": `" pad"`, "a: b": `"a: b"`, "#c": `"#c"`,
		"- x": `"- x"`, "x #y": `"x #y"`, "tab\tin": `"tab\tin"`, "line\nbreak": `"line\nbreak"`,
		"null": `"null"`, "~": `"~"`, ".inf": `".inf"`, "1_000": `"1_000"`, "http://x/y": "http://x/y",
		"{a}": `"{a}"`, "*ref": `"*ref"`, "key:": `"key:"`, "10.0.0.1:9100": "10.0.0.1:9100",
		"quote\"in": `quote"in`, "<b>": "<b>", "1:30": `"1:30"`, "2026-01-02": `"2026-01-02"`,
		"web:59": "web:59", "v1.2.3": "v1.2.3",
	} {
		if got := yamlString(in); got != want {
			t.Errorf("yamlString(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestYAMLRoundTrip emits YAML for JSON structures, parses it back with a
// small reader for the block subset we emit, and compares against what
// encoding/json decodes from the same input.
func TestYAMLRoundTrip(t *testing.T) {
	for _, js := range []string{
		`{"a":1,"b":"two","c":[1,2,{"d":null,"e":true}],"f":{},"g":[],"h":{"i":{"j":"yes"}}}`,
		`[{"job_name":"node","static_configs":[{"targets":["10.0.0.1:9100","10.0.0.2:9100"],"labels":{"env":"prod"}}]}]`,
		`{"weird keys":{"a: b":"#x","- y":"0755","":"empty"},"multi":"l1\nl2","n":-1.5e3}`,
		`[[1,[2,[3]]],[],[{}],{"k":[[]]}]`,
		`"just a string"`,
		`{"unicode":"héllo ☃","ctl":"a\u0001b"}`,
	} {
		var want any
		dec := json.NewDecoder(strings.NewReader(js))
		dec.UseNumber()
		if err := dec.Decode(&want); err != nil {
			t.Fatal(err)
		}
		y, err := ToYAML(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := parseYAML(y)
		if err != nil {
			t.Fatalf("%s\n%s\nparse: %v", js, y, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip %s\nyaml:\n%s\ngot  %#v\nwant %#v", js, y, got, want)
		}
	}
}

func TestHCL(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want string
	}{
		{"plain", `"plain"`},
		{"${x} %{if} $x %x {y}", `"$${x} %%{if} $x %x {y}"`},
		{"q\"\\\n\t", `"q\"\\\n\t"`},
		{"\x01", `"\u0001"`},
		{nil, "null"},
		{map[string]any{}, "{}"},
		{map[string]any{"1x": 1, "ok_1": []any{}}, `{ "1x" = 1, ok_1 = [] }`},
	} {
		got, err := ToHCL(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ToHCL(%#v) = %s %v, want %s", tc.in, got, err, tc.want)
		}
	}
}
