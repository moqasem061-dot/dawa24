# Dawa24 Store — Finish The System

**Measured:** 2026-08-16, against the live Elest.io database, at 34 migrations applied.
**Supersedes:** `REMAINING_WORK.md`, `PLAN_REPOSITORY_TESTS.md`, `REVIEW_AND_PLAN_2026-08-16b.md`.

This is the whole remaining programme, not one round of it. Work top to bottom.
Do not stop at the end of a task to ask what is next — the next task is the next
heading. Stop only when something here is genuinely blocked, and say what and why.

---

# PART 0 — Where the system actually is

Every number below was produced by a command, not by reading a summary.

| | Value | State |
|---|---|---|
| Migrations | **34 of 34 applied** | Current |
| Tables | 98 | Complete for scope |
| API routes | 196 | Complete |
| Admin routes | 47 across 64 paths | Complete |
| `go build` / `go vet` / unit suite | **all pass** | Green |
| Repository suites | **12 of 12 pass** | Green — see the bar below |
| Service coverage | 50.9% – 84.2% | Acceptable; two below 60% |
| `http/` coverage | 53% – 68% | Target met |
| templ files / page routes | 42 / 27 | **Shells — no data. See Phase B.** |
| ETL | 1,260 lines, 51.4% | Skeleton only |

## Repository suites, all green

```
billing      64.6%    identity     77.3%    org             78.2%
catalog      81.7%    ingest       77.5%    platform_admin  80.2%
commerce     37.9%    inventory    76.5%    promo           55.4%
hr           81.6%    notifications 54.9%   workflow        81.8%
```

Three are below the 60% bar and are Task A1: **commerce 37.9%**, **notifications
54.9%**, **promo 55.4%**.

## What was fixed getting here, and what it teaches

Ten defects were closed in this session. Eight were in application code, not tests.
They matter because the same shapes will recur:

1. **51 nullable text columns were selected into non-pointer Go strings.** pgx
   cannot scan NULL into a `string`, so one row with the field empty fails the
   whole query. This had already taken down two live endpoints
   (`org.organizations.tax_number`, `catalog.products.sku`) and 49 more were
   waiting. Migration 033 closed the class. `cmd/dbcheck -nullscan` now reports
   any new one and separates the columns that can be back-filled from those under
   a unique index, where `''` would collide but NULL does not.
2. **`org.organizations.type` rejected `pharmacy` and `chain_pharmacy`.** The
   CHECK constraint came from the legacy enum (`supplier`/`company`/`agency`)
   while the Go domain defines supplier/pharmacy/chain_pharmacy. Registering a
   pharmacy — the primary customer type on this marketplace — failed. Migration
   034 permits both sets. **See the open decision in Phase C.**
3. **`organizations.status` had no `suspended`**, so suspending an organisation
   was impossible although the domain and admin surface both offer it. Same
   migration.
4. **Optional-but-NOT-NULL columns 500'd on create and update** —
   `organizations.trade_name`, `users.name`, `branches.name`. An empty
   `i18n.Text` marshals to NULL, which violates the constraint rather than taking
   the column default. Guarded with `COALESCE` on both paths.
5. **`branches.code` is uniquely indexed**, so it could not be back-filled to
   `''` — the second code-less branch would collide. Fixed in Go with
   `COALESCE(code,'')` on read and `NULLIF($n,'')` on write.
6. **Two writers inverted `''` into NULL** on `error_message` columns, which is
   precisely what the read path then could not scan back.

The rest were test defects: a delete against `commerce.order_ratings` (a table
that never existed — migration 028 put the columns on `commerce.orders`), a
`title->>'en'` against a `TEXT` column, an `"open"` issue status that no
application code uses, MFA enabled without the confirmation its constraint
requires, and fixtures omitting NOT NULL columns.

**The lesson that generalises:** every one of these compiled, and most passed
`go vet` and the unit suite. They were only found by executing real SQL against a
real database. That is what the repository suites are for, and why Phase D puts a
database in CI.

## Two standing constraints — read before writing code

**The app connects as `postgres`.** The owner decided this. Consequence: all 53
RLS policies are **inert**, and tenant isolation rests entirely on `WHERE` clauses
in Go. **Never write a query whose only tenant guard is RLS.** Every tenant-scoped
query needs its own `organization_id` predicate.

**A green suite is not evidence.** `ok ... 0.0% of statements` means the tests
skipped and ran nothing — six suites reported exactly that earlier in this
project. Read the coverage number, not the `ok`.

---

# PART 1 — Rules that apply to every task below

These are not style preferences. Each one is here because violating it already
cost this project a bug.

1. **Measure before reporting.** Every claim in a status report must have a
   command behind it. Both the previous agent and the reviewer have published
   figures that did not survive a second look, in both directions.
2. **Never assert on an id you did not read back.** Repositories let PostgreSQL
   assign ids and scan them into the struct. Pinning `88100` and looking it up
   finds nothing, and every later subtest fails on a cascading foreign key.
3. **Identity comes from `authctx`, never from the request.** Not a query
   parameter, not a body field, not a header. Four endpoints once read the user
   from the query string; anyone could withdraw from any wallet.
4. **Money is `money.Amount`** — int64 minor units. Never `float64`, never a
   bare int. Assert money round-trips exactly to the cent.
5. **Bilingual text is `i18n.Text`** — a JSONB `{"ar","en"}` column. Arabic is
   the primary language, not a translation of English.
6. **Tenant scope is `SET LOCAL app.current_org_id` inside the transaction.**
   Never session-level: pooled connections would leak it across tenants.
7. **Status transitions are compare-and-swap.** Read-then-write loses the race
   and silently double-applies.
8. **A nullable column is scanned into a pointer, or made NOT NULL DEFAULT ''.**
   Run `cmd/dbcheck -nullscan` after any migration that adds a text column.
9. **Cross-tenant reads use `database.AsSystem(ctx)`** with a comment saying why.
   It is deliberately greppable.
10. **Every admin mutation writes `platform.audit_log` in the same transaction**
    as the change it records. A separate transaction can disagree with it.
11. **No provider name outside `internal/platform/gateway/`.** Enforced by
    `make check-provider-isolation`. The Store stays provider-agnostic.
12. **Do not delete a migration.** 029 was deleted once; a fresh database would
    have shipped with no RLS at all and `migrate` would have reported success.

**Gate to run before claiming any task complete:**

```bash
go build ./... && go vet ./... && go test -count=1 ./... && make check-provider-isolation
```

Plus, for anything touching SQL:

```bash
DATABASE_URL="postgres://postgres:<pw>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require" \
  go test -count=1 -cover ./internal/modules/.../postgres/
```

---

# PHASE A — Close the test gaps

Three repository suites are below the 60% bar. Raising them is mechanical and
finds real SQL bugs, which is why it comes first.

## A1 — `commerce/postgres` 37.9% → 60%+

The lowest in the system and the module with the most business logic. Find what
is uncovered before writing anything:

```bash
DATABASE_URL="..." go test -count=1 -coverprofile=/tmp/c.out ./internal/modules/commerce/postgres/
go tool cover -func=/tmp/c.out | grep -E "\s0.0%$"
```

Cover, at minimum: cart mutation including `SetCartItemQuantity`; order creation
with lines across multiple vendors; `order_lines` totals summing to
`orders.final_price` **to the cent**; the full status history chain; shipment
compare-and-swap including the losing writer getting a conflict; quote requests
through their whole lifecycle; and a row with every optional field empty scanning
cleanly.

## A2 — `notifications/postgres` 54.9% → 60%+ and `promo/postgres` 55.4% → 60%+

Same method. For `promo`, note that `offer_packages.name` is JSONB (`->>` is
correct) while `ads.title` is `TEXT` (it is not) — the two are genuinely
different and the domain type matches each.

## A3 — Raise `identity` service coverage from 50.9%

The lowest service-layer figure and the one guarding every other module. Cover
registration validation, login including lockout after repeated failure, password
reset token expiry, MFA enable/confirm/disable including the
`mfa_enabled_requires_secret` rule, and session revocation.

## A4 — Lock in the invariants that keep breaking

Add these as permanent tests, not one-off checks:

- **`test/schema_consistency_test.go`** already parses migrations and verifies
  every column named in SELECT/INSERT/UPDATE exists. Extend it to **CHECK
  constraint values**: any string literal a Go const assigns to a constrained
  column must be permitted by that column's constraint. This exact class produced
  defects 2, 3 and the `"open"` issue status.
- **A nullscan test.** Wrap `cmd/dbcheck -nullscan` in a Go test that fails when
  the count exceeds the known-good exclusions
  (`commerce.order_status_history.from_status`). Otherwise column 52 arrives
  silently.
- **A money round-trip test per module** — write `money.FromMinor(n)`, read it
  back, assert equality for a positive, a negative and a zero amount.

**Phase A done when:** all 12 repository suites ≥60%, identity service ≥60%, and
the three invariant tests exist and pass.

---

# PHASE B — The frontend

**This is the largest remaining piece of work in the system, and it is larger
than any previous plan admitted.**

## B0 — What is actually there

42 templ files and 27 page routes exist. The components are real: `skeleton`,
`emptystate`, `toast`, `datatable`, `pagination`, `modal`, `drawer`, `filedropzone`,
`moneydisplay`, `badges`, `avatar`, `datepicker`, `tabs`, `forms`, `buttons`.

**But every page handler is a one-line render with no data.** `internal/ui/handlers.go`
is 203 lines for 27 routes and contains **zero** calls to any service or
repository. It takes `catSvc`, `orgSvc` and `ingSvc` in its constructor and never
uses any of them. Typical handler, verbatim:

```go
func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerCatalog().Render(r.Context(), w)
}
```

Three separate problems in four lines: the template receives no data, the render
error is discarded into `_`, and the injected services are ignored. **The
frontend is a static mockup.** Counting routes and templ files measured the
scaffolding, not the product.

## B1 — The handler contract

Rewrite every page handler to this shape. Do all 27; do not leave a mixture.

```go
func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Identity comes from the authenticated context, never the request. A page
	// that reads its tenant from a query parameter is a cross-tenant read.
	actor, err := authctx.From(ctx)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	products, err := h.catSvc.ListProducts(ctx, catalog.ListFilter{
		OrganizationID: actor.OrganizationID,
		Query:          r.URL.Query().Get("q"),
		Limit:          pageLimit(r),
		Offset:         pageOffset(r),
	})
	if err != nil {
		// The error state is a first-class render, not a 500 page. The user keeps
		// their navigation and can retry.
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Render errors are logged, never discarded. A half-written response with a
	// 200 already sent is exactly the failure that is impossible to diagnose later.
	if err := pages.CustomerCatalog(products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render catalog", "error", err)
	}
}
```

Requirements, each of which currently fails:

- **Data comes from the service layer.** Add the services the handler needs to
  `UIHandler` — today only three are wired and none are called.
- **Identity from `authctx`.** Never a query parameter.
- **Render errors logged, never `_ =`.**
- **Errors render the error state**, not a bare 500.
- **HTMX partial requests** (`HX-Request: true`) render the fragment alone,
  without the layout. Otherwise every partial ships a whole page.

## B2 — Four states on every screen

This is the difference between a demo and a product, and retrofitting it means
touching every screen twice. The components already exist — `skeleton.templ` and
`emptystate.templ` are written and largely unused.

For each of the 27 routes:

| State | Requirement |
|---|---|
| **Loading** | `skeleton.templ` at the shape of the real content, not a spinner |
| **Empty** | `emptystate.templ` with an Arabic message and the action that resolves it |
| **Error** | The message with a retry affordance; never a stack trace, never a blank page |
| **Partial** | The rows that loaded render; the failed region shows its own error |

## B3 — The 27 screens, with what each must actually do

Group these however is convenient, but finish all of them.

**Public and auth (8):** `/`, `/privacy`, `/terms`, `/auth/login`,
`/auth/register`, `/auth/forgot`, `/auth/reset`, `/onboarding`.
Login posts to the real endpoint, shows field-level validation in Arabic, and
survives a wrong password without losing the entered email. Registration creates
a real organisation — **and this is the path that 500'd until migration 034**, so
test it with `pharmacy` and `chain_pharmacy`, not only `supplier`. Onboarding
persists progress across a reload.

**Customer (7):** `/catalog`, `/catalog/{id}`, `/cart`, `/checkout`, `/orders`,
`/orders/{id}`, `/notifications`.
Catalog paginates, filters and searches server-side against real products. Product
detail shows real variants and real customer pricing. Cart mutates through the
real service and reflects quantity changes without a full reload. Checkout shows
a real total that **equals the sum of its lines to the cent** and refuses to
submit an empty cart. Order detail renders the real status history. Notifications
mark as read.

**Vendor (7):** `/vendor/products`, `/vendor/products/new`,
`/vendor/products/{id}`, `/vendor/inventory`, `/vendor/transfers`,
`/vendor/ingest`, `/vendor/orders`.
The product editor creates and updates real products with Arabic and English
names, validates that a discount cannot exceed price, and reports the failure on
the field. Inventory shows real stock. **Transfers exercise the full two-phase
flow — dispatch, in transit, receive — and a losing concurrent receive shows the
conflict rather than silently succeeding.** Ingest uploads a real spreadsheet,
streams progress over SSE, and shows the per-row defect report.

**Admin (4):** `/admin/dashboard`, `/admin/users`, `/admin/approvals`,
`/admin/settings`.
Real cross-tenant data via `database.AsSystem(ctx)`. Approvals actually approve
and the audit row lands in the same transaction. A non-admin gets 403 on every
one.

**One route to fix while you are here:** `/orders/{id}` is registered to
`h.CustomerOrdersPage`, the same handler as the list. Order detail needs its own
handler.

## B4 — Arabic and RTL, verified rather than assumed

- **Every string in Arabic first.** `i18n.Text` renders `ar` unless the user
  chose otherwise. No English fallback visible in the Arabic UI.
- **`dir="rtl"` on the document**, and every layout mirrored — not only text
  alignment but icon direction, table column order, and pagination controls.
- **Test with real Arabic product names**, which change line height and
  truncation behaviour. Not lorem ipsum. Long names must truncate without
  breaking the row.
- **Numbers and money render correctly in an RTL context** — a price is not
  mirrored, and `moneydisplay.templ` must be checked against that.

## B5 — Streaming and uploads

- **SSE sets `X-Accel-Buffering: no`.** Exactly one occurrence exists in the
  codebase; every SSE endpoint needs it. Without it the reverse proxy buffers
  every event until the stream closes, so import progress looks frozen in
  production while working perfectly on localhost. The `Flush()` on
  `statusWriter` is already fixed — do not regress it.
- **Uploads are presigned** via `storage.PresignPut`, never proxied through the
  app. A 200 MB vendor spreadsheet through a handler holds a goroutine for
  minutes. Three call sites exist; the ingest UI must use one.

## B6 — Budget and proof

Under **40 KB of JavaScript**, first contentful paint under **1.2s on 3G**.
templ + HTMX + Alpine is the whole stack (ADR 0001) — do not add a framework.

**Phase B done when** a vendor can register → be approved → create a product →
import a spreadsheet and watch its progress → receive an order → dispatch it, and
a customer can register → browse → add to cart → check out → rate the order,
**entirely through the UI**, with every intermediate state rendering and every
page reading real data. Walk it end to end and say you did.

---

# PHASE C — The ETL

1,260 lines with real connections. The structure is genuine; the volume is not.

## C0 — Two questions to answer before writing transform code

Both change what gets migrated, and both are discovered after cutover if guessed
wrong.

> **Which legacy order system is authoritative:** `orders`+`order_items`, or
> `main_orders`+`adv_orders`? The target schema assumed the latter. Verify with
> row counts and the most recent `created_at` in both, and report the evidence.

> **How do legacy `company` and `agency` map onto supplier / pharmacy /
> chain_pharmacy?** Migration 034 accepts all five so nothing blocks, but the
> mapping is a business decision. **Ask the owner. Do not guess.** Report the row
> count for each legacy value so the question is answerable.

## C1 — The six stages

Per `../docs/rebuild/REBUILD_MASTER_PLAN.md` §11:

1. **Extract** every legacy table to newline-delimited JSON. Chunked and
   resumable — a run that dies at table 90 of 141 must not start over.
2. **Validate, then stop.** Orphan sweep across the **36 `*_id` columns that have
   no foreign key**, invalid JSON, out-of-range enums, duplicate emails, negative
   money. **Emit a defect report and halt. Never transform dirty data.**
3. **Transform** — `users` decomposition, order unification, entitlement
   consolidation, blobs to object storage, and **explicit UTC → `TIMESTAMPTZ`**.
   A naive cast shifts every timestamp by 2–3 hours; every order date in the
   system would be wrong by a working day's margin.
4. **Load** — `COPY`, foreign keys deferred, indexes created after.
5. **Verify — the gate computes both sides itself.** It previously took the
   counts as parameters and compared them, which is why it reported success
   having read nothing. It must query source and target independently and
   compare: row counts, checksums, money sums to the cent, zero FK orphans, 100%
   JSON parse, timestamp min/max, and `SUM(lines.line_total) = order.final_price`.
6. **Reconcile** — per-table go/no-go, printed.

**Preserve legacy primary keys.** User 4417 stays user 4417.

**Phase C done when** a full dry run against a scratch database completes, the
verification gate passes on its own computed figures, and the reconciliation
report is green per table.

---

# PHASE D — Production hardening

## D1 — CI with a real database

**Highest priority in this phase.** The 12 repository suites are worthless if CI
runs them without `DATABASE_URL` and they skip — which is exactly what happened
before. Add a `postgres` service to the workflow, run migrations, run the suites,
and **fail the build when any suite reports 0.0% coverage**. A skip must be a red
build, not a green one.

Also in CI: `go vet`, `make check-provider-isolation`, the schema consistency
test, and the nullscan test from A4.

## D2 — Credentials

**Rotate everything that has appeared in a transcript**: the PostgreSQL password,
the Redis password, the Gateway virtual key, and the temporary `dawa24_app`
password. All four are in chat logs and must be considered public.

## D3 — The `dawa24_app` decision

The app runs as `postgres`, so **all 53 RLS policies are inert**. The provisioning
work is already done (`cmd/dbcheck -provision`) and switching is a config change.
This is the owner's call, not the agent's — but until it is made, the second line
of tenant defence does not exist. State this plainly in the next report rather
than letting it disappear.

## D4 — Object storage

MinIO service, bucket, `STORAGE_*` variables. Required before `APP_ENV=prod` will
boot and before presigned uploads work at all — Phase B5 depends on it.

## D5 — Backups

PITR, plus a **restore actually tested into a scratch database**. An untested
backup is not a backup.

## D6 — Metrics

RED per module, River queue depth, gateway latency and fallback rate, connection
pool saturation.

---

# PART 2 — Failure patterns seen in this codebase

Every one was real here. Check new code against the list.

| Pattern | What it cost |
|---|---|
| Nullable column into a non-pointer | 51 columns; two live endpoints down before the class was closed |
| Constraint disagreeing with the domain | Pharmacies could not register at all |
| Optional field that is NOT NULL | Create and update both 500'd on an empty trade name |
| Back-filling a unique column to `''` | Would have collided on the second row; NULLs do not |
| Identity from the request | Anyone could withdraw from any wallet |
| Trusting a header | `X-Dawa-Org-ID` honoured without a membership check |
| Capturing a dependency at construction | Repositories held a nil pool forever |
| Casting a parameter in SQL | `$1::text` with an int64 broke every tenant transaction |
| Type assertion through a wrapper | `w.(http.Flusher)` failed behind middleware; all SSE was dead |
| Column that does not exist | 32 of them; compiles, fails at runtime |
| Hashing bytes instead of content | CRLF vs LF failed two deploys with no schema change |
| A test that skips | Six suites reported `ok` at 0% having executed nothing |
| A gate that checks nothing | The ETL verification compared two numbers handed to it |
| Counting scaffolding as progress | 27 page routes, none of which read any data |
| Reporting without measuring | Claims in both directions that did not survive a check |

---

# PART 3 — Reporting

At the end of every working session:

1. Which tasks are complete, which are in progress.
2. Files created or changed.
3. **Commands run and their real output** — coverage numbers, not `ok`.
4. Anything contradicting this document, with the evidence.
5. What is blocked, and on what. Both Phase C questions go here until answered.
6. Confirmation that Part 0 of this file was updated to match reality.

**Measure, then write the summary.** Not the other way round.
