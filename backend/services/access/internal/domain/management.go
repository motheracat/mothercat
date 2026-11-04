package domain

import "errors"

var (
	ErrNotFound             = errors.New("access resource not found")
	ErrConflict             = errors.New("access resource already exists")
	ErrSystemRole           = errors.New("system role cannot be modified")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrLastActiveSuperAdmin = errors.New("cannot remove the last active SuperAdmin")
	ErrInvalidPermissionSet = errors.New("invalid permission set")
)

type Role struct {
	ID           string   `json:"role_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	IsSystemRole bool     `json:"is_system_role"`
	Permissions  []string `json:"permissions"`
}

type Permission struct {
	Key         string `json:"permission_key"`
	Description string `json:"description"`
}

type UserRoleAssignment struct {
	UserID           string   `json:"user_id"`
	FullName         string   `json:"full_name"`
	StaffID          string   `json:"staff_id"`
	Email            string   `json:"email"`
	EmploymentStatus string   `json:"employment_status"`
	AccountActive    bool     `json:"account_active"`
	RoleIDs          []string `json:"role_ids"`
}
