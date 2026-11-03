package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
)

type staffStoreStub struct {
	staff             domain.Staff
	err               error
	created           bool
	request           domain.CreateStaffRequest
	completed         bool
	listed            bool
	list              []domain.Staff
	filter            domain.StaffListFilter
	updated           bool
	update            domain.UpdateStaffRequest
	profileImage      string
	deactivated       bool
	departments       []domain.Department
	departmentRequest domain.DepartmentRequest
	departmentCreated bool
	departmentUpdated bool
	imported          bool
	importRows        []domain.ImportStaffRow
	importBatch       domain.StaffImportBatch
	failedImport      domain.StaffImportResult
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

func (store *staffStoreStub) UpdateProfileImage(_ context.Context, staffID, imageURL string) (domain.Staff, error) {
	if store.err != nil {
		return domain.Staff{}, store.err
	}
	store.profileImage = imageURL
	store.staff.ID = staffID
	store.staff.ProfileImageURL = &imageURL
	return store.staff, nil
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

func (store *staffStoreStub) ListStaff(_ context.Context, filter domain.StaffListFilter) ([]domain.Staff, int, error) {
	store.listed, store.filter = true, filter
	if store.err != nil {
		return nil, 0, store.err
	}
	return store.list, len(store.list), nil
}

func (store *staffStoreStub) UpdateStaff(_ context.Context, staffID string, request domain.UpdateStaffRequest) (domain.Staff, error) {
	store.updated, store.update = true, request
	if store.err != nil {
		return domain.Staff{}, store.err
	}
	store.staff.ID = staffID
	if request.FullName.Set && request.FullName.Value != nil {
		store.staff.FullName = *request.FullName.Value
	}
	if request.WorkEmail.Set {
		store.staff.WorkEmail = request.WorkEmail.Value
	}
	if request.JobTitle.Set {
		store.staff.JobTitle = request.JobTitle.Value
	}
	if request.DepartmentID.Set {
		store.staff.DepartmentID = request.DepartmentID.Value
	}
	return store.staff, nil
}

func (store *staffStoreStub) DeactivateStaff(_ context.Context, staffID string) (domain.Staff, error) {
	if store.err != nil {
		return domain.Staff{}, store.err
	}
	store.deactivated = true
	store.staff.ID = staffID
	store.staff.EmploymentStatus = "inactive"
	return store.staff, nil
}

func (store *staffStoreStub) ListDepartments(context.Context) ([]domain.Department, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.departments, nil
}

func (store *staffStoreStub) CreateDepartment(_ context.Context, id string, request domain.DepartmentRequest) (domain.Department, error) {
	if store.err != nil {
		return domain.Department{}, store.err
	}
	store.departmentCreated, store.departmentRequest = true, request
	name := ""
	if request.Name.Value != nil {
		name = *request.Name.Value
	}
	return domain.Department{ID: id, Name: name, Description: request.Description.Value}, nil
}

func (store *staffStoreStub) UpdateDepartment(_ context.Context, id string, request domain.DepartmentRequest) (domain.Department, error) {
	if store.err != nil {
		return domain.Department{}, store.err
	}
	store.departmentUpdated, store.departmentRequest = true, request
	return domain.Department{ID: id}, nil
}

func (store *staffStoreStub) ImportStaffBatch(_ context.Context, id, _ string, rows []domain.ImportStaffRow) (domain.StaffImportBatch, error) {
	if store.err != nil {
		return domain.StaffImportBatch{}, store.err
	}
	store.imported, store.importRows = true, rows
	result := domain.StaffImportResult{TotalRows: len(rows), ImportedRows: len(rows)}
	return domain.StaffImportBatch{ID: id, Status: "completed", Result: result}, nil
}

func (store *staffStoreStub) RecordFailedStaffImport(_ context.Context, id, _ string, result domain.StaffImportResult) (domain.StaffImportBatch, error) {
	if store.err != nil {
		return domain.StaffImportBatch{}, store.err
	}
	store.failedImport = result
	return domain.StaffImportBatch{ID: id, Status: "failed", Result: result}, nil
}

func (store *staffStoreStub) FindStaffImportBatch(_ context.Context, id string) (domain.StaffImportBatch, error) {
	if store.err != nil {
		return domain.StaffImportBatch{}, store.err
	}
	if store.importBatch.ID == "" || store.importBatch.ID != id {
		return domain.StaffImportBatch{}, domain.ErrImportNotFound
	}
	return store.importBatch, nil
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
	accountID     string
	provisionErr  error
	activationErr error
	provisioned   bool
	activated     bool
	suspended     bool
	suspendErr    error
}

func (identity *onboardingIdentityStub) ProvisionAccount(_ context.Context, _, _, _ string) (string, error) {
	identity.provisioned = true
	return identity.accountID, identity.provisionErr
}
func (identity *onboardingIdentityStub) SendActivation(_ context.Context, _ string) error {
	identity.activated = true
	return identity.activationErr
}
func (identity *onboardingIdentityStub) SuspendAccount(_ context.Context, _, _ string) error {
	identity.suspended = true
	return identity.suspendErr
}

type staffRoleAssignerStub struct {
	accountID string
	roleKey   string
	err       error
	called    bool
}

func (access *staffRoleAssignerStub) AssignRole(_ context.Context, accountID, roleKey string) error {
	access.accountID, access.roleKey = accountID, roleKey
	access.called = true
	return access.err
}

func authorizedPrincipal() application.Principal {
	return application.Principal{
		ID: "1c9491b8-21ce-4c66-a025-006bb346b810",
		Permissions: []string{
			"staff.read", "staff.create", "staff.update", "staff.deactivate",
			"staff.import", "departments.manage", "user_roles.assign",
		},
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

func authorizedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = "example.test"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.test")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	return request
}

func staffImportRequest(t *testing.T, csvText string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "staff.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, csvText); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff/imports", &body)
	request.Host = "example.test"
	request.Header.Set("Origin", "http://example.test")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	return request
}

type profileImageUploaderStub struct {
	contentType string
	image       []byte
	imageURL    string
	err         error
}

func (uploader *profileImageUploaderStub) Upload(_ context.Context, contentType string, image []byte) (string, error) {
	uploader.contentType, uploader.image = contentType, image
	return uploader.imageURL, uploader.err
}

func profileImageRequest(t *testing.T, pngBytes []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "profile.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(pngBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/staff/1c9491b8-21ce-4c66-a025-006bb346b810/profile-picture", &body)
	request.Host = "example.test"
	request.Header.Set("Origin", "http://example.test")
	request.Header.Set("Content-Type", writer.FormDataContentType())
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
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"23847","full_name":"Ada Okafor","work_email":"ada@example.test","job_title":"Site Engineer"}`))

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if !store.created || !audit.called || auth.cookie != "opaque-session" {
		t.Fatalf("created=%v audited=%v authenticated cookie=%q", store.created, audit.called, auth.cookie)
	}
	if store.request.StaffID != "23847" || store.request.FullName != "Ada Okafor" {
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
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"23847","full_name":"Ada Okafor"}`))

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
			request := staffRequest(`{"staff_id":"23847","full_name":"Ada Okafor"}`)
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
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"23847","full_name":"Ada Okafor"}`))

	if response.Code != http.StatusServiceUnavailable || store.created {
		t.Fatalf("status=%d created=%v; wanted 503 and no staff write", response.Code, store.created)
	}
}

func TestCreateStaffMapsDuplicateStaffID(t *testing.T) {
	store := &staffStoreStub{err: domain.ErrDuplicateStaffID}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"23847","full_name":"Ada Okafor"}`))

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "duplicate_staff_id") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreateStaffRejectsUnknownJSONFields(t *testing.T) {
	store := &staffStoreStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffRequest(`{"staff_id":"23847","full_name":"Ada Okafor","is_admin":true}`))

	if response.Code != http.StatusBadRequest || store.created {
		t.Fatalf("status=%d created=%v; want 400 with no write", response.Code, store.created)
	}
}

func TestCreateStaffRejectsInvalidStaffNumber(t *testing.T) {
	for _, staffID := range []string{"ID:23847", "123", "123456", "23A47"} {
		store := &staffStoreStub{}
		audit := &auditStub{}
		handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
			slog.New(slog.NewJSONHandler(io.Discard, nil)))
		response := httptest.NewRecorder()
		body := `{"staff_id":"` + staffID + `","full_name":"Ada Okafor"}`
		handler.ServeHTTP(response, staffRequest(body))
		if response.Code != http.StatusBadRequest || store.created || audit.called {
			t.Errorf("staff_id %q: status=%d created=%v audit=%v; want 400 with no writes",
				staffID, response.Code, store.created, audit.called)
		}
	}
}

func TestProvisionStaffCreatesInactiveAccountAssignsStaffRoleAndSendsInvitation(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	const accountID = "1c9491b8-21ce-4c66-a025-006bb346b810"
	email := "staff@example.test"
	store := &staffStoreStub{staff: domain.Staff{
		ID: staffID, StaffID: "23847", FullName: "Ada Okafor",
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
		access.roleKey != "staff" || !identity.activated || !store.completed {
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

func TestProvisionAdminCreatesAccountWithAdminRoleAndSendsInvitation(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	const accountID = "1c9491b8-21ce-4c66-a025-006bb346b810"
	email := "admin@example.test"
	store := &staffStoreStub{staff: domain.Staff{
		ID: staffID, StaffID: "23847", FullName: "Ada Okafor",
		WorkEmail: &email, EmploymentStatus: "pending",
	}}
	identity := &onboardingIdentityStub{accountID: accountID}
	access := &staffRoleAssignerStub{}
	audit := &auditStub{}
	services := application.OnboardingServices{Identity: identity, Access: access}
	principal := authorizedPrincipal()
	principal.Permissions = append(principal.Permissions, "roles.manage")
	handler := NewHandler(store, &authenticatorStub{principal: principal}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff/"+staffID+"/provision-admin", nil)
	request.Host = "example.test"
	request.Header.Set("Origin", "http://example.test")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !identity.provisioned || !access.called || access.accountID != accountID ||
		access.roleKey != "admin" || !identity.activated || !store.completed {
		t.Fatalf("provisioned=%v assigned=%v role=%q activated=%v completed=%v",
			identity.provisioned, access.called, access.roleKey, identity.activated, store.completed)
	}
	if !strings.Contains(response.Body.String(), `"role":"admin"`) ||
		!strings.Contains(response.Body.String(), `"invitation_sent":true`) {
		t.Fatalf("response does not report Admin provisioning: %s", response.Body.String())
	}
	if len(audit.events) != 2 || audit.events[0].Details["role"] != "admin" {
		t.Fatalf("audit events = %+v", audit.events)
	}
}

func TestProvisionAdminRequiresRoleManagementPermission(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	store := &staffStoreStub{staff: domain.Staff{ID: staffID}}
	identity := &onboardingIdentityStub{}
	access := &staffRoleAssignerStub{}
	services := application.OnboardingServices{Identity: identity, Access: access}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()},
		&auditStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff/"+staffID+"/provision-admin", nil)
	request.Host = "example.test"
	request.Header.Set("Origin", "http://example.test")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || identity.provisioned || access.called {
		t.Fatalf("status=%d provisioned=%v roleAssigned=%v", response.Code, identity.provisioned, access.called)
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
		ID: staffID, StaffID: "23847", FullName: "Ada Okafor",
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

func TestListStaffUsesBoundedPaginationAndPermission(t *testing.T) {
	staffID := "7da27cda-3496-4d02-9d99-fb0e956297e5"
	store := &staffStoreStub{list: []domain.Staff{{ID: staffID, StaffID: "23847"}}}
	auth := &authenticatorStub{principal: authorizedPrincipal()}
	handler := NewHandler(store, auth, &auditStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := authorizedRequest(http.MethodGet, "/api/v1/staff?limit=25&offset=10&search=ada&employment_status=pending", "")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !store.listed {
		t.Fatalf("status=%d listed=%v body=%s", response.Code, store.listed, response.Body.String())
	}
	if store.filter.Limit != 25 || store.filter.Offset != 10 ||
		store.filter.Search != "ada" || store.filter.EmploymentStatus != "pending" {
		t.Fatalf("list filter = %+v", store.filter)
	}
	if !strings.Contains(response.Body.String(), `"total":1`) {
		t.Fatalf("pagination total missing: %s", response.Body.String())
	}

	auth.principal.Permissions = []string{"staff.create"}
	store.listed = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/staff", ""))
	if response.Code != http.StatusForbidden || store.listed {
		t.Fatalf("unauthorized list status=%d listed=%v", response.Code, store.listed)
	}
}

func TestListStaffRejectsInvalidQuery(t *testing.T) {
	store := &staffStoreStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/staff?limit=10000", ""))
	if response.Code != http.StatusBadRequest || store.listed {
		t.Fatalf("status=%d listed=%v body=%s", response.Code, store.listed, response.Body.String())
	}
}

func TestGetStaffRequiresUUIDAndReturnsRecord(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	store := &staffStoreStub{staff: domain.Staff{ID: staffID, StaffID: "23847"}}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/staff/"+staffID, ""))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"staff_id":"23847"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/staff/not-a-uuid", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid identifier status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUploadProfilePictureValidatesAndPersistsCloudinaryURL(t *testing.T) {
	var encoded bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.White)
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	accountID := authorizedPrincipal().ID
	store := &staffStoreStub{staff: domain.Staff{AccountID: &accountID}}
	audit := &auditStub{}
	uploader := &profileImageUploaderStub{imageURL: "https://res.cloudinary.com/demo/image/upload/profile.png"}
	auth := &authenticatorStub{principal: application.Principal{ID: accountID, Permissions: []string{"staff.read"}}}
	handler := NewHandler(store, auth, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		application.OnboardingServices{ProfileImageUploader: uploader})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, profileImageRequest(t, encoded.Bytes()))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if uploader.contentType != "image/png" || len(uploader.image) == 0 ||
		store.profileImage != uploader.imageURL || !audit.called {
		t.Fatalf("upload=%+v saved_url=%q audited=%v", uploader, store.profileImage, audit.called)
	}
	if !strings.Contains(response.Body.String(), `"profile_image_url":"https://res.cloudinary.com/demo/image/upload/profile.png"`) {
		t.Fatalf("response does not return the saved Cloudinary URL: %s", response.Body.String())
	}

	otherStaffAccountID := "9c9491b8-21ce-4c66-a025-006bb346b810"
	otherStore := &staffStoreStub{staff: domain.Staff{AccountID: &otherStaffAccountID}}
	otherUploader := &profileImageUploaderStub{imageURL: uploader.imageURL}
	otherHandler := NewHandler(otherStore, auth, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		application.OnboardingServices{ProfileImageUploader: otherUploader})
	response = httptest.NewRecorder()
	otherHandler.ServeHTTP(response, profileImageRequest(t, encoded.Bytes()))
	if response.Code != http.StatusForbidden || len(otherUploader.image) != 0 {
		t.Fatalf("another user's profile upload status=%d uploaded=%v", response.Code, len(otherUploader.image) > 0)
	}
}

func TestUpdateStaffSupportsExplicitNullAndAuditsFieldNamesOnly(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	store := &staffStoreStub{staff: domain.Staff{StaffID: "23847"}}
	audit := &auditStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(
		http.MethodPatch, "/api/v1/staff/"+staffID,
		`{"full_name":"Updated Name","work_email":null,"job_title":null}`,
	))
	if response.Code != http.StatusOK || !store.updated || !audit.called {
		t.Fatalf("status=%d updated=%v audited=%v body=%s", response.Code, store.updated, audit.called, response.Body.String())
	}
	if !store.update.FullName.Set || store.update.FullName.Value == nil ||
		*store.update.FullName.Value != "Updated Name" ||
		!store.update.WorkEmail.Set || store.update.WorkEmail.Value != nil {
		t.Fatalf("update request = %+v", store.update)
	}
	if strings.Contains(response.Body.String(), "work_email") && !strings.Contains(response.Body.String(), `"work_email":null`) {
		t.Fatalf("cleared work email should be null: %s", response.Body.String())
	}
	if strings.Contains(audit.event.Details["fields"].([]string)[0], "Updated Name") {
		t.Fatalf("audit details must not contain submitted values: %+v", audit.event)
	}
}

func TestStaffCanUpdateOwnPersonalProfileWithoutStaffUpdatePermission(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	accountID := "1c9491b8-21ce-4c66-a025-006bb346b810"
	store := &staffStoreStub{staff: domain.Staff{AccountID: &accountID}}
	audit := &auditStub{}
	auth := &authenticatorStub{principal: application.Principal{ID: accountID}}
	handler := NewHandler(store, auth, audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(
		http.MethodPatch, "/api/v1/staff/"+staffID,
		`{"full_name":"Updated Name","work_email":"updated@example.test"}`,
	))

	if response.Code != http.StatusOK || !store.updated || !audit.called {
		t.Fatalf("status=%d updated=%v audited=%v body=%s",
			response.Code, store.updated, audit.called, response.Body.String())
	}
	if store.update.JobTitle.Set || store.update.DepartmentID.Set {
		t.Fatalf("self-service update unexpectedly included administrative fields: %+v", store.update)
	}
}

func TestStaffCannotUpdateAnotherProfileOrAdministrativeFields(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	otherAccountID := "9c9491b8-21ce-4c66-a025-006bb346b810"
	auth := &authenticatorStub{principal: application.Principal{
		ID: "1c9491b8-21ce-4c66-a025-006bb346b810",
	}}
	audit := &auditStub{}
	store := &staffStoreStub{staff: domain.Staff{AccountID: &otherAccountID}}
	handler := NewHandler(store, auth, audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(
		http.MethodPatch, "/api/v1/staff/"+staffID, `{"full_name":"Changed Name"}`,
	))
	if response.Code != http.StatusForbidden || store.updated || audit.called {
		t.Fatalf("cross-profile status=%d updated=%v audited=%v",
			response.Code, store.updated, audit.called)
	}

	store = &staffStoreStub{staff: domain.Staff{AccountID: &auth.principal.ID}}
	handler = NewHandler(store, auth, audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(
		http.MethodPatch, "/api/v1/staff/"+staffID, `{"job_title":"Director"}`,
	))
	if response.Code != http.StatusForbidden || store.updated || audit.called {
		t.Fatalf("self administrative-field status=%d updated=%v audited=%v",
			response.Code, store.updated, audit.called)
	}
}

func TestUpdateStaffRejectsCrossOriginAndUnknownFields(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	store := &staffStoreStub{}
	audit := &auditStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	request := authorizedRequest(http.MethodPatch, "/api/v1/staff/"+staffID, `{"full_name":"Name"}`)
	request.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || store.updated || audit.called {
		t.Fatalf("cross-origin status=%d updated=%v audited=%v", response.Code, store.updated, audit.called)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(
		http.MethodPatch, "/api/v1/staff/"+staffID, `{"full_name":"Name","employment_status":"active"}`,
	))
	if response.Code != http.StatusBadRequest || store.updated {
		t.Fatalf("unknown-field status=%d updated=%v", response.Code, store.updated)
	}
}

func TestDeactivateStaffSuspendsLoginBeforeMarkingEmploymentInactive(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	accountID := "1c9491b8-21ce-4c66-a025-006bb346b810"
	store := &staffStoreStub{staff: domain.Staff{
		ID: staffID, StaffID: "23847", AccountID: &accountID, EmploymentStatus: "active",
	}}
	identity := &onboardingIdentityStub{}
	audit := &auditStub{}
	services := application.OnboardingServices{IdentityState: identity}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "/api/v1/staff/"+staffID+"/deactivate", ""))
	if response.Code != http.StatusOK || !identity.suspended || !store.deactivated || !audit.called {
		t.Fatalf("status=%d suspended=%v deactivated=%v audited=%v body=%s",
			response.Code, identity.suspended, store.deactivated, audit.called, response.Body.String())
	}
}

func TestDeactivateStaffDoesNotChangeEmploymentIfAccountSuspensionFails(t *testing.T) {
	const staffID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	accountID := "1c9491b8-21ce-4c66-a025-006bb346b810"
	store := &staffStoreStub{staff: domain.Staff{
		ID: staffID, StaffID: "23847", AccountID: &accountID, EmploymentStatus: "active",
	}}
	identity := &onboardingIdentityStub{suspendErr: errors.New("identity database unavailable")}
	services := application.OnboardingServices{IdentityState: identity}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)), services)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "/api/v1/staff/"+staffID+"/deactivate", ""))
	if response.Code != http.StatusServiceUnavailable || !identity.suspended || store.deactivated {
		t.Fatalf("status=%d suspended=%v deactivated=%v body=%s",
			response.Code, identity.suspended, store.deactivated, response.Body.String())
	}
}

func TestStaffImportIsBoundedAuditedAndReturnsBatch(t *testing.T) {
	store := &staffStoreStub{}
	audit := &auditStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffImportRequest(t,
		"staff_id,full_name,work_email,job_title\n23847,Ada Okafor,ada@example.test,Engineer\n"))
	if response.Code != http.StatusCreated || !store.imported || !audit.called ||
		len(store.importRows) != 1 || store.importRows[0].Staff.StaffID != "23847" {
		t.Fatalf("status=%d imported=%v audited=%v rows=%+v body=%s",
			response.Code, store.imported, audit.called, store.importRows, response.Body.String())
	}
}

func TestStaffImportAllowsMultipartEnvelopeAroundMaximumFileSize(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "staff.csv")
	if err != nil {
		t.Fatal(err)
	}
	content := "staff_id,full_name\n23847," + strings.Repeat("A", maxImportFileSize-len("staff_id,full_name\n23847,"))
	if _, err := io.WriteString(file, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff/imports", &body)
	request.Host = "example.test"
	request.Header.Set("Origin", "http://example.test")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	handler := NewHandler(&staffStoreStub{}, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("maximum-sized file plus multipart envelope was rejected: %s", response.Body.String())
	}
}

func TestStaffImportReportsValidationIssuesWithoutImportingAnyRows(t *testing.T) {
	store := &staffStoreStub{}
	audit := &auditStub{}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, audit,
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffImportRequest(t,
		"staff_id,full_name\n23847,Ada Okafor\n23847,Duplicate Person\n"))
	if response.Code != http.StatusUnprocessableEntity || store.imported ||
		store.failedImport.ErrorCount != 1 || !audit.called ||
		!strings.Contains(response.Body.String(), "duplicate_staff_id_in_file") {
		t.Fatalf("status=%d imported=%v errors=%+v audited=%v body=%s",
			response.Code, store.imported, store.failedImport, audit.called, response.Body.String())
	}
}

func TestStaffImportRequiresPermissionAndMultipartFile(t *testing.T) {
	auth := &authenticatorStub{principal: application.Principal{ID: authorizedPrincipal().ID, Permissions: []string{"staff.read"}}}
	store := &staffStoreStub{}
	handler := NewHandler(store, auth, &auditStub{}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, staffImportRequest(t, "staff_id,full_name\n23847,Ada\n"))
	if response.Code != http.StatusForbidden || store.imported {
		t.Fatalf("permission status=%d imported=%v", response.Code, store.imported)
	}
	auth.principal = authorizedPrincipal()
	request := authorizedRequest(http.MethodPost, "/api/v1/staff/imports", "")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGetStaffImportRequiresValidIDAndReturnsOwnerReadableResult(t *testing.T) {
	const importID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	store := &staffStoreStub{importBatch: domain.StaffImportBatch{
		ID: importID, Status: "completed",
		Result: domain.StaffImportResult{TotalRows: 2, ImportedRows: 2},
	}}
	handler := NewHandler(store, &authenticatorStub{principal: authorizedPrincipal()}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/staff/imports/"+importID, ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/staff/imports/not-a-uuid", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid ID status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDepartmentEndpointsRequirePermissionAndAuditMutations(t *testing.T) {
	store := &staffStoreStub{departments: []domain.Department{{ID: "dept-1", Name: "Engineering"}}}
	audit := &auditStub{}
	auth := &authenticatorStub{principal: authorizedPrincipal()}
	handler := NewHandler(store, auth, audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodGet, "/api/v1/departments", ""))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"Engineering"`) {
		t.Fatalf("list departments status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "/api/v1/departments", `{"name":"Engineering"}`))
	if response.Code != http.StatusCreated || !store.departmentCreated || !audit.called {
		t.Fatalf("create department status=%d created=%v audit=%v body=%s",
			response.Code, store.departmentCreated, audit.called, response.Body.String())
	}
	const departmentID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPatch,
		"/api/v1/departments/"+departmentID, `{"description":null}`))
	if response.Code != http.StatusOK || !store.departmentUpdated {
		t.Fatalf("update department status=%d updated=%v body=%s",
			response.Code, store.departmentUpdated, response.Body.String())
	}
	auth.principal.Permissions = []string{"staff.read"}
	store.departmentCreated = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedRequest(http.MethodPost, "/api/v1/departments", `{"name":"Restricted"}`))
	if response.Code != http.StatusForbidden || store.departmentCreated {
		t.Fatalf("unauthorized department create status=%d created=%v", response.Code, store.departmentCreated)
	}
}

func TestErrorResponsesRetainRequestIDs(t *testing.T) {
	handler := NewHandler(&staffStoreStub{}, &authenticatorStub{}, &auditStub{},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/staff", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header")
	}
	var body struct {
		Error struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.RequestID == "" {
		t.Fatalf("expected request ID in error response: %s", response.Body.String())
	}
}
