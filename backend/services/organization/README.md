# Organization service

The Organization service owns staff profiles and departments. This first API
slice implements:

- `POST /api/v1/staff` — create an approved staff profile in `pending` status.
- `GET /health/live` and `GET /health/ready`.

The official company staff directory has not yet been provided. Authorized
administrators can manually enter approved staff records; identifiers are
validated for format and uniqueness, not checked against a fabricated directory.
The profile remains pending until the later account-association and role
assignment onboarding steps are completed. This endpoint does not create an
Identity account or assign an Access role.

## Create staff

Request body:

```json
{
  "staff_id": "ENG-104",
  "full_name": "Ada Okafor",
  "work_email": "ada@example.com",
  "job_title": "Site Engineer",
  "department_id": "optional-department-uuid"
}
```

The request requires the Identity `mothercat_session` cookie, a matching
same-origin `Origin` header, and the `staff.create` permission returned by
Identity/Access. A successful response is `201 Created` with the staff profile.
The browser-facing frontend must route this API through the same public origin
as the session cookie; direct browser calls between separate Render service
domains are intentionally rejected.
Duplicate official staff IDs return `409 duplicate_staff_id`; an unknown
department returns `400 department_not_found`. The service durably records the
creation request through Audit before writing the staff record; if Audit is
unavailable, no staff row is written.

## Runtime configuration

| Variable | Required | Behavior |
| --- | --- | --- |
| `APP_ENV` | No | `development`, `test`, `staging`, or `production` |
| `PORT` | No | Render port; otherwise `HTTP_ADDR` or `:8080` |
| `DATABASE_URL` | Yes | PostgreSQL URL with explicit `sslmode` |
| `DATABASE_SSLMODE` | If URL omits it | Explicit TLS mode |
| `DATABASE_ALLOW_UNVERIFIED_TLS` | For Render production TLS | Explicit opt-in for private Render `sslmode=require` |
| `IDENTITY_SERVICE_URL` | Production/staging | HTTPS Identity base URL |
| `AUDIT_SERVICE_URL` | Production/staging | HTTPS Audit base URL |
| `AUDIT_SERVICE_TOKEN` | Production/staging | Shared with Audit; at least 32 characters |

Run the owned migrations separately before deployment:
`go run ./cmd/migrate -dir migrations`. Do not run migrations at API startup.

The Organization runtime database credential should have `USAGE` on schema
`organization`, `INSERT, SELECT` on `organization.staff_members`, and `SELECT`
on `organization.departments`. Migration credentials must remain separate.

The API validates the caller's session against Identity and checks
`staff.create` server-side; it never trusts role names or permission claims from
the request body. It logs request IDs and durations without logging submitted
staff names, email addresses, staff IDs, cookies, or service credentials.