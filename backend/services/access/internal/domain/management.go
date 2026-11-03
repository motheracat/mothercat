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
