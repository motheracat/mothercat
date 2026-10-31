# Backend

The backend is one Go API server with Identity, Organization, Access, and Audit as
internal domain modules. The single server connects to MongoDB using `MONGODB_URI`
and `MONGODB_DATABASE`; domain operations run in-process on that single server.
Indexes and required MongoDB setup are applied by the separate
`services/monolith/cmd/setup-mongodb` command.

## MongoDB configuration

The API and the MongoDB setup command connect only through the supplied
`MONGODB_URI` and `MONGODB_DATABASE`. Provide the MongoDB URI through the deployment
environment or secret store; there is no localhost default or local-database
fallback. Do not start or configure a local MongoDB instance for this workflow.
Run the MongoDB setup command against that same configured database before deploying
the API.

Use a TLS-enabled MongoDB replica set, managed secrets, restricted network access,
backups, and separately controlled database setup. Brevo is used for transactional
email when configured. See the [single-server MongoDB deployment
guide](docs/DEPLOYMENT-MONGODB.md) and [backend project blueprint](docs/PROJECT_BLUEPRINT.md).
Cloudinary is not wired to a media-upload flow in the current API.

## Initial administrator

The one-time `POST /api/v1/auth/bootstrap` endpoint creates the first active
SuperAdmin account. It is available only when `IDENTITY_BOOTSTRAP_TOKEN` is
configured and the MongoDB database contains no Identity accounts. Send the
high-entropy token as `Authorization: Bearer <token>` over HTTPS, with JSON fields
`staff_id`, `email`, `display_name`, and a strong `password` (at least 14 characters,
including at least three character classes). The endpoint atomically creates the
account, grants the SuperAdmin role, records an audit event, and permanently closes
bootstrap. It does not return a session; sign in through the normal login endpoint.

After a successful `201 Created`, remove `IDENTITY_BOOTSTRAP_TOKEN` from the hosting
secret store and redeploy. The database prevents repeat bootstrap even if the
environment variable is accidentally left configured. The MongoDB deployment must
support transactions, and the MongoDB setup command must have run first. Never send
the token or password over HTTP or store them in source control.
