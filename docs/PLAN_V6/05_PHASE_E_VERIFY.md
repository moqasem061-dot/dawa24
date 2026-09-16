# PHASE E — Verify against the database, not the screenshot

**Depends on:** Phases A–D.
**Principle:** the previous verification passed because it asked "does the page
render?". Every check here asks "does the page reflect the database?".

**You are looking for your own gaps.** A verification pass that finds nothing has
failed — the last one certified a system where 44 pages read no data.

---

## TASK E.1 — The seven mechanical scans

Run all seven. Record the output of each in
`docs/PLAN_V6/VERIFICATION_SCANS.md`. Every one must hit its target.

| # | Scan | Target | Command |
|---|---|---|---|
| 1 | Data-less pages | **0** | `grep -rnoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)' internal/ui/*.go \| sort -u` |
| 2 | Submit handlers with no service call | **0** | the Python scan in `01_PHASE_A_TRUTH.md` §A.4 |
| 3 | Swallowed errors | **0** | `make check-error-swallow` |
| 4 | Dead tables | **0** (or ≤3 with written decisions) | the loop in `03_PHASE_C_CONNECT.md` §C.1.2 |
| 5 | Dead template targets | **0** (excluding `/ready`) | `00_MASTER.md` §A.5 |
| 6 | Unregistered handlers | **0** | compare `UIHandler` methods vs registered names |
| 7 | Hardcoded data literals | **0** | `grep -rnE '= \[\]pages\.[A-Za-z]+\{' internal/ui/*.go` |

Scan 4 is the headline metric: **it went 21 → 32 under PLAN_V5.** If it has not
gone to ~0, Phase C did not finish.

---

## TASK E.2 — The six-link chain audit

`CHAIN_AUDIT.md` must have one row per screen with **six non-empty cells**:

| Screen | Table | Repository | Interface | Service | Handler | Template takes data? |
|---|---|---|---|---|---|---|
| `/vendor/payments` | `billing.payments` | `billing/postgres/payments.go` | `repository.go:L` | `service.go:L` | `vendor_finance_handlers.go:L` | ✅ `[]*billing.Payment` |

Verify each row mechanically, not from memory:

```bash
S=VendorPayments; T=billing.payments
grep -l "$T" db/migrations/*.up.sql
grep -rl "$T" internal/modules/*/postgres/
grep -rn "$S" internal/ui/*.go
grep -rn "templ $S" internal/ui/pages/*.templ    # signature must take typed data
```

**The last line is the one that catches fakes.** `templ XPage(lang, dir string)`
is a broken link 6 even when links 1–5 exist.

---

## TASK E.3 — The delete-the-body test

The strongest available proof that a screen is real.

For a **random sample of 15 screens**, one at a time:

1. Replace the handler's data-loading block with nothing (render empty).
2. Run that screen's D1 test.
3. **It must fail.**
4. Revert.

Any screen whose D1 still passes with the body gone has a worthless test.
Rewrite the test, then re-run.

Record all 15 in `VERIFICATION_SCANS.md` with pass/fail.

---

## TASK E.4 — Walk the product as each account type

Scripted where possible, manual where not. For **customer**, **vendor**,
**staff**:

### E.4.1 Every navigation item
Click every sidebar entry. Record: 200 / 404 / 500. **Target: zero non-200 for
links visible to that actor.**

### E.4.2 Every button, on every page
This is tedious and it is the point — the original complaint was that clicking
things does nothing.

Per page record: interactive elements, how many were exercised, and any that
produced no visible change. For every write action, **check the database
afterwards**.

### E.4.3 The five states
For each list screen, force and confirm:

| State | How to force | Must show |
|---|---|---|
| populated | seed rows | the rows |
| empty | delete rows | `EmptyState` with Arabic copy |
| error | rename the table | `ErrorState`, **not** an empty list |
| loading | slow query / HTMX swap | `Skeleton` |
| partial | break one widget's query | that section degraded, page still usable |

The empty-vs-error distinction is the single most important check in this task.

### E.4.4 Localization and RTL
Every page in `ar` and `en`. No untranslated key, no raw `{"ar":…}` JSON leaking
to the UI. RTL: layout direction, icon mirroring, number and date formatting,
label alignment. Language switch preserves the current page.

### E.4.5 Responsive
375px, 768px, 1280px. Sidebar collapse, table behaviour, touch targets ≥44px, no
horizontal body scroll.

Record everything in `docs/PLAN_V6/VERIFICATION_WALKTHROUGH.md`.

---

## TASK E.5 — Security verification

### E.5.1 Audience isolation
Every registered route × {customer, vendor, staff, no-org, signed-out}. Script
it. Wrong audience → **404**. Not signed in → 302 to login. Pending org → 302 to
`/onboarding/pending`.

### E.5.2 Permission isolation
Every admin route, with a staff actor lacking its permission → **404**.
`test/admin_guard_test.go` must walk the **real router** and cover every route
added in Phases B–C.

### E.5.3 Tenant isolation
Every tenant-owned table needs a test proving a cross-tenant read returns zero
rows. Confirm coverage for **every table touched in Phase C**. List them and
their test in `VERIFICATION_SCANS.md`.

### E.5.4 Write-path tenancy
Separate from reads, and more often missed: for every write handler that takes an
ID from the URL or form, org B posting against org A's ID must **fail and leave
the row unchanged**. This is D3-for-writes. Confirm for all of them.

### E.5.5 `AsSystem` audit
```bash
grep -rn "database.AsSystem" internal/ --include=*.go | grep -v _test
```
Every call needs a comment justifying cross-tenant access. Any without one is a
finding. Pay particular attention to calls added in Phase C — a vendor-facing
screen using `AsSystem` is a tenancy leak.

### E.5.6 Secrets
```bash
grep -rniE "api_key|apikey|secret|token|password" internal/ui/pages/*.templ
```
No stored secret rendered to a browser. Specifically re-check
`/admin/api-integrations` after Phase C: masked display only, with a test
asserting the response body contains no value from the credential column.

### E.5.7 2FA (if C.9 shipped)
- A wrong code is **rejected** — the deleted version accepted anything
- `LoginSubmit` refuses to issue a session when `RequiresMFA` and the code is unverified
- The challenge is rate-limited

---

## TASK E.6 — Business-logic parity spot-checks

Construct identical inputs in Laravel and Go; compare to the minor unit.

| Rule | Both sides must agree |
|---|---|
| Vendor earnings | vendor screen **and** admin screen use the same formula |
| Order totals | subtotal, discount, final price |
| Coverage visibility | in-range, out-of-range, wrong day, inactive |
| Institutional filter | both modes, incl. the unrestricted-product asymmetry |
| Rating average | three criteria → scalar |
| Trash counts | the number shown equals `SELECT COUNT(*)` |

Record inputs, Laravel output, Go output, match/mismatch in
`docs/PLAN_V6/VERIFICATION_LOGIC.md`. Any mismatch is a defect.

The trash-count row is deliberately included: it was **fabricated** before
(1240 products, 14200 orders). Prove it is now a query.

---

## TASK E.7 — Structural health

| Metric | Before (review) | Target |
|---|---|---|
| Total routes | 420 | **lower** |
| Admin routes | 186 | **lower** |
| Admin sidebar entries | 24 | ~50, Laravel's 10 groups |
| Admin routes not reachable from navigation | 162 | **0** |
| Pages | 97 | **lower or equal** |
| Tables | 140 | **lower** |
| Dead tables | 32 | **0** |
| Data-less pages | 44 | **0** |
| Silent-failure sites | 233 | **0** |
| Duplicate clusters | 7 | **0** |

**Every count in the left column must go down or stay flat.** If routes, pages or
tables increased during PLAN_V6, features were added instead of connected —
which is the failure mode this whole plan exists to correct.

---

## TASK E.8 — The final report

`docs/PLAN_V6/FINAL_REPORT.md`:

1. **The metrics table** from E.7, before/after.
2. **Everything deleted**, with the reason (from `DELETED.md`).
3. **Everything merged**, with the canonical survivor and its 301s.
4. **Everything connected**, with its `CHAIN_AUDIT.md` row.
5. **Everything deferred**, with the written justification (max 3 tables).
6. **The delete-the-body results** for all 15 sampled screens.
7. **Known limitations and risks.**
8. **Production readiness**: `make check` green · full suite green with `-race` ·
   `make test-integration` actually running · `migratecheck -from 1 -roundtrip`
   green · `TestNewRouterMountsWithoutPanicking` green · rollback tested.

---

## The final gate

The system is done when **all** of these hold:

- [ ] All seven scans hit their targets
- [ ] Every screen has six non-empty chain cells
- [ ] 15/15 delete-the-body tests fail when the body is removed
- [ ] Every navigation item resolves for the actor who can see it
- [ ] Every button either does something or is gone
- [ ] Empty and error states are visibly different everywhere
- [ ] No success message on a path that did not write
- [ ] No fabricated data anywhere
- [ ] Cross-tenant isolation proven for reads **and** writes
- [ ] Every metric in E.7 moved the right way
- [ ] Every business-logic spot-check matches Laravel

---

## Closing instruction

The previous implementation shipped with a green build, a green test suite, 420
routes and 97 pages — and 44 pages that read nothing, 21 buttons that lied, and
5 screens showing invented data.

**Green is not evidence.** Evidence is: you seeded a row, opened the page as that
account type, saw the row, clicked the button, and confirmed the database
changed.

If you cannot say that sentence about a feature, it is not done.
