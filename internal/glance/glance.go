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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/widget"
	"gopkg.in/yaml.v3"
)

var buildVersion = "dev"
var buildNumber = "1"

var sequentialWhitespacePattern = regexp.MustCompile(`\s+`)
var sequentialHyphenPattern = regexp.MustCompile(`-+`)
var invalidSlugCharsPattern = regexp.MustCompile(`[^a-z0-9\-]`)

type Application struct {
	Version     string
	BuildNumber string
	Config      Config
	ConfigPath  string
	slugToPage  map[string]*Page
	configMu    sync.RWMutex
	configFileMu sync.Mutex
}

type Theme struct {
	BackgroundColor                *widget.HSLColorField `yaml:"background-color"`
	PrimaryColor                   *widget.HSLColorField `yaml:"primary-color"`
	PositiveColor                  *widget.HSLColorField `yaml:"positive-color"`
	NegativeColor                  *widget.HSLColorField `yaml:"negative-color"`
	Light                          bool                  `yaml:"light"`
	ContrastMultiplier             float32               `yaml:"contrast-multiplier"`
	TextSaturationMultiplier       float32               `yaml:"text-saturation-multiplier"`
	CustomCSSFile                  string                `yaml:"custom-css-file"`
	WidgetGap                      *string               `yaml:"widget-gap,omitempty"`
	WidgetContentVerticalPadding   *string               `yaml:"widget-content-vertical-padding,omitempty"`
	WidgetContentHorizontalPadding *string               `yaml:"widget-content-horizontal-padding,omitempty"`
	BorderRadius                   *string               `yaml:"border-radius,omitempty"`
}

type Server struct {
	Host       string    `yaml:"host"`
	Port       uint16    `yaml:"port"`
	AssetsPath string    `yaml:"assets-path"`
	Timezone   string    `yaml:"timezone"`
	StartedAt  time.Time `yaml:"-"`
}

type Column struct {
	Size    string         `yaml:"size"`
	Widgets widget.Widgets `yaml:"widgets"`
}

type templateData struct {
	App  *Application
	Page *Page
}

type Page struct {
	Title                 string         `yaml:"name"`
	Slug                  string         `yaml:"slug"`
	ShowMobileHeader      bool           `yaml:"show-mobile-header"`
	HideDesktopNavigation bool           `yaml:"hide-desktop-navigation"`
	HeadWidgets           widget.Widgets `yaml:"head-widgets"`
	Columns               []Column       `yaml:"columns"`
	mu                    sync.Mutex
	isUpdating            bool           `yaml:"-"`
}

func (p *Page) UpdateOutdatedWidgets() bool {
	now := time.Now()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Semaphore to bound concurrency and prevent resource exhaustion
	const maxConcurrentUpdates = 5
	var sem = make(chan struct{}, maxConcurrentUpdates)
	
	var mu sync.Mutex
	anyUpdated := false

	// Update columns widgets
	for c := range p.Columns {
		for w := range p.Columns[c].Widgets {
			wItem := p.Columns[c].Widgets[w]

			if !wItem.RequiresUpdate(&now) {
				continue
			}

			wg.Add(1)
			go func(wd widget.Widget) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				wd.Update(ctx)
				mu.Lock()
				anyUpdated = true
				mu.Unlock()
			}(wItem)
		}
	}

	// Update head widgets
	for w := range p.HeadWidgets {
		wItem := p.HeadWidgets[w]

		if !wItem.RequiresUpdate(&now) {
			continue
		}

		wg.Add(1)
		go func(wd widget.Widget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			wd.Update(ctx)
			mu.Lock()
			anyUpdated = true
			mu.Unlock()
		}(wItem)
	}

	wg.Wait()
	return anyUpdated
}

func titleToSlug(s string) string {
	s = strings.ToLower(s)
	s = sequentialWhitespacePattern.ReplaceAllString(s, "-")
	s = invalidSlugCharsPattern.ReplaceAllString(s, "")
	s = sequentialHyphenPattern.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	return s
}

func NewApplication(config *Config, configPath string) (*Application, error) {
	if len(config.Pages) == 0 {
		return nil, fmt.Errorf("no pages configured")
	}

	app := &Application{
		Version:     buildVersion,
		BuildNumber: buildNumber,
		Config:      *config,
		ConfigPath:  configPath,
		slugToPage:  make(map[string]*Page),
	}

	app.slugToPage[""] = &config.Pages[0]

	for i := range config.Pages {
		if config.Pages[i].Slug == "" {
			config.Pages[i].Slug = titleToSlug(config.Pages[i].Title)
		}

		app.slugToPage[config.Pages[i].Slug] = &config.Pages[i]
	}

	return app, nil
}

func (a *Application) HandlePageRequest(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	page, exists := a.slugToPage[r.PathValue("page")]
	a.configMu.RUnlock()

	if !exists {
		a.HandleNotFound(w, r)
		return
	}

	pageData := templateData{
		Page: page,
		App:  a,
	}

	var responseBytes bytes.Buffer
	err := assets.PageTemplate.Execute(&responseBytes, pageData)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write(responseBytes.Bytes())
}

func (a *Application) HandlePageContentRequest(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	page, exists := a.slugToPage[r.PathValue("page")]
	a.configMu.RUnlock()

	if !exists {
		a.HandleNotFound(w, r)
		return
	}

	pageData := templateData{
		Page: page,
	}

	var responseBytes bytes.Buffer
	err := assets.PageContentTemplate.Execute(&responseBytes, pageData)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write(responseBytes.Bytes())
}

func (a *Application) HandleNotFound(w http.ResponseWriter, r *http.Request) {
	// TODO: add proper not found page
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Page not found"))
}

func FileServerWithCache(fs http.FileSystem, cacheDuration time.Duration) http.Handler {
	server := http.FileServer(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: fix always setting cache control even if the file doesn't exist
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cacheDuration.Seconds())))
		server.ServeHTTP(w, r)
	})
}

func (a *Application) Serve() error {
	// TODO: add gzip support, static files must have their gzipped contents cached
	// TODO: add HTTPS support
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", a.HandlePageRequest)
	mux.HandleFunc("GET /{page}", a.HandlePageRequest)
	mux.HandleFunc("GET /api/pages/{page}/content/{$}", a.HandlePageContentRequest)
	mux.Handle("GET /static/{path...}", http.StripPrefix("/static/", FileServerWithCache(http.FS(assets.PublicFS), 2*time.Hour)))

	// Register WebSocket endpoint
	mux.HandleFunc("GET /api/ws", handleWebSocket)

	// Spotify Auth routes
	mux.HandleFunc("GET /api/spotify/login", a.HandleSpotifyLogin)
	mux.HandleFunc("GET /api/spotify/callback", a.HandleSpotifyCallback)

	// Google Calendar Auth routes
	mux.HandleFunc("GET /api/google/login", a.HandleGoogleLogin)
	mux.HandleFunc("GET /api/google/callback", a.HandleGoogleCallback)
	mux.HandleFunc("GET /api/google/calendars", a.HandleGoogleCalendarsGet)

	// Spotify playback actions
	mux.HandleFunc("POST /api/spotify/play", a.HandleSpotifyPlay)
	mux.HandleFunc("POST /api/spotify/pause", a.HandleSpotifyPause)
	mux.HandleFunc("POST /api/spotify/skip", a.HandleSpotifySkip)
	mux.HandleFunc("POST /api/spotify/volume", a.HandleSpotifyVolume)

	// Layout and widget configuration API endpoints
	mux.HandleFunc("POST /api/layout/save", a.HandleLayoutSave)
	mux.HandleFunc("POST /api/widgets/add", a.HandleWidgetAdd)
	mux.HandleFunc("POST /api/widgets/delete", a.HandleWidgetDelete)
	mux.HandleFunc("GET /api/widgets/get", a.HandleWidgetGet)
	mux.HandleFunc("POST /api/widgets/update", a.HandleWidgetUpdate)

	// Settings & Page API endpoints
	mux.HandleFunc("GET /api/settings", a.HandleSettingsGet)
	mux.HandleFunc("POST /api/settings/save", a.HandleSettingsSave)
	mux.HandleFunc("POST /api/pages/add", a.HandlePageAdd)

	mux.HandleFunc("POST /api/config/import", a.HandleConfigImport)

	if a.Config.Server.AssetsPath != "" {
		absAssetsPath, err := filepath.Abs(a.Config.Server.AssetsPath)

		if err != nil {
			return fmt.Errorf("invalid assets path: %s", a.Config.Server.AssetsPath)
		}

		slog.Info("Serving assets", "path", absAssetsPath)
		assetsFS := FileServerWithCache(http.Dir(a.Config.Server.AssetsPath), 2*time.Hour)
		mux.Handle("/assets/{path...}", http.StripPrefix("/assets/", assetsFS))
	}

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.Config.Server.Host, a.Config.Server.Port),
		Handler: mux,
	}

	slog.Info("Starting server", "host", a.Config.Server.Host, "port", a.Config.Server.Port)
	
	// Pre-warm caches for all pages in the background upon startup
	go func() {
		for i := range a.Config.Pages {
			page := &a.Config.Pages[i]
			slog.Info("Pre-warming widget cache for page", "page", page.Title)
			if updated := page.UpdateOutdatedWidgets(); !updated {
				slog.Info("Pre-warm completed for page (no updates needed)", "page", page.Title)
			} else {
				slog.Info("Pre-warm completed for page", "page", page.Title)
			}
		}
		slog.Info("Pre-warming widget cache completed")
	}()

	return server.ListenAndServe()
}

// HandleSpotifyLogin initiates the Spotify OAuth login sequence.
func (a *Application) HandleSpotifyLogin(w http.ResponseWriter, r *http.Request) {
	if spotifyConfig.ClientID == "" {
		http.Error(w, "Spotify Client ID is not configured. Add it under the spotify: block in glance.yml or as SPOTIFY_CLIENT_ID environment variable.", http.StatusInternalServerError)
		return
	}

	redirectURI := getSpotifyRedirectURI(r)
	slog.Info("[Spotify] Initiating OAuth login", "redirect_uri", redirectURI, "configured_url", spotifyConfig.RedirectURL, "request_host", r.Host)
	scopes := "user-read-playback-state user-modify-playback-state"
	authURL := fmt.Sprintf("https://accounts.spotify.com/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s",
		url.QueryEscape(spotifyConfig.ClientID),
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

	redirectURI := getSpotifyRedirectURI(r)
	slog.Info("[Spotify] Callback received", "redirect_uri", redirectURI, "configured_url", spotifyConfig.RedirectURL)
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth := base64.StdEncoding.EncodeToString([]byte(spotifyConfig.ClientID + ":" + spotifyConfig.ClientSecret))
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

	if err := dbSetSetting("spotify_access_token", res.AccessToken); err != nil {
		slog.Error("[Spotify] Failed to persist access token", "error", err)
	}
	if err := dbSetSetting("spotify_refresh_token", res.RefreshToken); err != nil {
		slog.Error("[Spotify] Failed to persist refresh token", "error", err)
	}
	if res.ExpiresIn > 0 {
		expiryTime := time.Now().Unix() + int64(res.ExpiresIn)
		if err := dbSetSetting("spotify_access_token_expiry", strconv.FormatInt(expiryTime, 10)); err != nil {
			slog.Error("[Spotify] Failed to persist token expiry", "error", err)
		}
	}
	if err := dbSetSetting("spotify_authorized", "true"); err != nil {
		slog.Error("[Spotify] Failed to persist authorized flag", "error", err)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleSpotifyPlay sends the playback resume command.
func (a *Application) HandleSpotifyPlay(w http.ResponseWriter, r *http.Request) {
	if err := spotifyControlAction("PUT", "play", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	TriggerImmediateSpotifyBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleSpotifyPause sends the playback pause command.
func (a *Application) HandleSpotifyPause(w http.ResponseWriter, r *http.Request) {
	if err := spotifyControlAction("PUT", "pause", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	TriggerImmediateSpotifyBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleSpotifySkip skips to the next or previous track.
func (a *Application) HandleSpotifySkip(w http.ResponseWriter, r *http.Request) {
	direction := r.URL.Query().Get("direction")
	action := "next"
	if direction == "prev" {
		action = "previous"
	}
	if err := spotifyControlAction("POST", action, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	TriggerImmediateSpotifyBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleSpotifyVolume sets the active player device volume percent.
func (a *Application) HandleSpotifyVolume(w http.ResponseWriter, r *http.Request) {
	volumeStr := r.URL.Query().Get("volume")
	volume, err := strconv.Atoi(volumeStr)
	if err != nil || volume < 0 || volume > 100 {
		http.Error(w, "invalid volume percentage", http.StatusBadRequest)
		return
	}

	if err := spotifyControlAction("PUT", fmt.Sprintf("volume?volume_percent=%d", volume), nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	TriggerImmediateSpotifyBroadcast()
	w.WriteHeader(http.StatusOK)
}

// HandleLayoutSave rearranges column and header widgets inside glance.yml on disk and reloads configuration.
func (a *Application) HandleLayoutSave(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

	// Limit request body size to prevent OOM
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

	// Read original config YAML as a Node AST tree to preserve formatting/comments
	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pageNode, err := findPageNode(&rootNode, payload.PageSlug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	columnsNode := findMapValue(pageNode, "columns")
	if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
		http.Error(w, "columns not found on page", http.StatusInternalServerError)
		return
	}

	// Build a map of all original widgets by their coordinates before making any edits.
	// This prevents AST mutations from corrupting our ability to look up widgets.
	originalNodesMap := make(map[string]*yaml.Node)

	headNode := findMapValue(pageNode, "head-widgets")
	if headNode != nil && headNode.Kind == yaml.SequenceNode {
		for wIdx, w := range headNode.Content {
			id := fmt.Sprintf("head:%d", wIdx)
			originalNodesMap[id] = w

			// If it's a group, catalog its original children
			widgetsNode := findMapValue(w, "widgets")
			if widgetsNode != nil && widgetsNode.Kind == yaml.SequenceNode {
				for nwIdx, nw := range widgetsNode.Content {
					childId := fmt.Sprintf("head:%d:%d", wIdx, nwIdx)
					originalNodesMap[childId] = nw
				}
			}
		}
	}

	// Catalog widgets from columns
	if columnsNode != nil && columnsNode.Kind == yaml.SequenceNode {
		for colIdx, colNode := range columnsNode.Content {
			widgetsNode := findMapValue(colNode, "widgets")
			if widgetsNode != nil && widgetsNode.Kind == yaml.SequenceNode {
				for wIdx, w := range widgetsNode.Content {
					id := fmt.Sprintf("%d:%d", colIdx, wIdx)
					originalNodesMap[id] = w

					// If it's a group, catalog its original children
					groupWidgets := findMapValue(w, "widgets")
					if groupWidgets != nil && groupWidgets.Kind == yaml.SequenceNode {
						for nwIdx, nw := range groupWidgets.Content {
							childId := fmt.Sprintf("%d:%d:%d", colIdx, wIdx, nwIdx)
							originalNodesMap[childId] = nw
						}
					}
				}
			}
		}
	}

	// Helper function to resolve old widget node using the pre-cataloged map
	lookupNode := func(idStr string) *yaml.Node {
		return originalNodesMap[idStr]
	}

	// collectReferencedIds extracts all base widget IDs from the payload before mutation
	collectReferencedIds := func() []string {
		var ids []string
		for _, idStr := range payload.Head {
			if strings.Contains(idStr, "[") {
				baseId := idStr[:strings.Index(idStr, "[")]
				ids = append(ids, baseId)
			} else {
				ids = append(ids, idStr)
			}
		}
		for _, col := range payload.Columns {
			for _, idStr := range col {
				if strings.Contains(idStr, "[") {
					baseId := idStr[:strings.Index(idStr, "[")]
					ids = append(ids, baseId)
				} else {
					ids = append(ids, idStr)
				}
			}
		}
		return ids
	}

	// Validate that every referenced widget ID exists in the original nodes map
	for _, id := range collectReferencedIds() {
		if lookupNode(id) == nil {
			slog.Warn("Layout save referenced unknown widget", "id", id)
			http.Error(w, "unknown widget reference: "+id, http.StatusBadRequest)
			return
		}
	}

	// Helper function to recursively resolve widgets (including Groups with nested children)
	var resolveWidgetNode func(idStr string) *yaml.Node
	resolveWidgetNode = func(idStr string) *yaml.Node {
		if strings.Contains(idStr, "[") {
			openBrack := strings.Index(idStr, "[")
			closeBrack := strings.Index(idStr, "]")
			if openBrack == -1 || closeBrack == -1 || closeBrack <= openBrack {
				return nil
			}
			baseId := idStr[:openBrack]
			childrenStr := idStr[openBrack+1 : closeBrack]

			groupNode := lookupNode(baseId)
			if groupNode == nil {
				return nil
			}

			widgetsNode := findMapValue(groupNode, "widgets")
			if widgetsNode == nil {
				widgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				groupNode.Content = append(groupNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "widgets"}, widgetsNode)
			}

			widgetsNode.Content = []*yaml.Node{}
			if childrenStr != "" {
				childIds := strings.Split(childrenStr, ",")
				for _, childId := range childIds {
					childNode := resolveWidgetNode(strings.TrimSpace(childId))
					if childNode != nil {
						widgetsNode.Content = append(widgetsNode.Content, childNode)
					}
				}
			}
			return groupNode
		}
		return lookupNode(idStr)
	}

	// Reconstruct columns array if new column sizes are provided
	if len(payload.ColumnSizes) > 0 {
		var newCols []*yaml.Node
		for _, size := range payload.ColumnSizes {
			colMap := map[string]interface{}{
				"size":    size,
				"widgets": []interface{}{},
			}
			var colDoc yaml.Node
			_ = colDoc.Encode(colMap)
			var colNode *yaml.Node
			if colDoc.Kind == yaml.DocumentNode && len(colDoc.Content) > 0 {
				colNode = colDoc.Content[0]
			} else {
				colNode = &colDoc
			}
			newCols = append(newCols, colNode)
		}
		columnsNode.Content = newCols
	}

	// 3. Reconstruct head-widgets if payload contains them or page had them
	if len(payload.Head) > 0 {
		if headNode == nil {
			headKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "head-widgets"}
			headNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			pageNode.Content = append(pageNode.Content, headKeyNode, headNode)
		}
		headNode.Content = []*yaml.Node{}
		for _, idStr := range payload.Head {
			node := resolveWidgetNode(idStr)
			if node != nil {
				headNode.Content = append(headNode.Content, node)
			}
		}
	} else if headNode != nil {
		headNode.Content = []*yaml.Node{}
	}

	// 4. Re-distribute nodes based on the incoming layout indexes in columns
	for colIdx, colNode := range columnsNode.Content {
		widgetsNode := findMapValue(colNode, "widgets")
		if widgetsNode == nil {
			widgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			colNode.Content = append(colNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "widgets"}, widgetsNode)
		}

		widgetsNode.Content = []*yaml.Node{}
		if colIdx < len(payload.Columns) {
			for _, idStr := range payload.Columns[colIdx] {
				node := resolveWidgetNode(idStr)
				if node != nil {
					widgetsNode.Content = append(widgetsNode.Content, node)
				}
			}
		}
	}

	// Validate AST config before saving
	if err := validateASTConfig(&rootNode); err != nil {
		http.Error(w, "invalid layout structure: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Write AST node back to disk
	if err := saveNodeToDisk(a.ConfigPath, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot-reload config in-memory
	if err := a.reloadConfig(); err != nil {
		http.Error(w, "saved layout but failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// JSONStringOrInt represents a string that can be unmarshaled from either a JSON string or a JSON number.
// This handles cases where client payloads serialize integer column indexes as actual numbers instead of strings.
type JSONStringOrInt string

// UnmarshalJSON parses both numeric values and strings into the JSONStringOrInt type.
func (s *JSONStringOrInt) UnmarshalJSON(data []byte) error {
	var val interface{}
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	switch v := val.(type) {
	case string:
		*s = JSONStringOrInt(v)
	case float64:
		*s = JSONStringOrInt(strconv.Itoa(int(v)))
	default:
		return fmt.Errorf("invalid type for column index: expected string or number, got %T", val)
	}
	return nil
}

// HandleWidgetAdd appends a new widget schema directly into a column in glance.yml.
func (a *Application) HandleWidgetAdd(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

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

	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(rootNode.Content) == 0 {
		http.Error(w, "empty YAML document", http.StatusInternalServerError)
		return
	}
	rootMap := rootNode.Content[0]

	// Process Spotify widgets to store credentials globally and tokens in SQLite database
	if payload.Type == "spotify" && payload.Properties != nil {
		var clientID, clientSecret, redirectURL, accessToken, refreshToken string
		if cid, ok := payload.Properties["client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := payload.Properties["client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := payload.Properties["redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}
		if at, ok := payload.Properties["access_token"].(string); ok {
			accessToken = strings.TrimSpace(at)
		}
		if rt, ok := payload.Properties["refresh_token"].(string); ok {
			refreshToken = strings.TrimSpace(rt)
		}

		// Clean up the local widget properties so they are not saved inside the page's widget config
		delete(payload.Properties, "client_id")
		delete(payload.Properties, "client_secret")
		delete(payload.Properties, "redirect_url")
		delete(payload.Properties, "access_token")
		delete(payload.Properties, "refresh_token")

		// Write tokens to SQLite database if provided and not placeholder
		if accessToken != "" && accessToken != "********" {
			if err := dbSetSetting("spotify_access_token", accessToken); err != nil {
				slog.Error("[Spotify] Failed to persist access token", "error", err)
			}
		}
		if refreshToken != "" && refreshToken != "********" {
			if err := dbSetSetting("spotify_refresh_token", refreshToken); err != nil {
				slog.Error("[Spotify] Failed to persist refresh token", "error", err)
			}
		}
		if (accessToken != "" && accessToken != "********") || (refreshToken != "" && refreshToken != "********") {
			if err := dbSetSetting("spotify_authorized", "true"); err != nil {
				slog.Error("[Spotify] Failed to persist authorized flag", "error", err)
			}
		}

		// Write Client ID, Client Secret, and Redirect URL globally under the 'spotify' block in glance.yml if provided
		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			spotifyNode := findMapValue(rootMap, "spotify")
			if spotifyNode == nil || spotifyNode.Kind != yaml.MappingNode {
				spotifyNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "spotify"}
				rootMap.Content = append(rootMap.Content, keyNode, spotifyNode)
			}
			if clientID != "" {
				updateMapValue(spotifyNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(spotifyNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(spotifyNode, "redirect-url", redirectURL)
			}
		}
	}

	// Process Calendar widgets to store Google credentials globally
	if payload.Type == "calendar" && payload.Properties != nil {
		var clientID, clientSecret, redirectURL string
		if cid, ok := payload.Properties["google_client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := payload.Properties["google_client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := payload.Properties["google_redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}

		delete(payload.Properties, "google_client_id")
		delete(payload.Properties, "google_client_secret")
		delete(payload.Properties, "google_redirect_url")

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			googleNode := findMapValue(rootMap, "google")
			if googleNode == nil || googleNode.Kind != yaml.MappingNode {
				googleNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "google"}
				rootMap.Content = append(rootMap.Content, keyNode, googleNode)
			}
			if clientID != "" {
				updateMapValue(googleNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(googleNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(googleNode, "redirect-url", redirectURL)
			}
		}
	}

	pageNode, err := findPageNode(&rootNode, payload.PageSlug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var widgetsNode *yaml.Node

	if payload.ColumnIndex == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			headKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "head-widgets"}
			widgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			pageNode.Content = append(pageNode.Content, headKeyNode, widgetsNode)
		}
	} else {
		columnsNode := findMapValue(pageNode, "columns")
		if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
			http.Error(w, "columns not found", http.StatusInternalServerError)
			return
		}

		colIdx, err := strconv.Atoi(string(payload.ColumnIndex))
		if err != nil || colIdx < 0 || colIdx >= len(columnsNode.Content) {
			http.Error(w, "invalid column index", http.StatusBadRequest)
			return
		}

		colNode := columnsNode.Content[colIdx]
		widgetsNode = findMapValue(colNode, "widgets")
		if widgetsNode == nil {
			widgetsNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			colNode.Content = append(colNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "widgets"}, widgetsNode)
		}
	}

	// Encode new widget map into a yaml Node
	widgetMap := map[string]interface{}{
		"type": payload.Type,
	}
	if payload.Type == "group" {
		widgetMap["widgets"] = []interface{}{}
	}
	for k, v := range payload.Properties {
		widgetMap[k] = v
	}

	newWidgetNode := &yaml.Node{}
	if err := newWidgetNode.Encode(widgetMap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	widgetsNode.Style = 0
	widgetsNode.Content = append(widgetsNode.Content, newWidgetNode)

	// Validate AST config before saving
	if err := validateASTConfig(&rootNode); err != nil {
		http.Error(w, "invalid widget structure: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := saveNodeToDisk(a.ConfigPath, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.reloadConfig(); err != nil {
		http.Error(w, "added widget but failed to reload config: "+err.Error(), http.StatusInternalServerError)
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

// HandleWidgetDelete deletes a widget from a specific column of glance.yml.
func (a *Application) HandleWidgetDelete(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

	// Limit request body size to prevent OOM
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

	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pageNode, err := findPageNode(&rootNode, payload.PageSlug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var widgetsNode *yaml.Node

	if payload.ColumnIndex == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			http.Error(w, "head-widgets not found", http.StatusBadRequest)
			return
		}
	} else if strings.Contains(string(payload.ColumnIndex), ":") {
		// Nested widget inside a group!
		// Format: "origCol:groupWidgetIdx" (e.g. "1:0" or "head:0")
		parts := strings.Split(string(payload.ColumnIndex), ":")
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

		if baseNode == nil {
			http.Error(w, "group widget not found", http.StatusBadRequest)
			return
		}

		widgetsNode = findMapValue(baseNode, "widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			http.Error(w, "nested widgets sequence not found in group", http.StatusBadRequest)
			return
		}
	} else {
		columnsNode := findMapValue(pageNode, "columns")
		if columnsNode == nil || columnsNode.Kind != yaml.SequenceNode {
			http.Error(w, "columns not found", http.StatusInternalServerError)
			return
		}

		colIdx, err := strconv.Atoi(string(payload.ColumnIndex))
		if err != nil || colIdx < 0 || colIdx >= len(columnsNode.Content) {
			http.Error(w, "invalid column index", http.StatusBadRequest)
			return
		}

		colNode := columnsNode.Content[colIdx]
		widgetsNode = findMapValue(colNode, "widgets")
		if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode {
			http.Error(w, "no widgets inside target column", http.StatusBadRequest)
			return
		}
	}

	if payload.WidgetIndex < 0 || payload.WidgetIndex >= len(widgetsNode.Content) {
		http.Error(w, "invalid widget index", http.StatusBadRequest)
		return
	}

	// Delete from sequence content
	widgetsNode.Content = append(widgetsNode.Content[:payload.WidgetIndex], widgetsNode.Content[payload.WidgetIndex+1:]...)

	// Validate AST config before saving
	if err := validateASTConfig(&rootNode); err != nil {
		http.Error(w, "invalid widget deletion: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := saveNodeToDisk(a.ConfigPath, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.reloadConfig(); err != nil {
		http.Error(w, "deleted widget but failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleWidgetGet retrieves the type and properties of a single widget from glance.yml.
func (a *Application) HandleWidgetGet(w http.ResponseWriter, r *http.Request) {
	pageSlug := r.URL.Query().Get("page")
	columnStr := r.URL.Query().Get("column")
	widgetIdxStr := r.URL.Query().Get("widget")

	widgetIdx, err := strconv.Atoi(widgetIdxStr)
	if err != nil {
		http.Error(w, "invalid widget index", http.StatusBadRequest)
		return
	}

	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
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
		accessToken, _ := dbGetSetting("spotify_access_token", "")
		refreshToken, _ := dbGetSetting("spotify_refresh_token", "")
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

// HandleWidgetUpdate edits the parameters of an existing widget directly inside glance.yml.
func (a *Application) HandleWidgetUpdate(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

	// Limit request body size to prevent OOM
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

	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(rootNode.Content) == 0 {
		http.Error(w, "empty YAML document", http.StatusInternalServerError)
		return
	}
	rootMap := rootNode.Content[0]

	pageNode, err := findPageNode(&rootNode, payload.PageSlug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var widgetsNode *yaml.Node

	if payload.ColumnIndex == "head" {
		widgetsNode = findMapValue(pageNode, "head-widgets")
	} else if strings.Contains(string(payload.ColumnIndex), ":") {
		parts := strings.Split(string(payload.ColumnIndex), ":")
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
		colIdx, err := strconv.Atoi(string(payload.ColumnIndex))
		if err == nil && columnsNode != nil && columnsNode.Kind == yaml.SequenceNode && colIdx >= 0 && colIdx < len(columnsNode.Content) {
			colNode := columnsNode.Content[colIdx]
			widgetsNode = findMapValue(colNode, "widgets")
		}
	}

	if widgetsNode == nil || widgetsNode.Kind != yaml.SequenceNode || payload.WidgetIndex < 0 || payload.WidgetIndex >= len(widgetsNode.Content) {
		http.Error(w, "widget not found", http.StatusNotFound)
		return
	}

	targetWidgetNode := widgetsNode.Content[payload.WidgetIndex]
	var currentWidgetMap map[string]interface{}
	if err := targetWidgetNode.Decode(&currentWidgetMap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	widgetType, _ := currentWidgetMap["type"].(string)

	// Process and clean up Spotify credentials if present
	if widgetType == "spotify" && payload.Properties != nil {
		var clientID, clientSecret, redirectURL, accessToken, refreshToken string
		if cid, ok := payload.Properties["client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := payload.Properties["client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := payload.Properties["redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}
		if at, ok := payload.Properties["access_token"].(string); ok {
			accessToken = strings.TrimSpace(at)
		}
		if rt, ok := payload.Properties["refresh_token"].(string); ok {
			refreshToken = strings.TrimSpace(rt)
		}

		delete(payload.Properties, "client_id")
		delete(payload.Properties, "client_secret")
		delete(payload.Properties, "redirect_url")
		delete(payload.Properties, "access_token")
		delete(payload.Properties, "refresh_token")

		if accessToken != "" && accessToken != "********" {
			if err := dbSetSetting("spotify_access_token", accessToken); err != nil {
				slog.Error("[Spotify] Failed to persist access token", "error", err)
			}
		}
		if refreshToken != "" && refreshToken != "********" {
			if err := dbSetSetting("spotify_refresh_token", refreshToken); err != nil {
				slog.Error("[Spotify] Failed to persist refresh token", "error", err)
			}
		}
		if (accessToken != "" && accessToken != "********") || (refreshToken != "" && refreshToken != "********") {
			if err := dbSetSetting("spotify_authorized", "true"); err != nil {
				slog.Error("[Spotify] Failed to persist authorized flag", "error", err)
			}
		}

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			spotifyNode := findMapValue(rootMap, "spotify")
			if spotifyNode == nil || spotifyNode.Kind != yaml.MappingNode {
				spotifyNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "spotify"}
				rootMap.Content = append(rootMap.Content, keyNode, spotifyNode)
			}
			if clientID != "" {
				updateMapValue(spotifyNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(spotifyNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(spotifyNode, "redirect-url", redirectURL)
			}
		}
	}

	// Process Calendar widgets to store Google credentials globally
	if widgetType == "calendar" && payload.Properties != nil {
		var clientID, clientSecret, redirectURL string
		if cid, ok := payload.Properties["google_client_id"].(string); ok {
			clientID = strings.TrimSpace(cid)
		}
		if csec, ok := payload.Properties["google_client_secret"].(string); ok {
			clientSecret = strings.TrimSpace(csec)
		}
		if rurl, ok := payload.Properties["google_redirect_url"].(string); ok {
			redirectURL = strings.TrimSpace(rurl)
		}

		delete(payload.Properties, "google_client_id")
		delete(payload.Properties, "google_client_secret")
		delete(payload.Properties, "google_redirect_url")

		if clientID != "" || (clientSecret != "" && clientSecret != "********") || redirectURL != "" {
			googleNode := findMapValue(rootMap, "google")
			if googleNode == nil || googleNode.Kind != yaml.MappingNode {
				googleNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "google"}
				rootMap.Content = append(rootMap.Content, keyNode, googleNode)
			}
			if clientID != "" {
				updateMapValue(googleNode, "client-id", clientID)
			}
			if clientSecret != "" && clientSecret != "********" {
				updateMapValue(googleNode, "client-secret", clientSecret)
			}
			if redirectURL != "" {
				updateMapValue(googleNode, "redirect-url", redirectURL)
			}
		}
	}

	// Prepare updated widget dictionary by merging new properties over existing config.
	// This preserves fields not represented in the edit form (e.g. cache, limit, collapse-after).
	newWidgetMap := map[string]interface{}{
		"type": widgetType,
	}
	for k, v := range currentWidgetMap {
		if k == "type" {
			continue
		}
		newWidgetMap[k] = v
	}
	for k, v := range payload.Properties {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			switch k {
			case "update-interval", "height", "limit", "collapse-after", "thumbnail-height", "viewport-limit", "max-days-ahead":
				v = int(f)
			}
		}
		if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(s); err == nil {
				switch k {
				case "update-interval", "height", "limit", "collapse-after", "thumbnail-height", "viewport-limit", "max-days-ahead":
					v = n
				}
			}
		}
		newWidgetMap[k] = v
	}

	newWidgetNode := &yaml.Node{}
	if err := newWidgetNode.Encode(newWidgetMap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	widgetsNode.Content[payload.WidgetIndex] = newWidgetNode

	// Validate AST config before saving
	if err := validateASTConfig(&rootNode); err != nil {
		http.Error(w, "invalid widget properties: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := saveNodeToDisk(a.ConfigPath, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.reloadConfig(); err != nil {
		http.Error(w, "updated widget but failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if widgetType == "spotify" {
		InitSpotify(a.Config.Spotify.ClientID, a.Config.Spotify.ClientSecret, a.Config.Spotify.RedirectURL)
	}
	if widgetType == "calendar" {
		InitGoogle(a.Config.Google.ClientID, a.Config.Google.ClientSecret, a.Config.Google.RedirectURL)
	}

	w.WriteHeader(http.StatusOK)
}

// reloadConfig parses glance.yml from disk and updates in-memory states in real-time.
func (a *Application) reloadConfig() error {
	configFile, err := os.Open(a.ConfigPath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	config, err := NewConfigFromYml(configFile)
	if err != nil {
		return err
	}

	a.configMu.RLock()
	oldPages := a.Config.Pages
	a.configMu.RUnlock()

	// Match and copy cached state from old configuration to the new one
	for i := range config.Pages {
		newPage := &config.Pages[i]
		
		// Find matching old page
		var oldPage *Page
		for i := range oldPages {
			op := &oldPages[i]
			if op.Title == newPage.Title {
				oldPage = op
				break
			}
		}
		
		if oldPage != nil {
			oldWidgets := oldPage.GetFlatWidgets()
			newWidgets := newPage.GetFlatWidgets()
			
			// Track matched old widgets to prevent duplicate matching
			matched := make(map[widget.Widget]bool)
			
			for _, nw := range newWidgets {
				// Marshal new widget config to YAML to get a normalized config string
				nwYaml, err := yaml.Marshal(nw)
				if err != nil {
					continue
				}
				
				// Search for a matching old widget that hasn't been matched yet
				for _, ow := range oldWidgets {
					if matched[ow] {
						continue
					}
					
					owYaml, err := yaml.Marshal(ow)
					if err != nil {
						continue
					}
					
					if string(nwYaml) == string(owYaml) {
						widget.CopyWidgetState(ow, nw)
						matched[ow] = true
						break
					}
				}
			}
		}
	}

	newSlugToPage := make(map[string]*Page)
	newSlugToPage[""] = &config.Pages[0]

	for i := range config.Pages {
		if config.Pages[i].Slug == "" {
			config.Pages[i].Slug = titleToSlug(config.Pages[i].Title)
		}
		newSlugToPage[config.Pages[i].Slug] = &config.Pages[i]
	}

	a.configMu.Lock()
	a.Config = *config
	a.slugToPage = newSlugToPage
	a.configMu.Unlock()

	widget.GlobalTimezone = config.Server.Timezone

	for i := range config.Pages {
		page := &config.Pages[i]
		page.UpdateOutdatedWidgets()
	}

	return nil
}

func flattenWidgets(widgets []widget.Widget) []widget.Widget {
	var flat []widget.Widget
	for _, w := range widgets {
		if g, ok := w.(*widget.Group); ok {
			flat = append(flat, g)
			flat = append(flat, flattenWidgets(g.Widgets)...)
		} else {
			flat = append(flat, w)
		}
	}
	return flat
}

// GetFlatWidgets flattens and returns all head and column widgets of the page recursively (resolving groups).
func (p *Page) GetFlatWidgets() []widget.Widget {
	var all []widget.Widget
	all = append(all, p.HeadWidgets...)
	for c := range p.Columns {
		all = append(all, p.Columns[c].Widgets...)
	}
	return flattenWidgets(all)
}

// findPageNode locates the yaml MappingNode representing the page matching slug.
func findPageNode(rootNode *yaml.Node, targetSlug string) (*yaml.Node, error) {
	if len(rootNode.Content) == 0 {
		return nil, fmt.Errorf("empty YAML document")
	}
	rootMap := rootNode.Content[0]
	pagesNode := findMapValue(rootMap, "pages")
	if pagesNode == nil || pagesNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("pages block not found or invalid")
	}

	for _, pageNode := range pagesNode.Content {
		nameNode := findMapValue(pageNode, "name")
		slugNode := findMapValue(pageNode, "slug")
		slug := ""
		if slugNode != nil {
			slug = slugNode.Value
		} else if nameNode != nil {
			slug = titleToSlug(nameNode.Value)
		}
		if slug == targetSlug {
			return pageNode, nil
		}
	}
	return nil, fmt.Errorf("page not found: %s", targetSlug)
}

// findMapValue helper finds the value node corresponding to key inside mapping node.
func findMapValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// saveNodeToDisk serializes a yaml.Node AST to path with 2-space indentation.
func saveNodeToDisk(path string, node *yaml.Node) error {
	fixYamlScalarTypes(node)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	defer enc.Close()
	if err := enc.Encode(node); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

var yamlKnownIntKeys = map[string]bool{
	"update-interval":    true,
	"height":             true,
	"limit":              true,
	"collapse-after":     true,
	"thumbnail-height":   true,
	"port":               true,
	"pull-requests-limit": true,
	"issues-limit":        true,
}

// fixYamlScalarTypes walks the YAML AST and fixes scalar value nodes that should be
// integers but got tagged as !!str by yaml.Node.Encode(map[string]interface{}).
// Only known numeric keys are corrected to avoid corrupting string values like API keys.
func fixYamlScalarTypes(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			if valNode.Kind == yaml.ScalarNode && valNode.Tag == "!!str" && yamlKnownIntKeys[keyNode.Value] {
				if _, err := strconv.Atoi(valNode.Value); err == nil {
					valNode.Tag = "!!int"
				}
			}
			fixYamlScalarTypes(valNode)
		}
	} else {
		for _, child := range node.Content {
			fixYamlScalarTypes(child)
		}
	}
}

// ----------------------------------------------------
// Settings and Dashboard/Page Management Models & APIs
// ----------------------------------------------------

type brandingSettingsPayload struct {
	AppName      string `json:"app-name" yaml:"app-name"`
	CustomFooter string `json:"custom-footer" yaml:"custom-footer"`
}

type serverSettingsPayload struct {
	Host       string `json:"host" yaml:"host"`
	Port       uint16 `json:"port" yaml:"port"`
	AssetsPath string `json:"assets-path" yaml:"assets-path"`
	Timezone   string `json:"timezone" yaml:"timezone"`
}

type themeSettingsPayload struct {
	Light                          bool    `json:"light" yaml:"light"`
	BackgroundColor                string  `json:"background-color" yaml:"background-color,omitempty"`
	PrimaryColor                   string  `json:"primary-color" yaml:"primary-color,omitempty"`
	PositiveColor                  string  `json:"positive-color" yaml:"positive-color,omitempty"`
	NegativeColor                  string  `json:"negative-color" yaml:"negative-color,omitempty"`
	ContrastMultiplier             float32 `json:"contrast-multiplier" yaml:"contrast-multiplier,omitempty"`
	TextSaturationMultiplier       float32 `json:"text-saturation-multiplier" yaml:"text-saturation-multiplier,omitempty"`
	CustomCSSFile                  string  `json:"custom-css-file" yaml:"custom-css-file,omitempty"`
	WidgetGap                      string  `json:"widget-gap" yaml:"widget-gap,omitempty"`
	WidgetContentVerticalPadding   string  `json:"widget-content-vertical-padding" yaml:"widget-content-vertical-padding,omitempty"`
	WidgetContentHorizontalPadding string  `json:"widget-content-horizontal-padding" yaml:"widget-content-horizontal-padding,omitempty"`
	BorderRadius                   string  `json:"border-radius" yaml:"border-radius,omitempty"`
}

type spotifySettingsPayload struct {
	ClientID     string `json:"client-id" yaml:"client-id"`
	ClientSecret string `json:"client-secret" yaml:"client-secret"`
	RedirectURL  string `json:"redirect-url" yaml:"redirect-url,omitempty"`
}

type settingsPayload struct {
	Branding brandingSettingsPayload `json:"branding"`
	Server   serverSettingsPayload   `json:"server"`
	Theme    themeSettingsPayload    `json:"theme"`
	Spotify  spotifySettingsPayload  `json:"spotify"`
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "********"
}

// formatHSL formats an HSL Color Field back to a space-separated string (e.g. "240 8 9")
func formatHSL(field *widget.HSLColorField) string {
	if field == nil {
		return ""
	}
	return fmt.Sprintf("%d %d %d", field.Hue, field.Saturation, field.Lightness)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

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

// HandleSettingsSave writes the new configurations back to glance.yml and reloads memory structures.
func (a *Application) HandleSettingsSave(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

	// Limit request body size to prevent OOM
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload settingsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(rootNode.Content) == 0 {
		http.Error(w, "empty YAML document", http.StatusInternalServerError)
		return
	}
	rootMap := rootNode.Content[0]

	// Update top-level YAML keys preserving structure and other blocks (e.g. pages)
	if err := updateTopLevelKey(rootMap, "branding", payload.Branding); err != nil {
		http.Error(w, "failed to update branding settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateTopLevelKey(rootMap, "server", payload.Server); err != nil {
		http.Error(w, "failed to update server settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateTopLevelKey(rootMap, "theme", payload.Theme); err != nil {
		http.Error(w, "failed to update theme settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := updateTopLevelKey(rootMap, "spotify", payload.Spotify); err != nil {
		http.Error(w, "failed to update spotify settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Validate the updated configuration AST in memory before saving to disk.
	if err := validateASTConfig(&rootNode); err != nil {
		http.Error(w, "invalid configuration settings: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := saveNodeToDisk(a.ConfigPath, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.reloadConfig(); err != nil {
		http.Error(w, "saved settings but failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot reload credentials
	InitSpotify(a.Config.Spotify.ClientID, a.Config.Spotify.ClientSecret, a.Config.Spotify.RedirectURL)

	w.WriteHeader(http.StatusOK)
}

// HandlePageAdd appends a new dashboard/page to glance.yml with a default column.
func (a *Application) HandlePageAdd(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

	// Limit request body size to prevent OOM
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		http.Error(w, "page name cannot be empty", http.StatusBadRequest)
		return
	}

	configBytes, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(configBytes, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(rootNode.Content) == 0 {
		http.Error(w, "empty YAML document", http.StatusInternalServerError)
		return
	}
	rootMap := rootNode.Content[0]
	pagesNode := findMapValue(rootMap, "pages")
	if pagesNode == nil || pagesNode.Kind != yaml.SequenceNode {
		http.Error(w, "pages block not found", http.StatusInternalServerError)
		return
	}

	// Create new page object with a single full-width column
	newPageMap := map[string]interface{}{
		"name": payload.Name,
		"columns": []map[string]interface{}{
			{
				"size":    "full",
				"widgets": []interface{}{},
			},
		},
	}

	var newPageDoc yaml.Node
	if err := newPageDoc.Encode(newPageMap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var newPageNode *yaml.Node
	if newPageDoc.Kind == yaml.DocumentNode && len(newPageDoc.Content) > 0 {
		newPageNode = newPageDoc.Content[0]
	} else {
		newPageNode = &newPageDoc
	}

	pagesNode.Content = append(pagesNode.Content, newPageNode)

	// Validate AST config before saving
	if err := validateASTConfig(&rootNode); err != nil {
		http.Error(w, "invalid page addition: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := saveNodeToDisk(a.ConfigPath, &rootNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.reloadConfig(); err != nil {
		http.Error(w, "added page but failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// updateTopLevelKey helper updates a top-level key inside the YAML root MappingNode.
func updateTopLevelKey(rootMap *yaml.Node, key string, data interface{}) error {
	if rootMap.Kind != yaml.MappingNode {
		return fmt.Errorf("root node is not a mapping node")
	}

	var newNode yaml.Node
	if err := newNode.Encode(data); err != nil {
		return err
	}

	var valNode *yaml.Node
	if newNode.Kind == yaml.DocumentNode && len(newNode.Content) > 0 {
		valNode = newNode.Content[0]
	} else {
		valNode = &newNode
	}

	// Search and replace key if exists
	for i := 0; i < len(rootMap.Content); i += 2 {
		if rootMap.Content[i].Value == key {
			rootMap.Content[i+1] = valNode
			return nil
		}
	}

	// Key not found, append key-value pair to root map
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	rootMap.Content = append(rootMap.Content, keyNode, valNode)
	return nil
}

// updateMapValue helper updates a key's value inside a MappingNode.
// If the key does not exist, it appends the key-value pair to the MappingNode.
func updateMapValue(node *yaml.Node, key string, val string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Value = val
			node.Content[i+1].Kind = yaml.ScalarNode
			node.Content[i+1].Tag = "!!str"
			return
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val}
	node.Content = append(node.Content, keyNode, valNode)
}

func (a *Application) HandleConfigImport(w http.ResponseWriter, r *http.Request) {
	a.configFileMu.Lock()
	defer a.configFileMu.Unlock()

	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)

	err := r.ParseMultipartForm(2 * 1024 * 1024)
	if err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing or invalid file upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".yml") &&
		!strings.HasSuffix(strings.ToLower(header.Filename), ".yaml") {
		http.Error(w, "only .yml or .yaml files are accepted", http.StatusBadRequest)
		return
	}

	contentBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}

	var testConfig Config
	if err := yaml.Unmarshal(contentBytes, &testConfig); err != nil {
		http.Error(w, "invalid YAML syntax: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate using existing validation without full widget initialization
	if err := configIsValid(&testConfig); err != nil {
		http.Error(w, "config validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Write the raw YAML bytes directly to disk, preserving all formatting/comments
	if err := os.WriteFile(a.ConfigPath, contentBytes, 0644); err != nil {
		http.Error(w, "failed to write config file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := a.reloadConfig(); err != nil {
		http.Error(w, "config written but reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	InitSpotify(a.Config.Spotify.ClientID, a.Config.Spotify.ClientSecret, a.Config.Spotify.RedirectURL)

	slog.Info("Config imported successfully", "filename", header.Filename)
	w.WriteHeader(http.StatusOK)
}

// validateASTConfig checks if the modified AST tree would produce a valid config structure.
func validateASTConfig(rootNode *yaml.Node) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(rootNode); err != nil {
		return fmt.Errorf("failed to encode YAML AST: %w", err)
	}

	config := NewConfig()
	if err := yaml.Unmarshal(buf.Bytes(), config); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	if err := configIsValid(config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}

