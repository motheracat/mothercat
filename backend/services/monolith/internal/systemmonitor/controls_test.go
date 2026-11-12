package systemmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const testMonitorActorID = "00000000-0000-4000-8000-000000000001"

func TestPauseEndpointPersistsPauseAndHistoryAtomically(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired AS .*INSERT INTO system_monitor\.endpoint_control_history`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_controls`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", testMonitorActorID,
			"approved incident INC-123", 503, "Temporarily unavailable", expiresAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_control_history`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", testMonitorActorID,
			"approved incident INC-123", 503, "Temporarily unavailable", expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	engine := NewEngine()
	err = engine.PauseEndpoint(context.Background(), db, PauseRequest{
		Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}", ActorID: testMonitorActorID,
		Reason: "approved incident INC-123", ResponseStatus: 503,
		ResponseMessage: "Temporarily unavailable", ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("PauseEndpoint() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPauseEndpointRejectsProtectedOrUnknownRoutes(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := NewEngine()
	for _, target := range []Endpoint{
		{Method: http.MethodPost, Route: "/api/v1/auth/login"},
		{Method: http.MethodGet, Route: "/api/v1/system-monitor/overview"},
		{Method: http.MethodDelete, Route: "/api/v1/tasks/{taskID}"},
	} {
		err := engine.PauseEndpoint(context.Background(), db, PauseRequest{
			Method: target.Method, Route: target.Route, ActorID: testMonitorActorID,
			Reason: "approved incident INC-123", ExpiresAt: time.Now().Add(time.Hour),
		})
		if !errors.Is(err, ErrEndpointNotControllable) {
			t.Errorf("PauseEndpoint(%s %s) error = %v, want not controllable",
				target.Method, target.Route, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeEndpointRemovesPauseAndRecordsHistory(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expiresAt := time.Now().Add(time.Hour).UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)DELETE FROM system_monitor\.endpoint_controls.*RETURNING`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}").
		WillReturnRows(sqlmock.NewRows([]string{"response_status", "response_message", "expires_at"}).
			AddRow(503, "Temporarily unavailable", expiresAt))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_control_history`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", testMonitorActorID,
			"incident resolved", 503, "Temporarily unavailable", expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	engine := NewEngine()
	err = engine.ResumeEndpoint(context.Background(), db, ResumeRequest{
		Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}",
		ActorID: testMonitorActorID, Reason: "incident resolved",
	})
	if err != nil {
		t.Fatalf("ResumeEndpoint() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeEndpointReportsWhenNotPaused(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)DELETE FROM system_monitor\.endpoint_controls.*RETURNING`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}").
		WillReturnRows(sqlmock.NewRows([]string{"response_status", "response_message", "expires_at"}))
	mock.ExpectRollback()

	err = NewEngine().ResumeEndpoint(context.Background(), db, ResumeRequest{
		Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}",
		ActorID: testMonitorActorID, Reason: "incident resolved",
	})
	if !errors.Is(err, ErrEndpointNotPaused) {
		t.Fatalf("ResumeEndpoint() error = %v, want ErrEndpointNotPaused", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExpireEndpointControlsWritesExpirationHistory(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(`(?s)WITH expired AS .*INSERT INTO system_monitor\.endpoint_control_history`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	count, err := ExpireEndpointControls(context.Background(), db)
	if err != nil {
		t.Fatalf("ExpireEndpointControls() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expired count = %d, want 2", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMiddlewareEnforcesPersistedPauseServerSide(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expiresAt := time.Now().Add(time.Hour).UTC()
	mock.ExpectQuery(`(?s)SELECT method, route, response_status, response_message, expires_at.*endpoint_controls`).
		WillReturnRows(sqlmock.NewRows([]string{
			"method", "route", "response_status", "response_message", "expires_at",
		}).AddRow(http.MethodGet, "/api/v1/tasks/{taskID}", 503, "Temporarily unavailable", expiresAt))
	engine := NewEngine()
	engine.SetControlStore(db, nil)
	nextCalled := false
	handler := engine.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/tasks/00000000-0000-4000-8000-000000000099", nil))
	if nextCalled {
		t.Fatal("paused endpoint handler was called")
	}
	if response.Code != 503 || !strings.Contains(response.Body.String(), "Temporarily unavailable") {
		t.Fatalf("paused response = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("paused response is missing Retry-After header")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMiddlewareFailsClosedWhenControlStateCannotBeRead(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)SELECT method, route, response_status, response_message, expires_at.*endpoint_controls`).
		WillReturnError(errors.New("control storage unavailable"))
	engine := NewEngine()
	engine.SetControlStore(db, nil)
	nextCalled := false
	handler := engine.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/tasks/00000000-0000-4000-8000-000000000099", nil))
	if nextCalled {
		t.Fatal("application handler was called while endpoint-control state was unavailable")
	}
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want service unavailable", response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPauseHandlerRequiresIndependentMonitorGrantAndRecordsActor(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*system_monitor\.access_grants`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired AS .*INSERT INTO system_monitor\.endpoint_control_history`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_controls`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", testMonitorActorID,
			"approved incident INC-456", 503, "Temporarily unavailable", expiresAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_control_history`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", testMonitorActorID,
			"approved incident INC-456", 503, "Temporarily unavailable", expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	payload, err := json.Marshal(pauseEndpointPayload{
		Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}", Reason: "approved incident INC-456",
		ResponseStatus: 503, ResponseMessage: "Temporarily unavailable", ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db, NewEngine(), testAuthenticator{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system-monitor/endpoints/pause",
		strings.NewReader(string(payload)))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-System-Monitor-Action", "confirm")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want OK; response: %s", response.Code, response.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPauseHandlerRequiresExplicitActionHeader(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*system_monitor\.access_grants`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	handler := NewHandler(db, NewEngine(), testAuthenticator{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system-monitor/endpoints/pause",
		strings.NewReader(`{}`))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want unsupported media type", response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
