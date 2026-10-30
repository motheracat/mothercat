# Render production deployment

The Blueprint at `backend/render.yaml` deploys Identity, Organization, Access,
Audit, and their shared PostgreSQL database. All resources must be in the same
Render region so the services can use the database's private internal URL.

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
4. Wait for the database to be available. The Blueprint leaves each service's
   `DATABASE_URL` unsynced intentionally; do not use the database owner
   connection string as an API's permanent runtime credential.
5. Apply the Identity, Organization, Access, and Audit migrations, including
   Identity migration `004_secure_recovery_tokens.sql`, using the database
   owner credentials as described below.
6. Create separate restricted runtime roles for each service. In `psql`,
   connected as the database owner:

   ```sql
   CREATE ROLE mothercat_identity_runtime LOGIN;
   CREATE ROLE mothercat_organization_runtime LOGIN;
   CREATE ROLE mothercat_access_runtime LOGIN;
   CREATE ROLE mothercat_audit_runtime LOGIN;
   ```

   Set each password interactively with:

   ```text
   \password mothercat_identity_runtime
   \password mothercat_organization_runtime
   \password mothercat_access_runtime
   \password mothercat_audit_runtime
   ```

   Grant each role only its service's runtime access:

   ```sql
   GRANT CONNECT ON DATABASE mothercat_identity TO mothercat_identity_runtime;
   GRANT CONNECT ON DATABASE mothercat_identity TO mothercat_organization_runtime;
   GRANT CONNECT ON DATABASE mothercat_identity TO mothercat_access_runtime;
   GRANT CONNECT ON DATABASE mothercat_identity TO mothercat_audit_runtime;
   GRANT USAGE ON SCHEMA identity TO mothercat_identity_runtime;
   GRANT SELECT, UPDATE ON identity.user_accounts TO mothercat_identity_runtime;
   GRANT SELECT, INSERT, UPDATE ON identity.user_sessions TO mothercat_identity_runtime;
   GRANT SELECT, INSERT, UPDATE ON identity.password_reset_tokens TO mothercat_identity_runtime;
   GRANT INSERT ON identity.authentication_events TO mothercat_identity_runtime;
   GRANT USAGE ON SCHEMA organization TO mothercat_organization_runtime;
   GRANT SELECT, INSERT ON organization.staff_members TO mothercat_organization_runtime;
   GRANT SELECT ON organization.departments TO mothercat_organization_runtime;
   GRANT USAGE ON SCHEMA access TO mothercat_access_runtime;
   GRANT SELECT ON access.roles, access.role_permissions, access.user_roles TO mothercat_access_runtime;
   GRANT USAGE ON SCHEMA audit TO mothercat_audit_runtime;
   GRANT SELECT, INSERT ON audit.audit_events TO mothercat_audit_runtime;
   ```

7. Set each service's `DATABASE_URL` to a connection URL using the internal
   database host, database `mothercat_identity`, and that service's runtime
   credentials, with `sslmode=require`. Treat all URLs as secrets and
   URL-encode special characters in usernames or passwords.
8. Configure Organization with `IDENTITY_SERVICE_URL` (Identity HTTPS URL),
   `AUDIT_SERVICE_URL` (Audit HTTPS URL), and `AUDIT_SERVICE_TOKEN` (a fresh
   random value of at least 32 characters). Set the same audit token on Audit.
10. Configure Identity with its Access settings:
   - `ACCESS_SERVICE_URL`: Access's HTTPS Render URL.
   - `ACCESS_SERVICE_TOKEN`: a fresh random value of at least 32 characters;
     set the exact same value on Access.
   - `IDENTITY_ONBOARDING_TOKEN`: a different fresh random value of at least 32
     characters.
   Email is optional for now. Leave all `SMTP_*` settings and `PUBLIC_BASE_URL`
   unset to run without email; password recovery and account activation
   requests will return `503 email_delivery_disabled`. To enable those flows,
   configure all SMTP settings and `PUBLIC_BASE_URL` together; SMTP must
   support STARTTLS on port 587. If `SMTP_PORT` or any other SMTP setting was
   previously added to Render, remove it when leaving email disabled.
11. Deploy all four services and verify `/health/live` and `/health/ready` on
    each. `APP_ENV`, `DATABASE_SSLMODE`, and the explicit Render TLS opt-in are
    set by the Blueprint. Auto-deploy is disabled; later updates require
    **Manual Deploy → Deploy latest commit**.

The Organization staff-creation endpoint requires a matching `Origin` and
`Host` to protect its session-cookie write. Browser traffic must reach
Organization through a same-origin path/reverse proxy alongside the frontend
and Identity session; calls directly from a frontend on a separate Render
service domain will be rejected. Configure that routing layer before exposing
the staff-creation UI.

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

From `backend/services/identity`, run the versioned migrations owned by each
service:

```sh
read -rsp 'Render external PostgreSQL URL: ' MIGRATION_DATABASE_URL
printf '\n'
export MIGRATION_DATABASE_URL
export APP_ENV=production DATABASE_SSLMODE=require DATABASE_ALLOW_UNVERIFIED_TLS=true
go run ./cmd/migrate -dir migrations
cd ../organization
go run ./cmd/migrate -dir migrations
cd ../access
go run ./cmd/migrate -dir migrations
cd ../audit
go run ./cmd/migrate -dir migrations
unset MIGRATION_DATABASE_URL
cd ../identity
cd ../..
```

Paste the owner URL only at the terminal prompt; never commit or send it in chat.
Each migration runner records versions and checksums and is safe to rerun.
Keep the temporary external IP allowlist entry until the initial administrator
bootstrap is complete; then remove it and use only the restricted runtime URL
in the Render service.

## Bootstrap the initial SuperAdmin

The bootstrap command is operator-run and does not add a public setup route. It
creates the account inactive, assigns the initial Milestone 1 SuperAdmin
permissions through Access, writes the Audit event, and only then activates the
account and consumes the secret. Keep the temporary IP allowlist in place only
for this operation.

From `backend`, generate a 32-byte random secret in the current shell, give it
a 30-minute expiry, and read the external database URL without echoing it:

```sh
export APP_ENV=production DATABASE_ALLOW_UNVERIFIED_TLS=true
export BOOTSTRAP_SECRET="$(openssl rand -hex 32)"
export BOOTSTRAP_SECRET_EXPIRES_AT="$(date -u -d '+30 minutes' '+%Y-%m-%dT%H:%M:%SZ')"
read -rsp 'Render external PostgreSQL URL: ' BOOTSTRAP_DATABASE_URL
printf '\n'
export BOOTSTRAP_DATABASE_URL
cd cmd/bootstrap-superadmin
go run .
unset BOOTSTRAP_SECRET BOOTSTRAP_SECRET_EXPIRES_AT BOOTSTRAP_DATABASE_URL
```

The command prompts for the initial staff ID, email, display name, and a strong
password (hidden input; minimum 14 characters and at least three character
classes). It requires the database-owner URL in `BOOTSTRAP_DATABASE_URL` because
the operation writes through the Identity-, Access-, and Audit-owned bootstrap
packages. Do not place that URL or bootstrap secret in the running API service.
The database stores only the secret hash and clears it after success. If a
service step fails, the account remains inactive; retry with the same secret
and password before it expires. If the secret expires, generate a new one and
rerun with the original account details and password. After successful
completion, remove the temporary external IP allowlist entry immediately.

## Verify and upgrade

- Check the Render deploy logs for successful image builds and service startup.
- Check `/health/live` and `/health/ready` on Identity and Access; readiness
  requires a database connection.
- Before accepting real users or production data, upgrade the database to a paid
  plan that provides backups/point-in-time recovery. Do not wait until the free
  database approaches its deletion date.
- After upgrading, update the `plan` values in `backend/render.yaml` before a
  later Blueprint sync so it does not request the free plans again.
- Review the migration output and verify the restricted runtime account can log
  in and use Identity while it cannot alter schemas or tables.

The Blueprint deploys all four service APIs. Organization currently exposes
only staff creation; Access exposes only its internal authorization lookup;
Audit exposes only its internal write endpoint. Public staff listing, account
provisioning/association, department management, role management, and audit
search are not yet implemented.
