package glance

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/glanceapp/glance/internal/feed"
	"github.com/glanceapp/glance/internal/widget"
)

type HermesActionRequest struct {
	PageSlug  string `json:"page"`
	Column    string `json:"column"`
	WidgetIdx int    `json:"widget"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"` // approve | reject
	ApiUrl    string `json:"api_url,omitempty"`
	ApiKey    string `json:"api_key,omitempty"`
}

func normalizeHermesAPIURL(rawURL string) string {
	u := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if u == "" {
		return "http://localhost:3000"
	}
	return u
}

// hermesAPIKeyForURL resolves the key server-side so it never needs to be
// exposed in the widget DOM or sent back from the browser.
func (a *Application) hermesAPIKeyForURL(apiURL string) string {
	targetURL := normalizeHermesAPIURL(apiURL)

	a.configMu.RLock()
	defer a.configMu.RUnlock()

	var fallbackKey string
	for _, page := range a.slugToPage {
		if page == nil {
			continue
		}
		for _, wd := range page.GetFlatWidgets() {
			hw, ok := wd.(*widget.HermesApprove)
			if ok {
				hwURL := normalizeHermesAPIURL(string(hw.ApiUrl))
				if hwURL == targetURL && string(hw.ApiKey) != "" {
					return string(hw.ApiKey)
				}
				if fallbackKey == "" && string(hw.ApiKey) != "" {
					fallbackKey = string(hw.ApiKey)
				}
			}
		}
	}

	if fallbackKey != "" {
		return fallbackKey
	}

	if envKey := os.Getenv("HERMES_API_KEY"); envKey != "" {
		return envKey
	}
	if envKey := os.Getenv("HERMES_KEY"); envKey != "" {
		return envKey
	}

	return ""
}

// HandleHermesAction executes an approval or rejection on a pending Hermes request.
func (a *Application) HandleHermesAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload HermesActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if payload.RequestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}

	action := strings.ToLower(strings.TrimSpace(payload.Action))
	if action != "approve" && action != "reject" {
		http.Error(w, "action must be approve or reject", http.StatusBadRequest)
		return
	}

	apiURL := payload.ApiUrl
	apiKey := payload.ApiKey

	// If apiURL is not provided directly, try looking it up from the widget configuration
	if apiURL == "" && payload.PageSlug != "" {
		a.configMu.RLock()
		page, exists := a.slugToPage[payload.PageSlug]
		a.configMu.RUnlock()

		if exists {
			var wd widget.Widget
			if payload.Column == "head" {
				if payload.WidgetIdx >= 0 && payload.WidgetIdx < len(page.HeadWidgets) {
					wd = page.HeadWidgets[payload.WidgetIdx]
				}
			} else {
				colIdx, err := strconv.Atoi(payload.Column)
				if err == nil && colIdx >= 0 && colIdx < len(page.Columns) {
					col := &page.Columns[colIdx]
					if payload.WidgetIdx >= 0 && payload.WidgetIdx < len(col.Widgets) {
						wd = col.Widgets[payload.WidgetIdx]
					}
				}
			}

			if hw, ok := wd.(*widget.HermesApprove); ok {
				apiURL = string(hw.ApiUrl)
				if apiKey == "" {
					apiKey = string(hw.ApiKey)
				}
			}
		}
	}

	if apiURL == "" {
		apiURL = "http://localhost:3000"
	}
	if apiKey == "" {
		apiKey = a.hermesAPIKeyForURL(apiURL)
	}

	if err := feed.PerformHermesAction(r.Context(), apiURL, apiKey, payload.RequestID, action); err != nil {
		slog.Error("[Hermes] Failed to execute action", "action", action, "id", payload.RequestID, "url", apiURL, "err", err)
		http.Error(w, "failed to execute action: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Immediately remove the request from all in-memory Hermes widgets across pages
	a.configMu.RLock()
	for slug, page := range a.slugToPage {
		if page == nil {
			continue
		}
		for cIdx, col := range page.Columns {
			for wIdx, wd := range col.Widgets {
				if hw, ok := wd.(*widget.HermesApprove); ok {
					hw.RemoveRequest(payload.RequestID)
					if a.Hub != nil {
						a.Hub.BroadcastMessage("widget_update", map[string]interface{}{
							"page":       slug,
							"col":        strconv.Itoa(cIdx),
							"idx":        wIdx,
							"nested_idx": -1,
						})
					}
				}
			}
		}
		for wIdx, wd := range page.HeadWidgets {
			if hw, ok := wd.(*widget.HermesApprove); ok {
				hw.RemoveRequest(payload.RequestID)
				if a.Hub != nil {
					a.Hub.BroadcastMessage("widget_update", map[string]interface{}{
						"page":       slug,
						"col":        "head",
						"idx":        wIdx,
						"nested_idx": -1,
					})
				}
			}
		}
	}
	a.configMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"action":  action,
		"id":      payload.RequestID,
	})
}

// HandleHermesNotify handles active push notifications from Hermes Agent or external webhooks.
// When triggered, it forces an immediate update of all Hermes widgets and broadcasts real-time
// WebSocket updates to all active browser sessions.
func (a *Application) HandleHermesNotify(w http.ResponseWriter, r *http.Request) {
	slog.Info("[Hermes] Push notification received, triggering real-time widget refresh")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	updatedCount := 0

	a.configMu.RLock()
	for slug, page := range a.slugToPage {
		if page == nil {
			continue
		}
		for cIdx, col := range page.Columns {
			for wIdx, wd := range col.Widgets {
				if hw, ok := wd.(*widget.HermesApprove); ok {
					hw.Update(ctx, &glanceServiceProvider{})
					updatedCount++
					if a.Hub != nil {
						a.Hub.BroadcastMessage("widget_update", map[string]interface{}{
							"page":       slug,
							"col":        strconv.Itoa(cIdx),
							"idx":        wIdx,
							"nested_idx": -1,
						})
					}
				}
			}
		}
		for wIdx, wd := range page.HeadWidgets {
			if hw, ok := wd.(*widget.HermesApprove); ok {
				hw.Update(ctx, &glanceServiceProvider{})
				updatedCount++
				if a.Hub != nil {
					a.Hub.BroadcastMessage("widget_update", map[string]interface{}{
						"page":       slug,
						"col":        "head",
						"idx":        wIdx,
						"nested_idx": -1,
					})
				}
			}
		}
	}
	a.configMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"updated": updatedCount,
		"message": "Hermes widgets refreshed and pushed via WebSocket",
	})
}
