# Identity service

This service provides login/session/logout, Access-backed role and permission
details, password recovery/reset, authenticated password change, and invited
account activation. The restricted initial SuperAdmin bootstrap command is
documented below; there is no public account-creation route. Database
migrations are never run at API startup.

## Runtime configuration

Configuration is supplied through environment variables by the local development
environment or the deployment platform's secret/configuration facilities. The
service does not load `.env` files itself.

| Variable | Required | Default / behavior |
| --- | --- | --- |
| `APP_ENV` | No | `development`; allowed values are `development`, `test`, `staging`, and `production` |
| `PORT` | No | When set, binds to `:<PORT>`; takes precedence over `HTTP_ADDR` (Render supplies this) |
| `HTTP_ADDR` | No | `:8080`, used when `PORT` is unset |
| `DATABASE_URL` | One DB option | PostgreSQL URL; include `sslmode`, or provide `DATABASE_SSLMODE` explicitly |
| `DATABASE_SSLMODE` | For URL without `sslmode` | Explicit TLS mode appended to `DATABASE_URL`; Render private networking uses `require` |
| `DATABASE_ALLOW_UNVERIFIED_TLS` | No | `false`; explicitly permits `sslmode=require` in production when the database provider's private endpoint uses a self-signed certificate |
| `POSTGRES_HOST` | Alternative DB option | `localhost` |
| `POSTGRES_PORT` | Alternative DB option | `5432` |
| `POSTGRES_DB` | Alternative DB option | Required when `DATABASE_URL` is unset |
| `POSTGRES_USER` | Alternative DB option | Required when `DATABASE_URL` is unset |
| `POSTGRES_PASSWORD` | Alternative DB option | Required when `DATABASE_URL` is unset; never logged |
| `POSTGRES_SSLMODE` | Alternative DB option | `disable` in development/test, `require` in staging, `verify-full` in production |
| `DATABASE_MAX_CONNS` | No | `10`; allowed range is 1–1000 |
| `DB_CONNECT_TIMEOUT` | No | `10s`; bounds each new PostgreSQL connection attempt |
| `HEALTH_CHECK_TIMEOUT` | No | `2s`; bounds the readiness database ping |
| `SHUTDOWN_TIMEOUT` | No | `10s`; bounds graceful HTTP shutdown |
| `ACCESS_SERVICE_URL` | Production/staging | HTTPS URL of the Access service |
| `ACCESS_SERVICE_TOKEN` | Production/staging | Bearer credential shared with Access; minimum 32 characters |
| `ACCESS_SERVICE_TIMEOUT` | No | `3s`; bounded authorization lookup |
| `IDENTITY_ONBOARDING_TOKEN` | Production/staging | Separate service credential for initiating activation email |
| `SMTP_HOST`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_PORT` | Production/staging | SMTP delivery; port must be `587` and STARTTLS is mandatory |
| `PUBLIC_BASE_URL` | Production/staging | HTTPS frontend base URL used in reset/activation links |
| `SMTP_TIMEOUT` | No | `10s`; bounds SMTP connection and send time |

Production requires `sslmode=verify-full` by default, whether configuration is
provided as a URL or as separate PostgreSQL variables. Render private PostgreSQL
connections use self-signed certificates and cannot use `verify-full`; for that
private-network connection only, set `sslmode=require` and explicitly enable
`DATABASE_ALLOW_UNVERIFIED_TLS=true`. This encrypts the connection but does not
authenticate the database certificate, so keep the database private, in the
same Render region, and block external access unless temporarily needed for
administration. For other production database providers, leave the opt-in unset
and use certificate verification. Supply credentials through a secret manager
or equivalent secure runtime mechanism; do not commit them or place them in
container build arguments.

The runtime database account should have only the permissions needed by the
Identity service. Migration credentials and migration execution are separate,
controlled deployment concerns and must not be added to the application runtime.
See [`RENDER-PRODUCTION.md`](RENDER-PRODUCTION.md) for the Render deployment and
controlled migration steps.

## Health endpoints

- `GET /health/live` reports whether the HTTP process is alive; it does not depend
	on PostgreSQL.
- `GET /health/ready` pings PostgreSQL and returns `503` while the dependency is
	unavailable. The process remains running so the connection pool can recover.

HTTP responses expose a cryptographically random `X-Request-ID` header, and
request logs include the same ID. Health errors are generic and do not return
internal database details.

## Account and authorization workflows

- `POST /api/v1/auth/password/forgot` accepts `{"staff_id":"..."}` and always
  returns a generic `202` response to avoid account enumeration.
- `POST /api/v1/auth/password/reset` accepts a one-time email token and matching
  new password fields.
- `POST /api/v1/auth/password/change` requires the session cookie and current
  password. Other sessions are revoked after a successful change.
- `POST /internal/v1/accounts/{accountID}/activation` requires
  `IDENTITY_ONBOARDING_TOKEN` as a bearer token. This trusted onboarding hook
  sends an activation email for an inactive account.
- `POST /api/v1/auth/activation/complete` consumes the single-use token and
  activates the account.

Recovery tokens are stored as SHA-256 hashes, expire, and are consumed
transactionally. Password reset revokes existing sessions. Recovery and
activation endpoints have per-IP in-process rate limits; deploy multiple
instances behind the platform edge and consider a shared limiter before
increasing scale.

Login and session responses include `role`, `roles`, and `permissions` resolved
from Access. A configured Access failure fails authentication closed rather
than returning stale or empty authorization.

## Authentication timing logs

Each Identity request logs its route, status, request ID, and total
`duration_ms`. Login additionally emits `authentication stage` entries with
the same request ID and a `duration_ms` for account lookup, password hash
verification, Access authorization, and session persistence. Identity also
forwards the request ID to Access, whose logs report total request duration and
the authorization database lookup duration. Search both services' logs for the
same request ID to isolate latency. Logs deliberately omit passwords, session
tokens, staff IDs, and account IDs.

## Container

Build the image with this directory as its build context. It uses a pinned Go
toolchain, a multi-stage build, a minimal final image, and a non-root runtime user.
TLS termination is expected at the HTTPS edge/reverse proxy, as specified by the
platform blueprint. Local Docker Compose configuration is for development only;
it is not production deployment configuration.

## Render deployment

See [`RENDER-PRODUCTION.md`](RENDER-PRODUCTION.md) and `backend/render.yaml`.
Free Render resources are temporary bootstrap infrastructure, not an acceptable
production runtime or store for real production data.
Before deploying the updated blueprint, set its `sync: false` variables for both
services. Use independent random values for `ACCESS_SERVICE_TOKEN` and
`IDENTITY_ONBOARDING_TOKEN`; configure `ACCESS_SERVICE_URL` as the Access
service's HTTPS URL, and use SMTP credentials from a trusted provider.

## Initial SuperAdmin bootstrap

The initial administrator is created through the operator-only
`backend/cmd/bootstrap-superadmin` command. It reads account fields from a
terminal, reads the password without echoing it, creates the Identity account
inactive, assigns the system SuperAdmin permissions through the Access-owned
bootstrap package, records an immutable initialization event through the
Audit-owned package, then activates the account and consumes the short-lived
secret. No public setup endpoint is registered.

Apply migrations for Identity, Access, and Audit before running the command.
The bootstrap command requires `BOOTSTRAP_DATABASE_URL`, `BOOTSTRAP_SECRET`,
`BOOTSTRAP_SECRET_EXPIRES_AT` (RFC3339, no more than one hour ahead), and
`APP_ENV`. The connection is elevated and temporary; do not use the running
Identity service's runtime database credential. See
[`RENDER-PRODUCTION.md`](RENDER-PRODUCTION.md) for the complete procedure.
If a step fails, the account remains inactive. Retry with the same secret and
password before expiry; after expiry, issue a new short-lived secret and retry.
Remove the secret from the environment/secret store after success.
