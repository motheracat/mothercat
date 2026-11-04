package monolith

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mothercat-enterprise/mothercat/backend/services/access/internal/domain"
)

var (
	ErrNotFound             = domain.ErrNotFound
	ErrConflict             = domain.ErrConflict
	ErrSystemRole           = domain.ErrSystemRole
	ErrPermissionDenied     = domain.ErrPermissionDenied
	ErrLastActiveSuperAdmin = domain.ErrLastActiveSuperAdmin
	ErrInvalidPermissionSet = domain.ErrInvalidPermissionSet
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

func (s *Store) ListRoles(ctx context.Context) ([]domain.Role, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("PostgreSQL database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.role_key, r.display_name, r.description, r.is_system_role, rp.permission_key
		FROM access.roles r
		LEFT JOIN access.role_permissions rp ON rp.role_key = r.role_key
		ORDER BY r.display_name, rp.permission_key
	`)
	if err != nil {
		return nil, fmt.Errorf("list access roles: %w", err)
	}
	defer rows.Close()
	roles := make([]domain.Role, 0)
	indexes := make(map[string]int)
	for rows.Next() {
		var roleID, name, description string
		var isSystem bool
		var permission sql.NullString
		if err := rows.Scan(&roleID, &name, &description, &isSystem, &permission); err != nil {
			return nil, fmt.Errorf("scan access role: %w", err)
		}
		index, ok := indexes[roleID]
		if !ok {
			index = len(roles)
			indexes[roleID] = index
			roles = append(roles, domain.Role{
				ID: roleID, Name: name, Description: description,
				IsSystemRole: isSystem, Permissions: make([]string, 0),
			})
		}
		if permission.Valid {
			roles[index].Permissions = append(roles[index].Permissions, permission.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access roles: %w", err)
	}
	return roles, nil
}

func (s *Store) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("PostgreSQL database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT permission_key, description
		FROM access.permissions
		ORDER BY permission_key
	`)
	if err != nil {
		return nil, fmt.Errorf("list access permissions: %w", err)
	}
	defer rows.Close()
	permissions := make([]domain.Permission, 0)
	for rows.Next() {
		var permission domain.Permission
		if err := rows.Scan(&permission.Key, &permission.Description); err != nil {
			return nil, fmt.Errorf("scan access permission: %w", err)
		}
		permissions = append(permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access permissions: %w", err)
	}
	return permissions, nil
}

func (s *Store) CreateRole(ctx context.Context, actorID string, role domain.Role) (domain.Role, error) {
	if s == nil || s.db == nil {
		return domain.Role{}, errors.New("PostgreSQL database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Role{}, fmt.Errorf("begin role creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAccessAdministration(ctx, tx); err != nil {
		return domain.Role{}, err
	}
	if err := requireActorPermission(ctx, tx, actorID, "roles.manage"); err != nil {
		return domain.Role{}, err
	}
	role.ID = strings.TrimSpace(role.ID)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO access.roles (role_key, display_name, description, is_system_role)
		VALUES ($1, $2, $3, false)
	`, role.ID, role.Name, role.Description)
	if isUniqueViolation(err) {
		return domain.Role{}, domain.ErrConflict
	}
	if err != nil {
		return domain.Role{}, fmt.Errorf("insert access role: %w", err)
	}
	role.IsSystemRole = false
	role.Permissions = []string{}
	if err := recordAccessEvent(ctx, tx, actorID, "access.role.create", "role", role.ID,
		map[string]any{"name": role.Name}); err != nil {
		return domain.Role{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Role{}, fmt.Errorf("commit role creation: %w", err)
	}
	return role, nil
}

func (s *Store) UpdateRole(
	ctx context.Context,
	actorID, roleID string,
	name, description *string,
) (domain.Role, error) {
	if s == nil || s.db == nil {
		return domain.Role{}, errors.New("PostgreSQL database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Role{}, fmt.Errorf("begin role update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAccessAdministration(ctx, tx); err != nil {
		return domain.Role{}, err
	}
	if err := requireActorPermission(ctx, tx, actorID, "roles.manage"); err != nil {
		return domain.Role{}, err
	}
	var role domain.Role
	err = tx.QueryRowContext(ctx, `
		UPDATE access.roles
		SET display_name = COALESCE($2, display_name),
		    description = COALESCE($3, description)
		WHERE role_key = $1 AND NOT is_system_role
		RETURNING role_key, display_name, description, is_system_role
	`, roleID, name, description).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystemRole)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRowContext(ctx,
			`SELECT is_system_role FROM access.roles WHERE role_key = $1`, roleID,
		).Scan(&role.IsSystemRole); errors.Is(err, sql.ErrNoRows) {
			return domain.Role{}, domain.ErrNotFound
		} else if err != nil {
			return domain.Role{}, fmt.Errorf("check access role: %w", err)
		} else if role.IsSystemRole {
			return domain.Role{}, domain.ErrSystemRole
		}
		return domain.Role{}, domain.ErrNotFound
	}
	if isUniqueViolation(err) {
		return domain.Role{}, domain.ErrConflict
	}
	if err != nil {
		return domain.Role{}, fmt.Errorf("update access role: %w", err)
	}
	role.Permissions, err = listRolePermissions(ctx, tx, roleID)
	if err != nil {
		return domain.Role{}, err
	}
	if err := recordAccessEvent(ctx, tx, actorID, "access.role.update", "role", role.ID,
		map[string]any{"name": role.Name, "description": role.Description}); err != nil {
		return domain.Role{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Role{}, fmt.Errorf("commit role update: %w", err)
	}
	return role, nil
}

func (s *Store) ReplaceRolePermissions(
	ctx context.Context,
	actorID, roleID string,
	permissionKeys []string,
) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("PostgreSQL database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin role permission update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAccessAdministration(ctx, tx); err != nil {
		return nil, err
	}
	actorPermissions, err := actorPermissions(ctx, tx, actorID, "roles.manage")
	if err != nil {
		return nil, err
	}
	var isSystem bool
	err = tx.QueryRowContext(ctx,
		`SELECT is_system_role FROM access.roles WHERE role_key = $1 FOR UPDATE`, roleID,
	).Scan(&isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find role for permission update: %w", err)
	}
	if isSystem {
		return nil, domain.ErrSystemRole
	}
	keys := uniqueSorted(permissionKeys)
	for _, key := range keys {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM access.permissions WHERE permission_key = $1)`, key,
		).Scan(&exists); err != nil {
			return nil, fmt.Errorf("validate permission %q: %w", key, err)
		}
		if !exists {
			return nil, domain.ErrInvalidPermissionSet
		}
		if _, permitted := actorPermissions[key]; !permitted {
			return nil, domain.ErrPermissionDenied
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM access.role_permissions WHERE role_key = $1`, roleID); err != nil {
		return nil, fmt.Errorf("remove existing role permissions: %w", err)
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO access.role_permissions (role_key, permission_key) VALUES ($1, $2)
		`, roleID, key); err != nil {
			return nil, fmt.Errorf("assign permission %q to role: %w", key, err)
		}
	}
	if err := recordAccessEvent(ctx, tx, actorID, "access.role.permissions.replace", "role", roleID,
		map[string]any{"permission_keys": keys}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit role permission update: %w", err)
	}
	return keys, nil
}

func (s *Store) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("PostgreSQL database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ur.role_key
		FROM identity.user_accounts a
		LEFT JOIN access.user_roles ur ON ur.user_id = a.id
		WHERE a.id = $1::uuid
		ORDER BY ur.role_key
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user roles: %w", err)
	}
	defer rows.Close()
	roles := make([]string, 0)
	found := false
	for rows.Next() {
		found = true
		var role sql.NullString
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		if role.Valid {
			roles = append(roles, role.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}
	if !found {
		return nil, domain.ErrNotFound
	}
	return roles, nil
}

func (s *Store) ListUserRoleAssignments(
	ctx context.Context,
	search, roleID string,
	limit, offset int,
) ([]domain.UserRoleAssignment, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("PostgreSQL database is unavailable")
	}
	const conditions = `
		WHERE ($1 = '' OR s.full_name ILIKE '%' || $1 || '%'
			OR s.staff_id ILIKE '%' || $1 || '%'
			OR COALESCE(s.work_email, a.email) ILIKE '%' || $1 || '%')
			AND ($2 = '' OR EXISTS (
				SELECT 1 FROM access.user_roles filtered_role
				WHERE filtered_role.user_id = a.id AND filtered_role.role_key = $2
			))
	`
	var total int
	if err := s.db.QueryRowContext(ctx, `
			SELECT count(*)
			FROM organization.staff_members s
			JOIN identity.user_accounts a ON a.id = s.account_id
	`+conditions, search, roleID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count user role assignments: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id::text, s.full_name, s.staff_id,
			COALESCE(s.work_email, a.email), s.employment_status,
			(a.is_active AND a.suspended_at IS NULL),
			COALESCE(
				json_agg(ur.role_key ORDER BY ur.role_key)
					FILTER (WHERE ur.role_key IS NOT NULL),
				'[]'::json
			)
		FROM organization.staff_members s
		JOIN identity.user_accounts a ON a.id = s.account_id
		LEFT JOIN access.user_roles ur ON ur.user_id = a.id
	`+conditions+`
		GROUP BY a.id, s.full_name, s.staff_id, s.work_email, a.email,
			s.employment_status, a.is_active, a.suspended_at
		ORDER BY lower(s.full_name), s.staff_id
		LIMIT $3 OFFSET $4
	`, search, roleID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query user role assignments: %w", err)
	}
	defer rows.Close()
	assignments := make([]domain.UserRoleAssignment, 0)
	for rows.Next() {
		var assignment domain.UserRoleAssignment
		var roleIDsJSON []byte
		if err := rows.Scan(
			&assignment.UserID, &assignment.FullName, &assignment.StaffID,
			&assignment.Email, &assignment.EmploymentStatus, &assignment.AccountActive,
			&roleIDsJSON,
		); err != nil {
			return nil, 0, fmt.Errorf("scan user role assignment: %w", err)
		}
		if err := json.Unmarshal(roleIDsJSON, &assignment.RoleIDs); err != nil {
			return nil, 0, fmt.Errorf("decode user role assignment: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate user role assignments: %w", err)
	}
	return assignments, total, nil
}

func (s *Store) ReplaceUserRoles(
	ctx context.Context,
	actorID, userID string,
	roleKeys []string,
) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("PostgreSQL database is unavailable")
	}
	if actorID == userID {
		return nil, domain.ErrPermissionDenied
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin user role update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAccessAdministration(ctx, tx); err != nil {
		return nil, err
	}
	actorPermissions, err := actorPermissions(ctx, tx, actorID, "user_roles.assign")
	if err != nil {
		return nil, err
	}
	keys := uniqueSorted(roleKeys)
	for _, key := range keys {
		var exists bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM access.roles WHERE role_key = $1)
		`, key).Scan(&exists); err != nil {
			return nil, fmt.Errorf("validate role %q: %w", key, err)
		}
		if !exists {
			return nil, domain.ErrNotFound
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT DISTINCT permission_key
			FROM access.role_permissions
			WHERE role_key = $1
		`, key)
		if err != nil {
			return nil, fmt.Errorf("read permissions for role %q: %w", key, err)
		}
		for rows.Next() {
			var permission string
			if err := rows.Scan(&permission); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan role permission: %w", err)
			}
			if _, permitted := actorPermissions[permission]; !permitted {
				rows.Close()
				return nil, domain.ErrPermissionDenied
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate role permissions: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close role permissions: %w", err)
		}
	}
	var targetExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.user_accounts WHERE id = $1::uuid)`, userID,
	).Scan(&targetExists); err != nil {
		return nil, fmt.Errorf("validate user for role update: %w", err)
	}
	if !targetExists {
		return nil, domain.ErrNotFound
	}
	var wasActiveSuperAdmin bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM access.user_roles ur
			JOIN identity.user_accounts a ON a.id = ur.user_id
			WHERE ur.user_id = $1::uuid AND ur.role_key = 'super_admin'
			  AND a.is_active AND a.suspended_at IS NULL
		)
	`, userID).Scan(&wasActiveSuperAdmin); err != nil {
		return nil, fmt.Errorf("check active SuperAdmin assignment: %w", err)
	}
	willBeSuperAdmin := contains(keys, "super_admin")
	if wasActiveSuperAdmin && !willBeSuperAdmin {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*)
			FROM access.user_roles ur
			JOIN identity.user_accounts a ON a.id = ur.user_id
			WHERE ur.role_key = 'super_admin' AND a.is_active AND a.suspended_at IS NULL
		`).Scan(&activeAdmins); err != nil {
			return nil, fmt.Errorf("count active SuperAdmins: %w", err)
		}
		if activeAdmins <= 1 {
			return nil, domain.ErrLastActiveSuperAdmin
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM access.user_roles WHERE user_id = $1::uuid`, userID); err != nil {
		return nil, fmt.Errorf("remove existing user roles: %w", err)
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO access.user_roles (user_id, role_key) VALUES ($1::uuid, $2)
		`, userID, key); err != nil {
			return nil, fmt.Errorf("assign role %q to user: %w", key, err)
		}
	}
	if err := recordAccessEvent(ctx, tx, actorID, "access.user.roles.replace", "user", userID,
		map[string]any{"role_ids": keys}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit user role update: %w", err)
	}
	return keys, nil
}

func requireActorPermission(ctx context.Context, tx *sql.Tx, actorID, permission string) error {
	_, err := actorPermissions(ctx, tx, actorID, permission)
	return err
}

func lockAccessAdministration(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(761039123)`); err != nil {
		return fmt.Errorf("lock access administration changes: %w", err)
	}
	return nil
}

func actorPermissions(ctx context.Context, tx *sql.Tx, actorID, required string) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT rp.permission_key
		FROM access.user_roles ur
		JOIN identity.user_accounts a ON a.id = ur.user_id
		JOIN access.role_permissions rp ON rp.role_key = ur.role_key
		WHERE ur.user_id = $1::uuid AND a.is_active AND a.suspended_at IS NULL
	`, actorID)
	if err != nil {
		return nil, fmt.Errorf("read actor permissions: %w", err)
	}
	defer rows.Close()
	permissions := make(map[string]struct{})
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan actor permission: %w", err)
		}
		permissions[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate actor permissions: %w", err)
	}
	if _, ok := permissions[required]; !ok {
		return nil, domain.ErrPermissionDenied
	}
	return permissions, nil
}

func recordAccessEvent(
	ctx context.Context,
	tx *sql.Tx,
	actorID, action, resourceType, resourceID string,
	details map[string]any,
) error {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("generate access audit event key: %w", err)
	}
	eventKey := "access:" + hex.EncodeToString(random[:])
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode access audit event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type,
			resource_id, details
		) VALUES (gen_random_uuid(), $1, 'user', $2, $3, $4, $5, $6::jsonb)
	`, eventKey, actorID, action, resourceType, resourceID, encoded); err != nil {
		return fmt.Errorf("persist access audit event: %w", err)
	}
	return nil
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[strings.TrimSpace(value)] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func listRolePermissions(ctx context.Context, tx *sql.Tx, roleID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT permission_key FROM access.role_permissions
		WHERE role_key = $1 ORDER BY permission_key
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions for role %q: %w", roleID, err)
	}
	defer rows.Close()
	permissions := make([]string, 0)
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		permissions = append(permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role permissions: %w", err)
	}
	return permissions, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isUniqueViolation(err error) bool {
	var sqlState interface{ SQLState() string }
	return errors.As(err, &sqlState) && sqlState.SQLState() == "23505"
}
