package monolith

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

type Reader struct {
	db *sql.DB
}

func NewReader(db *sql.DB) *Reader {
	return &Reader{db: db}
}

func (r *Reader) List(ctx context.Context, filter domain.EventFilter) ([]domain.EventRecord, int, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("PostgreSQL database is unavailable")
	}
	where, args := eventWhere(filter)
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT count(*) FROM audit.audit_events"+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit events: %w", err)
	}

	pageArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, actor_type, actor_id, action, resource_type, resource_id,
		       occurred_at, details
		FROM audit.audit_events`+where+fmt.Sprintf(`
		ORDER BY occurred_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.EventRecord, 0)
	for rows.Next() {
		var event domain.EventRecord
		if err := rows.Scan(
			&event.ID, &event.ActorType, &event.ActorID, &event.Action,
			&event.ResourceType, &event.ResourceID, &event.OccurredAt, &event.Details,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, total, nil
}

func (r *Reader) Get(ctx context.Context, id string) (domain.EventRecord, error) {
	if r == nil || r.db == nil {
		return domain.EventRecord{}, errors.New("PostgreSQL database is unavailable")
	}
	var event domain.EventRecord
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, actor_type, actor_id, action, resource_type, resource_id,
		       occurred_at, details
		FROM audit.audit_events
		WHERE id = $1::uuid
	`, id).Scan(
		&event.ID, &event.ActorType, &event.ActorID, &event.Action,
		&event.ResourceType, &event.ResourceID, &event.OccurredAt, &event.Details,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EventRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.EventRecord{}, fmt.Errorf("get audit event: %w", err)
	}
	return event, nil
}

func eventWhere(filter domain.EventFilter) (string, []any) {
	conditions := make([]string, 0, 6)
	args := make([]any, 0, 6)
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if filter.ActorID != "" {
		add("actor_id = $%d", filter.ActorID)
	}
	if filter.Action != "" {
		add("action = $%d", filter.Action)
	}
	if filter.ResourceType != "" {
		add("resource_type = $%d", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		add("resource_id = $%d", filter.ResourceID)
	}
	if filter.From != nil {
		add("occurred_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("occurred_at <= $%d", *filter.To)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
