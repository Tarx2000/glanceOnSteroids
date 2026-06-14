package widget

import (
	"context"
	"html/template"

	"github.com/glanceapp/glance/internal/assets"
)

func init() {
	Register("spotify", func() Widget { return &Spotify{} })
}

// Spotify represents the Spotify player widget on the dashboard.
type Spotify struct {
	widgetBase `yaml:",inline"`
	Authorized bool `yaml:"-"`
}

// Initialize configures the widget's title and update behavior.
func (widget *Spotify) Initialize() error {
	widget.withTitle("Spotify").withCacheDuration(-1) // Managed in real-time by WS
	return nil
}

// Update prepares the widget data (nothing to fetch synchronously).
func (widget *Spotify) Update(ctx context.Context, services ExternalServiceProvider) {
	widget.Lock()
	widget.Authorized = services.SpotifyAuthorized()
	widget.ContentAvailable = true
	widget.Unlock()
}

// Render compiles the HTML of the Spotify widget.
func (widget *Spotify) Render() template.HTML {
	return widget.render(widget, assets.SpotifyTemplate)
}

// IsAuthorized returns true if the Spotify account is connected/authorized.
func (widget *Spotify) IsAuthorized() bool {
	return widget.Authorized
}
