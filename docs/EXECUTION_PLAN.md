# Dawa24 Store — Execution Plan

**For:** an AI coding agent continuing this codebase
**Written:** 2026-08-16 · **Measured against:** commit `f831b1b`

Read `AGENTS.md` first. Then Part 0 and Part 1 of this file. Then execute tasks
in order. **Do not skip the verification block at the end of a task** — every
one of them exists because something silently broke without it.

---

# PART 0 — Where the project actually is

Measured, not estimated:

| | Value |
|---|---|
| Migrations | **31 of 31** applied (029 was deleted from disk and restored — see REVIEW_AND_PLAN_2026-08-16b.md) |
| Tables | 98 |
| API routes | 165+ |
| Admin routes | **37** (measured; the 60+ figure was not accurate) |
| templ files | **38** (16 components, 4 layouts, 18 pages) |
| UI page routes | **24** (all 20 planned screens + static assets) |
| ETL | 740 lines with real connections; extract/load still thin |
| Coverage: domain/service | Verified with unit test suites |
| Coverage: every `http/` package | test files exist for all 12 modules, **9–17% coverage** — authorization paths only |
| Coverage: `postgres/` packages | 6 suites exist; **5 still skip or fail**, only `catalog` runs (81.7%) |

**The system runs.** Register, login, authenticated and anonymous access all
work against PostgreSQL, and the unit suite passes.

**Caution when reading a green suite:** `ok ... 0.0% of statements` means the
tests skipped and executed nothing. Six repository suites reported exactly that.
Check the coverage number, not the `ok`.

## The database is real and connected

```
host  postgres-u74003.vm.elestio.app:5432
db    dawa24_store          ← underscore, NOT dawa24-store
roles postgres (superuser, migrations) · dawa24_app (least privilege, app)
```

**The application currently connects as `postgres`.** That is the owner's
decision and is respected. Consequence to keep in mind: a superuser bypasses
row-level security unconditionally, so the 53 RLS policies are inert until the
connection switches to `dawa24_app`. Tenant isolation is presently enforced only
by `WHERE` clauses in Go. **Do not write code that relies on RLS as the only
guard.**

---

# PART 1 — Rules that are enforced by tooling

Breaking any of these fails the build. They are not style preferences.

1. **Money is never `float64`.** Use `internal/shared/money.Amount`.
2. **No AI provider name outside `internal/platform/gateway/`.**
   `make check-provider-isolation`.
3. **400 lines per Go file.** `make check-file-size`.
4. **`modules/A` must not import `modules/B`.** `depguard` in `.golangci.yml`.
5. **Never edit an applied migration.** The runner checksums them and refuses to
   start. Add a new numbered migration.
6. **Identity comes from the session, never from the request.** No handler may
   read a user or organisation id from a query parameter, path parameter or
   body. Use `authctx.UserID(ctx)` or `database.TenantFrom(ctx)`. Four separate
   auth bypasses have already been fixed from exactly this pattern — under the
   names `user_id`, `customer_id`, `org_id` and `vendor_org_id`.
7. **Tenant-scoped queries go through `db.InTx` / `db.InReadTx`.** Never
   `db.Pool()` in a module.

## The verification block

Run this after every task. It is the whole quality gate:

```bash
cd "F:/Dawa 24/dawa24-store"
gofmt -l . && go build ./... && go vet ./... && go test ./... && go test ./test/
```

Plus, when SQL changed:

```bash
go test -run "TestRepositorySQL|TestWriteSQL" ./test/
```

That second one parses the migrations and checks every column named in every
`SELECT`, `INSERT` and `UPDATE` against them. **It has caught 32 runtime-fatal
bugs.** It cannot be skipped.

---

# PART 2 — Task list

Tasks are ordered by dependency. Each has: files, exact work, and a
verification that must pass before moving on.

---

## TASK 1 — Repository tests against a real PostgreSQL

**Why first:** every `postgres/` package is at 0%. All 32 SQL bugs found so far
were found by a parser or by running the server by hand. Repository tests catch
them at commit time instead.

**Pattern to copy:** `test/integration/rls_test.go`. It connects via
`DATABASE_URL`, skips when unset, and skips when connected as a superuser.

**Files to create**, one per module:

```
internal/modules/catalog/postgres/repository_test.go
internal/modules/inventory/postgres/repository_test.go
internal/modules/commerce/postgres/repository_test.go
internal/modules/org/postgres/repository_test.go
internal/modules/billing/postgres/repository_test.go
internal/modules/identity/postgres/repository_test.go
```

**Each file must:**

1. Skip cleanly when `DATABASE_URL` is unset — CI without a database must still
   pass.
2. Create fixtures at fixed high ids (88000+) and delete them at the start of
   the run, not the end. A failed run leaves data behind for inspection; the
   next run clears it. `resetFixtures` in `rls_test.go` shows the shape.
3. Exercise **every** repository method: create, read, update, delete, list.
4. Assert money round-trips exactly. `NUMERIC` → `money.Amount` → `NUMERIC`
   must be identical to the cent.
5. Assert nullable columns scan. A row with every optional field empty must not
   error — that exact failure took down `/org/organizations`.

**Run with:**

```bash
DATABASE_URL="postgres://postgres:<password>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require" go test ./internal/modules/.../postgres/
```

**Done when:** every `postgres/` package reports above 60% coverage, and
`go test ./...` still passes with no `DATABASE_URL` set.

---

## TASK 2 — Handler tests

**Why:** handlers are where authorization lives, and RLS cannot protect an
endpoint that never checks the caller.

**Pattern to copy:** `internal/modules/identity/http/handlers_test.go`. It uses
a `stubRepo` whose every method calls `t.Fatalf` — if a request reaches the
repository, it got past a check it should not have.

**Files to create:** `<module>/http/handlers_test.go` for catalog, inventory,
commerce, org, billing, promo, notifications, ingest, workflow, hr,
platform_admin.

**Each file must assert:**

1. Every route in that module rejects an anonymous caller with **401**.
2. Every route rejects a forged session token with **401**.
3. Tenant-scoped routes return **403** with code `tenant.required` when the
   caller has no active organisation.
4. Malformed JSON returns **422**, and unknown JSON fields return **422**
   (`DecodeJSON` uses `DisallowUnknownFields`).
5. Error responses parse as `httpx.ErrorBody` and carry a non-empty
   `request_id`.

**Verify the tests are not vacuous** by mutation: temporarily make
`SessionFrom` return `ok=true` for a nil session *and* remove the
`ValidateSession` error check in `RequireAuth`. The tests must fail. Removing
only one must not, because the two layers are deliberate defence in depth.
Restore both afterwards.

**Done when:** every `http/` package above 50% coverage; mutation check
confirmed.

---

## TASK 3 — Admin surface

**Why:** 4 routes exist against ~275 in Laravel. This is the largest API gap.

**Architecture — read this before writing code.** There is **no admin module**.
An admin module would have to import every other module to do its job, which
`depguard` forbids and which would collapse the boundaries the whole design
rests on. **Admin is a permission level, not a bounded context.**

Each module gains `internal/modules/<name>/http/admin.go` registering routes
under `/api/v1/admin/<module>/…`, gated by
`identityHttp.RequirePermission("<module>.admin", log)`.

**Routes to add, by module:**

| Module | Admin routes |
|---|---|
| `org` | list pending orgs · approve · reject · suspend · force-update · list all members across orgs |
| `identity` | list users (filter role/status) · get · suspend · reactivate · reset MFA · assign platform role · list sessions · revoke session |
| `catalog` | list all products across tenants · force-deactivate · category/brand CRUD (already partly present — move under admin) |
| `commerce` | search orders across tenants · view any order · force status transition · refund |
| `billing` | list all subscriptions · adjust wallet with reason · list all payments · mark invoice paid |
| `promo` | approve/reject ads · approve sponsorships · manage `ad_plans` |
| `platform_admin` | countries/cities/currencies/languages CRUD · settings CRUD · translations CRUD · audit-log viewer |
| `ingest` | list all import sessions across tenants |

**Cross-tenant reads require `database.AsSystem(ctx)`** — explicit, greppable,
and every call site needs a comment saying why.

**Every state-changing admin action writes `platform.audit_log`** in the same
transaction as the change. Columns: `actor_user_id`, `action`, `entity_type`,
`entity_id`, `before` JSONB, `after` JSONB, `ip`, `request_id`. The table is
append-only for `dawa24_app` by grant.

**Also add**, migration `031_admin_permissions.up.sql`: one `<module>.admin`
permission per module, granted to the `admin` and `super_admin` roles via
`identity.role_permissions`.

**Done when:** ~60 admin routes exist, each requires its permission, each
mutation writes an audit row, and a non-admin user gets 403 on every one.

---

## TASK 4 — Frontend

**The largest remaining body of work.** 7 page routes exist against a 20-screen
plan. `internal/ui` is 2,685 lines across 13 templ files.

**Stack is settled (ADR 0001): templ + HTMX + Alpine + Tailwind. Do not
introduce React, Next.js, or a client-side router.**

### 4a — Finish the component library

Existing: `base`, `buttons`, `forms`, `badges`, `pagination`.

Add to `internal/ui/components/`:

```
DataTable      sticky header, sortable columns, keyboard nav, row actions
Modal          focus trap, ESC to close, backdrop click
Drawer         cart and filter panels
Toast          success/error, auto-dismiss, ARIA live region
Tabs
Dropdown
DatePicker     Arabic month names
FileDropzone   presigned upload (see 4c)
MoneyDisplay   tabular-nums, always 2 decimals, currency always shown
Avatar
Skeleton
EmptyState     illustration slot + the action that fills it
```

**Every component ships with four states: loading, empty, error, partial.**
This is the single biggest determinant of whether the app feels finished, and
retrofitting them later means touching every screen again.

### 4b — Screens, in dependency order

Each screen is a `.templ` file plus a handler in `internal/ui/`.

| # | Screen | Route | API it consumes |
|---|---|---|---|
| 1 | Login + MFA challenge | `/auth/login` (exists, finish MFA) | `POST /auth/login`, `/auth/mfa/verify` |
| 2 | Password reset | `/auth/forgot`, `/auth/reset` | `/auth/password/*` |
| 3 | Register + org onboarding | `/auth/register`, `/onboarding` | `/auth/register`, `POST /org/organizations` |
| 4 | Vendor product list | `/vendor/products` | `GET /catalog/products` |
| 5 | Vendor product editor | `/vendor/products/{id}` | `PUT /catalog/products/{id}`, variants |
| 6 | Vendor inventory | `/vendor/inventory` | `/inventory/warehouses`, `/stocks/low` |
| 7 | Vendor transfers | `/vendor/transfers` | `/inventory/transfers` + receive/cancel |
| 8 | Import wizard | `/vendor/ingest` (exists, finish) | full `/ingest/*` incl. SSE |
| 9 | Customer catalogue + search | `/catalog` | `GET /catalog/search` |
| 10 | Product detail | `/catalog/{id}` | `GET /catalog/products/{id}` |
| 11 | Cart | `/cart` | `/commerce/cart`, `PATCH` quantity |
| 12 | Checkout | `/checkout` | `POST /commerce/checkout` |
| 13 | Customer orders | `/orders`, `/orders/{id}` | `/commerce/orders` + history |
| 14 | Vendor fulfilment | `/vendor/orders` | `/commerce/vendor/shipments`, shipment status |
| 15 | Offers + sponsorships | `/vendor/offers` | `/promo/*` |
| 16 | Admin org approvals | `/admin/approvals` (exists, wire to real API) | admin org routes |
| 17 | Admin users | `/admin/users` | admin identity routes |
| 18 | Admin reference data | `/admin/settings` | admin platform routes |
| 19 | Notifications centre | `/notifications` | `/notifications/*` + SSE badge |
| 20 | Public landing + policy pages | `/`, `/privacy`, `/terms` | `/platform/settings/public` |

### 4c — Three things that will bite

**Arabic first.** Design RTL, verify LTR second. Test every screen with real
Arabic product names — they change line height and truncation. `i18n.Lang.Dir()`
returns `rtl`/`ltr`; the `Locale` middleware already resolves it into context.

**SSE needs the right headers.** `X-Accel-Buffering: no` or the reverse proxy
buffers everything until the stream closes and progress looks frozen. The import
wizard's stream in `internal/modules/ingest/http/wizard_handlers.go` is the
working reference.

**Uploads are presigned, never proxied.** `storage.PresignPut` exists. The
browser PUTs directly to MinIO/S3; the client then calls a confirm endpoint with
the object key. Proxying a 200 MB vendor spreadsheet through the app holds a
request goroutine for minutes and competes with checkout.

### 4d — Anti-slop rules

Flat surfaces, one border colour, one shadow level for overlays. No gradients,
no glassmorphism, no decorative icons, no emoji. Numbers right-aligned and
`tabular-nums`. Money never abbreviated in a table. Density over whitespace —
this is a tool people use for hours, not a marketing page.

Budget: **under 40 KB JS total**, first contentful paint under 1.2s on 3G, no
layout shift.

**Done when:** all 20 screens exist, every one has all four states, RTL verified,
and a vendor can log in → add a product → import a file → see an order, entirely
through the UI.

---

## TASK 5 — The ETL

**Current state:** `cmd/etl/` has `pipeline.go`, `transformer.go`,
`validator.go` (334 lines) and `internal/modules/etl/` has domain and helpers
(347 lines). Structure exists. **There is no real extract or load.**

**Spec:** `../docs/rebuild/REBUILD_MASTER_PLAN.md` §11.

**Six stages, in `cmd/etl`:**

1. **Extract** — connect to the legacy MariaDB, stream each table to
   newline-delimited JSON, chunked and resumable. The driver
   (`go-sql-driver/mysql`) is already in `go.mod`.
2. **Validate** — orphan sweep across the **36 `*_id` columns with no foreign
   key**; invalid JSON; out-of-range enums; duplicate emails; negative money.
   Emit a **pre-migration defect report and stop**. Do not transform dirty data.
3. **Transform** — type mapping per master plan §5.5; `users` decomposition;
   order-system unification; entitlement consolidation; blobs to object storage;
   **UTC → `TIMESTAMPTZ` explicitly** (a naive cast shifts every timestamp by
   2–3 hours).
4. **Load** — `COPY`, foreign keys deferred, indexes created after.
5. **Verify** — the gate must compute **both sides itself**. It previously took
   the counts as parameters and compared them, which is why it reported success
   having read nothing. Row counts exact; checksums; money sums **to the cent**;
   zero FK orphans; 100% JSON parse; timestamp min/max after conversion; business
   invariants (`SUM(lines.line_total) = order.final_price`).
6. **Reconcile** — per-table go/no-go report.

**Preserve legacy primary keys.** User 4417 stays user 4417. That makes
verification a matter of comparing ids and makes rollback possible.

**Blocking question — resolve before writing transform code:** which legacy
order system is authoritative, `orders`+`order_items` or
`main_orders`+`adv_orders`? The target schema assumed the latter. Verify with
row counts and recent `created_at` in both. Getting this wrong migrates the
wrong data and is discovered after cutover.

**Done when:** a full legacy dataset extracts, validates, transforms, loads and
passes every verification gate — with the gate computing both sides.

---

## TASK 6 — Production hardening

1. **Switch the app to `dawa24_app`** (owner's decision when ready). Until then,
   RLS is inert. `cmd/dbcheck -provision` creates the role and grants.
2. **Rotate exposed credentials.** The Gateway virtual key
   `sk-virt-a5f7e81f…`, the Redis password, and the temporary `dawa24_app`
   password have all appeared in transcripts.
3. **Object storage** — provision MinIO, create the bucket, set `STORAGE_*`.
   Required before `APP_ENV=prod` will boot and before Task 4c.
4. **`APP_ENV=prod`** once storage exists.
5. **Backups** — PITR on `dawa24_store` plus a **tested restore into a scratch
   database**. An untested backup is not a backup.
6. **Metrics** — Prometheus endpoint: RED per module, River queue depth, gateway
   latency and fallback rate, connection pool saturation.

---

# PART 3 — Failure patterns already seen in this codebase

Every one of these was a real bug here. Check for them in new code.

| Pattern | What happened |
|---|---|
| **Identity from the request** | Four endpoints read the user or org from a query parameter. Anyone could withdraw from any wallet. |
| **Trusting a header** | `X-Dawa-Org-ID` was honoured without checking membership, so any caller could name any tenant. |
| **Capturing a dependency at construction** | Repositories grabbed the DB pool at route-mount time, before it connected, and held nil forever. |
| **Casting a parameter in SQL** | `set_config(…, $1::text, …)` with an int64 made pgx fail to encode. Every tenant transaction broke. |
| **Type assertion through a wrapper** | `w.(http.Flusher)` failed because the logger middleware wrapped the writer. All SSE was dead. |
| **Column that does not exist** | 32 columns were named in SQL but never created. Compiles fine, fails at runtime. |
| **Nullable column into a non-pointer** | `cannot scan NULL into *string` took down an endpoint the moment one row had an empty field. |
| **Errors classified as internal** | `ErrNoTenant` returned 500 "something went wrong on our side" for a legitimate request. |
| **Non-atomic state change** | Transfer status and stock credit in separate transactions; concurrent receives could double-credit. Fixed with compare-and-swap. |
| **A verification query that itself errors** | The RLS check used `pg_tables.forcerowsecurity`, which does not exist. It would have errored rather than reported. |
| **Claiming completion without measuring** | A commit claimed "100% test coverage" while every `http/` and `postgres/` package was at 0%. |

---

# PART 4 — Reporting

At the end of every session, state:

1. Which task, and whether it is complete or in progress.
2. Files created or changed.
3. **Commands run and their actual output** — especially build, test and
   migration results. Do not paraphrase a test suite you did not run.
4. Anything discovered that contradicts this document, with evidence.
5. What is blocked, and on what.
6. Confirmation that Part 0 of this file was updated to match reality.

**Do not report a task complete without running its verification block.** The
single most expensive failure mode in this project has been a summary that was
more optimistic than the code.
