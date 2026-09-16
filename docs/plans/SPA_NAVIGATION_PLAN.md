# In-place navigation plan (no full page reload)

**Goal:** clicking a sidebar link or submitting an ordinary form in the admin,
pharmacy (customer) and vendor dashboards replaces only `<main>`, without a full
page reload. The sidebar, scroll position and open state stay where they are.
Anything that cannot be swapped safely falls back to a normal page load, so the
worst case is today's behaviour.

**Stack:** Go + templ + htmx 1.9.10 (`internal/ui/static/vendor/htmx-1.9.10.min.js`)
+ Alpine 3.13.5. No new libraries. No htmx upgrade.

**Who this is for:** an executing agent. Follow the steps in order. Do not
improvise beyond them. When a step says STOP, stop and report.

---

## 0. Background: why the first attempt was reverted

On 2026-09-12, commit `cc5de945` added `hx-boost="true"` to the sidebars, and
commit `3cdef605` removed it 19 minutes later. The failure causes were found
while writing this plan. Each one is addressed by a step below:

| # | Cause | Fixed in |
|---|-------|----------|
| A | Boosted requests carry `HX-Request: true`. Handlers that check that header returned *partials* (or JSON, or a 204) to a normal page navigation. | Step 2 |
| B | CSP uses a **new nonce per request** (`internal/platform/httpx/middleware.go:245-261`). Inline `<script nonce=…>` in swapped HTML carries the new nonce, which the already-loaded page does not accept, so all 71 inline page scripts were blocked. | Step 3 (`inlineScriptNonce`) |
| C | The default boost target is `<body>`, which swaps the whole shell. Per-shell CSS (`admin.css` / `vendor.css`) is not in the swapped HTML, and crossing shells broke styling. | Step 3 (swap `<main>` only, shell check) |
| D | htmx 1.9.10 **ignores `preventDefault()`**. Forms with `onsubmit="return confirm()"`, Alpine `@submit.prevent`, or Alpine-bound `:action` were submitted anyway, or to the wrong URL. | Step 3 (`htmx:beforeProcessNode` opt-out) |
| E | Page scripts that wait for `DOMContentLoaded`, add `document`-level listeners, or declare top-level `let`/`const` break when the page is reached a second time without a reload. | Step 4 |
| F | `app.js` calls `Alpine.initTree` on `htmx:load`. Alpine's MutationObserver already initialises new nodes, so the call double-binds `@click`, and a dropdown opens and closes on the same click. | Step 3.4 |
| G | The htmx history cache stores full page HTML (prices, orders) in `localStorage`, and restoring it breaks Alpine. | Step 3 (`historyCacheSize = 0`) |

---

## 1. Preconditions

Run from `F:\Dawa 24\dawa24-store`.

```bash
git status --short
templ generate
go build ./...
go test ./internal/ui/... ./internal/platform/authctx/... -count=1
```

- If the build or tests fail **before** you change anything, STOP and report.
  Do not fix unrelated failures.
- Other tools edit this repository concurrently (GitHub Desktop, `templ generate --watch`,
  other agent sessions). Re-read each file immediately before you edit it.
- After **every** `.templ` edit, run `templ generate`. Never hand-edit
  `*_templ.go` files.

---

## 2. Server: treat boosted requests as full-page requests

The helper already exists: `internal/ui/request_helpers.go:305`:

```go
func (h *UIHandler) isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true"
}
```

Make exactly these replacements. Each one appears once at the line given
(the line numbers are approximate, so match on the text):

| File | Find | Replace with |
|------|------|--------------|
| `internal/ui/request_helpers.go` (~185, inside `redirectWithNotice`, the `if err != nil` branch) | `if r.Header.Get("HX-Request") == "true" {` | `if h.isHTMX(r) {` |
| `internal/ui/request_helpers.go` (~226, end of `redirectWithNotice`) | `if r.Header.Get("HX-Request") == "true" {` | `if h.isHTMX(r) {` |
| `internal/ui/compare_mapping_modal_handlers.go` (~97) | `if r.Header.Get("HX-Request") == "true" \|\| r.URL.Query().Get("modal") == "1" {` | `if h.isHTMX(r) \|\| r.URL.Query().Get("modal") == "1" {` |
| `internal/ui/admin_import_handlers.go` (~187) | `if r.Header.Get("HX-Request") == "true" {` | `if h.isHTMX(r) {` |
| `internal/ui/admin_import_handlers.go` (~247) | `if r.Header.Get("HX-Request") == "true" {` | `if h.isHTMX(r) {` |
| `internal/ui/admin_temp_warehouse_mapping.go` (~262, func `isJSONOrAJAX`) | `\|\| r.Header.Get("HX-Request") == "true"` | `\|\| (r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true")` |
| `internal/ui/page_maintenance_handler.go` (~24) | `(r.Header.Get("HX-Request") == "true" && r.Method != http.MethodGet)` | `(r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" && r.Method != http.MethodGet)` |
| `internal/platform/authctx/audience.go` (~248, the block that answers 204 for unapproved orgs) | `if r.Header.Get("HX-Request") == "true" {` (the one directly under the comment "An htmx request refused here must NOT be answered with a 302") | `if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" {` |

Before replacing in `admin_import_handlers.go` and `compare_mapping_modal_handlers.go`,
confirm that the enclosing function's receiver is `(h *UIHandler)`. If it is not,
use the inline form instead:
`r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true"`.

Do **not** change `audience.go` ~359/~383 (forbidden handling). It already
distinguishes boosted requests, and `HX-Redirect` triggers a full navigation,
which is the correct result there.

Do **not** change the `HX-Redirect` calls in `customer_cart_handlers.go`,
`customer_cart_offer_handlers.go` or `favorites_handlers.go`. Those send the
user to login, which should be a full load.

### 2.1 Test

Add this test case to `internal/ui/b1_redirect_filters_test.go`, inside
`TestB1_RedirectWithNotice_PreservesFilters`, after the `"supports HTMX HX-Redirect"` subtest:

```go
	t.Run("boosted request gets a real 303, not HX-Redirect", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/admin/users/123/suspend", nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Boosted", "true")
		rec := httptest.NewRecorder()
		h.redirectWithNotice(rec, req, "/admin/users", "success", "ok")
		if rec.Code != 303 {
			t.Fatalf("expected 303 for boosted request, got %d", rec.Code)
		}
		if rec.Header().Get("HX-Redirect") != "" {
			t.Fatalf("boosted request must not receive HX-Redirect")
		}
		if rec.Header().Get("Location") == "" {
			t.Fatalf("expected Location header")
		}
	})
```

Run: `go test ./internal/ui/... ./internal/platform/authctx/... -count=1`. All tests must pass.

---

## 3. Client: the boost controller

### 3.1 Create `internal/ui/static/js/boost-nav.js`

The file is embedded automatically by `//go:embed static/*` in
`internal/ui/static.go`. Write exactly this:

```js
// boost-nav.js -- in-place navigation for the dashboard shells.
//
// hx-boost sits on .app-shell. Every boosted response is reduced to the inner
// HTML of <main id="main-content"> and swapped into the current <main>. The
// sidebar is never replaced. Anything that cannot be swapped safely -- another
// shell, a download, a page that opted out with data-hard-nav -- falls back to
// an ordinary page load, so the worst case is the behaviour before boosting.
(function () {
  'use strict';
  if (!window.htmx) return;

  var MAIN_ID = 'main-content';
  var HARD_NAV_MARK = 'data-hard-nav';
  var DOWNLOAD_PATH = /\/(export|download|print)(\/|$)|\.(csv|xlsx?|pdf|zip|png|jpe?g|webp)$/i;
  var FULL_LOAD_PATH = /^\/(auth|api|lang)\/|\/set-branch$|\/logout$/;
  var LINK_OPT_OUT_ATTRS = ['onclick', '@click', 'x-on:click', ':href', 'x-bind:href',
    'data-preview-file', 'data-preview-doc', 'data-receipt-preview', 'data-modal-open',
    'data-dialog-target', 'data-open-modal', 'data-tab-target', 'data-sidebar-toggle',
    'data-drawer-toggle', 'data-sidebar-mobile-toggle', 'data-drawer-close', 'data-no-boost'];
  var FORM_OPT_OUT_ATTRS = ['onsubmit', '@submit', 'x-on:submit', ':action', 'x-bind:action',
    ':method', 'x-bind:method', 'data-no-boost'];

  // -- htmx configuration ----------------------------------------------------
  // No page HTML in localStorage; back/forward does a normal load instead.
  htmx.config.historyCacheSize = 0;
  htmx.config.refreshOnHistoryMiss = true;
  htmx.config.scrollIntoViewOnBoost = false;
  // CSP nonces are per request. Scripts arriving in a swap must carry the
  // nonce of the document that is already loaded, not the one in the response.
  var nonceSource = document.querySelector('script[nonce]');
  if (nonceSource && nonceSource.nonce) {
    htmx.config.inlineScriptNonce = nonceSource.nonce;
  }

  function hasAnyAttr(el, names) {
    var own = el.getAttributeNames();
    for (var i = 0; i < own.length; i++) {
      for (var j = 0; j < names.length; j++) {
        if (own[i] === names[j] || own[i].indexOf(names[j] + '.') === 0) return true;
      }
    }
    return false;
  }

  function pathOf(raw) {
    try { return new URL(raw, window.location.href); } catch (_) { return null; }
  }

  function skipLink(a) {
    if (a.hasAttribute('download')) return true;
    if (a.target && a.target !== '_self') return true;
    if (hasAnyAttr(a, LINK_OPT_OUT_ATTRS)) return true;
    var u = pathOf(a.getAttribute('href') || '');
    if (!u || u.origin !== window.location.origin) return true;
    if (FULL_LOAD_PATH.test(u.pathname) || DOWNLOAD_PATH.test(u.pathname)) return true;
    if (u.pathname.indexOf('/documents/') !== -1 && /\/view$/.test(u.pathname)) return true;
    if (u.pathname.indexOf('/receipts/') !== -1 || u.pathname.indexOf('/uploads/') !== -1) return true;
    return false;
  }

  function skipForm(f) {
    if ((f.getAttribute('enctype') || '').toLowerCase() === 'multipart/form-data') return true;
    if ((f.getAttribute('method') || '').toLowerCase() === 'dialog') return true;
    var target = f.getAttribute('target');
    if (target && target !== '_self') return true;
    if (hasAnyAttr(f, FORM_OPT_OUT_ATTRS)) return true;
    if (f.querySelector('[formaction],[formmethod],[formenctype],[formtarget]')) return true;
    var u = pathOf(f.getAttribute('action') || window.location.href);
    if (!u || u.origin !== window.location.origin) return true;
    if (FULL_LOAD_PATH.test(u.pathname) || DOWNLOAD_PATH.test(u.pathname)) return true;
    return false;
  }

  // -- 1. Opt elements out before htmx wires them ------------------------------
  // htmx 1.9 ignores preventDefault() from inline/Alpine handlers and captures a
  // form's action once, so anything that relies on either must stay native.
  document.addEventListener('htmx:beforeProcessNode', function (evt) {
    var el = evt.target;
    if (!el || !el.tagName || el.hasAttribute('hx-boost')) return;
    if (!el.closest('[hx-boost="true"]')) return;
    if ((el.tagName === 'A' && skipLink(el)) || (el.tagName === 'FORM' && skipForm(el))) {
      el.setAttribute('hx-boost', 'false');
    }
  });

  // -- 2. Progress state -------------------------------------------------------
  var navFrom = null;

  function endNav() {
    document.documentElement.classList.remove('is-navigating');
  }

  document.addEventListener('htmx:beforeRequest', function (evt) {
    if (!evt.detail || !evt.detail.boosted) return;
    navFrom = window.location.pathname;
    document.documentElement.classList.add('is-navigating');
  });
  document.addEventListener('htmx:afterRequest', function (evt) {
    if (evt.detail && evt.detail.boosted) endNav();
  });
  document.addEventListener('htmx:sendError', endNav);
  document.addEventListener('htmx:timeout', endNav);

  // -- 3. Decide what a boosted response does ----------------------------------
  document.addEventListener('htmx:beforeSwap', function (evt) {
    var d = evt.detail;
    if (!d || !d.boosted || !d.xhr) return;
    var xhr = d.xhr;
    var verb = (d.requestConfig && d.requestConfig.verb) || 'get';
    var requested = pathOf((d.pathInfo && d.pathInfo.finalRequestPath) || window.location.href);
    var landed = pathOf(xhr.responseURL || '') || requested;
    var redirected = !!(requested && landed && requested.href !== landed.href);

    function fullLoad() {
      d.shouldSwap = false;
      endNav();
      if (verb === 'get' || redirected) {
        window.location.assign(landed.href);
      } else {
        // A POST that was not redirected has already run on the server;
        // resubmitting would run it twice. Show the resulting state instead.
        window.location.reload();
      }
    }

    if (landed && FULL_LOAD_PATH.test(landed.pathname)) return fullLoad();
    var type = (xhr.getResponseHeader('Content-Type') || '').toLowerCase();
    var disposition = (xhr.getResponseHeader('Content-Disposition') || '').toLowerCase();
    if (xhr.status === 204 || type.indexOf('text/html') === -1 || disposition.indexOf('attachment') !== -1) {
      return fullLoad();
    }

    var html = xhr.responseText || '';
    if (html.indexOf(HARD_NAV_MARK) !== -1) return fullLoad();

    var current = document.getElementById(MAIN_ID);
    var doc = new DOMParser().parseFromString(html, 'text/html');
    var incoming = doc.getElementById(MAIN_ID);
    if (!current || !incoming ||
        incoming.getAttribute('data-shell') !== current.getAttribute('data-shell') ||
        doc.documentElement.getAttribute('lang') !== document.documentElement.getAttribute('lang')) {
      return fullLoad();
    }

    d.target = current;
    d.serverResponse = incoming.innerHTML;
    d.shouldSwap = true;
    d.isError = false; // error pages that render inside the shell are shown in place
    if (doc.title) document.title = doc.title;
  });

  // -- 4. After the swap -------------------------------------------------------
  document.addEventListener('htmx:afterSettle', function (evt) {
    if (!evt.detail || !evt.detail.boosted) return;
    endNav();
    if (navFrom !== null && navFrom !== window.location.pathname) {
      window.scrollTo({ top: 0, behavior: 'instant' });
      var main = document.getElementById(MAIN_ID);
      if (main) {
        main.setAttribute('tabindex', '-1');
        main.focus({ preventScroll: true });
      }
    }
    navFrom = null;
  });
})();
```

### 3.2 Load it: `internal/ui/layouts/base.templ`

Directly **after** the line
`<script defer src={ Asset("/static/vendor/htmx-1.9.10.min.js") }></script>`
insert:

```templ
			// In-place navigation for the dashboard shells. Must run after htmx and
			// before htmx initialises (DOMContentLoaded); defer preserves that order.
			<script defer src={ Asset("/static/js/boost-nav.js") }></script>
```

### 3.3 Mark the shells

In each of the three dashboard shells, make two edits.

**`internal/ui/layouts/admin.templ`**
- `<div class="app-shell">` → `<div class="app-shell" hx-boost="true">`
- `<main class="main-content">` → `<main class="main-content" id="main-content" data-shell="admin">`

**`internal/ui/layouts/pharmacy.templ`** (`CustomerShell`)
- `<div class="app-shell">` → `<div class="app-shell" hx-boost="true">`
- `<main class="main-content">` → `<main class="main-content" id="main-content" data-shell="customer">`

**`internal/ui/layouts/vendor.templ`**
- `<div class="app-shell">` → `<div class="app-shell" hx-boost="true">`
- `<main class="main-content">` → `<main class="main-content" id="main-content" data-shell="vendor">`

Do **not** touch `customer.templ` (`PublicShell`), `auth` pages or `sidebar.templ`.
Do **not** add `hx-target`, `hx-select` or `hx-swap` to `.app-shell`: those
attributes are inherited and would retarget the ~100 existing `hx-get`/`hx-post`
elements inside it.

Then run: `grep -rn 'id="main-content"' internal/ui --include=*.templ`.
It must print exactly 3 lines. If another element already uses that id, STOP and report.

### 3.4 Edit `internal/ui/static/js/app.js`

**(a)** In the `htmx:beforeSwap` listener (starts at the comment
`// 1. Boosted request (SPA navigation across pages/sidebar)`), replace the whole
`if (evt.detail && evt.detail.boosted) { ... return; }` block (about 13 lines,
including the `/auth/` redirect check) with:

```js
    // 1. Boosted navigation is decided by boost-nav.js.
    if (evt.detail && evt.detail.boosted) {
      return;
    }
```

**(b)** Delete the whole `document.body.addEventListener('htmx:load', ...)` listener
that calls `window.Alpine.initTree(elt)`, including its comment line
`// Re-initialization on HTMX content swaps ...` if present. Alpine 3 initialises
added nodes through its own MutationObserver. The manual call double-binds
directives.

Leave the `htmx:afterSettle` listener that calls `initSidebarNav()` etc. unchanged.

### 3.5 Progress bar and focus style

Append to the **end** of `internal/ui/static/css/nav.css`:

```css
/* In-place navigation progress (boost-nav.js toggles html.is-navigating). */
@layer components {
  html.is-navigating::before {
    content: "";
    position: fixed;
    inset-block-start: 0;
    inset-inline: 0;
    block-size: 3px;
    z-index: var(--z-toast);
    background: var(--primary);
    transform-origin: 0 50%;
    animation: nav-progress 1.2s ease-out forwards;
    pointer-events: none;
  }

  html[dir="rtl"].is-navigating::before {
    transform-origin: 100% 50%;
  }

  html.is-navigating #main-content {
    opacity: 0.6;
    transition: opacity 150ms ease-out;
  }

  #main-content:focus {
    outline: none;
  }

  @keyframes nav-progress {
    from { transform: scaleX(0); }
    to   { transform: scaleX(0.85); }
  }

  @media (prefers-reduced-motion: reduce) {
    html.is-navigating::before {
      animation: none;
      transform: scaleX(1);
    }
  }
}
```

Rules this CSS must keep (the `make check` gates enforce them): no `!important`,
no `transition: all`, no `left`/`right` physical properties, and it must stay
inside `@layer`.

---

## 4. Make page scripts safe to run more than once

### 4.1 Mark pages that must still load fully

These templates contain scripts that are **not yet** safe to re-run: they wait for
`DOMContentLoaded`, add `document`/`window` listeners, start intervals, or load an
external script. In each file listed below, add the bare attribute `data-hard-nav`
to **every** `<script` opening tag in that file.

Example: `<script nonce={ httpx.Nonce(ctx) }>` → `<script nonce={ httpx.Nonce(ctx) } data-hard-nav>`
(Keep whatever nonce expression the file already uses.)

All files are under `internal/ui/pages/`:

```
admin_import_review.templ
admin_product_images_import.templ
admin_saving_products_modal.templ
admin_translations.templ
admin_weekly_coverages_modal.templ
ai_consumption_logs.templ
compare_tool.templ                    (also the <script src="/static/js/compare_tool.js"> tag)
customer_branches_form.templ
customer_team_modals.templ
customer_user_org_modals.templ
invoice_payment_modal.templ
invoice_printable.templ
saving_import_review.templ
saving_import_wizard.templ
smart_order_results_row.templ
smart_order_steps.templ
suppliers_map.templ
suppliers_profile.templ
vendor_delivery_map.templ
vendor_delivery_route_script.templ    (the <script defer src=...delivery-route.js> tag)
vendor_ingest_results.templ
vendor_ingest_review_script.templ
vendor_jobs.templ
vendor_payments_modal.templ
vendor_product_editor_pricing.templ
vendor_user_organizations.templ
```

Do **NOT** mark `internal/ui/components/capsule_assistant_script.templ`. It
renders in `<body>`, outside `<main>`, so it is never re-run.

Do **NOT** mark scripts with `type="application/ld+json"` or `type="application/json"`.

Verify with `grep -rc "data-hard-nav" internal/ui/pages/*.templ | grep -v ":0"`.
It must list all 26 files above.

### 4.2 Top-level `let`/`const` → `var`

A second run of a classic script that declares top-level `let x` throws
`Identifier 'x' has already been declared`. In the inline script that starts at
the given line, change **only lines at the script's outermost indentation** that
begin with `let ` or `const ` so they begin with `var `. Do not touch indented
lines inside functions, and do not wrap anything in an IIFE (inline `onclick=`
handlers need the functions to stay global).

```
internal/ui/pages/admin_import_wizard.templ:73
internal/ui/pages/compare_results.templ:357
internal/ui/pages/customer_jobs.templ:256
internal/ui/pages/customer_saving_script.templ:8
internal/ui/pages/jobs.templ:277
internal/ui/pages/smart_order_review_script.templ:8
internal/ui/pages/vendor_products_script.templ:8
internal/ui/pages/vendor_saving_import_script.templ:8
internal/ui/pages/vendor_saving_script.templ:8
```

Example (`compare_results.templ`): `let currentFilter = 'all';` → `var currentFilter = 'all';`

Then run `templ generate` and `go build ./...`.

---

## 5. Verify

### 5.1 Automated

```bash
templ generate
go build ./...
go vet ./internal/ui/... ./internal/platform/...
go test ./internal/ui/... ./internal/platform/authctx/... -count=1
make check-important check-transition-all check-css-layered check-no-cdn check-physical-properties check-undefined-classes
```

Every command must succeed. (On Windows, run `make` targets from Git Bash.)

### 5.2 Manual: run the app and check with the browser

Start the server with `run-dev.ps1` (or `make run`), then sign in as each role:
an admin, a pharmacy user and a vendor user. Open DevTools and keep the Console
and Network panels visible. For **each role**:

| # | Action | Expected |
|---|--------|----------|
| 1 | Click 5 different sidebar links | The sidebar does not flash. The URL bar updates. The tab title updates. The active sidebar item moves. The page scrolls to the top. The Network panel shows XHR requests, not document loads. |
| 2 | Browser Back, then Forward | The correct page appears (a full load is fine). |
| 3 | Submit an ordinary action form (for example approve/suspend on `/admin/users`, or add to cart) | No full reload. A toast appears. Filters and page number in the URL are kept. Scroll position is kept on the same page. |
| 4 | A form with `onclick="return confirm(...)"` or `onsubmit="return confirm(...)"`: click it, then press **Cancel** | **Nothing is submitted** (check the Network panel). |
| 5 | Open an Alpine dropdown on a page reached via sidebar (for example the branch selector or the user menu) | Opens on one click, closes on the next. |
| 6 | Click an export/download button (for example `/customer/saving-products/export`) | The file downloads. The page is not replaced by binary text. |
| 7 | Open a page from the §4.1 list (for example Compare tool, suppliers map) via the sidebar | A normal full load happens, and the page works. |
| 8 | Upload a file anywhere (documents, product import) | Works as before (native submit). |
| 9 | Switch language, switch branch, log out | A full page load happens and the result is correct. |
| 10 | Visit a page twice via the sidebar (for example `/customer/saving-products` → another page → back to it via the sidebar) | No console errors such as `already been declared` or `Refused to execute inline script`. |
| 11 | As an admin, follow a link that goes to a pharmacy/vendor-shell page (if one exists) | A full load happens, and the styling is correct. |
| 12 | Leave a dashboard page open for more than 60s | The notifications badge and cart badge still update. |

**If check 5 fails** (the dropdown does not open at all), restore the deleted
`htmx:load` listener from Step 3.4(b) and report it. **If check 4 fails**, STOP.
Revert Step 3.3 and report which form it was.

**Any console error in row 10** means a script in that page needs work. Add
`data-hard-nav` to that page's script tags (as in Step 4.1) and add the file to
the list in this document.

### 5.3 Rollback

Remove `hx-boost="true"` from the three shells (Step 3.3) and run `templ generate`.
Everything else is inert without it.

---

## 6. Phase 2: remove the remaining full reloads (one page per change)

Do this only after Phase 1 has been in production without regressions.
Each item is independent. Take them one at a time, in this order, and run
§5.2 checks 3, 4, 5 and 10 on the touched page after each one.

### 6.1 Convert a `data-hard-nav` page to boost-safe

For one file from §4.1:

1. Replace `document.addEventListener('DOMContentLoaded', fn)` with
   `(document.readyState === 'loading' ? document.addEventListener('DOMContentLoaded', fn) : fn())`.
2. Guard every `document.addEventListener(...)` / `window.addEventListener(...)`
   so it binds once per page load:
   ```js
   if (!window.__dawaBound_<uniqueName>) {
     window.__dawaBound_<uniqueName> = true;
     document.addEventListener('click', handler);
   }
   ```
   `<uniqueName>` is the file name in snake case, for example `__dawaBound_vendor_jobs`.
   The handler must look elements up **inside** the handler
   (`document.getElementById(...)`), never through a variable captured at bind time.
3. For every `setInterval`, store the id on `window` and clear the previous one
   first: `clearInterval(window.__dawaTimer_<uniqueName>); window.__dawaTimer_<uniqueName> = setInterval(...)`.
4. Apply §4.2 (`let`/`const` → `var`) to that script.
5. Remove `data-hard-nav` from that file's script tags, run `templ generate`, and run the §5.2 checks.

Order: `vendor_jobs`, `customer_branches_form`, `customer_team_modals`,
`customer_user_org_modals`, `vendor_user_organizations`, `admin_translations`,
`vendor_payments_modal`, `invoice_payment_modal`, `admin_saving_products_modal`,
`admin_weekly_coverages_modal`, `smart_order_results_row`, `suppliers_profile`,
`ai_consumption_logs`, `vendor_product_editor_pricing`. Leave the map, import and
compare pages as `data-hard-nav` permanently.

### 6.2 Replace `window.location.reload()` after fetch actions

Files: `admin_documents.templ:177`, `admin_temp_warehouses.templ:451,471,477,651,676`,
`smart_order_results_row.templ:299`, `smart_order_review_script.templ:69,74`,
`vendor_saving_review_script.templ:157`, `static/js/wizard.js:59`.
Leave the `onDone` reloads in the import wizards (`admin_import_review`,
`saving_import_wizard`, `smart_order_steps`, `vendor_ingest_results`) as they are,
because those pages are full-load pages.

Replace each `window.location.reload();` with:

```js
if (window.htmx) {
  htmx.ajax('GET', window.location.pathname + window.location.search,
    { target: '#main-content', select: '#main-content', swap: 'outerHTML' });
} else {
  window.location.reload();
}
```

`setTimeout(function () { window.location.reload(); }, N)` becomes the same code
inside the `setTimeout`. After the change, reload that page manually and repeat
the action twice. The page must update both times, and there must be no console errors.

### 6.3 Replace `onchange="...window.location.href=this.value"`

`internal/ui/components/pagination.templ:190` and `internal/ui/pages/admin_products.templ:265`:
replace `window.location.href=this.value` with
`(window.dawaNavigate ? window.dawaNavigate(this.value) : (window.location.href=this.value))`.
**Superseded (review 2026-09-16):** the original `htmx.ajax` + `history.pushState({})` snippet broke Back: htmx ignores popstate entries it did not create. Page scripts must use `window.dawaNavigate(url)` / `window.dawaRefresh()` from `boost-nav.js`.
Keep the leading `if(this.value)` guard.

### 6.4 Server-side fragments (optional, for speed)

Only after 6.1–6.3. In `internal/ui/layouts/admin.templ`, `pharmacy.templ` and
`vendor.templ`, when the request is boosted, render only `<main>` instead of the
full document. This needs the request in context, which requires a new
`layouts` context helper. **Do not attempt this without a separate reviewed
plan.** It is listed here only so nobody improvises it.

---

## 7. Phase 3: smooth full loads (optional, 10 minutes)

The pages that still load fully can cross-fade in Chromium and Safari. Append to the
end of `internal/ui/static/css/layout.css`, **after** the comment
`/* Closes the @layer layout block ... */` (outside the layer, because
`@view-transition` is not a cascade rule):

```css
/* Cross-document fade for navigations that still load a full page. */
@media (prefers-reduced-motion: no-preference) {
  @view-transition {
    navigation: auto;
  }
}
```

Run `make check-css-layered` (it must still pass) and check that a hard-nav page
(Compare tool) fades in instead of flashing white.

---

## 7b. Phase 1b: generic fixes (DONE 2026-09-16, in `boost-nav.js`)

These were done directly, because each one is subtle and was verified in a
browser test harness that loads the real vendored htmx 1.9.10, Alpine 3.13.5,
`boost-nav.js` and `app.js`. **Do not undo them.**

| Fix | Symptom it removes |
|-----|--------------------|
| `htmx.config.attributesToSettle = []` | The notification panel, account menu and any `id`'d `x-show` element opened by itself after navigation. htmx's settle step restored the server's `style` after Alpine had set `display:none`. |
| Open `<dialog>`s in `<main>` are closed before the swap | The page stayed scroll-locked (`body.modal-open`) after navigating from inside a modal. |
| Boosted links/forms use a private `hx-trigger="dawaboost"` fired from a `window` click/submit listener only when nobody called `preventDefault()` (replaced the earlier `guardHandlers()`, which depended on listener order) | `onsubmit="return confirm()"` (46 forms), `@submit.prevent`, `@click.prevent` and `onclick` no longer need to opt out of boosting, and **Cancel really cancels**. |
| Cancelled submit unlocks the form | `app.js`'s 8-second double-submit lock no longer sticks after a confirm() Cancel (a bug that predates boosting). |
| `htmx:configRequest` re-reads `href` / `action` at request time | Alpine-bound `:action` / `:href` (26 forms) are boosted and post to the current URL. |
| `HTMLFormElement.prototype.submit` routes boosted forms through `htmx.ajax` | `onchange="this.form.submit()"` (filters, per-row status selects; 49 call sites) updates in place. |
| `htmx:beforeOnLoad` turns `HX-Redirect` into a boosted click | `hx-post` actions answered by `redirectWithNotice` (approve/save/delete buttons) no longer hard-reload. `/auth/*`, `/api/*`, `/lang/*` and downloads still do. |
| `/set-branch` removed from `FULL_LOAD_PATH` | Changing the branch updates in place. |
| Public shell boosted (`layouts/customer.templ`: root div `hx-boost="true"`, `<main id="main-content" data-shell="public">`; `public_shell_footer.templ`: `<footer ... hx-boost="true">`) plus `syncPublicNav()` | Landing, about, how-it-works, contact, FAQ, plans, and catalogue/suppliers for guests navigate in place. Public ↔ dashboard crossings still do a full load (shell check). |

The things that still do a full page load **by design**: login, logout, register,
password reset, MFA (`/auth/*`); language switch (`/lang/*`; `<html dir>` and the
sidebar must change); moving between shells (public ↔ admin ↔ pharmacy ↔ vendor);
downloads, exports and print pages; import wizards and uploads (`hx-boost="false"`,
multipart); pages carrying `data-hard-nav`.

### 7b.1 Manual checks for Phase 1b (run on the real app)

In addition to §5.2, for each role:

| # | Action | Expected |
|---|--------|----------|
| 13 | Open the notification bell, click a notification link | Navigates in place. On the new page the panel is **closed**, and it opens and closes normally. |
| 14 | Open the account menu, then navigate via the sidebar | The menu is closed on the new page. |
| 15 | Open a modal, click a link inside it that goes to another page | The page changes, and the page still scrolls (no stuck scroll lock). |
| 16 | A row "delete" with `return confirm()`: press Cancel, then click again and press OK within 8s | Cancel sends nothing. OK deletes, in place. |
| 17 | An edit modal whose form uses `:action` (for example Admin → Users → edit) | Saves the right record, in place. |
| 18 | A status `<select>` in a table row or a filter select with `onchange="this.form.submit()"` | Updates in place, and the filters stay in the URL. |
| 19 | An `hx-post` approve/reject button | Toast is shown, the list refreshes in place, no white flash. |
| 20 | Switch branch from the top bar | The catalogue and prices change, the sidebar does not flash, and the branch label updates. |
| 21 | Public site (signed out): click through header, footer and mobile-drawer links | In place. The active header link moves. The mobile drawer closes. Login/Register do full loads. |
| 22 | Public page → "Dashboard" link while signed in | Full load into the dashboard shell (expected). |

---

## 7c. Phase 4: remaining work (status on 2026-09-16)

Take these in order. Each is independent. Stop and report after each.

1. **Phase 2 (§6.1–6.3) — DONE 2026-09-16.**
   - **§6.1:** Converted 14 pages to boost-safe (`vendor_jobs`, `customer_branches_form`, `customer_team_modals`, `customer_user_org_modals`, `vendor_user_organizations`, `admin_translations`, `vendor_payments_modal`, `invoice_payment_modal`, `admin_saving_products_modal`, `admin_weekly_coverages_modal`, `smart_order_results_row`, `suppliers_profile`, `ai_consumption_logs`, `vendor_product_editor_pricing`).
   - The following 12 files intentionally kept `data-hard-nav`:
     - `admin_import_review.templ` (multi-step import review flow with onDone reload)
     - `admin_product_images_import.templ` (heavy zip/image processor with custom progress)
     - `compare_tool.templ` (comparison engine loading external `compare_tool.js`)
     - `invoice_printable.templ` (standalone print document with dedicated stylesheet)
     - `saving_import_review.templ` (multi-stage catalog staging and matching review)
     - `saving_import_wizard.templ` (multi-step file upload and parsing wizard)
     - `smart_order_steps.templ` (multi-step order calculation and verification flow)
     - `suppliers_map.templ` (interactive Leaflet map with complex canvas lifecycle)
     - `vendor_delivery_map.templ` (Leaflet map and geofence picker)
     - `vendor_delivery_route_script.templ` (dynamic route optimization using `delivery-route.js`)
     - `vendor_ingest_results.templ` (catalog ingest table with terminal onDone handler)
     - `vendor_ingest_review_script.templ` (file reconciliation script)
   - **§6.2:** Replaced `window.location.reload()` calls with `htmx.ajax` in `admin_documents.templ`, `admin_temp_warehouses.templ` (5 call sites), `smart_order_results_row.templ`, `smart_order_review_script.templ` (2 call sites), `vendor_saving_review_script.templ`, and `wizard.js`.
   - **§6.3:** Replaced `window.location.href=this.value` in `pagination.templ` and `admin_products.templ` with `htmx.ajax` and `history.pushState`.
2. **Simple upload forms — DONE 2026-09-16.**
   - Changed `skipForm()` in `boost-nav.js` to allow forms with `data-boost-upload`.
   - Added `data-boost-upload` to all 9 qualifying forms: `admin_brands.templ` (2), `admin_products_modals.templ` (2), `admin_settings_site.templ` (1), `vendor_ad_edit_modal.templ` (1), `organization_documents_modals.templ` (1), `wallet_modal_deposit.templ` (1), and `admin_finance_modals.templ` (1).
   - Fixed scroll position jumping in `app.js` (prevented legacy `restoreScroll()` from fighting boosted swaps and eliminated window scrolling from sidebar navigation).
   - Added `@view-transition` CSS to `internal/ui/static/css/layout.css` (§7) for smooth cross-document fades on full loads.
3. **Row-level updates for heavy tables (optional, performance).** Row actions
   now swap the whole `<main>`, without a page reload. To swap only the row:
   give the `<tr>` a stable id (`id={ fmt.Sprintf("row-%d", item.ID) }`), and on
   the row's form add
   `hx-post={ same URL as action } hx-target="closest tr" hx-select={ "#row-" + id } hx-swap="outerHTML"`.
   The server then needs to answer that request with a 303 (not `HX-Redirect`),
   which means a separate reviewed change to `redirectWithNotice`. **Do not start
   this without a reviewed design.** Tables that would benefit most:
   `admin_users`, `admin_products_table`, `vendor_products_table`, `vendor_saving_table`.
4. **Never** set `hx-target`, `hx-select` or `hx-swap` on a `hx-boost="true"`
   container (they are inherited), and never add `hx-boost="true"` to
   `<body>` in `base.templ` (auth pages would double-request every link).

---

## 7d. Review of the Phase 2/4 execution (2026-09-16)

Fixed during review; **do not reintroduce**:

| Problem | Fix |
|---------|-----|
| htmx runs swapped inline scripts in its settle step (20ms later), after Alpine has initialised the new nodes, so `x-data="pageManager()"` failed with "pageManager is not defined" on a page's first in-place visit. | `htmx.config.defaultSettleDelay = 0` in `boost-nav.js`. |
| With zero settle delay, htmx wires swapped elements before Alpine binds `@click.prevent` / `@submit.prevent`, so the old guard listener no longer ran first and prevented actions were sent. | A private trigger (`hx-trigger="dawaboost"`) fired from a `window`-level listener only when `defaultPrevented` is false. This does not depend on listener order. |
| §6.2/§6.3 used `htmx.ajax(...)` + `history.pushState({})`: Back changed the URL but not the page, and the shell/hard-nav checks were skipped. | `window.dawaNavigate(url)` and `window.dawaRefresh()` in `boost-nav.js`. All 13 call sites now use them. |
| `ai_consumption_logs`: the Enter-to-search listener was bound once per page lifetime, so it was dead after the first in-place visit. | Bound once per element. |
| `smart_order_results_row`: an in-place refresh while the catalogue dropdown was open left its `<body>`-level backdrop covering the page. | `closeAllCatalogDropdowns()` before `dawaRefresh()`. |
| §4.2 was never applied to `customer_saving_script`, `smart_order_review_script`, `vendor_products_script`, `vendor_saving_import_script`, `vendor_saving_script` (top-level `let` threw on a second visit). | Changed to `var`. |
| `location.href` navigations on boosted pages (`ai_consumption_logs`, `customer_catalog` sort/view/product, `vendor_payments_modal` row click). | `dawaNavigate`. |

Rule for page scripts from now on: navigate with `dawaNavigate(url)`, refresh with
`dawaRefresh()`, bind per-element listeners once per element (a property on
the element), and bind `document`/`window` listeners once per page lifetime
(a `window.__dawaBound_*` flag) with element lookups inside the handler.

Not caused by this work, and still failing: `TestSettingsProfileAvatarUpload`;
`check-transition-all` (admin.css) and `check-physical-properties` (public.css).

---

## 8. Done criteria

- §5.1 passes.
- All 12 checks in §5.2 pass for admin, pharmacy and vendor.
- No `Refused to execute inline script` and no `already been declared` errors in the console
  after 10 minutes of normal clicking through each dashboard.
- Report: files changed, any page added to the `data-hard-nav` list, and any check that failed.
