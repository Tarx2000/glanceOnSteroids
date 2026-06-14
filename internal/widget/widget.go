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

var GlobalTimezone string

type GmailEmail struct {
	Subject string `json:"subject"`
	Sender  string `json:"sender"`
	Date    string `json:"date"`
}

type HueResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // room, light, scene
	On   bool   `json:"on"`
}

// ExternalServiceProvider defines the interface for third-party integrations
// such as Spotify and Google Calendar to bypass circular dependencies.
type ExternalServiceProvider interface {
	SpotifyAuthorized() bool
	GoogleAuthorized() bool
	FetchGoogleEvents(ctx context.Context, calendarIDs []string, maxDaysAhead int) ([]GoogleCalendarEvent, error)
	FetchGmailUnreadCount(ctx context.Context) (int, []GmailEmail, error)
	HueAuthorized() bool
	FetchHueStatuses(ctx context.Context, rooms, lights, scenes []string) ([]HueResource, error)
}

var registry = make(map[string]func() Widget)

func Register(widgetType string, factory func() Widget) {
	registry[widgetType] = factory
}

func New(widgetType string) (Widget, error) {
	factory, ok := registry[widgetType]
	if !ok {
		return nil, fmt.Errorf("unknown widget type: %s", widgetType)
	}
	return factory(), nil
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
	Update(context.Context, ExternalServiceProvider)
	Render() template.HTML
	GetType() string
}

type StatefulWidget interface {
	Widget
	CopyStateFrom(other Widget)
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
	LastUpdate          time.Time     `yaml:"-"` // Tracks when the widget data was last successfully updated
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

func (w *widgetBase) Update(ctx context.Context, services ExternalServiceProvider) {

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
	if w.CustomCacheDuration != 0 {
		w.CacheType = cacheTypeDuration
		w.CacheDuration = time.Duration(w.CustomCacheDuration)
	} else {
		w.CacheType = cacheTypeOnTheHour
	}
	return w
}

func (w *widgetBase) withCacheDaily() *widgetBase {
	if w.CustomCacheDuration != 0 {
		w.CacheType = cacheTypeDuration
		w.CacheDuration = time.Duration(w.CustomCacheDuration)
	} else {
		w.CacheType = cacheTypeDaily
	}
	return w
}

func (w *widgetBase) withNotice(err error) *widgetBase {
	w.Lock()
	defer w.Unlock()
	w.Notice = err
	return w
}

func (w *widgetBase) withError(err error) *widgetBase {
	w.Lock()
	defer w.Unlock()
	if err == nil && !w.ContentAvailable {
		w.ContentAvailable = true
	}
	w.Error = err
	return w
}

func (w *widgetBase) canContinueUpdateAfterHandlingErr(err error) bool {
	w.Lock()
	defer w.Unlock()

	if err != nil {
		// scheduleEarlyUpdate inline to prevent re-entrancy deadlock
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

		if !errors.Is(err, feed.ErrPartialContent) {
			w.Error = err
			w.Notice = nil
			w.ContentAvailable = false
			return false
		}

		w.Error = nil
		w.Notice = err
		if !w.ContentAvailable {
			w.ContentAvailable = true
		}
		w.LastUpdate = time.Now() // Set LastUpdate on partial success
		return true
	}

	w.Notice = nil
	w.Error = nil
	if !w.ContentAvailable {
		w.ContentAvailable = true
	}
	
	// scheduleNextUpdate inline to prevent re-entrancy deadlock
	w.NextUpdate = w.getNextUpdateTime()
	w.UpdateRetriedTimes = 0
	w.LastUpdate = time.Now() // Set LastUpdate on success
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
	w.Lock()
	defer w.Unlock()
	w.NextUpdate = w.getNextUpdateTime()
	w.UpdateRetriedTimes = 0
	w.LastUpdate = time.Now() // Set LastUpdate on success
	return w
}

func (w *widgetBase) scheduleEarlyUpdate() *widgetBase {
	w.Lock()
	defer w.Unlock()
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

func (w *widgetBase) copyBaseStateFromLocked(src *widgetBase) {
	w.ContentAvailable = src.ContentAvailable
	w.Error = src.Error
	w.Notice = src.Notice
	w.TemplateBuffer.Reset()
	if src.TemplateBuffer.Len() > 0 {
		w.TemplateBuffer.Write(src.TemplateBuffer.Bytes())
	}
	w.NextUpdate = src.NextUpdate
	w.LastUpdate = src.LastUpdate // Copy LastUpdate state
	w.UpdateRetriedTimes = src.UpdateRetriedTimes
}

func (w *widgetBase) copyBaseStateFrom(src *widgetBase) {
	src.Lock()
	defer src.Unlock()
	w.Lock()
	defer w.Unlock()
	w.copyBaseStateFromLocked(src)
}

type stateCopier interface {
	GetBase() *widgetBase
}

func (w *widgetBase) GetBase() *widgetBase {
	return w
}

// CopyWidgetState copies base and cached fields from src widget to dst widget.
func CopyWidgetState(src, dst Widget) {
	var srcBase, dstBase *widgetBase
	if scSrc, ok := src.(stateCopier); ok {
		srcBase = scSrc.GetBase()
	}
	if scDst, ok := dst.(stateCopier); ok {
		dstBase = scDst.GetBase()
	}

	if srcBase != nil {
		srcBase.Lock()
		defer srcBase.Unlock()
	}
	if dstBase != nil {
		dstBase.Lock()
		defer dstBase.Unlock()
	}

	if srcBase != nil && dstBase != nil {
		dstBase.copyBaseStateFromLocked(srcBase)
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

	if swDst, ok := dst.(StatefulWidget); ok {
		swDst.CopyStateFrom(src)
	}
}

