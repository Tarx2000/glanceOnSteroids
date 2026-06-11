package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/glanceapp/glance/internal/widget"
)

var (
	googleConfig  GoogleConfig
	googleStateMu sync.Mutex
)

// InitGoogle configures Google client credentials and redirect URI.
func InitGoogle(clientID, clientSecret, redirectURL string) {
	googleStateMu.Lock()
	defer googleStateMu.Unlock()
	googleConfig.ClientID = clientID
	googleConfig.ClientSecret = clientSecret
	googleConfig.RedirectURL = redirectURL

	if googleConfig.ClientID == "" {
		googleConfig.ClientID = os.Getenv("GOOGLE_CLIENT_ID")
	}
	if googleConfig.ClientSecret == "" {
		googleConfig.ClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	}
	if googleConfig.RedirectURL == "" {
		googleConfig.RedirectURL = os.Getenv("GOOGLE_REDIRECT_URL")
	}

	if googleConfig.ClientID != "" {
		slog.Info("[Google] Configured Client ID successfully.")
	} else {
		slog.Warn("[Google] Warning: Client ID is empty. Google Calendar widget will need Client ID configured in glance.yml or env.")
	}
}

// getGoogleRedirectURI resolves redirect URI for Google OAuth.
func getGoogleRedirectURI(r *http.Request) string {
	if googleConfig.RedirectURL != "" {
		return googleConfig.RedirectURL
	}

	scheme := "http"
	host := r.Host

	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return fmt.Sprintf("%s://%s/api/google/callback", scheme, host)
}

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

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", googleConfig.ClientID)
	data.Set("client_secret", googleConfig.ClientSecret)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
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

	if err := dbSetSetting("google_access_token", res.AccessToken); err != nil {
		slog.Error("[Google] Failed to persist access token", "error", err)
	}
	if res.RefreshToken != "" {
		if err := dbSetSetting("google_refresh_token", res.RefreshToken); err != nil {
			slog.Error("[Google] Failed to persist refresh token", "error", err)
		}
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		if err := dbSetSetting("google_access_token_expiry", strconv.FormatInt(expiryTime, 10)); err != nil {
			slog.Error("[Google] Failed to persist token expiry", "error", err)
		}
	}
	if err := dbSetSetting("google_authorized", "true"); err != nil {
		slog.Error("[Google] Failed to persist authorized flag", "error", err)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// getGoogleAccessToken returns a valid access token, refreshing it if expired.
func getGoogleAccessToken() (string, error) {
	accessToken, _ := dbGetSetting("google_access_token", "")
	tokenExpiryStr, _ := dbGetSetting("google_access_token_expiry", "")

	if accessToken != "" && tokenExpiryStr != "" {
		expiry, err := strconv.ParseInt(tokenExpiryStr, 10, 64)
		if err == nil && time.Now().Unix() < expiry-60 {
			return accessToken, nil
		}
	}

	refreshToken, _ := dbGetSetting("google_refresh_token", "")
	if refreshToken == "" {
		return "", fmt.Errorf("no refresh token available, user must re-authorize")
	}

	if googleConfig.ClientID == "" || googleConfig.ClientSecret == "" {
		return "", fmt.Errorf("google credentials not configured")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", googleConfig.ClientID)
	data.Set("client_secret", googleConfig.ClientSecret)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errRes map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errRes)
		slog.Error("[Google] Token refresh failed", "status", resp.StatusCode, "error", errRes)
		return "", fmt.Errorf("refresh token exchange failed status: %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if err := dbSetSetting("google_access_token", res.AccessToken); err != nil {
		slog.Error("[Google] Failed to persist access token", "error", err)
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		if err := dbSetSetting("google_access_token_expiry", strconv.FormatInt(expiryTime, 10)); err != nil {
			slog.Error("[Google] Failed to persist token expiry", "error", err)
		}
	}

	return res.AccessToken, nil
}

// GoogleCalendarEntry represents a calendar resource in Google.
type GoogleCalendarEntry struct {
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	Primary         bool   `json:"primary"`
	BackgroundColor string `json:"backgroundColor"`
	ForegroundColor string `json:"foregroundColor"`
}

// HandleGoogleCalendarsGet returns the list of available calendars for checkboxes layout.
func (a *Application) HandleGoogleCalendarsGet(w http.ResponseWriter, r *http.Request) {
	auth, _ := dbGetSetting("google_authorized", "false")
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
		_ = dbSetSetting("google_access_token", "")
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

// fetchGoogleEventsFromAPI fetches, merges, and filters events from multiple calendar IDs.
func fetchGoogleEventsFromAPI(ctx context.Context, calendarIDs []string, maxDaysAhead int) ([]widget.GoogleCalendarEvent, error) {
	token, err := getGoogleAccessToken()
	if err != nil {
		return nil, err
	}

	if len(calendarIDs) == 0 {
		calendarIDs = []string{"primary"}
	}

	// 1. Fetch Calendar List to build color mapping
	colorMap := make(map[string]string)
	listReq, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/calendar/v3/users/me/calendarList", nil)
	if err == nil {
		listReq.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 5 * time.Second}
		if listResp, err := client.Do(listReq); err == nil {
			defer listResp.Body.Close()
			if listResp.StatusCode == http.StatusOK {
				var listData struct {
					Items []GoogleCalendarEntry `json:"items"`
				}
				if json.NewDecoder(listResp.Body).Decode(&listData) == nil {
					for _, item := range listData.Items {
						colorMap[item.ID] = item.BackgroundColor
					}
				}
			}
		}
	}

	var allEvents []widget.GoogleCalendarEvent
	var mu sync.Mutex
	var wg sync.WaitGroup

	now := time.Now()
	timeMin := now.Format(time.RFC3339)
	timeMax := now.Add(time.Duration(maxDaysAhead) * 24 * time.Hour).Format(time.RFC3339)

	// Fetch events concurrently from Google API
	for _, calID := range calendarIDs {
		wg.Add(1)
		go func(cID string) {
			defer wg.Done()

			u := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime&maxResults=20",
				url.PathEscape(cID),
				url.QueryEscape(timeMin),
				url.QueryEscape(timeMax),
			)

			req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
			if err != nil {
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)

			client := &http.Client{Timeout: 8 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return
			}

			var data struct {
				Items []struct {
					Summary string `json:"summary"`
					Start   struct {
						DateTime string `json:"dateTime"`
						Date     string `json:"date"`
					} `json:"start"`
					End struct {
						DateTime string `json:"dateTime"`
						Date     string `json:"date"`
					} `json:"end"`
				} `json:"items"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				return
			}

			calColor := colorMap[cID]
			if calColor == "" {
				calColor = "var(--color-primary)" // Fallback
			}

			var localEvents []widget.GoogleCalendarEvent
			for _, item := range data.Items {
				var startVal, endVal time.Time
				allDay := false

				if item.Start.DateTime != "" {
					startVal, _ = time.Parse(time.RFC3339, item.Start.DateTime)
				} else if item.Start.Date != "" {
					startVal, _ = time.Parse("2006-01-02", item.Start.Date)
					allDay = true
				}

				if item.End.DateTime != "" {
					endVal, _ = time.Parse(time.RFC3339, item.End.DateTime)
				} else if item.End.Date != "" {
					endVal, _ = time.Parse("2006-01-02", item.End.Date)
				}

				if startVal.IsZero() {
					continue
				}

				localEvents = append(localEvents, widget.GoogleCalendarEvent{
					Summary:    item.Summary,
					Start:      startVal,
					End:        endVal,
					AllDay:     allDay,
					CalendarID: cID,
					Color:      calColor,
				})
			}

			mu.Lock()
			allEvents = append(allEvents, localEvents...)
			mu.Unlock()
		}(calID)
	}

	wg.Wait()

	// Sort events chronologically by Start time
	sort.Slice(allEvents, func(i, j int) bool {
		if allEvents[i].Start.Equal(allEvents[j].Start) {
			return allEvents[i].End.Before(allEvents[j].End)
		}
		return allEvents[i].Start.Before(allEvents[j].Start)
	})

	// Limit to maximum 20 events for widget scrolling
	if len(allEvents) > 20 {
		allEvents = allEvents[:20]
	}

	return allEvents, nil
}
