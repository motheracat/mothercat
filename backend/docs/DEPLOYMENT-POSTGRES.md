# PostgreSQL staging, migration, and cutover guide

Use this guide to validate the PostgreSQL deployment in an isolated staging
environment before any production cutover. It does not replace the supplied
project blueprint or the hosting provider's secret-management procedures.

## Safety requirements

- Use a dedicated staging database/project. Never use production data or
  production credentials for a staging test.
- Keep the MongoDB source unchanged and available until the PostgreSQL
  deployment has been accepted in production.
- Do not paste database URLs, tokens, exports, or personal data into chat,
  source control, CI logs, or command output.
- Configure `DATABASE_URL` with a least-privilege application role and
  `MIGRATIONS_DATABASE_URL` with a separate, short-lived migration/import role.
  Both URLs must require TLS.
- Migrations have no automatic rollback. Take a recoverable backup before
  applying them; review corrective/forward migrations separately.
- Never restore a backup over the source database. Restore to a separate
  isolated database and verify it there.

## 1. Prepare staging

1. Create an isolated PostgreSQL staging database compatible with the target
   Supabase PostgreSQL version. Enable encrypted backups and document the
   restore procedure.
2. Configure the API's `DATABASE_URL` and the operator's
   `MIGRATIONS_DATABASE_URL` through approved secret storage. Confirm both
   connections use TLS, and do not give the runtime API role migration-owner
   privileges.
3. Configure the API with `APP_ENV=staging`, an identity onboarding token if
   testing account onboarding, and any email settings needed for the test.
   Configure the frontend's `API_BASE_URL` to the staging API origin and use
   strict authentication settings.
4. Record the source export date, collection counts, and a checksum for each
   prepared MongoDB export file. Restrict access to exports because they may
   contain personal data and password hashes.

## 2. Apply schema migrations

From the `backend/` directory, make `MIGRATIONS_DATABASE_URL` available to the
approved operator environment and run:

```sh
make db-migrate
```

The migration command requires a PostgreSQL URL with `sslmode=require`,
`sslmode=verify-ca`, or `sslmode=verify-full`. It applies the embedded,
versioned migrations under a database advisory lock. Confirm the command
completed successfully and that the expected entries exist in
`public.schema_migrations` before deploying an API version that depends on
them.

## 3. Import a MongoDB export when required

Import only into a newly prepared, empty PostgreSQL target. The importer refuses
to proceed if application data already exists in its target tables. It expects
newline-delimited JSON files named for the collections, such as
`identity_accounts.json` and `staff_members.json`.

With the restricted export directory and `MIGRATIONS_DATABASE_URL` configured
for the approved operator environment, run from `backend/`:

```sh
go run ./services/monolith/cmd/import-mongodb-json
```

The importer requires `MONGODB_EXPORT_DIR` and `MIGRATIONS_DATABASE_URL`, and
imports the supplied records in one transaction. Review its output for each
collection. Compare source and target counts, then sample records and verify
account IDs, staff associations, role assignments, audit events, and the
expiry/revocation state of imported sessions and recovery tokens. Investigate
every discrepancy before proceeding; do not rerun an import against a populated
database.

If staging is intentionally initialized without a MongoDB import, document
that decision and use the one-time bootstrap flow only for the empty staging
database. Do not bootstrap over imported identities.

## 4. Validate the build and deploy staging

Run the backend checks from `backend/`:

```sh
make format-check
make test
make vet
make build
```

Run the frontend checks from `mothercat-frontend/`:

```sh
npm ci
npm run lint
npm run typecheck
npm run build
```

Deploy the API and frontend to staging using the same build configuration
intended for production, with staging-only URLs and secrets. Apply required
migrations before deploying code that depends on them; migrations are not run
automatically by API replicas or the production CI/CD workflow.

## 5. Milestone 1 staging acceptance

Record a pass/fail result, evidence, and any follow-up issue for each check:

- `/health/live` responds successfully and `/health/ready` verifies the
  database connection.
- A provisioned staff account can activate, sign in, retrieve its session,
  change its password, sign out, and no longer use the revoked session.
- Staff and department reads return the expected staging data; authorized
  create/update/import operations work and unauthorized operations are denied.
- Role and permission changes take effect as intended. A user cannot obtain
  access merely by changing frontend state or role labels.
- Privileged operations are durably recorded in the audit log; verify both
  allowed and denied access to audit records.
- Invalid inputs, duplicate operations, stale versions, and unavailable
  dependencies return explicit errors without partial success.
- The frontend proxy, cookie, origin/CSRF checks, and authenticated navigation
  work on the deployed HTTPS staging origin.
- Database counts and representative records match the migration/import
  evidence. Verify that no secrets or sensitive record contents appear in
  application logs.
- Restore a recent staging backup into a separate database, start the API
  against that restored database, and repeat readiness and smoke tests. Record
  recovery time, data-loss window, gaps, and corrective actions.

Do not declare Milestone 1 accepted until every required acceptance criterion
has evidence and all blocking gaps are closed. Preserve the signed-off results
and restore-drill record in the project's approved operations documentation.

## 6. Production cutover gate

Production cutover is a separate, explicitly approved operation. Before it:

1. Confirm staging acceptance, source/target count reconciliation, sampled
   records, and a successful restore drill.
2. Take and verify a production backup; prepare a rollback/forward-recovery
   plan and assign an owner for each step.
3. Apply reviewed migrations using the controlled migration credential before
   deploying dependent API code. Do not place that credential in the API
   runtime environment.
4. Keep MongoDB unchanged and available for rollback until the PostgreSQL
   production system has passed its smoke tests and has been formally accepted.
5. Monitor API readiness, authentication, authorization, audit writes, error
   rates, and data reconciliation after release. Roll back or stop the cutover
   if critical checks fail; do not silently retry data imports.
