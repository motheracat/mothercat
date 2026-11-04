package domain

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrNotFound = errors.New("audit event not found")

type EventFilter struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}

type EventRecord struct {
	ID           string          `json:"id"`
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id"`
	ActorName    string          `json:"actor_name,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	ResourceName string          `json:"resource_name,omitempty"`
	OccurredAt   time.Time       `json:"occurred_at"`
	Details      json.RawMessage `json:"details"`
}
