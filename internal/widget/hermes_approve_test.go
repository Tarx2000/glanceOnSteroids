package widget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glanceapp/glance/internal/feed"
	"gopkg.in/yaml.v3"
)

func TestHermesApproveWidget(t *testing.T) {
	// 1. Test widget creation & registry
	wd, err := New("hermes-approve")
	if err != nil {
		t.Fatalf("failed to create hermes-approve widget: %v", err)
	}

	hw, ok := wd.(*HermesApprove)
	if !ok {
		t.Fatalf("expected *HermesApprove, got %T", wd)
	}

	if err := hw.Initialize(); err != nil {
		t.Fatalf("failed to initialize widget: %v", err)
	}

	if hw.Title != "Hermes Approvals" {
		t.Errorf("expected title 'Hermes Approvals', got '%s'", hw.Title)
	}
	if hw.Limit != 10 {
		t.Errorf("expected default limit 10, got %d", hw.Limit)
	}
	if hw.ApiUrl != "http://localhost:3000" {
		t.Errorf("expected default ApiUrl 'http://localhost:3000', got '%s'", hw.ApiUrl)
	}
	if hw.CacheDuration != 5*time.Second {
		t.Errorf("expected default cache duration 5s, got %v", hw.CacheDuration)
	}

	// 2. Test YAML decoding
	yamlConfig := `
type: hermes-approve
title: My Agent Approvals
api-url: http://127.0.0.1:3000
api-key: secret-token-123
limit: 5
cache: 10s
`
	var decoded HermesApprove
	if err := yaml.Unmarshal([]byte(yamlConfig), &decoded); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}
	if err := decoded.Initialize(); err != nil {
		t.Fatalf("failed to initialize decoded widget: %v", err)
	}
	if decoded.Title != "My Agent Approvals" {
		t.Errorf("expected title 'My Agent Approvals', got '%s'", decoded.Title)
	}
	if decoded.ApiUrl != "http://127.0.0.1:3000" {
		t.Errorf("expected ApiUrl 'http://127.0.0.1:3000', got '%s'", decoded.ApiUrl)
	}
	if decoded.ApiKey != "secret-token-123" {
		t.Errorf("expected ApiKey 'secret-token-123', got '%s'", decoded.ApiKey)
	}
	if decoded.Limit != 5 {
		t.Errorf("expected limit 5, got %d", decoded.Limit)
	}
	if decoded.CacheDuration != 10*time.Second {
		t.Errorf("expected CacheDuration 10s, got %v", decoded.CacheDuration)
	}

	// 3. Test mock HTTP server for pending requests and actions
	actionReceived := make(map[string]any)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/hermes/requests") {
			// Return mock pending requests
			res := feed.HermesRequestsResponse{
				Pending: 2,
				Requests: []feed.HermesRequest{
					{
						ID:            "req-1",
						Origin:        "hermes",
						Kind:          "oneshot",
						Title:         "Deploy Container to Prod",
						Prompt:        "docker compose up -d --build",
						SideEffecting: true,
						Status:        "awaiting_approval",
						CreatedAt:     time.Now().Add(-5 * time.Minute),
					},
					{
						ID:            "req-2",
						Origin:        "web",
						Kind:          "cron.run",
						Title:         "Run Database Backup Job",
						Prompt:        "pg_dump -Fc > backup.dump",
						SideEffecting: false,
						Status:        "awaiting_approval",
						CreatedAt:     time.Now().Add(-1 * time.Hour),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
			return
		}

		if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/hermes/requests/") {
			_ = json.NewDecoder(r.Body).Decode(&actionReceived)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success": true}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	// 4. Update widget against mock server
	hw.ApiUrl = OptionalEnvString(mockServer.URL)
	hw.Update(context.Background(), nil)

	if len(hw.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(hw.Requests))
	}
	if hw.PendingCount != 2 {
		t.Errorf("expected PendingCount 2, got %d", hw.PendingCount)
	}

	// 5. Test rendering
	html := string(hw.Render())
	if !strings.Contains(html, "Deploy Container to Prod") {
		t.Errorf("rendered HTML missing request title: %s", html)
	}
	if !strings.Contains(html, "side-effecting") {
		t.Errorf("rendered HTML missing side-effecting badge: %s", html)
	}
	if !strings.Contains(html, "docker compose up -d --build") {
		t.Errorf("rendered HTML missing prompt content: %s", html)
	}
	if !strings.Contains(html, "data-hermes-action=\"approve\"") {
		t.Errorf("rendered HTML missing approve button: %s", html)
	}
	if !strings.Contains(html, "data-hermes-action=\"reject\"") {
		t.Errorf("rendered HTML missing reject button: %s", html)
	}

	// 6. Test RemoveRequest in-memory
	hw.RemoveRequest("req-1")
	if len(hw.Requests) != 1 || hw.PendingCount != 1 {
		t.Errorf("expected 1 request after RemoveRequest, got %d (pending %d)", len(hw.Requests), hw.PendingCount)
	}
	if hw.Requests[0].ID != "req-2" {
		t.Errorf("expected remaining request to be req-2, got %s", hw.Requests[0].ID)
	}

	// 7. Test empty state render
	hw.RemoveRequest("req-2")
	emptyHtml := string(hw.Render())
	if !strings.Contains(emptyHtml, "All clear") {
		t.Errorf("rendered HTML missing empty state: %s", emptyHtml)
	}

	// 8. Test perform actions (approve, reject)
	err = feed.PerformHermesAction(context.Background(), mockServer.URL, "", "req-1", "approve")
	if err != nil {
		t.Fatalf("failed to perform approve action: %v", err)
	}
	if actionReceived["action"] != "approve" {
		t.Errorf("expected action 'approve', got %v", actionReceived["action"])
	}

	err = feed.PerformHermesAction(context.Background(), mockServer.URL, "", "req-2", "reject")
	if err != nil {
		t.Fatalf("failed to perform reject action: %v", err)
	}
	if actionReceived["action"] != "reject" {
		t.Errorf("expected action 'reject', got %v", actionReceived["action"])
	}
}
