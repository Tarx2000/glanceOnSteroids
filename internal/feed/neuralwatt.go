package feed

import (
	"context"
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

// NeuralWattBaseURL is the base API endpoint URL for NeuralWatt.
// Overridable for testing purposes.
var NeuralWattBaseURL = "https://api.neuralwatt.com/v1"

var neuralwattHTTPClient = &http.Client{Timeout: 10 * time.Second}

// FetchNeuralWattSummary retrieves the 30-day usage summary from NeuralWatt.
func FetchNeuralWattSummary(ctx context.Context, apiKey string) (NeuralWattSummary, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", NeuralWattBaseURL+"/usage/summary", nil)
	if err != nil {
		return NeuralWattSummary{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	return decodeJsonFromRequest[NeuralWattSummary](neuralwattHTTPClient, req)
}

// FetchNeuralWattEnergy retrieves daily energy consumption logs from NeuralWatt.
func FetchNeuralWattEnergy(ctx context.Context, apiKey string, startDate, endDate string) (NeuralWattEnergy, error) {
	url := fmt.Sprintf(
		"%s/usage/energy?start_date=%s&end_date=%s",
		NeuralWattBaseURL, startDate, endDate,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return NeuralWattEnergy{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	return decodeJsonFromRequest[NeuralWattEnergy](neuralwattHTTPClient, req)
}

// NeuralWattQuota represents the current subscription quota and plan details from the /v1/quota endpoint.
type NeuralWattQuota struct {
	SnapshotAt   string                 `json:"snapshot_at"`
	Balance      NeuralWattQuotaBalance `json:"balance"`
	Usage        NeuralWattQuotaUsage   `json:"usage"`
	Limits       NeuralWattQuotaLimits  `json:"limits"`
	Subscription NeuralWattQuotaSub     `json:"subscription"`
}

// NeuralWattQuotaBalance represents account credit balance and accounting method.
type NeuralWattQuotaBalance struct {
	CreditsRemainingUSD float64 `json:"credits_remaining_usd"`
	TotalCreditsUSD     float64 `json:"total_credits_usd"`
	CreditsUsedUSD      float64 `json:"credits_used_usd"`
	AccountingMethod    string  `json:"accounting_method"`
}

// NeuralWattQuotaUsage tracks user lifetime and current month metrics.
type NeuralWattQuotaUsage struct {
	Lifetime     NeuralWattQuotaStats `json:"lifetime"`
	CurrentMonth NeuralWattQuotaStats `json:"current_month"`
}

// NeuralWattQuotaStats represents core usage counters like cost, request count, tokens, and energy.
type NeuralWattQuotaStats struct {
	CostUSD   float64 `json:"cost_usd"`
	Requests  int     `json:"requests"`
	Tokens    int64   `json:"tokens"`
	EnergyKwh float64 `json:"energy_kwh"`
}

// NeuralWattQuotaLimits describes the overage limits and standard rate limit tiers.
type NeuralWattQuotaLimits struct {
	OverageLimitUSD *float64 `json:"overage_limit_usd"`
	RateLimitTier   string   `json:"rate_limit_tier"`
	ConcurrentSlots int      `json:"concurrent_slots"`
}

// NeuralWattQuotaSub details plan status, billing periods, and kilowatt limits/consumption.
type NeuralWattQuotaSub struct {
	Plan               string  `json:"plan"`
	Status             string  `json:"status"`
	BillingInterval    string  `json:"billing_interval"`
	CurrentPeriodStart string  `json:"current_period_start"`
	CurrentPeriodEnd   string  `json:"current_period_end"`
	AutoRenew          bool    `json:"auto_renew"`
	KwhIncluded        float64 `json:"kwh_included"`
	KwhUsed            float64 `json:"kwh_used"`
	KwhRemaining       float64 `json:"kwh_remaining"`
	InOverage          bool    `json:"in_overage"`
}

// FetchNeuralWattQuota retrieves the subscription quota and plan details from NeuralWatt.
func FetchNeuralWattQuota(ctx context.Context, apiKey string) (NeuralWattQuota, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", NeuralWattBaseURL+"/quota", nil)
	if err != nil {
		return NeuralWattQuota{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	return decodeJsonFromRequest[NeuralWattQuota](neuralwattHTTPClient, req)
}

