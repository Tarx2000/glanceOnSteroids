package glance

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	spotifyConfig   SpotifyConfig
	spotifyStateMu  sync.Mutex
	lastTrackID     string
	lastIsPlaying   bool
	lastProgressMS  int
	lastVolume      int
	lastSpotifyError string
	spotifyPoller   sync.Once

	// ActiveConnectionsCheck callback returns the number of active WebSocket connections.
	// Wired dynamically from main server initialization to avoid circular package imports.
	ActiveConnectionsCheck func() int
)

func publishSpotifyEvent(msgType string, data interface{}) {
	select {
	case SpotifyEventChan <- SpotifyEvent{Type: msgType, Data: data}:
	default:
		log.Printf("[Spotify] Event channel full, dropping event: %s", msgType)
	}
}

// SpotifyTrack represents the currently playing track status.
type SpotifyTrack struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	ImageURL  string `json:"image_url"`
	Progress  int    `json:"progress_ms"`
	Duration  int    `json:"duration_ms"`
	IsPlaying bool   `json:"is_playing"`
	Volume    int    `json:"volume"`
}

// InitSpotify configures the Spotify client credentials and redirect URI.
func InitSpotify(clientID, clientSecret, redirectURL string) {
	spotifyConfig.ClientID = clientID
	spotifyConfig.ClientSecret = clientSecret
	spotifyConfig.RedirectURL = redirectURL

	if spotifyConfig.ClientID == "" {
		spotifyConfig.ClientID = os.Getenv("SPOTIFY_CLIENT_ID")
	}
	if spotifyConfig.ClientSecret == "" {
		spotifyConfig.ClientSecret = os.Getenv("SPOTIFY_CLIENT_SECRET")
	}
	if spotifyConfig.RedirectURL == "" {
		spotifyConfig.RedirectURL = os.Getenv("SPOTIFY_REDIRECT_URL")
	}

	if spotifyConfig.ClientID != "" {
		log.Println("[Spotify] Configured Client ID successfully.")
	} else {
		log.Println("[Spotify] Warning: Client ID is empty. Spotify widget will need SPOTIFY_CLIENT_ID configured in glance.yml or env.")
	}
}

// triggerInitialSpotifyPush broadcasts the initial playback state to a newly connected client.
func triggerInitialSpotifyPush() {
	time.Sleep(300 * time.Millisecond) // wait for websocket to fully open
	status, err := getSpotifyPlaybackStatus()
	if err != nil {
		errMsg := err.Error()
		spotifyStateMu.Lock()
		lastSpotifyError = errMsg
		spotifyStateMu.Unlock()
		log.Printf("[Spotify] Initial push failed: %v", err)
		auth, _ := Store.GetSetting("spotify_authorized", "false")
		publishSpotifyEvent("spotify_update", map[string]interface{}{
			"authorized": auth == "true",
			"track":      nil,
			"error":      errMsg,
		})
		return
	}

	spotifyStateMu.Lock()
	lastTrackID = status.ID
	lastIsPlaying = status.IsPlaying
	lastProgressMS = status.Progress
	lastVolume = status.Volume
	lastSpotifyError = ""
	spotifyStateMu.Unlock()

	log.Printf("[Spotify] Initial push: track=%q is_playing=%v", status.ID, status.IsPlaying)
	publishSpotifyEvent("spotify_update", map[string]interface{}{
		"authorized": true,
		"track":      status,
		"error":      "",
	})
}

// StartSpotifyPoller starts the background loop that polls Spotify status and pushes it via WebSockets.
// To optimize resources on low-spec hardware (like Fire HD tablets), polling is paused when no client is connected.
func StartSpotifyPoller() {
	spotifyPoller.Do(func() {
		go func() {
			log.Println("[Spotify] Background status poller started.")
			for {
				time.Sleep(2 * time.Second)

				// Skip polling if there are no active WebSocket connections
				if ActiveConnectionsCheck != nil && ActiveConnectionsCheck() == 0 {
					continue
				}

				auth, _ := Store.GetSetting("spotify_authorized", "false")
				if auth != "true" {
					continue
				}

				status, err := getSpotifyPlaybackStatus()
				if err != nil {
					errMsg := err.Error()
					spotifyStateMu.Lock()
					changed := errMsg != lastSpotifyError
					if changed {
						lastSpotifyError = errMsg
					}
					spotifyStateMu.Unlock()

					if changed {
						log.Printf("[Spotify] Poller broadcast error: %v", errMsg)
						publishSpotifyEvent("spotify_update", map[string]interface{}{
							"authorized": true,
							"track":      nil,
							"error":      errMsg,
						})
					}
					continue
				}

				spotifyStateMu.Lock()
				changed := false
				if lastSpotifyError != "" {
					lastSpotifyError = ""
					changed = true
				}
				if status.ID != lastTrackID ||
					status.IsPlaying != lastIsPlaying ||
					status.Volume != lastVolume ||
					abs(status.Progress-lastProgressMS) > 5000 {
					lastTrackID = status.ID
					lastIsPlaying = status.IsPlaying
					lastProgressMS = status.Progress
					lastVolume = status.Volume
					changed = true
				}
				spotifyStateMu.Unlock()

				if changed {
					log.Printf("[Spotify] Poller broadcast: track=%q is_playing=%v progress=%d duration=%d", status.ID, status.IsPlaying, status.Progress, status.Duration)
					publishSpotifyEvent("spotify_update", map[string]interface{}{
						"authorized": true,
						"track":      status,
						"error":      "",
					})
				}
			}
		}()
	})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// getSpotifyRedirectURI resolves the redirect URI for the OAuth flow.
// Priority: explicitly configured value > X-Forwarded-* headers > request Host header.
func getSpotifyRedirectURI(r *http.Request) string {
	if spotifyConfig.RedirectURL != "" {
		return spotifyConfig.RedirectURL
	}

	scheme := "http"
	host := r.Host

	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return fmt.Sprintf("%s://%s/api/spotify/callback", scheme, host)
}

// getSpotifyPlaybackStatus queries Spotify for the current active playback state.
func getSpotifyPlaybackStatus() (*SpotifyTrack, error) {
	token, err := getSpotifyAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		// Nothing is currently playing
		log.Println("[Spotify] API returned 204 No Content (nothing playing)")
		return &SpotifyTrack{IsPlaying: false}, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Token expired, force refresh next time
		log.Println("[Spotify] API returned 401 Unauthorized")
		if err := dbSetSetting("spotify_access_token", ""); err != nil {
			log.Printf("[Spotify] Failed to clear access token: %v", err)
		}
		return nil, fmt.Errorf("unauthorized")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[Spotify] API returned status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("spotify API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		IsPlaying  bool `json:"is_playing"`
		ProgressMS int  `json:"progress_ms"`
		Item       struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
			DurationMS int `json:"duration_ms"`
		} `json:"item"`
		Device struct {
			VolumePercent int `json:"volume_percent"`
		} `json:"device"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Guard against empty item (e.g., private podcasts or ad slots)
	if data.Item.ID == "" {
		log.Printf("[Spotify] API returned 200 but item.ID is empty (is_playing=%v)", data.IsPlaying)
		return &SpotifyTrack{IsPlaying: false}, nil
	}

	artists := []string{}
	for _, a := range data.Item.Artists {
		artists = append(artists, a.Name)
	}

	imgURL := ""
	if len(data.Item.Album.Images) > 0 {
		imgURL = data.Item.Album.Images[0].URL
	}

	track := &SpotifyTrack{
		ID:        data.Item.ID,
		Title:     data.Item.Name,
		Artist:    strings.Join(artists, ", "),
		Album:     data.Item.Album.Name,
		ImageURL:  imgURL,
		Progress:  data.ProgressMS,
		Duration:  data.Item.DurationMS,
		IsPlaying: data.IsPlaying,
		Volume:    data.Device.VolumePercent,
	}
	log.Printf("[Spotify] API track: %s by %s (is_playing=%v)", track.Title, track.Artist, track.IsPlaying)
	return track, nil
}

// getSpotifyAccessToken returns a valid access token, auto-refreshing it if needed.
func getSpotifyAccessToken() (string, error) {
	accessToken, _ := Store.GetSetting("spotify_access_token", "")
	tokenExpiryStr, _ := Store.GetSetting("spotify_access_token_expiry", "")
	
	if accessToken != "" && tokenExpiryStr != "" {
		expiry, err := strconv.ParseInt(tokenExpiryStr, 10, 64)
		if err == nil && time.Now().Unix() < expiry-60 {
			return accessToken, nil
		}
	}

	refreshToken, _ := Store.GetSetting("spotify_refresh_token", "")
	if refreshToken == "" {
		return "", fmt.Errorf("no refresh token available, user must re-authorize")
	}

	if spotifyConfig.ClientID == "" || spotifyConfig.ClientSecret == "" {
		return "", fmt.Errorf("spotify credentials not configured")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(spotifyConfig.ClientID + ":" + spotifyConfig.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("refresh token exchange failed: %d - %s", resp.StatusCode, string(body))
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if err := Store.SetSetting("spotify_access_token", res.AccessToken); err != nil {
		log.Printf("[Spotify] Failed to persist access token: %v", err)
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		if err := Store.SetSetting("spotify_access_token_expiry", strconv.FormatInt(expiryTime, 10)); err != nil {
			log.Printf("[Spotify] Failed to persist token expiry: %v", err)
		}
	}
	return res.AccessToken, nil
}

// spotifyControlAction helper sends volume or playback command to Spotify API.
func spotifyControlAction(method, path string, body io.Reader) error {
	token, err := getSpotifyAccessToken()
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, "https://api.spotify.com/v1/me/player/"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if err := Store.SetSetting("spotify_access_token", ""); err != nil {
			log.Printf("[Spotify] Failed to clear access token: %v", err)
		}
		return fmt.Errorf("unauthorized")
	}

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("spotify API error: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// TriggerImmediateSpotifyBroadcast runs in a background goroutine, waiting for 250ms
// before querying the current playback status and broadcasting it to all active clients.
// This allows immediate visual feedback after play, pause, volume, or skip actions.
func TriggerImmediateSpotifyBroadcast() {
	go func() {
		// Wait a short duration to let the Spotify API update its state
		time.Sleep(250 * time.Millisecond)

		auth, _ := Store.GetSetting("spotify_authorized", "false")
		if auth != "true" {
			return
		}

		status, err := getSpotifyPlaybackStatus()
		if err != nil {
			errMsg := err.Error()
			spotifyStateMu.Lock()
			lastSpotifyError = errMsg
			spotifyStateMu.Unlock()
			
			publishSpotifyEvent("spotify_update", map[string]interface{}{
				"authorized": true,
				"track":      nil,
				"error":      errMsg,
			})
			return
		}

		spotifyStateMu.Lock()
		lastTrackID = status.ID
		lastIsPlaying = status.IsPlaying
		lastProgressMS = status.Progress
		lastVolume = status.Volume
		lastSpotifyError = ""
		spotifyStateMu.Unlock()

		publishSpotifyEvent("spotify_update", map[string]interface{}{
			"authorized": true,
			"track":      status,
			"error":      "",
		})
	}()
}