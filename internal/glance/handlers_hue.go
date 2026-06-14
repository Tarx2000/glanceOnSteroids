package glance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HandleHueLogin redirects the user to Signify (Philips Hue) remote authorization portal.
func (a *Application) HandleHueLogin(w http.ResponseWriter, r *http.Request) {
	if hueConfig.ClientID == "" {
		http.Error(w, "Philips Hue Client ID is not configured. Add it under the hue: block in glance.yml.", http.StatusInternalServerError)
		return
	}

	redirectURI := getHueRedirectURI(r)
	slog.Info("[Hue] Initiating remote OAuth login", "redirect_uri", redirectURI)

	// Signify Hue Remote API V2 OAuth login
	authURL := fmt.Sprintf("https://api.meethue.com/v2/oauth2/authorize?client_id=%s&response_type=code&redirect_uri=%s&state=glance-hue-state",
		url.QueryEscape(hueConfig.ClientID),
		url.QueryEscape(redirectURI),
	)

	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// HandleHueCallback handles the redirect callback from Signify Hue remote portal.
func (a *Application) HandleHueCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Authorization code missing", http.StatusBadRequest)
		return
	}

	redirectURI := getHueRedirectURI(r)
	slog.Info("[Hue] OAuth callback received", "redirect_uri", redirectURI)

	tokenURL := "https://api.meethue.com/v2/oauth2/token"
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth := base64EncodeCredentials(hueConfig.ClientID, hueConfig.ClientSecret)
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hueHTTPClient.Do(req)
	if err != nil {
		slog.Error("[Hue] Token exchange request failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errRes map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errRes)
		slog.Error("[Hue] Token exchange failed", "status", resp.StatusCode, "error", errRes)
		http.Error(w, fmt.Sprintf("token exchange failed: status %d", resp.StatusCode), http.StatusInternalServerError)
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

	_ = Store.SetSetting("hue_access_token", res.AccessToken)
	if res.RefreshToken != "" {
		_ = Store.SetSetting("hue_refresh_token", res.RefreshToken)
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		_ = Store.SetSetting("hue_access_token_expiry", strconv.FormatInt(expiryTime, 10))
	}
	_ = Store.SetSetting("hue_authorized", "true")

	// Trigger remote pairing to generate/retrieve hue_username immediately
	slog.Info("[Hue] Triggering bridge remote pairing handshake...")
	username, err := pairHueRemote(res.AccessToken)
	if err != nil {
		slog.Error("[Hue] Remote pairing failed", "error", err)
		// We still let the login succeed since tokens are correct. User can retry pairing.
	} else {
		slog.Info("[Hue] Pairing successful, application username saved", "username", username)
	}

	// Force update all Hue widgets immediately
	a.configMu.RLock()
	for i := range a.Config.Pages {
		for j := range a.Config.Pages[i].Columns {
			for k := range a.Config.Pages[i].Columns[j].Widgets {
				wgt := a.Config.Pages[i].Columns[j].Widgets[k]
				if wgt.GetType() == "hue" {
					wgt.Update(r.Context(), &glanceServiceProvider{})
				}
			}
		}
	}
	a.configMu.RUnlock()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleHueResourcesGet returns a list of rooms, lights, and scenes from Hue Remote API.
func (a *Application) HandleHueResourcesGet(w http.ResponseWriter, r *http.Request) {
	resources, err := fetchHueResourcesFromAPI(r.Context())
	if err != nil {
		slog.Error("[Hue] Failed to fetch resources", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resources)
}

// HandleHueControl controls Hue lights, rooms, and scenes remotely.
func (a *Application) HandleHueControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID   string `json:"id"`
		Type string `json:"type"` // room, light, scene
		On   bool   `json:"on"`   // true=on, false=off
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.ID == "" || body.Type == "" {
		http.Error(w, "id and type are required fields", http.StatusBadRequest)
		return
	}

	err := controlHueResource(r.Context(), body.ID, body.Type, body.On)
	if err != nil {
		slog.Error("[Hue] Control failed", "id", body.ID, "type", body.Type, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Trigger background status update for Hue widgets to keep states synced
	go func() {
		a.configMu.RLock()
		defer a.configMu.RUnlock()
		for i := range a.Config.Pages {
			for j := range a.Config.Pages[i].Columns {
				for k := range a.Config.Pages[i].Columns[j].Widgets {
					wgt := a.Config.Pages[i].Columns[j].Widgets[k]
					if wgt.GetType() == "hue" {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						wgt.Update(ctx, &glanceServiceProvider{})
						cancel()
					}
				}
			}
		}
	}()

	w.WriteHeader(http.StatusOK)
}

func base64EncodeCredentials(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
