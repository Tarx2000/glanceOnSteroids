package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type Releases struct {
	widgetBase      `yaml:",inline"`
	Releases        feed.AppReleases  `yaml:"-"`
	Repositories    []string          `yaml:"repositories"`
	Token           OptionalEnvString `yaml:"token"`
	GitlabToken     OptionalEnvString `yaml:"gitlab-token"`
	ShowSourceIcon  bool              `yaml:"show-source-icon"`
	Limit           int               `yaml:"limit"`
	CollapseAfter   int               `yaml:"collapse-after"`
}

func init() {
	Register("releases", func() Widget { return &Releases{} })
}

func (widget *Releases) Initialize() error {
	widget.withTitle("Releases").withCacheDuration(2 * time.Hour)

	if widget.Limit <= 0 {
		widget.Limit = 10
	}

	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	return nil
}

func (widget *Releases) Update(ctx context.Context, services ExternalServiceProvider) {
	releases, err := feed.FetchLatestReleasesFromGithub(widget.Repositories, string(widget.Token))

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if len(releases) > widget.Limit {
		releases = releases[:widget.Limit]
	}

	widget.Lock()
	widget.Releases = releases
	widget.Unlock()
}

func (widget *Releases) Render() template.HTML {
	return widget.render(widget, assets.ReleasesTemplate)
}
