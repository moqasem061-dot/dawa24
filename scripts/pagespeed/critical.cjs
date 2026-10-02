// Start the fixture: go run ./scripts/pagespeed
// Generate: npm ci --prefix scripts/pagespeed && npm run critical --prefix scripts/pagespeed
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { chromium } = require('playwright');
const postcss = require('postcss');
const root = path.resolve(__dirname, '../..');
const origin = process.env.PAGESPEED_ORIGIN || 'http://127.0.0.1:8097';
const output = path.join(root, 'internal/ui/static/critical');

function sources(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const filename = path.join(directory, entry.name);
    if (entry.isDirectory()) return entry.name === 'critical' ? [] : sources(filename);
    return /\.(templ|css|woff2)$/.test(filename) ? [filename] : [];
  });
}

function sourceHash() {
  const files = [...sources(path.join(root, 'internal/ui')),
    path.join(root, 'internal/ui/static_build.go'), __filename].sort();
  const hash = crypto.createHash('sha256');
  for (const filename of files) {
    hash.update(path.relative(root, filename).split(path.sep).join('/') + '\0');
    const source = fs.readFileSync(filename);
    hash.update(filename.endsWith('.woff2') ? source : source.toString().replace(/\r\n/g, '\n'));
    hash.update('\0');
  }
  return hash.digest('hex');
}

async function criticalStyles(page, url) {
  const response = await page.request.get(url);
  if (!response.ok()) throw new Error(`Stylesheet ${url}: ${response.status()}`);
  const stylesheet = postcss.parse(await response.text());
  const rules = [];
  stylesheet.walkRules(rule => rules.push(rule));
  const keep = await page.evaluate(selectors => selectors.map(selector => {
    // Preserve component families with hidden, conditional, or interactive
    // content before the full sheet arrives (including live ad inventory).
    if (/nav-|hero-|landing-|modal|capsule|btn|badge|preview/.test(selector)) return true;
    const probe = selector
      .replace(/\[data-theme=["']?dark["']?\]/g, ':root')
      .replace(/:(hover|active|focus-visible|focus-within|focus|visited|disabled|checked)\b/g, '')
      .replace(/::?(before|after|placeholder|selection|marker)\b/g, '');
    try { return !!document.querySelector(probe); }
    catch (error) {
      if (error.name !== 'SyntaxError') throw error;
      return true;
    }
  }), rules.map(rule => rule.selector));
  rules.forEach((rule, index) => {
    if (rule.parent.type === 'atrule' && /keyframes$/.test(rule.parent.name)) return;
    if (!keep[index]) rule.remove();
  });
  for (let pass = 0; pass < 5; pass++) {
    stylesheet.walkAtRules(rule => { if (rule.nodes && !rule.nodes.length) rule.remove(); });
  }
  stylesheet.walkComments(comment => comment.remove());
  return stylesheet.toString();
}

(async () => {
  const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || [
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
  ].find(candidate => fs.existsSync(candidate));
  const browser = await chromium.launch({ headless: true, ...(executablePath ? { executablePath } : {}) });
  try {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
    await page.goto(origin);
    // Use complete sheets even when the fixture already has inline styles.
    const urls = await page.locator('link[rel="stylesheet"]').evaluateAll(links => links.map(link => link.href));
    if (urls.length !== 2) throw new Error(`Expected two full stylesheets, found ${urls.length}`);
    let css = '';
    for (const url of urls) css += await criticalStyles(page, url);
    fs.mkdirSync(output, { recursive: true });
    fs.writeFileSync(path.join(output, 'home.css'), css + '\n');
    fs.writeFileSync(path.join(output, 'home.sha256'), sourceHash() + '\n');
    console.log(`Homepage critical CSS: ${Buffer.byteLength(css)} bytes. Rebuild and verify both themes and screen sizes.`);
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
