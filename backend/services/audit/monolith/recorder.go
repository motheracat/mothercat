package monolith

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Event struct {
	EventKey   string
	ActorID    string
	Action     string
	ResourceID string
	Details    map[string]any
}

type Recorder struct {
	db *sql.DB
}

func NewRecorder(db *sql.DB) *Recorder {
	return &Recorder{db: db}
}

func (r *Recorder) Record(ctx context.Context, event Event) error {
	if r == nil || r.db == nil {
		return errors.New("PostgreSQL database is unavailable")
	}
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type,
			resource_id, occurred_at, details
		) VALUES (gen_random_uuid(), $1, 'user', $2, $3, 'staff_member', $4, $5, $6::jsonb)
		ON CONFLICT (event_key) DO NOTHING
	`, event.EventKey, event.ActorID, event.Action, event.ResourceID, time.Now().UTC(), details)
	if err != nil {
		return fmt.Errorf("persist audit event: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check audit event insert: %w", err)
	}
	if inserted == 1 {
		return nil
	}
	var action, resourceID, actorID string
	if err := r.db.QueryRowContext(ctx, `
		SELECT action, resource_id, actor_id
		FROM audit.audit_events WHERE event_key = $1
	`, event.EventKey).Scan(&action, &resourceID, &actorID); err != nil {
		return fmt.Errorf("verify idempotent audit event: %w", err)
	}
	if action != event.Action || resourceID != event.ResourceID || actorID != event.ActorID {
		return errors.New("idempotency key conflicts with an existing audit event")
	}
	return nil
}
