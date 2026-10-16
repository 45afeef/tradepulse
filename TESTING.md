# TESTING.md — test register & strategy

Single place to see **what is tested**, **what kind**, and **what is not yet covered**. Update it in the same change as any test (Definition of Done).

**Run everything (Docker only):** `docker compose run --rm check`  ·  one package: `docker compose run --rm test go test ./internal/auth/...`

**Totals:** 65 tests (41 security-focused). **Status of all tests below: WRITTEN, NOT YET RUN** — the authoring sandbox had no Go toolchain. First action for the owner: run `check`, then record results in the *Verification log* below.

## Conventions
- stdlib `testing`, table-driven where useful, always `-race`.
- No live network. Connector tests use recorded fixtures (from M3).
- Every security rule in `AGENTS.md` needs at least one test that fails if the rule is weakened.
- Type `security` = abuse/attack/isolation/limit test; `unit` = behavior.

## Test register

| # | Package / file | Test | Type | What it proves |
|---|---|---|---|---|
| 1 | `examples/market/market_test.go` | `TestDetectorFiresOnceWithCooldown` | unit | Signal fires once when price move + volume spike coincide; cooldown suppresses repeats |
| 2 | `examples/market/market_test.go` | `TestNSETickProcess` | unit | Tick -> price_changed + volume_spike events validate against registry and render correctly |
| 3 | `examples/market/market_test.go` | `TestNSETickRejectsInvalid` | security | Malformed/zero/empty tick payloads are rejected (untrusted source input) |
| 4 | `examples/market/market_test.go` | `TestRegisterTypes` | unit | Market example types register without errors |
| 5 | `examples/status/status_test.go` | `TestStatusProcessAndRender` | unit | Non-market example works on the unchanged core (domain-agnostic proof) |
| 6 | `examples/status/status_test.go` | `TestStatusRejectsInvalid` | security | Invalid status messages (bad JSON, unknown status, negative latency) rejected |
| 7 | `internal/api/api_test.go` | `TestAuthEndpointsAreRateLimited` | security | Login brute force hits 429 |
| 8 | `internal/api/api_test.go` | `TestCORSAllowlist` | security | Only allow-listed origins receive CORS headers |
| 9 | `internal/api/api_test.go` | `TestDocsOnlyWhenEnabled` | security | /docs and /openapi.* exist only when enabled; relaxed CSP only on /docs; 404 in production |
| 10 | `internal/api/api_test.go` | `TestEventTypesEndpoint` | unit | GET /event-types lists registry types and built-in fields |
| 11 | `internal/api/api_test.go` | `TestLoginDoesNotRevealWhetherEmailExists` | security | Unknown email and wrong password give identical 401 responses (no account enumeration) |
| 12 | `internal/api/api_test.go` | `TestOversizedBodyRejected` | security | Request bodies over the operation limit are rejected |
| 13 | `internal/api/api_test.go` | `TestPrivateRoutesRequireValidToken` | security | Every private route returns 401 for missing, garbage or tampered tokens |
| 14 | `internal/api/api_test.go` | `TestRegisterValidationAndDuplicates` | security | Bad email/password rejected; duplicate email (case-insensitive) -> 409 |
| 15 | `internal/api/api_test.go` | `TestSecurityHeaders` | security | nosniff, frame deny, no-referrer, no-store, strict CSP, HSTS (prod), request id |
| 16 | `internal/api/api_test.go` | `TestSubscriptionValidation` | security | Unknown fields/kinds/types/properties and malformed JSON are rejected with 4xx |
| 17 | `internal/api/api_test.go` | `TestSubscriptionsAreIsolatedBetweenUsers` | security | User B can never read user A's subscriptions (data isolation) |
| 18 | `internal/api/api_test.go` | `TestWebSocketTicketIsSingleUseEvenOnFailedHandshake` | security | WS ticket is single-use even if the handshake fails; missing/bad ticket -> 401 |
| 19 | `internal/api/middleware_test.go` | `TestBodyLimitMiddleware` | security | Global 1 MB request body cap |
| 20 | `internal/api/middleware_test.go` | `TestClientIPTrustsProxyHeaderOnlyWhenConfigured` | security | X-Forwarded-For spoofing ignored unless TRUST_PROXY; last (proxy-added) entry used |
| 21 | `internal/api/middleware_test.go` | `TestLimiterTokenBucket` | unit | Token bucket honors burst, limit and per-key isolation |
| 22 | `internal/api/middleware_test.go` | `TestPanicIsRecoveredWithoutLeakingDetails` | security | Panics return generic 500; internal detail never reaches the client |
| 23 | `internal/auth/auth_test.go` | `TestJWTRoundTripAndTamper` | security | JWT issue/verify; tampered and expired tokens rejected |
| 24 | `internal/auth/auth_test.go` | `TestPassword` | security | PBKDF2 verify accepts right password, rejects wrong |
| 25 | `internal/auth/auth_test.go` | `TestTicketSingleUse` | security | Ticket cannot be redeemed twice |
| 26 | `internal/auth/security_test.go` | `TestJWTRejectsAlgNoneAndForgeries` | security | alg=none, missing/extra segments, unsigned tokens rejected |
| 27 | `internal/auth/security_test.go` | `TestJWTRejectsShortSecret` | security | Secrets under 32 bytes refused |
| 28 | `internal/auth/security_test.go` | `TestJWTRejectsTamperedPayload` | security | Payload edited (privilege escalation attempt) is detected |
| 29 | `internal/auth/security_test.go` | `TestJWTRejectsWrongSecretAndIssuer` | security | Tokens signed with another secret or issuer rejected |
| 30 | `internal/auth/security_test.go` | `TestPasswordHashesAreSalted` | security | Same password hashes differently; no plaintext in hash |
| 31 | `internal/auth/security_test.go` | `TestPasswordMalformedHashesRejected` | security | Malformed/absurd-iteration stored hashes never verify |
| 32 | `internal/auth/security_test.go` | `TestTicketExpiresAndIsUnguessable` | security | Tickets expire, are long/random, unknown tickets rejected |
| 33 | `internal/config/config_test.go` | `TestDevelopmentDefaults` | unit | Dev: docs on, market demo |
| 34 | `internal/config/config_test.go` | `TestInvalidSimDemoRejected` | unit | Invalid SIM_DEMO value fails startup |
| 35 | `internal/config/config_test.go` | `TestProductionRejectsPlaceholderSecret` | security | Production refuses the placeholder JWT secret |
| 36 | `internal/config/config_test.go` | `TestProductionSafeDefaults` | security | Production: docs off, demo none, proxy trust off |
| 37 | `internal/config/config_test.go` | `TestSecretRequiredAndLong` | security | Short/missing JWT secret refuses to start |
| 38 | `internal/delivery/delivery_test.go` | `TestDeliverTargetsOnlyOwnerAndDropsSlowClients` | security | Feed items reach only their owner; slow consumers are disconnected |
| 39 | `internal/delivery/delivery_test.go` | `TestHubConnectionCapPerUser` | security | Per-user WebSocket connection cap enforced |
| 40 | `internal/delivery/delivery_test.go` | `TestOriginCheck` | security | Cross-site WebSocket hijacking blocked by Origin allowlist/same-origin |
| 41 | `internal/delivery/delivery_test.go` | `TestReadFrameAcceptsMaskedText` | unit | Valid masked client frame parsed |
| 42 | `internal/delivery/delivery_test.go` | `TestReadFrameRejectsProtocolViolations` | security | Unmasked, fragmented, oversized, reserved-bit and long control frames rejected |
| 43 | `internal/delivery/delivery_test.go` | `TestShutdownRejectsNewConnections` | unit | Hub refuses registrations after shutdown |
| 44 | `internal/delivery/delivery_test.go` | `TestWriteFrameEncoding` | unit | Server frame length encoding (short and extended) |
| 45 | `internal/domain/event/registry_test.go` | `TestKindOf` | unit | Field whitelist lookup (built-in vs unknown) |
| 46 | `internal/domain/event/registry_test.go` | `TestRegisterValidation` | security | Invalid type ids, field collisions, bad kinds, unknown template fields, duplicates rejected |
| 47 | `internal/domain/event/registry_test.go` | `TestRenderAndValidate` | unit | Template rendering, severity, payload kind validation, unknown type rejected |
| 48 | `internal/domain/rule/rule_test.go` | `TestMatch` | unit | Condition operators across number/string/bool; missing field never matches |
| 49 | `internal/domain/rule/rule_test.go` | `TestNormalizeRejects` | security | Unknown fields, wrong operators/value types, oversized rule lists rejected |
| 50 | `internal/domain/user/user_test.go` | `TestNormalizeProfile` | unit | Profile normalization: case/space folding, keyword phrases, de-duplication |
| 51 | `internal/domain/user/user_test.go` | `TestNormalizeProfileRejects` | security | Invalid kinds, lengths, control chars, unknown types/fields, limits rejected |
| 52 | `internal/infrastructure/eventbus/memory_test.go` | `TestPublishDeliversToAllSubscribers` | unit | Bus fan-out |
| 53 | `internal/infrastructure/eventbus/memory_test.go` | `TestSlowSubscriberNeverBlocksPublisher` | security | Slow subscriber cannot stall ingestion; drops are counted (availability) |
| 54 | `internal/infrastructure/eventbus/memory_test.go` | `TestSubscriberPanicIsIsolated` | security | A panicking subscriber does not kill the bus |
| 55 | `internal/infrastructure/eventbus/memory_test.go` | `TestUnsubscribeAndClose` | unit | Unsubscribe/Close idempotent; publish after close errors |
| 56 | `internal/ingestion/ingestion_test.go` | `TestMalformedAndInvalidEventsAreDropped` | security | Ingestion drops malformed messages and registry-invalid events; stops on cancel |
| 57 | `internal/personalization/engine_test.go` | `TestEntityWithConditions` | unit | Entity subscription + condition filter; item rendered and persisted |
| 58 | `internal/personalization/engine_test.go` | `TestKeywordIsWholeWord` | unit | Keyword matches whole words only |
| 59 | `internal/personalization/engine_test.go` | `TestKeywordSubscription` | unit | Keyword subscription delivers an explained item; others get nothing |
| 60 | `internal/personalization/engine_test.go` | `TestOneItemPerUserPerEvent` | unit | Multiple matching subscriptions yield one item |
| 61 | `internal/personalization/engine_test.go` | `TestTypeRestrictionAndMute` | unit | Type restriction and mutes suppress items |
| 62 | `internal/personalization/engine_test.go` | `TestUpsertAndRemoveUpdateIndex` | unit | Index updated on profile change and removal (e.g. suspension) |
| 63 | `internal/personalization/engine_test.go` | `TestUsersAreIsolated` | security | Items only reach the subscribed user |
| 64 | `internal/storage/memory/repos_test.go` | `TestFeedRepoCapOrderAndIsolation` | security | Per-user cap, newest-first, no cross-user reads |
| 65 | `internal/storage/memory/repos_test.go` | `TestUserRepoEmailUniqueCaseInsensitive` | unit | Email uniqueness case-insensitive; not-found errors |

## Security coverage matrix

| Threat / control | Tests |
|---|---|
| Forged / tampered / expired JWT, alg=none | TestJWTRoundTripAndTamper, TestJWTRejectsAlgNoneAndForgeries, TestJWTRejectsTamperedPayload, TestJWTRejectsWrongSecretAndIssuer, TestPrivateRoutesRequireValidToken |
| Weak / placeholder secrets | TestJWTRejectsShortSecret, TestSecretRequiredAndLong, TestProductionRejectsPlaceholderSecret |
| Password storage | TestPassword, TestPasswordHashesAreSalted, TestPasswordMalformedHashesRejected |
| Account enumeration / brute force | TestLoginDoesNotRevealWhetherEmailExists, TestAuthEndpointsAreRateLimited, TestLimiterTokenBucket |
| Cross-user data access (isolation) | TestSubscriptionsAreIsolatedBetweenUsers, TestUsersAreIsolated, TestFeedRepoCapOrderAndIsolation, TestDeliverTargetsOnlyOwnerAndDropsSlowClients |
| WebSocket abuse (hijacking, frames, tickets, caps) | TestOriginCheck, TestReadFrameRejectsProtocolViolations, TestWebSocketTicketIsSingleUseEvenOnFailedHandshake, TestTicketSingleUse, TestTicketExpiresAndIsUnguessable, TestHubConnectionCapPerUser |
| Input validation / injection surface | TestSubscriptionValidation, TestNormalizeProfileRejects, TestNormalizeRejects, TestRegisterValidation, TestRegisterValidationAndDuplicates, TestOversizedBodyRejected, TestBodyLimitMiddleware |
| Untrusted source data | TestNSETickRejectsInvalid, TestStatusRejectsInvalid, TestMalformedAndInvalidEventsAreDropped |
| Information leakage | TestPanicIsRecoveredWithoutLeakingDetails, TestDocsOnlyWhenEnabled, TestSecurityHeaders |
| Proxy header spoofing | TestClientIPTrustsProxyHeaderOnlyWhenConfigured |
| Cross-origin | TestCORSAllowlist, TestOriginCheck |
| Availability (slow consumers, panics) | TestSlowSubscriberNeverBlocksPublisher, TestSubscriberPanicIsIsolated |
| Production-safe defaults | TestProductionSafeDefaults |

## Known gaps (tracked in BACKLOG.md)

| Gap | Planned in |
|---|---|
| WebSocket end-to-end (real upgrade, receive a feed item) | TP-002 |
| Invite redemption race / single-use / expiry | TP-008 |
| Role checks (admin 403 for members), suspension cuts sessions | TP-008, TP-009 |
| SSRF guard (private IPs, metadata server, redirects, DNS rebinding) | TP-010 |
| Connector fixtures (RSS, web monitor), dedupe across restarts | TP-012–TP-015 |
| X API budget cap / spend stop | TP-016 |
| Postgres integration tests, migrations up/down | TP-007 |
| Load test (many WS clients, ingest burst) | M6 |

## Verification log (append a row every time the suite is run)

| Date | Who | Command | Result | Notes |
|---|---|---|---|---|
| _pending_ | owner | `docker compose run --rm check` | not yet run | first run expected to need small fixes (code never compiled) |
