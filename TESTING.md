# TESTING.md — test register & strategy

Single place to see **what is tested**, **what kind**, and **what is not yet covered**. Update it in the same change as any test (Definition of Done).

**Run everything (Docker only):** `docker compose run --rm check`  ·  one package: `docker compose run --rm test go test ./internal/auth/...`

**Totals:** 165 tests (106 security-focused). Postgres integration tests need `TEST_DATABASE_URL` (set automatically by `docker compose run --rm check` and CI; skipped otherwise).

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
| 40 | `internal/api/sources_test.go` | `TestCatalogShowsOnlyEnabledSourcesWithoutURLs` | security | Member catalog lists only enabled sources and never source URLs |
| 41 | `internal/api/sources_test.go` | `TestCreateSourceRejectsDangerousAndInvalidInput` | security | SSRF targets (metadata, localhost, private, file:, odd ports, credentials) and invalid configs rejected |
| 42 | `internal/api/sources_test.go` | `TestManualPostPublishesAndIsAudited` | unit | Manual item published to the bus with normalized fields and recorded in the audit log |
| 43 | `internal/api/sources_test.go` | `TestManualPostValidation` | security | Manual posts: unknown/non-manual source, empty title, javascript: and credentialed links rejected |
| 44 | `internal/api/sources_test.go` | `TestRunNowRequiresEnabledPollingSource` | unit | Run-now only for enabled polling sources; sets next_run_at |
| 45 | `internal/api/sources_test.go` | `TestSourceAdminRoutesRequireAdmin` | security | All source admin routes: member 403, anonymous 401; members may read the catalog |
| 46 | `internal/api/sources_test.go` | `TestSourceLifecycleAndNameImmutability` | security | Create (disabled by default), duplicate 409, update re-checks SSRF, name immutable, delete |
| 47 | `internal/api/sources_test.go` | `TestTestEndpointPreviewsWithoutSaving` | security | Draft test shows extracted items, saves nothing, reports fetch errors safely, rejects SSRF |
| 48 | `internal/api/ws_e2e_test.go` | `TestWebSocketEndToEnd` | unit | TP-002: real server + raw WS client: handshake, connected, live feed_item, history, close on suspension |
| 49 | `internal/auth/auth_test.go` | `TestJWTRoundTripAndTamper` | security | JWT issue/verify; tampered and expired tokens rejected |
| 50 | `internal/auth/auth_test.go` | `TestPassword` | security | PBKDF2 verify accepts right password, rejects wrong |
| 51 | `internal/auth/auth_test.go` | `TestTicketSingleUse` | security | Ticket cannot be redeemed twice |
| 52 | `internal/auth/security_test.go` | `TestJWTRejectsAlgNoneAndForgeries` | security | alg=none, missing/extra segments, unsigned tokens rejected |
| 53 | `internal/auth/security_test.go` | `TestJWTRejectsShortSecret` | security | Secrets under 32 bytes refused |
| 54 | `internal/auth/security_test.go` | `TestJWTRejectsTamperedPayload` | security | Payload edited (privilege escalation attempt) is detected |
| 55 | `internal/auth/security_test.go` | `TestJWTRejectsWrongSecretAndIssuer` | security | Tokens signed with another secret or issuer rejected |
| 56 | `internal/auth/security_test.go` | `TestPasswordHashesAreSalted` | security | Same password hashes differently; no plaintext in hash |
| 57 | `internal/auth/security_test.go` | `TestPasswordMalformedHashesRejected` | security | Malformed/absurd-iteration stored hashes never verify |
| 58 | `internal/auth/security_test.go` | `TestTicketExpiresAndIsUnguessable` | security | Tickets expire, are long/random, unknown tickets rejected |
| 59 | `internal/config/config_test.go` | `TestBootstrapAdminMustBeCompleteAndStrong` | security | Bootstrap env must be complete and password >= 12 chars |
| 60 | `internal/config/config_test.go` | `TestConfigLogValueRedactsSecrets` | security | Logging Config never reveals JWT/DB/X/bootstrap secrets |
| 61 | `internal/config/config_test.go` | `TestDatabaseConfig` | unit | DB config defaults, migration URL override, invalid values rejected |
| 62 | `internal/config/config_test.go` | `TestDevelopmentDefaults` | unit | Dev: docs on, market demo |
| 63 | `internal/config/config_test.go` | `TestInvalidSimDemoRejected` | unit | Invalid SIM_DEMO value fails startup |
| 64 | `internal/config/config_test.go` | `TestProductionRejectsPlaceholderSecret` | security | Production refuses the placeholder JWT secret |
| 65 | `internal/config/config_test.go` | `TestProductionRequiresDatabase` | security | Production refuses to start without DATABASE_URL |
| 66 | `internal/config/config_test.go` | `TestProductionSafeDefaults` | security | Production: docs off, demo none, proxy trust off |
| 67 | `internal/config/config_test.go` | `TestSecretRequiredAndLong` | security | Short/missing JWT secret refuses to start |
| 68 | `internal/config/config_test.go` | `TestSourceAndXConfigValidation` | unit | Source interval bounds and X budget/config validation |
| 69 | `internal/connector/connector_test.go` | `TestParseAtom` | unit | Atom entries parsed with alternate link and fragment removed |
| 70 | `internal/connector/connector_test.go` | `TestParseFeedDoesNotExpandEntities` | security | XML entity-expansion (billion laughs) payloads do not expand |
| 71 | `internal/connector/connector_test.go` | `TestParseFeedHandlesLatin1AndIsBounded` | security | Non-UTF-8 feeds decoded; item count capped at 100 |
| 72 | `internal/connector/connector_test.go` | `TestParseFeedRejectsGarbageAndHTML` | security | Non-feed bodies (empty, HTML, JSON, broken XML) are rejected |
| 73 | `internal/connector/connector_test.go` | `TestParseHTMLWithSelectors` | unit | Web monitor extraction with CSS selectors, relative links, max items, link-as-item |
| 74 | `internal/connector/connector_test.go` | `TestParseRSSSanitizesAndKeys` | security | RSS: HTML/script stripped to text, javascript: links dropped, relative links resolved, stable dedupe keys |
| 75 | `internal/connector/connector_test.go` | `TestParseRobotsRules` | unit | robots.txt groups, longest-match, wildcards, $ anchors, our token beats * |
| 76 | `internal/connector/connector_test.go` | `TestRSSFetchConditionalAndStatuses` | unit | Conditional GET headers sent, 304 handled, HTTP errors and safefetch errors propagate |
| 77 | `internal/connector/connector_test.go` | `TestRobotsFailsClosedOnServerErrorAndCaches` | security | robots.txt 5xx fails closed; successful robots responses are cached |
| 78 | `internal/connector/connector_test.go` | `TestStripTagsAndSafeLink` | security | Text extraction drops script/style/iframe; only plain http(s) links accepted; rune-safe truncation |
| 79 | `internal/connector/connector_test.go` | `TestWebMonitorRespectsRobotsAndOverride` | security | robots.txt disallow blocks the fetch; recorded override and 404 robots allow it |
| 80 | `internal/connector/connector_test.go` | `TestWebMonitorValidateSelectors` | security | Invalid CSS selectors and missing config rejected before any fetch |
| 81 | `internal/delivery/delivery_test.go` | `TestDeliverTargetsOnlyOwnerAndDropsSlowClients` | security | Feed items reach only their owner; slow consumers are disconnected |
| 82 | `internal/delivery/delivery_test.go` | `TestDisconnectUserClosesOnlyThatUsersConnections` | security | Suspension disconnects only the target user's sockets |
| 83 | `internal/delivery/delivery_test.go` | `TestHubConnectionCapPerUser` | security | Per-user WebSocket connection cap enforced |
| 84 | `internal/delivery/delivery_test.go` | `TestOriginCheck` | security | Cross-site WebSocket hijacking blocked by Origin allowlist/same-origin |
| 85 | `internal/delivery/delivery_test.go` | `TestReadFrameAcceptsMaskedText` | unit | Valid masked client frame parsed |
| 86 | `internal/delivery/delivery_test.go` | `TestReadFrameRejectsProtocolViolations` | security | Unmasked, fragmented, oversized, reserved-bit and long control frames rejected |
| 87 | `internal/delivery/delivery_test.go` | `TestShutdownRejectsNewConnections` | unit | Hub refuses registrations after shutdown |
| 88 | `internal/delivery/delivery_test.go` | `TestWriteFrameEncoding` | unit | Server frame length encoding (short and extended) |
| 89 | `internal/domain/event/registry_test.go` | `TestKindOf` | unit | Field whitelist lookup (built-in vs unknown) |
| 90 | `internal/domain/event/registry_test.go` | `TestRegisterValidation` | security | Invalid type ids, field collisions, bad kinds, unknown template fields, duplicates rejected |
| 91 | `internal/domain/event/registry_test.go` | `TestRenderAndValidate` | unit | Template rendering, severity, payload kind validation, unknown type rejected |
| 92 | `internal/domain/invite/invite_test.go` | `TestNewCodeIsRandomPrefixedAndHashed` | security | Invite codes unique, prefixed, high-entropy; hash differs from code |
| 93 | `internal/domain/invite/invite_test.go` | `TestStatus` | unit | Invite status: pending/used/expired/revoked precedence |
| 94 | `internal/domain/rule/rule_test.go` | `TestMatch` | unit | Condition operators across number/string/bool; missing field never matches |
| 95 | `internal/domain/rule/rule_test.go` | `TestNormalizeRejects` | security | Unknown fields, wrong operators/value types, oversized rule lists rejected |
| 96 | `internal/domain/sourcedef/sourcedef_test.go` | `TestHealth` | unit | Source health states: disabled, pending, ok, degraded, failing, overdue (silence watchdog) |
| 97 | `internal/domain/sourcedef/sourcedef_test.go` | `TestNormalizeRejects` | security | Bad names/types/URLs/intervals/entities/selectors and robots overrides without a reason are rejected |
| 98 | `internal/domain/sourcedef/sourcedef_test.go` | `TestNormalizeValidSources` | unit | Source config normalization for rss, webmonitor and manual (slug, URL, entities, defaults) |
| 99 | `internal/domain/sourcedef/sourcedef_test.go` | `TestRobotsOverrideKeepsReasonOnlyWhenIgnoring` | unit | robots.txt override is recorded only when used; stale reasons cleared |
| 100 | `internal/domain/user/user_test.go` | `TestNormalizeProfile` | unit | Profile normalization: case/space folding, keyword phrases, de-duplication |
| 101 | `internal/domain/user/user_test.go` | `TestNormalizeProfileRejects` | security | Invalid kinds, lengths, control chars, unknown types/fields, limits rejected |
| 102 | `internal/infrastructure/eventbus/memory_test.go` | `TestPublishDeliversToAllSubscribers` | unit | Bus fan-out |
| 103 | `internal/infrastructure/eventbus/memory_test.go` | `TestSlowSubscriberNeverBlocksPublisher` | security | Slow subscriber cannot stall ingestion; drops are counted (availability) |
| 104 | `internal/infrastructure/eventbus/memory_test.go` | `TestSubscriberPanicIsIsolated` | security | A panicking subscriber does not kill the bus |
| 105 | `internal/infrastructure/eventbus/memory_test.go` | `TestUnsubscribeAndClose` | unit | Unsubscribe/Close idempotent; publish after close errors |
| 106 | `internal/ingestion/ingestion_test.go` | `TestMalformedAndInvalidEventsAreDropped` | security | Ingestion drops malformed messages and registry-invalid events; stops on cancel |
| 107 | `internal/ingestion/scheduler_test.go` | `TestFailureBacksOffRecordsRunAndRecovers` | unit | Failures back off, are recorded, turn health to failing, and recovery resets |
| 108 | `internal/ingestion/scheduler_test.go` | `TestFirstRunIsBaselineThenOnlyNewItemsPublish` | unit | First success stores items without flooding; later runs publish only new items; dedupe; own interval |
| 109 | `internal/ingestion/scheduler_test.go` | `TestNextIntervalAndFailureDelay` | unit | Adaptive interval and failure back-off arithmetic |
| 110 | `internal/ingestion/scheduler_test.go` | `TestNotModifiedKeepsValidatorsAndStretchesInterval` | unit | 304 keeps ETag/Last-Modified; unchanged sources are polled less often |
| 111 | `internal/ingestion/scheduler_test.go` | `TestPrunesOldRunsAndItems` | unit | Retention prunes old runs and raw items, keeps recent ones |
| 112 | `internal/ingestion/scheduler_test.go` | `TestSlowSourceDoesNotBlockOthersAndIsNotRunTwice` | security | A slow source cannot delay others and is never started twice (availability) |
| 113 | `internal/ingestion/scheduler_test.go` | `TestStaleItemsAreStoredButNotPublished` | unit | Items older than the max age are remembered but never published |
| 114 | `internal/ingestion/scheduler_test.go` | `TestStorageFailureDoesNotAdvanceSuccessState` | security | If items cannot be recorded the run fails and nothing is published |
| 115 | `internal/ingestion/scheduler_test.go` | `TestTickRespectsScheduleEnabledAndType` | unit | Per-source intervals honored; disabled and manual sources never polled |
| 116 | `internal/personalization/engine_test.go` | `TestEntityWithConditions` | unit | Entity subscription + condition filter; item rendered and persisted |
| 117 | `internal/personalization/engine_test.go` | `TestKeywordIsWholeWord` | unit | Keyword matches whole words only |
| 118 | `internal/personalization/engine_test.go` | `TestKeywordSubscription` | unit | Keyword subscription delivers an explained item; others get nothing |
| 119 | `internal/personalization/engine_test.go` | `TestOneItemPerUserPerEvent` | unit | Multiple matching subscriptions yield one item |
| 120 | `internal/personalization/engine_test.go` | `TestTypeRestrictionAndMute` | unit | Type restriction and mutes suppress items |
| 121 | `internal/personalization/engine_test.go` | `TestUpsertAndRemoveUpdateIndex` | unit | Index updated on profile change and removal (e.g. suspension) |
| 122 | `internal/personalization/engine_test.go` | `TestUsersAreIsolated` | security | Items only reach the subscribed user |
| 123 | `internal/safefetch/policy_test.go` | `TestNoOutboundHTTPOutsideSafefetch` | security | Architecture rule: no outbound HTTP code outside internal/safefetch (except the localhost healthcheck) |
| 124 | `internal/safefetch/safefetch_test.go` | `TestBlockedAddressTable` | security | SSRF: loopback, private, link-local, metadata, CGNAT, multicast, mapped IPv6 and doc ranges blocked; public allowed |
| 125 | `internal/safefetch/safefetch_test.go` | `TestBodySizeLimit` | security | Responses over the cap are rejected (memory exhaustion) |
| 126 | `internal/safefetch/safefetch_test.go` | `TestDNSRebindingToPrivateAddressBlocked` | security | Hostnames resolving to private/metadata addresses are refused at dial time |
| 127 | `internal/safefetch/safefetch_test.go` | `TestDoesNotStoreCookies` | security | No cookie jar: session cookies are never replayed |
| 128 | `internal/safefetch/safefetch_test.go` | `TestExtraHeadersAreForwarded` | unit | Caller headers such as If-None-Match are forwarded |
| 129 | `internal/safefetch/safefetch_test.go` | `TestIgnoresProxyEnvironment` | security | HTTP(S)_PROXY from the environment is ignored |
| 130 | `internal/safefetch/safefetch_test.go` | `TestPickAllowedDropsPrivateAnswers` | security | Mixed DNS answers: only allowed addresses are ever dialed |
| 131 | `internal/safefetch/safefetch_test.go` | `TestRedirectToDisallowedSchemeBlocked` | security | Redirect to file:// is refused |
| 132 | `internal/safefetch/safefetch_test.go` | `TestRedirectToMetadataBlocked` | security | Redirect to the cloud metadata IP is blocked |
| 133 | `internal/safefetch/safefetch_test.go` | `TestRejectsBadURLs` | security | SSRF: bad schemes, ports, credentials in URL, metadata/localhost/.internal hosts, IP literals refused |
| 134 | `internal/safefetch/safefetch_test.go` | `TestSuccessfulFetch` | unit | Fetch returns status, headers, body, final URL; no cookies/auth sent |
| 135 | `internal/safefetch/safefetch_test.go` | `TestTimeout` | security | Slow servers cannot hold a fetch open (slowloris) |
| 136 | `internal/safefetch/safefetch_test.go` | `TestTooManyRedirects` | security | Redirect loops are bounded |
| 137 | `internal/safefetch/safefetch_test.go` | `TestValidateURLStaticChecks` | security | ValidateURL rejects private/metadata/odd-port/credential/scheme URLs without network |
| 138 | `internal/storage/memory/invites_test.go` | `TestAuditRepoOrderAndCap` | unit | Audit repo newest-first and capped |
| 139 | `internal/storage/memory/invites_test.go` | `TestInviteStoredOnlyAsHash` | security | No plaintext invite code in storage |
| 140 | `internal/storage/memory/invites_test.go` | `TestItemStoreDedupesAndPrunes` | security | Memory dedupe store: duplicates rejected, 50 concurrent inserts win once, prune |
| 141 | `internal/storage/memory/invites_test.go` | `TestRedeemExactlyOnceUnderConcurrency` | security | 100 concurrent redeems of one invite succeed exactly once (race test) |
| 142 | `internal/storage/memory/invites_test.go` | `TestRedeemRejections` | security | Expired, unknown and wrong-email redemptions rejected |
| 143 | `internal/storage/memory/invites_test.go` | `TestReleaseRestoresOnlyForSameUser` | security | Only the redeeming user can release an invite |
| 144 | `internal/storage/memory/invites_test.go` | `TestRevokeOnlyPending` | unit | Revoke semantics: only pending invites |
| 145 | `internal/storage/memory/invites_test.go` | `TestSourceRepoCRUDAndIndependentState` | unit | Memory source repo: unique names, Update vs SaveState independence, delete frees the name |
| 146 | `internal/storage/memory/repos_test.go` | `TestFeedRepoCapOrderAndIsolation` | security | Per-user cap, newest-first, no cross-user reads |
| 147 | `internal/storage/memory/repos_test.go` | `TestUserRepoEmailUniqueCaseInsensitive` | unit | Email uniqueness case-insensitive; not-found errors |
| 148 | `internal/storage/postgres/db_test.go` | `TestInvalidURLErrorNeverLeaksPassword` | security | Invalid DATABASE_URL errors never echo credentials |
| 149 | `internal/storage/postgres/db_test.go` | `TestMigrationFilesAreComplete` | security | Every migration has up+down, sequential versions, and enables RLS when it creates tables |
| 150 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigAutoDetectsTransactionPoolerPort` | unit | Port 6543 auto-selects simple protocol; direct keeps prepared statements |
| 151 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigExplicitModesOverrideAuto` | unit | DB_SIMPLE_PROTOCOL true/false overrides auto detection |
| 152 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigStripsPgbouncerAndUsesSimpleProtocol` | unit | Prisma-style pgbouncer param removed and simple protocol selected |
| 153 | `internal/storage/postgres/db_test.go` | `TestUnreachableDatabaseErrorNeverLeaksPassword` | security | Connection failures never echo the password |
| 154 | `internal/storage/postgres/postgres_test.go` | `TestAuditRepoAndAgePrune` | integration | Audit newest-first, limit honored, age-based retention |
| 155 | `internal/storage/postgres/postgres_test.go` | `TestDeletingUserCascadesFeed` | integration | Deleting a user removes feed items; foreign key rejects items for unknown users |
| 156 | `internal/storage/postgres/postgres_test.go` | `TestEveryTableHasRowLevelSecurity` | security | Every table has RLS enabled (provider HTTP APIs cannot read data) |
| 157 | `internal/storage/postgres/postgres_test.go` | `TestFeedRepoOrderIsolationAndPrune` | security | Feed newest-first, per-user isolation, retention keeps newest N per user |
| 158 | `internal/storage/postgres/postgres_test.go` | `TestInviteRedeemExactlyOnceConcurrently` | security | 50 concurrent redeems against real Postgres succeed exactly once (atomic UPDATE) |
| 159 | `internal/storage/postgres/postgres_test.go` | `TestInviteRules` | security | Postgres invites: expired, unknown, wrong-email rejected; release only by redeemer; revoke rules; no plaintext code stored |
| 160 | `internal/storage/postgres/postgres_test.go` | `TestMigrateUpDownUpAndIdempotent` | integration | Migrations apply, re-apply as a no-op, revert (down) and re-apply on a real Postgres |
| 161 | `internal/storage/postgres/postgres_test.go` | `TestSQLInjectionStringsAreInert` | security | Injection payloads in email/id lookups match nothing and damage nothing; quotes in data round-trip |
| 162 | `internal/storage/postgres/postgres_test.go` | `TestUserRepoRoundTrip` | integration | Users: case-insensitive unique email, profile (conditions, lists) round-trips, status, CHECK constraints, not-found errors |
| 163 | `internal/storage/postgres/sources_test.go` | `TestItemStoreAtomicDedupeUnderConcurrency` | security | 40 concurrent inserts of one key against Postgres win exactly once; FK rejects unknown sources |
| 164 | `internal/storage/postgres/sources_test.go` | `TestRunRepoRecentPruneAndCascadeDelete` | integration | Run history order, retention, and cascade delete with the source |
| 165 | `internal/storage/postgres/sources_test.go` | `TestSourceRepoRoundTripAndIndependentState` | integration | Postgres sources: web config round-trip, unique name, Update/SaveState independence, CHECK constraint |

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
| Source ingestion safety (feeds, HTML, robots, entities, links) | TestParseRSSSanitizesAndKeys, TestParseFeedRejectsGarbageAndHTML, TestParseFeedHandlesLatin1AndIsBounded, TestParseFeedDoesNotExpandEntities, TestStripTagsAndSafeLink, TestWebMonitorValidateSelectors, TestWebMonitorRespectsRobotsAndOverride, TestRobotsFailsClosedOnServerErrorAndCaches |
| Source admin API (authz, SSRF, no URL leaks, manual posts) | TestSourceAdminRoutesRequireAdmin, TestCreateSourceRejectsDangerousAndInvalidInput, TestSourceLifecycleAndNameImmutability, TestTestEndpointPreviewsWithoutSaving, TestCatalogShowsOnlyEnabledSourcesWithoutURLs, TestManualPostValidation |
| Scheduler reliability (isolation, idempotency, failure handling) | TestSlowSourceDoesNotBlockOthersAndIsNotRunTwice, TestStorageFailureDoesNotAdvanceSuccessState, TestItemStoreDedupesAndPrunes, TestItemStoreAtomicDedupeUnderConcurrency |
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
| Live check of the three real news sources (terms/robots/selectors) | owner run + M3 gate |
| X connector, budget cap, webhook inbound | M4 |
| Restore-from-backup drill | M6 |
| Load test (many WS clients, ingest burst) | M6 |

## Verification log (append a row every time the suite is run)

| Date | Who | Command | Result | Notes |
|---|---|---|---|---|
| 2026-10-17 | owner | `docker compose run --rm check` (Windows 11) | M0 + M1 confirmed working | owner statement; output not shared. Covers code up to 0.2.0-dev |
| 2026-10-17 | owner | `docker compose run --rm check` (Windows 11) + manual run | 0.3.0-dev and 0.4.0-dev working; data persists across restarts | owner statement (one safefetch test failed and was fixed) |
| _pending_ | owner | `docker compose run --rm check` | not yet run on 0.5.0-dev | adds access, admin, docs, WS e2e, Postgres, safefetch (Postgres tests need the compose `postgres` service) |
