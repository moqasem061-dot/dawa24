const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';
let browser;

before(async () => {
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || [
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
  ].find(filename => fs.existsSync(filename));
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
});
after(async () => { if (browser) await browser.close(); });

async function heroGeometry(page) {
  await page.evaluate(() => document.fonts.ready);
  return page.locator('.nav-root, .landing-hero-text-col, .landing-hero-title-main, .hero-cta-group').evaluateAll(elements =>
    elements.map(element => {
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height, color: style.color, display: style.display };
    }));
}

for (const width of [390, 1440]) {
  for (const theme of ['light', 'dark']) {
    for (const lang of ['ar', 'en']) {
      test(`critical CSS preserves ${width}px ${theme} ${lang} geometry and menu`, async () => {
        const context = await browser.newContext({ viewport: { width, height: 844 } });
        try {
          const complete = await context.newPage();
          const errors = [];
          complete.on('pageerror', error => errors.push(error.message));
          await complete.goto(`${origin}/?theme=${theme}&lang=${lang}`);
          const expected = await heroGeometry(complete);
          const critical = await context.newPage();
          await critical.route('**/static/css/*.css*', route => route.abort());
          critical.on('pageerror', error => errors.push(error.message));
          await critical.goto(`${origin}/?theme=${theme}&lang=${lang}`);
          const actual = await heroGeometry(critical);
          assert.deepEqual(actual, expected);
          const title = await critical.locator('h1').boundingBox();
          assert.ok(title.x >= 0 && title.x + title.width <= width, 'headline must fit the viewport');
          assert.equal(await critical.evaluate(() => document.documentElement.scrollWidth), width);
          if (width < 640) {
            await critical.locator('.nav-mobile-only').first().click();
            assert.ok(await critical.locator('.nav-drawer').isVisible());
          }
          assert.deepEqual(errors, []);
        } finally { await context.close(); }
      });
    }
  }
}

test('complete styles apply and public navigation still works', async () => {
  const page = await browser.newPage();
  try {
    await page.goto(origin);
    assert.equal(await page.locator('link[data-deferred-css]').evaluateAll(links => links.every(link => link.media === 'all')), true);
    await page.locator('.nav-links a[href="/about"]').click();
    await page.waitForURL('**/about');
    assert.ok(await page.locator('h1').isVisible());
    await page.goto(origin + '/auth/login');
    assert.equal(await page.locator('#home-critical').count(), 0);
    assert.ok(await page.locator('input[type="password"]').isVisible());
  } finally { await page.close(); }
});

test('homepage remains styled without JavaScript', async () => {
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 390, height: 844 } });
  try {
    const page = await context.newPage();
    await page.goto(origin);
    assert.ok(await page.locator('h1').isVisible());
    assert.equal(await page.locator('.landing-hero-split-grid').evaluate(element => getComputedStyle(element).display), 'grid');
  } finally { await context.close(); }
});
