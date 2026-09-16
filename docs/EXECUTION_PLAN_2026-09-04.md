# Dawa24 Store — Execution Plan (18 requirements)

Traced 2026-09-04 against `F:\Dawa 24\dawa24-store` @ working tree and the live
database `postgres-u74003.vm.elestio.app/dawa24_store`.

Every claim below marked **[proven]** was verified by reading the code path
end-to-end and, where SQL is involved, by executing it against the live schema.
Claims marked **[suspected]** need the named diagnostic before the fix is written.

---

## 0. Architecture facts this plan depends on

Read these once; every task assumes them.

| Fact | Where |
|---|---|
| Go modular monolith, `chi` router, `templ` views, `pgx` | `cmd/server/routes.go` |
| Six audience groups mount the UI; each has its own middleware stack | `cmd/server/routes.go:76-146` |
| `RequireCustomer` / `RequireVendor` / `RequireStaff` → **404** on wrong audience | `internal/platform/authctx/audience.go` |
| `RequireTenantPagePermission(...)` → **404** (not 403) when the permission is missing | `authctx/audience.go:104` |
| Sidebar is generated from one registry: `rbac.Nav(scope)` filtered by `rbac.VisibleNav` | `internal/platform/rbac/nav*.go`, `internal/ui/layouts/sidebar.templ` |
| Permission catalogue: `catalog_admin.go`, `catalog_vendor.go`, `catalog_pharmacy.go`; DB tables `identity.permissions`, `org.roles`, `org.role_permissions(role_id, permission_key)` | `internal/platform/rbac/` |
| Tenant queries must run in `db.InTx` / `db.InReadTx`; cross-tenant needs `database.AsSystem(ctx)` | AGENTS.md rule 4 |
| Money is `money.Amount` (minor units), never `float64` | AGENTS.md rule 1 |
| Migrations are `db/migrations/NNN_name.up.sql` + `.down.sql`, never edited after applying. **Next free number: 175** | AGENTS.md |
| River queue exists and is wired (`internal/platform/queue`, `cmd/worker`) but **`public.river_job` has 0 rows** — no import has ever used it | live DB |
| All import wizards use **in-process, in-memory** session stores + detached goroutines | `internal/ui/saving_products_sessions.go`, `team_import_types.go` |

### CI gates every task must keep green (`make check`)

- `check-file-size` / `check-file-size-count`: **0 Go files over 400 lines**. Splitting is mandatory, not optional.
- `check-hardcoded-arabic`: **0** Arabic string literals in `internal/ui`, `internal/modules`, `cmd`. Every new message goes through `i18n.T(lang, key)` with a key added to `internal/shared/i18n/`.
- `check-modal-handwritten`: **0** raw `<dialog>` in `internal/ui/pages/*.templ` — use `components.Modal`.
- `check-modal-legacy`: **0** `modal-overlay` / `window.openModal` / `window.closeModal`.
- `check-error-swallow`: no `err == nil {`, no `x, _ = h.*Svc.`, no `_ = pages.` in `internal/ui/*.go` without `// nolint:errswallow` + reason.
- `check-undefined-classes` ceiling 64, `check-inline-styles` ceiling 11, `check-emoji` ceiling 8 (templates only), `check-important` ceiling 3, `check-deadcode` ceiling 211, `check-unused-components` ceiling 0.
- `test/route_audience_test.go` requires every UI route to be registered by one of the named `Register*Routes` functions.
- `test/visual_regression_test.go` writes PNG baselines; already dirty in the tree — commit or reset before starting so diffs are attributable.

---

## Task 1 — `/customer/team` replaced by the branch-management employees tab

### Area
- Page to remove/replace: `/customer/team` → `internal/ui/customer_team_handlers.go:CustomerTeamPage`, view `internal/ui/pages/tenant_team.templ`.
- Source of the wanted UI: `/customer/branches` tab 2 → `internal/ui/pages/customer_branches_employees.templ` (`CustomerEmployeesTab`), driven by `internal/ui/pages/customer_branches.templ`.
- Handlers: `internal/ui/customer_employee_handlers.go` (create/edit/delete/status), `internal/ui/customer_branch_handlers.go:CustomerBranchesPage`.
- Routes: `internal/ui/customer_routes.go:registerCustomerTeamRoutes` and `registerCustomerCompanyRoutes`.
- Nav: `internal/platform/rbac/nav_pharmacy.go` (`team` item → `/customer/team`, `branches` item → `/customer/branches`).

### What is wrong today
1. Two different employee screens exist over the same data (`org.members` + `identity.users`), with different feature sets. `tenant_team.templ` has no branch assignment, no employee code, no job title edit, no add/edit modal — only a role dropdown.
2. **[proven]** `tenant_team.templ` posts the wrong identifier: `{ActionBase}/%d/delete` uses `m.ID` (the `org.members` row id) but `CustomerEmployeeDeleteSubmit` (`customer_employee_handlers.go:180`) parses `{id}` as a **user id** and calls `orgSvc.RemoveMember(orgID, targetUserID)`. Deleting from `/customer/team` therefore deletes the wrong member or nothing.
3. **[proven]** The same ambiguity exists on the branches tab: `customer_branches_employees.templ` uses `emp.Member.UserID` for `/edit` and `/delete` but `emp.Member.ID` for `/status`. The route set has no per-route documentation of which id it takes.
4. `CustomerEmployeeEditSubmit` calls `AddMemberDirect` (an upsert) with a fully-rebuilt `org.Member`, so any field absent from the form is overwritten with its zero value.
5. Both screens redirect back to `/customer/branches?tab=employees` regardless of where the action was invoked from.

### What to change
1. **Move the tab content into the team page.** Rename `pages.CustomerEmployeesTab` → `pages.CustomerTeamContent(v CustomerTeamView, ...)` in a new file `internal/ui/pages/customer_team.templ`, replacing `tenant_team.templ`. It keeps: toolbar (search + branch filter), the employees `data-table`, `components.B2BPagination`, the add-employee modal, and the per-employee edit modals — but the modals become **one** edit modal populated by JS from `data-*` attributes (see below), not N modals in the DOM.
2. **Keep only two actions in the page header**, per the requirement: `استيراد موظفين Excel` (link to `/customer/team/import`, existing) and `إضافة موظف` (opens `add-employee-modal`, the branch-tab modal moved verbatim). Both must be gated on `pharmacy.team.create` via `layouts.CanSee(ctx, "pharmacy.team.create")`.
3. **Remove the tab system from `/customer/branches`.** In `customer_branches.templ`: delete the `.tabs-nav` block (lines ~208-228), delete `<div x-show="activeTab === 'employees'">@CustomerEmployeesTab(...)</div>` (lines 523-526), delete `filterCustomerEmployees`, `openAddEmployeeModal*`, `openEditEmployeeModal*`, `closeEditEmployeeModal` from its inline script, and drop `activeTab` from `customerBranchManager()` (keep the rest of the Alpine state — the branch add/edit form uses it). Keep the "+ موظف" button per branch card, but make it a link to `/customer/team?branch=<id>` instead of a modal opener.
4. **Delete from `CustomerBranchesData`**: `Employees`, `ActiveTab`, `EmpPage`, `EmpPerPage`, `EmpTotalCount`, and the methods `GetBranchEmployees`, `GetBranchName`. Keep `cbRoleLabelArabic`/`cbRoleBadgeClass` by **moving** them to the new team template (they are used by the employee rows). `CustomerBranchesPage` stops calling `ListEmployeesWithTotal`; the per-branch staff-count pill becomes a single aggregate query `orgSvc.CountMembersByBranch(orgID) map[int64]int` (new repository method, one `GROUP BY branch_id`) so the branches page does not fetch employees at all.
5. **Fix the id contract once.** Decide: **all `/customer/employees/{id}/*` routes take the `org.members.id`.** Change `CustomerEmployeeEditSubmit` and `CustomerEmployeeDeleteSubmit` to resolve the member row first (`orgSvc.GetMember(ctx, orgID, memberID)`), then act on `member.UserID`. Add `GetMember` to `org.Service`/`Repository` if absent. Update both templates to emit `emp.Member.ID` everywhere. Add a doc comment on the route group stating the contract.
6. **Make edit non-destructive.** Replace the `AddMemberDirect` call in `CustomerEmployeeEditSubmit` with a real `UpdateMember(ctx, orgID, memberID, patch)` that only writes the columns present in the form (`branch_id`, `role_key`/`org_role_id`, `job_title`, `employee_code`, `is_active`).
7. **Wire the role dropdown to the tenant's own roles.** The branch tab hard-codes six `role_key` strings; `/customer/team` used `org.roles` ids. The DB has both `org.members.role_key` and `org.members.org_role_id`. Keep both in sync: the modal offers `orgSvc.ListRoles(orgID)` (id + name), and the handler writes `org_role_id` and derives `role_key` from `role.Key`. Call `h.ensureCompanyRoles(ctx, orgID, actor.OrgType)` on page load (it already exists, used by the import path).
8. **Routes.** In `registerCustomerTeamRoutes`, keep `/customer/team` as the single page. Add `g.Get("/customer/branches/employees", redirect → /customer/team)` and keep `/customer/branches?tab=employees` working via a redirect in `CustomerBranchesPage` when `tab=employees` is present (bookmarks, and every `redirectWithNotice` target in `customer_employee_handlers.go` currently points there — change all of them to `/customer/team`).
9. **Nav.** `nav_pharmacy.go`: the `team` item stays (`/customer/team`, perm `pharmacy.team.view`). No new permission needed.

### DB / backend / API
- No schema change. New repository methods only: `GetMember`, `UpdateMember`, `CountMembersByBranch` on `internal/modules/org` (+ `postgres/`).
- Permissions unchanged: `pharmacy.team.{view,create,update,delete}`, `pharmacy.role.assign` already exist and already gate the routes.

### Shared / duplicated code to fix centrally
- `settings_employees_handlers.go` + `pages/settings_employees.templ` are a **third** employee CRUD (`/settings/employees`, `/settings/employees/{id}/delete`, `/settings/employees/assign-manager`). Either delete them or make `/settings/employees` redirect to `/customer/team` / `/vendor/team`. Deleting is preferred; run `make check-deadcode` after.
- `vendor_team_page_handlers.go` + `pages/vendor_team.templ` is the vendor twin. **Do not merge it in this task**, but extract the shared row/modal markup into `pages/team_members_table.templ` so the vendor side can adopt it later without a second rewrite.

### Edge cases
- Employee with `branch_id = NULL` → "غير معين بفرع" (already handled; keep).
- `?branch=<id>` deep-link preselects the branch in the add modal and pre-filters the table.
- Deleting yourself → already blocked in `CustomerEmployeeDeleteSubmit`; keep and also block demoting the last `is_owner` role holder.
- Creating an employee whose email already exists → existing branch reuses the user (`GetUserByEmail`); must not silently move that user out of another organization. Add a guard: if the user is already a member of a *different* org, refuse with a translated message.
- Pagination: `EmpTotalCount` must come from `ListEmployeesWithTotal`; `B2BPagination` `BaseURL` becomes `/customer/team` with no `tab` query value.

### Tests
- `internal/ui/team_import_test.go`, `phase_b_test.go`, `audience_separation_test.go`, `route_guard_audit_test.go`, `test/rbac_guard_test.go` all reference these routes — update.
- New: a table test asserting `/customer/employees/{id}/edit|delete|status` all resolve the same id kind, and that a member of org A cannot address a member of org B (returns 404/forbidden).

---

## Task 2 — Remove `رابط خارجي مخصص` (external URL) from the ads system

### Area
- `internal/ui/pages/vendor_ads_wizard.templ:347` — the `<option value="external_url">`.
- `internal/modules/promo/sponsorship.go:43` — `ClickTargetExternal`.
- `internal/modules/promo/domain.go:253` — the `case ClickTargetExternal:` arm of `ResolveClickURL`.
- DB: `promo.ads.click_target_type TEXT NOT NULL DEFAULT 'vendor_page'` with `CHECK (click_target_type IN ('product','vendor_page','offer','external_url'))` (migration 163).

### What is wrong / what to change
1. Delete the `<option value="external_url">` line.
2. Delete the constant `ClickTargetExternal` and its `case` in `ResolveClickURL`. The `default:` arm already falls back to the supplier page.
3. **Server-side validation is missing entirely** — `VendorAdCreateSubmit` writes whatever `click_target_type` arrives. Add an allow-list in `internal/ui/vendor_ads_handlers.go`: anything not in `{product, vendor_page, offer}` becomes `product`. Removing the option from the `<select>` does not stop a crafted POST.
4. `Ad.TargetURL` is checked *before* `ClickTargetType` in `ResolveClickURL`, so a stored `target_url` still wins. Since external targets are gone, stop writing `target_url` from user input; derive it (or leave it empty and let `ResolveClickURL` build the path).

### Migration (175)
```sql
BEGIN;
UPDATE promo.ads SET click_target_type = 'vendor_page' WHERE click_target_type = 'external_url';
ALTER TABLE promo.ads DROP CONSTRAINT IF EXISTS ads_click_target_type_check;
ALTER TABLE promo.ads ADD CONSTRAINT ads_click_target_type_check
  CHECK (click_target_type IN ('product','vendor_page','offer'));
COMMIT;
```
Live data check already run: `promo.ads` holds only `vendor_page` (1) and `product` (5) — **the UPDATE is a no-op today**, but keep it: migrations run before the new image is promoted (expand/contract), so the old binary must still be able to write, and the old binary can only write the three remaining values once the option is gone. Ship the constraint change in a **later** migration than the code deploy if you want strict expand/contract; otherwise ship together, since no row uses the value.

`.down.sql` restores the four-value constraint.

### Edge cases / tests
- `promo.ads.pending_changes` JSONB may carry `click_target_type: "external_url"` from an unreviewed edit request. The migration cannot CHECK JSONB — add a one-off `UPDATE promo.ads SET pending_changes = pending_changes - 'click_target_type' WHERE pending_changes->>'click_target_type' = 'external_url';`.
- Test: POST `/vendor/ads/new` with `click_target_type=external_url` must persist `product`, not fail with a 500.

---

## Task 3 — `/catalog`: in-stock full products first, on page 1

### Area
- Handler: `internal/ui/customer_handlers.go:CustomerCatalogPage` (route `/catalog` in `public_routes.go:88`, behind `h.scrape.Protect`).
- Service: `internal/modules/catalog/service.go:SearchWithTotal`.
- SQL: `internal/modules/catalog/postgres/repository.go:SearchProducts` (216) and `CountProducts` (316), `catalogOrderBy` (360).
- Card assembly + in-page sort: `internal/ui/customer_catalog_cards.go:buildCatalogVariantCards`.

### What currently causes the problem — **[proven]**
Pagination happens in SQL at the *product* level, ordered by relevance then `sold_times DESC, created_at DESC`. `buildCatalogVariantCards` then re-sorts **only the 24 rows of the current page**, putting orderable cards first *within that page*. Live counts: **19,996 products, 851 with any in-stock variant.** So the ~851 buyable products are scattered across ~833 pages and page 1 is almost entirely "طلب توريد خاص" placeholders.

### What to change
1. Add a leading `ORDER BY` term to `SearchProducts` expressing "has a positive-quantity variant", so the ordering that page 1 shows is the ordering the whole result set has:

```sql
ORDER BY
  (EXISTS (
     SELECT 1 FROM catalog.product_variants pv
     JOIN inventory.stocks st
       ON st.product_variant_id = pv.id AND st.deleted_at IS NULL
     WHERE pv.product_id = catalog.products.id
       AND pv.deleted_at IS NULL
       AND pv.status = 'active'
       AND st.quantity > 0
  )) DESC,
  CASE ... relevance ... END,
  <catalogOrderBy(params.Sort)>
```
   Put the stock term **before** the relevance CASE only when `params.Query == ''`; when the user typed a query, relevance must win or search stops working. Implement as: `stockFirstExpr + ", " + relevanceCase + ", " + catalogOrderBy(...)` when `Query == ""`, and `relevanceCase + ", " + stockFirstExpr + ", " + catalogOrderBy(...)` when `Query != ""`.

2. **Do not touch `CountProducts`** — ordering does not change the total.

3. Extract the `EXISTS(...)` fragment into one Go constant (e.g. `productHasStockSQL`) used by `SearchProducts`, `CountProducts`'s `InStock` filter, and the new order term, so the three cannot drift.

4. `buildCatalogVariantCards`'s in-page sort stays — it orders *variants within* the page, which is still needed. But its "master product placeholder" branch currently only emits when `!inStock && !hasDiscount`; leave as-is.

### DB
Index to make this affordable at 20k products:
```sql
-- migration 175/176
CREATE INDEX IF NOT EXISTS idx_stocks_variant_positive
  ON inventory.stocks (product_variant_id)
  WHERE deleted_at IS NULL AND quantity > 0;
```
(`catalog.product_variants` already has `product_variants_product_idx`.)

### Edge cases
- Filter `in_stock=true` already restricts to the same predicate — with the new ordering the first page is unchanged, which is the correct behaviour.
- `sort=price_asc` etc. must still work *within* the stock tier. Confirm the sort dropdown still visibly reorders.
- Sponsored ranking (`RankedSponsorshipsForProducts`) is applied per page and must not outrank stock — it already cannot, since it only re-sorts inside the page.
- The anti-scrape page-depth cap (`guestListingBounds`, 200 pages) is unaffected.

### Tests
- Repository integration test: seed 3 products, give one stock, assert it is row 1 with `Query: ""` and `Limit: 2`.
- `internal/ui/catalog_location_and_tabs_e2e_test.go` may assert current ordering — update.
- Measure: `EXPLAIN ANALYZE` the new query at `LIMIT 24 OFFSET 0` and `OFFSET 4800`; if the seq-scan cost is unacceptable, precompute a `has_stock boolean` column on `catalog.product_index` and switch `/catalog` to `FastSearch`. Do that **only** if measured, not preemptively.

---

## Task 4 — `خصومات السوق العامة`: cards, and switch the source to admin temp warehouses

This is two changes; do the data source first, then the cards, because the card
fields depend on which table feeds them.

### 4a. Source: temp warehouses only — **[proven the current source is wrong for the requirement]**

**Area**: `internal/modules/compare/postgres/market_discounts.go`, `market_discounts_query.go`, handler `internal/ui/compare_results_handlers_split2.go:MarketDiscountsPage`, route `internal/ui/public_routes.go:137`.

**Current state (deliberate, and now being reversed):** on 2026-09-03 this page was moved *off* `compare.file_rows` and onto `catalog.product_variants ⋈ inventory.stocks`. The doc comment at the top of `market_discounts.go` records why. The requirement asks for the opposite: show the moderator-uploaded **temporary warehouses**, and only those.

**Live data**: `compare.files` has 226 rows with `is_temp_warehouse = TRUE` (75 live: not deleted, not archived), all with `organization_id IS NULL` (moderator uploads); their live `compare.file_rows` count is **47,154**, of which 46,862 are priced and 44,386 carry a discount. Ordinary compare-tool uploads are 54 files / 29,637 rows with `is_temp_warehouse = FALSE`.

**Change**: rewrite the CTE/FROM to

```sql
FROM compare.file_rows r
JOIN compare.files f ON f.id = r.file_id
WHERE f.is_temp_warehouse = TRUE          -- the only thing this page shows
  AND f.deleted_at IS NULL
  AND f.archived_at IS NULL
  AND f.status = 'ready'
  AND r.price > 0
```
- `supplier_name` = `f.supplier_name` (drop `marketSupplierNameSQL`, which reads `org.organizations`; temp-warehouse files have no organization).
- `original_price` = `r.price`; `discount_percent` = `COALESCE(r.discount,0)`; `price_after_discount` = `COALESCE(NULLIF(r.price_after_discount,0), r.price*(100-COALESCE(r.discount,0))/100)`.
- `created_at` = `r.created_at` (or `f.created_at` — see 4b, the card must show "date of adding"; use `f.created_at`, the upload date, and alias it).
- Delete `marketStockCTE` and `marketNetPriceSQL` from this file (keep them only if `LoadMarketOffers` needs them — it does not; it has its own SQL in `market_offers.go`).
- `ListDistinctSuppliers` follows the same FROM.
- `MarketDiscountsFilter.OrganizationID` becomes unused for this query — remove it from the filter or ignore it explicitly with a comment. **Do not** let a vendor's own uploads leak in: that is what `is_temp_warehouse = TRUE` guarantees, and it is the invariant to pin with a test.
- `MarketDiscountRow.AvailableStock` has no meaning for a spreadsheet row. Remove the field from the row struct and from both card layouts, or set it to 0 and hide the badge. Prefer removing — a "متاح: 0" badge on every card is worse than nothing.
- `MarketDiscountRow.VariantID` / the `/catalog/{VariantID}` "طلب/شراء" button becomes wrong: a `compare.file_rows.id` is not a catalog id. Use `r.matched_product_id` when present → `/catalog/{matched_product_id}`; otherwise render a disabled state (there is already a `.market-no-order` class in `components.css:5518`). **Live data: `matched_product_id` is NULL for all 94,308 temp-warehouse rows**, so today every card will be non-orderable. That is correct and honest; the matching pipeline (`compare.EnqueueCatalogMatch`) is what fills it in.
- `MinDiscount`/`MaxDiscount` filters move from `v.discount` to `r.discount`; `MinPrice`/`MaxPrice` from the variant net-price expression to the row's `price_after_discount`.

**Index** (migration): the page filters and sorts over `compare.file_rows` joined to a small `compare.files` subset.
```sql
CREATE INDEX IF NOT EXISTS idx_compare_files_temp_live
  ON compare.files (id) WHERE is_temp_warehouse AND deleted_at IS NULL AND archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_compare_file_rows_file_price
  ON compare.file_rows (file_id, price) WHERE price > 0;
```

**Upload-mapping defect to fix alongside — [proven]**: `internal/ui/admin_temp_warehouse_upload.go:86` does `headers := rawRows[0]` and never calls `productmatch.AnalyzeLayout`. Files with title rows above the header get mis-mapped. Live evidence: rows in file "اكتف شبين الكوم نقدى 27.88" have `sku = "24.5"`, `"21"`, `"13"` (numbers, i.e. a discount/price column landed in the code column) and `discount = 0` with `price_after_discount = price`. Replace with the same `productmatch.AnalyzeLayout(rawRows)` + `layout.Headers` / `layout.FirstDataRow` the team and saving importers use. This is the reason the product code on these cards is garbage — which is also why requirement 4 asks to remove it.

### 4b. Card changes

**Area**: `internal/ui/pages/market_discounts.templ`, CSS `internal/ui/static/css/components.css:5359-5600`.

| Requirement | Change |
|---|---|
| Remove product code from **both** card types | Delete the `market-list-meta` `market-code` span (list view, lines ~176-181) and the whole `<div class="market-card-code">` block (grid view, ~236-243). Remove `.market-card-code` from CSS; keep `.market-code` only if another page uses it (`grep -rn "market-code"` — `layout.css:744` references it, remove that too). Drop `SKU` from the SELECT and from `MarketDiscountRow` if nothing else reads it. |
| `سعر الجمهور` without the `------` | `.market-money-was { text-decoration: line-through }` → remove the `text-decoration` and `text-decoration-thickness` declarations (and the `.is-empty` override that undoes them). |
| `سعر الجمهور` font bigger than `السعر بعد الخصم` | `.market-money-was` → `font-size: 1.125rem; font-weight: 900; color: var(--text-primary)`. Since the next bullet removes `السعر بعد الخصم` entirely, this becomes "the public price is the primary number on the card". |
| Remove `السعر بعد الخصم` from both cards | List view: delete the third `.market-money-cell.is-net`. Grid view: delete the second `.market-card-rail-row`. Keep the discount pill (`الخصم %`) — the requirement removes the *net price*, not the discount. Then delete `.market-money-net` and `.market-card-rail .market-money-net` from CSS **only if** no other template uses them (`grep -rn "market-money-net"`). |
| Show the date of adding in card view | Grid view has no date. Add `<span class="market-list-date tabular-nums">{ item.CreatedAt.Format("2006-01-02") }</span>` into `.market-card-head` (or a new `.market-card-foot-meta`). List view already has it — keep. |
| `المستودعات المؤقتة` must actually show | Covered by 4a. |

**Also update**: the header subtitle "أصناف الموردين المتوفرة فعلياً في المخازن…" and the empty state "الصفحة تعرض أصناف الموردين المتوفرة في المخازن فقط…" now lie. Rewrite both to name temporary warehouses. The advanced-range label "السعر بعد الخصم (ج.م)" should become "السعر بعد الخصم للفلترة" or just "السعر" since the column is no longer shown.

### Edge cases
- 47k rows: `COUNT(*) OVER()` in the paginated query is already there; verify it does not table-scan badly with the new indexes.
- `discount = 0` on ~6% of rows → pill shows `0%`. Decide: hide the pill when zero.
- Supplier filter list: `ListDistinctSuppliers` over 75 files returns ≤75 names; fine.
- The page is gated on `billing.FeatureMarketDiscounts` for non-staff (`compare_results_handlers_split2.go:112`) and refuses customers outright. Unchanged.

### Tests
- `internal/modules/compare/postgres/market_discounts_query_test.go` asserts the stock CTE is always present (`TestMarketDiscountsQueryAlwaysGatesOnStock`). **Rewrite it** to assert `is_temp_warehouse = TRUE` is always present and that no request parameter can remove it.
- `internal/ui/compare_upload_e2e_split2_test.go:303` and `_split3_test.go:33` render `/market-discounts` — update expectations.
- New test: a `compare.files` row with `is_temp_warehouse = FALSE` never appears, whatever the filter.

---

## Task 5 — Overhaul the "add ad" wizard (responsive modal + product picker)

### Area
`internal/ui/pages/vendor_ads_wizard.templ` (694 lines, one `templ` + one edit modal), CSS `components.css:241-400` (modal system), `components.css:2655` (`.modal-backdrop`), `components.css:5035` (`.modal-content`).

### What currently causes the problem — **[proven]**
`.modal-dialog` is `display:flex; flex-direction:column; max-height:calc(100dvh - 3rem); overflow:hidden`. Its children are: `.modal-header`, the step-indicator `<div>`, and **`<form class="m-0">`**. The form has no `display:flex`, no `flex:1 1 auto`, no `min-height:0`. `.modal-body` inside it has `overflow-y:auto; flex:1 1 auto; max-height:calc(100dvh - 11rem)` — but those apply relative to the *form*, which is a plain block that grows to its content. The form therefore overflows the dialog and is clipped by `overflow:hidden`; the `.modal-footer` (Next / Submit buttons) is what gets clipped off the bottom. On step 2 (four placement radio cards + two selects) and step 3 (dropzone + four text fields) this is reproducible at any viewport under ~900px tall.

### What to change

**5.1 — Fix the modal shell once, for every Alpine modal.** In `components.css`, add:
```css
.modal-dialog > form,
.modal-content > form {
  display: flex;
  flex-direction: column;
  min-block-size: 0;
  flex: 1 1 auto;
  overflow: hidden;
}
```
and change `.modal-body`'s `max-height` from a viewport calculation to `max-block-size: none` when it is a flex child (the flex parent already bounds it). Keep the `@media (max-width:768px)` `.modal-body { max-height: calc(92dvh - 8rem) }` rule as a floor. This fixes the ads wizard, the ad edit modal, and every other `.modal-dialog`/`.modal-content` that wraps a form — `grep -rn "modal-dialog\|modal-content" internal/ui/pages` to enumerate them and re-check each.

**5.2 — Make the step body the only scroller.** `.modal-body` keeps `overflow-y:auto`; the step-indicator strip becomes `flex-shrink:0`; the footer already is. Add `overscroll-behavior: contain` so scrolling the body does not scroll the page behind it.

**5.3 — Responsive step indicator.** `d-grid grid-cols-4` at 4×`text-2xs` labels does not fit under 420px. Under `--bp-sm`, collapse to a single line: `الخطوة 2 من 4 · الموضع والحملة` with a hairline progress bar. Use `components.Stepper` if it fits (`components/stepper.templ` exists and `.stepper` is defined in `components.css:5053`) — prefer reusing it over a fifth bespoke indicator.

**5.4 — Placement cards**: `d-grid grid-cols-2` → `grid-auto-fit-sm` (already a project utility) so they stack on phones.

**5.5 — Rework the product picker.** Current implementation embeds **every in-stock item as JSON in the `x-data` attribute** (`InStockItemsToJSON(data.ItemOptions)`), filters client-side, and slices to 15. Problems: (a) the whole inventory is inlined into the HTML on every page load; (b) `@click.outside="showDropdown = false"` fires before the item's `@click`, which is the "unstable" behaviour — the dropdown closes and the selection is lost on some browsers; (c) no keyboard navigation, no `aria-*`, no loading/empty distinction; (d) `filteredItems` re-filters the full array on every keystroke.

Replace with a **shared combobox component** `internal/ui/components/product_picker.templ`:
- Server endpoint: reuse `GET /vendor/catalog/search-json?q=` (`vendor_catalog_routes.go:23`, `VendorCatalogSearchJSON`) — add an `&in_stock=1` parameter so it returns only variants with positive stock, matching the ads eligibility rule.
- Debounced fetch (250 ms), `AbortController` on each new keystroke, minimum 2 characters, explicit `جارٍ البحث…` / `لا توجد نتائج` states.
- Use `@mousedown.prevent` on options (not `@click`) so the option is chosen **before** the input blurs — this is the stability fix.
- Keyboard: `ArrowDown`/`ArrowUp`/`Enter`/`Escape`, `role="combobox"` + `aria-expanded` + `aria-activedescendant`, `role="listbox"`/`role="option"`.
- Emits into a hidden `<input name="click_target_id">` plus a visible selected-chip with a "تغيير الصنف" button.
- Adopt the same component in `pages/vendor_products_modals.templ` (`searchMasterCatalogLive`, which is a fourth hand-rolled copy of the same thing) and in the sponsorship-request item picker.

**5.6 — Client validation must not be the only validation.** `nextStep()` blocks on `!selectedProduct` and `!titleAr`, and the submit button is `:disabled` on credits. `VendorAdCreateSubmit` must independently reject: missing/foreign `click_target_id`, `title_ar` empty, `duration_days` outside `{7,15,30,60}`, `position` outside the five known values, and insufficient credits. Today it does not.

### Edge cases
- Zero in-stock items → the wizard must open and say so, not present an empty search box.
- Vendor with 0 credits → step 4 already warns; the submit must also be refused server-side.
- Video upload > `MaxUploadBytes` → currently silently produces no media; return a translated error.
- RTL: `position-absolute start-3` on the search icon is already logical; verify the dropdown's `position-absolute z-50 w-full` does not overflow the dialog's `overflow:hidden` — after 5.1 it will be clipped. Give the dropdown `position: fixed` anchored by JS, or remove `overflow:hidden` from `.modal-body` in favour of `overflow: auto` with the dropdown rendered inside the scroll area. **This is the one interaction 5.1 can break; test it explicitly.**

### Tests
- `internal/ui/vendor_ads_wizard_test.go` exists — extend with: renders with 0 items; the `external_url` option is gone (Task 2); the form element carries the flex classes.
- Manual matrix: 360×640, 390×844, 768×1024, 1440×900, each in light and dark, RTL and LTR.

---

## Task 6 — `/auth/register`: governorate/city as a searchable input

### Area
`internal/ui/pages/auth.templ:153-166` (the only `city_id` `<select>`, currently inside the **job-seeker-only** block `data-type-visibility="job_seeker"`), `internal/ui/pages/register_form.go` (`CityID`, `CitySelected`), the map picker `internal/ui/components/map_picker.templ:60-107` (a governorate `<select>` of 27 hard-coded `lat,lon` options), `internal/ui/static/js/registration.js:411-435` (`[data-city-selector]` syncing), handler `internal/ui/auth_handlers*.go`.

### What is wrong
1. There are **two** location controls on this page with different vocabularies: the job-seeker city `<select>` (real `platform_admin.cities` rows, from `h.listCities`) and the map picker's governorate `<select>` (27 hard-coded strings + coordinates, no database ids).
2. Neither is searchable; Egypt has 27 governorates and hundreds of cities, so the city list is a long scroll.
3. The map picker writes into a hidden `branch_city_id` via `registration.js`, matching by **name string**, not id.

### What to change
1. Build a shared, accessible **searchable select** component `internal/ui/components/geo_picker.templ` used by both controls (and reusable by `/customer/branches`, `/vendor/branches`, `/vendor/coverage`):
   - `<input type="text" role="combobox" aria-expanded aria-controls autocomplete="off">` + `<ul role="listbox">` + hidden `<input type="hidden" name="{{name}}">` carrying the id.
   - Data source: `GET /api/geo/governorates` and the existing `GET /api/geo/governorates/{id}/cities` (`vendor_routes.go:88` — currently registered **only inside the vendor group**; move it to `public_routes.go` or a new pre-auth group so the registration page can call it, and keep the vendor alias).
   - Client filtering when the list is already loaded (governorates: 27, always inline); server search for cities (`GET /api/geo/cities?q=&governorate_id=`) — add this endpoint; it must be rate-limited and go through `h.scrape` if it exposes the full city table.
   - Same `@mousedown.prevent` + keyboard contract as Task 5.5. **Write one component, use it in both places** — do not fork.
2. Two chained pickers on the register page: `المحافظة` then `المدينة` (cities filtered by the chosen governorate). Selecting a governorate clears the city.
3. On selection, move the Leaflet marker (reuse `window.syncCityDropdownsWithCoordinates` in reverse: a new `window.setMapFromCity(lat, lon)`), and on map move / "موقعي الحالي", populate both pickers from the nearest city — replacing the string-matching in `registration.js:411-435` with id-based matching against the fetched list.
4. `RegisterFormData` gains `GovernorateID string` and `GovernorateName`/`CityName` for the error re-render; `CitySelected` is replaced by echoing the hidden id and the display name.
5. Server: validate that `city_id` exists and belongs to `governorate_id` before creating the organization/branch. Today `city_id` is taken on trust.

### DB
`platform_admin.governorates` and `platform_admin.cities` already exist. Confirm `cities.governorate_id` is indexed; add `CREATE INDEX IF NOT EXISTS idx_cities_governorate ON platform_admin.cities (governorate_id);` and a trigram index on `name->>'ar'` if the search endpoint uses `ILIKE`.

### Edge cases
- JS disabled / component fails to load → keep a `<noscript>`-friendly fallback: render the real `<select>` and let the component replace it on init (progressive enhancement). This also keeps the form submittable in old browsers (Task 11).
- Validation failure re-render must restore both pickers' display text, not just the ids.
- Arabic search must be diacritic/alef-insensitive — reuse `platform.normalize_arabic()` (already used by the catalogue search) in the cities endpoint.

### Tests
- `internal/ui/auth_pages_test.go` renders the register page — assert the combobox markup and that no bare `<select name="city_id">` remains.
- Handler test: `city_id` from a different governorate is rejected.

---

## Task 7 — A `اعدادات الحساب` sidebar item for every user type

### Area
- Page: `/settings` → `internal/ui/settings_handlers.go:SettingsIndex`, view `internal/ui/pages/settings_unified.templ`.
- Mount: `RegisterApprovedSharedRoutes` in `internal/ui/handlers.go:204`, inside the Tier-B group (`RequireApproved` only, **no audience gate**) — `cmd/server/routes.go:114-122`.
- Nav registry: `internal/platform/rbac/nav_pharmacy.go`, `nav_vendor.go`, `nav_admin.go`.
- Title strings: `internal/shared/i18n/catalog_frontend_ui_b.go:83-86`.

### What is wrong
1. **[proven]** `SettingsIndex` starts with `if actor.IsVendor() { http.Redirect("/vendor/organization") }` — a vendor can never open their own account settings; they are sent to the company profile, which is a different thing and needs `vendor.organization.view` (404 without it).
2. There is no sidebar item for `/settings` in any of the three navs. The only entry point is the avatar dropdown.
3. The page title is `settings.unified_title` = "إعدادات الحساب والمنشأة" and it carries an **organization tab** with `POST /settings/organization` — a second write path onto `org.organizations` competing with `/vendor/organization` (Task 14) and with `/customer/branches`.
4. **[proven]** The page header hard-codes `<a href="/customer/wallet">` — a 404 for vendors and admins.
5. `RequireApproved` lets staff through (`audience.go:277`), so admins can open `/settings`; but `ShellFor` renders `AdminShell` while the page's organization tab is meaningless for staff.

### What to change
1. **Rename**: `settings.unified_title` → `"اعدادات الحساب"` / `"Account Settings"`; `settings.unified_desc` → describe personal data only.
2. **Remove the organization tab** from `settings_unified.templ` and delete the routes `GET /settings/organization` (redirect) and `POST /settings/organization`, `POST /settings/organization/member*`. Company data lives at `/vendor/organization` (Task 14) and `/customer/branches`. Run `check-deadcode` after removing `SettingsOrgUpdateSubmit`, `SettingsMemberRoleSubmit`, `SettingsMemberAddSubmit`.
3. **Remove the vendor redirect** from `SettingsIndex`. Every authenticated user renders the same three tabs: **الملف الشخصي** (name, email, phone, avatar, password), **الأمان** (sessions, MFA, session plans, delete-account request), **التفضيلات** (`profile.user_preferences`).
4. **Fix the header action**: replace the hard-coded `/customer/wallet` link with a helper that returns `/customer/wallet` for customers, `/vendor/wallet` for vendors, and nothing for staff — and only when `actor.Can("pharmacy.wallet.view")` / `("vendor.wallet.view")`. Put this helper next to `components.orgLink` (Task 10 introduces a shared `navTargets` helper — use it here).
5. **Add the sidebar item to all three navs.** The `NavItem.Perm` field is mandatory and `NavItem.Visible` requires a held permission, so:
   - Add `identity.account.view` (admin scope), `vendor.account.view`, `pharmacy.account.view` to the three catalogues — **or**, simpler and preferred, add an `AlwaysVisible bool` field to `rbac.NavItem` and make `Visible()` return `true` for it. One field, three one-line nav entries, no permission migration, no risk of a role missing the grant. Choose this.
   ```go
   // rbac/nav.go
   func (n NavItem) Visible(s Set) bool {
       if n.AlwaysVisible { return true }
       ...
   }
   ```
   Then add to each nav's "account"/"settings" section:
   ```go
   {Key: "account_settings", Href: "/settings", Icon: "settings",
    NameAr: "اعدادات الحساب", NameEn: "Account settings", AlwaysVisible: true},
   ```
   `rbac_test.go` asserts every nav item names a permission declared in the same scope — **update that assertion** to skip `AlwaysVisible` items.
6. **Route gating**: `/settings` stays in Tier B (`RequireApproved`). That means a *pending* organization's member cannot open it. That is wrong for an account-settings page — a user must be able to change their own password while under review. **Move `/settings`, `/settings/profile`, `/settings/password`, `/settings/security/*`, `/settings/preferences`, `/settings/delete-request` into `RegisterPreApprovalRoutes`** and leave the billing-adjacent ones (`/settings/payment-methods*`) in Tier B. Update `test/route_audience_test.go`'s `audienceOf` map accordingly.
   - Consequence: the unapproved-user sidebar filter in `layouts/sidebar.templ:visibleNav` keeps only items whose key/href contains `document`/`notification`. Add `account_settings` to that allow-list.
7. **Staff**: `AdminShell` gets the same item. `SettingsIndex` must tolerate `actor.OrganizationID == 0` (it already does — every org read is optional).

### DB
None. `profile.user_preferences`, `identity.user_security`, `identity.user_sessions`, `identity.user_mfa`, `identity.session_plans`, `identity.account_deletion_requests` all exist and are already used by this page.

### Edge cases
- Staff have no wallet, no organization → tabs must not render empty panels; guard each block.
- A user in two organizations (`org.user_organizations`) — the settings page is per-*user*, so nothing org-scoped may appear.
- `SettingsProfileSubmit` redirects to `/settings#profile`; keep the fragment working with the Alpine `activeTab` initialiser (it already reads `location.hash`).

### Tests
- `internal/ui/settings_profile_avatar_test.go`, `platform_phase9_test.go`, `phase_a_test.go` touch `/settings/*` — update.
- New: render the sidebar for a vendor, a pharmacy, an admin, and a pending-org member; assert `/settings` appears in all four and that `GET /settings` returns 200 for all four.
- `test/rbac_guard_test.go` and `internal/ui/permission_matrix_test.go` will need the new nav item.

---

## Task 8 — `إضافة عرض خاص` (vendor special offers) never saves

### Root cause — **[proven by executing the statement against the live schema]**

`internal/modules/promo/postgres/repository_split2.go:CreateSpecialOffer` names **15 columns** and supplies **14 value expressions**:

```
columns: organization_id, branch_id, title, description, discount_type, discount_value,
         min_order_amount, total_price, starts_at, expires_at, is_active, is_draft,
         admin_status, image, source                                            -- 15
values : $1,$2,$3,$4, $5,$6,$7,$8, COALESCE($9,now()), COALESCE($10,...),
         COALESCE($11,'active')='active',
         COALESCE(NULLIF($12,''),'approved'), COALESCE($13,''), 'special'       -- 14
```
The expression for `is_draft` is missing. Postgres answers:
```
ERROR: INSERT has more target columns than expressions (SQLSTATE 42601)
```
Verified by running the exact statement. **Every** special-offer creation has always failed. The handler catches the error and shows `h.safeMessage(err, lang)`, which for an internal error is a generic sentence, so the user sees "nothing gets added".

### Second bug — the offers page shows offers with no products — **[proven]**

Three queries reference a column that does not exist:
`internal/modules/promo/postgres/repository_split2.go:160`, `:269`, and `repository_split3.go:109` all select `COALESCE(pv.price, prod.base_price, 0)`. `catalog.products` has **`price`**, not `base_price`. Executed live:
```
ERROR: column prod.base_price does not exist (SQLSTATE 42703)
```
All three call sites discard the error (`pRows, _ := tx.Query(...)`; `if pRows != nil`), so every offer loads with an empty `Products` slice, silently. This is why `GetSpecialOffer`, `ListSpecialOffersByOrg` and the admin list all show offers as empty bundles.

### Third bug — offer products cannot be inserted for an unknown variant

`promo.offer_products.product_id` is `NOT NULL`, and the INSERT supplies
`(SELECT product_id FROM catalog.product_variants WHERE id = $2)`. If `$2` is not
a live variant the subquery yields NULL → `23502 not_null_violation`, aborting the
whole transaction and losing the offer.

### What to change

1. **Fix the INSERT.** Add the missing expression:
   ```sql
   COALESCE($11, 'active') = 'active',        -- is_active
   COALESCE($11, 'active') = 'draft',         -- is_draft   <-- added
   COALESCE(NULLIF($12, ''), 'approved'),     -- admin_status
   COALESCE($13, ''),                         -- image
   'special'                                  -- source
   ```
2. **Fix `base_price` → `price`** at all three sites. Extract the products query into one `const offerProductsSQL` so a fourth copy cannot drift.
3. **Stop swallowing.** Change `pRows, _ := tx.Query(...)` to check the error and return it (or log with `nolint:errswallow` and a reason). `make check-error-swallow` currently passes only because the pattern is `x, _ := tx.Query`, not `h.*Svc.` — fix it anyway, it is the reason this went unnoticed.
4. **Guard the offer-product insert**: resolve `product_id` in Go before the insert (`catSvc.GetVariant`) and skip/reject variants that do not belong to the vendor's organization. A vendor must not be able to bundle another vendor's variant — there is no check today.
5. **`ListSpecialOffersByOrg` has no `source` filter.** `/vendor/offers` is the "عروض خاصة" screen but lists every `promo.offers` row of the org, including `source='standard'`. Add `AND o.source = 'special'` — or, if standard offers are also meant to appear there, label them. Decide and document; the vendor-facing filter chips (`status`) already assume special offers.
6. **`AdminStatus` handling**: `VendorOfferNewSubmit` sets `AdminStatus: "pending"`, but the SQL defaults an empty value to `'approved'`. Since the handler always sets it, this is dead — but the fallback is dangerous. Change the fallback to `'pending'`.
7. **Form validation** in `VendorOfferNewSubmit`: `title_ar` required; `discount_percentage` in 0..100; `total_price`/`min_order_amount` parsed with `money.Parse`, **not** `strconv.ParseFloat` + `int64(x*100)` (AGENTS.md rule 1 — `float64` for money is forbidden and `int64(29.99*100)` = 2998 on some inputs). Same in `VendorOfferEditSubmit` and `VendorOfferLocationNewSubmit`.
8. **`VendorOfferLocationNewSubmit` swallows the error**: `_ = h.promoSvc.AddSpecialOfferLocation(...)` then reports success unconditionally. Fix.
9. **`VendorOfferDeleteSubmit`** likewise (`_ = h.promoSvc.DeleteSpecialOffer(...)` → always "deleted successfully").

### DB
No schema change required. Optional backfill: none (0 rows in `promo.offer_products`, 1 row in `promo.offers`).

Consider dropping the empty, unreferenced `promo.special_offers` table (migration 172 created it; `commerce/postgres/carts.go:60-72` probes for it with `to_regclass` and degrades gracefully, so dropping is safe **after** that probe is removed). Do this last, in its own migration, and only if you confirm nothing else references it.

### Edge cases
- Offer with no products (a pure discount offer) must still save.
- `start_date > end_date` → reject.
- Editing resets `admin_status` to `pending` and the offer disappears from the storefront — that is intended; make sure the vendor is told.
- Multi-branch vendor: `branch_id` must belong to the acting organization.
- Cart/checkout read the offer through `GetSpecialOffer`; fixing the products query changes what `AddOfferToCartSubmit` computes as `unitPrice` (it sums `p.CustomPrice * p.Quantity` when `TotalPrice` is zero). Re-verify Task 16 after this lands — **Task 16 depends on Task 8**.

### Tests
- `internal/ui/offers_e2e_test.go`, `promo_phase8_test.go`, `internal/modules/promo/promo_test.go` — extend.
- **New integration test against real Postgres**: create a special offer with two products, read it back, assert both products and their prices. This test would have caught both bugs; the existing mock-repo tests cannot.

---

## Task 9 — `معاينة الأعمدة` misaligned in the vendor import (and everywhere else it is wrong)

### Root cause — **[proven]**

`internal/ui/pages/vendor_ingest_mapping.go:RawFilePreview()` builds preview "rows" by transposing each column's `Preview` slice:
```go
row[c] = cols[c].Preview[r]
```
`Column.Preview` comes from `productmatch.previewOf(profile)` → `profile.Sample[0:4]`, and `sheet.ColumnProfile.Sample` is appended **only in `observeDistinct`**, i.e. only for values that are **non-empty and not already seen**. Different columns skip different cells, so index `r` of column A and index `r` of column B are cells from **different spreadsheet rows**. The table therefore shows a grid of unrelated values under correct headers — exactly "the columns are not wired to the same positioning as the row data".

Two further defects in the same function:
- `if depth < 0 || len(c.Preview) < depth { depth = len(c.Preview) }` takes the **minimum** across columns. One all-blank column (`len(Preview) == 0`) makes `depth == 0` and the function returns `nil, nil` — **no preview at all**.
- `depth` is capped by `vendorImportRawPreviewRows` after the minimum, not before.

### The correct data already exists
`productmatch.Analysis.Preview` is built by `previewSlice(head.Rows, layout)` (`analyze.go:96, 187`): real spreadsheet rows starting at `layout.HeaderRow`, each `padRow(row, layout.Width)`. It is row-aligned and width-padded. It is simply not used by the preview.

### What to change
1. Rewrite `RawFilePreview()`:
   ```go
   func (v VendorImportView) RawFilePreview() ([]string, [][]string) {
       if v.Analysis == nil || len(v.Analysis.Preview) == 0 { return nil, nil }
       headers := v.Analysis.Layout.Headers          // already padded to Width
       rows := v.Analysis.Preview
       if v.Analysis.Layout.HeaderRow >= 0 && len(rows) > 0 {
           rows = rows[1:]                            // Preview[0] is the header row
       }
       if len(rows) > vendorImportRawPreviewRows { rows = rows[:vendorImportRawPreviewRows] }
       return headers, rows
   }
   ```
   Placeholder labels (`العمود N`) for empty headers move into the shared component, not here.
2. **Fix the component to own the padding and the placeholder.** `internal/ui/components/import_preview.templ` already has `padPreviewRow`. Add a `padPreviewHeaders(headers []string, width int)` and derive `width = max(len(headers), max(len(row)) over rows)` so a row *wider* than the header row is not truncated (today `padPreviewRow` truncates with `row[:width]`, hiding real data).
3. **Audit the other five call sites** — the same bug class, same component:
   | Call site | Data source | Verdict |
   |---|---|---|
   | `pages/team_import_wizard.templ:238` | `sess.Headers` = `layout.Headers`, `sess.SampleRows` = `dataRows[:5]` (`customer_team_handlers.go:180-200`) | Row-aligned. **But** `headers` comes from `layout.Headers` (width-padded) while `sampleRows` come from `rawRows[dataStart:]` (ragged) → widen/pad in the component. |
   | `pages/saving_import_wizard.templ:230` | same shape | same fix |
   | `pages/admin_import_mapping.templ:49` | `view.FileHeaders`, `view.FileSampleRows` — verify the builder | check and align |
   | `pages/admin_product_images_import.templ:206` | `Session.Headers/SampleRows` | check and align |
   | `pages/smart_order_steps.templ:95` | `data.Headers`, `data.Preview` | check and align |
   The rule to enforce everywhere: **the preview is raw file rows, padded to a single width, with the header row removed exactly once.**
4. Add a `// Preview must be row-aligned raw cells — never a per-column value sample.` comment on `components.ImportFilePreview`.

### Also (RTL/Arabic, since the requirement mentions it)
- `.import-preview-scroll` must be `overflow-x: auto` with `direction: rtl` inherited; verify the row-number column stays first in reading order (it is `<th class="import-preview-rownum">` first, which in RTL renders right-most — correct).
- Cells with Latin content inside an RTL table need `dir="auto"` on the `<td>` or numbers reorder visually. Add `dir="auto"` to the preview `<td>`.

### Tests
- Unit test for `RawFilePreview` with a fixture where column 2 has a blank cell in row 1 and a repeated value in row 3 — assert the output equals the raw rows.
- Golden test for `padPreviewHeaders` with a ragged sheet.

---

## Task 10 — Avatar/profile dropdown must show only reachable pages, for every audience

### Area
`internal/ui/components/user_menu.templ` (the whole file), plus `dashboardURL`, `accountEyebrow`, `orgLink` at its foot.

### What is wrong — **[proven by comparing each `href` to its route gate]**

| Menu item | Rendered when | Route gate | Result |
|---|---|---|---|
| `/orders` "طلبات التوريد" | `actor.IsVendor()` | registered only in `RegisterCustomerRoutes` behind `RequireCustomer` | **404 for every vendor** |
| `/orders` "طلباتي" | `actor.IsCustomer()` | `pharmacy.order.view` | 404 without the grant |
| `/customer/wallet` | `actor.IsCustomer()` | `pharmacy.wallet.view` | 404 without the grant |
| `/suppliers/followed` | `actor.IsCustomer()` | `pharmacy.supplier.view` | 404 without the grant |
| `/vendor/wallet` | `actor.IsVendor()` | `vendor.wallet.view` | 404 without the grant |
| `/settings` | always (approved) | Tier B; for vendors `SettingsIndex` redirects to `/vendor/organization`, gated on `vendor.organization.view` | 404 for a vendor without that grant |
| `/customer/documents` / `/vendor/documents` (pending branch) | `!IsOrgApproved()` | Tier C, no permission gate | OK |
| `dashboardURL(actor)` | always | `/admin/dashboard` ungated for staff; `/vendor/dashboard` needs `vendor.dashboard.view`; `/customer/dashboard` needs `pharmacy.dashboard.view` | 404 possible |
| `orgLink(actor)` | already `Can()`-checked | — | the only correct one |

Admins get no items at all beyond `/settings` and the dashboard link.

### What to change
1. **Build the menu from the same registry the sidebar uses.** Add to `internal/platform/rbac`:
   ```go
   // AccountMenu returns the account-menu entries a holder may reach.
   func AccountMenu(scope Scope, held Set, approved bool) []NavItem
   ```
   declared in three new files `menu_admin.go` / `menu_vendor.go` / `menu_pharmacy.go`, each naming `Perm` exactly as the route gate does. `user_menu.templ` then renders `for _, it := range rbac.AccountMenu(...)`. This makes hiding and authorization read the same list, and a test can walk it.
2. **Correct the vendor orders link** to `/vendor/orders` (perm `vendor.order.view`).
3. **`dashboardURL`** must return `""` when the actor holds no dashboard permission, and the menu then renders the highest-priority item they *can* reach (documents, then notifications).
4. **`/settings`** becomes always-visible (Task 7 removes the vendor redirect, so it is genuinely reachable by everyone).
5. **Admin entries**: give staff a real group — `/admin/dashboard` (`platform.dashboard.view`), `/admin/approvals` (`org.approval.view`), `/admin/organizations` (`org.organization.view`), `/admin/notifications` (`notifications.center.view`) — each `Perm`-gated.
6. **Do not rely on hiding.** Every route named in the menu already has a gate; the point of this task is that hiding and gating agree. Add a test that walks `rbac.AccountMenu` for a matrix of actors and asserts each `Href` resolves to a route whose gate the actor satisfies.

### Shared fix
The same `Href` → `Perm` mapping is needed by Task 7 (the settings header wallet link) and by `pages/customer_home.templ` and the mobile drawer if they hard-code links. `grep -rn 'href="/customer/\|href="/vendor/\|href="/admin/' internal/ui/components internal/ui/layouts` and route each through the registry.

### Edge cases
- Pending / rejected / suspended organizations: `IsOrgApproved()` is false; the menu must show documents + notifications only, and `dashboardURL` already redirects them to `/documents`. Keep.
- Staff: `IsOrgApproved()` returns `true` unconditionally (`actor.go:117`) — do not use it as a proxy for "has a company".
- A member of two organizations after `GET /org/switch/{id}` — permissions are re-resolved per request by `ResolveTenant`, so the menu is correct automatically.

### Tests
- `internal/ui/permission_matrix_test.go` and `route_guard_audit_test.go` are the right homes.
- New: for each of {super_admin, support-with-2-perms, vendor owner, vendor warehouse clerk, pharmacy owner, pharmacy counter assistant, pending vendor}, render the menu, extract every `href`, issue a `GET` for each through the real router, and assert **200 or 3xx, never 404**.

---

## Task 11 — `موقعي الآن` cross-browser, and a platform compatibility pass

### 11a. The geolocation button

**There is exactly one implementation** — `internal/ui/static/js/maps.js:382-417`. The "duplication" is in markup: eight templates emit a button and rely on `maps.js` finding it:
`components/map_picker.templ:109` (`data-locate-me-btn`), `pages/admin_cities.templ:446,530,695,805`, `pages/customer_branches.templ:473`, `pages/customer_branch_form.templ:258`, `pages/vendor_branches.templ:339`, `pages/vendor_branch_form.templ:312`, `pages/vendor_offer_locations.templ:79` — all `data-map-locate` or `data-locate-me-btn`.

**Why it fails outside Chrome (in likely order):**
1. **Insecure context.** `navigator.geolocation` is unavailable in Safari and Firefox on plain HTTP for any host; Chrome exempts `localhost`. `.env` has `APP_BASE_URL=http://localhost:8080` and `SESSION_SECURE=false`. If staging/production is served over HTTP or with mixed content, the API is simply absent in Safari. **Verify the deployed scheme first** — this is the single most likely cause and no JS change fixes it.
2. **Binding time.** `locateBtn` is resolved once, inside `initMapPickers()`, from `container.querySelector(...) || parentScope.querySelector('[data-map-locate]')`. Buttons that are added later (Alpine `x-if` templates, modals opened after load) never get a listener. `initMapPickers` is not re-run on DOM mutation.
3. **`parentScope` fallback is `document`**, so with two pickers on one page (e.g. `admin_cities.templ` has four) the *first* button can be bound to the *second* map, or bound twice.
4. **`enableHighAccuracy: true, timeout: 10000, maximumAge: 0`** on iOS Safari with a cold GPS commonly returns `TIMEOUT`; the code then shows "تعذر جلب موقع GPS" which reads as "the button does not work".
5. **`locateBtn.innerHTML = '<span>📍 موقعي الحالي</span>'`** overwrites the button's original label (which differs per template — "موقعي", "تحديد موقعي الحالي GPS") on success *and* on error, so after one click every button says the same thing.
6. `Permissions-Policy: geolocation=(self)` (`httpx/middleware.go:185`) is correct for same-origin, but blocks the API in any iframe. Confirm no picker renders inside an iframe.

**Fix, once, in `maps.js`:**
- Replace the per-map binding with **one delegated listener**: `document.addEventListener('click', e => { const btn = e.target.closest('[data-map-locate],[data-locate-me-btn],.btn-locate'); ... })`, resolving the owning `[data-map-picker]` via `btn.closest('[data-map-picker]')`. This fixes (2) and (3) and makes late-rendered buttons work.
- Feature-detect properly and explain: if `!window.isSecureContext` say "تحديد الموقع يتطلب اتصالاً آمناً (HTTPS)"; if `!navigator.geolocation` say the browser does not support it. Two different messages, two different fixes.
- Two-stage acquisition: first call with `{enableHighAccuracy:false, timeout:6000, maximumAge:60000}`; on success refine with a second `{enableHighAccuracy:true, timeout:15000}`. This is what makes it work on Safari/iOS.
- Handle `err.code`: `PERMISSION_DENIED` (1) → tell them how to re-enable in Settings; `POSITION_UNAVAILABLE` (2); `TIMEOUT` (3) → offer "حاول مرة أخرى".
- Save and restore `btn.innerHTML` instead of overwriting it; use `btn.dataset.originalLabel`.
- Also try `navigator.permissions.query({name:'geolocation'})` (guarded — Safari added it late) to pre-empt a denied state without triggering a prompt.
- Move the two remaining emoji out of `maps.js` (they are in JS, not `.templ`, so `check-emoji` does not see them, but the design system says icons).

**Then normalise the markup**: give all eight buttons the same attribute (`data-map-locate`), the same label key, and the same classes. Keep `data-locate-me-btn` in the selector for one release, then remove.

### 11b. Platform / browser compatibility pass

Findings from a census of the stylesheets and scripts:

| Risk | Count | Where | Action |
|---|---|---|---|
| `oklch()` colours | 157 | all stylesheets | Safari ≥15.4, Firefox ≥113, Chrome ≥111. **No fallback anywhere.** On an older browser every colour is `unset` → unreadable page. Add a plain `rgb()`/`hex` declaration immediately before each `oklch()` in `tokens.css` (the token layer only — the 157 uses read tokens), or wrap the token block in `@supports (color: oklch(0 0 0))` with a hex fallback block before it. |
| `color-mix()` | 66 | components/nav/public | Safari ≥16.2. Same treatment: precede with a static fallback. |
| `dvh` units | 10 | modal sizing, shells | Safari ≥15.4. Precede each with a `vh` fallback (`max-height: calc(100vh - 3rem); max-height: calc(100dvh - 3rem);`). |
| `:has()` | 7 | — | Safari ≥15.4, Firefox ≥121. Verify each is progressive (loses polish, not function). |
| `text-wrap: balance/pretty` | 8 | utilities | Purely cosmetic; safe. |
| `backdrop-filter` | 9 | modal/overlay | **0 `-webkit-backdrop-filter`** anywhere. Safari needs the prefix. Add it. Note `make check-backdrop-filter` has a ceiling of 4 "including vendor prefix" — raise the ceiling deliberately and record why. |
| `@layer` | 32 | every stylesheet | Safari ≥15.4. An unsupported browser ignores `@layer` blocks entirely → **completely unstyled site**. This is the highest-impact item; decide the minimum supported Safari version and state it in `docs/`. |
| `<dialog>` + `showModal()` | many | `components.Modal` | Safari ≥15.4, Firefox ≥98. Consistent with the above baseline. |
| `e.explicitOriginalTarget` | 1 | `app.js:462` | Firefox-only property; `undefined` elsewhere, so the `\|\|` fallback covers it. Leave, but comment. |
| `Intl`/`toLocaleString('en-US')` | `import-progress.js` | fine | — |
| `scrollbar-gutter` | 2 | — | Safari ≥16; cosmetic. |
| `aspect-ratio` | 1 | — | fine. |

**Deliverable for 11b**: a short `docs/BROWSER_SUPPORT.md` stating the baseline (proposal: Chrome 111+, Edge 111+, Safari 16.4+, Firefox 113+, iOS Safari 16.4+), plus the fallback work above, plus `.lighthouserc.json` already exists — add a second Lighthouse run in mobile emulation.

**Manual matrix to run at the end**: Windows (Chrome, Edge, Firefox), macOS (Safari, Chrome), iOS Safari, Android Chrome. Pages: `/`, `/auth/register`, `/catalog`, `/cart`, `/checkout`, `/orders/{id}`, `/customer/branches`, `/vendor/products`, `/vendor/ads`, `/market-discounts`, `/settings`. Check in both themes and both directions.

---

## Task 12 — Adding/editing a wallet payment method: `نوع وسيلة الدفع غير صالح`

### Root cause — **[proven]**

`internal/ui/pages/wallet_modals.templ:147` posts the field **`provider`**:
```html
<select name="provider" x-model="paymentType" required>
  <option value="bank">…</option>
  <option value="instapay">…</option>
  <option value="vodafone_cash">…</option>
  <option value="card">…</option>
</select>
```
`internal/ui/settings_payment_handlers.go:18` reads **`type`**:
```go
payType := strings.TrimSpace(r.PostFormValue("type"))
switch payType { case "bank": … case "instapay": … case "wallet", "vodafone_cash": … case "card": …
default: return "", "", fmt.Errorf("%s", i18n.T(lang, "payment.invalid_type")) }
```
`type` is never sent, so the switch always hits `default`. This affects both `/customer/wallet` and `/vendor/wallet` because both post to `TenantPaymentMethodAddSubmit`, which calls the same `buildPaymentMethodIdentifier`.

**Editing is unreachable**: `POST /settings/payment-methods/{id}/edit` and `/default` exist (`handlers.go:241-242`) but no template renders a form for them. `pages/wallet.templ:277` renders only the delete form. So "edit anyone" fails because there is nothing to submit.

### What to change
1. `buildPaymentMethodIdentifier`: read `type` **or** `provider`:
   ```go
   payType := strings.TrimSpace(r.PostFormValue("type"))
   if payType == "" { payType = strings.TrimSpace(r.PostFormValue("provider")) }
   ```
   Keep both names accepted permanently — this function is called from three handlers and any of them may be reached from an older cached page.
2. Rename the form field in `wallet_modals.templ` to `type` for consistency, and keep the `x-model="paymentType"` binding.
3. **Normalise the stored provider.** The switch returns `"wallet"` for both `wallet` and `vodafone_cash`, so the DB will hold `wallet` while the form option says `vodafone_cash`; the edit form must map back. Add `func normalizePaymentProvider(s string) string` and `func paymentProviderFormValue(stored string) string` so display, storage and re-edit agree. Live data: `billing.user_payment_methods.provider` currently holds only `bank` (1) and `instapay` (1); there is no CHECK constraint, so no migration is needed — but add one:
   ```sql
   ALTER TABLE billing.user_payment_methods
     ADD CONSTRAINT user_payment_methods_provider_check
     CHECK (provider IN ('bank','instapay','wallet','card'));
   ```
   after normalising any stray values.
4. **Build the edit modal.** `billing.user_payment_methods` stores only `provider` + `account_identifier` (a rendered string like `"CIB • أحمد • IBAN: EG12"`). You cannot round-trip that back into fields. Two options:
   - **(a) Preferred, small:** add a `details JSONB NOT NULL DEFAULT '{}'` column, write the structured fields alongside the rendered identifier, and populate the edit form from it. Backfill is not required — an old row simply opens with empty sub-fields and the rendered identifier shown read-only until re-saved.
   - **(b) Minimal:** make "edit" a delete-and-recreate. Rejected — it loses `is_default` and `created_at` and is what the user is complaining about.
   Take (a). Migration 175+:
   ```sql
   ALTER TABLE billing.user_payment_methods ADD COLUMN IF NOT EXISTS details JSONB NOT NULL DEFAULT '{}'::jsonb;
   ```
5. Add `POST {base}/wallet/payment-methods/{id}/edit` and `/default` to **both** `registerCustomerBuyingRoutes` (`pharmacy.wallet.manage`) and `registerVendorCommerceRoutes` (`vendor.wallet.manage`) — today only `/settings/payment-methods/{id}/edit` exists, in Tier B, with no wallet-permission gate at all. That is also an authorization gap: a member without `*.wallet.manage` can add a payment method through `/settings/payment-methods`. **Gate the `/settings/payment-methods*` routes or remove them** in favour of the wallet ones.
6. `SettingsPaymentMethodEditSubmit` must verify the method belongs to `actor.UserID` before updating. Check `billSvc.UpdatePaymentMethod` — if it does not scope by user id, add it.
7. Card numbers: the current code stores `"Visa (•••• 4444)"`. Never store the full PAN. Confirm `card_number` is not persisted anywhere (it is not, today) and add a comment.

### Edge cases
- `is_default`: setting a new default must clear the previous one in the same transaction.
- Deleting the default method when others exist → promote the oldest remaining.
- A payment method referenced by a pending `billing.wallet_deposits` row — decide whether delete is a soft delete. Check `DeletePaymentMethod`.

### Tests
- `internal/ui/tenant_wallet_test.go`, `vendor_payments_e2e_test.go` — extend.
- New: POST with `provider=bank` and with `type=bank` both succeed; POST with `provider=bitcoin` fails with the translated message; a user cannot edit another user's method.

---

## Task 13 — `تعديل تفاصيل الأصناف` destroys data and ignores fields

### Area
Modal `internal/ui/pages/vendor_products_modals.templ` (`edit-variant-modal`, opener JS at :403), row attributes `internal/ui/pages/vendor_products_table.templ:33-48`, handler `internal/ui/vendor_catalog_handlers_split2.go:VendorVariantUpdateSubmit`, SQL `internal/modules/catalog/postgres/variants_split2.go:100-126`, create path `internal/ui/vendor_variant_handlers.go:VendorVariantNewSubmit`.

### What is wrong — **[proven]**
1. **The edit wipes SKU and barcode.** The modal has no `sku` or `barcode` input. The handler does, unconditionally:
   ```go
   existing.SKU     = strings.TrimSpace(r.FormValue("sku"))      // "" 
   existing.Barcode = strings.TrimSpace(r.FormValue("barcode"))  // ""
   ```
   and `UpdateVariant` writes `sku = $3, barcode = $4`. Live evidence: **915 of 2,107 live variants have a blank SKU and 2,107 of 2,107 have a blank barcode.** The unique index `product_variants_org_sku_key` excludes `sku <> ''`, so blanking never errors — it silently destroys the data.
2. **It wipes the English name.** `nameEn` is absent from the form, so `existing.Name = i18n.New(nameAr, "")`.
3. **It clears cost price and cost discount** whenever those inputs are absent or empty (`else { existing.CostPrice = nil }`, `else { existing.CostDiscountPercentage = 0 }`).
4. **Status and negotiability are never prefilled.** The table row carries `data-name/price/discount/cost-price/cost-discount/stock/min-qty/batch/expiry/branch-id` but **no `data-status` and no `data-negotiable`**, and `openEditVariantModal` does not set those selects. They therefore always submit the first option: `status=active`, `is_negotiable=false`. Editing an inactive variant silently reactivates it and clears negotiability.
5. `existing.Unit` and `existing.Image` are preserved only by accident (guarded by `!= ""`), which is the pattern the other fields should follow.
6. `recordInitialStock` is called after `UpdateVariant` with the raw `stock_qty`; it sets an absolute quantity rather than recording a movement. Check `internal/modules/inventory` — `inventory.stock_movements` exists and should get a row for auditability.
7. The success/error redirects go to `/vendor/products`, losing the page/filter the user was on.

### What to change
1. **Make the handler a patch, not a replace.** Only assign a field when its form key is *present*:
   ```go
   if v, ok := formValue(r, "sku");     ok { existing.SKU = v }
   if v, ok := formValue(r, "barcode"); ok { existing.Barcode = v }
   ```
   with `func formValue(r *http.Request, k string) (string, bool)` checking `r.PostForm` membership. Apply to every field.
2. **Add the missing inputs to the modal**: `name_en`, `sku`, `barcode`, `unit`. They are real columns the vendor should control.
3. **Add `data-status`, `data-negotiable`, `data-name-en`, `data-sku`, `data-barcode`, `data-unit`, `data-image` to the table row** and set them in `openEditVariantModal`. Better: replace the eleven `data-*` attributes with one `data-variant='{...json...}'` produced by a `templ` helper, and `JSON.parse` it — eleven attributes is where the two lists drift.
4. **Prefill the selects**: `document.getElementById('edit-form-status').value = row.dataset.status || 'active'` and the same for negotiable.
5. **Preserve the listing context**: carry `?page=&q=&status=&stock=&sort=` through the redirect.
6. **Fix "adding a new صنف".** Read `VendorVariantNewSubmit` (`vendor_variant_handlers.go:207-323`) against the same checklist: does it set `organization_id`, `branch_id`, `min_order_qty`, `status`, `sku` uniqueness, and create the `inventory.stocks` row? Enforce `product_variants_org_sku_key` explicitly with a friendly message instead of a 500 on `23505`.
7. **Stock**: route quantity changes through `inventory.Service.AdjustStock` (which writes `inventory.stock_movements`) rather than an absolute set, or, if an absolute set is intended, record a movement of the delta. Confirm which by reading `recordInitialStock`.
8. **Guard the transaction**: `UpdateVariant`'s `WHERE id = $1 AND deleted_at IS NULL` has **no organization check**. The handler checks `existing.OrganizationID != actor.OrganizationID` first, which is correct but leaves the repository unsafe for any other caller. Add `AND organization_id = $N`.

### DB
No schema change. Consider a **data-repair migration**? No — the SKUs are gone and cannot be reconstructed. Instead, after the fix, offer a re-import path (`/vendor/ingest`) and note this in the release notes.

### Edge cases
- Two variants of the same org with the same non-empty SKU → `23505`; catch and translate.
- `expiry_date` cleared (empty string) must set `NULL`, not be ignored.
- `discount` is a percentage stored in a `NUMERIC` read as `money.Amount` (`money.Parse(dStr)`); `catalog.ProductVariant.EffectiveSellingPrice` treats it as a percent. Confirm the modal's "نسبة الخصم (%)" and the storage agree; a value of `26.40` must mean 26.4%.
- `branch_id` fallback picks the main branch when none is chosen — keep, but only for creation, not for an edit that deliberately clears it.

### Tests
- `internal/ui/vendor_phase6_test.go`, `vendor_transfer_stock_test.go` — extend.
- **New**: create a variant with `sku=ABC`, `barcode=123`, `name_en=X`, `status=inactive`, `is_negotiable=true`; POST the edit form with only `price` changed; assert every other column is unchanged. This is the regression test for the whole task.

---

## Task 14 — `بيانات المنشأة` never appears to save; add an edit-request workflow

### Area
`internal/ui/vendor_handlers.go` (`VendorOrganizationPage`, `VendorOrganizationSubmit`), `internal/ui/pages/vendor_organization.templ`, `internal/modules/org/service.go:UpdateSupplierProfile`, `internal/modules/org/postgres/repository.go:87-169`, routes `internal/ui/vendor_routes.go:38-46`.

### Why it looks like nothing saves — **[proven]**
The form edits `org.organizations.name` (via `name = jsonb_build_object('ar',$1,'en',$2)`), but **every other surface in the platform displays `trade_name` or `legal_name` first**:
- `org/postgres/admin_org_list.go:53` → `COALESCE(NULLIF(legal_name,''), NULLIF(name->>'ar',''), NULLIF(trade_name->>'ar',''), '')`
- `org/postgres/repository.go:287` → same
- `compare/postgres/market_discounts.go:44` (`marketSupplierNameSQL`) → `trade_name.ar → trade_name.en → legal_name`, `name` never consulted
- `org/postgres/admin_reviews.go:93`, `repository_reviews.go:28`, `repository_split3.go:158` → `trade_name → name`

Live proof, organization 51: `name.ar = "شركة ويزر فارماالاب"` (edited through this page) while `trade_name.ar = "شركة ويزر فارما"` and `legal_name = "شركة ويزر فارما"` are unchanged. **The write succeeds and is invisible.** The form also has no field for `legal_name`, `trade_name`, `commercial_register`, `pharmacist_license`, `credit_limit`, `payment_terms_days`, `branch_count` or `is_chain`.

Second defect: the `النوع` select offers only `supplier | company | agency` and marks `supplier` selected when the stored type is `vendor`. Saving therefore **silently rewrites `type` from `vendor` to `supplier`**. Both fold to the vendor scope via `rbac.TenantScopeFor`, so nothing breaks visibly — but it is an unrequested mutation of a column that `organizations_type_check` constrains and that admin filters read.

Third: `VendorOrganizationPage` renders with `_ = pages.VendorOrganizationPage(...).Render(ctx, w)` — a `check-error-swallow` violation waiting to be caught, and it skips `h.renderPage`'s `Content-Type` header and error logging.

### What to change

**14a — Make the save actually change what people see.**
- Add `legal_name`, `trade_name_ar`, `trade_name_en`, `commercial_register` to the form and to `SupplierOrgProfile`.
- `UpdateSupplierProfile` writes `name`, `trade_name` and `legal_name` together (keeping `name` as the legacy mirror of the Arabic trade name, which is what `org/postgres/mutations.go:21` already does elsewhere: `trade_name = COALESCE($2, jsonb_build_object('ar',$1,'en',$1))`).
- Remove the `النوع` select entirely, or restrict it to the values already stored and preserve `vendor` as an option. Preferred: **remove it** — an organization's type is set at registration and changed by an admin, not by the vendor.
- Replace `_ = pages...Render(...)` with `h.renderPage(...)`.

**14b — Split the one giant save into section forms.**
The page is currently one `<form>` with one "حفظ التعديلات والتحديث الفوري" button covering identity, order limits, contact, description and media. Split into five independent forms, each posting to its own endpoint:

| Section | Endpoint | Fields |
|---|---|---|
| الهوية التجارية | `POST /vendor/organization/identity` | legal_name, trade_name_ar/en, commercial_register, tax_number |
| حدود الطلب | `POST /vendor/organization/limits` | min_order_price, max_order_price |
| بيانات التواصل | `POST /vendor/organization/contact` | email, phone, address, organization_number |
| الوصف | `POST /vendor/organization/description` | description_ar, description_en |
| الشعار والغلاف | `POST /vendor/organization/media` | logo_file, coverage_file |

All five gated on `vendor.organization.update`. Each returns to `/vendor/organization#<section>` with its own notice. This is smaller code, not larger: one `applyOrgSection(ctx, orgID, section string, form url.Values)` in the service, driven by a whitelist per section.

**14c — Route the changes through an admin-approved edit request.**
Model it on the existing ads flow (`promo.ads.pending_changes` + `POST /admin/ads/{id}/approve-edit` + `AdminAdApproveEditSubmit`, migration 164) so the platform has one pattern, not two.

New table (migration 175+):
```sql
CREATE TABLE org.profile_change_requests (
  id                BIGGENERATED ALWAYS AS IDENTITY PRIMARY KEY,   -- match project convention
  public_id         UUID NOT NULL DEFAULT gen_random_uuid(),
  organization_id   BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
  requested_by      BIGINT NOT NULL REFERENCES identity.users(id)  ON DELETE RESTRICT,
  section           TEXT   NOT NULL CHECK (section IN ('identity','limits','contact','description','media')),
  proposed          JSONB  NOT NULL,       -- only the section's whitelisted keys
  previous          JSONB  NOT NULL,       -- snapshot, so the admin sees a diff
  status            TEXT   NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','approved','rejected','withdrawn')),
  admin_notes       TEXT   NOT NULL DEFAULT '',
  reviewed_by       BIGINT REFERENCES identity.users(id),
  reviewed_at       TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX org_profile_change_one_pending
  ON org.profile_change_requests (organization_id, section) WHERE status = 'pending';
CREATE INDEX org_profile_change_pending
  ON org.profile_change_requests (status, created_at DESC);
-- tenant-owned: enable + force RLS with platform.tenant_visible(organization_id)
```
(Follow the exact column/PK style of a recent migration — check `164_ads_pending_changes.up.sql` — and carry Arabic column comments as AGENTS.md requires.)

Behaviour:
- Which sections need approval is a policy decision. **Proposal**: `identity` (legal name, trade name, CR, tax number) requires approval; `limits`, `contact`, `description`, `media` apply immediately. Encode it as `var sectionsNeedingApproval = map[string]bool{...}` so it is one line to change.
- Vendor side: submitting a gated section creates a `pending` row and shows "التعديل قيد مراجعة الإدارة" inline in that section, with the proposed values shown beside the live ones and a "سحب الطلب" button (`status='withdrawn'`).
- Admin side: new page `GET /admin/organizations/change-requests` (and a badge on `/admin/approvals`), gated on `org.approval.view`; `POST /admin/organizations/change-requests/{id}/approve` and `/reject` gated on `org.approval.decide`. Approving applies `proposed` onto `org.organizations` inside one transaction and stamps `reviewed_by/at`.
- Add a nav item under the admin "المؤسسة والمشرفين" section: `{Key:"org_change_requests", Href:"/admin/organizations/change-requests", Perm:"org.approval.view"}`.
- Notify the vendor on approve/reject via `h.dispatchOrgNotification` (already used elsewhere).

**14d — Same treatment for the pharmacy?** The requirement names `/vendor/organization`. The pharmacy has no equivalent page (its org data is edited via `/settings/organization`, which Task 7 removes). **Add `/customer/organization`** with the same five sections and the same request flow, reusing every piece. If that is out of scope for this pass, say so explicitly and leave `/settings/organization` intact until it is built — do **not** remove the pharmacy's only org-edit path in Task 7 before this exists.

### Edge cases
- `org_price_range CHECK (max_order_price >= min_order_price)` — validate before the write and return a translated message (currently done in the handler; keep).
- Two members submitting the same section → the partial unique index rejects the second; catch `23505` and say "يوجد طلب تعديل قيد المراجعة لهذا القسم".
- An admin approving a request whose organization changed since submission → show the `previous` snapshot vs current and warn on conflict.
- Media uploads: store the file immediately (so the admin can see it) but only swap `image`/`coverage_image` on approval. Orphaned uploads need a sweeper or a `withdrawn` cleanup.
- RLS: the vendor may read only their own rows; the admin reads all via `database.AsSystem`.

### Tests
- `internal/ui/vendor_organization_test.go` exists — extend heavily.
- New: submit each section; assert immediate sections write through and gated sections do not; assert the admin approve applies exactly the whitelisted keys and nothing else; assert a vendor cannot approve their own request.
- Cross-tenant: org A's member cannot fetch or approve org B's request.

---

## Task 15 — Package-credit consumption log + `استهلاك الباقة` button (vendor)

### Area
Vendor page `/vendor/sponsorship-requests` → `internal/ui/pages/promo_revenue_vendor.templ:404-430` (the `رصيد الرعايات النشط والمتاح` panel), handler in `internal/ui/promo_revenue_handlers*.go`. Credit consumption sites: `internal/modules/promo/ads_service.go:53,69`, `sponsorship_service.go:181,251,307`, `sponsorship_moderation.go:69`, all calling `Repository.IncrementSponsorshipPurchaseCreditsUsed` (`promo/postgres/sponsorship.go:104`).

### What is wrong
`promo.sponsorship_purchases.credits_used` is a **counter with no ledger**. There is no record of *what* each credit was spent on. `promo.offer_sponsorships` carries `item_type/item_id/credits_used/package_id` for sponsorships, and `promo.sponsorship_requests` for requests, but **ads consume 2 credits (`AdCreditCost`) with no link at all** — `ads_service.go:53` increments the counter and stores nothing tying the ad to the purchase. Refunds (`-AdCreditCost`, `-sr.CreditsUsed`) are equally invisible. So "كشف حساب للباقة" cannot be produced from today's data.

### What to change

**15a — Add the ledger.** Migration 175+:
```sql
CREATE TABLE promo.sponsorship_credit_entries (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id       UUID   NOT NULL DEFAULT gen_random_uuid(),
  purchase_id     BIGINT NOT NULL REFERENCES promo.sponsorship_purchases(id) ON DELETE CASCADE,
  organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
  delta           INTEGER NOT NULL,          -- negative = spent, positive = refunded
  reason          TEXT NOT NULL CHECK (reason IN ('ad','sponsorship','sponsorship_request','refund','adjustment')),
  item_type       TEXT,                      -- 'ad' | 'offer' | 'product' | 'variant'
  item_id         BIGINT,
  ref_id          BIGINT,                    -- ad id / sponsorship id / request id
  note            TEXT NOT NULL DEFAULT '',
  actor_user_id   BIGINT REFERENCES identity.users(id),
  balance_after   INTEGER NOT NULL,          -- credits_total - credits_used, after this entry
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON promo.sponsorship_credit_entries (purchase_id, created_at DESC);
CREATE INDEX ON promo.sponsorship_credit_entries (organization_id, created_at DESC);
-- tenant-owned: ENABLE + FORCE ROW LEVEL SECURITY, policy platform.tenant_visible(organization_id)
```

**15b — Write to it atomically.** Replace `IncrementSponsorshipPurchaseCreditsUsed(ctx, purchaseID, credits)` with
`ConsumeSponsorshipCredits(ctx, entry promo.CreditEntry) (balanceAfter int, err error)` which, in **one transaction**:
1. `UPDATE promo.sponsorship_purchases SET credits_used = credits_used + $delta WHERE id = $1 AND credits_used + $delta BETWEEN 0 AND credits_total RETURNING credits_total - credits_used;`
2. Insert the ledger row with the returned `balance_after`.
3. `RowsAffected() == 0` → `apperr.Validation("sponsorship.insufficient_credits", …)`.

This also closes a real hole: today nothing prevents `credits_used` exceeding `credits_total` under concurrency (`ads_service.go` reads the purchase, then increments in a separate statement).

Update all six call sites. Keep the old repository method only if an external caller needs it — otherwise delete it and let `check-deadcode` confirm.

**Backfill** (best-effort, in the same migration):
```sql
INSERT INTO promo.sponsorship_credit_entries (purchase_id, organization_id, delta, reason, item_type, item_id, ref_id, balance_after, created_at)
SELECT s.package_id_purchase, s.organization_id, -s.credits_used, 'sponsorship', s.item_type, s.item_id, s.id, 0, s.created_at
FROM promo.offer_sponsorships s ...;
```
Live tables are effectively empty, so the backfill is cosmetic — write it anyway so the migration is correct if run against a populated copy, and set `balance_after = 0` with a `note` marking it reconstructed.

**15c — The `استهلاك الباقة` button.** In `promo_revenue_vendor.templ`, each purchase card in `رصيد الرعايات النشط والمتاح` gains:
```
<a href="/vendor/offers-packages/purchases/{id}/statement" class="btn btn-secondary btn-xs">استهلاك الباقة</a>
```
New route in `registerVendorPromoRoutes` under `vendor.offer_package.view`:
`GET /vendor/offers-packages/purchases/{id}/statement` → a page (not a modal — it is a statement) showing:
- header: package name (join `promo.offer_packages`), price, `credits_total / credits_used / remaining`, `starts_at`–`expires_at`, status;
- the ledger table: date, reason (translated), the item it was spent on with a link (`/vendor/ads/{id}`, `/vendor/offers/{id}`, `/vendor/products?...`), delta, balance after;
- `components.B2BPagination`;
- an export button reusing the existing export helper pattern (`/…/statement.xlsx`) if one exists; otherwise skip.

The card currently shows `باقة #{PackageID}` — resolve the package name too.

### Edge cases
- Refund entries (positive delta) must render distinctly and must not be counted as usage.
- A purchase that expired with credits remaining — show `منتهية` and keep the statement readable.
- A vendor must only see their own purchases: scope by `organization_id` in the handler **and** rely on RLS.
- Concurrency: two ads created simultaneously against the last 2 credits — the `BETWEEN 0 AND credits_total` guard makes the second fail cleanly.

### Tests
- Unit: `ConsumeSponsorshipCredits` refuses to overdraw; a refund restores the balance; `balance_after` is monotone.
- Integration: create purchase(10) → create ad(-2) → reject ad(+2) → assert 3 ledger rows and `credits_used = 0`.

---

## Task 16 — Cart/checkout with offers, and the order-edit overhaul

**Depends on Task 8** (special offers cannot be created today, so this flow cannot be exercised end-to-end until Task 8 lands).

### 16a — `بيانات الطلب غير صالحة` at checkout with an offer in the cart

**Where the message comes from**: `internal/shared/apperr/apperr.go:163` maps `KindValidation` to `"بيانات الطلب غير صالحة."`. `internal/ui/customer_checkout_handlers.go:checkoutValidationMessage` translates five specific codes and returns `false` for everything else, which falls through to `h.renderError` and the generic envelope.

**Validation codes reachable from `Checkout` that are NOT mapped — [proven by enumeration]:**
| Code | Raised by | Likely? |
|---|---|---|
| `documents.incomplete` | `cmd/server/routes_api.go:64` docs gate, called first thing in `Checkout` | **High** — fires for any pharmacy with a missing mandatory document, on every checkout |
| `item.price_overflow` | `service.go:142` | low |
| `order.empty` / repo errors | `postgres/orders.go` | low |
| `checkout.line_unavailable.*` | mapped | — |

**Diagnostic to run first** (10 minutes, do this before writing the fix): reproduce with an offer in the cart and read the server log line `"checkout failed"` (`customer_checkout_handlers.go:339`) — it logs the real error. That single line names the code.

**Fixes regardless of which code it is:**
1. **Map every validation code.** Add `documents.incomplete` → a message naming the missing documents and linking to `/customer/documents`; add a `default:` arm that surfaces `ae.Code` in the log and a generic-but-actionable message. A generic envelope for a *validation* error is the actual defect: the user cannot act on it.
2. **Offer lines carry `ProductID: &pID` and `ProductVariantID: &vID` where both are 0** (`customer_checkout_handlers.go:186-193`). `postgres/orders.go:150-157` normalises them to `nil`, so no FK violation — but pass `nil` from the handler so the intent is explicit and `revalidateCheckoutLines` cannot be confused.
3. **Vendor resolution for offer lines**: the cart query (`commerce/postgres/carts.go:75-100`) resolves `organization_id` via `COALESCE(po.organization_id, spo.organization_id, pv…, p…, stocks…, 0)`. `promo.special_offers` is **empty** (0 rows) — every special offer actually lives in `promo.offers` with `source='special'`, so `po.organization_id` is the branch that fires. Verify with the offer you reproduce on; if `organization_id` comes back `0`, that is `item.vendor_required` and the mapped message already exists.
4. **Remove the dead `spo` join and the `to_regclass` probe** from `GetCartWithItems` once `promo.special_offers` is dropped (Task 8, step 9). Until then leave it.
5. **`input.VendorBranchID` is a single value applied to every shipment** (`service.go:227`). With two vendors, vendor B's shipment gets vendor A's branch. Change `OrderShipment.BranchID` to be resolved per `vendorOrgID` (main branch of that vendor), and only fall back to `input.VendorBranchID` for the offer's own vendor.

### 16b — Offer quantity locked on the product-details page; normal products unchanged

Current state:
- `pages/customer_product_detail.templ:270-283`: a `cart-stepper-control` with `min = MinOrderQty`, `max = AvailableStock` — this is a **normal variant**, keep it as is.
- `pages/customer_product_detail_offers.templ:107`: `<input type="hidden" name="qty" value="{max(1, MinOrderQty)}">` — already fixed.
- `pages/offers_detail.templ:143-148`: `<input type="number" name="quantity" min="1" max="99" value="1">` on `/cart/add-offer` — **this is the offer bundle multiplier and is what must be locked.**
- `pages/customer_cart.templ:175-208`: the cart stepper posts `quantity = item.Quantity ± 1` for **every** line, including offer lines; `UpdateCartQuantitySubmit:282` explicitly handles the offer case.

**Change**: an offer bundle is bought as one unit.
1. `offers_detail.templ`: replace the number input with a read-only `1` (or a hidden field) and a note.
2. `customer_cart.templ`: render the stepper only when `item.OfferID == nil`; for offer lines show the quantity as static text with the remove button still available.
3. `AddOfferToCartSubmit`: force `bundleMultiplier = 1` (keep parsing it so an old client does not error).
4. `UpdateCartQuantitySubmit`: for an offer line, allow only `0` (remove); reject any other value with a translated message.
5. Order-edit form (16c): offer lines get `readonly` quantity.

*Assumption stated explicitly*: "an offer" means a promo bundle (`cart_items.offer_id IS NOT NULL`), not a supplier's price offer on a variant. Supplier price offers keep their normal stepper. If the intent was the opposite, only step 1 and 2 change.

### 16c — Order editing: no modal, no adding items

**Remove:**
- `internal/ui/pages/customer_order_detail_edit_modal.templ` — the whole `components.Modal` wrapper.
- The "إضافة صنف جديد يدوياً للطلب" button and `window.addNewEditRow`.
- The `else` branch in `commerce/postgres/order_edit.go:174-194` that inserts a brand-new line with `sku = "CUSTOM"` and a hand-built `nameJSON` (a JSON-injection risk via `%q` on a user string — it happens to be safe for `"` but not for control characters).
- Reject `l.ID <= 0` in `commerce.Service.UpdateCustomerPendingOrder` with `apperr.Validation("order.line_unknown", …)` so the API cannot add lines either.
- The free-text `product_name[]`, `unit_price[]` and `discount_amount[]` inputs: the repository already **ignores** the submitted unit price and discount for existing lines (it re-reads `dbUnitPrice` and recomputes `unitDiscount`), so those inputs are decorative and misleading. Render them read-only.

**Replace with**: an inline `<form>` on the order detail page (`pages/customer_order_detail.templ`), visible when `order.Status == commerce.StatusPending` and the actor holds `pharmacy.order.update`. One row per line: product name (static), quantity stepper, unit price (static), line total (live), and a "حذف" checkbox/button. Plus notes and a single "حفظ التعديلات".

**Quantity rules (client and server, same numbers):**
- min = `max(1, line.MinOrderQty, offerCustomQty)`; the "−" button is disabled at min.
- max = `min(line.MaxQtyPerOrder or ∞, line.AvailableStock or ∞)`.
- Offer lines (`offer_product_id IS NOT NULL`): quantity fixed, stepper hidden.
- Removing the **last** line is refused *before* the delete, not after (today `order_edit.go:198-202` deletes first, counts second, and relies on the transaction rollback — correct but the message arrives after the user has already lost the row visually).
- A line whose quantity is already 1 cannot be decreased and cannot be removed **if it is the only line**. If other lines exist, removal is allowed — the requirement says "if an order has one quantity for one product, then the user cannot decrease it below 1 or remove that item", which is the single-line case.

**Multi-vendor / offer correctness — [proven bugs]:**
1. `order_edit.go:257-263` updates **only the first shipment** (`defaultShipmentID`) and gives it the **whole order's** subtotal and total. An order with two vendors ends with shipment 1 holding everything and shipment 2 stale. Fix: recompute per shipment —
   ```sql
   UPDATE commerce.order_shipments s
   SET subtotal = agg.sub,
       total_amount = agg.sub + s.shipping_fee,
       updated_at = now()
   FROM (SELECT shipment_id, SUM(total_price) AS sub
         FROM commerce.order_lines WHERE order_id = $1 GROUP BY shipment_id) agg
   WHERE s.id = agg.shipment_id AND s.order_id = $1;
   ```
   and delete shipments left with zero lines (or mark them cancelled — decide; deleting loses the vendor's history, so prefer `status = 'cancelled'` with `subtotal = 0`).
2. `order_edit.go:204-227` recomputes `newSubtotal` as `SUM(unit_price * quantity)` (**gross**) while `commerce.Service.Checkout` sets `Subtotal` to the sum of line **totals** (net of discount). A no-op edit therefore changes the displayed subtotal. Make the edit path match checkout: `newSubtotal = SUM(total_price)` and `total_discount = SUM(discount_amount)`; then `total = subtotal + shipping + tax`.
3. The status-history row is written with `from_status = to_status = currentStatus` and a fixed note — fine, but include what changed (line count, delta) in `notes`.
4. `defaultOrgID` falls back to `order.OrganizationID`, which is the **customer's** org, not a vendor's. It is only used for the removed insert path — deleting that path removes the bug.

**Also fix while here:**
- The order-detail edit currently accepts `X-Requested-With: XMLHttpRequest` and returns JSON, but the form posts normally. Pick one; keep the JSON path only if the new inline form uses `fetch`.
- `pages/customer_order_detail_script.templ` holds `window.stepQty`, `window.recalcEditOrder`, `window.deleteEditRow`, `window.addNewEditRow` — rewrite for the inline form and delete `addNewEditRow`.

### Edge cases to test
- Order with one line, quantity 1: `−` disabled, delete disabled.
- Order with lines from two vendors: reduce vendor A's line to 0 → vendor A's shipment goes to zero/cancelled, vendor B's totals unchanged, order total correct.
- Order containing one offer bundle + normal products from the **same** vendor.
- Order containing one offer bundle from vendor A + a normal product from vendor B.
- Offer whose `promo.offer_products.max_qty_per_order` is set → exceeding it is refused with the existing message.
- Order that leaves `pending` between page render and submit → `apperr.Forbidden("order.locked")` (already handled; keep and surface it).
- Concurrent edit by two members → the `FOR UPDATE` on the order row serialises; verify.

### Tests
- `internal/ui/customer_order_edit_test.go` and `checkout_offer_helpers_test.go` exist — extend.
- New integration tests for the four multi-vendor/offer permutations above, asserting `orders.total_amount == SUM(order_shipments.total_amount)` after every edit. Make that equality an invariant assertion in a helper and call it from every order test.

---

## Task 17 — Expand `/admin/offers-packages` with per-organization usage and statements

**Depends on Task 15** (the ledger is the data source).

### Area
`internal/ui/promo_revenue_handlers.go:AdminOffersPackagesHubPage` (+ `_split2.go`), routes `internal/ui/admin_routes_commerce.go:106-119`, view `internal/ui/pages/promo_revenue.templ` (1,393 lines — it will need splitting to stay under 400 in its Go companions).

### What exists today
Tabs: `packages` (`AdminListPackages`), `sponsorships` (`AdminListSponsorshipRequestsWithTotal`), `promotions`, `views`. There is **no per-organization view**: you cannot ask "what did pharmacy X buy and what did they spend it on".

### What to add
1. **New tab `المنشآت` / route `GET /admin/offers-packages/organizations`** (perm: the same one the hub uses — check `admin_routes_commerce.go:104` for the group's `RequirePagePermission`; likely `promo.package.view`). One row per organization that has ever purchased, with: org name (`trade_name → legal_name`), type, packages purchased (count), total credits bought, total used, remaining, last purchase date, active purchases count. Server-side paginated + searchable by org name.
   Query shape:
   ```sql
   SELECT o.id,
          COALESCE(NULLIF(o.trade_name->>'ar',''), NULLIF(o.legal_name,''), o.name->>'ar') AS org_name,
          o.type,
          COUNT(sp.id)                       AS purchases,
          COALESCE(SUM(sp.credits_total),0)  AS credits_total,
          COALESCE(SUM(sp.credits_used),0)   AS credits_used,
          MAX(sp.created_at)                 AS last_purchase,
          COUNT(*) FILTER (WHERE sp.status='active' AND sp.expires_at > now()) AS active
   FROM promo.sponsorship_purchases sp
   JOIN org.organizations o ON o.id = sp.organization_id
   GROUP BY 1,2,3
   ```
2. **Drill-down `GET /admin/offers-packages/organizations/{orgID}`**: the organization's purchases, each with package name, price, credits, window, status — and per purchase a **`كشف حساب للباقة`** button linking to
3. **`GET /admin/offers-packages/purchases/{purchaseID}/statement`** — the same statement view built in Task 15c, rendered in the admin shell with the organization named. **Build the statement as one shared `templ` component** (`pages/sponsorship_statement.templ`) taking a view model, used by both the vendor route and the admin route. Do not fork it.
4. Add an export (`.xlsx`) on the admin statement using the existing export helper pattern (`internal/ui/vendor_saving_export_handlers.go` is a working example of the header/writer style).
5. **Nav**: the hub is already reachable; add a sub-item or a tab, whichever the page uses today.

### Permissions
Reuse the existing gate on the `/admin/offers-packages` group. Add no new permission unless the group is currently ungated — check `admin_routes_commerce.go:100-120` and gate it if not.

### Edge cases
- Organizations deleted (`org.organizations.deleted_at`) still have purchases — show them with a "محذوفة" badge rather than dropping the rows.
- A purchase whose `package_id` points at a deleted package — `LEFT JOIN` and fall back to `باقة #id`.
- Large ledgers: paginate the statement, index `(purchase_id, created_at DESC)` (already in the Task 15 migration).
- Money: `promo.sponsorship_purchases.amount` is `NUMERIC` → scan into `money.Amount`, never `float64`.

### Tests
- `internal/ui/admin_offers_packages_test.go` exists — extend with the new tab and the statement.
- Assert an admin without the gate permission gets 404.

---

## Task 18 — River queue for every import, with real-time progress

### Current state — **[proven]**

| Import | Entry | Where the work runs | Progress | Survives restart |
|---|---|---|---|---|
| Compare bulk upload | `POST /compare/upload` → `compare_upload_handlers.go:92` | **in-request**, 6 goroutines, `ctx` = request | none | no |
| Compare catalogue matching | `compare.EnqueueCatalogMatch` | own bounded pool (2 workers, depth 256), `context.Background()` | none | no |
| Saving products (vendor) | `POST /vendor/saving-products/import/start` → `vendor_saving_import_json_split2.go:21` | detached `go func` with `context.Background()` | in-memory session, polled | **no** |
| Saving products (pharmacy) | `POST /customer/saving-products/import/start` | same | same | **no** |
| Saving products **commit** ("معالجة") | `POST …/import/session/{id}/commit` | **in-request, synchronous** | **none** | no |
| Team import (both) | `POST /{aud}/team/import/upload` | in-request parse, in-memory session | none | no |
| Vendor catalogue ingest | `POST /vendor/ingest/{id}/mapping` | `ingest.StageInBackground` — detached goroutine, `context.WithoutCancel`, 45-min timeout, **DB-backed progress** in `ingest.import_progress` | polled page | partially (a sweeper in `cmd/worker/main.go:92-101` releases wedged sessions) |
| Admin catalogue import | `/admin/products/import/*` | check `admin_import_handlers*.go` | `admin_import_review.templ:492` uses `ImportProgress` | — |
| Admin temp warehouses | `POST /admin/user/temparte-warehouses/upload` | in-request, parallel per file | none | no |
| Smart order | `POST /smart-order/*` | **River** (`queue.SmartOrderRunArgs`, `cmd/worker/smartorder.go`) | DB-backed | **yes** |

`public.river_job` has **0 rows** — only smart-order is wired to River and it has never run here. `ingest.import_progress` and `ingest.import_sessions` are both empty; `catalog.import_sessions` has 3 rows.

`internal/ui/static/js/import-progress.js` is a good, shared, honest progress bar. It is used by 5 screens. **The problem is not the bar; it is that there is nothing durable behind it.**

### Why the vendor progress page "does not update in real time"
It does poll (`vendor_saving_script.templ:328`, every 600 ms) — but it polls an **in-memory** session (`globalSavingImportSessionStore`). If the web process restarts, or if there is more than one replica behind the proxy, the poll hits a process that has never heard of the session and gets `{"success": false}` → the bar fails or freezes. Also `UpdateProgress` is only called every 100 rows, so a slow AI stage looks frozen (the drift animation masks this partially).

### Why the "معالجة" step of saving products has no progress at all
The commit (`CustomerSavingProductsImportCommitJSON` / `VendorSavingProductsImportCommitJSON`) writes every staged row synchronously inside the POST. There is no session phase, no progress endpoint, no page.

### The plan

**18.1 — One durable import-session table.** Do not invent a fourth session store. Promote the existing `ingest` pattern to a shared one, or add a small platform-level table:
```sql
CREATE TABLE platform.import_runs (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id       UUID NOT NULL DEFAULT gen_random_uuid(),
  organization_id BIGINT NOT NULL,
  user_id         BIGINT NOT NULL,
  kind            TEXT NOT NULL,     -- 'saving_products' | 'team' | 'compare' | 'temp_warehouse' | 'catalog'
  audience        TEXT NOT NULL,     -- 'vendor' | 'customer' | 'admin'
  filename        TEXT NOT NULL DEFAULT '',
  state           TEXT NOT NULL DEFAULT 'queued'
                    CHECK (state IN ('queued','processing','ready','committing','committed','failed','cancelled')),
  phase           TEXT NOT NULL DEFAULT '',
  percent         SMALLINT NOT NULL DEFAULT 0 CHECK (percent BETWEEN 0 AND 100),
  total_rows      INTEGER NOT NULL DEFAULT 0,
  processed_rows  INTEGER NOT NULL DEFAULT 0,
  payload         JSONB  NOT NULL DEFAULT '{}',  -- mapping choices, flags
  result          JSONB  NOT NULL DEFAULT '{}',  -- counters for the review screen
  error_message   TEXT   NOT NULL DEFAULT '',
  river_job_id    BIGINT,
  started_at      TIMESTAMPTZ,
  finished_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON platform.import_runs (organization_id, created_at DESC);
CREATE INDEX ON platform.import_runs (state) WHERE state IN ('queued','processing','committing');
-- staged rows go in a child table or in an object-storage blob; see 18.2
```
Staged rows (`StagedSavingItem`, `TeamImportRow`) are today held in Go slices in memory. For 30k-row files, put them in a child table `platform.import_run_rows (run_id, row_number, data JSONB, included BOOL, ...)` — the `ingest.import_rows` / `catalog.import_staging_rows` tables already do exactly this and can be the model.

**18.2 — Two River job kinds** in `internal/platform/queue/jobs.go`:
```go
type ImportStageArgs  struct{ RunID int64; OrganizationID int64 }
func (ImportStageArgs) Kind() string { return "imports.stage" }
func (ImportStageArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: "imports", MaxAttempts: 1} }

type ImportCommitArgs struct{ RunID int64; OrganizationID int64 }
func (ImportCommitArgs) Kind() string { return "imports.commit" }
func (ImportCommitArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: "imports", MaxAttempts: 3} }
```
`MaxAttempts: 1` for staging (a retry would re-bill the AI stage, the same reasoning already written for `SmartOrderRunArgs`); `MaxAttempts: 3` for the commit, which must be **idempotent** — key inserts on `(run_id, row_number)` and use `ON CONFLICT DO NOTHING`.

Workers live in `internal/modules/<owner>/jobs/` per AGENTS.md (`catalog/jobs`, `smartorder/jobs` are the precedent). Register them in `cmd/worker/main.go` next to `ingestBatchWorker`.

**18.3 — Enqueue transactionally.** `queue.EnqueueTx` already exists (`queue.go` doc comment names the outbox pattern). The upload handler must, in one transaction: insert `platform.import_runs` + the raw rows, then `EnqueueTx` the stage job. If the transaction rolls back, no orphan job.

**18.4 — Progress endpoints become DB reads.** One shared handler:
`GET /imports/{publicID}/progress` → `{state, phase, percent, processed, total, error, done}`, scoped to the caller's organization. Replace `/vendor/saving-products/import/session/{id}/progress` and `/customer/…` with it (keep the old paths as aliases for one release). `import-progress.js` needs no change — it already consumes this shape.

**18.5 — Add the missing progress screen for the commit.** After `POST …/commit`, redirect to the run page in `state='committing'`; the same bar polls the same endpoint. This is the "معالجة step gives no progress page" fix.

**18.6 — Convert each importer, in this order** (each is independently shippable):
1. **Saving products (vendor + pharmacy)** — highest value, both stage and commit. They share `saving_products_*.go`; convert the shared code once.
2. **Team import (vendor + pharmacy)** — shares `team_import_ops*.go`.
3. **Temp warehouse bulk upload (admin)** — combine with the `AnalyzeLayout` fix from Task 4.
4. **Compare bulk upload** — the largest change; keep `MakeRoomForFiles` (the batch quota reservation) in the request, move the per-file parse+insert into jobs. `/compare/upload` is in `httpx.longRunningPrefixes`; once the work is queued, **remove it from that list** (that exemption exists only because the work is in-request).
5. **Vendor catalogue ingest** — already background + DB-backed; convert `StageInBackground` to enqueue `ImportStageArgs` so the run survives a web-process restart, and delete the `runs.claim` in-memory lock in favour of the `state` column + a partial unique index.
6. **Admin catalogue / image import** — same pattern.

**18.7 — Delete the in-memory stores** after each conversion: `globalSavingImportSessionStore` (`saving_products_sessions.go`), `globalTeamImportSessionStore` (`team_import_types.go`), and their `init()` cleanup goroutines. Run `check-deadcode`.

**18.8 — Operational requirements.**
- The worker must be running. `docker-compose.yml` and `Dockerfile` build three binaries; confirm the worker service is deployed and that `cfg.Worker.Queues` includes `"imports"` with a sensible `MaxWorkers` (start at 2–4; each job holds a DB connection and the pool is 20).
- A stuck-run sweeper: the worker already has one for `ingest` (`cmd/worker/main.go:92-101`); generalise it to `platform.import_runs` — anything in `processing`/`committing` with `updated_at < now() - interval '30 minutes'` and no live River job becomes `failed` with a message.
- River retention: add a periodic job or rely on River's built-in job cleaner; `public.river_job` will grow.

**18.9 — Real-time without polling (optional, after the above).** River publishes `river_notification` (`LISTEN/NOTIFY`). An SSE endpoint `GET /imports/{publicID}/events` would remove the 600 ms poll. `httpx.IsLongRunning` already exempts `Accept: text/event-stream`. Do this only after 18.1–18.8 are green — polling a DB row is already a correct real-time update, and SSE adds a connection-per-viewer cost.

### Edge cases
- Two uploads by the same user at once → allowed, each gets its own run; the list page shows both.
- A run whose owner loses `*.saving_product.manage` mid-run → the commit worker must re-check the permission via a stored `user_id`, or fail closed.
- Cancel: `state='cancelled'` + `river.JobCancel` on the stored `river_job_id`.
- A 30k-row file: `payload`/`result` JSONB must stay small — rows go in the child table, never in `payload`.
- RLS: `platform.import_runs` is tenant-owned; the worker runs under `database.AsSystem` with the run's `organization_id` re-bound for tenant queries.
- The AI stage (`h.enhanceSaving`) currently runs inside the web process and bills through `aiusage`; the worker already has a `usageRecorder` wired (`cmd/worker/main.go:70`) — make sure the moved code uses it.

### Tests
- Integration: enqueue a stage job, run the worker in-process, assert `state` transitions `queued → processing → ready` and that `percent` is monotone.
- Kill-and-resume: mark a run `processing` with a stale `updated_at`, run the sweeper, assert `failed`.
- Idempotency: run the commit worker twice on the same run, assert row counts do not double.
- `internal/ui/saving_products_import_test.go`, `customer_saving_test.go`, `team_import_test.go`, `vendor_ingest_test.go`, `compare_upload_e2e*_test.go` all exercise the current synchronous paths — they must be rewritten to drive the job, not the goroutine.

---

# Final implementation order

Grouped into waves. Everything inside a wave is independent and can be parallelised;
each wave depends on the ones before it.

### Wave 0 — Foundations (do first, they unblock or de-risk everything)
| # | Task | Why first |
|---|---|---|
| 0.1 | **Task 8** — special-offer INSERT + `base_price` fixes | Nothing about offers can be tested until offers can be created. Two one-line SQL fixes with proven root causes; ship them alone. |
| 0.2 | **Task 12** — `provider`/`type` field name | One-line fix, immediately unblocks wallet testing. |
| 0.3 | **Task 9** — `RawFilePreview` + shared preview padding | Pure function; blocks nothing but makes every import task verifiable. |
| 0.4 | **Task 2** — remove `external_url` | Small, isolated, needs its own migration slot. |
| 0.5 | Commit or reset the dirty `test/visual_baselines/*.png` | So every later diff is attributable. |

### Wave 1 — Shared UI primitives (built once, consumed by many later tasks)
| # | Task |
|---|---|
| 1.1 | **Task 5.1/5.2** — modal flex/scroll fix in `components.css` (fixes the ads wizard *and* every other Alpine modal) |
| 1.2 | **Task 5.5** — the shared searchable combobox (`components/product_picker.templ`) |
| 1.3 | **Task 6** step 1 — the shared geo combobox (`components/geo_picker.templ`), built on 1.2 |
| 1.4 | **Task 11a** — delegated geolocation listener in `maps.js` |
| 1.5 | **Task 11b** — CSS fallbacks (`oklch`, `color-mix`, `dvh`, `-webkit-backdrop-filter`, `@layer` baseline decision) + `docs/BROWSER_SUPPORT.md` |

### Wave 2 — Permission / navigation truth
| # | Task | Depends on |
|---|---|---|
| 2.1 | **Task 7** — `اعدادات الحساب` page + `rbac.NavItem.AlwaysVisible` + route tier move | — |
| 2.2 | **Task 10** — `rbac.AccountMenu` + rewrite `user_menu.templ` | 2.1 (the settings link is one of its entries) |

### Wave 3 — Page consolidations
| # | Task | Depends on |
|---|---|---|
| 3.1 | **Task 1** — `/customer/team` ⇐ branch employees tab | 1.1 (modals), 2.x (nav) |
| 3.2 | **Task 3** — `/catalog` stock-first ordering + index | — |
| 3.3 | **Task 4a** — market discounts source → temp warehouses (+ the `AnalyzeLayout` upload fix) | 0.3 |
| 3.4 | **Task 4b** — market discount card changes | 3.3 |
| 3.5 | **Task 5.3–5.6** — ads wizard redesign | 1.1, 1.2, 0.4 |
| 3.6 | **Task 6** steps 2–5 — register page pickers | 1.3 |
| 3.7 | **Task 13** — variant edit patch semantics + missing fields | — |

### Wave 4 — Workflows with new schema
| # | Task | Depends on |
|---|---|---|
| 4.1 | **Task 14** — `/vendor/organization` sectioned save + `org.profile_change_requests` + admin approval page | 2.1 (must land before `/settings/organization` is removed) |
| 4.2 | **Task 15** — `promo.sponsorship_credit_entries` + `ConsumeSponsorshipCredits` + vendor statement | — |
| 4.3 | **Task 17** — admin `/admin/offers-packages` organizations tab + shared statement component | 4.2 |
| 4.4 | **Task 12** steps 4–7 — payment-method `details` column + edit modal + route gating | 0.2 |

### Wave 5 — Commerce correctness
| # | Task | Depends on |
|---|---|---|
| 5.1 | **Task 16a** — checkout validation messages + per-vendor shipment branch | 0.1 |
| 5.2 | **Task 16b** — offer quantity locking | 0.1 |
| 5.3 | **Task 16c** — order edit: inline form, no add-item, per-shipment recalculation | 5.1 |

### Wave 6 — Background processing
| # | Task | Depends on |
|---|---|---|
| 6.1 | **Task 18.1–18.5** — `platform.import_runs`, two River job kinds, shared progress endpoint, commit progress screen | — |
| 6.2 | **Task 18.6.1** — saving products (vendor + pharmacy) | 6.1 |
| 6.3 | **Task 18.6.2** — team import | 6.1, 3.1 |
| 6.4 | **Task 18.6.3** — temp warehouse bulk upload | 6.1, 3.3 |
| 6.5 | **Task 18.6.4** — compare bulk upload | 6.1 |
| 6.6 | **Task 18.6.5–6** — vendor ingest, admin imports | 6.1 |
| 6.7 | **Task 18.7–18.8** — delete in-memory stores, sweeper, worker deployment | 6.2–6.6 |

### Wave 7 — Verification
Full `make check`, the browser matrix from 11b, and the checklists below.

---

# Dependency map

```
0.1 Task8 (offer INSERT) ──┬─► 5.1 Task16a ──► 5.3 Task16c
                           └─► 5.2 Task16b
0.2 Task12 (field name) ─────► 4.4 Task12 (details column, edit modal, gating)
0.3 Task9 (preview) ─────────► 3.3 Task4a (temp-warehouse upload AnalyzeLayout)
0.4 Task2 (external_url) ────► 3.5 Task5 (ads wizard)

1.1 modal flex fix ──┬─► 3.1 Task1 (team page modals)
                     ├─► 3.5 Task5 (ads wizard)
                     └─► 4.4 Task12 (payment edit modal)
1.2 combobox ────────┬─► 1.3 geo combobox ──► 3.6 Task6 (register)
                     └─► 3.5 Task5 (product picker)
1.4 geolocation ─────────► 3.6 Task6 (map ↔ picker sync)
1.5 CSS fallbacks ───────► Wave 7 browser matrix

2.1 Task7 (settings) ─┬─► 2.2 Task10 (account menu)
                      └─► 4.1 Task14 (org data moves off /settings/organization)
2.2 Task10 ──────────────► Wave 7 permission matrix

3.3 Task4a ──────────────► 3.4 Task4b
4.2 Task15 (ledger) ─────► 4.3 Task17 (admin statements)

6.1 import_runs + River ─┬─► 6.2 saving products
                         ├─► 6.3 team import  (also needs 3.1)
                         ├─► 6.4 temp warehouse (also needs 3.3)
                         ├─► 6.5 compare upload
                         └─► 6.6 vendor/admin ingest
                                 └─► 6.7 delete in-memory stores
```

**Migration slot ordering** (do not reuse numbers; next free is **175**):
1. `175_ads_drop_external_click_target` (Task 2)
2. `176_catalog_stock_first_index` (Task 3)
3. `177_temp_warehouse_market_indexes` (Task 4)
4. `178_user_payment_method_details` (Task 12)
5. `179_org_profile_change_requests` (Task 14)
6. `180_sponsorship_credit_ledger` (Task 15)
7. `181_platform_import_runs` (Task 18)
Each must be expand/contract safe: the currently deployed binary has to keep working after the migration runs and before the new image is promoted.

---

# Verification checklist

Tick each after implementing; every item names the thing to look at, not "check it works".

**Task 1 — team page**
- [ ] `/customer/team` shows the advanced table (search, branch filter, role badge, status, actions) and paginates.
- [ ] Only two header buttons: `استيراد موظفين Excel`, `إضافة موظف`; both hidden without `pharmacy.team.create`.
- [ ] Add-employee modal creates a user + `org.members` row with branch, role, job title, employee code; the row appears without a refresh loop.
- [ ] Edit changes only the submitted fields; job title, code, branch, role and active flag all persist (verify by SELECT).
- [ ] Delete and status toggle act on the intended member (create two employees, delete the second, confirm the first survives).
- [ ] `/customer/branches` has no tab bar and no employees table; the per-branch staff count is still correct.
- [ ] `/customer/branches?tab=employees` redirects to `/customer/team`.

**Task 2 — ads**
- [ ] No `رابط خارجي مخصص` option anywhere.
- [ ] `POST /vendor/ads/new` with `click_target_type=external_url` stores `product`, HTTP 3xx not 500.
- [ ] `SELECT DISTINCT click_target_type FROM promo.ads` ⊆ {product, vendor_page, offer}; the CHECK constraint rejects the fourth.

**Task 3 — catalog**
- [ ] `/catalog` page 1 with no query: every card in the first screenful is orderable (`CanAddToCart`).
- [ ] `SELECT count(*)` of in-stock products (851 today) equals the number of stock-tier cards before the first placeholder.
- [ ] Typing a search term still returns relevance-ordered results.
- [ ] `EXPLAIN ANALYZE` at `OFFSET 0` and `OFFSET 4800` under 200 ms.

**Task 4 — market discounts**
- [ ] Rows come from `compare.file_rows` of `is_temp_warehouse = TRUE` files only; a compare-tool file's rows never appear under any filter.
- [ ] Neither card shows a product code.
- [ ] `سعر الجمهور` has no strike-through and is visually the largest number on the card.
- [ ] `السعر بعد الخصم` is absent from both cards.
- [ ] The grid card shows the upload date.
- [ ] A newly uploaded temp warehouse appears within one page load.
- [ ] A file with two title rows above the header maps its columns correctly (the `AnalyzeLayout` fix).

**Task 5 — ads wizard**
- [ ] At 360×640, 390×844 and 768×1024, the Next/Submit buttons are visible on all four steps without page zoom.
- [ ] The step body scrolls; the header, indicator and footer do not.
- [ ] The product dropdown: 2-char minimum, debounced, shows loading/empty states, selection survives the blur, keyboard-navigable, and the dropdown is not clipped by the modal.
- [ ] Submitting with a foreign `click_target_id` is refused server-side.

**Task 6 — register**
- [ ] Governorate and city are search inputs with listboxes; selecting a governorate filters the cities.
- [ ] Selecting a city moves the map marker; `موقعي الحالي` fills both pickers.
- [ ] A validation failure re-renders both pickers with the chosen names, not blanks.
- [ ] A `city_id` from another governorate is rejected server-side.

**Task 7 — account settings**
- [ ] `اعدادات الحساب` appears in the sidebar for admin, vendor, pharmacy, and a pending-org member.
- [ ] `GET /settings` returns 200 for all four; no redirect to `/vendor/organization`.
- [ ] The page title reads `اعدادات الحساب`; there is no organization tab.
- [ ] The header action links to the right wallet (or nothing) per audience.
- [ ] Changing name/phone/avatar/password/preferences persists.

**Task 8 — special offers**
- [ ] Creating an offer with two products returns to `/vendor/offers` with a success notice **and the offer is in `promo.offers`**.
- [ ] `/vendor/offers` lists it with both products, names and prices.
- [ ] Editing it preserves the products and resets `admin_status` to `pending`.
- [ ] An offer referencing another vendor's variant is refused.

**Task 9 — import preview**
- [ ] For a file where column B has a blank cell in row 1, the preview row 1 shows a blank under B and the correct values under A and C.
- [ ] A file with one entirely blank column still renders a preview.
- [ ] The same holds on the vendor ingest, saving-products, team, admin catalogue, admin images and smart-order previews.

**Task 10 — account menu**
- [ ] For each of the seven actor profiles, every menu `href` returns 200/3xx, never 404.
- [ ] A vendor's orders link goes to `/vendor/orders`.
- [ ] A member without `pharmacy.wallet.view` sees no wallet item.
- [ ] An admin sees admin destinations, all permission-gated.

**Task 11 — geolocation & compatibility**
- [ ] `موقعي الحالي` works on Chrome, Edge, Firefox, macOS Safari and iOS Safari over HTTPS on all eight templates that render it.
- [ ] Over plain HTTP the button explains that HTTPS is required rather than failing silently.
- [ ] Denied permission, unavailable position and timeout each produce a distinct message.
- [ ] The button label is restored after the attempt.
- [ ] On the browsers named in `docs/BROWSER_SUPPORT.md`, `/`, `/catalog` and `/checkout` render with correct colours in both themes.

**Task 12 — payment methods**
- [ ] Adding a bank / InstaPay / wallet / card method from `/customer/wallet` and `/vendor/wallet` succeeds.
- [ ] Editing an existing method loads its current values and saves.
- [ ] Setting a default clears the previous default.
- [ ] A member without `*.wallet.manage` cannot add one by any route.

**Task 13 — variant edit**
- [ ] Editing only the price leaves SKU, barcode, English name, unit, cost price, cost discount, status and negotiability unchanged (verify by SELECT before/after).
- [ ] The modal prefills status and negotiability from the row.
- [ ] Adding a new variant persists every field and creates the stock row.
- [ ] A duplicate SKU within the org gives a readable message, not a 500.

**Task 14 — organization data**
- [ ] Editing the trade name changes what `/suppliers/{id}`, `/admin/organizations` and `/market-discounts` display.
- [ ] Each of the five sections saves independently.
- [ ] A gated section creates a pending request and does not change the live row.
- [ ] The admin page lists it, shows the before/after, and approving applies exactly the proposed keys.
- [ ] A second pending request for the same section is refused with a readable message.
- [ ] The organization `type` is not silently changed by saving.

**Task 15 — package statements (vendor)**
- [ ] Each active purchase card has an `استهلاك الباقة` button.
- [ ] The statement lists every consumption with date, reason, linked item, delta and running balance.
- [ ] Creating an ad adds an entry; rejecting it adds the refund entry.
- [ ] `credits_used` can never exceed `credits_total` (try two concurrent ads against the last 2 credits).

**Task 16 — cart, checkout, orders**
- [ ] Checking out with an offer in the cart creates the order.
- [ ] Every validation refusal names what to fix; none shows the bare `بيانات الطلب غير صالحة`.
- [ ] The offer quantity cannot be changed on the offer page or in the cart; normal products keep their stepper.
- [ ] The order edit is an inline form; there is no modal and no "add item" control.
- [ ] A single-line order cannot have that line removed or reduced below 1.
- [ ] After any edit, `orders.total_amount == SUM(order_shipments.total_amount)` and each shipment's subtotal equals the sum of its own lines.
- [ ] Verified for: same-vendor products; same-vendor offer + products; two vendors; two vendors where one is an offer.

**Task 17 — admin packages**
- [ ] `/admin/offers-packages` has an organizations tab listing every purchasing organization with credits bought/used/remaining.
- [ ] Drilling into an organization lists its purchases; each has a `كشف حساب للباقة` button.
- [ ] The admin statement and the vendor statement render from the same component and show the same numbers.

**Task 18 — imports**
- [ ] `public.river_job` contains rows after every import kind.
- [ ] Restarting the web process mid-import does not lose the run; the progress page reconnects and finishes.
- [ ] The saving-products **commit** shows a progress page that reaches 100% only when the write is done.
- [ ] The vendor saving-products progress advances without a manual refresh.
- [ ] A killed worker leaves the run in `processing`; the sweeper marks it `failed` within 30 minutes with a readable message.
- [ ] Re-running a commit job does not duplicate rows.
- [ ] `/compare/upload` returns immediately and the batch completes in the background.

---

# Regression checklist (existing behaviour that must not break)

**Authentication & authorization**
- Wrong-audience access still 404s: a vendor spelling `/customer/*`, a pharmacy spelling `/vendor/*`, either spelling `/admin/*`.
- A member lacking a permission still gets 404 (not 403, not a blank page) on the gated page.
- Pending / rejected / suspended organizations still land on `/documents` or `/onboarding/pending`.
- `POST /vendor/password` and `POST /customer/password` remain ungated (a locked-out member must still be able to change their password).
- `test/rbac_guard_test.go`, `test/admin_guard_test.go`, `test/route_audience_test.go` pass unchanged except for deliberate, reviewed edits.

**Catalogue & search**
- Arabic fuzzy search on `/catalog` still finds products by scientific name, active ingredient, SKU and barcode.
- Category, brand, dosage form, price range, `in_stock` and `has_discount` filters still narrow the list.
- The institutional-works gate (`AllowedWorkIDs`, `FilterMode`) still restricts what a pharmacy sees.
- Anti-scraping: honeypot, page-depth cap, `h.scrape.Protect` on `/catalog` and `/customer/suppliers`.

**Commerce**
- Checkout with only normal products, one vendor, no offer.
- Availability refusals (out of stock, out of coverage) still block checkout with their own Arabic reason.
- Delivery quotes are still per vendor, measured from that vendor's own branch.
- Negotiation orders (`/customer/negotiate-order`) still work.
- Vendor order status transitions and negotiation accept/reject.
- Invoices and the printable invoice.

**Promotions**
- Ads still render in all five placements and record impressions/clicks.
- Ad edit requests still go through `pending_changes` and `POST /admin/ads/{id}/approve-edit`.
- Sponsorship requests: purchase, request, admin approve/reject, credit refund on rejection.
- The storefront highlight sections.

**Imports (after Wave 6)**
- Column auto-detection still picks the right columns for the sample files in `test/corpus`.
- The AI matching stage still runs and still records usage in `ai.usage_events`.
- The deterministic fallback still produces a usable result with `GATEWAY_ENABLED=false` (AGENTS.md rule 3).
- Compare-tool quota and eviction still behave for a batch of eight files.
- The column-mapping wizard still advances through `setup_queue`.

**Data integrity**
- `test/schema_consistency_test.go`, `test/check_constraints_test.go`, `test/nullscan_test.go`, `test/money_roundtrip_test.go` pass.
- `cmd/migratecheck` reports no drift.
- Every new tenant-owned table has an RLS cross-tenant test (CI gate per AGENTS.md).

**Build gates**
- `make check` green: `fmt-check vet lint test check-provider-isolation check-prompt-version check-file-size check-inline-styles check-error-swallow check-no-cdn check-file-size-count check-hardcoded-arabic check-emoji check-unused-components check-important check-backdrop-filter check-transition-all check-breakpoints check-physical-properties check-topbar-impls check-modal-legacy check-modal-handwritten check-css-layered check-deadcode check-undefined-classes`.
- `templ generate` re-run and the `_templ.go` files committed.

---

# Risky areas — extra validation before calling the work done

1. **Task 4a reverses a deliberate decision made on 2026-09-03.** `market_discounts.go`'s header comment argues the page must show real inventory, not spreadsheet rows, because the spreadsheet version showed "forty-six thousand rows at 100% خصم / 0.00 ج.م that no pharmacy could order". After the change, **every card is non-orderable** (`matched_product_id` is NULL for all 94,308 temp-warehouse rows). Confirm with the product owner that a read-only price-intelligence board is what is wanted, and make the non-orderable state honest and deliberate rather than a broken button. Also re-check the data-quality problem the old comment describes: with `discount = 0` on ~6% of live rows and mis-mapped SKUs, the board will show bad numbers until the upload mapping (Task 4's `AnalyzeLayout` fix) is applied **and the existing 226 files are re-mapped**.

2. **Task 3's ORDER BY over 20k products with an `EXISTS` subquery.** Measure before and after with `EXPLAIN (ANALYZE, BUFFERS)` at deep offsets. If it regresses, the fallback is a materialised `has_stock` flag on `catalog.product_index` refreshed by the existing `catalog.reindex` River job — but that introduces staleness, which is its own risk. Do not guess.

3. **Task 7 moves `/settings` between route tiers.** `test/route_audience_test.go` walks the source and will fail loudly, which is good — but the move also changes who can reach `/settings/payment-methods*`. Re-derive the full list of `/settings/*` routes and assign each to a tier explicitly; do not move the whole block.

4. **Task 7 removes `/settings/organization`, which is the pharmacy's only organization-edit path.** Do not remove it until Task 14d gives pharmacies `/customer/organization`, or you will take away a working feature.

5. **Task 14's approval workflow changes who can change a company's identity.** Get the section policy (`sectionsNeedingApproval`) confirmed before building, and make sure an organization cannot be left unable to fix a wrong phone number while an identity request is pending.

6. **Task 16c changes money arithmetic on existing orders.** Changing `subtotal` from gross to net makes historical orders' stored totals inconsistent with newly-edited ones. Decide: (a) leave historical rows alone and only apply the new arithmetic on edit, accepting a one-time discontinuity, or (b) write a backfill. Whichever you choose, add an assertion that `total_amount = subtotal - discount + shipping + tax` and run it over every existing order **before** shipping, to see how many are already inconsistent.

7. **Task 18 is the largest change and the only one that needs an operational change** (a running worker). If the worker is not deployed, every import silently queues and never completes — worse than today. Gate the conversion behind a config check that refuses to enqueue when no worker has claimed a job recently, and fall back to the current in-request path until the worker is confirmed. Convert **one** importer first, run it in production for a few days, then continue.

8. **`promo.special_offers` is empty but referenced by a runtime `to_regclass` probe** in `commerce/postgres/carts.go`. Dropping it (Task 8, step 9) touches the cart read path for every pharmacy. Do it last, alone, after the probe is removed, and verify the cart on staging first.

9. **Deleting `settings_employees_handlers.go` / `vendor_team_page_handlers.go` (Task 1)** will move `check-deadcode` in both directions and may break `internal/ui/account_lifecycle_matrix_test.go`, which asserts `/settings/employees/create` and `/settings/organization/member` exist. Read that test before deleting.

10. **The `@layer` baseline (Task 11b).** Every stylesheet except `app.css` and `invoice_printable.css` is wrapped in `@layer` (`check-css-layered` enforces it). A browser without `@layer` support ignores those blocks entirely and renders a completely unstyled site — not a degraded one. Whatever minimum version you publish, verify it on a real device, not a user-agent string.

11. **RTL.** Nine of the eighteen tasks touch layout. `check-physical-properties` enforces logical properties, but flex `order`, `text-align`, icon direction and the new comboboxes' dropdown anchoring all need visual checking in both directions.

12. **The live database used for this analysis is shared.** None of the migrations above have been applied. Before running any of them, take a dump; several (the ads CHECK, the payment-method CHECK) are not reversible without data loss if a stray value exists that this analysis did not sample.
