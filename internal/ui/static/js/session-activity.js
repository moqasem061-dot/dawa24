// HttpOnly cookies stay server-owned. Activity is confirmed by the auth endpoint.
(function () {
  'use strict';
  var session = document.querySelector('meta[name="session-instance"]');
  if (!session || /^\/auth\//.test(window.location.pathname)) return;

  var timeout = document.querySelector('meta[name="session-idle-timeout-mins"]');
  var minutes = Number(timeout && timeout.content);
  var idleLimit = (minutes > 0 ? minutes : 30) * 60000;
  var initial = document.querySelector('meta[name="session-last-active"]');
  var lastActivity = Number(initial && initial.content) || Date.now();
  var storageKey = 'dawa24_activity_' + session.content;
  var lastSync = 0;
  var lastStored = 0;
  var inFlight = false;
  var stopped = false;

  function sharedActivity() {
    try {
      var saved = Number(localStorage.getItem(storageKey));
      if (saved > 0 && saved <= Date.now()) lastActivity = Math.max(lastActivity, saved);
    } catch (_) { /* The in-memory clock works when browser storage is blocked. */ }
    return lastActivity;
  }

  function signInAgain(code) {
    stopped = true;
    try { localStorage.removeItem(storageKey); } catch (_) {}
    var path = window.location.pathname + window.location.search;
    var reason = code === 'session.idle_timeout' ? '&reason=idle_timeout'
      : code === 'session.evicted_duplicate_login' ? '&error=duplicate_login'
      : code === 'session.evicted_concurrent_limit' ? '&error=concurrent_limit' : '';
    window.location.assign('/auth/login?redirect=' + encodeURIComponent(path) + reason);
  }

  function syncSession(background) {
    if (stopped || inFlight || Date.now() - lastSync < 30000) return;
    lastSync = Date.now();
    inFlight = true;
    var headers = { Accept: 'application/json' };
    if (background) headers['X-Dawa-Background'] = '1';
    var controller = new AbortController();
    var timer = setTimeout(function () { controller.abort(); }, 10000);
    fetch('/api/v1/auth/me', {
      credentials: 'same-origin', cache: 'no-store', headers: headers,
      signal: controller.signal
    }).then(function (response) {
      if (response.status === 401) {
        return response.json().then(function (body) {
          signInAgain(body.error && body.error.code);
        }, function () { signInAgain(); });
      }
    }).catch(function (error) {
      // A disconnected network or failed dependency does not invalidate the session.
      if (window.console) console.debug('Session check deferred:', error.name);
    }).finally(function () { clearTimeout(timer); inFlight = false; });
  }

  function recordActivity(evt) {
    if (!evt.isTrusted || document.visibilityState === 'hidden' || stopped) return;
    lastActivity = Date.now();
    if (lastActivity - lastStored >= 3000) {
      lastStored = lastActivity;
      try { localStorage.setItem(storageKey, String(lastActivity)); } catch (_) {}
    }
    syncSession(false);
  }

  ['pointerdown', 'keydown', 'input', 'wheel', 'touchstart'].forEach(function (event) {
    window.addEventListener(event, recordActivity, { passive: true, capture: true });
  });
  window.addEventListener('storage', function (evt) {
    if (evt.key === storageKey) sharedActivity();
  });
  setInterval(function () {
    if (document.visibilityState !== 'hidden' && Date.now() - sharedActivity() >= idleLimit) syncSession(true);
  }, 10000);
})();
