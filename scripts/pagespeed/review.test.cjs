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

for (const width of [390, 768, 1024, 1467, 1920]) {
  for (const lang of ['ar', 'en']) {
    for (const theme of ['light', 'dark']) {
      test(`review fields fit ${width}px ${lang} ${theme}`, async () => {
        const page = await browser.newPage({ viewport: { width, height: 1000 } });
        try {
          const errors = [];
          page.on('pageerror', error => errors.push(error.message));
          await page.goto(`${origin}/review?lang=${lang}&theme=${theme}`);
          const layout = await page.locator('.review-table-shell').evaluate(shell => {
            const bounds = shell.getBoundingClientRect();
            const outside = [...shell.querySelectorAll('tbody td, input:not([type="hidden"]), .review-money-form button')]
              .filter(element => {
                if (!element.getClientRects().length) return false;
                const rect = element.getBoundingClientRect();
                return rect.left < bounds.left - 1 || rect.right > bounds.right + 1;
              }).map(element => element.className);
            return { outside, width: bounds.width, left: bounds.left, right: bounds.right, scrollWidth: shell.scrollWidth, clientWidth: shell.clientWidth };
          });
          assert.deepEqual(layout.outside, [], 'every field and save control stays within the table');
          assert.ok(layout.width <= width, 'table fits viewport');
          assert.ok(layout.left >= -1 && layout.right <= width + 1, 'table is not clipped outside the viewport');
          assert.ok(layout.scrollWidth <= layout.clientWidth + 1, 'no hidden or scrolling content');
          assert.equal(await page.locator('.vendor-review-row').first().locator('td').count(), 12);
          const quantity = page.locator('.vendor-review-row').first().locator('input[name="quantity"][type="number"]');
          await quantity.scrollIntoViewIfNeeded();
          assert.ok(await quantity.isVisible(), 'last column is visible');
          assert.equal(await quantity.inputValue(), '123456');
          const reasons = await page.locator('.match-reasons').allTextContents();
          assert.ok(reasons.some(text => text.includes(lang === 'ar' ? 'اختلاف في كلمات الاسم' : 'Different name details')));
          assert.ok(reasons.every(text => !/word_mismatch|ambiguous_siblings|retrieval_incomplete|match\.reason\./.test(text)));
          assert.deepEqual(errors, []);
        } finally { await page.close(); }
      });
    }
  }
}

test('review retains quantity editing, row selection, sorting, and catalogue picker', async () => {
  const page = await browser.newPage({ viewport: { width: 1024, height: 1000 } });
  try {
    await page.goto(`${origin}/review?lang=en`);
    const firstRow = page.locator('.vendor-review-row').first();
    await firstRow.locator('.review-pick').check();
    assert.match(await page.locator('#review-bulk-count').textContent(), /1 items selected/);
    assert.ok(await page.locator('.review-table thead a[href*="sort=quantity"]').isVisible());
    await firstRow.locator('[data-on-click="handleDropdownToggle(this)"]').click();
    assert.ok(await page.locator('#catalog-dropdown-1').isVisible());
    await page.locator('#catalog-dropdown-1 [data-on-click="closeAllCatalogDropdowns()"]').click();
    const quantity = firstRow.locator('input[name="quantity"][type="number"]');
    await quantity.fill('42');
    const requestPromise = page.waitForRequest(request => request.method() === 'POST');
    await page.route('**/review-fixture/**', route => route.fulfill({ status: 200, contentType: 'text/html', body: '<p>Saved</p>' }));
    await quantity.locator('..').locator('button[type="submit"]').click();
    const request = await requestPromise;
    assert.equal(new URLSearchParams(request.postData()).get('quantity'), '42');
  } finally { await page.close(); }
});
