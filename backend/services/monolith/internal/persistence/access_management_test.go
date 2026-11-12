package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	accessstore "github.com/mothercat-enterprise/mothercat/backend/services/access/monolith"
)

func TestReplaceRolePermissionsRejectsUndelegatedPermission(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT DISTINCT rp\.permission_key.*WHERE ur\.user_id = \$1::uuid`).
		WithArgs("00000000-0000-4000-8000-000000000001").
		WillReturnRows(sqlmock.NewRows([]string{"permission_key"}).AddRow("roles.manage"))
	mock.ExpectQuery(`SELECT is_system_role FROM access\.roles WHERE role_key = \$1 FOR UPDATE`).
		WithArgs("auditor").
		WillReturnRows(sqlmock.NewRows([]string{"is_system_role"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM access\.permissions WHERE permission_key = \$1\)`).
		WithArgs("settings.manage").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, err = accessstore.NewStore(db).ReplaceRolePermissions(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		"auditor",
		[]string{"settings.manage"},
	)
	if !errors.Is(err, accessstore.ErrPermissionDenied) {
		t.Fatalf("ReplaceRolePermissions() error = %v, want permission denied", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceUserRolesRejectsSelfAssignmentBeforeDatabaseAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	accountID := "00000000-0000-4000-8000-000000000001"
	_, err = accessstore.NewStore(db).ReplaceUserRoles(
		context.Background(), accountID, accountID, []string{"super_admin"},
	)
	if !errors.Is(err, accessstore.ErrPermissionDenied) {
		t.Fatalf("ReplaceUserRoles() error = %v, want permission denied", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceUserRolesRejectsAdminAssigningSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	actorID := "00000000-0000-4000-8000-000000000001"
	targetID := "00000000-0000-4000-8000-000000000002"
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT DISTINCT rp\.permission_key.*WHERE ur\.user_id = \$1::uuid`).
		WithArgs(actorID).
		WillReturnRows(sqlmock.NewRows([]string{"permission_key"}).
			AddRow("user_roles.assign").AddRow("roles.manage"))
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*ur\.role_key = 'super_admin'.*a\.is_active`).
		WithArgs(actorID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()

	_, err = accessstore.NewStore(db).ReplaceUserRoles(
		context.Background(), actorID, targetID, []string{"super_admin"},
	)
	if !errors.Is(err, accessstore.ErrPermissionDenied) {
		t.Fatalf("ReplaceUserRoles() error = %v, want permission denied", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceUserRolesRejectsRoleWithUndelegatedPermissions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT DISTINCT rp\.permission_key.*WHERE ur\.user_id = \$1::uuid`).
		WithArgs("00000000-0000-4000-8000-000000000001").
		WillReturnRows(sqlmock.NewRows([]string{"permission_key"}).AddRow("user_roles.assign"))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM access\.roles WHERE role_key = \$1\)`).
		WithArgs("administrator").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT DISTINCT permission_key FROM access\.role_permissions`).
		WithArgs("administrator").
		WillReturnRows(sqlmock.NewRows([]string{"permission_key"}).AddRow("settings.manage"))
	mock.ExpectRollback()

	_, err = accessstore.NewStore(db).ReplaceUserRoles(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		[]string{"administrator"},
	)
	if !errors.Is(err, accessstore.ErrPermissionDenied) {
		t.Fatalf("ReplaceUserRoles() error = %v, want permission denied", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceUserRolesCannotRemoveLastActiveSuperAdmin(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	actorID := "00000000-0000-4000-8000-000000000001"
	targetID := "00000000-0000-4000-8000-000000000002"
	mock.ExpectBegin()
	mock.ExpectExec(`pg_advisory_xact_lock\(761039123\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT DISTINCT rp\.permission_key.*WHERE ur\.user_id = \$1::uuid`).
		WithArgs(actorID).
		WillReturnRows(sqlmock.NewRows([]string{"permission_key"}).
			AddRow("user_roles.assign").AddRow("staff.read"))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM access\.roles WHERE role_key = \$1\)`).
		WithArgs("staff").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT DISTINCT permission_key FROM access\.role_permissions`).
		WithArgs("staff").
		WillReturnRows(sqlmock.NewRows([]string{"permission_key"}).AddRow("staff.read"))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM identity\.user_accounts WHERE id = \$1::uuid\)`).
		WithArgs(targetID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*ur\.role_key = 'super_admin'.*a\.is_active`).
		WithArgs(targetID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*ur\.role_key = 'super_admin'.*a\.is_active`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	_, err = accessstore.NewStore(db).ReplaceUserRoles(
		context.Background(), actorID, targetID, []string{"staff"},
	)
	if !errors.Is(err, accessstore.ErrLastActiveSuperAdmin) {
		t.Fatalf("ReplaceUserRoles() error = %v, want last active SuperAdmin protection", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
