// Verifies that every API path the web client calls exists in the server's OpenAPI document.
//   npm run contract                      (reads ../openapi.json)
//   API_URL=http://localhost:8080 npm run contract   (reads the live /openapi.json)
import { readFileSync, readdirSync } from 'node:fs';

const spec = process.env.API_URL
  ? await (await fetch(process.env.API_URL + '/openapi.json')).json()
  : JSON.parse(readFileSync(new URL('../../openapi.json', import.meta.url), 'utf8'));

const src = readFileSync(new URL('../src/api/endpoints.ts', import.meta.url), 'utf8');
const calls = [...src.matchAll(/\b(get|post|put|del)\(\s*[`'"](\/api\/v1\/[^`'"?$]*)/g)];
const methodOf = { get: 'get', post: 'post', put: 'put', del: 'delete' };
let bad = 0;
for (const [, m, raw] of calls) {
  const path = raw.replace(/\$\{[^}]*\}/g, '{id}').replace(/\/$/, '');
  const hit = Object.keys(spec.paths).find((p) => p === path || p.replace(/\{[^}]+\}/g, '{id}') === path);
  if (!hit || !spec.paths[hit][methodOf[m]]) { console.error(`NOT IN SPEC: ${m.toUpperCase()} ${raw}`); bad++; }
}
console.log(`${calls.length} client calls checked, ${bad} not in the API contract`);
process.exit(bad ? 1 : 0);
