package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HermesRequest struct {
	ID            string    `json:"id"`
	Origin        string    `json:"origin"`
	Kind          string    `json:"kind"`
	Title         string    `json:"title"`
	Prompt        string    `json:"prompt"`
	SideEffecting bool      `json:"sideEffecting"`
	Status        string    `json:"status"`
	Result        *string   `json:"result,omitempty"`
	Error         *string   `json:"error,omitempty"`
	HermesTaskId  *string   `json:"hermesTaskId,omitempty"`
	DecidedAt     *string   `json:"decidedAt,omitempty"`
	StartedAt     *string   `json:"startedAt,omitempty"`
	FinishedAt    *string   `json:"finishedAt,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	TimeAgo       string    `json:"timeAgo,omitempty"`
}

func (r *HermesRequest) UnmarshalJSON(data []byte) error {
	type Alias HermesRequest
	aux := &struct {
		CreatedAtRaw any `json:"createdAt"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.CreatedAtRaw != nil {
		switch v := aux.CreatedAtRaw.(type) {
		case string:
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				r.CreatedAt = t
			} else if t, err := time.Parse(time.RFC3339, v); err == nil {
				r.CreatedAt = t
			} else if t, err := time.Parse("2006-01-02T15:04:05", v); err == nil {
				r.CreatedAt = t
			}
		case float64:
			r.CreatedAt = time.UnixMilli(int64(v))
		}
	}

	if !r.CreatedAt.IsZero() {
		r.TimeAgo = FormatRelativeTime(r.CreatedAt)
	}

	return nil
}

type HermesRequestsResponse struct {
	Requests []HermesRequest `json:"requests"`
	Pending  int             `json:"pending"`
}

func FormatRelativeTime(t time.Time) string {
	diff := time.Since(t)
	if diff < 0 {
		diff = 0
	}
	seconds := int(diff.Seconds())
	if seconds < 45 {
		return "just now"
	}
	minutes := int(diff.Minutes())
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := int(diff.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd ago", days)
}

// FetchHermesPendingRequests retrieves requests waiting for approval from the Hermes API.
func FetchHermesPendingRequests(ctx context.Context, apiURL string, apiKey string, limit int) (*HermesRequestsResponse, error) {
	if apiURL == "" {
		apiURL = "http://localhost:3000"
	}
	apiURL = strings.TrimRight(apiURL, "/")

	if limit <= 0 {
		limit = 50
	}

	endpoint := fmt.Sprintf("%s/api/hermes/requests?status=awaiting_approval&take=%d", apiURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		if strings.HasPrefix(apiKey, "Bearer ") {
			req.Header.Set("Authorization", apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach Hermes API at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hermes API returned status %d: %s", resp.StatusCode, truncateString(string(bodyBytes), 200))
	}

	// The API response might be { requests: [...], pending: N } or a raw array [...]
	var res HermesRequestsResponse
	if err := json.Unmarshal(bodyBytes, &res); err == nil && (res.Requests != nil || res.Pending > 0) {
		if res.Pending == 0 {
			res.Pending = len(res.Requests)
		}
		return &res, nil
	}

	// Fallback to raw array
	var rawList []HermesRequest
	if err := json.Unmarshal(bodyBytes, &rawList); err == nil {
		return &HermesRequestsResponse{
			Requests: rawList,
			Pending:  len(rawList),
		}, nil
	}

	return nil, fmt.Errorf("failed to parse hermes response: %s", truncateString(string(bodyBytes), 200))
}

// PerformHermesAction sends an approval, rejection, or edited approval to the Hermes API.
func PerformHermesAction(ctx context.Context, apiURL, apiKey, reqID, action, title, prompt string) error {
	if apiURL == "" {
		apiURL = "http://localhost:3000"
	}
	apiURL = strings.TrimRight(apiURL, "/")

	if reqID == "" {
		return fmt.Errorf("request ID is required")
	}

	payload := map[string]any{
		"action": action,
	}
	if action == "edit" {
		if title != "" {
			payload["title"] = title
		}
		if prompt != "" {
			payload["prompt"] = prompt
		}
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/hermes/requests/%s", apiURL, url.PathEscape(reqID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to create patch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		if strings.HasPrefix(apiKey, "Bearer ") {
			req.Header.Set("Authorization", apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send action to Hermes API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hermes API returned status %d: %s", resp.StatusCode, truncateString(string(bodyBytes), 200))
	}

	return nil
}
