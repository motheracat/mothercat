# CI/CD setup

The GitHub Actions workflow at
[`../../.github/workflows/ci-cd.yml`](../../.github/workflows/ci-cd.yml)
validates both applications and deploys production only from `main`.

## What runs

For pull requests targeting `development` or `main`, and pushes to either
branch, the workflow:

- Checks every Go module for formatting, tests, vet errors, build failures, and
  known Go vulnerabilities using `govulncheck`.
- Builds the same API Dockerfile used by Render.
- Installs frontend dependencies reproducibly with `npm ci`.
- Runs ESLint, TypeScript checking, a production-dependency npm audit, and a
  Next.js production build.

The two checks run independently. A production deployment is gated on both
passing. Pushes to `development` validate but do not deploy production.

On a push to `main`, the workflow triggers the Render API deployment, then
deploys the frontend to Vercel. A manual workflow run can deploy production
only when it is run against `main` and `deploy_production` is explicitly
selected.

No database migration is run by CI/CD. Run and verify controlled migrations
separately before deploying a release that requires a schema change.

## Configure GitHub

1. In GitHub, create an environment named `production`. Add required reviewers
   if production deploys must be approved.
2. Add these environment **secrets**:

   | Secret | Where to obtain it |
   |---|---|
   | `RENDER_DEPLOY_HOOK_URL` | Render API service settings → Deploy Hook. Create a hook that deploys the `main` branch. |
   | `VERCEL_TOKEN` | Vercel account settings → Tokens; use a token scoped to the project/team as narrowly as possible. |
   | `VERCEL_ORG_ID` | Vercel team/account ID for the frontend project. |
   | `VERCEL_PROJECT_ID` | Vercel project ID for `mothercat-frontend`. |

   Treat deploy hooks and tokens as credentials. Do not commit them, print
   them, or paste them into chat.
3. Configure the Vercel production environment variables for the frontend
   project, especially `API_BASE_URL` (the production backend origin),
   `APP_ENV=production`, and `AUTH_MODE=strict`. Set `NEXT_PUBLIC_SITE_URL`
   to the real HTTPS frontend URL if the application uses it.
4. Confirm Render's service is connected to this repository and configured to
   build `backend/services/monolith/Dockerfile` with `backend` as its Docker
   context. Set Render's deploy hook to `main`.
5. Disable provider-side automatic production deployment on push if you want
   GitHub Actions checks and the `production` environment approval to be the
   sole production release gate. Keep preview deployments separately if
   desired.

## Protect release branches

In repository branch protection/rulesets, protect `main` and require pull
requests plus the `Backend checks` and `Frontend checks` status checks before
merge. Restrict who can approve production deployments in the GitHub
`production` environment. Do not store production database or migration
credentials as GitHub Actions secrets: migrations are intentionally not part
of this deployment workflow.

## Release and verify

1. Open a pull request to `development` or `main`; wait for both checks to
   pass. Review dependency-audit and vulnerability findings before merge.
2. Merge the reviewed release to `main`. GitHub Actions starts the backend
   Render deployment and then the Vercel production deployment.
3. Monitor the Render and Vercel deployment dashboards until both releases are
   live. A successful GitHub hook request confirms a deployment was triggered,
   not that the provider finished building or that the app passed its runtime
   health check.
4. Verify the API readiness endpoint and smoke-test login/session, staff and
   department reads, authorized mutations, and frontend navigation. Check
   provider logs for startup, health-check, and runtime errors.
5. If either deployment fails, use the provider's previous successful release
   rollback, then investigate before retrying.
