// خصومات السوق العامة — browser regression for the rebuilt board.
//
// Runs against the real templates, scripts and CSS served by the pagespeed
// fixture (no database):
//
//   go run ./scripts/pagespeed                       # serves 127.0.0.1:8097
//   node --test scripts/test-market-discounts.cjs    # Playwright on NODE_PATH
//
// PAGESPEED_ORIGIN selects another origin; PLAYWRIGHT_CHROMIUM_EXECUTABLE a
// browser. The fixture board has three offers, or seventy with ?fixture=many.
const { test, before, after, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

function loadPlaywright() {
  try { return require('playwright'); } catch (_) {
    return require(path.join(__dirname, 'pagespeed', 'node_modules', 'playwright'));
  }
}
const { chromium } = loadPlaywright();

const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';
let browser, page, fragmentRequests, suggestRequests, pageErrors;

before(async () => {
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || [
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  ].find(p => fs.existsSync(p));
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
});
after(async () => { await browser?.close(); });

beforeEach(async () => {
  page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  fragmentRequests = [];
  suggestRequests = [];
  pageErrors = [];
  page.on('pageerror', e => pageErrors.push(e.message));
  page.on('request', req => {
    const url = new URL(req.url());
    if (url.pathname === '/market-discounts/suggest') suggestRequests.push(url.search);
    else if (url.pathname === '/market-discounts' && req.headers()['hx-target'] === 'md-results') fragmentRequests.push(url.search);
  });
});
afterEach(async () => {
  assert.deepEqual(pageErrors, [], 'no uncaught page errors');
  await page.close();
});

// Marks the controls, the results region and the whole document, so a test can
// tell a results swap from a re-render of the shell or a full navigation.
async function mark() {
  await page.evaluate(() => {
    window.__documentMarker = 'same-document';
    window.__formNode = document.getElementById('market-filter-form');
    window.__resultsNode = document.getElementById('md-results');
    window.__headerNode = document.querySelector('.md-header');
  });
}
async function untouched() {
  return page.evaluate(() => ({
    document: window.__documentMarker === 'same-document',
    form: window.__formNode === document.getElementById('market-filter-form') && document.contains(window.__formNode),
    results: window.__resultsNode === document.getElementById('md-results'),
    header: window.__headerNode === document.querySelector('.md-header') && document.contains(window.__headerNode),
  }));
}
const search = () => page.locator('#market-search-input');
const options = () => page.locator('#md-suggest-list [role="option"]');
const settle = () => page.waitForFunction(() => document.getElementById('md-results').getAttribute('aria-busy') === 'false');

test('typing shows suggestions and never touches the board', async () => {
  await page.goto(origin + '/market-discounts');
  await mark();
  const before = await page.locator('#md-results').innerHTML();
  await search().click();
  await search().pressSequentially('pana', { delay: 40 });
  await options().first().waitFor();

  assert.equal(await search().getAttribute('aria-expanded'), 'true');
  assert.equal(await search().getAttribute('role'), 'combobox');
  assert.equal(await search().getAttribute('aria-controls'), 'md-suggest-list');
  assert.equal(await page.locator('#md-suggest-list').getAttribute('role'), 'listbox');
  assert.match(await options().first().textContent(), /pana/, 'the first line is "view all results for the typed text"');
  assert.equal(await options().first().getAttribute('data-md-kind'), 'all');
  assert.ok(await options().count() >= 2);

  // Debounced: four keystrokes make one request, not four.
  assert.equal(suggestRequests.length, 1, 'one debounced suggestion request: ' + JSON.stringify(suggestRequests));
  assert.equal(fragmentRequests.length, 0, 'typing must not request the board');
  assert.equal(await page.locator('#md-results').innerHTML(), before, 'the results are untouched');
  assert.deepEqual(await untouched(), { document: true, form: true, results: true, header: true });
  assert.equal(new URL(page.url()).search, '', 'typing does not change the URL');
});

test('suggestions below two characters are not requested', async () => {
  await page.goto(origin + '/market-discounts');
  await search().click();
  await search().pressSequentially('p', { delay: 40 });
  await page.waitForTimeout(500);
  assert.equal(suggestRequests.length, 0);
  assert.equal(await search().getAttribute('aria-expanded'), 'false');
});

test('keyboard: arrows move, Home and End jump, Escape closes, Enter picks', async () => {
  await page.goto(origin + '/market-discounts');
  await search().click();
  await search().fill('pana');
  await options().first().waitFor();
  const count = await options().count();

  await search().press('ArrowDown');
  assert.equal(await search().getAttribute('aria-activedescendant'), 'md-opt-0');
  assert.equal(await page.locator('#md-opt-0').getAttribute('aria-selected'), 'true');
  await search().press('ArrowDown');
  assert.equal(await search().getAttribute('aria-activedescendant'), 'md-opt-1');
  await search().press('End');
  assert.equal(await search().getAttribute('aria-activedescendant'), 'md-opt-' + (count - 1));
  await search().press('Home');
  assert.equal(await search().getAttribute('aria-activedescendant'), 'md-opt-0');
  await search().press('ArrowUp');
  assert.equal(await search().getAttribute('aria-activedescendant'), 'md-opt-' + (count - 1), 'ArrowUp wraps');

  await search().press('Escape');
  assert.equal(await search().getAttribute('aria-expanded'), 'false');
  assert.equal(await search().getAttribute('aria-activedescendant'), null);
  assert.equal(fragmentRequests.length, 0, 'Escape does not commit');

  await search().press('ArrowDown');
  assert.equal(await search().getAttribute('aria-expanded'), 'true', 'ArrowDown reopens the list');
  await search().press('ArrowDown');
  await search().press('Enter');
  await page.waitForFunction(() => location.search.includes('product=7'));
  assert.equal(fragmentRequests.length, 1);
});

test('Enter commits the typed text: only #md-results is swapped, the URL follows', async () => {
  await page.goto(origin + '/market-discounts?lang=en');
  await mark();
  await search().click();
  await search().fill('Panadol');
  await search().press('Enter');
  await page.waitForFunction(() => location.search.includes('q=Panadol'));
  await settle();

  assert.equal(fragmentRequests.length, 1);
  assert.match(fragmentRequests[0], /q=Panadol/);
  assert.deepEqual(await untouched(), { document: true, form: true, results: true, header: true }, 'only the results contents changed');
  assert.match(await page.locator('#md-results .md-chips').textContent(), /Panadol/, 'the query shows as a chip');
  assert.equal(await page.locator('#md-results article').count(), 2);
  assert.equal(await search().inputValue(), 'Panadol', 'the box keeps what was typed');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'market-search-input', 'focus stays in the box');
});

test('picking a product suggestion filters the board to that product; its chip removes it', async () => {
  await page.goto(origin + '/market-discounts?lang=en');
  await mark();
  await search().click();
  await search().fill('pana');
  await page.locator('#md-suggest-list [data-md-kind="product"]').first().waitFor();
  await page.locator('#md-suggest-list [data-md-kind="product"]').first().click();
  await page.waitForFunction(() => location.search.includes('product=7'));
  await settle();

  assert.equal(new URL(page.url()).searchParams.get('q'), null, 'a picked product replaces the typed text');
  assert.equal(await page.locator('#md-results article').count(), 2, 'every offer for the product');
  assert.match(await page.locator('#md-results .md-chip').first().textContent(), /Panadol 500 mg tablets/);
  assert.deepEqual(await untouched(), { document: true, form: true, results: true, header: true });

  await page.locator('#md-results .md-chip').first().click();
  await page.waitForFunction(() => !location.search.includes('product='));
  await settle();
  assert.equal(await page.locator('#md-results article').count(), 3);
  assert.equal(await page.locator('#md-results .md-chip').count(), 0);
  assert.deepEqual(await untouched(), { document: true, form: true, results: true, header: true });
});

test('a stale suggestion response is ignored', async () => {
  await page.goto(origin + '/market-discounts');
  await search().click();
  await search().fill('slowest');
  await page.waitForTimeout(260);           // the slow request is now in flight (600 ms)
  await search().fill('panadol');
  await options().first().waitFor();
  await page.waitForTimeout(900);           // long enough for the slow one to have answered

  assert.equal(suggestRequests.length, 2);
  assert.match(await options().first().textContent(), /panadol/, 'the later query wins');
  assert.doesNotMatch(await page.locator('#md-suggest-list').textContent(), /slowest/);
});

test('a stale results response is ignored', async () => {
  await page.goto(origin + '/market-discounts?fixture=many');
  await mark();
  // Request A is slow, request B is fast and newer: B must be what stays.
  await page.evaluate(() => {
    const f = document.getElementById('market-filter-form');
    f.elements.sort.value = 'price_asc';
  });
  await page.evaluate(() => { window.__slow = fetch; });
  await page.route('**/market-discounts?*', async route => {
    const url = route.request().url();
    if (route.request().headers()['hx-target'] === 'md-results' && url.includes('q=slowone')) await new Promise(r => setTimeout(r, 700));
    await route.continue();
  });
  await search().fill('slowone');
  await search().press('Enter');
  await page.waitForTimeout(150);
  await search().fill('صنف رقم 5');
  await search().press('Enter');
  await page.waitForFunction(() => new URLSearchParams(location.search).get('q') === 'صنف رقم 5');
  await page.waitForTimeout(1000);
  assert.equal(new URL(page.url()).searchParams.get('q'), 'صنف رقم 5', 'the newer search is the one in the URL after the slow one answered');
  assert.match(await page.locator('#md-results article').first().textContent(), /صنف رقم 5/);
  assert.equal(await page.locator('#md-results').getAttribute('aria-busy'), 'false');
});

test('sort, warehouse, view and page size change only the results; no autosubmit on ranges', async () => {
  await page.goto(origin + '/market-discounts?fixture=many&lang=en');
  await mark();

  await page.locator('select[name="sort"]').selectOption('price_asc');
  await page.waitForFunction(() => location.search.includes('sort=price_asc'));
  await settle();
  assert.equal(fragmentRequests.length, 1);

  // Advanced panel: typing in a range does not commit; Apply (or Enter) does.
  await page.locator('[data-md-advanced-toggle]').click();
  assert.equal(await page.locator('#md-advanced').isVisible(), true);
  await page.locator('input[name="min_discount"]').fill('30');
  await page.locator('input[name="date_from"]').fill('2026-01-01');
  await page.waitForTimeout(500);
  assert.equal(fragmentRequests.length, 1, 'ranges and dates must not auto-submit');
  await page.locator('input[name="min_discount"]').press('Enter');
  await page.waitForFunction(() => location.search.includes('min_discount=30'));
  await settle();
  assert.equal(fragmentRequests.length, 2);
  assert.match(await page.locator('#md-results .md-chips').textContent(), /30%/);

  // Layout switch: a class, no request.
  const requests = fragmentRequests.length;
  await page.locator('[data-md-view="grid"]').click();
  assert.equal(await page.locator('#md-results .md-offers').evaluate(el => el.classList.contains('is-grid')), true);
  assert.equal(await page.locator('[data-md-view="grid"]').getAttribute('aria-pressed'), 'true');
  assert.equal(await page.evaluate(() => localStorage.getItem('d24_market_view')), 'grid');
  assert.match(page.url(), /view=grid/);
  assert.equal(fragmentRequests.length, requests, 'switching layout makes no request');
  assert.deepEqual(await untouched(), { document: true, form: true, results: true, header: true });

  // The layout survives the next swap.
  await page.locator('select[name="sort"]').selectOption('discount_desc');
  await settle();
  await page.waitForFunction(() => location.search.includes('sort=discount_desc'));
  assert.equal(await page.locator('#md-results .md-offers').evaluate(el => el.classList.contains('is-grid')), true);
});

test('list and grid are the same markup', async () => {
  await page.goto(origin + '/market-discounts?fixture=many');
  const articles = await page.locator('#md-results article').count();
  assert.equal(articles, 24);
  assert.equal(await page.locator('#md-results .md-offers').count(), 1, 'one container, not a list and a grid');
  const html = await page.locator('#md-results').innerHTML();
  assert.doesNotMatch(html, /x-show/);
});

test('the pager swaps in place, prefetches on hover, and back restores the page', async () => {
  await page.goto(origin + '/market-discounts?fixture=many&lang=en');
  await mark();
  const stat = () => page.locator('.md-stat.is-accent dd').textContent();
  const highest = await stat();
  assert.match(highest, /59%/, 'the highest discount of the whole board, not of page 1');

  const next = page.locator('.b2b-pagination-controls a', { hasText: '2' }).first();
  await next.hover();
  await page.waitForFunction(() => performance.getEntriesByType('resource').some(e => e.name.includes('page=2')));
  assert.ok(fragmentRequests.some(s => s.includes('page=2')), 'page 2 was prefetched on hover');
  const prefetched = fragmentRequests.length;

  await next.click();
  await page.waitForFunction(() => location.search.includes('page=2'));
  await settle();
  assert.equal(fragmentRequests.length, prefetched, 'the click used the prefetched fragment');
  assert.match(await page.locator('#md-results article').first().textContent(), /صنف رقم 2[5-9]|صنف رقم 3/);
  assert.equal(await stat(), highest, 'the highest discount does not change with the page');
  assert.deepEqual(await untouched(), { document: true, form: true, results: true, header: true });
  assert.equal(await page.evaluate(() => document.activeElement.id), 'md-results-heading', 'focus moves to the results heading');

  await page.goBack();
  await page.waitForFunction(() => !location.search.includes('page=2'));
  await settle();
  assert.match(await page.locator('#md-results article').first().textContent(), /بانادول/);
  assert.equal(await page.locator('#md-results .b2b-pagination-btn.active').textContent(), '1');
});

test('the pager keeps working with the keyboard', async () => {
  await page.goto(origin + '/market-discounts?fixture=many');
  const link = page.locator('.b2b-pagination-controls a', { hasText: '3' }).first();
  await link.focus();
  await link.press('Enter');
  await page.waitForFunction(() => location.search.includes('page=3'));
  await settle();
  assert.equal(await page.evaluate(() => document.activeElement.id), 'md-results-heading');
});

test('a failed refresh keeps the old results and says so', async () => {
  await page.goto(origin + '/market-discounts?fixture=many');
  await page.route('**/market-discounts?*', route => route.request().headers()['hx-target'] === 'md-results'
    ? route.fulfill({ status: 503, body: 'down' }) : route.continue());
  const before = await page.locator('#md-results article').count();
  await page.locator('select[name="sort"]').selectOption('price_asc');
  await page.locator('#md-results .md-error').waitFor();
  assert.equal(await page.locator('#md-results .md-error').getAttribute('role'), 'alert');
  assert.equal(await page.locator('#md-results article').count(), before);
  assert.equal(await page.locator('#md-results').getAttribute('aria-busy'), 'false');
});

test('no horizontal overflow on a phone, in both layouts and directions', async () => {
  for (const lang of ['ar', 'en']) for (const view of ['list', 'grid']) {
    await page.setViewportSize({ width: 390, height: 800 });
    await page.goto(`${origin}/market-discounts?fixture=many&lang=${lang}&view=${view}`);
    await search().fill('pana');
    await options().first().waitFor();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
    assert.equal(overflow, false, `${lang} ${view}: page fits the viewport`);
  }
});
