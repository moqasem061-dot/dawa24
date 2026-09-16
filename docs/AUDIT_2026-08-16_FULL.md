# Dawa24 Store — Full System Audit

**Date:** 2026-08-16 · **Commit:** `a12938d` · **Method:** every figure produced by a command against the live Elest.io database. Nothing here is taken from a previous summary.

---

# PART 0 — Verdict

The backend is close to done. The **security model had a hole big enough to matter**, the
**file-upload capability does not exist at all**, and the **frontend is roughly half-finished** —
wired to real data, but missing most of its loading and error states.

Gemini's latest round was good work: commerce repository coverage went 37.9% → 81.4%, promo
55.4% → 80.0%, the UI handlers went from zero service calls to 25, and the three invariant
tests requested in the last plan were written and pass.

| Area | State |
|---|---|
| Build / vet / unit suite | ✅ Green |
| Repository suites | ✅ **12 of 12 pass**, 11 above 60% |
| Migrations | ✅ 34 of 34 applied |
| API surface | ✅ 165 routes, 12 modules |
| Authorization | ⚠️ **Was broken — fixed in this audit** |
| Frontend data wiring | ⚠️ 18 of 27 pages wired |
| Frontend states | ❌ Loading 0/24, error 1/24 |
| File uploads | ❌ **No upload path exists** |
| ETL | ❌ Skeleton, 17 statements |
| Object storage | ❌ Never constructed |
| CI | ✅ Runs postgres + migrations; ⚠️ no skip detection |

---

# PART 1 — What I fixed during this audit

These were live defects. All are committed, tested, and verified green.

## 1.1 Three admin route groups had no permission check — **critical**

`ingest`, `org` and `promo` registered their `/api/v1/admin/` routes with no middleware.
The other nine modules all wrap theirs in `RequirePermission`. The handlers behind them run
under `database.AsSystem`, which deliberately bypasses tenant scoping.

Reachable by **any authenticated user**:

```
POST /api/v1/admin/org/{id}/approve      approve your own organization
POST /api/v1/admin/org/{id}/suspend      suspend a competitor
PUT  /api/v1/admin/org/{id}              rewrite any organization
POST /api/v1/admin/promo/ads/{id}/approve
POST /api/v1/admin/promo/sponsorships/{id}/review
GET  /api/v1/admin/ingest/sessions       every tenant's import history
```

Fixed: all three now wrap in `RequirePermission("<module>.admin", h.log)`.

**Why no one noticed:** an unguarded route behaves identically to a guarded one until
someone who should not reach it does. Counting admin routes cannot see this — the routes
existed and were counted as delivered.

## 1.2 Nine org handlers accepted any tenant's id — **critical**

Handlers read `{id}` from the URL and passed it to a repository running `AsSystem`, with no
comparison against the caller. Authentication was in place; authorization was absent.

```
PUT    /api/v1/org/organizations/{id}                  legal name, credit limit, payment terms
DELETE /api/v1/org/organizations/{id}                  suspend any organization
POST   /api/v1/org/organizations/{id}/status
POST   /api/v1/org/organizations/{id}/branches
PUT    /api/v1/org/organizations/{id}/branches/{bid}
DELETE /api/v1/org/organizations/{id}/branches/{bid}
POST   /api/v1/org/organizations/{id}/members
PUT    /api/v1/org/organizations/{id}/members/{uid}
DELETE /api/v1/org/organizations/{id}/members/{uid}
```

Any logged-in user could raise their own credit limit by changing one number in the URL.

Fixed: added `authctx.SameOrgOrForbidden` — the tenant counterpart to the existing
`SameUserOrForbidden` — and applied it to all nine. The **read** paths are deliberately left
open: reviewing and following another organization is what a marketplace is for.

## 1.3 `DeletePaymentMethod` deleted by id alone

`DELETE FROM billing.user_payment_methods WHERE id = $1` on a table with **no row-level
security**. It has no service method or route yet, so it was not exploitable — but the
signature made an IDOR inevitable for whoever wired it up. `userID` is now part of the
predicate rather than an argument a caller can omit, and a row owned by someone else reports
as absent rather than forbidden, so the endpoint cannot be used to probe which ids exist.

## 1.4 The UI rendered raw error text to the browser

`renderError` passed `err.Error()` into the page. For an `apperr` that leaks the internal
form; for anything else it is the raw driver text, naming tables, columns and constraints —
`violates foreign key constraint "user_addresses_city_id_fkey"` in a user's browser.

`apperr.Msg` is documented user-safe and `LocalizedMsg(lang)` gives the Arabic wording by
code; the UI was bypassing both. Fixed, and a failed page load now returns its real status
code instead of `200` — an error page returning 200 is invisible to uptime monitoring.

## 1.5 Regression tests

`test/admin_guard_test.go` locks both classes in: every `/api/v1/admin/` route must sit in a
group calling `RequirePermission`, and every org mutation taking an org id must call
`SameOrgOrForbidden`. Both pass, and the second verifies it found all nine handlers, so it
cannot pass vacuously.

**The point that generalises:** every one of these compiled, passed `go vet`, and passed the
full unit suite. Route counts and coverage percentages cannot see missing authorization.

---

# PART 2 — What is remaining

Ordered by what would hurt most in production.

## 🔴 R1 — There is no file upload path at all

`internal/platform/storage/storage.go` defines `PresignPut`. **Nothing calls it.** The
storage client is **never constructed** — zero references in `cmd/server/main.go` or
`routes.go`. `STORAGE_*` variables exist in `.env.example` and are read by nothing.

`ingest.RegisterUpload` accepts **JSON metadata about a file**, not a file:

```go
func (h *Handler) RegisterUpload(w http.ResponseWriter, r *http.Request) {
	var f ingest.FileUpload
	if err := httpx.DecodeJSON(w, r, &f); err != nil {
```

There is no `FormFile`, no `ParseMultipartForm`, no presigned flow anywhere in the codebase.
So the ingest wizard registers metadata about a file that **has no way to arrive**, then runs
column detection against it.

Bulk catalogue import is a core vendor feature on a B2B pharmaceutical marketplace. As built
it cannot work end to end. This is the single largest functional gap.

**Needed:** construct the storage client at startup, wire `STORAGE_*` config, add a presign
endpoint, have the ingest UI PUT directly to the presigned URL, and have `RegisterUpload`
record the resulting object key. Never proxy the bytes — a 200 MB spreadsheet through a
handler holds a goroutine for minutes.

## 🔴 R2 — Frontend states are largely missing

Across 24 page templates:

| State | Present |
|---|---|
| Empty | 12 of 24 |
| Error | **1 of 24** |
| Loading / skeleton | **0 of 24** |

`skeleton.templ` and `emptystate.templ` are written and largely unused. `renderError` now
handles the error path centrally, which covers a lot of R2's error column, but no page shows
a loading state and half show nothing when there is no data.

Nine handlers remain unwired to data. Most are legitimately static (privacy, terms, and the
login/forgot/reset/onboarding forms, which need no data on GET). **Two genuinely need it:**
`AdminDashboardPage` and `AdminSettingsPage` render nothing real.

**Also unverified, because it cannot be checked by grep:** RTL with real Arabic product
names, which change line height and truncation; the 40 KB JS budget; and the 1.2s FCP target.
Someone has to open the pages.

## 🟠 R3 — The ETL is still a skeleton

1,263 lines, 2 connections, **17 SQL statements** for 141 legacy tables. Structure is real,
volume is not. The six stages (extract → validate → transform → load → verify → reconcile)
are described in `FINISH_THE_SYSTEM.md` Phase C.

**Two questions block correctness and neither can be guessed:**

1. **Which legacy order system is authoritative** — `orders`+`order_items`, or
   `main_orders`+`adv_orders`? The target schema assumed the latter. Wrong choice migrates
   the wrong data and is discovered after cutover.
2. **How do legacy `company` and `agency` map onto supplier / pharmacy / chain_pharmacy?**
   Migration 034 accepts all five so nothing is blocked, but the ETL cannot translate them
   until you decide. **This one is yours to answer.**

## 🟠 R4 — RLS is inert, and 27 queries depend on it being otherwise

The app connects as `postgres` — your decision, and it is respected. The consequence is worth
restating precisely because this audit found what it costs:

**53 RLS policies do nothing.** 27 mutations key on `id` with no owner predicate; they are
safe *only* if RLS applies. It does not. The authorization fixes in Part 1 closed the
reachable ones, but the underlying dependency remains across the codebase.

Additionally, **4 user-owned tables have no RLS policy at all** and so would remain unguarded
even after switching: `billing.user_payment_methods`, `commerce.wishlists`,
`catalog.product_alerts`, `promo.ad_clicks`. The other 11 unprotected tables are genuinely
global reference data (`plans`, `brands`, `categories`, `job_categories`, `ad_plans`) and are
correct as they are.

Switching to `dawa24_app` is a config change; the provisioning is already done
(`cmd/dbcheck -provision`).

## 🟡 R5 — Coverage gaps

- `notifications/postgres` at **54.9%** — the only repository suite below 60%.
- `identity` service at **50.9%** — the lowest service layer, and it guards every other
  module. Registration validation, lockout, reset-token expiry, MFA lifecycle, session
  revocation.

Everything else is 60–84%.

## 🟡 R6 — CI does not detect a skipped suite

Better than expected: CI already runs a postgres service, applies migrations, and runs
`go test -race` with `DATABASE_URL` set. What it lacks is the check that matters here — **a
suite reporting `ok ... 0.0% of statements` still passes the build.** Six suites once
reported exactly that while executing nothing. Add coverage thresholds and fail on 0.0%.

## 🟡 R7 — Operational

1. **Rotate every credential** that has appeared in a transcript: PostgreSQL, Redis, the
   Gateway virtual key, the temporary `dawa24_app` password. Treat all four as public.
2. **MinIO service and bucket** — a prerequisite for R1, not an afterthought.
3. **Backups** — PITR plus a restore actually tested into a scratch database.
4. **Metrics** — RED per module, River queue depth, gateway latency and fallback rate, pool
   saturation.

---

# PART 3 — Answering "many endpoints are missing"

Measured: **165 API routes** across 12 modules, plus 27 UI page routes.

| Module | Routes | | Module | Routes |
|---|---|---|---|---|
| catalog | 25 | | billing | 12 |
| org | 23 | | ingest | 11 |
| commerce | 21 | | platform_admin | 9 |
| identity | 18 | | workflow | 5 |
| promo | 16 | | notifications | 5 |
| inventory | 15 | | hr | 5 |

The breadth is genuinely there. What is missing is **not mostly endpoints** — it is:

- the **upload path** behind the ingest endpoints (R1),
- **states and polish** behind the UI routes (R2),
- and, until this audit, the **authorization** behind the admin routes (Part 1).

One concrete missing endpoint found: users can add and list payment methods but **cannot
delete one** — `DeletePaymentMethod` exists in the repository with no service method and no
route.

If specific endpoints you expect are absent, name them and I will check against the legacy
inventory — `docs/rebuild/SCHEMA_INVENTORY.md` covers all 141 legacy tables, which is the
right basis for a completeness diff.

---

# PART 4 — Suggested order

1. **R1 — the upload path.** Largest functional gap; blocks a core vendor workflow.
2. **R2 — frontend states**, and wire the two admin pages.
3. **R6 — CI skip detection.** Cheap, and protects everything else.
4. **R5 — the two coverage gaps.**
5. **R3 — the ETL**, once you have answered the two questions in it.
6. **R4 / R7 — the `dawa24_app` switch, credential rotation, storage, backups, metrics.**

---

# PART 5 — Failure patterns, updated

New entries from this audit are marked ★.

| Pattern | What it cost |
|---|---|
| ★ Authentication mistaken for authorization | Any user could approve, suspend or rewrite any organization |
| ★ An admin path assumed to be admin-guarded | Three modules shipped `/admin/` routes open to everyone |
| ★ Raw error text rendered to the user | Table, column and constraint names in the browser |
| ★ A capability defined but never constructed | `PresignPut` exists; no upload path does |
| Nullable column into a non-pointer | 51 columns; two live endpoints down |
| Constraint disagreeing with the domain | Pharmacies could not register |
| Identity from the request | Anyone could withdraw from any wallet |
| Casting a parameter in SQL | Every tenant transaction failed |
| Type assertion through a wrapper | All SSE was dead |
| A test that skips | Six suites reported `ok` at 0% |
| A gate that checks nothing | The ETL verification compared two numbers handed to it |
| Counting scaffolding as progress | 27 page routes, none of which read data |
| Reporting without measuring | Claims in both directions that did not survive a check |
