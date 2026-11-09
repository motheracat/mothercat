# Frontend API Guide

This guide covers the production API endpoints currently implemented for task
management and in-app notifications. It is an integration reference for the
frontend; it does not describe database or migration credentials.

## Base URL and browser integration

Production API base URL:

```text
https://mothercat-api.onrender.com
```

All paths below are relative to that URL. Use HTTPS and send browser requests
with `credentials: "include"` so the browser sends the authenticated session
cookie:

```js
const response = await fetch(
  "https://mothercat-api.onrender.com/api/v1/me/tasks",
  { credentials: "include" },
);
```

The API uses the `mothercat_session` cookie; frontend code should not read,
store, or construct session tokens. Authentication must be performed through
the backend's existing identity endpoints.

### Important: origin requirement for writes

Task and notification write endpoints require an `Origin` header whose scheme
and host match the API request's scheme and host. A browser frontend served from
a different host (for example, a separate frontend domain calling
`mothercat-api.onrender.com` directly) will not satisfy this check, so its
`POST`, `PATCH`, and `PUT` requests will be rejected with
`403 origin_not_allowed`.

For browser-based writes, serve the frontend and API through one origin (for
example, proxy `/api` through the frontend host to this API). Alternatively,
the backend must be intentionally updated with an explicit allowed-origin,
CORS, cookie, and CSRF policy before direct cross-origin integration. Do not
try to work around this by setting a forged `Origin` header in frontend code;
browsers control that header.

The current API server does not add CORS response headers. Therefore a
different-origin browser frontend cannot call it directly, even for reads,
unless CORS is deliberately configured. The session cookie is `HttpOnly`,
`Secure`, and `SameSite=Lax`; use a same-origin frontend proxy rather than
trying to access the cookie from JavaScript.

## Common request and response behavior

- Send JSON requests with `Content-Type: application/json`.
- JSON request bodies reject unknown fields.
- Successful JSON responses use `application/json`.
- Errors generally have this shape:

  ```json
  {
    "error": {
      "code": "invalid_request",
      "request_id": "..."
    }
  }
  ```

- Capture the `X-Request-ID` response header when reporting an API failure.
- `401 unauthorized` means the session is missing, invalid, or expired. Direct
  the user through the normal sign-in flow.
- `403 permission_denied` means the account is authenticated but lacks
  permission or access to that resource. `403 origin_not_allowed` indicates
  the browser-origin restriction above.
- `400` indicates an invalid body, path ID, or query parameter. `404 not_found`
  can mean the resource does not exist or is not visible to the current user.
- `409 task_conflict` means a task workflow transition is not valid for its
  current state. `412 version_conflict` means a task changed since it was read;
  fetch it again before retrying an edit.
- `503 service_unavailable` indicates a temporary backend or database issue.

## Sign-in and session

These identity endpoints use the same production base URL. Send requests with
`credentials: "include"` so the browser accepts and sends the session cookie.

| Method and path | Request | Success |
|---|---|---|
| `POST /api/v1/auth/login` | `{"staff_id":"...","password":"..."}` | `200`, `{"user": ...}`; sets the session cookie |
| `GET /api/v1/auth/session` | No body | `200`, `{"user": ...}` |
| `POST /api/v1/auth/logout` | No body | `200`, `{"status":"logged_out"}`; clears the cookie |
| `POST /api/v1/auth/password/forgot` | `{"staff_id":"..."}` | `202`, generic `{"status":"accepted","message":"..."}` |
| `POST /api/v1/auth/password/reset` | `{"token":"...","password":"...","confirm_password":"..."}` | `200`, `{"status":"password_reset"}` |
| `POST /api/v1/auth/password/change` | `{"current_password":"...","password":"...","confirm_password":"..."}` | `200`; clears the current session cookie |
| `POST /api/v1/auth/activation/complete` | `{"token":"...","password":"...","confirm_password":"..."}` | `200`, `{"status":"activated"}` |

Use the staff identifier (not email) for login and password recovery. A
successful login returns the account in `user`; use that response or the
session endpoint to populate the current-user state. Do not persist passwords,
session identifiers, or recovery/activation tokens in local storage. Login and
recovery endpoints are rate limited; the recovery response is deliberately
generic to avoid disclosing whether an account exists.

## Tasks

All task IDs, account IDs, and department IDs are UUIDs. Task timestamps are
RFC 3339 values. Priorities are `low`, `normal`, `high`, or `urgent`. Statuses
are `draft`, `assigned`, `in_progress`, `submitted`, `changes_requested`,
`approved`, `rejected`, or `cancelled`.

### Task object

Task responses use this shape:

```json
{
  "id": "uuid",
  "title": "Prepare site",
  "description": "",
  "priority": "normal",
  "status": "draft",
  "created_by": "account-uuid",
  "department_id": "department-uuid",
  "assignee_account_id": "account-uuid",
  "due_at": "2026-11-08T12:00:00Z",
  "version": 1,
  "created_at": "2026-11-07T12:00:00Z",
  "updated_at": "2026-11-07T12:00:00Z",
  "latest_submission": {
    "id": "submission-uuid",
    "submission_number": 1,
    "summary": "Completed the work",
    "links": ["https://example.com/evidence"],
    "submitter_account_id": "account-uuid",
    "submitted_at": "2026-11-07T12:00:00Z"
  }
}
```

`assignee_account_id`, `due_at`, and `latest_submission` may be omitted when
there is no value. A single-task response is wrapped as `{"task": ...}`.

### List and read tasks

| Method and path | Description |
|---|---|
| `GET /api/v1/tasks` | List tasks visible to the user |
| `GET /api/v1/me/tasks` | List tasks created by or assigned to the current user |
| `GET /api/v1/tasks/{taskId}` | Read one visible task |

Supported list query parameters:

- `status`: one of the task statuses above
- `assignee`: assignee account UUID
- `department`: department UUID
- `due_from`, `due_to`: RFC 3339 timestamps
- `limit`: 1–100; defaults to 50
- `offset`: 0–1,000,000; defaults to 0

The list response is:

```json
{
  "tasks": [],
  "page": { "limit": 50, "offset": 0, "total": 0 }
}
```

Unknown or repeated query parameters are rejected. The API only returns tasks
the current account is permitted to see.

### Create a task

`POST /api/v1/tasks` requires `tasks.create`. The current user must belong to
the selected department.

```json
{
  "title": "Prepare site",
  "description": "Complete the pre-work inspection.",
  "priority": "normal",
  "department_id": "department-uuid",
  "due_at": "2026-11-08T12:00:00Z"
}
```

`title` is required (1–200 characters), `description` is at most 20,000
characters, and `department_id` is required. `priority` defaults to `normal`;
`due_at` is optional and may be `null`. Success is `201 Created`, wrapped as
`{"task": ...}`, with an `ETag` response header.

### Edit a task

`PATCH /api/v1/tasks/{taskId}` edits a draft task as its creator. Include the
exact `ETag` value returned by the latest task read/create in `If-Match`:

```http
If-Match: "1"
Content-Type: application/json
```

Send at least one supported field:

```json
{
  "title": "Prepare site",
  "description": "Updated inspection instructions.",
  "priority": "high",
  "due_at": null
}
```

Supported fields are `title`, `description`, `priority`, and `due_at`. To clear
the due date, send `"due_at": null`; omitting `due_at` leaves it unchanged.
Every successful update returns the updated task and a new `ETag`. Missing
`If-Match` returns `428 if_match_required`; a stale version returns
`412 version_conflict`. Fetch the task again and apply the user's changes to
the new version instead of retrying with the stale ETag.

### Workflow actions

| Method and path | Body | Permission / behavior |
|---|---|---|
| `POST /api/v1/tasks/{taskId}/assign` | `{"assignee_account_id":"uuid"}` | Requires `tasks.assign`; assignee must be active and in the task department |
| `POST /api/v1/tasks/{taskId}/start` | None | Assigned user starts work |
| `POST /api/v1/tasks/{taskId}/submit` | `{"summary":"...","links":["https://..."]}` | Assigned user submits work; summary is required, at most 20,000 characters, up to 10 HTTPS links |
| `POST /api/v1/tasks/{taskId}/approve` | `{"feedback":"..."}` | Requires `tasks.review`; feedback optional, max 2,000 characters |
| `POST /api/v1/tasks/{taskId}/request-changes` | `{"feedback":"..."}` | Requires `tasks.review`; feedback required, max 2,000 characters |
| `POST /api/v1/tasks/{taskId}/reject` | `{"feedback":"..."}` | Requires `tasks.review`; feedback required, max 2,000 characters |
| `POST /api/v1/tasks/{taskId}/cancel` | None | Requires `tasks.cancel` |

Workflow action success responses are `200 OK`, wrapped as `{"task": ...}`, and
include the updated `ETag`. An invalid state transition returns `409
task_conflict`. The backend enforces actor, permission, and state rules; the
frontend should use these errors as authoritative if local state is stale.

### Task history and comments

| Method and path | Description |
|---|---|
| `GET /api/v1/tasks/{taskId}/history?limit=50&offset=0` | Paginated status history |
| `GET /api/v1/tasks/{taskId}/comments?limit=50&offset=0` | Paginated comments |
| `POST /api/v1/tasks/{taskId}/comments` | Add a comment |

History and comment list responses contain their collection plus a `page`
object with `limit`, `offset`, and `total`. A comment request is:

```json
{ "body": "The inspection is complete." }
```

Comment body is required and limited to 5,000 characters. Successful comment
creation returns `201 Created` with `{"comment": ...}`.

## Notifications

Notifications belong to the authenticated account. Supported event types are
`task.assigned`, `task.submitted`, `task.approved`, `task.changes_requested`,
`task.rejected`, `task.comment_added`, `task.due_soon`, and `task.overdue`.
Reminder notifications are generated by the backend.

| Method and path | Description |
|---|---|
| `GET /api/v1/notifications?unread=true&limit=50&offset=0` | List notifications; `unread` is optional and defaults to `false` |
| `GET /api/v1/notifications/unread-count` | Get unread count |
| `POST /api/v1/notifications/{notificationId}/read` | Mark one notification read; returns `204 No Content` |
| `POST /api/v1/notifications/read-all` | Mark all current user's unread notifications read |
| `GET /api/v1/notifications/preferences` | Get event preferences |
| `PUT /api/v1/notifications/preferences` | Replace event preferences |

Notification list response:

```json
{
  "notifications": [
    {
      "id": "notification-uuid",
      "event_type": "task.assigned",
      "title": "Task assigned",
      "body": "A task was assigned to you.",
      "resource": { "type": "task", "id": "task-uuid" },
      "created_at": "2026-11-07T12:00:00Z"
    }
  ],
  "page": { "limit": 50, "offset": 0, "total": 1 }
}
```

`resource` and `read_at` are omitted when not set. Unread count is
`{"unread_count": 3}`. Mark-all-read returns `{"marked_read": 3}`.

Preferences response:

```json
{
  "preferences": [
    { "event_type": "task.assigned", "enabled": true },
    { "event_type": "task.overdue", "enabled": false }
  ]
}
```

Submit the full replacement list to update preferences:

```json
{
  "preferences": [
    { "event_type": "task.assigned", "enabled": true },
    { "event_type": "task.submitted", "enabled": true },
    { "event_type": "task.approved", "enabled": true },
    { "event_type": "task.changes_requested", "enabled": true },
    { "event_type": "task.rejected", "enabled": true },
    { "event_type": "task.comment_added", "enabled": true },
    { "event_type": "task.due_soon", "enabled": false },
    { "event_type": "task.overdue", "enabled": true }
  ]
}
```

Omitted event preferences revert to the backend default (`enabled: true`).
Notification writes require the same-origin condition described above.

## Opportunities and SIWES applications

Migration `011_opportunities` adds job/SIWES opportunity management and private
SIWES application records. All paths below use the same production base URL.
The frontend should call these through its same-origin `/api` proxy; direct
cross-origin browser calls are not supported by the current API.

### Public opportunities

| Method and path | Description |
|---|---|
| `GET /api/v1/opportunities?type=siwes&limit=20&offset=0` | Public list of published, not-expired opportunities |
| `GET /api/v1/opportunities/{opportunityId}` | Public detail for an open opportunity; authorized managers can also read drafts/closed records |

`type` can be `job` or `siwes`. Public lists do not return drafts or closed
opportunities. The API does not return expired opportunities. An opportunity
contains the fields `id`, `type`, `title`, `location`, `work_mode`, `summary`,
`description`, `requirements`, `application_deadline`, `status`, `created_at`,
and `updated_at`. The deadline is a `YYYY-MM-DD` date and can be absent while
the opportunity is a draft. Lists return `opportunities` and a `page` object.

### Manage opportunities

The management list requires at least one of `opportunities.publish`,
`applications.review`, or `it_applications.review`; each account only sees
opportunity types allowed by its permission. Creation, editing, and status
changes require `opportunities.publish`.

| Method and path | Description |
|---|---|
| `GET /api/v1/opportunities/manage?type=siwes&status=draft&limit=25&offset=0` | Authorized management list |
| `POST /api/v1/opportunities` | Create a draft |
| `PATCH /api/v1/opportunities/{opportunityId}` | Edit a draft |
| `POST /api/v1/opportunities/{opportunityId}/publish` | Publish a draft |
| `POST /api/v1/opportunities/{opportunityId}/unpublish` | Return a published opportunity to draft |
| `POST /api/v1/opportunities/{opportunityId}/close` | Close a published opportunity |

Creation example:

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

Limits are title 160, location 120, work mode 80, summary 400, description
8,000, and requirements 4,000 characters. Publication requires all public
content fields and a deadline that has not passed. Successful mutations return
`{"opportunity": ...}`.

### Submit and review SIWES applications

`POST /api/v1/opportunities/{opportunityId}/siwes-applications` is public and
accepts `multipart/form-data`. Send text fields `full_name`, `email`, `phone`,
`institution`, `course`, and `matric_number`; `message` is optional. Include
exactly one file field named `document`. The backend accepts PDF, legacy DOC,
or DOCX up to 10 MiB and checks file signatures/container structure as well as
the filename extension. The browser's claimed MIME type is not trusted.

The application is accepted only for an open SIWES opportunity. The document
is stored privately in PostgreSQL and is never included in a public opportunity
response. All application metadata and document bytes are deleted after 365
days. Requests are origin checked and limited to five per IP per hour per API
process; production deployments should additionally configure edge-level abuse
controls because process-local limits do not coordinate across replicas.

**Malware scanning is fail-closed.** Configure `SIWES_CLAMAV_ADDR` as a
private `host:port` for a reachable ClamAV daemon. Without a configured and
healthy scanner, uploads return `503 document_scanning_unavailable`; infected
uploads return `422 unsafe_document`. Do not expose the scanner publicly.
Uploads are not enabled until the frontend proxy has a bounded request-size
exception for this exact route (10 MiB file plus at most 64 KiB multipart
overhead).

Review endpoints require `it_applications.review`:

| Method and path | Description |
|---|---|
| `GET /api/v1/siwes-applications?opportunity_id={uuid}&status=submitted&limit=25&offset=0` | Paginated reviewer list with private applicant metadata and document metadata |
| `GET /api/v1/siwes-applications/{applicationId}` | Read one application |
| `PATCH /api/v1/siwes-applications/{applicationId}` | Update review status and/or reviewer note |
| `GET /api/v1/siwes-applications/{applicationId}/document` | Download the file after authorization |

Allowed review statuses are `submitted`, `under_review`, `shortlisted`,
`accepted`, and `rejected`. Transitions are enforced: submitted applications
can move to under review or rejected; under review can move to shortlisted,
accepted, or rejected; shortlisted can move to accepted or rejected. A
reviewer may update `reviewer_note` (up to 2,000 characters) without changing
status. Applicant email/status notifications and applicant withdrawal are not
implemented in this contract. Document download is attachment-only, uses
`Cache-Control: no-store`, and never returns a permanent public URL.

## Endpoints outside this guide

The API also exposes staff, department, role, permission, and audit routes.
This guide does not specify their complete request/response contracts. Use
their existing backend/API documentation (including
[`FRONTEND_STAFF_PROFILE_EDITING.md`](./FRONTEND_STAFF_PROFILE_EDITING.md))
or add a verified contract before implementing those frontend screens.
