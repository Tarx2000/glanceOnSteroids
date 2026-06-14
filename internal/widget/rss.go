package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

// RSS defines the configuration structure and runtime state for the RSS feed widget.
type RSS struct {
	widgetBase       `yaml:",inline"`
	FeedRequests     []feed.RSSFeedRequest `yaml:"feeds"`
	Style            string                `yaml:"style"`
	ThumbnailHeight  float64               `yaml:"thumbnail-height"`
	CardHeight       float64               `yaml:"card-height"`
	Items            feed.RSSFeedItems     `yaml:"-"`
	Limit            int                   `yaml:"limit"`
	CollapseAfter    int                   `yaml:"collapse-after"`
	// PreserveOrder maintains the order of feeds as defined in the configuration instead of sorting by newest.
	PreserveOrder    bool                  `yaml:"preserve-order"`
	// SingleLineTitles truncates feed post titles to a single line in vertical list style.
	SingleLineTitles bool                  `yaml:"single-line-titles"`
}

func init() {
	Register("rss", func() Widget { return &RSS{} })
}

// Initialize sets up default values for limit, collapse-after, and height parameters.
func (widget *RSS) Initialize() error {
	widget.withTitle("RSS Feed").withCacheDuration(1 * time.Hour)

	if widget.Limit <= 0 {
		widget.Limit = 25
	}

	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	if widget.ThumbnailHeight < 0 {
		widget.ThumbnailHeight = 0
	}

	if widget.CardHeight < 0 {
		widget.CardHeight = 0
	}

	return nil
}

// Update fetches the latest items from all registered RSS feeds.
func (widget *RSS) Update(ctx context.Context, services ExternalServiceProvider) {
	// Fetch feed items, passing the PreserveOrder setting to determine sorting behavior.
	items, err := feed.GetItemsFromRSSFeeds(widget.FeedRequests, widget.PreserveOrder)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if len(items) > widget.Limit {
		items = items[:widget.Limit]
	}

	widget.Lock()
	widget.Items = items
	widget.Unlock()
}

// Render returns the compiled HTML representation of the RSS feed widget.
func (widget *RSS) Render() template.HTML {
	if widget.Style == "horizontal-cards" {
		return widget.render(widget, assets.RSSHorizontalCardsTemplate)
	}

	if widget.Style == "horizontal-cards-2" {
		return widget.render(widget, assets.RSSHorizontalCards2Template)
	}

	if widget.Style == "detailed-list" {
		return widget.render(widget, assets.RSSDetailedListTemplate)
	}

	return widget.render(widget, assets.RSSListTemplate)
}
