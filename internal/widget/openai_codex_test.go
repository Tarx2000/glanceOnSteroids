package widget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAICodexWidgetRegistration(t *testing.T) {
	w, err := New("openai-codex")
	if err != nil {
		t.Fatalf("failed to create openai-codex widget: %v", err)
	}
	if _, ok := w.(*OpenAICodex); !ok {
		t.Fatalf("expected *OpenAICodex, got %T", w)
	}
	if err := w.Initialize(); err != nil {
		t.Fatalf("failed to initialize widget: %v", err)
	}
	if w.GetType() != "openai-codex" {
		t.Errorf("expected type 'openai-codex', got '%s'", w.GetType())
	}
}

func TestOpenAICodexWidgetUnmarshal(t *testing.T) {
	yamlStr := `
type: openai-codex
title: "My Codex Limits"
token: "my-test-token"
account-id: "acc-123"
endpoint: "https://example.com/usage"
show-session-limit: true
show-weekly-limit: false
show-credits: true
`
	var widgets Widgets
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &node); err != nil {
		t.Fatalf("failed to unmarshal yaml node: %v", err)
	}
	// The document node wraps the mapping or sequence
	var seqNode yaml.Node
	seqNode.Kind = yaml.SequenceNode
	seqNode.Content = []*yaml.Node{node.Content[0]}

	if err := widgets.UnmarshalYAML(&seqNode); err != nil {
		t.Fatalf("failed to unmarshal widgets: %v", err)
	}

	if len(widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(widgets))
	}

	cw, ok := widgets[0].(*OpenAICodex)
	if !ok {
		t.Fatalf("expected *OpenAICodex, got %T", widgets[0])
	}

	if cw.Title != "My Codex Limits" {
		t.Errorf("expected title 'My Codex Limits', got '%s'", cw.Title)
	}
	if cw.Token != "my-test-token" {
		t.Errorf("expected token 'my-test-token', got '%s'", cw.Token)
	}
	if cw.AccountID != "acc-123" {
		t.Errorf("expected account-id 'acc-123', got '%s'", cw.AccountID)
	}
	if cw.Endpoint != "https://example.com/usage" {
		t.Errorf("expected endpoint 'https://example.com/usage', got '%s'", cw.Endpoint)
	}
	if cw.ShowSessionLimit == nil || !*cw.ShowSessionLimit {
		t.Errorf("expected show-session-limit true")
	}
	if cw.ShowWeeklyLimit == nil || *cw.ShowWeeklyLimit {
		t.Errorf("expected show-weekly-limit false")
	}
	if cw.ShowCredits == nil || !*cw.ShowCredits {
		t.Errorf("expected show-credits true")
	}
}

func TestOpenAICodexUpdateAndRender(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"plan_type": "pro",
			"rate_limit": {
				"primary_window": {
					"used_percent": 20.0,
					"limit_window_seconds": 18000,
					"reset_after_seconds": 7200
				},
				"secondary_window": {
					"used_percent": 45.0,
					"limit_window_seconds": 604800,
					"reset_after_seconds": 259200
				}
			},
			"credits": {
				"balance": 25.00,
				"unlimited": false
			}
		}`))
	}))
	defer server.Close()

	w, err := New("openai-codex")
	if err != nil {
		t.Fatalf("failed to create widget: %v", err)
	}
	codexWidget := w.(*OpenAICodex)
	codexWidget.Token = "test-token"
	codexWidget.Endpoint = server.URL

	ctx := context.Background()
	codexWidget.Update(ctx, nil)

	if codexWidget.Error != nil {
		t.Fatalf("unexpected widget error: %v", codexWidget.Error)
	}
	if codexWidget.PlanType != "PRO" {
		t.Errorf("expected PlanType 'PRO', got '%s'", codexWidget.PlanType)
	}
	if codexWidget.PrimaryUsedPct != 20.0 {
		t.Errorf("expected PrimaryUsedPct 20.0, got %v", codexWidget.PrimaryUsedPct)
	}
	if codexWidget.PrimaryLeftPct != 80.0 {
		t.Errorf("expected PrimaryLeftPct 80.0, got %v", codexWidget.PrimaryLeftPct)
	}
	if codexWidget.SecondaryUsedPct != 45.0 {
		t.Errorf("expected SecondaryUsedPct 45.0, got %v", codexWidget.SecondaryUsedPct)
	}
	if codexWidget.SecondaryLeftPct != 55.0 {
		t.Errorf("expected SecondaryLeftPct 55.0, got %v", codexWidget.SecondaryLeftPct)
	}
	if codexWidget.CreditBalance != "$25.00" {
		t.Errorf("expected CreditBalance '$25.00', got '%s'", codexWidget.CreditBalance)
	}

	html := string(codexWidget.Render())
	if !strings.Contains(html, "PRO") {
		t.Errorf("rendered HTML missing plan badge 'PRO': %s", html)
	}
	if !strings.Contains(html, "80% left") {
		t.Errorf("rendered HTML missing '80%% left': %s", html)
	}
	if !strings.Contains(html, "55% left") {
		t.Errorf("rendered HTML missing '55%% left': %s", html)
	}
	if !strings.Contains(html, "$25.00") {
		t.Errorf("rendered HTML missing credit balance '$25.00': %s", html)
	}
}

func TestOpenAICodexAuthFileResolution(t *testing.T) {
	tempDir := t.TempDir()
	authPath := filepath.Join(tempDir, "auth.json")
	content := `{"tokens":{"access_token":"from-file-token"}}`
	if err := os.WriteFile(authPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test auth file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer from-file-token" {
			http.Error(w, "Bad Token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"plan_type": "plus"}`))
	}))
	defer server.Close()

	w, err := New("openai-codex")
	if err != nil {
		t.Fatalf("failed to create widget: %v", err)
	}
	codexWidget := w.(*OpenAICodex)
	codexWidget.AuthFile = authPath
	codexWidget.Endpoint = server.URL

	ctx := context.Background()
	codexWidget.Update(ctx, nil)

	if codexWidget.Error != nil {
		t.Fatalf("unexpected error resolving auth from file: %v", codexWidget.Error)
	}
	if codexWidget.PlanType != "PLUS" {
		t.Errorf("expected PlanType 'PLUS', got '%s'", codexWidget.PlanType)
	}
}
