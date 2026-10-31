# Backend Project Blueprint — Working Reference

This note is a concise backend-focused reference derived from the supplied *Construction & Civil Engineering Management System: Project Breakdown and Implementation Blueprint* (19 pages). The supplied PDF remains authoritative if this summary omits or conflicts with a detail. Review this note before backend implementation; keep all code and documentation changes inside `backend/` unless the user explicitly changes that scope.

## Target architecture

- Backend: one deployable Go modular monolith, one public HTTP server, and versioned REST/JSON APIs. Keep Identity, Organization, Access, and Audit as internal packages/modules for maintainability, not separate deployed services.
- Persistence: one MongoDB database accessed through a single long-lived MongoDB client. Keep collections logically separated by domain (`user_accounts`, `user_sessions`, `staff_members`, `roles`, `audit_events`, and related collections). Use a controlled MongoDB setup command for indexes and validators.
- Cross-domain operations run through in-process application interfaces. Do not make the server call its own public listener or retain inter-service URLs/tokens when consolidating.
- Keep current public API paths and response shapes where practical so the frontend does not need an unrelated rewrite.
- Deploy one API application and the frontend, plus a managed MongoDB deployment. Do not add Kubernetes or a message broker in the first release.
- **Implementation status:** the backend runs as one Go server using MongoDB. Identity, Organization, Access, and Audit remain internal Go modules; their operations communicate in-process. The current API is partial; full API coverage, collection validators, and replica-set integration coverage are not yet implemented or verified. The initial administrator bootstrap is available as a one-time, token-protected endpoint. Do not treat the prototype as production-ready until the remaining items are complete.

## Service ownership

- **Identity:** accounts, password hashes, login attempts, persistent sessions, recovery, activation, authentication, and session revocation. It does not own staff employment records or departments.
- **Organization:** staff profiles and official staff IDs, departments, memberships/reporting relationships, account-to-staff association, lifecycle, directory import, and authorized staff search.
- **Access:** roles, permissions, role assignments, delegation restrictions, and authorization policy evaluation. Resource-owning services enforce authorization; never trust frontend role names.
- **Audit:** immutable privileged-operation events and permission-controlled search. Security-sensitive work must not silently succeed when its required audit event cannot be durably recorded.

## API scope from Milestone 1

- Identity: one-time `POST /api/v1/auth/bootstrap`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/session`, `POST /auth/password/forgot`, `POST /auth/password/reset`, `POST /auth/password/change`, `POST /auth/activation/complete`.
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
- Bootstrap the initial administrator through a one-time endpoint protected by a high-entropy secret, with atomic database closure after success and an audit record. Never expose the endpoint without the secret or retain the secret after setup.

## Data and cross-domain workflows

- MongoDB has no relational foreign keys; validate references in application code and create unique/compound indexes for identifiers, lookup paths, and idempotency keys.
- Set schema validation for security-sensitive collections. Render runs the bundled, idempotent index and baseline Access setup command before each deployment, rather than from every application replica at startup. Treat future schema-validator or destructive data changes as explicitly reviewed migrations.
- Identity flows that update multiple documents (for example password recovery plus session revocation) must use MongoDB transactions. The target MongoDB deployment must support transactions (a replica set or sharded cluster); a standalone `mongod` is insufficient.
- Keep production data out of source, fixtures, and logs. Grant the MongoDB user the application database permissions needed by the API and automated pre-deploy setup, including index creation.
- Staff onboarding spans domains: prepare account, associate staff, assign authorized role, and record action. Keep the account pending/unusable until required steps succeed. Use an explicit idempotent workflow and compensation for partial failure; use a transaction only where all involved writes are in the same MongoDB deployment and transaction boundary.
- Every runtime and database setup operation must use the supplied MongoDB connection URI through `MONGODB_URI` and the explicit `MONGODB_DATABASE`, provided by the deployment environment or secret store. Do not use a local MongoDB instance, localhost URI, or local-database fallback. A connection string configures connectivity; it does not create application indexes or validators.
- Use Brevo for transactional email. Add Cloudinary only with a defined media-upload feature; do not store Cloudinary credentials in frontend code.

## Production and verification bar

- Production deployments require separate staging/production environments, HTTPS, managed secrets, restricted/private database connectivity where supported, encrypted backups and tested restore, the automated pre-deploy MongoDB setup, rollback/forward-recovery plans, and resource limits.
- The single API process needs health/readiness endpoints, structured JSON logs with correlation IDs, metrics/error monitoring, bounded database operations, and graceful shutdown of its shared MongoDB client.
- Verify with formatting, vet/static/dependency checks, unit and MongoDB replica-set integration tests, API contract validation, container/build checks, and real end-to-end workflows. Completion means verified behavior, not merely files or screens.
- Use only the supplied MongoDB URI for application and setup connections; do not rely on a local Docker Compose database.

## Delivery order and scope

1. Keep Identity, Organization, Access, and Audit in one deployable Go server while preserving internal domain boundaries and public API contracts.
2. Complete MongoDB repositories, indexes, validators, and replica-set transaction tests.
3. Verify onboarding, authentication, authorization, audit durability, failure handling, and the single-server staging deployment.
4. Do not implement later milestones or touch the frontend as part of backend work unless explicitly requested.

## Open decisions — confirm before production

- Hosting provider, region, production domain, and provider-specific deployment capabilities.
- MongoDB hosting provider, region, cluster tier, backup retention, and connection limits.
- Final MongoDB collection validators, indexes, retention policies, and production setup window.
- Official staff directory (pending from the company).
- Any policy values not specified in the blueprint, such as exact session lifetimes, revocation/cache windows, password policy, retention, or resource limits.
