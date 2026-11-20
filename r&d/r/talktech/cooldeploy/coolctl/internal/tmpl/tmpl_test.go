package tmpl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDockerfiles(t *testing.T) {
	var b bytes.Buffer
	d := Docker{Preset: "go-htmx", Registry: "registry.example", Port: 8080, Main: "./cmd/server"}
	if err := Text(&b, "dockerfile.go.tmpl", d); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM registry.example/base/golang:1.24 AS build",
		"go build -trimpath",
		"./cmd/server",
		"USER 65532:65532",
		"ENV PORT=8080",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("Go Dockerfile lacks %q", want)
		}
	}
	if strings.Contains(b.String(), "[[") {
		t.Error("Go Dockerfile has an unrendered action")
	}

	b.Reset()
	if err := Text(&b, "dockerfile.static.tmpl", Docker{Preset: "static", Registry: "registry.example", Port: 8080}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "FROM registry.example/platform/cool-static:1") {
		t.Errorf("static Dockerfile:\n%s", b.String())
	}
}

// The caller workflow must name the pipeline by its v1 tag: cool-api trusts
// jobs by that reference and nothing else.
func TestWorkflow(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, "workflow.tmpl", nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"uses: CoolEngOrg/cool-actions/.github/workflows/cool-pipeline.yml@v1",
		"id-token: write",
		"contents: read",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("workflow lacks %q", want)
		}
	}
	if strings.Contains(b.String(), "\n    secrets:") {
		t.Error("the caller workflow must pass no secrets")
	}
}

// The dashboard is JSON for Grafana. Its PromQL legends use {{ }}, which is
// why the text templates use [[ ]].
func TestDashboardIsJSON(t *testing.T) {
	var b bytes.Buffer
	d := Dashboard{Team: "payments", App: "checkout", Environment: "prd", Namespace: "payments-checkout"}
	if err := Text(&b, "dashboard.tmpl", d); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		UID    string `json:"uid"`
		Title  string `json:"title"`
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatalf("dashboard is not JSON: %v", err)
	}
	if doc.UID != "cool-payments-checkout-prd" || len(doc.UID) > 40 {
		t.Errorf("uid = %q; Grafana allows 40 characters", doc.UID)
	}
	if len(doc.Panels) == 0 {
		t.Fatal("dashboard has no panels")
	}
	for _, p := range doc.Panels {
		for _, q := range p.Targets {
			if !strings.Contains(q.Expr, `"payments-checkout"`) {
				t.Errorf("panel %q does not select the namespace: %s", p.Title, q.Expr)
			}
		}
	}
	if !strings.Contains(b.String(), "{{destination_workload}}") {
		t.Error("legend placeholders were lost")
	}
}

// HTML templates escape what they insert: app data reaches the portal.
func TestStatusEscapes(t *testing.T) {
	// Maps stand in for api.AppStatus: a test here cannot import api, which
	// imports this package.
	release := map[string]any{"ID": "raaaaaaa", "Digest": "sha256:" + strings.Repeat("a", 64), "Weight": 100, "At": "2026-11-12T09:00:00Z"}
	env := map[string]any{"Environment": "dev", "URL": "https://checkout.dev.coolapps.example",
		"Sync": "Synced", "Health": "Healthy", "Releases": []map[string]any{release}}
	data := map[string]any{
		"Team": "payments", "App": "checkout", "Repo": "CoolEngOrg/<script>alert(1)</script>", "Chart": "cool-app-1.0.0",
		"Environments": []map[string]any{env},
	}
	var b bytes.Buffer
	if err := HTML(&b, "status.html", data); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "<script>alert") {
		t.Error("status.html does not escape values")
	}
	for _, want := range []string{"raaaaaaa", "aaaaaaaaaaaa", "2026-11-12 09:00", "checkout.dev.coolapps.example"} {
		if !strings.Contains(out, want) {
			t.Errorf("status.html lacks %q", want)
		}
	}
}
