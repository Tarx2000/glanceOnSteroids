package widget

import (
	"context"
	"html/template"

	"github.com/glanceapp/glance/internal/assets"
)

// SpotifyAuthorizedCheck callback returns true if Spotify is authorized.
// Wired dynamically from the main server code to bypass circular dependency.
var SpotifyAuthorizedCheck func() bool

// Spotify represents the Spotify player widget on the dashboard.
type Spotify struct {
	widgetBase `yaml:",inline"`
}

// Initialize configures the widget's title and update behavior.
func (widget *Spotify) Initialize() error {
	widget.withTitle("Spotify").withCacheDuration(-1) // Managed in real-time by WS
	return nil
}

// Update prepares the widget data (nothing to fetch synchronously).
func (widget *Spotify) Update(ctx context.Context) {
	widget.ContentAvailable = true
}

// Render compiles the HTML of the Spotify widget.
func (widget *Spotify) Render() template.HTML {
	return widget.render(widget, assets.SpotifyTemplate)
}

// IsAuthorized returns true if the Spotify account is connected/authorized.
func (widget *Spotify) IsAuthorized() bool {
	if SpotifyAuthorizedCheck != nil {
		return SpotifyAuthorizedCheck()
	}
	return false
}
