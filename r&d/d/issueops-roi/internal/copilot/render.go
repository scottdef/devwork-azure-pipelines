package copilot

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/render"
)

// Palette follows the validated reference categorical palette (slot order is
// the colour-blind-safety mechanism; see docs/request-types/copilot-usage-report.md).
var (
	paletteLight = []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#4a3aa7", "#e34948"}
	paletteDark  = []string{"#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#008300", "#9085e9", "#e66767"}
)

// Artifacts are the files written for a report.
type Artifacts struct {
	JSON string   `json:"json"`
	HTML string   `json:"html,omitempty"`
	CSV  []string `json:"csv,omitempty"`
	Prom string   `json:"prometheus,omitempty"`
}

// WriteAll writes JSON (always) plus optional HTML, CSV and Prometheus files.
func WriteAll(r *Report, dir, assetsHost string) (*Artifacts, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	a := &Artifacts{JSON: filepath.Join(dir, "report.json")}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(a.JSON, b, 0o644); err != nil {
		return nil, err
	}
	if r.Spec.HTML {
		a.HTML = filepath.Join(dir, "copilot-roi.html")
		f, err := os.Create(a.HTML)
		if err != nil {
			return nil, err
		}
		err = RenderHTML(f, r, assetsHost)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	if r.Spec.CSV {
		files, err := WriteCSV(r, dir)
		if err != nil {
			return nil, err
		}
		a.CSV = files
	}
	if r.Spec.Grafana {
		a.Prom = filepath.Join(dir, "metrics.prom")
		if err := os.WriteFile(a.Prom, []byte(Prometheus(r, "")), 0o644); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// ---- CSV --------------------------------------------------------------

func fptr(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', 4, 64)
}

func iptr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// WriteCSV writes daily.csv, teams.csv, repositories.csv and users.csv (if any).
func WriteCSV(r *Report, dir string) ([]string, error) {
	var files []string
	write := func(name string, header []string, rows [][]string) error {
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)
		_ = w.Write(header)
		_ = w.WriteAll(rows)
		w.Flush()
		if err := f.Close(); err != nil {
			return err
		}
		files = append(files, p)
		return w.Error()
	}
	var daily [][]string
	for _, d := range r.Daily {
		daily = append(daily, []string{d.Day, strconv.Itoa(d.Active), strconv.Itoa(d.Engaged), fmt.Sprintf("%.2f", d.Credits), fmt.Sprintf("%.2f", d.AICostUSD)})
	}
	if err := write("daily.csv", []string{"day", "active_users", "engaged_users", "ai_credits", "ai_cost_usd"}, daily); err != nil {
		return nil, err
	}
	var teams [][]string
	for _, t := range r.Teams {
		teams = append(teams, []string{t.Slug, strconv.Itoa(t.Seats), strconv.Itoa(t.ActiveUsers), strconv.Itoa(t.EngagedUsers), fmt.Sprintf("%.2f", t.Credits),
			fmt.Sprintf("%.2f", t.AICostUSD), fmt.Sprintf("%.2f", t.SeatCostUSD), fmt.Sprintf("%.2f", t.TotalCostUSD), strconv.Itoa(t.LocAdded),
			fptr(t.AcceptanceRate), iptr(t.MergedPRs), fptr(t.MedianCycleHrs), iptr(t.Reviews), fptr(t.CostPerMergedPR)})
	}
	if err := write("teams.csv", []string{"team", "seats", "active_users", "engaged_users", "ai_credits", "ai_cost_usd", "seat_cost_usd", "total_cost_usd", "ai_loc_added", "acceptance_rate", "merged_prs", "median_cycle_hours", "reviews", "cost_per_merged_pr_usd"}, teams); err != nil {
		return nil, err
	}
	var repos [][]string
	for _, x := range r.Repos {
		repos = append(repos, []string{x.Name, strconv.Itoa(x.CreatedPRs), strconv.Itoa(x.MergedPRs), strconv.Itoa(x.MergedCopilot), strconv.Itoa(x.ReviewedByCopilot),
			fptr(x.MedianMinutesMerge), fptr(x.CopilotShare), iptr(x.Commits), iptr(x.Additions), iptr(x.Deletions), fptr(x.AttributedAICostUSD), fptr(x.CostPerMergedPR)})
	}
	if err := write("repositories.csv", []string{"repository", "created_prs", "merged_prs", "merged_copilot_authored", "reviewed_by_copilot", "median_minutes_to_merge", "copilot_authored_share", "commits", "additions", "deletions", "attributed_ai_cost_usd", "attributed_ai_cost_per_merged_pr_usd"}, repos); err != nil {
		return nil, err
	}
	if len(r.Users) > 0 {
		var users [][]string
		for _, u := range r.Users {
			users = append(users, []string{u.Login, strings.Join(u.Teams, ";"), strconv.Itoa(u.ActiveDays), strconv.Itoa(u.EngagedDays), strconv.Itoa(u.AgenticDays),
				fmt.Sprintf("%.2f", u.Credits), fmt.Sprintf("%.2f", u.AICostUSD), strconv.Itoa(u.LocAdded), fptr(u.AcceptanceRate), iptr(u.MergedPRs), u.AdoptionPhase})
		}
		if err := write("users.csv", []string{"login", "teams", "active_days", "engaged_days", "agentic_days", "ai_credits", "ai_cost_usd", "ai_loc_added", "acceptance_rate", "merged_prs", "adoption_phase"}, users); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// ---- Prometheus text (for Pushgateway -> Prometheus -> Grafana OSS 12) --

func promEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// Prometheus renders gauges in the text exposition format.
func Prometheus(r *Report, extraLabels string) string {
	var b strings.Builder
	window := fmt.Sprintf("%dd", r.Spec.Days)
	base := fmt.Sprintf(`org="%s",scope="%s",window="%s"`, promEscape(r.Org), r.Spec.Scope, window)
	if extraLabels != "" {
		base += "," + extraLabels
	}
	g := func(name, help string, series [][2]string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		for _, s := range series {
			fmt.Fprintf(&b, "%s{%s%s} %s\n", name, base, s[0], s[1])
		}
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	t := r.Totals
	g("issueops_copilot_active_users", "Distinct active Copilot users in the window.", [][2]string{{"", strconv.Itoa(t.ActiveUsers)}})
	g("issueops_copilot_engaged_users", "Distinct engaged Copilot users in the window.", [][2]string{{"", strconv.Itoa(t.EngagedUsers)}})
	g("issueops_copilot_ai_cost_usd", "AI-credit cost in USD for the window.", [][2]string{{"", f(t.AICostUSD)}})
	g("issueops_copilot_total_cost_usd", "AI-credit plus prorated seat cost in USD for the window.", [][2]string{{"", f(t.TotalCostUSD)}})
	if t.MergedPRs != nil {
		g("issueops_copilot_merged_prs", "Merged pull requests in scope for the window.", [][2]string{{"", strconv.Itoa(*t.MergedPRs)}})
	}
	if t.CostPerMergedPR != nil {
		g("issueops_copilot_cost_per_merged_pr_usd", "Total cost per merged pull request.", [][2]string{{"", f(*t.CostPerMergedPR)}})
	}
	var teamCost, teamPR, teamCPP, teamEng [][2]string
	for _, tr := range r.Teams {
		l := fmt.Sprintf(`,team="%s"`, promEscape(tr.Slug))
		teamCost = append(teamCost, [2]string{l, f(tr.TotalCostUSD)})
		teamEng = append(teamEng, [2]string{l, strconv.Itoa(tr.EngagedUsers)})
		if tr.MergedPRs != nil {
			teamPR = append(teamPR, [2]string{l, strconv.Itoa(*tr.MergedPRs)})
		}
		if tr.CostPerMergedPR != nil {
			teamCPP = append(teamCPP, [2]string{l, f(*tr.CostPerMergedPR)})
		}
	}
	if len(teamCost) > 0 {
		g("issueops_copilot_team_total_cost_usd", "Team total cost in USD for the window.", teamCost)
		g("issueops_copilot_team_engaged_users", "Team engaged users for the window.", teamEng)
	}
	if len(teamPR) > 0 {
		g("issueops_copilot_team_merged_prs", "Team merged pull requests for the window.", teamPR)
	}
	if len(teamCPP) > 0 {
		g("issueops_copilot_team_cost_per_merged_pr_usd", "Team cost per merged pull request.", teamCPP)
	}
	var repoPR, repoCop [][2]string
	for _, x := range r.Repos {
		l := fmt.Sprintf(`,repository="%s"`, promEscape(x.Name))
		repoPR = append(repoPR, [2]string{l, strconv.Itoa(x.MergedPRs)})
		repoCop = append(repoCop, [2]string{l, strconv.Itoa(x.MergedCopilot)})
	}
	if len(repoPR) > 0 {
		g("issueops_copilot_repo_merged_prs", "Repository merged pull requests for the window.", repoPR)
		g("issueops_copilot_repo_merged_copilot_prs", "Repository merged Copilot-authored pull requests for the window.", repoCop)
	}
	return b.String()
}

// ---- HTML (go-echarts snippets inside an accessible page) ---------------

type chartBlock struct {
	ID      string
	Title   string
	Desc    string
	Element template.HTML
	Script  template.HTML
	Kind    string // line | bar | hbar | scatter | stack
	Table   tableData
}

type tableData struct {
	Head []string
	Rows [][]string
}

type kpi struct{ Label, Value, Sub string }

type pageData struct {
	Title       string
	Subtitle    string
	Purpose     string
	KPIs        []kpi
	Findings    []string
	Charts      []chartBlock
	Tables      []namedTable
	Notes       []string
	Definitions [][2]string
	AssetsHost  string
	PaletteJS   template.JS
	ChartMetaJS template.JS
}

type namedTable struct {
	Title string
	tableData
}

func money(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")
	var out []byte
	for i, c := range []byte(intPart) {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return sign + "$" + string(out) + "." + frac
}

func pctPtr(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", *p*100)
}

func moneyPtr(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return money(*p)
}

func intPtr(p *int) string {
	if p == nil {
		return "n/a"
	}
	return strconv.Itoa(*p)
}

func hoursPtr(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f h", *p)
}

// defaultGrid leaves room for the legend on top; containLabel keeps tick
// labels inside the chart (axis names are not covered by containLabel).
var defaultGrid = opts.Grid{Left: "8", Right: "24", Top: "40", Bottom: "8", ContainLabel: opts.Bool(true)}

// baseChart returns shared options. go-echarts appends grids, so the grid is
// passed here (exactly one per chart) rather than overridden later.
func baseChart(id string, grid ...opts.Grid) []charts.GlobalOpts {
	g := defaultGrid
	if len(grid) > 0 {
		g = grid[0]
	}
	return []charts.GlobalOpts{
		charts.WithInitializationOpts(opts.Initialization{ChartID: id, Width: "100%", Height: "340px", Renderer: "svg"}),
		charts.WithColorsOpts(opts.Colors(paletteLight)),
		charts.WithAnimation(false),
		charts.WithGridOpts(g),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
	}
}

func snippet(r render.Renderer) (template.HTML, template.HTML) {
	s := r.RenderSnippet()
	// Content is generated from report numbers and escaped labels by
	// go-echarts; it is trusted output of this program.
	return template.HTML(s.Element), template.HTML(s.Script) //nolint:gosec
}

func buildCharts(r *Report) []chartBlock {
	var out []chartBlock

	// 1. Daily adoption: active vs engaged users (two series, one unit).
	{
		days := make([]string, len(r.Daily))
		act := make([]opts.LineData, len(r.Daily))
		eng := make([]opts.LineData, len(r.Daily))
		tbl := tableData{Head: []string{"Day", "Active users", "Engaged users"}}
		for i, d := range r.Daily {
			days[i] = d.Day
			act[i] = opts.LineData{Value: d.Active}
			eng[i] = opts.LineData{Value: d.Engaged}
			tbl.Rows = append(tbl.Rows, []string{d.Day, strconv.Itoa(d.Active), strconv.Itoa(d.Engaged)})
		}
		c := charts.NewLine()
		c.SetGlobalOptions(append(baseChart("adoption"),
			charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "0", Left: "0"}),
			charts.WithYAxisOpts(opts.YAxis{MinInterval: 1}))...)
		c.SetXAxis(days).
			AddSeries("Active users", act).
			AddSeries("Engaged users", eng)
		c.SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{ShowSymbol: opts.Bool(false)}))
		el, sc := snippet(c)
		out = append(out, chartBlock{ID: "adoption", Title: "Daily active and engaged users", Desc: "Users per day. Engaged users accepted code or used an agentic surface that day.", Element: el, Script: sc, Kind: "line", Table: tbl})
	}

	// 2. Daily AI-credit cost (single series).
	{
		days := make([]string, len(r.Daily))
		cost := make([]opts.BarData, len(r.Daily))
		tbl := tableData{Head: []string{"Day", "AI credits", "AI cost"}}
		for i, d := range r.Daily {
			days[i] = d.Day
			cost[i] = opts.BarData{Value: d.AICostUSD}
			tbl.Rows = append(tbl.Rows, []string{d.Day, fmt.Sprintf("%.2f", d.Credits), money(d.AICostUSD)})
		}
		c := charts.NewBar()
		c.SetGlobalOptions(baseChart("daily_cost")...)
		c.SetXAxis(days).AddSeries("AI cost (USD)", cost)
		el, sc := snippet(c)
		out = append(out, chartBlock{ID: "daily_cost", Title: "Daily AI-credit cost (USD)", Desc: "Metered AI credits consumed by users in scope, priced at the configured credit price.", Element: el, Script: sc, Kind: "bar", Table: tbl})
	}

	// 3. Cost per merged PR by team (nominal categories: one colour).
	var teamsWithOutput []TeamRow
	for _, t := range r.Teams {
		if t.CostPerMergedPR != nil {
			teamsWithOutput = append(teamsWithOutput, t)
		}
	}
	if len(teamsWithOutput) > 0 {
		sort.SliceStable(teamsWithOutput, func(i, j int) bool { return *teamsWithOutput[i].CostPerMergedPR < *teamsWithOutput[j].CostPerMergedPR })
		names := make([]string, len(teamsWithOutput))
		vals := make([]opts.BarData, len(teamsWithOutput))
		tbl := tableData{Head: []string{"Team", "Total cost", "Merged PRs", "Cost per merged PR"}}
		for i, t := range teamsWithOutput {
			names[i] = t.Slug
			vals[i] = opts.BarData{Value: *t.CostPerMergedPR}
			tbl.Rows = append(tbl.Rows, []string{t.Slug, money(t.TotalCostUSD), intPtr(t.MergedPRs), moneyPtr(t.CostPerMergedPR)})
		}
		c := charts.NewBar()
		c.SetGlobalOptions(append(baseChart("team_cpp", opts.Grid{Left: "28", Right: "32", Top: "40", Bottom: "8", ContainLabel: opts.Bool(true)}),
			charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}))...)
		c.SetXAxis(names).AddSeries("Cost per merged PR", vals)
		c.XYReversal()
		el, sc := snippet(c)
		out = append(out, chartBlock{ID: "team_cpp", Title: "Cost per merged pull request by team (USD)", Desc: "Lower is better. Teams with no merged pull requests are omitted.", Element: el, Script: sc, Kind: "hbar", Table: tbl})

		// 4. Team spend vs output scatter (single series, labelled points).
		pts := make([]opts.ScatterData, 0, len(teamsWithOutput))
		for _, t := range teamsWithOutput {
			pts = append(pts, opts.ScatterData{Name: t.Slug, Value: []any{t.TotalCostUSD, *t.MergedPRs}, SymbolSize: 12})
		}
		s := charts.NewScatter()
		// Axis names sit outside containLabel's box: centre them and reserve room.
		s.SetGlobalOptions(append(baseChart("team_scatter", opts.Grid{Left: "40", Right: "96", Top: "40", Bottom: "36", ContainLabel: opts.Bool(true)}),
			charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}),
			charts.WithXAxisOpts(opts.XAxis{Name: "Total cost (USD)", Type: "value", NameLocation: "middle", NameGap: 30}),
			charts.WithYAxisOpts(opts.YAxis{Name: "Merged PRs", Type: "value", NameLocation: "middle", NameGap: 40, MinInterval: 1}))...)
		s.AddSeries("Teams", pts, charts.WithLabelOpts(opts.Label{Show: opts.Bool(true), Position: "right", Formatter: "{b}"}))
		el2, sc2 := snippet(s)
		out = append(out, chartBlock{ID: "team_scatter", Title: "Team spend versus merged pull requests", Desc: "Each point is a team: total cost (USD) across, merged pull requests up. Up and to the left means more output per dollar.", Element: el2, Script: sc2, Kind: "scatter", Table: tbl})
	}

	// 5. Merged PRs by repository: Copilot-authored vs other (stacked, 2 series).
	if len(r.Repos) > 0 {
		repos := r.Repos
		if len(repos) > 15 {
			repos = repos[:15]
		}
		names := make([]string, len(repos))
		cop := make([]opts.BarData, len(repos))
		oth := make([]opts.BarData, len(repos))
		tbl := tableData{Head: []string{"Repository", "Merged PRs", "Copilot-authored", "Copilot share", "Median time to merge"}}
		for i := len(repos) - 1; i >= 0; i-- { // largest at top of a reversed axis
			x := repos[i]
			names[len(repos)-1-i] = x.Name
			cop[len(repos)-1-i] = opts.BarData{Value: x.MergedCopilot}
			oth[len(repos)-1-i] = opts.BarData{Value: x.MergedPRs - x.MergedCopilot}
		}
		for _, x := range repos {
			mm := "n/a"
			if x.MedianMinutesMerge != nil {
				mm = fmt.Sprintf("%.1f h", *x.MedianMinutesMerge/60)
			}
			tbl.Rows = append(tbl.Rows, []string{x.Name, strconv.Itoa(x.MergedPRs), strconv.Itoa(x.MergedCopilot), pctPtr(x.CopilotShare), mm})
		}
		c := charts.NewBar()
		c.SetGlobalOptions(append(baseChart("repo_stack", opts.Grid{Left: "28", Right: "32", Top: "40", Bottom: "8", ContainLabel: opts.Bool(true)}),
			charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "0", Left: "0"}))...)
		c.SetXAxis(names).
			AddSeries("Copilot-authored", cop, charts.WithBarChartOpts(opts.BarChart{Stack: "prs"})).
			AddSeries("Other", oth, charts.WithBarChartOpts(opts.BarChart{Stack: "prs"}))
		c.XYReversal()
		el, sc := snippet(c)
		out = append(out, chartBlock{ID: "repo_stack", Title: "Merged pull requests by repository", Desc: "Split by whether the Copilot cloud agent authored the pull request (from the repository usage-metrics report).", Element: el, Script: sc, Kind: "stack", Table: tbl})
	}

	// 6. Weekly merged pull requests (single series).
	hasPR := false
	for _, w := range r.Weekly {
		if w.MergedPRs > 0 {
			hasPR = true
		}
	}
	if hasPR {
		weeks := make([]string, len(r.Weekly))
		vals := make([]opts.LineData, len(r.Weekly))
		tbl := tableData{Head: []string{"Week starting", "Merged PRs", "AI cost"}}
		for i, w := range r.Weekly {
			weeks[i] = w.Week
			vals[i] = opts.LineData{Value: w.MergedPRs}
			tbl.Rows = append(tbl.Rows, []string{w.Week, strconv.Itoa(w.MergedPRs), money(w.AICostUSD)})
		}
		c := charts.NewLine()
		c.SetGlobalOptions(append(baseChart("weekly_prs"), charts.WithYAxisOpts(opts.YAxis{MinInterval: 1}))...)
		c.SetXAxis(weeks).AddSeries("Merged PRs", vals)
		el, sc := snippet(c)
		out = append(out, chartBlock{ID: "weekly_prs", Title: "Merged pull requests per week", Desc: "Weeks start on Monday; partial weeks at the edges of the period are included.", Element: el, Script: sc, Kind: "line", Table: tbl})
	}
	return out
}

// RenderHTML writes the interactive report page.
func RenderHTML(w io.Writer, r *Report, assetsHost string) error {
	t := r.Totals
	scope := r.Org
	switch r.Spec.Scope {
	case "teams":
		scope = strings.Join(r.Spec.Teams, ", ")
	case "repositories":
		scope = strings.Join(r.Spec.Repos, ", ")
	}
	engagedSub := ""
	if t.EngagedShare != nil {
		engagedSub = fmt.Sprintf("%d engaged (%s)", t.EngagedUsers, pctPtr(t.EngagedShare))
	}
	costSub := "AI credits only"
	if r.Spec.IncludeSeats {
		costSub = fmt.Sprintf("%s AI + %s seats", money(t.AICostUSD), money(t.SeatCostUSD))
	}
	pd := pageData{
		Title:      "Copilot usage and ROI",
		Subtitle:   fmt.Sprintf("%s · %s · %s to %s (%d days with data)", r.Org, scope, r.Spec.Start.Format("2006-01-02"), r.Spec.End.Format("2006-01-02"), r.DaysFound),
		Purpose:    r.Spec.Purpose,
		AssetsHost: assetsHost,
		Findings:   r.Findings,
		Notes:      r.Notes,
		KPIs: []kpi{
			{"Total cost", money(t.TotalCostUSD), costSub},
			{"Cost per merged PR", moneyPtr(t.CostPerMergedPR), "AI only: " + moneyPtr(t.AICostPerPR)},
			{"Merged pull requests", intPtr(t.MergedPRs), "Copilot-authored: " + pctPtr(t.CopilotShare)},
			{"Active users", strconv.Itoa(t.ActiveUsers), engagedSub},
			{"Code acceptance rate", pctPtr(t.AcceptanceRate), fmt.Sprintf("%d AI-assisted lines added", t.LocAdded)},
			{"Median time to merge", hoursPtr(t.MedianCycleHrs), "p90: " + hoursPtr(t.P90CycleHrs)},
		},
	}
	keys := make([]string, 0, len(r.Definitions))
	for k := range r.Definitions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		pd.Definitions = append(pd.Definitions, [2]string{k, r.Definitions[k]})
	}
	pd.Charts = buildCharts(r)
	if len(r.Teams) > 0 {
		nt := namedTable{Title: "Teams", tableData: tableData{Head: []string{"Team", "Seats", "Active", "Engaged", "AI cost", "Seat cost", "Total cost", "Acceptance", "Merged PRs", "Median cycle", "Reviews", "Cost / PR"}}}
		for _, x := range r.Teams {
			nt.Rows = append(nt.Rows, []string{x.Slug, strconv.Itoa(x.Seats), strconv.Itoa(x.ActiveUsers), strconv.Itoa(x.EngagedUsers), money(x.AICostUSD), money(x.SeatCostUSD), money(x.TotalCostUSD), pctPtr(x.AcceptanceRate), intPtr(x.MergedPRs), hoursPtr(x.MedianCycleHrs), intPtr(x.Reviews), moneyPtr(x.CostPerMergedPR)})
		}
		pd.Tables = append(pd.Tables, nt)
	}
	if len(r.Repos) > 0 {
		nt := namedTable{Title: "Repositories", tableData: tableData{Head: []string{"Repository", "Created PRs", "Merged PRs", "Copilot-authored", "Copilot share", "Reviewed by Copilot", "Commits", "Additions", "Deletions", "Attributed AI cost", "AI cost / PR"}}}
		for _, x := range r.Repos {
			nt.Rows = append(nt.Rows, []string{x.Name, strconv.Itoa(x.CreatedPRs), strconv.Itoa(x.MergedPRs), strconv.Itoa(x.MergedCopilot), pctPtr(x.CopilotShare), strconv.Itoa(x.ReviewedByCopilot), intPtr(x.Commits), intPtr(x.Additions), intPtr(x.Deletions), moneyPtr(x.AttributedAICostUSD), moneyPtr(x.CostPerMergedPR)})
		}
		pd.Tables = append(pd.Tables, nt)
	}
	if len(r.Users) > 0 {
		nt := namedTable{Title: "Users (approved per-user detail)", tableData: tableData{Head: []string{"Login", "Teams", "Active days", "Engaged days", "Agentic days", "AI cost", "AI lines added", "Acceptance", "Merged PRs", "Adoption phase"}}}
		for _, u := range r.Users {
			nt.Rows = append(nt.Rows, []string{u.Login, strings.Join(u.Teams, ", "), strconv.Itoa(u.ActiveDays), strconv.Itoa(u.EngagedDays), strconv.Itoa(u.AgenticDays), money(u.AICostUSD), strconv.Itoa(u.LocAdded), pctPtr(u.AcceptanceRate), intPtr(u.MergedPRs), u.AdoptionPhase})
		}
		pd.Tables = append(pd.Tables, nt)
	}
	pal, _ := json.Marshal(map[string][]string{"light": paletteLight, "dark": paletteDark})
	pd.PaletteJS = template.JS(pal) //nolint:gosec // static palette
	meta := map[string]string{}
	for _, c := range pd.Charts {
		meta[c.ID] = c.Kind
	}
	mj, _ := json.Marshal(meta)
	pd.ChartMetaJS = template.JS(mj) //nolint:gosec // ids and kinds are constants
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, pd); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
.viz-root{color-scheme:light;--page:#f9f9f7;--surface-1:#fcfcfb;--text-primary:#0b0b0b;--text-secondary:#52514e;--muted:#898781;--grid:#e1e0d9;--axis:#c3c2b7;--border:rgba(11,11,11,.10)}
@media (prefers-color-scheme:dark){:root:where(:not([data-theme="light"])) .viz-root{color-scheme:dark;--page:#0d0d0d;--surface-1:#1a1a19;--text-primary:#ffffff;--text-secondary:#c3c2b7;--muted:#898781;--grid:#2c2c2a;--axis:#383835;--border:rgba(255,255,255,.10)}}
:root[data-theme="dark"] .viz-root{color-scheme:dark;--page:#0d0d0d;--surface-1:#1a1a19;--text-primary:#ffffff;--text-secondary:#c3c2b7;--muted:#898781;--grid:#2c2c2a;--axis:#383835;--border:rgba(255,255,255,.10)}
*{box-sizing:border-box}
body{margin:0;background:var(--page,#f9f9f7)}
.viz-root{background:var(--page);color:var(--text-primary);font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;min-height:100vh;padding:24px 16px 48px}
main{max-width:1120px;margin:0 auto}
h1{font-size:28px;margin:0 0 4px}
h2{font-size:18px;margin:32px 0 8px}
.sub{color:var(--text-secondary);margin:0 0 4px}
.purpose{color:var(--text-secondary);margin:0 0 20px;font-style:italic}
.kpis{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:12px;margin:16px 0}
.kpi{background:var(--surface-1);border:1px solid var(--border);border-radius:10px;padding:14px 16px}
.kpi .l{color:var(--text-secondary);font-size:13px}
.kpi .v{font-size:28px;font-weight:600;margin:2px 0}
.kpi .s{color:var(--muted);font-size:12px}
.card{background:var(--surface-1);border:1px solid var(--border);border-radius:10px;padding:16px;margin:16px 0}
.card h3{margin:0;font-size:16px}
.card p{margin:2px 0 8px;color:var(--text-secondary);font-size:13px}
.container{width:100%}
details{margin-top:8px}
summary{cursor:pointer;color:var(--text-secondary);font-size:13px}
.tw{overflow-x:auto}
table{border-collapse:collapse;width:100%;font-size:13px;font-variant-numeric:tabular-nums}
th,td{padding:6px 8px;border-bottom:1px solid var(--grid);text-align:right;white-space:nowrap}
th:first-child,td:first-child{text-align:left}
th{color:var(--text-secondary);font-weight:600}
ul{padding-left:20px}
dl{display:grid;grid-template-columns:max-content 1fr;gap:4px 16px;font-size:13px}
dt{color:var(--text-secondary)}
dd{margin:0}
</style>
<script src="{{.AssetsHost}}echarts.min.js"></script>
</head>
<body>
<div class="viz-root">
<main>
<h1>{{.Title}}</h1>
<p class="sub">{{.Subtitle}}</p>
{{if .Purpose}}<p class="purpose">Purpose: {{.Purpose}}</p>{{end}}
<section class="kpis" aria-label="Headline metrics">
{{range .KPIs}}<div class="kpi"><div class="l">{{.Label}}</div><div class="v">{{.Value}}</div><div class="s">{{.Sub}}</div></div>
{{end}}</section>
<h2>Findings</h2>
<ul>{{range .Findings}}<li>{{.}}</li>{{end}}</ul>
{{range .Charts}}
<section class="card" aria-labelledby="h-{{.ID}}">
<h3 id="h-{{.ID}}">{{.Title}}</h3>
<p>{{.Desc}}</p>
{{.Element}}
<details><summary>Table view</summary><div class="tw"><table>
<thead><tr>{{range .Table.Head}}<th scope="col">{{.}}</th>{{end}}</tr></thead>
<tbody>{{range .Table.Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody>
</table></div></details>
</section>
{{.Script}}
{{end}}
{{range .Tables}}
<h2>{{.Title}}</h2>
<div class="card tw"><table>
<thead><tr>{{range .Head}}<th scope="col">{{.}}</th>{{end}}</tr></thead>
<tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody>
</table></div>
{{end}}
<h2>Definitions</h2>
<dl>{{range .Definitions}}<dt>{{index . 0}}</dt><dd>{{index . 1}}</dd>{{end}}</dl>
<h2>Notes and caveats</h2>
<ul>{{range .Notes}}<li>{{.}}</li>{{end}}</ul>
</main>
</div>
<script>
(function(){
  "use strict";
  const PALETTE = {{.PaletteJS}};
  const META = {{.ChartMetaJS}};
  const css = (n) => getComputedStyle(document.querySelector('.viz-root')).getPropertyValue(n).trim();
  const isDark = () => {
    const t = document.documentElement.getAttribute('data-theme');
    if (t) return t === 'dark';
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  };
  function style(id, kind) {
    const inst = echarts.getInstanceByDom(document.getElementById(id));
    if (!inst) return;
    const ink = css('--text-secondary'), muted = css('--muted'), grid = css('--grid'), axis = css('--axis'), surf = css('--surface-1');
    const ax = {axisLine:{lineStyle:{color:axis}}, axisTick:{show:false}, axisLabel:{color:muted}, splitLine:{lineStyle:{color:grid, type:'solid', width:1}}, nameTextStyle:{color:muted}};
    const opt = {
      color: isDark() ? PALETTE.dark : PALETTE.light,
      backgroundColor: 'transparent',
      textStyle: {fontFamily: 'system-ui,-apple-system,"Segoe UI",sans-serif', color: ink},
      legend: {textStyle:{color: ink}},
      tooltip: {backgroundColor: surf, borderColor: grid, textStyle:{color: css('--text-primary')}},
      xAxis: [ax], yAxis: [ax]
    };
    const n = (inst.getOption().series || []).length;
    opt.legend.show = n > 1; // a single series is named by the chart title
    const series = [];
    for (let i = 0; i < n; i++) {
      if (kind === 'line') series.push({lineStyle:{width:2}, symbolSize:8, endLabel:{show: n > 1 && n <= 4, formatter:'{a}', color: ink}, emphasis:{focus:'series'}});
      else if (kind === 'bar') series.push({barMaxWidth:18, itemStyle:{borderRadius:[4,4,0,0]}});
      else if (kind === 'hbar') series.push({barMaxWidth:18, itemStyle:{borderRadius:[0,4,4,0]}});
      else if (kind === 'stack') series.push({barMaxWidth:18, itemStyle:{borderColor: surf, borderWidth:2, borderRadius: i === n-1 ? [0,4,4,0] : 0}});
      else if (kind === 'scatter') series.push({symbolSize:10, itemStyle:{borderColor: surf, borderWidth:2}, label:{color: ink}});
    }
    // Direct end labels (2-4 lines): nudge apart labels whose last values collide.
    if (kind === 'line' && n > 1 && n <= 4) {
      const src = inst.getOption().series;
      const last = src.map(s => { const d = s.data || []; const v = d.length ? d[d.length-1] : 0; return Number(v && v.value !== undefined ? v.value : v) || 0; });
      const max = Math.max(1, ...src.flatMap(s => (s.data || []).map(v => Number(v && v.value !== undefined ? v.value : v) || 0)));
      const order = last.map((v, i) => [v, i]).sort((a, b) => b[0] - a[0]);
      for (let k = 1; k < order.length; k++) {
        const [v, i] = order[k], [pv, pi] = order[k-1];
        if (Math.abs(pv - v) < 0.08 * max) {
          const prev = (series[pi].endLabel.offset || [0, 0])[1];
          series[i].endLabel.offset = [0, prev + 14];
        }
      }
    }
    opt.series = series;
    if (kind === 'line' && n > 1) opt.grid = [{right: 110}];
    inst.setOption(opt);
  }
  function applyAll(){ for (const id in META) style(id, META[id]); }
  applyAll();
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyAll);
  new MutationObserver(applyAll).observe(document.documentElement, {attributes:true, attributeFilter:['data-theme']});
  window.addEventListener('resize', () => { for (const id in META) { const i = echarts.getInstanceByDom(document.getElementById(id)); if (i) i.resize(); } });
})();
</script>
</body>
</html>
`))
