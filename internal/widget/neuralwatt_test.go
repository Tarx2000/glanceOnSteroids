package widget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glanceapp/glance/internal/feed"
)

// TestNeuralWattWidgetQuotaView verifies that the NeuralWatt widget correctly handles
// the "quota" view option, fetching from the /v1/quota endpoint and computing stats.
func TestNeuralWattWidgetQuotaView(t *testing.T) {
	// 1. Setup local mock HTTP server to return the quota API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quota" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		mockResponse := map[string]any{
			"snapshot_at": "2026-06-19T14:47:59Z",
			"balance": map[string]any{
				"credits_remaining_usd": 32.6774,
				"total_credits_usd":     52.34,
				"credits_used_usd":      19.6626,
				"accounting_method":    "energy",
			},
			"usage": map[string]any{
				"lifetime": map[string]any{
					"cost_usd":   243.9145,
					"requests":   37801,
					"tokens":     1235477176,
					"energy_kwh": 15.6009,
				},
				"current_month": map[string]any{
					"cost_usd":   160.1463,
					"requests":   23902,
					"tokens":     1116658995,
					"energy_kwh": 9.7278,
				},
			},
			"limits": map[string]any{
				"overage_limit_usd": nil,
				"rate_limit_tier":   "Basic Tier",
				"concurrent_slots":   3,
			},
			"subscription": map[string]any{
				"plan":                 "Basic",
				"status":               "active",
				"billing_interval":     "month",
				"current_period_start": "2026-06-18T00:00:00Z",
				"current_period_end":   "2026-07-18T00:00:00Z",
				"auto_renew":           true,
				"kwh_included":         6.0,
				"kwh_used":             0.563,
				"kwh_remaining":        5.437,
				"in_overage":           false,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	// Backup original API URL and override it with our local mock server URL
	oldURL := feed.NeuralWattBaseURL
	feed.NeuralWattBaseURL = server.URL
	defer func() { feed.NeuralWattBaseURL = oldURL }()

	// Test Case 1: API Key empty validation
	nwWidget := &NeuralWatt{
		ApiKey:             "",
		View:               "quota",
		UpdateIntervalMins: 10,
	}
	err := nwWidget.Initialize()
	if err == nil {
		t.Error("expected error for empty ApiKey, got nil")
	}

	// Test Case 2: Successful fetch and property mapping
	nwWidget = &NeuralWatt{
		ApiKey:             "sk-testkey",
		View:               "quota",
		UpdateIntervalMins: 10,
	}
	err = nwWidget.Initialize()
	if err != nil {
		t.Errorf("expected no error during init, got: %v", err)
	}

	nwWidget.Update(context.Background(), nil)

	if nwWidget.Error != nil {
		t.Errorf("expected no update error, got: %v", nwWidget.Error)
	}

	if nwWidget.Quota == nil {
		t.Fatal("expected Quota data to be populated, got nil")
	}

	// Verify quota metrics parsing
	if nwWidget.Quota.Subscription.Plan != "Basic" {
		t.Errorf("expected plan Basic, got: %s", nwWidget.Quota.Subscription.Plan)
	}

	if nwWidget.Quota.Subscription.KwhIncluded != 6.0 {
		t.Errorf("expected included kWh 6.0, got: %f", nwWidget.Quota.Subscription.KwhIncluded)
	}

	if nwWidget.Quota.Subscription.KwhRemaining != 5.437 {
		t.Errorf("expected remaining kWh 5.437, got: %f", nwWidget.Quota.Subscription.KwhRemaining)
	}

	// Verify formatted billing period
	expectedBillingPeriod := "18.06.2026 – 18.07.2026"
	if nwWidget.QuotaBillingPeriodStr != expectedBillingPeriod {
		t.Errorf("expected billing period %q, got: %q", expectedBillingPeriod, nwWidget.QuotaBillingPeriodStr)
	}

	// Verify calculated percentages (used = 0.563 / 6.0 * 100 = 9.3833...%)
	if nwWidget.QuotaPercentUsed < 9.38 || nwWidget.QuotaPercentUsed > 9.39 {
		t.Errorf("expected percent used around 9.38%%, got: %f", nwWidget.QuotaPercentUsed)
	}

	if nwWidget.QuotaPercentLeft < 90.61 || nwWidget.QuotaPercentLeft > 90.62 {
		t.Errorf("expected percent left around 90.61%%, got: %f", nwWidget.QuotaPercentLeft)
	}

	if nwWidget.QuotaPercentLeftScale < 0.9061 || nwWidget.QuotaPercentLeftScale > 0.9062 {
		t.Errorf("expected percent left scale around 0.9061, got: %f", nwWidget.QuotaPercentLeftScale)
	}

	// Verify remaining days calculation relative to current time.
	// Since mock current_period_end is "2026-07-18T00:00:00Z" and current test time is 2026-06-19,
	// days remaining should be around 28-29 days.
	if nwWidget.QuotaDaysRemaining <= 0 {
		t.Errorf("expected positive remaining days, got: %d", nwWidget.QuotaDaysRemaining)
	}
}
