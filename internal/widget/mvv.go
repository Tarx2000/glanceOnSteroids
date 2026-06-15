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
var mvvBaseURL = "https://www.mvg.de/api/bgw-pt/v3"

type MvvDeparture struct {
	Line        string `json:"line"`
	Destination string `json:"destination"`
	Time        string `json:"time"`
	DelayMin    int    `json:"delay_min"`
	HasDelay    bool   `json:"has_delay"`
	Type        string `json:"type"` // sbahn, ubahn, bus, tram
}

type Mvv struct {
	widgetBase        `yaml:",inline"`
	StationID         string         `yaml:"station-id"`
	StationName       string         `yaml:"station-name"`
	Limit             int            `yaml:"limit"`
	ShowSBahn         BoolField      `yaml:"show-sbahn"`
	ShowUBahn         BoolField      `yaml:"show-ubahn"`
	ShowBus           BoolField      `yaml:"show-bus"`
	ShowTram          BoolField      `yaml:"show-tram"`
	Directions        string         `yaml:"directions"`
	ExcludeDirections string         `yaml:"exclude-directions"`
	Departures        []MvvDeparture `yaml:"-"`
}

func init() {
	Register("mvv", func() Widget {
		return &Mvv{
			ShowSBahn: true,
			ShowUBahn: true,
			ShowBus:   true,
			ShowTram:  true,
		}
	})
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

	apiURL := fmt.Sprintf("%s/departures?globalId=%s", mvvBaseURL, url.QueryEscape(widget.StationID))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

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
		PlannedDepartureTime  int64  `json:"plannedDepartureTime"`
		RealtimeDepartureTime int64  `json:"realtimeDepartureTime"`
		TransportType         string `json:"transportType"` // UBAHN, SUBURBAN, BUS, TRAM
		Label                 string `json:"label"`
		Destination           string `json:"destination"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawDepartures); err != nil {
		widget.withError(err).scheduleEarlyUpdate()
		return
	}

	var departures []MvvDeparture

	for _, rd := range rawDepartures {
		// Determine transport type
		var t string
		switch rd.TransportType {
		case "SUBURBAN", "SBAHN":
			t = "sbahn"
			if !widget.ShowSBahn {
				continue
			}
		case "UBAHN":
			t = "ubahn"
			if !widget.ShowUBahn {
				continue
			}
		case "BUS", "REGIONALBUS":
			t = "bus"
			if !widget.ShowBus {
				continue
			}
		case "TRAM":
			t = "tram"
			if !widget.ShowTram {
				continue
			}
		default:
			t = "bus"
			if !widget.ShowBus {
				continue
			}
		}

		// Filter by directions (include/exclude)
		if widget.Directions != "" {
			include := false
			target := strings.ToLower(rd.Destination)
			parts := strings.Split(widget.Directions, ",")
			for _, p := range parts {
				p = strings.TrimSpace(strings.ToLower(p))
				if p != "" && strings.Contains(target, p) {
					include = true
					break
				}
			}
			if !include {
				continue
			}
		}

		if widget.ExcludeDirections != "" {
			exclude := false
			target := strings.ToLower(rd.Destination)
			parts := strings.Split(widget.ExcludeDirections, ",")
			for _, p := range parts {
				p = strings.TrimSpace(strings.ToLower(p))
				if p != "" && strings.Contains(target, p) {
					exclude = true
					break
				}
			}
			if exclude {
				continue
			}
		}

		// Parse departure time
		timeStr := ""
		if rd.RealtimeDepartureTime > 0 {
			timeStr = time.UnixMilli(rd.RealtimeDepartureTime).Format("15:04")
		} else if rd.PlannedDepartureTime > 0 {
			timeStr = time.UnixMilli(rd.PlannedDepartureTime).Format("15:04")
		}

		// Calculate delay in minutes
		delayMin := int((rd.RealtimeDepartureTime - rd.PlannedDepartureTime) / 60000)
		hasDelay := rd.RealtimeDepartureTime != rd.PlannedDepartureTime

		departures = append(departures, MvvDeparture{
			Line:        rd.Label,
			Destination: rd.Destination,
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
