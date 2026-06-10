package widget

import (
	"bytes"
	"context"
	"html/template"
	"sync"
	"time"
)

// Group holds a list of nested widgets and forwards updates and renders sequentially or in parallel.
type Group struct {
	widgetBase `yaml:",inline"`
	Widgets    Widgets `yaml:"widgets"`
}

func (g *Group) Initialize() error {
	// Group does not have its own cache duration and is driven by nested widget update checks
	g.withTitle("Group").withCacheDuration(-1)
	for i := range g.Widgets {
		if err := g.Widgets[i].Initialize(); err != nil {
			return err
		}
	}
	return nil
}

func (g *Group) Update(ctx context.Context) {
	now := time.Now()
	var wg sync.WaitGroup
	for i := range g.Widgets {
		w := g.Widgets[i]
		if w.RequiresUpdate(&now) {
			wg.Add(1)
			go func(wd Widget) {
				defer wg.Done()
				wd.Update(ctx)
			}(w)
		}
	}
	wg.Wait()
}

func (g *Group) RequiresUpdate(now *time.Time) bool {
	for i := range g.Widgets {
		if g.Widgets[i].RequiresUpdate(now) {
			return true
		}
	}
	return false
}

func (g *Group) Render() template.HTML {
	var buf bytes.Buffer
	// Wrap the group widget in a standard widget container to act as a single draggable tile.
	// We also render a group-only-header that will be visible only in Edit Layout Mode.
	buf.WriteString(`<div class="widget widget-type-group">`)
	buf.WriteString(`<div class="widget-header group-only-header">`)
	buf.WriteString(`<div class="uppercase">Group</div>`)
	buf.WriteString(`</div>`)
	buf.WriteString(`<div class="widget-group">`)
	for i := range g.Widgets {
		buf.WriteString(string(g.Widgets[i].Render()))
	}
	buf.WriteString(`</div>`)
	buf.WriteString(`</div>`)
	return template.HTML(buf.String())
}
