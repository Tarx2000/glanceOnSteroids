package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

type Hue struct {
	widgetBase       `yaml:",inline"`
	Rooms            []string      `yaml:"rooms"`
	Lights           []string      `yaml:"lights"`
	Scenes           []string      `yaml:"scenes"`
	Authorized       bool          `yaml:"-"`
	ResourceStatuses []HueResource `yaml:"-"`
}

func init() {
	Register("hue", func() Widget { return &Hue{} })
}

func (widget *Hue) Initialize() error {
	widget.withTitle("Philips Hue").withCacheDuration(1 * time.Minute)
	return nil
}

func (widget *Hue) Update(ctx context.Context, services ExternalServiceProvider) {
	authorized := false
	if services != nil {
		authorized = services.HueAuthorized()
	}

	widget.Lock()
	widget.Authorized = authorized
	widget.Unlock()

	if !authorized {
		widget.Lock()
		widget.ResourceStatuses = nil
		widget.Unlock()
		widget.withError(nil).scheduleNextUpdate()
		return
	}

	if services != nil {
		statuses, err := services.FetchHueStatuses(ctx, widget.Rooms, widget.Lights, widget.Scenes)
		if err != nil {
			widget.withError(err).scheduleEarlyUpdate()
			return
		}

		widget.Lock()
		widget.ResourceStatuses = statuses
		widget.Unlock()

		widget.withError(nil).scheduleNextUpdate()
	}
}

func (widget *Hue) Render() template.HTML {
	return widget.render(widget, assets.HueTemplate)
}
