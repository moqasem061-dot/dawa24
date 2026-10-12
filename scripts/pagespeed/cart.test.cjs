const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';

test('product purchase button submits the selected supply and updates the cart', async () => {
  const executablePath = ['C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(fs.existsSync);
  const browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const requests = [];
    await page.route('**/cart/add', async route => {
      requests.push(new URLSearchParams(route.request().postData()));
      await route.fulfill({ status: 200, headers: { 'HX-Trigger': JSON.stringify({ showToast: { message: 'Added', type: 'success' }, cartUpdated: { count: 6 } }) }, body: '' });
    });
    await page.goto(origin + '/product');
    await page.evaluate(() => document.body.addEventListener('cartUpdated', event => { window.testCartCount = event.detail.count; }));
    const form = page.locator('.product-detail-buy-box form[action="/cart/add"]');
    assert.equal(await form.locator('button[type="submit"]').isDisabled(), false);
    assert.equal(await page.locator('#product-offer-select').inputValue(), '101');
    await page.locator('#product-offer-qty').fill('6');
    await form.locator('button[type="submit"]').click({ timeout: 3000 });
    await page.waitForFunction(() => window.testCartCount === 6);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].get('variant_id'), '101');
    assert.equal(requests[0].get('vendor_org_id'), '20');
    assert.equal(requests[0].get('product_id'), '12345');
    assert.equal(requests[0].get('qty'), '6');
    assert.equal(requests[0].get('offer_id'), '0');
    await page.locator('#product-offer-select').selectOption('102');
    assert.equal(await form.locator('button[type="submit"]').isDisabled(), true);
    await page.locator('#product-offer-select').selectOption('101');
    assert.equal(await form.locator('button[type="submit"]').isDisabled(), false);
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
});
