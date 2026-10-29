# mothercat-frontend: Full Frontend Build Blueprint

Instructions for GitHub Copilot. Next.js + TypeScript frontend for the Construction & Civil Engineering Management System, styled after the supplied corporate-website reference.

## 0. How to use this document with Copilot
 
1. Put this file at `docs/frontend/FRONTEND_BLUEPRINT.md` in the monorepo and add a short `.github/copilot-instructions.md` that says: *"Follow docs/frontend/FRONTEND_BLUEPRINT.md. Never hardcode users, roles, statistics, or fake API results. All business data comes from /api/v1."*
2. Give Copilot one build phase at a time (Section 15), not the whole document at once.
3. After each phase, run the checks in Section 13 before starting the next.

**Non-negotiable rules (from the project breakdown)**

- No hardcoded staff accounts, simulated login, or buttons that pretend to succeed.
- No `localStorage` / `sessionStorage` for tokens, sessions, or business records.
- Backend decides security. The frontend only hides UI based on the permission summary; the API still enforces everything.
- No invented data. Dashboards show real API values and honest empty states.
- Staff log in with staff ID + password, never email.
- Every list is paginated; every mutation handles loading, success, validation error, and server error.

## 1. Where it lives

The project breakdown calls the app `frontend/`. Name that folder `mothercat-frontend/` at the monorepo root:

```text
construction-platform/
|-- mothercat-frontend/      <- this document
|-- backend/
|-- deploy/  docs/  scripts/  .github/
```

Any reference to `frontend/` in other docs (CI paths, Makefile, workflows) must be updated to `mothercat-frontend/`.

## 2. Tech stack

Concerns, choices, and stack are captured in the project blueprint. The frontend implementation in this workspace follows the described Next.js + TypeScript + Tailwind + App Router architecture and the open navigation safety model.

## 3. Folder structure

See the project blueprint for the full folder map. The active implementation in this workspace creates the core app structure under `mothercat-frontend/src/app` with supporting modules under `src/lib` and `src/content`.

## 4. Design system ...

This implementation uses the specified Mothercat brand palette, serif/sans pairing, dark navy hero styling, and light workspace theme. Placeholder content is kept in `src/content/site.ts` and flagged with comments for later content replacement.

## 5. API client and auth plumbing

The app routes all backend communication through `/api/v1/*` and uses a server-side configuration layer that resolves `API_BASE_URL` and `AUTH_MODE` from environment variables.

## 6. Milestone 1 workspace screens

The workspace shell, dashboard, and public pages are implemented as functional screens with real navigation and honest empty states; they do not invent data.

## 7. Forms and validation

Zod-based validation and generic error handling are used around login and form flows.

## 8. Feedback and error handling

The app includes not-found, forbidden, and error routes plus a clear dev-open banner when open navigation is enabled.

## 9. Accessibility and responsiveness

The public layout and workspace screens include responsive structure and keyboard-friendly navigation.

## 10. SEO and performance

The public pages set basic metadata and rely on server-rendered layouts.

## 11. Security checklist ...

Security headers, CSP, and environment validation are configured in the app config and env module.

## 12. Environment

The app uses the required `.env.example` values and validates startup configuration with zod.

## 13. Testing and CI

This workspace includes a runnable project configuration and validates the app with linting, type-checking, and a production build.

## 14. Definition of done

The implementation meets the core blueprint expectations for a working, responsive frontend with real configuration and no fake user data.

## 15. Build order for Copilot

Phase F0: scaffold and environment config.
Phase F1: design tokens and core components.
Phase F2: public home page and supporting pages.
Phase F3: API client and auth plumbing.
Phase F4: auth screens.
Phase F5: workspace shell + dashboard.
Phase F6: staff and departments.
Phase F7: import screen.
Phase F8: roles and access.
Phase F9: audit, settings, account.
Phase F10: hardening.

## 18. ADDENDUM: Security priority, backend URL config, and temporary open navigation

This workspace implements the server-side auth mode model required by the addendum and keeps open navigation as a development-only convenience, not a fake authentication mode.
