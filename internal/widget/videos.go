package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type Videos struct {
	widgetBase       `yaml:",inline"`
	Videos           feed.Videos `yaml:"-"`
	VideoUrlTemplate string      `yaml:"video-url-template"`
	Style            string      `yaml:"style"`
	Channels         []string    `yaml:"channels"`
	Playlists        []string    `yaml:"playlists"`
	Limit            int         `yaml:"limit"`
	CollapseAfter    int         `yaml:"collapse-after"`
	CollapseAfterRows int        `yaml:"collapse-after-rows"`
	IncludeShorts    bool        `yaml:"include-shorts"`
}

func init() {
	Register("videos", func() Widget { return &Videos{} })
}

func (widget *Videos) Initialize() error {
	widget.withTitle("Videos").withCacheDuration(time.Hour)

	if widget.Limit <= 0 {
		widget.Limit = 25
	}

	return nil
}

func (widget *Videos) Update(ctx context.Context, services ExternalServiceProvider) {
	videos, err := feed.FetchYoutubeChannelUploads(ctx, widget.Channels, widget.VideoUrlTemplate)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if len(videos) > widget.Limit {
		videos = videos[:widget.Limit]
	}

	widget.Lock()
	widget.Videos = videos
	widget.Unlock()
}

func (widget *Videos) Render() template.HTML {
	if widget.Style == "grid-cards" {
		return widget.render(widget, assets.VideosGridTemplate)
	}

	return widget.render(widget, assets.VideosTemplate)
}
