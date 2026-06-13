package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/glanceapp/glance/internal/widget"
)

// HandleGoogleLogin redirects user to Google OAuth consent screen.
func (a *Application) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if googleConfig.ClientID == "" {
		http.Error(w, "Google Client ID is not configured. Add it under the google: block in glance.yml or as GOOGLE_CLIENT_ID environment variable.", http.StatusInternalServerError)
		return
	}

	redirectURI := getGoogleRedirectURI(r)
	slog.Info("[Google] Initiating OAuth login", "redirect_uri", redirectURI)
	scope := "https://www.googleapis.com/auth/calendar.readonly"
	authURL := fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&access_type=offline&prompt=consent",
		url.QueryEscape(googleConfig.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(scope),
	)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// HandleGoogleCallback exchanges authorization code for tokens.
func (a *Application) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Authorization code missing", http.StatusBadRequest)
		return
	}

	redirectURI := getGoogleRedirectURI(r)
	slog.Info("[Google] Callback received", "redirect_uri", redirectURI)

	tokenURL := "https://oauth2.googleapis.com/token"
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", googleConfig.ClientID)
	data.Set("client_secret", googleConfig.ClientSecret)

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		slog.Error("[Google] Token request failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errRes map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errRes)
		slog.Error("[Google] Token exchange failed", "status", resp.StatusCode, "error", errRes)
		http.Error(w, fmt.Sprintf("token exchange failed: %d - %v", resp.StatusCode, errRes), http.StatusInternalServerError)
		return
	}

	var res struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := Store.SetSetting("google_access_token", res.AccessToken); err != nil {
		slog.Error("[Google] Failed to persist access token", "error", err)
	}
	if res.RefreshToken != "" {
		if err := Store.SetSetting("google_refresh_token", res.RefreshToken); err != nil {
			slog.Error("[Google] Failed to persist refresh token", "error", err)
		}
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		if err := Store.SetSetting("google_access_token_expiry", strconv.FormatInt(expiryTime, 10)); err != nil {
			slog.Error("[Google] Failed to persist token expiry", "error", err)
		}
	}
	if err := Store.SetSetting("google_authorized", "true"); err != nil {
		slog.Error("[Google] Failed to persist authorized flag", "error", err)
	}

	// Force update all calendar widgets immediately
	a.configMu.RLock()
	for i := range a.Config.Pages {
		page := &a.Config.Pages[i]
		flat := page.GetFlatWidgets()
		for _, w := range flat {
			if cal, ok := w.(*widget.Calendar); ok {
				cal.Lock()
				cal.NextUpdate = time.Time{} // Clear cache duration
				cal.Unlock()
				cal.Update(context.Background())
			}
		}
	}
	a.configMu.RUnlock()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleGoogleCalendarsGet returns the list of available calendars.
func (a *Application) HandleGoogleCalendarsGet(w http.ResponseWriter, r *http.Request) {
	auth, _ := Store.GetSetting("google_authorized", "false")
	if auth != "true" {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	token, err := getGoogleAccessToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	req, err := http.NewRequest("GET", "https://www.googleapis.com/calendar/v3/users/me/calendarList", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		_ = Store.SetSetting("google_access_token", "")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Google API status: %d", resp.StatusCode), http.StatusInternalServerError)
		return
	}

	var data struct {
		Items []GoogleCalendarEntry `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data.Items)
}
