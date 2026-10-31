package monolith

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Event struct {
	EventKey   string
	ActorID    string
	Action     string
	ResourceID string
	Details    map[string]any
}

type Recorder struct {
	db *mongo.Database
}

func NewRecorder(db *mongo.Database) *Recorder {
	return &Recorder{db: db}
}

func (r *Recorder) Record(ctx context.Context, event Event) error {
	if r == nil || r.db == nil {
		return errors.New("MongoDB is unavailable")
	}
	document := bson.M{
		"event_key": event.EventKey, "actor_type": "user", "actor_id": event.ActorID,
		"action": event.Action, "resource_type": "staff_member", "resource_id": event.ResourceID,
		"occurred_at": time.Now().UTC(), "details": event.Details,
	}
	_, err := r.db.Collection("audit_events").InsertOne(ctx, document)
	if mongo.IsDuplicateKeyError(err) {
		var existing bson.M
		if findErr := r.db.Collection("audit_events").FindOne(ctx, bson.M{"event_key": event.EventKey}).Decode(&existing); findErr != nil {
			return fmt.Errorf("verify idempotent audit event: %w", findErr)
		}
		if existing["action"] != event.Action || existing["resource_id"] != event.ResourceID || existing["actor_id"] != event.ActorID {
			return errors.New("idempotency key conflicts with an existing audit event")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("persist audit event: %w", err)
	}
	return nil
}

func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return errors.New("MongoDB database is unavailable")
	}
	_, err := db.Collection("audit_events").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "event_key", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "resource_type", Value: 1}, {Key: "resource_id", Value: 1}, {Key: "occurred_at", Value: -1}}},
	})
	if err != nil {
		return fmt.Errorf("create audit-event indexes: %w", err)
	}
	return nil
}
