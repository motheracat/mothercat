package tasks

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

const testAccountID = "00000000-0000-4000-8000-000000000001"

type authenticatorStub struct {
	principal Principal
	err       error
}

func (a authenticatorStub) Authenticate(context.Context, string) (Principal, error) {
	return a.principal, a.err
}

type discardWriter struct{}

func (discardWriter) Write(data []byte) (int, error) { return len(data), nil }

func taskTestHandler(auth Authenticator) http.Handler {
	return NewHandler(nil, auth, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
}

func taskRequest(method, target, body string, authenticated bool) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Origin", "http://example.com")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
	}
	return request
}

func TestTaskEndpointsRequireAuthentication(t *testing.T) {
	for _, route := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/tasks", ""},
		{http.MethodPost, "/api/v1/tasks", `{"title":"Task","department_id":"00000000-0000-4000-8000-000000000002"}`},
		{http.MethodGet, "/api/v1/me/tasks", ""},
		{http.MethodGet, "/api/v1/tasks/00000000-0000-4000-8000-000000000002/history", ""},
		{http.MethodPost, "/api/v1/tasks/00000000-0000-4000-8000-000000000002/submit", `{"summary":"done"}`},
	} {
		response := httptest.NewRecorder()
		taskTestHandler(authenticatorStub{}).ServeHTTP(
			response, taskRequest(route.method, route.path, route.body, false),
		)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status=%d, want %d", route.method, route.path, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestTaskWritesRequireExplicitPermission(t *testing.T) {
	auth := authenticatorStub{principal: Principal{ID: testAccountID}}
	response := httptest.NewRecorder()
	taskTestHandler(auth).ServeHTTP(response, taskRequest(
		http.MethodPost, "/api/v1/tasks",
		`{"title":"Task","department_id":"00000000-0000-4000-8000-000000000002"}`, true,
	))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusForbidden, response.Body)
	}
}

func TestGlobalTaskReaderCanCreateOutsideDepartmentMembership(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const taskID = "00000000-0000-4000-8000-000000000003"
	const departmentID = "00000000-0000-4000-8000-000000000002"
	auth := authenticatorStub{principal: Principal{
		ID: testAccountID, Permissions: []string{"tasks.create", "tasks.read_all"},
	}}
	handler := NewHandler(db, auth, slog.New(slog.NewTextHandler(discardWriter{}, nil)))

	createdAt := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO tasks\.tasks`).
		WithArgs("Task", "", "normal", testAccountID, departmentID, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "description", "priority", "status", "created_by", "department_id",
			"assignee_account_id", "due_at", "version", "created_at", "updated_at",
			"submission_id", "submission_number", "submission_summary", "links",
			"submitter_account_id", "submitted_at",
		}).AddRow(taskID, "Task", "", "normal", "draft", testAccountID, departmentID,
			nil, nil, 1, createdAt, createdAt, nil, nil, nil, nil, nil, nil))
	mock.ExpectExec(`INSERT INTO tasks\.task_status_history`).
		WithArgs(taskID, testAccountID, nil, "draft", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit\.audit_events`).
		WithArgs(testAccountID, "task.created", taskID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, taskRequest(
		http.MethodPost, "/api/v1/tasks",
		`{"title":"Task","department_id":"`+departmentID+`"}`, true,
	))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskInputValidationPrecedesDatabaseAccess(t *testing.T) {
	auth := authenticatorStub{principal: Principal{ID: testAccountID, Permissions: []string{"tasks.create"}}}
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/tasks", `{"title":"","department_id":"invalid"}`},
		{http.MethodPost, "/api/v1/tasks/00000000-0000-4000-8000-000000000002/submit", `{"summary":"","links":[]}`},
		{http.MethodGet, "/api/v1/tasks?status=unknown", ""},
		{http.MethodGet, "/api/v1/tasks/invalid", ""},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		taskTestHandler(auth).ServeHTTP(response, taskRequest(test.method, test.path, test.body, true))
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d, want %d: %s",
				test.method, test.path, response.Code, http.StatusBadRequest, response.Body)
		}
	}
}

func TestTaskCreateRejectsMissingJSONContentType(t *testing.T) {
	auth := authenticatorStub{principal: Principal{
		ID: testAccountID, Permissions: []string{"tasks.create"},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks",
		strings.NewReader(`{"title":"Task","department_id":"00000000-0000-4000-8000-000000000002"}`))
	request.Header.Set("Origin", "http://example.com")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	taskTestHandler(auth).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusBadRequest, response.Body)
	}
}

func TestTaskWritesRequireSameOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	taskTestHandler(authenticatorStub{}).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusForbidden, response.Body)
	}
}

func TestParseETag(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
		ok    bool
	}{
		{`"1"`, 1, true},
		{`"17"`, 17, true},
		{"1", 0, false},
		{`"0"`, 0, false},
		{`W/"1"`, 0, false},
		{"*", 0, false},
	} {
		version, ok := parseETag(test.value)
		if version != test.want || ok != test.ok {
			t.Errorf("parseETag(%q)=(%d,%t), want (%d,%t)",
				test.value, version, ok, test.want, test.ok)
		}
	}
}

func TestEnqueueTaskEventUsesCallingTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`INSERT INTO tasks\.outbox_events`).
		WithArgs("task.assigned", "00000000-0000-4000-8000-000000000002",
			testAccountID, "00000000-0000-4000-8000-000000000003",
			"Task assigned", "You have been assigned a task.").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := enqueueTaskEvent(context.Background(), tx, testAccountID,
		"00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003",
		"task.assigned", "Task assigned",
		"You have been assigned a task."); err != nil {
		t.Fatalf("enqueueTaskEvent() error = %v", err)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
