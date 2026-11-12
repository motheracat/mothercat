# Opportunities and SIWES: Frontend API Requirements

## Status and scope

This document records the API contract used by the opportunities and SIWES
frontend. The backend handlers and database migration are implemented in the
Go monolith; deployment-specific security settings and staging acceptance
remain operational requirements.

The current frontend:

- Shows published job and SIWES opportunities on the public `/opportunities`
  page.
- Lets authorized workspace users create draft opportunities and publish or
  unpublish them at `/opportunities/manage`.
- Lets a student submit an SIWES application with contact, institution, course,
  matriculation, message, and supporting-document fields.
- Shows submitted applications to authorized reviewers in the workspace.
- Loads opportunities and applications from the backend API. It does not seed,
  read, or write opportunity or applicant records in browser storage.
- Refreshes public listings every 60 seconds and workspace lists every 30
  seconds while the page is visible, and refreshes again when the page regains
  focus. This is near-real-time polling; the API does not currently push events
  to connected browsers.

Student submission is SIWES-only. Applicants may provide a CV or SIWES
introduction letter in PDF, DOC, or DOCX format, up to 10 MiB. Job candidates
continue through the Contact route. Application status emails are not currently
sent. No applicant personal information or document is written to browser
storage.

## Existing frontend routes and permissions

| Purpose | Frontend route | Permission |
|---|---|---|
| Browse published job and SIWES opportunities | `/opportunities` (public) | None |
| Manage opportunities and review applications | `/opportunities/manage` (workspace) | `opportunities.publish`, `applications.review`, or `it_applications.review` |
| Create, edit, publish, and unpublish opportunities | `/opportunities/manage` | `opportunities.publish` |
| Review job applications | `/opportunities/manage` | `applications.review` |
| Review IT/SIWES applications | `/opportunities/manage` | `it_applications.review` |

Workspace authorization is enforced by the backend; hiding controls in the
frontend is not an authorization boundary. The permission keys are declared in
`src/lib/auth/permissions.ts`.

## API conventions

- Use the existing same-origin frontend proxy at `/api/v1/...`; do not call the
  API host directly from browser code.
- The proxy forwards the `mothercat_session` cookie and enforces same-origin
  checks for writes. Requests must use the existing fetch helpers with browser
  credentials included.
- JSON requests use `Content-Type: application/json`. Do not manually set
  `Content-Type` for multipart requests; the browser supplies the boundary.
- Use UUIDs for opportunity and application IDs.
- List responses should include `{ page: { limit, offset, total } }` and accept
  `limit` (1–100) and `offset` (0 or greater).
- Return consistent JSON errors such as
  `{ "error": { "code": "invalid_request", "message": "...", "request_id": "..." } }`.
  The frontend should display safe error messages and retain the request ID for
  support.
- All mutation endpoints validate authorization, request origin, input,
  document type, and size on the server.

## Opportunity object

Public/admin representation:

```json
{
  "id": "uuid",
  "type": "job",
  "title": "Graduate Civil Engineer",
  "location": "Lagos, Nigeria",
  "work_mode": "Full-time",
  "summary": "Short public summary.",
  "description": "Full role or placement description.",
  "requirements": "Eligibility and requirements.",
  "application_deadline": "2027-01-31",
  "status": "published",
  "created_at": "2026-11-01T09:00:00Z",
  "updated_at": "2026-11-01T09:00:00Z"
}
```

Allowed values:

- `type`: `job`, `siwes`
- `status`: `draft`, `published`, `closed`

The frontend and API use the canonical snake_case fields shown above. Published
public results exclude drafts, closed, and expired opportunities.

## Opportunity endpoints

### Public list

`GET /api/v1/opportunities?status=published&type=job&limit=20&offset=0`

- Public; no session required.
- `type` is optional and may be `job` or `siwes`.
- Return only published opportunities that are open for applications.
- Response:

```json
{
  "opportunities": [],
  "page": { "limit": 20, "offset": 0, "total": 0 }
}
```

The frontend may request the two types separately or fetch all published
opportunities and split them into the “Open roles” and “IT / SIWES placements”
sections.

### Admin list and detail

- `GET /api/v1/opportunities/manage?type=job&status=draft&limit=25&offset=0`
  requires `opportunities.publish`, `applications.review`, or
  `it_applications.review`, and returns records visible to that permission.
- `GET /api/v1/opportunities/{opportunityId}` returns one record after
  authorization. Public reads should only allow published/open records; admin
  reads may include drafts.

### Create opportunity

`POST /api/v1/opportunities` requires `opportunities.publish`.

```json
{
  "type": "siwes",
  "title": "SIWES Industrial Training Placement",
  "location": "Lagos, Nigeria",
  "work_mode": "On-site",
  "summary": "A supervised industrial training placement.",
  "description": "Full placement details.",
  "requirements": "Eligibility requirements.",
  "application_deadline": "2027-02-28"
}
```

- The frontend field constraints are: title 160 characters, location 120,
  work mode 80, summary 400,
  description 8,000, requirements 4,000. Deadline is a date (`YYYY-MM-DD`).
- Creation should default to `draft`; the frontend currently creates drafts
  before publication.
- Success: `201 Created`, `{ "opportunity": { ... } }`.

### Edit, publish, unpublish, close

The frontend and backend support editing and the following status actions:

- `PATCH /api/v1/opportunities/{opportunityId}` requires
  `opportunities.publish`; accept one or more editable opportunity fields.
- `POST /api/v1/opportunities/{opportunityId}/publish` requires
  `opportunities.publish`.
- `POST /api/v1/opportunities/{opportunityId}/unpublish` requires
  `opportunities.publish`; returns it to `draft`.
- `POST /api/v1/opportunities/{opportunityId}/close` moves a
  published opportunity to `closed`.
- Return `200 OK`, `{ "opportunity": { ...updated record... } }`.
- Server must reject publication when required fields are missing or the
  deadline has passed.

## SIWES application endpoints

### Submit an application

`POST /api/v1/opportunities/{opportunityId}/siwes-applications`

- Public/student flow; no staff session is required.
- Accept `multipart/form-data` with these text fields and one file field named
  `document`:
  - `full_name` (required; current UI max 160)
  - `email` (required; current UI max 254)
  - `phone` (required; current UI max 40)
  - `institution` (required; current UI max 180)
  - `course` (required; current UI max 160)
  - `matric_number` (required; current UI max 80)
  - `message` (optional; current UI max 2,000)
  - `document` (required; exactly one file)
- The UI accepts PDF, DOC, or DOCX up to 10 MiB. The backend verifies actual
  file content and size; it does not rely only on browser MIME type or
  filename.
- The UI allows either a CV or SIWES introduction letter as the supporting
  document.
- Reject applications for unpublished/closed/expired placements.
- The backend validates file contents and size, stores documents privately in
  PostgreSQL, and applies 365-day retention. Malware scanning is not performed.
- Success: `201 Created`, `{ "application": { ...metadata... } }`. Never return
  the uploaded file contents in JSON.

Example response:

```json
{
  "application": {
    "id": "uuid",
    "opportunity_id": "uuid",
    "type": "siwes",
    "full_name": "Student Example",
    "email": "student@example.edu",
    "phone": "+2348000000000",
    "institution": "Example University",
    "course": "Civil Engineering",
    "matric_number": "ENG/2024/001",
    "message": "",
    "status": "submitted",
    "document": {
      "file_name": "siwes-letter.pdf",
      "content_type": "application/pdf",
      "size_bytes": 123456,
      "download_available": true
    },
    "submitted_at": "2026-11-09T21:00:00Z"
  }
}
```

Do not expose private student details or document URLs in the public
opportunity-list response.

### Review SIWES applications

- `GET /api/v1/siwes-applications?opportunity_id={uuid}&status=submitted&limit=25&offset=0`
  requires `it_applications.review`.
- `GET /api/v1/siwes-applications/{applicationId}` requires
  `it_applications.review` and returns the full private application metadata.
- `PATCH /api/v1/siwes-applications/{applicationId}` requires
  `it_applications.review`; accept a review status and optional reviewer note.
- Statuses: `submitted`, `under_review`, `shortlisted`, `accepted`, and
  `rejected`, with transitions enforced by the backend. Applicant status emails
  are not currently sent.
- `GET /api/v1/siwes-applications/{applicationId}/document` requires
  `it_applications.review` and returns the document as a protected download.
- Reviewer list responses should include `document` metadata and pagination,
  but must not expose a permanent public document URL.

The reviewer UI shows application detail, permits allowed status changes and
reviewer notes, and downloads documents through the permission-protected API.

## Job application decision

The public job card continues to direct candidates to `/contact`. The
`applications.review` permission remains available for a future, separately
specified job-application workflow; job submissions are not routed through the
SIWES API.

## Frontend proxy and upload requirements

The proxy at `src/app/api/v1/[...path]/route.ts` forwards supported methods
and validates same-origin writes. JSON requests retain the 16 KiB default
limit. The exact SIWES application route has a bounded 10 MiB plus 64 KiB
multipart limit and a longer upstream timeout. Multipart requests use the
browser-generated boundary; the proxy forwards only the staff session cookie
when present.

## Implementation and release checklist

- [x] Backend public opportunity list, management list, detail, create, update,
  publish, unpublish, and close endpoints.
- [x] Backend SIWES submission, paginated review list, detail, status/note
  update, and permission-protected document download.
- [x] Frontend API client and removal of browser-local demo store and seed data.
- [x] Public SIWES multipart submission and protected workspace review UI.
- [x] Exact-route bounded upload allowance and proxy regression coverage.
- [x] Loading/error states, cancellation, pagination, and periodic visible-page
  refreshes.
- [ ] Run frontend lint, typecheck, tests, and production build in CI.
- [ ] Run backend tests/build and test public, permission-denial, scanner,
  review, and document-access paths against isolated staging.
- [ ] Configure `SIWES_CLAMAV_ADDR` and verify scan-failure behavior before
  accepting real student uploads.
- [ ] Verify database backups and the 365-day retention job in staging.

The `[x]` items describe implemented source code; they do not imply that the
live deployment or production database has been independently verified.
