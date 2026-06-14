package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

type Gmail struct {
	widgetBase  `yaml:",inline"`
	UnreadCount int          `yaml:"-"`
	Emails      []GmailEmail `yaml:"-"`
	Authorized  bool         `yaml:"-"`
}

func init() {
	Register("gmail", func() Widget { return &Gmail{} })
}

func (widget *Gmail) Initialize() error {
	widget.withTitle("Gmail").withCacheDuration(10 * time.Minute)
	return nil
}

func (widget *Gmail) Update(ctx context.Context, services ExternalServiceProvider) {
	authorized := false
	if services != nil {
		authorized = services.GoogleAuthorized()
	}

	widget.Lock()
	widget.Authorized = authorized
	widget.Unlock()

	if !authorized {
		widget.Lock()
		widget.UnreadCount = 0
		widget.Emails = nil
		widget.Unlock()
		widget.withError(nil).scheduleNextUpdate()
		return
	}

	if services != nil {
		count, emails, err := services.FetchGmailUnreadCount(ctx)
		if err != nil {
			widget.withError(err).scheduleEarlyUpdate()
			return
		}

		widget.Lock()
		widget.UnreadCount = count
		widget.Emails = emails
		widget.Unlock()

		widget.withError(nil).scheduleNextUpdate()
	}
}

func (widget *Gmail) Render() template.HTML {
	return widget.render(widget, assets.GmailTemplate)
}
