package widget

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// CustomAPI handles loading data from external APIs and rendering dynamic content from the config template.
type CustomAPI struct {
	widgetBase  `yaml:",inline"`
	Template    string             `yaml:"template"`
	CachedHTML  string             `yaml:"-"`
	parsedTempl *template.Template `yaml:"-"`
}

func init() {
	Register("custom-api", func() Widget { return &CustomAPI{} })
}

func (widget *CustomAPI) Initialize() error {
	widget.withTitle("Custom API").withCacheDuration(time.Minute)

	// Precompile template for performance and syntax checking
	tmpl, err := template.New("custom-api-user").
		Funcs(assets.GlobalTemplateFunctions()).
		Parse(widget.Template)
	if err != nil {
		return fmt.Errorf("failed compiling custom-api template: %w", err)
	}
	widget.parsedTempl = tmpl
	return nil
}

func (widget *CustomAPI) Update(ctx context.Context, services ExternalServiceProvider) {
	if widget.parsedTempl == nil {
		widget.withError(fmt.Errorf("template not compiled"))
		return
	}

	var buf bytes.Buffer
	// Execute template inside Update phase and cache the HTML to prevent rate-limiting and performance bottlenecks.
	err := widget.parsedTempl.Execute(&buf, widget)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	widget.Lock()
	widget.CachedHTML = buf.String()
	widget.Unlock()
	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *CustomAPI) RenderHTML() template.HTML {
	widget.Lock()
	defer widget.Unlock()
	return template.HTML(widget.CachedHTML)
}

func (widget *CustomAPI) Render() template.HTML {
	return widget.render(widget, assets.CustomAPITemplate)
}
