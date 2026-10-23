// Local browser fixtures only: node --test scripts/test-ui-stability.cjs.
const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { chromium } = require('playwright');

const staticRoot = path.join(__dirname, '..', 'internal', 'ui', 'static');
const assets = ['js/security.js', 'js/combobox.js', 'js/app.js', 'js/compare_tool.js', 'js/import-progress.js'];
const styles = ['css/tokens.css', 'css/base.css', 'css/components.css', 'css/foundations.css'];
let browser, server, origin;

before(async () => {
  server = http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    const asset = url.pathname.slice('/static/'.length);
    if (url.pathname.startsWith('/static/') && [...assets, ...styles].includes(asset)) {
      res.setHeader('Content-Type', asset.endsWith('.css') ? 'text/css' : 'application/javascript');
      res.end(fs.readFileSync(path.join(staticRoot, asset)));
      return;
    }
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(`<!doctype html><html lang="ar" dir="rtl"><head>
      ${styles.map(file => `<link rel="stylesheet" href="/static/${file}">`).join('')}
      ${assets.map(file => `<script defer src="/static/${file}"></script>`).join('')}
      </head><body><button id="drawer" data-drawer-toggle>Menu</button>
      <button id="collapse" data-sidebar-toggle>Collapse</button>
      <button id="theme" data-set-theme="dark">Dark</button>
      <aside id="app-sidebar"><nav class="sidebar-nav"><a class="sidebar-link" href="/fixture">Home</a></nav></aside>
      <main id="main-content"><input id="instant-search-input"><div id="instant-search-results"></div>
      <div id="half-step" class="m-0.5">Spacing</div></main></body></html>`);
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

async function fixture(options = {}) {
  const page = await browser.newPage(options);
  await page.goto(origin + '/fixture');
  return page;
}

test('blocked browser storage leaves drawer and theme controls usable', async () => {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  try {
    await page.addInitScript(() => {
      Storage.prototype.getItem = () => { throw new DOMException('Storage blocked', 'SecurityError'); };
      Storage.prototype.setItem = () => { throw new DOMException('Storage blocked', 'SecurityError'); };
    });
    await page.goto(origin + '/fixture');
    await page.locator('#drawer').click();
    assert.ok(await page.locator('#app-sidebar').evaluate(el => el.classList.contains('mobile-open')));
    await page.locator('#theme').click();
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
    assert.equal(await page.evaluate(() => getBrowserPreference('dawa24-theme')), 'dark');
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});

test('flash notices keep the HTMX history state and fragment', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(() => {
      history.replaceState({ htmx: true, marker: 'retained' }, '', '?notice=success&msg=saved#anchor');
      showNoticeFromQuery();
      return { state: history.state, hash: location.hash, search: location.search };
    });
    assert.deepEqual(snapshot, { state: { htmx: true, marker: 'retained' }, hash: '#anchor', search: '' });
  } finally { await page.close(); }
});

test('a rejected preference write keeps the new choice despite an old stored value', async () => {
  const page = await fixture();
  try {
    const preference = await page.evaluate(() => {
      localStorage.setItem('dawa24-theme', 'light');
      Storage.prototype.setItem = () => { throw new DOMException('Storage full', 'QuotaExceededError'); };
      applyTheme('dark', true);
      return getBrowserPreference('dawa24-theme');
    });
    assert.equal(preference, 'dark');
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
  } finally { await page.close(); }
});

test('combobox clear and dependency reset ignore completed old requests without AbortController', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(async () => {
      window.AbortController = undefined;
      const pending = [];
      window.fetch = () => new Promise(resolve => pending.push(resolve));
      const box = dawaCombobox({ name: 'city', url: '/cities?q={q}' });
      box.$refs = {};
      box.$dispatch = () => {};
      box.init();
      box.loading = true;
      box._fetch('old');
      box.dependencyChanged();
      pending.shift()({ ok: true, json: () => Promise.resolve([{ id: 'old', label: 'Old city' }]) });
      await new Promise(resolve => setTimeout(resolve, 0));
      return { results: box.results, loading: box.loading, selected: box.selected };
    });
    assert.deepEqual(snapshot, { results: [], loading: false, selected: null });
  } finally { await page.close(); }
});

test('choosing a combobox result cancels the old request and preserves the selection', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(async () => {
      let resolveSearch;
      window.fetch = () => new Promise(resolve => { resolveSearch = resolve; });
      const box = dawaCombobox({ url: '/products?q={q}' });
      box.$refs = {};
      box.$dispatch = () => {};
      box.loading = true;
      box._fetch('old');
      box.choose({ id: 'new', label: 'Selected product' });
      resolveSearch({ ok: true, json: () => Promise.resolve([{ id: 'old', label: 'Old result' }]) });
      await new Promise(resolve => setTimeout(resolve, 0));
      return { id: box.selected.id, query: box.query, loading: box.loading, results: box.results };
    });
    assert.deepEqual(snapshot, { id: 'new', query: 'Selected product', loading: false, results: [] });
  } finally { await page.close(); }
});

test('instant comparison search ignores late results after a newer query or clearing', async () => {
  const page = await fixture();
  try {
    await page.evaluate(() => {
      window.AbortController = undefined;
      window.pendingSearches = [];
      window.fetch = () => new Promise(resolve => pendingSearches.push(resolve));
      filterSearchLocal('old');
    });
    await page.waitForFunction(() => pendingSearches.length === 1);
    await page.evaluate(() => filterSearchLocal('new'));
    await page.waitForFunction(() => pendingSearches.length === 2);
    await page.evaluate(() => pendingSearches[1]({ ok: true, json: () => Promise.resolve({ items: [] }) }));
    await page.waitForFunction(() => document.getElementById('instant-search-results').textContent.includes('new'));
    await page.evaluate(() => pendingSearches[0]({ ok: true, json: () => Promise.resolve({ items: [] }) }));
    await page.waitForTimeout(30);
    assert.ok((await page.locator('#instant-search-results').textContent()).includes('new'));
    await page.evaluate(() => filterSearchLocal('another'));
    await page.waitForFunction(() => pendingSearches.length === 3);
    await page.evaluate(() => {
      filterSearchLocal('');
      pendingSearches[2]({ ok: true, json: () => Promise.resolve({ items: [] }) });
    });
    await page.waitForTimeout(30);
    assert.ok((await page.locator('#instant-search-results').textContent()).includes('اكتب اسم الصنف'));
  } finally { await page.close(); }
});

test('small-screen RTL toasts fit, announce errors and offer a touch-size close target', async () => {
  const page = await fixture({ viewport: { width: 320, height: 568 } });
  try {
    for (const theme of ['light', 'dark']) {
      const metrics = await page.evaluate(theme => {
        applyTheme(theme);
        document.getElementById('toast-container')?.remove();
        showToast('تعذّر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.', 'error');
        const toast = document.querySelector('.toast');
        const close = toast.querySelector('.toast-close');
        const rect = toast.getBoundingClientRect();
        return {
          left: rect.left, right: rect.right, viewport: innerWidth,
          closeWidth: close.getBoundingClientRect().width, closeHeight: close.getBoundingClientRect().height,
          label: close.getAttribute('aria-label'), role: toast.getAttribute('role'),
          margin: getComputedStyle(document.getElementById('half-step')).marginTop
        };
      }, theme);
      assert.ok(metrics.left >= 0 && metrics.right <= metrics.viewport);
      assert.ok(metrics.closeWidth >= 44 && metrics.closeHeight >= 44);
      assert.equal(metrics.label, 'إغلاق');
      assert.equal(metrics.role, 'alert');
      assert.equal(metrics.margin, '2px');
    }
  } finally { await page.close(); }
});

test('import polling retains snapshot callbacks after SSE construction or connection fails', async () => {
  const page = await fixture();
  try {
    for (const failure of ['construction', 'connection']) {
      const snapshot = await page.evaluate(async failure => {
        let source;
        window.EventSource = function () {
          if (failure === 'construction') throw new Error('EventSource unavailable');
          source = this;
          this.addEventListener = () => {};
          this.close = () => { this.closed = true; };
        };
        const headers = [];
        window.fetch = (url, options) => {
          headers.push(options.headers);
          return Promise.resolve({ ok: true, json: () => Promise.resolve({ percent: 42 }) });
        };
        const fill = document.createElement('div');
        document.body.append(fill);
        const received = [];
        const bar = new ImportProgress({ fill });
        bar.follow('/stream', '/progress', 1000, snapshot => received.push(snapshot.percent));
        if (source) source.onerror();
        await new Promise(resolve => setTimeout(resolve, 0));
        bar.stop();
        fill.remove();
        return { received, headers, closed: source ? source.closed : true };
      }, failure);
      assert.deepEqual(snapshot.received, [42]);
      assert.equal(snapshot.headers[0]['X-Dawa-Background'], '1');
      assert.equal(snapshot.closed, true);
    }
  } finally { await page.close(); }
});

test('a detached import bar aborts polling and ignores late completion callbacks', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(async () => {
      let resolvePoll, signal;
      window.fetch = (url, options) => {
        signal = options.signal;
        return new Promise(resolve => { resolvePoll = resolve; });
      };
      const fill = document.createElement('div');
      document.body.append(fill);
      let completed = 0, received = 0;
      const bar = new ImportProgress({ fill, onDone: () => { completed++; } });
      bar.onSnapshot = () => { received++; };
      bar.poll('/progress');
      fill.remove();
      resolvePoll({ ok: true, json: () => Promise.resolve({ done: true, percent: 100 }) });
      await new Promise(resolve => setTimeout(resolve, 0));
      return { completed, received, stopped: bar.stopped, timer: bar.timer, aborted: signal.aborted };
    });
    assert.deepEqual(snapshot, { completed: 0, received: 0, stopped: true, timer: null, aborted: true });
  } finally { await page.close(); }
});

test('detaching an import bar closes its stream and drift timer', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(async () => {
      let source;
      window.EventSource = function () {
        source = this;
        this.addEventListener = () => {};
        this.close = () => { this.closed = true; };
      };
      const fill = document.createElement('div');
      document.body.append(fill);
      const bar = new ImportProgress({ fill });
      bar.follow('/stream', '/progress');
      fill.remove();
      await new Promise(resolve => setTimeout(resolve, 150));
      return { closed: source.closed, timer: bar.timer, stopped: bar.stopped };
    });
    assert.deepEqual(snapshot, { closed: true, timer: null, stopped: true });
  } finally { await page.close(); }
});

test('direct import polling works without DOM elements or an EventSource', async () => {
  const page = await fixture();
  try {
    const target = await page.evaluate(async () => {
      window.fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve({ percent: 37 }) });
      const bar = new ImportProgress({});
      bar.poll('/progress');
      await new Promise(resolve => setTimeout(resolve, 0));
      bar.stop();
      return bar.target;
    });
    assert.equal(target, 37);
  } finally { await page.close(); }
});


test('the real smart-order ring bootstraps, updates and reloads automatically at completion', async () => {
  const page = await fixture();
  try {
    const template = fs.readFileSync(path.join(__dirname, '..', 'internal/ui/pages/smart_order_steps.templ'), 'utf8');
    const source = [...template.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)]
      .map(match => match[1]).find(script => script.includes("querySelector('.so-progress-card')"));
    assert.ok(source, 'test must run the real progress initializer');
    await page.evaluate(source => {
      document.querySelector('main').innerHTML = '<div class="so-progress-card"><div class="so-ring" role="progressbar" aria-valuenow="46"><svg><circle class="so-ring-value"></circle></svg><span class="so-ring-percent">46%</span></div><p class="so-progress-caption"></p></div>';
      window.EventSource = function () {
        window.progressStream = this;
        this.listeners = {};
        this.addEventListener = (name, callback) => { this.listeners[name] = callback; };
        this.close = () => {};
      };
      (0, eval)(source);
      if (document.querySelector('.so-ring-percent').textContent !== '46%') throw new Error('progress rewound during initialization');
      window.progressStream.listeners.progress({ data: JSON.stringify({ percent: 60, message: 'Matching' }) });
    }, source);
    await page.waitForFunction(() => Number(document.querySelector('.so-ring-percent').textContent.replace('%', '')) >= 59);
    await Promise.all([
      page.waitForEvent('framenavigated'),
      page.evaluate(() => window.progressStream.listeners.progress({ data: JSON.stringify({ percent: 100, done: true }) }))
    ]);
    assert.ok(page.url().endsWith('/fixture'));
  } finally { await page.close(); }
});

test('a stream that breaks or freezes after its first snapshot falls back and completes once', async () => {
  const page = await fixture();
  try {
    for (const failure of ['error', 'frozen']) {
      const snapshot = await page.evaluate(async failure => {
        const nativeTimeout = window.setTimeout;
        window.setTimeout = (fn, ms) => nativeTimeout(fn, ms === 30000 ? 20 : ms);
        let source, completed = 0, polls = 0;
        window.EventSource = function () {
          source = this; this.listeners = {};
          this.addEventListener = (name, fn) => { this.listeners[name] = fn; };
          this.close = () => { this.closed = true; };
        };
        window.fetch = () => { polls++; return Promise.resolve({ ok: true, json: async () => ({ percent: 100, done: true }) }); };
        const fill = document.createElement('div'); document.body.append(fill);
        const bar = new ImportProgress({ fill, onDone() { completed++; } });
        bar.follow('/stream', '/progress');
        source.listeners.progress({ data: JSON.stringify({ percent: 20 }) });
        if (failure === 'error') source.onerror();
        await new Promise(resolve => nativeTimeout(resolve, 60));
        window.setTimeout = nativeTimeout;
        bar.stop(); fill.remove();
        return { completed, polls, closed: source.closed };
      }, failure);
      assert.deepEqual(snapshot, { completed: 1, polls: 1, closed: true });
    }
  } finally { await page.close(); }
});

test('import polling recovers automatically after more than eight network failures', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(async () => {
      const nativeTimeout = window.setTimeout;
      window.setTimeout = (fn, ms) => nativeTimeout(fn, Math.min(ms, 2));
      let attempts = 0, completed = 0;
      window.fetch = () => {
        attempts++;
        return attempts <= 9 ? Promise.reject(new Error('offline')) : Promise.resolve({ ok: true, json: async () => ({ done: true }) });
      };
      const bar = new ImportProgress({ onDone() { completed++; } });
      bar.poll('/progress');
      await new Promise(resolve => nativeTimeout(resolve, 150));
      window.setTimeout = nativeTimeout;
      bar.stop();
      return { completed, attempts };
    });
    assert.deepEqual(snapshot, { completed: 1, attempts: 10 });
  } finally { await page.close(); }
});

test('failed stream snapshots preserve progress and never display successful completion', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(() => {
      const percent = document.createElement('span'); document.body.append(percent);
      const bar = new ImportProgress({ percent });
      bar.update({ percent: 60, state: 'failed', done: true, error: 'Import failed' });
      const snapshot = { done: bar.done, label: percent.textContent, stopped: bar.stopped };
      bar.stop(); percent.remove();
      return snapshot;
    });
    assert.deepEqual(snapshot, { done: false, label: '0%', stopped: true });
  } finally { await page.close(); }
});


test('a hung progress request times out and retries without refreshing', async () => {
  const page = await fixture();
  try {
    const snapshot = await page.evaluate(async () => {
      const nativeTimeout = window.setTimeout;
      window.setTimeout = (fn, ms) => nativeTimeout(fn, Math.min(ms, 2));
      let attempts = 0, completed = 0, firstSignal;
      window.fetch = (url, options) => {
        attempts++;
        if (attempts === 1) { firstSignal = options.signal; return new Promise(() => {}); }
        return Promise.resolve({ ok: true, json: async () => ({ done: true }) });
      };
      const bar = new ImportProgress({ onDone() { completed++; } });
      bar.poll('/progress');
      await new Promise(resolve => nativeTimeout(resolve, 100));
      bar.stop(); window.setTimeout = nativeTimeout;
      return { attempts, completed, aborted: firstSignal.aborted };
    });
    assert.deepEqual(snapshot, { attempts: 2, completed: 1, aborted: true });
  } finally { await page.close(); }
});
