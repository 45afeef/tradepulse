# TESTING.md — test register & strategy

Single place to see **what is tested**, **what kind**, and **what is not yet covered**. Update it in the same change as any test (Definition of Done).

**Run everything (Docker only):** `docker compose run --rm check`  ·  one package: `docker compose run --rm test go test ./internal/auth/...`

**Totals:** 126 tests (86 security-focused). Postgres integration tests need `TEST_DATABASE_URL` (set automatically by `docker compose run --rm check` and CI; skipped otherwise).

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
| 39 | `internal/api/ready_test.go` | `TestReadyzReflectsDependencyHealth` | security | /readyz 503 when DB down without leaking driver details; /healthz independent |
| 40 | `internal/api/ws_e2e_test.go` | `TestWebSocketEndToEnd` | unit | TP-002: real server + raw WS client: handshake, connected, live feed_item, history, close on suspension |
| 41 | `internal/auth/auth_test.go` | `TestJWTRoundTripAndTamper` | security | JWT issue/verify; tampered and expired tokens rejected |
| 42 | `internal/auth/auth_test.go` | `TestPassword` | security | PBKDF2 verify accepts right password, rejects wrong |
| 43 | `internal/auth/auth_test.go` | `TestTicketSingleUse` | security | Ticket cannot be redeemed twice |
| 44 | `internal/auth/security_test.go` | `TestJWTRejectsAlgNoneAndForgeries` | security | alg=none, missing/extra segments, unsigned tokens rejected |
| 45 | `internal/auth/security_test.go` | `TestJWTRejectsShortSecret` | security | Secrets under 32 bytes refused |
| 46 | `internal/auth/security_test.go` | `TestJWTRejectsTamperedPayload` | security | Payload edited (privilege escalation attempt) is detected |
| 47 | `internal/auth/security_test.go` | `TestJWTRejectsWrongSecretAndIssuer` | security | Tokens signed with another secret or issuer rejected |
| 48 | `internal/auth/security_test.go` | `TestPasswordHashesAreSalted` | security | Same password hashes differently; no plaintext in hash |
| 49 | `internal/auth/security_test.go` | `TestPasswordMalformedHashesRejected` | security | Malformed/absurd-iteration stored hashes never verify |
| 50 | `internal/auth/security_test.go` | `TestTicketExpiresAndIsUnguessable` | security | Tickets expire, are long/random, unknown tickets rejected |
| 51 | `internal/config/config_test.go` | `TestBootstrapAdminMustBeCompleteAndStrong` | security | Bootstrap env must be complete and password >= 12 chars |
| 52 | `internal/config/config_test.go` | `TestConfigLogValueRedactsSecrets` | security | Logging Config never reveals JWT/DB/X/bootstrap secrets |
| 53 | `internal/config/config_test.go` | `TestDatabaseConfig` | unit | DB config defaults, migration URL override, invalid values rejected |
| 54 | `internal/config/config_test.go` | `TestDevelopmentDefaults` | unit | Dev: docs on, market demo |
| 55 | `internal/config/config_test.go` | `TestInvalidSimDemoRejected` | unit | Invalid SIM_DEMO value fails startup |
| 56 | `internal/config/config_test.go` | `TestProductionRejectsPlaceholderSecret` | security | Production refuses the placeholder JWT secret |
| 57 | `internal/config/config_test.go` | `TestProductionRequiresDatabase` | security | Production refuses to start without DATABASE_URL |
| 58 | `internal/config/config_test.go` | `TestProductionSafeDefaults` | security | Production: docs off, demo none, proxy trust off |
| 59 | `internal/config/config_test.go` | `TestSecretRequiredAndLong` | security | Short/missing JWT secret refuses to start |
| 60 | `internal/config/config_test.go` | `TestSourceAndXConfigValidation` | unit | Source interval bounds and X budget/config validation |
| 61 | `internal/delivery/delivery_test.go` | `TestDeliverTargetsOnlyOwnerAndDropsSlowClients` | security | Feed items reach only their owner; slow consumers are disconnected |
| 62 | `internal/delivery/delivery_test.go` | `TestDisconnectUserClosesOnlyThatUsersConnections` | security | Suspension disconnects only the target user's sockets |
| 63 | `internal/delivery/delivery_test.go` | `TestHubConnectionCapPerUser` | security | Per-user WebSocket connection cap enforced |
| 64 | `internal/delivery/delivery_test.go` | `TestOriginCheck` | security | Cross-site WebSocket hijacking blocked by Origin allowlist/same-origin |
| 65 | `internal/delivery/delivery_test.go` | `TestReadFrameAcceptsMaskedText` | unit | Valid masked client frame parsed |
| 66 | `internal/delivery/delivery_test.go` | `TestReadFrameRejectsProtocolViolations` | security | Unmasked, fragmented, oversized, reserved-bit and long control frames rejected |
| 67 | `internal/delivery/delivery_test.go` | `TestShutdownRejectsNewConnections` | unit | Hub refuses registrations after shutdown |
| 68 | `internal/delivery/delivery_test.go` | `TestWriteFrameEncoding` | unit | Server frame length encoding (short and extended) |
| 69 | `internal/domain/event/registry_test.go` | `TestKindOf` | unit | Field whitelist lookup (built-in vs unknown) |
| 70 | `internal/domain/event/registry_test.go` | `TestRegisterValidation` | security | Invalid type ids, field collisions, bad kinds, unknown template fields, duplicates rejected |
| 71 | `internal/domain/event/registry_test.go` | `TestRenderAndValidate` | unit | Template rendering, severity, payload kind validation, unknown type rejected |
| 72 | `internal/domain/invite/invite_test.go` | `TestNewCodeIsRandomPrefixedAndHashed` | security | Invite codes unique, prefixed, high-entropy; hash differs from code |
| 73 | `internal/domain/invite/invite_test.go` | `TestStatus` | unit | Invite status: pending/used/expired/revoked precedence |
| 74 | `internal/domain/rule/rule_test.go` | `TestMatch` | unit | Condition operators across number/string/bool; missing field never matches |
| 75 | `internal/domain/rule/rule_test.go` | `TestNormalizeRejects` | security | Unknown fields, wrong operators/value types, oversized rule lists rejected |
| 76 | `internal/domain/user/user_test.go` | `TestNormalizeProfile` | unit | Profile normalization: case/space folding, keyword phrases, de-duplication |
| 77 | `internal/domain/user/user_test.go` | `TestNormalizeProfileRejects` | security | Invalid kinds, lengths, control chars, unknown types/fields, limits rejected |
| 78 | `internal/infrastructure/eventbus/memory_test.go` | `TestPublishDeliversToAllSubscribers` | unit | Bus fan-out |
| 79 | `internal/infrastructure/eventbus/memory_test.go` | `TestSlowSubscriberNeverBlocksPublisher` | security | Slow subscriber cannot stall ingestion; drops are counted (availability) |
| 80 | `internal/infrastructure/eventbus/memory_test.go` | `TestSubscriberPanicIsIsolated` | security | A panicking subscriber does not kill the bus |
| 81 | `internal/infrastructure/eventbus/memory_test.go` | `TestUnsubscribeAndClose` | unit | Unsubscribe/Close idempotent; publish after close errors |
| 82 | `internal/ingestion/ingestion_test.go` | `TestMalformedAndInvalidEventsAreDropped` | security | Ingestion drops malformed messages and registry-invalid events; stops on cancel |
| 83 | `internal/personalization/engine_test.go` | `TestEntityWithConditions` | unit | Entity subscription + condition filter; item rendered and persisted |
| 84 | `internal/personalization/engine_test.go` | `TestKeywordIsWholeWord` | unit | Keyword matches whole words only |
| 85 | `internal/personalization/engine_test.go` | `TestKeywordSubscription` | unit | Keyword subscription delivers an explained item; others get nothing |
| 86 | `internal/personalization/engine_test.go` | `TestOneItemPerUserPerEvent` | unit | Multiple matching subscriptions yield one item |
| 87 | `internal/personalization/engine_test.go` | `TestTypeRestrictionAndMute` | unit | Type restriction and mutes suppress items |
| 88 | `internal/personalization/engine_test.go` | `TestUpsertAndRemoveUpdateIndex` | unit | Index updated on profile change and removal (e.g. suspension) |
| 89 | `internal/personalization/engine_test.go` | `TestUsersAreIsolated` | security | Items only reach the subscribed user |
| 90 | `internal/safefetch/policy_test.go` | `TestNoOutboundHTTPOutsideSafefetch` | security | Architecture rule: no outbound HTTP code outside internal/safefetch (except the localhost healthcheck) |
| 91 | `internal/safefetch/safefetch_test.go` | `TestBlockedAddressTable` | security | SSRF: loopback, private, link-local, metadata, CGNAT, multicast, mapped IPv6 and doc ranges blocked; public allowed |
| 92 | `internal/safefetch/safefetch_test.go` | `TestBodySizeLimit` | security | Responses over the cap are rejected (memory exhaustion) |
| 93 | `internal/safefetch/safefetch_test.go` | `TestDNSRebindingToPrivateAddressBlocked` | security | Hostnames resolving to private/metadata addresses are refused at dial time |
| 94 | `internal/safefetch/safefetch_test.go` | `TestDoesNotStoreCookies` | security | No cookie jar: session cookies are never replayed |
| 95 | `internal/safefetch/safefetch_test.go` | `TestExtraHeadersAreForwarded` | unit | Caller headers such as If-None-Match are forwarded |
| 96 | `internal/safefetch/safefetch_test.go` | `TestIgnoresProxyEnvironment` | security | HTTP(S)_PROXY from the environment is ignored |
| 97 | `internal/safefetch/safefetch_test.go` | `TestPickAllowedDropsPrivateAnswers` | security | Mixed DNS answers: only allowed addresses are ever dialed |
| 98 | `internal/safefetch/safefetch_test.go` | `TestRedirectToDisallowedSchemeBlocked` | security | Redirect to file:// is refused |
| 99 | `internal/safefetch/safefetch_test.go` | `TestRedirectToMetadataBlocked` | security | Redirect to the cloud metadata IP is blocked |
| 100 | `internal/safefetch/safefetch_test.go` | `TestRejectsBadURLs` | security | SSRF: bad schemes, ports, credentials in URL, metadata/localhost/.internal hosts, IP literals refused |
| 101 | `internal/safefetch/safefetch_test.go` | `TestSuccessfulFetch` | unit | Fetch returns status, headers, body, final URL; no cookies/auth sent |
| 102 | `internal/safefetch/safefetch_test.go` | `TestTimeout` | security | Slow servers cannot hold a fetch open (slowloris) |
| 103 | `internal/safefetch/safefetch_test.go` | `TestTooManyRedirects` | security | Redirect loops are bounded |
| 104 | `internal/storage/memory/invites_test.go` | `TestAuditRepoOrderAndCap` | unit | Audit repo newest-first and capped |
| 105 | `internal/storage/memory/invites_test.go` | `TestInviteStoredOnlyAsHash` | security | No plaintext invite code in storage |
| 106 | `internal/storage/memory/invites_test.go` | `TestRedeemExactlyOnceUnderConcurrency` | security | 100 concurrent redeems of one invite succeed exactly once (race test) |
| 107 | `internal/storage/memory/invites_test.go` | `TestRedeemRejections` | security | Expired, unknown and wrong-email redemptions rejected |
| 108 | `internal/storage/memory/invites_test.go` | `TestReleaseRestoresOnlyForSameUser` | security | Only the redeeming user can release an invite |
| 109 | `internal/storage/memory/invites_test.go` | `TestRevokeOnlyPending` | unit | Revoke semantics: only pending invites |
| 110 | `internal/storage/memory/repos_test.go` | `TestFeedRepoCapOrderAndIsolation` | security | Per-user cap, newest-first, no cross-user reads |
| 111 | `internal/storage/memory/repos_test.go` | `TestUserRepoEmailUniqueCaseInsensitive` | unit | Email uniqueness case-insensitive; not-found errors |
| 112 | `internal/storage/postgres/db_test.go` | `TestInvalidURLErrorNeverLeaksPassword` | security | Invalid DATABASE_URL errors never echo credentials |
| 113 | `internal/storage/postgres/db_test.go` | `TestMigrationFilesAreComplete` | security | Every migration has up+down, sequential versions, and enables RLS when it creates tables |
| 114 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigAutoDetectsTransactionPoolerPort` | unit | Port 6543 auto-selects simple protocol; direct keeps prepared statements |
| 115 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigExplicitModesOverrideAuto` | unit | DB_SIMPLE_PROTOCOL true/false overrides auto detection |
| 116 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigStripsPgbouncerAndUsesSimpleProtocol` | unit | Prisma-style pgbouncer param removed and simple protocol selected |
| 117 | `internal/storage/postgres/db_test.go` | `TestUnreachableDatabaseErrorNeverLeaksPassword` | security | Connection failures never echo the password |
| 118 | `internal/storage/postgres/postgres_test.go` | `TestAuditRepoAndAgePrune` | integration | Audit newest-first, limit honored, age-based retention |
| 119 | `internal/storage/postgres/postgres_test.go` | `TestDeletingUserCascadesFeed` | integration | Deleting a user removes feed items; foreign key rejects items for unknown users |
| 120 | `internal/storage/postgres/postgres_test.go` | `TestEveryTableHasRowLevelSecurity` | security | Every table has RLS enabled (provider HTTP APIs cannot read data) |
| 121 | `internal/storage/postgres/postgres_test.go` | `TestFeedRepoOrderIsolationAndPrune` | security | Feed newest-first, per-user isolation, retention keeps newest N per user |
| 122 | `internal/storage/postgres/postgres_test.go` | `TestInviteRedeemExactlyOnceConcurrently` | security | 50 concurrent redeems against real Postgres succeed exactly once (atomic UPDATE) |
| 123 | `internal/storage/postgres/postgres_test.go` | `TestInviteRules` | security | Postgres invites: expired, unknown, wrong-email rejected; release only by redeemer; revoke rules; no plaintext code stored |
| 124 | `internal/storage/postgres/postgres_test.go` | `TestMigrateUpDownUpAndIdempotent` | integration | Migrations apply, re-apply as a no-op, revert (down) and re-apply on a real Postgres |
| 125 | `internal/storage/postgres/postgres_test.go` | `TestSQLInjectionStringsAreInert` | security | Injection payloads in email/id lookups match nothing and damage nothing; quotes in data round-trip |
| 126 | `internal/storage/postgres/postgres_test.go` | `TestUserRepoRoundTrip` | integration | Users: case-insensitive unique email, profile (conditions, lists) round-trips, status, CHECK constraints, not-found errors |

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
| Production-safe defaults | TestProductionSafeDefaults, TestProductionRequiresDatabase, TestBootstrapAdminMustBeCompleteAndStrong |
| SSRF (outbound requests) | TestBlockedAddressTable, TestRejectsBadURLs, TestDNSRebindingToPrivateAddressBlocked, TestPickAllowedDropsPrivateAnswers, TestRedirectToMetadataBlocked, TestRedirectToDisallowedSchemeBlocked, TestTooManyRedirects, TestBodySizeLimit, TestTimeout, TestIgnoresProxyEnvironment, TestDoesNotStoreCookies, TestNoOutboundHTTPOutsideSafefetch |
| Database security (RLS, injection, credentials) | TestEveryTableHasRowLevelSecurity, TestMigrationFilesAreComplete, TestSQLInjectionStringsAreInert, TestInvalidURLErrorNeverLeaksPassword, TestUnreachableDatabaseErrorNeverLeaksPassword, TestReadyzReflectsDependencyHealth, TestConfigLogValueRedactsSecrets |
| Atomic invite redemption on real Postgres | TestInviteRedeemExactlyOnceConcurrently, TestInviteRules |
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
| Connector fixtures (RSS, web monitor), dedupe across restarts | TP-012–TP-015 |
| X API budget cap / spend stop | TP-016 |
| Connector fixtures for the three news pages and X (once built) | M3/M4 |
| Restore-from-backup drill | M6 |
| Load test (many WS clients, ingest burst) | M6 |

## Verification log (append a row every time the suite is run)

| Date | Who | Command | Result | Notes |
|---|---|---|---|---|
| 2026-10-17 | owner | `docker compose run --rm check` (Windows 11) | M0 + M1 confirmed working | owner statement; output not shared. Covers code up to 0.2.0-dev |
| _pending_ | owner | `docker compose run --rm check` | not yet run on 0.3.0-dev / 0.4.0-dev | adds access, admin, docs, WS e2e, Postgres, safefetch (Postgres tests need the compose `postgres` service) |
