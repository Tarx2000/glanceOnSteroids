package feed

import (
	"fmt"
	"net/http"
	"time"
)

type NeuralWattSummary struct {
	Period           NeuralWattPeriod     `json:"period"`
	AccountingMethod string              `json:"accounting_method"`
	Totals           NeuralWattTotals    `json:"totals"`
	TimeSeries       []NeuralWattTimePt  `json:"time_series"`
}

type NeuralWattPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type NeuralWattTotals struct {
	Requests          int     `json:"requests"`
	TotalTokens       int     `json:"total_tokens"`
	PromptTokens      int     `json:"prompt_tokens"`
	CompletionTokens  int     `json:"completion_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	EnergyKwhConsumed float64 `json:"energy_kwh_consumed"`
	EnergyKwhCharged  float64 `json:"energy_kwh_charged"`
	EnergyKwh         float64 `json:"energy_kwh"`
	CachedTokens      int     `json:"cached_tokens"`
}

type NeuralWattTimePt struct {
	Date        string  `json:"date"`
	Requests    int     `json:"requests"`
	CostUSD     float64 `json:"cost_usd"`
	TotalTokens int     `json:"total_tokens"`
}

type NeuralWattEnergy struct {
	Period NeuralWattPeriod      `json:"period"`
	Totals NeuralWattEnergyTotal `json:"totals"`
	Daily  []NeuralWattDaily     `json:"daily"`
}

type NeuralWattEnergyTotal struct {
	Requests          int     `json:"requests"`
	RequestsEnergy    int     `json:"requests_with_energy"`
	EnergyKwh         float64 `json:"energy_kwh"`
	EnergyJoules      float64 `json:"energy_joules"`
}

type NeuralWattDaily struct {
	Date          string  `json:"date"`
	Requests      int     `json:"requests"`
	RequestsEnergy int   `json:"requests_with_energy"`
	EnergyKwh     float64 `json:"energy_kwh"`
	EnergyJoules  float64 `json:"energy_joules"`
}

func FetchNeuralWattSummary(apiKey string) (NeuralWattSummary, error) {
	req, err := http.NewRequest("GET", "https://api.neuralwatt.com/v1/usage/summary", nil)
	if err != nil {
		return NeuralWattSummary{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	return decodeJsonFromRequest[NeuralWattSummary](client, req)
}

func FetchNeuralWattEnergy(apiKey string, startDate, endDate string) (NeuralWattEnergy, error) {
	url := fmt.Sprintf(
		"https://api.neuralwatt.com/v1/usage/energy?start_date=%s&end_date=%s",
		startDate, endDate,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return NeuralWattEnergy{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	return decodeJsonFromRequest[NeuralWattEnergy](client, req)
}
