package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCodexAuthFile(t *testing.T) {
	tempDir := t.TempDir()
	authPath := filepath.Join(tempDir, "auth.json")

	content := `{
		"auth_mode": "chatgpt",
		"tokens": {
			"access_token": "secret-test-token-123",
			"refresh_token": "refresh-123",
			"account_id": "acc-456"
		}
	}`
	if err := os.WriteFile(authPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test auth file: %v", err)
	}

	tok, acc, err := ReadCodexAuthFile(authPath)
	if err != nil {
		t.Fatalf("unexpected error reading auth file: %v", err)
	}
	if tok != "secret-test-token-123" {
		t.Errorf("expected token 'secret-test-token-123', got '%s'", tok)
	}
	if acc != "acc-456" {
		t.Errorf("expected account 'acc-456', got '%s'", acc)
	}
}

func TestFetchCodexUsageSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("ChatGPT-Account-Id") != "test-acc" {
			http.Error(w, "Bad Account", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"plan_type": "pro",
			"rate_limit": {
				"primary_window": {
					"used_percent": 25.5,
					"limit_window_seconds": 18000,
					"reset_at": 1783440000,
					"reset_after_seconds": 7200
				},
				"secondary_window": {
					"used_percent": 12.0,
					"limit_window_seconds": 604800,
					"reset_at": 1783440000,
					"reset_after_seconds": 532000
				}
			},
			"credits": {
				"balance": 15.50,
				"unlimited": false
			}
		}`))
	}))
	defer server.Close()

	ctx := context.Background()
	resp, err := FetchCodexUsage(ctx, server.Client(), server.URL, "valid-token", "test-acc")
	if err != nil {
		t.Fatalf("FetchCodexUsage failed: %v", err)
	}

	if resp.PlanType != "pro" {
		t.Errorf("expected plan_type 'pro', got '%s'", resp.PlanType)
	}
	if resp.RateLimit == nil || resp.RateLimit.PrimaryWindow == nil {
		t.Fatalf("expected rate_limit.primary_window to be present")
	}
	if resp.RateLimit.PrimaryWindow.UsedPercent != 25.5 {
		t.Errorf("expected primary used_percent 25.5, got %v", resp.RateLimit.PrimaryWindow.UsedPercent)
	}
	if resp.RateLimit.SecondaryWindow.UsedPercent != 12.0 {
		t.Errorf("expected secondary used_percent 12.0, got %v", resp.RateLimit.SecondaryWindow.UsedPercent)
	}
	if resp.Credits == nil || resp.Credits.Balance != 15.50 {
		t.Errorf("expected credits balance 15.50, got %v", resp.Credits)
	}
}

func TestFetchCodexUsageErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/401":
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case "/429":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{
				"plan_type": "plus",
				"rate_limit": {
					"primary_window": {
						"used_percent": 100,
						"limit_window_seconds": 18000,
						"reset_after_seconds": 3600
					}
				}
			}`))
		default:
			http.Error(w, "Server Error", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	ctx := context.Background()

	// 401
	_, err := FetchCodexUsage(ctx, server.Client(), server.URL+"/401", "bad-token", "")
	if err == nil {
		t.Errorf("expected error on 401, got nil")
	}

	// 429 returns payload with rate limit if present
	resp, err := FetchCodexUsage(ctx, server.Client(), server.URL+"/429", "token", "")
	if err != nil {
		t.Fatalf("expected successful recovery of 429 rate limit data, got error: %v", err)
	}
	if resp.RateLimit.PrimaryWindow.UsedPercent != 100 {
		t.Errorf("expected primary used_percent 100, got %v", resp.RateLimit.PrimaryWindow.UsedPercent)
	}

	// 500
	_, err = FetchCodexUsage(ctx, server.Client(), server.URL+"/500", "token", "")
	if err == nil {
		t.Errorf("expected error on 500, got nil")
	}
}
