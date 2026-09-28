package copilot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/testutil"
)

func fixtureReport(t *testing.T, sp *Spec) *Report {
	t.Helper()
	var s Settings
	s.defaults()
	src := &FixtureSource{Dir: testutil.Path(t, "examples", "copilot-usage-report", "fixtures")}
	rep, err := Build(context.Background(), src, sp, Options{Org: "CoolEngOrg", Pricing: config.Pricing{AICreditUSD: 0.01, SeatMonthlyUSD: 39, SeatPlan: "Copilot Enterprise"},
		Settings: s, Now: time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC), WithReviews: true})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func span(sp *Spec) *Spec {
	sp.Start = time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	sp.End = time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	sp.Days = 28
	return sp
}

func TestTeamsReport(t *testing.T) {
	rep := fixtureReport(t, span(&Spec{Scope: "teams", Teams: []string{"platform-eng", "payments"}, IncludeSeats: true, HTML: true, CSV: true, Grafana: true, UserDetail: true}))
	if rep.DaysFound != 28 || len(rep.Teams) != 2 {
		t.Fatalf("days=%d teams=%d", rep.DaysFound, len(rep.Teams))
	}
	tot := rep.Totals
	if tot.Seats != 11 || tot.ActiveUsers == 0 || tot.EngagedUsers > tot.ActiveUsers || tot.MergedPRs == nil || *tot.MergedPRs == 0 {
		t.Fatalf("totals = %+v", tot)
	}
	if tot.TotalCostUSD <= tot.AICostUSD || tot.CostPerMergedPR == nil {
		t.Fatalf("seat cost missing: %+v", tot)
	}
	if len(rep.Users) == 0 {
		t.Fatal("per-user rows expected when user detail is approved")
	}
	for _, tr := range rep.Teams {
		if tr.Slug == "mobile" {
			t.Fatal("out-of-scope team in report")
		}
	}
	dir := t.TempDir()
	arts, err := WriteAll(rep, dir, "https://go-echarts.github.io/go-echarts-assets/assets/")
	if err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(arts.HTML)
	for _, want := range []string{"echarts.min.js", "<table", "prefers-color-scheme"} {
		if !strings.Contains(string(html), want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	prom, _ := os.ReadFile(arts.Prom)
	if !strings.Contains(string(prom), "# TYPE ") || !strings.Contains(string(prom), `team="payments"`) {
		t.Errorf("prometheus output:\n%s", prom)
	}
	if _, err := os.Stat(filepath.Join(dir, "users.csv")); err != nil {
		t.Error("users.csv missing")
	}
}

func TestNoUserDetailWithoutApproval(t *testing.T) {
	rep := fixtureReport(t, span(&Spec{Scope: "organization", HTML: true}))
	if len(rep.Users) != 0 {
		t.Fatal("per-user rows must not be produced without the user-detail option")
	}
	if rep.Totals.ActiveUsers < 10 {
		t.Fatalf("organization totals = %+v", rep.Totals)
	}
}

func TestMissingDaysAreReported(t *testing.T) {
	sp := &Spec{Scope: "organization"}
	sp.Start = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	sp.End = time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	rep := fixtureReport(t, sp)
	if len(rep.DaysMissing) != 5 || len(rep.Notes) == 0 {
		t.Fatalf("missing = %v notes = %v", rep.DaysMissing, rep.Notes)
	}
}
