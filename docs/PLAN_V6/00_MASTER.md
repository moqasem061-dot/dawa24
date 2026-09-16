# PLAN V6 — Remediation: make it real

**Audience:** Gemini 3.7 Flash (executing agent)
**Evidence base:** `docs/REVIEW_V5_IMPLEMENTATION.md` — read it fully before starting.
**Supersedes:** `docs/PLAN_V5/*` where they conflict. PLAN_V5's *inventory* of
what Laravel has is still correct; its *acceptance criteria* were too weak and
are replaced here.

---

## A.0 Why this plan exists, and how it differs

PLAN_V5 was executed as surface. 44 pages render zero data. 21 submit handlers
write nothing while telling the user they succeeded. 5 admin screens display
fabricated datasets. 32 database tables are referenced by no Go code. The test
suite is green because the tests construct the handler with **every service
`nil`** and assert HTTP 200 — which only a page that reads nothing can pass.

The single task in PLAN_V5 whose acceptance criterion a shell could not satisfy
— *"create coverage through the handler, then an in-range customer sees the
offer and an out-of-range one does not"* — was implemented correctly.

**That is the whole lesson.** This plan therefore replaces "the page renders"
with database assertions everywhere.

---

## A.1 THE THREE LAWS

These override every other instruction in this plan and in PLAN_V5.

### LAW 1 — Delete before you build

This codebase's problem is **too much**, not too little. 420 routes, 97 pages,
140 tables, and a quarter of it is disconnected. Every phase below starts by
removing something.

When you find a screen that is fake, your first question is
**"should this exist at all?"** — not "how do I fill it?". A screen Laravel does
not have, or that duplicates another screen, gets **deleted**. Only then do you
connect what remains.

### LAW 2 — No acceptance criterion may be satisfiable by a shell

Forbidden as a completion test:
```go
// NEVER — this passes for a page that reads nothing
handler := ui.NewUIHandler(nil, nil, ..., logger)
// assert 200
```

Required shape for every feature:
```go
// 1. seed a row in the real database
// 2. drive the REAL handler over HTTP with REAL services
// 3. assert the response CONTAINS the seeded value
// 4. for writes: assert the DATABASE CHANGED
```

If a test would still pass after you delete the handler's body and replace it
with a static render, **the test is worthless**. Delete it and write a real one.

### LAW 3 — A success message is a promise

`h.redirectWithNotice(..., "success", "تم الحفظ بنجاح")` may only be emitted on a
line that is reachable *only after* a service call returned `nil` error.

Forbidden:
```go
func (h *UIHandler) XSubmit(w http.ResponseWriter, r *http.Request) {
	h.redirectWithNotice(w, r, "/x", "success", "تم الحذف بنجاح.")   // writes nothing
}
```

Required:
```go
if err := h.xSvc.Delete(ctx, actor.OrganizationID, id); err != nil {
	h.log.ErrorContext(ctx, "delete x", "error", err, "id", id, "org", actor.OrganizationID)
	h.redirectWithNotice(w, r, "/x", "error", h.safeMessage(err, langOf(r)))
	return
}
h.redirectWithNotice(w, r, "/x", "success", "تم الحذف بنجاح.")
```

Lying to the user about a destructive action is the most serious defect class in
this codebase. Treat it as such.

---

## A.2 The two questions for every screen

Before touching any screen, answer both **in writing** in `docs/PLAN_V6/DECISIONS.md`:

**Q1 — Does Laravel have this screen?**
```bash
grep -ril "<concept>" "F:/Dawa 24/Laravel/app/Livewire/"
grep -rn "<path>" "F:/Dawa 24/Laravel/routes/"
```
- **No** → delete it. It was invented. No exceptions.
- **Yes** → continue to Q2.

**Q2 — Does another Go screen already do this?**
```bash
grep -rn "<table or concept>" internal/ui/*.go internal/ui/pages/*.templ
```
- **Yes** → merge into the existing one, delete the new one, keep a 301 from the
  dead path. Do not maintain two write paths to one table.
- **No** → connect it (Phase C).

Record every answer. `DECISIONS.md` is a deliverable, not a scratchpad.

---

## A.3 Phase order

| Phase | File | What | Why this order |
|---|---|---|---|
| **A** | `01_PHASE_A_TRUTH.md` | Delete the lies: security theatre, fake datasets, no-op destructive actions | These actively mislead users **right now**. Nothing else matters until a "delete" button either deletes or is gone. |
| **B** | `02_PHASE_B_CONSOLIDATE.md` | Merge 7 duplicate clusters; rebuild navigation | Shrinks the surface before you spend effort connecting it. Connecting a screen you are about to delete is pure waste. |
| **C** | `03_PHASE_C_CONNECT.md` | Connect what survives: 44 data-less pages, 21 dead handlers, 32 dead tables | The bulk of the work, now against a minimal surface. |
| **D** | `04_PHASE_D_SILENCE.md` | Eliminate 233 silent-failure sites + add the CI gate | Last, because Phase C rewrites many of these sites anyway. The gate stops recurrence. |
| **E** | `05_PHASE_E_VERIFY.md` | Verification with database assertions | Mandatory. |

---

## A.4 The test doctrine (read this twice)

Every feature you touch needs **exactly these four tests**. Not six, not two.

### D1 — Read test: the page shows what the database holds

```go
func TestVendorPaymentsPage_ShowsSeededPayment(t *testing.T) {
    db := testdb.New(t)                       // real Postgres
    org := seedOrg(t, db, "vendor")
    seedPayment(t, db, org.ID, money.FromMinor(123456), "settled")

    h := newRealUIHandler(t, db)              // REAL services, not nil
    rec := doGET(t, h, "/vendor/payments", actorFor(org))

    require.Equal(t, 200, rec.Code)
    require.Contains(t, rec.Body.String(), "1,234.56")   // the seeded value
}
```

The `Contains` assertion is the point. A shell returns 200 and fails this.

**Build `newRealUIHandler(t, db)` once, in `internal/ui/testsupport_test.go`,
and use it everywhere.** Its existence is a Phase A deliverable — see Task A.0.

### D2 — Write test: the database changed

```go
func TestVendorBranchDelete_ActuallyDeletes(t *testing.T) {
    db := testdb.New(t)
    org := seedOrg(t, db, "vendor")
    br := seedBranch(t, db, org.ID)

    h := newRealUIHandler(t, db)
    doPOST(t, h, fmt.Sprintf("/vendor/branches/%d/delete", br.ID), nil, actorFor(org))

    require.False(t, branchExists(t, db, br.ID))   // the DATABASE changed
}
```

### D3 — Tenant test: another org sees nothing

```go
rec := doGET(t, h, "/vendor/payments", actorFor(otherOrg))
require.NotContains(t, rec.Body.String(), "1,234.56")
```

For writes: org B posting against org A's row ID must fail and leave the row
untouched.

### D4 — Failure test: an error surfaces

Force the service to fail (close the pool, or use a row ID that does not exist)
and assert the user sees an **error**, not a success message and not an empty
list.

```go
doPOST(t, h, "/vendor/branches/999999/delete", nil, actorFor(org))
// must NOT redirect with "success"
require.NotContains(t, location, "notice=success")
```

**D4 is the test that catches Law 3 violations.** It is not optional.

---

## A.5 What "connected" means — the six-link chain

A feature is connected only when all six links exist and are traceable:

```
1. TABLE      db/migrations/NNN_*.up.sql          — exists, has RLS
2. REPOSITORY internal/modules/<m>/postgres/*.go  — real SQL, inside db.InTx/InReadTx
3. INTERFACE  internal/modules/<m>/repository.go  — method declared
4. SERVICE    internal/modules/<m>/service.go     — validation + apperr
5. HANDLER    internal/ui/*.go                    — calls the service, handles the error
6. TEMPLATE   internal/ui/pages/*.templ           — receives typed data as a parameter
```

**Verify the chain mechanically for every feature:**

```bash
T=promo.offer_packages
echo "TABLE:";      grep -l "$T" db/migrations/*.up.sql
echo "REPOSITORY:"; grep -rl "$T" internal/modules/*/postgres/
echo "SERVICE:";    grep -rl "OfferPackage" internal/modules/*/service.go
echo "HANDLER:";    grep -rl "OfferPackage" internal/ui/*.go
echo "TEMPLATE:";   grep -rl "OfferPackage" internal/ui/pages/*.templ
```

Any empty line is a broken chain. A template that takes only `(lang, dir)` is a
broken link 6 even if links 1–5 exist.

---

## A.6 Preserved — do not touch

Verified correct in this review. Changing them is a regression:

| Component | Why |
|---|---|
| `internal/ui/coverage_handlers.go` + `test/integration/coverage_chain_test.go` | the P0 fix, genuinely done, and the model for every test in this plan |
| `internal/ui/admin_routes_*.go` permission groups | `RequirePagePermission` correctly applied |
| `test/admin_guard_test.go`, `test/institutional_guard_test.go` | real guard tests that walk the router |
| `catalog.product_index` + its 7 consumers | genuinely wired |
| `compare.files` / `compare.file_rows` / `compare.plans` repositories | real repository work |
| `internal/platform/authctx/audience.go` | 404-not-403 policy |
| `internal/ui/layouts/shell_for.templ` | shared-page shell dispatcher |
| `internal/modules/promo/postgres/visibility.go` | the single canonical coverage query |
| `internal/shared/money` | single-currency by design |
| `cmd/migratecheck` | migration safety net |

---

## A.7 Engineering rules still in force

From `AGENTS.md`, unchanged and CI-enforced:

- **R1** money never touches `float64` — use `money.Amount`
- **R2** no AI provider names outside `internal/platform/gateway/`
- **R3** every AI capability has a deterministic fallback
- **R4** tenant queries inside `db.InTx`/`db.InReadTx`; `AsSystem` needs a justifying comment
- **R5** `modules/A` must not import `modules/B`; compose in `cmd/server/routes.go`
- **R6** 400 lines per Go file
- **R7** never edit an applied migration; add a new one; write the `.down.sql`
- **R8** tenant tables get `organization_id` + RLS + `platform.tenant_visible`
- **R9** carry Arabic column comments

---

## A.8 Deliverables you must maintain

| File | Contents | Updated |
|---|---|---|
| `docs/PLAN_V6/DECISIONS.md` | Q1/Q2 answers for every screen touched: delete / merge / connect, with the grep evidence | before each screen |
| `docs/PLAN_V6/PROGRESS.md` | one row per task: status, commit, tests D1–D4, notes | after each task |
| `docs/PLAN_V6/DELETED.md` | every route, page, handler, table and test removed, with the reason | at deletion time |
| `docs/PLAN_V6/CHAIN_AUDIT.md` | the six-link table for every feature | Phase C and E |

---

## A.9 Commit discipline

One commit per task. Message:

```
<area>: <what changed>

<why — reference the review finding number, e.g. REVIEW §2.3>

Phase <X> Task <X.N>. Tests: D1-D4.
Before: <the fake behaviour>
After:  <the real behaviour, with the DB assertion that proves it>
```

Never commit with `make check` failing. Never commit a `_templ.go` without its
`.templ`.

---

## A.10 Start here

1. Read `docs/REVIEW_V5_IMPLEMENTATION.md` end to end.
2. Create `DECISIONS.md`, `PROGRESS.md`, `DELETED.md`, `CHAIN_AUDIT.md`.
3. Open `01_PHASE_A_TRUTH.md`, **start at Task A.0** — the test harness. Nothing
   else in this plan is verifiable until `newRealUIHandler` exists.
