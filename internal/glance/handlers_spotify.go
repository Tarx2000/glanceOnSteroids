package glance

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HandleSpotifyLogin initiates the Spotify OAuth login sequence.
func (a *Application) HandleSpotifyLogin(w http.ResponseWriter, r *http.Request) {
	if a.SpotifyPoller == nil || a.SpotifyPoller.config.ClientID == "" {
		http.Error(w, "Spotify Client ID is not configured. Add it under the spotify: block in glance.yml or as SPOTIFY_CLIENT_ID environment variable.", http.StatusInternalServerError)
		return
	}

	redirectURI := a.SpotifyPoller.getSpotifyRedirectURI(r)
	slog.Info("[Spotify] Initiating OAuth login", "redirect_uri", redirectURI, "configured_url", a.SpotifyPoller.config.RedirectURL, "request_host", r.Host)
	scopes := "user-read-playback-state user-modify-playback-state"
	authURL := fmt.Sprintf("https://accounts.spotify.com/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s",
		url.QueryEscape(a.SpotifyPoller.config.ClientID),
		url.QueryEscape(scopes),
		url.QueryEscape(redirectURI),
	)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// HandleSpotifyCallback receives the OAuth code, exchanges it for tokens, and persists them.
func (a *Application) HandleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Authorization code missing", http.StatusBadRequest)
		return
	}

	if a.SpotifyPoller == nil {
		http.Error(w, "Spotify poller not initialized", http.StatusInternalServerError)
		return
	}

	redirectURI := a.SpotifyPoller.getSpotifyRedirectURI(r)
	slog.Info("[Spotify] Callback received", "redirect_uri", redirectURI, "configured_url", a.SpotifyPoller.config.RedirectURL)
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth := base64.StdEncoding.EncodeToString([]byte(a.SpotifyPoller.config.ClientID + ":" + a.SpotifyPoller.config.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("[Spotify] Token exchange failed", "status", resp.StatusCode, "body", string(body), "redirect_uri", redirectURI)
		http.Error(w, fmt.Sprintf("token exchange failed: %d - %s (redirect_uri used: %s — make sure this matches your Spotify app settings)", resp.StatusCode, string(body), redirectURI), http.StatusInternalServerError)
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

	if err := Store.SetSetting("spotify_access_token", res.AccessToken); err != nil {
		slog.Error("[Spotify] Failed to persist access token", "error", err)
	}
	if err := Store.SetSetting("spotify_refresh_token", res.RefreshToken); err != nil {
		slog.Error("[Spotify] Failed to persist refresh token", "error", err)
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		if err := Store.SetSetting("spotify_access_token_expiry", strconv.FormatInt(expiryTime, 10)); err != nil {
			slog.Error("[Spotify] Failed to persist token expiry", "error", err)
		}
	}
	if err := Store.SetSetting("spotify_authorized", "true"); err != nil {
		slog.Error("[Spotify] Failed to persist authorized flag", "error", err)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleSpotifyPlay sends the playback resume command.
func (a *Application) HandleSpotifyPlay(w http.ResponseWriter, r *http.Request) {
	if a.SpotifyPoller == nil {
		http.Error(w, "Spotify poller not initialized", http.StatusInternalServerError)
		return
	}
	if err := a.SpotifyPoller.spotifyControlAction("PUT", "play", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.SpotifyPoller.TriggerImmediateBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleSpotifyPause sends the playback resume command.
func (a *Application) HandleSpotifyPause(w http.ResponseWriter, r *http.Request) {
	if a.SpotifyPoller == nil {
		http.Error(w, "Spotify poller not initialized", http.StatusInternalServerError)
		return
	}
	if err := a.SpotifyPoller.spotifyControlAction("PUT", "pause", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.SpotifyPoller.TriggerImmediateBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleSpotifySkip skips to the next or previous track.
func (a *Application) HandleSpotifySkip(w http.ResponseWriter, r *http.Request) {
	if a.SpotifyPoller == nil {
		http.Error(w, "Spotify poller not initialized", http.StatusInternalServerError)
		return
	}
	direction := r.URL.Query().Get("direction")
	action := "next"
	if direction == "prev" {
		action = "previous"
	}
	if err := a.SpotifyPoller.spotifyControlAction("POST", action, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.SpotifyPoller.TriggerImmediateBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleSpotifyVolume sets the active player device volume percent.
func (a *Application) HandleSpotifyVolume(w http.ResponseWriter, r *http.Request) {
	if a.SpotifyPoller == nil {
		http.Error(w, "Spotify poller not initialized", http.StatusInternalServerError)
		return
	}
	volumeStr := r.URL.Query().Get("volume")
	volume, err := strconv.Atoi(volumeStr)
	if err != nil || volume < 0 || volume > 100 {
		http.Error(w, "invalid volume percentage", http.StatusBadRequest)
		return
	}

	if err := a.SpotifyPoller.spotifyControlAction("PUT", fmt.Sprintf("volume?volume_percent=%d", volume), nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.SpotifyPoller.TriggerImmediateBroadcast()
	w.WriteHeader(http.StatusOK)
}
