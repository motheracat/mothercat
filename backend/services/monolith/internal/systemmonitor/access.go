package systemmonitor

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	maxGrantDuration = 30 * 24 * time.Hour
	accessLockID     = 761039124
)

var (
	ErrTargetUnavailable = errors.New("System Monitor target account is unavailable")
	ErrNoActiveGrant     = errors.New("account has no active System Monitor grant")
	ErrInvalidGrant      = errors.New("invalid System Monitor access grant")
)

type GrantRequest struct {
	AccountID string
	Operator  string
	Reason    string
	ExpiresAt time.Time
}

type RevokeRequest struct {
	AccountID string
	Operator  string
	Reason    string
}

func HasAccess(ctx context.Context, db *sql.DB, accountID string) (bool, error) {
	if db == nil {
		return false, errors.New("System Monitor database is unavailable")
	}
	var allowed bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM system_monitor.access_grants g
			JOIN identity.user_accounts a ON a.id = g.account_id
			WHERE g.account_id = $1::uuid
			  AND g.revoked_at IS NULL AND g.expires_at > now()
			  AND a.is_active AND a.suspended_at IS NULL
		)
	`, accountID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check System Monitor access grant: %w", err)
	}
	return allowed, nil
}

func GrantAccess(ctx context.Context, db *sql.DB, request GrantRequest) error {
	if db == nil {
		return errors.New("System Monitor authorization database is unavailable")
	}
	if !validOperator(request.Operator) || !validReason(request.Reason) {
		return ErrInvalidGrant
	}
	now := time.Now()
	if !request.ExpiresAt.After(now) || request.ExpiresAt.After(now.Add(maxGrantDuration)) {
		return ErrInvalidGrant
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin System Monitor access grant: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAccessChanges(ctx, tx); err != nil {
		return err
	}

	var grantID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO system_monitor.access_grants (
			account_id, granted_by, grant_reason, expires_at
		)
		SELECT a.id, $2, $3, $4
		FROM identity.user_accounts a
		WHERE a.id = $1::uuid AND a.is_active AND a.suspended_at IS NULL
		  AND $4 > now()
		RETURNING id::text
	`, request.AccountID, request.Operator, request.Reason, request.ExpiresAt).Scan(&grantID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTargetUnavailable
	}
	if err != nil {
		return fmt.Errorf("insert System Monitor access grant: %w", err)
	}
	details, err := json.Marshal(map[string]any{
		"grant_id": grantID, "reason": request.Reason, "expires_at": request.ExpiresAt.UTC(),
	})
	if err != nil {
		return fmt.Errorf("encode System Monitor grant audit details: %w", err)
	}
	if err := recordGrantAudit(ctx, tx, request.Operator, "system_monitor.access.granted",
		request.AccountID, details); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit System Monitor access grant: %w", err)
	}
	return nil
}

func RevokeAccess(ctx context.Context, db *sql.DB, request RevokeRequest) error {
	if db == nil {
		return errors.New("System Monitor authorization database is unavailable")
	}
	if !validOperator(request.Operator) || !validReason(request.Reason) {
		return ErrInvalidGrant
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin System Monitor access revocation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAccessChanges(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		UPDATE system_monitor.access_grants
		SET revoked_at = now(), revoked_by = $2, revoke_reason = $3
		WHERE account_id = $1::uuid AND revoked_at IS NULL AND expires_at > now()
		RETURNING id::text
	`, request.AccountID, request.Operator, request.Reason)
	if err != nil {
		return fmt.Errorf("revoke System Monitor access grants: %w", err)
	}
	grantIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read revoked System Monitor grant: %w", err)
		}
		grantIDs = append(grantIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate revoked System Monitor grants: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close revoked System Monitor grants: %w", err)
	}
	if len(grantIDs) == 0 {
		return ErrNoActiveGrant
	}
	details, err := json.Marshal(map[string]any{
		"grant_ids": grantIDs, "reason": request.Reason,
	})
	if err != nil {
		return fmt.Errorf("encode System Monitor revocation audit details: %w", err)
	}
	if err := recordGrantAudit(ctx, tx, request.Operator, "system_monitor.access.revoked",
		request.AccountID, details); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit System Monitor access revocation: %w", err)
	}
	return nil
}

func lockAccessChanges(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, accessLockID); err != nil {
		return fmt.Errorf("lock System Monitor access changes: %w", err)
	}
	return nil
}

func recordGrantAudit(ctx context.Context, tx *sql.Tx, operator, action, accountID string, details []byte) error {
	eventKey, err := newEventKey()
	if err != nil {
		return fmt.Errorf("generate System Monitor access audit key: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type, resource_id, details
		) VALUES (gen_random_uuid(), $1, 'operator', $2, $3, 'system_monitor_access', $4, $5::jsonb)
	`, eventKey, operator, action, accountID, details); err != nil {
		return fmt.Errorf("record System Monitor access audit event: %w", err)
	}
	return nil
}

func newEventKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "system-monitor-access:" + hex.EncodeToString(value[:]), nil
}

func validOperator(operator string) bool {
	if operator != strings.TrimSpace(operator) || len(operator) == 0 || len(operator) > 200 {
		return false
	}
	for _, char := range operator {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func validReason(reason string) bool {
	if reason != strings.TrimSpace(reason) || len(reason) == 0 || len(reason) > 1000 {
		return false
	}
	for _, char := range reason {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return false
		}
	}
	return true
}
