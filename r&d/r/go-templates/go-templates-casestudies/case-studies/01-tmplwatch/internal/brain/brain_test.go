package brain

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

func mustParse(t *testing.T, name, text string) *Template {
	t.Helper()
	tp, err := Parse(name, name, text, fstest.MapFS{}, TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

func TestDependencyTracking(t *testing.T) {
	b := New()
	b.Update(source.Snapshot{Data: map[string]string{
		"web/port": "80", "web/hosts/a": "1", "web/hosts/b": "2", "db/url": "pg", "other": "x",
	}})
	web := mustParse(t, "web", `{{ key "web/port" }}{{ range ls "web/hosts" }}{{ .Value }}{{ end }}`)
	db := mustParse(t, "db", `{{ keyOrDefault "db/url" "" }}{{ keyExists "db/pool" }}`)
	for _, tp := range []*Template{web, db} {
		if !b.Dirty(tp.Name, nil) {
			t.Errorf("%s: never rendered, want dirty", tp.Name)
		}
		if r := b.Render(tp); !r.Complete() {
			t.Fatalf("%s: %v %v", tp.Name, r.Err, r.Missing)
		}
	}
	if got := b.Deps("web").List(); !reflect.DeepEqual(got, []string{"web/hosts/", "web/port"}) {
		t.Errorf("web deps %v", got)
	}
	if got := b.Deps("db").List(); !reflect.DeepEqual(got, []string{"db/pool", "db/url"}) {
		t.Errorf("db deps %v", got)
	}
	for _, tc := range []struct {
		changed []string
		web, db bool
	}{
		{[]string{"other"}, false, false},
		{[]string{"web/port"}, true, false},
		{[]string{"web/hosts/c"}, true, false}, // new child under a listed prefix
		{[]string{"web/hostsX"}, false, false}, // sibling, not under prefix
		{[]string{"db/pool"}, false, true},     // absent key read by keyExists appears
		{[]string{"db/url", "web/port"}, true, true},
	} {
		if got := b.Dirty("web", tc.changed); got != tc.web {
			t.Errorf("web dirty after %v = %v", tc.changed, got)
		}
		if got := b.Dirty("db", tc.changed); got != tc.db {
			t.Errorf("db dirty after %v = %v", tc.changed, got)
		}
	}
}

func TestUpdateDiff(t *testing.T) {
	b := New()
	if got := b.Update(source.Snapshot{Data: map[string]string{"a": "1", "b": "2"}}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("first update %v", got)
	}
	got := b.Update(source.Snapshot{Data: map[string]string{"a": "1", "b": "3", "c": "4"}})
	if !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("second update %v", got)
	}
	got = b.Update(source.Snapshot{Data: map[string]string{"c": "4"}})
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("deletes %v", got)
	}
}

func TestIncompleteRender(t *testing.T) {
	b := New()
	b.Update(source.Snapshot{Data: map[string]string{"have": "x"}})
	tp := mustParse(t, "t", `{{ key "have" }} {{ key "need" }} {{ key "need" }}`)
	r := b.Render(tp)
	if r.Complete() || !reflect.DeepEqual(r.Missing, []string{"need"}) {
		t.Fatalf("want incomplete on need, got %v %v", r.Missing, r.Err)
	}
	// The missing key is a dependency: its arrival makes the template dirty.
	if !b.Dirty("t", []string{"need"}) {
		t.Error("missing key not tracked as dependency")
	}
	b.Update(source.Snapshot{Data: map[string]string{"have": "x", "need": "y"}})
	r = b.Render(tp)
	if !r.Complete() || string(r.Out) != "x y y" {
		t.Errorf("after arrival: %q %v %v", r.Out, r.Missing, r.Err)
	}
}

func TestRenderError(t *testing.T) {
	b := New()
	tp := mustParse(t, "t", `{{ parseJSON "{" }}`)
	r := b.Render(tp)
	if r.Err == nil || r.Complete() {
		t.Fatal("want error")
	}
	if _, err := Parse("bad", "bad", "{{ nosuchfunc }}", nil, TemplateOptions{}); err == nil {
		t.Error("unknown function parsed")
	}
	if _, err := Parse("bad", "bad", "x", nil, TemplateOptions{MissingKey: "explode"}); err == nil {
		t.Error("bad missingkey accepted")
	}
}

func TestDelimsAndMissingKey(t *testing.T) {
	b := New()
	b.Update(source.Snapshot{Data: map[string]string{"k": "v"}})
	tp, err := Parse("d", "d", `[[ key "k" ]] {{ not a template }}`, nil, TemplateOptions{Left: "[[", Right: "]]"})
	if err != nil {
		t.Fatal(err)
	}
	if r := b.Render(tp); string(r.Out) != "v {{ not a template }}" {
		t.Errorf("delims: %q", r.Out)
	}
	tp, _ = Parse("m", "m", `{{ (dict "a" 1).b }}`, nil, TemplateOptions{MissingKey: "error"})
	if r := b.Render(tp); r.Err == nil || !strings.Contains(r.Err.Error(), "map has no entry") {
		t.Errorf("missingkey=error: %v", r.Err)
	}
}

func TestConcurrentRenders(t *testing.T) {
	b := New()
	b.Update(source.Snapshot{Data: map[string]string{"k": "v"}})
	tp := mustParse(t, "t", `{{ key "k" }}`)
	done := make(chan bool)
	for range 8 {
		go func() {
			for range 50 {
				if r := b.Render(tp); string(r.Out) != "v" {
					t.Errorf("got %q", r.Out)
				}
			}
			done <- true
		}()
	}
	for range 8 {
		<-done
	}
}
