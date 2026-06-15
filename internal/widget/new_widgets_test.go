package widget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockServiceProvider implements the ExternalServiceProvider interface for testing.
// It allows simulating authorized/unauthorized states, Google Gmail query responses,
// and Philips Hue status retrievals.
type mockServiceProvider struct {
	gmailAuthorized bool
	gmailCount      int
	gmailEmails     []GmailEmail
	gmailErr        error

	hueAuthorized   bool
	hueStatuses     []HueResource
	hueErr          error
}

func (m *mockServiceProvider) SpotifyAuthorized() bool { return false }
func (m *mockServiceProvider) GoogleAuthorized() bool  { return m.gmailAuthorized }
func (m *mockServiceProvider) FetchGoogleEvents(ctx context.Context, calendarIDs []string, maxDaysAhead int) ([]GoogleCalendarEvent, error) {
	return nil, nil
}
func (m *mockServiceProvider) FetchGmailUnreadCount(ctx context.Context) (int, []GmailEmail, error) {
	return m.gmailCount, m.gmailEmails, m.gmailErr
}
func (m *mockServiceProvider) HueAuthorized() bool { return m.hueAuthorized }
func (m *mockServiceProvider) FetchHueStatuses(ctx context.Context, rooms, lights, scenes []string) ([]HueResource, error) {
	return m.hueStatuses, m.hueErr
}

// TestMvvWidget verifies the Munich Live transit departure retrieval widget.
// It mocks the transport REST API using httptest.NewServer, testing error conditions
// for empty configuration as well as correct parsing and filtering logic.
func TestMvvWidget(t *testing.T) {
	// 1. Setup local mock HTTP server to return transport API responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockResponse := []map[string]any{
			{
				"plannedDepartureTime":  int64(1781479260000),
				"realtimeDepartureTime": int64(1781479380000), // 2 minutes delay (120000 ms)
				"transportType":         "UBAHN",
				"label":                 "U3",
				"destination":           "Marienplatz",
			},
			{
				"plannedDepartureTime":  int64(1781479500000),
				"realtimeDepartureTime": int64(1781479500000), // no delay
				"transportType":         "SUBURBAN",
				"label":                 "S1",
				"destination":           "Feldmoching",
			},
			{
				"plannedDepartureTime":  int64(1781479560000),
				"realtimeDepartureTime": int64(1781479560000),
				"transportType":         "SBAHN",
				"label":                 "S8",
				"destination":           "Flughafen München",
			},
			{
				"plannedDepartureTime":  int64(1781479620000),
				"realtimeDepartureTime": int64(1781479620000),
				"transportType":         "REGIONALBUS",
				"label":                 "X30",
				"destination":           "Harras",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	// Backup original API URL and override it with our local mock server URL
	oldURL := mvvBaseURL
	mvvBaseURL = server.URL
	defer func() { mvvBaseURL = oldURL }()

	// Test Case 1: Station ID is empty
	mvvWidget := &Mvv{
		StationID: "",
	}
	_ = mvvWidget.Initialize()
	mvvWidget.Update(context.Background(), nil)
	if mvvWidget.Error == nil {
		t.Error("expected error for empty StationID, got nil")
	}

	// Test Case 2: Successful fetch and filtering (all transport types enabled by default)
	mvvWidget = &Mvv{
		StationID: "de:09162:6", // Munich Marienplatz ID
		Limit:     4,
		ShowUBahn: true,
		ShowSBahn: true,
		ShowBus:   true,
		ShowTram:  true,
	}
	_ = mvvWidget.Initialize()
	mvvWidget.Update(context.Background(), nil)

	if mvvWidget.Error != nil {
		t.Errorf("expected no error, got: %v", mvvWidget.Error)
	}

	if len(mvvWidget.Departures) != 4 {
		t.Errorf("expected 4 departures, got %d", len(mvvWidget.Departures))
	}

	dep1 := mvvWidget.Departures[0]
	if dep1.Line != "U3" || dep1.Destination != "Marienplatz" || dep1.DelayMin != 2 || !dep1.HasDelay || dep1.Type != "ubahn" {
		t.Errorf("first departure parsed incorrectly: %+v", dep1)
	}

	dep3 := mvvWidget.Departures[2]
	if dep3.Line != "S8" || dep3.Destination != "Flughafen München" || dep3.Type != "sbahn" {
		t.Errorf("third departure (SBAHN type) parsed incorrectly: %+v", dep3)
	}

	dep4 := mvvWidget.Departures[3]
	if dep4.Line != "X30" || dep4.Destination != "Harras" || dep4.Type != "bus" {
		t.Errorf("fourth departure (REGIONALBUS type) parsed incorrectly: %+v", dep4)
	}

	// Test Case 3: Filter by Directions (inclusion)
	mvvWidget = &Mvv{
		StationID:  "de:09162:6",
		Limit:      4,
		ShowUBahn:  true,
		ShowSBahn:  true,
		ShowBus:    true,
		ShowTram:   true,
		Directions: "Flughafen, Feldmoching",
	}
	_ = mvvWidget.Initialize()
	mvvWidget.Update(context.Background(), nil)

	if len(mvvWidget.Departures) != 2 {
		t.Errorf("expected 2 departures after directions inclusion filter, got %d", len(mvvWidget.Departures))
	}
	if mvvWidget.Departures[0].Destination != "Feldmoching" || mvvWidget.Departures[1].Destination != "Flughafen München" {
		t.Errorf("unexpected filtered departures: %+v", mvvWidget.Departures)
	}

	// Test Case 4: Filter by ExcludeDirections
	mvvWidget = &Mvv{
		StationID:         "de:09162:6",
		Limit:             4,
		ShowUBahn:         true,
		ShowSBahn:         true,
		ShowBus:           true,
		ShowTram:          true,
		ExcludeDirections: "Feldmoching",
	}
	_ = mvvWidget.Initialize()
	mvvWidget.Update(context.Background(), nil)

	if len(mvvWidget.Departures) != 3 {
		t.Errorf("expected 3 departures after exclusion filter, got %d", len(mvvWidget.Departures))
	}
	for _, dep := range mvvWidget.Departures {
		if dep.Destination == "Feldmoching" {
			t.Error("found excluded destination 'Feldmoching' in departures")
		}
	}
}

// TestGmailWidget verifies the Gmail unread email counter widget.
// It checks widget behavior in both unauthorized and authorized configurations.
func TestGmailWidget(t *testing.T) {
	// Test Case 1: Unauthorized
	gmailWidget := &Gmail{}
	_ = gmailWidget.Initialize()
	mockSvc := &mockServiceProvider{gmailAuthorized: false}
	gmailWidget.Update(context.Background(), mockSvc)

	if gmailWidget.Authorized {
		t.Error("expected unauthorized, got authorized")
	}
	if gmailWidget.UnreadCount != 0 {
		t.Errorf("expected 0 unread count when unauthorized, got %d", gmailWidget.UnreadCount)
	}

	// Test Case 2: Authorized with unread messages
	mockEmails := []GmailEmail{
		{Subject: "Meeting Tomorrow", Sender: "boss@company.com", Date: "Mon, 15 Jun 2026"},
	}
	mockSvc = &mockServiceProvider{
		gmailAuthorized: true,
		gmailCount:      4,
		gmailEmails:     mockEmails,
	}
	gmailWidget.Update(context.Background(), mockSvc)

	if !gmailWidget.Authorized {
		t.Error("expected authorized, got unauthorized")
	}
	if gmailWidget.UnreadCount != 4 {
		t.Errorf("expected 4 unread messages, got %d", gmailWidget.UnreadCount)
	}
	if len(gmailWidget.Emails) != 1 || gmailWidget.Emails[0].Subject != "Meeting Tomorrow" {
		t.Errorf("emails list not updated correctly: %+v", gmailWidget.Emails)
	}
}

// TestHueWidget verifies the Philips Hue remote status widget.
// It ensures selected light, room, and scene configurations are retrieved
// and mapped correctly.
func TestHueWidget(t *testing.T) {
	// Test Case 1: Unauthorized
	hueWidget := &Hue{}
	_ = hueWidget.Initialize()
	mockSvc := &mockServiceProvider{hueAuthorized: false}
	hueWidget.Update(context.Background(), mockSvc)

	if hueWidget.Authorized {
		t.Error("expected unauthorized, got authorized")
	}
	if len(hueWidget.ResourceStatuses) != 0 {
		t.Errorf("expected no resources when unauthorized, got %d", len(hueWidget.ResourceStatuses))
	}

	// Test Case 2: Authorized with configured rooms and lights
	mockStatuses := []HueResource{
		{ID: "room-1", Name: "Wohnzimmer", Type: "room", On: true},
		{ID: "light-1", Name: "Stehlampe", Type: "light", On: false},
	}
	mockSvc = &mockServiceProvider{
		hueAuthorized: true,
		hueStatuses:   mockStatuses,
	}
	hueWidget = &Hue{
		Rooms:  []string{"room-1"},
		Lights: []string{"light-1"},
	}
	_ = hueWidget.Initialize()
	hueWidget.Update(context.Background(), mockSvc)

	if !hueWidget.Authorized {
		t.Error("expected authorized, got unauthorized")
	}
	if len(hueWidget.ResourceStatuses) != 2 {
		t.Errorf("expected 2 resources, got %d", len(hueWidget.ResourceStatuses))
	}
	res1 := hueWidget.ResourceStatuses[0]
	if res1.ID != "room-1" || res1.Name != "Wohnzimmer" || res1.Type != "room" || !res1.On {
		t.Errorf("first resource status parsed incorrectly: %+v", res1)
	}
}
