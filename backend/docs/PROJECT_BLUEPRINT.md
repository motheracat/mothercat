# Backend Project Blueprint — Working Reference

This note is a concise backend-focused reference derived from the supplied *Construction & Civil Engineering Management System: Project Breakdown and Implementation Blueprint* (19 pages). The supplied PDF remains authoritative if this summary omits or conflicts with a detail. Review this note before backend implementation; keep all code and documentation changes inside `backend/` unless the user explicitly changes that scope.

## Fixed architecture

- Backend: independently runnable Go services with versioned REST/JSON APIs under `/api/v1`.
- Initial services: Identity, Organization, Access, and Audit. Do not split further without a workflow need.
- Persistence: PostgreSQL; one schema owned by each service. Each service owns its schema, migrations, and queries. Other services interact through APIs or explicit integration contracts, not direct writes to another service's tables.
- Each service is its own Go module with `cmd/api/main.go`, `internal/{config,domain,application,repository,http,observability}`, `internal/http/{handlers,middleware,routes.go}`, `migrations/`, `tests/`, `Dockerfile`, `go.mod`, and `README.md`.
- OpenAPI contracts live in `contracts/openapi/`, one specification per service. Migrations live with their owning service and are run through a controlled deployment workflow, not automatically by every service at startup.
- Start with managed hosting; no Kubernetes or message broker in the first release. Use HTTP between services initially.

## Service ownership

- **Identity:** accounts, password hashes, login attempts, persistent sessions, recovery, activation, authentication, and session revocation. It does not own staff employment records or departments.
- **Organization:** staff profiles and official staff IDs, departments, memberships/reporting relationships, account-to-staff association, lifecycle, directory import, and authorized staff search.
- **Access:** roles, permissions, role assignments, delegation restrictions, and authorization policy evaluation. Resource-owning services enforce authorization; never trust frontend role names.
- **Audit:** immutable privileged-operation events and permission-controlled search. Security-sensitive work must not silently succeed when its required audit event cannot be durably recorded.

## API scope from Milestone 1

- Identity: `POST /auth/login`, `POST /auth/logout`, `GET /auth/session`, `POST /auth/password/forgot`, `POST /auth/password/reset`, `POST /auth/password/change`, `POST /auth/activation/complete`.
- Organization: staff list/create/get/update/deactivate/import and import-status endpoints; department list/create/update endpoints.
- Access: role list/create/update; permission list; replace role permissions; inspect and assign user roles.
- Audit: paginated/filterable audit-event search and authorized event retrieval. No general edit/delete endpoint.
- Use consistent machine-readable errors, meaningful status codes, input/identifier validation, pagination, correlation IDs in logs/errors, and documented permissions/request/response/error schemas. Use concurrency protection where needed.

## Security and identity requirements

- Authenticate staff with official staff ID plus password. The official staff ID is not the internal account ID; keep staff records separate from credentials and associate them through immutable internal account IDs.
- Use server-managed persistent sessions. Store only an opaque, cryptographically random session identifier in a Secure, HttpOnly, appropriately SameSite cookie; rotate on sensitive transitions and protect cookie-authenticated writes against CSRF.
- Revoke sessions on logout, password reset, account suspension, and configured security events. Define a revocation window if session/permission data is cached.
- Use generic login failures and rate limits. Recovery/activation tokens must be random, short-lived, single-use, and stored securely.
- Never hardcode staff accounts/credentials, expose password hashes or secrets, or trust frontend authorization as enforcement. No local-storage authoritative business data or fake success behavior.
- Bootstrap the initial administrator through a restricted, one-time operation with a short-lived secret, no permanent public setup endpoint, and an audit record.

## Data, migrations, and cross-service workflows

- The PDF's schema/table listing is illustrative, not a finalized production schema. Before the first migration, finalize identity account/session tables, audit fields, cross-service foreign-key/integrity strategy, and exact status constraints.
- Use versioned SQL migrations (the PDF suggests golang-migrate). Test clean installs and representative upgrades. Use expand-and-contract for zero-downtime changes; document rollback/forward recovery.
- Keep separate least-privilege migration and runtime database credentials; runtime services must not alter arbitrary schemas. Never put real credentials or production data in migrations or fixtures.
- Staff onboarding spans services: prepare account (Identity), associate staff (Organization), assign authorized role (Access), record action (Audit). Use an explicit orchestrated workflow with idempotency, retries, and compensations; keep the account pending/unusable until required steps succeed. Do not claim a distributed transaction.

## Production and verification bar

- Production deployments require separate staging/production environments, HTTPS, managed secrets, restricted/private database connectivity where supported, backups and tested restore, controlled migration jobs, rollback plans, resource/scaling limits, and least-privilege credentials per service.
- Services need health/readiness endpoints, structured JSON logs with correlation IDs, metrics/error monitoring, and graceful shutdown.
- Verify with formatting, vet/static/dependency checks, unit and PostgreSQL integration tests, API contract validation, container/build checks, and real end-to-end workflows. Completion means verified behavior, not merely files or screens.
- Clearly label local Docker Compose as development-only; it is not production deployment configuration.

## Delivery order and scope

1. Repository/service foundations and backend development workflow.
2. Database foundation and owned, tested migrations.
3. Implement and verify the Identity + Organization onboarding/authentication flow.
4. Add Access and Audit capabilities required to complete the Milestone 1 workflows, then verify authorization, audit, failure handling, and staging readiness.
5. Do not implement later milestones or touch the frontend as part of backend work unless explicitly requested.

## Open decisions — do not assume

- Hosting provider, region, production domain, and provider-specific deployment capabilities.
- Final production schema details listed above.
- Official staff directory (pending from the company).
- Any policy values not specified in the blueprint, such as exact session lifetimes, revocation/cache windows, password policy, retention, or resource limits.
