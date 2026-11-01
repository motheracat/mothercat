package monolith

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCompleteProvisioningActivatesStaffWithPostgresColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const update = `UPDATE organization.staff_members
		SET account_id = $2::uuid, employment_status = 'active', updated_at = now()
		WHERE id = $1::uuid AND employment_status = 'pending'
		  AND (account_id IS NULL OR account_id = $2::uuid)
		RETURNING id, account_id, staff_id, department_id, full_name, work_email, job_title, employment_status`
	rows := sqlmock.NewRows([]string{
		"id", "account_id", "staff_id", "department_id", "full_name", "work_email", "job_title", "employment_status",
	}).AddRow(
		"staff-uuid", "account-uuid", "STAFF-001", nil, "Test Staff", "test@example.test", nil, "active",
	)
	mock.ExpectQuery(regexp.QuoteMeta(update)).
		WithArgs("staff-uuid", "account-uuid").
		WillReturnRows(rows)

	store := &staffStore{db: db}
	staff, err := store.CompleteProvisioning(context.Background(), "staff-uuid", "account-uuid")
	if err != nil {
		t.Fatalf("CompleteProvisioning() error = %v", err)
	}
	if staff.EmploymentStatus != "active" || staff.AccountID == nil || *staff.AccountID != "account-uuid" {
		t.Fatalf("CompleteProvisioning() returned staff = %+v", staff)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
