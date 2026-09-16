# PHASE 10 — Completeness Verification & Production Readiness

**Depends on:** Phases 0–9 complete.
**This phase is mandatory.** Its purpose is to catch what the previous nine
phases missed, systematically rather than by memory.

**Rule for this phase: you are looking for your own gaps.** A verification pass
that finds nothing has failed. If a section below produces zero findings, say so
explicitly and show the command output that proves it.

---

## TASK 10.1 — Route-by-route diff against Laravel

### 10.1.1 Build both inventories

```bash
# LARAVEL — every route, with its prefix and permission
cd "F:/Dawa 24/Laravel"
for f in web admin vendor customer api; do
  echo "### routes/$f.php"
  grep -nE "Route::(get|post|put|patch|delete)\(" routes/$f.php
done > /tmp/laravel_routes.txt

# GO — every registered route, from the real router (not a hand list)
cd "F:/Dawa 24/dawa24-store"
# write a small test or cmd that chi.Walks newRouter() and prints method+pattern
go test ./test/... -run TestDumpRoutes -v > /tmp/go_routes.txt
```

If `TestDumpRoutes` does not exist, **write it** — Phase 10 needs it, and so
does the guard test from Phase 0.

### 10.1.2 Produce the diff table

Create `docs/PLAN_V5/VERIFICATION_ROUTES.md` with one row per Laravel route:

| Laravel route | Laravel permission | Go route | Go permission | Status | Evidence |
|---|---|---|---|---|---|
| `/admin/products` | `products_view` | `/admin/products` | `catalog.product.view` | ✅ | |
| … | | | | | |

Status values: `✅ ported` · `🔀 renamed` · `🔗 merged into X` ·
`❌ missing` · `⛔ deliberately dropped (reason)`.

**Every Laravel route must appear with a non-empty status.** A blank row is an
incomplete verification.

Any `❌ missing` row is a Phase 10 finding — fix it or record it in
`OPEN_QUESTIONS.md` with an explicit product decision.

---

## TASK 10.2 — Screen-by-screen diff

353 Laravel Livewire components vs the Go templ pages.

```bash
cd "F:/Dawa 24/Laravel"
find app/Livewire -name "*.php" | sed 's|app/Livewire/||;s|\.php||' | sort > /tmp/laravel_screens.txt
cd "F:/Dawa 24/dawa24-store"
ls internal/ui/pages/*.templ | sed 's|.*/||;s|\.templ||' | sort > /tmp/go_screens.txt
```

Create `docs/PLAN_V5/VERIFICATION_SCREENS.md` mapping every Laravel component.

**Exclusions that are legitimate** (mark them, do not silently skip):
- `old_*`, `OLD*`, `POLD*`, `*  2` duplicates — Laravel's dead code
- `Partials/*` that map to Go components rather than pages
- `Test`, `test/*`

For each mapped screen, verify the **contents** match, not just that a page
exists:
- [ ] same columns / fields
- [ ] same filters
- [ ] same sort options
- [ ] same page size
- [ ] same actions/buttons
- [ ] same Arabic labels
- [ ] same empty-state copy

Sample at least 20% of screens for content parity, prioritising the ones built
earliest (they had the least-mature conventions).

---

## TASK 10.3 — Table-by-table schema diff

```bash
cd "F:/Dawa 24"
grep -oE "CREATE TABLE \`[a-z_0-9]+\`" u924222867_Testv5.sql | sed "s/CREATE TABLE //;s/\`//g" | sort > /tmp/laravel_tables.txt
cd dawa24-store
# from the live database, not from migrations — migrations can drift
psql "$DATABASE_URL" -Atc "SELECT schemaname||'.'||tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema') ORDER BY 1" > /tmp/go_tables.txt
```

Create `docs/PLAN_V5/VERIFICATION_SCHEMA.md`:

| Laravel table | Go table | Status | Columns verified | Notes |
|---|---|---|---|---|

**Column-level verification is required for every ported table**, not just
table-level. For each, list any Laravel column with no Go counterpart and state
why.

### The two lists that must be empty at the end

**(a) Laravel business tables with no Go counterpart.** Framework tables
(`cache`, `jobs`, `sessions`, `telescope_*`, `migrations`,
`password_reset_tokens`, `personal_access_tokens`, `failed_jobs`, `job_batches`,
`cache_locks`) and read-model views (`all_products`, `basic_products`,
`products_view`, `organizations_overview`, `product_search_index`) are excluded.

**(b) Go tables with no route touching them.** The audit found 21. Re-run:

```bash
# for each table, does any Go file reference it?
psql "$DATABASE_URL" -Atc "SELECT schemaname||'.'||tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')" \
| while read t; do
    n=$(grep -rl "$t" internal/ --include=*.go | wc -l)
    [ "$n" -eq 0 ] && echo "DEAD TABLE: $t"
  done
```

Every dead table is either a missing feature or a table that should not exist.
Resolve each one.

---

## TASK 10.4 — Permission matrix verification

The `/what-in` page publishes a 20-row permission matrix across Admin / Vendor /
Customer. Verify **each row** against the running system:

| Feature | Admin | Vendor | Customer |
|---|---|---|---|
| Advanced Discount & Price Comparison | ✓ | ✓ | ✓ |
| Multiple Supplier Upload & Column Mapping | ✓ | ✓ | ✓ |
| Supplier vs Supplier Comparison | ✓ | ✓ | ✓ |
| Vendor vs Market Discounts | ✓ | ✓ | ✓ |
| AI Auto-Recognition & Matching | ✓ | ✓ | ✓ |
| Auto-Archive Policy Application | ✓ | ✓ | ✓ |
| Purchase Priority Engine | ✓ | ✓ | ✓ |
| Saving Products Management | ✓ | ✓ | ✓ |
| Direct Purchase Requests | ✓ | ✓ | ✓ |
| Bulk Excel Chunk Imports | ✓ | ✓ | ✗ |
| Arabic PDF Invoice Generation | ✓ | ✓ | ✓ |
| Branch & Repository Management | ✓ | ✓ | ✗ |
| Weekly Coverage Schedule | ✓ | ✓ | ✗ |
| Institutional Work & Partnerships | ✓ | ✓ | ✗ |
| Electronic Wallet & Transactions | ✓ | ✓ | ✓ |
| Advertising Plans & Analytics | ✓ | ✓ | ✗ |
| Full Power Dynamic Keys | ✓ | ✗ | ✗ |
| Audit Tracking & Change Logs | ✓ | ✗ | ✗ |
| Trash & Data Recovery | ✓ | ✗ | ✗ |
| Admin Role & Permission Controls | ✓ | ✗ | ✗ |

For each ✓, sign in as that account type and confirm the feature is reachable
and works. For each ✗, confirm it returns **404** (per the audience policy), not
403 and not a working page.

Record the result of all 60 cells in `docs/PLAN_V5/VERIFICATION_PERMISSIONS.md`.
This is 60 manual or scripted checks; script them as an integration test if
possible so they run in CI thereafter.

---

## TASK 10.5 — Access control & security verification

### 10.5.1 Audience isolation
For each of `customer`, `vendor`, `staff`, `no-org user`, `signed out`, attempt
**every** registered route and assert the expected outcome. Script it; the
matrix is ~250 routes × 5 actors.

Expected: wrong audience → 404. Not signed in → 302 to login. Pending org → 302
to `/onboarding/pending`.

### 10.5.2 Permission isolation
For each admin route, a staff actor **without** its permission gets 404.
Already covered by the Phase 0 guard test — confirm it still walks the real
router and now covers every route added in Phases 5–9.

### 10.5.3 Tenant isolation
Every tenant-owned table needs a test proving a cross-tenant read returns zero
rows. This is already a CI gate — confirm **every new table from Phases 1–9** is
covered. List them and their test.

### 10.5.4 `AsSystem` audit
```bash
grep -rn "database.AsSystem" internal/ --include=*.go
```
Every call must have a comment justifying why cross-tenant access is correct
there. Any without one is a finding.

### 10.5.5 Secrets
```bash
grep -rniE "api_key|apikey|secret|password|token" internal/ui/pages/*.templ
```
No stored secret may be rendered to a browser. Masked display only.

### 10.5.6 Input validation
Every POST handler validates. Sample 20 and confirm: missing required field,
wrong type, out-of-range, oversized payload, and hostile input (SQL
metacharacters, script tags in Arabic text fields) each produce a clean
`apperr.Validation`, not a 500.

### 10.5.7 Rate limiting
Confirm on: login, 2FA challenge, password reset, guest order tracking, public
click endpoints (ads, offers, promotions), contact form, registration.

### 10.5.8 Open redirect
`?redirect=` on login, ad/promotion/offer click endpoints. Confirm only
same-origin paths are honoured.

### 10.5.9 File upload
Confirm: content-type allowlist, size cap, filename sanitisation, no execution
of uploaded content, storage outside the web root or behind signed URLs, and
SSRF protection on any URL-fetching import.

Record all of 10.5 in `docs/PLAN_V5/VERIFICATION_SECURITY.md`.

---

## TASK 10.6 — Frontend consistency verification

### 10.6.1 Dead ends
```bash
# dead template targets — must be 0
# unregistered handlers — must be 0
# (commands in 00_MASTER.md §0.8)
```

### 10.6.2 Every button
Click **every** button, link and form submit on **every** page, as each account
type. This is tedious and it is the point — the original complaint was "there
are many functionalities where clicking something does nothing".

Record per page: total interactive elements, how many were exercised, and any
that did nothing.

### 10.6.3 The five states
For each list screen and form, force and verify: loading, empty, error, success,
partial. The error state must be reachable — temporarily break a query if
necessary, and confirm the user sees an error, not silence.

### 10.6.4 Localization
- Every page in `ar` and in `en`
- No untranslated key, no raw `{"ar":…}` JSON leaking to the UI
- RTL correct in Arabic: layout direction, icon mirroring, number and date
  formatting, form label alignment
- Language switch preserves the current page and its state

### 10.6.5 Responsive
Every page at 375px, 768px, 1280px. Sidebar collapse, table behaviour, touch
targets ≥44px, no horizontal body scroll.

### 10.6.6 Sidebar parity
Compare the three rendered sidebars against `00_MASTER.md` §0.7.3 and the
Laravel layouts, entry by entry, in order.

Record all of 10.6 in `docs/PLAN_V5/VERIFICATION_FRONTEND.md`.

---

## TASK 10.7 — Business-logic parity spot-checks

For each of these, construct identical inputs in both systems and compare
outputs to the minor unit / exact value:

| Rule | Where |
|---|---|
| Offer discount application | `promo` — percent vs fixed, `min_order_amount` |
| Order totals | `commerce` — subtotal, discount, final price |
| Coverage visibility | `promo/postgres/visibility.go` — in-range, out-of-range, wrong day, inactive |
| Institutional filter | both modes, including the unrestricted-product asymmetry |
| Priority engine scoring | each flag alone and combined |
| Compare-engine best price/discount | ties, nulls, zero-price rows |
| Earnings / commission | admin and vendor must agree |
| Rating average | three criteria → scalar |
| Archive policy | at, below and above the file limit |
| Session cap | at, below and above the seat limit |

Record inputs, Laravel output, Go output, and match/mismatch in
`docs/PLAN_V5/VERIFICATION_LOGIC.md`. **Any mismatch is a defect**, even if the
Go behaviour looks more correct (rule 7 — port first, fix after cutover,
deliberately).

---

## TASK 10.8 — Performance & scalability

| Check | Threshold |
|---|---|
| Every page's queries | no N+1; confirm with query logging on a seeded dataset |
| Slowest 10 queries under realistic data | `EXPLAIN ANALYZE`; no sequential scan on a large table |
| Analytics screens (`offer_views`, `offer_clicks`, `audit_log`) | must not aggregate raw rows per request |
| Product/offer listings | paginated; no unbounded `SELECT` |
| Import of 50,000 rows | bounded memory (Phase 4 T2) |
| Concurrent imports | do not deadlock |
| `product_index` reindex | does not block reads |
| Cache | confirm what is cached and that it invalidates |

Seed a realistic dataset: ≥50 organizations, ≥500 branches, ≥50,000 products,
≥100,000 orders, ≥1,000,000 view/click rows. Record timings in
`docs/PLAN_V5/VERIFICATION_PERFORMANCE.md`.

---

## TASK 10.9 — Production readiness

- [ ] `make check` green
- [ ] Full test suite green with `-race`
- [ ] All migrations round-trip: `migratecheck -from 1 -roundtrip`
- [ ] Migrations are expand/contract safe — they run **before** the new image is
      promoted, and rollback is "redeploy the old image" (`AGENTS.md`)
- [ ] `TestNewRouterMountsWithoutPanicking` passes (the boot-panic guard)
- [ ] Health endpoints report accurately, including a degraded state
- [ ] Structured logging: every error carries actor, org, route, request id
- [ ] No secret in logs
- [ ] `.env.example` documents every required variable
- [ ] `DEPLOY.md` updated for anything new (storage, mailer, PDF fonts, `pg_trgm`)
- [ ] Worker deployment covers every new River job
- [ ] Graceful shutdown drains in-flight requests and jobs
- [ ] Backup/restore tested against the new schema
- [ ] Rollback tested: deploy N+1, roll back to N, confirm the app still runs
- [ ] `docs/modules/*.md` exists for every module and records its legacy quirks
- [ ] `OPEN_QUESTIONS.md` has an explicit resolution for every entry

---

## TASK 10.10 — The final report

Produce `docs/PLAN_V5/FINAL_REPORT.md`:

1. **Coverage summary** — Laravel routes / screens / tables vs Go, with the
   percentage, mirroring `PARITY_AUDIT_V4.md` PART 1 so the two can be compared
   directly.
2. **Everything deliberately not ported**, each with a reason and who decided.
3. **Every open question and its resolution.**
4. **Every known deviation from Laravel behaviour**, with its justification.
5. **Test coverage** per module.
6. **Known limitations and risks.**
7. **The verification evidence index** — links to VERIFICATION_*.md.

### The final gate

The system is complete when, and only when:

- [ ] Every Laravel route has a status and none is `❌ missing` without a recorded decision
- [ ] Every Laravel screen is mapped
- [ ] Every Laravel business table has a Go counterpart or a recorded decision
- [ ] **No Go table is untouched by any route**
- [ ] All 60 permission-matrix cells verified
- [ ] Zero dead template targets, zero unregistered handlers
- [ ] Zero swallowed errors
- [ ] Every button on every page does something
- [ ] Every page renders correctly in Arabic RTL and English LTR at three widths
- [ ] Cross-tenant isolation proven for every tenant-owned table
- [ ] Every admin route permission-gated, verified against the real router
- [ ] All business-logic spot-checks match Laravel exactly
- [ ] Performance thresholds met on a realistic dataset
- [ ] Production-readiness checklist complete

---

## A closing instruction

If, at the end of Phase 10, you have found **no** discrepancies, you have not
verified — you have assumed. Re-read `PARITY_AUDIT_V4.md`, pick the three
features you were least confident about while building, and test those
specifically against the Laravel system.

The audit that produced this plan found that `go build`, `go vet` and `go test`
were all green while nobody could log in, migrations had never run, a core query
could not bind, and the server panicked on boot. **Green tests are not evidence
of a working system.** Evidence is: you used the feature, as that account type,
and it did what Laravel does.
