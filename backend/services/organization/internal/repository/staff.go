package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
)

type StaffRepository struct {
	pool *pgxpool.Pool
}

func NewStaffRepository(pool *pgxpool.Pool) *StaffRepository {
	return &StaffRepository{pool: pool}
}

func (repository *StaffRepository) Ping(ctx context.Context) error {
	if repository == nil || repository.pool == nil {
		return errors.New("Organization database is unavailable")
	}
	return repository.pool.Ping(ctx)
}

func (repository *StaffRepository) Create(ctx context.Context, id string, request domain.CreateStaffRequest) (domain.Staff, error) {
	if repository == nil || repository.pool == nil {
		return domain.Staff{}, errors.New("Organization database is unavailable")
	}
	var staff domain.Staff
	err := repository.pool.QueryRow(ctx, `
		INSERT INTO organization.staff_members
			(id, staff_id, department_id, full_name, work_email, job_title, employment_status)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		RETURNING id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status
	`, id, request.StaffID, request.DepartmentID, request.FullName, request.WorkEmail, request.JobTitle).Scan(
		&staff.ID, &staff.AccountID, &staff.StaffID, &staff.DepartmentID, &staff.FullName,
		&staff.WorkEmail, &staff.JobTitle, &staff.EmploymentStatus,
	)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) {
			if postgresError.Code == "23505" {
				return domain.Staff{}, domain.ErrDuplicateStaffID
			}
			if postgresError.Code == "23503" {
				return domain.Staff{}, domain.ErrDepartmentNotFound
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Staff{}, fmt.Errorf("create staff returned no record")
		}
		return domain.Staff{}, fmt.Errorf("insert staff record: %w", err)
	}
	return staff, nil
}

func (repository *StaffRepository) FindByID(ctx context.Context, id string) (domain.Staff, error) {
	if repository == nil || repository.pool == nil {
		return domain.Staff{}, errors.New("Organization database is unavailable")
	}
	var staff domain.Staff
	err := repository.pool.QueryRow(ctx, `
		SELECT id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status
		FROM organization.staff_members
		WHERE id = $1
	`, id).Scan(
		&staff.ID, &staff.AccountID, &staff.StaffID, &staff.DepartmentID,
		&staff.FullName, &staff.WorkEmail, &staff.JobTitle, &staff.EmploymentStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Staff{}, domain.ErrStaffNotFound
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("find staff for provisioning: %w", err)
	}
	return staff, nil
}

func (repository *StaffRepository) CompleteProvisioning(ctx context.Context, staffID, accountID string) (domain.Staff, error) {
	if repository == nil || repository.pool == nil {
		return domain.Staff{}, errors.New("Organization database is unavailable")
	}
	var staff domain.Staff
	err := repository.pool.QueryRow(ctx, `
		UPDATE organization.staff_members
		SET account_id = $2, employment_status = 'active', updated_at = NOW()
		WHERE id = $1 AND employment_status = 'pending'
		  AND (account_id IS NULL OR account_id = $2)
		RETURNING id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status
	`, staffID, accountID).Scan(
		&staff.ID, &staff.AccountID, &staff.StaffID, &staff.DepartmentID,
		&staff.FullName, &staff.WorkEmail, &staff.JobTitle, &staff.EmploymentStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, findErr := repository.FindByID(ctx, staffID)
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
