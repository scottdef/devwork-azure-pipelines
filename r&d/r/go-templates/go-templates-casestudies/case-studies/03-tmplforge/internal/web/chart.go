package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// ChartID is the DOM id of the latency chart; /chart and /chart.js agree
// on it.
const ChartID = "latency"

// EchartsHost is where the page loads echarts.min.js from. It is local:
// go-echarts' default AssetsHost is a public CDN, which an air-gapped
// deployment cannot reach and a CSP of script-src 'self' forbids anyway.
const EchartsHost = "/static/echarts/"

// latencyChart builds the go-echarts line chart of p50/p95 per window.
func latencyChart(pts []Point) *charts.Line {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			PageTitle:  "tmplforge render latency",
			AssetsHost: EchartsHost,
			ChartID:    ChartID,
			Width:      "100%",
			Height:     "320px",
		}),
		charts.WithTitleOpts(opts.Title{Title: "Render latency (ms)", Subtitle: "sandboxed, incl. process spawn"}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Right: "10%"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "ms"}),
		charts.WithAnimation(false),
	)
	x := make([]string, len(pts))
	p50 := make([]opts.LineData, len(pts))
	p95 := make([]opts.LineData, len(pts))
	for i, p := range pts {
		x[i] = p.At.UTC().Format("15:04:05")
		p50[i] = opts.LineData{Value: ms(p.P50)}
		p95[i] = opts.LineData{Value: ms(p.P95)}
	}
	line.SetXAxis(x).
		AddSeries("p50", p50).
		AddSeries("p95", p95).
		SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{ShowSymbol: opts.Bool(true)}))
	return line
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

// chartJS returns the chart's initialisation script without its <script>
// wrapper, so it can be served from /chart.js under script-src 'self'.
// It is guarded: without echarts.min.js (not vendored; see
// scripts/fetch-echarts.sh) it reveals a notice and the data table still
// carries the numbers.
func chartJS(line *charts.Line) string {
	s := line.RenderSnippet().Script
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, `<script type="text/javascript">`)
	s = strings.TrimSuffix(s, `</script>`)
	return fmt.Sprintf(`if (typeof echarts === "undefined") {
  var n = document.getElementById("chart-missing");
  if (n) n.hidden = false;
} else {
%s
}
`, s)
}

// chartAssets lists the script URLs go-echarts wants, already rebased on
// EchartsHost by the chart's Validate.
func chartAssets(line *charts.Line) []string {
	line.Validate()
	return append([]string(nil), line.JSAssets.Values...)
}
