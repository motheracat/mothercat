# Render staging smoke test

This is a disposable staging setup, not production configuration. The Blueprint
at `backend/render.yaml` creates a free Docker web service and a separate free
PostgreSQL database. Both default to Render's Oregon region; edit the Blueprint
to choose another supported region, keeping the service and database together.

## Deploy

1. Push the backend changes to the Git branch you want Render to use.
2. In Render, create a Blueprint from `backend/render.yaml` and review the
   resources before applying it. The web service is public; do not create real
   staff accounts or put production data in this environment.
3. Wait for the database and service to become available. Render supplies
   `PORT`; the service binds to that port. `/health/ready` checks PostgreSQL.
4. The free plan has no pre-deploy command, so the service does not run schema
   changes at startup. Before testing database-backed routes, apply migrations
   from your workstation as described below.

Because auto-deploy is disabled in the Blueprint, trigger later updates from the
service's **Manual Deploy → Deploy latest commit** menu.

## Apply migrations from your workstation

The database's external URL must be reachable temporarily. In the Render
Postgres Networking settings, allow only your current public IP as a `/32`; do
not open the database to all IPs. Copy the external URL from Render's dashboard,
then run these commands from `backend/services/identity`:

```sh
read -rsp 'Render external PostgreSQL URL: ' MIGRATION_DATABASE_URL
printf '\n'
export MIGRATION_DATABASE_URL
export APP_ENV=staging DATABASE_SSLMODE=require
go run ./cmd/migrate -dir migrations
unset MIGRATION_DATABASE_URL
```

Paste the URL only into the terminal prompt; do not commit it or send it in chat.
The runner records migration versions and checksums, and is safe to rerun. After
it succeeds, remove the temporary `/32` database allowlist entry. The web service
uses the internal Render connection string with TLS required (`sslmode=require`);
Render's internal certificates are self-signed, so production's
`verify-full` policy is intentionally not relaxed.

## Verify

- Check the Render deploy log for a successful image build and service start.
- Request `/health/live` and `/health/ready`; both should return success when the
  service and database are available.
- From a terminal, check the deployed service with `curl -i https://<service-name>.onrender.com/health/live` and `curl -i https://<service-name>.onrender.com/health/ready`.
- Review migration output for both numbered migrations being applied (or already
  applied on a rerun).
- Free web services can spin down after 15 idle minutes. Free Postgres has a
  1 GB limit and expires after 30 days; do not keep data you need there.

This validates a limited live health/database connection only. It does not
provide test accounts or complete end-to-end Identity workflows.