package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	accessdomain "github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

type AuthorizationRepository struct {
	pool *pgxpool.Pool
}

func NewAuthorizationRepository(pool *pgxpool.Pool) *AuthorizationRepository {
	return &AuthorizationRepository{pool: pool}
}

func (r *AuthorizationRepository) Ping(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("Access database is unavailable")
	}
	return r.pool.Ping(ctx)
}

func (r *AuthorizationRepository) GetUserAuthorization(ctx context.Context, userID string) (accessdomain.Authorization, error) {
	if r == nil || r.pool == nil {
		return accessdomain.Authorization{}, fmt.Errorf("Access database is unavailable")
	}

	rows, err := r.pool.Query(ctx, `
		SELECT r.display_name, rp.permission_key
		FROM access.user_roles ur
		JOIN access.roles r ON r.id = ur.role_id
		LEFT JOIN access.role_permissions rp ON rp.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY r.role_key, rp.permission_key
	`, userID)
	if err != nil {
		return accessdomain.Authorization{}, fmt.Errorf("query user authorization: %w", err)
	}
	defer rows.Close()

	roles := make([]string, 0)
	permissions := make([]string, 0)
	roleSet := make(map[string]struct{})
	permissionSet := make(map[string]struct{})
	for rows.Next() {
		var role string
		var permission *string
		if err := rows.Scan(&role, &permission); err != nil {
			return accessdomain.Authorization{}, fmt.Errorf("scan user authorization: %w", err)
		}
		if _, exists := roleSet[role]; !exists {
			roleSet[role] = struct{}{}
			roles = append(roles, role)
		}
		if permission != nil {
			if _, exists := permissionSet[*permission]; !exists {
				permissionSet[*permission] = struct{}{}
				permissions = append(permissions, *permission)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return accessdomain.Authorization{}, fmt.Errorf("iterate user authorization: %w", err)
	}
	role := "Staff"
	if len(roles) != 0 {
		role = roles[0]
	}
	return accessdomain.Authorization{Role: role, Roles: roles, Permissions: permissions}, nil
}

func (r *AuthorizationRepository) AssignDefaultStaffRole(ctx context.Context, userID string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("Access database is unavailable")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO access.user_roles (user_id, role_id)
		SELECT $1, id FROM access.roles WHERE role_key = 'staff' AND is_system_role = TRUE
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("assign default Staff role: %w", err)
	}
	return nil
}
