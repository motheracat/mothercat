# Identity service

This service currently provides operational health endpoints and the initial
login, session lookup, and logout APIs. Account activation, recovery, password
change, Access integration, and complete onboarding workflows remain future
work. It does not create mock users, fabricate business responses, or run
database migrations at startup.

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
| `DATABASE_SSLMODE` | For URL without `sslmode` | Explicit TLS mode appended to `DATABASE_URL`; Render staging uses `require` |
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

Production requires `sslmode=verify-full` whether configuration is provided as
a URL or as separate PostgreSQL variables. Staging requires TLS; Render's
internal PostgreSQL connection uses a self-signed certificate, so its staging
connection uses `sslmode=require`. Supply credentials through a secret manager
or equivalent secure runtime mechanism; do not commit them or place them in
container build arguments.

The runtime database account should have only the permissions needed by the
Identity service. Migration credentials and migration execution are separate,
controlled deployment concerns and must not be added to the application runtime.
For the limited free-tier Render smoke test, migrations are run manually from a
developer workstation; see `RENDER-STAGING.md`.

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

## Render staging test

See [`RENDER-STAGING.md`](RENDER-STAGING.md) and `backend/render.yaml`. This
configuration is disposable smoke-test infrastructure only; free Render Postgres
expires after 30 days and the free web service can spin down when idle.
