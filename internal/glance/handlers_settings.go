package glance

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleSettingsGet fetches the active configuration parameters.
func (a *Application) HandleSettingsGet(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	payload := settingsPayload{
		Branding: brandingSettingsPayload{
			AppName:      a.Config.Branding.AppName,
			CustomFooter: a.Config.Branding.CustomFooter,
		},
		Server: serverSettingsPayload{
			Host:       a.Config.Server.Host,
			Port:       a.Config.Server.Port,
			AssetsPath: a.Config.Server.AssetsPath,
			Timezone:   a.Config.Server.Timezone,
		},
		Theme: themeSettingsPayload{
			Light:                          a.Config.Theme.Light,
			BackgroundColor:                formatHSL(a.Config.Theme.BackgroundColor),
			PrimaryColor:                   formatHSL(a.Config.Theme.PrimaryColor),
			PositiveColor:                  formatHSL(a.Config.Theme.PositiveColor),
			NegativeColor:                  formatHSL(a.Config.Theme.NegativeColor),
			ContrastMultiplier:             a.Config.Theme.ContrastMultiplier,
			TextSaturationMultiplier:       a.Config.Theme.TextSaturationMultiplier,
			CustomCSSFile:                  a.Config.Theme.CustomCSSFile,
			WidgetGap:                      derefString(a.Config.Theme.WidgetGap),
			WidgetContentVerticalPadding:   derefString(a.Config.Theme.WidgetContentVerticalPadding),
			WidgetContentHorizontalPadding: derefString(a.Config.Theme.WidgetContentHorizontalPadding),
			BorderRadius:                   derefString(a.Config.Theme.BorderRadius),
		},
		Spotify: spotifySettingsPayload{
			ClientID:     a.Config.Spotify.ClientID,
			ClientSecret: maskSecret(a.Config.Spotify.ClientSecret),
			RedirectURL:  a.Config.Spotify.RedirectURL,
		},
	}
	a.configMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// HandleSettingsSave saves application-wide settings in glance.yml.
func (a *Application) HandleSettingsSave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload settingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Hot reload credentials: check if secret is masked placeholder
	a.configMu.RLock()
	spotifySecret := a.Config.Spotify.ClientSecret
	a.configMu.RUnlock()

	if strings.TrimSpace(payload.Spotify.ClientSecret) == "********" {
		payload.Spotify.ClientSecret = spotifySecret
	}

	if err := a.ConfigManager.SaveSettings(payload.Branding, payload.Server, payload.Theme, payload.Spotify); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot reload credentials
	w.WriteHeader(http.StatusOK)
}

// HandleLayoutSave updates the widget ordering of a page.
func (a *Application) HandleLayoutSave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		PageSlug    string     `json:"page"`
		Head        []string   `json:"head"`
		Columns     [][]string `json:"columns"`
		ColumnSizes []string   `json:"column_sizes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.SaveLayout(payload.PageSlug, payload.Head, payload.Columns, payload.ColumnSizes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleLayoutBatchSave processes bulk layout updates across multiple pages.
func (a *Application) HandleLayoutBatchSave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		Pages []struct {
			PageSlug    string     `json:"page"`
			Head        []string   `json:"head"`
			Columns     [][]string `json:"columns"`
			ColumnSizes []string   `json:"column_sizes"`
		} `json:"pages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, p := range payload.Pages {
		if err := a.ConfigManager.SaveLayout(p.PageSlug, p.Head, p.Columns, p.ColumnSizes); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
