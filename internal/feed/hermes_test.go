package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchHermesPendingRequestsUsesBearerAndParsesResponse(t *testing.T) {
	const apiKey = "feed-test-key"
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/hermes/requests" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("expected bearer authorization, got %q", got)
		}
		if got := r.URL.Query().Get("take"); got != "3" {
			t.Fatalf("expected take=3, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pending": 1,
			"requests": []map[string]string{{
				"id":        "req-1",
				"title":     "Approve test",
				"createdAt": createdAt,
			}},
		})
	}))
	defer server.Close()

	result, err := FetchHermesPendingRequests(context.Background(), server.URL, apiKey, 3)
	if err != nil {
		t.Fatalf("fetch pending requests: %v", err)
	}
	if result.Pending != 1 || len(result.Requests) != 1 {
		t.Fatalf("expected one pending request, got pending=%d requests=%d", result.Pending, len(result.Requests))
	}
	if result.Requests[0].ID != "req-1" || result.Requests[0].TimeAgo == "" {
		t.Fatalf("unexpected parsed request: %+v", result.Requests[0])
	}
}

func TestPerformHermesActionUsesBearerAndEscapesRequestID(t *testing.T) {
	const apiKey = "action-test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.EscapedPath() != "/api/hermes/requests/req%2F1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("expected bearer authorization, got %q", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode action payload: %v", err)
		}
		if payload["action"] != "approve" {
			t.Fatalf("expected normalized approve action, got %q", payload["action"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := PerformHermesAction(context.Background(), server.URL, apiKey, "req/1", " APPROVE "); err != nil {
		t.Fatalf("perform approve action: %v", err)
	}
}

func TestPerformHermesActionFallsBackToPostAfterMethodNotAllowed(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST fallback, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := PerformHermesAction(context.Background(), server.URL, "", "req-1", "reject"); err != nil {
		t.Fatalf("perform reject action with POST fallback: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected PATCH plus POST fallback, got %d calls", got)
	}
}

func TestPerformHermesActionRejectsEditBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := PerformHermesAction(context.Background(), server.URL, "", "req-1", "edit")
	if err == nil || !strings.Contains(err.Error(), "approve or reject") {
		t.Fatalf("expected edit validation error, got %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("expected invalid edit action to avoid network, got %d calls", got)
	}
}

func TestHermesRequestUnmarshalJSONSupportsUnixMilliseconds(t *testing.T) {
	const timestamp = int64(1710000000123)
	var request HermesRequest
	payload, err := json.Marshal(map[string]any{"id": "req-1", "createdAt": timestamp})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.CreatedAt.UnixMilli() != timestamp {
		t.Fatalf("expected unix milliseconds %d, got %d", timestamp, request.CreatedAt.UnixMilli())
	}
	if request.TimeAgo == "" {
		t.Fatal("expected relative time to be populated")
	}
}
