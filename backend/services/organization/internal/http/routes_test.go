package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
)

type staffStoreStub struct {
	staff   domain.Staff
	err     error
	created bool
	request domain.CreateStaffRequest
}

func (*staffStoreStub) Ping(context.Context) error { return nil }
func (store *staffStoreStub) Create(_ context.Context, id string, request domain.CreateStaffRequest) (domain.Staff, error) {
	store.created, store.request = true, request
	if store.err != nil {
		return domain.Staff{}, store.err
	}
	staff := store.staff
	staff.ID = id
	staff.StaffID = request.StaffID
	staff.FullName = request.FullName
	staff.WorkEmail = request.WorkEmail
	staff.JobTitle = request.JobTitle
	staff.DepartmentID = request.DepartmentID
	staff.EmploymentStatus = "pending"
	return staff, nil
}

type authenticatorStub struct {
	principal application.Principal
	err       error
	cookie    string
}

func (auth *authenticatorStub) Authenticate(_ context.Context, cookie string) (application.Principal, error) {
	auth.cookie = cookie
	return auth.principal, auth.err
}

type auditStub struct {
	event  application.AuditEvent
	err    error
	called bool
}

func (audit *auditStub) Record(_ context.Context, event application.AuditEvent) error {
	audit.event, audit.called = event, true
	return audit.err
}

func authorizedPrincipal() application.Principal {
	return application.Principal{
		ID:          "1c9491b8-21ce-4c66-a025-006bb346b810",
		Permissions: []string{"staff.create"},
	}
}

func staffRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff", strings.NewReader(body))
	request.Host = "example.test"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.test")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	return request
}

func TestCreateStaffPersistsPendingRecordAndAudits(t *testing.T) {
	if request := staffRequest(`{}`); !sameOrigin(request) {
		t.Fatalf("test request origin is invalid: host=%q origin=%q tls=%v", request.Host, request.Header.Get("Origin"), request.TLS)
	}
	store := &staffStoreStub{}
	auth := &authenticatorStub{principal: authorizedPrincipal()}
	audit := &auditStub{}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := NewHandler(store, auth, audit, logger)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"ENG-104","full_name":"Ada Okafor","work_email":"ada@example.test","job_title":"Site Engineer"}`))

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if !store.created || !audit.called || auth.cookie != "opaque-session" {
		t.Fatalf("created=%v audited=%v authenticated cookie=%q", store.created, audit.called, auth.cookie)
	}
	if store.request.StaffID != "ENG-104" || store.request.FullName != "Ada Okafor" {
		t.Fatalf("persisted request = %+v", store.request)
	}
	if audit.event.ActorID != authorizedPrincipal().ID ||
		audit.event.Action != "staff.create.requested" ||
		audit.event.ResourceID == "" {
		t.Fatalf("audit event = %+v", audit.event)
	}
	if !strings.Contains(response.Body.String(), `"employment_status":"pending"`) {
		t.Fatalf("new staff record should be pending: %s", response.Body.String())
	}
}

func TestCreateStaffRequiresStaffCreatePermission(t *testing.T) {
	store := &staffStoreStub{}
	audit := &auditStub{}
	auth := &authenticatorStub{principal: application.Principal{ID: authorizedPrincipal().ID, Permissions: []string{"staff.read"}}}
	handler := NewHandler(store, auth, audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"ENG-104","full_name":"Ada Okafor"}`))

	if response.Code != http.StatusForbidden || store.created || audit.called {
		t.Fatalf("status=%d created=%v audit=%v; want 403 with no writes", response.Code, store.created, audit.called)
	}
}

func TestCreateStaffRequiresSameOriginAndSession(t *testing.T) {
	tests := []struct {
		name         string
		removeOrigin bool
		removeCookie bool
		wantStatus   int
	}{
		{name: "origin required", removeOrigin: true, wantStatus: http.StatusForbidden},
		{name: "session required", removeCookie: true, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := staffRequest(`{"staff_id":"ENG-104","full_name":"Ada Okafor"}`)
			if test.removeOrigin {
				request.Header.Del("Origin")
			}
			if test.removeCookie {
				request.Header.Del("Cookie")
			}
			response := httptest.NewRecorder()
			NewHandler(&staffStoreStub{}, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
				slog.New(slog.NewJSONHandler(io.Discard, nil))).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestCreateStaffDoesNotWriteIfAuditUnavailable(t *testing.T) {
	store := &staffStoreStub{}
	audit := &auditStub{err: errors.New("audit unavailable")}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"ENG-104","full_name":"Ada Okafor"}`))

	if response.Code != http.StatusServiceUnavailable || store.created {
		t.Fatalf("status=%d created=%v; wanted 503 and no staff write", response.Code, store.created)
	}
}

func TestCreateStaffMapsDuplicateStaffID(t *testing.T) {
	store := &staffStoreStub{err: domain.ErrDuplicateStaffID}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"ENG-104","full_name":"Ada Okafor"}`))

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "duplicate_staff_id") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreateStaffRejectsUnknownJSONFields(t *testing.T) {
	store := &staffStoreStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"ENG-104","full_name":"Ada Okafor","is_admin":true}`))

	if response.Code != http.StatusBadRequest || store.created {
		t.Fatalf("status=%d created=%v; want 400 with no write", response.Code, store.created)
	}
}
