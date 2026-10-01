# TradePulse – personalized real-time information backend (Go)

Pipeline: `Source → Listener → RawMessage → Processor → Event → EventBus → Signals/Rules → Personalization → FeedItem → WebSocket`

Modular monolith, **standard library only** (no third-party deps). Go 1.24+.

## Run
```
export JWT_SECRET=$(openssl rand -base64 48)
go test ./... && go run ./cmd/server
```
A simulated NSE source (`SIM_ENABLED=true`) emits ticks so you can see the full flow.

## Try it
```
curl -s localhost:8080/api/v1/auth/register -d '{"email":"a@b.com","password":"a-long-password"}'   # -> token
curl -s -X PUT localhost:8080/api/v1/me/profile -H "Authorization: Bearer $TOKEN" \
  -d '{"watchlist":["RELIANCE","TCS","SBIN"],"price_move_threshold_pct":0.5}'
curl -s -X POST localhost:8080/api/v1/ws-ticket -H "Authorization: Bearer $TOKEN"   # -> ticket (30s, single use)
websocat "ws://localhost:8080/ws?ticket=$TICKET"                                      # live feed
curl -s localhost:8080/api/v1/me/feed -H "Authorization: Bearer $TOKEN"
```

## Endpoints
| Method | Path | Auth |
|---|---|---|
| POST | /api/v1/auth/register, /login | – (rate-limited) |
| GET/PUT | /api/v1/me/profile | JWT |
| GET | /api/v1/me/feed?limit= | JWT |
| POST | /api/v1/ws-ticket | JWT |
| GET | /ws?ticket= | one-time ticket |
| GET | /healthz, /readyz | – |

## Security
PBKDF2-SHA256 (600k) passwords with constant-time compare and timing-equalised login; HS256 JWT with strict header check; one-time WS tickets (no JWT in URLs); per-IP token-bucket rate limits (strict on auth); strict size-limited JSON; WS origin check, frame/size caps, ping/pong timeouts; security headers, CORS allowlist; server timeouts; panic recovery; non-root distroless image.

## Scalability / robustness
- Bounded per-subscriber queues on the bus (slow consumers never block the pipeline); slow WebSocket clients are dropped.
- Entity→users index: each event is matched only against users following that entity.
- Listener supervision with exponential backoff; graceful shutdown.

## Going to production (all behind interfaces)
1. Implement `user.Repository` / `feed.Repository` on PostgreSQL (`internal/storage/postgres`) and swap in `main.go`.
2. Move `TicketStore` and rate limiters to Redis for multi-instance deployments.
3. Implement `event.Bus` with NATS/Kafka when scale demands it (and shard personalization by user).
4. Add a real NSE/BSE `source.Listener` + `processing.Processor`.
5. Add refresh tokens/revocation, metrics (Prometheus) and tracing.

Note: this code was written without a Go toolchain in the authoring sandbox; run `go vet ./... && go test ./...` first.
