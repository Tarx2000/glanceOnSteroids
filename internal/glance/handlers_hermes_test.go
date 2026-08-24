package glance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		ApiUrl: widget.OptionalEnvString(api.URL),
		ApiKey: widget.OptionalEnvString(apiKey),
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
}

func TestHandleHermesNotify(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hermes/requests" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pending":0,"requests":[]}`))
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
					},
				}}},
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/hermes/notify", nil)
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
}
