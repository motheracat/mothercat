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
		SELECT ua.id, ua.staff_id, ua.email, ua.display_name, ua.is_active, ua.must_change_password
		FROM identity.user_accounts ua
		JOIN identity.user_sessions us ON us.account_id = ua.id
		WHERE us.session_token_hash = $1 AND us.revoked_at IS NULL AND us.expires_at > NOW() AND ua.is_active = TRUE
		LIMIT 1
	`, hashSessionToken(token)).Scan(
		&record.ID,
		&record.StaffID,
		&record.Email,
		&record.DisplayName,
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

func hashSessionToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
