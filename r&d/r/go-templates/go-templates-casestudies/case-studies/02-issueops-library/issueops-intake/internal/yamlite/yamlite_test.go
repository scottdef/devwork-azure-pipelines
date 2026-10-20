package yamlite

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	src := `# comment
name: "Quoted: name"
labels: [a, "b c", 'd''e']
empty:
flag: true
num: 7 # trailing comment
lit: |
  line one
    indented

  after blank
fold: >-
  one
  two

  three
list:
  - plain
  - key: v
    other: w
  - - nested
seqAtKeyIndent:
- x
- y
`
	got, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"name":   "Quoted: name",
		"labels": []any{"a", "b c", "d'e"},
		"empty":  nil,
		"flag":   true,
		"num":    "7",
		"lit":    "line one\n  indented\n\nafter blank\n",
		"fold":   "one two\nthree",
		"list": []any{
			"plain",
			map[string]any{"key": "v", "other": "w"},
			[]any{"nested"},
		},
		"seqAtKeyIndent": []any{"x", "y"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{
		"a: 1\na: 2\n",
		"a: {x: 1}\n",
		"a: *ref\n",
		"\ta: 1\n",
		"a: \"open\n",
		"a: 1\n   b: 2\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q): want error", src)
		}
	}
}

func TestCRLFAndBOM(t *testing.T) {
	got, err := Parse([]byte("\ufeffa: 1\r\nb:\r\n  - x\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"a": "1", "b": []any{"x"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}
