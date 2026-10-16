# DECISIONS.md — decision log (append; never rewrite history, supersede instead)

| ID | Date | Decision | Rationale / consequences | Status |
|---|---|---|---|---|
| D-001 | 2026-10 | Modular monolith, Go, Huma code-first OpenAPI, stdlib-lean | Low ops complexity; docs generated from types | Decided |
| D-002 | 2026-10 | Docker-only dev workflow | Owner on Windows 11, colleagues on Unix; `make` optional | Decided |
| D-003 | 2026-10 | `/docs`, `/openapi.json` dev-only (absent in prod) | Remove free recon surface | Decided |
| D-004 | 2026-10 | Platform is domain-agnostic; domains = configuration | Owner: generic platform for anyone aggregating sources | Decided |
| D-005 | 2026-10-15 | Invite-only; codes created by admin and shared manually (Q-01) | No email infrastructure needed for beta | Decided |
| D-006 | 2026-10-15 | Host on Google Cloud Run (+ Cloud SQL, Secret Manager) (Q-03) | Beta must run **min=1, max=1, CPU always allocated** because pollers/WS push run outside requests and state is in-memory; scale-out needs Redis/Pub/Sub first | Decided |
| D-007 | 2026-10-15 | First sources: news pages and X accounts (Q-02) | Needs RSS / web-page monitor / X API connectors; exact URLs/handles still needed | Decided (details open) |
| D-008 | 2026-10-15 | Privacy-compliance work deferred for beta (Q-04) | Security baseline retained (hashing, access control, no PII in logs, manual deletion). Revisit before public/paid use or members outside the trusted circle. **Source terms of use are NOT deferred.** | Decided |
| D-009 | 2026-10 | Alerts are structured, whitelisted rules; never user code | Security + performance | Decided |
| D-010 | 2026-10 | In-memory bus/state until a measured need | Avoid premature Kafka/NATS/Redis | Decided |
| D-011 | 2026-10-15 | X data only via official/licensed API with budget caps; no scraping X | X is pay-per-use (about $0.005 per post read per published pricing; verify at docs.x.com); scraping violates terms and breaks | **Proposed** — owner to confirm |
| D-012 | 2026-10-15 | MVP alert channels: in-app + outbound webhook; email later | "Never miss" needs an off-page channel; webhook covers Slack/Discord/Telegram cheaply | **Proposed** — owner to confirm |
| D-013 | 2026-10-15 | pgx + a migration tool (goose vs golang-migrate) | Decide via ADR inside TP-007 | **Proposed** |
| D-014 | 2026-10 | Frontend only after API freeze (M7) | Avoid rework | Decided |

## Open questions
See `PROJECT_BRAIN.md` §17 (Q-02 exact sources, Q-03 GCP project/region/budgets + X budget, Q-05 extra channel, Q-07 latency, Q-08 retention, Q-09 name, Q-10 repo/CI, Q-11 beta size).
