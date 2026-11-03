package monolith

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Authorization struct {
	Role        string
	Roles       []string
	Permissions []string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) GetUserAuthorization(ctx context.Context, userID string) (Authorization, error) {
	if s == nil || s.db == nil {
		return Authorization{}, errors.New("PostgreSQL database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.display_name, rp.permission_key
		FROM access.user_roles ur
		JOIN access.roles r ON r.role_key = ur.role_key
		LEFT JOIN access.role_permissions rp ON rp.role_key = r.role_key
		WHERE ur.user_id = $1::uuid
		ORDER BY r.display_name, rp.permission_key
	`, userID)
	if err != nil {
		return Authorization{}, fmt.Errorf("query user authorization: %w", err)
	}
	defer rows.Close()
	roles := make([]string, 0)
	permissions := make([]string, 0)
	roleSet := make(map[string]struct{})
	permissionSet := make(map[string]struct{})
	for rows.Next() {
		var roleName string
		var permissionKey sql.NullString
		if err := rows.Scan(&roleName, &permissionKey); err != nil {
			return Authorization{}, fmt.Errorf("scan user authorization: %w", err)
		}
		if _, ok := roleSet[roleName]; !ok {
			roleSet[roleName] = struct{}{}
			roles = append(roles, roleName)
		}
		if permissionKey.Valid {
			if _, ok := permissionSet[permissionKey.String]; !ok {
				permissionSet[permissionKey.String] = struct{}{}
				permissions = append(permissions, permissionKey.String)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Authorization{}, fmt.Errorf("iterate user authorization: %w", err)
	}
	role := "Staff"
	if len(roles) > 0 {
		role = roles[0]
	}
	return Authorization{Role: role, Roles: roles, Permissions: permissions}, nil
}

func (s *Store) AssignDefaultStaffRole(ctx context.Context, userID string) error {
	return s.AssignRole(ctx, userID, "staff")
}

func (s *Store) AssignRole(ctx context.Context, userID, roleKey string) error {
	if s == nil || s.db == nil {
		return errors.New("PostgreSQL database is unavailable")
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO access.user_roles (user_id, role_key)
		SELECT $1::uuid, role_key
		FROM access.roles
		WHERE role_key = $2 AND is_system_role
		ON CONFLICT (user_id, role_key) DO NOTHING
	`, userID, roleKey)
	if err != nil {
		return fmt.Errorf("assign role %q: %w", roleKey, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check role %q assignment: %w", roleKey, err)
	}
	if rowsAffected == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM access.roles WHERE role_key = $1 AND is_system_role)`, roleKey,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check initialized role %q: %w", roleKey, err)
		}
		if !exists {
			return fmt.Errorf("system role %q is not initialized; run PostgreSQL migrations", roleKey)
		}
	}
	return nil
}
