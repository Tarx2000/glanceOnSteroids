package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultCodexEndpoint is the official ChatGPT web/Codex internal telemetry usage endpoint.
const DefaultCodexEndpoint = "https://chatgpt.com/backend-api/wham/usage"

// CodexHTTPClient is the HTTP client used for fetching Codex usage data.
var CodexHTTPClient = &http.Client{Timeout: 10 * time.Second}

// CodexWindow represents a quota tracking window (e.g. 5-hour primary or 7-day secondary).
type CodexWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAt            int64   `json:"reset_at"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
}

// CodexRateLimit contains the primary (short-term) and secondary (weekly) rate-limit windows.
type CodexRateLimit struct {
	PrimaryWindow   *CodexWindow `json:"primary_window"`
	SecondaryWindow *CodexWindow `json:"secondary_window"`
}

// CodexCredits details account credits and balance.
type CodexCredits struct {
	Balance   any  `json:"balance"` // Can be numeric or string
	Unlimited bool `json:"unlimited"`
}

// CodexUsageResponse represents the payload returned by https://chatgpt.com/backend-api/wham/usage.
type CodexUsageResponse struct {
	PlanType  string          `json:"plan_type"`
	RateLimit *CodexRateLimit `json:"rate_limit"`
	Credits   *CodexCredits   `json:"credits"`
}

// CodexAuthFile represents the JSON structure of ~/.codex/auth.json.
type CodexAuthFile struct {
	AuthMode string `json:"auth_mode"`
	Tokens   struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id"`
}

// ExpandPath resolves tilde (~) to the user's home directory.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine user home dir: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ReadCodexAuthFile reads and parses a Codex auth.json file, extracting the access token and optional account ID.
func ReadCodexAuthFile(filePath string) (token string, accountID string, err error) {
	if filePath == "" {
		filePath = "~/.codex/auth.json"
	}
	expanded, err := ExpandPath(filePath)
	if err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(expanded)
	if err != nil {
		return "", "", fmt.Errorf("failed to read Codex auth file (%s): %w", expanded, err)
	}

	var auth CodexAuthFile
	if err := json.Unmarshal(data, &auth); err != nil {
		return "", "", fmt.Errorf("failed to parse Codex auth file (%s): %w", expanded, err)
	}

	tok := auth.Tokens.AccessToken
	if tok == "" {
		tok = auth.AccessToken
	}
	if tok == "" {
		return "", "", fmt.Errorf("no access_token found in Codex auth file (%s)", expanded)
	}

	acc := auth.Tokens.AccountID
	if acc == "" {
		acc = auth.AccountID
	}

	return tok, acc, nil
}

// FetchCodexUsage fetches real-time Codex subscription rate limit and quota information.
func FetchCodexUsage(ctx context.Context, client RequestDoer, endpoint string, token string, accountID string) (*CodexUsageResponse, error) {
	if endpoint == "" {
		endpoint = DefaultCodexEndpoint
	}
	if client == nil {
		client = CodexHTTPClient
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GlanceDashboard/1.0 (CodexQuotaWidget)")
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Codex usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("OpenAI Codex unauthorized (HTTP 401): access token is invalid or expired")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		// Some 429 responses still include JSON payload with reset times
		var quotaResp CodexUsageResponse
		if json.Unmarshal(body, &quotaResp) == nil && quotaResp.RateLimit != nil {
			return &quotaResp, nil
		}
		return nil, fmt.Errorf("OpenAI Codex rate limit exceeded (HTTP 429)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI Codex API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var quotaResp CodexUsageResponse
	if err := json.Unmarshal(body, &quotaResp); err != nil {
		return nil, fmt.Errorf("failed to parse Codex usage JSON: %w", err)
	}

	return &quotaResp, nil
}
