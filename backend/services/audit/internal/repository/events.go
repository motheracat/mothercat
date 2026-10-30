package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

func (repository *EventRepository) Ping(ctx context.Context) error {
	if repository == nil || repository.pool == nil {
		return errors.New("Audit database is unavailable")
	}
	return repository.pool.Ping(ctx)
}

func (repository *EventRepository) Record(ctx context.Context, event domain.Event) error {
	if repository == nil || repository.pool == nil {
		return errors.New("Audit database is unavailable")
	}
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("encode event details: %w", err)
	}
	_, err = repository.pool.Exec(ctx, `
		INSERT INTO audit.audit_events
			(event_key, actor_type, actor_id, action, resource_type, resource_id, details)
		VALUES ($1, 'user', $2, $3, 'staff_member', $4, $5)
		ON CONFLICT (event_key) DO NOTHING
	`, event.EventKey, event.ActorID, event.Action, event.ResourceID, details)
	if err != nil {
		return fmt.Errorf("persist audit event: %w", err)
	}
	var action, resourceID, actorID string
	if err := repository.pool.QueryRow(ctx, `
		SELECT action, resource_id, actor_id
		FROM audit.audit_events
		WHERE event_key = $1
	`, event.EventKey).Scan(&action, &resourceID, &actorID); err != nil {
		return fmt.Errorf("verify persisted audit event: %w", err)
	}
	if action != event.Action || resourceID != event.ResourceID || actorID != event.ActorID {
		return errors.New("idempotency key conflicts with an existing audit event")
	}
	return nil
}
