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

type Authorization struct {
	Role        string
	Roles       []string
	Permissions []string
}

type Store struct {
	db *mongo.Database
}

type permissionDefinition struct {
	key         string
	description string
}

var permissionCatalog = []permissionDefinition{
	{key: "staff.read", description: "Search and view authorized staff profiles"},
	{key: "staff.create", description: "Create approved staff profiles"},
	{key: "staff.update", description: "Update permitted staff profile fields"},
	{key: "staff.deactivate", description: "Deactivate staff employment records"},
	{key: "staff.import", description: "Import and validate the official staff directory"},
	{key: "departments.manage", description: "Create and update departments"},
	{key: "roles.read", description: "View roles and permissions"},
	{key: "roles.manage", description: "Create roles and manage role permissions"},
	{key: "user_roles.assign", description: "Assign roles within the actor's delegation rights"},
	{key: "audit.read", description: "Search and view authorized audit events"},
	{key: "settings.manage", description: "Manage system settings"},
}

func NewStore(db *mongo.Database) *Store {
	return &Store{db: db}
}

func (s *Store) GetUserAuthorization(ctx context.Context, userID string) (Authorization, error) {
	if s == nil || s.db == nil {
		return Authorization{}, errors.New("MongoDB is unavailable")
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"user_id": userID}}},
		{{Key: "$lookup", Value: bson.M{
			"from": "roles", "localField": "role_key", "foreignField": "role_key", "as": "role",
		}}},
		{{Key: "$unwind", Value: "$role"}},
		{{Key: "$lookup", Value: bson.M{
			"from": "role_permissions", "localField": "role_key", "foreignField": "role_key", "as": "permissions",
		}}},
	}
	cursor, err := s.db.Collection("user_roles").Aggregate(ctx, pipeline)
	if err != nil {
		return Authorization{}, fmt.Errorf("aggregate user authorization: %w", err)
	}
	defer cursor.Close(ctx)
	type permission struct {
		Key string `bson:"permission_key"`
	}
	type result struct {
		Role struct {
			DisplayName string `bson:"display_name"`
		} `bson:"role"`
		Permissions []permission `bson:"permissions"`
	}
	roles := make([]string, 0)
	permissions := make([]string, 0)
	roleSet := make(map[string]struct{})
	permissionSet := make(map[string]struct{})
	for cursor.Next(ctx) {
		var row result
		if err := cursor.Decode(&row); err != nil {
			return Authorization{}, fmt.Errorf("decode user authorization: %w", err)
		}
		if _, ok := roleSet[row.Role.DisplayName]; !ok {
			roleSet[row.Role.DisplayName] = struct{}{}
			roles = append(roles, row.Role.DisplayName)
		}
		for _, item := range row.Permissions {
			if _, ok := permissionSet[item.Key]; !ok {
				permissionSet[item.Key] = struct{}{}
				permissions = append(permissions, item.Key)
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return Authorization{}, fmt.Errorf("iterate user authorization: %w", err)
	}
	role := "Staff"
	if len(roles) > 0 {
		role = roles[0]
	}
	return Authorization{Role: role, Roles: roles, Permissions: permissions}, nil
}

func (s *Store) AssignDefaultStaffRole(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return errors.New("MongoDB is unavailable")
	}
	var role struct {
		RoleKey string `bson:"role_key"`
	}
	if err := s.db.Collection("roles").FindOne(ctx, bson.M{"role_key": "staff", "is_system_role": true}).Decode(&role); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return errors.New("default Staff role is not initialized")
		}
		return fmt.Errorf("find default Staff role: %w", err)
	}
	_, err := s.db.Collection("user_roles").UpdateOne(ctx,
		bson.M{"user_id": userID, "role_key": role.RoleKey},
		bson.M{"$setOnInsert": bson.M{"user_id": userID, "role_key": role.RoleKey, "assigned_at": time.Now().UTC()}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("assign default Staff role: %w", err)
	}
	return nil
}

func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return errors.New("MongoDB database is unavailable")
	}
	indexes := []struct {
		collection string
		indexes    []mongo.IndexModel
	}{
		{"roles", []mongo.IndexModel{
			{Keys: bson.D{{Key: "role_key", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "display_name", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{"user_roles", []mongo.IndexModel{{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "role_key", Value: 1}},
			Options: options.Index().SetUnique(true),
		}}},
		{"role_permissions", []mongo.IndexModel{{
			Keys:    bson.D{{Key: "role_key", Value: 1}, {Key: "permission_key", Value: 1}},
			Options: options.Index().SetUnique(true),
		}}},
		{"permissions", []mongo.IndexModel{{
			Keys:    bson.D{{Key: "permission_key", Value: 1}},
			Options: options.Index().SetUnique(true),
		}}},
	}
	for _, spec := range indexes {
		if _, err := db.Collection(spec.collection).Indexes().CreateMany(ctx, spec.indexes); err != nil {
			return fmt.Errorf("create %s indexes: %w", spec.collection, err)
		}
	}
	_, err := db.Collection("roles").UpdateOne(ctx, bson.M{"role_key": "staff"}, bson.M{
		"$setOnInsert": bson.M{
			"role_key": "staff", "display_name": "Staff",
			"description": "Default least-privilege staff role", "is_system_role": true,
			"created_at": time.Now().UTC(),
		},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("initialize default Staff role: %w", err)
	}
	_, err = db.Collection("roles").UpdateOne(ctx, bson.M{"role_key": "super_admin"}, bson.M{
		"$setOnInsert": bson.M{
			"role_key": "super_admin", "display_name": "SuperAdmin",
			"description": "Initial system administrator role", "is_system_role": true,
			"created_at": time.Now().UTC(),
		},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("initialize SuperAdmin role: %w", err)
	}
	for _, permission := range permissionCatalog {
		if _, err := db.Collection("permissions").UpdateOne(ctx,
			bson.M{"permission_key": permission.key},
			bson.M{"$setOnInsert": bson.M{
				"permission_key": permission.key,
				"description":    permission.description,
			}},
			options.Update().SetUpsert(true),
		); err != nil {
			return fmt.Errorf("initialize permission %q: %w", permission.key, err)
		}
		if _, err := db.Collection("role_permissions").UpdateOne(ctx,
			bson.M{"role_key": "super_admin", "permission_key": permission.key},
			bson.M{"$setOnInsert": bson.M{"role_key": "super_admin", "permission_key": permission.key}},
			options.Update().SetUpsert(true),
		); err != nil {
			return fmt.Errorf("assign %q to SuperAdmin role: %w", permission.key, err)
		}
	}
	if _, err := db.Collection("role_permissions").UpdateOne(ctx,
		bson.M{"role_key": "staff", "permission_key": "staff.read"},
		bson.M{"$setOnInsert": bson.M{"role_key": "staff", "permission_key": "staff.read"}},
		options.Update().SetUpsert(true),
	); err != nil {
		return fmt.Errorf("assign staff.read to default Staff role: %w", err)
	}
	return nil
}
