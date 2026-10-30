# Access service

The Access service owns role and permission assignments. Its internal
authorization endpoint is consumed by Identity when login or session details
are returned:

- `GET /internal/v1/users/{userID}/authorization` requires a bearer
  `ACCESS_SERVICE_TOKEN`.
- `GET /health/live` is process-only health; `GET /health/ready` also checks
  PostgreSQL.
- There is no public role-management or bootstrap route.
- Structured logs include request ID, route, HTTP duration, and the duration of
  the authorization database lookup. Identity forwards its request ID so a
  login can be traced across both services.

## Runtime configuration

| Variable | Required | Behavior |
| --- | --- | --- |
| `DATABASE_URL` | Yes | PostgreSQL URL with a database and password |
| `DATABASE_SSLMODE` | If URL omits it | Explicit PostgreSQL TLS mode |
| `DATABASE_ALLOW_UNVERIFIED_TLS` | For Render's `sslmode=require` | Must be `true` to permit this mode in production |
| `ACCESS_SERVICE_TOKEN` | Yes | Shared bearer credential; generate at least 32 random bytes |
| `PORT` | No | Render HTTP port; otherwise use `HTTP_ADDR` or `:8080` |
| `APP_ENV` | No | Defaults to `development`; production rejects plaintext PostgreSQL modes |

Use the same database as Identity until the data model is deliberately split:
the services share account UUIDs, while Access owns its schema. Apply Access
migrations before starting this service. Do not apply migrations at startup.

The Render blueprint currently uses a shared database credential to keep the
bootstrap workflow compatible. Before production use, create a dedicated
least-privilege runtime database role and grant it only the schema/table rights
required by Access; keep migration/bootstrap credentials separate.

## Container

Build with this directory as the Docker context. The image is multi-stage and
runs as a non-root user.

The initial SuperAdmin role/permissions are provisioned only by the restricted
one-time operator command at
[`backend/cmd/bootstrap-superadmin`](../../cmd/bootstrap-superadmin).