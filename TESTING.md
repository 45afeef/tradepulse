# TESTING.md — test register & strategy

Single place to see **what is tested**, **what kind**, and **what is not yet covered**. Update it in the same change as any test (Definition of Done).

**Run everything (Docker only):** `docker compose run --rm check`  ·  one package: `docker compose run --rm test go test ./internal/auth/...`

**Totals:** 92 tests (63 security-focused). **Status of all tests below: WRITTEN, NOT YET RUN** — the authoring sandbox had no Go toolchain. First action for the owner: run `check`, then record results in the *Verification log* below.

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
| 7 | `internal/access/access_test.go` | `TestEnsureAdminCreatesOnceAndNeverOverwrites` | security | Bootstrap admin created once with hashed password; existing admin never overwritten |
| 8 | `internal/access/access_test.go` | `TestEnsureAdminValidation` | security | Bootstrap rejects malformed email and short password |
| 9 | `internal/api/admin_test.go` | `TestAdminCannotSuspendSelfAndUnknownUser404` | security | Admin lock-out guard (no self-suspend) and 404 for unknown user |
| 10 | `internal/api/admin_test.go` | `TestAdminRoutesRejectMembersAndAnonymous` | security | All /admin routes: member -> 403, anonymous -> 401 (authorization) |
| 11 | `internal/api/admin_test.go` | `TestAuditTrailRecordsAdminActions` | security | Audit log records admin actions with actor id and never leaks invite codes |
| 12 | `internal/api/admin_test.go` | `TestEmailBoundInviteOnlyWorksForThatEmail` | security | Email-bound invites reject other emails; matching is case-insensitive |
| 13 | `internal/api/admin_test.go` | `TestFailedRegistrationDoesNotBurnInvite` | security | Duplicate-email failure releases the invite so it stays usable |
| 14 | `internal/api/admin_test.go` | `TestInviteIsSingleUseOverHTTP` | security | A used invite cannot register a second account (403) |
| 15 | `internal/api/admin_test.go` | `TestInviteListNeverExposesCodes` | security | Invite list leaks neither the code nor its hash |
| 16 | `internal/api/admin_test.go` | `TestRegistrationRequiresValidInvite` | security | Registration needs a valid invite_code: missing -> 4xx, unknown -> 403, valid -> 201 |
| 17 | `internal/api/admin_test.go` | `TestRevokeInvite` | security | Revoked invites cannot be redeemed; double revoke 409; unknown 404 |
| 18 | `internal/api/admin_test.go` | `TestSuspensionIsImmediateAndReversible` | security | Suspended token/login rejected at once; reactivation restores access |
| 19 | `internal/api/admin_test.go` | `TestTokenOfDeletedOrUnknownUserIsRejected` | security | A validly signed token for a non-existent account is rejected |
| 20 | `internal/api/admin_test.go` | `TestUserListNeverExposesPasswordHashes` | security | Admin user list never contains password hashes |
| 21 | `internal/api/api_test.go` | `TestAuthEndpointsAreRateLimited` | security | Login brute force hits 429 |
| 22 | `internal/api/api_test.go` | `TestCORSAllowlist` | security | Only allow-listed origins receive CORS headers |
| 23 | `internal/api/api_test.go` | `TestDocsOnlyWhenEnabled` | security | /docs and /openapi.* exist only when enabled; relaxed CSP only on /docs; 404 in production |
| 24 | `internal/api/api_test.go` | `TestEventTypesEndpoint` | unit | GET /event-types lists registry types and built-in fields |
| 25 | `internal/api/api_test.go` | `TestLoginDoesNotRevealWhetherEmailExists` | security | Unknown email and wrong password give identical 401 responses (no account enumeration) |
| 26 | `internal/api/api_test.go` | `TestOversizedBodyRejected` | security | Request bodies over the operation limit are rejected |
| 27 | `internal/api/api_test.go` | `TestPrivateRoutesRequireValidToken` | security | Every private route returns 401 for missing, garbage or tampered tokens |
| 28 | `internal/api/api_test.go` | `TestRegisterValidationAndDuplicates` | security | Bad email/password rejected; duplicate email (case-insensitive) -> 409 |
| 29 | `internal/api/api_test.go` | `TestSecurityHeaders` | security | nosniff, frame deny, no-referrer, no-store, strict CSP, HSTS (prod), request id |
| 30 | `internal/api/api_test.go` | `TestSubscriptionValidation` | security | Unknown fields/kinds/types/properties and malformed JSON are rejected with 4xx |
| 31 | `internal/api/api_test.go` | `TestSubscriptionsAreIsolatedBetweenUsers` | security | User B can never read user A's subscriptions (data isolation) |
| 32 | `internal/api/api_test.go` | `TestWebSocketTicketIsSingleUseEvenOnFailedHandshake` | security | WS ticket is single-use even if the handshake fails; missing/bad ticket -> 401 |
| 33 | `internal/api/docs_test.go` | `TestAdminOperationsDeclareSecurity` | security | Every /me and /admin operation declares a security requirement in the OpenAPI spec |
| 34 | `internal/api/docs_test.go` | `TestEveryOperationIsDocumented` | unit | Docs completeness: every operation has summary, description, declared tag; overview has required sections |
| 35 | `internal/api/middleware_test.go` | `TestBodyLimitMiddleware` | security | Global 1 MB request body cap |
| 36 | `internal/api/middleware_test.go` | `TestClientIPTrustsProxyHeaderOnlyWhenConfigured` | security | X-Forwarded-For spoofing ignored unless TRUST_PROXY; last (proxy-added) entry used |
| 37 | `internal/api/middleware_test.go` | `TestLimiterTokenBucket` | unit | Token bucket honors burst, limit and per-key isolation |
| 38 | `internal/api/middleware_test.go` | `TestPanicIsRecoveredWithoutLeakingDetails` | security | Panics return generic 500; internal detail never reaches the client |
| 39 | `internal/api/ws_e2e_test.go` | `TestWebSocketEndToEnd` | unit | TP-002: real server + raw WS client: handshake, connected, live feed_item, history, close on suspension |
| 40 | `internal/auth/auth_test.go` | `TestJWTRoundTripAndTamper` | security | JWT issue/verify; tampered and expired tokens rejected |
| 41 | `internal/auth/auth_test.go` | `TestPassword` | security | PBKDF2 verify accepts right password, rejects wrong |
| 42 | `internal/auth/auth_test.go` | `TestTicketSingleUse` | security | Ticket cannot be redeemed twice |
| 43 | `internal/auth/security_test.go` | `TestJWTRejectsAlgNoneAndForgeries` | security | alg=none, missing/extra segments, unsigned tokens rejected |
| 44 | `internal/auth/security_test.go` | `TestJWTRejectsShortSecret` | security | Secrets under 32 bytes refused |
| 45 | `internal/auth/security_test.go` | `TestJWTRejectsTamperedPayload` | security | Payload edited (privilege escalation attempt) is detected |
| 46 | `internal/auth/security_test.go` | `TestJWTRejectsWrongSecretAndIssuer` | security | Tokens signed with another secret or issuer rejected |
| 47 | `internal/auth/security_test.go` | `TestPasswordHashesAreSalted` | security | Same password hashes differently; no plaintext in hash |
| 48 | `internal/auth/security_test.go` | `TestPasswordMalformedHashesRejected` | security | Malformed/absurd-iteration stored hashes never verify |
| 49 | `internal/auth/security_test.go` | `TestTicketExpiresAndIsUnguessable` | security | Tickets expire, are long/random, unknown tickets rejected |
| 50 | `internal/config/config_test.go` | `TestBootstrapAdminMustBeCompleteAndStrong` | security | Bootstrap env must be complete and password >= 12 chars |
| 51 | `internal/config/config_test.go` | `TestDevelopmentDefaults` | unit | Dev: docs on, market demo |
| 52 | `internal/config/config_test.go` | `TestInvalidSimDemoRejected` | unit | Invalid SIM_DEMO value fails startup |
| 53 | `internal/config/config_test.go` | `TestProductionRejectsPlaceholderSecret` | security | Production refuses the placeholder JWT secret |
| 54 | `internal/config/config_test.go` | `TestProductionSafeDefaults` | security | Production: docs off, demo none, proxy trust off |
| 55 | `internal/config/config_test.go` | `TestSecretRequiredAndLong` | security | Short/missing JWT secret refuses to start |
| 56 | `internal/delivery/delivery_test.go` | `TestDeliverTargetsOnlyOwnerAndDropsSlowClients` | security | Feed items reach only their owner; slow consumers are disconnected |
| 57 | `internal/delivery/delivery_test.go` | `TestDisconnectUserClosesOnlyThatUsersConnections` | security | Suspension disconnects only the target user's sockets |
| 58 | `internal/delivery/delivery_test.go` | `TestHubConnectionCapPerUser` | security | Per-user WebSocket connection cap enforced |
| 59 | `internal/delivery/delivery_test.go` | `TestOriginCheck` | security | Cross-site WebSocket hijacking blocked by Origin allowlist/same-origin |
| 60 | `internal/delivery/delivery_test.go` | `TestReadFrameAcceptsMaskedText` | unit | Valid masked client frame parsed |
| 61 | `internal/delivery/delivery_test.go` | `TestReadFrameRejectsProtocolViolations` | security | Unmasked, fragmented, oversized, reserved-bit and long control frames rejected |
| 62 | `internal/delivery/delivery_test.go` | `TestShutdownRejectsNewConnections` | unit | Hub refuses registrations after shutdown |
| 63 | `internal/delivery/delivery_test.go` | `TestWriteFrameEncoding` | unit | Server frame length encoding (short and extended) |
| 64 | `internal/domain/event/registry_test.go` | `TestKindOf` | unit | Field whitelist lookup (built-in vs unknown) |
| 65 | `internal/domain/event/registry_test.go` | `TestRegisterValidation` | security | Invalid type ids, field collisions, bad kinds, unknown template fields, duplicates rejected |
| 66 | `internal/domain/event/registry_test.go` | `TestRenderAndValidate` | unit | Template rendering, severity, payload kind validation, unknown type rejected |
| 67 | `internal/domain/invite/invite_test.go` | `TestNewCodeIsRandomPrefixedAndHashed` | security | Invite codes unique, prefixed, high-entropy; hash differs from code |
| 68 | `internal/domain/invite/invite_test.go` | `TestStatus` | unit | Invite status: pending/used/expired/revoked precedence |
| 69 | `internal/domain/rule/rule_test.go` | `TestMatch` | unit | Condition operators across number/string/bool; missing field never matches |
| 70 | `internal/domain/rule/rule_test.go` | `TestNormalizeRejects` | security | Unknown fields, wrong operators/value types, oversized rule lists rejected |
| 71 | `internal/domain/user/user_test.go` | `TestNormalizeProfile` | unit | Profile normalization: case/space folding, keyword phrases, de-duplication |
| 72 | `internal/domain/user/user_test.go` | `TestNormalizeProfileRejects` | security | Invalid kinds, lengths, control chars, unknown types/fields, limits rejected |
| 73 | `internal/infrastructure/eventbus/memory_test.go` | `TestPublishDeliversToAllSubscribers` | unit | Bus fan-out |
| 74 | `internal/infrastructure/eventbus/memory_test.go` | `TestSlowSubscriberNeverBlocksPublisher` | security | Slow subscriber cannot stall ingestion; drops are counted (availability) |
| 75 | `internal/infrastructure/eventbus/memory_test.go` | `TestSubscriberPanicIsIsolated` | security | A panicking subscriber does not kill the bus |
| 76 | `internal/infrastructure/eventbus/memory_test.go` | `TestUnsubscribeAndClose` | unit | Unsubscribe/Close idempotent; publish after close errors |
| 77 | `internal/ingestion/ingestion_test.go` | `TestMalformedAndInvalidEventsAreDropped` | security | Ingestion drops malformed messages and registry-invalid events; stops on cancel |
| 78 | `internal/personalization/engine_test.go` | `TestEntityWithConditions` | unit | Entity subscription + condition filter; item rendered and persisted |
| 79 | `internal/personalization/engine_test.go` | `TestKeywordIsWholeWord` | unit | Keyword matches whole words only |
| 80 | `internal/personalization/engine_test.go` | `TestKeywordSubscription` | unit | Keyword subscription delivers an explained item; others get nothing |
| 81 | `internal/personalization/engine_test.go` | `TestOneItemPerUserPerEvent` | unit | Multiple matching subscriptions yield one item |
| 82 | `internal/personalization/engine_test.go` | `TestTypeRestrictionAndMute` | unit | Type restriction and mutes suppress items |
| 83 | `internal/personalization/engine_test.go` | `TestUpsertAndRemoveUpdateIndex` | unit | Index updated on profile change and removal (e.g. suspension) |
| 84 | `internal/personalization/engine_test.go` | `TestUsersAreIsolated` | security | Items only reach the subscribed user |
| 85 | `internal/storage/memory/invites_test.go` | `TestAuditRepoOrderAndCap` | unit | Audit repo newest-first and capped |
| 86 | `internal/storage/memory/invites_test.go` | `TestInviteStoredOnlyAsHash` | security | No plaintext invite code in storage |
| 87 | `internal/storage/memory/invites_test.go` | `TestRedeemExactlyOnceUnderConcurrency` | security | 100 concurrent redeems of one invite succeed exactly once (race test) |
| 88 | `internal/storage/memory/invites_test.go` | `TestRedeemRejections` | security | Expired, unknown and wrong-email redemptions rejected |
| 89 | `internal/storage/memory/invites_test.go` | `TestReleaseRestoresOnlyForSameUser` | security | Only the redeeming user can release an invite |
| 90 | `internal/storage/memory/invites_test.go` | `TestRevokeOnlyPending` | unit | Revoke semantics: only pending invites |
| 91 | `internal/storage/memory/repos_test.go` | `TestFeedRepoCapOrderAndIsolation` | security | Per-user cap, newest-first, no cross-user reads |
| 92 | `internal/storage/memory/repos_test.go` | `TestUserRepoEmailUniqueCaseInsensitive` | unit | Email uniqueness case-insensitive; not-found errors |

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
| Production-safe defaults | TestProductionSafeDefaults, TestBootstrapAdminMustBeCompleteAndStrong |
| Invite-only access (single-use, race, expiry, binding, hashing) | TestRegistrationRequiresValidInvite, TestInviteIsSingleUseOverHTTP, TestRedeemExactlyOnceUnderConcurrency, TestRedeemRejections, TestEmailBoundInviteOnlyWorksForThatEmail, TestFailedRegistrationDoesNotBurnInvite, TestInviteStoredOnlyAsHash, TestRevokeInvite |
| Authorization (roles) and suspension | TestAdminRoutesRejectMembersAndAnonymous, TestSuspensionIsImmediateAndReversible, TestDisconnectUserClosesOnlyThatUsersConnections, TestTokenOfDeletedOrUnknownUserIsRejected, TestAdminCannotSuspendSelfAndUnknownUser404 |
| Secret / hash exposure | TestInviteListNeverExposesCodes, TestUserListNeverExposesPasswordHashes, TestAuditTrailRecordsAdminActions |
| Bootstrap admin | TestEnsureAdminCreatesOnceAndNeverOverwrites, TestEnsureAdminValidation |
| Docs completeness / spec security | TestEveryOperationIsDocumented, TestAdminOperationsDeclareSecurity |

## Known gaps (tracked in BACKLOG.md)

| Gap | Planned in |
|---|---|
| Per-account login throttling / lockout | TP-009 follow-up |
| Refresh-token reuse detection | M6 |
| SSRF guard (private IPs, metadata server, redirects, DNS rebinding) | TP-010 |
| Connector fixtures (RSS, web monitor), dedupe across restarts | TP-012–TP-015 |
| X API budget cap / spend stop | TP-016 |
| Postgres integration tests, migrations up/down | TP-007 |
| Load test (many WS clients, ingest burst) | M6 |

## Verification log (append a row every time the suite is run)

| Date | Who | Command | Result | Notes |
|---|---|---|---|---|
| _pending_ | owner | `docker compose run --rm check` | not yet run | first run expected to need small fixes (code never compiled) |
