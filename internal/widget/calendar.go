package widget

import (
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// GoogleCalendarEvent represents an event from Google Calendar API.
type GoogleCalendarEvent struct {
	Summary    string    `yaml:"summary"`
	Start      time.Time `yaml:"start"`
	End        time.Time `yaml:"end"`
	AllDay     bool      `yaml:"all-day"`
	CalendarID string    `yaml:"calendar-id"`
	Color      string    `yaml:"color"`
}

// Event is the formatted event for rendering in the calendar HTML template.
type Event struct {
	Summary      string    `json:"summary"`
	Color        string    `json:"color"`
	TimeString   string    `json:"time_string"`
	DateString   string    `json:"date_string"`
	RelativeTime string    `json:"relative_time,omitempty"` // E.g. "in 2h 15m" (only for today's future events)
	Start        time.Time `json:"-"`
}

// DayGroup represents a set of events grouped under a single calendar day header (e.g. "Today").
type DayGroup struct {
	DateString string  `json:"date_string"`
	Events     []Event `json:"events"`
}

type Calendar struct {
	widgetBase    `yaml:",inline"`
	ViewportLimit int        `yaml:"viewport-limit"`
	TimeFormat    string     `yaml:"time-format"`
	Calendars     []string   `yaml:"calendars"`
	MaxDaysAhead  int        `yaml:"max-days-ahead"`
	DayGroups     []DayGroup `yaml:"-"`
	Authorized    bool       `yaml:"-"`
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
	if Services != nil {
		authorized = Services.GoogleAuthorized()
	}

	widget.Lock()
	widget.Authorized = authorized
	widget.Unlock()

	if !authorized {
		widget.Lock()
		widget.DayGroups = nil
		widget.Unlock()
		widget.withError(nil).scheduleNextUpdate()
		return
	}

	if Services != nil {
		rawEvents, err := Services.FetchGoogleEvents(ctx, widget.Calendars, widget.MaxDaysAhead)
		if err != nil {
			widget.withError(err).scheduleEarlyUpdate()
			return
		}

		events := make([]Event, len(rawEvents))
		now := time.Now()
		nyear, nmonth, nday := now.Date()

		for i, re := range rawEvents {
			summary := re.Summary
			if summary == "" {
				summary = "No Title"
			}

			// Calculate relative time if it starts today and is in the future
			relativeTime := ""
			year, month, day := re.Start.Date()
			if year == nyear && month == nmonth && day == nday && re.Start.After(now) && !re.AllDay {
				diff := re.Start.Sub(now)
				hours := int(diff.Hours())
				minutes := int(diff.Minutes()) % 60
				if hours > 0 {
					if minutes > 0 {
						relativeTime = fmt.Sprintf("in %dh %dm", hours, minutes)
					} else {
						relativeTime = fmt.Sprintf("in %dh", hours)
					}
				} else if minutes > 0 {
					relativeTime = fmt.Sprintf("in %dm", minutes)
				} else {
					relativeTime = "now"
				}
			}

			events[i] = Event{
				Summary:      summary,
				Color:        re.Color,
				TimeString:   formatTimeString(re.Start, re.End, re.AllDay, widget.TimeFormat),
				DateString:   formatDateString(re.Start),
				RelativeTime: relativeTime,
				Start:        re.Start,
			}
		}

		// Group events chronologically by day
		var dayGroups []DayGroup
		for _, eq := range events {
			if len(dayGroups) == 0 || dayGroups[len(dayGroups)-1].DateString != eq.DateString {
				dayGroups = append(dayGroups, DayGroup{
					DateString: eq.DateString,
					Events:     []Event{eq},
				})
			} else {
				idx := len(dayGroups) - 1
				dayGroups[idx].Events = append(dayGroups[idx].Events, eq)
			}
		}

		widget.Lock()
		widget.DayGroups = dayGroups
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
