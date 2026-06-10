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
	DateLabel  string
	HeightPct  float64
	Requests   int
	CostUSD    float64
}

type NeuralWattEnergyBar struct {
	DateLabel string
	HeightPct float64
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
}

func (widget *NeuralWatt) Initialize() error {
	widget.withTitle("NeuralWatt")

	cacheMins := 15
	if widget.UpdateIntervalMins > 0 {
		cacheMins = widget.UpdateIntervalMins
	}
	widget.withCacheDuration(time.Duration(cacheMins) * time.Minute)

	if widget.ApiKey == "" {
		return fmt.Errorf("neuralwatt widget requires an api-key")
	}

	return nil
}

func (widget *NeuralWatt) Update(ctx context.Context) {
	apiKey := string(widget.ApiKey)

	summary, err := feed.FetchNeuralWattSummary(apiKey)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}
	widget.Summary = &summary

	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	energy, err := feed.FetchNeuralWattEnergy(apiKey, startDate, endDate)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}
	widget.Energy = &energy

	today := time.Now().Format("2006-01-02")
	widget.TodayCost = 0
	widget.TodayRequests = 0
	widget.TodayTokens = 0
	widget.TodayEnergyKwh = 0
	computedTotalCost := 0.0
	for _, d := range summary.TimeSeries {
		computedTotalCost += d.CostUSD
		if d.Date == today {
			widget.TodayCost = d.CostUSD
			widget.TodayRequests = d.Requests
			widget.TodayTokens = d.TotalTokens
		}
	}
	if computedTotalCost > 0 {
		slog.Info("[NeuralWatt] cost comparison", "api_total", summary.Totals.TotalCostUSD, "computed_total", computedTotalCost, "api_period_start", summary.Period.Start, "api_period_end", summary.Period.End, "time_series_days", len(summary.TimeSeries))
		widget.Summary.Totals.TotalCostUSD = computedTotalCost
	}

	if widget.Energy != nil {
		for _, d := range widget.Energy.Daily {
			if d.Date == today {
				widget.TodayEnergyKwh = d.EnergyKwh
				break
			}
		}
	}

	inputCost := float64(summary.Totals.PromptTokens-summary.Totals.CachedTokens) * 0.50 / 1_000_000
	outputCost := float64(summary.Totals.CompletionTokens) * 2.00 / 1_000_000
	widget.EstimatedTokenCost = inputCost + outputCost

	maxRequests := 1
	for _, d := range summary.TimeSeries {
		if d.Requests > maxRequests {
			maxRequests = d.Requests
		}
	}

	widget.DailyChartData = make([]NeuralWattDailyBar, len(summary.TimeSeries))
	for i, d := range summary.TimeSeries {
		t, _ := time.Parse("2006-01-02", d.Date)
		label := t.Format("Jan 02")
		hpct := math.Round(float64(d.Requests) / float64(maxRequests) * 100)
		widget.DailyChartData[i] = NeuralWattDailyBar{
			DateLabel: label,
			HeightPct: hpct,
			Requests:  d.Requests,
			CostUSD:   d.CostUSD,
		}
	}

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

		widget.EnergyChartData = make([]NeuralWattEnergyBar, len(energy.Daily))
		for i, d := range energy.Daily {
			t, _ := time.Parse("2006-01-02", d.Date)
			label := t.Format("Jan 02")
			hpct := math.Round(d.EnergyKwh / maxKwh * 100)
			widget.EnergyChartData[i] = NeuralWattEnergyBar{
				DateLabel: label,
				HeightPct: hpct,
			}
		}
	}

	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *NeuralWatt) Render() template.HTML {
	return widget.render(widget, assets.NeuralWattTemplate)
}
