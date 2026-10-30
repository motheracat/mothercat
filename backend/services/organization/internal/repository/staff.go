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
		RETURNING id, staff_id, department_id, full_name, work_email, job_title, employment_status
	`, id, request.StaffID, request.DepartmentID, request.FullName, request.WorkEmail, request.JobTitle).Scan(
		&staff.ID, &staff.StaffID, &staff.DepartmentID, &staff.FullName,
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
