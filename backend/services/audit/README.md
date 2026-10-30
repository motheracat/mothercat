# Audit service

The Audit service's search API is not implemented yet. It owns the append-only
audit record for the initial SuperAdmin bootstrap.

Apply its versioned SQL migrations with `go run ./cmd/migrate -dir migrations`.
The restricted bootstrap orchestrator calls the Audit-owned package to record
the event idempotently; there is no public setup endpoint.