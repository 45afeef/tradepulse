// k6 load test: read API, signed webhook ingest, WebSocket fan-out. See docs/LOADTEST.md.
// Run:  k6 run -e BASE_URL=https://staging.example.com -e TOKEN=<member access token> \
//             -e HOOK_NAME=loadtest -e HOOK_SECRET=whsec_... deploy/loadtest/k6.js
import http from 'k6/http';
import ws from 'k6/ws';
import crypto from 'k6/crypto';
import { check, sleep } from 'k6';

const BASE = __ENV.BASE_URL;
const TOKEN = __ENV.TOKEN;
const HOOK = __ENV.HOOK_NAME;
const SECRET = __ENV.HOOK_SECRET;
const auth = { headers: { Authorization: `Bearer ${TOKEN}` } };

export const options = {
  scenarios: {
    api_read: { executor: 'ramping-vus', exec: 'readApi', startVUs: 1,
      stages: [{ duration: '1m', target: 25 }, { duration: '3m', target: 25 }, { duration: '30s', target: 0 }] },
    webhook_ingest: { executor: 'constant-arrival-rate', exec: 'ingest', rate: 5, timeUnit: '1s', duration: '4m', preAllocatedVUs: 10 },
    ws_fanout: { executor: 'ramping-vus', exec: 'listen', startVUs: 1,
      stages: [{ duration: '1m', target: 50 }, { duration: '3m', target: 50 }, { duration: '30s', target: 0 }] },
  },
  thresholds: {
    'http_req_failed{scenario:api_read}': ['rate<0.01'],
    'http_req_duration{scenario:api_read}': ['p(95)<500'],
    'http_req_failed{scenario:webhook_ingest}': ['rate<0.01'],
    'ws_connecting': ['p(95)<1000'],
  },
};

export function readApi() {
  const r = http.batch([['GET', `${BASE}/api/v1/me/feed?limit=50`, null, auth], ['GET', `${BASE}/api/v1/event-types`, null, auth]]);
  check(r[0], { 'feed 200': (x) => x.status === 200 });
  sleep(1);
}

export function ingest() {
  const body = JSON.stringify({ id: `lt-${__VU}-${__ITER}-${Date.now()}`, title: `load test ${__ITER}`, summary: 'k6' });
  const ts = String(Math.floor(Date.now() / 1000));
  const sig = 'sha256=' + crypto.hmac('sha256', SECRET, `${ts}.${body}`, 'hex');
  const r = http.post(`${BASE}/api/v1/hooks/${HOOK}`, body, { headers: { 'X-TP-Timestamp': ts, 'X-TP-Signature': sig, 'Content-Type': 'application/json' } });
  check(r, { 'accepted': (x) => x.status === 201 || x.status === 200 });
}

export function listen() {
  const t = http.post(`${BASE}/api/v1/ws-ticket`, null, auth);
  if (t.status !== 200) { sleep(5); return; }
  const url = `${BASE.replace('http', 'ws')}/ws?ticket=${t.json('ticket')}`;
  const res = ws.connect(url, {}, (socket) => {
    let got = 0;
    socket.on('message', () => { got++; });
    socket.setTimeout(() => socket.close(), 45000);
  });
  check(res, { 'ws upgraded': (x) => x && x.status === 101 });
}
