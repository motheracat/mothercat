package systemmonitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	maxControlDuration = 30 * 24 * time.Hour
	defaultPauseStatus = 503
	maxHistoryLimit     = 200
)

var (
	ErrInvalidEndpointControl  = errors.New("invalid endpoint control")
	ErrEndpointNotControllable = errors.New("endpoint cannot be paused")
	ErrEndpointNotPaused        = errors.New("endpoint is not paused")
)

type PauseRequest struct {
	Method          string
	Route           string
	ActorID         string
	Reason          string
	ResponseStatus  int
	ResponseMessage string
	ExpiresAt       time.Time
}

type ResumeRequest struct {
	Method  string
	Route   string
	ActorID string
	Reason  string
}

type ControlHistory struct {
	Method          string     `json:"method"`
	Route           string     `json:"route"`
	Action          string     `json:"action"`
	ActorID         *string    `json:"actor_id,omitempty"`
	ActorType       string     `json:"actor_type"`
	Reason          string     `json:"reason"`
	ResponseStatus  *int       `json:"response_status,omitempty"`
	ResponseMessage *string    `json:"response_message,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	ChangedAt       time.Time  `json:"changed_at"`
}

func (e *Engine) PauseEndpoint(ctx context.Context, db *sql.DB, request PauseRequest) error {
	endpoint, exists := e.catalog.Lookup(request.Method, request.Route)
	if !exists || !endpoint.Pausable {
		return ErrEndpointNotControllable
	}
	now := time.Now()
	if db == nil || !validOperator(request.ActorID) || !validReason(request.Reason) ||
		len(request.ResponseMessage) > 500 || !request.ExpiresAt.After(now) ||
		request.ExpiresAt.After(now.Add(maxControlDuration)) {
		return ErrInvalidEndpointControl
	}
	if strings.TrimSpace(request.ResponseMessage) == "" {
		request.ResponseMessage = "This endpoint is temporarily unavailable."
	}
	if request.ResponseStatus == 0 {
		request.ResponseStatus = defaultPauseStatus
	}
	if request.ResponseStatus < 400 || request.ResponseStatus > 599 {
		return ErrInvalidEndpointControl
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin endpoint pause: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		WITH expired AS (
			DELETE FROM system_monitor.endpoint_controls
			WHERE method = $1 AND route = $2 AND expires_at <= now()
			RETURNING method, route, reason, response_status, response_message, expires_at
		)
		INSERT INTO system_monitor.endpoint_control_history (
			method, route, action, actor_type, reason,
			response_status, response_message, expires_at
		)
		SELECT method, route, 'expired', 'system', reason,
		       response_status, response_message, expires_at
		FROM expired
	`, request.Method, request.Route)
	if err != nil {
		return fmt.Errorf("record expired endpoint pause before re-pause: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO system_monitor.endpoint_controls (
			method, route, paused_by, reason, response_status, response_message, expires_at
		) VALUES ($1, $2, $3::uuid, $4, $5, $6, $7)
		ON CONFLICT (method, route) DO UPDATE SET
			paused_by = EXCLUDED.paused_by,
			paused_at = now(),
			reason = EXCLUDED.reason,
			response_status = EXCLUDED.response_status,
			response_message = EXCLUDED.response_message,
			expires_at = EXCLUDED.expires_at
	`, request.Method, request.Route, request.ActorID, request.Reason, request.ResponseStatus,
		request.ResponseMessage, request.ExpiresAt)
	if err != nil {
		return fmt.Errorf("save endpoint pause: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO system_monitor.endpoint_control_history (
			method, route, action, actor_id, actor_type, reason,
			response_status, response_message, expires_at
		) VALUES ($1, $2, 'paused', $3::uuid, 'staff', $4, $5, $6, $7)
	`, request.Method, request.Route, request.ActorID, request.Reason, request.ResponseStatus,
		request.ResponseMessage, request.ExpiresAt)
	if err != nil {
		return fmt.Errorf("record endpoint pause history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit endpoint pause: %w", err)
	}
	e.InvalidateControlCache()
	return nil
}

func (e *Engine) ResumeEndpoint(ctx context.Context, db *sql.DB, request ResumeRequest) error {
	endpoint, exists := e.catalog.Lookup(request.Method, request.Route)
	if !exists || !endpoint.Pausable {
		return ErrEndpointNotControllable
	}
	if db == nil || !validOperator(request.ActorID) || !validReason(request.Reason) {
		return ErrInvalidEndpointControl
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin endpoint resume: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status int
	var message string
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		DELETE FROM system_monitor.endpoint_controls
		WHERE method = $1 AND route = $2 AND expires_at > now()
		RETURNING response_status, response_message, expires_at
	`, request.Method, request.Route).Scan(&status, &message, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEndpointNotPaused
	}
	if err != nil {
		return fmt.Errorf("remove endpoint pause: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO system_monitor.endpoint_control_history (
			method, route, action, actor_id, actor_type, reason,
			response_status, response_message, expires_at
		) VALUES ($1, $2, 'resumed', $3::uuid, 'staff', $4, $5, $6, $7)
	`, request.Method, request.Route, request.ActorID, request.Reason, status, message, expiresAt)
	if err != nil {
		return fmt.Errorf("record endpoint resume history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit endpoint resume: %w", err)
	}
	e.InvalidateControlCache()
	return nil
}

func EndpointControlHistory(ctx context.Context, db *sql.DB, limit int) ([]ControlHistory, error) {
	if db == nil || limit < 1 || limit > maxHistoryLimit {
		return nil, ErrInvalidEndpointControl
	}
	rows, err := db.QueryContext(ctx, `
		SELECT method, route, action, actor_id::text, actor_type, reason,
		       response_status, response_message, expires_at, changed_at
		FROM system_monitor.endpoint_control_history
		ORDER BY changed_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query endpoint control history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	history := make([]ControlHistory, 0)
	for rows.Next() {
		var item ControlHistory
		var actorID sql.NullString
		var status sql.NullInt64
		var message sql.NullString
		var expiresAt sql.NullTime
		if err := rows.Scan(&item.Method, &item.Route, &item.Action, &actorID, &item.ActorType,
			&item.Reason, &status, &message, &expiresAt, &item.ChangedAt); err != nil {
			return nil, fmt.Errorf("read endpoint control history: %w", err)
		}
		if actorID.Valid {
			item.ActorID = &actorID.String
		}
		if status.Valid {
			value := int(status.Int64)
			item.ResponseStatus = &value
		}
		if message.Valid {
			item.ResponseMessage = &message.String
		}
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate endpoint control history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close endpoint control history: %w", err)
	}
	return history, nil
}

func ExpireEndpointControls(ctx context.Context, db *sql.DB) (int64, error) {
	if db == nil {
		return 0, errors.New("System Monitor database is unavailable")
	}
	result, err := db.ExecContext(ctx, `
		WITH expired AS (
			DELETE FROM system_monitor.endpoint_controls
			WHERE expires_at <= now()
			RETURNING method, route, reason, response_status, response_message, expires_at
		)
		INSERT INTO system_monitor.endpoint_control_history (
			method, route, action, actor_type, reason,
			response_status, response_message, expires_at
		)
		SELECT method, route, 'expired', 'system', reason,
		       response_status, response_message, expires_at
		FROM expired
	`)
	if err != nil {
		return 0, fmt.Errorf("expire endpoint controls: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count expired endpoint controls: %w", err)
	}
	return count, nil
}

func RunEndpointControlExpiry(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	expire := func() {
		expired, err := ExpireEndpointControls(ctx, db)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(ctx, "expire endpoint pauses", "error", err)
			}
			return
		}
		if expired > 0 && logger != nil {
			logger.InfoContext(ctx, "expired endpoint pauses", "count", expired)
		}
	}
	expire()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expire()
		}
	}
}
