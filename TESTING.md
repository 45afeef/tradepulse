# TESTING.md — test register & strategy

Single place to see **what is tested**, **what kind**, and **what is not yet covered**. Update it in the same change as any test (Definition of Done).

**Run everything (Docker only):** `docker compose run --rm check`  ·  one package: `docker compose run --rm test go test ./internal/auth/...`

**Totals:** 245 tests (150 security-focused). Postgres integration tests need `TEST_DATABASE_URL` (set automatically by `docker compose run --rm check` and CI; skipped otherwise).

## Conventions
- stdlib `testing`, table-driven where useful, always `-race`.
- No live network. Connector tests use recorded fixtures (from M3).
- Every security rule in `AGENTS.md` needs at least one test that fails if the rule is weakened.
- Type `security` = abuse/attack/isolation/limit test; `unit` = behavior.

## Test register

| # | Package / file | Test | Type | What it proves |
|---|---|---|---|---|
| 1 | `examples/market/market_test.go` | `TestAlertTemplatesAreValid` | unit | Example-domain templates normalize against their event types |
| 2 | `examples/market/market_test.go` | `TestDetectorFiresOnceWithCooldown` | unit | Signal fires once when price move + volume spike coincide; cooldown suppresses repeats |
| 3 | `examples/market/market_test.go` | `TestNSETickProcess` | unit | Tick -> price_changed + volume_spike events validate against registry and render correctly |
| 4 | `examples/market/market_test.go` | `TestNSETickRejectsInvalid` | security | Malformed/zero/empty tick payloads are rejected (untrusted source input) |
| 5 | `examples/market/market_test.go` | `TestRegisterTypes` | unit | Market example types register without errors |
| 6 | `examples/status/status_test.go` | `TestAlertTemplatesAreValid` | unit | Example-domain templates normalize against their event types |
| 7 | `examples/status/status_test.go` | `TestStatusProcessAndRender` | unit | Non-market example works on the unchanged core (domain-agnostic proof) |
| 8 | `examples/status/status_test.go` | `TestStatusRejectsInvalid` | security | Invalid status messages (bad JSON, unknown status, negative latency) rejected |
| 9 | `internal/access/access_test.go` | `TestEnsureAdminCreatesOnceAndNeverOverwrites` | security | Bootstrap admin created once with hashed password; existing admin never overwritten |
| 10 | `internal/access/access_test.go` | `TestEnsureAdminValidation` | security | Bootstrap rejects malformed email and short password |
| 11 | `internal/alerting/engine_test.go` | `TestCooldownSuppressesRepeats` | unit | Cooldown suppresses repeats and releases after the interval |
| 12 | `internal/alerting/engine_test.go` | `TestDisabledRulesSuspensionAndReload` | security | Disabled rules, suspended users and removed rules never fire; reactivation restores them |
| 13 | `internal/alerting/engine_test.go` | `TestEntityKeywordAndSourceRulesFire` | unit | Entity, whole-phrase keyword and source rules fire; explainable fires; in-app and webhook dispatch |
| 14 | `internal/alerting/engine_test.go` | `TestHourlyCapAutoPausesAndReEnableResetsTheCount` | security | Hourly cap auto-pauses a noisy rule, tells the user once, and re-enabling restarts the count |
| 15 | `internal/alerting/engine_test.go` | `TestLoadSkipsInactiveUsersAndRestoresCooldown` | security | Startup skips suspended users' rules and restores cooldowns from last_fired_at |
| 16 | `internal/alerting/engine_test.go` | `TestPreviewReplaysRecentEventsWithoutSideEffects` | unit | Preview counts matches/fires/pause with cooldown, ignores old events, stores and sends nothing |
| 17 | `internal/alerting/engine_test.go` | `TestTypesAndConditionsFilter` | unit | Type and condition filters decide whether a rule fires |
| 18 | `internal/alerting/engine_test.go` | `TestUsersAreIsolated` | security | Alerts reach only the rule's owner |
| 19 | `internal/alerting/sender_test.go` | `TestChannelIsDisabledAfterConsecutiveFailuresAndOwnerIsNotified` | security | Channel auto-disabled after N consecutive failures; owner notified; no further calls |
| 20 | `internal/alerting/sender_test.go` | `TestDeliveryIsSignedAndRecorded` | security | Outbound delivery is HMAC-signed (receiver can verify), carries event headers, and is recorded |
| 21 | `internal/alerting/sender_test.go` | `TestNetworkErrorsRetryThenRecordAFailure` | unit | Network errors retry then count as one failed delivery |
| 22 | `internal/alerting/sender_test.go` | `TestNeverSendsForDeletedForeignOrDisabledChannels` | security | No delivery to deleted, disabled or another user's channels |
| 23 | `internal/alerting/sender_test.go` | `TestOutboundSecretsAreSeparateFromInboundSecrets` | security | Channel secrets differ from inbound source secrets and per channel/version |
| 24 | `internal/alerting/sender_test.go` | `TestQueueOverflowIsRecordedNotSilent` | security | A full delivery queue leaves a record instead of dropping silently |
| 25 | `internal/alerting/sender_test.go` | `TestRunStopsOnCancelAndDeliversQueuedJobs` | unit | Sender workers deliver queued jobs and stop on cancel |
| 26 | `internal/alerting/sender_test.go` | `TestSuccessResetsTheFailureCounter` | unit | Failures must be consecutive to disable a channel |
| 27 | `internal/alerting/sender_test.go` | `TestTestDeliveriesAreClearlyMarked` | unit | Test deliveries carry test:true |
| 28 | `internal/alerting/sender_test.go` | `TestTransientFailuresAreRetriedPermanentOnesAreNot` | unit | 5xx/429 retried; 4xx and redirects are not |
| 29 | `internal/api/admin_test.go` | `TestAdminCannotSuspendSelfAndUnknownUser404` | security | Admin lock-out guard (no self-suspend) and 404 for unknown user |
| 30 | `internal/api/admin_test.go` | `TestAdminRoutesRejectMembersAndAnonymous` | security | All /admin routes: member -> 403, anonymous -> 401 (authorization) |
| 31 | `internal/api/admin_test.go` | `TestAuditTrailRecordsAdminActions` | security | Audit log records admin actions with actor id and never leaks invite codes |
| 32 | `internal/api/admin_test.go` | `TestEmailBoundInviteOnlyWorksForThatEmail` | security | Email-bound invites reject other emails; matching is case-insensitive |
| 33 | `internal/api/admin_test.go` | `TestFailedRegistrationDoesNotBurnInvite` | security | Duplicate-email failure releases the invite so it stays usable |
| 34 | `internal/api/admin_test.go` | `TestInviteIsSingleUseOverHTTP` | security | A used invite cannot register a second account (403) |
| 35 | `internal/api/admin_test.go` | `TestInviteListNeverExposesCodes` | security | Invite list leaks neither the code nor its hash |
| 36 | `internal/api/admin_test.go` | `TestRegistrationRequiresValidInvite` | security | Registration needs a valid invite_code: missing -> 4xx, unknown -> 403, valid -> 201 |
| 37 | `internal/api/admin_test.go` | `TestRevokeInvite` | security | Revoked invites cannot be redeemed; double revoke 409; unknown 404 |
| 38 | `internal/api/admin_test.go` | `TestSuspensionIsImmediateAndReversible` | security | Suspended token/login rejected at once; reactivation restores access |
| 39 | `internal/api/admin_test.go` | `TestTokenOfDeletedOrUnknownUserIsRejected` | security | A validly signed token for a non-existent account is rejected |
| 40 | `internal/api/admin_test.go` | `TestUserListNeverExposesPasswordHashes` | security | Admin user list never contains password hashes |
| 41 | `internal/api/alerts_test.go` | `TestAlertEndpointsRequireAuthentication` | security | All 16 alert/channel endpoints return 401 anonymously |
| 42 | `internal/api/alerts_test.go` | `TestAlertFiresEndToEndWithExplanationAndIsolation` | unit | Manual item -> keyword rule fires with reason; webhook dispatch; other users get nothing |
| 43 | `internal/api/alerts_test.go` | `TestAlertLifecycleAndDefaults` | unit | Create (defaults/normalization), get, update, list, delete |
| 44 | `internal/api/alerts_test.go` | `TestAlertOwnershipIsolation` | security | Other users get 404 on get/update/delete and never see my rules |
| 45 | `internal/api/alerts_test.go` | `TestAlertRuleLimit` | security | 51st rule is refused (limit 50) |
| 46 | `internal/api/alerts_test.go` | `TestAlertTemplatesListed` | unit | Templates endpoint lists the shipped templates |
| 47 | `internal/api/alerts_test.go` | `TestAutoPauseIsVisibleAndReversibleViaAPI` | security | Noisy rule auto-pauses (visible reason), stays silent, and resumes when re-enabled |
| 48 | `internal/api/alerts_test.go` | `TestChannelCreationRules` | security | HTTPS-only, SSRF-safe channel URLs; secret shown once and never listed; isolation; limit 5 |
| 49 | `internal/api/alerts_test.go` | `TestChannelSecretRotationEnableAndTest` | security | Rotation, enable and test only for the owner; disabled channels cannot be tested |
| 50 | `internal/api/alerts_test.go` | `TestChannelsNeedSigningKey` | security | Channels refused when WEBHOOK_SIGNING_KEY is not configured |
| 51 | `internal/api/alerts_test.go` | `TestCreateAlertFromTemplate` | unit | From-template creation, name override, unknown template/empty values/bad channel |
| 52 | `internal/api/alerts_test.go` | `TestCreateAlertValidation` | security | 10 invalid rule bodies rejected, including other users' channel ids |
| 53 | `internal/api/alerts_test.go` | `TestDeleteChannelDetachesItFromRules` | unit | Deleting a channel removes it from rules (fallback in_app) |
| 54 | `internal/api/alerts_test.go` | `TestDeliveriesAreVisibleOnlyToTheOwner` | security | Delivery history is scoped to the owner |
| 55 | `internal/api/alerts_test.go` | `TestPreviewEndpointHasNoSideEffects` | unit | Preview reports matches/fires and saves/sends nothing |
| 56 | `internal/api/alerts_test.go` | `TestSuspendedMemberAlertsStopAndResume` | security | Suspension stops alerts immediately; reactivation restores them |
| 57 | `internal/api/api_test.go` | `TestAuthEndpointsAreRateLimited` | security | Login brute force hits 429 |
| 58 | `internal/api/api_test.go` | `TestCORSAllowlist` | security | Only allow-listed origins receive CORS headers |
| 59 | `internal/api/api_test.go` | `TestDocsOnlyWhenEnabled` | security | /docs and /openapi.* exist only when enabled; relaxed CSP only on /docs; 404 in production |
| 60 | `internal/api/api_test.go` | `TestEventTypesEndpoint` | unit | GET /event-types lists registry types and built-in fields |
| 61 | `internal/api/api_test.go` | `TestLoginDoesNotRevealWhetherEmailExists` | security | Unknown email and wrong password give identical 401 responses (no account enumeration) |
| 62 | `internal/api/api_test.go` | `TestOversizedBodyRejected` | security | Request bodies over the operation limit are rejected |
| 63 | `internal/api/api_test.go` | `TestPrivateRoutesRequireValidToken` | security | Every private route returns 401 for missing, garbage or tampered tokens |
| 64 | `internal/api/api_test.go` | `TestRegisterValidationAndDuplicates` | security | Bad email/password rejected; duplicate email (case-insensitive) -> 409 |
| 65 | `internal/api/api_test.go` | `TestSecurityHeaders` | security | nosniff, frame deny, no-referrer, no-store, strict CSP, HSTS (prod), request id |
| 66 | `internal/api/api_test.go` | `TestSubscriptionValidation` | security | Unknown fields/kinds/types/properties and malformed JSON are rejected with 4xx |
| 67 | `internal/api/api_test.go` | `TestSubscriptionsAreIsolatedBetweenUsers` | security | User B can never read user A's subscriptions (data isolation) |
| 68 | `internal/api/api_test.go` | `TestWebSocketTicketIsSingleUseEvenOnFailedHandshake` | security | WS ticket is single-use even if the handshake fails; missing/bad ticket -> 401 |
| 69 | `internal/api/docs_test.go` | `TestAdminOperationsDeclareSecurity` | security | Every /me and /admin operation declares a security requirement in the OpenAPI spec |
| 70 | `internal/api/docs_test.go` | `TestEveryOperationIsDocumented` | unit | Docs completeness: every operation has summary, description, declared tag; overview has required sections |
| 71 | `internal/api/hooks_test.go` | `TestWebhookAcceptsSignedPayloadAndDeduplicates` | unit | Signed webhook publishes a normalized event; retries and identical payloads are idempotent |
| 72 | `internal/api/hooks_test.go` | `TestWebhookBodyLimitAndRateLimit` | security | 64 KB body cap and per-source rate limit |
| 73 | `internal/api/hooks_test.go` | `TestWebhookPayloadValidation` | security | Unknown fields, missing/long titles, javascript:/credential links, bad dates rejected with 422 |
| 74 | `internal/api/hooks_test.go` | `TestWebhookRejectsBadCredentialsIdentically` | security | 10 failure modes (signature, timestamp, token, unknown/disabled source...) return the same 401 and publish nothing |
| 75 | `internal/api/hooks_test.go` | `TestWebhookSecretNeverExposedAndRotationInvalidatesOld` | security | Secrets never appear in GET/audit; rotation kills the old secret; only admins rotate |
| 76 | `internal/api/hooks_test.go` | `TestWebhookTokenMode` | security | Token mode accepts header/Bearer, rejects wrong tokens and signatures |
| 77 | `internal/api/hooks_test.go` | `TestWebhookUpdatesSourceStatsAndIsNotPolled` | unit | Deliveries update source stats; webhook sources cannot be run or test-fetched |
| 78 | `internal/api/hooks_test.go` | `TestWebhooksDisabledWithoutSigningKey` | security | Without WEBHOOK_SIGNING_KEY webhook sources cannot be created and deliveries get 503 |
| 79 | `internal/api/middleware_test.go` | `TestBodyLimitMiddleware` | security | Global 1 MB request body cap |
| 80 | `internal/api/middleware_test.go` | `TestClientIPTrustsProxyHeaderOnlyWhenConfigured` | security | X-Forwarded-For spoofing ignored unless TRUST_PROXY; last (proxy-added) entry used |
| 81 | `internal/api/middleware_test.go` | `TestLimiterTokenBucket` | unit | Token bucket honors burst, limit and per-key isolation |
| 82 | `internal/api/middleware_test.go` | `TestPanicIsRecoveredWithoutLeakingDetails` | security | Panics return generic 500; internal detail never reaches the client |
| 83 | `internal/api/ready_test.go` | `TestReadyzReflectsDependencyHealth` | security | /readyz 503 when DB down without leaking driver details; /healthz independent |
| 84 | `internal/api/sources_test.go` | `TestCatalogShowsOnlyEnabledSourcesWithoutURLs` | security | Member catalog lists only enabled sources and never source URLs |
| 85 | `internal/api/sources_test.go` | `TestCreateSourceRejectsDangerousAndInvalidInput` | security | SSRF targets (metadata, localhost, private, file:, odd ports, credentials) and invalid configs rejected |
| 86 | `internal/api/sources_test.go` | `TestManualPostPublishesAndIsAudited` | unit | Manual item published to the bus with normalized fields and recorded in the audit log |
| 87 | `internal/api/sources_test.go` | `TestManualPostValidation` | security | Manual posts: unknown/non-manual source, empty title, javascript: and credentialed links rejected |
| 88 | `internal/api/sources_test.go` | `TestRunNowRequiresEnabledPollingSource` | unit | Run-now only for enabled polling sources; sets next_run_at |
| 89 | `internal/api/sources_test.go` | `TestSourceAdminRoutesRequireAdmin` | security | All source admin routes: member 403, anonymous 401; members may read the catalog |
| 90 | `internal/api/sources_test.go` | `TestSourceLifecycleAndNameImmutability` | security | Create (disabled by default), duplicate 409, update re-checks SSRF, name immutable, delete |
| 91 | `internal/api/sources_test.go` | `TestTestEndpointPreviewsWithoutSaving` | security | Draft test shows extracted items, saves nothing, reports fetch errors safely, rejects SSRF |
| 92 | `internal/api/ws_e2e_test.go` | `TestWebSocketEndToEnd` | unit | TP-002: real server + raw WS client: handshake, connected, live feed_item, history, close on suspension |
| 93 | `internal/api/xsource_test.go` | `TestCreateXSourceResolvesUserAndIgnoresClientUserID` | security | User id comes from X, never from the client; lookup spend is recorded |
| 94 | `internal/api/xsource_test.go` | `TestUsageEndpointIsAdminOnly` | security | Usage endpoint: member 403, anonymous 401 |
| 95 | `internal/api/xsource_test.go` | `TestXDraftTestShowsPostsWithinBudget` | unit | Draft test previews X posts through the budgeted connector |
| 96 | `internal/api/xsource_test.go` | `TestXSourceNeedsServerConfiguration` | security | X sources are refused with a clear message when X_BEARER_TOKEN is not configured |
| 97 | `internal/api/xsource_test.go` | `TestXSourceValidation` | security | Bad handles, missing config, interval bounds and unknown accounts rejected |
| 98 | `internal/api/xsource_test.go` | `TestXUpdateKeepsResolvedIDForSameHandleAndReResolvesOnChange` | unit | No extra billed lookup for the same handle; changed handle re-resolves |
| 99 | `internal/auth/auth_test.go` | `TestJWTRoundTripAndTamper` | security | JWT issue/verify; tampered and expired tokens rejected |
| 100 | `internal/auth/auth_test.go` | `TestPassword` | security | PBKDF2 verify accepts right password, rejects wrong |
| 101 | `internal/auth/auth_test.go` | `TestTicketSingleUse` | security | Ticket cannot be redeemed twice |
| 102 | `internal/auth/security_test.go` | `TestJWTRejectsAlgNoneAndForgeries` | security | alg=none, missing/extra segments, unsigned tokens rejected |
| 103 | `internal/auth/security_test.go` | `TestJWTRejectsShortSecret` | security | Secrets under 32 bytes refused |
| 104 | `internal/auth/security_test.go` | `TestJWTRejectsTamperedPayload` | security | Payload edited (privilege escalation attempt) is detected |
| 105 | `internal/auth/security_test.go` | `TestJWTRejectsWrongSecretAndIssuer` | security | Tokens signed with another secret or issuer rejected |
| 106 | `internal/auth/security_test.go` | `TestPasswordHashesAreSalted` | security | Same password hashes differently; no plaintext in hash |
| 107 | `internal/auth/security_test.go` | `TestPasswordMalformedHashesRejected` | security | Malformed/absurd-iteration stored hashes never verify |
| 108 | `internal/auth/security_test.go` | `TestTicketExpiresAndIsUnguessable` | security | Tickets expire, are long/random, unknown tickets rejected |
| 109 | `internal/config/config_test.go` | `TestAlertSettings` | unit | ALERT_MAX_FIRES_PER_HOUR default, override and invalid values |
| 110 | `internal/config/config_test.go` | `TestBootstrapAdminMustBeCompleteAndStrong` | security | Bootstrap env must be complete and password >= 12 chars |
| 111 | `internal/config/config_test.go` | `TestConfigLogValueRedactsSecrets` | security | Logging Config never reveals JWT/DB/X/bootstrap secrets |
| 112 | `internal/config/config_test.go` | `TestDatabaseConfig` | unit | DB config defaults, migration URL override, invalid values rejected |
| 113 | `internal/config/config_test.go` | `TestDevelopmentDefaults` | unit | Dev: docs on, market demo |
| 114 | `internal/config/config_test.go` | `TestInvalidSimDemoRejected` | unit | Invalid SIM_DEMO value fails startup |
| 115 | `internal/config/config_test.go` | `TestLogValueRedactsWebhookKey` | security | Logging config never reveals the webhook signing key |
| 116 | `internal/config/config_test.go` | `TestProductionRejectsPlaceholderSecret` | security | Production refuses the placeholder JWT secret |
| 117 | `internal/config/config_test.go` | `TestProductionRequiresDatabase` | security | Production refuses to start without DATABASE_URL |
| 118 | `internal/config/config_test.go` | `TestProductionSafeDefaults` | security | Production: docs off, demo none, proxy trust off |
| 119 | `internal/config/config_test.go` | `TestSecretRequiredAndLong` | security | Short/missing JWT secret refuses to start |
| 120 | `internal/config/config_test.go` | `TestSourceAndXConfigValidation` | unit | Source interval bounds and X budget/config validation |
| 121 | `internal/config/config_test.go` | `TestWebhookAndXSettings` | unit | Webhook/X env settings: defaults, overrides, invalid values rejected |
| 122 | `internal/connector/connector_test.go` | `TestParseAtom` | unit | Atom entries parsed with alternate link and fragment removed |
| 123 | `internal/connector/connector_test.go` | `TestParseFeedDoesNotExpandEntities` | security | XML entity-expansion (billion laughs) payloads do not expand |
| 124 | `internal/connector/connector_test.go` | `TestParseFeedHandlesLatin1AndIsBounded` | security | Non-UTF-8 feeds decoded; item count capped at 100 |
| 125 | `internal/connector/connector_test.go` | `TestParseFeedRejectsGarbageAndHTML` | security | Non-feed bodies (empty, HTML, JSON, broken XML) are rejected |
| 126 | `internal/connector/connector_test.go` | `TestParseHTMLWithSelectors` | unit | Web monitor extraction with CSS selectors, relative links, max items, link-as-item |
| 127 | `internal/connector/connector_test.go` | `TestParseRSSSanitizesAndKeys` | security | RSS: HTML/script stripped to text, javascript: links dropped, relative links resolved, stable dedupe keys |
| 128 | `internal/connector/connector_test.go` | `TestParseRobotsRules` | unit | robots.txt groups, longest-match, wildcards, $ anchors, our token beats * |
| 129 | `internal/connector/connector_test.go` | `TestRSSFetchConditionalAndStatuses` | unit | Conditional GET headers sent, 304 handled, HTTP errors and safefetch errors propagate |
| 130 | `internal/connector/connector_test.go` | `TestRobotsFailsClosedOnServerErrorAndCaches` | security | robots.txt 5xx fails closed; successful robots responses are cached |
| 131 | `internal/connector/connector_test.go` | `TestStripTagsAndSafeLink` | security | Text extraction drops script/style/iframe; only plain http(s) links accepted; rune-safe truncation |
| 132 | `internal/connector/connector_test.go` | `TestWebMonitorRespectsRobotsAndOverride` | security | robots.txt disallow blocks the fetch; recorded override and 404 robots allow it |
| 133 | `internal/connector/connector_test.go` | `TestWebMonitorValidateSelectors` | security | Invalid CSS selectors and missing config rejected before any fetch |
| 134 | `internal/connector/x_test.go` | `TestXBudgetIsEnforcedBeforeEveryCall` | security | X: no call is made without budget; page size shrinks to the remaining budget |
| 135 | `internal/connector/x_test.go` | `TestXErrorsAreSafeAndNeverLeakTheToken` | security | X: HTTP/transport errors are explained without ever exposing the bearer token |
| 136 | `internal/connector/x_test.go` | `TestXFetchParsesPostsAndUsesCursor` | unit | X: posts parsed to plain text, since_id cursor sent, auth header, spend recorded for posts returned |
| 137 | `internal/connector/x_test.go` | `TestXResolveLooksUpUserAndChargesOneRead` | unit | X: handle lookup, one read charged, unknown account and no-budget cases |
| 138 | `internal/connector/x_test.go` | `TestXSpendAccumulatesAndStopsAtCap` | security | X: ledger accumulates across calls and polling stops at the monthly cap |
| 139 | `internal/connector/x_test.go` | `TestXValidateAndUnresolvedSource` | security | X: token required, handle required, unresolved sources are not fetched |
| 140 | `internal/delivery/delivery_test.go` | `TestDeliverTargetsOnlyOwnerAndDropsSlowClients` | security | Feed items reach only their owner; slow consumers are disconnected |
| 141 | `internal/delivery/delivery_test.go` | `TestDisconnectUserClosesOnlyThatUsersConnections` | security | Suspension disconnects only the target user's sockets |
| 142 | `internal/delivery/delivery_test.go` | `TestHubConnectionCapPerUser` | security | Per-user WebSocket connection cap enforced |
| 143 | `internal/delivery/delivery_test.go` | `TestOriginCheck` | security | Cross-site WebSocket hijacking blocked by Origin allowlist/same-origin |
| 144 | `internal/delivery/delivery_test.go` | `TestReadFrameAcceptsMaskedText` | unit | Valid masked client frame parsed |
| 145 | `internal/delivery/delivery_test.go` | `TestReadFrameRejectsProtocolViolations` | security | Unmasked, fragmented, oversized, reserved-bit and long control frames rejected |
| 146 | `internal/delivery/delivery_test.go` | `TestShutdownRejectsNewConnections` | unit | Hub refuses registrations after shutdown |
| 147 | `internal/delivery/delivery_test.go` | `TestWriteFrameEncoding` | unit | Server frame length encoding (short and extended) |
| 148 | `internal/domain/alert/alert_test.go` | `TestNormalizeAppliesDefaultsAndCanonicalForms` | unit | Alert spec defaults (cooldown 300, severity, in_app) and canonical scope/types/name |
| 149 | `internal/domain/alert/alert_test.go` | `TestNormalizeRejectsInvalidSpecs` | security | 16 invalid alert specs rejected (names, scope, control chars, types, fields, cooldown, severity, foreign channels) |
| 150 | `internal/domain/alert/alert_test.go` | `TestTemplates` | unit | Template registry: defaults, spec from template, duplicates/invalid rejected, sorted list |
| 151 | `internal/domain/event/registry_test.go` | `TestKindOf` | unit | Field whitelist lookup (built-in vs unknown) |
| 152 | `internal/domain/event/registry_test.go` | `TestRegisterValidation` | security | Invalid type ids, field collisions, bad kinds, unknown template fields, duplicates rejected |
| 153 | `internal/domain/event/registry_test.go` | `TestRenderAndValidate` | unit | Template rendering, severity, payload kind validation, unknown type rejected |
| 154 | `internal/domain/invite/invite_test.go` | `TestNewCodeIsRandomPrefixedAndHashed` | security | Invite codes unique, prefixed, high-entropy; hash differs from code |
| 155 | `internal/domain/invite/invite_test.go` | `TestStatus` | unit | Invite status: pending/used/expired/revoked precedence |
| 156 | `internal/domain/rule/rule_test.go` | `TestMatch` | unit | Condition operators across number/string/bool; missing field never matches |
| 157 | `internal/domain/rule/rule_test.go` | `TestNormalizeRejects` | security | Unknown fields, wrong operators/value types, oversized rule lists rejected |
| 158 | `internal/domain/sourcedef/sourcedef_test.go` | `TestHealth` | unit | Source health states: disabled, pending, ok, degraded, failing, overdue (silence watchdog) |
| 159 | `internal/domain/sourcedef/sourcedef_test.go` | `TestNormalizeRejects` | security | Bad names/types/URLs/intervals/entities/selectors and robots overrides without a reason are rejected |
| 160 | `internal/domain/sourcedef/sourcedef_test.go` | `TestNormalizeRejectsBadWebhookAndXConfig` | security | Bad auth modes, handles, user ids and intervals rejected |
| 161 | `internal/domain/sourcedef/sourcedef_test.go` | `TestNormalizeValidSources` | unit | Source config normalization for rss, webmonitor and manual (slug, URL, entities, defaults) |
| 162 | `internal/domain/sourcedef/sourcedef_test.go` | `TestNormalizeWebhookAndXSources` | unit | Webhook/X normalization: defaults, handle cleanup, type-specific config cleared elsewhere |
| 163 | `internal/domain/sourcedef/sourcedef_test.go` | `TestRobotsOverrideKeepsReasonOnlyWhenIgnoring` | unit | robots.txt override is recorded only when used; stale reasons cleared |
| 164 | `internal/domain/user/user_test.go` | `TestNormalizeProfile` | unit | Profile normalization: case/space folding, keyword phrases, de-duplication |
| 165 | `internal/domain/user/user_test.go` | `TestNormalizeProfileRejects` | security | Invalid kinds, lengths, control chars, unknown types/fields, limits rejected |
| 166 | `internal/infrastructure/eventbus/memory_test.go` | `TestPublishDeliversToAllSubscribers` | unit | Bus fan-out |
| 167 | `internal/infrastructure/eventbus/memory_test.go` | `TestSlowSubscriberNeverBlocksPublisher` | security | Slow subscriber cannot stall ingestion; drops are counted (availability) |
| 168 | `internal/infrastructure/eventbus/memory_test.go` | `TestSubscriberPanicIsIsolated` | security | A panicking subscriber does not kill the bus |
| 169 | `internal/infrastructure/eventbus/memory_test.go` | `TestUnsubscribeAndClose` | unit | Unsubscribe/Close idempotent; publish after close errors |
| 170 | `internal/ingestion/ingestion_test.go` | `TestMalformedAndInvalidEventsAreDropped` | security | Ingestion drops malformed messages and registry-invalid events; stops on cancel |
| 171 | `internal/ingestion/scheduler_test.go` | `TestCursorIsPersistedAndPushSourcesAreNeverPolled` | unit | Scheduler keeps connector cursors and never polls webhook sources |
| 172 | `internal/ingestion/scheduler_test.go` | `TestFailureBacksOffRecordsRunAndRecovers` | unit | Failures back off, are recorded, turn health to failing, and recovery resets |
| 173 | `internal/ingestion/scheduler_test.go` | `TestFirstRunIsBaselineThenOnlyNewItemsPublish` | unit | First success stores items without flooding; later runs publish only new items; dedupe; own interval |
| 174 | `internal/ingestion/scheduler_test.go` | `TestNextIntervalAndFailureDelay` | unit | Adaptive interval and failure back-off arithmetic |
| 175 | `internal/ingestion/scheduler_test.go` | `TestNotModifiedKeepsValidatorsAndStretchesInterval` | unit | 304 keeps ETag/Last-Modified; unchanged sources are polled less often |
| 176 | `internal/ingestion/scheduler_test.go` | `TestPrunesOldRunsAndItems` | unit | Retention prunes old runs and raw items, keeps recent ones |
| 177 | `internal/ingestion/scheduler_test.go` | `TestSlowSourceDoesNotBlockOthersAndIsNotRunTwice` | security | A slow source cannot delay others and is never started twice (availability) |
| 178 | `internal/ingestion/scheduler_test.go` | `TestStaleItemsAreStoredButNotPublished` | unit | Items older than the max age are remembered but never published |
| 179 | `internal/ingestion/scheduler_test.go` | `TestStorageFailureDoesNotAdvanceSuccessState` | security | If items cannot be recorded the run fails and nothing is published |
| 180 | `internal/ingestion/scheduler_test.go` | `TestTickRespectsScheduleEnabledAndType` | unit | Per-source intervals honored; disabled and manual sources never polled |
| 181 | `internal/personalization/engine_test.go` | `TestEntityWithConditions` | unit | Entity subscription + condition filter; item rendered and persisted |
| 182 | `internal/personalization/engine_test.go` | `TestKeywordIsWholeWord` | unit | Keyword matches whole words only |
| 183 | `internal/personalization/engine_test.go` | `TestKeywordSubscription` | unit | Keyword subscription delivers an explained item; others get nothing |
| 184 | `internal/personalization/engine_test.go` | `TestOneItemPerUserPerEvent` | unit | Multiple matching subscriptions yield one item |
| 185 | `internal/personalization/engine_test.go` | `TestTypeRestrictionAndMute` | unit | Type restriction and mutes suppress items |
| 186 | `internal/personalization/engine_test.go` | `TestUpsertAndRemoveUpdateIndex` | unit | Index updated on profile change and removal (e.g. suspension) |
| 187 | `internal/personalization/engine_test.go` | `TestUsersAreIsolated` | security | Items only reach the subscribed user |
| 188 | `internal/safefetch/policy_test.go` | `TestNoOutboundHTTPOutsideSafefetch` | security | Architecture rule: no outbound HTTP code outside internal/safefetch (except the localhost healthcheck) |
| 189 | `internal/safefetch/safefetch_test.go` | `TestBlockedAddressTable` | security | SSRF: loopback, private, link-local, metadata, CGNAT, multicast, mapped IPv6 and doc ranges blocked; public allowed |
| 190 | `internal/safefetch/safefetch_test.go` | `TestBodySizeLimit` | security | Responses over the cap are rejected (memory exhaustion) |
| 191 | `internal/safefetch/safefetch_test.go` | `TestDNSRebindingToPrivateAddressBlocked` | security | Hostnames resolving to private/metadata addresses are refused at dial time |
| 192 | `internal/safefetch/safefetch_test.go` | `TestDoesNotStoreCookies` | security | No cookie jar: session cookies are never replayed |
| 193 | `internal/safefetch/safefetch_test.go` | `TestExtraHeadersAreForwarded` | unit | Caller headers such as If-None-Match are forwarded |
| 194 | `internal/safefetch/safefetch_test.go` | `TestIgnoresProxyEnvironment` | security | HTTP(S)_PROXY from the environment is ignored |
| 195 | `internal/safefetch/safefetch_test.go` | `TestPickAllowedDropsPrivateAnswers` | security | Mixed DNS answers: only allowed addresses are ever dialed |
| 196 | `internal/safefetch/safefetch_test.go` | `TestPostIsSubjectToTheSameDestinationRules` | security | safefetch.Post refuses metadata, loopback, private, file:, credentials and odd ports |
| 197 | `internal/safefetch/safefetch_test.go` | `TestPostSendsBodyAndHeadersAndNeverFollowsRedirects` | security | safefetch.Post sends body/headers, caps the response, and returns 3xx instead of following it |
| 198 | `internal/safefetch/safefetch_test.go` | `TestRedirectToDisallowedSchemeBlocked` | security | Redirect to file:// is refused |
| 199 | `internal/safefetch/safefetch_test.go` | `TestRedirectToMetadataBlocked` | security | Redirect to the cloud metadata IP is blocked |
| 200 | `internal/safefetch/safefetch_test.go` | `TestRejectsBadURLs` | security | SSRF: bad schemes, ports, credentials in URL, metadata/localhost/.internal hosts, IP literals refused |
| 201 | `internal/safefetch/safefetch_test.go` | `TestSuccessfulFetch` | unit | Fetch returns status, headers, body, final URL; no cookies/auth sent |
| 202 | `internal/safefetch/safefetch_test.go` | `TestTimeout` | security | Slow servers cannot hold a fetch open (slowloris) |
| 203 | `internal/safefetch/safefetch_test.go` | `TestTooManyRedirects` | security | Redirect loops are bounded |
| 204 | `internal/safefetch/safefetch_test.go` | `TestValidateURLStaticChecks` | security | ValidateURL rejects private/metadata/odd-port/credential/scheme URLs without network |
| 205 | `internal/storage/memory/alerts_test.go` | `TestAlertRuleRepoIsScopedToTheOwner` | security | Memory: other users cannot read/update/delete rules; pause and mark-fired |
| 206 | `internal/storage/memory/alerts_test.go` | `TestChannelRepoCountersScopingAndSecrets` | security | Memory: failure counter, enable/disable reset, secret rotation only by the owner |
| 207 | `internal/storage/memory/alerts_test.go` | `TestFireAndDeliveryHistoryOrderScopeAndPrune` | unit | Memory: history newest-first, per-user scope, retention |
| 208 | `internal/storage/memory/invites_test.go` | `TestAuditRepoOrderAndCap` | unit | Audit repo newest-first and capped |
| 209 | `internal/storage/memory/invites_test.go` | `TestInviteStoredOnlyAsHash` | security | No plaintext invite code in storage |
| 210 | `internal/storage/memory/invites_test.go` | `TestItemStoreDedupesAndPrunes` | security | Memory dedupe store: duplicates rejected, 50 concurrent inserts win once, prune |
| 211 | `internal/storage/memory/invites_test.go` | `TestRedeemExactlyOnceUnderConcurrency` | security | 100 concurrent redeems of one invite succeed exactly once (race test) |
| 212 | `internal/storage/memory/invites_test.go` | `TestRedeemRejections` | security | Expired, unknown and wrong-email redemptions rejected |
| 213 | `internal/storage/memory/invites_test.go` | `TestReleaseRestoresOnlyForSameUser` | security | Only the redeeming user can release an invite |
| 214 | `internal/storage/memory/invites_test.go` | `TestRevokeOnlyPending` | unit | Revoke semantics: only pending invites |
| 215 | `internal/storage/memory/invites_test.go` | `TestSecretVersionAndUsageLedger` | unit | Memory: secret version bump, concurrent ledger adds without lost updates, month rollover |
| 216 | `internal/storage/memory/invites_test.go` | `TestSourceRepoCRUDAndIndependentState` | unit | Memory source repo: unique names, Update vs SaveState independence, delete frees the name |
| 217 | `internal/storage/memory/repos_test.go` | `TestFeedRepoCapOrderAndIsolation` | security | Per-user cap, newest-first, no cross-user reads |
| 218 | `internal/storage/memory/repos_test.go` | `TestUserRepoEmailUniqueCaseInsensitive` | unit | Email uniqueness case-insensitive; not-found errors |
| 219 | `internal/storage/postgres/alerts_test.go` | `TestAlertRuleRepoRoundTripScopingAndPause` | integration | Postgres: rule JSON round-trip, owner scoping, update/pause/mark-fired |
| 220 | `internal/storage/postgres/alerts_test.go` | `TestChannelRepoAtomicFailureCounterAndScoping` | security | Postgres: 20 concurrent failures counted exactly; CHECK on type; owner scoping |
| 221 | `internal/storage/postgres/alerts_test.go` | `TestDeliveryHistoryOrderScopeAndPrune` | integration | Postgres: delivery history order, scope and retention |
| 222 | `internal/storage/postgres/alerts_test.go` | `TestFiresPruneAndUserDeletionCascadesEverything` | integration | Postgres: fire ordering, retention, and cascade delete of all alert data with the user |
| 223 | `internal/storage/postgres/db_test.go` | `TestInvalidURLErrorNeverLeaksPassword` | security | Invalid DATABASE_URL errors never echo credentials |
| 224 | `internal/storage/postgres/db_test.go` | `TestMigrationFilesAreComplete` | security | Every migration has up+down, sequential versions, and enables RLS when it creates tables |
| 225 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigAutoDetectsTransactionPoolerPort` | unit | Port 6543 auto-selects simple protocol; direct keeps prepared statements |
| 226 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigExplicitModesOverrideAuto` | unit | DB_SIMPLE_PROTOCOL true/false overrides auto detection |
| 227 | `internal/storage/postgres/db_test.go` | `TestPrepareConfigStripsPgbouncerAndUsesSimpleProtocol` | unit | Prisma-style pgbouncer param removed and simple protocol selected |
| 228 | `internal/storage/postgres/db_test.go` | `TestUnreachableDatabaseErrorNeverLeaksPassword` | security | Connection failures never echo the password |
| 229 | `internal/storage/postgres/postgres_test.go` | `TestAuditRepoAndAgePrune` | integration | Audit newest-first, limit honored, age-based retention |
| 230 | `internal/storage/postgres/postgres_test.go` | `TestDeletingUserCascadesFeed` | integration | Deleting a user removes feed items; foreign key rejects items for unknown users |
| 231 | `internal/storage/postgres/postgres_test.go` | `TestEveryTableHasRowLevelSecurity` | security | Every table has RLS enabled (provider HTTP APIs cannot read data) |
| 232 | `internal/storage/postgres/postgres_test.go` | `TestFeedRepoOrderIsolationAndPrune` | security | Feed newest-first, per-user isolation, retention keeps newest N per user |
| 233 | `internal/storage/postgres/postgres_test.go` | `TestInviteRedeemExactlyOnceConcurrently` | security | 50 concurrent redeems against real Postgres succeed exactly once (atomic UPDATE) |
| 234 | `internal/storage/postgres/postgres_test.go` | `TestInviteRules` | security | Postgres invites: expired, unknown, wrong-email rejected; release only by redeemer; revoke rules; no plaintext code stored |
| 235 | `internal/storage/postgres/postgres_test.go` | `TestMigrateUpDownUpAndIdempotent` | integration | Migrations apply, re-apply as a no-op, revert (down) and re-apply on a real Postgres |
| 236 | `internal/storage/postgres/postgres_test.go` | `TestSQLInjectionStringsAreInert` | security | Injection payloads in email/id lookups match nothing and damage nothing; quotes in data round-trip |
| 237 | `internal/storage/postgres/postgres_test.go` | `TestUserRepoRoundTrip` | integration | Users: case-insensitive unique email, profile (conditions, lists) round-trips, status, CHECK constraints, not-found errors |
| 238 | `internal/storage/postgres/sources_test.go` | `TestItemStoreAtomicDedupeUnderConcurrency` | security | 40 concurrent inserts of one key against Postgres win exactly once; FK rejects unknown sources |
| 239 | `internal/storage/postgres/sources_test.go` | `TestRunRepoRecentPruneAndCascadeDelete` | integration | Run history order, retention, and cascade delete with the source |
| 240 | `internal/storage/postgres/sources_test.go` | `TestSourceRepoRoundTripAndIndependentState` | integration | Postgres sources: web config round-trip, unique name, Update/SaveState independence, CHECK constraint |
| 241 | `internal/storage/postgres/sources_test.go` | `TestUsageLedgerIsAtomicAndMonthly` | integration | Postgres: 50 concurrent upserts lose nothing; months are separate |
| 242 | `internal/storage/postgres/sources_test.go` | `TestWebhookXConfigCursorAndSecretVersion` | integration | Postgres: webhook/x config, cursor and secret version round-trip; CHECK still enforced |
| 243 | `internal/webhook/webhook_test.go` | `TestSecretDerivation` | security | Webhook secrets are deterministic, differ per source/version/key, and never contain the signing key |
| 244 | `internal/webhook/webhook_test.go` | `TestSignAndVerify` | security | HMAC verification: valid, tampered body, wrong secret, bad/missing/stale/future timestamps, signature bound to timestamp |
| 245 | `internal/webhook/webhook_test.go` | `TestVerifyToken` | security | Constant-time token comparison |

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
| Inbound webhooks (auth, replay, enumeration, limits, rotation) | TestSignAndVerify, TestSecretDerivation, TestWebhookRejectsBadCredentialsIdentically, TestWebhookTokenMode, TestWebhookPayloadValidation, TestWebhookBodyLimitAndRateLimit, TestWebhookSecretNeverExposedAndRotationInvalidatesOld, TestWebhooksDisabledWithoutSigningKey |
| Paid API cost control and secret handling (X) | TestXBudgetIsEnforcedBeforeEveryCall, TestXSpendAccumulatesAndStopsAtCap, TestUsageLedgerIsAtomicAndMonthly, TestXErrorsAreSafeAndNeverLeakTheToken, TestXSourceNeedsServerConfiguration, TestCreateXSourceResolvesUserAndIgnoresClientUserID, TestLogValueRedactsWebhookKey |
| Alerts: isolation, guardrails, auto-pause, suspension | TestAlertOwnershipIsolation, TestAlertRuleLimit, TestHourlyCapAutoPausesAndReEnableResetsTheCount, TestAutoPauseIsVisibleAndReversibleViaAPI, TestSuspendedMemberAlertsStopAndResume, TestDisabledRulesSuspensionAndReload, TestCreateAlertValidation |
| Outbound webhook channels (signing, SSRF, retries, disable, secrets) | TestDeliveryIsSignedAndRecorded, TestChannelCreationRules, TestChannelIsDisabledAfterConsecutiveFailuresAndOwnerIsNotified, TestNeverSendsForDeletedForeignOrDisabledChannels, TestPostSendsBodyAndHeadersAndNeverFollowsRedirects, TestPostIsSubjectToTheSameDestinationRules, TestOutboundSecretsAreSeparateFromInboundSecrets, TestChannelSecretRotationEnableAndTest |
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
| Live X API call with a real token (owner has not tested X) | owner, when ready |
| Live webhook from the owner's own automation | owner run |
| Restore-from-backup drill | M6 |
| Load test (many WS clients, ingest burst) | M6 |

## Verification log (append a row every time the suite is run)

| Date | Who | Command | Result | Notes |
|---|---|---|---|---|
| 2026-10-17 | owner | `docker compose run --rm check` (Windows 11) | M0 + M1 confirmed working | owner statement; output not shared. Covers code up to 0.2.0-dev |
| 2026-10-17 | owner | `docker compose run --rm check` (Windows 11) + manual run | 0.3.0-dev and 0.4.0-dev working; data persists across restarts | owner statement (one safefetch test failed and was fixed) |
| 2026-10-18 | owner | `docker compose run --rm check` (Windows 11) | 0.6.0-dev: 8 failures (1 real bug, 4 test mistakes), all other tests passed | fixed in 0.6.1-dev |
| _pending_ | owner | `docker compose run --rm check` | re-run on 0.6.1-dev (M4 fixes) and 0.7.0-dev (M5) | adds access, admin, docs, WS e2e, Postgres, safefetch (Postgres tests need the compose `postgres` service) |
