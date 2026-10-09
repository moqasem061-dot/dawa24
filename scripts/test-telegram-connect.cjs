// Unit tests for the one-click Telegram connect, against the real script and a
// small DOM stub: node --test scripts/test-telegram-connect.cjs
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const source = fs.readFileSync(
  path.join(__dirname, '..', 'internal', 'ui', 'static', 'js', 'telegram-connect.js'), 'utf8');

// A tab the browser would open. Navigation is recorded, not performed.
function fakeTab(url) {
  return {
    url,
    closed: false,
    opener: 'page',
    location: { href: url },
    close() { this.closed = true; },
  };
}

// Loads the script the way the page does, with listeners captured so the test
// can fire the events itself. blocked=true makes window.open return null, which
// is what a browser does when it refuses the popup.
function load({ blocked = false } = {}) {
  const listeners = {};
  const opened = [];
  const document = {
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); },
  };
  const window = {
    open(url) {
      if (blocked) return null;
      const tab = fakeTab(url);
      opened.push(tab);
      return tab;
    },
  };
  new Function('window', 'document', source)(window, document);
  const fire = (type, detail) => {
    for (const fn of listeners[type] || []) fn({ type, detail, target: null });
  };
  const click = (onConnect) => {
    const target = {
      closest(selector) {
        return onConnect && selector === '[data-telegram-connect]' ? target : null;
      },
    };
    for (const fn of listeners.click || []) fn({ target });
  };
  return { fire, click, opened };
}

const BOT = 'https://t.me/dawa24_bot?start=abc123';

test('clicking connect opens a blank tab inside the click', () => {
  const page = load();
  page.click(true);
  assert.equal(page.opened.length, 1);
  assert.equal(page.opened[0].url, 'about:blank');
});

test('clicks elsewhere open nothing', () => {
  const page = load();
  page.click(false);
  assert.equal(page.opened.length, 0);
});

test('the server answer points the waiting tab at the bot', () => {
  const page = load();
  page.click(true);
  const tab = page.opened[0];
  page.fire('telegram-link', { url: BOT });
  assert.equal(tab.location.href, BOT);
  assert.equal(tab.opener, null, 'the bot page must not hold a handle on the settings page');
  assert.equal(tab.closed, false);
});

test('an empty answer closes the waiting tab', () => {
  const page = load();
  page.click(true);
  const tab = page.opened[0];
  page.fire('telegram-link', { url: '' });
  assert.equal(tab.closed, true);
  assert.equal(tab.location.href, 'about:blank');
});

test('a link that is not a Telegram bot link is never opened', () => {
  for (const url of ['https://evil.example/?start=abc', 'http://t.me/dawa24_bot?start=abc', 'javascript:alert(1)']) {
    const page = load();
    page.click(true);
    const tab = page.opened[0];
    page.fire('telegram-link', { url });
    assert.equal(tab.closed, true, `closed for ${url}`);
    assert.equal(tab.location.href, 'about:blank', `not navigated for ${url}`);
  }
});

test('a link arriving with no waiting tab does nothing', () => {
  const page = load();
  assert.doesNotThrow(() => page.fire('telegram-link', { url: BOT }));
  assert.equal(page.opened.length, 0);
});

test('a blocked popup leaves the page usable and the card link as the fallback', () => {
  const page = load({ blocked: true });
  page.click(true);
  assert.doesNotThrow(() => page.fire('telegram-link', { url: BOT }));
});

test('the waiting tab is used once', () => {
  const page = load();
  page.click(true);
  const tab = page.opened[0];
  page.fire('telegram-link', { url: BOT });
  tab.location.href = 'about:blank';
  page.fire('telegram-link', { url: 'https://t.me/dawa24_bot?start=second' });
  assert.equal(tab.location.href, 'about:blank');
});

test('a second connect click replaces the first tab', () => {
  const page = load();
  page.click(true);
  const first = page.opened[0];
  page.click(true);
  assert.equal(first.closed, true);
  assert.equal(page.opened.length, 2);
});

test('a failed request closes the waiting tab', () => {
  const page = load();
  page.click(true);
  const tab = page.opened[0];
  page.fire('htmx:responseError', {});
  assert.equal(tab.closed, true);
});
