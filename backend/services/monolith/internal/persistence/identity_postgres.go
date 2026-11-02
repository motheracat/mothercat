package persistence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	app "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
)

type PostgresRepository struct {
	db *sql.DB
}

var _ app.Repository = (*PostgresRepository)(nil)

func NewIdentityRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindByStaffID(ctx context.Context, staffID string) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	record, err := scanAccount(r.db.QueryRowContext(ctx, `
		SELECT id, staff_id, email, display_name, password_hash, is_active, must_change_password
		FROM identity.user_accounts WHERE staff_id = $1 AND suspended_at IS NULL
	`, staffID))
	if errors.Is(err, sql.ErrNoRows) {
		return app.AccountRecord{}, app.ErrAccountNotFound
	}
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("find account by staff ID: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) FindBySessionToken(ctx context.Context, token string) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	record, err := scanAccount(r.db.QueryRowContext(ctx, `
		SELECT a.id, a.staff_id, a.email, a.display_name, a.password_hash, a.is_active, a.must_change_password
		FROM identity.sessions s
		JOIN identity.user_accounts a ON a.id = s.account_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL
		  AND s.expires_at > now() AND a.is_active AND a.suspended_at IS NULL
	`, hashToken(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return app.AccountRecord{}, app.ErrSessionNotFound
	}
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("find account by session token: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) FindByAccountID(ctx context.Context, accountID string) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	record, err := scanAccount(r.db.QueryRowContext(ctx, `
		SELECT id, staff_id, email, display_name, password_hash, is_active, must_change_password
		FROM identity.user_accounts WHERE id = $1::uuid AND suspended_at IS NULL
	`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return app.AccountRecord{}, app.ErrAccountNotFound
	}
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("find account by account ID: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) CreateInactiveAccount(ctx context.Context, staffID, email, passwordHash, displayName string) (string, error) {
	if err := r.available(); err != nil {
		return "", err
	}
	accountID, err := newUUID()
	if err != nil {
		return "", err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO identity.user_accounts
			(id, staff_id, email, display_name, password_hash, is_active, must_change_password)
		VALUES ($1::uuid, $2, $3, $4, $5, false, false)
	`, accountID, staffID, email, displayName, passwordHash)
	if err == nil {
		return accountID, nil
	}
	if !isUniqueViolation(err) {
		return "", fmt.Errorf("insert inactive account: %w", err)
	}
	existing, findErr := r.FindByStaffID(ctx, staffID)
	if findErr == nil && !existing.IsActive && existing.Email == email && existing.DisplayName == displayName {
		return existing.ID, nil
	}
	if findErr != nil && !errors.Is(findErr, app.ErrAccountNotFound) {
		return "", findErr
	}
	return "", app.ErrAccountConflict
}

func (r *PostgresRepository) BootstrapSuperAdmin(ctx context.Context, account app.AccountRecord) error {
	if err := r.available(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin administrator bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.user_accounts)`,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check bootstrap account state: %w", err)
	}
	if exists {
		return app.ErrBootstrapClosed
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO identity.bootstrap_state (key, state, account_id, completed_at)
		VALUES ('initial_super_admin', 'completed', $1::uuid, now())
		ON CONFLICT (key) DO NOTHING
	`, account.ID)
	if err != nil {
		return fmt.Errorf("close administrator bootstrap: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check administrator bootstrap state: %w", err)
	}
	if inserted != 1 {
		return app.ErrBootstrapClosed
	}
	var roleExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM access.roles WHERE role_key = 'super_admin' AND is_system_role)`,
	).Scan(&roleExists); err != nil {
		return fmt.Errorf("check SuperAdmin role: %w", err)
	}
	if !roleExists {
		return errors.New("SuperAdmin role is not initialized; run PostgreSQL migrations")
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO identity.user_accounts
			(id, staff_id, email, display_name, password_hash, is_active, must_change_password)
		VALUES ($1::uuid, $2, $3, $4, $5, true, false)
	`, account.ID, account.StaffID, account.Email, account.DisplayName, account.PasswordHash)
	if isUniqueViolation(err) {
		return app.ErrAccountConflict
	}
	if err != nil {
		return fmt.Errorf("create initial administrator: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO access.user_roles (user_id, role_key)
		VALUES ($1::uuid, 'super_admin')
	`, account.ID); err != nil {
		return fmt.Errorf("assign SuperAdmin role: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type, resource_id
		) VALUES (gen_random_uuid(), $1, 'system', 'system:bootstrap',
			'administrator.bootstrap.completed', 'identity_account', $2)
	`, "bootstrap:"+account.ID, account.ID); err != nil {
		return fmt.Errorf("record administrator bootstrap audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit administrator bootstrap: %w", err)
	}
	return nil
}

func (r *PostgresRepository) SuspendAccount(ctx context.Context, staffID, actorID string) error {
	if err := r.available(); err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin account suspension: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(761039123)`); err != nil {
		return fmt.Errorf("lock account suspension state: %w", err)
	}
	var accountID string
	var active, suspended, superAdmin bool
	err = tx.QueryRowContext(ctx, `
		SELECT a.id, a.is_active, a.suspended_at IS NOT NULL,
		       EXISTS (
		           SELECT 1 FROM access.user_roles ur
		           WHERE ur.user_id = a.id AND ur.role_key = 'super_admin'
		       )
		FROM identity.user_accounts a
		WHERE a.staff_id = $1
		FOR UPDATE
	`, staffID).Scan(&accountID, &active, &suspended, &superAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrAccountNotFound
	}
	if err != nil {
		return fmt.Errorf("load account for suspension: %w", err)
	}
	if !active || suspended {
		return app.ErrAccountStatusConflict
	}
	if superAdmin {
		var remaining int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*)
			FROM identity.user_accounts a
			JOIN access.user_roles ur ON ur.user_id = a.id AND ur.role_key = 'super_admin'
			WHERE a.id <> $1::uuid AND a.is_active AND a.suspended_at IS NULL
		`, accountID).Scan(&remaining); err != nil {
			return fmt.Errorf("check remaining active SuperAdmins: %w", err)
		}
		if remaining == 0 {
			return app.ErrLastActiveSuperAdmin
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.user_accounts
		SET suspended_at = now(), suspended_by = $2::uuid, updated_at = now()
		WHERE id = $1::uuid AND is_active AND suspended_at IS NULL
	`, accountID, actorID); err != nil {
		return fmt.Errorf("mark account suspended: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.sessions SET revoked_at = now()
		WHERE account_id = $1::uuid AND revoked_at IS NULL
	`, accountID); err != nil {
		return fmt.Errorf("revoke sessions for suspended account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.recovery_tokens SET used_at = now()
		WHERE account_id = $1::uuid AND used_at IS NULL
	`, accountID); err != nil {
		return fmt.Errorf("invalidate recovery tokens for suspended account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type, resource_id
		) VALUES ($1::uuid, $2, 'user', $3, 'identity.account.suspended', 'identity_account', $4)
	`, eventID, "identity:suspend:"+eventID, actorID, accountID); err != nil {
		return fmt.Errorf("audit account suspension: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account suspension: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ReactivateAccount(ctx context.Context, staffID, actorID string) error {
	if err := r.available(); err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin account reactivation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(761039123)`); err != nil {
		return fmt.Errorf("lock account suspension state: %w", err)
	}
	var accountID string
	var active, suspended bool
	err = tx.QueryRowContext(ctx, `
		SELECT id, is_active, suspended_at IS NOT NULL
		FROM identity.user_accounts
		WHERE staff_id = $1
		FOR UPDATE
	`, staffID).Scan(&accountID, &active, &suspended)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrAccountNotFound
	}
	if err != nil {
		return fmt.Errorf("load account for reactivation: %w", err)
	}
	if !active || !suspended {
		return app.ErrAccountStatusConflict
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.user_accounts
		SET suspended_at = NULL, suspended_by = NULL, updated_at = now()
		WHERE id = $1::uuid AND is_active AND suspended_at IS NOT NULL
	`, accountID); err != nil {
		return fmt.Errorf("clear account suspension: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type, resource_id
		) VALUES ($1::uuid, $2, 'user', $3, 'identity.account.reactivated', 'identity_account', $4)
	`, eventID, "identity:reactivate:"+eventID, actorID, accountID); err != nil {
		return fmt.Errorf("audit account reactivation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account reactivation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, accountID, staffID, token string, expiresAt time.Time) error {
	if err := r.available(); err != nil {
		return err
	}
	sessionID, err := newUUID()
	if err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var accountExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.user_accounts WHERE id = $1::uuid)`, accountID,
	).Scan(&accountExists); err != nil {
		return fmt.Errorf("check session account: %w", err)
	}
	if !accountExists {
		return app.ErrAccountNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO identity.sessions (id, account_id, token_hash, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, sessionID, accountID, hashToken(token), expiresAt); err != nil {
		if isUniqueViolation(err) {
			return app.ErrAccountConflict
		}
		return fmt.Errorf("persist login session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO identity.authentication_events (id, account_id, event_type, staff_id, successful)
		VALUES ($1::uuid, $2::uuid, 'login', $3, true)
	`, eventID, accountID, staffID); err != nil {
		return fmt.Errorf("record successful login: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session creation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, token string) error {
	if err := r.available(); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE identity.sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, hashToken(token))
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RecordAuthenticationEvent(ctx context.Context, staffID string, accountID *string, successful bool) error {
	if err := r.available(); err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO identity.authentication_events (id, account_id, event_type, staff_id, successful)
		VALUES ($1::uuid, $2::uuid, 'login', $3, $4)
	`, eventID, nullableUUID(accountID), staffID, successful)
	if err != nil {
		return fmt.Errorf("record authentication event: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateRecoveryToken(ctx context.Context, accountID, purpose, tokenHash string, expiresAt time.Time) error {
	if err := r.available(); err != nil {
		return err
	}
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recovery-token creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var accountExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.user_accounts WHERE id = $1::uuid)`, accountID,
	).Scan(&accountExists); err != nil {
		return fmt.Errorf("check recovery account: %w", err)
	}
	if !accountExists {
		return app.ErrAccountNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.recovery_tokens SET used_at = now()
		WHERE account_id = $1::uuid AND purpose = $2 AND used_at IS NULL
	`, accountID, purpose); err != nil {
		return fmt.Errorf("invalidate older recovery tokens: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO identity.recovery_tokens (id, account_id, token_hash, purpose, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5)
	`, tokenID, accountID, tokenHash, purpose, expiresAt); err != nil {
		return fmt.Errorf("persist recovery token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery-token creation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CompleteRecovery(ctx context.Context, tokenHash, purpose, passwordHash string, activate bool) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	eventID, err := newUUID()
	if err != nil {
		return app.AccountRecord{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("begin credential recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var accountID string
	err = tx.QueryRowContext(ctx, `
		UPDATE identity.recovery_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
		RETURNING account_id
	`, tokenHash, purpose).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return app.AccountRecord{}, app.ErrInvalidRecoveryToken
	}
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("claim recovery token: %w", err)
	}
	query := `
		UPDATE identity.user_accounts
		SET password_hash = $2, must_change_password = false, updated_at = now()
		WHERE id = $1::uuid
	`
	if activate {
		query = `
			UPDATE identity.user_accounts
			SET password_hash = $2, must_change_password = false, is_active = true, updated_at = now()
			WHERE id = $1::uuid AND is_active = false AND suspended_at IS NULL
		`
	} else {
		query += ` AND suspended_at IS NULL`
	}
	result, err := tx.ExecContext(ctx, query, accountID, passwordHash)
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("update recovered account credentials: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("check recovered account update: %w", err)
	}
	if updated != 1 {
		return app.AccountRecord{}, app.ErrInvalidRecoveryToken
	}
	var record app.AccountRecord
	record, err = scanAccount(tx.QueryRowContext(ctx, `
		SELECT id, staff_id, email, display_name, password_hash, is_active, must_change_password
		FROM identity.user_accounts WHERE id = $1::uuid
	`, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return app.AccountRecord{}, app.ErrInvalidRecoveryToken
	}
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("load recovered account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.recovery_tokens SET used_at = now()
		WHERE account_id = $1::uuid AND purpose = $2 AND used_at IS NULL
	`, accountID, purpose); err != nil {
		return app.AccountRecord{}, fmt.Errorf("invalidate remaining recovery tokens: %w", err)
	}
	if purpose == "password_reset" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE identity.sessions SET revoked_at = now()
			WHERE account_id = $1::uuid AND revoked_at IS NULL
		`, accountID); err != nil {
			return app.AccountRecord{}, fmt.Errorf("revoke sessions after password reset: %w", err)
		}
	}
	eventType := "password_reset"
	if activate {
		eventType = "account_activated"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO identity.authentication_events (id, account_id, event_type, staff_id, successful)
		VALUES ($1::uuid, $2::uuid, $3, $4, true)
	`, eventID, accountID, eventType, record.StaffID); err != nil {
		return app.AccountRecord{}, fmt.Errorf("record credential recovery: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return app.AccountRecord{}, fmt.Errorf("commit credential recovery: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) ChangePassword(ctx context.Context, sessionToken, passwordHash string) error {
	if err := r.available(); err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin password change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var accountID string
	var staffID string
	err = tx.QueryRowContext(ctx, `
		SELECT a.id, a.staff_id
		FROM identity.sessions s JOIN identity.user_accounts a ON a.id = s.account_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND a.is_active
		FOR UPDATE OF a
	`, hashToken(sessionToken)).Scan(&accountID, &staffID)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("find session for password change: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.user_accounts
		SET password_hash = $2, must_change_password = false, updated_at = now()
		WHERE id = $1::uuid
	`, accountID, passwordHash); err != nil {
		return fmt.Errorf("update account password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE identity.sessions SET revoked_at = now()
		WHERE account_id = $1::uuid AND revoked_at IS NULL
	`, accountID); err != nil {
		return fmt.Errorf("revoke sessions after password change: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO identity.authentication_events (id, account_id, event_type, staff_id, successful)
		VALUES ($1::uuid, $2::uuid, 'password_change', $3, true)
	`, eventID, accountID, staffID); err != nil {
		return fmt.Errorf("record password change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row rowScanner) (app.AccountRecord, error) {
	var record app.AccountRecord
	err := row.Scan(
		&record.ID, &record.StaffID, &record.Email, &record.DisplayName,
		&record.PasswordHash, &record.IsActive, &record.MustChangePassword,
	)
	return record, err
}

func (r *PostgresRepository) available() error {
	if r == nil || r.db == nil {
		return errors.New("PostgreSQL repository unavailable")
	}
	return nil
}

func nullableUUID(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func isUniqueViolation(err error) bool {
	type sqlStateError interface {
		SQLState() string
	}
	var stateErr sqlStateError
	return errors.As(err, &stateErr) && stateErr.SQLState() == "23505"
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
