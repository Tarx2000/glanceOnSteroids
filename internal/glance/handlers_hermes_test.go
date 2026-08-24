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

	app := &Application{
		slugToPage: map[string]*Page{
			"tech": {
				Columns: []Column{{Widgets: widget.Widgets{
					&widget.HermesApprove{
						ApiUrl: widget.OptionalEnvString(api.URL),
						ApiKey: widget.OptionalEnvString(apiKey),
					},
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
