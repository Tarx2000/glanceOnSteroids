package glance

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/glanceapp/glance/internal/widget"
	"gopkg.in/yaml.v3"
)

// HandleWidgetAdd appends a new widget to a specific page column.
func (a *Application) HandleWidgetAdd(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to prevent OOM
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		PageSlug    string                 `json:"page"`
		ColumnIndex JSONStringOrInt        `json:"column"`
		Type        string                 `json:"type"`
		Properties  map[string]interface{} `json:"properties"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.AddWidget(payload.PageSlug, string(payload.ColumnIndex), payload.Type, payload.Properties); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot reload credentials if we just added/updated a Spotify or calendar widget
	if payload.Type == "spotify" {
		InitSpotify(a.Config.Spotify.ClientID, a.Config.Spotify.ClientSecret, a.Config.Spotify.RedirectURL)
	}
	if payload.Type == "calendar" {
		InitGoogle(a.Config.Google.ClientID, a.Config.Google.ClientSecret, a.Config.Google.RedirectURL)
	}

	w.WriteHeader(http.StatusOK)
}

// HandleWidgetDelete deletes a widget from a specific column.
func (a *Application) HandleWidgetDelete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		PageSlug    string          `json:"page"`
		ColumnIndex JSONStringOrInt `json:"column"`
		WidgetIndex int             `json:"widget"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.DeleteWidget(payload.PageSlug, string(payload.ColumnIndex), payload.WidgetIndex); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleWidgetRender compiles the HTML of a widget.
func (a *Application) HandleWidgetRender(w http.ResponseWriter, r *http.Request) {
	pageSlug := r.URL.Query().Get("page")
	columnStr := r.URL.Query().Get("column")
	widgetIdxStr := r.URL.Query().Get("widget")
	nestedIdxStr := r.URL.Query().Get("nested")

	a.configMu.RLock()
	page, exists := a.slugToPage[pageSlug]
	a.configMu.RUnlock()

	if !exists {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	var wd widget.Widget

	if columnStr == "head" {
		idx, err := strconv.Atoi(widgetIdxStr)
		if err == nil && idx >= 0 && idx < len(page.HeadWidgets) {
			wd = page.HeadWidgets[idx]
		}
	} else {
		colIdx, err := strconv.Atoi(columnStr)
		if err == nil && colIdx >= 0 && colIdx < len(page.Columns) {
			column := &page.Columns[colIdx]
			idx, err := strconv.Atoi(widgetIdxStr)
			if err == nil && idx >= 0 && idx < len(column.Widgets) {
				wd = column.Widgets[idx]
			}
		}
	}

	if wd == nil {
		http.Error(w, "widget not found", http.StatusNotFound)
		return
	}

	if nestedIdxStr != "" {
		nestedIdx, err := strconv.Atoi(nestedIdxStr)
		if err == nil && nestedIdx >= 0 {
			if group, ok := wd.(*widget.Group); ok {
				if nestedIdx < len(group.Widgets) {
					wd = group.Widgets[nestedIdx]
				} else {
					http.Error(w, "nested widget not found", http.StatusNotFound)
					return
				}
			} else {
				http.Error(w, "widget is not a group", http.StatusBadRequest)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write([]byte(wd.Render()))
}

// HandleWidgetGet retrieves the type and properties of a single widget.
func (a *Application) HandleWidgetGet(w http.ResponseWriter, r *http.Request) {
	pageSlug := r.URL.Query().Get("page")
	columnStr := r.URL.Query().Get("column")
	widgetIdxStr := r.URL.Query().Get("widget")

	widgetIdx, err := strconv.Atoi(widgetIdxStr)
	if err != nil {
		http.Error(w, "invalid widget index", http.StatusBadRequest)
		return
	}

	rootNode, err := a.ConfigManager.ReadAST()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pageNode, err := findPageNode(&rootNode, pageSlug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var widgetsNode *yaml.Node

	if columnStr == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
	} else if strings.Contains(columnStr, ":") {
		parts := strings.Split(columnStr, ":")
		origColStr := parts[0]
		origIdx, _ := strconv.Atoi(parts[1])

		var baseNode *yaml.Node
		if origColStr == "head" {
			headNode := findMapValue(pageNode, "head-widgets")
			if headNode != nil && headNode.Kind == yaml.SequenceNode && origIdx >= 0 && origIdx < len(headNode.Content) {
				baseNode = headNode.Content[origIdx]
			}
		} else {
			columnsNode := findMapValue(pageNode, "columns")
			colIdx, err := strconv.Atoi(origColStr)
			if err == nil && columnsNode != nil && columnsNode.Kind == yaml.SequenceNode && colIdx >= 0 && colIdx < len(columnsNode.Content) {
				colNode := columnsNode.Content[colIdx]
				colWidgets := findMapValue(colNode, "widgets")
				if colWidgets != nil && colWidgets.Kind == yaml.SequenceNode && origIdx >= 0 && origIdx < len(colWidgets.Content) {
					baseNode = colWidgets.Content[origIdx]
				}
			}
		}
		if baseNode != nil {
			widgetsNode = findMapValue(baseNode, "widgets")
		}
	} else {
		columnsNode := findMapValue(pageNode, "columns")
		colIdx, err := strconv.Atoi(columnStr)
		if err == nil && columnsNode != nil && columnsNode.Kind == yaml.SequenceNode && colIdx >= 0 && colIdx < len(columnsNode.Content) {
			colNode := columnsNode.Content[colIdx]
			widgetsNode = findMapValue(colNode, "widgets")
		}
	}

	if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode || widgetIdx < 0 || widgetIdx >= len(widgetsNode.Content) {
		http.Error(w, "widget not found", http.StatusNotFound)
		return
	}

	targetWidgetNode := widgetsNode.Content[widgetIdx]
	var decoded map[string]interface{}
	if err := targetWidgetNode.Decode(&decoded); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if decoded["type"] == "spotify" {
		a.configMu.RLock()
		decoded["client_id"] = a.Config.Spotify.ClientID
		decoded["redirect_url"] = a.Config.Spotify.RedirectURL
		hasSecret := a.Config.Spotify.ClientSecret != ""
		a.configMu.RUnlock()

		if hasSecret {
			decoded["client_secret"] = "********"
		}
		accessToken, _ := Store.GetSetting("spotify_access_token", "")
		refreshToken, _ := Store.GetSetting("spotify_refresh_token", "")
		if accessToken != "" {
			decoded["access_token"] = "********"
		}
		if refreshToken != "" {
			decoded["refresh_token"] = "********"
		}
	}

	if decoded["type"] == "calendar" {
		a.configMu.RLock()
		decoded["google_client_id"] = a.Config.Google.ClientID
		decoded["google_redirect_url"] = a.Config.Google.RedirectURL
		hasSecret := a.Config.Google.ClientSecret != ""
		a.configMu.RUnlock()

		if hasSecret {
			decoded["google_client_secret"] = "********"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(decoded)
}

// HandleWidgetUpdate updates an existing widget directly.
func (a *Application) HandleWidgetUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		PageSlug    string                 `json:"page"`
		ColumnIndex JSONStringOrInt        `json:"column"`
		WidgetIndex int                    `json:"widget"`
		Properties  map[string]interface{} `json:"properties"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.UpdateWidget(payload.PageSlug, string(payload.ColumnIndex), payload.WidgetIndex, payload.Properties); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Dynamic config/service hot reload
	// Need to check the type from config after reloading
	a.configMu.RLock()
	spotifyClientID := a.Config.Spotify.ClientID
	spotifyClientSecret := a.Config.Spotify.ClientSecret
	spotifyRedirectURL := a.Config.Spotify.RedirectURL
	googleClientID := a.Config.Google.ClientID
	googleClientSecret := a.Config.Google.ClientSecret
	googleRedirectURL := a.Config.Google.RedirectURL
	a.configMu.RUnlock()

	InitSpotify(spotifyClientID, spotifyClientSecret, spotifyRedirectURL)
	InitGoogle(googleClientID, googleClientSecret, googleRedirectURL)

	w.WriteHeader(http.StatusOK)
}
