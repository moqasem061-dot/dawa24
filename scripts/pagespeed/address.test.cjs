const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';

test('address lookup follows pins, preserves in-flight edits and ignores stale responses', async () => {
  const executablePath = ['C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(fs.existsSync);
  const browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto(origin + '/product');
    await page.addScriptTag({ url: origin + '/static/js/registration.js' });
    await page.evaluate(() => {
      const form = document.createElement('form');
      const address = document.createElement('input');
      address.id = 'reg-address'; address.name = 'address';
      const hint = document.createElement('span'); hint.id = 'reg-address-hint';
      form.append(address, hint); document.body.append(form);
      window.addressResponses = [];
      window.fetch = url => new Promise(resolve => window.addressResponses.push({ url, resolve }));
    });
    const detail = await page.evaluate(() => window.dawaDetailedAddress({ address: { house_number: '17', road: 'شارع التحرير', neighbourhood: 'وسط البلد', quarter: 'قصر النيل', suburb: 'قصر النيل', city: 'القاهرة', state: 'القاهرة', postcode: '11511', country: 'مصر' } }, 'ar'));
    assert.equal(detail, '17 شارع التحرير، وسط البلد، قصر النيل، القاهرة، 11511، مصر');
    await page.evaluate(() => window.fetchDetailedAddressFromCoords(30, 31));
    await page.waitForFunction(() => window.addressResponses.length === 1);
    await page.evaluate(() => window.fetchDetailedAddressFromCoords(31, 32));
    await page.waitForFunction(() => window.addressResponses.length === 2);
    await page.evaluate(() => {
      window.addressResponses[1].resolve({ ok: true, json: async () => ({ address: { house_number: '17', road: 'New street' } }) });
      window.addressResponses[0].resolve({ ok: true, json: async () => ({ address: { road: 'Stale street' } }) });
    });
    await page.waitForFunction(() => document.getElementById('reg-address').value === '17 New street');
    assert.match(await page.locator('#reg-address-hint').textContent(), /راجع العنوان/);
    await page.locator('#reg-address').fill('My building, floor 3');
    await page.evaluate(() => window.fetchDetailedAddressFromCoords(32, 33));
    await page.waitForFunction(() => window.addressResponses.length === 3);
    await page.evaluate(() => window.addressResponses[2].resolve({ ok: true, json: async () => ({ address: { road: 'Another street' } }) }));
    await page.waitForFunction(() => document.getElementById('reg-address-hint').textContent.includes('راجع'));
    assert.equal(await page.locator('#reg-address').inputValue(), 'Another street');
    await page.evaluate(() => window.fetchDetailedAddressFromCoords(33, 34));
    await page.waitForFunction(() => window.addressResponses.length === 4);
    await page.locator('#reg-address').fill('New manual edit while loading');
    await page.evaluate(() => window.addressResponses[3].resolve({ ok: true, json: async () => ({ address: { road: 'Late street' } }) }));
    await page.waitForFunction(() => document.getElementById('reg-address-hint').textContent.includes('راجع'));
    assert.equal(await page.locator('#reg-address').inputValue(), 'New manual edit while loading');
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
});
