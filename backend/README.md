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
initial task permissions to the built-in Admin and SuperAdmin roles. Migration
`013_admin_task_status` adds administrative task status fields and the
`tasks.update_status` permission.

Authenticated task routes:

- `GET|POST /api/v1/tasks`
- `GET|PATCH /api/v1/tasks/{taskId}`
- `PATCH /api/v1/tasks/{taskId}/admin-status`
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
Creating a task requires `tasks.create`; callers with `tasks.read_all` may
create tasks outside their department memberships, while other creators must
belong to the selected department. Assignment requires `tasks.assign`,
department membership, and an active staff account in that department. Draft
edits require the creator and a matching `If-Match` version. Task state history
and audit events are written in the same database transaction as the
corresponding change. Cookie-authenticated writes require a same-origin
`Origin` header.

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
10 MiB document. The backend validates file extension, content signature, and
document structure before storing uploaded bytes privately in PostgreSQL;
malware scanning is not performed. Configure additional edge-level rate limits
for production, because the API's five-per-hour submission limit is
process-local. Submission confirmations and status-change emails are queued
transactionally and delivered with retry through the configured Brevo sender
(`BREVO_API_KEY`, `BREVO_FROM_EMAIL`, and `PUBLIC_BASE_URL`). If email delivery
is not configured, queued messages remain pending until it is configured and
the API restarts.
The [Frontend API Guide](docs/FRONTEND-API-GUIDE.md) documents the routes,
permission requirements, status transitions, upload constraints, and proxy
requirements.

## Applicant identity

Migration `012_applicant_identity` adds applicant-only accounts, email
verification tokens, and sessions in separate tables from staff identity.
Applicant authentication is served by a separate handler and uses the
`mothercat_applicant_session` cookie; it does not grant staff workspace
permissions or authenticate staff API routes.

- `POST /api/v1/auth/applicant/register` creates an unverified applicant and
  sends a verification email. Responses do not disclose whether an email
  address is already registered.
- `POST /api/v1/auth/applicant/verify-email` consumes a single-use,
  time-limited verification token and returns `400` for invalid or expired
  tokens.
- `POST /api/v1/auth/applicant/resend-verification` requests another
  verification email with a generic response.
- `POST /api/v1/auth/applicant/login` accepts verified applicant credentials
  and sets the isolated applicant session cookie.

Registration requires a valid email address, display name, and a 14–72 byte
password containing at least three character classes. Public applicant
authentication requests are rate limited per client IP and email/account.
Email delivery uses the configured identity mail provider and fails
explicitly when unavailable.

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

- `Admin` is granted the current 18 permissions in the backend catalogue:
  staff read/create/update/deactivate/import, department management, role
  read/management, user-role assignment, audit read, system settings
  management, and task permissions including administrative task status.
- `SuperAdmin` retains the same current permissions. The initial bootstrap
  account is assigned this role; Admin and SuperAdmin remain distinct system
  roles even though their current permission sets match.

Migration `006_admin_full_permissions` grants the Milestone 1 permission set
to Admin on existing and fresh databases; migration `007_tasks` grants the
initial task permissions, and migration `013_admin_task_status` grants
`tasks.update_status`, to both administrator roles. Future permissions are not
automatically granted.

An authenticated `SuperAdmin` can provision an eligible pending staff profile
directly as an `Admin` through `POST /api/v1/staff/{staffID}/provision-admin`.
The workflow sends the standard activation invitation. The ID in the path is
the staff profile UUID. This endpoint requires `roles.manage` and a matching
Origin; the regular `/provision` endpoint continues to provision a Staff
account.

## Independent System Monitor access

System Monitor access is not an Access role or permission. Neither `Admin` nor
`SuperAdmin` receives it by virtue of their role, and the regular role and
user-role APIs cannot create or revoke monitor grants. Migration
`015_system_monitor_access` stores separate, expiring grants. Apply it through
the controlled migration process before using the operator command. Every
grant and revocation is written to the audit log in the same transaction. An
account must be active and unsuspended, and a grant expires within 30 days.

Only a trusted database operator should use the dedicated `monitor-access`
command after the organization's independent approval process has authorized
the change. The command looks for the ignored `backend/.env` file when the
authorization URL is not set in the shell. Prefer setting
`SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL` there, using a distinct
TLS-required operator database login. For local convenience, the CLI falls
back to `DATABASE_URL` if the dedicated variable is absent. Any explicitly
exported `SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL` or `DATABASE_URL` takes
precedence over values in the file. Do not use the API or migration credential
for operator access; the selected login should be limited to reading eligible
Identity accounts, managing rows in `system_monitor.access_grants`, and
inserting audit events. The API runtime login needs read access to
`system_monitor.access_grants` before any monitor handler is enabled. Keep the
operator login out of API/CI runtime environments.

Grant access for a maximum of 30 days:

```sh
cd backend/services/monolith
go run ./cmd/monitor-access grant \
  --account-id ACCOUNT_UUID \
  --operator OPERATOR_ID \
  --reason "Approved change CHG-123" \
  --expires-at 2026-11-20T12:00:00Z
```

Revoke all active grants for an account:

```sh
cd backend/services/monolith
go run ./cmd/monitor-access revoke \
  --account-id ACCOUNT_UUID \
  --operator OPERATOR_ID \
  --reason "Access no longer required; CHG-124"
```

The operator identifier and approval reference are recorded as supplied; the
command does not independently authenticate the named operator or validate an
approval ticket. The security boundary is the restricted authorization
database credential and the out-of-band approval process. Do not use the CLI
until those controls and database grants are provisioned. System Monitor
read-only endpoints require both a staff session and an active independent
grant. The System Monitor endpoint APIs are:

- `GET /api/v1/system-monitor/overview`
- `GET /api/v1/system-monitor/endpoints`
- `GET /api/v1/system-monitor/events?limit=50`
- `GET /api/v1/system-monitor/endpoints/history?limit=50`
- `GET /api/v1/system-monitor/analytics?method=GET&route=%2Fapi%2Fv1%2Ftasks%2F%7BtaskID%7D&interval=hour`
- `POST /api/v1/system-monitor/endpoints/pause`
- `POST /api/v1/system-monitor/endpoints/resume`
- `GET /api/v1/system-monitor/security/status`
- `POST /api/v1/system-monitor/security/mfa/enroll`
- `POST /api/v1/system-monitor/security/mfa/confirm`
- `POST /api/v1/system-monitor/security/mfa/disable`
- `GET /api/v1/system-monitor/security/sessions`
- `POST /api/v1/system-monitor/security/sessions/revoke`
- `GET /api/v1/system-monitor/security/events?limit=50`
- `GET /api/v1/system-monitor/security/alerts?limit=50`
- `GET /api/v1/system-monitor/security/audit?limit=50`
- `GET /api/v1/system-monitor/security/grants?limit=50`

## System Monitor security centre

Migration `018_system_monitor_security` adds encrypted TOTP credential storage,
short-lived enrollment challenges, and a bounded security-event history.
Configure `SYSTEM_MONITOR_MFA_ENCRYPTION_KEY` in the API secret manager as
base64-encoded random 32-byte key material before enabling Security Centre
operations. Keep it separate from database credentials and retain a protected
backup for recovery; rotating it requires a planned MFA-secret re-encryption
migration. Without the key, Security Centre endpoints fail closed while
existing non-security Monitor reads remain available. A malformed configured
key prevents the API from starting.

Monitor users enroll with their current account password at the MFA enrollment
endpoint, then confirm a six-digit RFC 6238 TOTP code from an authenticator
app. The one-time enrollment response contains the secret/otpauth URI and is
`Cache-Control: no-store`; never log, screenshot, or retain it. Enrollment
challenges expire after ten minutes. Codes allow a one-step clock skew and
cannot be reused; failed verification attempts are persisted and limited to
ten per account per 15 minutes. A fresh code must be sent in
`X-System-Monitor-MFA` on each MFA-protected request. Endpoint pause/resume,
MFA disable, session listing/revocation, alerts, event history, audit history,
and active-grant listing all require MFA as well as an active Monitor grant.
MFA enrollment/confirmation and the status view require a staff session and an
active Monitor grant; enrollment additionally requires password reauthentication.

Session management only reports session ID, creation/expiry timestamps, and
which session is current. The Identity schema has no device, IP, or user-agent
metadata, so those details are intentionally not invented or captured here.
The current session cannot be revoked through this API, preventing accidental
self-lockout; revoke a different session with an explicit reason. MFA secrets
and raw session tokens are never returned or written to audit records.

Security alerts combine bursts of failed identity authentications (five or
more in the preceding 24 hours, grouped by account/event type) with recent
Monitor warning/critical events. Privileged actions are recorded in the
existing audit log in the same transaction as the change; the security event
view provides MFA failures and other Monitor events, while the audit endpoint
includes grant, endpoint-control, MFA, and session-control changes. Privileged
grant issuance remains an out-of-band trusted operator process, not an
administrator-facing grant API. Grant the runtime API role access to the
new MFA challenge/credential/security-event tables, `identity.sessions`,
`identity.authentication_events`, and scoped reads from `audit.audit_events`.
Restrict session updates to the revocation operation and audit inserts to the
documented security mutations; do not grant the API role schema ownership.

Migration `016_system_monitor_endpoint_controls` stores current pause state and
append-only pause/resume/expiry history. Apply it after migration 015 and grant
the API runtime database role `USAGE` on the `system_monitor` schema,
`SELECT`, `INSERT`, `UPDATE`, and `DELETE` on
`system_monitor.endpoint_controls`, and `SELECT`/`INSERT` on
`system_monitor.endpoint_control_history` before deploying endpoint controls.
Keep the migration and runtime credentials distinct.

Pause requests identify a catalogued HTTP method and route template, require a
reason and an expiration (maximum 30 days), and may set a custom HTTP status
from 400 through 599 and a response message up to 500 characters. A missing
status defaults to 503; a missing/blank message uses the standard maintenance
message. For example:

```json
{
  "method": "GET",
  "route": "/api/v1/tasks/{taskID}",
  "reason": "Approved change CHG-123",
  "response_status": 503,
  "response_message": "Tasks are temporarily unavailable.",
  "expires_at": "2026-11-20T12:00:00Z"
}
```

Send the JSON body to the pause route with the `mothercat_session` cookie,
`Content-Type: application/json`, and `X-System-Monitor-Action: confirm`.
Resume accepts `method`, `route`, and a reason. The API enforces pauses in
server-side middleware and returns a bounded JSON maintenance response with
`Retry-After`; the database is authoritative and cache refresh between API
replicas is bounded to one second. A control-store read failure fails closed
for pausable application routes.

Health checks, authentication and applicant identity routes, internal
provisioning routes, and all System Monitor routes are explicitly non-pausable
to avoid locking out access or disabling recovery. Unknown/unregistered routes
are not controllable. A background expiry job runs every 15 seconds and writes
an `expired` history record; the expiration timestamp is also checked in the
request middleware, so an endpoint becomes available at its expiry even if the
job is delayed. There is no frontend control page yet.

Migration `017_system_monitor_endpoint_analytics` adds per-endpoint, per-minute
historical aggregates for request and success/error counts, status-code
distribution, average and maximum latency, and fixed latency buckets.
Middleware queues only the method and route template, response status, elapsed
time, and minute bucket; it does not persist request paths with IDs, query
strings, headers, bodies, or response contents. A background worker batches
updates every five seconds and at graceful shutdown. Pending aggregates remain
in process memory until a successful flush, so an abrupt process or host failure
can lose the most recent unflushed interval. Completed historical aggregates
are retained for 90 days and pruned daily.
The pending buffer is capped at 10,000 endpoint/minute buckets. If database
outage or load reaches that cap, additional distinct-bucket requests are
counted as dropped in the System Monitor overview and emit a bounded
`analytics_buffer_full` event; this keeps telemetry from exhausting API memory.

The analytics API requires `method` and the exact catalog `route` template. It
accepts optional RFC3339 `from` and `to` timestamps (default: the last 24
hours), and `interval=minute|hour|day` (default: `hour`). The selected range
must be at most 90 days and produce at most 2,000 chart points. Empty buckets
are returned as zeroes; each point contains counts, error rate, average and
approximate p95 latency, status-code counts, and latency histogram counts.
Percentiles are estimates from the fixed latency buckets, not request-level
latency samples. Grant the API runtime database role `SELECT`, `INSERT`,
`UPDATE`, and `DELETE` on `system_monitor.endpoint_analytics`, and `EXECUTE` on
`system_monitor.add_jsonb_counters`, in addition to the endpoint-control table
privileges above. Analytics are currently exposed by API only; no frontend
charts or live alert stream are included.

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
