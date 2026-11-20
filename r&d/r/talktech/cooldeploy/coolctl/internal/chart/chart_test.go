package chart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write lays out a chart directory from name → content.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var demo = map[string]string{
	"Chart.yaml": "apiVersion: v2\nname: demo\nversion: 1.2.3\n",
	"values.yaml": `# A comment header is allowed.
{
  "name": "web",
  "port": 8080,
  "replicas": 2,
  "labels": {"tier": "front"},
  "releases": [],
  "nested": {"keep": "yes", "change": "no"}
}
`,
	"templates/_helpers.tpl": `{{- define "demo.labels" -}}
{{- $l := dict "app" .Values.name -}}
{{- range $k, $v := .Values.labels -}}{{- $_ := set $l $k $v -}}{{- end -}}
{{- toJson $l -}}
{{- end }}
`,
	"templates/NOTES.txt": "ignored {{ fail \"never rendered\" }}",
	"templates/empty.yaml": `{{- if .Values.absent }}
kind: Never
{{- end }}
`,
	"templates/web.yaml": `{{- /* a comment */ -}}
kind: Thing
metadata:
  name: {{ .Values.name }}
  namespace: {{ .Release.Namespace }}
  chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
  labels: {{ include "demo.labels" . }}
spec:
  port: {{ int .Values.port | quote }}
  replicas: {{ int .Values.replicas }}
  big: {{ if gt (int .Values.replicas) 1 }}yes{{ else }}no{{ end }}
  missing: "{{ .Values.nothing }}"
  owner: {{ default "nobody" .Values.owner | quote }}
  nested: {{ toJson .Values.nested }}
  hash: {{ "abc" | sha256sum | trunc 8 }}
  key: {{ replace "_" "-" "A_B_C" | lower }}
{{- range .Values.releases }}
---
kind: Release
id: {{ .id }}
weight: {{ int .weight }}
{{- end }}
`,
}

func TestRender(t *testing.T) {
	c, err := Load(write(t, demo))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "demo" || c.Version != "1.2.3" {
		t.Fatalf("chart is %s-%s", c.Name, c.Version)
	}
	over := map[string]any{
		"replicas": float64(1),
		"nested":   map[string]any{"change": "yes"},
		"releases": []any{
			map[string]any{"id": "raaaaaaa", "weight": float64(90)},
			map[string]any{"id": "rbbbbbbb", "weight": float64(10)},
		},
	}
	out, err := c.Render("demo", "team-app", over)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Source: demo/templates/web.yaml",
		"name: web",
		"namespace: team-app",
		`chart: "demo-1.2.3"`,
		`labels: {"app":"web","tier":"front"}`,
		`port: "8080"`,
		"replicas: 1",
		"big: no",
		`missing: ""`,
		`owner: "nobody"`,
		`nested: {"change":"yes","keep":"yes"}`,
		"hash: ba7816bf",
		"key: a-b-c",
		"id: raaaaaaa\nweight: 90",
		"id: rbbbbbbb\nweight: 10",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q", want)
		}
	}
	for _, bad := range []string{"Never", "NOTES", "<no value>", "empty.yaml"} {
		if strings.Contains(out, bad) {
			t.Errorf("render contains %q", bad)
		}
	}
	// The chart's defaults must survive a render untouched.
	if c.Defaults["replicas"] != float64(2) || c.Defaults["nested"].(map[string]any)["change"] != "no" {
		t.Errorf("Render changed the chart's defaults: %v", c.Defaults)
	}
}

func TestRequiredAndFail(t *testing.T) {
	files := map[string]string{
		"Chart.yaml":         "name: strict\nversion: 0.1.0\n",
		"values.yaml":        `{"owner": ""}`,
		"templates/one.yaml": `owner: {{ required "owner is required" .Values.owner }}`,
	}
	c, err := Load(write(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render("strict", "ns"); err == nil || !strings.Contains(err.Error(), "owner is required") {
		t.Errorf("render without owner: %v", err)
	}
	out, err := c.Render("strict", "ns", map[string]any{"owner": "payments"})
	if err != nil || !strings.Contains(out, "owner: payments") {
		t.Errorf("render with owner: %q, %v", out, err)
	}
}

func TestLoadRefusesYAML(t *testing.T) {
	files := map[string]string{
		"Chart.yaml":  "name: plain\nversion: 0.1.0\n",
		"values.yaml": "name: web\nport: 8080\n",
	}
	if _, err := Load(write(t, files)); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Errorf("Load of block-style YAML values: %v", err)
	}
}

func TestMerge(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"x": 1.0, "y": 2.0}, "list": []any{1.0}, "keep": true}
	src := map[string]any{"a": map[string]any{"y": 3.0, "z": 4.0}, "list": []any{2.0, 3.0}, "new": "v"}
	got := Merge(dst, src)
	a := got["a"].(map[string]any)
	if a["x"] != 1.0 || a["y"] != 3.0 || a["z"] != 4.0 {
		t.Errorf("maps should merge key by key: %v", a)
	}
	if l := got["list"].([]any); len(l) != 2 {
		t.Errorf("lists should be replaced: %v", l)
	}
	if got["keep"] != true || got["new"] != "v" {
		t.Errorf("merge lost or missed a key: %v", got)
	}
	// A merged-in map must be a copy: later merges may not write through it.
	src["a"].(map[string]any)["z"] = 99.0
	Merge(map[string]any{}, map[string]any{"fresh": src["a"]})["fresh"].(map[string]any)["z"] = 5.0
	if src["a"].(map[string]any)["z"] != 99.0 {
		t.Error("Merge shares maps with its source")
	}
}

func TestFuncs(t *testing.T) {
	if !empty(nil) || !empty("") || !empty(0.0) || !empty([]any{}) || !empty(map[string]any{}) || empty("x") || empty(1.0) {
		t.Error("empty disagrees with Sprig")
	}
	for in, want := range map[any]int{8080.0: 8080, "42": 42, 7: 7, true: 1, nil: 0} {
		if got := toInt(in); got != want {
			t.Errorf("toInt(%v) = %d, want %d", in, got, want)
		}
	}
	if got := indent(2, "a\nb"); got != "  a\n  b" {
		t.Errorf("indent = %q", got)
	}
}
