// Run with node --test scripts/test-session-search.cjs and Playwright on NODE_PATH.
const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');

const staticRoot = path.join(__dirname, '..', 'internal', 'ui', 'static');
let browser, server, origin;
const searchRequests = [];
const sessionRequests = [];
let sessionStatus = 200;

function escapeHTML(text) {
  return text.replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
}

function pageHTML(url) {
  const query = escapeHTML(url.searchParams.get('q') || '');
  const sessionMeta = url.pathname === '/session-test'
    ? `<meta name="session-instance" content="user-session-1"><meta name="session-last-active" content="${Date.now()}"><meta name="session-idle-timeout-mins" content="1">` : '';
  const scripts = ['js/security.js', 'vendor/htmx-1.9.10.min.js', 'js/boost-nav.js',
    'js/actions.js', 'js/components.js', 'vendor/alpine-csp-3.17.3.min.js', 'js/app.js', 'js/session-activity.js'];
  return `<!doctype html><html lang="ar" dir="rtl"><head><title>Search test</title>${sessionMeta}
    ${scripts.map(script => `<script defer nonce="fixture" src="/static/${script}"></script>`).join('')}
    </head><body><div hx-boost="true" hx-target="#main-content" hx-swap="innerHTML" hx-push-url="true">
    <aside id="sidebar">Fixed sidebar</aside><main id="main-content" data-shell="vendor">
    <form id="market-filter-form" method="GET" action="${url.pathname}" data-filter-form>
    <input id="market-search-input" type="search" name="q" value="${query}" data-min-chars="3">
    <select name="supplier" data-autosubmit><option value="">All</option><option value="one" ${url.searchParams.get('supplier') === 'one' ? 'selected' : ''}>One</option></select>
    <button type="submit">Search</button></form><div id="results">${query}</div>
    <a id="clear-filters" href="${url.pathname}">Clear</a>
    <a id="other-page" href="/other-page">Other page</a>
    <script nonce="fixture">window.fragmentInitCount = (window.fragmentInitCount || 0) + 1;</script>
    </main></div></body></html>`;
}

before(async () => {
  server = http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    if (url.pathname.startsWith('/static/')) {
      res.setHeader('Content-Type', 'application/javascript');
      res.end(fs.readFileSync(path.join(staticRoot, url.pathname.slice(8))));
      return;
    }
    if (url.pathname === '/api/v1/auth/me') {
      sessionRequests.push(req.headers['x-dawa-background'] || 'active');
      res.writeHead(sessionStatus, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: { code: 'session.idle_timeout' } }));
      return;
    }
    if (url.pathname === '/auth/login') {
      res.end('Sign in');
      return;
    }
    if (req.headers['hx-request']) searchRequests.push(url.search);
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.setHeader('Content-Security-Policy', "script-src 'self' 'nonce-fixture'; script-src-attr 'none'; require-trusted-types-for 'script'");
    setTimeout(() => {
      if (url.searchParams.get('q') === 'outage') {
        res.writeHead(503);
        res.end('Temporarily unavailable');
      } else res.end(pageHTML(url));
    }, req.headers['hx-request'] ? (url.searchParams.get('q') === 'slow-page' ? 900 : 250) : 0);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || [
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
  ].find(candidate => fs.existsSync(candidate));
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
});

after(async () => {
  if (browser) await browser.close();
  if (server) await new Promise(resolve => server.close(resolve));
});

async function searchPage() {
  searchRequests.length = 0;
  const page = await browser.newPage();
  await page.goto(origin + '/market-discounts');
  await page.evaluate(() => {
    window.originalInput = document.getElementById('market-search-input');
    window.originalForm = document.getElementById('market-filter-form');
  });
  return page;
}

test('Arabic search keeps the same input, focus and caret after updates', async () => {
  const page = await searchPage();
  try {
    const input = page.locator('#market-search-input');
    await input.fill('بانادول');
    await page.waitForFunction(() => document.getElementById('results').textContent === 'بانادول');
    assert.equal(await page.evaluate(() => window.originalInput === document.activeElement), true);
    await input.press('End');
    await input.pressSequentially(' اكسترا', { delay: 20 });
    await page.waitForFunction(() => document.getElementById('results').textContent === 'بانادول اكسترا');
    assert.equal(await input.inputValue(), 'بانادول اكسترا');
    assert.equal(searchRequests.length, 2);
    assert.equal(await page.evaluate(() => document.activeElement.selectionStart), 'بانادول اكسترا'.length);
    await page.locator('#clear-filters').click();
    await page.waitForFunction(() => document.getElementById('market-search-input').value === '');
  } finally { await page.close(); }
});

test('slow search responses cannot overwrite newer input', async () => {
  const page = await searchPage();
  try {
    const input = page.locator('#market-search-input');
    await input.fill('first');
    await page.waitForRequest(req => req.url().includes('q=first'));
    await input.fill('latest');
    await page.waitForFunction(() => document.getElementById('results').textContent === 'latest');
    assert.equal(await input.inputValue(), 'latest');
    assert.equal(await page.evaluate(() => window.originalInput === document.activeElement), true);
    assert.equal(new URL(page.url()).searchParams.get('q'), 'latest');
  } finally { await page.close(); }
});

test('composition waits until committed and a select makes only one request', async () => {
  const page = await searchPage();
  try {
    await page.locator('#market-search-input').evaluate(input => {
      input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
      input.value = 'بانادول';
      input.dispatchEvent(new InputEvent('input', { bubbles: true, isComposing: true }));
    });
    await page.waitForTimeout(550);
    assert.equal(searchRequests.length, 0);
    await page.locator('#market-search-input').dispatchEvent('compositionend');
    await page.waitForFunction(() => document.getElementById('results').textContent === 'بانادول');
    const beforeSelect = searchRequests.length;
    await page.locator('select[name=supplier]').selectOption('one');
    await page.waitForFunction(() => new URL(location.href).searchParams.get('supplier') === 'one');
    await page.waitForTimeout(300);
    assert.equal(searchRequests.length - beforeSelect, 1);
  } finally { await page.close(); }
});

test('a server outage leaves the current screen and input usable', async () => {
  const page = await searchPage();
  try {
    await page.locator('#market-search-input').fill('outage');
    await page.waitForResponse(res => res.status() === 503);
    await page.waitForTimeout(300);
    assert.equal(await page.evaluate(() => window.originalInput === document.getElementById('market-search-input')), true);
    assert.equal(searchRequests.length, 1);
    await page.locator('#market-search-input').fill('recovered');
    await page.waitForFunction(() => document.getElementById('results').textContent === 'recovered');
  } finally { await page.close(); }
});

test('HttpOnly sessions track real activity even with blocked localStorage', async () => {
  sessionRequests.length = 0;
  sessionStatus = 200;
  const page = await browser.newPage();
  try {
    await page.context().addCookies([{ name: 'dawa24_session', value: 'test-session', url: origin, httpOnly: true }]);
    await page.addInitScript(() => {
      Storage.prototype.getItem = Storage.prototype.setItem = function () { throw new DOMException('Blocked', 'SecurityError'); };
    });
    await page.clock.install();
    await page.goto(origin + '/session-test');
    assert.equal(await page.evaluate(() => document.cookie.includes('dawa24_session')), false);
    const activityResponse = page.waitForResponse(res => res.url().endsWith('/api/v1/auth/me'));
    await page.locator('#market-search-input').click();
    await activityResponse;
    assert.deepEqual(sessionRequests, ['active']);
    sessionStatus = 503;
    const outageResponse = page.waitForResponse(res => res.url().endsWith('/api/v1/auth/me') && res.status() === 503);
    await page.clock.fastForward(65000);
    await outageResponse;
    assert.equal(page.url(), origin + '/session-test');
    sessionStatus = 401;
    await page.clock.fastForward(31000);
    await page.waitForURL('**/auth/login?**');
    assert.equal(new URL(page.url()).searchParams.get('reason'), 'idle_timeout');
    assert.ok(sessionRequests.includes('1'), 'idle checks must not renew the session');
  } finally { await page.close(); }
});

test('a late filter response cannot take over a different page', async () => {
  const page = await searchPage();
  try {
    await page.locator('#market-search-input').fill('slow-page');
    await page.waitForRequest(req => req.url().includes('q=slow-page'));
    await page.locator('#other-page').click();
    await page.waitForURL('**/other-page');
    await page.waitForTimeout(1000);
    assert.equal(new URL(page.url()).pathname, '/other-page');
    assert.equal(await page.locator('#market-search-input').inputValue(), '');
  } finally { await page.close(); }
});

test('tabs share real activity and programmatic scroll does not renew login', async () => {
  sessionRequests.length = 0;
  sessionStatus = 200;
  const context = await browser.newContext();
  try {
    const first = await context.newPage();
    const second = await context.newPage();
    const now = Date.now();
    await first.clock.install({ time: now });
    await second.clock.install({ time: now });
    await first.goto(origin + '/session-test');
    await second.goto(origin + '/session-test');
    await first.bringToFront();
    await first.evaluate(() => {
      document.body.style.height = '3000px';
      window.scrollTo(0, 500);
    });
    await first.clock.fastForward(45000);
    assert.equal(sessionRequests.length, 0);
    const activityResponse = first.waitForResponse(res => res.url().endsWith('/api/v1/auth/me'));
    await first.locator('#market-search-input').click();
    await activityResponse;
    await second.bringToFront();
    // The context clock already advanced 45 seconds for both tabs.
    await second.clock.fastForward(20000);
    await second.waitForTimeout(50);
    assert.deepEqual(sessionRequests, ['active'], 'another tab\'s activity must postpone idle checks');
  } finally { await context.close(); }
});
