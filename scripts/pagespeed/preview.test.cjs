const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');

const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';

test('product image preview displays images and preserves details on every close path', async () => {
  const executablePath = ['C:/Program Files/Google/Chrome/Application/chrome.exe', 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'].find(path => fs.existsSync(path));
  const browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  // Exercise the current viewer source even when the long-lived fixture predates an edit.
  await page.route('**/static/js/preview.js*', route => route.fulfill({ contentType: 'text/javascript', body: fs.readFileSync(require('node:path').join(__dirname, '../../internal/ui/static/js/preview.js'), 'utf8') }));
  const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a2ioAAAAASUVORK5CYII=', 'base64');
  await page.route('**/static/images/product-fixture-missing.png', route => route.fulfill({ contentType: 'image/png', body: image }));
  await page.route('**/image-without-extension', route => route.fulfill({ contentType: 'image/png', body: image }));
  try {
    await page.goto(`${origin}/product`);
    const title = await page.locator('.b2b-product-info-panel h1').textContent();
    const url = page.url();
    const trigger = page.locator('.b2b-product-media-card [data-preview-file]');
    const modal = page.locator('#universal-file-preview-modal');
    for (const close of ['header', 'footer', 'escape', 'backdrop', 'immediate-reopen']) {
      await trigger.click();
      await modal.waitFor({ state: 'visible' });
      await page.waitForFunction(() => document.getElementById('universal-preview-img').naturalWidth > 0);
      await page.locator('#universal-preview-img').waitFor({ state: 'visible', timeout: 5000 });
      assert.equal(await page.locator('#universal-preview-img').isVisible(), true, close + ' image viewport: ' + await page.locator('#universal-preview-img-container').getAttribute('class'));
      if (close === 'header') await modal.locator('.modal-header button').click();
      else if (close === 'footer') await modal.locator('.modal-footer button').click();
      else if (close === 'escape') await page.keyboard.press('Escape');
      else if (close === 'backdrop') await page.mouse.click(2, 2);
      else {
        await page.evaluate(() => { window.closeFilePreview(); window.openFilePreview({ url: '/static/images/product-fixture-missing.png' }); });
        await page.waitForTimeout(100);
        assert.equal(await page.locator('#universal-preview-img').isVisible(), true, 'queued close does not clear reopened image');
        assert.ok(await page.locator('#universal-preview-img').evaluate(img => img.naturalWidth > 0));
        await modal.locator('.modal-footer button').click();
      }
      await modal.waitFor({ state: 'hidden' });
      await page.waitForFunction(() => !document.body.classList.contains('modal-open'));
      assert.equal(page.url(), url, 'closing never navigates away');
      assert.equal(await page.locator('.b2b-product-info-panel h1').textContent(), title);
      assert.equal(await page.locator('.product-detail-buy-box').isVisible(), true);
      assert.equal(await page.locator('body').evaluate(body => body.classList.contains('modal-open')), false);
    }
    await page.evaluate(() => window.openFilePreview({ url: '/image-without-extension', isImage: true }));
    await page.waitForFunction(() => document.getElementById('universal-preview-img').naturalWidth > 0);
    await page.locator('#universal-preview-img').waitFor({ state: 'visible', timeout: 5000 });
    assert.equal(await page.locator('#universal-preview-img').isVisible(), true);
    await modal.locator('.modal-footer button').click();
    await modal.waitFor({ state: 'hidden' });
    assert.deepEqual(errors, []);
  } catch (error) {
    console.log(await page.locator('#universal-preview-img').evaluate(img => { const result = []; for (let el = img; el; el = el.parentElement) { const style = getComputedStyle(el); result.push({ tag: el.tagName, id: el.id, classes: el.className, display: style.display, visibility: style.visibility, rect: el.getBoundingClientRect().toJSON() }); } return result; }));
    throw error;
  } finally { await browser.close(); }
});
