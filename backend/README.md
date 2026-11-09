# Backend

The backend is a Go modular monolith with Identity, Organization, Access,
Audit, and Tasks modules in one API process. PostgreSQL is the system of
record; Supabase provides the managed PostgreSQL database. The API does not use
MongoDB.

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

## Tasks API

The task workflow is an internal module in the same API server, not a separately
deployed service. Migration `007_tasks` creates the `tasks` schema and grants the
task permissions to the built-in Admin and SuperAdmin roles.

Authenticated task routes:

- `GET|POST /api/v1/tasks`
- `GET|PATCH /api/v1/tasks/{taskId}`
- `GET /api/v1/me/tasks`
- `POST /api/v1/tasks/{taskId}/assign`
- `POST /api/v1/tasks/{taskId}/start`
- `POST /api/v1/tasks/{taskId}/submit`
- `POST /api/v1/tasks/{taskId}/approve`
- `POST /api/v1/tasks/{taskId}/request-changes`
- `POST /api/v1/tasks/{taskId}/reject`
- `POST /api/v1/tasks/{taskId}/cancel`
- `GET /api/v1/tasks/{taskId}/history`
- `GET|POST /api/v1/tasks/{taskId}/comments`

Task reads are restricted to the creator, active assignee, members of the task
department with `tasks.read_department`, or users with `tasks.read_all`.
Assignment requires `tasks.assign`, department membership, and an active staff
account in that department. Draft edits require the creator and a matching
`If-Match` version. Task state history and audit events are written in the same
database transaction as the corresponding change. Cookie-authenticated writes
require a same-origin `Origin` header.

## Notifications API

The in-app notification module runs in the same API server. Migrations
`008_notifications` and `009_notification_outbox` create recipient-owned
notification, preference, and transactional outbox tables. Task assignment,
submission, review, and comment events write an outbox event in the same
transaction as the task change. An internal dispatcher delivers events to
notifications with bounded exponential retry, and records delivery and
notification rows atomically. The outbox event ID is the idempotency key;
disabled event preferences suppress new notifications.

The internal task reminder job runs immediately at API startup and every 15
minutes. It queues one `task.due_soon` event per task due within 24 hours and
one `task.overdue` event per overdue task while the task is active. Migration
`010_task_reminder_idempotency` allows system-generated events without a user
actor and makes concurrent or repeated job runs safe. Reminder events notify
the task creator and current assignee.

- `GET /api/v1/notifications?unread=true&limit=50&offset=0`
- `GET /api/v1/notifications/unread-count`
- `POST /api/v1/notifications/{notificationId}/read`
- `POST /api/v1/notifications/read-all`
- `GET /api/v1/notifications/preferences`
- `PUT /api/v1/notifications/preferences` with
  `{"preferences":[{"event_type":"task.assigned","enabled":true}]}`

All notification reads and updates are scoped to the authenticated account.
The list, history, and comments endpoints are paginated; notification write
requests require a same-origin `Origin` header.

Frontend integration details, including the production API URL, session-cookie
usage, task and notification request/response shapes, and browser-origin
requirements, are in the [Frontend API Guide](docs/FRONTEND-API-GUIDE.md).

## Opportunities and SIWES applications

Migration `011_opportunities` adds public job/SIWES opportunity listings,
permission-protected opportunity management, public SIWES application
submission, private reviewer-only document downloads, and the associated
permission keys. SIWES application metadata and document bytes are purged after
365 days by an in-process daily retention job. Public uploads are bounded to a
10 MiB document and require a healthy, private ClamAV service configured by
`SIWES_CLAMAV_ADDR` (`host:port`); upload requests fail closed if malware
scanning is unavailable. Configure additional edge-level rate limits for
production, because the API's five-per-hour submission limit is process-local.
The [Frontend API Guide](docs/FRONTEND-API-GUIDE.md) documents the routes,
permission requirements, status transitions, upload constraints, and proxy
requirements.

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

## Administrator roles

The database seeds two administrator roles:

- `Admin` is granted the current 17 permissions in the backend catalogue:
  staff read/create/update/deactivate/import, department management, role
  read/management, user-role assignment, audit read, system settings
  management, and the six task permissions.
- `SuperAdmin` retains the same current permissions. The initial bootstrap
  account is assigned this role; Admin and SuperAdmin remain distinct system
  roles even though their current permission sets match.

Migration `006_admin_full_permissions` grants the Milestone 1 permission set
to Admin on existing and fresh databases; migration `007_tasks` grants the six
task permissions to both administrator roles. Future permissions are not
automatically granted.

An authenticated `SuperAdmin` can provision an eligible pending staff profile
directly as an `Admin` through `POST /api/v1/staff/{staffID}/provision-admin`.
The workflow sends the standard activation invitation. The ID in the path is
the staff profile UUID. This endpoint requires `roles.manage` and a matching
Origin; the regular `/provision` endpoint continues to provision a Staff
account.

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

An authenticated administrator with the `settings.manage` permission (currently
Admin and SuperAdmin) can
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
