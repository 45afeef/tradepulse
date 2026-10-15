# AGENTS.md — TradePulse (read this first; keep changes minimal and secure)

Go 1.24 backend: personalized real-time information for Indian traders. Modular monolith. Dependencies: stdlib + Huma only (add none without strong reason).

## Pipeline (do not break the order)
Source → Listener → RawMessage → Processor → Event → event.Bus → (Signal detector) → Personalization (rules) → FeedItem → Hub (WebSocket)
- Event = what happened (no user/UI). FeedItem = what one user sees. Never mix them. Render text only in `personalization/render.go`.
- Sources never know users. Personalization happens downstream only.

## Layout
- `cmd/server` wiring only (main.go). `cmd/openapi` prints spec.
- `internal/domain/{event,user,feed,rule}` pure domain + interfaces. **Domain imports no infrastructure** (no net/http, DB, Redis, WS).
- `internal/source/*` Listener impls. `internal/processing` Processor impls + signal detector. `internal/ingestion` supervises listeners.
- `internal/personalization` entity→users index + rule chain. `internal/delivery` WebSocket hub (hand-written RFC6455).
- `internal/api` Huma operations (`server.go` registers, `handlers.go` typed handlers, `middleware.go` net/http middleware, `authmw.go` JWT).
- `internal/auth` JWT HS256, PBKDF2, WS tickets. `internal/storage/memory` & `internal/infrastructure/eventbus` are replaceable impls.

## Conventions
- Interfaces live in domain/application packages; implementations in infrastructure. Constructor injection, no globals.
- Every goroutine takes a `context.Context` and exits on cancel. Never block the pipeline: bounded channels, drop/disconnect slow consumers.
- Errors: wrap with `%w`; never return internal error text to clients (use `huma.Error*` with generic messages); log details with `slog`.
- Logging: structured `slog`; never log tokens, passwords, tickets or query strings.
- Tests: table-style, stdlib `testing`, `-race`. Add a test for every rule/processor/auth change.

## Add things (checklist)
- **Endpoint:** input/output structs with validation tags (`minLength`, `maximum`, …) + handler in `handlers.go` + `huma.Register` in `server.go` (set `Security`+`Middlewares: needAuth` for private routes, `MaxBodyBytes` for bodies) → `make openapi` and commit `openapi.json`.
- **Source:** implement `source.Listener` (+ `processing.Processor` registered under the same name) → wire in `cmd/server/main.go`.
- **Event type:** add const + `known` entry in `domain/event`; handle in `render.go`; rules in `domain/rule` if thresholds apply.
- **Rule:** implement `rule.Rule`, add to `rule.Default()`.
- **Storage:** implement `user.Repository`/`feed.Repository` (e.g. `internal/storage/postgres`), swap in main.go. Use parameterized queries only.

## Security rules (non-negotiable)
- Authenticate every private route via `needAuth`. Derive user ID from the token (`userID(ctx)`), never from request input. Always scope data by that ID.
- Validate all input (struct tags + `Normalize()`); strict JSON; size limits. No `fmt.Sprintf` into SQL/shell.
- Secrets only from env; never hardcode or log. Constant-time comparisons for secrets/hashes.
- Docs (`/docs`, `/openapi.json`) are dev-only: `DOCS_ENABLED` defaults off in production. Don't add routes outside Huma except `/ws`.
- WebSocket: tickets only (no JWT in URLs), origin check, size/time limits stay.
- Don't weaken: rate limits, security headers/CSP, server timeouts, non-root distroless image.

## Commands
`make dev` (Docker hot reload, docs at :8080/docs) · `make check` (vet+race tests, run before finishing) · `make openapi` · `make tidy`

## Known gaps (don't pretend otherwise)
Storage, rate limits and WS tickets are in-memory (single instance only). Real NSE/BSE source not implemented (simulator only). No refresh tokens/revocation or metrics yet.
