package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type database interface {
	Begin(context.Context) (pgx.Tx, error)
}

type permission struct {
	key         string
	description string
}

var initialPermissions = []permission{
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

// AssignInitialSuperAdmin seeds the system role and permissions and assigns the
// supplied identity account. It is safe to retry for the same account.
func AssignInitialSuperAdmin(ctx context.Context, db database, accountID string) error {
	if db == nil {
		return errors.New("Access bootstrap database is unavailable")
	}
	if strings.TrimSpace(accountID) == "" {
		return errors.New("Identity account ID is required")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Access bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO access.roles (role_key, display_name, description, is_system_role)
		VALUES ('super_admin', 'SuperAdmin', 'Initial system administrator role', TRUE)
		ON CONFLICT (role_key) DO NOTHING
	`); err != nil {
		return fmt.Errorf("create SuperAdmin role: %w", err)
	}
	var roleID string
	var systemRole bool
	if err := tx.QueryRow(ctx, `
		SELECT id, is_system_role FROM access.roles WHERE role_key = 'super_admin'
	`).Scan(&roleID, &systemRole); err != nil {
		return fmt.Errorf("read SuperAdmin role: %w", err)
	}
	if !systemRole {
		return errors.New("existing super_admin role is not marked as a system role")
	}

	for _, item := range initialPermissions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO access.permissions (permission_key, description)
			VALUES ($1, $2)
			ON CONFLICT (permission_key) DO NOTHING
		`, item.key, item.description); err != nil {
			return fmt.Errorf("create permission %q: %w", item.key, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO access.role_permissions (role_id, permission_key)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, roleID, item.key); err != nil {
			return fmt.Errorf("assign permission %q to SuperAdmin: %w", item.key, err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO access.user_roles (user_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, accountID, roleID); err != nil {
		return fmt.Errorf("assign SuperAdmin role to initial account: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Access bootstrap: %w", err)
	}
	return nil
}

// InitialPermissionKeys returns a copy of the permission keys seeded for the first admin.
func InitialPermissionKeys() []string {
	keys := make([]string, len(initialPermissions))
	for i, item := range initialPermissions {
		keys[i] = item.key
	}
	return keys
}
