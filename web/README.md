# TradePulse web app

React + TypeScript + Vite. Mobile-first, light/dark, accessible. The full guide is [`docs/FRONTEND.md`](../docs/FRONTEND.md).

```bash
# from the repository root (Docker only; no local Node needed)
docker compose up --build            # api + postgres + web
# open http://localhost:5173   (dev operator: admin@example.com / dev-admin-password-change-me)

docker compose run --rm web-check    # typecheck + unit tests + production build
```
Without Docker: `cd web && npm install && npm run dev` (needs the API on `http://localhost:8080`, or set `API_URL`).

| Script | What |
|---|---|
| `npm run dev` | dev server with hot reload; `/api` and `/ws` are proxied to the API (no CORS setup) |
| `npm run typecheck` | strict TypeScript |
| `npm test` | Vitest (logic and component tests) |
| `npm run build` | typecheck + production bundle in `dist/` |
| `npm run contract` | every API call in `src/api/endpoints.ts` exists in `../openapi.json` (or the live `API_URL`) |

Commit `package-lock.json` after the first install.
