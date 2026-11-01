package monolith

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	app "github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
	organizationhttp "github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/http"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Principal struct {
	ID          string
	StaffID     string
	Permissions []string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type AuditEvent struct {
	EventKey   string
	ActorID    string
	Action     string
	ResourceID string
	Details    map[string]any
}

type AuditRecorder interface {
	Record(context.Context, AuditEvent) error
}

type AccountProvisioner interface {
	ProvisionAccount(context.Context, string, string, string) (string, error)
	SendActivation(context.Context, string) error
}

type StaffRoleAssigner interface {
	AssignDefaultStaffRole(context.Context, string) error
}

type staffStore struct {
	db *sql.DB
}

func NewHandler(
	db *sql.DB,
	authenticator Authenticator,
	audit AuditRecorder,
	identity AccountProvisioner,
	access StaffRoleAssigner,
	logger *slog.Logger,
) http.Handler {
	store := &staffStore{db: db}
	onboarding := app.OnboardingServices{
		Identity: identity, Access: access,
	}
	return organizationhttp.NewHandler(store, authBridge{authenticator}, auditBridge{audit}, logger, onboarding)
}

type authBridge struct{ Authenticator }

func (b authBridge) Authenticate(ctx context.Context, token string) (app.Principal, error) {
	principal, err := b.Authenticator.Authenticate(ctx, token)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return app.Principal{}, app.ErrUnauthenticated
		}
		return app.Principal{}, err
	}
	return app.Principal{ID: principal.ID, StaffID: principal.StaffID, Permissions: principal.Permissions}, nil
}

type auditBridge struct{ AuditRecorder }

func (b auditBridge) Record(ctx context.Context, event app.AuditEvent) error {
	return b.AuditRecorder.Record(ctx, AuditEvent{
		EventKey: event.EventKey, ActorID: event.ActorID, Action: event.Action,
		ResourceID: event.ResourceID, Details: event.Details,
	})
}

func (s *staffStore) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("PostgreSQL database is unavailable")
	}
	return s.db.PingContext(ctx)
}

func (s *staffStore) Create(ctx context.Context, id string, request domain.CreateStaffRequest) (domain.Staff, error) {
	if s == nil || s.db == nil {
		return domain.Staff{}, errors.New("PostgreSQL database is unavailable")
	}
	staff, err := scanStaff(s.db.QueryRowContext(ctx, `
		INSERT INTO organization.staff_members (
			id, staff_id, department_id, full_name, work_email, job_title, employment_status
		) VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, 'pending')
		RETURNING id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status
	`, id, request.StaffID, nullableUUID(request.DepartmentID), request.FullName,
		nullableString(request.WorkEmail), nullableString(request.JobTitle)))
	if isUniqueViolation(err) {
		return domain.Staff{}, domain.ErrDuplicateStaffID
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("insert staff record: %w", err)
	}
	return staff, nil
}

func (s *staffStore) FindByID(ctx context.Context, id string) (domain.Staff, error) {
	if s == nil || s.db == nil {
		return domain.Staff{}, errors.New("PostgreSQL database is unavailable")
	}
	staff, err := scanStaff(s.db.QueryRowContext(ctx, `
		SELECT id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status
		FROM organization.staff_members WHERE id = $1::uuid
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Staff{}, domain.ErrStaffNotFound
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("find staff record: %w", err)
	}
	return staff, nil
}

func (s *staffStore) CompleteProvisioning(ctx context.Context, staffID, accountID string) (domain.Staff, error) {
	if s == nil || s.db == nil {
		return domain.Staff{}, errors.New("PostgreSQL database is unavailable")
	}
	staff, err := scanStaff(s.db.QueryRowContext(ctx, `
		UPDATE organization.staff_members
		SET account_id = $2::uuid, employment_status = 'active', updated_at = now()
		WHERE id = $1::uuid AND employment_status = 'pending'
		  AND (account_id IS NULL OR account_id = $2::uuid)
		RETURNING id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status
	`, staffID, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		existing, findErr := s.FindByID(ctx, staffID)
		if findErr != nil {
			return domain.Staff{}, findErr
		}
		if existing.EmploymentStatus == "active" && existing.AccountID != nil && *existing.AccountID == accountID {
			return existing, nil
		}
		return domain.Staff{}, domain.ErrStaffAlreadyProvisioned
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("complete staff provisioning: %w", err)
	}
	return staff, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStaff(row rowScanner) (domain.Staff, error) {
	var staff domain.Staff
	var accountID, departmentID, workEmail, jobTitle sql.NullString
	err := row.Scan(
		&staff.ID, &accountID, &staff.StaffID, &departmentID,
		&staff.FullName, &workEmail, &jobTitle, &staff.EmploymentStatus,
	)
	if err != nil {
		return domain.Staff{}, err
	}
	staff.AccountID = nullableStringPointer(accountID)
	staff.DepartmentID = nullableStringPointer(departmentID)
	staff.WorkEmail = nullableStringPointer(workEmail)
	staff.JobTitle = nullableStringPointer(jobTitle)
	return staff, nil
}

func nullableUUID(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func nullableString(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func isUniqueViolation(err error) bool {
	type sqlStateError interface {
		SQLState() string
	}
	var stateErr sqlStateError
	return errors.As(err, &stateErr) && stateErr.SQLState() == "23505"
}
