// Dawa24 Frontend Application Script
// Resilient Vanilla JS Engine: Tabs, Modals, Dropdowns, HTMX & Flash Notices.

// Global Cookie Helper
function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^|;\\s*)' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

function getCsrfToken() {
  return getCookie('dawa_csrf') || '';
}
window.getCsrfToken = getCsrfToken;
window.getCookie = getCookie;

// Restore the scroll position across a post/redirect/get on list screens.
// Keyed by pathname so two tabs on different screens do not fight.
(function() {
  function getStorageKey(pathname) {
    return 'dawa24_scroll_' + (pathname || window.location.pathname);
  }

  function saveScroll() {
    try {
      var key = getStorageKey(window.location.pathname);
      sessionStorage.setItem(key, JSON.stringify({
        y: window.scrollY || 0,
        path: window.location.pathname,
        time: Date.now()
      }));
    } catch(e) {}
  }

  function restoreScroll() {
    try {
      var key = getStorageKey(window.location.pathname);
      var raw = sessionStorage.getItem(key);
      if (!raw) return;
      var data = JSON.parse(raw);
      sessionStorage.removeItem(key);
      if (data && data.path === window.location.pathname && typeof data.y === 'number' && (Date.now() - data.time < 30000)) {
        window.scrollTo({ top: data.y, behavior: 'instant' });
        setTimeout(function() {
          if (Math.abs(window.scrollY - data.y) > 10) {
            window.scrollTo({ top: data.y, behavior: 'instant' });
          }
        }, 50);
        setTimeout(function() {
          if (Math.abs(window.scrollY - data.y) > 10) {
            window.scrollTo({ top: data.y, behavior: 'instant' });
          }
        }, 150);
      }
    } catch(e) {}
  }

  document.addEventListener('submit', function(e) {
    saveScroll();
  }, true);

  document.addEventListener('click', function(e) {
    var btn = e.target.closest('button[type="submit"], a.btn, [data-keep-scroll], [data-preserve-scroll]');
    if (btn) {
      saveScroll();
    }
  }, true);

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', restoreScroll);
  } else {
    restoreScroll();
  }
  window.addEventListener('pageshow', restoreScroll);

  document.addEventListener('htmx:beforeRequest', function(evt) {
    if (evt.detail && evt.detail.boosted) return;
    saveScroll();
  });
  document.addEventListener('htmx:afterSettle', function(evt) {
    if (evt.detail && evt.detail.boosted) return;
    restoreScroll();
  });
})();

// ==========================================================================
// Universal Button Resilience, Anti-Spam & Selection Lock Protection
// ==========================================================================

// 1. Prevent rapid multi-clicks (e.detail > 1) from selecting text or initiating
//    accidental text drag-and-drop on any button, stepper, tab or interactive element.
document.addEventListener('mousedown', (e) => {
  if (e.detail > 1) {
    const clickable = e.target.closest(
      'button, [role="button"], .btn, a.btn, .cart-stepper-btn, .cart-stepper-control, ' +
      'input[type="submit"], input[type="button"], [data-tab-target], [data-modal-open], ' +
      '[data-modal-close], [data-sidebar-toggle], .fav-icon-btn, .product-card-glass'
    );
    if (clickable) {
      e.preventDefault();
    }
  }
}, false);

// 2. Clear accidental text selection range on button clicks to guarantee cursor never
//    gets stuck on text-selection / I-beam ("sign of select").
document.addEventListener('click', (e) => {
  const clickable = e.target.closest('button, [role="button"], .btn, a.btn, .cart-stepper-btn, input[type="submit"], input[type="button"]');
  if (clickable && window.getSelection) {
    const sel = window.getSelection();
    if (sel && sel.type === 'Range') {
      try {
        sel.removeAllRanges();
      } catch (_) {}
    }
  }
}, true);

// 3. Global anti-spam debounce on action & submit buttons:
//    Disallow rapid multi-clicks within 350ms on the exact same button to avoid
//    hammering the backend or queuing duplicate operations.
document.addEventListener('click', (e) => {
  const btn = e.target.closest(
    'button[type="submit"], button[hx-post], button[hx-get], button[hx-delete], ' +
    'button[hx-patch], button[hx-put], a[hx-post], a[hx-delete], [data-action-submit]'
  );
  if (!btn) return;

  // If already marked busy or disabled, swallow duplicate clicks
  if (btn.classList.contains('is-busy') || btn.hasAttribute('disabled')) {
    e.preventDefault();
    e.stopPropagation();
    return;
  }

  const now = Date.now();
  const lastClick = parseInt(btn.dataset.dawaLastClick || '0', 10);
  if (now - lastClick < 350) {
    e.preventDefault();
    e.stopPropagation();
    return;
  }
  btn.dataset.dawaLastClick = now.toString();
}, true);

// 4. In-flight visual indicator & click-lock for HTMX requests
document.body ? initHtmxBusyGuards() : document.addEventListener('DOMContentLoaded', initHtmxBusyGuards);
function initHtmxBusyGuards() {
  document.body.addEventListener('htmx:beforeRequest', (evt) => {
    const elt = evt.detail && evt.detail.elt;
    if (elt && elt instanceof HTMLElement) {
      const btn = elt.tagName === 'BUTTON' ? elt : elt.querySelector('button[type="submit"], .btn-primary');
      if (btn) {
        btn.classList.add('is-busy');
        btn.setAttribute('aria-busy', 'true');
      }
    }
  });

  document.body.addEventListener('htmx:afterRequest', (evt) => {
    const elt = evt.detail && evt.detail.elt;
    if (elt && elt instanceof HTMLElement) {
      const btn = elt.tagName === 'BUTTON' ? elt : elt.querySelector('button[type="submit"], .btn-primary');
      if (btn) {
        btn.classList.remove('is-busy');
        btn.removeAttribute('aria-busy');
      }
    }
  });
}

// 5. Protect native HTML form double-submissions
document.addEventListener('submit', (e) => {
  const form = e.target;
  if (form && form.tagName === 'FORM' && !form.hasAttribute('hx-post') && !form.hasAttribute('hx-get')) {
    if (form.dataset.submitting === 'true') {
      e.preventDefault();
      return;
    }
    form.dataset.submitting = 'true';
    const submitBtn = form.querySelector('button[type="submit"], input[type="submit"]');
    if (submitBtn) {
      setTimeout(() => {
        submitBtn.classList.add('is-busy');
      }, 20);
    }
    setTimeout(() => {
      delete form.dataset.submitting;
      if (submitBtn) submitBtn.classList.remove('is-busy');
    }, 8000);
  }
}, true);

// Auto-inject _csrf token into all Native HTML form submissions
document.addEventListener('submit', (e) => {
  const form = e.target;
  if (form && form.tagName === 'FORM') {
    const method = (form.getAttribute('method') || 'GET').toUpperCase();
    if (method !== 'GET') {
      const csrfToken = getCookie('dawa_csrf');
      if (csrfToken) {
        let input = form.querySelector('input[name="_csrf"]');
        if (!input) {
          input = document.createElement('input');
          input.type = 'hidden';
          input.name = '_csrf';
          form.appendChild(input);
        }
        input.value = csrfToken;
      }
    }
  }
}, true);

// Programmatic form.submit() interceptor for CSRF (programmatic .submit() does not fire 'submit' DOM event)
if (typeof HTMLFormElement !== 'undefined' && HTMLFormElement.prototype) {
  const _origFormSubmit = HTMLFormElement.prototype.submit;
  HTMLFormElement.prototype.submit = function() {
    const method = (this.getAttribute('method') || 'GET').toUpperCase();
    if (method !== 'GET') {
      const csrfToken = getCookie('dawa_csrf');
      if (csrfToken) {
        let input = this.querySelector('input[name="_csrf"]');
        if (!input) {
          input = document.createElement('input');
          input.type = 'hidden';
          input.name = '_csrf';
          this.appendChild(input);
        }
        input.value = csrfToken;
      }
    }
    return _origFormSubmit.call(this);
  };
}

// Global Fetch Interceptor for CSRF
if (typeof window.fetch === 'function') {
  const _origFetch = window.fetch;
  window.fetch = function(url, options = {}) {
    const method = (options.method || 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
      const csrfToken = getCookie('dawa_csrf');
      if (csrfToken) {
        if (!options.headers) {
          options.headers = { 'X-CSRF-Token': csrfToken };
        } else if (options.headers instanceof Headers) {
          if (!options.headers.has('X-CSRF-Token')) {
            options.headers.set('X-CSRF-Token', csrfToken);
          }
        } else if (Array.isArray(options.headers)) {
          options.headers.push(['X-CSRF-Token', csrfToken]);
        } else if (typeof options.headers === 'object') {
          if (!options.headers['X-CSRF-Token'] && !options.headers['x-csrf-token']) {
            options.headers['X-CSRF-Token'] = csrfToken;
          }
        }
      }
    }
    return _origFetch.call(this, url, options);
  };
}

document.addEventListener('DOMContentLoaded', () => {
  // HTMX CSRF Attachment
  document.body.addEventListener('htmx:configRequest', (evt) => {
    const csrfToken = getCookie('dawa_csrf');
    if (csrfToken) {
      evt.detail.headers['X-CSRF-Token'] = csrfToken;
    }
  });

  // Guard: never swap a whole HTML document into an unselected partial target.
  //
  // A partial endpoint that sits behind an auth/approval gate can answer with a
  // 302 to a full page (e.g. an unapproved account polling the notifications
  // badge got redirected to /documents). The browser follows the redirect, htmx
  // swaps the entire document into the small element that made the request, and
  // if that document carries the same polling element the cycle repeats — the
  // page nests inside itself and grows without bound. If the response body looks
  // like a full document, or the request was redirected somewhere else, drop
  // the swap.
  //
  // Boosted requests (SPA navigation) and requests with explicit hx-select are
  // exempted: HTMX handles boosted body swaps and selected fragments properly.
  document.body.addEventListener('htmx:beforeSwap', (evt) => {
    // 1. Boosted navigation is decided by boost-nav.js.
    if (evt.detail && evt.detail.boosted) {
      return;
    }

    // 2. Explicit hx-select partial extraction
    const hasExplicitSelect = Boolean(
      (evt.detail && evt.detail.select) ||
      (evt.detail && evt.detail.elt && typeof evt.detail.elt.hasAttribute === 'function' && evt.detail.elt.hasAttribute('hx-select')) ||
      (evt.target && typeof evt.target.hasAttribute === 'function' && evt.target.hasAttribute('hx-select'))
    );
    if (hasExplicitSelect) {
      return;
    }

    // 3. Unselected partial target safety guard
    const xhr = evt.detail && evt.detail.xhr;
    if (!xhr) return;
    const body = (evt.detail.serverResponse || xhr.responseText || '');
    const looksLikeFullPage = /<\s*(!doctype|html)[\s>]/i.test(body.slice(0, 500));
    let redirectedAway = false;
    try {
      if (xhr.responseURL) {
        const to = new URL(xhr.responseURL, window.location.origin);
        const from = new URL(evt.detail.pathInfo && evt.detail.pathInfo.requestPath
          ? evt.detail.pathInfo.requestPath : window.location.pathname, window.location.origin);
        redirectedAway = to.pathname !== from.pathname;
      }
    } catch (e) { /* ignore */ }
    if (looksLikeFullPage || redirectedAway) {
      evt.detail.shouldSwap = false;
      evt.detail.isError = false;
      console.warn('htmx: suppressed swap of a full document / redirected response into', evt.target);
    }
  });

  document.body.addEventListener('htmx:afterSettle', () => {
    initSidebarNav();
    initNavScrollState();
    initScrollReveal();
    showNoticeFromQuery();

    // Close mobile drawer if open
    const sidebar = document.getElementById('app-sidebar') || document.querySelector('.sidebar');
    if (sidebar && sidebar.classList.contains('mobile-open')) {
      sidebar.classList.remove('mobile-open');
      const backdrop = document.querySelector('.sidebar-backdrop');
      if (backdrop) backdrop.classList.remove('active');
      document.body.classList.remove('sidebar-mobile-open');
    }

    // Re-initialize map pickers if present
    if (typeof initMapPickers === 'function' && document.querySelector('[data-map-picker], .map-canvas, .leaflet-container')) {
      initMapPickers();
    }
  });

  // HTMX Error Handling & Custom Events
  document.body.addEventListener('htmx:responseError', (evt) => {
    const xhr = evt.detail && evt.detail.xhr ? evt.detail.xhr : null;
    const hxTrigger = xhr ? (xhr.getResponseHeader('HX-Trigger') || xhr.getResponseHeader('hx-trigger')) : null;
    if (hxTrigger && hxTrigger.includes('showToast')) {
      // The server sent a customized business toast via HX-Trigger; suppress duplicate generic error
      return;
    }
    const status = xhr ? xhr.status : 500;
    console.error('Request failed:', status, xhr ? xhr.responseText : '');
    showToast(messageForStatus(status), 'error');
  });

  document.body.addEventListener('htmx:sendError', () => {
    const netErr = typeof window.dawaT === 'function'
      ? window.dawaT('toast.network_error', 'تعذر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.')
      : 'تعذر الاتصال بالخادم. تحقق من اتصالك بالإنترنت.';
    showToast(netErr, 'error');
  });

  document.body.addEventListener('showToast', (evt) => {
    const detail = evt.detail || {};
    const defaultSuccess = typeof window.dawaT === 'function'
      ? window.dawaT('toast.success', 'تمت العملية بنجاح')
      : 'تمت العملية بنجاح';
    const msg = detail.message || detail.value || defaultSuccess;
    const type = detail.type || 'info';
    showToast(msg, type);
  });

  document.body.addEventListener('cartUpdated', (evt) => {
    const detail = evt.detail || {};
    const count = detail.count;
    if (typeof count === 'number') {
      document.querySelectorAll('.cart-badge, .cart-count, [data-cart-count]').forEach((el) => {
        el.textContent = count > 0 ? count.toString() : '';
        el.style.display = count > 0 ? 'inline-flex' : 'none';
      });
    }
  });

  document.body.addEventListener('favoriteToggled', (evt) => {
    const detail = evt.detail || {};
    const pid = detail.productId;
    const isFav = detail.isFavorite;
    if (pid) {
      document.querySelectorAll(`[data-favorite-product-id="${pid}"]`).forEach((btn) => {
        btn.classList.toggle('is-active', isFav);
        btn.classList.toggle('btn-secondary', isFav);
        btn.classList.toggle('text-rose', isFav);
        btn.classList.toggle('btn-ghost', !isFav);
        btn.classList.toggle('text-muted', !isFav);
        const heartOutline = btn.querySelector('.fav-icon-outline');
        const heartFilled = btn.querySelector('.fav-icon-filled');
        if (heartOutline && heartFilled) {
          heartOutline.style.display = isFav ? 'none' : 'inline-block';
          heartFilled.style.display = isFav ? 'inline-block' : 'none';
        }
      });
    }
  });

  showNoticeFromQuery();
  initTabSystem();
  initModalManager();
  initDropdowns();
  initSidebarToggle();
  initThemeSystem();
  initScrollReveal();
  initNavScrollState();
});

// The public header lifts off the page once there is content behind it.
//
// A bar that carries a shadow at rest is carrying it for no reason -- the
// shadow's whole job is to say "something is scrolling under me". A sentinel
// and an IntersectionObserver do this without a scroll handler firing on every
// frame; the observer fires twice per page, at the two crossings.
function initNavScrollState() {
  const bar = document.querySelector('[data-nav-scroll]');
  if (!bar || typeof IntersectionObserver === 'undefined') return;
  if (bar.previousElementSibling && bar.previousElementSibling.classList.contains('nav-scroll-sentinel')) return;

  const sentinel = document.createElement('div');
  sentinel.setAttribute('aria-hidden', 'true');
  sentinel.className = 'nav-scroll-sentinel';
  bar.parentNode.insertBefore(sentinel, bar);

  new IntersectionObserver(
    ([entry]) => bar.classList.toggle('is-scrolled', !entry.isIntersecting),
    { threshold: 1 }
  ).observe(sentinel);
}

// Theme Management System
function initThemeSystem() {
  // The URL parameter wins over the stored preference, and it has to be read
  // here as well as in the inline script in base.templ. That script sets
  // data-theme before first paint; this function runs a moment later and used
  // to reset it from localStorage alone, silently undoing a ?theme= link.
  //
  // The visual regression harness varies theme through the URL, so every
  // baseline it captured came out in the browser's default theme -- four of the
  // eight were duplicates of the other four and the suite could not see a
  // light-mode regression at all. A shared link carrying ?theme= was equally
  // broken for real users, just less visibly.
  //
  // The URL value is deliberately not persisted: it styles this view, it does
  // not change what the reader chose.
  const urlTheme = new URLSearchParams(window.location.search).get('theme');
  const stored = localStorage.getItem('dawa24-theme') || 'system';
  const currentTheme = (urlTheme === 'light' || urlTheme === 'dark') ? urlTheme : stored;
  applyTheme(currentTheme, false);

  document.addEventListener('click', (e) => {
    const themeBtn = e.target.closest('[data-set-theme]');
    if (themeBtn) {
      e.preventDefault();
      const theme = themeBtn.getAttribute('data-set-theme');
      applyTheme(theme, true);
    }
  });

  if (window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if ((localStorage.getItem('dawa24-theme') || 'system') === 'system') {
        applyTheme('system', false);
      }
    });
  }
}

function applyTheme(theme, persist = false) {
  if (persist) {
    localStorage.setItem('dawa24-theme', theme);
  }
  let isDark = false;
  if (theme === 'dark') {
    isDark = true;
  } else if (theme === 'light') {
    isDark = false;
  } else {
    isDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  }

  document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');

  document.querySelectorAll('[data-set-theme]').forEach((btn) => {
    const btnTheme = btn.getAttribute('data-set-theme');
    const on = btnTheme === theme;
    btn.classList.toggle('active', on);
    // The class is what a sighted reader sees; aria-pressed is what a screen
    // reader announces. Both have to move or the control lies to one of them.
    if (btn.tagName === 'BUTTON') btn.setAttribute('aria-pressed', on ? 'true' : 'false');
  });
}

// Sidebar Collapse, Drawer & Navigation Shell
function initSidebarToggle() {
  const sidebar = document.getElementById('app-sidebar') || document.querySelector('.sidebar');
  if (!sidebar) return;

  // Persisted Desktop Collapsed State (above 1024px)
  const savedState = localStorage.getItem('dawa24-sidebar-collapsed');
  if (savedState === 'true' && window.innerWidth >= 1024) {
    sidebar.classList.add('collapsed');
    document.body.classList.add('sidebar-collapsed');
  }

  // Ensure Backdrop element exists
  let backdrop = document.querySelector('.sidebar-backdrop');
  if (!backdrop) {
    backdrop = document.createElement('div');
    backdrop.className = 'sidebar-backdrop';
    document.body.appendChild(backdrop);
  }

  let lastDrawerOpener = null;

  function setDrawerOpen(isOpen, openerBtn) {
    sidebar.classList.toggle('mobile-open', isOpen);
    backdrop.classList.toggle('active', isOpen);
    document.body.classList.toggle('sidebar-mobile-open', isOpen);

    if (openerBtn) {
      lastDrawerOpener = openerBtn;
    }

    const toggles = document.querySelectorAll('[data-drawer-toggle], [data-sidebar-mobile-toggle]');
    toggles.forEach((btn) => {
      btn.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
    });

    if (isOpen) {
      // Focus first interactive element in sidebar
      const firstFocusable = sidebar.querySelector('a, button, input, [tabindex]:not([tabindex="-1"])');
      if (firstFocusable) {
        setTimeout(() => firstFocusable.focus(), 50);
      }
    } else if (lastDrawerOpener && typeof lastDrawerOpener.focus === 'function') {
      lastDrawerOpener.focus();
    }
  }

  document.addEventListener('click', (e) => {
    // Desktop Collapse Toggle
    const toggleBtn = e.target.closest('[data-sidebar-toggle]');
    if (toggleBtn) {
      e.preventDefault();
      const isCollapsed = sidebar.classList.toggle('collapsed');
      document.body.classList.toggle('sidebar-collapsed', isCollapsed);
      localStorage.setItem('dawa24-sidebar-collapsed', isCollapsed ? 'true' : 'false');
      return;
    }

    // Mobile Off-canvas Drawer Toggle
    const drawerToggle = e.target.closest('[data-drawer-toggle], [data-sidebar-mobile-toggle]');
    if (drawerToggle) {
      e.preventDefault();
      const isCurrentlyOpen = sidebar.classList.contains('mobile-open');
      setDrawerOpen(!isCurrentlyOpen, drawerToggle);
      return;
    }

    // The drawer's own close button. It sits where the desktop collapse
    // control does and replaces it below 1024px, so a phone gets "close" in
    // the corner instead of a control that shrinks the drawer to an icon rail.
    if (e.target.closest('[data-drawer-close]')) {
      e.preventDefault();
      setDrawerOpen(false);
      return;
    }

    // Backdrop Click Dismissal
    if (e.target === backdrop) {
      setDrawerOpen(false);
    }
  });

  // The viewport can change without a navigation — a rotation, a split screen,
  // a resized window — and the two sidebar modes must not be left mixed.
  // Collapse belongs to the desktop column and is reapplied from storage when
  // the column comes back; an open drawer is dismissed when there is no longer
  // a drawer to be open.
  let wasDesktop = window.innerWidth >= 1024;
  window.addEventListener('resize', () => {
    const isDesktop = window.innerWidth >= 1024;
    if (isDesktop === wasDesktop) return;
    wasDesktop = isDesktop;

    if (isDesktop) {
      setDrawerOpen(false);
      const collapsed = localStorage.getItem('dawa24-sidebar-collapsed') === 'true';
      sidebar.classList.toggle('collapsed', collapsed);
      document.body.classList.toggle('sidebar-collapsed', collapsed);
    } else {
      sidebar.classList.remove('collapsed');
      document.body.classList.remove('sidebar-collapsed');
    }
  }, { passive: true });

  // Escape to close mobile drawer & Focus Trap
  document.addEventListener('keydown', (e) => {
    if (sidebar.classList.contains('mobile-open')) {
      if (e.key === 'Escape') {
        e.preventDefault();
        setDrawerOpen(false);
        return;
      }

      if (e.key === 'Tab') {
        const focusables = Array.from(sidebar.querySelectorAll('a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'));
        if (focusables.length === 0) return;
        const first = focusables[0];
        const last = focusables[focusables.length - 1];

        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
  });

  initSidebarNav();
}

// Active Link Resolution & Scroll Position Preservation
function initSidebarNav() {
  const nav = document.querySelector('.sidebar-nav');
  if (!nav) return;

  try {
    const currentPath = window.location.pathname;
    const links = Array.from(nav.querySelectorAll('.sidebar-link'));

    // Resolve best matching link for current pathname
    let exactMatch = links.find((l) => l.getAttribute('href') === currentPath);
    let bestMatch = exactMatch;
    if (!bestMatch && links.length > 0) {
      const sorted = links.slice().sort((a, b) => {
        return (b.getAttribute('href') || '').length - (a.getAttribute('href') || '').length;
      });
      for (let i = 0; i < sorted.length; i++) {
        const href = sorted[i].getAttribute('href');
        if (href && (href !== '/admin/dashboard' && href !== '/vendor' && href !== '/customer' && currentPath.indexOf(href) === 0)) {
          bestMatch = sorted[i];
          break;
        }
      }
    }

    if (bestMatch) {
      links.forEach((l) => {
        const isCurrent = l === bestMatch;
        l.classList.toggle('active', isCurrent);
        if (isCurrent) {
          l.setAttribute('aria-current', 'page');
        } else {
          l.removeAttribute('aria-current');
        }
      });
    }

    const saved = sessionStorage.getItem('dawa_sidebar_scroll_top');
    if (saved !== null) {
      nav.scrollTop = parseInt(saved, 10);
    } else {
      const act = nav.querySelector('.sidebar-link.active');
      if (act && typeof act.getBoundingClientRect === 'function' && typeof nav.getBoundingClientRect === 'function') {
        const aRect = act.getBoundingClientRect();
        const nRect = nav.getBoundingClientRect();
        if (aRect.top < nRect.top) {
          nav.scrollTop -= (nRect.top - aRect.top);
        } else if (aRect.bottom > nRect.bottom) {
          nav.scrollTop += (aRect.bottom - nRect.bottom);
        }
      }
    }

    if (!nav.dataset.scrollBound) {
      nav.dataset.scrollBound = 'true';
      nav.addEventListener('scroll', () => {
        sessionStorage.setItem('dawa_sidebar_scroll_top', nav.scrollTop);
      }, { passive: true });
    }
  } catch (e) {}
}

// Universal Accessible Native Modal Manager
let openDialogCount = 0;
let scrollLockY = 0;
const lastActiveElements = new WeakMap();

function initModalManager() {
  // Delegate Trigger Opens
  document.addEventListener('click', (e) => {
    const openBtn = e.target.closest('[data-modal-open], [data-dialog-target], [data-open-modal]');
    if (openBtn) {
      e.preventDefault();
      const targetId = openBtn.getAttribute('data-modal-open') || openBtn.getAttribute('data-dialog-target') || openBtn.getAttribute('data-open-modal');
      if (!targetId) return;
      const dialog = document.getElementById(targetId.trim());
      if (dialog && typeof dialog.showModal === 'function') {
        if (dialog.open) return;
        try {
          lastActiveElements.set(dialog, openBtn);
          dialog.showModal();
        } catch (_) {}
      }
      return;
    }

    // Delegate Trigger Closes
    const closeBtn = e.target.closest('[data-modal-close], [data-close-modal], .modal-close');
    if (closeBtn) {
      const dialog = closeBtn.closest('dialog');
      if (dialog && typeof dialog.close === 'function') {
        e.preventDefault();
        try {
          dialog.close();
        } catch (_) {}
      }
      return;
    }

    // Backdrop Click on Native Dialog
    if (e.target.tagName === 'DIALOG' && e.target.classList.contains('modal')) {
      const rect = e.target.getBoundingClientRect();
      const isOutside = (
        e.clientX < rect.left ||
        e.clientX > rect.right ||
        e.clientY < rect.top ||
        e.clientY > rect.bottom ||
        e.target === e.currentTarget
      );
      // If click was directly on dialog backdrop (not on modal-box child)
      if (e.target === e.currentTarget && !e.target.querySelector('.modal-box')?.contains(e.explicitOriginalTarget || e.target)) {
        if (typeof e.target.close === 'function') {
          try {
            e.target.close();
          } catch (_) {}
        }
      }
    }
  });

  // Alpine's $dispatch('open-modal', 'some-id') bubbles a CustomEvent.
  document.addEventListener('open-modal', (e) => {
    const id = typeof e.detail === 'string' ? e.detail : (e.detail && (e.detail.id || e.detail.modalId || e.detail.target));
    if (!id) return;
    const dialog = document.getElementById(String(id).trim());
    if (dialog && typeof dialog.showModal === 'function') {
      if (dialog.open) return;
      try {
        if (e.target && e.target instanceof HTMLElement) {
          lastActiveElements.set(dialog, e.target);
        }
        dialog.showModal();
      } catch (_) {}
    }
  });

  document.addEventListener('close-modal', (e) => {
    const id = typeof e.detail === 'string' ? e.detail : (e.detail && (e.detail.id || e.detail.modalId || e.detail.target));
    if (id) {
      const dialog = document.getElementById(String(id).trim());
      if (dialog && typeof dialog.close === 'function') {
        try { dialog.close(); } catch (_) {}
      }
    } else if (e.target && typeof e.target.closest === 'function') {
      const dialog = e.target.closest('dialog');
      if (dialog && typeof dialog.close === 'function') {
        try { dialog.close(); } catch (_) {}
      }
    }
  });

  // Listen for native dialog open and close events for scroll locking and focus restoration
  document.addEventListener('close', (e) => {
    if (e.target.tagName === 'DIALOG') {
      handleDialogClose(e.target);
    }
  }, true);

  // Body scroll lock on dialog show
  const originalShowModal = HTMLDialogElement.prototype.showModal;
  HTMLDialogElement.prototype.showModal = function() {
    if (this.open) return;
    if (openDialogCount === 0) {
      document.body.classList.add('modal-open');
    }
    openDialogCount += 1;

    // Trigger map invalidation if dialog contains map
    if (this.querySelector('[data-map-picker], .map-canvas, .leaflet-container')) {
      setTimeout(() => {
        if (typeof initMapPickers === 'function') initMapPickers();
        window.dispatchEvent(new Event('resize'));
      }, 100);
    }

    try {
      return originalShowModal.apply(this, arguments);
    } catch (err) {
      handleDialogClose(this);
      throw err;
    }
  };
}

function handleDialogClose(dialog) {
  if (openDialogCount > 0) {
    openDialogCount -= 1;
    if (openDialogCount === 0) {
      document.body.classList.remove('modal-open');
    }
  }

  const opener = lastActiveElements.get(dialog);
  if (opener && typeof opener.focus === 'function') {
    try { opener.focus(); } catch (_) {}
    lastActiveElements.delete(dialog);
  }
}

// Resilient Vanilla Tab System
function initTabSystem() {
  document.addEventListener('click', (e) => {
    const tabBtn = e.target.closest('[data-tab-target]');
    if (!tabBtn) return;

    e.preventDefault();
    const targetId = tabBtn.getAttribute('data-tab-target');
    const tabList = tabBtn.closest('.tab-list, .tabs, .nav-tabs, .pill-track, .sp-tabs-nav, .cb-tabs-nav');
    switchTab(targetId, tabList);
  });

  if (window.location.hash) {
    const hash = window.location.hash.substring(1);
    switchTab(hash);
  }
}

function switchTab(tabId, specificList) {
  if (!tabId || typeof tabId !== 'string') return;

  const cleanId = tabId.trim();
  const targetPane = document.getElementById('tab-' + cleanId) || document.querySelector(`[data-tab-pane="${cleanId}"]`);
  const targetBtn = document.querySelector(`[data-tab-target="${cleanId}"]`);

  if (!targetPane) return;

  const container = targetPane.closest('.tabs-container, .card, main, form, section') || document.body;
  const siblingPanes = container.querySelectorAll('.tab-pane');
  siblingPanes.forEach((pane) => pane.classList.remove('active'));

  const btnList = specificList || (targetBtn ? targetBtn.closest('.tabs-nav') : null) || container;
  const siblingBtns = btnList.querySelectorAll('[data-tab-target]');
  siblingBtns.forEach((btn) => btn.classList.remove('active'));

  targetPane.classList.add('active');
  if (targetBtn) targetBtn.classList.add('active');

  try {
    history.replaceState(null, null, '#' + cleanId);
  } catch (_) {}
}

// Universal Dropdown Toggle
function initDropdowns() {
  document.addEventListener('click', (e) => {
    const dropdownBtn = e.target.closest('.dropdown > button, .dropdown > .btn, [data-dropdown-toggle]');
    const clickedDropdown = dropdownBtn ? dropdownBtn.closest('.dropdown') : null;

    document.querySelectorAll('.dropdown.open, .dropdown.is-active').forEach((d) => {
      if (d !== clickedDropdown) {
        d.classList.remove('open', 'is-active');
        const menu = d.querySelector('.dropdown-menu');
        if (menu) menu.classList.remove('is-active');
      }
    });

    if (clickedDropdown) {
      e.stopPropagation();
      const isOpen = clickedDropdown.classList.toggle('open');
      clickedDropdown.classList.toggle('is-active', isOpen);
      const menu = clickedDropdown.querySelector('.dropdown-menu');
      if (menu) {
        menu.classList.toggle('is-active', isOpen);
      }
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      document.querySelectorAll('.dropdown.open, .dropdown.is-active').forEach((d) => {
        d.classList.remove('open', 'is-active');
        const menu = d.querySelector('.dropdown-menu');
        if (menu) menu.classList.remove('is-active');
      });
    }
  });
}

// Flash Query Parameter Toast
function showNoticeFromQuery() {
  const params = new URLSearchParams(window.location.search);
  const kind = params.get('notice');
  const msg = params.get('msg');
  if (!kind || !msg) return;

  showToast(msg, kind === 'error' ? 'error' : 'success');

  params.delete('notice');
  params.delete('msg');
  const qs = params.toString();
  window.history.replaceState({}, '', window.location.pathname + (qs ? '?' + qs : ''));
}

function messageForStatus(status) {
  if (typeof window.dawaT === 'function') {
    switch (status) {
      case 401: return window.dawaT('toast.unauthorized', 'انتهت الجلسة. يرجى تسجيل الدخول مرة أخرى.');
      case 403: return window.dawaT('toast.forbidden', 'ليس لديك صلاحية لتنفيذ هذا الإجراء.');
      case 404: return window.dawaT('toast.not_found', 'العنصر المطلوب غير موجود.');
      case 409: return window.dawaT('toast.conflict', 'تعارض في البيانات المدخلة.');
      case 422: return window.dawaT('toast.validation_error', 'بيانات غير صحيحة، يرجى مراجعة الحقول.');
      case 500: return window.dawaT('toast.server_error', 'حدث خطأ في الخادم. يرجى المحاولة لاحقاً.');
      default: return window.dawaT('toast.unexpected_error', 'حدث خطأ غير متوقع. يرجى المحاولة مرة أخرى.');
    }
  }
  switch (status) {
    case 401: return 'انتهت الجلسة. يرجى تسجيل الدخول مرة أخرى.';
    case 403: return 'ليس لديك صلاحية لتنفيذ هذا الإجراء.';
    case 404: return 'العنصر المطلوب غير موجود.';
    case 409: return 'تعارض في البيانات المدخلة.';
    case 422: return 'بيانات غير صحيحة، يرجى مراجعة الحقول.';
    case 500: return 'حدث خطأ في الخادم. يرجى المحاولة لاحقاً.';
    default: return 'حدث خطأ غير متوقع. يرجى المحاولة مرة أخرى.';
  }
}

// Automatic UTF-8 Mojibake decoder for headers/APIs returned as Latin-1
function decodeUtf8Mojibake(str) {
  if (typeof str !== 'string' || !str) return str;
  if (/[\u00C0-\u00FF]/.test(str)) {
    try {
      const bytes = Uint8Array.from(str, (c) => c.charCodeAt(0));
      const decoded = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
      if (decoded) return decoded;
    } catch (_) {}
  }
  return str;
}

function showToast(message, type = 'info') {
  message = decodeUtf8Mojibake(message);
  let container = document.getElementById('toast-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-container';
    container.className = 'toast-container';
    document.body.appendChild(container);
  }

  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  
  const text = document.createElement('span');
  text.textContent = message;
  text.style.flex = '1';
  toast.appendChild(text);

  const closeBtn = document.createElement('button');
  closeBtn.type = 'button';
  closeBtn.className = 'toast-close';
  closeBtn.textContent = '×';
  closeBtn.setAttribute('aria-label', 'Close');
  closeBtn.onclick = () => {
    toast.remove();
  };
  toast.appendChild(closeBtn);

  container.prepend(toast);

  setTimeout(() => {
    if (toast.parentNode) {
      toast.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
      toast.style.opacity = '0';
      toast.style.transform = 'translateY(-10px)';
      setTimeout(() => {
        if (toast.parentNode) toast.remove();
      }, 300);
    }
  }, 4500);
}

// Scroll Reveal
function initScrollReveal() {
  var targets = document.querySelectorAll('.reveal:not(.reveal-observed)');
  if (!targets.length) return;

  var prefersReduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (!('IntersectionObserver' in window) || prefersReduced) {
    targets.forEach(function(el) {
      el.classList.add('visible', 'reveal-observed');
    });
    return;
  }

  var observer = new IntersectionObserver(function(entries) {
    entries.forEach(function(entry) {
      if (entry.isIntersecting) {
        entry.target.classList.add('visible');
        observer.unobserve(entry.target);
      }
    });
  }, { threshold: 0.2 });

  targets.forEach(function(el) {
    el.classList.add('reveal-observed');
    observer.observe(el);
  });
}

// Universal Session Idle Timeout Watchdog
(function() {
  var path = window.location.pathname;
  if (path.startsWith('/auth/login') || path.startsWith('/auth/register') || path.startsWith('/auth/forgot') || path.startsWith('/auth/reset')) {
    return;
  }

  var sessionCookie = getCookie('dawa24_session') || getCookie('session');
  if (!sessionCookie) return;

  var metaTimeout = document.querySelector('meta[name="session-idle-timeout-mins"]');
  var timeoutMins = metaTimeout ? parseInt(metaTimeout.getAttribute('content'), 10) : 30;
  if (!timeoutMins || isNaN(timeoutMins) || timeoutMins <= 0) {
    timeoutMins = 30;
  }

  var idleLimitMs = timeoutMins * 60 * 1000;
  var lastActivityKey = 'dawa24_last_user_activity';
  var lastRecordedTime = 0;

  function recordActivity() {
    var now = Date.now();
    if (now - lastRecordedTime < 3000) return; // Throttle to once every 3s to prevent UI thread lock
    lastRecordedTime = now;
    try {
      localStorage.setItem(lastActivityKey, now.toString());
    } catch(e) {}
  }

  recordActivity();

  var events = ['mousedown', 'keydown', 'scroll', 'touchstart', 'click'];
  events.forEach(function(evt) {
    window.addEventListener(evt, recordActivity, { passive: true, capture: true });
  });

  var idleLogoutKey = 'dawa24_idle_logged_out';

  function redirectIdle() {
    try {
      localStorage.setItem(idleLogoutKey, Date.now().toString());
    } catch(e) {}
    localStorage.removeItem(lastActivityKey);
    var curPath = window.location.pathname + window.location.search;
    window.location.href = '/auth/login?reason=idle_timeout&redirect=' + encodeURIComponent(curPath);
  }

  function checkIdle() {
    try {
      // Another tab already logged out for idleness: follow immediately so no
      // tab stays on an authenticated screen with a dead session.
      if (localStorage.getItem(idleLogoutKey)) {
        localStorage.removeItem(lastActivityKey);
        var curPath2 = window.location.pathname + window.location.search;
        window.location.href = '/auth/login?reason=idle_timeout&redirect=' + encodeURIComponent(curPath2);
        return;
      }
      var stored = localStorage.getItem(lastActivityKey);
      var last = stored ? parseInt(stored, 10) : Date.now();
      var elapsed = Date.now() - last;
      if (elapsed >= idleLimitMs) {
        redirectIdle();
      }
    } catch(e) {}
  }

  // A fresh login clears a stale cross-tab logout flag from a previous session.
  try {
    localStorage.removeItem(idleLogoutKey);
  } catch(e) {}

  setInterval(checkIdle, 10000);
})();

// maps.js and the combobox reach for the toast through window; a top-level
// function declaration is already a window property in a classic script, but
// naming it here means a later move into a module or an IIFE cannot silently
// take the toast away from them.
window.showToast = showToast;
