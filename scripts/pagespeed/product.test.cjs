const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require('playwright');

const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';
let browser;

before(async () => {
  const executablePath = [
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  ].find(path => fs.existsSync(path));
  browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
});

after(async () => browser?.close());

for (const width of [360, 390, 768, 1024, 1440]) {
  for (const lang of ['ar', 'en']) {
    for (const theme of ['light', 'dark']) {
    test(`product geometry ${width}px ${lang} ${theme}`, async () => {
      const page = await browser.newPage({ viewport: { width, height: 1000 } });
      try {
        const errors = [];
        page.on('pageerror', error => errors.push(error.message));
        await page.goto(`${origin}/product?lang=${lang}&theme=${theme}`);
        const showcase = page.locator('.b2b-customer-showcase');
        await showcase.waitFor({ state: 'visible' });
        const columns = await showcase.evaluate(element => {
          const boxes = [...element.children].map(child => {
            const { x, y, width: boxWidth, height } = child.getBoundingClientRect();
            return { x, y, right: x + boxWidth, bottom: y + height };
          });
          return {
            boxes,
            columns: getComputedStyle(element).gridTemplateColumns.split(' ').length,
            overflow: document.documentElement.scrollWidth > innerWidth + 1,
          };
        });
        assert.equal(columns.overflow, false, 'the page fits its viewport');
        const expectedColumns = width > 1280 ? 3 : width > 768 ? 2 : 1;
        assert.equal(columns.columns, expectedColumns, 'the responsive product layout uses its designed column count');
        assert.equal(columns.boxes.length, 3, 'gallery, details, and purchase panel are present');
        for (let first = 0; first < columns.boxes.length; first++) {
          for (let second = first + 1; second < columns.boxes.length; second++) {
            const a = columns.boxes[first];
            const b = columns.boxes[second];
            const overlap = a.x < b.right && a.right > b.x && a.y < b.bottom && a.bottom > b.y;
            assert.equal(overlap, false, `product panels ${first} and ${second} do not overlap`);
          }
        }
        assert.match(await page.locator('.b2b-product-info-panel h1').textContent(), lang === 'ar' ? /بنادول/ : /Panadol/);
        assert.equal(await page.locator('.product-detail-buy-box').count(), 1);
        const offerPicker = page.locator('#product-offer-select');
        assert.equal(await offerPicker.count(), 1);
        await offerPicker.selectOption('102', { timeout: 3000 });
        const buyBox = page.locator('.product-detail-buy-box');
        assert.equal((await buyBox.locator('[data-product-offer-price]').textContent()).trim(), '39.50');
        assert.match(await buyBox.locator('[data-product-offer-supplier]').textContent(), /ابن سينا/);
        assert.match(await buyBox.locator('[data-product-offer-stock]').textContent(), lang === 'ar' ? /غير متوفر/ : /Out of stock/);
        assert.match(await buyBox.locator('[data-product-offer-coverage-text]').textContent(), lang === 'ar' ? /خارج نطاق التغطية/ : /Outside delivery coverage/);
        const selectedCartFields = await buyBox.locator('form[action="/cart/add"]').evaluate(form => ({
          variant: form.elements.namedItem('variant_id').value,
          offer: form.elements.namedItem('offer_id').value,
          supplier: form.elements.namedItem('vendor_org_id').value,
          quantity: form.elements.namedItem('qty').value,
          minimum: form.elements.namedItem('qty').min,
        }));
        assert.deepEqual(selectedCartFields, { variant: '102', offer: '0', supplier: '25', quantity: '10', minimum: '10' });
        assert.equal(await buyBox.locator('form[action="/cart/add"] button[type="submit"]').isDisabled(), true);
        await offerPicker.selectOption('101', { timeout: 3000 });
        assert.equal((await buyBox.locator('[data-product-offer-price]').textContent()).trim(), '38.00');
        assert.equal(await buyBox.locator('form[action="/cart/add"] button[type="submit"]').isDisabled(), false);
        await page.waitForTimeout(500);
        const imageFallbackState = await page.evaluate(() => {
          const image = document.querySelector('.b2b-product-media-card img');
          const fallback = document.querySelector('.b2b-product-media-card .product-img-fallback');
          return { complete: image?.complete, naturalWidth: image?.naturalWidth, fallbackClass: fallback?.className };
        });
        assert.equal(imageFallbackState.fallbackClass?.includes('hidden'), false, JSON.stringify(imageFallbackState));
        assert.equal(await page.locator('.b2b-product-media-card a').getAttribute('rel'), 'noopener noreferrer');
        assert.equal(await page.locator('html').getAttribute('data-theme'), theme);
        assert.deepEqual(errors, []);
      } finally {
        await page.close();
      }
    });
    }
  }
}
