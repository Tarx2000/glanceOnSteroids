package glance

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
)

type MvvSearchItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *Application) HandleMvvSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}

	apiURL := fmt.Sprintf("https://www.mvg.de/api/bgw-pt/v3/locations?query=%s", url.QueryEscape(query))
	resp, err := http.Get(apiURL)
	if err != nil {
		slog.Error("[MVV] Search request failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "failed to query locations API", http.StatusInternalServerError)
		return
	}

	var results []struct {
		GlobalId string `json:"globalId"`
		Name     string `json:"name"`
		Place    string `json:"place"`
		Type     string `json:"type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []MvvSearchItem
	for _, res := range results {
		if res.Type == "STATION" && res.GlobalId != "" {
			name := res.Name
			if res.Place != "" {
				name = fmt.Sprintf("%s, %s", res.Place, res.Name)
			}
			items = append(items, MvvSearchItem{
				ID:   res.GlobalId,
				Name: name,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}
