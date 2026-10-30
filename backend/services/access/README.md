# Access service

The Access service's public role-management API is not implemented yet. This
service owns the initial SuperAdmin role, its Milestone 1 permissions, and user
role assignments created by the restricted one-time bootstrap.

Apply its versioned SQL migrations with `go run ./cmd/migrate -dir migrations`.
The bootstrap operation is orchestrated by
[`backend/cmd/bootstrap-superadmin`](../../cmd/bootstrap-superadmin) and writes
only through this service's package. No public bootstrap route is exposed.