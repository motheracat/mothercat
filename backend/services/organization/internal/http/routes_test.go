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
	completed bool
}

func (*staffStoreStub) Ping(context.Context) error { return nil }
func (store *staffStoreStub) Create(_ context.Context, id string, request domain.CreateStaffRequest) (domain.Staff, error) {
	store.created, store.request = true, request
	if store.err != nil {
		return domain.Staff{}, store.err
	}
	func (store *staffStoreStub) FindByID(_ context.Context, id string) (domain.Staff, error) {
		if store.err != nil {
			return domain.Staff{}, store.err
		}
		staff := store.staff
		if staff.ID == "" {
			staff.ID = id
		}
		if staff.ID != id {
			return domain.Staff{}, domain.ErrStaffNotFound
		}
		return staff, nil
	}
	func (store *staffStoreStub) CompleteProvisioning(_ context.Context, staffID, accountID string) (domain.Staff, error) {
		if store.err != nil {
			return domain.Staff{}, store.err
		}
		store.completed = true
		store.staff.ID = staffID
		store.staff.AccountID = &accountID
		store.staff.EmploymentStatus = "active"
		return store.staff, nil
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
	events []application.AuditEvent
}

func (audit *auditStub) Record(_ context.Context, event application.AuditEvent) error {
	audit.event, audit.called = event, true
	audit.events = append(audit.events, event)
	return audit.err
}

type onboardingIdentityStub struct {
	accountID    string
	provisionErr error
	activationErr error
	provisioned  bool
	activated    bool
}

func (identity *onboardingIdentityStub) ProvisionAccount(_ context.Context, _, _, _ string) (string, error) {
	identity.provisioned = true
	return identity.accountID, identity.provisionErr
}
func (identity *onboardingIdentityStub) SendActivation(_ context.Context, _ string) error {
	identity.activated = true
	return identity.activationErr
}

type staffRoleAssignerStub struct {
	accountID string
	err       error
	called    bool
}

func (access *staffRoleAssignerStub) AssignDefaultStaffRole(_ context.Context, accountID string) error {
	access.accountID = accountID
	access.called = true
	return access.err
}

func authorizedPrincipal() application.Principal {
	return application.Principal{
		ID:          "1c9491b8-21ce-4c66-a025-006bb346b810",
		Permissions: []string{"staff.create", "user_roles.assign"},
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

func provisionRequest(staffID string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff/"+staffID+"/provision", nil)
	request.Host = "example.test"
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

	func TestProvisionStaffCreatesInactiveAccountAssignsStaffRoleAndSendsInvitation(t *testing.T) {
		const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
		const accountID = "1c9491b8-21ce-4c66-a025-006bb346b810"
		email := "staff@example.test"
		store := &staffStoreStub{staff: domain.Staff{
			ID: staffID, StaffID: "ENG-104", FullName: "Ada Okafor",
			WorkEmail: &email, EmploymentStatus: "pending",
		}}
		identity := &onboardingIdentityStub{accountID: accountID}
		access := &staffRoleAssignerStub{}
		audit := &auditStub{}
		services := application.OnboardingServices{Identity: identity, Access: access}
		handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
			slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, provisionRequest(staffID))

		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if !identity.provisioned || !access.called || access.accountID != accountID ||
			!identity.activated || !store.completed {
			t.Fatalf("provisioned=%v assigned=%v activated=%v completed=%v", identity.provisioned, access.called, identity.activated, store.completed)
		}
		if len(audit.events) != 2 || audit.events[0].Action != "staff.provision.requested" ||
			audit.events[1].Action != "staff.provision.completed" {
			t.Fatalf("audit events = %+v", audit.events)
		}
		if !strings.Contains(response.Body.String(), `"employment_status":"active"`) ||
			!strings.Contains(response.Body.String(), `"invitation_sent":true`) {
			t.Fatalf("response does not report completed provisioning: %s", response.Body.String())
		}
	}

	func TestProvisionStaffRequiresRoleAssignmentPermission(t *testing.T) {
		const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
		store := &staffStoreStub{staff: domain.Staff{ID: staffID}}
		identity := &onboardingIdentityStub{}
		access := &staffRoleAssignerStub{}
		services := application.OnboardingServices{Identity: identity, Access: access}
		auth := &authenticatorStub{principal: application.Principal{
			ID: authorizedPrincipal().ID, Permissions: []string{"staff.create"},
		}}
		handler := NewHandler(store, auth, &auditStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, provisionRequest(staffID))

		if response.Code != http.StatusForbidden || identity.provisioned || access.called {
			t.Fatalf("status=%d provisioned=%v roleAssigned=%v", response.Code, identity.provisioned, access.called)
		}
	}

	func TestProvisionStaffRequiresWorkEmail(t *testing.T) {
		const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
		store := &staffStoreStub{staff: domain.Staff{ID: staffID, EmploymentStatus: "pending"}}
		identity := &onboardingIdentityStub{}
		access := &staffRoleAssignerStub{}
		audit := &auditStub{}
		services := application.OnboardingServices{Identity: identity, Access: access}
		handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
			slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, provisionRequest(staffID))

		if response.Code != http.StatusBadRequest || identity.provisioned || audit.called {
			t.Fatalf("status=%d provisioned=%v audit=%v", response.Code, identity.provisioned, audit.called)
		}
	}

	func TestProvisionStaffLeavesRecordPendingWhenEmailIsUnavailable(t *testing.T) {
		const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
		const accountID = "1c9491b8-21ce-4c66-a025-006bb346b810"
		email := "staff@example.test"
		store := &staffStoreStub{staff: domain.Staff{
			ID: staffID, StaffID: "ENG-104", FullName: "Ada Okafor",
			WorkEmail: &email, EmploymentStatus: "pending",
		}}
		identity := &onboardingIdentityStub{accountID: accountID, activationErr: application.ErrEmailDeliveryUnavailable}
		access := &staffRoleAssignerStub{}
		audit := &auditStub{}
		services := application.OnboardingServices{Identity: identity, Access: access}
		handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
			slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, provisionRequest(staffID))

		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "email_delivery_disabled") ||
			store.completed {
			t.Fatalf("status=%d completed=%v body=%s", response.Code, store.completed, response.Body.String())
		}
	}
}
