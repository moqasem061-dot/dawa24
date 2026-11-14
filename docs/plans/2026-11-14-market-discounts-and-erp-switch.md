# Plan — خصومات السوق rebuild + تكامل الأنظمة on/off switch (2026-11-14)

Two independent pieces of work. Read `AGENTS.md`, `docs/modules/compare.md` and
`docs/modules/erp.md` first. **The 400-line file rule is waived by the owner —
skip `check-file-size`.** Every other rule in `AGENTS.md` applies (money never
`float64`, bilingual strings, RTL first, no module→module imports, tenant
queries in `InTx`/`InReadTx`, never edit an applied migration).

Owner decisions (already made, do not re-ask):

| Question | Decision |
| --- | --- |
| Pagination | Numbered pages, results area swapped in place, next page prefetched on hover/focus |
| Clicking a search suggestion | Filters the board to that product (all warehouse offers for it) |
| ERP switched off — admins | Admins keep read access to `/admin/integrations` with an "off" banner; everyone else: hidden + blocked |
| Where the switch lives | A card on `/admin/settings` |

---

## Part A — خصومات السوق (`/market-discounts`) rebuild

### Diagnosis (why it lags)

The SQL is already fast (index scans, cached count — see compare.md "Market-board
rewrite"). The lag is the front end:

1. Every typing pause (250 ms) submits the whole GET form through the generic
   filter-form machinery (`static/js/actions.js` `queueFilterSearch` →
   `requestSubmit` → `boost-nav.js`), which re-renders the full page and swaps
   all of `<main>`, re-initialising Alpine for the whole page.
2. Each response renders **every item twice** — a list view and a grid view, one
   hidden with `x-show`. At `limit=96` that is 192 cards of DOM per keystroke.
3. Cards are `.glass-panel` (shadow + transform transitions) with no
   `content-visibility`, so off-screen cards are laid out and painted.
4. Date inputs, number inputs and selects auto-submit the full page on `change`.
5. The header stat "أعلى خصم" is computed from the current page only, which is
   wrong (it changes as you page).

### Target behaviour

- **Typing never touches the board.** Typing shows a dropdown of suggestions
  under the search box. The board only changes when the user commits: presses
  Enter, clicks "عرض كل النتائج لـ «…»", or picks a suggestion.
- **Only the results region updates** on any filter, sort, page or page-size
  change. Header, controls and the sidebar are never re-rendered or swapped.
- **One markup per offer.** List and grid are two CSS layouts of the same
  `<article>`, switched by a class on the container — instant, no request.
- **Numbered pagination swapped in place**, scroll to the top of the results,
  next/prev page prefetched on hover/focus, URL kept in sync (shareable, back
  button works).

### A1. Server: results fragment

- Extract the results surface (stats + active-filter chips + offers + pager)
  into a templ component, e.g. `MarketDiscountsResults(...)`, in its own file
  (`internal/ui/pages/market_discounts_results.templ`). The full page renders it
  inside `<section id="md-results" aria-live="polite">`.
- Handler (`internal/ui/compare_intelligence_handlers.go` `MarketDiscountsPage`):
  when the request is an htmx request targeting the results region
  (`HX-Request: true` and `HX-Target: md-results`), render **only** the results
  component, not the shell. Set `Vary: HX-Request, HX-Target` on both
  responses so a cached fragment is never shown as a page.
- Move the header numbers (count, warehouses, highest discount) inside the
  fragment (or use `hx-swap-oob`) so they update with the results.
- **Fix "أعلى خصم":** compute the board-wide maximum discount for the current
  filter in the **same** cached count query (`COUNT(*)` + `MAX(clamped
  discount)` in one scan, same cache key, same `marketCountBudget`). Carry it on
  `MarketDiscountsResult` (e.g. `MaxDiscount float64` + uses `TotalKnown`).
  When unknown, show `—`. Drop `calculateMaxDiscount`.
- Factor the query-string parsing out of the handler into one function
  (`parseMarketDiscountsFilter(r)`) shared by the page, the fragment and the
  suggest endpoint.

### A2. Server: product filter (target of a picked suggestion)

- New filter field `ProductID *int64` (query param `product`). When set, the
  board shows every eligible offer whose `matched_product_id` equals it (all
  warehouses), with the same ordering, count and cache semantics as other
  filters. It uses the catalog-link index from migration 243 — add an
  `EXPLAIN` style test in the same spirit as
  `TestMarketDiscountSortMatchesItsIndex` if the existing test helpers allow it.
- Shown as a removable chip ("الصنف: <name>") in the active filters row.
- `marketDiscountsQueryValues` carries it through paging.

### A3. Server: suggestions endpoint

- `GET /market-discounts/suggest?q=…` — same access rules as the page (signed
  in, not a customer, entitlement `billing.FeatureMarketDiscounts` unless
  staff). Register next to the page in `public_routes.go`. Customers and
  unentitled callers get 403/redirect-free errors (it is an XHR), never data.
- Returns an **HTML fragment** (templ, so escaping and bilingual text are
  handled the same way as the page) of at most **8** suggestions.
- Service method on `compare` (e.g. `SuggestMarketDiscounts(ctx, q, limit)`):
  reuse the existing indexed word-match clause from
  `postgres/market_discounts_query.go`; group matched offers by
  `matched_product_id` (unmatched offers grouped by normalised file name); per
  suggestion return: display name (catalog AR/EN by lang, else file name),
  matched product id (nullable), number of offers, number of warehouses, best
  (max) discount, lowest public price (`money.Amount`). **No COUNT of the
  board, no fuzzy fallback** (fuzzy only if the direct match returns nothing,
  under the existing 500 ms candidate deadline, or skip fuzzy entirely for
  suggestions — your call, document it). Overall deadline ~400 ms; on timeout
  return an empty fragment with a "تعذّر جلب الاقتراحات" note, never a 5xx page.
- Min 2 characters (Arabic composition respected), max 128.
- Short cache (30–60 s) keyed by normalised query + board stamp is welcome but
  optional; prices may be cached here only as long as the board's own rules
  allow (compare.md says prices are never cached — if in doubt, cache only the
  product grouping, or do not cache).

### A4. Client: one small dedicated script

New `internal/ui/static/js/market-discounts.js`, loaded only on this page with
the CSP nonce (follow how other page scripts are loaded; Alpine is the CSP build
— no inline methods in `x-data`; Trusted Types / sanitiser rules from
`security.js` apply to any HTML insertion — use htmx swaps or the existing
sanitised path, never raw `innerHTML` of untrusted strings).

- Mark the form `data-no-filter data-no-boost` so `actions.js` auto-submit and
  `boost-nav.js` leave it alone; this script owns it.
- **Search combobox** (ARIA combobox pattern: `role=combobox`,
  `aria-expanded`, `aria-controls`, `aria-activedescendant`, listbox/options):
  debounce 200 ms, abort the previous request (`AbortController`) and drop stale
  responses by request revision, skip during IME composition, Up/Down/Home/End/
  Enter/Escape/Tab, close on outside click / blur (commit on `mousedown` with
  default prevented — the same reason as in `combobox.js`). Loading and
  "no suggestions" states are visually distinct. First item: "عرض كل النتائج
  لـ «q»". Reuse `combobox.js` if it can be extended cleanly with a "commit"
  callback and an HTML (not JSON) source; otherwise write the small dedicated
  module — do not fork a copy of combobox.js.
- **Committing** (Enter / "view all" / suggestion / sort / warehouse / page size
  / range apply / chip removal / page link): build the URL from the form
  (+ `page`), `htmx.ajax('GET', url, {target:'#md-results', swap:'outerHTML'
  or 'innerHTML', headers:{'HX-Target':'md-results'}})` or a declarative
  `hx-get` — your choice — then `history.pushState` the full URL. `popstate`
  re-fetches the fragment for the restored URL (or does a normal load).
- While loading: `aria-busy="true"` on the results, old results dimmed (CSS
  opacity, no spinner overlay that shifts layout), min-height kept so the page
  does not jump. A request superseded by a newer one is aborted/ignored.
- **Pagination:** intercept clicks on pager links inside `#md-results`; on
  `mouseenter`/`focus` prefetch that page's fragment into a tiny in-memory map
  (≤ 6 entries, 30 s TTL) and use it on click. After swap, scroll the results'
  top into view only if it is above the viewport (`scrollIntoView({block:
  'start'})`, honour `prefers-reduced-motion`), and move focus to the results
  heading for keyboard/screen-reader users.
- **View switch** (list/grid): toggles a class on the results container,
  persists to `localStorage` (try/catch) and `history.replaceState` (`view=`),
  no request.
- Date and number inputs no longer auto-submit; they live in the "فلاتر
  متقدمة" panel and apply with its button (Enter inside also applies).

### A5. Markup and CSS

- One `<article class="md-offer">` per offer; container
  `.md-offers.is-list | .md-offers.is-grid`. Move the list/grid differences into
  CSS (`internal/ui/static/css/components.css`, the existing Market Discounts
  section). Remove the duplicated grid loop.
- `content-visibility: auto; contain-intrinsic-size: auto <h>px` on offers; no
  transform/box-shadow transitions on these cards; keep the visual language
  (badges, discount pill, `is-hot` ≥ 40 %).
- Controls layout (RTL first, wraps on mobile, 44 px touch targets):
  row 1 — search (with dropdown), warehouse (keep the datalist input or use the
  shared combobox), sort, "فلاتر متقدمة" toggle, view switch;
  collapsible panel — date range, price range, discount range, apply;
  row 2 — active filter chips, each removable, plus "مسح الكل".
- Empty state stays, keyed to whether filters are active.
- No HTML comments that describe internals (existing convention).

### A6. Tests and docs

- Handler: full page vs fragment (fragment has no shell/sidebar; has
  `Vary`), product filter parsed and carried in pager URLs, invalid values
  ignored.
- Suggest: unauthenticated, customer and unentitled callers get no data;
  ≤ 8 items; 1-char query returns empty; grouping by product; never includes a
  private compare upload (mirror the existing audience test in
  `compare_market_discounts_e2e_test.go`).
- Query: product filter SQL and the COUNT+MAX query (unit tests on the query
  builder; integration test where a DB is available).
- Update existing tests that assert on the old markup (two loops, `x-show`
  views, `market_discounts_footer_test.go`, `scripts/pagespeed`), keeping their
  intent.
- Browser regression: extend `scripts/test-session-search.cjs` (or add
  `scripts/test-market-discounts.cjs` in the same style) for: typing does not
  swap results, dropdown keyboard nav, commit swaps only `#md-results`, stale
  response ignored, pager prefetch, back button.
- `docs/modules/compare.md`: new section "Market-board UI rebuild
  (2026-11-14)" — what changed and why, the endpoint, the product filter, the
  MAX-discount fix.

---

## Part B — تكامل الأنظمة runtime switch

### Today

`cfg.ERP.Enabled` (`ERP_API_ENABLED`) is read once at boot in
`cmd/server/erp_wiring.go`: off → `/erp/v1` not mounted, UI routes not
registered, `rbac.NavSuppressions["integrations"] = true`. `cmd/worker/main.go`
skips ERP workers. That stays as the **deployment** switch. We add a
**runtime** switch on top: effective state = `cfg.ERP.Enabled AND flag`.

### B1. The flag

- Reuse `internal/platform/features` and `platform_admin.feature_flags`. New key
  **`erp.enabled`**, default **on** (so deploying this changes nothing).
- New migration `262_erp_runtime_switch.up.sql/.down.sql` (`BEGIN/COMMIT`):
  insert the row with bilingual `name`/`description`
  (`{"ar":"تكامل الأنظمة (ERP)","en":"Systems integration (ERP)"}`),
  `ON CONFLICT (key) DO NOTHING`; down deletes the row. Check the table's
  columns in `054_feature_flags.up.sql` before writing it.
- Add the key to `defaultCoreFlags` too.
- Add `(*Engine).Set(ctx, key, enabled, actorID)` + package-level
  `features.Set(...)`: upsert under `database.AsSystem` inside `InTx`, then
  reload the in-memory map **immediately** in this process. Other server
  instances pick it up within the existing 60 s reload; document that.
- Add one helper the whole app uses, e.g. `features.ERPEnabled(ctx)` or a
  small `internal/platform/erpgate` (platform must not import modules) that
  returns `deploymentEnabled && features.Enabled(ctx, "erp.enabled")`. The
  deployment bit is set once at boot from `cfg.ERP.Enabled`.

### B2. Blocking access (the switch is a hard gate, not just hiding)

When off:

| Surface | Behaviour |
| --- | --- |
| `/erp/v1/*` (external ERP clients) | Middleware in front of the mounted handler: **503** with the ERP error envelope, code **`ERP_DISABLED`** (add it to `erp.AllCodes` and any code docs/tests that enumerate codes). Checked **before** key authentication. |
| `/vendor/integrations*`, `/customer/integrations*` (GET and POST) | **404** — same as a non-existent route. Middleware on the route groups in `vendor_routes_erp.go` and `customer_routes.go`. |
| `/admin/integrations*` | Still reachable for staff with `platform.integration.view`; shows a clear "التكامل متوقف حالياً" banner with a link to the switch. Revoke stays allowed. |
| Sidebar (vendor + pharmacy) | Hidden at render time. `NavSuppressions` is a static map written once at boot — add a runtime hook (e.g. `rbac.NavSuppressedFunc func(key string) bool` set by the composition root, consulted in `nav.go` alongside the map) so the vendor/pharmacy `integrations` section disappears the moment the flag flips. Admin nav entry stays. |
| Checkout ERP-code modal (`checkout_erp_code_modal.templ`), the integrations mention on `customer_home.templ`, and any other ERP UI entry point you find (`grep -ri "integrations\|erp" internal/ui/pages/*.templ`) | Not rendered. Any POST endpoint behind them refuses. |
| Worker (`cmd/worker/erp.go`, `modules/erp/jobs`) | ERP sync chunk jobs **snooze** (River `JobSnooze`, e.g. 5 min) while off — never discarded, so turning it back on resumes them. Cleanup may continue. The worker reads the flag through `features` (init it in the worker too) or a direct cheap query. |
| RBAC permissions | Unchanged. The switch is independent of permissions. |

If `cfg.ERP.Enabled` is false, nothing above is mounted anyway; the admin card
shows the switch disabled with "معطّل على مستوى النشر (ERP_API_ENABLED)".

### B3. Admin switch

- A card on `/admin/settings` titled **"تشغيل تكامل الأنظمة"**: current state,
  one toggle, and short text on what turning it off does (hides it from all
  pharmacies and vendors, ERP clients get `ERP_DISABLED`, pending syncs pause).
- `POST /admin/settings/erp-integration` with CSRF, `data-confirm` on the off
  action, permission = whatever already guards `POST /admin/settings` in
  `admin_routes_platform.go` (check; super admin at least). Writes an audit
  log entry the same way other admin settings changes do (find the existing
  audit helper). Redirect back with a bilingual flash notice.
- **Do not** reintroduce the removed generic feature-flags section:
  `admin_settings_feature_flags_removed_test.go` forbids the strings
  "Feature Flags", "مفاتيح تشغيل الميزات والأنظمة", the other flag keys and
  `/admin/settings/features/toggle`. This is one purpose-built card. That test
  must still pass.
- New i18n keys go in the right `internal/shared/i18n/catalog_*.go` file.

### B4. Tests and docs

- Off: vendor and pharmacy integration routes → 404 (GET and POST); sidebar
  has no integrations section for vendor/pharmacy; `/erp/v1` with a valid key →
  503 `ERP_DISABLED`; checkout modal absent; admin page → 200 with banner.
- On again: all of the above restored without a restart.
- Switch handler: missing permission → refused; CSRF enforced; flag persisted;
  audit entry written.
- Worker: chunk job snoozes when off.
- Existing tests `TestErpUIRoutesFollowDisabledFlag`,
  `TestNavSuppressionsHideIntegrationsWhenERPIsDisabled` still pass.
- `docs/modules/erp.md`: a "Runtime switch (2026-11-14)" section.

---

## Order of work and gates

1. Part B (smaller, well-bounded), then Part A.
2. After each part: `templ generate`, `go build ./...`, `go vet ./...`,
   `go test ./...` (integration tests need Postgres — run them if a database is
   reachable, otherwise say so), `golangci-lint run` if installed,
   `make check-provider-isolation`. Skip `check-file-size` (owner waiver).
3. Browser check of `/market-discounts` if a dev server can be started.
4. **No commits, no pushes.** Leave changes in the working tree.
5. Write a report to the path given in your brief: what changed per file, test
   results with real output, anything skipped or not verified, open questions.
