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

const eventSelect = `
	SELECT e.id::text, e.actor_type, e.actor_id,
	       COALESCE(NULLIF(actor_staff.full_name, ''), NULLIF(actor_account.display_name, ''),
	                NULLIF(actor_staff_id.full_name, ''),
	                CASE WHEN e.actor_type = 'system' THEN 'System' END, ''),
	       e.action, e.resource_type, e.resource_id,
	       COALESCE(NULLIF(resource_staff.full_name, ''), NULLIF(resource_account_staff.full_name, ''),
	                NULLIF(resource_account.display_name, ''), NULLIF(resource_role.display_name, ''),
	                NULLIF(resource_department.name, ''), ''),
	       e.occurred_at, e.details
	FROM audit.audit_events e
	LEFT JOIN identity.user_accounts actor_account
	       ON actor_account.id::text = e.actor_id
	LEFT JOIN organization.staff_members actor_staff
	       ON actor_staff.account_id = actor_account.id
	LEFT JOIN organization.staff_members actor_staff_id
	       ON actor_staff_id.staff_id = e.actor_id
	LEFT JOIN identity.user_accounts resource_account
	       ON e.resource_type IN ('identity_account', 'user')
	      AND resource_account.id::text = e.resource_id
	LEFT JOIN organization.staff_members resource_account_staff
	       ON resource_account_staff.account_id = resource_account.id
	LEFT JOIN organization.staff_members resource_staff
	       ON e.resource_type = 'staff_member'
	      AND (resource_staff.id::text = e.resource_id OR resource_staff.staff_id = e.resource_id)
	LEFT JOIN access.roles resource_role
	       ON e.resource_type = 'role' AND resource_role.role_key = e.resource_id
	LEFT JOIN organization.departments resource_department
	       ON e.resource_type = 'department' AND resource_department.id::text = e.resource_id
`

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
		"SELECT count(*) FROM audit.audit_events e"+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit events: %w", err)
	}

	pageArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx, eventSelect+where+fmt.Sprintf(`
		ORDER BY e.occurred_at DESC, e.id DESC
		LIMIT $%d OFFSET $%d`, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.EventRecord, 0)
	for rows.Next() {
		var event domain.EventRecord
		if err := rows.Scan(
			&event.ID, &event.ActorType, &event.ActorID, &event.ActorName,
			&event.Action, &event.ResourceType, &event.ResourceID, &event.ResourceName,
			&event.OccurredAt, &event.Details,
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
	err := r.db.QueryRowContext(ctx, eventSelect+`
		WHERE e.id = $1::uuid
	`, id).Scan(
		&event.ID, &event.ActorType, &event.ActorID, &event.ActorName, &event.Action,
		&event.ResourceType, &event.ResourceID, &event.ResourceName, &event.OccurredAt, &event.Details,
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
		add("e.actor_id = $%d", filter.ActorID)
	}
	if filter.Action != "" {
		add("e.action = $%d", filter.Action)
	}
	if filter.ResourceType != "" {
		add("e.resource_type = $%d", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		add("e.resource_id = $%d", filter.ResourceID)
	}
	if filter.From != nil {
		add("e.occurred_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("e.occurred_at <= $%d", *filter.To)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
