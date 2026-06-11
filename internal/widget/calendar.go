package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// GoogleAuthorizedCheck callback returns true if Google Calendar is authorized.
// Wired dynamically from the main server code to bypass circular dependency.
var GoogleAuthorizedCheck func() bool

// GoogleCalendarEvent represents an event from Google Calendar API.
type GoogleCalendarEvent struct {
	Summary    string    `yaml:"summary"`
	Start      time.Time `yaml:"start"`
	End        time.Time `yaml:"end"`
	AllDay     bool      `yaml:"all-day"`
	CalendarID string    `yaml:"calendar-id"`
	Color      string    `yaml:"color"`
}

// FetchGoogleEvents callback fetches events from Google Calendar API.
// Wired dynamically from the main server code to bypass circular dependency.
var FetchGoogleEvents func(ctx context.Context, calendarIDs []string, maxDaysAhead int) ([]GoogleCalendarEvent, error)

// Event is the formatted event for rendering in the calendar HTML template.
type Event struct {
	Summary    string    `json:"summary"`
	Color      string    `json:"color"`
	TimeString string    `json:"time_string"`
	DateString string    `json:"date_string"`
	Start      time.Time `json:"-"`
}

type Calendar struct {
	widgetBase    `yaml:",inline"`
	ViewportLimit int      `yaml:"viewport-limit"`
	TimeFormat    string   `yaml:"time-format"`
	Calendars     []string `yaml:"calendars"`
	MaxDaysAhead  int      `yaml:"max-days-ahead"`
	Events        []Event  `yaml:"-"`
	Authorized    bool     `yaml:"-"`
}

func (widget *Calendar) Initialize() error {
	widget.withTitle("Calendar").withCacheDuration(10 * time.Minute)

	if widget.ViewportLimit <= 0 {
		widget.ViewportLimit = 5
	}
	if widget.MaxDaysAhead <= 0 {
		widget.MaxDaysAhead = 14
	}
	if widget.TimeFormat == "" {
		widget.TimeFormat = "24h"
	}

	return nil
}

func (widget *Calendar) Update(ctx context.Context) {
	authorized := false
	if GoogleAuthorizedCheck != nil {
		authorized = GoogleAuthorizedCheck()
	}

	widget.Lock()
	widget.Authorized = authorized
	widget.Unlock()

	if !authorized {
		widget.Lock()
		widget.Events = nil
		widget.Unlock()
		widget.withError(nil).scheduleNextUpdate()
		return
	}

	if FetchGoogleEvents != nil {
		rawEvents, err := FetchGoogleEvents(ctx, widget.Calendars, widget.MaxDaysAhead)
		if err != nil {
			widget.withError(err).scheduleEarlyUpdate()
			return
		}

		events := make([]Event, len(rawEvents))
		for i, re := range rawEvents {
			summary := re.Summary
			if summary == "" {
				summary = "No Title"
			}
			events[i] = Event{
				Summary:    summary,
				Color:      re.Color,
				TimeString: formatTimeString(re.Start, re.End, re.AllDay, widget.TimeFormat),
				DateString: formatDateString(re.Start),
				Start:      re.Start,
			}
		}

		widget.Lock()
		widget.Events = events
		widget.Unlock()
	}

	widget.withError(nil).scheduleNextUpdate()
}

func (widget *Calendar) Render() template.HTML {
	return widget.render(widget, assets.CalendarTemplate)
}

func formatTimeString(start, end time.Time, allDay bool, format string) string {
	if allDay {
		return "All Day"
	}

	timeFmt := "15:04"
	if format == "12h" {
		timeFmt = "3:04 PM"
	}

	return start.Format(timeFmt) + " - " + end.Format(timeFmt)
}

func formatDateString(t time.Time) string {
	now := time.Now()

	// Compare dates ignoring times
	year, month, day := t.Date()
	nyear, nmonth, nday := now.Date()

	if year == nyear && month == nmonth && day == nday {
		return "Today"
	}

	tomorrow := now.Add(24 * time.Hour)
	tyear, tmonth, tday := tomorrow.Date()
	if year == tyear && month == tmonth && day == tday {
		return "Tomorrow"
	}

	return t.Format("Monday, Jan 2")
}
