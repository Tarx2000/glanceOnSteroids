package glance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glanceapp/glance/internal/feed"
	"github.com/glanceapp/glance/internal/widget"
)

func TestHandleHermesActionUsesConfiguredKeyWhenAPIURLIsProvided(t *testing.T) {
	const apiKey = "test-hermes-key"

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPatch || r.URL.Path != "/api/hermes/requests/req-1" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer api.Close()

	hub := NewHub()
	hw := &widget.HermesApprove{
		ApiUrl:       widget.OptionalEnvString(api.URL),
		ApiKey:       widget.OptionalEnvString(apiKey),
		Requests:     []feed.HermesRequest{{ID: "req-1"}},
		PendingCount: 1,
	}

	app := &Application{
		Hub: hub,
		slugToPage: map[string]*Page{
			"tech": {
				Slug: "tech",
				Columns: []Column{{Widgets: widget.Widgets{
					hw,
				}}},
			},
		},
	}

	body, err := json.Marshal(HermesActionRequest{
		RequestID: "req-1",
		Action:    "approve",
		ApiUrl:    api.URL,
		ApiKey:    "wrong-client-supplied-key",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/hermes/action", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app.HandleHermesAction(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(hw.Requests) != 0 || hw.PendingCount != 0 {
		t.Fatalf("expected approved request to be removed from widget state, got %d requests and %d pending", len(hw.Requests), hw.PendingCount)
	}
}

func TestHandleHermesNotify(t *testing.T) {
	const apiKey = "notify-test-key"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/hermes/requests" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(feed.HermesRequestsResponse{
			Pending:  0,
			Requests: []feed.HermesRequest{},
		})
	}))
	defer api.Close()

	hub := NewHub()
	app := &Application{
		Hub: hub,
		slugToPage: map[string]*Page{
			"home": {
				Slug: "home",
				Columns: []Column{{Widgets: widget.Widgets{
					&widget.HermesApprove{
						ApiUrl: widget.OptionalEnvString(api.URL),
						ApiKey: widget.OptionalEnvString("Bearer " + apiKey),
					},
				}}},
			},
		},
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/hermes/notify", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	app.HandleHermesNotify(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing webhook token to return 401, got %d", unauthorizedRecorder.Code)
	}

	wrongToken := httptest.NewRequest(http.MethodPost, "/api/hermes/notify", nil)
	wrongToken.Header.Set("Authorization", "Bearer wrong-token")
	wrongTokenRecorder := httptest.NewRecorder()
	app.HandleHermesNotify(wrongTokenRecorder, wrongToken)
	if wrongTokenRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid webhook token to return 401, got %d", wrongTokenRecorder.Code)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/hermes/notify", nil)
	getRequest.Header.Set("Authorization", "Bearer "+apiKey)
	getRecorder := httptest.NewRecorder()
	app.HandleHermesNotify(getRecorder, getRequest)
	if getRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected GET webhook to return 405, got %d", getRecorder.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/hermes/notify", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	recorder := httptest.NewRecorder()
	app.HandleHermesNotify(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var res map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&res); err != nil {
		t.Fatalf("decode notify response: %v", err)
	}
	if res["success"] != true {
		t.Errorf("expected success true, got %v", res["success"])
	}
	if res["updated"] != float64(1) {
		t.Errorf("expected one refreshed widget, got %v", res["updated"])
	}
}

func TestHandleHermesActionRejectsEdit(t *testing.T) {
	body, err := json.Marshal(HermesActionRequest{
		RequestID: "req-1",
		Action:    "edit",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/hermes/action", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	(&Application{}).HandleHermesAction(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected edit action to return 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
