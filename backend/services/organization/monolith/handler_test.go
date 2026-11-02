package monolith

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
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
		"staff-uuid", "account-uuid", "12345", nil, "Test Staff", "test@example.test", nil, "active",
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

func TestListStaffUsesParameterizedFiltersAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	filter := domain.StaffListFilter{Limit: 25, Offset: 5, EmploymentStatus: "pending"}
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT count(*) FROM organization.staff_members WHERE employment_status = $1",
	)).WithArgs("pending").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, account_id, staff_id, department_id, full_name, work_email,\s+job_title, employment_status\s+FROM organization\.staff_members WHERE employment_status = \$1 ORDER BY created_at DESC, id LIMIT \$2 OFFSET \$3`).
		WithArgs("pending", 25, 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "staff_id", "department_id", "full_name", "work_email", "job_title", "employment_status",
		}).AddRow("staff-uuid", nil, "12345", nil, "Test Staff", nil, nil, "pending"))

	staff, total, err := (&staffStore{db: db}).ListStaff(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListStaff() error = %v", err)
	}
	if total != 1 || len(staff) != 1 || staff[0].StaffID != "12345" {
		t.Fatalf("ListStaff() = %+v, total %d", staff, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportStaffBatchCommitsAllRowsAndCompletionTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const batchID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	const actorID = "1c9491b8-21ce-4c66-a025-006bb346b810"
	createdAt := time.Now().UTC()
	completedAt := createdAt.Add(time.Second)
	records := []domain.ImportStaffRow{{
		RowNumber: 2,
		Staff:     domain.CreateStaffRequest{StaffID: "23847", FullName: "Ada Okafor"},
	}}
	resultJSON, err := json.Marshal(domain.StaffImportResult{TotalRows: 1, ImportedRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("WITH issues AS").WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"row_number", "staff_id", "code", "message", "count"}))
	mock.ExpectExec("INSERT INTO organization.staff_import_batches").
		WithArgs(batchID, actorID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO organization.staff_members").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"staff_id"}).AddRow("23847"))
	mock.ExpectExec("UPDATE organization.staff_import_batches").
		WithArgs(batchID, string(resultJSON)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, status, created_at, completed_at, result
		FROM organization.staff_import_batches WHERE id = $1::uuid
	`)).WithArgs(batchID).WillReturnRows(sqlmock.NewRows([]string{
		"id", "status", "created_at", "completed_at", "result",
	}).AddRow(batchID, "completed", createdAt, completedAt, resultJSON))

	batch, err := (&staffStore{db: db}).ImportStaffBatch(context.Background(), batchID, actorID, records)
	if err != nil {
		t.Fatalf("ImportStaffBatch() error = %v", err)
	}
	if batch.Status != "completed" || batch.Result.ImportedRows != 1 || batch.Result.ErrorCount != 0 {
		t.Fatalf("import batch = %+v", batch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportStaffBatchRecordsDuplicateFailureWithoutCommittingStaff(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const batchID = "7da27cda-3496-4d02-9d99-fb0e956297e5"
	const actorID = "1c9491b8-21ce-4c66-a025-006bb346b810"
	createdAt := time.Now().UTC()
	resultJSON, err := json.Marshal(domain.StaffImportResult{
		TotalRows: 1, ErrorCount: 1,
		Errors: []domain.StaffImportIssue{{Row: 2, StaffID: "23847", Code: "duplicate_staff_id", Message: "staff ID already exists"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("WITH issues AS").WithArgs(sqlmock.AnyArg()).WillReturnRows(
		sqlmock.NewRows([]string{"row_number", "staff_id", "code", "message", "count"}).
			AddRow(2, "23847", "duplicate_staff_id", "staff ID already exists", 1),
	)
	mock.ExpectQuery("INSERT INTO organization.staff_import_batches").
		WithArgs(batchID, "failed", actorID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "created_at", "completed_at", "result",
		}).AddRow(batchID, "failed", createdAt, createdAt, resultJSON))
	mock.ExpectCommit()

	batch, err := (&staffStore{db: db}).ImportStaffBatch(context.Background(), batchID, actorID, []domain.ImportStaffRow{{
		RowNumber: 2,
		Staff:     domain.CreateStaffRequest{StaffID: "23847", FullName: "Ada Okafor"},
	}})
	if err != nil {
		t.Fatalf("ImportStaffBatch() error = %v", err)
	}
	if batch.Status != "failed" || batch.Result.ErrorCount != 1 || batch.Result.ImportedRows != 0 {
		t.Fatalf("failed import batch = %+v", batch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
