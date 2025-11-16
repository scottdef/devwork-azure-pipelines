package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// ============================================================================
// GitHub Audit Log Monitoring - Dashboard Visualization
// ============================================================================
// Purpose: Generate visual dashboards for GitHub audit event metrics
// Go Version: 1.21
// External Package: go-echarts (allowed)
// Usage: go run dashboard.go
// ============================================================================

// MetricData represents time-series metric data
type MetricData struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	EventType string    `json:"event_type"`
	Org       string    `json:"organization"`
	Actor     string    `json:"actor"`
}

// DashboardGenerator creates visual dashboards
type DashboardGenerator struct {
	DynatraceURL string
	APIToken     string
	HTTPClient   *http.Client
}

// NewDashboardGenerator creates a new dashboard generator
func NewDashboardGenerator(url, token string) *DashboardGenerator {
	return &DashboardGenerator{
		DynatraceURL: url,
		APIToken:     token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchMetricData retrieves metric data from Dynatrace
func (dg *DashboardGenerator) FetchMetricData(metricSelector string) ([]MetricData, error) {
	endpoint := fmt.Sprintf("%s/api/v2/metrics/query?metricSelector=%s&from=now-24h", 
		dg.DynatraceURL, metricSelector)
	
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Api-Token %s", dg.APIToken))
	
	resp, err := dg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	
	// Parse and transform data
	var metrics []MetricData
	// Note: Actual parsing would depend on Dynatrace API response format
	
	return metrics, nil
}

// CreateEventCountChart creates a line chart for event counts over time
func (dg *DashboardGenerator) CreateEventCountChart() *charts.Line {
	line := charts.NewLine()
	
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "GitHub Security Events - Last 24 Hours",
			Subtitle: "Credential Grants and Token Events",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
		charts.WithDataZoomOpts(opts.DataZoom{
			Type:  "slider",
			Start: 0,
			End:   100,
		}),
	)
	
	// Generate sample time series (in production, fetch from Dynatrace)
	xAxis := make([]string, 24)
	credentialGrants := make([]opts.LineData, 24)
	tokenRequests := make([]opts.LineData, 24)
	tokenGrants := make([]opts.LineData, 24)
	
	for i := 0; i < 24; i++ {
		t := time.Now().Add(time.Duration(-24+i) * time.Hour)
		xAxis[i] = t.Format("15:04")
		
		// Sample data (replace with actual Dynatrace data)
		credentialGrants[i] = opts.LineData{Value: 0}
		tokenRequests[i] = opts.LineData{Value: 0}
		tokenGrants[i] = opts.LineData{Value: 0}
	}
	
	line.SetXAxis(xAxis).
		AddSeries("Credential Grants", credentialGrants,
			charts.WithLineChartOpts(opts.LineChart{
				Smooth: true,
			}),
			charts.WithMarkPointNameTypeItemOpts(
				opts.MarkPointNameTypeItem{Name: "Max", Type: "max"},
			),
			charts.WithItemStyleOpts(opts.ItemStyle{
				Color: "#FF4444",
			}),
		).
		AddSeries("Token Requests", tokenRequests,
			charts.WithLineChartOpts(opts.LineChart{
				Smooth: true,
			}),
			charts.WithItemStyleOpts(opts.ItemStyle{
				Color: "#FFA500",
			}),
		).
		AddSeries("Token Grants", tokenGrants,
			charts.WithLineChartOpts(opts.LineChart{
				Smooth: true,
			}),
			charts.WithItemStyleOpts(opts.ItemStyle{
				Color: "#FF6B6B",
			}),
		)
	
	return line
}

// CreateEventDistributionPie creates a pie chart for event distribution
func (dg *DashboardGenerator) CreateEventDistributionPie() *charts.Pie {
	pie := charts.NewPie()
	
	pie.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Event Distribution by Type",
			Subtitle: "Last 30 Days",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
		charts.WithLegendOpts(opts.Legend{
			Show:   true,
			Orient: "vertical",
			Left:   "left",
		}),
	)
	
	// Sample data (replace with actual Dynatrace metrics)
	items := []opts.PieData{
		{Name: "org_credential_authorization.grant", Value: 5},
		{Name: "personal_access_token.request_created", Value: 23},
		{Name: "personal_access_token.access_granted", Value: 12},
	}
	
	pie.AddSeries("Events", items).
		SetSeriesOptions(
			charts.WithLabelOpts(opts.Label{
				Show:      true,
				Formatter: "{b}: {c}",
			}),
		)
	
	return pie
}

// CreateTopActorsBar creates a bar chart for top actors
func (dg *DashboardGenerator) CreateTopActorsBar() *charts.Bar {
	bar := charts.NewBar()
	
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Top 10 Actors by Event Count",
			Subtitle: "Last 7 Days",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
		charts.WithXAxisOpts(opts.XAxis{
			Name: "Actor",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Name: "Event Count",
		}),
	)
	
	// Sample data (replace with actual Dynatrace data)
	actors := []string{"user1", "user2", "user3", "user4", "user5", 
		"user6", "user7", "user8", "user9", "user10"}
	
	credentialEvents := make([]opts.BarData, 10)
	tokenEvents := make([]opts.BarData, 10)
	
	for i := 0; i < 10; i++ {
		credentialEvents[i] = opts.BarData{Value: 0}
		tokenEvents[i] = opts.BarData{Value: 0}
	}
	
	bar.SetXAxis(actors).
		AddSeries("Credential Events", credentialEvents).
		AddSeries("Token Events", tokenEvents)
	
	return bar
}

// CreateOrganizationHeatmap creates a heatmap for organization activity
func (dg *DashboardGenerator) CreateOrganizationHeatmap() *charts.HeatMap {
	heatmap := charts.NewHeatMap()
	
	heatmap.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Organization Activity Heatmap",
			Subtitle: "Events per Hour per Organization",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: true}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Calculable: true,
			Max:        10,
			Min:        0,
			InRange: &opts.VisualMapInRange{
				Color: []string{"#50a3ba", "#eac736", "#d94e5d"},
			},
		}),
	)
	
	// Sample data
	hours := []string{"00:00", "01:00", "02:00", "03:00", "04:00", "05:00",
		"06:00", "07:00", "08:00", "09:00", "10:00", "11:00",
		"12:00", "13:00", "14:00", "15:00", "16:00", "17:00",
		"18:00", "19:00", "20:00", "21:00", "22:00", "23:00"}
	
	orgs := []string{"org-1", "org-2", "org-3", "org-4", "org-5"}
	
	data := make([]opts.HeatMapData, 0)
	for i, hour := range hours {
		for j, org := range orgs {
			// Sample values (replace with actual data)
			value := 0
			data = append(data, opts.HeatMapData{
				Value: [3]interface{}{i, j, value},
			})
		}
	}
	
	heatmap.SetXAxis(hours).
		SetYAxis(orgs).
		AddSeries("events", data)
	
	return heatmap
}

// CreateTimelineGauge creates a gauge for current threat level
func (dg *DashboardGenerator) CreateThreatLevelGauge() *charts.Gauge {
	gauge := charts.NewGauge()
	
	gauge.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Current Security Threat Level",
		}),
	)
	
	// Calculate threat level based on recent events (sample data)
	threatLevel := 0.0 // 0-100 scale
	
	gauge.AddSeries("threat", []opts.GaugeData{
		{Name: "Threat Level", Value: threatLevel},
	}).
		SetSeriesOptions(
			charts.WithGaugeOpts(opts.Gauge{
				Min: 0,
				Max: 100,
				SplitNumber: 10,
				Detail: &opts.GaugeDetail{
					Formatter: "{value}%",
				},
			}),
		)
	
	return gauge
}

// GenerateDashboard creates the complete dashboard
func (dg *DashboardGenerator) GenerateDashboard(outputFile string) error {
	page := components.NewPage()
	
	page.PageTitle = "GitHub Audit Log Security Dashboard"
	
	page.AddCharts(
		dg.CreateEventCountChart(),
		dg.CreateEventDistributionPie(),
		dg.CreateTopActorsBar(),
		dg.CreateOrganizationHeatmap(),
		dg.CreateThreatLevelGauge(),
	)
	
	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()
	
	return page.Render(io.MultiWriter(f))
}

func main() {
	fmt.Println("GitHub Audit Log Monitoring - Dashboard Generator")
	fmt.Println("="*80)
	
	dynatraceURL := os.Getenv("DYNATRACE_URL")
	apiToken := os.Getenv("DYNATRACE_API_TOKEN")
	
	if dynatraceURL == "" || apiToken == "" {
		fmt.Println("Error: DYNATRACE_URL and DYNATRACE_API_TOKEN must be set")
		os.Exit(1)
	}
	
	generator := NewDashboardGenerator(dynatraceURL, apiToken)
	
	outputFile := "github-audit-dashboard.html"
	
	fmt.Println("Generating dashboard...")
	
	if err := generator.GenerateDashboard(outputFile); err != nil {
		fmt.Printf("Error generating dashboard: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Dashboard generated successfully: %s\n", outputFile)
	fmt.Println("\nOpen the file in your web browser to view the dashboard.")
}
