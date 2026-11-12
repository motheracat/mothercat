package systemmonitor

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEndpointCatalogResolvesMethodAndRouteTemplate(t *testing.T) {
	catalog := newEndpointCatalog()
	endpoint, ok := catalog.Resolve(http.MethodGet, "/api/v1/tasks/00000000-0000-4000-8000-000000000001")
	if !ok {
		t.Fatal("Resolve() did not find task endpoint")
	}
	if endpoint.Route != "/api/v1/tasks/{taskID}" || endpoint.Method != http.MethodGet {
		t.Fatalf("Resolve() = %#v, want GET /api/v1/tasks/{taskID}", endpoint)
	}
	if _, ok := catalog.Resolve(http.MethodDelete, "/api/v1/tasks/00000000-0000-4000-8000-000000000001"); ok {
		t.Fatal("Resolve() unexpectedly found an unsupported method")
	}
}

func TestMiddlewareRecordsTemplateOnlyAndEndpointHealth(t *testing.T) {
	engine := NewEngine()
	handler := engine.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/00000000-0000-4000-8000-000000000001?secret=ignored", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	events := engine.Events(10)
	if len(events) != 1 {
		t.Fatalf("Events() returned %d events, want 1", len(events))
	}
	if events[0].Route != "/api/v1/tasks/{taskID}" {
		t.Fatalf("event route = %q, want route template", events[0].Route)
	}
	if strings.Contains(events[0].Route, "00000000") || strings.Contains(events[0].Route, "secret") {
		t.Fatalf("event contains request-specific data: %#v", events[0])
	}
	if events[0].Type != "server_error" || events[0].Status != http.StatusInternalServerError {
		t.Fatalf("unexpected event: %#v", events[0])
	}

	var taskHealth *EndpointHealth
	for _, endpoint := range engine.Endpoints() {
		if endpoint.Route == "/api/v1/tasks/{taskID}" && endpoint.Method == http.MethodGet {
			endpointCopy := endpoint
			taskHealth = &endpointCopy
			break
		}
	}
	if taskHealth == nil {
		t.Fatal("task endpoint missing from catalog")
	}
	if taskHealth.Requests != 1 || taskHealth.FailedRequests != 1 ||
		taskHealth.StatusDistribution[http.StatusInternalServerError] != 1 {
		t.Fatalf("unexpected endpoint metrics: %#v", taskHealth)
	}
	var latencyCount uint64
	for _, count := range taskHealth.LatencyBuckets {
		latencyCount += count
	}
	if latencyCount != 1 {
		t.Fatalf("request missing from latency histogram: %#v", taskHealth.LatencyBuckets)
	}
}

func TestAuthenticationFailureBurstEvent(t *testing.T) {
	engine := NewEngine()
	for index := 0; index < 10; index++ {
		engine.record(Endpoint{
			Method: http.MethodPost, Route: "/api/v1/auth/login",
		}, http.StatusUnauthorized, time.Millisecond, time.Now().UTC())
	}
	events := engine.Events(20)
	if len(events) != 11 {
		t.Fatalf("Events() returned %d events, want 10 attempts and one burst alert", len(events))
	}
	last := events[len(events)-1]
	if last.Type != "authentication_failure_burst" || last.Route != "/api/v1/auth/login" {
		t.Fatalf("last event = %#v, want authentication failure burst", last)
	}
}

func TestEventsRetainOnlyBoundedRecentItems(t *testing.T) {
	engine := NewEngine()
	endpoint := Endpoint{Method: http.MethodGet, Route: "/health/live"}
	for index := 0; index < eventCapacity+10; index++ {
		engine.record(endpoint, http.StatusOK, time.Millisecond, time.Now().UTC())
	}
	events := engine.Events(eventCapacity + 1)
	if len(events) != eventCapacity {
		t.Fatalf("Events() returned %d items, want capacity %d", len(events), eventCapacity)
	}
	if events[0].OccurredAt.After(events[len(events)-1].OccurredAt) {
		t.Fatal("events are not returned in chronological order")
	}
}

func TestOverviewReportsDatabaseNotReadyWhenPingFails(t *testing.T) {
	engine := NewEngine()
	overview := engine.Overview(context.Background(), nil)
	if overview.DatabaseStatus != "unavailable" {
		t.Fatalf("DatabaseStatus = %q, want unavailable", overview.DatabaseStatus)
	}
	if overview.EndpointCount == 0 || overview.UptimeSeconds < 0 {
		t.Fatalf("unexpected overview: %#v", overview)
	}
}

func TestHandlerDeniesMissingSession(t *testing.T) {
	handler := NewHandler(nil, NewEngine(), nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system-monitor/endpoints", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want unauthorized", response.Code)
	}
}

func TestHandlerFailsClosedWhenIdentityUnavailable(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	authenticator := testAuthenticator{err: errors.New("identity database unavailable")}
	handler := NewHandler(db, NewEngine(), authenticator, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system-monitor/endpoints", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want service unavailable", response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerDeniesUnprivilegedIdentityAndCancelsAuthorizationContext(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*system_monitor\.access_grants`).
		WithArgs("00000000-0000-4000-8000-000000000001").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	var authContext context.Context
	handler := NewHandler(db, NewEngine(), testAuthenticator{capturedContext: &authContext}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system-monitor/endpoints", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want forbidden", response.Code)
	}
	if authContext == nil {
		t.Fatal("authenticator context was not captured")
	}
	select {
	case <-authContext.Done():
	default:
		t.Fatal("authorization context was not canceled after the response")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type testAuthenticator struct {
	err             error
	capturedContext *context.Context
}

func (a testAuthenticator) Authenticate(ctx context.Context, _ string) (string, error) {
	if a.capturedContext != nil {
		*a.capturedContext = ctx
	}
	if a.err != nil {
		return "", a.err
	}
	return "00000000-0000-4000-8000-000000000001", nil
}

func newMockDB() (*sql.DB, sqlmock.Sqlmock, error) {
	return sqlmock.New()
}
