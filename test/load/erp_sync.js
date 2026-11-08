// k6 load check for the ERP product sync and the website latency it must not
// hurt (quickstart.md section 9; plan T247, T247a). Run it by hand, not in CI.
//
//   Baseline, no sync (SC-003 without load):
//     k6 run test/load/erp_sync.js -e BASE=... -e EMAIL=... -e PASSWORD=... -e SYNC=0
//   Website while syncs run (SC-002 and SC-003):
//     k6 run test/load/erp_sync.js -e BASE=... -e KEYS=k1,...,k10 -e ITEMS=50000 \
//       -e EMAIL=... -e PASSWORD=... -e SYNC=1
//
// The web scenario logs in once and requests the website pages with 20 virtual
// users for 8 minutes, tagged scenario:web. With SYNC=1 the sync scenario posts one
// batch of ITEMS products for each key in KEYS every 7 minutes, and each sync is
// polled until it is completed or failed. The end summary prints the p95 of the web
// requests as a line `WEB_P95_MS <number>`.
import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Trend } from 'k6/metrics';

const BASE = __ENV.BASE;
const KEYS = (__ENV.KEYS || __ENV.KEY || '').split(',').map((k) => k.trim()).filter(Boolean);
const ITEMS = parseInt(__ENV.ITEMS || '50000', 10);
const SYNC = __ENV.SYNC === '1';
const EMAIL = __ENV.EMAIL || '';
const PASSWORD = __ENV.PASSWORD || '';
const pages = ['/customer/dashboard', '/catalog', '/offers', '/orders', '/cart', '/notifications'];

if (!BASE) {
  throw new Error('BASE is required, for example -e BASE=http://localhost:8080');
}
if (SYNC && KEYS.length === 0) {
  throw new Error('KEYS is required when SYNC=1 (the API keys created in the website)');
}

const acceptMs = new Trend('sync_accept_ms', true);
const completeMs = new Trend('sync_complete_ms', true);

const scenarios = {
  web: {
    executor: 'constant-vus',
    vus: 20,
    duration: '8m',
    exec: 'web',
    tags: { scenario: 'web' },
  },
};
if (SYNC) {
  scenarios.sync = {
    executor: 'constant-arrival-rate',
    rate: KEYS.length,
    timeUnit: '7m',
    duration: '8m',
    preAllocatedVUs: KEYS.length,
    maxVUs: KEYS.length * 2,
    exec: 'sync',
  };
}
export const options = { scenarios };

export function setup() {
  if (!EMAIL) {
    return { cookies: {} };
  }
  http.post(`${BASE}/auth/login`, { email: EMAIL, password: PASSWORD }, { redirects: 0 });
  return { cookies: http.cookieJar().cookiesForURL(BASE) };
}

export function web(data) {
  const jar = http.cookieJar();
  for (const [k, v] of Object.entries(data.cookies || {})) {
    jar.set(BASE, k, v[0]);
  }
  const page = pages[Math.floor(Math.random() * pages.length)];
  const res = http.get(`${BASE}${page}`, { tags: { scenario: 'web' } });
  check(res, { 'web page 200': (r) => r.status === 200 });
}

function batch(n) {
  const items = [];
  for (let i = 0; i < ITEMS; i++) {
    items.push({
      externalProductCode: `LOAD-${n}-${i}`,
      arName: `صنف ${i}`,
      publicPrice: 10 + (i % 90),
      discount: 0,
      tax: 0,
      stock: 1 + (i % 50),
    });
  }
  return { clientBatchId: `load-${Date.now()}-${n}`, items };
}

export function sync() {
  const n = exec.scenario.iterationInTest;
  const key = KEYS[n % KEYS.length];
  const headers = { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' };

  const res = http.post(`${BASE}/erp/v1/products/sync`, JSON.stringify(batch(n)), { headers });
  check(res, { '202 within 5 s': (r) => r.status === 202 && r.timings.duration < 5000 });
  acceptMs.add(res.timings.duration);
  if (res.status !== 202) {
    return;
  }

  const syncId = res.json('data.syncId');
  const started = Date.now();
  while (Date.now() - started < 7 * 60 * 1000) {
    sleep(5);
    const st = http.get(`${BASE}/erp/v1/products/sync/${syncId}`, { headers });
    const state = st.status === 200 ? st.json('data.state') : '';
    if (state === 'completed' || state === 'failed') {
      completeMs.add(Date.now() - started);
      return;
    }
  }
}

export function handleSummary(data) {
  const web = data.metrics['http_req_duration{scenario:web}'];
  const p95 = web && web.values ? web.values['p(95)'] : 'no-data';
  return {
    stdout: `WEB_P95_MS ${p95}\nSYNC ${SYNC ? 1 : 0} KEYS ${KEYS.length} ITEMS ${ITEMS}\n`,
  };
}
