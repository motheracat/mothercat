# PostgreSQL and Supabase deployment guide

## Deployment status

The Go API now uses PostgreSQL. MongoDB is not a runtime dependency. A controlled
schema migration and a one-time Mongo export importer are available, but a live
Supabase migration/cutover has not been run. **Do not deploy the current API
against the production service until you have migrated and verified its data.**
Use a separate Supabase staging project and Render staging service first.

## 1. Prepare Supabase

1. Create isolated staging and production Supabase projects in the appropriate
   region. Enable backups/PITR where available and test a restore.
2. Obtain TLS-enabled PostgreSQL connection URLs from Supabase. Use the direct
   connection if the runtime network supports it; otherwise use Supabase's
   recommended pooler mode. Confirm that the chosen pooler mode supports the
   API driver's connection behavior.
3. Create/use separate database roles for the API and controlled migrations.
   The migration/import role needs schema/table creation and write access. The
   API role needs only the DML privileges required by the application; it must
   not own schemas or have user/role administration privileges.
4. Store the API URL as `DATABASE_URL` and the migration URL as
   `MIGRATIONS_DATABASE_URL` in the relevant secret manager. Do not put either
   URL in source control, deployment logs, shell history, or chat. Require TLS
   (`sslmode=require` or stronger).
5. Restrict database access to the API/migration networks where supported.
   Keep production and staging credentials and data isolated.

## 2. Apply the PostgreSQL schema

Run the controlled migration once against the intended empty staging database
before deploying the API:

```sh
cd backend
make db-migrate
```

Provide `MIGRATIONS_DATABASE_URL` through a secure environment/secret mechanism;
do not place the URL directly in the command. The command applies embedded,
versioned migrations and initializes the baseline Access roles/permissions.
It is not run automatically by API replicas. Confirm that the migration exits
successfully and that the `identity`, `organization`, `access`, and `audit`
schemas and `public.schema_migrations` exist.

## 3. Export existing MongoDB data

Take a recoverable backup of the source database first. Schedule a maintenance
window for the final export so writes stop before the export and remain stopped
until cutover is verified. Never delete or overwrite the source database during
this procedure.

Use MongoDB Database Tools `mongoexport` to produce relaxed Extended JSON
newline-delimited files for the following collections:

```text
identity_accounts
identity_sessions
identity_recovery_tokens
identity_authentication_events
system_bootstrap_state
roles
permissions
role_permissions
user_roles
departments (optional)
department_memberships (optional)
staff_import_batches (optional)
staff_members
audit_events
```

Create a private export directory outside the repository. Configure the
`mongoexport` connection with a private configuration file restricted to the
current user (mode `0600`) so the URI does not appear in command history or
process arguments. Export each collection with `--jsonFormat=relaxed` and a
separate output file named `<collection>.json`. For an absent/empty optional
collection, provide an empty file. The importer requires each other named file,
even when it contains no records.

Before importing, inspect the export record counts without printing record
contents. Check that the account, staff, role, permission, and assignment counts
match expectations. Keep the exports private: they contain personal information,
password hashes, and authentication metadata.

## 4. Import into the empty staging database

The importer requires the migrations to have run and rejects a target that
already contains application accounts, staff, or audit events. It reads
newline-delimited JSON exports and imports them in one transaction. If a record
fails validation or violates a constraint, the import rolls back; fix the source
export or mapping issue, then retry against a clean target.

Run it from the backend workspace with both secrets injected securely:

```sh
cd backend
go run ./services/monolith/cmd/import-mongodb-json
```

Set `MONGODB_EXPORT_DIR` to the private export directory and
`MIGRATIONS_DATABASE_URL` to the staging migration URL in the execution
environment. The importer preserves UUIDs, bcrypt hashes, active/password-change
flags, role and permission assignments, staff fields, audit history, and
session/recovery-token hashes with their expiry and revocation/use status. The
importer does not copy MongoDB `_id` values that are ObjectIds; application UUID
fields are preserved.

After import, independently compare source and destination counts for every
collection/table. Verify a sample of account-to-staff associations, roles and
permissions, department references, audit events, account activation states,
password login, session behavior, and token expiry. Test backup/restore and
application flows on staging before considering production.

## 5. Deploy and verify staging

1. Set the Render staging API's `DATABASE_URL` to the staging app-role URL.
   Keep `MIGRATIONS_DATABASE_URL` out of the web service's runtime environment.
   Keep the bootstrap token unset unless intentionally bootstrapping a new
   installation.
2. Deploy the PostgreSQL API only after schema and import verification. Check
   readiness, login, logout, session lookup, password recovery, activation,
   staff reads/writes, authorization decisions, and audit durability.
3. Confirm API logs do not include database URLs, tokens, password hashes, or
   personal record contents. Exercise database-unavailable readiness and
   graceful shutdown.
4. Run all backend tests, vet, build, container build, and PostgreSQL integration
   tests against a Supabase-compatible staging database.

## 6. Production cutover and rollback

1. Back up MongoDB and Supabase and verify restore procedures. Agree on a
   maintenance window and a rollback owner.
2. Stop writes to the MongoDB-backed API. Export the final data and run the same
   schema/import/verification sequence against the production Supabase project.
   Do not attempt dual writes.
3. Update Render's production `DATABASE_URL` to the least-privilege production
   PostgreSQL URL, then deploy the PostgreSQL API. Keep the MongoDB service and
   its final backup intact and read-only during the observation period.
4. Verify readiness, authentication, authorization, staff operations, audit
   events, counts, and monitoring before reopening normal traffic.
5. If verification fails, stop writes to the PostgreSQL API and restore the
   previous MongoDB-backed deployment and connection configuration. Do not
   point the old MongoDB API at PostgreSQL or discard either database.
6. Retire MongoDB only after the agreed production acceptance and retention
   period. Securely dispose of private export files according to the data
   retention policy.

## Organization API migration

The Organization API adds staff and department management, including synchronous
CSV directory import. Apply pending PostgreSQL migrations from `backend/` before
deploying this API version:

```sh
make db-migrate
```

Provide `MIGRATIONS_DATABASE_URL` through a secure environment or secret
mechanism, require TLS, and never paste the URL into chat or source control.
On a database that has already applied migrations 001 and 002, this applies
`003_organization_api`, which adds indexes used by staff listing, import-batch
lookup, and department ordering.

The staff import endpoint accepts one multipart `file` containing a CSV with
required `staff_id` and `full_name` headers. Optional headers are `work_email`,
`job_title`, and `department_id`; unknown or duplicate headers fail validation.
Uploads are limited to 5 MiB and 5,000 data rows. Each import is all-or-nothing:
successful rows create pending staff records only and do not create login
accounts or assign roles. Failed batches retain a bounded set of row issues and
can be retrieved using the import-status endpoint.

Staff deactivation also suspends a linked Identity account and invalidates its
sessions and outstanding recovery/activation tokens. If Identity suspension
succeeds but the Organization update fails, the account remains suspended; an
administrator must resolve the failure before considering deactivation
complete.

## Runtime settings

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | `staging` or `production`; reject unknown values |
| `PORT` | Port supplied by the hosting platform |
| `DATABASE_URL` | TLS-enabled runtime PostgreSQL connection URL |
| `MIGRATIONS_DATABASE_URL` | Privileged, controlled migration/import URL; not an API runtime secret |
| `IDENTITY_BOOTSTRAP_TOKEN` | Optional one-time bootstrap bearer secret; remove after bootstrap succeeds |
| `BREVO_API_KEY` | Brevo API key; optional until transactional email is enabled |
| `BREVO_FROM_EMAIL` | Verified Brevo sender email; required with the API key |
| `BREVO_FROM_NAME` | Optional sender display name |
| `PUBLIC_BASE_URL` | HTTPS frontend base URL; required with Brevo |

Brevo settings are optional when email delivery is disabled; when any is set,
configure the API key, sender email, and public URL together. Keep the API key
only in the backend secret manager.
