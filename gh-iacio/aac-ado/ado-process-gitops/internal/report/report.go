// Package report generates interactive HTML reports using go-echarts
// that visualise process drift across environments.
package report

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"

	"github.com/CoolADO/ado-process-gitops/internal/models"
)

// Generator builds go-echarts HTML reports from diff results.
type Generator struct {
	logger *slog.Logger
}

// NewGenerator creates a report generator.
func NewGenerator(logger *slog.Logger) *Generator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Generator{logger: logger}
}

// Generate creates an HTML report from diff results for all environments.
func (g *Generator) Generate(diffs map[string]*models.DiffResult, outputPath string) error {
	g.logger.Info("generating report", "output", outputPath)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}

	page := components.NewPage()
	page.PageTitle = "ADO Process GitOps – Drift Report"

	// Add charts to the page.
	page.AddCharts(
		g.driftSummaryBar(diffs),
		g.changeBreakdownPie(diffs),
		g.witStatusHeatmap(diffs),
		g.detailCategoryRadar(diffs),
	)

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	if err := page.Render(io.MultiWriter(f)); err != nil {
		return fmt.Errorf("render report: %w", err)
	}

	g.logger.Info("report written", "path", outputPath)
	return nil
}

// ---------------------------------------------------------------------------
// Chart: Drift Summary (Grouped Bar)
// ---------------------------------------------------------------------------

func (g *Generator) driftSummaryBar(diffs map[string]*models.DiffResult) *charts.Bar {
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Work Item Type Drift by Environment",
			Subtitle: fmt.Sprintf("Generated %s", time.Now().UTC().Format(time.RFC3339)),
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
	)

	var envNames []string
	var added, modified, removed, unchanged []opts.BarData

	for name, diff := range diffs {
		envNames = append(envNames, name)
		added = append(added, opts.BarData{Value: diff.Summary.Added})
		modified = append(modified, opts.BarData{Value: diff.Summary.Modified})
		removed = append(removed, opts.BarData{Value: diff.Summary.Removed})
		unchanged = append(unchanged, opts.BarData{Value: diff.Summary.Unchanged})
	}

	bar.SetXAxis(envNames).
		AddSeries("Added", added).
		AddSeries("Modified", modified).
		AddSeries("Removed", removed).
		AddSeries("Unchanged", unchanged)

	return bar
}

// ---------------------------------------------------------------------------
// Chart: Change Breakdown (Pie per environment)
// ---------------------------------------------------------------------------

func (g *Generator) changeBreakdownPie(diffs map[string]*models.DiffResult) *charts.Pie {
	pie := charts.NewPie()
	pie.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Change Distribution (All Environments)",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
	)

	var totalAdded, totalModified, totalRemoved, totalUnchanged int
	for _, diff := range diffs {
		totalAdded += diff.Summary.Added
		totalModified += diff.Summary.Modified
		totalRemoved += diff.Summary.Removed
		totalUnchanged += diff.Summary.Unchanged
	}

	items := []opts.PieData{
		{Name: "Added", Value: totalAdded},
		{Name: "Modified", Value: totalModified},
		{Name: "Removed", Value: totalRemoved},
		{Name: "Unchanged", Value: totalUnchanged},
	}

	pie.AddSeries("Changes", items).
		SetSeriesOptions(
			charts.WithLabelOpts(opts.Label{
				Show:      true,
				Formatter: "{b}: {c} ({d}%)",
			}),
		)

	return pie
}

// ---------------------------------------------------------------------------
// Chart: WIT Status Heatmap
// ---------------------------------------------------------------------------

func (g *Generator) witStatusHeatmap(diffs map[string]*models.DiffResult) *charts.HeatMap {
	hm := charts.NewHeatMap()
	hm.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Work Item Type Status Matrix",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
	)

	// Collect unique WIT names and environment names.
	witSet := make(map[string]bool)
	var envNames []string
	for name, diff := range diffs {
		envNames = append(envNames, name)
		for _, wi := range diff.WorkItems {
			witSet[wi.Name] = true
		}
	}
	var witNames []string
	for n := range witSet {
		witNames = append(witNames, n)
	}

	// Build heatmap data: env (x) × wit (y) → status code.
	statusCode := map[string]int{
		"unchanged": 0,
		"modified":  1,
		"added":     2,
		"removed":   3,
	}
	var data []opts.HeatMapData
	for envIdx, envName := range envNames {
		diff := diffs[envName]
		witStatus := make(map[string]string)
		for _, wi := range diff.WorkItems {
			witStatus[wi.Name] = wi.Status
		}
		for witIdx, witName := range witNames {
			code := 0
			if s, ok := witStatus[witName]; ok {
				code = statusCode[s]
			}
			data = append(data, opts.HeatMapData{
				Value: [3]interface{}{envIdx, witIdx, code},
			})
		}
	}

	hm.SetXAxis(envNames)
	hm.AddSeries("status", data)

	return hm
}

// ---------------------------------------------------------------------------
// Chart: Detail Category Radar
// ---------------------------------------------------------------------------

func (g *Generator) detailCategoryRadar(diffs map[string]*models.DiffResult) *charts.Radar {
	radar := charts.NewRadar()
	radar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Change Categories by Environment",
		}),
	)

	// Radar indicators: one axis per change category.
	maxVal := float32(1)
	for _, diff := range diffs {
		for _, v := range []int{
			diff.Summary.FieldChanges,
			diff.Summary.StateChanges,
			diff.Summary.RuleChanges,
			diff.Summary.Added,
			diff.Summary.Modified,
		} {
			if float32(v) > maxVal {
				maxVal = float32(v)
			}
		}
	}

	indicators := []*opts.Indicator{
		{Name: "Field Changes", Max: maxVal},
		{Name: "State Changes", Max: maxVal},
		{Name: "Rule Changes", Max: maxVal},
		{Name: "WITs Added", Max: maxVal},
		{Name: "WITs Modified", Max: maxVal},
	}
	radar.SetGlobalOptions(
		charts.WithRadarComponentOpts(opts.RadarComponent{
			Indicator: indicators,
		}),
	)

	for envName, diff := range diffs {
		data := []opts.RadarData{
			{
				Name: envName,
				Value: []interface{}{
					diff.Summary.FieldChanges,
					diff.Summary.StateChanges,
					diff.Summary.RuleChanges,
					diff.Summary.Added,
					diff.Summary.Modified,
				},
			},
		}
		radar.AddSeries(envName, data)
	}

	return radar
}
