package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

// BackgroundWidgetUpdateInterval is the duration between automatic background widget updates
// when at least one client is connected via WebSocket.
var BackgroundWidgetUpdateInterval = 30 * time.Second

var sequentialWhitespacePattern = regexp.MustCompile(`\s+`)
var sequentialHyphenPattern = regexp.MustCompile(`-+`)
var invalidSlugCharsPattern = regexp.MustCompile(`[^a-z0-9\-]`)

// Application represents the core glance application server and state.
type Application struct {
	Version       string
	BuildNumber   string
	Config        Config
	ConfigPath    string
	slugToPage    map[string]*Page
	configMu      sync.RWMutex
	configFileMu  sync.Mutex
	ConfigManager *ConfigManager
	Hub           *Hub
	SpotifyPoller *SpotifyPoller
	ctxCancel     context.CancelFunc
}

// Theme defines application-wide styling properties.
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

// Server defines host/port settings.
type Server struct {
	Host       string    `yaml:"host"`
	Port       uint16    `yaml:"port"`
	AssetsPath string    `yaml:"assets-path"`
	Timezone   string    `yaml:"timezone"`
	StartedAt  time.Time `yaml:"-"`
}

// Column groups widgets horizontally on a page.
type Column struct {
	Size    string         `yaml:"size"`
	Widgets widget.Widgets `yaml:"widgets"`
}

type templateData struct {
	App  *Application
	Page *Page
}

// Page represents a dashboard page/tab containing columns and widgets.
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

// UpdateOutdatedWidgets processes updates for outdated widgets concurrently.
func (p *Page) UpdateOutdatedWidgets(hub *Hub) bool {
	p.mu.Lock()
	if p.isUpdating {
		p.mu.Unlock()
		return false
	}
	p.isUpdating = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.isUpdating = false
		p.mu.Unlock()
	}()

	now := time.Now()

	var wg sync.WaitGroup

	// Semaphore to bound concurrency and prevent resource exhaustion
	const maxConcurrentUpdates = 5
	var sem = make(chan struct{}, maxConcurrentUpdates)
	
	var mu sync.Mutex
	anyUpdated := false

	// Helper to queue widget update
	queueUpdate := func(wd widget.Widget, col string, idx int, nestedIdx int) {
		if !wd.RequiresUpdate(&now) {
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			
			// Acquire semaphore slot
			sem <- struct{}{}
			defer func() { <-sem }()
			
			// Create a per-widget context with a timeout of 15 seconds.
			widgetCtx, widgetCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer widgetCancel()
			
			// Run update in a separate goroutine to prevent deadlocks if the update hangs
			done := make(chan struct{})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Widget update panicked", "type", wd.GetType(), "col", col, "idx", idx, "panic", r)
					}
					close(done)
				}()
				wd.Update(widgetCtx, &glanceServiceProvider{})
			}()
			
			select {
			case <-done:
				// Update completed within timeout
			case <-widgetCtx.Done():
				slog.Warn("Widget update timed out or hung during execution", "type", wd.GetType(), "col", col, "idx", idx)
			}
			
			// Broadcast update for this specific widget immediately!
			hub.BroadcastMessage("widget_update", map[string]interface{}{
				"page":       p.Slug,
				"col":        col,
				"idx":        idx,
				"nested_idx": nestedIdx,
			})
			
			mu.Lock()
			anyUpdated = true
			mu.Unlock()
		}()
	}

	// Update columns widgets
	for c := range p.Columns {
		for w := range p.Columns[c].Widgets {
			wItem := p.Columns[c].Widgets[w]
			colStr := strconv.Itoa(c)
			if group, ok := wItem.(*widget.Group); ok {
				for nwIdx, nw := range group.Widgets {
					queueUpdate(nw, colStr, w, nwIdx)
				}
			} else {
				queueUpdate(wItem, colStr, w, -1)
			}
		}
	}

	// Update head widgets
	for w := range p.HeadWidgets {
		wItem := p.HeadWidgets[w]
		if group, ok := wItem.(*widget.Group); ok {
			for nwIdx, nw := range group.Widgets {
				queueUpdate(nw, "head", w, nwIdx)
			}
		} else {
			queueUpdate(wItem, "head", w, -1)
		}
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

// NewApplication instantiates the Application and binds ConfigManager.
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

	app.Hub = NewHub()
	app.SpotifyPoller = NewSpotifyPoller(config.Spotify, app.Hub.ActiveConnections, app.Hub.BroadcastMessage)

	app.ConfigManager = NewConfigManager(configPath, &app.configFileMu, app.reloadConfig)

	app.slugToPage[""] = &config.Pages[0]

	for i := range config.Pages {
		if config.Pages[i].Slug == "" {
			config.Pages[i].Slug = titleToSlug(config.Pages[i].Title)
		}

		app.slugToPage[config.Pages[i].Slug] = &config.Pages[i]
	}

	return app, nil
}

// FileServerWithCache serves static files with an explicit HTTP Cache-Control header.
func FileServerWithCache(fs http.FileSystem, cacheDuration time.Duration) http.Handler {
	server := http.FileServer(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cacheDuration.Seconds())))
		server.ServeHTTP(w, r)
	})
}

// Serve initializes HTTP endpoints and routes.
func (a *Application) Serve() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", a.HandlePageRequest)
	mux.HandleFunc("GET /{page}", a.HandlePageRequest)
	mux.HandleFunc("GET /api/pages/{page}/content/{$}", a.HandlePageContentRequest)
	mux.Handle("GET /static/{path...}", http.StripPrefix("/static/", FileServerWithCache(http.FS(assets.PublicFS), 2*time.Hour)))

	// Register WebSocket endpoint
	mux.HandleFunc("GET /api/ws", a.handleWebSocket)

	// Spotify Auth routes
	mux.HandleFunc("GET /api/spotify/login", a.HandleSpotifyLogin)
	mux.HandleFunc("GET /api/spotify/callback", a.HandleSpotifyCallback)

	// Google Calendar Auth routes
	mux.HandleFunc("GET /api/google/login", a.HandleGoogleLogin)
	mux.HandleFunc("GET /api/google/callback", a.HandleGoogleCallback)
	mux.HandleFunc("GET /api/google/calendars", a.HandleGoogleCalendarsGet)

	// MVV München routes
	mux.HandleFunc("GET /api/mvv/search", a.HandleMvvSearch)

	// Philips Hue Remote routes
	mux.HandleFunc("GET /api/hue/login", a.HandleHueLogin)
	mux.HandleFunc("GET /api/hue/callback", a.HandleHueCallback)
	mux.HandleFunc("GET /api/hue", a.HandleHueCallback)
	mux.HandleFunc("GET /api/hue/", a.HandleHueCallback)
	mux.HandleFunc("GET /api/hue/resources", a.HandleHueResourcesGet)
	mux.HandleFunc("POST /api/hue/control", a.HandleHueControl)

	// Spotify playback actions
	mux.HandleFunc("POST /api/spotify/play", a.HandleSpotifyPlay)
	mux.HandleFunc("POST /api/spotify/pause", a.HandleSpotifyPause)
	mux.HandleFunc("POST /api/spotify/skip", a.HandleSpotifySkip)
	mux.HandleFunc("POST /api/spotify/volume", a.HandleSpotifyVolume)

	// Layout and widget configuration API endpoints
	mux.HandleFunc("POST /api/layout/save", a.HandleLayoutSave)
	mux.HandleFunc("POST /api/layout/batch-save", a.HandleLayoutBatchSave)
	mux.HandleFunc("POST /api/widgets/add", a.HandleWidgetAdd)
	mux.HandleFunc("POST /api/widgets/delete", a.HandleWidgetDelete)
	mux.HandleFunc("GET /api/widgets/get", a.HandleWidgetGet)
	mux.HandleFunc("GET /api/widgets/render", a.HandleWidgetRender)
	mux.HandleFunc("POST /api/widgets/update", a.HandleWidgetUpdate)

	// Settings & Page API endpoints
	mux.HandleFunc("GET /api/settings", a.HandleSettingsGet)
	mux.HandleFunc("POST /api/settings/save", a.HandleSettingsSave)
	mux.HandleFunc("POST /api/pages/add", a.HandlePageAdd)
	mux.HandleFunc("POST /api/pages/delete", a.HandlePageDelete)

	mux.HandleFunc("POST /api/config/import", a.HandleConfigImport)
	mux.HandleFunc("GET /api/config/export", a.HandleConfigExport)
	mux.HandleFunc("POST /api/config/preview", a.HandleConfigPreview)

	if a.Config.Server.AssetsPath != "" {
		absAssetsPath, err := filepath.Abs(a.Config.Server.AssetsPath)

		if err != nil {
			return fmt.Errorf("invalid assets path: %s", a.Config.Server.AssetsPath)
		}

		slog.Info("Serving assets", "path", absAssetsPath)
		assetsFS := FileServerWithCache(http.Dir(a.Config.Server.AssetsPath), 2*time.Hour)
		mux.Handle("/assets/{path...}", http.StripPrefix("/assets/", assetsFS))
	}

	var handler http.Handler = mux
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.Info("Request started", "method", r.Method, "url", r.URL.String(), "remote", r.RemoteAddr)
		mux.ServeHTTP(w, r)
		slog.Info("Request finished", "method", r.Method, "url", r.URL.String(), "duration", time.Since(start))
	})

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.Config.Server.Host, a.Config.Server.Port),
		Handler: handler,
	}

	slog.Info("Starting server", "host", a.Config.Server.Host, "port", a.Config.Server.Port)
	
	// Create context for background services
	ctx, cancel := context.WithCancel(context.Background())
	a.ctxCancel = cancel

	// Start WebSocket Hub loop
	go a.Hub.Run()

	// Start Spotify Poller loop
	if a.SpotifyPoller != nil {
		go a.SpotifyPoller.Run(ctx)
	}

	// Pre-warm caches for all pages in the background upon startup
	go func() {
		for i := range a.Config.Pages {
			page := &a.Config.Pages[i]
			slog.Info("Pre-warming widget cache for page", "page", page.Title)
			if updated := page.UpdateOutdatedWidgets(a.Hub); !updated {
				slog.Info("Pre-warm completed for page (no updates needed)", "page", page.Title)
			} else {
				slog.Info("Pre-warm completed for page", "page", page.Title)
			}
		}
		slog.Info("Pre-warming widget cache completed")
	}()

	// Periodic background updates to keep widget cache fresh
	go func() {
		ticker := time.NewTicker(BackgroundWidgetUpdateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				activePages := a.Hub.ActivePages()
				if len(activePages) == 0 {
					continue // Nobody is online, skip background updates entirely
				}

				a.configMu.RLock()
				pages := make([]*Page, len(a.Config.Pages))
				for i := range a.Config.Pages {
					pages[i] = &a.Config.Pages[i]
				}
				a.configMu.RUnlock()

				for _, page := range pages {
					if activePages[page.Slug] {
						page.UpdateOutdatedWidgets(a.Hub)
					}
				}
			}
		}
	}()

	return server.ListenAndServe()
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

	// Match and copy cached state from old configuration to the new one.
	// Pages are matched first by slug, then by title, so renaming a page
	// still preserves widget state when the slug stays the same.
	for i := range config.Pages {
		newPage := &config.Pages[i]
		newSlug := newPage.Slug
		if newSlug == "" {
			newSlug = titleToSlug(newPage.Title)
		}

		var oldPage *Page
		for i := range oldPages {
			op := &oldPages[i]
			oldSlug := op.Slug
			if oldSlug == "" {
				oldSlug = titleToSlug(op.Title)
			}
			if oldSlug != "" && oldSlug == newSlug {
				oldPage = op
				break
			}
		}
		// Fallback to title match if no slug match found.
		if oldPage == nil {
			for i := range oldPages {
				op := &oldPages[i]
				if op.Title == newPage.Title {
					oldPage = op
					break
				}
			}
		}

		if oldPage != nil {
			oldWidgets := oldPage.GetFlatWidgets()
			newWidgets := newPage.GetFlatWidgets()

			// Track matched old widgets to prevent duplicate matching.
			matched := make(map[widget.Widget]bool)

			for _, nw := range newWidgets {
				// Marshal new widget config to YAML to get a normalized config string.
				nwYaml, err := yaml.Marshal(nw)
				if err != nil {
					continue
				}

				// Search for a matching old widget that hasn't been matched yet.
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
	// Update SpotifyPoller config
	if a.SpotifyPoller != nil {
		a.SpotifyPoller.mu.Lock()
		a.SpotifyPoller.config = config.Spotify
		a.SpotifyPoller.mu.Unlock()
	}
	a.configMu.Unlock()

	InitGoogle(config.Google.ClientID, config.Google.ClientSecret, config.Google.RedirectURL)
	InitHue(config.Hue.ClientID, config.Hue.ClientSecret, config.Hue.RedirectURL)

	widget.GlobalTimezone = config.Server.Timezone

	for i := range config.Pages {
		page := &config.Pages[i]
		page.UpdateOutdatedWidgets(a.Hub)
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

// JSONStringOrInt represents a string that can be unmarshaled from either a JSON string or a JSON number.
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

type hueSettingsPayload struct {
	ClientID     string `json:"client-id" yaml:"client-id"`
	ClientSecret string `json:"client-secret" yaml:"client-secret"`
	RedirectURL  string `json:"redirect-url" yaml:"redirect-url,omitempty"`
}

type googleSettingsPayload struct {
	ClientID     string `json:"client-id" yaml:"client-id"`
	ClientSecret string `json:"client-secret" yaml:"client-secret"`
	RedirectURL  string `json:"redirect-url" yaml:"redirect-url,omitempty"`
}

type settingsPayload struct {
	Branding brandingSettingsPayload `json:"branding"`
	Server   serverSettingsPayload   `json:"server"`
	Theme    themeSettingsPayload    `json:"theme"`
	Spotify  spotifySettingsPayload  `json:"spotify"`
	Google   googleSettingsPayload   `json:"google"`
	Hue      hueSettingsPayload      `json:"hue"`
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
