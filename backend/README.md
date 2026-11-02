# Backend

The backend is a Go modular monolith with Identity, Organization, Access, and
Audit domains in one API process. PostgreSQL is the system of record; Supabase
provides the managed PostgreSQL database. The API does not use MongoDB.

## Database configuration

- `DATABASE_URL` is the TLS-enabled PostgreSQL URL used by the API at runtime.
- `MIGRATIONS_DATABASE_URL` is a separate, short-lived, privileged URL used only
  for controlled schema migrations and the one-time data import.
- Never commit either URL, send it in chat, or expose it to frontend code. Keep
  each in the hosting provider's secret manager. Use a least-privilege app role
  for `DATABASE_URL`; do not give the API role schema-owner privileges.
- `make db-migrate` runs the embedded, versioned migrations. It requires
  `MIGRATIONS_DATABASE_URL`, TLS, and network access to the selected PostgreSQL
  instance. Migrations are not automatically run by every API process.
- The `DATABASE_URL` connection must also require TLS in staging and production.
  Keep Supabase backups enabled and test restoring them before production use.

The migration from MongoDB is a separate, one-time operation. It imports relaxed
Extended JSON exports transactionally into an empty PostgreSQL database and
preserves account IDs, password hashes, role assignments, audit events, and
unexpired session/recovery-token hashes. See the [PostgreSQL and Supabase
deployment guide](docs/DEPLOYMENT-POSTGRES.md) for staging verification and
cutover steps. Do not deploy the PostgreSQL API against production until the
import is complete and verified.

## Initial administrator

The one-time `POST /api/v1/auth/bootstrap` endpoint creates the first active
SuperAdmin account. It is available only when `IDENTITY_BOOTSTRAP_TOKEN` is
configured and PostgreSQL contains no Identity accounts. Send the high-entropy
token in an `Authorization: Bearer <token>` header over HTTPS, with JSON fields
`staff_id`, `email`, `display_name`, and a strong `password` (at least 14
characters, including at least three character classes). The endpoint atomically
creates the account, grants the SuperAdmin role, records an audit event, and
permanently closes bootstrap. It does not return a session; sign in through the
normal login endpoint.

After a successful `201 Created`, remove `IDENTITY_BOOTSTRAP_TOKEN` from the
hosting secret store and redeploy. The database prevents repeat bootstrap even
if the environment variable is accidentally retained. The migration must have
run first. Never send the token or password over HTTP or store them in source
control.

## Account suspension and authentication safeguards

Before deploying account suspension code, apply pending migrations to the
intended Supabase database from `backend/` with the privileged migration URL
provided through a secure environment mechanism:

```sh
make db-migrate
```

Migration `002_account_suspension` adds the separate suspension state. It does
not change account activation state or staff employment status. Apply this
migration before deploying the API version that reads the new columns.

An authenticated administrator with the `settings.manage` permission can
temporarily suspend or reactivate a login account:

- `POST /api/v1/auth/accounts/{staffID}/suspend`
- `POST /api/v1/auth/accounts/{staffID}/reactivate`

These requests require the current session cookie and a matching `Origin`
header. Suspension revokes all current sessions and invalidates outstanding
recovery/activation tokens in the same transaction as the audit event. It
blocks login and session resolution without changing the account's activation
or staff employment state. Reactivation only clears the suspension; the user
must log in again. The API prevents suspending the last active SuperAdmin.
Expected outcomes are `200`, `401`, `403`, `404`, `409`, or `503`; errors
include a machine-readable code and request ID.

Login limits are five failed attempts per IP and account per ten-minute
process-local window. Recovery and activation workflows are limited to ten
requests per IP per ten minutes and return a generic accepted response to avoid
account enumeration. These in-memory limits reset on process restart and are
not shared between replicas; deployments with multiple API instances should
also enforce shared edge-level rate limits.

## Scope and status

The frontend is intentionally unchanged. The PostgreSQL migration is not
production-ready until a Supabase-compatible PostgreSQL instance has accepted
the schema, data import and restore have been tested, and the staging API has
passed end-to-end verification.
