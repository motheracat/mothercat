package persistence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	app "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	accountsCollection  = "identity_accounts"
	sessionsCollection  = "identity_sessions"
	recoveryCollection  = "identity_recovery_tokens"
	eventsCollection    = "identity_authentication_events"
	bootstrapCollection = "system_bootstrap_state"
)

// MongoRepository implements the Identity application's Repository contract.
// Account and related identifiers are UUID strings; MongoDB ObjectIDs are not
// exposed to the Identity application or routes.
type MongoRepository struct {
	db *mongo.Database
}

var _ app.Repository = (*MongoRepository)(nil)

func NewIdentityRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{db: db}
}

// EnsureIndexes creates the indexes required by the Identity repository.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return errors.New("MongoDB database is unavailable")
	}
	indexes := []struct {
		collection string
		models     []mongo.IndexModel
	}{
		{accountsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "staff_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{sessionsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "expires_at", Value: 1}}},
		}},
		{recoveryCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "purpose", Value: 1}, {Key: "expires_at", Value: 1}}},
		}},
		{eventsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "account_id", Value: 1}, {Key: "occurred_at", Value: -1}}},
			{Keys: bson.D{{Key: "staff_id", Value: 1}, {Key: "occurred_at", Value: -1}}},
		}},
	}
	for _, spec := range indexes {
		if _, err := db.Collection(spec.collection).Indexes().CreateMany(ctx, spec.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", spec.collection, err)
		}
	}
	return nil
}

type accountDocument struct {
	ID                 string    `bson:"_id"`
	StaffID            string    `bson:"staff_id"`
	Email              string    `bson:"email"`
	DisplayName        string    `bson:"display_name"`
	PasswordHash       string    `bson:"password_hash"`
	IsActive           bool      `bson:"is_active"`
	MustChangePassword bool      `bson:"must_change_password"`
	CreatedAt          time.Time `bson:"created_at"`
	UpdatedAt          time.Time `bson:"updated_at"`
}

type sessionDocument struct {
	ID        string     `bson:"_id"`
	AccountID string     `bson:"account_id"`
	TokenHash string     `bson:"token_hash"`
	ExpiresAt time.Time  `bson:"expires_at"`
	RevokedAt *time.Time `bson:"revoked_at,omitempty"`
	CreatedAt time.Time  `bson:"created_at"`
}

type recoveryDocument struct {
	ID        string     `bson:"_id"`
	AccountID string     `bson:"account_id"`
	TokenHash string     `bson:"token_hash"`
	Purpose   string     `bson:"purpose"`
	ExpiresAt time.Time  `bson:"expires_at"`
	UsedAt    *time.Time `bson:"used_at,omitempty"`
	CreatedAt time.Time  `bson:"created_at"`
}

type authenticationEvent struct {
	ID         string    `bson:"_id"`
	AccountID  *string   `bson:"account_id,omitempty"`
	EventType  string    `bson:"event_type"`
	StaffID    string    `bson:"staff_id"`
	Successful bool      `bson:"successful"`
	OccurredAt time.Time `bson:"occurred_at"`
}

func (r *MongoRepository) FindByStaffID(ctx context.Context, staffID string) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	var document accountDocument
	err := r.db.Collection(accountsCollection).FindOne(ctx, bson.M{"staff_id": staffID}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return app.AccountRecord{}, app.ErrAccountNotFound
	}
	if err != nil {
		return app.AccountRecord{}, err
	}
	return toRecord(document), nil
}

func (r *MongoRepository) FindBySessionToken(ctx context.Context, token string) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	var session sessionDocument
	err := r.db.Collection(sessionsCollection).FindOne(ctx, bson.M{
		"token_hash": hashToken(token),
		"revoked_at": bson.M{"$exists": false},
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return app.AccountRecord{}, app.ErrSessionNotFound
	}
	if err != nil {
		return app.AccountRecord{}, err
	}
	var account accountDocument
	err = r.db.Collection(accountsCollection).FindOne(ctx, bson.M{
		"_id": session.AccountID, "is_active": true,
	}).Decode(&account)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return app.AccountRecord{}, app.ErrSessionNotFound
	}
	if err != nil {
		return app.AccountRecord{}, err
	}
	return toRecord(account), nil
}

func (r *MongoRepository) FindByAccountID(ctx context.Context, accountID string) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	var document accountDocument
	err := r.db.Collection(accountsCollection).FindOne(ctx, bson.M{"_id": accountID}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return app.AccountRecord{}, app.ErrAccountNotFound
	}
	if err != nil {
		return app.AccountRecord{}, err
	}
	return toRecord(document), nil
}

func (r *MongoRepository) CreateInactiveAccount(ctx context.Context, staffID, email, passwordHash, displayName string) (string, error) {
	if err := r.available(); err != nil {
		return "", err
	}
	accountID, err := newUUID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	document := accountDocument{
		ID: accountID, StaffID: staffID, Email: email, PasswordHash: passwordHash,
		DisplayName: displayName, IsActive: false, CreatedAt: now, UpdatedAt: now,
	}
	_, err = r.db.Collection(accountsCollection).InsertOne(ctx, document)
	if err == nil {
		return accountID, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return "", err
	}

	var existing accountDocument
	findErr := r.db.Collection(accountsCollection).FindOne(ctx, bson.M{"staff_id": staffID}).Decode(&existing)
	if findErr == nil && !existing.IsActive && existing.Email == email && existing.DisplayName == displayName {
		return existing.ID, nil
	}
	if findErr != nil && !errors.Is(findErr, mongo.ErrNoDocuments) {
		return "", findErr
	}
	return "", app.ErrAccountConflict
}

func (r *MongoRepository) BootstrapSuperAdmin(ctx context.Context, account app.AccountRecord) error {
	if err := r.available(); err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		state, err := r.db.Collection(bootstrapCollection).UpdateOne(
			txCtx,
			bson.M{"_id": "initial_super_admin"},
			bson.M{"$setOnInsert": bson.M{
				"state": "completed", "account_id": account.ID, "completed_at": now,
			}},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return app.ErrBootstrapClosed
			}
			return err
		}
		if state.UpsertedCount != 1 {
			return app.ErrBootstrapClosed
		}
		accountCount, err := r.db.Collection(accountsCollection).CountDocuments(txCtx, bson.M{})
		if err != nil {
			return err
		}
		if accountCount != 0 {
			return app.ErrBootstrapClosed
		}
		if err := r.db.Collection("roles").FindOne(txCtx, bson.M{
			"role_key": "super_admin", "is_system_role": true,
		}).Err(); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return errors.New("SuperAdmin role is not initialized; run MongoDB setup")
			}
			return err
		}
		document := accountDocument{
			ID: account.ID, StaffID: account.StaffID, Email: account.Email,
			DisplayName: account.DisplayName, PasswordHash: account.PasswordHash,
			IsActive: true, MustChangePassword: false, CreatedAt: now, UpdatedAt: now,
		}
		if _, err := r.db.Collection(accountsCollection).InsertOne(txCtx, document); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return app.ErrAccountConflict
			}
			return err
		}
		if _, err := r.db.Collection("user_roles").InsertOne(txCtx, bson.M{
			"user_id": account.ID, "role_key": "super_admin", "assigned_at": now,
		}); err != nil {
			return err
		}
		eventID, err := newUUID()
		if err != nil {
			return err
		}
		_, err = r.db.Collection("audit_events").InsertOne(txCtx, bson.M{
			"_id": eventID, "event_key": "bootstrap:" + account.ID,
			"actor_type": "system", "actor_id": "system:bootstrap",
			"action": "administrator.bootstrap.completed", "resource_type": "identity_account",
			"resource_id": account.ID, "occurred_at": now,
		})
		return err
	})
}

func (r *MongoRepository) CreateSession(ctx context.Context, accountID, staffID, token string, expiresAt time.Time) error {
	if err := r.available(); err != nil {
		return err
	}
	sessionID, err := newUUID()
	if err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	session := sessionDocument{
		ID: sessionID, AccountID: accountID, TokenHash: hashToken(token),
		ExpiresAt: expiresAt, CreatedAt: now,
	}
	event := authenticationEvent{
		ID: eventID, AccountID: &accountID, EventType: "login",
		StaffID: staffID, Successful: true, OccurredAt: now,
	}
	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		var account accountDocument
		if err := r.db.Collection(accountsCollection).FindOne(txCtx, bson.M{"_id": accountID}).Decode(&account); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return app.ErrAccountNotFound
			}
			return err
		}
		if _, err := r.db.Collection(sessionsCollection).InsertOne(txCtx, session); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return app.ErrAccountConflict
			}
			return err
		}
		_, err := r.db.Collection(eventsCollection).InsertOne(txCtx, event)
		return err
	})
}

func (r *MongoRepository) RevokeSession(ctx context.Context, token string) error {
	if err := r.available(); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := r.db.Collection(sessionsCollection).UpdateOne(ctx,
		bson.M{"token_hash": hashToken(token), "revoked_at": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"revoked_at": now}},
	)
	return err
}

func (r *MongoRepository) RecordAuthenticationEvent(ctx context.Context, staffID string, accountID *string, successful bool) error {
	if err := r.available(); err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	event := authenticationEvent{
		ID: eventID, AccountID: accountID, EventType: "login", StaffID: staffID,
		Successful: successful, OccurredAt: time.Now().UTC(),
	}
	_, err = r.db.Collection(eventsCollection).InsertOne(ctx, event)
	return err
}

func (r *MongoRepository) CreateRecoveryToken(ctx context.Context, accountID, purpose, tokenHash string, expiresAt time.Time) error {
	if err := r.available(); err != nil {
		return err
	}
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	token := recoveryDocument{
		ID: tokenID, AccountID: accountID, TokenHash: tokenHash, Purpose: purpose,
		ExpiresAt: expiresAt, CreatedAt: time.Now().UTC(),
	}
	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		if err := r.db.Collection(accountsCollection).FindOne(txCtx, bson.M{"_id": accountID}).Err(); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return app.ErrAccountNotFound
			}
			return err
		}
		now := time.Now().UTC()
		if _, err := r.db.Collection(recoveryCollection).UpdateMany(txCtx,
			bson.M{"account_id": accountID, "purpose": purpose, "used_at": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"used_at": now}},
		); err != nil {
			return err
		}
		_, err := r.db.Collection(recoveryCollection).InsertOne(txCtx, token)
		return err
	})
}

func (r *MongoRepository) CompleteRecovery(ctx context.Context, tokenHash, purpose, passwordHash string, activate bool) (app.AccountRecord, error) {
	if err := r.available(); err != nil {
		return app.AccountRecord{}, err
	}
	eventID, err := newUUID()
	if err != nil {
		return app.AccountRecord{}, err
	}
	now := time.Now().UTC()
	var result app.AccountRecord
	err = r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		var recovery recoveryDocument
		err := r.db.Collection(recoveryCollection).FindOneAndUpdate(
			txCtx,
			bson.M{
				"token_hash": tokenHash, "purpose": purpose,
				"used_at": bson.M{"$exists": false}, "expires_at": bson.M{"$gt": now},
			},
			bson.M{"$set": bson.M{"used_at": now}},
			options.FindOneAndUpdate().SetReturnDocument(options.Before),
		).Decode(&recovery)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return app.ErrInvalidRecoveryToken
		}
		if err != nil {
			return err
		}

		filter := bson.M{"_id": recovery.AccountID}
		if activate {
			filter["is_active"] = false
		}
		update := bson.M{"$set": bson.M{
			"password_hash": passwordHash, "must_change_password": false, "updated_at": now,
		}}
		if activate {
			update["$set"].(bson.M)["is_active"] = true
		}
		var account accountDocument
		err = r.db.Collection(accountsCollection).FindOneAndUpdate(
			txCtx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&account)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return app.ErrInvalidRecoveryToken
		}
		if err != nil {
			return err
		}
		if _, err := r.db.Collection(recoveryCollection).UpdateMany(txCtx,
			bson.M{"account_id": recovery.AccountID, "purpose": purpose, "used_at": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"used_at": now}},
		); err != nil {
			return err
		}
		if purpose == "password_reset" {
			if _, err := r.db.Collection(sessionsCollection).UpdateMany(txCtx,
				bson.M{"account_id": recovery.AccountID, "revoked_at": bson.M{"$exists": false}},
				bson.M{"$set": bson.M{"revoked_at": now}},
			); err != nil {
				return err
			}
		}
		eventType := "password_reset"
		if activate {
			eventType = "account_activated"
		}
		event := authenticationEvent{
			ID: eventID, AccountID: &recovery.AccountID, EventType: eventType,
			StaffID: account.StaffID, Successful: true, OccurredAt: now,
		}
		if _, err := r.db.Collection(eventsCollection).InsertOne(txCtx, event); err != nil {
			return err
		}
		result = toRecord(account)
		return nil
	})
	if err != nil {
		return app.AccountRecord{}, err
	}
	return result, nil
}

func (r *MongoRepository) ChangePassword(ctx context.Context, sessionToken, passwordHash string) error {
	if err := r.available(); err != nil {
		return err
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		var session sessionDocument
		err := r.db.Collection(sessionsCollection).FindOne(txCtx, bson.M{
			"token_hash": hashToken(sessionToken),
			"revoked_at": bson.M{"$exists": false},
			"expires_at": bson.M{"$gt": now},
		}).Decode(&session)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return app.ErrSessionNotFound
		}
		if err != nil {
			return err
		}
		var account accountDocument
		err = r.db.Collection(accountsCollection).FindOneAndUpdate(
			txCtx,
			bson.M{"_id": session.AccountID, "is_active": true},
			bson.M{"$set": bson.M{
				"password_hash": passwordHash, "must_change_password": false, "updated_at": now,
			}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&account)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return app.ErrSessionNotFound
		}
		if err != nil {
			return err
		}
		if _, err := r.db.Collection(sessionsCollection).UpdateMany(txCtx,
			bson.M{"account_id": session.AccountID, "revoked_at": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"revoked_at": now}},
		); err != nil {
			return err
		}
		event := authenticationEvent{
			ID: eventID, AccountID: &session.AccountID, EventType: "password_change",
			StaffID: account.StaffID, Successful: true, OccurredAt: now,
		}
		_, err = r.db.Collection(eventsCollection).InsertOne(txCtx, event)
		return err
	})
}

func (r *MongoRepository) withTransaction(ctx context.Context, operation func(mongo.SessionContext) error) error {
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
		return nil, operation(txCtx)
	})
	return err
}

func (r *MongoRepository) available() error {
	if r == nil || r.db == nil {
		return errors.New("repository unavailable")
	}
	return nil
}

func toRecord(document accountDocument) app.AccountRecord {
	return app.AccountRecord{
		ID: document.ID, StaffID: document.StaffID, Email: document.Email,
		DisplayName: document.DisplayName, PasswordHash: document.PasswordHash,
		IsActive: document.IsActive, MustChangePassword: document.MustChangePassword,
	}
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
