package widget

import (
	"context"
	"html/template"

	"github.com/glanceapp/glance/internal/assets"
)

// Clock renders a local time and date display updated client-side via JavaScript.
type Clock struct {
	widgetBase `yaml:",inline"`
	HourFormat string `yaml:"hour-format"`
}

func (widget *Clock) Initialize() error {
	widget.withTitle("Clock").withCacheDuration(-1) // Updates client-side
	if widget.HourFormat == "" {
		widget.HourFormat = "24h"
	}
	return nil
}

func (widget *Clock) Update(ctx context.Context) {
	// No-op (handled client-side)
}

func (widget *Clock) Render() template.HTML {
	return widget.render(widget, assets.ClockTemplate)
}
