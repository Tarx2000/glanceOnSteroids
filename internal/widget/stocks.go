package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

// TODO: rename to Markets at some point
type Stocks struct {
	widgetBase `yaml:",inline"`
	Stocks     feed.Stocks `yaml:"stocks"`
	Markets    feed.Stocks `yaml:"markets"`
	Sort       string      `yaml:"sort-by"`
	Style      string      `yaml:"style"`
}

func init() {
	Register("stocks", func() Widget { return &Stocks{} })
	Register("markets", func() Widget { return &Stocks{} })
}

func (widget *Stocks) Initialize() error {
	widget.withTitle("Markets").withCacheDuration(time.Hour)

	if len(widget.Markets) > 0 {
		widget.Stocks = widget.Markets
	}

	return nil
}

func (widget *Stocks) Update(ctx context.Context, services ExternalServiceProvider) {
	stocks, err := feed.FetchStocksDataFromYahoo(ctx, widget.Stocks)

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if widget.Sort == "absolute-change" {
		stocks.SortByAbsChange()
	}

	widget.Lock()
	widget.Stocks = stocks
	widget.Unlock()
}

func (widget *Stocks) Render() template.HTML {
	return widget.render(widget, assets.StocksTemplate)
}
