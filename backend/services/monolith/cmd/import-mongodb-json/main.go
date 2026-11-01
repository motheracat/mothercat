package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type document map[string]json.RawMessage

type importFile struct {
	name     string
	required bool
	importFn func(context.Context, *sql.Tx, document) error
}

func main() {
	if err := run(); err != nil {
		slog.Error("MongoDB export import failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	directory := strings.TrimSpace(os.Getenv("MONGODB_EXPORT_DIR"))
	databaseURL := strings.TrimSpace(os.Getenv("MIGRATIONS_DATABASE_URL"))
	if directory == "" {
		return errors.New("MONGODB_EXPORT_DIR is required")
	}
	if databaseURL == "" {
		return errors.New("MIGRATIONS_DATABASE_URL is required")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil {
		return errors.New("MIGRATIONS_DATABASE_URL must be a valid PostgreSQL connection URL")
	}
	sslMode := parsed.Query().Get("sslmode")
	if sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
		return errors.New("MIGRATIONS_DATABASE_URL must require TLS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("configure PostgreSQL import connection")
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close PostgreSQL import connection", "error", err)
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		return errors.New("connect to PostgreSQL for data import")
	}
	return importExports(ctx, db, directory)
}

func importExports(ctx context.Context, db *sql.DB, directory string) error {
	var migrated bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = '001_initial')`,
	).Scan(&migrated); err != nil {
		return fmt.Errorf("check PostgreSQL schema migration: %w", err)
	}
	if !migrated {
		return errors.New("PostgreSQL schema migration 001_initial must be applied first")
	}
	for _, query := range []string{
		`SELECT EXISTS (SELECT 1 FROM identity.user_accounts)`,
		`SELECT EXISTS (SELECT 1 FROM identity.sessions)`,
		`SELECT EXISTS (SELECT 1 FROM identity.recovery_tokens)`,
		`SELECT EXISTS (SELECT 1 FROM identity.authentication_events)`,
		`SELECT EXISTS (SELECT 1 FROM identity.bootstrap_state)`,
		`SELECT EXISTS (SELECT 1 FROM organization.staff_members)`,
		`SELECT EXISTS (SELECT 1 FROM organization.departments)`,
		`SELECT EXISTS (SELECT 1 FROM organization.department_memberships)`,
		`SELECT EXISTS (SELECT 1 FROM organization.staff_import_batches)`,
		`SELECT EXISTS (SELECT 1 FROM access.user_roles)`,
		`SELECT EXISTS (SELECT 1 FROM audit.audit_events)`,
	} {
		var hasData bool
		if err := db.QueryRowContext(ctx, query).Scan(&hasData); err != nil {
			return fmt.Errorf("check target database is empty: %w", err)
		}
		if hasData {
			return errors.New("target PostgreSQL database already has application data; import requires an empty target")
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin data import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM access.role_permissions`); err != nil {
		return fmt.Errorf("prepare exact role-permission import: %w", err)
	}
	files := []importFile{
		{name: "roles", required: true, importFn: importRole},
		{name: "permissions", required: true, importFn: importPermission},
		{name: "identity_accounts", required: true, importFn: importAccount},
		{name: "user_roles", required: true, importFn: importUserRole},
		{name: "role_permissions", required: true, importFn: importRolePermission},
		{name: "identity_sessions", required: true, importFn: importSession},
		{name: "identity_recovery_tokens", required: true, importFn: importRecoveryToken},
		{name: "identity_authentication_events", required: true, importFn: importAuthenticationEvent},
		{name: "system_bootstrap_state", required: true, importFn: importBootstrapState},
		{name: "departments", required: false, importFn: importDepartment},
		{name: "department_memberships", required: false, importFn: importDepartmentMembership},
		{name: "staff_import_batches", required: false, importFn: importStaffImportBatch},
		{name: "staff_members", required: true, importFn: importStaff},
		{name: "audit_events", required: true, importFn: importAuditEvent},
	}
	for _, file := range files {
		count, err := importFileRows(ctx, tx, directory, file)
		if err != nil {
			return err
		}
		slog.Info("imported MongoDB export collection", "collection", file.name, "records", count)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit data import: %w", err)
	}
	slog.Info("MongoDB export data imported into PostgreSQL")
	return nil
}

func importFileRows(ctx context.Context, tx *sql.Tx, directory string, file importFile) (int, error) {
	path := filepath.Join(directory, file.name+".json")
	input, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && !file.required {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open export %s: %w", file.name, err)
	}
	defer input.Close()
	reader := bufio.NewReader(input)
	count := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		line = []byte(strings.TrimSpace(string(line)))
		if len(line) > 0 {
			var doc document
			if err := json.Unmarshal(line, &doc); err != nil {
				return count, fmt.Errorf("decode %s export record %d: %w", file.name, count+1, err)
			}
			if err := file.importFn(ctx, tx, doc); err != nil {
				return count, fmt.Errorf("import %s record %d: %w", file.name, count+1, err)
			}
			count++
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return count, fmt.Errorf("read %s export: %w", file.name, readErr)
		}
	}
	return count, nil
}

func importRole(ctx context.Context, tx *sql.Tx, doc document) error {
	key := stringField(doc, "role_key", "rolekey")
	name := stringField(doc, "display_name", "displayname")
	if key == "" || name == "" {
		return errors.New("role_key and display_name are required")
	}
	isSystemRole, err := requiredBoolField(doc, "is_system_role", "issystemrole")
	if err != nil {
		return err
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO access.roles (role_key, display_name, description, is_system_role, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (role_key) DO UPDATE SET display_name = EXCLUDED.display_name,
			description = EXCLUDED.description, is_system_role = EXCLUDED.is_system_role,
			created_at = EXCLUDED.created_at
	`, key, name, stringField(doc, "description"), isSystemRole, createdAt)
	return err
}

func importPermission(ctx context.Context, tx *sql.Tx, doc document) error {
	key := stringField(doc, "permission_key", "permissionkey")
	if key == "" {
		return errors.New("permission_key is required")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO access.permissions (permission_key, description)
		VALUES ($1, $2)
		ON CONFLICT (permission_key) DO UPDATE SET description = EXCLUDED.description
	`, key, stringField(doc, "description"))
	return err
}

func importAccount(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "_id", "id")
	if !isUUID(id) {
		return errors.New("account _id must be a UUID string")
	}
	staffID := stringField(doc, "staff_id", "staffid")
	email := stringField(doc, "email")
	displayName := stringField(doc, "display_name", "displayname")
	passwordHash := stringField(doc, "password_hash", "passwordhash")
	if staffID == "" || email == "" || displayName == "" || passwordHash == "" {
		return errors.New("account is missing required identity data")
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	updatedAt, err := requiredTimeField(doc, "updated_at", "updatedat")
	if err != nil {
		return err
	}
	isActive, err := requiredBoolField(doc, "is_active", "isactive")
	if err != nil {
		return err
	}
	mustChangePassword, err := requiredBoolField(doc, "must_change_password", "mustchangepassword")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO identity.user_accounts
			(id, staff_id, email, display_name, password_hash, is_active, must_change_password, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, staffID, email, displayName, passwordHash,
		isActive, mustChangePassword, createdAt, updatedAt)
	return err
}

func importUserRole(ctx context.Context, tx *sql.Tx, doc document) error {
	userID := stringField(doc, "user_id", "userid")
	roleKey := stringField(doc, "role_key", "rolekey")
	if !isUUID(userID) || roleKey == "" {
		return errors.New("user_role requires a UUID user_id and role_key")
	}
	assignedAt, err := requiredTimeField(doc, "assigned_at", "assignedat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO access.user_roles (user_id, role_key, assigned_at)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (user_id, role_key) DO UPDATE SET assigned_at = EXCLUDED.assigned_at
	`, userID, roleKey, assignedAt)
	return err
}

func importRolePermission(ctx context.Context, tx *sql.Tx, doc document) error {
	roleKey := stringField(doc, "role_key", "rolekey")
	permissionKey := stringField(doc, "permission_key", "permissionkey")
	if roleKey == "" || permissionKey == "" {
		return errors.New("role_permission requires role_key and permission_key")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO access.role_permissions (role_key, permission_key)
		VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, roleKey, permissionKey)
	return err
}

func importSession(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "_id", "id")
	if !isUUID(id) {
		return errors.New("session _id must be a UUID string")
	}
	accountID := stringField(doc, "account_id", "accountid")
	if !isUUID(accountID) {
		return errors.New("session account_id must be a UUID string")
	}
	tokenHash := stringField(doc, "token_hash", "tokenhash")
	if tokenHash == "" {
		return errors.New("session token_hash is required")
	}
	expiresAt, err := requiredTimeField(doc, "expires_at", "expiresat")
	if err != nil {
		return err
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	revokedAt, err := optionalTimeField(doc, "revoked_at", "revokedat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO identity.sessions (id, account_id, token_hash, expires_at, revoked_at, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
	`, id, accountID, tokenHash, expiresAt, revokedAt, createdAt)
	return err
}

func importRecoveryToken(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "_id", "id")
	if !isUUID(id) {
		return errors.New("recovery token _id must be a UUID string")
	}
	accountID := stringField(doc, "account_id", "accountid")
	if !isUUID(accountID) {
		return errors.New("recovery token account_id must be a UUID string")
	}
	expiresAt, err := requiredTimeField(doc, "expires_at", "expiresat")
	if err != nil {
		return err
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	usedAt, err := optionalTimeField(doc, "used_at", "usedat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO identity.recovery_tokens (id, account_id, token_hash, purpose, expires_at, used_at, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
	`, id, accountID, stringField(doc, "token_hash", "tokenhash"),
		stringField(doc, "purpose"), expiresAt, usedAt, createdAt)
	return err
}

func importAuthenticationEvent(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "_id", "id")
	if !isUUID(id) {
		return errors.New("authentication event _id must be a UUID string")
	}
	occurredAt, err := requiredTimeField(doc, "occurred_at", "occurredat")
	if err != nil {
		return err
	}
	successful, err := requiredBoolField(doc, "successful")
	if err != nil {
		return err
	}
	accountID := stringField(doc, "account_id", "accountid")
	_, err = tx.ExecContext(ctx, `
		INSERT INTO identity.authentication_events (id, account_id, event_type, staff_id, successful, occurred_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
	`, id, nullableUUID(accountID), stringField(doc, "event_type", "eventtype"),
		stringField(doc, "staff_id", "staffid"), successful, occurredAt)
	return err
}

func importBootstrapState(ctx context.Context, tx *sql.Tx, doc document) error {
	key := stringField(doc, "_id", "key")
	accountID := stringField(doc, "account_id", "accountid")
	completedAt, err := requiredTimeField(doc, "completed_at", "completedat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO identity.bootstrap_state (key, state, account_id, completed_at)
		VALUES ($1, $2, $3::uuid, $4)
		ON CONFLICT (key) DO UPDATE SET state = EXCLUDED.state,
			account_id = EXCLUDED.account_id, completed_at = EXCLUDED.completed_at
	`, key, stringField(doc, "state"), nullableUUID(accountID),
		completedAt)
	return err
}

func importDepartment(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "_id", "id")
	if !isUUID(id) {
		return errors.New("department id must be a UUID string")
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	updatedAt, err := requiredTimeField(doc, "updated_at", "updatedat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO organization.departments (id, name, description, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5)
	`, id, stringField(doc, "name"), stringField(doc, "description"),
		createdAt, updatedAt)
	return err
}

func importDepartmentMembership(ctx context.Context, tx *sql.Tx, doc document) error {
	departmentID := stringField(doc, "department_id", "departmentid")
	accountID := stringField(doc, "account_id", "accountid")
	if !isUUID(departmentID) || !isUUID(accountID) {
		return errors.New("department membership requires UUID department_id and account_id")
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO organization.department_memberships (department_id, account_id, created_at)
		VALUES ($1::uuid, $2::uuid, $3)
	`, departmentID, accountID, createdAt)
	return err
}

func importStaffImportBatch(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "_id", "id")
	if !isUUID(id) {
		return errors.New("staff import batch id must be a UUID string")
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	completedAt, err := optionalTimeField(doc, "completed_at", "completedat")
	if err != nil {
		return err
	}
	result := rawJSONField(doc, "result")
	if len(result) == 0 || string(result) == "null" {
		result = []byte("{}")
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO organization.staff_import_batches (id, status, created_by, created_at, completed_at, result)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6::jsonb)
	`, id, stringField(doc, "status"), nullableUUID(stringField(doc, "created_by", "createdby")),
		createdAt, completedAt, result)
	return err
}

func importStaff(ctx context.Context, tx *sql.Tx, doc document) error {
	id := stringField(doc, "id")
	if !isUUID(id) {
		return errors.New("staff id must be a UUID string")
	}
	accountID := stringField(doc, "account_id", "accountid")
	departmentID := stringField(doc, "department_id", "departmentid")
	staffID := stringField(doc, "staff_id", "staffid")
	if staffID == "" {
		return errors.New("staff_id is required")
	}
	createdAt, err := requiredTimeField(doc, "created_at", "createdat")
	if err != nil {
		return err
	}
	updatedAt, err := requiredTimeField(doc, "updated_at", "updatedat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO organization.staff_members (
			id, staff_id, account_id, department_id, full_name, work_email,
			job_title, employment_status, created_at, updated_at
		) VALUES ($1::uuid, $2, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, $10)
	`, id, staffID, nullableUUID(accountID), nullableUUID(departmentID),
		stringField(doc, "full_name", "fullname"), nullableString(stringField(doc, "work_email", "workemail")),
		nullableString(stringField(doc, "job_title", "jobtitle")),
		defaultString(stringField(doc, "employment_status", "employmentstatus"), "pending"),
		createdAt, updatedAt)
	return err
}

func importAuditEvent(ctx context.Context, tx *sql.Tx, doc document) error {
	eventKey := stringField(doc, "event_key", "eventkey")
	if eventKey == "" {
		return errors.New("audit event_key is required")
	}
	details := rawJSONField(doc, "details")
	if len(details) == 0 || string(details) == "null" {
		details = []byte("{}")
	}
	occurredAt, err := requiredTimeField(doc, "occurred_at", "occurredat")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type,
			resource_id, occurred_at, details
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		ON CONFLICT (event_key) DO NOTHING
	`, eventKey, defaultString(stringField(doc, "actor_type", "actortype"), "user"),
		stringField(doc, "actor_id", "actorid"), stringField(doc, "action"),
		defaultString(stringField(doc, "resource_type", "resourcetype"), "unknown"),
		stringField(doc, "resource_id", "resourceid"),
		occurredAt, details)
	return err
}

func stringField(doc document, names ...string) string {
	for _, name := range names {
		for key, value := range doc {
			if strings.EqualFold(key, name) {
				var result string
				if json.Unmarshal(value, &result) == nil {
					return result
				}
			}
		}
	}
	return ""
}

func requiredBoolField(doc document, names ...string) (bool, error) {
	for _, name := range names {
		for key, value := range doc {
			if strings.EqualFold(key, name) {
				var result bool
				if json.Unmarshal(value, &result) == nil {
					return result, nil
				}
				return false, fmt.Errorf("%s must be a boolean", name)
			}
		}
	}
	return false, fmt.Errorf("%s is required", names[0])
}

func requiredTimeField(doc document, names ...string) (time.Time, error) {
	value, err := optionalTimeField(doc, names...)
	if err != nil {
		return time.Time{}, err
	}
	if value == nil {
		return time.Time{}, fmt.Errorf("%s is required", names[0])
	}
	return *value, nil
}

func optionalTimeField(doc document, names ...string) (*time.Time, error) {
	for _, name := range names {
		for key, raw := range doc {
			if !strings.EqualFold(key, name) || string(raw) == "null" {
				continue
			}
			var text string
			if json.Unmarshal(raw, &text) == nil {
				if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
					parsed = parsed.UTC()
					return &parsed, nil
				}
				return nil, fmt.Errorf("%s is not a valid MongoDB date", name)
			}
			var extended map[string]json.RawMessage
			if json.Unmarshal(raw, &extended) == nil {
				if dateRaw, ok := extended["$date"]; ok {
					var dateText string
					if json.Unmarshal(dateRaw, &dateText) == nil {
						if parsed, err := time.Parse(time.RFC3339Nano, dateText); err == nil {
							parsed = parsed.UTC()
							return &parsed, nil
						}
						return nil, fmt.Errorf("%s is not a valid MongoDB date", name)
					}
					var number map[string]json.RawMessage
					if json.Unmarshal(dateRaw, &number) == nil {
						var millisText string
						if json.Unmarshal(number["$numberLong"], &millisText) == nil {
							millis, err := strconv.ParseInt(millisText, 10, 64)
							if err != nil {
								return nil, fmt.Errorf("%s has an invalid millisecond timestamp", name)
							}
							parsed := time.UnixMilli(millis).UTC()
							return &parsed, nil
						}
						var millis int64
						if json.Unmarshal(number["$numberLong"], &millis) == nil {
							parsed := time.UnixMilli(millis).UTC()
							return &parsed, nil
						}
					}
					return nil, fmt.Errorf("%s has an unsupported MongoDB date encoding", name)
				}
			}
			return nil, fmt.Errorf("%s has an unsupported MongoDB date encoding", name)
		}
	}
	return nil, nil
}

func rawJSONField(doc document, name string) json.RawMessage {
	for key, value := range doc {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return nil
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') &&
			!(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}
