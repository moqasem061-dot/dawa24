const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';
let browser;
before(async () => {
  const executablePath = ['C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(path => fs.existsSync(path));
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
});
after(async () => browser?.close());

test('official document preview survives refresh, replacement and load errors', async () => {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let failImage = false;
  const image = '<svg xmlns="http://www.w3.org/2000/svg" width="400" height="300"><rect width="400" height="300" fill="#1476b8"/></svg>';
  await page.route('**/documents/*/view', route => route.fulfill(failImage ? { status: 404, body: 'Missing document' } : { contentType: 'image/svg+xml', body: image }));
  try {
    for (const revision of ['1', '2']) {
      await page.goto(`${origin}/documents?revision=${revision}`);
      await page.reload();
      const trigger = page.locator('[data-on-click="openDocViewerFromBtn(this)"]').first();
      assert.equal(await trigger.getAttribute('data-doc-id'), revision);
      for (let n = 0; n < 3; n++) {
        await trigger.click();
        await page.locator('#modal-doc-img').waitFor({ state: 'visible' });
        assert.equal(await page.locator('#modal-doc-img').getAttribute('src'), `/documents/${revision}/view`);
        assert.equal(await page.locator('#modal-doc-iframe').isVisible(), false);
        await page.locator('#doc-viewer-modal .modal-footer button').click();
        await page.locator('#doc-viewer-modal').waitFor({ state: 'hidden' });
      }
    }
    failImage = true;
    await page.reload();
    await page.locator('[data-on-click="openDocViewerFromBtn(this)"]').first().click();
    await page.locator('#modal-doc-error').waitFor({ state: 'visible' });
    assert.equal(await page.locator('#modal-doc-iframe').isVisible(), false, 'failed images never leave a blank iframe');
    await page.keyboard.press('Escape');
    failImage = false;
    for (const path of ['/vendor/documents', '/admin/organizations/20/documents']) {
      await page.goto(origin + '/product');
      await page.evaluate(async path => {
        document.getElementById('main-content').setAttribute('hx-select', '#main-content > *');
        await window.htmx.ajax('GET', path, { target: '#main-content', swap: 'innerHTML', select: '#main-content > *' });
        history.pushState({}, '', path);
      }, path);
      await page.locator('[data-on-click="openDocViewerFromBtn(this)"]').first().click();
      await page.locator('#modal-doc-img').waitFor({ state: 'visible' });
      const prefix = path.startsWith('/admin') ? '/admin' : '/vendor';
      assert.equal(await page.locator('#modal-doc-img').getAttribute('src'), `${prefix}/documents/1/view`);
      await page.keyboard.press('Escape');
      await page.locator('#doc-viewer-modal').waitFor({ state: 'hidden' });
      await page.locator('[data-on-click="openDocViewerFromBtn(this)"]').first().click();
      await page.locator('#modal-doc-img').waitFor({ state: 'visible' });
      await page.keyboard.press('Escape');
    }
    await page.locator('#doc-viewer-modal').waitFor({ state: 'hidden' });
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});

for (const lang of ['ar', 'en']) {
  test(`activity list and details show localized text ${lang}`, async () => {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    try {
      await page.goto(`${origin}/activities?lang=${lang}`);
      const expected = lang === 'ar' ? 'تعديل بيانات الفرع' : 'Update branch details';
      assert.match(await page.locator('tbody').textContent(), new RegExp(expected));
      assert.doesNotMatch(await page.locator('tbody').textContent(), /POST|response_303|\/vendor\/branches|http\.operation/);
      await page.locator('tbody button').first().click();
      const modal = page.locator('#tenant-audit-details-modal');
      await modal.waitFor({ state: 'visible' });
      assert.match(await modal.textContent(), new RegExp(expected));
      assert.match(await modal.textContent(), /34\.50/);
      assert.doesNotMatch(await modal.textContent(), /POST|response_303|\/vendor\/branches|"parameters"|Before|After/);
      await modal.locator('[data-modal-close]').click();
      await modal.waitFor({ state: 'hidden' });
      await page.locator('input[type="search"]').fill(expected);
      assert.equal(await page.locator('tbody tr').first().isVisible(), true);
      assert.deepEqual(errors, []);
    } finally { await page.close(); }
  });
}
