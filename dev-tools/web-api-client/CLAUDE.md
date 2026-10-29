# web-api-client

Local-only Next.js 14 (App Router, TypeScript) console for calling the Airavata API with a CILogon token.
Separate from the Go module: the root Go hooks and Stop gate don't cover it.

- Install / run: `npm install`, then `npm run dev` (http://localhost:8082). Typecheck: `npx tsc --noEmit`. Lint: `npm run lint`.
- No tests. Done = `npx tsc --noEmit` passes and the page loads.
- `.env.local` holds the CILogon client secret and session secret. Never read, print or commit it
  (reads are denied in `.claude/settings.json`); edit `.env.local.example` for new variables.
- The OAuth flow is hand-rolled PKCE on purpose (`src/app/login`, `src/app/callback`): CILogon's registered redirect
  URI is exactly `http://localhost:8082/callback`. Don't swap in next-auth or change the port.
- Tokens stay server-side: browser requests go through `src/app/api/proxy/route.ts`, which attaches the bearer token.
  Don't expose the access token to client components.
- Next 14 audit advisories are accepted for local use (see README); don't upgrade to Next 15/16 as a drive-by.
