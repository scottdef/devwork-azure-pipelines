package jx

import (
	"encoding/json"
	"testing"
)

func TestRemapGUIDs(t *testing.T) {
	m := map[string]string{"aaaaaaaa-0000-0000-0000-000000000001": "bbbbbbbb-0000-0000-0000-000000000002"}
	in := "repoV2/AAAAAAAA-0000-0000-0000-000000000001/cccccccc-0000-0000-0000-000000000003"
	want := "repoV2/bbbbbbbb-0000-0000-0000-000000000002/cccccccc-0000-0000-0000-000000000003"
	if got := RemapGUIDs(in, m); got != want {
		t.Fatalf("got %s", got)
	}
	if g := GUIDs(in); len(g) != 2 {
		t.Fatalf("GUIDs = %v", g)
	}
}

func TestRemapJSONKeepsShape(t *testing.T) {
	m := map[string]string{"aaaaaaaa-0000-0000-0000-000000000001": "bbbbbbbb-0000-0000-0000-000000000002"}
	in := M{"a": []any{M{"id": "aaaaaaaa-0000-0000-0000-000000000001", "n": 3.0, "html": "<b>&</b>"}}}
	out := RemapJSON(in, m)
	item := Arr(out, "a")[0]
	if Str(item, "id") != "bbbbbbbb-0000-0000-0000-000000000002" || Str(item, "html") != "<b>&</b>" || Get(item, "n") != 3.0 {
		b, _ := json.Marshal(out)
		t.Fatalf("got %s", b)
	}
	if Str(in, "a") != "" || Arr(in, "a") == nil {
		t.Fatal("input changed")
	}
}

func TestRebasePath(t *testing.T) {
	cases := map[string]string{
		`Src`:          `Tgt`,
		`src\Team\Sub`: `Tgt\Team\Sub`,
		`SrcOther\X`:   `SrcOther\X`,
		`Other\Src`:    `Other\Src`,
	}
	for in, want := range cases {
		if got := RebasePath(in, "Src", "Tgt"); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestHelpers(t *testing.T) {
	m, _ := Decode([]byte(`{"a":{"b":{"c":"5"}},"n":7}`))
	if v, ok := Int(m, "a", "b", "c"); !ok || v != 5 {
		t.Fatal("Int from string")
	}
	if v, ok := Int(m, "n"); !ok || v != 7 {
		t.Fatal("Int from number")
	}
	Set(m, "x", "p", "q")
	if Str(m, "p", "q") != "x" {
		t.Fatal("Set")
	}
	if ReplaceFold("FROM 'SRC'", "'src'", "'Tgt'") != "FROM 'Tgt'" {
		t.Fatal("ReplaceFold")
	}
}
