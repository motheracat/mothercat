package domain

type Event struct {
	EventKey   string         `json:"event_key"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	ResourceID string         `json:"resource_id"`
	Details    map[string]any `json:"details"`
}
