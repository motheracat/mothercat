# Single-server MongoDB deployment guide

## Deployment status

The backend is one Go API server. Identity, Organization, Access, and Audit are
internal modules sharing one MongoDB client and database; they are not separately
deployed services. MongoDB is the system of record.
The current API remains partial and is not production-ready; collection validators
and replica-set integration coverage still need implementation and testing. The
one-time administrator bootstrap endpoint is described below. No Cloudinary upload
flow is currently present.

The safe assumption for this guide is a managed MongoDB replica set (for example,
MongoDB Atlas), a single Go API deployment, and a separately hosted frontend. Use a
staging environment first.

## 1. Prepare MongoDB

1. Create separate staging and production clusters in the region closest to the API.
   The production cluster must support multi-document transactions (replica set or
   sharded cluster); a standalone MongoDB server is not sufficient for the Identity
   recovery/session flows.
2. Require TLS, enable managed encryption and automated backups, and configure
   private networking where the hosting provider supports it. Restrict network access
   to the API deployment rather than allowing all IP addresses.
3. Create a runtime database user with only the required read/write privileges on the
   application database. Use a different restricted operator identity for collection
   validators, index setup, and initial administrator provisioning.
4. Store the URI in the hosting provider's secret manager. Prefer the DNS seed-list
   URI (`mongodb+srv://...`) provided by the cluster. Never commit it, paste it into
   logs, or expose it to the frontend.
5. Agree on backup retention and perform a restore test before production use.

## 2. Prepare the single API deployment

Build and deploy the unified Go server as one application/container. It should:

- Listen on the platform-provided `PORT` and expose liveness and readiness endpoints.
- Create one MongoDB client at process startup, reuse it for all requests and domain
  packages, and disconnect it during graceful shutdown.
- Fail readiness when MongoDB is unavailable and avoid exposing driver errors or
  connection details in HTTP responses.
- Use request-scoped contexts and bounded server-selection/operation deadlines.
- Apply collection indexes and validators through a controlled setup job before the
  new application is promoted; do not make every server replica alter the database.
- Keep health checks independent of authentication and avoid putting secrets or PII
  into structured logs.

The target runtime settings are:

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | `staging` or `production`; reject unknown values |
| `PORT` | Port supplied by the hosting platform |
| `MONGODB_URI` | Secret managed-cluster connection URI |
| `MONGODB_DATABASE` | Explicit application database name |
| `IDENTITY_BOOTSTRAP_TOKEN` | Optional one-time bootstrap bearer secret; remove after bootstrap succeeds |
| `BREVO_API_KEY` | Brevo API key; optional until transactional email is enabled |
| `BREVO_FROM_EMAIL` | Verified Brevo sender email; required with the API key |
| `BREVO_FROM_NAME` | Optional sender display name |
| `PUBLIC_BASE_URL` | HTTPS frontend base URL; required with Brevo |

The Brevo settings are optional when email delivery is disabled; when any is set,
configure the API key, sender email, and public URL together. The API key is only
used by the backend and must be stored as a secret.

Use the driver's standard pool defaults initially unless measured concurrency or
cluster connection limits justify changing them. Reuse a single client; do not create
one client per request or domain package. Observe connection counts, wait-queue
events, server-selection failures, operation latency, and API concurrency before
tuning pool limits. Account for every application replica when comparing client
connections with the cluster's connection limit.

## 3. Deploy staging and test

1. Deploy the single API service to staging without routing production traffic to
   it. Keep the frontend and API on the same HTTPS origin where possible so the
   existing secure, HttpOnly session-cookie behavior can be retained.
2. Run the controlled MongoDB setup job using the restricted operator credential.
   Confirm expected unique indexes for staff IDs, account IDs, session-token hashes,
   recovery-token hashes, role/permission assignments, and audit idempotency keys.
   Confirm the permission catalog is seeded and the default Staff role has only the
   `staff.read` permission. Verify TTL indexes only apply to records that are safe
   to expire.
3. Run unit and integration tests against a replica-set MongoDB instance, including
   transaction rollback, duplicate-key conflicts, single-use recovery tokens,
   session revocation, and audit write failures.
4. Verify the externally reachable API paths, cookie flags, CSRF protections,
   authentication/authorization failures, readiness behavior, and graceful shutdown.
   Confirm that the frontend cannot access database credentials.
5. Load-test the expected staging workload and check database CPU, memory, active
   connections, query latency, and application latency. Tune only from those
   measurements.

## Initial administrator bootstrap

1. Run the MongoDB setup command before starting the API. It creates the indexes,
   default roles, and permission catalog required for bootstrap.
2. Generate a high-entropy secret (at least 32 characters) and set it as
   `IDENTITY_BOOTSTRAP_TOKEN` in the API's secret manager. Do not use the normal
   onboarding token for this purpose.
3. Call `POST /api/v1/auth/bootstrap` over HTTPS with
   `Authorization: Bearer <IDENTITY_BOOTSTRAP_TOKEN>` and a JSON body containing
   `staff_id`, `email`, `display_name`, and a strong `password`. Never place the
   token or password in a URL, source file, or logs.
4. On `201 Created`, the operation has atomically created the active SuperAdmin,
   assigned the system role and permissions, written an audit event, and closed
   bootstrap. The database must be a replica set or sharded cluster to support the
   transaction.
5. Remove `IDENTITY_BOOTSTRAP_TOKEN` from the secret manager and redeploy. The
   database closure remains effective even if the variable is accidentally retained.
   The initial administrator signs in using the normal login endpoint.

The endpoint returns `404` when no bootstrap token is configured, `401` for an
invalid token, and `409` once bootstrap is closed or an Identity account already
exists. Repeated invalid-token attempts are rate-limited per client IP.

## 4. Production security and operations

- Enforce HTTPS and secure cookie settings; use a trusted origin and explicit
  same-site/CSRF behavior.
- Keep the MongoDB URI, Brevo API key, bootstrap material, and any signing keys in
  the hosting provider's secret manager. Rotate credentials regularly and after
  suspected exposure, and restrict who can read them.
- Do not expose MongoDB to the public internet. Restrict egress/ingress to the API
  network when supported, and require TLS for all remote connections.
- Use least privilege for the application database user; the application must not be
  able to create database users or administer the cluster.
- Monitor API latency/errors, readiness, MongoDB availability, transaction aborts,
  connection-pool wait time, and backup/restore status. Alert on repeated auth
  failures and anomalous administrative activity without logging credentials or
  session tokens.
- Keep staging and production credentials and data isolated. Never use real
  production data in ordinary tests.
