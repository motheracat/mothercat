package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
)

// Repository persists Identity auth records in PostgreSQL.
type Repository struct {
	db *pgxpool.Pool
}

func NewAccountRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByStaffID(ctx context.Context, staffID string) (app.AccountRecord, error) {
	if r == nil || r.db == nil {
		return app.AccountRecord{}, errors.New("repository unavailable")
	}
	var record app.AccountRecord
	if err := r.db.QueryRow(ctx, `
		SELECT id, staff_id, email, display_name, password_hash, is_active, must_change_password
		FROM identity.user_accounts
		WHERE staff_id = $1
		LIMIT 1
	`, staffID).Scan(
		&record.ID,
		&record.StaffID,
		&record.Email,
		&record.DisplayName,
		&record.PasswordHash,
		&record.IsActive,
		&record.MustChangePassword,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.AccountRecord{}, app.ErrAccountNotFound
		}
		return app.AccountRecord{}, err
	}
	return record, nil
}

func (r *Repository) FindBySessionToken(ctx context.Context, token string) (app.AccountRecord, error) {
	if r == nil || r.db == nil {
		return app.AccountRecord{}, errors.New("repository unavailable")
	}
	var record app.AccountRecord
	if err := r.db.QueryRow(ctx, `
		SELECT ua.id, ua.staff_id, ua.email, ua.display_name, ua.password_hash, ua.is_active, ua.must_change_password
		FROM identity.user_accounts ua
		JOIN identity.user_sessions us ON us.account_id = ua.id
		WHERE us.session_token_hash = $1 AND us.revoked_at IS NULL AND us.expires_at > NOW() AND ua.is_active = TRUE
		LIMIT 1
	`, hashSessionToken(token)).Scan(
		&record.ID,
		&record.StaffID,
		&record.Email,
		&record.DisplayName,
		&record.PasswordHash,
		&record.IsActive,
		&record.MustChangePassword,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.AccountRecord{}, app.ErrSessionNotFound
		}
		return app.AccountRecord{}, err
	}
	return record, nil
}

func (r *Repository) FindByAccountID(ctx context.Context, accountID string) (app.AccountRecord, error) {
	if r == nil || r.db == nil {
		return app.AccountRecord{}, errors.New("repository unavailable")
	}
	var record app.AccountRecord
	err := r.db.QueryRow(ctx, `
		SELECT id, staff_id, email, display_name, password_hash, is_active, must_change_password
		FROM identity.user_accounts
		WHERE id = $1
	`, accountID).Scan(
		&record.ID, &record.StaffID, &record.Email, &record.DisplayName,
		&record.PasswordHash, &record.IsActive, &record.MustChangePassword,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.AccountRecord{}, app.ErrAccountNotFound
	}
	if err != nil {
		return app.AccountRecord{}, err
	}
	return record, nil
}

func (r *Repository) CreateSession(ctx context.Context, accountID, staffID, token string, expiresAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("repository unavailable")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.user_sessions (account_id, session_token_hash, expires_at, revoked_at)
		VALUES ($1, $2, $3, NULL)
	`, accountID, hashSessionToken(token), expiresAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.authentication_events (account_id, event_type, staff_id, successful)
		VALUES ($1, 'login', $2, TRUE)
	`, accountID, staffID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RevokeSession(ctx context.Context, token string) error {
	if r == nil || r.db == nil {
		return errors.New("repository unavailable")
	}
	_, err := r.db.Exec(ctx, `
		UPDATE identity.user_sessions
		SET revoked_at = NOW()
		WHERE session_token_hash = $1 AND revoked_at IS NULL
	`, hashSessionToken(token))
	return err
}

func (r *Repository) RecordAuthenticationEvent(ctx context.Context, staffID string, accountID *string, successful bool) error {
	if r == nil || r.db == nil {
		return errors.New("repository unavailable")
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO identity.authentication_events (account_id, event_type, staff_id, successful)
		VALUES ($1, 'login', $2, $3)
	`, accountID, staffID, successful)
	return err
}

func (r *Repository) CreateRecoveryToken(ctx context.Context, accountID, purpose, tokenHash string, expiresAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("repository unavailable")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE identity.password_reset_tokens
		SET used_at = NOW()
		WHERE account_id = $1 AND purpose = $2 AND used_at IS NULL
	`, accountID, purpose); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.password_reset_tokens (account_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, $3, $4)
	`, accountID, tokenHash, purpose, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) CompleteRecovery(ctx context.Context, tokenHash, purpose, passwordHash string, activate bool) (app.AccountRecord, error) {
	if r == nil || r.db == nil {
		return app.AccountRecord{}, errors.New("repository unavailable")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return app.AccountRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var accountID string
	if err := tx.QueryRow(ctx, `
		SELECT account_id FROM identity.password_reset_tokens
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > NOW()
		FOR UPDATE
	`, tokenHash, purpose).Scan(&accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.AccountRecord{}, app.ErrInvalidRecoveryToken
		}
		return app.AccountRecord{}, err
	}

	var record app.AccountRecord
	err = tx.QueryRow(ctx, `
		UPDATE identity.user_accounts
		SET password_hash = $2, is_active = CASE WHEN $3 THEN TRUE ELSE is_active END,
		    must_change_password = FALSE, updated_at = NOW()
		WHERE id = $1 AND ($3 = FALSE OR is_active = FALSE)
		RETURNING id, staff_id, email, display_name, password_hash, is_active, must_change_password
	`, accountID, passwordHash, activate).Scan(
		&record.ID, &record.StaffID, &record.Email, &record.DisplayName,
		&record.PasswordHash, &record.IsActive, &record.MustChangePassword,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.AccountRecord{}, app.ErrInvalidRecoveryToken
	}
	if err != nil {
		return app.AccountRecord{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.password_reset_tokens SET used_at = NOW()
		WHERE account_id = $1 AND purpose = $2 AND used_at IS NULL
	`, accountID, purpose); err != nil {
		return app.AccountRecord{}, err
	}
	if purpose == "password_reset" {
		if _, err := tx.Exec(ctx, `
			UPDATE identity.user_sessions SET revoked_at = NOW()
			WHERE account_id = $1 AND revoked_at IS NULL
		`, accountID); err != nil {
			return app.AccountRecord{}, err
		}
	}
	eventType := "password_reset"
	if activate {
		eventType = "account_activated"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.authentication_events (account_id, event_type, staff_id, successful)
		VALUES ($1, $2, $3, TRUE)
	`, accountID, eventType, record.StaffID); err != nil {
		return app.AccountRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return app.AccountRecord{}, err
	}
	return record, nil
}

func (r *Repository) ChangePassword(ctx context.Context, sessionToken, passwordHash string) error {
	if r == nil || r.db == nil {
		return errors.New("repository unavailable")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var accountID, staffID string
	if err := tx.QueryRow(ctx, `
		SELECT ua.id, ua.staff_id
		FROM identity.user_accounts ua
		JOIN identity.user_sessions us ON us.account_id = ua.id
		WHERE us.session_token_hash = $1 AND us.revoked_at IS NULL
		  AND us.expires_at > NOW() AND ua.is_active = TRUE
		FOR UPDATE OF ua
	`, hashSessionToken(sessionToken)).Scan(&accountID, &staffID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrSessionNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.user_accounts SET password_hash = $2, must_change_password = FALSE, updated_at = NOW()
		WHERE id = $1
	`, accountID, passwordHash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.user_sessions SET revoked_at = NOW()
		WHERE account_id = $1 AND revoked_at IS NULL
	`, accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.authentication_events (account_id, event_type, staff_id, successful)
		VALUES ($1, 'password_change', $2, TRUE)
	`, accountID, staffID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func hashSessionToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
