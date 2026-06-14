package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// mvvBaseURL is the base URL for the public transport API.
// Defining this as a package-level variable allows overriding it during unit testing.
var mvvBaseURL = "https://v6.db.transport.rest"

type MvvDeparture struct {
	Line        string `json:"line"`
	Destination string `json:"destination"`
	Time        string `json:"time"`
	DelayMin    int    `json:"delay_min"`
	HasDelay    bool   `json:"has_delay"`
	Type        string `json:"type"` // sbahn, ubahn, bus, tram
}

type Mvv struct {
	widgetBase  `yaml:",inline"`
	StationID   string         `yaml:"station-id"`
	StationName string         `yaml:"station-name"`
	Limit       int            `yaml:"limit"`
	ShowSBahn   BoolField      `yaml:"show-sbahn"`
	ShowUBahn   BoolField      `yaml:"show-ubahn"`
	ShowBus     BoolField      `yaml:"show-bus"`
	ShowTram    BoolField      `yaml:"show-tram"`
	Departures  []MvvDeparture `yaml:"-"`
}

func init() {
	Register("mvv", func() Widget { return &Mvv{} })
}

func (widget *Mvv) Initialize() error {
	widget.withTitle("München Live").withCacheDuration(2 * time.Minute)

	if widget.Limit <= 0 {
		widget.Limit = 4
	}

	return nil
}

func (widget *Mvv) Update(ctx context.Context, services ExternalServiceProvider) {
	if widget.StationID == "" {
		widget.withError(fmt.Errorf("keine Haltestellen-ID konfiguriert")).scheduleNextUpdate()
		return
	}

	apiURL := fmt.Sprintf("%s/stops/%s/departures?duration=90&results=40", mvvBaseURL, url.PathEscape(widget.StationID))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		widget.withError(fmt.Errorf("API-Fehler: Status %d", resp.StatusCode)).scheduleEarlyUpdate()
		return
	}

	var rawDepartures []struct {
		Direction   string `json:"direction"`
		When        string `json:"when"`
		PlannedWhen string `json:"plannedWhen"`
		Delay       *int   `json:"delay"` // in seconds
		Line        struct {
			Name    string `json:"name"`
			Mode    string `json:"mode"`
			Product string `json:"product"`
		} `json:"line"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawDepartures); err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}

	var departures []MvvDeparture

	for _, rd := range rawDepartures {
		// Determine transport type
		prod := strings.ToLower(rd.Line.Product)
		mode := strings.ToLower(rd.Line.Mode)
		name := rd.Line.Name

		var t string
		if prod == "suburban" || mode == "train" || strings.HasPrefix(name, "S") {
			t = "sbahn"
			if !widget.ShowSBahn {
				continue
			}
		} else if prod == "subway" || strings.HasPrefix(name, "U") {
			t = "ubahn"
			if !widget.ShowUBahn {
				continue
			}
		} else if mode == "bus" || prod == "bus" {
			t = "bus"
			if !widget.ShowBus {
				continue
			}
		} else if mode == "tram" || prod == "tram" || strings.Contains(strings.ToLower(name), "tram") {
			t = "tram"
			if !widget.ShowTram {
				continue
			}
		} else {
			// Fallback filter check (default to S-Bahn if unclear, or skip if none selected)
			t = "bus"
			if !widget.ShowBus {
				continue
			}
		}

		// Parse departure time
		timeStr := ""
		tVal := rd.When
		if tVal == "" {
			tVal = rd.PlannedWhen
		}
		if tVal != "" {
			parsedTime, err := time.Parse(time.RFC3339, tVal)
			if err == nil {
				timeStr = parsedTime.Format("15:04")
			}
		}

		// Calculate delay in minutes
		delayMin := 0
		hasDelay := false
		if rd.Delay != nil {
			delayMin = *rd.Delay / 60
			hasDelay = true
		}

		departures = append(departures, MvvDeparture{
			Line:        name,
			Destination: rd.Direction,
			Time:        timeStr,
			DelayMin:    delayMin,
			HasDelay:    hasDelay,
			Type:        t,
		})

		if len(departures) >= widget.Limit {
			break
		}
	}

	widget.Lock()
	widget.Departures = departures
	widget.Unlock()

	widget.withError(nil).scheduleNextUpdate()
}

func (widget *Mvv) Render() template.HTML {
	return widget.render(widget, assets.MVVTemplate)
}
