package widget

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type NeuralWattDailyBar struct {
	DateLabel   string
	HeightPct   float64
	HeightScale float64
	Requests    int
	CostUSD     float64
}

type NeuralWattEnergyBar struct {
	DateLabel   string
	HeightPct   float64
	HeightScale float64
}

type NeuralWatt struct {
	widgetBase          `yaml:",inline"`
	ApiKey             OptionalEnvString       `yaml:"api-key"`
	UpdateIntervalMins  int                     `yaml:"update-interval"`
	Summary            *feed.NeuralWattSummary  `yaml:"-"`
	Energy             *feed.NeuralWattEnergy   `yaml:"-"`
	DailyChartData     []NeuralWattDailyBar     `yaml:"-"`
	EnergyChartData    []NeuralWattEnergyBar    `yaml:"-"`
	TodayCost          float64                  `yaml:"-"`
	TodayRequests       int                     `yaml:"-"`
	TodayEnergyKwh     float64                  `yaml:"-"`
	TodayTokens        int                     `yaml:"-"`
	EstimatedTokenCost float64                  `yaml:"-"`
	TodayDateLabel     string                   `yaml:"-"`
	NoticeMessage      string                   `yaml:"-"`
}

// Configurable pricing parameters (USD per 1,000,000 tokens)
// These variables allow customization of LLM API costs for input and output tokens.
var (
	// PromptTokenPriceUSD is the cost in USD per million prompt (input) tokens.
	PromptTokenPriceUSD = 0.50
	// CompletionTokenPriceUSD is the cost in USD per million completion (output) tokens.
	CompletionTokenPriceUSD = 2.00
)

// Initialize configures the title and the custom cache duration (update interval)
// for the NeuralWatt widget, falling back to 15 minutes if not specified.
func (widget *NeuralWatt) Initialize() error {
	widget.withTitle("NeuralWatt")

	// Set cache duration based on update-interval settings
	interval := 15
	if widget.UpdateIntervalMins > 0 {
		interval = widget.UpdateIntervalMins
	}
	widget.withCacheDuration(time.Duration(interval) * time.Minute)

	if widget.ApiKey == "" {
		return fmt.Errorf("neuralwatt widget requires an api-key")
	}

	return nil
}

// Update retrieves the summary and energy data from the NeuralWatt service,
// processes it using local variables, and updates the widget state under a mutex lock
// to prevent concurrent data access races.
func (widget *NeuralWatt) Update(ctx context.Context) {
	apiKey := string(widget.ApiKey)

	summary, err := feed.FetchNeuralWattSummary(apiKey)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	// Load the global timezone configured in System Settings. If empty or invalid,
	// fall back to UTC time.
	loc := time.UTC
	if GlobalTimezone != "" {
		if l, err := time.LoadLocation(GlobalTimezone); err == nil {
			loc = l
		} else {
			slog.Error("failed to load global timezone", "timezone", GlobalTimezone, "error", err)
		}
	}

	// Format the "Today" date label in the user's configured local timezone
	// (e.g., "Jun 11" instead of the active UTC day "Jun 10").
	nowLocal := time.Now().In(loc)
	todayDateLabel := nowLocal.Format("Jan 02")

	// Check if the current local time falls into the 2-hour crossover window (00:00 to 02:00 local time),
	// and set a short informational notice that the daily stats update at 2:00 AM (00:00 UTC).
	localHour := nowLocal.Hour()
	var noticeMessage string
	if localHour >= 0 && localHour < 2 {
		noticeMessage = "Stats update at 2:00 AM local (00:00 UTC)"
	}

	// Query and match metrics based on the active UTC day because the NeuralWatt API
	// groups all timeseries and daily usage data strictly by UTC dates.
	nowUTC := time.Now().UTC()
	endDate := nowUTC.Format("2006-01-02")
	startDate := nowUTC.AddDate(0, 0, -30).Format("2006-01-02")

	energy, err := feed.FetchNeuralWattEnergy(apiKey, startDate, endDate)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	today := nowUTC.Format("2006-01-02")
	todayCost := 0.0
	todayRequests := 0
	todayTokens := 0
	todayEnergyKwh := 0.0
	computedTotalCost := 0.0

	// Calculate today's metrics and total cost from timeseries data
	for _, d := range summary.TimeSeries {
		computedTotalCost += d.CostUSD
		if d.Date == today {
			todayCost = d.CostUSD
			todayRequests = d.Requests
			todayTokens = d.TotalTokens
		}
	}
	if computedTotalCost > 0 {
		slog.Info("[NeuralWatt] cost comparison", "api_total", summary.Totals.TotalCostUSD, "computed_total", computedTotalCost, "api_period_start", summary.Period.Start, "api_period_end", summary.Period.End, "time_series_days", len(summary.TimeSeries))
		summary.Totals.TotalCostUSD = computedTotalCost
	}

	// Calculate today's energy metrics from daily energy logs
	for _, d := range energy.Daily {
		if d.Date == today {
			todayEnergyKwh = d.EnergyKwh
			break
		}
	}

	// Calculate estimated token cost using configurable pricing variables
	inputCost := float64(summary.Totals.PromptTokens-summary.Totals.CachedTokens) * PromptTokenPriceUSD / 1_000_000
	outputCost := float64(summary.Totals.CompletionTokens) * CompletionTokenPriceUSD / 1_000_000
	estimatedTokenCost := inputCost + outputCost

	maxRequests := 1
	for _, d := range summary.TimeSeries {
		if d.Requests > maxRequests {
			maxRequests = d.Requests
		}
	}

	// Format daily chart data
	dailyChartData := make([]NeuralWattDailyBar, len(summary.TimeSeries))
	for i, d := range summary.TimeSeries {
		t, _ := time.Parse("2006-01-02", d.Date)
		label := t.Format("Jan 02")
		hpct := math.Round(float64(d.Requests) / float64(maxRequests) * 100)
		dailyChartData[i] = NeuralWattDailyBar{
			DateLabel:   label,
			HeightPct:   hpct,
			HeightScale: hpct / 100.0,
			Requests:    d.Requests,
			CostUSD:     d.CostUSD,
		}
	}

	// Format energy chart data if available
	var energyChartData []NeuralWattEnergyBar
	if len(energy.Daily) > 0 {
		maxKwh := 0.0
		for _, d := range energy.Daily {
			if d.EnergyKwh > maxKwh {
				maxKwh = d.EnergyKwh
			}
		}
		if maxKwh == 0 {
			maxKwh = 1
		}

		energyChartData = make([]NeuralWattEnergyBar, len(energy.Daily))
		for i, d := range energy.Daily {
			t, _ := time.Parse("2006-01-02", d.Date)
			label := t.Format("Jan 02")
			hpct := math.Round(d.EnergyKwh / maxKwh * 100)
			energyChartData[i] = NeuralWattEnergyBar{
				DateLabel:   label,
				HeightPct:   hpct,
				HeightScale: hpct / 100.0,
			}
		}
	}

	// Update the shared widget state with thread safety
	widget.Lock()
	widget.Summary = &summary
	if len(energy.Daily) > 0 || energy.Totals.EnergyKwh > 0 {
		widget.Energy = &energy
	} else {
		widget.Energy = nil
	}
	widget.DailyChartData = dailyChartData
	widget.EnergyChartData = energyChartData
	widget.TodayCost = todayCost
	widget.TodayRequests = todayRequests
	widget.TodayEnergyKwh = todayEnergyKwh
	widget.TodayTokens = todayTokens
	widget.EstimatedTokenCost = estimatedTokenCost
	widget.TodayDateLabel = todayDateLabel
	widget.NoticeMessage = noticeMessage
	widget.Unlock()

	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *NeuralWatt) Render() template.HTML {
	return widget.render(widget, assets.NeuralWattTemplate)
}
