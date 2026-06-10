package widget

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/glanceapp/glance/internal/feed"

	"gopkg.in/yaml.v3"
)

func New(widgetType string) (Widget, error) {
	switch widgetType {
	case "calendar":
		return &Calendar{}, nil
	case "weather":
		return &Weather{}, nil
	case "clock":
		return &Clock{}, nil
	case "bookmarks":
		return &Bookmarks{}, nil
	case "iframe":
		return &IFrame{}, nil
	case "hacker-news":
		return &HackerNews{}, nil
	case "releases":
		return &Releases{}, nil
	case "videos":
		return &Videos{}, nil
	case "stocks", "markets":
		return &Stocks{}, nil
	case "custom-api":
		return &CustomAPI{}, nil
	case "group":
		return &Group{}, nil
	case "reddit":
		return &Reddit{}, nil
	case "rss":
		return &RSS{}, nil
	case "monitor":
		return &Monitor{}, nil
	case "twitch-top-games":
		return &TwitchGames{}, nil
	case "twitch-channels":
		return &TwitchChannels{}, nil
	case "repository":
		return &Repository{}, nil
	case "spotify":
		return &Spotify{}, nil
	case "neuralwatt":
		return &NeuralWatt{}, nil
	default:
		return nil, fmt.Errorf("unknown widget type: %s", widgetType)
	}
}

type Widgets []Widget

func (w *Widgets) UnmarshalYAML(node *yaml.Node) error {
	var nodes []yaml.Node

	if err := node.Decode(&nodes); err != nil {
		return err
	}

	for _, node := range nodes {
		meta := struct {
			Type string `yaml:"type"`
		}{}

		if err := node.Decode(&meta); err != nil {
			return err
		}

		widget, err := New(meta.Type)

		if err != nil {
			return err
		}

		if err = node.Decode(widget); err != nil {
			return err
		}

		if err = widget.Initialize(); err != nil {
			return err
		}

		*w = append(*w, widget)
	}

	return nil
}

type Widget interface {
	Initialize() error
	RequiresUpdate(*time.Time) bool
	Update(context.Context)
	Render() template.HTML
	GetType() string
}

type cacheType int

const (
	cacheTypeInfinite cacheType = iota
	cacheTypeDuration
	cacheTypeOnTheHour
	cacheTypeDaily
)

// BoolField is a bool that accepts string representations during YAML unmarshal.
type BoolField bool

func (b *BoolField) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		switch strings.ToLower(node.Value) {
		case "true", "yes", "on", "1":
			*b = true
			return nil
		case "false", "no", "off", "0", "":
			*b = false
			return nil
		}
	}
	type plain BoolField
	return node.Decode((*plain)(b))
}

type widgetBase struct {
	Type                string        `yaml:"type"`
	Title               string        `yaml:"title"`
	HideTitle           BoolField     `yaml:"hide-title"`
	CustomCacheDuration DurationField `yaml:"cache"`
	ContentAvailable    bool          `yaml:"-"`
	Error               error         `yaml:"-"`
	Notice              error         `yaml:"-"`
	TemplateBuffer      bytes.Buffer  `yaml:"-"`
	CacheDuration       time.Duration `yaml:"-"`
	CacheType           cacheType     `yaml:"-"`
	NextUpdate          time.Time     `yaml:"-"`
	UpdateRetriedTimes  int           `yaml:"-"`
	mu                  sync.Mutex    `yaml:"-"`
}

func (w *widgetBase) Lock() {
	w.mu.Lock()
}

func (w *widgetBase) Unlock() {
	w.mu.Unlock()
}

func (w *widgetBase) RequiresUpdate(now *time.Time) bool {
	if w.CacheType == cacheTypeInfinite {
		return false
	}

	if w.NextUpdate.IsZero() {
		return true
	}

	return now.After(w.NextUpdate)
}

func (w *widgetBase) Update(ctx context.Context) {

}

func (w *widgetBase) GetType() string {
	return w.Type
}

func (w *widgetBase) render(data any, t *template.Template) template.HTML {
	w.Lock()
	defer w.Unlock()

	w.TemplateBuffer.Reset()
	err := t.Execute(&w.TemplateBuffer, data)

	if err != nil {
		w.ContentAvailable = false
		w.Error = err

		slog.Error("failed to render template", "error", err)

		// need to immediately re-render with the error,
		// otherwise risk breaking the page since the widget
		// will likely be partially rendered with tags not closed.
		w.TemplateBuffer.Reset()
		err2 := t.Execute(&w.TemplateBuffer, data)

		if err2 != nil {
			slog.Error("failed to render error within widget", "error", err2, "initial_error", err)
			w.TemplateBuffer.Reset()
			// TODO: add some kind of a generic widget error template when the widget
			// failed to render, and we also failed to re-render the widget with the error
		}
	}

	return template.HTML(w.TemplateBuffer.String())
}

func (w *widgetBase) withTitle(title string) *widgetBase {
	if w.Title == "" {
		w.Title = title
	}

	return w
}

func (w *widgetBase) withCacheDuration(duration time.Duration) *widgetBase {
	w.CacheType = cacheTypeDuration

	if duration == -1 || w.CustomCacheDuration == 0 {
		w.CacheDuration = duration
	} else {
		w.CacheDuration = time.Duration(w.CustomCacheDuration)
	}

	return w
}

func (w *widgetBase) withCacheOnTheHour() *widgetBase {
	w.CacheType = cacheTypeOnTheHour

	return w
}

func (w *widgetBase) withCacheDaily() *widgetBase {
	w.CacheType = cacheTypeDaily

	return w
}

func (w *widgetBase) withNotice(err error) *widgetBase {
	w.Notice = err

	return w
}

func (w *widgetBase) withError(err error) *widgetBase {
	if err == nil && !w.ContentAvailable {
		w.ContentAvailable = true
	}

	w.Error = err

	return w
}

func (w *widgetBase) canContinueUpdateAfterHandlingErr(err error) bool {
	// TODO: needs covering more edge cases.
	// if there's partial content and we update early there's a chance
	// the early update returns even less content than the initial update.
	// need some kind of mechanism that tells us whether we should update early
	// or not depending on the number of things that failed during the initial
	// and subsequent update and how they failed - ie whether it was server
	// error (like gateway timeout, do retry early) or client error (like
	// hitting a rate limit, don't retry early). will require reworking a
	// good amount of code in the feed package and probably having a custom
	// error type that holds more information because screw wrapping errors.
	// alternatively have a resource cache and only refetch the failed resources,
	// then rebuild the widget.

	if err != nil {
		w.scheduleEarlyUpdate()

		if !errors.Is(err, feed.ErrPartialContent) {
			w.withError(err)
			w.withNotice(nil)
			return false
		}

		w.withError(nil)
		w.withNotice(err)
		return true
	}

	w.withNotice(nil)
	w.withError(nil)
	w.scheduleNextUpdate()
	return true
}

func (w *widgetBase) getNextUpdateTime() time.Time {
	now := time.Now()

	if w.CacheType == cacheTypeDuration {
		return now.Add(w.CacheDuration)
	}

	if w.CacheType == cacheTypeOnTheHour {
		return now.Add(time.Duration(
			((60-now.Minute())*60)-now.Second(),
		) * time.Second)
	}

	if w.CacheType == cacheTypeDaily {
		nextDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
		return nextDay
	}

	return time.Time{}
}

func (w *widgetBase) scheduleNextUpdate() *widgetBase {
	w.NextUpdate = w.getNextUpdateTime()
	w.UpdateRetriedTimes = 0

	return w
}

func (w *widgetBase) scheduleEarlyUpdate() *widgetBase {
	w.UpdateRetriedTimes++

	if w.UpdateRetriedTimes > 5 {
		w.UpdateRetriedTimes = 5
	}

	nextEarlyUpdate := time.Now().Add(time.Duration(math.Pow(float64(w.UpdateRetriedTimes), 2)) * time.Minute)
	nextUsualUpdate := w.getNextUpdateTime()

	if nextEarlyUpdate.After(nextUsualUpdate) {
		w.NextUpdate = nextUsualUpdate
	} else {
		w.NextUpdate = nextEarlyUpdate
	}

	return w
}

func (w *widgetBase) copyBaseStateFrom(src *widgetBase) {
	w.ContentAvailable = src.ContentAvailable
	w.Error = src.Error
	w.Notice = src.Notice
	w.TemplateBuffer.Reset()
	if src.TemplateBuffer.Len() > 0 {
		w.TemplateBuffer.Write(src.TemplateBuffer.Bytes())
	}
	w.NextUpdate = src.NextUpdate
	w.UpdateRetriedTimes = src.UpdateRetriedTimes
}

type stateCopier interface {
	GetBase() *widgetBase
}

func (w *widgetBase) GetBase() *widgetBase {
	return w
}

// CopyWidgetState copies base and cached fields from src widget to dst widget.
func CopyWidgetState(src, dst Widget) {
	if scSrc, ok := src.(stateCopier); ok {
		if scDst, ok := dst.(stateCopier); ok {
			scDst.GetBase().copyBaseStateFrom(scSrc.GetBase())
		}
	}

	srcVal := reflect.ValueOf(src).Elem()
	dstVal := reflect.ValueOf(dst).Elem()

	if srcVal.Type() != dstVal.Type() {
		return
	}

	for i := 0; i < srcVal.NumField(); i++ {
		field := srcVal.Type().Field(i)
		if field.Name == "widgetBase" {
			continue
		}

		tag := field.Tag.Get("yaml")
		if tag == "-" {
			srcField := srcVal.Field(i)
			dstField := dstVal.Field(i)
			if dstField.CanSet() {
				dstField.Set(srcField)
			}
		}
	}

	// Special case for Monitor widget Sites statuses since they are nested in struct slice
	if srcMon, ok := src.(*Monitor); ok {
		if dstMon, ok := dst.(*Monitor); ok {
			for i := range dstMon.Sites {
				for j := range srcMon.Sites {
					if dstMon.Sites[i].Url == srcMon.Sites[j].Url {
						dstMon.Sites[i].Status = srcMon.Sites[j].Status
						dstMon.Sites[i].StatusText = srcMon.Sites[j].StatusText
						dstMon.Sites[i].StatusStyle = srcMon.Sites[j].StatusStyle
						break
					}
				}
			}
		}
	}
}

