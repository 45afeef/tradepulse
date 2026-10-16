# CHANGELOG.md — the single record of important changes

**Rules:** newest first · every change that alters behavior, API, config, data model, security, tests or process gets an entry in the same PR · say *what changed, why, which files, what breaks, how verified, what is next*.
Related trackers: status → `ROADMAP.md` · work items → `BACKLOG.md` · tests → `TESTING.md` · decisions → `DECISIONS.md`.
Verification legend: ✅ verified by running · ⚠️ written but **not yet run/compiled** · ⛔ failed.

---

## [0.2.0-dev] — 2026-10-16 — M1 Generic core (TP-004, TP-005, TP-006)
**Verification:** ⚠️ not compiled or run (no Go toolchain in the authoring sandbox). First step: `docker compose run --rm check`; paste errors for a fix pass.

### Why
The platform must be domain-agnostic (D-004). The v0 core hardcoded market concepts (price/volume event types, thresholds, watchlist, `render.go`). M1 replaces them with data-driven building blocks; markets become one *example*.

### Added
- **Event type registry** (`internal/domain/event/registry.go`): types are data — id, typed payload fields (the rule-field whitelist), `{field}`/`{field:%.2f}` title/body templates, severity. Default type `content_item`. A payload field name has one kind across all types.
- **Generic Event** (`event.go`): `Entities []string`, `Title`, `Summary`, `URL`, `Source`, `DedupeKey`, `Payload`; `Field()` resolver (built-ins: type, source, title, summary, url, entity); text helpers `NormEntity`, `Tokens`, `Phrase`.
- **Rule engine** (`internal/domain/rule/rule.go`): `field op value`, ops `> >= < <= == != in contains any_of`, validated against the registry whitelist; limits (10 conditions, 50 list items, 200 chars); missing field ⇒ no match; no expression evaluation.
- **Subscriptions** (`internal/domain/user/user.go`): kinds `entity | keyword | source`, optional `types` restriction and `conditions`; `muted` list; `Profile.Normalize(registry)`.
- **Matching engine** (`internal/personalization/engine.go`): inverted indexes (entity, source, keyword-first-token); whole-word phrase matching; one item per user per event; explainability (`reason`); `RemoveUser` for suspension.
- **Endpoints:** `GET/PUT /api/v1/me/subscriptions`, `GET /api/v1/event-types`.
- **Examples** (`examples/market`, `examples/status`): two independent domains on the unchanged core (proof of D-004). Selected with `SIM_DEMO=market|status|none`.
- **Ingestion** now validates every event against the registry before publishing.
- **Tests:** 65 total (41 security-focused) — see `TESTING.md`.
- **Docs:** `TESTING.md` (register, security matrix, gaps, verification log), this file's format.

### Changed (⚠️ breaking vs 0.1.x)
| Area | Before (0.1.x) | After (0.2.0-dev) |
|---|---|---|
| Event | `Type` enum consts, single `Entity`, market payloads | `Type` string from registry, `Entities[]`, generic fields |
| Profile | `watchlist`, `event_types`, `muted`, `volume_spike_threshold`, `price_move_threshold_pct` | `subscriptions[]` (kind/value/types/conditions), `muted` |
| Endpoint | `GET/PUT /api/v1/me/profile` | `GET/PUT /api/v1/me/subscriptions`; new `GET /api/v1/event-types` |
| FeedItem | entity, title, body, severity | + `source`, `entities[]`, `url`, `reason` |
| Rules | `Rule` chain with `UserContext`, hardcoded watchlist/threshold rules | stateless `rule.Match(conditions, event)` |
| Rendering | `personalization/render.go` (market-specific) | templates in the type registry |
| Config | `SIM_ENABLED=true|false` | `SIM_DEMO=market|status|none` (default: market in dev, none in production) |
| Wiring | `NewEngine(log, feeds, out, rules)`; `ingestion.Run(ctx, log, bus, listeners, procs)` | `NewEngine(log, feeds, out, registry)`; `ingestion.Run(ctx, log, bus, registry, listeners, procs)`; `api.Deps.Registry` added |

Migration for API clients: replace `PUT /me/profile {"watchlist":["TCS"],"volume_spike_threshold":2}` with
`PUT /me/subscriptions {"subscriptions":[{"kind":"entity","value":"TCS","types":["volume_spike"],"conditions":[{"field":"relative_volume","op":">=","value":2}]}]}`.

### Removed
`internal/processing/signals.go`, `internal/source/simulated/`, `internal/personalization/render.go`, the old `rule` package types (`Rule`, `UserContext`, `WatchlistRule`, `ThresholdRule`…), `SIM_ENABLED`. Market logic now lives only under `examples/market`.

### Security notes
Rule/subscription input is whitelisted and size-limited; control characters rejected; no expression evaluation; templates only substitute whitelisted fields; per-user isolation covered by tests (engine, feed repo, API).

### Files (main)
`internal/domain/{event,rule,user,feed}/*` · `internal/personalization/engine.go` · `internal/ingestion/ingestion.go` · `internal/processing/processor.go` · `internal/api/{server,handlers}.go` · `internal/config/config.go` · `cmd/server/main.go` · `examples/{market,status}/*` · `*_test.go` (all) · `TESTING.md`

### Open items from this change
- Unverified build (TP-001). Keyword/entity matching quality (stemming, synonyms) intentionally out of scope.
- Event `DedupeKey` exists but dedupe is not enforced yet (TP-012).

### Next
TP-001 → TP-002 → TP-007 (Postgres). See `ROADMAP.md` "Next action".

---

## [0.1.5] — 2026-10-15 — Project charter & tracking (TP-003)
Added `PROJECT_BRAIN.md` (charter, architecture, agent protocol, stop conditions), `ROADMAP.md`, `BACKLOG.md`, `DECISIONS.md`, `CHANGELOG.md`. Recorded owner decisions: Cloud Run hosting (min=1/max=1, CPU always allocated), invite codes shared manually, first sources = news pages + X accounts, privacy deferred (D-005…D-008). Removed `docs/PLAN.md` (superseded). `AGENTS.md` now points to the charter.

## [0.1.4] — 2026-10-15 — Docker-only, cross-platform dev
`compose.yaml` with one-shot tool services (`check`, `test`, `openapi`, `tidy`); `.air.toml` polling for Windows/macOS file watching; `.gitattributes` forces LF; `Makefile` reduced to optional wrappers; `Dockerfile.dev` sets `GOTOOLCHAIN=auto` (fixes `air` install failure). Fix: re-added `writeError` helper in `internal/api/middleware.go` (build error after Huma rewrite).

## [0.1.3] — 2026-10-15 — Huma API layer + dev/prod tooling
Rewrote `internal/api` on Huma (typed handlers → validation + OpenAPI); `/docs` and `/openapi.json` **only when `DOCS_ENABLED`** (default off in production; absent, not merely protected; relaxed CSP only on `/docs`); production guards (rejects placeholder `JWT_SECRET`, warns if docs enabled); `healthcheck` subcommand for Docker; distroless prod `Dockerfile`, `compose.prod.yaml`, CI workflow (vet, race tests, OpenAPI diff), `AGENTS.md`/`CLAUDE.md`, README (dev + production guide).

## [0.1.2] — 2026-10-15 — Hardening of the skeleton
WebSocket one-time tickets (no JWT in URLs); origin allowlist; frame/size caps; per-IP token-bucket rate limits (strict on auth routes); CORS allowlist; security headers; panic recovery; server timeouts.

## [0.1.1] — 2026-10-15 — Auth
Stdlib-only HS256 JWT (strict header), PBKDF2-SHA256 (600k) passwords with timing-equalised login, in-memory storage behind repository interfaces.

## [0.1.0] — 2026-10-15 — Initial skeleton
Pipeline `Source → Listener → RawMessage → Processor → Event → Bus → Rules → Personalization → FeedItem → WebSocket`; in-memory event bus with bounded per-subscriber queues; supervised listeners with backoff; simulated market source and unusual-activity signal; hand-written RFC 6455 WebSocket hub.
