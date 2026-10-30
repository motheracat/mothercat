package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type database interface {
	Begin(context.Context) (pgx.Tx, error)
}

// RecordInitialSuperAdmin writes one immutable, idempotent audit event for the
// bootstrap operation. The bootstrap process is recorded as the actor.
func RecordInitialSuperAdmin(ctx context.Context, db database, accountID, staffID string) error {
	if db == nil {
		return errors.New("Audit bootstrap database is unavailable")
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(staffID) == "" {
		return errors.New("Identity account ID and staff ID are required for the audit event")
	}
	eventKey := "identity:initial-super-admin:" + accountID
	details, err := json.Marshal(map[string]string{"staff_id": staffID})
	if err != nil {
		return fmt.Errorf("encode bootstrap audit details: %w", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Audit bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit.audit_events
			(event_key, actor_type, actor_id, action, resource_type, resource_id, details)
		VALUES ($1, 'bootstrap', NULL, 'initial_super_admin_created', 'identity_account', $2, $3)
		ON CONFLICT (event_key) DO NOTHING
	`, eventKey, accountID, details); err != nil {
		return fmt.Errorf("record initial SuperAdmin audit event: %w", err)
	}

	var storedAction, storedResourceType, storedResourceID, storedStaffID string
	if err := tx.QueryRow(ctx, `
		SELECT action, resource_type, resource_id, details->>'staff_id'
		FROM audit.audit_events
		WHERE event_key = $1
	`, eventKey).Scan(&storedAction, &storedResourceType, &storedResourceID, &storedStaffID); err != nil {
		return fmt.Errorf("verify initial SuperAdmin audit event: %w", err)
	}
	if storedAction != "initial_super_admin_created" ||
		storedResourceType != "identity_account" ||
		storedResourceID != accountID ||
		storedStaffID != staffID {
		return errors.New("existing initial SuperAdmin audit event conflicts with this bootstrap")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Audit bootstrap: %w", err)
	}
	return nil
}
