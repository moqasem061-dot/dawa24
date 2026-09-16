# Pagination UI unification — audit & execution plan

> **STATUS: EXECUTED 2026-09-02.** All six phases applied. `go build ./...` and
> `go test ./internal/ui/... ./internal/ui/pages/... ./internal/shared/pagination/...
> ./internal/modules/catalog/...` green. One unrelated pre-existing failure remains
> outside this work: `internal/platform/database` `TestLoadMigrationsUniqueness`
> (duplicate migration version 150 — nothing to do with pagination). See
> "Execution notes" at the end.

**Scope:** make the pagination bar and the "rows per page" control look and
behave identically on every dashboard page (admin, vendor, customer/pharmacy,
platform-admin), using the pagination style that already exists. **No new
design.** The canonical component and CSS below are the standard; everything
else is wired to match it exactly.

**Companion doc:** `PAGINATION_EXECUTION_PLAN.md` covers the *server-side*
`LIMIT/OFFSET` rollout. This doc is purely the *UI layer*: markup, classes,
placement, and the rows-per-page dropdown.

---

## 1. The standard (do not redesign — reuse verbatim)

| Layer | Canonical source | Notes |
|---|---|---|
| Component | `internal/ui/components/pagination.templ` → `components.B2BPagination(PaginationProps)` | Renders info line + rows-per-page `<select>` + first/prev/numbered/next/last controls. |
| CSS | `internal/ui/static/css/foundations.css` → `.b2b-pagination`, `.b2b-pagination-info`, `.b2b-pagination-controls`, `.b2b-pagination-btn` (+ `.active` / `.disabled`, `@media (max-width:640px)`) | The only pagination stylesheet that survives. |
| Server parse | `internal/shared/pagination` → `RowsPerPage(r)`, `PageNumber(r)`, `TableRows` (25), `RowsPerPageOptions` `{10,25,50,100}` | 50 handlers already use this. |
| Rows-per-page options | `components.PageSizeOptions()` → `{10,25,50,100}` | Single source. Selecting a size returns to page 1 (`buildPageSizeURL`). |
| Placement | Immediately **after** `.table-container`'s closing tag, as a sibling — never inside the horizontal scroll container. | Already documented in `PAGINATION_EXECUTION_PLAN.md` §1.2. |

**Canonical call:**

```go
@components.B2BPagination(components.PaginationProps{
    CurrentPage: data.Page,
    PageSize:    data.PerPage,
    TotalCount:  data.Total,
    BaseURL:     "/vendor/products",              // path only
    QueryValues: url.Values{"q": {data.Search}, "status": {data.Status}},
})
```

---

## 2. Audit results

### 2.1 Conformant — 64 `.templ` files (no action)

All files that already call `components.B2BPagination` and feed it from
`pagination.RowsPerPage` / `PageNumber`. Full list: `grep -rln B2BPagination
internal/ui/pages`.

### 2.2 Group B — B2BPagination present **but a duplicate rows-per-page control** in the filter toolbar

These pages render the canonical bar *and* a second, differently-styled
`<select name="limit">` (class `form-select text-xs` / `form-input`) up in the
filter row. Two controls for one setting, inconsistent placement and styling.

| File | Extra select |
|---|---|
| `internal/ui/pages/admin_brands.templ` | `:130` |
| `internal/ui/pages/admin_categories.templ` | `:91` |
| `internal/ui/pages/admin_products.templ` | `:216` |
| `internal/ui/pages/admin_warehouses.templ` | `:340` |
| `internal/ui/pages/admin_match_decisions.templ` | `:155` |
| `internal/ui/pages/customer_decision_memory.templ` | `:145` |
| `internal/ui/pages/vendor_inventory.templ` | `:148` |
| `internal/ui/pages/vendor_products.templ` | `:296` (plus hidden mirror `:145`) |

`admin_cities.templ:198` keeps only a **hidden** `limit` input to preserve the
size across a filter submit — that is fine, leave it.

### 2.3 Group C — fully custom pagination markup, `B2BPagination` not used

| File | Custom construct | CSS used | Rows-per-page options | Has total count? |
|---|---|---|---|---|
| `internal/ui/pages/customer_catalog_results.templ` | `CustomerCatalogPagination` | ad-hoc `btn btn-xs` | `12/24/48/96` (in `customer_catalog.templ:227`) | yes |
| `internal/ui/pages/market_discounts.templ` | inline footer `:374` | ad-hoc `btn btn-sm` | `24/48/96` (`:101`) | yes |
| `internal/ui/pages/smart_order_results.templ` | `.so-pagination` block `:359` | `.so-pagination*` (`customer.css:351`) | `10/25/50/100/-1 "الكل"` (`:279`) | yes |
| `internal/ui/pages/vendor_ingest_review.templ` | `reviewPagination` `:802` | `.so-pagination*` reuse | `10/25/50/100` (`:78`) | yes |
| `internal/ui/pages/saving_import_review.templ` | `savingPagination` `:444` | hand-rolls `.b2b-pagination*` classes | `10/25/50/100` (`:108`) | yes |
| `internal/ui/pages/suppliers_profile.templ` | prev/next only `:626` | `btn btn-secondary btn-sm` | none | page only |
| `internal/ui/pages/admin_import_review.templ` | `importRowsPager` `:387` | `.wiz-toolbar` / `.wiz-chip` | none | yes (`view.Pages`) |
| `internal/ui/pages/admin_temp_warehouses_modals.templ` | Alpine modal pager `:115` | ad-hoc Tailwind | none | yes (client `modalTotalPages`) |

Supporting/partial files that inherit from the above (change with their parent):
`customer_catalog.templ`, `customer_catalog_filter.templ`,
`smart_order_results_row.templ`, `saving_import_wizard.templ`,
`vendor_products_table.templ` (helper only — no markup, leave).

### 2.4 CSS fragmentation

| Selector family | File | Fate |
|---|---|---|
| `.b2b-pagination*` | `foundations.css:300` | **keep — canonical** |
| `.so-pagination*`, `.pagination-current` | `customer.css:351` | delete after Group C phase 2 |
| `.wiz-toolbar` / `.wiz-chip` as a pager | `admin_import_review.templ` local | stop using for pagination (may stay for wizard toolbars) |
| inline `btn btn-sm/xs` pagers | catalog, market_discounts, suppliers | replaced by component |

### 2.5 Option-set fragmentation

* Canonical tables: `{10,25,50,100}`.
* Card-grid pages (`customer_catalog`, `market_discounts`): `{12,24,48,96}` — 4-up
  grid maths. **Decision point (see §4).**
* `smart_order_results`: adds `-1` = "show all".

---

## 3. Execution plan

Ordered by risk/payoff. Each phase ends with `make check` + `templ generate`
(regenerate the `_templ.go` twins) + a visual pass. Heed `AGENTS.md` file-size
gates and the concurrent-tooling hazard (stop `templ --watch`, GitHub Desktop,
other agents before editing — see memory `dawa24-store-concurrent-tooling`).

### Phase 0 — extend the canonical component (additive, ~1 file)

`internal/ui/components/pagination.templ` — add **optional** fields to
`PaginationProps`; zero values reproduce today's behaviour exactly:

| New field | Purpose | Default |
|---|---|---|
| `PageSizeOptions []int` | override `{10,25,50,100}` for grid pages | nil → `PageSizeOptions()` |
| `AllowShowAll bool` | append an "الكل" option mapping to `limit=-1` | false |
| `HXTarget string` / `HXSelect string` | emit `hx-get`/`hx-target` on the links instead of plain `href` for partial-swap pages | "" → plain `<a href>` |

No visual change; same classes, same layout. Add a `pagination_test.go` case per
new field. This is the only place new code is written — everything else is
deletion + rewiring.

### Phase 1 — remove duplicate rows-per-page selects (Group B, 8 files)

For each file in §2.2:

1. Delete the toolbar `<select name="limit">` block (and its wrapping label/div).
2. Keep a **hidden** `<input type="hidden" name="limit" value={ ...PerPage }>`
   inside any filter `<form>` so filtering preserves the chosen size (mirror
   `admin_cities.templ`).
3. Confirm the handler reads `pagination.RowsPerPage(r)` — most already do; fix
   any that still parse `limit` by hand.
4. The B2BPagination bar becomes the *only* rows-per-page control.

Lowest risk, immediately visible consistency win across admin + vendor.

### Phase 2 — replace custom table pagers with B2BPagination (Group C tables)

Files: `saving_import_review.templ`, `vendor_ingest_review.templ`,
`smart_order_results.templ`, `market_discounts.templ` (list/table view).

Per file:

1. Delete the custom `*Pagination` templ func / inline footer and the toolbar
   `<select name="limit">`.
2. Insert `@components.B2BPagination(...)` right after `.table-container`, with
   `QueryValues` carrying **every** filter the page has (search, match level,
   sort, order, tab — the classic dropped-filter bug).
3. Handler: switch `limit`/`page` parsing to `pagination.RowsPerPage` /
   `PageNumber`. For `smart_order_results` pass `AllowShowAll: true` and keep the
   handler's `-1` branch.
4. `market_discounts` / any card-grid view: pass
   `PageSizeOptions: []int{12,24,48,96}` (per §4 decision).
5. Delete now-orphaned `.so-pagination*` / `.pagination-current` from
   `customer.css`; grep to confirm zero references.

### Phase 3 — card-grid pages (`customer_catalog` + `customer_catalog_results`)

1. Replace `CustomerCatalogPagination` with `@components.B2BPagination`, passing
   `PageSizeOptions: []int{12,24,48,96}`.
2. Move the rows-per-page `<select name="page_size">` out of the toolbar
   (`customer_catalog.templ:220-236`); rename param to `limit` for consistency,
   or keep `page_size` and teach the component via a `SizeParam` field —
   prefer renaming to `limit` and updating the handler.
3. Keep `buildCatalogURL` only if it adds params the component can't; otherwise
   rely on `QueryValues`.
4. HTMX: this page swaps `#catalog-results`; pass `HXTarget`/`HXSelect` from
   Phase 0.

### Phase 4 — prev/next-only pages

| File | Approach |
|---|---|
| `suppliers_profile.templ:626` | Handler already has `CurrentPage`; add a `TotalCount` (cheap `COUNT(*)`), then `@components.B2BPagination`. If count is genuinely unavailable, still render the component — it degrades to first/prev/next/last on the page numbers it can compute. |
| `admin_import_review.templ:387` (`importRowsPager`) | Has `view.Page` / `view.Pages` → `TotalCount = view.Pages * view.PerPage` (or thread a real count). Use `HXTarget:"#import-rows-container"`, `HXSelect:"#import-rows-container"` from Phase 0 so the htmx partial swap is preserved. Retire `.wiz-chip` pager markup. |
| `admin_temp_warehouses_modals.templ:115` | Client-side Alpine pager inside a modal. Two options: **(a)** render the modal body as a server fragment and drop `@components.B2BPagination` in (preferred, matches everything else); **(b)** if it must stay Alpine, hand-author the same `.b2b-pagination` / `.b2b-pagination-btn` DOM with `x-bind` so classes/markup match byte-for-byte. Pick (a) unless the modal's live-filter UX forbids a round-trip. |

### Phase 5 — cleanup & verification

1. `grep -rn "so-pagination\|pagination-current\|CustomerCatalogPagination\|savingPagination\|reviewPagination\|importRowsPager\|so-pagination-summary" internal` → expect **0** (or only dead defs to delete).
2. Delete `.so-pagination*` block from `customer.css`.
3. `templ generate` — commit regenerated `_templ.go` files.
4. `make check` (file-size, hardcoded-Arabic, build).
5. Manual visual pass — one page per surface: `/admin/products`,
   `/vendor/inventory`, `/customer/catalog`, `/customer/smart-order/{id}`,
   `/suppliers/{id}`, admin import review, temp-warehouse modal. Confirm:
   identical bar position, identical control styling, identical dropdown
   options, dropdown resets to page 1, all filters survive page change.
6. Add a lint guard (grep in `make check` or a test) that fails if a `.templ`
   under `internal/ui/pages` contains `name="limit"` in a `<select>` — forces
   future pages through the component.

---

## 4. Decision required from product owner

**Card-grid page sizes.** `customer_catalog` and `market_discounts` are 4-column
image grids, not tables; `{10,25,50,100}` produces ragged final rows.

* **Recommended:** keep the component but pass `PageSizeOptions:{12,24,48,96}` on
  those two pages. Same bar, same styling, same placement — only the numbers in
  the dropdown differ, which the grid layout justifies.
* **Alternative (strict uniformity):** force `{10,25,50,100}` everywhere and
  accept partial last rows.

All table pages are unaffected — they use `{10,25,50,100}`.

---

## 5. File-change summary

| Phase | Files touched | Type |
|---|---|---|
| 0 | `components/pagination.templ` (+ test) | add optional props |
| 1 | 8 `.templ` in §2.2 + their handlers | delete duplicate select |
| 2 | 4 `.templ` + 4 handlers + `customer.css` | replace custom pager |
| 3 | `customer_catalog*.templ` + catalog handler | replace custom pager |
| 4 | `suppliers_profile.templ`, `admin_import_review.templ`, `admin_temp_warehouses_modals.templ` + handlers | replace prev/next pager |
| 5 | `customer.css`, `Makefile`/lint | cleanup + guard |

~20 `.templ` files, ~10 handlers, 1 CSS file, 1 lint rule. No new visual design;
one component, one stylesheet, one option set (plus the documented grid
exception), one placement rule — everywhere.

---

## Execution notes (2026-09-02)

**Phase 0 — component.** `components.PaginationProps` gained four optional
fields, all zero-value-compatible: `PageSizeOptions []int`, `AllowShowAll bool`,
`HXTarget string`, `SizeParam string`. Link rendering was refactored into one
`paginationLink` sub-template so first/prev/numbered/next/last share a single
piece of markup. `buildPaginationURL` → `buildPageURL(props, page, limit)` and
now honours `SizeParam` (default `limit`). `PageSize <= 0` renders as
"show all": one page, every row, controls disabled, the "الكل" option selected.

**Phase 1 — duplicate selects removed (8 templates).** admin_brands,
admin_categories, admin_products, admin_warehouses, admin_match_decisions,
customer_decision_memory, vendor_inventory, vendor_products. Each toolbar
`<select name="limit">` became a hidden input that preserves the size across a
filter submit; the B2BPagination bar is now the only rows-per-page control. The
dropped 200 / 250 options were already dead — every one of these handlers parses
through `pagination.RowsPerPage`, which honours only {10,25,50,100}.
`catalog.PageSizes` / `DefaultPageSize` (vendor products listing) were realigned
from {25,50,100,200}/50 to {10,25,50,100}/25.

**Phase 2 — custom table pagers replaced.** market_discounts (grid options
{24,48,96} kept via `PageSizeOptions`), smart_order_results (`AllowShowAll`),
vendor_ingest_review, saving_import_review. Each got a small
`*PaginationQuery`/`*QueryValues` helper carrying every filter through a page
change. The saving-import handlers (customer + vendor) switched from an
unchecked `strconv.Atoi` to `pagination.RowsPerPage` / `PageNumber`.

**Phase 3 — catalogue grid.** `CustomerCatalogPagination` now delegates to
`B2BPagination` with `PageSizeOptions {12,24,48,96}` and `SizeParam "page_size"`
(the screen's pre-existing param, left untouched in the handler and
`buildCatalogURL`). Toolbar "عرض:" select and the `changePageSize` JS removed;
`catalogQueryValues` added beside `buildCatalogURL`.

**Phase 4 — prev/next-only pages.**
- suppliers_profile: handler moved to `pagination.RowsPerPage`/`PageNumber`
  (was hard-coded `limit := 24`), `SupplierProfileData.PerPage` added,
  `supplierCatalogQuery` helper added; bar replaced.
- admin_import_review: `importRowsPager` now renders `B2BPagination` with
  `HXTarget "#import-rows-container"` so the htmx partial swap is preserved;
  `importQuery` carries `limit`; page size is `pagination.RowsPerPage`
  (`importReviewPageSize`) — default drops from a fixed 100 to the platform 25,
  raisable via the control. `importPageURL` deleted (now unused).
- admin_temp_warehouses_modals: client-side Alpine pager reskinned to the exact
  `.b2b-pagination` / `.b2b-pagination-btn` markup with `:class` bindings; still
  Alpine-driven because the modal table is fetched without a round trip.

**Phase 5 — cleanup.** `.so-pagination`, `.so-pagination-summary`,
`.so-pagination-controls`, `.pagination-current`, `.so-filter-limit` deleted
from `customer.css` (`.so-footer-actions` kept and de-duplicated). No template
references any removed selector.

**Not done:** the lint guard (§3 Phase 5 item 6) that would fail CI on a future
`<select name="limit">` in `internal/ui/pages` — add to the Makefile `check`
target when convenient.
