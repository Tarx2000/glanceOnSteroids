package glance

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/glanceapp/glance/internal/feed"
	"github.com/glanceapp/glance/internal/widget"
)

type HermesActionRequest struct {
	PageSlug   string `json:"page"`
	Column     string `json:"column"`
	WidgetIdx  int    `json:"widget"`
	RequestID  string `json:"request_id"`
	Action     string `json:"action"` // approve | reject | edit
	Title      string `json:"title,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	ApiUrl     string `json:"api_url,omitempty"`
	ApiKey     string `json:"api_key,omitempty"`
}

// HandleHermesAction executes an action (approve, reject, edit) on a pending Hermes request.
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
	if action != "approve" && action != "reject" && action != "edit" {
		http.Error(w, "action must be approve, reject, or edit", http.StatusBadRequest)
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

	if err := feed.PerformHermesAction(r.Context(), apiURL, apiKey, payload.RequestID, action, payload.Title, payload.Prompt); err != nil {
		http.Error(w, "failed to execute action: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"action":  action,
		"id":      payload.RequestID,
	})
}
