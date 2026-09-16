# Dawa24 Store — Master Execution Plan

**Audience:** an autonomous coding agent with **no prior context** on this repository.
**Scope:** unify the purchase-availability module across every buying surface, make
Corporate Operations (الأعمال المؤسسية) correct, and close 42 reported defects.
**Status of this document:** every "Root cause" below was *verified* against the working
tree and the live production database on 2026-09-09. Line numbers are from that snapshot;
re-check them before editing, they drift.

---

## PART 0 — READ THIS FIRST

### 0.1 What the product is

Dawa24 is a **B2B pharmaceutical marketplace for Egypt**. Pharmacies buy from suppliers.
Suppliers also buy from other suppliers — that is not an edge case, it is a first-class
requirement and it is the reason this plan exists. Any code that says "the buyer is a
pharmacy" is wrong.

**Arabic is the primary language.** Every user-facing string is bilingual and RTL-first.

### 0.2 Where the code is

```
F:\Dawa 24\dawa24-store          <- the Go modular monolith. All work happens here.
```

Other directories under `F:\Dawa 24\` (`Laravel/`, `Dawa24/`, `docs/rebuild/`) are the
**legacy system being replaced**. Read them only for reference. Never edit them.

### 0.3 Non-negotiable rules of this codebase

These are enforced by CI gates. Breaking one fails the build.

1. **Money never touches `float64`.** Use `internal/shared/money.Amount`. The DB uses
   `NUMERIC(p,2)`.
2. **No AI provider or model names outside `internal/platform/gateway/`.** The rest of the
   app asks for a *capability* or a *role*, never a model. Gate: `check-provider-isolation`.
3. **Every AI capability has a deterministic fallback.** A pharmacy must still be able to
   order, and a supplier must still be able to import, with the AI Gateway switched off.
4. **Tenant-scoped queries run inside `db.InTx` / `db.InReadTx`.** Those set the Postgres
   GUC that row-level security reads. Never call `db.Pool()` from a module. Cross-tenant
   reads must go through `database.AsSystem(ctx)`, which is deliberately greppable.
5. **Module boundaries.** `internal/modules/A` may **not** import `internal/modules/B`.
   Modules may import `internal/shared/*` and `internal/platform/*`. `platform/*` may not
   import `modules/*`. Cross-module needs are solved with an interface declared by the
   consumer and implemented in the composition root (`cmd/server/`). `golangci-lint`
   (depguard) enforces this.
6. **400 lines per Go file, maximum** (excluding `*_templ.go`). Split by concern:
   `domain.go`, `service.go`, `repository.go`, `http/`, `jobs/`.
   *Note: 71 files currently exceed this. See §0.8 — do not make it worse.*
7. **Never edit an applied migration.** The runner checksums them. Add a new one.
8. **No hardcoded user-facing Arabic in `.go` files.** Add a key to
   `internal/shared/i18n` and call `i18n.T(lang, key)`. Gate: `check-hardcoded-arabic`
   (ceiling 0). Arabic inside `.templ` files is currently tolerated but new work should
   use i18n keys.
9. **No CDN links in templates.** htmx, Alpine and Leaflet are vendored under
   `internal/ui/static/vendor`. Gate: `check-no-cdn`.
10. **No raw `<dialog>` in page templates** beyond the existing ratchet ceiling. Use
    `components.Modal`. Gate: `check-modal-handwritten`.

### 0.4 Repository layout

```
cmd/server/          HTTP entrypoint AND the composition root (wiring lives here)
cmd/worker/          background job runner (River queue) — its OWN composition root
cmd/cli/             migrate, seed, reset
cmd/dbcheck/         schema verification helpers
internal/platform/   infrastructure: config, database, cache, storage, queue, httpx,
                     gateway (AI), rbac, progress, antiscrape, errtrack, importrun
internal/shared/     dependency-free leaves: money, i18n, arabic, apperr, pagination,
                     matchflow, productmatch, sheet, format
internal/modules/    one bounded context each, mirroring a Postgres schema:
                     identity org catalog inventory commerce promo billing ingest
                     workflow hr platform_admin compare smartorder assistant chat
                     notifications attachments aicapabilities etl
internal/ui/         ALL server-rendered HTTP handlers + routes (one big package)
internal/ui/pages/   templ templates (*.templ + generated *_templ.go)
internal/ui/components/  shared templ components (Modal, B2BPagination, icons, …)
internal/ui/static/  css/ js/ img/ vendor/
db/migrations/       NNN_name.up.sql / NNN_name.down.sql, embedded in the binary
docs/                architecture notes and prior plans
```

**Important:** the UI layer is *not* split per module. `internal/ui/` holds every handler
for every dashboard. Routes are registered in:

| File | Registers |
|---|---|
| `internal/ui/admin_routes_catalog.go` | `/admin/products`, `/admin/saving-products`, `/admin/match-decisions`, temp warehouses, imports |
| `internal/ui/admin_routes_commerce.go` | `/admin/orders`, `/admin/offers`, `/admin/finance`, `/admin/plans`, `/admin/offers-packages` |
| `internal/ui/admin_routes_identity.go` | `/admin/users`, `/admin/roles`, `/admin/employee-activities` |
| `internal/ui/admin_routes_org.go` | `/admin/organizations`, `/admin/approvals`, `/admin/branches`, `/admin/weekly-coverages`, `/admin/institutional` |
| `internal/ui/admin_routes_platform.go` | `/admin/settings`, `/admin/cities`, `/admin/analytics`, `/admin/messages`, `/admin/trash-list`, `/admin/developers` |
| `internal/ui/admin_routes_pagecontrol.go` | `/admin/system-pages` |
| `internal/ui/vendor_routes.go` | most `/vendor/*` |
| `internal/ui/vendor_catalog_routes.go` | `/vendor/products`, `/vendor/quotas`, `/vendor/warehouses`, `/vendor/ingest`, `/vendor/decision-memory` |
| `internal/ui/customer_routes.go` | most `/customer/*` |
| `internal/ui/buying_routes.go` | the **shared buying surface**: `/customer/catalog`, `/cart`, `/checkout`, `/orders`, `/customer/suppliers`, `/suppliers/{id}` |
| `internal/ui/public_routes.go` | public pages, `/catalog`, `/suppliers`, `/compare/*`, auth |

`buying_routes.go` is shared by pharmacies **and** suppliers. That is the design.

### 0.5 Build, generate, test

```bash
cd "F:/Dawa 24/dawa24-store"

# 1. Regenerate templates AFTER editing any .templ file.
#    *_templ.go files ARE committed (the Dockerfile does not run templ).
templ generate

# 2. Compile everything.
go build ./...

# 3. Static analysis.
go vet ./...

# 4. Tests (unit only; integration needs a database).
go test -short -count=1 ./...

# 5. Full test run with race detection.
go test -race -count=1 ./...

# 6. Every CI gate at once (needs GNU make + bash; see below).
make check
```

**`make` is not installed on this machine.** `bash` is (`/usr/bin/bash`), and `templ` is at
`/c/Users/mydwa/go/bin/templ`. If `make` is unavailable, run the individual gate commands
by copying them out of the `Makefile` — each target is a short shell snippet. The
critical ones to run manually are listed in §6.2.

### 0.6 Database access

Production/staging Postgres:

```
postgres://postgres:<REDACTED-ROTATE-THIS-PASSWORD>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require
```

`psql` is **not** installed. A tiny query tool already exists at `tmp/probe/main.go`
(`tmp/` is git-ignored). Use it:

```bash
cd "F:/Dawa 24/dawa24-store"
export PROBE_DSN="postgres://postgres:<REDACTED-ROTATE-THIS-PASSWORD>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require"
go run ./tmp/probe "select count(*) from catalog.product_variants"
```

If `tmp/probe/main.go` is missing, recreate it — it is 45 lines: connect with
`github.com/jackc/pgx/v5`, run `strings.Join(os.Args[1:], " ")`, print tab-separated rows.

**Rules for the live database:**
- **READ freely.** Every diagnosis in this plan came from reads.
- **Do NOT run ad-hoc `UPDATE` / `DELETE` / `ALTER`.** Every schema or data change ships
  as a numbered migration in `db/migrations/` and is applied with `go run ./cmd/cli migrate`.
- The highest migration file is `196_account_deletion_requests_fix`. **Your first new
  migration is `197_`.** Re-check with `ls db/migrations | sort -V | tail -3` before
  numbering; other agents may have added files.

### 0.7 Key facts about the live data (as of 2026-09-09)

Knowing these prevents you from "fixing" something that is actually correct data:

| Fact | Value |
|---|---|
| Master catalogue products | 19,996 (`catalog.products`) |
| Vendor offers (variants) | 12,501 rows, of which **3,462 are `status='active'`** |
| Active variants with stock | ~3,660 across **only two suppliers**: org 187 (142) and org 192 (1,969) |
| Organizations | 7 |
| Branches | 14 |
| Institutional works | 24 rows, **with duplicates** (see §5.3) |
| Institutional work connections | 18 edges |
| Weekly coverage rows | 805 |
| Orders | 12 |
| Applied migrations | 195 of 196 |

**The most important consequence:** only ~851 of the 19,996 master products have any
sellable stock at all. Any listing that paginates `catalog.products` and then filters to
"orderable" will show almost nothing. That is defect #10 and it is not a rendering bug.

### 0.8 Baseline health — do not blame yourself for these

Recorded on 2026-09-09 before any of this work:

- `go build ./...` → **PASS** (39s cold)
- `go vet ./...` → **PASS**
- `check-file-size-count` → **FAIL**: 71 Go files exceed 400 lines against a ceiling of 0.

**Capture your own baseline before your first edit** and keep it. When a gate fails,
compare against the baseline; only regressions are yours to fix. Do not attempt a
71-file refactor as part of this work — but every file *you* create or substantially
rewrite must come in under 400 lines.

### 0.9 Concurrency hazard

Other tools may be touching this working tree while you run: GitHub Desktop, a
`templ --watch` process, and other agent sessions. Before each work order:

```bash
git status --porcelain
git stash list
```

If files you did not touch have changed, stop and re-read them. Never `git checkout --`
or `git reset --hard` to "clean up" — you may destroy another process's work.

### 0.10 How to work through this document

1. Read PART 1 in full. It is the mental model; without it the fixes look arbitrary.
2. Execute **PART 2 (Workstream A)** first and completely. It is the client's stated top
   priority and roughly a third of the numbered defects dissolve once it lands.
3. Then **PART 3 (Workstream B)** — four platform-wide fixes that each close several
   numbered defects at one call site.
4. Then PART 4, the numbered work orders, in the order given (they are dependency-ordered,
   not severity-ordered).
5. PART 5 (database hygiene) can run in parallel with PART 4.
6. PART 6 is the definition of done. Nothing ships without it.

**Commit granularity:** one commit per work order. Message format:

```
<area>: <what changed>

<why, in prose. name the defect number.>

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
```

Never bundle unrelated work orders into one commit.

---

## PART 1 — ARCHITECTURE PRIMER

You must understand these seven mechanisms before changing anything.

### 1.1 Request → tenant → RLS

`internal/modules/identity/http/middleware.go:75-105` is the auth middleware. It:

1. loads the session,
2. reads `sess.ActiveOrgID`,
3. if `> 0`, calls `ctx = database.WithTenant(ctx, activeOrgID)`,
4. builds an `authctx.Actor` (permissions resolved **live** from the DB, not from the
   session snapshot) and puts it on the context.

`database.WithTenant` stores the org id on the context. `db.InTx` / `db.InReadTx` read it
and `SET LOCAL` the Postgres GUC that RLS policies (`platform.tenant_visible(...)`) check.

**Consequence you will hit repeatedly:** a service method that starts with

```go
orgID, ok := database.TenantFrom(ctx)
if !ok { return nil, database.ErrNoTenant }
```

will fail for any handler that stripped the tenant, or for a user whose session has no
active organization. `database.ErrNoTenant` is `apperr.Forbidden("tenant.required", …)`.

`database.AsSystem(ctx)` bypasses RLS. It is correct for genuinely cross-tenant reads
(a pharmacy reading a supplier's catalogue row to decide whether it may buy it) and is
deliberately easy to grep for. **Never widen an `AsSystem` read beyond the exact fields
the caller needs.**

### 1.2 Errors and messages

`internal/shared/apperr` classifies errors: `Validation`, `NotFound`, `Unauthorized`,
`Forbidden`, `Conflict`, plus internal. `internal/ui/request_helpers.go`:

- `statusForError(err)` maps a kind to an HTTP status.
- `h.safeMessage(err, lang)` produces the user-visible string. For an `apperr` it returns
  `appErr.LocalizedMsg(lang)`; otherwise a generic message. The generic Arabic for a
  validation kind is **`بيانات الطلب غير صالحة.`** — remember this string, it is the
  symptom in defect #9.
- `h.renderError(w, r, err)` renders the full error page (or an HTMX error fragment).

### 1.3 The redirect-and-notice pattern (critical — this is defect #1)

Every form POST ends with:

```go
h.redirectWithNotice(w, r, "/admin/users", "success", i18n.T(lang, "some.key"))
```

`redirectWithNotice` (`internal/ui/request_helpers.go:169`) parses the path, sets
`?notice=<kind>&msg=<text>`, and issues a 303 (or `HX-Redirect` for HTMX).

**There are 724 call sites and almost all of them pass a bare path.** The consequence is
that after any action the browser lands on `/admin/users?notice=success&msg=…` — every
filter, search term, page number and sort the user had applied is gone, and the page is
scrolled to the top. The client describes this as "خطأ لعين موجود في المنصة كلها".

`noticeFrom(r)` reads the flash back. It tolerates two spellings (`notice`/`msg` and
`notice_type`/`notice_msg`) because admin screens historically used both.

### 1.4 Pagination

`internal/shared/pagination`:

- `TableRows = 25` — the canonical dashboard page size.
- `RowsPerPage(r)` reads `?limit=N` and honours **only** `{10, 25, 50, 100}`; anything
  else falls back to 25.
- `PageNumber(r)` reads `?page=`, minimum 1.
- `components.B2BPagination(components.PaginationProps{...})` in
  `internal/ui/components/pagination.templ` is the one pagination widget. `PaginationProps`
  carries `CurrentPage`, `PageSize`, `TotalCount`, `BaseURL`, `QueryValues url.Values`.

**`QueryValues` is how filters survive paging.** A page that omits its active filters from
`QueryValues` loses them on page 2. Audit every pagination call you touch.

The `/catalog` page deliberately does **not** use `RowsPerPage`; it has its own set
`{12, 24, 48, 96}` with default 24 (`internal/ui/customer_handlers.go:110-130`). Keep that
set — it is a card grid, not a table — but make it honest (§2.4).

### 1.5 Modals

`components.Modal(components.ModalProps{ID, Title, Size, …})` renders a **native
`<dialog class="modal">`**. It is opened by JavaScript calling `dialog.showModal()`.

`internal/ui/static/js/app.js:482-500` (`initModalManager`) delegates clicks and opens the
dialog whose id appears in **`data-modal-open`**, `data-dialog-target`, or
`data-open-modal` on the clicked element. It also handles close buttons
(`data-modal-close`, `.modal-close`), backdrop clicks, body scroll lock and focus
restoration.

**There is no listener for the Alpine custom event `open-modal`.** Two pages
(`admin_offers.templ`, `admin_ai_logs.templ`) try to open modals with
`@click="$dispatch('open-modal', 'some-id')"`. Those buttons do nothing at all. That is
defect #8.

CSS lives in `internal/ui/static/css/components.css:216-260`. `dialog.modal` is a flex
container centred with `align-items: center`. Under `@media (max-width: 768px)` it becomes
`align-items: flex-end` — a deliberate mobile bottom-sheet.

Several screens (notably the temp-warehouse pages) bypass `components.Modal` entirely and
hand-roll `<div class="fixed inset-0 z-modal flex-center bg-black/75 p-5">` overlays driven
by Alpine. Those inherit none of the manager's behaviour, and `components.css:331-339`
gives `.fixed.inset-0 > *` a `margin-block: auto` on small screens. That is defect #6.

### 1.6 templ

Templates are `.templ` files compiled to `*_templ.go` by the `templ` binary. Both are
committed. Workflow:

```bash
# edit internal/ui/pages/foo.templ
templ generate
go build ./...
```

Never hand-edit a `*_templ.go` file.

### 1.7 RBAC

`internal/platform/rbac`:

- `catalog*.go` declare every permission key (296 rows in `identity.permissions`).
- `roles.go` declares `SystemRole` values: platform roles (in `identity.roles`) and
  **organization role templates** seeded into every company's own `org.roles`.
- `provision.go:EnsureCompanyRoles(ctx, db, orgID, orgType)` seeds those templates into a
  new company. Grants are written **once**, at creation, so an owner's later edits survive
  re-runs.
- `resolver.go` answers "may this actor do X" by reading the database now, not the session.

**The role templates are hardcoded Go.** An administrator cannot change what a new company
starts with. That is defect #31.

### 1.8 The AI Gateway

`internal/platform/gateway` is the only package allowed to know model names.

- `roles.go` defines `Role` values — `assistant.primary`, `assistant.attachment`,
  `assistant.transcribe`, `matching.adjudicate`, `import.detect_columns`,
  `search.expand_query`, `support.classify` — and maps each to a model in
  `defaultRoleModels`. `resolveRoleModel(role)` checks an **environment variable** first
  (`GATEWAY_MODEL_MATCHING`, etc.), then the default.
- `config_source.go` defines `Settings{BaseURL, VirtualKey, ClientApp, Enabled, FastModel,
  QualityModel}` and a `SettingsSource` interface the composition root backs with the
  `platform_admin` module, so the admin panel can change credentials at runtime.

**Role→model is env-only and cannot be edited from the admin panel; and one role,
`matching.adjudicate`, serves all four import tools.** That is defect #39.

---

## PART 2 — WORKSTREAM A: THE UNIFIED AVAILABILITY MODULE

> This is the client's number-one priority. Do it first, do it completely.
> Defects #7, #10, #34 and #38 are symptoms of it; #11 and #16 sit next to it.

### 2.1 What "availability" means and where the one true rule lives

`internal/modules/commerce/availability.go` holds the single source of truth:

```go
func (s *Service) CheckAvailability(ctx, req AvailabilityRequest) (AvailabilityResult, error)
```

`AvailabilityRequest{VariantID, VendorOrgID, CustomerOrgID, CustomerBranchID, Quantity, When}`
→ `AvailabilityResult{Allowed, MaxQuantity, Reason, MessageAr, MessageEn}`.

It runs these checks **in this order** and returns the first failure:

| # | Check | `Reason` on failure |
|---|---|---|
| 0 | probe wired at all (fails closed) | `variant_invalid` |
| 0b | `Quantity > 0` | `quantity_invalid` |
| 1 | `VendorOrgID > 0` | `vendor_invalid` |
| 1b | buyer org ≠ vendor org | `own_organization` |
| 1c | vendor exists and is a vendor type | `vendor_invalid` |
| 1d | vendor is `approved` | `vendor_unapproved` |
| 2 | variant exists / is active / belongs to that vendor | `variant_invalid`, `variant_inactive`, `wrong_vendor` |
| 3 | stock > 0; requested ≤ stock; requested ≥ min order qty | `out_of_stock`, `insufficient_stock`, `below_minimum` |
| 4 | delivery branch exists and belongs to the buyer | `branch_invalid`, `branch_not_owned` |
| 5 | buyer branch has ≥1 institutional work | `branch_no_institutional_works` |
| 5b | buyer branch's works connect to the vendor branch's works | `branch_institutional_mismatch` |
| 6 | branch has coordinates or a city; vendor covers it on `When`'s weekday | `branch_no_location`, `not_covered` |
| 7 | per-branch quota not exhausted / not exceeded | `quota_exhausted`, `quota_exceeded` |

On success `MaxQuantity = min(stock, remaining quota)`.

The cross-module data it needs comes through `commerce.AvailabilityProbe`, an interface
declared by `commerce` and implemented **once**, in the composition root at
`cmd/server/availability.go` (`availabilityProbe`). It reaches `catalog`, `org`,
`workflow.CoverageService` and `inventory`.

`Reason.IsQuota()` (`internal/modules/commerce/quota.go:74`) exists so the UI can
distinguish three classes of refusal:

- **hide the row** — the buyer's branch can never be served (`not_covered`,
  `branch_no_location`, `branch_no_institutional_works`, `branch_institutional_mismatch`);
- **show it, disabled, with a reason** — temporarily unbuyable (`out_of_stock`,
  `insufficient_stock`, both quota reasons);
- **show it, enabled** — `below_minimum` just needs a bigger number.

That classification is currently **duplicated verbatim** in two places
(`internal/ui/offers_storefront.go:170-205` and
`internal/ui/supplier_profile_handlers.go:150-185`) and is absent from a third
(`smartorder`). Unifying it is task A2.

### 2.2 Verified defect A-1 — smart ordering applies a *different, weaker* rule

Smart Ordering re-implements availability instead of calling `CheckAvailability`:

- `internal/modules/smartorder/eligibility.go:Evaluate(OfferCheck)` runs six checks:
  own-org, product/vendor active, institutionally visible, covered, `StockQty > 0`,
  min-order-qty.
- `internal/modules/smartorder/pipeline/offers.go:buildCandidates` fills that struct.

Divergences, each of which produces a line the review screen calls orderable and
`Checkout` then refuses:

| Divergence | Where | Effect |
|---|---|---|
| **No quota check at all.** `Evaluate` never looks at `quota_limit`. | `eligibility.go` | A branch that has exhausted its allowance is offered the item, selects it, and the whole checkout fails. |
| **No `RequestedQty > StockQty` check.** Only `StockQty <= 0` is tested. | `eligibility.go:70` | A line for 100 units against 5 in stock is "orderable". `CheckAvailability` returns `insufficient_stock`. |
| **A branch with no coordinates is treated as covered.** | `pipeline/offers.go:245-252` | `CheckAvailability` refuses the same branch with `branch_no_location`. Directly contradictory. |
| **Coverage is asked without the city id.** `coverageGate` in `cmd/worker/smartorder.go` builds `workflow.Coord{Lat, Lon}` and omits `CityID`. | `cmd/worker/smartorder.go` | `ServesPoint`'s first match arm is "vendor explicitly covers this city". Smart order never uses it, so city-only coverage rows never match. `availabilityProbe.VendorCovers` *does* pass the city. |
| **Re-verification substitutes Cairo for a missing branch location.** `reverifier.Recheck` sets `lat=30.0444, lon=31.2357` when the branch has none. | `cmd/server/smartorder.go` | Coverage is computed for a location the buyer does not have. |
| **No branch-ownership check** and **no vendor-approval check in the pipeline** (only in `Recheck`). | `pipeline/offers.go` | A hand-written run against another company's branch evaluates against the wrong address. |

### 2.3 Verified defect A-2 — coverage never checks the time-of-day window

`internal/modules/workflow/coverage_service.go:ServesPoint(ctx, orgID, day, target, optWhen ...time.Time)`
accepts `optWhen` and **never reads it**. The SQL filters on `wc.day_of_week` and
`wc.is_active` only. The doc comments throughout the codebase claim coverage means
"right weekday, inside the time window, inside the radius". The time window is not
enforced anywhere.

Decide deliberately (see §7, question Q1): either enforce it, or delete the parameter and
correct every comment. Do not leave it as a lie.

### 2.4 Verified defect A-3 — the catalogue paginates the wrong thing (defect #10)

**Symptom reported:** `/customer/catalog` shows a single product on first load; changing
the rows-per-page control suddenly shows more.

**Root cause, traced end to end:**

1. `internal/ui/customer_handlers.go:CustomerCatalogPage` computes `pageSize` (default 24)
   and `offset`, then calls
   `h.catSvc.SearchWithTotal(ctx, catalog.SearchParams{… Limit: pageSize, Offset: offset})`.
2. `SearchWithTotal` → `Repository.SearchProducts` (`internal/modules/catalog/postgres/repository.go:305`)
   pages **`catalog.products`** — the 19,996-row master catalogue. Its only availability
   predicate is `productHasStockSQL`: *"some active variant of this product has
   `inventory.stocks.quantity > 0`"*. That is global. It knows nothing about the buyer,
   the vendor's approval, coverage, institutional works, own-org exclusion or quota.
3. `totalCount` is therefore a **product** count, and `totalPages = ceil(totalCount/24)`
   is a page count over products.
4. The handler then calls `h.buildCatalogVariantCards(...)`
   (`internal/ui/customer_catalog_cards.go`), which expands each product into per-supplier
   offers via `h.offersForProduct(...)` and **drops** every offer where
   `isBuyer && !off.IsCovered`, or `!off.CanAddToCart`, or `AvailableStock <= 0`.
5. So the number of cards rendered has **no relationship** to `pageSize` or to
   `totalCount`. With 851 stocked products out of 19,996, and only two suppliers, page 1
   can legitimately produce one card.
6. The "changing the limit fixes it" behaviour: the filter form also submits
   `filter_applied=1`, which flips `inStock` from `true` to `false`
   (`customer_handlers.go:88-92`), producing a *different* product window that happens to
   contain more sellable products.

**The same class of bug on the supplier profile (defect #7):**
`internal/ui/supplier_profile_handlers.go` calls `ListVendorVariants(...)` with
`PageNumber/PerPage`, receives `(variants, total)`, then builds `availableVariants` by
skipping any variant that is inactive or has no stock, and finally does
`data.Variants = availableVariants` while `data.TotalVariants = total` and
`data.TotalPages = ceil(total/limit)` still describe the **unfiltered** set. The page
promises N results and shows fewer, and paging skips rows.

### 2.5 Verified defect A-4 — institutional works have two sources of truth

`org.branches` has **no `institutional_works` column**. The Go field
`org.Branch.InstitutionalWorks []string` is populated from the join table
`org.branch_institutional_works`, which has **both** a text `work_category` and a nullable
`institutional_work_id`:

```sql
SELECT DISTINCT COALESCE(institutional_work_id::text, work_category)
FROM org.branch_institutional_works WHERE branch_id = $1
```

So a branch whose row has `institutional_work_id IS NULL` yields a **slug string**, and
`org.branchWorkIDs` (`internal/modules/org/institutional_connection.go:120-150`) then does
`strconv.ParseInt(raw, 10, 64)` on it, fails, and silently treats the branch as having no
works. Live data confirms it: `(branch_id=67, work_category='group', institutional_work_id=NULL)`.

Worse, `saveBranchInstitutionalWorksTx` (`internal/modules/org/postgres/branches_repo.go:272`):

```go
if len(works) == 0 { return }   // <- removing every work is a silent no-op
```

and it only ever `INSERT … ON CONFLICT DO UPDATE`. **It never deletes.** Un-ticking an
institutional work on a branch has no effect whatsoever. Both are hard bugs against the
client's "الأعمال المؤسسية working perfectly" requirement.

**Live evidence that this is not theoretical:** branches 68 (`Cairo-v`, vendor 187), 74,
84 and 88802 (`الفرع الرئيسي` of vendor 88800) have **zero** rows in
`org.branch_institutional_works`. Every product on those branches is unbuyable by everyone,
platform-wide, with no message anywhere explaining why.

### 2.6 Verified defect A-5 — per-branch quota is not in every surface

`internal/modules/commerce/quota.go` is well built: the cap is
`catalog.product_variants.quota_limit`; consumption is **summed from `commerce.order_lines`**
rather than kept as a counter (deliberately, so it cannot drift); `QuotaReleasingStatuses`
= `{cancelled, failed, returned, refunded}`; a supplier can "release" a branch by writing
an instant into `commerce.variant_branch_quota_releases`, after which older orders stop
counting.

`CheckAvailability` step 7 applies it. **Smart ordering does not (§2.2). The `/vendor/quotas`
screen is reported non-functional (defect #34).**

---

### A1 — Make the availability decision one shared, reusable object

**Files:** `internal/modules/commerce/availability.go`, new
`internal/modules/commerce/availability_presentation.go`.

1. Add to `commerce`:

```go
// Disposition is how a buying surface should present a refusal.
type Disposition int

const (
    DispositionOrderable Disposition = iota // show, enabled
    DispositionBlocked                      // show, disabled, with MessageAr
    DispositionHidden                       // do not list this offer for this buyer
)

// Disposition classifies a result for the UI. It is the ONLY place this
// mapping exists; three surfaces used to carry their own copy.
func (r AvailabilityResult) Disposition() Disposition { … }
```

Mapping (preserve today's behaviour exactly):

| Reason | Disposition |
|---|---|
| `ReasonOK` | Orderable |
| `not_covered`, `branch_no_location`, `branch_no_institutional_works`, `branch_institutional_mismatch`, `own_organization`, `vendor_invalid`, `vendor_unapproved`, `wrong_vendor`, `variant_invalid`, `variant_inactive` | Hidden |
| `out_of_stock`, `insufficient_stock`, `quota_exhausted`, `quota_exceeded` | Blocked |
| `below_minimum` | Orderable (with `MinOrderQty` surfaced) |
| any error from the probe | Hidden, with `offers.cov_reason_verify_failed` |

2. Add a batch entry point, because every listing needs many decisions at once:

```go
type AvailabilityLine struct {
    VariantID   int64
    VendorOrgID int64
    Quantity    int
}

// CheckAvailabilityBatch answers many lines for one buyer/branch/moment.
// It exists so a listing costs a bounded number of queries instead of one
// CheckAvailability per row.
func (s *Service) CheckAvailabilityBatch(
    ctx context.Context, customerOrgID, customerBranchID int64,
    when time.Time, lines []AvailabilityLine,
) (map[int64]AvailabilityResult, error)
```

Implementation: extend `AvailabilityProbe` with batch methods
(`VariantsByIDs`, `VendorsByIDs`, `VendorInstitutionalConnections`) and cache the branch,
the coverage verdict per vendor, and the institutional verdict per
`(vendorOrgID, vendorBranchID)` for the whole call. Keep the single-line
`CheckAvailability` as a thin wrapper over the batch so **there is one rule**.

3. **Test:** a table-driven test asserting each `Reason` maps to the intended
   `Disposition`, plus a test that `CheckAvailabilityBatch` and `CheckAvailability` return
   identical results for the same inputs. Put them in
   `internal/modules/commerce/availability_test.go` (it exists; extend it).

**Acceptance:** `go test ./internal/modules/commerce/...` passes; no surface contains its
own copy of the reason→disposition switch.

---

### A2 — Delete the duplicated classification from the two UI surfaces

**Files:** `internal/ui/offers_storefront.go` (~lines 160-210),
`internal/ui/supplier_profile_handlers.go` (~lines 140-195).

Replace both hand-written switches with `res.Disposition()`. Behaviour must not change;
this is a de-duplication, and the test from A1 is what proves it.

**Acceptance:** `grep -rn "ReasonNotCovered" internal/ui/` returns nothing.

---

### A3 — Make the catalogue paginate offers, not products (defect #10)

This is the largest single change in the plan. **Read §2.4 before starting.**

**Principle:** the unit the buyer sees is a *supplier offer* (a `catalog.product_variants`
row), so that is the unit that must be counted, ordered, filtered and paged — in SQL,
against the buyer's own eligibility.

**Step 1 — new repository method.** Add to
`internal/modules/catalog/postgres/` a new file `buyer_offers.go` (keep it under 400
lines; split into `buyer_offers_filter.go` if needed):

```go
// BuyerOfferQuery is one page of the buying catalogue, from one buyer's
// point of view.
type BuyerOfferQuery struct {
    BuyerOrgID      int64   // excluded as a seller; 0 for a signed-out visitor
    BuyerBranchID   int64   // 0 when no branch is selected
    AllowedWorkIDs  []int64 // institutional works the buyer branch may buy from
    Weekday         int
    Query           string
    CategoryID      *int64
    BrandID         *int64
    DosageForm      string
    MinPriceMinor   *int64
    MaxPriceMinor   *int64
    OnlyDiscounted  bool
    OnlyInStock     bool
    Sort            string
    Limit, Offset   int
}
```

and

```go
func (r *Repository) ListBuyerOffers(ctx, q BuyerOfferQuery) ([]*catalog.BuyerOffer, int, error)
```

The SQL selects from `catalog.product_variants v` joined to `catalog.products p`,
`org.organizations o` (the supplier) and a `LEFT JOIN LATERAL` stock rollup **identical to
the one already in `internal/modules/catalog/postgres/vendor_variants.go:stockRollup`** —
reuse that constant, do not write a second one.

The `WHERE` clause must encode, in SQL, the checks that are cheap and set-shaped:

```sql
v.deleted_at IS NULL
AND v.status = 'active'
AND p.deleted_at IS NULL
AND o.status = 'approved'
AND o.type IN ('vendor','supplier','company','agency')
AND ($buyerOrgID = 0 OR v.organization_id <> $buyerOrgID)   -- own-org exclusion
AND st.qty > 0                                              -- when OnlyInStock
AND (
      $buyerBranchID = 0                                     -- signed out: show all
      OR EXISTS (                                            -- institutional connection
        SELECT 1
        FROM org.branch_institutional_works vw
        WHERE vw.institutional_work_id = ANY($allowedWorkIDs)
          AND (
                (v.branch_id IS NOT NULL AND vw.branch_id = v.branch_id)
             OR (v.branch_id IS NULL AND vw.branch_id IN (
                   SELECT b.id FROM org.branches b
                   WHERE b.organization_id = v.organization_id
                     AND b.deleted_at IS NULL AND b.status <> 'inactive'))
          )
      )
    )
```

`$allowedWorkIDs` is computed **once per request** in Go, by
`org.Service.ConnectedWorkIDsForBranch(ctx, buyerBranchID)` — a new method that returns
`GetConnectedInstitutionalWorkIDs(branchWorkIDs(buyerBranchID))`. This is exactly what
`BranchesInstitutionallyConnected` does internally; extract it so both use one query.

**Coverage stays in Go**, applied per distinct vendor id on the returned page (typically
1-5 vendors), using `CoverageService.ServesPoint` with the branch's city id. Do not attempt
to express the radius test in this query.

**Quota stays in Go**, applied per returned row via `CheckAvailabilityBatch`, and affects
only `MaxOrderQty` and the `Blocked` disposition — never the row count. (A quota-exhausted
row must remain visible and counted; see §2.1.)

Ordering: keep the existing intent — sponsored first, then orderable, then the user's sort
— but express the first two levels in SQL (`ORDER BY sponsored_rank DESC, st.qty > 0 DESC, …`)
so it holds across the whole result set rather than within a page.

**Step 2 — service method.** In `internal/modules/catalog/service.go` (or a new
`buyer_catalog.go` if that file is near 400 lines) add
`func (s *Service) ListBuyerOffers(ctx, q BuyerOfferQuery) ([]*BuyerOffer, int, error)`.

**Step 3 — rewrite the handler.** In `internal/ui/customer_handlers.go`:

- resolve `buyerOrgID(ctx)` and `h.buyingBranchID(ctx, &actor)`;
- resolve `AllowedWorkIDs` once;
- call `ListBuyerOffers` with the page window;
- run `CheckAvailabilityBatch` over exactly the returned rows;
- drop rows whose disposition is `Hidden` **and record how many were dropped**; if more
  than 20% of a page is dropped, log a warning — that means the SQL predicate and the Go
  rule disagree, which is a bug, not a normal outcome;
- set `TotalItems` from the SQL count and `StartItem`/`EndItem` from the window.

**Step 4 — sponsored products.** Today `activeSponsoredRankings` is prepended to page 1
*outside* the pagination window, which double-counts. Keep sponsorship as an **ordering
input** to the SQL (a `LEFT JOIN` on the sponsorship tier), not as a separate list glued on
top.

**Step 5 — `filter_applied`.** Delete the `in_stock` / `filter_applied` coupling in
`customer_handlers.go:88-92`. `in_stock` defaults to **true** and is a normal checkbox.
The current logic makes an unrelated form submission silently change the result set, which
is what makes the bug look like a pagination bug.

**Acceptance criteria for A3:**
- With any buyer, page 1 of `/customer/catalog` renders **exactly `pageSize` cards**
  whenever `TotalItems > pageSize`.
- `TotalItems` equals the number of offers the buyer could reach by walking every page.
- Changing `?page_size=` changes only how the same ordered list is sliced — never which
  products appear.
- Changing `?page=` never repeats or skips a row (every ordering ends in `v.id`).
- A supplier browsing `/customer/catalog` never sees its own variants, on any page.
- Integration test: `internal/modules/catalog/postgres/buyer_offers_test.go` seeding two
  suppliers, one connected branch and one unconnected, asserting the counts.

---

### A4 — Apply the same fix to the supplier profile (defect #7)

**File:** `internal/ui/supplier_profile_handlers.go`.

Replace the `ListVendorVariants` + post-filter shape with `ListBuyerOffers` scoped to one
supplier (`BuyerOfferQuery` gains `SupplierOrgID int64`). `TotalVariants` and `TotalPages`
then describe the filtered set.

The client's instruction for this page is explicit: **the supplier directory must not list
products that are not orderable**. So on `/suppliers/{id}` and `/customer/suppliers/{id}`:

- `Hidden` rows are excluded from the query, and therefore from the count;
- `Blocked` rows (out of stock, quota exhausted) stay, disabled, with their Arabic reason —
  because a delisted-looking catalogue is worse than an honest "لا يوجد رصيد حالياً";
- a signed-out visitor sees the catalogue with an "sign in to order" call to action.

Also note `ListVendorVariants` **does** populate `StockQty` from the lateral rollup, but
`catalog.Service.GetVariant` does **not** (there is no stock column on
`catalog.product_variants`; `cmd/server/availability.go` reads
`inventory.AvailableQuantity` instead). Make sure the profile reads stock from the same
rollup as everything else, never from `v.StockQty` on a variant that came from `GetVariant`.

**Acceptance:** on `/suppliers/{id}?page=2&limit=25`, the header count, the number of rows
and the pager all agree; and no row appears whose disposition is `Hidden`.

---

### A5 — Make smart ordering call the one rule (defect #38)

**Files:** `internal/modules/smartorder/eligibility.go`,
`internal/modules/smartorder/pipeline/offers.go`, `cmd/worker/smartorder.go`,
`cmd/server/smartorder.go`.

`smartorder` may not import `commerce` (rule 5). So invert it: declare in `smartorder` a
narrow port and implement it in **both** composition roots over
`commerce.Service.CheckAvailabilityBatch`:

```go
// AvailabilityGate is the platform's purchase rule, as smart ordering sees it.
// It MUST be backed by commerce.CheckAvailability — a second implementation is
// how the review screen and the checkout came to disagree.
type AvailabilityGate interface {
    Check(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
        lines []GateLine) (map[int64]GateVerdict, error)
}

type GateLine struct{ VariantID, VendorOrgID int64; Quantity int }
type GateVerdict struct {
    Allowed     bool
    MaxQuantity int
    Reason      string // the commerce Reason string, carried opaquely
}
```

Then:

1. `pipeline.Supplier.buildCandidates` calls the gate for the whole file's candidate set
   (it already batches offers; batch this too) and sets
   `c.Eligible` / `c.IneligibleReason` from the verdict.
2. **Delete** `smartorder.Evaluate` and `OfferCheck`, or reduce `Evaluate` to a pure
   mapping from a `commerce` reason string onto a `smartorder.IneligibleReason` for the
   results screen. Keep `OutcomeFor` — the "least severe obstacle across candidates"
   ranking is good UX and belongs to smart ordering.
3. Add the two missing `IneligibleReason` values so quota refusals are reportable:
   `ReasonQuota` → new `Outcome` `OutcomeQuotaBlocked`, with Arabic and English strings and
   a counter in `Stats` alongside `CoverageBlockedRows`.
4. **Delete** the "no coordinates means covered" branch in `pipeline/offers.go:245-252`.
   The gate refuses it as `branch_no_location`, which is the same answer checkout gives.
5. **Delete** the Cairo fallback in `cmd/server/smartorder.go:reverifier.Recheck`.
6. Replace `reverifier.Recheck` entirely with a call to the same gate. It currently
   re-implements a third version of the rule.
7. `cmd/worker/smartorder.go:coverageGate` becomes unnecessary once the gate does coverage;
   remove it rather than leaving a second coverage path that omits the city id.
8. `internal/ui/smart_order_branch_gate.go:smartOrderBranchRefusal` stays — it is a cheap
   pre-flight before a run is created — but rewrite it to call the gate with a probe line
   so its answer cannot drift from checkout's. If no variant is known yet, keep the two
   direct checks it has (ownership, location, works) and add a comment saying so.

**Acceptance:**
- `internal/modules/smartorder/pipeline/institutional_test.go` still passes.
- New test: a run whose branch has exhausted its quota for a variant reports
  `OutcomeQuotaBlocked` on the results screen and `Finalize` refuses the same line with the
  same reason.
- New test: a run against a branch with no coordinates and no city reports
  `branch_no_location` at the results screen, **not** "ordered".
- `grep -rn "30.0444" cmd/ internal/` returns only genuine geographic defaults in
  presentation code, never inside an eligibility decision.

---

### A6 — Repair Corporate Operations (الأعمال المؤسسية)

**Migration `197_branch_institutional_works_integrity.up.sql`:**

1. Backfill `institutional_work_id` from `work_category` wherever it is NULL and a slug
   matches:
   ```sql
   UPDATE org.branch_institutional_works biw
   SET institutional_work_id = iw.id
   FROM org.institutional_works iw
   WHERE biw.institutional_work_id IS NULL
     AND iw.deleted_at IS NULL
     AND (iw.slug = biw.work_category OR iw.id::text = biw.work_category);
   ```
2. Report — **do not delete** — the leftovers, so an operator can decide:
   ```sql
   -- rows that still cannot be resolved are left in place; they are visible in
   -- the admin screen added by work order #WO-A6-UI below.
   ```
3. Add `NOT NULL` **only** if step 1 leaves zero unresolved rows on this database; verify
   first with the probe. Otherwise add a `CHECK` in a later migration once the data is clean.
4. Add `CREATE INDEX IF NOT EXISTS idx_biw_work_id ON org.branch_institutional_works (institutional_work_id);`
   (077 created one but guarded behind a `DO $$` block — confirm it exists).

**Code fixes:**

5. `internal/modules/org/postgres/branches_repo.go:saveBranchInstitutionalWorksTx` —
   rewrite as a **replace**, not an append:
   - delete rows for the branch whose work id is not in the incoming set,
   - insert/update the incoming set,
   - **remove the `if len(works) == 0 { return }` guard** so clearing every work works.
   Change its signature to return `error` and have both call sites
   (`branches_repo.go:38` and `:81`) check it — today they discard it, which the
   `check-error-swallow` gate is meant to catch.
6. `internal/modules/org/institutional_connection.go:branchWorkIDs` — read
   `institutional_work_id` directly. Keep the slug fallback but resolve it through
   `org.institutional_works.slug` rather than `ParseInt`, and log at WARN when a row
   cannot be resolved (that is a data fault an operator must see).
7. Extract `ConnectedWorkIDsForBranch(ctx, branchID) ([]int64, error)` for A3 to reuse, and
   have `BranchesInstitutionallyConnected` call it. Today `branchWorkIDs` is called once per
   vendor branch inside a loop — an N+1. Replace the loop with one query:
   `SELECT DISTINCT branch_id FROM org.branch_institutional_works WHERE branch_id = ANY($vendorBranches) AND institutional_work_id = ANY($allowed) LIMIT 1`.

**Operator visibility (this is the part the client actually feels):**

8. On `/admin/institutional`, add a panel **"فروع بلا أعمال مؤسسية"** listing every branch
   with zero resolvable works, with its organization, type and a link to fix it. Right now
   four branches are in that state and nothing anywhere says so — their entire catalogue is
   invisible and unbuyable with no explanation.
9. On `/admin/branches/{id}` and on the vendor/pharmacy branch editors, show the branch's
   works and, for a vendor branch, **which buyer works it is reachable from**.

**Acceptance:**
- Un-ticking every institutional work on a branch and saving leaves zero rows in
  `org.branch_institutional_works` for that branch.
- Un-ticking one of three works removes exactly that one.
- `SELECT count(*) FROM org.branch_institutional_works WHERE institutional_work_id IS NULL`
  returns 0 after migration 197 (or the residue is listed in the new admin panel).
- Regression test in `internal/modules/org/postgres/` covering replace-semantics.

---

### A7 — Merge the quota into every surface (defect #34)

1. Smart ordering: done by A5.
2. `/vendor/quotas` (`internal/ui/vendor_quota_handlers.go`, routes in
   `vendor_catalog_routes.go`): audit end to end —
   - `VendorQuotasPage` must list, per (variant, buying branch): the cap, consumption
     summed from orders, remaining, percent used, and the last release instant;
   - filters: by variant, by buying organization, by branch, by "exhausted only";
   - `B2BPagination` with `QueryValues` carrying all filters;
   - `VendorQuotaLimitSubmit` writes `catalog.product_variants.quota_limit`;
   - `VendorQuotaReleaseSubmit` inserts into `commerce.variant_branch_quota_releases`;
     `…/undo` deletes the most recent release for that (variant, branch).
   - Every one of those must survive the redirect fix from B1 with filters intact.
3. Confirm `commerce/postgres/quota_enforce.go` sums only orders **after** the release
   instant and excludes `QuotaReleasingStatuses`. There is an existing test
   (`quota_report_test.go`); extend it with a release-then-buy-again case.

**Acceptance:** a supplier sets a cap of 3, a branch orders 3, the item shows
`quota_exhausted` on the catalogue and the supplier profile (visible, disabled, Arabic
reason), the smart order reports `OutcomeQuotaBlocked`, checkout refuses it, the supplier
releases the branch, and all four surfaces immediately allow 3 more.

---

### A8 — Coverage: one call signature, one meaning

1. Every caller of `CoverageService.ServesPoint` must pass `workflow.Coord{Lat, Lon, CityID}`.
   Audit: `cmd/server/availability.go` (correct today), `cmd/worker/smartorder.go`
   (missing `CityID` — fixed by A5), `cmd/server/smartorder.go` (`placeSmartOrder`'s
   delivery-fee calculation also omits it), `internal/ui/offers_coverage.go`.
2. Resolve §2.3 (`optWhen`) per decision Q1.
3. `internal/ui/offers_storefront.go:calculateHaversineKM` is a **second distance
   calculation** next to `platform.distance_meters` in SQL. It is used only for the
   "distance from you" label, which is legitimate — add a comment saying it is presentation
   only and must never gate an order, so the next reader does not use it as a coverage test.

---

## PART 3 — WORKSTREAM B: FOUR PLATFORM-WIDE FIXES

Each of these is one small change that closes several numbered defects at once. Do them
before PART 4.

### B1 — Preserve filters, search, page and scroll after every action (defect #1)

**File:** `internal/ui/request_helpers.go`.

Change `redirectWithNotice` itself rather than 724 call sites:

```go
// redirectWithNotice sends the caller on with a message to show when they land.
//
// When the target path is the page the caller is already on, the query string
// they arrived with is carried over. A list screen is a query — filters, a
// search term, a page number, a sort — and throwing that away after every
// action is why long sessions on /admin/users were unusable: approve one user
// and you are back at page 1 of an unfiltered list, scrolled to the top.
//
// Only same-path, same-origin referers are trusted, and only parameters the
// target does not already set are copied, so an explicit ?tab=x in the call
// still wins.
func (h *UIHandler) redirectWithNotice(w http.ResponseWriter, r *http.Request, path, kind, message string) {
    …
}
```

Rules:
- Parse `r.Referer()`. Ignore it unless its `Host` matches the request's host (or it is
  relative) and its `Path` equals the target's `Path`.
- Copy referer query parameters that the target does not already define.
- Never copy `notice`, `msg`, `notice_type`, `notice_msg`.
- Append the fragment `#` + a stable anchor if the target path already carries one.

**Scroll restoration** is separate. Add to `internal/ui/static/js/app.js`:

```js
// Restore the scroll position across a post/redirect/get on list screens.
// Keyed by pathname so two tabs on different screens do not fight.
```
Save `window.scrollY` to `sessionStorage` on `submit` of any form inside a
`[data-preserve-scroll]` container (or on every `form` in `/admin/*`), and restore it on
`DOMContentLoaded` when the URL carries `notice=`.

**Also:** for genuinely destructive actions where returning to the same filtered view would
be confusing (deleting the last row of a page), clamp `page` to the new last page.

**Acceptance:** on `/admin/users?q=ahmed&role=vendor&page=3&limit=50`, suspend a user →
land back on the same URL with the notice appended and the same scroll offset. Verify the
same on `/admin/organizations`, `/admin/products`, `/vendor/products`, `/admin/orders`,
`/admin/cities`, `/vendor/coverage`.

---

### B2 — Real client IP everywhere (defects #3 and #4)

`internal/platform/httpx/middleware.go:ClientIP(r, trustedHops)` already exists and is
correct: it counts X-Forwarded-For entries **from the right**, which is the only
non-forgeable end. `config.HTTP.TrustedProxyHops` is the hop count (1 for the current
Elest.io deployment, 0 when the process is exposed directly).

**Nothing in the UI layer uses it.** These call sites use `r.RemoteAddr`, which behind the
deployment's reverse proxy is the proxy — rendered as "localhost":

| File:line | What it records |
|---|---|
| `internal/ui/visitor.go:62` | `platform_admin.visitors.ip` → **defect #3** |
| `internal/ui/visitor.go:124` | private/loopback test that then defaults country to Egypt |
| `internal/ui/auth_handlers.go:116` | login session IP |
| `internal/ui/auth_handlers.go:267` | MFA completion IP |
| `internal/ui/promo_revenue_handlers.go:388,417` | ad impression/click IP |
| `internal/modules/identity/http/handlers.go:99` | session IP |
| `internal/modules/promo/http/handlers.go:160` | ad click IP |
| `internal/modules/promo/http/vendor.go:213` | ad impression IP |

**Do:**
1. Add `trustedProxyHops int` to `UIHandler` plus `func (h *UIHandler) SetTrustedProxyHops(n int)`
   in `internal/ui/handler_deps.go`, and call it from `cmd/server/routes_ui_builder.go`
   with `cfg.HTTP.TrustedProxyHops`.
2. Add `func (h *UIHandler) clientIP(r *http.Request) string { return httpx.ClientIP(r, h.trustedProxyHops) }`.
3. Replace every `r.RemoteAddr` above. For the module-level handlers, pass the hop count
   into the handler struct the same way.
4. `internal/platform/errtrack/errtrack.go:285-294` already reads X-Forwarded-For but takes
   the **first** entry — forgeable. Route it through `httpx.ClientIP` too.
5. `detectCountryAndCity` in `visitor.go` must test the resolved client IP, not `RemoteAddr`,
   or every visitor is classified as loopback and defaulted to Cairo.

**Defect #4 additionally needs a column.** `platform_admin.contact_messages` has
`(id, public_id, name, email, phone, subject, message, status, created_at)` — **no IP**.

Migration `198_contact_message_ip.up.sql`:
```sql
BEGIN;
ALTER TABLE platform_admin.contact_messages
  ADD COLUMN IF NOT EXISTS ip TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES identity.users(id) ON DELETE SET NULL;
COMMIT;
```
Then populate it in `ContactSubmit` (`internal/ui/public_handlers.go`) and render it in
`internal/ui/pages/admin_messages.templ` — IP, user agent, and the signed-in user when
there was one.

**Acceptance:** after deploying behind the proxy, `SELECT DISTINCT ip FROM platform_admin.visitors`
shows public addresses, and `/admin/messages` shows a sender IP per row.

---

### B3 — One way to open a modal (defect #8, foundation for #6, #12, #25, #26)

**File:** `internal/ui/static/js/app.js`, inside `initModalManager`.

Add a listener for the Alpine-style custom event so both idioms work:

```js
// Alpine's $dispatch('open-modal', 'some-id') bubbles a CustomEvent. Two admin
// screens open their review dialogs that way and nothing was listening, so the
// buttons did nothing at all. Supporting both spellings is cheaper than
// rewriting every template, and a modal that does not open is indistinguishable
// from a broken page.
document.addEventListener('open-modal', (e) => {
  const id = typeof e.detail === 'string' ? e.detail : (e.detail && e.detail.id);
  if (!id) return;
  const dialog = document.getElementById(String(id).trim());
  if (dialog && typeof dialog.showModal === 'function') dialog.showModal();
});
document.addEventListener('close-modal', (e) => { … dialog.close() … });
```

Then **standardise**: convert `internal/ui/pages/admin_offers.templ:318` and
`admin_ai_logs.templ` to `data-modal-open="…"` so the codebase has one idiom, and keep the
listener as a safety net.

**Acceptance:** the "مراجعة" button on `/admin/offers` opens the review dialog. Add a
browser smoke check to the verification protocol (§6.3).

---

### B4 — One modal component (defect #6)

Convert the hand-rolled Alpine overlays on the temp-warehouse screens
(`internal/ui/pages/admin_temp_warehouses_modals.templ` lines 12, 154, 341, and the upload
screen) from `<div class="fixed inset-0 z-modal flex-center bg-black/75 p-5">` to
`components.Modal`. They currently inherit none of the modal manager's behaviour (focus
trap, Escape, scroll lock, backdrop click) and are pushed to the bottom of the viewport on
narrow screens by `components.css:336-339` (`.fixed.inset-0 > * { margin-block: auto }`).

Keep the Alpine state (`modalItems`, `modalPage`, `modalLoading`) — only the shell changes.
Open them with `data-modal-open` (B3) or `dialog.showModal()` from the Alpine handler.

Then sweep for the same pattern elsewhere:

```bash
grep -rn 'class="fixed inset-0' internal/ui/pages/*.templ
```

Convert every hit. Watch the `check-modal-handwritten` ratchet — it counts raw `<dialog>`
in `internal/ui/pages/*.templ`; using `components.Modal` does not increase it.

**Acceptance:** every modal on `/admin/my/temparte-warehouses` and
`/admin/user/temparte-warehouses` is vertically centred at 1440×900 and at 390×844, closes
on Escape, and locks body scroll.

---

*(Work orders for defects #1–#42 continue in PART 4, below.)*

---

## PART 4 — WORK ORDERS, DEFECT BY DEFECT

Each work order states: the **symptom** as the client reported it, the **root cause**
(marked *verified* where it was traced in this audit, *to diagnose* where it was not), the
**files**, the **change**, and the **acceptance test**. Do not mark a work order done
without executing its acceptance test.

Cross-references to PART 2/3 mean: that workstream must land first, and it may already have
closed the defect.

---

### WO-01 — `/admin/users`: filters and scroll survive every action  *(Level 1, defect #1)*

**Symptom:** performing any action on a user clears the applied filter and returns the list
to the top, unfiltered, page 1. The client calls this the single most exhausting problem in
long sessions and says it exists platform-wide.

**Root cause — verified.** See §1.3 and B1. `redirectWithNotice` is called with a bare path
at 724 sites.

**Change:** implement **B1**. Then verify the ten highest-traffic list screens explicitly:
`/admin/users`, `/admin/organizations`, `/admin/orders`, `/admin/products`,
`/admin/product-child`, `/admin/cities`, `/admin/offers`, `/vendor/products`,
`/vendor/coverage`, `/customer/catalog`.

For each, also confirm the page's own `B2BPagination` call passes every active filter in
`QueryValues` — otherwise the filter survives the action but dies on page 2.

**Acceptance:** as in B1, on all ten screens.

---

### WO-02 — `/admin/organizations/change-requests`: make the page readable  *(Level 1, defect #2)*

**Symptom:** the supplier name is plain text, not a link. The page is "very plain". Tabs
carry no row counts. The reviewer cannot see the organization data while judging a change
request.

**Files:** `internal/ui/admin_org_changes_handlers.go`,
`internal/ui/pages/admin_org_changes.templ`, `internal/ui/pages/admin_org_changes_view.go`.

**Data available:** `org.profile_change_requests(id, public_id, organization_id,
requested_by, section, proposed, previous, status, admin_notes, reviewed_by, reviewed_at,
created_at, updated_at)`. `proposed` and `previous` are JSONB — the whole diff is already
stored.

**Change:**
1. Make the organization name a link to `/admin/organizations/{id}` (new tab). Also link the
   requesting user to `/admin/users/{id}`.
2. Add a count badge to every tab (`قيد المراجعة 4`, `مقبولة 12`, `مرفوضة 2`). Counts come
   from one grouped query, not from `len()` of the current page.
3. Render a **before → after diff table** per request from `previous` vs `proposed`, keyed
   by `section`, with field labels localised. A reviewer approving a trade-name change must
   see both names side by side.
4. Add filters: organization (searchable), section, status, date from/to. Wire them into
   `B2BPagination.QueryValues`.
5. Show `requested_by`, `reviewed_by`, `reviewed_at` and `admin_notes` on each row.

**Acceptance:** a change request for `section=trade_name` shows the old and new value; the
supplier name navigates to its admin page; tab badges match the grouped counts.

---

### WO-03 — `/admin/analytics`: show the visitor real IP  *(Level 1, defect #3)*

**Root cause — verified.** `internal/ui/visitor.go:62` records `truncateIP(r.RemoteAddr)`.
Behind the deployment reverse proxy that is the proxy address, which renders as localhost.

**Change:** implement **B2** (items 1-3 and 5). Additionally review `truncateIP`: if it
exists for privacy, keep it but make the truncation explicit in the UI; if it exists by
accident, remove it. The client asked to see the real address.

**Acceptance:** after B2, new rows in `platform_admin.visitors` carry public addresses and
`/admin/analytics` shows them. Historical rows stay as they are — do not rewrite them.

---

### WO-04 — `/admin/messages`: show the sender IP  *(Level 1, defect #4)*

**Root cause — verified.** `platform_admin.contact_messages` has no `ip` column at all:
`(id, public_id, name, email, phone, subject, message, status, created_at)`.

**Change:** migration `198_contact_message_ip` + handler + template, per **B2**.

**Acceptance:** submitting `/contact` while signed out records a public IP and a user agent;
`/admin/messages` renders both, plus the signed-in user when there was one.

---

### WO-05 — `/admin/trash-list`: say what is being deleted  *(Level 1, defect #5)*

**Symptom:** deleted items are listed without enough detail to know what you are restoring
or purging before confirming.

**Files:** `internal/ui/admin_trash_handlers.go`, `internal/ui/pages/admin_trash.templ`,
`internal/ui/admin_trash_test.go`.

**Change:**
1. For every model the trash supports, render a **human identity line**: the Arabic name,
   the SKU / code / order number, the owning organization, who deleted it, and when.
2. Add the row count per model on the index page, plus a per-model filter and search.
3. Purge must open a `components.ConfirmModal` naming the exact item and stating that the
   purge is irreversible; restore may stay a direct action.
4. Show *why* an item cannot be restored when a dependency is missing (its parent
   organization is itself deleted, say) rather than failing after the click.

**Acceptance:** every row on `/admin/trash-list/{model}` identifies its item without the
reviewer needing to open another screen.

---

### WO-06 — Temp-warehouse modals centred, uploads too  *(Level 1, defect #6)*

**Root cause — verified.** §1.5 and B4.

**Change:** implement **B4** for `/admin/my/temparte-warehouses`,
`/admin/user/temparte-warehouses`, `/admin/team/temparte-warehouses` and the upload screen.

**Acceptance:** as in B4.

---

### WO-07 — Supplier directory lists only orderable products  *(Level 1, defect #7)*

**Covered by A4.** Do not implement separately.

**Acceptance:** as in A4. Additionally, in the client's own terms: a product that cannot be
ordered because of coverage or institutional works does **not** appear on `/suppliers/{id}`;
a product that is merely out of stock or over quota appears, disabled, with its Arabic
reason.

---

### WO-08 — `/admin/offers`: the review button opens nothing  *(Level 1, defect #8)*

**Root cause — verified.** `internal/ui/pages/admin_offers.templ:318` uses
`@click="$dispatch('open-modal', 'admin-offer-review-modal-N')"`. Nothing in
`internal/ui/static/js/app.js` listens for that event; the manager only handles
`data-modal-open` / `data-dialog-target` / `data-open-modal`. The modal
(`AdminOfferReviewModal`, same file, line 378) is rendered correctly and never opened.

**Change:** implement **B3**, then convert this template to `data-modal-open`.

**Acceptance:** clicking مراجعة on `/admin/offers` opens a dialog containing the offer
banner, metadata, products and the approve / reject / request-changes forms. Escape closes
it. Repeat for `/admin/ai-logs`.

---

### WO-09 — `/vendor/products`: adding a product returns "بيانات الطلب غير صالحة"  *(Level 2, defect #9)*

**Root cause — verified, and it is a one-field bug.**
`internal/ui/pages/vendor_products_modals.templ:258` — the modal `add-custom-variant-modal`
posts to `/vendor/variants/new` with `name_ar`, `sku`, `price`, `discount`, `stock_qty`,
`cost_price`, … and **no `product_id`**.

`VendorVariantNewSubmit` (`internal/ui/vendor_variant_handlers.go:207`) reads `product_id`
as `0`, builds the variant, and calls `catSvc.CreateVariant`.
`internal/modules/catalog/variants_and_saving_service.go:20`:

```go
if v.ProductID <= 0 {
    return nil, apperr.Validation("variant.product_required", "Parent product ID is required.", nil)
}
```

`h.safeMessage` renders a validation `apperr` as the generic **`بيانات الطلب غير صالحة.`** —
exactly the reported string.

**Change — the whole page must be correct, not only this field:**
1. Add a **master-product selector** to the modal. A working one already exists at
   `internal/ui/pages/vendor_product_editor.templ:41` (`<select name="product_id" required>`),
   with a JSON search endpoint `GET /vendor/catalog/search-json` (`VendorCatalogSearchJSON`)
   and `GET /vendor/catalog/product-json/{id}`. Use `components.Combobox` against the search
   endpoint so a supplier can type an Arabic name.
2. If the client genuinely wants suppliers to add products the master catalogue lacks, that
   is a **different feature** (a product-creation request an admin approves). Do not silently
   create master products from a supplier form. Raise it as question **Q3** (§7); until it is
   answered, require selection from the catalogue.
3. Audit **every field in the modal against the handler and the DB column**: `name_ar` /
   `name_en`, `sku`, `barcode`, `price`, `cost_price`, `cost_discount_percentage` (the
   handler also accepts a legacy `cost_discount` — pick one and make the form use it),
   `discount`, `stock_qty`, `min_order_qty`, `quota_limit`, `branch_id`, `expiry_date`,
   `batch_number`, `is_negotiable`. Any field on the form the handler ignores must be removed
   or wired.
4. `branch_id` must be a **required** select of the supplier own branches. Today the handler
   silently falls back to the main branch. That silence is what puts stock on a branch with
   no institutional works and makes it invisible (§2.5).
5. Return validation errors **to the form**, not as a generic redirect banner: re-render the
   modal with the submitted values and a per-field message. A supplier who typed twenty
   fields must not lose them.
6. `stock_qty` is written to `inventory.stocks` by `h.recordInitialStock`, not to the variant
   (there is no stock column). Confirm the warehouse it picks belongs to the chosen branch.

**Acceptance:** adding a product from `/vendor/products` succeeds; the new variant appears in
`/vendor/products`, in `/vendor/inventory` with the entered quantity, and on
`/customer/catalog` for a buyer whose branch is institutionally connected to the chosen
branch. A submission missing a required field re-renders the form with the error and the
typed values intact.

---

### WO-10 — `/customer/catalog` pagination  *(Level 2, defect #10)*

**Covered by A3.** Do not implement separately.

The client is unsure whether this is still broken. It is — see §2.4 for the full trace. The
"one product" case is reproducible against the live data (851 stocked products out of
19,996 in the master catalogue).

---

### WO-11 — `/vendor/offers/{id}/locations`: governorate → city → map  *(Level 2, defect #11)*

**Symptom:** the offer-locations screen does not use the platform standard
governorate-then-city selector, the city choice does not move the map, and the page is
generally not usable.

**Files:** `internal/ui/vendor_offer_locations_handlers.go`, its page template under
`internal/ui/pages/`, `internal/ui/auth_geography.go`,
`internal/ui/components/map_picker.templ`, `internal/ui/components/maps.go`.

**The canonical pattern already exists** — reuse it rather than writing a fourth copy. It is
used by registration (`/auth/register`), branch creation (`/customer/branches/create`,
`/vendor/branches/new`) and coverage (`/vendor/coverage`):

- `GET /api/geo/governorates/{id}/cities` → `APIGovernorateCitiesJSON` (registered in
  `internal/ui/vendor_routes.go`, also mounted at `/vendor/coverage/governorates/{id}/cities`).
- `platform_admin.cities` carries `latitude`, `longitude`, `coverage_radius_meters`;
  `platform_admin.governorates` carries its own coordinates and radius (migration 192).
- `components.MapPicker` is the Leaflet wrapper (vendored — no CDN, per rule 9).

**Change:**
1. Replace the current controls with governorate select → city select populated from the
   JSON endpoint.
2. On city change, pan and zoom the map to the city coordinates and draw the coverage radius
   circle; on governorate change with no city yet, fit the governorate.
3. Keep a draggable marker that writes lat/lng into hidden inputs, so a supplier can pin a
   point the city centroid does not describe.
4. Persist and re-render the chosen governorate / city / coordinates when editing an existing
   location.
5. Redesign the list of existing locations as a `data-table` with delete, showing each
   location governorate, city, radius and coordinates.
6. `AdminOfferLocationsPage` (`/admin/offers/{id}/locations`) shares the template — confirm it
   still renders.

**Acceptance:** picking a governorate then a city recentres the map on that city; saving
stores the coordinates; reopening the page shows the same selection and map position. Compare
side by side with `/vendor/branches/new` — the two must behave identically.

---

### WO-12 — `/admin/saving-products`: linking opens a modal, not a wrong page  *(Level 2, defect #12)*

**Symptom:** linking an unlinked product, or editing a linked one, navigates to the wrong
page. It should open a selection modal like every other screen.

**Files:** `internal/ui/saving_products_handlers.go`,
`internal/ui/saving_products_match_choice.go`,
`internal/ui/pages/admin_saving_products.templ`.
Routes: `/admin/saving-products`, `/admin/saving-products/user/{userId}`,
`/admin/saving-products/org/{organizationId}` in `admin_routes_catalog.go`.

**Reference implementation to copy:** the vendor and pharmacy saving-products screens already
do this correctly — `GET /vendor/saving-products/search-products`
(`VendorSavingProductSearchJSON`) feeds a combobox inside a modal, and
`POST /vendor/saving-products/{id}/update` applies the choice.
`internal/ui/pages/vendor_saving.templ:539` shows the hidden `product_id` pattern.

**Change:**
1. Add `GET /admin/saving-products/search-products` (admin-scoped, cross-tenant via
   `AsSystem`) and `POST /admin/saving-products/{id}/link` returning to the same filtered URL
   (B1).
2. Replace the navigation with `components.Modal` + `components.Combobox`.
3. Inside the modal show the raw imported name, the current link if any, and the top
   candidates with their match scores, so the operator is choosing rather than guessing.
4. When a link is made, also write the decision to the shared memory — see WO-13; it is the
   same operator doing the same job.

**Acceptance:** clicking ربط on `/admin/saving-products?page=2&q=xyz` opens a modal, linking
succeeds, and the page returns to `?page=2&q=xyz` with the row updated.

---

### WO-13 — Decision memory: make it a real tool  *(Level 2, defect #13 — large)*

**Routes:** `/admin/match-decisions`, `/vendor/decision-memory`, `/customer/decision-memory`.
**Files:** `internal/ui/decision_memory_handlers.go` (340 lines),
`internal/modules/catalog/postgres/match_decisions.go`,
`internal/modules/ingest/postgres/catalog_ai.go`,
`internal/shared/matchflow/{memory.go,cachekey.go,ceilings.go}`,
`cmd/server/match_memory.go`,
`internal/ui/pages/{admin_match_decisions.templ,customer_decision_memory.templ}`.

**How it works today — read this before changing anything:**

- One table, `catalog.match_decisions(id, decision_key, norm_name, chosen_product_id,
  confidence, reason, prompt_version, hit_count, created_at, last_used_at, organization_id,
  user_id)`. 933 rows live. Unique on `(COALESCE(organization_id,0), decision_key)`.
- `matchflow.DecisionKey(normName, candidateIDs)` is the SHA-256 of
  `normName ␟ sorted-deduped-ids ␟ PromptVersion`. The text, the shortlist **and** the prompt
  version are all in the key: the same name against a different shortlist is a different
  question.
- `matchflow.Memory{Lookup, Save, SaveAlias}` is the port; `cmd/server/match_memory.go` adapts
  the ingest repository to it; four tools use it (vendor import, admin master-catalogue
  import, saving-products import, smart order).
- `MinMemoryConfidence = 0.45` — below that an answer is used but not remembered.
- A global kill switch lives in `platform_admin.system_settings`, key
  `decision_memory_enabled`.

**Verified defect: the "global" memory is never readable by an organization.**
`Repository.LookupDecisions` (`internal/modules/ingest/postgres/catalog_ai.go:70-122`)
branches:

```go
if orgID > 0 {  ... WHERE organization_id = $1 AND decision_key = ANY($2) ...
} else {        ... WHERE organization_id IS NULL AND decision_key = ANY($1) ... }
```

A signed-in organization reads **only its own** rows and never the platform-wide ones; there
is no way to promote a row to platform-wide; and there is no per-organization opt-out. All
three of the client asks for this screen are genuinely missing.

**Change — schema (`199_decision_memory_scope.up.sql`):**

```sql
BEGIN;
-- 'org'      : written by and for one organisation (today's only behaviour)
-- 'platform' : promoted by an administrator; readable by every organisation
--              that has not opted out.
ALTER TABLE catalog.match_decisions
  ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'org'
    CHECK (scope IN ('org','platform')),
  ADD COLUMN IF NOT EXISTS promoted_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS promoted_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'ai'
    CHECK (source IN ('ai','manual','admin','import'));

UPDATE catalog.match_decisions SET scope = 'platform' WHERE organization_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_match_decisions_scope_key
  ON catalog.match_decisions (scope, decision_key);
CREATE INDEX IF NOT EXISTS idx_match_decisions_org_lastused
  ON catalog.match_decisions (organization_id, last_used_at DESC);

-- Per-organisation opt-out of the platform-wide memory.
CREATE TABLE IF NOT EXISTS catalog.decision_memory_preferences (
  organization_id     BIGINT PRIMARY KEY REFERENCES org.organizations(id) ON DELETE CASCADE,
  use_platform_memory BOOLEAN NOT NULL DEFAULT true,
  updated_by          BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMIT;
```

**Change — lookup semantics.** `LookupDecisions` reads the organization own rows **and**,
unless the organization has opted out, the `scope='platform'` rows. **The organization own
answer always wins** on a key collision — a company that corrected a match must not be
overruled by the platform. Return which scope each answer came from so the UI can label it.

Keep the `hit_count` / `last_used_at` bump. If a platform row being bumped once per
organization makes the counter meaningless to the operator, add a separate
`platform_hit_count`.

**Change — the three screens.**

*`/admin/match-decisions`* (the operator tool; the client says a dedicated employee will run
it):
- Filters: free text over `norm_name` / product name / SKU / reason; organization; user;
  scope; source; confidence range; `chosen_product_id IS NULL` ("none of these"); date ranges
  on `created_at` and `last_used_at`; minimum `hit_count`; prompt version.
- Columns not shown today but needed: organization name, user, confidence as a bar, hit
  count, source, scope badge, prompt version, and **the raw name beside the chosen product
  Arabic name and SKU**.
- Row actions: **re-link** (the same combobox modal as WO-12; overwrite `chosen_product_id`,
  set `source='admin'`, `confidence=1.0`), **promote to platform**, **demote to org**,
  **delete**.
- Bulk actions across the current filter: promote, delete.
- **Link an unlinked product**: rows with `chosen_product_id IS NULL` are the residue the
  model refused; the operator must be able to resolve them here.
- Export the current filter to XLSX.
- `B2BPagination` with every filter in `QueryValues`.

*`/vendor/decision-memory` and `/customer/decision-memory`*:
- The same table scoped to the organization, plus the rows inherited from the platform scope,
  labelled and read-only.
- A prominent **"استخدام ذاكرة المنصة العامة"** toggle writing
  `catalog.decision_memory_preferences`. This is exactly what the client asked for.
- The same re-link ability for the organization own rows.

**Change — prove the memory is actually consulted everywhere.** Confirm by test, not by
reading, that all four tools hit the cache: `internal/modules/catalog/import_match_memory.go`
(admin master import), `internal/ui/saving_products_ai.go:239` (`applySavingMemory`),
`internal/modules/ingest/` (vendor import), and the smart-order enhance stage
(`internal/modules/smartorder/pipeline/enhance*.go`). For each, a test with a pre-seeded
decision must show zero Gateway calls for that row.

**Acceptance:**
- An answer recorded by organization A and promoted by an admin is used by organization B.
- Organization B toggles the platform memory off; the same import re-asks.
- Organization B own decision for the same key overrides the platform one.
- The admin filters compose (org + scope + date + unlinked) and survive paging.

---

### WO-14 — `/admin/cities`: disabling a city must not hide it  *(Level 1, defect #14)*

**Root cause — verified, one line.** `internal/ui/admin_geography_handlers.go:27` calls
`h.adminSvc.ListCities(database.AsSystem(ctx), 1)`. `ListCities`
(`internal/modules/platform_admin/postgres/repository.go:233`) ends with
`AND c.is_active = true`. `ListAllCities` (same file, line 264) is the identical query
without that predicate and already exists.

**Change:**
1. Use `ListAllCities` in `AdminCitiesPage`.
2. Add a status filter (الكل / مفعّلة / معطّلة), defaulting to الكل, carried in `QueryValues`.
3. Render a disabled city with a muted row and a معطّلة badge, and the toggle labelled تفعيل.
4. Audit the governorate screen for the same bug (`ListAllGovernorates` appears to be used
   already — confirm).
5. Confirm every *consumer* of city lists (registration, branches, coverage, offer locations)
   keeps using `ListCities`, active only. Those must **not** switch to `ListAllCities`.

**Acceptance:** disabling a city leaves it visible on `/admin/cities` with a معطّلة badge and
a working تفعيل button; the city disappears from the registration city dropdown.

---

### WO-15 — Subscription change cooldown and confirmation  *(Level 2, defect #15)*

**Routes:** `/customer/subscription`, `/vendor/subscription`.
**Files:** `internal/ui/account_handlers.go:90-190` (`TenantSubscriptionCheckoutSubmit`),
`internal/modules/billing/`, the subscription templates under `internal/ui/pages/`.

**Root cause — verified:** there is no cooldown of any kind.
`TenantSubscriptionCheckoutSubmit` computes `isUpgrade`, calls `billSvc.SubscribeWithWallet`
and redirects.

**⚠ The requirement as written is internally contradictory** — it says 25 days in one
sentence and 2 days in the next. See question **Q2** in §7. Until the client answers,
implement it as **configurable**, defaulting to the stricter reading:

```
platform_admin.system_settings
  subscription_change_cooldown_days   default 25   (paid plan -> any change)
  subscription_change_min_days        default 2    (minimum wait after any purchase)
```

**Rules:**
1. If the current subscription is on a **free / default plan** (`billing.plans.is_default`
   true, or `price_month = 0`), **no cooldown applies** — upgrading from free is always
   allowed. The client stated this explicitly.
2. Otherwise a change is refused until `now() >= current_subscription.starts_at + cooldown`.
3. Enforce it in `billing.Service.SubscribeWithWallet`, **not only in the handler** — the
   handler is one caller, the rule is a billing invariant. Return
   `apperr.Conflict("subscription.change_cooldown", …)` carrying the earliest allowed date.
4. In the UI, while the cooldown is active the plan-card buttons are disabled and the modals
   **do not open**, with an inline explanation naming the date.
5. Add a **second, smaller confirmation modal** after plan selection, restating the plan, the
   cycle, the amount to be deducted from the wallet, and that the change is final for the
   cooldown period. It contains nothing else — the client was explicit: "مش أكتر ولا أقل".

**Schema note — an existing invariant of this codebase:** one live `billing.subscriptions`
row per org/user, and the current plan is resolved by **`starts_at DESC`, not by
`expires_at`**. Honour it when reading the current subscription; do not introduce a second
live row.

**Acceptance:** an org on a paid plan that subscribed today cannot change plan; the button is
disabled and names the date; a direct `POST /customer/subscription/checkout` is refused with
the conflict error. An org on the free plan can upgrade immediately.

---

### WO-16 — Pharmacy orders: a real "offer details" action  *(Level 2, defect #16)*

**Routes:** `/customer/orders`, `/orders`, `/customer/orders/{id}` — handlers
`CustomerOrdersPage`, `CustomerOrderDetailPage` in `internal/ui/customer_order_handlers.go`.
The detail endpoint already exists: `GET /customer/orders/{id}/lines/{lineID}/offer-details`
→ `CustomerOrderLineOfferDetails`.

**Symptom:** when a line carries a promotional offer, the only way in is a link on the
product name. There is a description line under the name that adds noise.

**Change:**
1. Remove the description line under the product name.
2. Keep the product name itself clickable (it goes to the product).
3. Add a clearly labelled button beside the name — **"عرض تفاصيل العرض"** — at a normal
   button size (`btn btn-secondary btn-sm`, not `btn-xs`), shown **only** when the line has
   an offer. It opens `components.Modal` populated over HTMX from
   `/customer/orders/{id}/lines/{lineID}/offer-details` (use `ModalProps.State: "loading"`
   for the skeleton while it fetches).
4. The modal must show: the offer title, the offer type and mechanics, the list price, the
   discount applied to this line, the resulting unit price, the quantity, the line total,
   and the offer validity window.
5. Do the same on the vendor side (`/vendor/orders`) so both parties see the same evidence.

**Acceptance:** an order line with an offer shows the button; a line without one does not.
The modal opens, loads, and its numbers reconcile exactly with the line totals on the order.

---

### WO-17 — `/admin/employee-activities`: date range, export, pagination, real coverage  *(Level 2, defect #17)*

**Files:** `internal/ui/admin_user_directory_handlers.go:223-294`
(`AdminEmployeeActivitiesPage`), `internal/modules/platform_admin/domain.go:241`
(`AuditLogFilter`), `internal/modules/platform_admin/postgres/content.go:225`
(`ListAuditLogWithFilter`), `internal/platform/database/audit.go` (`WriteAudit`).

**Verified findings:**
- The page reads `platform.audit_log` (via `AuditLogFilter`), **not**
  `platform_admin.employee_activities`. That second table has columns
  `(organization_id, user_id, action, description, href, ip, data, created_at)` and
  **zero Go references** — it is dead. See §5.2.
- `perPage` is hardcoded to `pagination.TableRows`; there is no rows-per-page control and
  no `B2BPagination` render.
- There is no date filter and no export.
- **Only seven code paths write an audit row** (`billing/postgres/admin_payments_repo.go`,
  three in `identity/postgres/admin_repo.go`, `identity/postgres/moderator_repo.go`,
  `identity/postgres/register_org.go`, `platform/pagecontrol/store_writes.go`), so the
  screen is nearly empty by construction.

**Change:**
1. Add `DateFrom` / `DateTo` to `AuditLogFilter` and to the SQL; add the two date inputs.
2. Add `B2BPagination` with `RowsPerPage(r)` and every filter in `QueryValues`.
3. Add **Export to Excel** of the current filter (not the current page). There is an
   existing XLSX pattern — `github.com/xuri/excelize/v2` is already a dependency and
   `internal/ui/admin_temp_warehouse_handlers.go` has an export handler to copy.
4. **Localise the action labels.** Add an `i18n` key per action string
   (`user.suspend`, `org.approve`, `payment.adjust`, …) and render the label, keeping the
   raw action visible in a monospace badge for the operator.
5. **Extend coverage — but bounded.** The client explicitly warned against over-building
   ("متعملش over burning"). Add `WriteAudit` calls for exactly this set, each inside the
   transaction that performs the change:

   | Area | Actions |
   |---|---|
   | Organizations | approve, reject, suspend, reactivate, change-request approve/reject, deletion approve/reject |
   | Users | create staff, role assign, suspend, reactivate, reset MFA, password set, deletion approve/reject |
   | Catalogue | product create/edit/delete, variant create/edit/delete/toggle, bulk activate-all, bulk delete-all |
   | Commerce | order status change, refund, wallet adjust, deposit approve/reject, withdrawal approve/reject |
   | Promo | offer approve/reject, sponsorship approve/reject, ad approve/reject |
   | Reference data | city/governorate create/edit/toggle, institutional work create/edit/delete, plan create/edit/toggle |

   Nothing else. Do not audit reads, page views or navigation.
6. Record the actor IP on the audit row using `h.clientIP(r)` from **B2**
   (add an `ip` column in the same migration as B2, or reuse `request_id`).

**Acceptance:** approving an organization writes one audit row visible on
`/admin/employee-activities` with a localised label, the actor, the organization, the before
and after JSON, and the real IP; the date filter narrows it; Export produces an XLSX of the
whole filtered set.

---

### WO-18 — `/admin/plans?tab=subscriptions`: usable subscriber log  *(Level 2, defect #18)*

**Files:** `internal/ui/admin_finance_plans_handlers.go`,
`internal/ui/pages/admin_plans.templ`.
**Tables:** `billing.subscriptions(id, public_id, user_id, organization_id, plan_id, status,
starts_at, expires_at, source_system, source_id, created_at, updated_at, billing_cycle,
auto_renew, last_renewed_at, renewal_attempts)` and
`billing.subscription_histories(id, subscription_id, organization_id, user_id, plan_id,
action, amount_minor, currency, details, created_at)`.

**Change:**
1. Filters that matter: **organization name** (searchable — the client asked for this
   specifically), plan, status, billing cycle, auto-renew, `starts_at` from/to,
   `expires_at` from/to, "expiring within 30 days".
2. Render the organization as `اسم المنشأة` + type badge + link to
   `/admin/organizations/{id}`, not a bare id.
3. Columns: organization, plan, cycle, amount paid, status, start, expiry, days remaining,
   auto-renew, source.
4. `B2BPagination` with all filters in `QueryValues`.
5. Add a per-subscription detail drawer showing `billing.subscription_histories` for it —
   the upgrade/downgrade/renewal trail.
6. Honour the invariant from WO-15: current plan is `starts_at DESC`, one live row.

**Acceptance:** filtering by an organization name returns that organization's subscriptions
across pages, and the detail drawer shows its history.

---

### WO-19 — `/admin/product-child`: filters, pagination, branch and warehouse detail  *(Level 2, defect #19)*

**Root cause — verified.** `internal/ui/admin_catalog_handlers.go:61-67` calls
`ListAllVariants` with `Limit: 100, Offset: 0` — **hardcoded, no pagination at all**. It
also fetches organization names with `ListOrganizations(…, 500, 0)` to build a lookup map,
which is a second unbounded read.

**Change:**
1. Real pagination: `PageNumber(r)` / `RowsPerPage(r)` → `Limit` / `Offset`, plus
   `B2BPagination` with `TotalCount` from the repository.
2. Filters: supplier organization, **branch**, warehouse, status, stock state
   (in / out / low), expiring within 90 days, free-text over name / SKU / barcode / batch.
3. **Richer rows — this is the client's actual complaint.** Each row must show, per variant:
   the supplier, the **branch it sits on**, and for each warehouse holding it, the warehouse
   name and the quantity. Extend the repository query with the same
   `stockRollup` lateral used in `catalog/postgres/vendor_variants.go`, plus a
   per-warehouse breakdown (a `jsonb_agg` of `(warehouse name, quantity)` from
   `inventory.stocks` joined to `inventory.warehouses`) rendered as an expandable cell.
4. Replace the org-name map with a join in SQL.

**Acceptance:** `/admin/product-child?page=3&limit=50&branch_id=76` returns page 3 of that
branch's variants, each showing its warehouses and quantities, and the pager total matches
a `SELECT count(*)` with the same filters.

---

### WO-20 — `/admin/warehouses`: pagination, vendor filter, admin edit/toggle  *(Level 2, defect #20)*

**Files:** `internal/ui/admin_warehouse_handlers.go` (251 lines),
`internal/ui/pages/admin_warehouses.templ`, `internal/modules/inventory/warehouses.go`,
`internal/modules/inventory/postgres/`.
Existing routes: `GET /admin/warehouses`, `GET /admin/warehouses/{id}`,
`GET /admin/warehouses/{id}/stocks-json`.

**Change:**
1. Pagination + `B2BPagination`.
2. Filters: vendor organization (searchable), branch, active/inactive, warehouse type, and
   free-text name/code.
3. New admin routes in `admin_routes_catalog.go`:
   - `POST /admin/warehouses/{id}/toggle` — activate / deactivate
   - `POST /admin/warehouses/{id}/edit` — edit the warehouse's data
   - `POST /admin/warehouses/new` — create one on behalf of an organization
4. All three open as **`components.Modal` forms inside the admin shell**, matching the
   vendor's own warehouse modals in `internal/ui/pages/vendor_warehouse_detail_modals.templ`
   — same fields, same validation, same layout. Reuse the vendor service methods
   (`VendorWarehouseCreateSubmit` / `…UpdateSubmit` / `…ToggleSubmit` in
   `internal/ui/vendor_warehouse_handlers.go`) via the inventory service, running under
   `database.AsSystem` with the target organization, never under the admin's own tenant.
5. Every write records an audit row (WO-17).
6. Show, per warehouse: organization, branch, item count, total quantity, and last movement
   date.

**Acceptance:** an admin can create, edit, activate and deactivate any organization's
warehouse from `/admin/warehouses`; the filters and pager compose; the vendor sees the
change immediately on `/vendor/warehouses`.

---

### WO-21 — `/admin/orders`: the buyer filter is mislabelled and mis-scoped  *(Level 2, defect #21)*

**Symptom:** the filter is called "الصيدلية" (the pharmacy). Suppliers buy too, so the filter
excludes or mislabels half the orders.

**Files:** `internal/ui/admin_commerce_handlers.go` (`AdminOrdersPage`),
`internal/ui/pages/admin_orders.templ`, and the orders repository under
`internal/modules/commerce/postgres/orders.go`.

**Change:**
1. Rename the filter and every label to **المشتري / Buyer**. The underlying column is the
   buying organization, whatever its type.
2. The buyer picker must list **every** organization that has placed an order — pharmacies
   and suppliers alike — with a type badge on each entry.
3. Add a separate **البائع / Seller** filter, and a **نوع المشتري** filter
   (pharmacy / vendor / company).
4. **Sweep the whole codebase for the same mislabelling**, which is the client's explicit
   instruction. Start with:
   ```bash
   grep -rn "الصيدلية" internal/ui/pages/*.templ internal/shared/i18n/*.go | grep -i "filter\|buyer\|customer"
   grep -rn "pharmacy" internal/ui/*.go | grep -i "filter"
   ```
   Anywhere the word names *the buying party* rather than *a pharmacy specifically*, change
   it. Where it genuinely means a pharmacy (an audience filter, a role name), leave it.
5. Institutional/organization filters on this page must be driven by the buyer's actual
   organization type, not assumed.

**Acceptance:** an order placed by a supplier from another supplier appears on
`/admin/orders`, is findable through the المشتري filter, and its buyer is labelled with the
correct type.

---

### WO-22 — `/vendor/offers`: per-tab filters and pagination  *(Level 2, defect #22)*

**Files:** `internal/ui/vendor_offer_handlers.go`, `internal/ui/vendor_sponsorship_handlers.go`,
`internal/ui/vendor_ads_handlers.go`, `internal/ui/pages/` offers templates.
Routes: `/vendor/offers`, `/vendor/offers-packages`,
`/vendor/offers-packages/sponsorships`, `/vendor/offers-packages/promotions`.

**Change:** each tab (باقات / رعايات / عروض) gets its **own** filter set and its **own**
pagination, namespaced so switching tabs does not carry the wrong filters:

- use tab-prefixed query parameters (`offers_status`, `pkg_status`, `spons_status`, …) and
  keep the active `tab` in `QueryValues`;
- per tab: status, date from/to, free-text on title, and for offers also the branch and the
  discount type;
- `B2BPagination` per tab with its own `page`/`limit` parameter names.

The client's calibration is explicit: organise it like the catalogue screen, **but not as
heavy** ("بس مش قوي"). Do not build faceted counts or saved views here.

**Acceptance:** each tab filters and pages independently; switching tabs preserves the other
tab's state on return.

---

### WO-23 — `/admin/adv-products`: organise product sponsorship  *(Level 2, defect #23)*

**Files:** `internal/ui/admin_adv_products_handlers.go` (369 lines),
`internal/ui/pages/admin_adv_products.templ`, `internal/ui/admin_adv_products_test.go`.
Routes: `GET /admin/adv-products`, `POST /admin/adv-products/{id}/approve`,
`POST /admin/adv-products/{id}/reject`, `POST /admin/adv-products/new`.
**Tables:** `promo.sponsorship_requests`, `promo.sponsorship_purchases`,
`promo.offer_sponsorships`, `promo.sponsorship_credit_entries`, `promo.offer_packages`.

**Change:**
1. General filters: organization, product, package tier, status, admin status, date from/to.
2. A proper `data-table` listing: product (image + Arabic name + SKU), the sponsoring
   organization, the package and tier, credits consumed vs total, the active window, status,
   impressions and clicks.
3. **A detail page per sponsorship** — `GET /admin/adv-products/{id}` — showing the request,
   the purchase it draws credits from, the credit entries
   (`promo.sponsorship_credit_entries`), the impression and click history
   (`promo.ad_impressions`, `promo.offer_clicks`), and the moderation trail.
4. **Bind the page to real data.** Verify each figure against the database with the probe
   before trusting the template; the client's complaint is that the page is not connected to
   its data. Any number that cannot be sourced must be removed rather than shown as zero.
5. `B2BPagination` with filters in `QueryValues`.

**Acceptance:** every number on the list and the detail page reconciles with a SQL query you
can paste into the probe.

---

### WO-24 — `/admin/chat/history`: useful filters  *(Level 2, defect #24)*

**Files:** `internal/ui/admin_chat_handlers.go` (76 lines — it is thin),
`internal/ui/pages/admin_chat_history.templ`, `internal/modules/chat/`.
**Tables:** `chat.conversations`, `chat.messages`, `chat.participants`.

**Change:** add filters for
1. user name (participant),
2. organization name,
3. date from / to,
4. **flagged / unusual conversations**.

For (4), define "flagged" concretely rather than hand-waving — implement a stored
`is_flagged BOOLEAN` plus `flag_reason TEXT` on `chat.conversations`
(migration `200_chat_flags.up.sql`), set by a deterministic rule at message-write time:
- a message containing contact details that route trade off-platform (phone numbers,
  WhatsApp links, external URLs),
- a message over a length ceiling,
- a conversation where one party sends more than N messages with no reply,
- an explicit report from a participant.

Keep the rule list in one Go file with a named constant per rule so an operator can be told
*why* a conversation is flagged, and show that reason in the table. Do not use AI for this —
rule 3 requires a deterministic path, and this must work with the Gateway off.

Add `B2BPagination` and keep filters in `QueryValues`.

**Acceptance:** each filter works alone and in combination; a flagged conversation shows its
reason; paging preserves the filters.

---

### WO-25 — `/admin/approvals?tab=organizations`: the registration modal is empty  *(Level 2, defect #25)*

**Files:** `internal/ui/admin_approvals_handlers.go` (373 lines),
`internal/ui/pages/admin_approvals.templ`, `internal/ui/pages/admin_approvals_models.go`.

**Symptom:** the review modal shows nothing at all.

**To diagnose first (do this before writing code):** the pattern used by the sibling screens
is a `<script type="application/json" id="…-data-{id}">` block per row, parsed by an Alpine
handler on open (see `internal/ui/pages/admin_developers.templ`, function
`adminAuditManager`). An empty modal is almost always one of:
1. the JSON block is not rendered (the view model is not populated by the handler),
2. the modal is not opened at all (the B3 event problem — check this first),
3. the JSON fails to parse and the fallback stub renders.

Open the page, check the browser console for the "JSON parse fallback" warning, and check
whether the `<script id=…>` element exists in the DOM.

**Change — regardless of which it is, the modal must show the complete registration:**
organization legal name, trade name (ar/en), type, commercial register, tax number,
licence numbers and expiry, address, governorate/city, coordinates, phone/email/website,
the requesting user (name, email, phone, national id), the requested branches and their
institutional works, **every uploaded document with an inline preview link**
(`/admin/documents/{id}/view`), the plan chosen at signup, and the full status trail.

Everything above already exists in `org.organizations`, `org.branches`,
`org.branch_institutional_works`, `platform_admin.documents`, `identity.users` — this is a
rendering gap, not a data gap. Verify each field with the probe as you add it.

**Acceptance:** an administrator can approve or reject without opening any other screen.

---

### WO-26 — `/admin/developers?tab=errors`: the diagnostics modal is empty  *(Level 2, defect #26)*

**Root cause — verified.** `internal/ui/pages/admin_developers.templ` defines
`adminErrorsManager().openErrorDetails(id)`, which does:

```js
const script = document.getElementById('err-data-' + id);
if (!script) return;                       // silent no-op
try { this.selectedError = JSON.parse(script.textContent.trim()); }
catch (e) { this.selectedError = { id: id, error_message: 'تفاصيل الخطأ المسجل #' + id, error_level: 'ERROR' }; }
```

So a missing or unparseable JSON block yields either nothing or a two-field stub — which is
exactly the reported empty modal. The same pattern and the same failure mode exist in
`adminAuditManager`.

`platform_admin.error_logs` already stores everything worth showing:
`(id, user_id, user_name, user_email, organization_name, error_level, error_message,
exception_class, stack_trace, file_path, line_number, http_method, url_path, ip_address,
user_agent, request_payload, status, created_at)`.

**Change:**
1. Stop embedding a JSON blob per row. Add `GET /admin/developers/errors/{id}/details`
   returning the rendered modal body, and load it over HTMX into
   `components.Modal{State: "loading"}`. A 30-row page then carries no payload, and a parse
   failure becomes a visible server error instead of a silent stub.
2. The modal must render **all** of the above: level badge, message, exception class,
   **the full stack trace in a scrollable `<pre>`**, file and line, the HTTP method and
   path, the request payload pretty-printed, the user and organization (linked), the IP and
   user agent, the timestamp, and the status with its transition buttons
   (`POST /admin/developers/errors/{id}/status`).
3. Delete the silent `catch` fallback. If details cannot be loaded, say so.
4. Apply the identical treatment to the audit modal (`adminAuditManager`, `dev-audit-modal`)
   — it has the same bug.
5. Add filters to the errors tab: level, status, date from/to, URL path contains,
   organization, and free text over the message; plus `B2BPagination`.

**Acceptance:** clicking a row opens a modal containing the complete stack trace and request
payload for that error id, matching
`SELECT * FROM platform_admin.error_logs WHERE id = <id>`.

---

### WO-27 — One deletion-requests screen for organizations, users and text  *(Level 2, defect #27)*

**Current state — verified:**
- `org.organization_deletion_requests(id, public_id, organization_id, requested_by, reason,
  status, admin_notes, reviewed_by, reviewed_at, created_at, updated_at)` —
  screen `/admin/organizations/deletion-requests`, handler
  `internal/ui/admin_org_deletion_handlers.go` (125 lines), plus its own sidebar entry.
- `identity.account_deletion_requests(id, user_id, status, reason, admin_response,
  reviewed_by, requested_at, resolved_at, organization_id, admin_notes, created_at,
  updated_at)` — actions exist (`POST /admin/users/deletion/{id}/approve|reject`) but there
  is **no screen listing them**.

**Change:**
1. Build **`/admin/deletion-requests`** as one page with tabs — المنشآت / المستخدمين — each
   with its own filters (status, organization, requester, date from/to), its own
   `B2BPagination`, and row counts on the tabs.
2. Redirect `/admin/organizations/deletion-requests` to it (301) and replace the sidebar
   entry (`internal/platform/rbac/nav_admin.go`).
3. Use the platform's standard table and modal vocabulary. **No emoji** — the client said so
   and `check-emoji` enforces a ceiling of 8 across all templates.
4. **Deletion is a block, never a delete — this is the most important requirement here.**
   Approving a request must make the account behave as if it does not exist while its rows
   remain in the database:
   - the organization/user is marked with a terminal status (add
     `status = 'deleted'` handling; do **not** add a second column that means the same as
     `deleted_at` — see §5.1);
   - it disappears from every listing, search, filter and dropdown across admin, vendor,
     pharmacy and public surfaces;
   - its products and offers disappear from `/catalog`, `/suppliers`, smart ordering and
     the compare tool;
   - its users cannot sign in and existing sessions are revoked;
   - `CheckAvailability` refuses it (the `vendor_unapproved` path already does this once the
     status changes — verify);
   - nothing is physically removed.
5. Write the change through **one** service method so no surface can forget a step, and
   record an audit row.
6. Rejecting a request notifies the requester (WO-35).

**Acceptance:** approve a deletion for a test supplier; then confirm by walking
`/catalog`, `/suppliers`, `/admin/organizations`, `/admin/orders`, `/customer/smart-order/new`
and the compare tool that it is absent everywhere, while
`SELECT count(*) FROM org.organizations WHERE id = <id>` still returns 1.

---

### WO-28 — `/admin/finance`: reorganise, verify the numbers, add statements  *(Level 2, defect #28)*

**Files:** `internal/ui/admin_finance_handlers.go` (377 lines),
`internal/ui/admin_finance_withdrawals_handlers.go`, `internal/ui/pages/admin_finance.templ`,
`internal/ui/pages/admin_finance_subpages.templ`, `internal/ui/admin_finance_test.go`.

**Change:**
1. **Tab order, exactly as the client specified:**
   1. محافظ المنشآت
   2. سجل حركات المحافظ
   3. …everything else that remains.
2. **Delete the "تقرير الأرباح والعمولات" tab entirely** — the client says remove it, not
   hide it. Remove its handler, its template, its route and its sidebar entry, and check
   nothing else links to it (`grep -rn "profit\|commission" internal/ui/pages/admin_finance*.templ`).
3. De-duplicate: several tabs currently show the same figures under different names. List
   every tab, name what question it answers, and remove the ones that answer a question
   another tab already answers.
4. Reduce the prose. The client's words: "خليها أهدأ من ناحية كثرة الكلام". Headings and
   numbers, not paragraphs.
5. **Verify every displayed figure against the database.** For each stat card write the SQL
   into a code comment next to the handler that computes it, and check it with the probe.
   Tables involved: `billing.wallets` (12 rows), `billing.wallet_transactions` (16),
   `billing.wallet_deposits` (5), `billing.wallet_withdrawals` (2), `billing.invoices`,
   `billing.invoice_lines` (653), `billing.payments`. Any card whose number cannot be
   derived must be removed.
6. **New: "طباعة كشف الحساب" on the wallet-movements tab.** It prints the *currently
   filtered* table, taking the organization name from the active organization filter.
   Reuse the existing invoice print stack — `internal/ui/invoice_handlers.go`,
   `internal/ui/static/css/invoice_printable.css`, and `internal/ui/pages/credit_statement.templ`
   (`CreditStatementPage`) which already prints a statement for sponsorship purchases. The
   statement must carry the Dawa24 logo, the organization's logo, the period, the opening
   and closing balance, every movement with its date, type, reference, debit, credit and
   running balance, and a total row. No emoji, print-safe CSS, correct RTL.
   If no organization filter is set, disable the button rather than printing a mixed
   statement.

**Acceptance:** the tab order matches; the profit report is gone from the codebase; every
stat card has a comment containing the SQL that produces it and the two agree; printing a
statement for one organization produces a clean single-purpose PDF via the browser's print
dialog.

---

### WO-29 — `/admin/jobs`: full control over sector vacancies  *(Level 2, defect #29)*

**Files:** `internal/ui/jobs_handlers.go`, `internal/ui/job_form_handlers.go`,
`internal/ui/pages/admin_jobs.templ`, `internal/modules/hr/`.
**Tables:** `hr.job_offers`, `hr.job_applications`, `hr.job_categories`,
`hr.job_seeker_profiles`, `hr.work_times`.
Today only `GET /admin/jobs` exists. The vendor side already has the full set
(`POST /vendor/jobs`, `/vendor/jobs/{id}/edit`, `/toggle`, `/delete`,
`/applications/{appId}/accept|reject`) in `internal/ui/vendor_job_handlers.go` — copy it.

**Change:** add admin routes in `admin_routes_platform.go`:
- `GET  /admin/jobs/{id}` — detail with applications
- `POST /admin/jobs/new`, `POST /admin/jobs/{id}/edit` — modal forms
- `POST /admin/jobs/{id}/toggle` — publish / unpublish
- `POST /admin/jobs/{id}/delete`
- `POST /admin/jobs/{id}/applications/{appId}/accept|reject`

Plus filters (organization, category, status, governorate/city, date from/to), search,
`B2BPagination`, and an audit row per write (WO-17). All writes run under
`database.AsSystem` scoped to the owning organization.

**Acceptance:** an admin can create, edit, publish, unpublish and delete any vacancy, and act
on its applications, without impersonating the vendor.

---

> **Note on numbering:** the client's list has no item 30. The numbering below follows the
> client's own labels so cross-referencing their message stays exact.

---

### WO-31 — Editable default roles  *(Level 3, defect #31)*

**Root cause — verified.** Organization starter roles are **hardcoded Go**.
`internal/platform/rbac/roles.go` declares `SystemRole` values;
`OrganizationRolesFor(scope)` returns them; `provision.go:EnsureCompanyRoles` writes them
into a new company's `org.roles` (and grants once, at creation, so an owner's later edits
survive). `PlatformRoles()` does the same for `identity.roles`. An administrator cannot
change what a new pharmacy or supplier starts with.

Live shape: `identity.roles` 16 rows, `identity.role_permissions` 317; `org.roles` 88 rows
across 7 organizations, `org.role_permissions` 2,153; `identity.permissions` 296 keys.

**Change:**

1. **Migration `201_role_templates.up.sql`** — make the templates data:

```sql
BEGIN;
CREATE TABLE IF NOT EXISTS platform_admin.role_templates (
  id          BIGSERIAL PRIMARY KEY,
  key         TEXT NOT NULL,
  scope       TEXT NOT NULL CHECK (scope IN ('admin','vendor','pharmacy')),
  name        JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
  description TEXT NOT NULL DEFAULT '',
  is_owner    BOOLEAN NOT NULL DEFAULT false,
  is_staff    BOOLEAN NOT NULL DEFAULT false,
  is_builtin  BOOLEAN NOT NULL DEFAULT true,   -- shipped by the platform; cannot be deleted
  is_active   BOOLEAN NOT NULL DEFAULT true,   -- an inactive template is not seeded
  sort_order  INT NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_role_template UNIQUE (scope, key)
);

CREATE TABLE IF NOT EXISTS platform_admin.role_template_permissions (
  role_template_id BIGINT NOT NULL REFERENCES platform_admin.role_templates(id) ON DELETE CASCADE,
  permission_key   TEXT   NOT NULL REFERENCES identity.permissions(key) ON DELETE CASCADE,
  PRIMARY KEY (role_template_id, permission_key)
);
COMMIT;
```

2. **Seed from the Go definitions**, once, in a `cmd/cli` sub-command
   (`go run ./cmd/cli seed-role-templates`) rather than in the migration, so the seed can
   re-run after a permission catalogue change without a new migration. Idempotent upsert;
   never overwrite an operator's edit to an existing template's grants.

3. **`EnsureCompanyRoles` reads the table**, falling back to the Go defaults when the table
   is empty (so a fresh database still provisions). An `is_active = false` template is not
   seeded.

4. **`/admin/roles` gains a second surface.** Today it edits platform roles only
   (`AdminRolesPage`, `AdminRoleDetailPage`, `AdminRoleCreateSubmit`, `AdminRoleUpdateSubmit`,
   `AdminRoleDeleteSubmit` in `internal/ui/admin_roles_handlers.go`). Add tabs:
   - **أدوار المنصة** — today's screen.
   - **قوالب أدوار المنشآت** — the new templates, per scope (vendor / pharmacy), with the
     full 296-key permission picker grouped by module, `is_owner`, `is_active`, sort order.
   - **مزامنة** — an action that pushes a template change into existing companies. This must
     be **opt-in and explicit**, and must state clearly that it will overwrite those
     companies' edits to that role. Offer "apply to companies that have not customised this
     role" as the default and "apply to all" as a confirmed destructive action.
5. The admin role system must be **wider than the tenant one**, as the client asked:
   platform roles keep the existing screen plus permission search, a diff view against
   `super_admin`, per-role user counts, and the ability to clone a role.
6. Audit every write (WO-17).

**Acceptance:** an administrator edits the vendor `warehouse_keeper` template, registers a
new supplier, and that supplier's seeded role has the edited grants; an existing supplier is
untouched until the sync action is run.

---

### WO-32 — `/admin/plans`: more than four cards without breaking the layout  *(Level 3, defect #32)*

**Files:** `internal/ui/admin_finance_plans_handlers.go`,
`internal/ui/pages/admin_plans.templ`.

**Symptom:** the design breaks at the fifth or sixth plan card.

**To diagnose:** the card row is almost certainly a fixed-column grid or a flex row with
`flex-wrap: nowrap`. Check `internal/ui/static/css/` for the grid used by this page.

**Change:** make it a responsive auto-fill grid that wraps:
```css
grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
```
inside the existing `@layer components`, using the four canonical breakpoint tokens only
(`check-breakpoints` fails on any other pixel value) and logical properties
(`check-physical-properties`). Verify at 1440px, 1024px, 768px and 390px with 3, 6 and 9
plans. Add a horizontal-scroll fallback only if wrapping genuinely reads badly.

**Acceptance:** nine plans render legibly at every breakpoint with no clipped card and no
horizontal page scroll.

---

### WO-33 — `/admin/users`: full user administration  *(Level 3, defect #33)*

**Files:** `internal/ui/admin_users_handlers.go`, `internal/ui/admin_user_directory_handlers.go`,
`internal/ui/pages/admin_users.templ`, `internal/ui/pages/admin_full_user.templ`,
`internal/modules/identity/`.
Existing: `GET /admin/users`, `/admin/users/{id}`, `/{id}/info`, `/{id}/edit`;
`POST /{id}/suspend`, `/{id}/reactivate`, `/{id}/reset-mfa`, `/{id}/role`, `/users/staff`,
`/{id}/moderator-parent`. There is **no edit submit route**.

**Change:**
1. `POST /admin/users/{id}/edit` — a modal form editing name (ar/en), email, phone, avatar,
   national id, job title, status, organization membership and branch.
2. `POST /admin/users/{id}/password` — **set a new password**. Requirements:
   - the same strength validation the user-facing flow uses (find it in
     `internal/modules/identity/` and reuse it — do not write a second policy);
   - hash with the same helper (`golang.org/x/crypto` bcrypt/argon wrapper already in the
     identity module);
   - **revoke every active session** for that user afterwards;
   - notify the user (WO-35);
   - write an audit row that records *that* the password was changed and by whom, and
     **never the password itself, not even hashed**, in `before`/`after`.
3. Show the user's **name and avatar** wherever they appear — the list, the detail page and
   the modal. `identity.users` carries the name as JSONB and there is an avatar path; if the
   avatar is missing, render initials, never a broken image.
4. Make roles and permissions explicit on the detail page: the platform role, the
   organization role(s) per membership, and the **effective permission list** resolved by
   `rbac.Resolver` — grouped by module, searchable. An administrator must be able to answer
   "why can this person see that page" without reading code.
5. Widen the list: filters for role, status, organization, type, MFA enabled, last login
   from/to, and free text over name/email/phone; `B2BPagination`.
6. Every write is audited (WO-17) and preserves filters (B1).

**Acceptance:** an administrator can set a new password for a user, that user's sessions are
gone, they can sign in with the new password, and the audit row names the administrator but
contains no credential material.

---

### WO-34 — `/vendor/quotas`: full review  *(Level 3, defect #34)*

**Covered by A7.** Do not implement separately. Read §2.6 and §A7 first — the quota domain
is already well designed and the work is wiring, screens and the smart-order gap.

---

### WO-35 — Notifications for every event  *(Level 3, defect #35)*

**Files:** `internal/ui/notifications_dispatch.go` (530 lines — it will need splitting to
stay under 400), `internal/ui/notifications_handlers.go`,
`internal/ui/notifications_withdrawals.go`, `internal/modules/notifications/`.
**Tables:** `notifications.logs(id, public_id, user_id, organization_id, channel, recipient,
title, body, status, error_message, is_read, read_at, sent_at, created_at,
required_permission)` — 306 rows; `notifications.templates` — **0 rows**.

**What exists:** two primitives —
`dispatchInAppNotification(ctx, userID, orgID, requiredPerm, title, body)` and
`dispatchOrgNotification(ctx, orgID, requiredPerm, title, body)` — and 22 typed wrappers:
order placed / status changed, purchase request created / responded, wallet deposit /
rejected, account registered, admins-new-registration, org approved / rejected, document
verified, special-offer status, sponsorship status, ad status, negotiation offer / decision,
quote request / provided / decision, subscription updated.

**Missing — build these, each as a typed wrapper beside the existing ones:**

| Event | Recipient |
|---|---|
| Trade-name (and any profile) change **requested** | platform admins |
| Trade-name / profile change **approved** | the requesting organization |
| Trade-name / profile change **rejected**, with the reason | the requesting organization |
| Organization deletion **requested** | platform admins |
| Organization deletion **approved / rejected** | the organization owner |
| Account deletion **requested / approved / rejected** | admins / the user |
| Organization **suspended / reactivated** | the organization |
| Document **requested by admin** | the organization |
| Document **rejected** | the organization |
| Branch **created / disabled** | the organization owner |
| Institutional works **changed on a branch** | the organization owner |
| User **added to / removed from** an organization | both the user and the owner |
| Role **changed** for a user | that user |
| Password **set by an administrator** | that user |
| Subscription **expiring in 7 / 3 / 1 days**, and **expired** | the organization |
| Wallet **withdrawal approved / rejected** | the organization |
| **Refund issued** (WO-37) | the organization |
| Quota **exhausted** for a branch | the buying organization |
| Quota **released** by a supplier | the buying branch's organization |
| Order **cancelled by the buyer** | the supplier |
| Smart-order run **finished / failed** | the user who started it |
| Import run **finished / failed** | the user who started it |
| New **review** on an organization | that organization |
| New **chat message** while offline | the recipient |
| Job application **received** | the posting organization |

**Structural work — do this, not just the list:**
1. **A registry, not 40 loose functions.** Declare an `Event` type with a stable key, a
   default channel set, a `required_permission`, and bilingual title/body templates rendered
   from a typed payload. Move the existing 22 onto it. `notifications.templates` exists and
   is empty — either populate it and render from the database (so an operator can edit
   wording), or drop the table (§5.2). Decide; do not leave it empty and unused.
2. **Delivery must not depend on the request finishing.** Several call sites already use
   `go h.notify…(context.Background(), …)`, which loses the request's tenant and cannot be
   retried. Move dispatch onto the River queue (`internal/platform/queue`) with a
   `notifications.deliver` job, so a failed send is retried and visible.
   ⚠ **River trap recorded from earlier work on this codebase:** a queue that is not
   configured in the River client's `Queues` map silently swallows its jobs. Add the new
   queue to the worker's configuration and assert it in a test.
3. **Respect `required_permission`.** `dispatchOrgNotification` already fans out to the
   members holding a permission — keep that, and give every new event a sensible key so a
   warehouse keeper is not paged about billing.
4. **Every event must reach the account**, which is the client's actual requirement. Add an
   integration test that walks the table above and asserts a `notifications.logs` row per
   event.

**Acceptance:** perform each event in the table above against a test organization and find
exactly one `notifications.logs` row for it, addressed to the right recipients, in Arabic,
visible in the notification bell.

---

### WO-36 — `/admin/organizations/import`: use the real import tool  *(Level 3, defect #36)*

**Files:** `internal/ui/admin_org_import_handlers.go`,
`internal/ui/admin_org_import_saving_session_handlers.go`,
`internal/ui/pages/admin_org_import.templ`.
Existing routes: `GET /admin/organizations/import`,
`GET /admin/organizations/import/saving/{id}`,
`POST …/saving/upload|{id}/map|{id}/commit|{id}/cancel`,
`POST /admin/organizations/import/temp-warehouse/upload`.

**Requirement:** when an administrator imports **on behalf of** an organization, the tool must
be **byte-for-byte the same tool** the organization itself uses — same wizard, same column
mapping, same matching, same progress, same limits — differing only in which organization
the run is attributed to.

**Change:**
1. Identify the canonical implementations and reuse them rather than re-implementing:
   - products/saving-list import: `internal/ui/saving_products_import_*.go` +
     `internal/modules/catalog/import_*.go`;
   - vendor catalogue ingest: `internal/ui/vendor_ingest_*.go` + `internal/modules/ingest/`;
   - temp-warehouse (compare) upload: `internal/ui/compare_upload_*.go` +
     `internal/modules/compare/`.
2. Extract the "acting organization" from a URL parameter instead of from the session, guard
   it with an admin permission, and run every write under `database.AsSystem` scoped to that
   organization. Nothing else changes.
3. **Multi-file upload, ~80 files at once**, for the temp-warehouse path, with the same
   staging behaviour the organization-side tool has (`compare.files` + `CompareStagingStatus`
   at `/compare/files/staging`). Note the recorded operational history of this area: an
   earlier version held ~500 MB of heap in `ParseMultipartForm` and every slow upload was
   killed by the socket read deadline. Stream to storage, do not buffer, and keep the
   per-request deadline generous on the upload route specifically.
4. **The setup experience is a page, not a modal** — a real URL per step
   (`/admin/organizations/import/{orgID}/runs/{runID}/mapping`, `…/review`, `…/commit`), so
   a refresh or an accidental close loses nothing. Same as WO-40; build the mechanism once
   and use it in both places.

**Acceptance:** importing 80 files for organization X from the admin screen produces exactly
the same rows, mappings and matches as the organization doing it itself, and every step
survives a browser refresh.

---

### WO-37 — Refunds from the admin panel  *(Level 3, defect #37)*

**Route:** `/admin/finance?tab=deposits`.
**Files:** `internal/ui/admin_finance_handlers.go`,
`internal/ui/admin_finance_withdrawals_handlers.go`, `internal/modules/billing/`.
**Tables:** `billing.wallets`, `billing.wallet_transactions`, `billing.wallet_deposits`,
`billing.wallet_withdrawals`, `billing.payments`, `billing.invoices`.

**Change:**
1. **Refund any transaction a pharmacy or supplier paid from its organization wallet.**
   New route `POST /admin/finance/transactions/{id}/refund` with a reason. It must:
   - write a **compensating** `billing.wallet_transactions` row (type `refund`) that credits
     the organization's wallet — never mutate or delete the original row;
   - link the two rows (add `reverses_transaction_id BIGINT REFERENCES
     billing.wallet_transactions(id)` in migration `202_wallet_refunds.up.sql`);
   - be **idempotent**: a unique partial index on `reverses_transaction_id` so one
     transaction can be refunded once;
   - recompute `balance_after` correctly (the column exists — do not let it drift);
   - run inside one `db.InTx` with the audit row (WO-17).
2. **A refund button on wallet top-up requests** (`/admin/finance?tab=deposits`) that returns
   the money to the same organization's wallet, using the same compensating-entry mechanism.
3. **Notify the organization** that money was returned, naming the original transaction
   (WO-35).
4. Show, on both the deposits tab and the wallet-movements tab, whether a row has been
   refunded and by whom.

**Money rules:** every amount is `money.Amount`; never `float64`; assert exact values in
tests.

**Acceptance:** refunding a 250.00 EGP deposit credits the organization wallet by exactly
250.00, leaves the original row intact and marked, cannot be repeated, notifies the
organization, and appears on the printed statement from WO-28.

---

### WO-38 — Smart ordering bound to coverage and the branch  *(Level 3, defect #38)*

**Covered by A5** (with A1 and A8). Do not implement separately.

Additionally, on `/customer/smart-order/new`:
- the delivery-branch selector must be **required and explicit** — the whole run is
  evaluated against it;
- `smartOrderBranchRefusal` (`internal/ui/smart_order_branch_gate.go`) already blocks a
  branch that is not owned, has no location or has no institutional works. Keep that
  pre-flight and show its reason inline next to the selector rather than as a redirect
  banner, so the buyer fixes it before uploading a file;
- the results screen must show, per line, **which branch and which supplier branch** the
  selected offer comes from, and the refusal reason where there is one — including the new
  quota reason from A5.

---

### WO-39 — Per-tool AI model configuration  *(Level 3, defect #39)*

**Root cause — verified.** `internal/platform/gateway/roles.go` maps `Role → model` from
`defaultRoleModels`, overridable **only by environment variable**
(`GATEWAY_MODEL_MATCHING`, `GATEWAY_MODEL_COLUMNS`, `GATEWAY_MODEL_EXPAND`,
`GATEWAY_MODEL_CLASSIFY`, `GATEWAY_MODEL_ASSISTANT_*`). `gateway.Settings` (the DB-backed
`SettingsSource`) carries only `FastModel` and `QualityModel`. So an operator cannot repoint
a tool when a model is withdrawn, and **all four import tools share one role**
(`matching.adjudicate`).

**Change:**

1. **Split the roles** so each tool the client named is separately configurable. Add to
   `roles.go`:
   ```go
   RoleMatchVendorImport   Role = "matching.vendor_import"     // /vendor/ingest
   RoleMatchSavingProducts Role = "matching.saving_products"   // vendor + pharmacy saving lists
   RoleMatchSmartOrder     Role = "matching.smart_order"       // /customer/smart-order
   RoleMatchAdminCatalog   Role = "matching.admin_catalog"     // /admin/products/import
   ```
   Each defaults to today's `matching.adjudicate` model, so behaviour is unchanged until an
   operator changes something. Keep `RoleMatching` as the fallback when a specific role is
   unset.

2. **Make role→model data.** Migration `203_ai_role_models.up.sql`:
   ```sql
   BEGIN;
   CREATE TABLE IF NOT EXISTS platform_admin.ai_role_models (
     role        TEXT PRIMARY KEY,
     model       TEXT NOT NULL,
     is_active   BOOLEAN NOT NULL DEFAULT true,
     max_tokens  INT,
     notes       TEXT NOT NULL DEFAULT '',
     updated_by  BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
     updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   COMMIT;
   ```
   Extend `gateway.Settings` with `RoleModels map[string]string` and have the composition
   root's `SettingsSource` populate it. Resolution order becomes:
   **DB row → environment variable → `defaultRoleModels`.** Keep the existing settings cache
   TTL so a change takes effect without a restart but does not cost a query per call.
   `resolveRoleModel` becomes a method on the client (it needs the settings); the package
   boundary is unchanged and `check-provider-isolation` still passes because no model name
   leaves this package.

3. **One admin surface.** `/admin/developers?tab=ai` becomes the single place for AI
   configuration, restructured and simplified:
   - **الاتصال**: endpoint, admin credential, virtual key, active toggle, test button
     (the existing `POST /admin/developers/ai/test`).
   - **النماذج**: a row per role — the Arabic name of the *tool* ("مطابقة استيراد المورد",
     "الطلب الذكي بالذكاء الاصطناعي", "المساعد الذكي", "كشف أعمدة الملفات", …), the current
     model, a **dropdown populated live** from `POST /admin/developers/ai/fetch-models`
     (`AdminAIFetchModelsAPI` already exists), the model's advertised capabilities
     (`gateway.ModelCapabilities` — vision, thinking, context window), a max-tokens override,
     and an active toggle.
   - **الحدود**: the ceilings from `internal/shared/matchflow/ceilings.go` per profile,
     read-only for now with a clear statement of what each one costs.
   - **الاستهلاك**: link to `/admin/ai-logs` and `ai.usage_events`.
   The client asked for this to be *simpler*, not bigger. One table, one row per tool.

4. **Any model must work.** Nothing outside `gateway` may branch on a model name. Confirm
   with `make check-provider-isolation` (or the grep it runs). A model that does not support
   a JSON response format must degrade to the deterministic fallback rather than failing the
   run (rule 3).

5. **Every capability keeps its deterministic fallback**, and there must be a test per role
   proving the tool still completes with the Gateway disabled.

**Acceptance:** changing the model for "مطابقة استيراد المورد" in the admin panel changes
which model `/vendor/ingest` calls on the next run, without a restart and without touching
any other tool; setting it to a nonexistent model degrades that tool to its deterministic
path with a visible warning rather than breaking the import.

---

### WO-40 — Resumable warehouse-upload sessions, on a page  *(Level 3, defect #40)*

**Routes:** `/admin/my/temparte-warehouses`, `/admin/user/temparte-warehouses`
(plus `/admin/team/…`, `/admin/admins/…`, `/admin/plan/…` which share handlers).
**Files:** `internal/ui/admin_temp_warehouse_*.go` (handlers, mapping, staging, upload),
`internal/ui/admin_my_temp_warehouse_handlers.go`,
`internal/ui/admin_team_temp_warehouse_handlers.go`,
`internal/ui/pages/admin_temp_warehouses*.templ`, `internal/modules/compare/`.

**The client's requirement, in their words:** whenever the employee re-enters the page they
find the upload session still running and can continue it without redoing the column setup;
the whole setup experience lives **in a page, not a modal**; a refresh or an accidental close
must lose nothing.

**What you already have to build on:**
- `compare.files` carries `status`, `mapping_config JSONB`, `row_count`, `error_message`,
  `is_temp_warehouse`, `visibility` — the mapping is already persisted per file.
- `platform.import_runs` is the unified durable run table:
  `(kind, audience, filename, state, phase, percent, total_rows, processed_rows, payload
  JSONB, result JSONB, error_message, river_job_id, started_at, finished_at)` — 12 rows
  live, with `platform.import_run_rows` (8,540 rows) beneath it.
- `internal/platform/progress` + a Redis bridge + SSE already stream live progress; some
  tools are on it and some still poll.
- `CompareStagingStatus` (`/compare/files/staging`, and the `…/temparte-warehouses/staging`
  aliases) already reports in-flight staging.

**Change:**
1. **Model the session explicitly** on `platform.import_runs` — one run per upload batch,
   `phase` ∈ `{uploaded, mapping, staging, review, committing, done, failed}`, `payload`
   holding the file ids and the in-progress mapping.
2. **Give every phase a URL**:
   `/admin/my/temparte-warehouses/runs/{runID}` → redirects to the current phase;
   `…/runs/{runID}/mapping`, `…/runs/{runID}/review`, `…/runs/{runID}/progress`.
   No phase lives only inside a modal.
3. **On entering the index page, surface any unfinished run for this user/organization** as a
   prominent "استكمال جلسة الرفع" card with its filename count, phase and percent, linking
   straight back into it.
4. **Autosave the mapping** as it is edited (debounced POST to
   `…/runs/{runID}/mapping` writing `compare.files.mapping_config` and the run `payload`), so
   closing the tab mid-setup loses nothing.
5. **Put this screen on the SSE progress hub** rather than polling, and keep polling as the
   fallback when the hub is unavailable — that is the existing pattern, follow it.
6. **Multi-file**: accept ~80 files in one batch (see WO-36 item 3 for the streaming and
   deadline constraints).
7. Recorded operational traps for this area, honour all three:
   - a **batch quota race** existed on upload — check the quota once per batch, transactionally;
   - the **request deadline** must be extended on the upload route or slow uploads are killed;
   - **River queues that are not configured swallow jobs silently**, and Postgres
     `statement_timeout` outranks a job's own `JobTimeout`.

**Acceptance:** start a 20-file upload, map three columns, close the browser, reopen
`/admin/my/temparte-warehouses` → the session card is there, the mapping is intact, and the
run continues to completion.

---

### WO-41 — SEO, keywords and AI-discoverability tab  *(Level 3, defect #41)*

**Route:** a new tab on `/admin/developers`.
**Existing assets:** `internal/ui/sitemap_handler.go` (`GET /sitemap.xml`),
`internal/ui/public_handlers.go` (`GET /llms.txt`, `GET /.well-known/agents-index.json`),
`internal/ui/static/robots.txt`, `internal/ui/robots_test.go`,
`internal/ui/scrape_guard.go` + `internal/platform/antiscrape` (the guard on `/catalog` and
`/suppliers`), `internal/ui/site_context.go`, `platform_admin.managed_pages` (611 rows) and
`internal/platform/pagecontrol`.

**⚠ Do not undo the anti-scraping policy.** This platform deliberately welcomes search
engines and AI assistants and refuses training crawlers. `antiscrape.Classify(r)` drives
that, and `/catalog` and `/suppliers` are metered. Any SEO work must keep the guard intact;
"more indexable" must not become "freely scrapable".

**Change:**
1. **A `platform_admin.seo_pages` table** (migration `204_seo.up.sql`) keyed by route
   pattern, holding: title (ar/en), meta description (ar/en), canonical URL, robots
   directives, Open Graph title/description/image, Twitter card, JSON-LD payload, and a
   keyword list. Seed it from `platform_admin.managed_pages` so the 611 known routes appear.
2. **A tab at `/admin/developers?tab=seo`** to edit those rows, with a live preview of the
   Google result and the social card, and per-page validation (title length, description
   length, missing canonical, duplicate titles across routes).
3. **Render it.** The base layout (`internal/ui/layouts/`) must read the SEO row for the
   current route and emit `<title>`, `<meta name="description">`, canonical, hreflang
   (ar/en), Open Graph, Twitter and JSON-LD. Today most of this is either absent or
   hardcoded — audit `internal/ui/layouts/*.templ` first.
4. **Structured data** where it earns its place: `Organization` on the home page,
   `BreadcrumbList` on nested pages, `Product` + `Offer` on public product pages **only for
   data that is already public** — do not emit prices the guard is protecting.
5. **`sitemap.xml`**: make it dynamic and segmented (pages, suppliers, categories, public
   products), with `lastmod`, and a sitemap index if it exceeds 50,000 URLs.
6. **`robots.txt`**: make it editable from the same tab, generated from the table, with the
   existing crawler policy as the seeded default. `internal/ui/robots_test.go` asserts the
   current policy — keep that test passing or update it deliberately.
7. **`llms.txt` and `/.well-known/agents-index.json`**: generate them from the same table so
   the AI-assistant surface stays in step with the human one.
8. Add checks to the admin tab for the things that actually move rankings here: Arabic
   `lang`/`dir` correctness, canonical consistency, image `alt` coverage, heading order, and
   Core Web Vitals via the existing `.lighthouserc.json`.

**Acceptance:** editing a page's title and description in the admin tab changes the rendered
`<title>` and meta description on that route; `sitemap.xml` validates; `robots.txt` still
refuses training crawlers and still admits Googlebot and assistant user agents;
`go test ./internal/ui/ -run Robots` passes.

---

### WO-42 — Make the AI Assistant genuinely capable  *(Level 3, defect #42 — largest)*

**Files:** `internal/modules/assistant/` — `agent.go`, `context.go`, `prompt.go`,
`service.go`, `service_entities.go`, `service_memory.go`, `entities.go`, `access.go`,
`handles/`, `tools/`, `stream/`, `http/`, `postgres/`.
**Tables:** `assistant.conversations` (13), `assistant.turns` (16), `assistant.messages` (34),
`assistant.attachments`, `assistant.organization_memories`, `assistant.tool_audit`.

**Read `internal/modules/assistant/tools/registry.go` before anything else.** Its header
documents a ten-step security sequence that every tool call passes through: the caller comes
from the authenticated session and never from the model's arguments; the assistant gate
permission is re-checked; the tool is looked up by exact name; its dashboard scope must
match; the caller must hold the tool's permission; arguments are decoded strictly; handle
arguments are verified and bound (this defeats id enumeration and forgery); the call runs
under a timeout; results are truncated to row and byte ceilings; and the decision is written
to `assistant.tool_audit` whether allowed or denied.

**That design is correct and must not be weakened.** "Read the whole platform" means *more
tools inside this frame*, never a general SQL escape hatch and never trusting an id the
model produced.

**Current tool inventory — 21 tools:**
`ai_usage_summary`, `branches_list`, `low_stock`, `market_search`, `memory_forget`,
`memory_list`, `memory_remember`, `my_offers`, `my_products`, `order_details`, `orders_list`,
`organizations_search`, `platform_overview`, `sales_summary`, `spend_summary`,
`subscription_status`, `supply_order_details`, `supply_orders_list`,
`top_purchased_products`, `top_sold_products`, `wallet_summary`.

**Plan — five stages, in this order:**

**Stage 1 — measure before building.** Write an eval set of 60–100 real questions per
dashboard scope (pharmacy, vendor, admin) in Arabic, with the expected tool call and the
expected answer shape. Run it against the current assistant and record the pass rate. This
is the number every later stage is judged against. Put it in
`internal/modules/assistant/evals/` with a `go test` runner that skips when the Gateway is
off.

**Stage 2 — fix the plumbing before adding tools.** The client says it currently returns
almost no data and that "الأوس بتاعه فيه مشكلة". Diagnose in this order and fix what you
find:
1. Is a tool call even being emitted? `internal/platform/gateway/roles.go` documents a
   measured failure where the previous default model returned `finish_reason=length` with no
   tool call at all. Check the model behind `RolePrimary` and the token budget.
2. Are results being truncated below usefulness? The registry truncates to a row and byte
   ceiling — check the actual limits against a real answer.
3. Is the context assembly (`context.go`, `prompt.go`) spending the budget on history
   instead of on the answer?
4. Are denials being surfaced? A `denied_permission` that renders as an empty answer is
   indistinguishable from a broken tool. Read `assistant.tool_audit` for a real session and
   see what actually happened.
Fix these before adding a single tool — more tools on broken plumbing changes nothing.

**Stage 3 — widen coverage, per scope.** Add tools so each dashboard's assistant can answer
from the data that dashboard already shows. Every new tool declares its scope, its
permission key, its strict argument schema and its handle types, and inherits the ten steps
automatically.

*Pharmacy:* `catalog_search` (buyer-aware — it must call the **same** availability rule from
A1, so the assistant never offers something checkout would refuse), `offer_details`,
`cart_summary`, `purchase_requests_list`, `invoices_list`, `invoice_details`,
`payments_list`, `saving_products_list`, `smart_order_runs_list`, `smart_order_run_details`,
`decision_memory_search`, `branch_quota_status`, `coverage_check` ("who delivers to my
branch on Thursday"), `supplier_profile`, `favourites_list`, `notifications_list`.

*Vendor:* `variant_details`, `stock_by_warehouse`, `warehouse_transfers`,
`quota_report` (the A7 data), `coverage_report`, `orders_by_status`, `revenue_by_period`,
`revenue_by_product`, `customers_list` (buying organizations), `import_runs_list`,
`import_run_details`, `offers_performance` (impressions/clicks/conversions),
`sponsorship_status`, `team_list`, `reviews_list`.

*Admin:* `organizations_list` with filters, `organization_details`, `approvals_pending`,
`deletion_requests`, `users_search`, `user_details`, `error_logs_search`,
`audit_log_search`, `finance_overview`, `wallet_transactions_search`,
`subscriptions_report`, `visitors_report`, `platform_health`, `match_decisions_search`,
`institutional_graph` ("which buyer works reach this supplier branch").

**Stage 4 — make it answer, not just fetch.** Allow multi-step tool use within one turn
(fetch → aggregate → answer) with a per-turn call ceiling, and let a tool return a small
table the assistant can render rather than prose it has to re-describe.

**Stage 5 — re-run Stage 1's eval and report the delta.** Ship the numbers.

**Hard constraints throughout:**
- Never add a tool that takes a raw table or SQL string. `execute_sql` exists for the
  **admin developer console**, behind the developer permission — the assistant does not get
  it.
- Every id argument crossing the boundary is a **handle** (`internal/modules/assistant/handles`),
  not a database id.
- Every tool is tenant-scoped by the caller's session, never by an argument.
- Cross-tenant reads only in admin-scope tools, only through `database.AsSystem`, only for
  the fields the answer needs.
- The assistant must degrade gracefully with the Gateway off: the UI says it is unavailable;
  nothing else breaks.
- `assistant.tool_audit` records every call. Do not add a path that bypasses it.

**Acceptance:** the Stage-5 eval pass rate is materially above the Stage-1 baseline and is
reported as a number; a pharmacy asking "كام صنف ناقص عندي وأقرب مورد يوصّلي إمتى؟" gets a
real answer built from `low_stock` + `coverage_check`; a cross-tenant probe from a pharmacy
scope is refused and appears in `assistant.tool_audit` as `denied_scope`.

---

## PART 5 — DATABASE AND ARCHITECTURE HYGIENE

The client asked specifically for "no conflicts, no duplicated columns, no duplicated
tables, no duplicated logic". This part is that audit. It can run in parallel with PART 4.

**Method:** for each finding, (a) confirm it against the live database with the probe,
(b) confirm the code references with `grep`, (c) decide *remove / merge / keep and document*,
(d) if you remove, write a migration and a note in `docs/modules/<module>.md`.

**Never drop a table or column in the same release that stops writing to it.** Expand,
then contract, in two deploys — migrations run *before* the new image is promoted, and
rollback is "redeploy the old image".

### 5.1 Duplicated columns and dual sources of truth — verified

| Finding | Evidence | Action |
|---|---|---|
| **`org.branch_institutional_works` has both `work_category TEXT` and `institutional_work_id BIGINT`.** A row may carry only the text, and `branchWorkIDs` then silently drops the branch. Live: `(67,'group',NULL)`. | §2.5 | **A6.** Backfill, then treat `institutional_work_id` as the only truth and keep `work_category` as a display label. |
| **`catalog.product_variants` has no stock column, but `catalog.ProductVariant.StockQty` exists in Go** and is populated by *some* queries (`ListVendorVariants` via the `stockRollup` lateral) and not others (`GetVariant`). Reading it from the wrong source yields a permanent zero. | `catalog/domain.go:129-135`, `cmd/server/availability.go:57-70` | Rename the field to `StockQtyRollup` **or** add a `HasStock bool` guard so an unpopulated value cannot be mistaken for zero stock. Document which repository methods populate it. |
| **Deletion state is expressed two ways**: `deleted_at TIMESTAMPTZ` on most tables, and a `status` string on organizations/branches/users. WO-27 must not add a third. | `org.organizations`, `org.branches`, `identity.users` | Decide one meaning per entity and document it. `deleted_at` = soft-deleted row; `status` = lifecycle state. A blocked account is a `status`, not a `deleted_at`. |
| **`match_decisions` uniqueness is `(COALESCE(organization_id,0), decision_key)`** while the intended semantics are org-scoped *and* platform-scoped rows. WO-13 adds `scope`; make sure the unique constraint still expresses exactly one row per (scope, org, key). | §WO-13 | Verify the constraint after migration 199. |

### 5.2 Dead and duplicate tables — verified by row count and grep

| Table | Rows | Go references | Verdict |
|---|---|---|---|
| `inventory.temp_warehouses` | 0 | **0** | **Dead.** Temp warehouses live in `compare.files` (`is_temp_warehouse = true`). Drop in a contract migration after confirming no SQL string references it. |
| `inventory.father_user_temparte_warehouses` | 0 | **0** | **Dead.** Same. |
| `promo.special_offers` | 0 | 2 files | Suspicious: `promo.offers` has 3 rows and 16 referencing files, and the Go type is `promo.SpecialOffer`. Determine which table `promo.SpecialOffer` actually maps to, delete the loser, and rename the Go type to match if needed. |
| `notifications.templates` | 0 | — | Either populate and use it (WO-35 item 1) or drop it. Do not leave it. |
| `platform_admin.employee_activities` | — | **0** | **Dead.** `/admin/employee-activities` reads `platform.audit_log`. Drop it, or migrate the screen onto it — but not both. WO-17 assumes `platform.audit_log` wins. |
| `commerce.quote_requests` | 0 | — | The quote flow has notification wrappers (`notifyQuoteRequest`, `notifyQuoteProvided`, `notifyQuoteDecision`) but no rows. Confirm the feature is reachable; if it is not, say so rather than leaving three unreferenced notification types. |
| `workflow.requests` | 0 | — | Confirm reachability. |
| `compare.plans` / `compare.subscriptions` / `compare.plan_features` / `compare.subscription_users` vs `billing.*` | 3 / 0 / 9 / 0 | both used | **Probably intentional** — the compare tool sells its own subscription separately from the platform plan. Do **not** merge them without asking (question **Q4**). Document the split in `docs/modules/compare.md`. |
| `catalog.import_sessions` (1) vs `ingest.import_sessions` (1) vs `platform.import_runs` (12) | | all used | Three import-session models. `platform.import_runs` is the newest and is the unified one. Document which tool uses which, and state the intended end state (everything on `platform.import_runs`). WO-36 and WO-40 move two of them. |

### 5.3 Duplicated reference data — verified, and it breaks Corporate Operations

`org.institutional_works` contains near-duplicate rows with different ids, which means a
branch tagged with one of them has **no connection edges** while its twin does:

| Duplicate pair | Arabic title |
|---|---|
| 15 `wholesale---wholesale` / 16 `جملة-جملة` | جملة جملة |
| 10 `retail` / 17 `تجزئة` | تجزئة |
| 9 `factory` / 18 `مصنع` / 26 `مصنع` | مصنع |
| 14 `startups` / 19 `شركات-ناشئة` | شركات ناشئة |
| 13 `cooperatives` / 20 `تعاونيات` | تعاونيات |
| 5 `non-profit-organization` / 21 `منظمة-غير-ربحية` | منظمة غير ربحية |

The 18 connection edges reference only `{2, 3, 16, 22, 23, 24, 25, 37}`. So a branch tagged
`26 مصنع` (branch 81, `ZQ-alex`) is connected to **nothing** and its 416 in-stock variants
are unbuyable by every pharmacy on the platform — with no message explaining why.

**Action (migration `205_institutional_works_dedupe.up.sql`, and get it signed off first —
question Q5):**
1. Choose a canonical id per concept.
2. Repoint `org.branch_institutional_works`, `org.employee_institutional_works`,
   `org.institutional_work_connections` and `catalog.products.institutional_work_ids` onto it.
3. Soft-delete the duplicates (`deleted_at = now()`), do not hard-delete.
4. Add a **uniqueness guard** so this cannot recur: a unique index on the normalised Arabic
   title among non-deleted rows, plus validation in `AdminInstitutionalNewSubmit`
   (`internal/ui/admin_institutional_handlers.go`).
5. Add to `/admin/institutional` a **connection-matrix view** — buyer works down the side,
   supplier works across the top, a tick per edge — so an operator can see at a glance that
   `مصنع` reaches nothing. This is the screen that would have caught the problem.

### 5.4 Duplicated logic — verified

| Duplication | Where | Action |
|---|---|---|
| Availability reason → UI disposition, written twice | `offers_storefront.go`, `supplier_profile_handlers.go` | **A2** |
| The purchase rule itself, written twice (three times counting `reverifier`) | `commerce/availability.go`, `smartorder/eligibility.go`, `cmd/server/smartorder.go` | **A5** |
| Coverage called with and without the city id | `cmd/server/availability.go` vs `cmd/worker/smartorder.go` | **A8** |
| Two distance calculations | `platform.distance_meters` (SQL) and `calculateHaversineKM` (Go) | **A8** item 3 — keep the Go one for display only, comment it. |
| Institutional-works lookup, N+1 inside a loop | `org/institutional_connection.go` | **A6** item 7 |
| Client IP extraction, four ways | `httpx.ClientIP`, `errtrack`, `visitor.go`, raw `r.RemoteAddr` | **B2** |
| Modal opening, two idioms, one unimplemented | `data-modal-open` vs `$dispatch('open-modal')` | **B3** |
| Modal shells, component vs hand-rolled | `components.Modal` vs `fixed inset-0` divs | **B4** |
| Stock rollup SQL | `catalog/postgres/vendor_variants.go:stockRollup` — reuse it in A3 and WO-19 rather than writing a second | A3, WO-19 |
| Rows-per-page parsing | `pagination.RowsPerPage` vs the catalogue's own `{12,24,48,96}` | Keep both, but document why the catalogue differs (card grid, not table). |

### 5.5 Migration discipline

- Numbers `197`+ are free. Re-check `ls db/migrations | sort -V | tail -3` before each.
- Every migration ships `.up.sql` **and** `.down.sql`, both wrapped in `BEGIN; … COMMIT;`.
- Every tenant-owned table gets `organization_id`, `ENABLE`/`FORCE ROW LEVEL SECURITY`, and a
  policy using `platform.tenant_visible(organization_id)`.
- Carry Arabic column comments across; for some columns they are the only documentation.
- Every migration must be backward-compatible with the currently deployed image.
- Apply with `go run ./cmd/cli migrate`; check with `go run ./cmd/cli migrate-status`.
- **Tenant isolation is a CI gate:** every tenant-owned table needs a test proving a
  cross-tenant read returns zero rows.

---

## PART 6 — VERIFICATION AND DEFINITION OF DONE

### 6.1 Per work order

A work order is done when **all** of these hold:

1. The acceptance test in the work order has been executed and passes.
2. `templ generate` has been run if any `.templ` changed, and the `*_templ.go` files are
   committed alongside.
3. `go build ./...` passes.
4. `go vet ./...` passes.
5. `go test -short -count=1 ./...` passes.
6. New behaviour has a test. New *rules* have table-driven unit tests; new repository
   queries have integration tests against a real Postgres.
7. No new file exceeds 400 lines.
8. No user-facing Arabic string was added to a `.go` file (use `i18n`).
9. The change is one commit with a message that names the defect number and says *why*.
10. If the work order touched a module's invariants, `docs/modules/<module>.md` was updated.

### 6.2 Gate commands (when `make` is unavailable)

Run these from `F:/Dawa 24/dawa24-store`. Compare each against the baseline you captured
in §0.8 — only regressions are yours.

```bash
gofmt -l ./cmd ./internal                                    # must print nothing
go vet ./...
go build ./...
go test -short -count=1 ./...

# AI provider isolation (rule 2)
grep -rn --include='*.go' -iE '(openai|anthropic|gpt-|claude-|gemini|qwen|gemma|whisper)' \
  ./cmd ./internal | grep -v '/platform/gateway/' | grep -v '_test.go'   # must be empty

# 400-line ceiling — count must not exceed your baseline (71)
find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + \
  | grep -v ' total$' | awk '$1>400' | wc -l

# Hardcoded Arabic in Go (ceiling 0)
LC_ALL=C grep -rn --include='*.go' '"[^"]*[أ-ي][^"]*"' ./internal/ui ./internal/modules ./cmd \
  | grep -v '_test\|_templ\|/i18n/'

# CDN assets in templates (must be empty)
grep -rn --include='*.templ' -E '(src|href)="https?://(unpkg|cdnjs|cdn\.jsdelivr)' internal/ui

# Raw <dialog> in pages — must not grow
grep -rn '<dialog' internal/ui/pages/*.templ | wc -l

# Emoji in templates (ceiling 8)
LC_ALL=C grep -ro $'\xf0\x9f' --include='*.templ' internal/ui | wc -l

# !important in CSS (ceiling 3)
grep -rn '!important' internal/ui/static/css/*.css | wc -l
```

### 6.3 Manual browser verification

There is a browser preview available. For any UI work order, take a screenshot at
**1440×900** and **390×844**, in Arabic (RTL), and confirm:

- the page has no horizontal scroll at either width;
- every modal is vertically centred on desktop and closes on Escape;
- filters survive an action (B1);
- pagination counts match the rendered rows;
- no undefined CSS class renders as an unstyled element — this codebase hand-rolls its CSS
  with Tailwind-*looking* class names, so **a class no stylesheet defines renders as
  nothing**, and the page silently loses its design. Run the undefined-class check after any
  template edit.

### 6.4 End-to-end scenario — run this after Workstream A

This single walkthrough exercises the entire unified availability path. Do it with the live
data, using organization **192** (vendor, branches 76/81) and organization **188** (customer,
branches 69/73).

1. Sign in as a user of org 188, select branch **73** (`t-aswan`, works `{26, 2}`).
2. `/customer/catalog` — page 1 must show exactly `pageSize` cards. Every card must come from
   branch **76** (works `{23}`, connected via `2 → 23`) or from vendor 187 branch **75**
   (works `{16}`, connected via `2 → 16`). **No card may come from branch 81** (works
   `{26}`, no edges).
3. Walk to page 2 and page 3 — no repeats, no skips.
4. Open `/suppliers/192` — the count, the rows and the pager agree; branch-81 items are absent.
5. Add an item to the cart; check out. It succeeds.
6. Set that variant's `quota_limit` to 1 from `/vendor/quotas` (as org 192). Re-order: the
   catalogue now shows the item **disabled** with the quota reason, and checkout refuses it.
7. Run a smart order from `/customer/smart-order/new` against branch 73 with that item: the
   results screen reports `OutcomeQuotaBlocked` — **not** "ordered".
8. Release the branch's quota from `/vendor/quotas`. Re-run: the item is orderable again on
   all four surfaces.
9. Switch to branch **69** (`t-cairo`, works `{2}`) and repeat step 2 — the connected set
   changes and the catalogue changes with it.
10. Sign in as a user of org **192** and open `/customer/catalog`. **None** of org 192's own
    variants may appear, on any page. Attempt `POST /cart/add` with one of them by hand — it
    must be refused with `own_organization`.

Record the result of every step. If any step disagrees with `CheckAvailability`, the
unification is not finished.

### 6.5 Reporting

At the end of each workstream, produce a short report: what was changed, what was verified,
what was **not** done and why, and the current state of every gate against the baseline.
State failures plainly with the output. Do not describe a work order as complete if part of
it was skipped — say which part and why.

---

## PART 7 — DECISIONS NEEDED FROM THE CLIENT

Do not guess on these. Implement the stated default, mark it clearly in the code with a
comment, and raise the question.

**Q1 — Coverage time windows.** `workflow.CoverageService.ServesPoint` accepts a time and
ignores it; only the weekday is enforced. Should a supplier's delivery *hours* block an
order outside them, or is the weekday the whole rule?
*Default until answered:* keep today's behaviour (weekday only), delete the unused
parameter, and correct every comment that claims otherwise.

**Q2 — Subscription cooldown: 25 days or 2?** The requirement says both. Which applies to a
paid plan, and does the other number mean something else (a minimum wait before *any*
second change)?
*Default until answered:* 25 days for a paid plan, 2 days as an absolute minimum after any
purchase, both configurable in `platform_admin.system_settings`, free plans exempt.

**Q3 — May a supplier add a product the master catalogue does not contain?** WO-09's fix
requires selecting a master product. If suppliers must be able to introduce new products,
that is a separate approval workflow.
*Default until answered:* selection from the master catalogue is required.

**Q4 — Are the compare tool's plans deliberately separate from billing plans?**
`compare.plans` (3 rows) and `billing.plans` (3 rows) are parallel subscription systems.
*Default until answered:* keep them separate and document the split.

**Q5 — Institutional-works de-duplication.** Six concepts exist twice or three times
(§5.3), and one of the duplicates has no connection edges, which silently makes 416
in-stock variants unbuyable. Merging changes which suppliers a pharmacy can reach, so it is
a business decision, not a cleanup.
*Default until answered:* do **not** merge. Ship the connection-matrix view and the
"branches with no institutional works" panel (A6 items 8-9) so the client can see the
problem and decide.

**Q6 — Blocked-account semantics (WO-27).** When an organization is blocked, what happens to
its *existing* orders, invoices and wallet balance? They must remain for accounting, but
should the counterparty still see them?
*Default until answered:* existing orders and invoices remain visible to the counterparty and
to admins; the organization disappears from every listing, search and dropdown; its
catalogue disappears; its users cannot sign in.

---

## APPENDIX A — DEFECT → WORK ORDER INDEX

| # | Client's summary | Work order | Depends on |
|---|---|---|---|
| 1 | Filter cleared after any action | WO-01 | **B1** |
| 2 | Change-requests page too plain | WO-02 | — |
| 3 | Analytics shows localhost IP | WO-03 | **B2** |
| 4 | Messages missing sender IP | WO-04 | **B2** + migration 198 |
| 5 | Trash list too vague | WO-05 | — |
| 6 | Temp-warehouse modals at the bottom | WO-06 | **B4** |
| 7 | Non-orderable products in supplier directory | WO-07 | **A1 A3 A4** |
| 8 | Offers review button does nothing | WO-08 | **B3** |
| 9 | Catalogue rejects new products | WO-09 | — |
| 10 | Catalogue pagination shows one product | WO-10 | **A1 A3** |
| 11 | Offer locations: governorate/city/map | WO-11 | — |
| 12 | Saving-products linking navigates wrongly | WO-12 | — |
| 13 | Decision memory too simple | WO-13 | migration 199 |
| 14 | Disabling a city hides it | WO-14 | — |
| 15 | Subscription change cooldown | WO-15 | **Q2** |
| 16 | No clear offer-details action on orders | WO-16 | — |
| 17 | Employee activity page incomplete | WO-17 | **B2** |
| 18 | Subscriber log needs organising | WO-18 | — |
| 19 | Supplier items need filters and pagination | WO-19 | — |
| 20 | Admin warehouses incomplete | WO-20 | WO-17 |
| 21 | "Pharmacy" filter should be "Buyer" | WO-21 | — |
| 22 | Offers page needs per-tab filters | WO-22 | — |
| 23 | Product sponsorship needs organising | WO-23 | — |
| 24 | Chat history needs filters | WO-24 | migration 200 |
| 25 | Approvals registration modal empty | WO-25 | **B3** |
| 26 | Error diagnostics modal empty | WO-26 | — |
| 27 | Unify deletion requests | WO-27 | **Q6**, WO-35 |
| 28 | Finance page disorganised | WO-28 | — |
| 29 | Jobs page needs full control | WO-29 | WO-17 |
| 31 | Default roles not editable | WO-31 | migration 201 |
| 32 | Plans page breaks past four cards | WO-32 | — |
| 33 | User management needs widening | WO-33 | WO-17, WO-35 |
| 34 | Branch quotas not working | WO-34 | **A7** |
| 35 | Notifications incomplete | WO-35 | — |
| 36 | Org import must use the real tool | WO-36 | WO-40's mechanism |
| 37 | Admin refunds | WO-37 | migration 202, WO-35 |
| 38 | Smart order not bound to coverage | WO-38 | **A5 A8** |
| 39 | Per-tool AI model configuration | WO-39 | migration 203 |
| 40 | Resumable warehouse uploads | WO-40 | — |
| 41 | SEO / keywords / AI tab | WO-41 | migration 204 |
| 42 | AI Assistant too limited | WO-42 | **A1** (for `catalog_search`) |

*(The client's list has no item 30.)*

## APPENDIX B — SUGGESTED EXECUTION ORDER

| Phase | Contents | Why here |
|---|---|---|
| 1 | **B1, B2, B3, B4** | Four small changes that unblock or partly close #1, #3, #4, #6, #8, #25, #26. Cheapest value in the plan. |
| 2 | **A1, A2** | The shared decision object. Everything in Workstream A depends on it. |
| 3 | **A6** + migration 197 | Corporate Operations must be correct before A3 filters on it, or A3 will hide products for the wrong reason. |
| 4 | **A3, A4** | The catalogue and supplier profile. Closes #7, #10. |
| 5 | **A5, A7, A8** | Smart ordering, quotas, coverage. Closes #34, #38. |
| 6 | **§6.4 end-to-end scenario** | Prove Workstream A before building on it. |
| 7 | WO-09, WO-14, WO-05, WO-02, WO-16, WO-21 | Small, self-contained, high visibility. |
| 8 | WO-17, WO-19, WO-20, WO-18, WO-22, WO-23, WO-24, WO-29, WO-32 | List-screen work; they share the pagination/filter idiom, so do them together. |
| 9 | WO-11, WO-12, WO-13 | Geography and matching memory. |
| 10 | WO-15, WO-27, WO-33, WO-35, WO-37 | Lifecycle, notifications and money. WO-35 first — the others notify. |
| 11 | WO-40, WO-36 | Resumable import sessions; build the mechanism once. |
| 12 | WO-31, WO-39, WO-41 | Configuration surfaces. |
| 13 | WO-42 | The assistant. Largest, and it consumes A1's availability rule, so it is last. |
| 14 | **PART 5** hygiene contractions | Drop dead tables only after two clean deploys. |

## APPENDIX C — QUICK REFERENCE

```
Build            go build ./...
Generate templ   templ generate            (after ANY .templ edit; commit the *_templ.go)
Vet              go vet ./...
Test             go test -short -count=1 ./...
Migrate          go run ./cmd/cli migrate
Migration status go run ./cmd/cli migrate-status
Run server       go run ./cmd/server        (needs DATABASE_URL, REDIS_URL, SESSION_SECRET)
Run worker       go run ./cmd/worker
Query the DB     PROBE_DSN=... go run ./tmp/probe "select ..."

Next migration number: 197  (verify: ls db/migrations | sort -V | tail -3)

The one availability rule:  internal/modules/commerce/availability.go
Its only probe:             cmd/server/availability.go
The institutional rule:     internal/modules/org/institutional_connection.go
The coverage rule:          internal/modules/workflow/coverage_service.go
The quota rule:             internal/modules/commerce/quota.go
The redirect helper:        internal/ui/request_helpers.go:169
The modal manager:          internal/ui/static/js/app.js:482
Pagination:                 internal/shared/pagination/pagination.go
                            internal/ui/components/pagination.templ
AI roles and models:        internal/platform/gateway/roles.go
Decision memory port:       internal/shared/matchflow/memory.go
```

---

*End of plan. If something in this document contradicts the code, the code is the truth —
re-verify, then correct this document in the same commit.*

---

# PART 8 — REVIEW OF WO-01..17 AS SHIPPED, AND THE PLAN FROM HERE

**Reviewed:** 2026-09-09, commits `98c46cd0..8f8744b0` (25 commits, 320 files,
+34,497 / −14,331). Migrations 197–201 written and applied to the live database
(200 of 201 applied).

**Baseline at review time:** `go build ./...` PASS, `go vet ./...` PASS,
`go test -short ./...` PASS. The failures below are behavioural; none of them
show up in the build.

## 8.1 What was built correctly

Stated plainly, because it is most of the work and the fixes below should not
be read as a verdict on the whole:

- **A3 offer-level pagination is real.** `catalog.postgres.ListBuyerOffers`
  pages `catalog.product_variants` joined to products, organizations and the
  shared `stockRollup`, with vendor approval, own-org exclusion and the
  institutional-connection EXISTS clause all *in the SQL*. Verified against the
  live database: for branch 73 it counts 1,695 offers, which is exactly
  142 (vendor 187 / branch 75) + 1,553 (vendor 192 / branch 76).
- **A1 `CheckAvailabilityBatch`** runs the checks in the documented order,
  batches vendors and variants, caches coverage per vendor, and returns the same
  refusal reasons as the single-line path.
- **A5 wiring** is present in *both* composition roots (`cmd/server/smartorder.go`
  and `cmd/worker/smartorder.go`), and `reverifier` now calls the gate instead
  of carrying a third copy of the rule.
- **A6 replace semantics** are correct: `saveBranchInstitutionalWorksTx` deletes
  then re-inserts and returns its error, so un-ticking a work now takes effect.
  All three branch forms submit the field, so the delete cannot silently wipe.
- **B1** preserves the referer's query string on same-path redirects and
  excludes the notice keys; scroll restoration is in `app.js`.
- **B3** listens for the Alpine `open-modal`/`close-modal` events.
- **WO-14** switched to `ListAllCities` — one line, correct.
- **WO-16** offer-details buttons exist on the customer *and* vendor sides.
- **WO-17** has date filters, XLSX export and `B2BPagination` with the filters in
  `QueryValues`; `WriteAudit` call sites went from 7 to 43.
- **WO-13** lookup semantics are right: org rows plus platform rows, org wins on
  collision, per-organisation opt-out honoured.

## 8.2 Defects found, and what was done about them

### D1 — The flagship bug was reproduced one layer down *(fixed, commit `bb0388ea`)*

**Severity: critical.** This is almost certainly what the client is seeing.

A3 moved pagination onto offers so the pager could count what it renders, and
left **coverage** behind in Go, applied to the rows a page had already returned
(`customer_handlers.go` step 6, `buildCatalogVariantCards` dropping
`DispositionHidden`).

Measured on the live database for branch 69 (`t-cairo`, org 188):

| | offers |
|---|---|
| counted by the SQL | **1,695** |
| actually orderable (vendor 192 does not deliver to Cairo) | **142** |

Page 1 of 24 rendered roughly **two cards** under a pager offering 71 pages.
That is the original WO-10 complaint, produced by its own fix. The 20 %-drop
warning the plan asked for was implemented and would have logged it — but a log
line is not a fix.

**Fixed by** `CoverageService.VendorsServing`, which resolves the covering
suppliers as a set using the same `WHERE` clause `ServesPoint` applies to one
supplier, and `BuyerOfferQuery.CoveredVendorOrgIDs` / `ApplyCoverage`, which put
that set in the query. `ApplyCoverage` distinguishes "coverage was evaluated and
nobody covers you" from "coverage does not apply here" — a distinction a nil
slice cannot make, and getting it wrong is how the uncovered suppliers got into
the count. Both the catalogue and the supplier profile resolve it through one
helper, `h.coveringVendorsFor`. Regression test added and run against the live
database.

### D2 — The second purchase rule was kept as a fallback, and made worse *(fixed, commit `265b8fa9`)*

**Severity: high (latent).** The plan said to delete `smartorder.Evaluate`'s
inline rule. It was kept, reached whenever `s.gate == nil`. That is not a dead
branch:

- both composition roots now construct the runner with a **nil `CoverageGate`**
  (`pipeline.NewRunner(repo, nil, …)`), and
- `cmd/server/smartorder_runner.go` calls `SetAvailabilityGate` only inside
  `if commSvc != nil`.

The fallback reads a nil coverage gate as `covered: true`. So a wiring slip
would not restore the old behaviour — it would produce something worse than
anything that shipped before: every supplier treated as delivering everywhere.

**Fixed by** deleting the inline rule. A missing gate now leaves the verdict map
empty and refuses every candidate, matching how `CheckAvailability` fails closed
without a probe. The institutional tests were rewritten against the gate
contract the stage actually has now.

### D3 — Six invented CSS classes render unstyled *(fixed, commit `bb0388ea`)*

**Severity: medium, and highly visible.** New markup used class names no
stylesheet defines, so those buttons and badges render as unstyled inline text —
the "corrupted design" failure mode this codebase is known for. Confirmed absent
from the tree before these commits:

| invented | occurrences | replaced with |
|---|---|---|
| `btn-outline-primary` | 4 files | `btn-outline-brand` |
| `btn-outline-emerald` | 1 | `btn-outline-success` |
| `btn-outline-sky` | 1 | `btn-outline-secondary` |
| `badge-subtle` | 2 | `badge-slate` |
| `alert-neutral` | 1 | `alert-info` |
| `match-decisions-page` | 1 | removed (no rule, no purpose) |

`check-undefined-classes` is at **94 against a ceiling of 64** and therefore
failing. Most of the remainder are Alpine state names in `:class` bindings,
which the gate's own comment allows for; the six above were not.

### D4 — Blank success banner after publishing a variant *(fixed, commit `bb0388ea`)*

**Severity: low, user-visible.** `VendorVariantNewSubmit` hand-built
`"/vendor/products?notice=" + message + "&notice_type=success"`. `noticeFrom`
reads the *kind* from `notice_type` and the *message* from `notice_msg` then
`msg` — so it found the kind and no message, and every successful publish showed
an empty green banner. Now routed through `redirectWithNotice`, which is the
only place that knows those parameter names.

### D5 — Quota read one row at a time *(fixed, commit `3913bf1e`)*

**Severity: medium (performance).** `CheckAvailabilityBatch` batched the vendor
and variant loads and then called `BranchQuotaFor` **inside the per-line loop**.
On a 96-card catalogue page of capped variants that is 96 sequential round trips
to a database the application reaches over the public internet.

**Fixed by** `BranchQuotaUsedBatch` — the same `SUM` over the same
`quotaCountsSQL` predicate, grouped — declared as its own optional interface so
the in-memory test repositories keep compiling and fall back to the per-variant
read.

### D6 — Dead 200-row product fetch *(fixed, commit `bb0388ea`)*

`VendorVariantNewPage` fetched 200 of the 20,000 catalogue products into
`MasterProducts`, which the template never reads — the picker is a combobox
backed by `/vendor/catalog/search-json`. Removed.

## 8.3 Defects found and NOT yet fixed

These are recorded deliberately; each is a work item below.

### D7 — New hardcoded Arabic in Go *(gate ceiling 0)*

Seven files added user-facing Arabic string literals to `.go` sources, which
`check-hardcoded-arabic` forbids outright:

```
internal/modules/commerce/availability_batch.go
internal/modules/platform_admin/postgres/audit_repo.go
internal/ui/admin_decision_memory_handlers.go
internal/ui/admin_employee_activities_handlers.go
internal/ui/customer_order_edit_handlers.go
internal/ui/customer_order_review_handlers.go
internal/modules/billing/subscription_service.go   (the cooldown messages)
```

The gate was already red (179 occurrences across the tree, most pre-existing),
which is why this passed unnoticed. **Work item R1** below.

### D8 — Decision-memory lookup writes to shared rows

`LookupDecisions` bumps `hit_count`/`last_used_at` with an `UPDATE … RETURNING`
whose `WHERE` is `(organization_id = $1 OR scope = 'platform')`. Every
organisation's import therefore takes row locks on the shared platform rows.
With one importer this is invisible; with two large imports running at once it
serialises them. **Work item R2.**

### D9 — `check-undefined-classes` still failing at 94/64

D3 removed six. The ceiling needs either the remaining genuine misses fixed or
a deliberate, documented raise. **Work item R3.**

### D10 — `gofmt` and file-size gates

134 files are unformatted and 58 exceed the 400-line ceiling. Both were red
before this work (the baseline was 71 oversized files, so the count actually
*improved*). Not regressions, but `make check` cannot pass until they are
addressed. **Work item R4**, low priority.

## 8.4 Corrections to my own earlier reading

Recorded so the next reader does not re-litigate them:

- I initially believed the WO-15 subscription cooldown was enforced only in the
  page handler and was bypassable by a direct POST. **That is wrong.** It is
  enforced inside `billing.Service.SubscribeWithWallet`
  (`subscription_service.go:150-175`), which is where the plan asked for it. My
  first grep was truncated by `head -20`.
- A6's unconditional `DELETE` before re-insert looked like it could wipe a
  branch's works on any partial update. It cannot: all three `UpdateBranch`
  callers submit `institutional_works`, and all three forms render the field.
  Worth a regression test (**work item R5**), not a fix.

## 8.5 Remaining work, in order

**Repairs first (R1–R5), then the untouched work orders.**

| Item | What | Why here |
|---|---|---|
| R1 | Move the new hardcoded Arabic in the seven files above into `internal/shared/i18n` keys | Gate ceiling is 0; every new violation makes the eventual cleanup larger |
| ~~R2~~ | ~~Stop bumping `hit_count` on platform-scope rows during an org's lookup~~ **DONE** (`5f7…`): the org's own rows are still stamped, the platform rows are read through a `UNION ALL` and excluded when the org has its own answer | Cross-tenant lock contention on the shared cache |
| R3 | Clear the remaining genuine undefined classes, or raise the ceiling with a written argument | `check-undefined-classes` red |
| R4 | `gofmt -w` the tree; do not attempt the 58-file split | Pre-existing, cheap for gofmt, expensive for the rest |
| R5 | Regression test: editing a branch without touching its works must not clear them | A6's delete-then-insert has no guard other than the forms |
| — | ~~**WO-18**~~ **DONE** (`85f3f691`) — subscriber log joined, filtered, paged; expiry read from `expires_at`; the renewal trail now actually written | |
| — | ~~**WO-19**~~ **DONE** (`00c38766`) — supplier stock listing joined; branch + per-warehouse quantities; seven filters | |
| — | ~~**WO-20**~~ **DONE** (`60d40311`) — admin warehouses filtered, paged, and create/edit/enable/disable added | |
| — | ~~**WO-21**~~ **DONE** (`13abcb72`) — the order filter is the buyer, derived from the orders themselves | |
| — | ~~**WO-26**~~ **DONE** (`256497e2`, `+audit`) — error and audit modals load their body instead of embedding escaped JSON | |
| — | ~~**WO-22**~~ **DONE** (`e8aec662`) — offers tab filtered and paged; moderation and lifecycle status separated | |
| — | ~~**WO-24**~~ **DONE** (`765029ea`) — conversation audit: user, org, date and flagged filters; flag derived from `assistant.tool_audit` denials | |
| — | ~~**WO-25**~~ **DONE** (`6b6fbbbe`) — registration modal reads `name` as well as `legal_name`; branches, works, documents and requester added | |
| — | **WO-23, WO-27, WO-28, WO-29 and the Level-3 systems (31–42)** | The remaining work |

Nothing in §8.2 changes the sequencing in Appendix B. Phase 6 — the end-to-end
scenario in §6.4 — should now be re-run, because D1 invalidated its result.
