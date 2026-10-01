// Package report writes the HTML parity report (go-echarts).
package report

import (
	"bytes"
	"html/template"
	"os"
	"strings"

	"github.com/coolado/adoclone/internal/state"
	"github.com/coolado/adoclone/internal/verify"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

var tables = template.Must(template.New("t").Parse(`
<div style="max-width:960px;margin:24px auto;font-family:system-ui,sans-serif;font-size:14px">
<h2>{{.R.Source}} &rarr; {{.R.Target}}</h2>
<p>{{if .R.Target}}{{.R.Problems}} problem(s){{else}}Source inventory{{end}}. {{.R.Todos}} manual follow-up(s) in todo.json.</p>
<table style="border-collapse:collapse;width:100%">
<tr><th align="left">Component</th><th align="right">Source</th><th align="right">Target</th><th align="left">Note</th></tr>
{{range .R.Counts}}<tr style="border-top:1px solid #ddd{{if and $.R.Target .Missing}};background:#fdecea{{end}}">
<td>{{.Component}}</td><td align="right">{{.Source}}</td><td align="right">{{if $.R.Target}}{{.Target}}{{end}}</td><td>{{.Note}}</td></tr>
{{end}}</table>
{{with .R.Fields}}<h3>Work item sample: {{.Matched}} of {{.Sampled}} match</h3>
{{if .Mismatches}}<table style="border-collapse:collapse;width:100%">
<tr><th align="left">Source ID</th><th align="left">Target ID</th><th align="left">Differs</th></tr>
{{range .Mismatches}}<tr style="border-top:1px solid #ddd"><td>{{.Source}}</td><td>{{.Target}}</td><td>{{range $i, $f := .Fields}}{{if $i}}, {{end}}{{$f}}{{end}}</td></tr>
{{end}}</table>{{end}}{{end}}
{{if .Todos}}<h3>Manual follow-ups</h3>
<table style="border-collapse:collapse;width:100%">
<tr><th align="left">Component</th><th align="left">Item</th><th align="left">Action</th></tr>
{{range .Todos}}<tr style="border-top:1px solid #ddd"><td>{{.Component}}</td><td>{{.Item}}</td><td>{{.Action}}</td></tr>
{{end}}</table>{{end}}
</div>`))

// Write renders counts (bar), the work item sample (pie) and the tables.
func Write(path string, r verify.Result, todos []state.Todo) error {
	page := components.NewPage()
	page.PageTitle = "adoclone parity"

	bar := charts.NewBar()
	title := "adoclone: " + r.Source
	if r.Target != "" {
		title += " vs " + r.Target
	}
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: title}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
		charts.WithLegendOpts(opts.Legend{Show: true, Right: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "960px", Height: "480px"}),
		charts.WithXAxisOpts(opts.XAxis{AxisLabel: &opts.AxisLabel{Show: true, Rotate: 35, Interval: "0"}}),
	)
	var x []string
	var s, t []opts.BarData
	for _, row := range r.Counts {
		x = append(x, row.Component)
		s = append(s, opts.BarData{Value: row.Source})
		t = append(t, opts.BarData{Value: row.Target})
	}
	bar.SetXAxis(x).AddSeries("source", s)
	if r.Target != "" {
		bar.AddSeries("target", t)
	}
	page.AddCharts(bar)

	if r.Fields != nil && r.Fields.Sampled > 0 {
		pie := charts.NewPie()
		pie.SetGlobalOptions(
			charts.WithTitleOpts(opts.Title{Title: "Work item sample"}),
			charts.WithTooltipOpts(opts.Tooltip{Show: true}),
			charts.WithInitializationOpts(opts.Initialization{Width: "960px", Height: "360px"}),
		)
		pie.AddSeries("sample", []opts.PieData{
			{Name: "match", Value: r.Fields.Matched},
			{Name: "differs", Value: r.Fields.Sampled - r.Fields.Matched},
		})
		page.AddCharts(pie)
	}

	var buf bytes.Buffer
	if err := page.Render(&buf); err != nil {
		return err
	}
	var extra bytes.Buffer
	if err := tables.Execute(&extra, map[string]any{"R": r, "Todos": todos}); err != nil {
		return err
	}
	html := strings.Replace(buf.String(), "</body>", extra.String()+"</body>", 1)
	return os.WriteFile(path, []byte(html), 0o644)
}
