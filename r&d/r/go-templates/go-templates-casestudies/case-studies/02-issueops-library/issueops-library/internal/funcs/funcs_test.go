package funcs

import (
	"bytes"
	"encoding/json"
	"testing"
	"text/template"
	"time"
)

func TestFuncs(t *testing.T) {
	data := map[string]any{
		"name": "Payments API", "port": json.Number("8443"), "empty": "", "list": []any{"a", "b"},
		"when": time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), "size": 1536,
	}
	cases := map[string]string{
		`{{.port | default 8080}}`:                           "8443",
		`{{.missing | default 8080}}`:                        "8080",
		`{{.empty | default "x"}}`:                           "x",
		`{{toJSON .list}}`:                                   `["a","b"]`,
		`{{toJSON (dict "a" 1 "b" (list 1 2))}}`:             `{"a":1,"b":[1,2]}`,
		`{{indent 2 "a\nb"}}`:                                "  a\n  b",
		`x:{{nindent 2 "a"}}`:                                "x:\n  a",
		`{{sha256sum "abc"}}`:                                "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		`{{humanSize .size}} {{humanSize 10}}`:               "1.5 KiB 10 B",
		`{{humanSize 3145728}}`:                              "3.0 MiB",
		`{{dateFmt "date" .when}} {{dateFmt "human" .when}}`: "2026-10-19 19 Oct 2026 12:00 UTC",
		`{{dateFmt "2006" "2025-01-02T03:04:05Z"}}`:          "2025",
		`{{plural 1 "item"}}, {{plural 3 "item"}}, {{plural 2 "entry" "entries"}}`: "1 item, 3 items, 2 entries",
		`{{slugify .name}} {{slugify "--Ünïcode  & más--"}}`:                       "payments-api n-code-m-s",
		`{{add 1 2 .port}} {{add 1.5 1}}`:                                          "8446 2.5",
		`{{join ", " .list}} {{quote "a\"b"}}`:                                     `a, b "a\"b"`,
		`{{hasKey . "name"}} {{hasKey . "nope"}}`:                                  "true false",
		`{{upper "a"}}{{lower "B"}}{{trim "  c "}}`:                                "Abc",
	}
	for src, want := range cases {
		tm := template.Must(template.New("").Funcs(Map()).Parse(src))
		var b bytes.Buffer
		if err := tm.Execute(&b, data); err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if b.String() != want {
			t.Errorf("%s = %q, want %q", src, b.String(), want)
		}
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{`{{dict "a"}}`, `{{dict 1 2}}`, `{{add "x" 1}}`, `{{dateFmt "date" 3}}`, `{{dateFmt "date" "yesterday"}}`} {
		tm := template.Must(template.New("").Funcs(Map()).Parse(src))
		if err := tm.Execute(&bytes.Buffer{}, nil); err == nil {
			t.Errorf("%s: want error", src)
		}
	}
}
