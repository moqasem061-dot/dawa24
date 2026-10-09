// telegram-connect.js -- Settings > Telegram: one click opens the bot.
//
// The connect button posts to the server, which issues a one-time code and
// answers with the bot link in an HX-Trigger "telegram-link" event. Browsers
// block a tab opened after an asynchronous answer, so the tab is opened here,
// inside the click, blank. The link arrives with the answer and the tab is
// pointed at it. If the browser blocked the tab anyway, the card keeps its own
// "open Telegram" link, so nothing is lost.
(function () {
  'use strict';

  var CONNECT = '[data-telegram-connect]';

  // The blank tab waiting for the link, or null.
  var tab = null;

  function closeTab() {
    if (tab && !tab.closed) {
      try { tab.close(); } catch (_) { /* the browser kept the tab; nothing else to undo */ }
    }
    tab = null;
  }

  // Only the Telegram bot link may be opened from here.
  function isBotLink(url) {
    try {
      var u = new URL(url);
      return u.protocol === 'https:' && u.hostname === 't.me';
    } catch (_) {
      return false;
    }
  }

  // Capture phase: the tab opens before htmx starts the request.
  document.addEventListener('click', function (evt) {
    var el = evt.target;
    if (!el || !el.closest || !el.closest(CONNECT)) return;
    closeTab();
    tab = window.open('about:blank', '_blank');
  }, true);

  // The server's answer. An empty or foreign URL closes the waiting tab.
  document.addEventListener('telegram-link', function (evt) {
    var waiting = tab;
    tab = null;
    if (!waiting) return;
    var url = evt.detail && evt.detail.url;
    if (!url || !isBotLink(url)) {
      try { waiting.close(); } catch (_) { /* already gone */ }
      return;
    }
    try {
      waiting.opener = null;
      waiting.location.href = url;
    } catch (_) {
      // The tab went away before the link arrived; the card's own link remains.
    }
  });

  // A failed request never answers with a link, so its tab is closed.
  document.addEventListener('htmx:responseError', closeTab);
  document.addEventListener('htmx:sendError', closeTab);
})();
