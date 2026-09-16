// k6 run -e BASE=https://your-host -e EMAIL=... -e PASSWORD=... test/load/k6_pages.js
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  vus: 100, duration: '2m',
  thresholds: { http_req_duration: ['p(95)<500'], http_req_failed: ['rate<0.01'] },
};
const BASE = __ENV.BASE;
const pages = ['/customer/dashboard', '/catalog', '/offers', '/orders', '/cart', '/notifications'];

export function setup() {
  http.post(`${BASE}/auth/login`, { email: __ENV.EMAIL, password: __ENV.PASSWORD }, { redirects: 0 });
  return { cookies: http.cookieJar().cookiesForURL(BASE) };
}

export default function (data) {
  const jar = http.cookieJar();
  for (const [k, v] of Object.entries(data.cookies)) jar.set(BASE, k, v[0]);
  const res = http.get(`${BASE}${pages[Math.floor(Math.random() * pages.length)]}`);
  check(res, { ok: (r) => r.status === 200 });
}
