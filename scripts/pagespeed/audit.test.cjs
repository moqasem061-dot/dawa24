const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8099';
let browser;

before(async () => {
  const executablePath = ['C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(fs.existsSync);
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
});
after(async () => { await browser?.close(); });

for (const width of [390, 768, 1440]) for (const theme of ['light', 'dark']) {
  test(`organization names stay readable at ${width}px in ${theme}`, async () => {
    const page = await browser.newPage({ viewport: { width, height: 1000 } });
    try {
      await page.addInitScript(theme => localStorage.setItem('dawa24-theme', theme), theme);
      await page.goto(origin + '/admin/organizations');
      const cell = page.locator('.organization-name-cell');
      const metrics = await cell.evaluate(cell => ({
        width: cell.getBoundingClientRect().width,
        nameHeight: cell.querySelector('a').getBoundingClientRect().height,
        avatarWidth: cell.querySelector('.flex-shrink-0').getBoundingClientRect().width,
        overflow: document.documentElement.scrollWidth > innerWidth + 1
      }));
      assert.ok(metrics.width >= 250, JSON.stringify(metrics));
      assert.ok(metrics.nameHeight < 100, JSON.stringify(metrics));
      assert.ok(metrics.avatarWidth >= 35, JSON.stringify(metrics));
      assert.equal(metrics.overflow, false);
      if (width === 1440 && theme === 'dark') await page.screenshot({ path: 'tmp/audit-organizations.png', fullPage: true });
    } finally { await page.close(); }
  });
}

test('sale price mode updates the real vendor form using exact decimal arithmetic', async () => {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  try {
    await page.goto(origin + '/vendor/products/new');
    const sale = page.locator('[name="sale_price"]');
    assert.equal(await sale.isVisible(), false);
    await page.locator('[name="discount_input_type"]').selectOption('sale_price');
    await sale.fill('80');
    assert.equal(await page.locator('[name="discount"]').inputValue(), '20.00');
    assert.equal(await page.locator('[name="discount"]').getAttribute('readonly'), '');
    await page.locator('[name="price"]').fill('32');
    await sale.fill('31.96');
    assert.equal(await page.locator('[name="discount"]').inputValue(), '0.13');
    assert.equal(await page.locator('[name="discount"]').getAttribute('step'), '0.01');
    await page.locator('[name="discount_input_type"]').selectOption('percentage');
    assert.equal(await sale.isVisible(), false);
    assert.equal(await sale.isDisabled(), true);
    assert.deepEqual(errors, []);
    await page.screenshot({ path: 'tmp/audit-vendor-pricing.png', fullPage: true });
  } finally { await page.close(); }
});

test('deposit rejection sends the selected deposit and reason through the real modal', async () => {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  try {
    await page.context().addCookies([{ name: 'dawa_csrf', value: 'fixture-csrf', url: origin }]);
    await page.route('**/admin/finance/deposits/81/reject', route => route.fulfill({ contentType: 'text/html', body: '<p>Rejected</p>' }));
    await page.goto(origin + '/admin/finance/deposits');
    await page.getByRole('button', { name: 'رفض', exact: true }).click();
    await page.locator('#rejection_reason_admin').fill('لم يصل التحويل');
    const requestPromise = page.waitForRequest(request => request.url().endsWith('/81/reject') && request.method() === 'POST');
    await page.getByRole('button', { name: 'تأكيد رفض الطلب', exact: true }).click();
    const request = await requestPromise;
    const form = new URLSearchParams(request.postData());
    assert.equal(form.get('rejection_reason'), 'لم يصل التحويل');
    assert.equal(form.get('_csrf'), 'fixture-csrf');
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});
