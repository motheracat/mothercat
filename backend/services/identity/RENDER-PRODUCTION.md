# Render production deployment

The Blueprint at `backend/render.yaml` deploys the Identity API and its PostgreSQL
database. Both resources must be in the same Render region so the service can
use the database's private internal URL.

The Blueprint currently selects Render's free plans only to bootstrap deployment.
A keep-alive job can reduce idle spin-down for the web service, but it does not
provide an uptime guarantee. Free PostgreSQL has no point-in-time recovery and
expires after 30 days. Do not put real production accounts or data on these
temporary resources. Upgrade the database before production use, and use a paid
web plan if you need an uptime guarantee.

## Deploy and configure

1. Push the backend changes to the Git branch Render should deploy.
2. If you already created resources from the previous test Blueprint, pause
   before applying this one: the resource names changed, so Render may create
   parallel resources. Keep the existing database until its data has been
   backed up or deliberately migrated.
3. In Render, create a Blueprint from `backend/render.yaml` and check that the
   service and database are in the same region.
4. Wait for the database to be available. The Blueprint leaves `DATABASE_URL`
   unsynced intentionally; do not use the database's owner connection string as
   the API's permanent runtime credential.
5. Apply the Identity migrations using the database owner credentials as
   described below.
6. Create a restricted runtime database role and grant it only the access
   Identity uses. In `psql`, connected as the database owner:

   ```sql
   CREATE ROLE mothercat_identity_runtime LOGIN;
   ```

   Set its password interactively with:

   ```text
   \password mothercat_identity_runtime
   ```

   Then grant its required permissions:

   ```sql
   GRANT CONNECT ON DATABASE mothercat_identity TO mothercat_identity_runtime;
   GRANT USAGE ON SCHEMA identity TO mothercat_identity_runtime;
   GRANT SELECT ON identity.user_accounts TO mothercat_identity_runtime;
   GRANT SELECT, INSERT, UPDATE ON identity.user_sessions TO mothercat_identity_runtime;
   GRANT INSERT ON identity.authentication_events TO mothercat_identity_runtime;
   ```

7. In the Render Identity service's environment settings, set `DATABASE_URL` to
   a connection URL using the internal database host, database
   `mothercat_identity`, and the `mothercat_identity_runtime` credentials, with
   `sslmode=require`. Treat the URL as a secret and URL-encode special
   characters in its username or password.
8. Deploy the service and verify `/health/live` and `/health/ready`. `APP_ENV`,
   `DATABASE_SSLMODE`, and the explicit Render TLS opt-in are set by the
   Blueprint. Auto-deploy is disabled; later updates require **Manual Deploy →
   Deploy latest commit**.

Render's private PostgreSQL connections use self-signed certificates and do not
support certificate verification. `sslmode=require` encrypts the connection but
does not authenticate the database certificate. The Blueprint enables
`DATABASE_ALLOW_UNVERIFIED_TLS=true` specifically for this private-network
connection. Keep the service and database in the same region and leave the
database `ipAllowList` empty except during controlled administration. For other
production database providers, keep the opt-in disabled and use
`sslmode=verify-full`.

## Apply migrations

Production migrations require a separate `MIGRATION_DATABASE_URL`; never add
that variable to the running API service. For a workstation migration, briefly
allow only your current public IP as a `/32` in the Render database's external
access settings. Copy the database owner's external URL from Render and add
`sslmode=require` if it is not already present. Do not open access to all IPs.

From `backend/services/identity`, run:

```sh
read -rsp 'Render external PostgreSQL URL: ' MIGRATION_DATABASE_URL
printf '\n'
export MIGRATION_DATABASE_URL
export APP_ENV=production DATABASE_SSLMODE=require DATABASE_ALLOW_UNVERIFIED_TLS=true
go run ./cmd/migrate -dir migrations
unset MIGRATION_DATABASE_URL
```

Paste the owner URL only at the terminal prompt; never commit or send it in chat.
The migration runner records versions and checksums, and is safe to rerun. After
the role and grants are configured, remove the temporary external IP allowlist
entry and use only the restricted runtime URL in the Render service.

## Verify and upgrade

- Check the Render deploy logs for successful image build and service startup.
- Check `/health/live` and `/health/ready`; readiness requires a database
  connection.
- Before accepting real users or production data, upgrade the database to a paid
  plan that provides backups/point-in-time recovery. Do not wait until the free
  database approaches its deletion date.
- After upgrading, update the `plan` values in `backend/render.yaml` before a
  later Blueprint sync so it does not request the free plans again.
- Review the migration output and verify the restricted runtime account can log
  in and use Identity while it cannot alter schemas or tables.

This deployment runs only the Identity API. The other backend services are not
included in this Blueprint and are not yet implemented for production use.
