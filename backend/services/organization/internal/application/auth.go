package application

import (
	"context"
	"errors"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrAuthUnavailable = errors.New("identity_unavailable")
)

type Principal struct {
	ID          string   `json:"id"`
	StaffID     string   `json:"staff_id"`
	Permissions []string `json:"permissions"`
}

type requestIDContextKey struct{}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type AuditEvent struct {
	EventKey   string         `json:"event_key"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	ResourceID string         `json:"resource_id"`
	Details    map[string]any `json:"details"`
}

type AuditRecorder interface {
	Record(context.Context, AuditEvent) error
}
