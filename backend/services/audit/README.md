# Audit service

The Audit service stores immutable administrative events. Its search API is
not implemented yet. It exposes health checks and an internal write-only
ingestion endpoint:

- `GET /health/live`
- `GET /health/ready`
- `POST /internal/v1/audit-events` (requires the configured service token)

Event keys are idempotent. The restricted bootstrap orchestrator calls the
Audit-owned package to record its event; there is no public setup endpoint.
Organization records a `staff.create.requested` event before inserting a staff
record and fails closed if Audit cannot durably accept it.

Apply versioned SQL migrations separately with
`go run ./cmd/migrate -dir migrations`, using the restricted migration
credential. Runtime credentials require only schema usage plus `SELECT,
INSERT` on `audit.audit_events`; do not grant update/delete.

Required runtime settings are `APP_ENV`, `DATABASE_URL`, and a high-entropy
`AUDIT_SERVICE_TOKEN` of at least 32 characters. Set
`DATABASE_ALLOW_UNVERIFIED_TLS=true` only when using Render's private
`sslmode=require` connection.
