This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

## Getting Started

First, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Deploy on Vercel

The easiest way to deploy your Next.js app is to use the [Vercel Platform](https://vercel.com/new?utm_medium=default-template&filter=next.js&utm_source=create-next-app&utm_campaign=create-next-app-readme) from the creators of Next.js.

Check out our [Next.js deployment documentation](https://nextjs.org/docs/app/building-your-application/deploying) for more details.

## Authentication API

Copy `.env.example` to `.env.local` for local work. Set `API_BASE_URL` to the
organization API origin supplied for that environment to enable API requests.
The development server can start without it for frontend-only work, but
authenticated pages and API features require it. Production and staging
deployments must configure it. Do not include `/api/v1` in the value or
hard-code a production API host.
The `/api/v1` Next.js rewrite keeps browser requests on the frontend origin,
avoiding cross-site cookie and CORS issues.

Credentialed mutation requests also require the API's Origin check to accept
the deployed frontend/API hosting arrangement. A rewrite or permissive CORS
configuration alone does not change that policy; coordinate the accepted
origin with the backend owner.

The backend owns the `mothercat_session` Secure, HttpOnly cookie. The frontend
does not read or store its value; protected server-rendered pages validate it
with `GET /api/v1/auth/session`. Use `AUTH_MODE=strict` outside local
development. Deployed frontend/API traffic must use HTTPS; do not weaken the
backend cookie policy to make plain-HTTP development work.

Password reset and activation links must include their single-use token in the
`token` query parameter. New passwords must be 14–72 bytes and contain at
least three of uppercase letters, lowercase letters, digits, and other
characters.

Account suspend/reactivate controls are intentionally not exposed yet: the
backend currently requires an `Origin` matching the API host, which a
frontend hosted on a different origin cannot satisfy. Coordinate an explicit
backend origin-policy change before adding those controls.
