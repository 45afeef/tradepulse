# Load test (k6)

Goal: find the real limits of one instance before inviting members (10 to 50 expected). Script: `deploy/loadtest/k6.js` (read API, signed webhook ingest, WebSocket fan-out).

## Run (against staging, never production)
1. Create a member and a `webhook` source named `loadtest` (enabled); note its secret. Optionally subscribe the member to the `loadtest` source so the WebSocket clients receive items.
2. `k6 run -e BASE_URL=https://STAGING -e TOKEN=<member access token, valid >10 min: use JWT_TTL=1h on staging> -e HOOK_NAME=loadtest -e HOOK_SECRET=whsec_... deploy/loadtest/k6.js`
3. Watch `/metrics`, Cloud Run CPU/memory and database connections during the run.

## Indicative targets (validate, then replace with measured numbers)
- Read API p95 < 500 ms at 25 concurrent users; error rate < 1%.
- Webhook ingest 5/s sustained without dropped events (`bus_dropped_events` stays 0).
- 50 simultaneous WebSocket clients connected for 5 minutes; item delivery latency (ingest -> client) well under 1 s.
- Memory stays flat (no leak) and CPU well below 1 vCPU.

## Results (fill in after each run)
| Date | Version | Scenario | VUs / rate | p95 | Errors | CPU / memory | Notes |
|---|---|---|---|---|---|---|---|
| _not yet run_ | | | | | | | |

Record limits found (connections, DB pool pressure, rate limits that tripped) in `docs/RUNBOOK.md` "Known limits".
