package glance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glanceapp/glance/internal/widget"
)

var (
	hueConfig          HueConfig
	hueStateMu         sync.Mutex
	hueTokenRefreshMu  sync.Mutex
	hueHTTPClient      = &http.Client{Timeout: 10 * time.Second}

	// lastActiveSceneMu protects access to the lastActiveScene map.
	lastActiveSceneMu sync.RWMutex
	// lastActiveScene tracks the ID of the last activated scene per room/zone ID.
	lastActiveScene = make(map[string]string)
)

// InitHue configures Philips Hue client credentials and redirect URI.
func InitHue(clientID, clientSecret, redirectURL string) {
	hueStateMu.Lock()
	defer hueStateMu.Unlock()
	hueConfig.ClientID = clientID
	hueConfig.ClientSecret = clientSecret
	hueConfig.RedirectURL = redirectURL
}

func getHueRedirectURI(r *http.Request) string {
	if hueConfig.RedirectURL != "" {
		return hueConfig.RedirectURL
	}

	scheme := "http"
	host := r.Host

	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return fmt.Sprintf("%s://%s/api/hue/callback", scheme, host)
}

func getHueAccessToken() (string, error) {
	if token, ok := hueTokenStillValid(); ok {
		return token, nil
	}

	hueTokenRefreshMu.Lock()
	defer hueTokenRefreshMu.Unlock()

	if token, ok := hueTokenStillValid(); ok {
		return token, nil
	}

	refreshToken, _ := Store.GetSetting("hue_refresh_token", "")
	if refreshToken == "" {
		return "", fmt.Errorf("no refresh token available, user must authorize")
	}

	if hueConfig.ClientID == "" || hueConfig.ClientSecret == "" {
		return "", fmt.Errorf("hue credentials not configured in glance.yml")
	}

	data := url.Values{}
	data.Set("refresh_token", refreshToken)

	tokenURL := "https://api.meethue.com/oauth2/refresh?grant_type=refresh_token"
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(hueConfig.ClientID + ":" + hueConfig.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hueHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("[Hue] Token refresh failed", "status", resp.StatusCode, "body", string(body))
		return "", fmt.Errorf("hue token refresh failed: status %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	_ = Store.SetSetting("hue_access_token", res.AccessToken)
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		_ = Store.SetSetting("hue_access_token_expiry", strconv.FormatInt(expiryTime, 10))
	}

	return res.AccessToken, nil
}

func hueTokenStillValid() (string, bool) {
	accessToken, _ := Store.GetSetting("hue_access_token", "")
	tokenExpiryStr, _ := Store.GetSetting("hue_access_token_expiry", "")
	if accessToken == "" || tokenExpiryStr == "" {
		return "", false
	}
	expiry, err := strconv.ParseInt(tokenExpiryStr, 10, 64)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() >= expiry-60 {
		return "", false
	}
	return accessToken, true
}

// pairHueRemote links the remote Hue API to generate a username (application key)
func pairHueRemote(accessToken string) (string, error) {
	// 1. Simulating link button press via Remote config PUT
	// PUT https://api.meethue.com/route/api/0/config
	linkURL := "https://api.meethue.com/route/api/0/config"
	body := []byte(`{"linkbutton": true}`)
	req, err := http.NewRequest("PUT", linkURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := hueHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	// 2. Generate username via Remote API POST
	// POST https://api.meethue.com/route/api
	pairURL := "https://api.meethue.com/route/api"
	pBody := []byte(`{"devicetype": "glance_dashboard"}`)
	pReq, err := http.NewRequest("POST", pairURL, bytes.NewBuffer(pBody))
	if err != nil {
		return "", err
	}
	pReq.Header.Set("Authorization", "Bearer "+accessToken)
	pReq.Header.Set("Content-Type", "application/json")

	pResp, err := hueHTTPClient.Do(pReq)
	if err != nil {
		return "", err
	}
	defer pResp.Body.Close()

	var result []struct {
		Success struct {
			Username string `json:"username"`
		} `json:"success"`
		Error struct {
			Description string `json:"description"`
		} `json:"error"`
	}

	if err := json.NewDecoder(pResp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result) > 0 {
		if result[0].Error.Description != "" {
			return "", fmt.Errorf("pairing error: %s", result[0].Error.Description)
		}
		if result[0].Success.Username != "" {
			username := result[0].Success.Username
			_ = Store.SetSetting("hue_username", username)
			_ = Store.SetSetting("hue_authorized", "true")
			return username, nil
		}
	}

	return "", fmt.Errorf("pairing failed, no success username returned")
}

// HueConfigResource represents resource info for checkboxes inside Settings modal
type HueConfigResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // room, light, scene
}

func fetchHueResourcesFromAPI(ctx context.Context) ([]HueConfigResource, error) {
	token, err := getHueAccessToken()
	if err != nil {
		return nil, err
	}

	username, _ := Store.GetSetting("hue_username", "")
	if username == "" {
		// Attempt auto-pairing if token exists but no username is saved
		username, err = pairHueRemote(token)
		if err != nil {
			return nil, err
		}
	}

	var list []HueConfigResource

	// Fetch Rooms
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/room", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID       string `json:"id"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, r := range resData.Data {
					list = append(list, HueConfigResource{ID: r.ID, Name: r.Metadata.Name, Type: "room"})
				}
			}
		}
	}

	// Fetch Lights
	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/light", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID       string `json:"id"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, r := range resData.Data {
					list = append(list, HueConfigResource{ID: r.ID, Name: r.Metadata.Name, Type: "light"})
				}
			}
		}
	}

	// Fetch Scenes
	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/scene", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID       string `json:"id"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, r := range resData.Data {
					list = append(list, HueConfigResource{ID: r.ID, Name: r.Metadata.Name, Type: "scene"})
				}
			}
		}
	}

	return list, nil
}

func fetchHueStatusesFromAPI(ctx context.Context, rooms, lights, scenes []string) ([]widget.HueResource, error) {
	token, err := getHueAccessToken()
	if err != nil {
		return nil, err
	}

	username, _ := Store.GetSetting("hue_username", "")
	if username == "" {
		username, err = pairHueRemote(token)
		if err != nil {
			return nil, err
		}
	}

	// Fetch all lights to build a status map
	lightMap := make(map[string]bool)
	lightNameMap := make(map[string]string)
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/light", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID       string `json:"id"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					On struct {
						On bool `json:"on"`
					} `json:"on"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, l := range resData.Data {
					lightMap[l.ID] = l.On.On
					lightNameMap[l.ID] = l.Metadata.Name
				}
			}
		}
	}

	// Fetch all rooms to get their grouped_light status and names
	roomGroupedLightMap := make(map[string]string) // roomID -> groupedLightID
	roomNameMap := make(map[string]string)
	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/room", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID       string `json:"id"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Services []struct {
						Rid   string `json:"rid"`
						Rtype string `json:"rtype"`
					} `json:"services"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, rm := range resData.Data {
					roomNameMap[rm.ID] = rm.Metadata.Name
					for _, s := range rm.Services {
						if s.Rtype == "grouped_light" {
							roomGroupedLightMap[rm.ID] = s.Rid
						}
					}
				}
			}
		}
	}

	// Fetch all grouped_lights to get room status
	groupedLightStateMap := make(map[string]bool)
	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/grouped_light", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID string `json:"id"`
					On struct {
						On bool `json:"on"`
					} `json:"on"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, gl := range resData.Data {
					groupedLightStateMap[gl.ID] = gl.On.On
				}
			}
		}
	}

	// Fetch all scenes to get their names and group associations
	sceneNameMap := make(map[string]string)
	sceneGroupMap := make(map[string]string)
	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/scene", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					ID       string `json:"id"`
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Group struct {
						Rid   string `json:"rid"`
						Rtype string `json:"rtype"`
					} `json:"group"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil {
				for _, sc := range resData.Data {
					sceneNameMap[sc.ID] = sc.Metadata.Name
					sceneGroupMap[sc.ID] = sc.Group.Rid
				}
			}
		}
	}

	var results []widget.HueResource

	// 1. Process Rooms
	for _, roomID := range rooms {
		name := roomNameMap[roomID]
		if name == "" {
			name = "Unbekannter Raum"
		}
		glID := roomGroupedLightMap[roomID]
		isOn := groupedLightStateMap[glID]
		results = append(results, widget.HueResource{
			ID:   roomID,
			Name: name,
			Type: "room",
			On:   isOn,
		})
	}

	// 2. Process Lights
	for _, lightID := range lights {
		name := lightNameMap[lightID]
		if name == "" {
			name = "Unbekannte Lampe"
		}
		isOn := lightMap[lightID]
		results = append(results, widget.HueResource{
			ID:   lightID,
			Name: name,
			Type: "light",
			On:   isOn,
		})
	}

	// 3. Process Scenes
	for _, sceneID := range scenes {
		name := sceneNameMap[sceneID]
		if name == "" {
			name = "Unbekannte Szene"
		}

		groupRid := sceneGroupMap[sceneID]

		// Determine if the scene's associated room/zone is turned on
		roomIsOn := false
		if groupRid != "" {
			glID := roomGroupedLightMap[groupRid]
			roomIsOn = groupedLightStateMap[glID]
		}

		// A scene is active if its group is on and it is the last active scene for that group
		lastActiveSceneMu.RLock()
		activeSceneID := lastActiveScene[groupRid]
		lastActiveSceneMu.RUnlock()

		isOn := roomIsOn && (activeSceneID == sceneID)

		results = append(results, widget.HueResource{
			ID:      sceneID,
			Name:    name,
			Type:    "scene",
			On:      isOn,
			GroupID: groupRid,
		})
	}

	return results, nil
}

func controlHueResource(ctx context.Context, id, rtype string, state bool) error {
	token, err := getHueAccessToken()
	if err != nil {
		return err
	}

	username, _ := Store.GetSetting("hue_username", "")
	if username == "" {
		return fmt.Errorf("hue is not paired, please authorize first")
	}

	var apiURL string
	var body []byte

	switch rtype {
	case "light":
		apiURL = fmt.Sprintf("https://api.meethue.com/route/clip/v2/resource/light/%s", id)
		body, _ = json.Marshal(map[string]interface{}{
			"on": map[string]bool{"on": state},
		})
	case "room", "zone":
		// Resolve room/zone grouped_light ID first
		groupGLID := ""
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/"+rtype+"/"+id, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("hue-application-key", username)
		if resp, err := hueHTTPClient.Do(req); err == nil {
			defer resp.Body.Close()
			var resData struct {
				Data []struct {
					Services []struct {
						Rid   string `json:"rid"`
						Rtype string `json:"rtype"`
					} `json:"services"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&resData) == nil && len(resData.Data) > 0 {
				for _, s := range resData.Data[0].Services {
					if s.Rtype == "grouped_light" {
						groupGLID = s.Rid
						break
					}
				}
			}
		}

		if groupGLID == "" {
			return fmt.Errorf("could not find grouped_light for %s %s", rtype, id)
		}

		apiURL = fmt.Sprintf("https://api.meethue.com/route/clip/v2/resource/grouped_light/%s", groupGLID)
		body, _ = json.Marshal(map[string]interface{}{
			"on": map[string]bool{"on": state},
		})

		// If turning off the room/zone, clear the last active scene for it
		if !state {
			lastActiveSceneMu.Lock()
			lastActiveScene[id] = ""
			lastActiveSceneMu.Unlock()
		}

	case "scene":
		if state {
			apiURL = fmt.Sprintf("https://api.meethue.com/route/clip/v2/resource/scene/%s", id)
			body, _ = json.Marshal(map[string]interface{}{
				"recall": map[string]string{"action": "active"},
			})

			// Resolve scene's group (room/zone) ID so we can track the active scene
			var groupRid string
			req, err := http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/scene/"+id, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("hue-application-key", username)
				if resp, err := hueHTTPClient.Do(req); err == nil {
					defer resp.Body.Close()
					var resData struct {
						Data []struct {
							Group struct {
								Rid   string `json:"rid"`
								Rtype string `json:"rtype"`
							} `json:"group"`
						} `json:"data"`
					}
					if json.NewDecoder(resp.Body).Decode(&resData) == nil && len(resData.Data) > 0 {
						groupRid = resData.Data[0].Group.Rid
					}
				}
			}
			if groupRid != "" {
				lastActiveSceneMu.Lock()
				lastActiveScene[groupRid] = id
				lastActiveSceneMu.Unlock()
			}
		} else {
			// Turning OFF a scene: resolve the scene's group (room/zone) ID first
			var groupRid string
			var groupRtype string
			req, err := http.NewRequestWithContext(ctx, "GET", "https://api.meethue.com/route/clip/v2/resource/scene/"+id, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("hue-application-key", username)
				if resp, err := hueHTTPClient.Do(req); err == nil {
					defer resp.Body.Close()
					var resData struct {
						Data []struct {
							Group struct {
								Rid   string `json:"rid"`
								Rtype string `json:"rtype"`
							} `json:"group"`
						} `json:"data"`
					}
					if json.NewDecoder(resp.Body).Decode(&resData) == nil && len(resData.Data) > 0 {
						groupRid = resData.Data[0].Group.Rid
						groupRtype = resData.Data[0].Group.Rtype
					}
				}
			}
			if groupRid != "" && groupRtype != "" {
				// Turn off the group (room/zone) recursively
				err := controlHueResource(ctx, groupRid, groupRtype, false)
				if err != nil {
					return err
				}
				lastActiveSceneMu.Lock()
				lastActiveScene[groupRid] = ""
				lastActiveSceneMu.Unlock()
				return nil
			}
			return fmt.Errorf("could not find group for scene %s", id)
		}
	default:
		return fmt.Errorf("unsupported resource type: %s", rtype)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("hue-application-key", username)
	req.Header.Set("Content-Type", "application/json")

	resp, err := hueHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("control request failed: status %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}
