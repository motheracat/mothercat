# Identity service

This service currently provides operational health endpoints and the initial
login, session lookup, and logout APIs. Account activation, recovery, password
change, Access integration, and complete onboarding workflows remain future
work. The restricted initial SuperAdmin bootstrap command is documented below;
there is no public account-creation route. The service does not create mock users,
fabricate business responses, or run database migrations at startup.

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
