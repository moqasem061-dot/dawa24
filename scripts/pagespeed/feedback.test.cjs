const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';
const executablePath = ['C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(fs.existsSync);

test('stock alert dialog identifies the exact warehouse and submits its stock ID', async () => {
  const browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
  try {
    for (const width of [390, 768, 1440]) {
      const page = await browser.newPage({ viewport: { width, height: 800 } });
      const errors = [];
      page.on('pageerror', error => errors.push(error.message));
      await page.route('**/notifications/unread-badge', route => route.fulfill({ status: 200, body: '' }));
      await page.goto(origin + '/vendor/inventory/alerts');
      await page.locator('tbody tr').nth(1).getByRole('button', { name: 'تعديل / تعبئة', exact: true }).click();
      const dialog = page.getByRole('dialog').filter({ has: page.locator('input[name="delta"]') });
      await dialog.waitFor({ state: 'visible' });
      assert.match(await dialog.textContent(), /مخزن الجيزة/);
      assert.match(await dialog.textContent(), /بانادول/);
      await dialog.locator('input[name="delta"]').fill('15');
      const form = dialog.locator('form');
      assert.match(await form.getAttribute('action'), /\/102\/adjust$/);
      const submit = dialog.locator('button[type="submit"]');
      const bounds = await submit.boundingBox();
      assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= width && bounds.y + bounds.height <= 800);
      let posted;
      await page.route('**/vendor/inventory/102/adjust', async route => {
        posted = route.request().postData();
        await route.fulfill({ status: 204 });
      });
      const submission = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/vendor/inventory/102/adjust'));
      await submit.click();
      await submission;
      assert.equal(new URLSearchParams(posted).get('delta'), '15');
      assert.deepEqual(errors, []);
      await page.close();
    }
  } finally { await browser.close(); }
});

test('branch maps expose tile failures and recover when retried', async () => {
  const browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let offline = true;
    await page.route('https://tile.openstreetmap.org/**', async route => {
      if (offline) return route.abort();
      await route.fulfill({ contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a3ioAAAAASUVORK5CYII=', 'base64') });
    });
    await page.goto(origin + '/vendor/branches');
    await page.locator('.map-load-status').waitFor({ state: 'visible' });
    assert.ok(await page.locator('.leaflet-tile').evaluateAll(tiles => tiles.every(tile => getComputedStyle(tile).display !== 'none')));
    offline = false;
    await page.locator('.map-load-status button').click();
    await page.waitForFunction(() => [...document.querySelectorAll('.leaflet-tile')].some(tile => tile.naturalWidth > 0));
    await page.locator('.map-load-status').waitFor({ state: 'hidden' });
    await page.evaluate(() => { initMapPickers(); initMapPickers(); });
    assert.equal(await page.locator('#vendor-branch-map-picker .leaflet-map-pane').count(), 1);
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
});
