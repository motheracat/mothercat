package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

type storeStub struct {
	event domain.Event
	err   error
	calls int
}

func (store *storeStub) Ping(context.Context) error { return nil }
func (store *storeStub) Record(_ context.Context, event domain.Event) error {
	store.event, store.calls = event, store.calls+1
	return store.err
}

func TestRecordEventRequiresServiceToken(t *testing.T) {
	handler := NewHandler(&storeStub{}, strings.Repeat("x", 32), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/audit-events", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRecordEventPersistsValidEvent(t *testing.T) {
	store := &storeStub{}
	token := strings.Repeat("x", 32)
	handler := NewHandler(store, token, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/audit-events", strings.NewReader(
		`{"event_key":"organization:staff.create:1c9491b8-21ce-4c66-a025-006bb346b810","actor_id":"1c9491b8-21ce-4c66-a025-006bb346b810","action":"staff.create.requested","resource_id":"1c9491b8-21ce-4c66-a025-006bb346b810","details":{"staff_id":"ENG-1"}}`,
	))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || store.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, store.calls, response.Body.String())
	}
	if store.event.Action != "staff.create.requested" || store.event.Details["staff_id"] != "ENG-1" {
		t.Fatalf("stored event = %+v", store.event)
	}
	if err := json.Unmarshal(response.Body.Bytes(), new(map[string]string)); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
}

func TestRecordEventRejectsInvalidActor(t *testing.T) {
	store := &storeStub{}
	token := strings.Repeat("x", 32)
	handler := NewHandler(store, token, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/audit-events", strings.NewReader(
		`{"event_key":"event-1","actor_id":"bad","action":"staff.create.requested","resource_id":"1c9491b8-21ce-4c66-a025-006bb346b810","details":{}}`,
	))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || store.calls != 0 {
		t.Fatalf("status=%d calls=%d; want 400 and no persistence", response.Code, store.calls)
	}
}
