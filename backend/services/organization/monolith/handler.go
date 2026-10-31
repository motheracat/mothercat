package monolith

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	app "github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/domain"
	organizationhttp "github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/http"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Principal struct {
	ID          string
	StaffID     string
	Permissions []string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type AuditEvent struct {
	EventKey   string
	ActorID    string
	Action     string
	ResourceID string
	Details    map[string]any
}

type AuditRecorder interface {
	Record(context.Context, AuditEvent) error
}

type AccountProvisioner interface {
	ProvisionAccount(context.Context, string, string, string) (string, error)
	SendActivation(context.Context, string) error
}

type StaffRoleAssigner interface {
	AssignDefaultStaffRole(context.Context, string) error
}

type staffStore struct {
	db *mongo.Database
}

func NewHandler(
	db *mongo.Database,
	authenticator Authenticator,
	audit AuditRecorder,
	identity AccountProvisioner,
	access StaffRoleAssigner,
	logger *slog.Logger,
) http.Handler {
	store := &staffStore{db: db}
	onboarding := app.OnboardingServices{
		Identity: identity, Access: access,
	}
	return organizationhttp.NewHandler(store, authBridge{authenticator}, auditBridge{audit}, logger, onboarding)
}

type authBridge struct{ Authenticator }

func (b authBridge) Authenticate(ctx context.Context, token string) (app.Principal, error) {
	principal, err := b.Authenticator.Authenticate(ctx, token)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return app.Principal{}, app.ErrUnauthenticated
		}
		return app.Principal{}, err
	}
	return app.Principal{ID: principal.ID, StaffID: principal.StaffID, Permissions: principal.Permissions}, nil
}

type auditBridge struct{ AuditRecorder }

func (b auditBridge) Record(ctx context.Context, event app.AuditEvent) error {
	return b.AuditRecorder.Record(ctx, AuditEvent{
		EventKey: event.EventKey, ActorID: event.ActorID, Action: event.Action,
		ResourceID: event.ResourceID, Details: event.Details,
	})
}

func (s *staffStore) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("MongoDB is unavailable")
	}
	return s.db.Client().Ping(ctx, nil)
}

func (s *staffStore) Create(ctx context.Context, id string, request domain.CreateStaffRequest) (domain.Staff, error) {
	staff := domain.Staff{
		ID: id, StaffID: request.StaffID, DepartmentID: request.DepartmentID,
		FullName: request.FullName, WorkEmail: request.WorkEmail, JobTitle: request.JobTitle,
		EmploymentStatus: "pending",
	}
	_, err := s.db.Collection("staff_members").InsertOne(ctx, staff)
	if mongo.IsDuplicateKeyError(err) {
		return domain.Staff{}, domain.ErrDuplicateStaffID
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("insert staff record: %w", err)
	}
	return staff, nil
}

func (s *staffStore) FindByID(ctx context.Context, id string) (domain.Staff, error) {
	var staff domain.Staff
	err := s.db.Collection("staff_members").FindOne(ctx, bson.M{"id": id}).Decode(&staff)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Staff{}, domain.ErrStaffNotFound
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("find staff record: %w", err)
	}
	return staff, nil
}

func (s *staffStore) CompleteProvisioning(ctx context.Context, staffID, accountID string) (domain.Staff, error) {
	filter := bson.M{
		"id": staffID, "employment_status": "pending",
		"$or": bson.A{bson.M{"account_id": bson.M{"$exists": false}}, bson.M{"account_id": nil}, bson.M{"account_id": accountID}},
	}
	update := bson.M{"$set": bson.M{
		"account_id": accountID, "employment_status": "active", "updated_at": time.Now().UTC(),
	}}
	var staff domain.Staff
	err := s.db.Collection("staff_members").FindOneAndUpdate(
		ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&staff)
	if errors.Is(err, mongo.ErrNoDocuments) {
		existing, findErr := s.FindByID(ctx, staffID)
		if findErr != nil {
			return domain.Staff{}, findErr
		}
		if existing.EmploymentStatus == "active" && existing.AccountID != nil && *existing.AccountID == accountID {
			return existing, nil
		}
		return domain.Staff{}, domain.ErrStaffAlreadyProvisioned
	}
	if err != nil {
		return domain.Staff{}, fmt.Errorf("complete staff provisioning: %w", err)
	}
	return staff, nil
}

func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return errors.New("MongoDB database is unavailable")
	}
	_, err := db.Collection("staff_members").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "staff_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "account_id", Value: 1}}, Options: options.Index().SetSparse(true)},
	})
	if err != nil {
		return fmt.Errorf("create staff indexes: %w", err)
	}
	return nil
}
