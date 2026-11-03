package monolith

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	app "github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/cloudinary"
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

type ProfileImageUploader interface {
	Upload(context.Context, string, []byte) (string, error)
}

func NewCloudinaryUploader(cloudName, apiKey, apiSecret string) (ProfileImageUploader, error) {
	return cloudinary.New(cloudName, apiKey, apiSecret)
}

func NewHandler(
	db *sql.DB,
	authenticator Authenticator,
	audit AuditRecorder,
	identity AccountProvisioner,
	access StaffRoleAssigner,
	logger *slog.Logger,
	uploaders ...ProfileImageUploader,
) http.Handler {
	store := &staffStore{db: db}
	onboarding := app.OnboardingServices{
		Identity: identity, Access: access,
	}
	if stateService, ok := identity.(app.AccountSuspender); ok {
		onboarding.IdentityState = stateService
	}
	if len(uploaders) > 0 {
		onboarding.ProfileImageUploader = uploaders[0]
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
	staff, err := scanStaffWithImage(s.db.QueryRowContext(ctx, `
		SELECT id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status, profile_image_url
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
	staff, err := scanStaffWithImage(s.db.QueryRowContext(ctx, `
		UPDATE organization.staff_members
		SET account_id = $2::uuid, employment_status = 'active', updated_at = now()
		WHERE id = $1::uuid AND employment_status = 'pending'
		  AND (account_id IS NULL OR account_id = $2::uuid)
		RETURNING id, account_id, staff_id, department_id, full_name, work_email,
		          job_title, employment_status, profile_image_url
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

func (s *staffStore) ListStaff(ctx context.Context, filter domain.StaffListFilter) ([]domain.Staff, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("PostgreSQL database is unavailable")
	}
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 5)
	if filter.Search != "" {
		args = append(args, strings.ToLower(filter.Search))
		conditions = append(conditions, fmt.Sprintf(
			"(strpos(lower(staff_id), $%d) > 0 OR strpos(lower(full_name), $%d) > 0 OR strpos(lower(coalesce(work_email, '')), $%d) > 0)",
			len(args), len(args), len(args)))
	}
	if filter.DepartmentID != "" {
		args = append(args, filter.DepartmentID)
		conditions = append(conditions, fmt.Sprintf("department_id = $%d::uuid", len(args)))
	}
	if filter.EmploymentStatus != "" {
		args = append(args, filter.EmploymentStatus)
		conditions = append(conditions, fmt.Sprintf("employment_status = $%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM organization.staff_members"+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count staff records: %w", err)
	}
	pageArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, staff_id, department_id, full_name, work_email,
		       job_title, employment_status, profile_image_url
		FROM organization.staff_members`+where+
		fmt.Sprintf(" ORDER BY created_at DESC, id LIMIT $%d OFFSET $%d", len(pageArgs)-1, len(pageArgs)),
		pageArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list staff records: %w", err)
	}
	defer rows.Close()
	staff := make([]domain.Staff, 0)
	for rows.Next() {
		record, err := scanStaffWithImage(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan staff record: %w", err)
		}
		staff = append(staff, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read staff records: %w", err)
	}
	return staff, total, nil
}

func (s *staffStore) UpdateProfileImage(ctx context.Context, id, imageURL string) (domain.Staff, error) {
	if s == nil || s.db == nil {
		return domain.Staff{}, errors.New("PostgreSQL database is unavailable")
	}
	staff, err := scanStaffWithImage(s.db.QueryRowContext(ctx, `
		UPDATE organization.staff_members
		SET profile_image_url = $2, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id, account_id, staff_id, department_id, full_name, work_email,
		          job_title, employment_status, profile_image_url
	`, id, imageURL))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Staff{}, domain.ErrStaffNotFound
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("save staff profile image URL: %w", err)
	}
	return staff, nil
}

func (s *staffStore) UpdateStaff(ctx context.Context, id string, request domain.UpdateStaffRequest) (domain.Staff, error) {
	if s == nil || s.db == nil {
		return domain.Staff{}, errors.New("PostgreSQL database is unavailable")
	}
	staff, err := scanStaffWithImage(s.db.QueryRowContext(ctx, `
		UPDATE organization.staff_members SET
			full_name = CASE WHEN $2 THEN $3 ELSE full_name END,
			work_email = CASE WHEN $4 THEN $5 ELSE work_email END,
			job_title = CASE WHEN $6 THEN $7 ELSE job_title END,
			department_id = CASE WHEN $8 THEN $9::uuid ELSE department_id END,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id, account_id, staff_id, department_id, full_name, work_email,
		          job_title, employment_status, profile_image_url
	`, id,
		request.FullName.Set, nullableString(request.FullName.Value),
		request.WorkEmail.Set, nullableString(request.WorkEmail.Value),
		request.JobTitle.Set, nullableString(request.JobTitle.Value),
		request.DepartmentID.Set, nullableUUID(request.DepartmentID.Value)))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Staff{}, domain.ErrStaffNotFound
	}
	if isForeignKeyViolation(err) {
		return domain.Staff{}, domain.ErrDepartmentNotFound
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("update staff record: %w", err)
	}
	return staff, nil
}

func (s *staffStore) DeactivateStaff(ctx context.Context, id string) (domain.Staff, error) {
	if s == nil || s.db == nil {
		return domain.Staff{}, errors.New("PostgreSQL database is unavailable")
	}
	staff, err := scanStaffWithImage(s.db.QueryRowContext(ctx, `
		UPDATE organization.staff_members
		SET employment_status = 'inactive', updated_at = now()
		WHERE id = $1::uuid AND employment_status NOT IN ('inactive', 'deactivated')
		RETURNING id, account_id, staff_id, department_id, full_name, work_email,
		          job_title, employment_status, profile_image_url
	`, id))
	if err == nil {
		return staff, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Staff{}, fmt.Errorf("deactivate staff record: %w", err)
	}
	staff, err = s.FindByID(ctx, id)
	if err != nil {
		return domain.Staff{}, err
	}
	if staff.EmploymentStatus == "inactive" || staff.EmploymentStatus == "deactivated" {
		return staff, nil
	}
	return domain.Staff{}, domain.ErrStaffNotFound
}

func (s *staffStore) ListDepartments(ctx context.Context) ([]domain.Department, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("PostgreSQL database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description FROM organization.departments
		ORDER BY lower(name), id
	`)
	if err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	defer rows.Close()
	departments := make([]domain.Department, 0)
	for rows.Next() {
		department, err := scanDepartment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan department: %w", err)
		}
		departments = append(departments, department)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read departments: %w", err)
	}
	return departments, nil
}

func (s *staffStore) CreateDepartment(ctx context.Context, id string, request domain.DepartmentRequest) (domain.Department, error) {
	if s == nil || s.db == nil {
		return domain.Department{}, errors.New("PostgreSQL database is unavailable")
	}
	department, err := scanDepartment(s.db.QueryRowContext(ctx, `
		INSERT INTO organization.departments (id, name, description)
		VALUES ($1::uuid, $2, $3)
		RETURNING id, name, description
	`, id, *request.Name.Value, nullableString(request.Description.Value)))
	if isUniqueViolation(err) {
		return domain.Department{}, domain.ErrDepartmentConflict
	}
	if err != nil {
		return domain.Department{}, fmt.Errorf("create department: %w", err)
	}
	return department, nil
}

func (s *staffStore) UpdateDepartment(ctx context.Context, id string, request domain.DepartmentRequest) (domain.Department, error) {
	if s == nil || s.db == nil {
		return domain.Department{}, errors.New("PostgreSQL database is unavailable")
	}
	department, err := scanDepartment(s.db.QueryRowContext(ctx, `
		UPDATE organization.departments SET
			name = CASE WHEN $2 THEN $3 ELSE name END,
			description = CASE WHEN $4 THEN $5 ELSE description END,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id, name, description
	`, id, request.Name.Set, nullableString(request.Name.Value),
		request.Description.Set, nullableString(request.Description.Value)))
	if isUniqueViolation(err) {
		return domain.Department{}, domain.ErrDepartmentConflict
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Department{}, domain.ErrDepartmentNotFound
	}
	if err != nil {
		return domain.Department{}, fmt.Errorf("update department: %w", err)
	}
	return department, nil
}

func (s *staffStore) ImportStaffBatch(
	ctx context.Context,
	batchID, actorID string,
	importRows []domain.ImportStaffRow,
) (domain.StaffImportBatch, error) {
	if s == nil || s.db == nil {
		return domain.StaffImportBatch{}, errors.New("PostgreSQL database is unavailable")
	}
	type importRecord struct {
		RowNumber    int     `json:"row_number"`
		StaffID      string  `json:"staff_id"`
		DepartmentID *string `json:"department_id"`
		FullName     string  `json:"full_name"`
		WorkEmail    *string `json:"work_email"`
		JobTitle     *string `json:"job_title"`
	}
	records := make([]importRecord, len(importRows))
	for index, row := range importRows {
		records[index] = importRecord{
			RowNumber: row.RowNumber, StaffID: row.Staff.StaffID,
			DepartmentID: row.Staff.DepartmentID, FullName: row.Staff.FullName,
			WorkEmail: row.Staff.WorkEmail, JobTitle: row.Staff.JobTitle,
		}
	}
	payload, err := json.Marshal(records)
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("encode staff import rows: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("begin staff import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	issues, errorCount, err := inspectImportRows(ctx, tx, payload)
	if err != nil {
		return domain.StaffImportBatch{}, err
	}
	if len(issues) > 0 {
		result := domain.StaffImportResult{
			TotalRows: len(importRows), ErrorCount: errorCount,
			ErrorsTruncated: errorCount > len(issues), Errors: issues,
		}
		batch, err := insertImportBatch(ctx, tx, batchID, actorID, "failed", result)
		if err != nil {
			return domain.StaffImportBatch{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.StaffImportBatch{}, fmt.Errorf("commit failed staff import result: %w", err)
		}
		return batch, nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organization.staff_import_batches (id, status, created_by)
		VALUES ($1::uuid, 'processing', $2::uuid)
	`, batchID, actorID); err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("create staff import batch: %w", err)
	}
	insertedRows, err := tx.QueryContext(ctx, `
		INSERT INTO organization.staff_members (
			id, staff_id, department_id, full_name, work_email, job_title, employment_status
		)
		SELECT gen_random_uuid(), import_row.staff_id, import_row.department_id, import_row.full_name,
		       import_row.work_email, import_row.job_title, 'pending'
		FROM jsonb_to_recordset($1::jsonb) AS import_row(
			row_number integer, staff_id text, department_id uuid,
			full_name text, work_email text, job_title text
		)
		ON CONFLICT (staff_id) DO NOTHING
		RETURNING staff_id
	`, string(payload))
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("insert staff import records: %w", err)
	}
	inserted := make(map[string]struct{}, len(importRows))
	for insertedRows.Next() {
		var staffID string
		if err := insertedRows.Scan(&staffID); err != nil {
			_ = insertedRows.Close()
			return domain.StaffImportBatch{}, fmt.Errorf("read imported staff identifier: %w", err)
		}
		inserted[staffID] = struct{}{}
	}
	if err := insertedRows.Err(); err != nil {
		_ = insertedRows.Close()
		return domain.StaffImportBatch{}, fmt.Errorf("read imported staff records: %w", err)
	}
	if err := insertedRows.Close(); err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("close imported staff rows: %w", err)
	}
	if len(inserted) != len(importRows) {
		issues := make([]domain.StaffImportIssue, 0, maxImportIssueCount)
		errorCount := 0
		for _, row := range importRows {
			if _, ok := inserted[row.Staff.StaffID]; !ok {
				errorCount++
				if len(issues) < maxImportIssueCount {
					issues = append(issues, domain.StaffImportIssue{
						Row: row.RowNumber, StaffID: row.Staff.StaffID,
						Code: "duplicate_staff_id", Message: "staff ID already exists",
					})
				}
			}
		}
		_ = tx.Rollback()
		return s.RecordFailedStaffImport(ctx, batchID, actorID, domain.StaffImportResult{
			TotalRows: len(importRows), ErrorCount: errorCount,
			ErrorsTruncated: errorCount > len(issues), Errors: issues,
		})
	}
	result := domain.StaffImportResult{TotalRows: len(importRows), ImportedRows: len(inserted)}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("encode staff import result: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE organization.staff_import_batches
		SET status = 'completed', completed_at = now(), result = $2::jsonb
		WHERE id = $1::uuid
	`, batchID, string(resultJSON)); err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("complete staff import batch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("commit staff import: %w", err)
	}
	return s.FindStaffImportBatch(ctx, batchID)
}

const maxImportIssueCount = 100

func inspectImportRows(ctx context.Context, tx *sql.Tx, payload []byte) ([]domain.StaffImportIssue, int, error) {
	rows, err := tx.QueryContext(ctx, `
		WITH issues AS (
		SELECT import_row.row_number, import_row.staff_id, 'duplicate_staff_id' AS code,
		       'staff ID already exists' AS message
		FROM jsonb_to_recordset($1::jsonb) AS import_row(
			row_number integer, staff_id text, department_id uuid,
			full_name text, work_email text, job_title text
		)
		JOIN organization.staff_members existing ON existing.staff_id = import_row.staff_id
		UNION ALL
		SELECT import_row.row_number, import_row.staff_id, 'department_not_found' AS code,
		       'department ID does not exist'
		FROM jsonb_to_recordset($1::jsonb) AS import_row(
			row_number integer, staff_id text, department_id uuid,
			full_name text, work_email text, job_title text
		)
		LEFT JOIN organization.departments department ON department.id = import_row.department_id
		WHERE import_row.department_id IS NOT NULL AND department.id IS NULL
		)
		SELECT row_number, staff_id, code, message, count(*) OVER ()
		FROM issues ORDER BY row_number
	`, string(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("validate staff import rows: %w", err)
	}
	defer rows.Close()
	issues := make([]domain.StaffImportIssue, 0, maxImportIssueCount)
	errorCount := 0
	for rows.Next() {
		var issue domain.StaffImportIssue
		if err := rows.Scan(&issue.Row, &issue.StaffID, &issue.Code, &issue.Message, &errorCount); err != nil {
			return nil, 0, fmt.Errorf("scan staff import validation issue: %w", err)
		}
		if len(issues) < maxImportIssueCount {
			issues = append(issues, issue)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read staff import validation issues: %w", err)
	}
	return issues, errorCount, nil
}

func (s *staffStore) RecordFailedStaffImport(
	ctx context.Context,
	batchID, actorID string,
	result domain.StaffImportResult,
) (domain.StaffImportBatch, error) {
	if s == nil || s.db == nil {
		return domain.StaffImportBatch{}, errors.New("PostgreSQL database is unavailable")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("encode failed staff import result: %w", err)
	}
	batch, err := scanStaffImportBatch(s.db.QueryRowContext(ctx, `
		INSERT INTO organization.staff_import_batches (id, status, created_by, completed_at, result)
		VALUES ($1::uuid, 'failed', $2::uuid, now(), $3::jsonb)
		RETURNING id, status, created_at, completed_at, result
	`, batchID, actorID, string(payload)))
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("record failed staff import: %w", err)
	}
	return batch, nil
}

func (s *staffStore) FindStaffImportBatch(ctx context.Context, id string) (domain.StaffImportBatch, error) {
	if s == nil || s.db == nil {
		return domain.StaffImportBatch{}, errors.New("PostgreSQL database is unavailable")
	}
	batch, err := scanStaffImportBatch(s.db.QueryRowContext(ctx, `
		SELECT id, status, created_at, completed_at, result
		FROM organization.staff_import_batches WHERE id = $1::uuid
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.StaffImportBatch{}, domain.ErrImportNotFound
	}
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("find staff import batch: %w", err)
	}
	return batch, nil
}

func insertImportBatch(
	ctx context.Context,
	tx *sql.Tx,
	id, actorID, status string,
	result domain.StaffImportResult,
) (domain.StaffImportBatch, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("encode staff import result: %w", err)
	}
	return scanStaffImportBatch(tx.QueryRowContext(ctx, `
		INSERT INTO organization.staff_import_batches (id, status, created_by, completed_at, result)
		VALUES ($1::uuid, $2, $3::uuid, now(), $4::jsonb)
		RETURNING id, status, created_at, completed_at, result
	`, id, status, actorID, string(payload)))
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

func scanStaffWithImage(row rowScanner) (domain.Staff, error) {
	var staff domain.Staff
	var accountID, departmentID, workEmail, jobTitle, profileImageURL sql.NullString
	err := row.Scan(
		&staff.ID, &accountID, &staff.StaffID, &departmentID,
		&staff.FullName, &workEmail, &jobTitle, &staff.EmploymentStatus, &profileImageURL,
	)
	if err != nil {
		return domain.Staff{}, err
	}
	staff.AccountID = nullableStringPointer(accountID)
	staff.DepartmentID = nullableStringPointer(departmentID)
	staff.WorkEmail = nullableStringPointer(workEmail)
	staff.JobTitle = nullableStringPointer(jobTitle)
	staff.ProfileImageURL = nullableStringPointer(profileImageURL)
	return staff, nil
}

func scanDepartment(row rowScanner) (domain.Department, error) {
	var department domain.Department
	var description sql.NullString
	if err := row.Scan(&department.ID, &department.Name, &description); err != nil {
		return domain.Department{}, err
	}
	department.Description = nullableStringPointer(description)
	return department, nil
}

func scanStaffImportBatch(row rowScanner) (domain.StaffImportBatch, error) {
	var batch domain.StaffImportBatch
	var completedAt sql.NullTime
	var result []byte
	if err := row.Scan(&batch.ID, &batch.Status, &batch.CreatedAt, &completedAt, &result); err != nil {
		return domain.StaffImportBatch{}, err
	}
	if completedAt.Valid {
		batch.CompletedAt = &completedAt.Time
	}
	if err := json.Unmarshal(result, &batch.Result); err != nil {
		return domain.StaffImportBatch{}, fmt.Errorf("decode staff import result: %w", err)
	}
	return batch, nil
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

func isForeignKeyViolation(err error) bool {
	type sqlStateError interface {
		SQLState() string
	}
	var stateErr sqlStateError
	return errors.As(err, &stateErr) && stateErr.SQLState() == "23503"
}
