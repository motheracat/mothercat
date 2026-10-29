# Backend development

The backend is organized as four independently runnable Go services: Identity, Organization, Access, and Audit.

## Local PostgreSQL

The local database runs in Docker Compose and binds only to `127.0.0.1`. Its data is kept in the named `postgres_data` volume.

1. Review `backend/.env` and replace the local placeholder password before starting the database.
2. From this directory, run `make db-up`.
3. Check readiness with `make db-status` or inspect logs with `make db-logs`.
4. Stop the container with `make db-down`. This preserves the database volume.

To intentionally delete local database data, run `docker compose down --volumes` from this directory. This is destructive.

Database migrations are not run automatically by service startup. Per the project blueprint, migrations will be owned by their service and applied through a controlled workflow after the schema decisions are finalized. Do not reuse local credentials in staging or production.
