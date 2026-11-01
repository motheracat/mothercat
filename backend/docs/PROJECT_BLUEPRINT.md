# Backend Project Blueprint — Working Reference

This backend reference follows the supplied project blueprint's PostgreSQL
target. The supplied blueprint remains authoritative if this summary omits a
requirement. Keep backend migration changes inside `backend/`; do not change the
frontend as part of this database migration.

## Target architecture

- One deployable Go modular monolith, one public HTTP server, and versioned
  REST/JSON APIs. Identity, Organization, Access, and Audit remain internal
  modules, not separately deployed services.
- Supabase-hosted PostgreSQL is the system of record. One database contains
  service-owned `identity`, `organization`, `access`, and `audit` schemas.
- Keep current public API paths and response shapes where practical. In-process
  application interfaces connect domain operations; do not make the server call
  its own public listener.
- Deploy one API application and the frontend with a managed PostgreSQL database.
  Do not add Kubernetes or a message broker in the first release.
- **Migration status:** the API persistence layer and an initial schema migration
  have been implemented, but database-backed integration/cutover verification
  remains. Do not treat the migration as production-ready until staging has
  verified schema, imported data, permissions, restore, and end-to-end flows.

## Service ownership

- **Identity:** accounts, password hashes, login events, persistent sessions,
  recovery, activation, authentication, and session revocation. It does not own
  staff employment records or departments.
- **Organization:** staff profiles and official staff IDs, departments,
  memberships/reporting relationships, account-to-staff association, lifecycle,
  directory import, and authorized staff search.
- **Access:** roles, permissions, role assignments, delegation restrictions, and
  authorization policy evaluation. Resource-owning services enforce
  authorization; never trust frontend role names.
- **Audit:** immutable privileged-operation events and permission-controlled
  search. Security-sensitive work must not silently succeed when its required
  audit event cannot be durably recorded.

## Data and persistence

- Use PostgreSQL foreign keys, unique constraints, and indexes for identity,
  relationship, lookup, and idempotency guarantees.
- Keep domain data in its owning schema. Avoid cross-domain writes except within
  explicit application workflows and transaction boundaries.
- Use controlled, versioned migrations. Do not run privileged DDL from every API
  replica or startup. Review destructive schema changes separately.
- Use separate credentials: a least-privilege API role in `DATABASE_URL` and a
  privileged, short-lived migration/import role in `MIGRATIONS_DATABASE_URL`.
  Require TLS for both in staging and production.
- Do not use a local database fallback. Keep production data out of source,
  fixtures, and logs.
- Staff onboarding spans domains: create/associate the account, assign the
  authorized role, and record the action. Keep the account unusable until
  required steps succeed; provide idempotency and compensation for partial
  failures.
- Keep existing bcrypt password hashes during migration. Import only opaque
  session/recovery-token hashes, and preserve their expiry/use/revocation state.

## API scope from Milestone 1

- Identity: one-time `POST /api/v1/auth/bootstrap`, login/logout/session,
  password recovery/reset/change, and activation completion.
- Organization: staff list/create/get/update/deactivate/import and
  import-status endpoints; department list/create/update endpoints.
- Access: role list/create/update; permission list; replace role permissions;
  inspect and assign user roles.
- Audit: paginated/filterable audit-event search and authorized event retrieval.
  No general edit/delete endpoint.
- Use consistent machine-readable errors, meaningful status codes,
  input/identifier validation, pagination, correlation IDs in logs/errors,
  documented permissions/request/response/error schemas, and concurrency
  protection where needed.

## Security and identity requirements

- Authenticate staff with official staff ID plus password. The official staff ID
  is not the internal account ID; keep staff records separate from credentials
  and associate them through immutable internal account IDs.
- Use server-managed persistent sessions. Store only an opaque,
  cryptographically random session identifier in a Secure, HttpOnly,
  appropriately SameSite cookie; rotate on sensitive transitions and protect
  cookie-authenticated writes against CSRF.
- Revoke sessions on logout, password reset, account suspension, and configured
  security events. Define a revocation window if session/permission data is
  cached.
- Use generic login failures and rate limits. Recovery/activation tokens must be
  random, short-lived, single-use, and stored securely.
- Never hardcode staff accounts/credentials, expose password hashes or secrets,
  or trust frontend authorization as enforcement.
- Bootstrap the initial administrator through a one-time endpoint protected by a
  high-entropy secret, with atomic database closure after success and an audit
  record. Remove the secret after setup.

## Production and verification bar

- Use isolated staging/production Supabase projects, HTTPS, managed secrets,
  restricted database access, encrypted backups and tested restore, controlled
  migrations, and rollback/forward-recovery plans.
- The API process needs health/readiness endpoints, structured logs with
  correlation IDs, monitoring, bounded database operations, and graceful
  shutdown of its shared connection pool.
- Verify formatting, tests, vet/static/dependency checks, Supabase-compatible
  PostgreSQL integration tests, API contracts, container/build checks, and
  real end-to-end workflows. Completion means verified behavior, not merely
  files or screens.

## Delivery order and scope

1. Keep all four domains in one deployable Go server while preserving internal
   boundaries and public API contracts.
2. Verify schema migrations on a clean PostgreSQL database and import a
   representative MongoDB export into an empty target.
3. Verify onboarding, authentication, authorization, audit durability, error
   handling, backups/restores, and a single-server staging deployment.
4. Cut production over only after data counts and sampled records are verified.
   Keep MongoDB unchanged and available for rollback until the PostgreSQL API has
   been accepted in production.
5. Do not implement later milestones or touch the frontend as part of this
   backend work unless explicitly requested.
