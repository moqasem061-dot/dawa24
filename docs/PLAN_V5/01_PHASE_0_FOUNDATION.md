# PHASE 0 — Unblock & Secure

**Prerequisite:** read `00_MASTER.md` and `docs/PARITY_AUDIT_V4.md`.
**Blocks:** every other phase.
**Estimated tasks:** 6.

## Why this phase exists

Two facts make every other phase untestable until they are fixed:

1. **No vendor can save a weekly coverage row**, and `ListOffersVisibleTo`
   INNER JOINs `workflow.weekly_coverages`. Therefore **every customer offer
   listing returns zero rows, permanently.** You cannot verify any customer
   feature you build in Phases 2–8 until a vendor can define coverage.
2. **Any staff account can drop any table** through `/admin/developers/sql`.
   Building 24 more admin screens on top of that gate multiplies the exposure.

Plus one hygiene fix (§0.4 of the master) that must land before new code is
written, or it will be copied into everything you build.

---

## TASK 0.1 — Vendor weekly coverage: read + write

**Severity:** P0. This is the single highest-value task in the entire plan.

### 0.1.1 Inspect first (mandatory)

Read these before writing anything:

```bash
# Laravel: the vendor coverage screens (7 of them)
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/EmployeeWeeklyCoverage.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/EmployeeAddWeeklyCoverage.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/EmployeeShowWeeklyCoverage.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/EmployeeBranchWeeklyCoverage.php
# and their Blade views under resources/views/livewire/employee/
# Laravel: the model and table
cat F:/Dawa\ 24/Laravel/app/Models/WeeklyCoverage.php
cat F:/Dawa\ 24/Laravel/app/Models/BranchWeeklyLocation.php
sed -n "/CREATE TABLE \`weekly_coverages\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
sed -n "/CREATE TABLE \`branch_weekly_locations\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
```

Record in `docs/modules/workflow.md`:
- the exact field list on the Laravel add/edit form
- the validation rules (`$rules` in the Livewire component)
- how `day_of_week` is numbered (Laravel vs Go — Go uses 0 = Sunday; **verify Laravel matches**)
- whether Laravel allows multiple coverage rows per branch per day
- what `coverage_from` / `coverage_to` (time window) do, and whether
  `ListOffersVisibleTo` should filter on them
- the relationship between `weekly_coverages` and `branch_weekly_locations`

**`branch_weekly_locations` does not exist in Go.** Decide from the Laravel code
whether it is (a) a genuinely separate per-branch weekly *location* concept that
needs its own table, or (b) redundant with `weekly_coverages`. Record the
decision and its reasoning in `docs/modules/workflow.md`. If (a), add the table
in Task 0.1.3.

### 0.1.2 Fix the read path

**File:** `internal/ui/vendor_handlers.go:1305` (`VendorCoveragePage`).

Current bug:
```go
var coverages []*workflow.WeeklyCoverage   // declared
// ... never assigned ...
pages.VendorCoverage(coverages, bands, lang, dir).Render(ctx, w)   // always empty
```

Required:
```go
coverages, err := h.wfSvc.ListCoverageForOrganization(ctx, actor.OrganizationID)
if err != nil {
    h.log.ErrorContext(ctx, "list weekly coverage", "error", err, "org", actor.OrganizationID)
    h.renderError(w, r, err)
    return
}
```

Add `ListCoverageForOrganization` if it does not exist:
- `internal/modules/workflow/repository.go` — interface method
- `internal/modules/workflow/postgres/repository.go` — SQL, inside `db.InReadTx`,
  **tenant-scoped (no `AsSystem`)**, joined to `org.branches` for the branch name
- `internal/modules/workflow/service.go` — service method

The page needs branch names, so return a view type
(`workflow.CoverageView` with `BranchName`), not the bare `WeeklyCoverage`.

Also fix the `bands` load in the same handler — it currently uses the forbidden
`err == nil` swallow (§0.4 of the master).

### 0.1.3 Migration (only if 0.1.1 concluded `branch_weekly_locations` is needed)

`db/migrations/080_branch_weekly_locations.up.sql` / `.down.sql`.
Rules R7, R8, R9. Then `migratecheck -from 80 -roundtrip`.

### 0.1.4 Build the write path — UI

**File:** `internal/ui/pages/vendor_coverage.templ` — currently 106 lines with
**zero** `form`, `hx-`, `@click` or `button`. It must become a working screen
reproducing the Laravel one.

Required UI, matching `EmployeeWeeklyCoverage` + `EmployeeAddWeeklyCoverage`:

1. **Weekly grid** — 7 columns (Sunday…Saturday, Arabic day names, RTL order),
   rows = branches. Each cell shows coverage radius or "غير مغطى".
2. **Add coverage form** (modal via `@components.Modal`), fields:
   - Branch selector (`@components.Combobox`) — the vendor's own branches only
   - Day of week (7 checkboxes or a multi-select — match Laravel)
   - Centre point: `@components.MapPicker` (already exists, Google Maps key is
     already wired in `cmd/server/routes.go`) **plus** manual lat/lng entry as
     the fallback, because the picker degrades when the key is absent
   - Radius in metres (`distance_meters`) — number input with the same min/max
     Laravel enforces
   - Time window `coverage_from` / `coverage_to` if Laravel has it
   - Address text (ar/en) if Laravel has it
   - Active toggle
3. **Edit** — same form, prefilled.
4. **Delete / deactivate** — with a confirm modal.
5. **Delivery bands** section — already loads `bands`; give it its own
   create/edit/delete forms if Laravel has them (check
   `EmployeeWeeklyCoverage`'s Blade view).
6. All five UI states (§0.7.4 of the master). The empty state must say what to
   do next and link to the add form.

### 0.1.5 Build the write path — routes

Add to `RegisterVendorRoutes` in `internal/ui/handlers.go`:

```go
r.Get("/vendor/coverage",                      h.VendorCoveragePage)
r.Post("/vendor/coverage",                     h.VendorCoverageCreateSubmit)
r.Post("/vendor/coverage/{id}",                h.VendorCoverageUpdateSubmit)
r.Post("/vendor/coverage/{id}/delete",         h.VendorCoverageDeleteSubmit)
r.Post("/vendor/coverage/{id}/toggle",         h.VendorCoverageToggleSubmit)
r.Get("/vendor/coverage/branch/{branchID}",    h.VendorBranchCoveragePage)
```

Handlers go in a **new file** `internal/ui/coverage_handlers.go` —
`vendor_handlers.go` is already large and rule R6 caps files at 400 lines.

Each submit handler must:
- resolve the actor, reject if `actor.OrganizationID <= 0`
- **verify the branch belongs to the actor's organization** before writing
  (do not trust the form's `branch_id`) — this is a tenancy check, write a test
- validate: `day_of_week` in 0..6, `distance_meters` > 0 and ≤ the Laravel max,
  lat in [-90,90], lng in [-180,180], time window ordering
- return `apperr.Validation` with Arabic field messages on failure
- on success `h.redirectWithNotice(w, r, "/vendor/coverage", "success", "<arabic>")`

Reuse the existing service behind `POST /api/v1/workflow/branches/{id}/coverage`
(`internal/modules/workflow/http/handlers.go:29`) — do **not** duplicate the
write logic. If the service method's signature does not fit, extend the service,
not the handler.

### 0.1.6 Sidebar

`internal/ui/layouts/vendor.templ` — add "التغطية الأسبوعية" at position 7 per
`00_MASTER.md` §0.7.3.

### 0.1.7 Tests (T1–T6, plus the chain test)

| Test | Assertion |
|---|---|
| T1 | radius/day/coordinate validation rules, table-driven, including boundaries |
| T2 | repository create/list/update/delete round-trip against real Postgres |
| T3 | vendor B cannot read or write vendor A's coverage — **zero rows**, and the write is rejected |
| T4 | `/vendor/coverage*` is in the vendor group only; a customer gets 404 |
| T5 | n/a (not an admin route) |
| T6 | page renders with rows; form submits; invalid radius surfaces an inline Arabic error |
| **T9 — the chain test** | **This is the point of the whole task.** Seed: one vendor org + branch, one customer org + branch with coordinates, one approved active offer on the vendor branch. With **no** coverage row → `ListOffersVisibleTo` returns 0. Create coverage through the **HTTP handler** covering the customer's coordinates on that weekday → returns 1. Move the customer branch outside the radius → returns 0. |

T9 must live in `test/integration/` and must be demonstrated failing (returning
0 in the middle case) against the pre-fix tree.

### 0.1.8 Completion criteria

- [ ] A vendor can create, edit, deactivate and delete coverage entirely through the UI
- [ ] The page shows existing coverage (the `coverages` variable is populated)
- [ ] T9 passes: an offer becomes visible to an in-range customer and invisible to an out-of-range one
- [ ] Dead-target scan on `vendor_coverage.templ` = 0
- [ ] Sidebar entry present
- [ ] `docs/modules/workflow.md` records the `branch_weekly_locations` decision

---

## TASK 0.2 — Per-page permission gates on the admin panel

**Severity:** P0 security.

### 0.2.1 The problem

`cmd/server/routes.go:196` gates all 74 admin routes with one check, and
`authctx.RequireStaff` (`audience.go:47`) tests one boolean:

```go
if !actor.IsStaff { notFound(w, r); return }
```

Laravel gates **every** admin page individually (`permission:products_view`,
`permission:sql-console-developer`, …). `authctx.RequirePermission` exists but
is used 15 times, all on JSON APIs, zero times on admin UI routes.

Consequence: the seeded `support` role passes `IsStaff` and reaches
`/admin/developers/sql`.

### 0.2.2 Build an HTML-appropriate permission gate

`authctx.RequirePermission` calls `httpx.Error(...)` which renders **JSON** and
returns **403**. Both are wrong for an HTML admin page: the audience policy for
this app is **404 for "you may not know this exists"** (`audience.go` header
comment), and an HTML route must render HTML.

Add to `internal/platform/authctx/audience.go`:

```go
// RequirePagePermission gates an HTML admin page on a single permission key.
// It mirrors RequirePermission's rules (super_admin and developer bypass) but
// answers with the audience policy's 404 rather than a JSON 403: a support
// agent must not learn that /admin/developers exists.
func RequirePagePermission(permissionKey string, log *slog.Logger) func(http.Handler) http.Handler
```

Behaviour:
- no actor → `redirectToLogin`
- `actor.Role` is `super_admin` or `developer` → pass (matches `RequirePermission`)
- `actor.Can(permissionKey)` → pass
- otherwise → `notFound(w, r)` and log at Warn with the actor ID, the route and the missing key

### 0.2.3 Seed the admin permission keys

Laravel's admin permission set (~50 keys) is defined in
`Laravel/database/seeders/AdminRolePermissionSeeder.php` (163 lines, `key` /
`label` / `group` triples). Go has ~35 module-scoped keys and no equivalent for
several Laravel keys.

**Inspect first:**
```bash
cat F:/Dawa\ 24/Laravel/database/seeders/AdminRolePermissionSeeder.php
cat F:/Dawa\ 24/Laravel/database/seeders/ActivityLogPermissionsSeeder.php
cat F:/Dawa\ 24/Laravel/database/seeders/ErrorLogPermissionsSeeder.php
grep -ohE "permission:[a-zA-Z0-9_|.-]+" F:/Dawa\ 24/Laravel/routes/*.php | sed 's/permission://' | tr '|' '\n' | sort -u
```

Write `db/migrations/081_admin_page_permissions.up.sql`:
- insert every Laravel admin permission key that has no Go equivalent, into
  `identity.permissions` (`key`, `name` bilingual, `module`, `description`)
- **keep the Go naming convention** (`<module>.<resource>.<action>`), and record
  the Laravel-key → Go-key mapping in a table in `docs/modules/identity.md`.
  Example: `products_view` → `catalog.product.view`;
  `sql-console-developer` → `platform.developer.sql`;
  `offer_sponsorships_update` → `promo.sponsorship.update`.
- grant the full set to `super_admin` and `admin`
- grant a **deliberately narrow** set to `support` — inspect what Laravel's
  support-equivalent role holds; if Laravel has no such role, grant read-only
  keys only (`*.view`, `*.read`) and record the decision
- grant `platform.developer.*` to `developer` only

`.down.sql` deletes exactly those rows.

### 0.2.4 Apply the gate to all 74 admin routes

Rewrite `RegisterAdminRoutes` in `internal/ui/handlers.go` so each route or
route cluster is wrapped. Pattern:

```go
func (h *UIHandler) RegisterAdminRoutes(r chi.Router) {
    // Dashboard is reachable by any staff member.
    r.Get("/admin/dashboard", h.AdminDashboardPage)

    r.Group(func(g chi.Router) {
        g.Use(authctx.RequirePagePermission("catalog.product.view", h.log))
        g.Get("/admin/products", h.AdminProductsPage)
        g.Get("/admin/products/sample.csv", h.AdminProductsSampleCSV)
        g.Get("/admin/products/sample.xlsx", h.AdminProductsSampleXLSX)
    })
    r.Group(func(g chi.Router) {
        g.Use(authctx.RequirePagePermission("catalog.product.update", h.log))
        g.Post("/admin/products/{id}/edit",   h.AdminProductEditSubmit)
        g.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
        g.Post("/admin/products/import",      h.AdminProductsImportSubmit)
    })
    r.Group(func(g chi.Router) {
        g.Use(authctx.RequirePagePermission("catalog.product.delete", h.log))
        g.Post("/admin/products/{id}/delete", h.AdminProductDeleteSubmit)
    })
    // ... etc
}
```

`RegisterAdminRoutes` will exceed 400 lines. Split it: keep the function in
`handlers.go` and move the per-area groups into
`internal/ui/admin_routes_<area>.go` files (`admin_routes_catalog.go`,
`admin_routes_org.go`, `admin_routes_billing.go`, …), each exporting
`func (h *UIHandler) registerAdmin<Area>Routes(r chi.Router)`.

**Read permissions gate GET; write permissions gate POST.** Never gate a POST
with a `.view` key.

The full route → permission mapping table must be written into
`docs/modules/identity.md` so Phase 10 can verify it.

### 0.2.5 Extend the guard test

`test/admin_guard_test.go` currently proves module API sub-routers carry
`RequirePermission`. Extend it to walk every route registered by
`RegisterAdminRoutes` and fail if any route has no `RequirePagePermission` in
its middleware chain — except an explicit allowlist (`/admin/dashboard`).

The test must enumerate routes from the **real router** (`chi.Walk`), not from a
hand-maintained list, or it will drift the way `route_audience_test.go` did.

### 0.2.6 Tests

| Test | Assertion |
|---|---|
| T5a | a `support` actor requesting `/admin/developers` gets **404** |
| T5b | a `support` actor POSTing `/admin/developers/sql` gets **404** |
| T5c | an `admin` actor with `catalog.product.view` gets 200 on `/admin/products` |
| T5d | an `admin` actor **without** `catalog.product.delete` gets 404 on the delete POST |
| T5e | `super_admin` and `developer` bypass every gate |
| T5f | the guard test fails when a new admin route is added without a permission |

T5f is proven by temporarily adding an ungated route, running the test, seeing
it fail, then removing it.

### 0.2.7 Completion criteria

- [ ] Every admin route except `/admin/dashboard` carries `RequirePagePermission`
- [ ] The route → permission map is documented in `docs/modules/identity.md`
- [ ] `support` cannot reach the developer section
- [ ] `test/admin_guard_test.go` walks the real router and fails on an ungated route
- [ ] T5a–T5f pass, each demonstrated failing first

---

## TASK 0.3 — Harden the SQL console

**Severity:** P0 security. Depends on Task 0.2.

### 0.3.1 The problem

`internal/modules/platform_admin/postgres/repository.go:455` `ExecuteSQL`:
- runs inside `database.AsSystem(ctx)` — **RLS bypassed**
- routes non-`SELECT` statements to `tx.Exec` — **DDL and DML permitted**
- calls `ensureDeveloperTables` (line 470), creating schema at request time,
  outside `db/migrations` and invisible to `migratecheck`

### 0.3.2 Required changes

1. **Read-only by default.** Reject anything that is not `SELECT`, `WITH … SELECT`
   or `EXPLAIN`. Do the check on the **parsed statement**, not a string prefix —
   a prefix check is defeated by `WITH x AS (DELETE … RETURNING *) SELECT * FROM x`.
   Minimum viable approach: run the statement inside a transaction with
   `SET LOCAL transaction_read_only = on` and let Postgres reject writes. That
   is authoritative and cannot be tricked. Keep the prefix check as a fast
   pre-filter for a friendlier error message.
2. **Statement timeout.** `SET LOCAL statement_timeout = '10s'` inside the same
   transaction.
3. **Row cap.** Wrap or truncate at 1000 rows and tell the user it was truncated.
4. **Always roll back.** The transaction must never commit.
5. **Move `ensureDeveloperTables` into a real migration**
   (`db/migrations/082_developer_tables.up.sql`): `platform_admin.sql_query_logs`
   and `platform_admin.error_logs`. Match the Laravel columns — the Laravel
   `full_error_logs` table has 69 columns; port the ones that are meaningful in
   Go (drop `php_version`, `laravel_version`; keep `error_uuid`, `error_type`,
   `error_group`, `error_class`, `severity`, `status_code`, `title`, `message`,
   `trace`, `request_id`, `route_name`, `url`, `method`, `ip_address`,
   `user_agent`, `execution_time`, `is_fixed`, `fixed_by`, `fixed_at`,
   `developer_notes`, `tags`, `meta`, `user_id`, `organization_id`). Record the
   dropped columns in `docs/modules/platform_admin.md`.
   Delete `ensureDeveloperTables` from the repository.
6. **Log every execution** to `platform_admin.sql_query_logs` — actor, query,
   duration, row count, error. Laravel has `sql_query_histories`; match its
   intent.

### 0.3.3 Tests

- `INSERT`, `UPDATE`, `DELETE`, `DROP`, `TRUNCATE`, `ALTER`, `CREATE` each rejected
- the CTE-write bypass (`WITH x AS (DELETE …) SELECT …`) rejected
- a slow query is cut off by the statement timeout
- a >1000-row result is truncated and flagged
- every execution appears in `sql_query_logs`
- combined with T5b: a `support` actor cannot reach the endpoint at all

### 0.3.4 Completion criteria

- [ ] Only read statements execute, enforced by `transaction_read_only`
- [ ] Timeout and row cap in place
- [ ] `ensureDeveloperTables` deleted; tables created by migration 082
- [ ] Every execution logged
- [ ] All tests above pass

---

## TASK 0.4 — Eliminate all 60 swallowed-error sites

**Severity:** P0 for maintainability. Do this **before** writing new code, or
the pattern gets copied into every new page.

### 0.4.1 Find them

```bash
grep -rn 'err == nil {' internal/ui/*.go
```
Expect ~60. Also check `internal/modules/*/` for the same shape.

### 0.4.2 Fix each one

Apply the decision rule from `00_MASTER.md` §0.4:

| Situation | Action |
|---|---|
| The page is meaningless without this data | log at Error, `h.renderError(w, r, err)`, return |
| It is a secondary widget | log at Warn, set `data.<X>Unavailable = true`, render `@components.ErrorState` in that section |
| The value is genuinely optional (a nullable lookup) | keep it optional but still log at Debug and add a comment saying why silence is correct |

**Never** leave a bare `err == nil` swallow. If you decide a site is genuinely
fine, it still gets a comment explaining why.

Templates must gain the corresponding `Unavailable` branches — a struct field
nobody renders is not a fix.

### 0.4.3 Prevent recurrence

Add to `.golangci.yml`:
- `errcheck` is already enabled — confirm it is not excluding `internal/ui`
- add a `forbidigo` or custom `ruleguard` rule matching
  `if $_, err := $_; err == nil` in `internal/ui/**`, or if that is impractical,
  add a `make check-error-swallow` target using the grep above with a `wc -l`
  assertion of 0, and wire it into `make check`

### 0.4.4 Completion criteria

- [ ] `grep -rn 'err == nil {' internal/ui/*.go | wc -l` returns 0
- [ ] Each former site either surfaces an error or carries a justifying comment
- [ ] Templates render the degraded state where one was introduced
- [ ] `make check` fails if the pattern is reintroduced

---

## TASK 0.5 — Wire the orphans

Small, fast, high visibility. Do it now so the system stops looking broken.

### 0.5.1 `JobApplySubmit`

`internal/ui/jobs_handlers.go:72` exists and is registered nowhere.

- Register `r.Post("/jobs/{id}/apply", h.JobApplySubmit)` — decide the audience
  by reading Laravel: `Customer/ShowJobOpportunities` is a customer screen, and
  `/job-offers/{id}` is public. If Laravel lets a signed-out visitor apply, it
  goes in `RegisterPublicRoutes`; if it requires login, `RegisterSharedRoutes`.
  **Inspect `Laravel/app/Livewire/Front/ShowJobOfferPage.php` and
  `Customer/ShowJobOpportunities.php` before choosing.**
- Add the apply form to `internal/ui/pages/jobs.templ` with the same fields
  Laravel collects (CV upload, cover note, contact — read the Blade view).
- CV upload goes through the existing attachments/storage path.
- T6: submitting creates an `hr.job_applications` row; the vendor sees it under
  `/vendor/jobs`.

### 0.5.2 Link the 11 orphaned admin pages

These are registered and working but absent from
`internal/ui/layouts/admin.templ`:

`/admin/audit` · `/admin/messages` · `/admin/content` · `/admin/translations` ·
`/admin/jobs` · `/admin/policies` · `/admin/finder` · `/admin/services` ·
`/admin/plans` · `/admin/vendors` · `/admin/suppliers`

Add them to the admin sidebar in the grouping Laravel uses (see the Laravel
admin sidebar extraction in `06_PHASE_5_ADMIN.md` §5.2 — do that extraction now
if you are doing this task first). Each entry must be wrapped in the permission
check added in Task 0.2, so a staff member without the permission does not see a
link that 404s.

**Sidebar entries must be permission-aware.** Add a helper to the layout:
```
if actor.Can("catalog.product.view") { <a href="/admin/products">…</a> }
```

### 0.5.3 Completion criteria

- [ ] Unregistered-handler scan returns 0
- [ ] Every admin route reachable from the sidebar, or deliberately hidden with a recorded reason
- [ ] No sidebar link 404s for the actor who can see it

---

## TASK 0.6 — Resolve the open schema decisions

These are recorded in `PARITY_AUDIT_V4.md` PART 8. They must be settled before
later phases build on them.

### 0.6.1 `saving_products` (audit §8.1)

Migration 071 dropped `catalog.saving_products` claiming its semantics are
"superseded by promo offers". The `/what-in` page lists منتجات التوفير as a
**separate customer pillar alongside** Offers, with admin, vendor and customer
screens (10 + 3 + 3 Laravel routes).

**Decision for this plan: reinstate it.** It is a distinct concept — a
customer/vendor-owned list of products with quantity and price used for savings
tracking, not a time-boxed promotional offer.

Action: create `db/migrations/083_saving_products.up.sql` reproducing the
Laravel table on Go conventions:

| Laravel column | Go column | Notes |
|---|---|---|
| `id` | `id BIGSERIAL` | |
| — | `public_id TEXT` | Go convention, see neighbouring tables |
| `user_id` | `user_id BIGINT REFERENCES identity.users(id)` | |
| `user_organization_id` | `user_organization_id BIGINT REFERENCES org.organizations(id)` | **inspect the Laravel model to confirm what this points at** |
| `organization_id` | `organization_id BIGINT NOT NULL REFERENCES org.organizations(id)` | RLS key |
| `product_id` | `product_id BIGINT REFERENCES catalog.products(id)` | nullable — unmatched rows are allowed |
| `name_product` | `name_product TEXT NOT NULL` | the raw name as uploaded |
| `sku` | `sku TEXT` | |
| `qty` | `qty NUMERIC(10,2) DEFAULT 0` | |
| `price` | `price NUMERIC(12,2) DEFAULT 0` | money → `NUMERIC`, read as `money.Amount` |
| `deleted_at`/`created_at`/`updated_at` | same | soft delete |

RLS on `organization_id` (rule R8). Arabic comments (R9).
Table goes in the `catalog` schema.

Record the decision and its reasoning in `docs/modules/catalog.md`.

The screens are built in Phase 6 (vendor), Phase 7 (customer) and Phase 5 (admin).

### 0.6.2 `commerce.orders.user_address_id` FK target (audit §8.3)

Migration 063 points it at `identity.user_address_histories`; Laravel's
`main_orders.user_address_id` points at `user_addresses`.

**Inspect** `Laravel/app/Models/MainOrder.php` and `UserAddress.php` /
`UserAddressHistory.php`. Determine whether Laravel snapshots the address at
order time (in which case the history target is arguably better and should be
kept, with a note) or references the live address.

Whichever you conclude: record it in `docs/modules/commerce.md` with the
evidence. If a migration is needed, add it now — later phases display order
addresses and must not be built on an ambiguous FK.

### 0.6.3 `billing.plans` vs a compare-plan family (audit §8.2)

`compare_handlers.go:53` subscribes against `billing.plans` with a `"compare"`
string. Laravel has a separate plan family with features, requests, and
seat/session tables.

**Decision: defer to Phase 2**, which builds the compare plan tables. Record in
`OPEN_QUESTIONS.md` that the current `"compare"` string subscription is
temporary and will be migrated in Phase 2 Task 2.1, including what happens to
subscriptions taken before then.

### 0.6.4 The `support` role (audit §8.4)

Settled by Task 0.2.3 — either it gets a narrow permission set, or it stops
being seeded. Record which, in `docs/modules/identity.md`.

### 0.6.5 Completion criteria

- [ ] `catalog.saving_products` exists, with RLS and Arabic comments
- [ ] The `user_address_id` FK question is answered in writing with evidence
- [ ] `OPEN_QUESTIONS.md` records the compare-plan deferral
- [ ] The `support` role's surface is defined

---

## PHASE 0 COMPLETION GATE

Do not start Phase 1 until **all** of these pass:

```bash
make check                                          # green
go run ./cmd/migratecheck -from 80 -roundtrip       # green
grep -rn 'err == nil {' internal/ui/*.go | wc -l    # 0
go test ./test/... -run 'Audience|AdminGuard'       # green
go test ./test/integration/... -run Coverage        # T9 green
```

- [ ] **A vendor can save weekly coverage through the UI, and an in-range customer sees the offer** (T9)
- [ ] Every admin route carries a permission gate; `support` gets 404 on `/admin/developers`
- [ ] The SQL console is read-only, timed out, row-capped and logged
- [ ] Zero swallowed errors in `internal/ui`
- [ ] Zero unregistered handlers, zero dead template targets
- [ ] `catalog.saving_products` reinstated
- [ ] `PROGRESS.md` updated for tasks 0.1–0.6
- [ ] `OPEN_QUESTIONS.md` contains every decision you had to infer

**The single most important line in this gate:** if T9 does not pass, nothing
you build in Phases 2–8 can be verified end to end. Do not proceed.
