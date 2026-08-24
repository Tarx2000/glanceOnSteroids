package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type HermesApprove struct {
	widgetBase   `yaml:",inline"`
	ApiUrl       OptionalEnvString    `yaml:"api-url"`
	ApiKey       OptionalEnvString    `yaml:"api-key"`
	Limit        int                  `yaml:"limit"`
	Requests     []feed.HermesRequest `yaml:"-"`
	PendingCount int                  `yaml:"-"`
}

func init() {
	Register("hermes-approve", func() Widget { return &HermesApprove{} })
}

func (widget *HermesApprove) Initialize() error {
	widget.withTitle("Hermes Approvals").withCacheDuration(5 * time.Second)

	if widget.ApiUrl == "" {
		widget.ApiUrl = "http://localhost:3000"
	}
	if widget.Limit <= 0 {
		widget.Limit = 10
	}

	return nil
}

func (widget *HermesApprove) RemoveRequest(id string) {
	widget.Lock()
	defer widget.Unlock()

	filtered := make([]feed.HermesRequest, 0, len(widget.Requests))
	for _, req := range widget.Requests {
		if req.ID != id {
			filtered = append(filtered, req)
		}
	}
	widget.Requests = filtered
	widget.PendingCount = len(filtered)
}

func (widget *HermesApprove) Update(ctx context.Context, services ExternalServiceProvider) {
	apiURL := string(widget.ApiUrl)
	apiKey := string(widget.ApiKey)
	limit := widget.Limit

	res, err := feed.FetchHermesPendingRequests(ctx, apiURL, apiKey, limit)
	if err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}

	widget.Lock()
	widget.Requests = res.Requests
	widget.PendingCount = res.Pending
	widget.Unlock()

	widget.withError(nil).scheduleNextUpdate()
}

func (widget *HermesApprove) Render() template.HTML {
	return widget.render(widget, assets.HermesApproveTemplate)
}
