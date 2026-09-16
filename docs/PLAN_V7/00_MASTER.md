# PLAN V7 — Consolidation

**Audience:** Gemini 3.7 Flash
**Evidence:** `docs/AUDIT_V7_CONSOLIDATION.md` — read it fully first.
**Supersedes:** PLAN_V5 and PLAN_V6 where they conflict. Their inventories remain
useful; their instruction to *build* is replaced by an instruction to *consolidate*.

---

## The rule that governs this entire plan

> **Every phase must end with fewer routes, fewer pages, fewer tables, or fewer
> lines than it started with — except Phase 1, which fixes two bugs.**

If a phase adds surface, it was executed wrong. Record before/after counts for
every phase in `PROGRESS.md`.

```bash
routes(){ grep -rhoE '(r|g|pub)\.(Get|Post)\("[^"]+"' internal/ui/handlers.go internal/ui/admin_routes_*.go | sort -u | wc -l; }
pages(){  ls internal/ui/pages/*.templ | wc -l; }
tables(){ grep -ohiE 'CREATE TABLE (IF NOT EXISTS )?[a-z_]+\.[a-z_0-9]+' db/migrations/*.up.sql | sort -u | wc -l; }
styles(){ grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l; }
dataless(){ grep -rhoE 'pages\.[A-Za-z0-9_]+\(lang, dir\)' internal/ui/*.go | sort -u | wc -l; }
```

Baseline: routes **447** · pages **97** · tables **161** · styles **4,938** · data-less **31**.
Target: **≤330** · **≤75** · **≤140** · **≤1,000** · **0**.

---

## Phase order — by user impact, not by size

| # | File | What | Why here |
|---|---|---|---|
| **1** | `01_FIX_BLOCKERS.md` | Weekly coverage (2 bugs) + cart availability | The marketplace returns zero offers and the cart accepts anything. Nothing else matters. |
| **2** | `02_MERGE_DUPLICATES.md` | 9 duplicate clusters → 9 single screens | Shrinks the surface before anything else touches it. |
| **3** | `03_DELETE.md` | Translations, cPanel, dead layouts, orphan pages | Pure removal. Fastest reduction in complexity. |
| **4** | `04_PRODUCT_MODEL.md` | Category tree + category→brand relationship | Needs a migration; do it after the surface settles. |
| **5** | `05_DESIGN_SYSTEM.md` | Design tokens, component adoption, footer, navigation | Last, because it touches every surviving page — do it once, on the final set. |

Do not start a phase until the previous one's gate passes.

---

## The three laws (carried from PLAN_V6, still in force)

1. **Delete before you build.** When you find a broken screen, ask *"should this
   exist?"* before *"how do I fix it?"*.
2. **No acceptance criterion may be satisfiable by a shell.** If a test still
   passes after you delete the handler's body, the test is worthless. Never
   construct `ui.NewUIHandler(nil, nil, …)` in a test.
3. **A success message is a promise.** `"success"` may only be emitted after a
   service call returned a nil error.

---

## Test doctrine

Four tests per feature, against a **real database** with **real services**:

| | Asserts |
|---|---|
| **D1 read** | seed a row → the page body **contains** it |
| **D2 write** | POST → the **database changed** |
| **D3 tenant** | org B cannot read or write org A's row, and the row survives |
| **D4 failure** | a failing path shows an **error**, not success and not an empty list |

Use `newRealUIHandler(t, db)`. If PLAN_V6 Task A.0 has not been done, **do it
first** — it is a prerequisite for every gate in this plan.

**Local runs skip integration tests when `DATABASE_URL` is unset (22 of them
today) and still print `ok`.** Before trusting any gate here, run:
```bash
DATABASE_URL="postgres://..." go test ./... -v 2>&1 | grep -c "^--- SKIP"
```
and confirm the count is 0 for the tests you care about.

---

## Merge procedure (use this every time)

When two screens do one job:

1. **Pick the survivor.** Default to the one with the better structure and more
   data connected. Where the user has stated a preference, that preference wins.
2. **Diff the features.** List what each does. Write it into `MERGE_LOG.md`.
3. **Port the missing features** into the survivor — one at a time, each with D1/D2.
4. **301 the dead path** to the survivor. Do not delete the route; bookmarks and
   Laravel-era links exist.
5. **Delete** the dead handler, template, and its tests.
6. **Update every reference**: sidebars, in-page links, redirects after submit,
   `redirectWithNotice` targets, tests.
7. **Verify:** `grep -rn "<dead path>" internal/` returns only the 301.

---

## Deliverables

| File | Contents |
|---|---|
| `docs/PLAN_V7/PROGRESS.md` | one row per task; before/after counts per phase |
| `docs/PLAN_V7/MERGE_LOG.md` | every merge: survivor, feature diff, what was ported, what was dropped |
| `docs/PLAN_V7/DELETED.md` | every route, page, handler, table, file removed + reason |

---

## Preserved — do not touch

`authctx/audience.go` · `layouts/shell_for.templ` ·
`promo/postgres/visibility.go` (the canonical coverage query — you may fix its
inputs, never fork it) · `admin_routes_*.go` permission groups ·
`test/admin_guard_test.go` · `test/institutional_guard_test.go` ·
`cmd/migratecheck` · `internal/shared/money` · `internal/ui/components/`

---

## Engineering rules (`AGENTS.md`, CI-enforced)

R1 money never `float64` · R2 no provider names outside `platform/gateway` ·
R3 every AI capability has a deterministic fallback · R4 tenant queries inside
`db.InTx`/`InReadTx`, `AsSystem` needs a justifying comment · R5 no
cross-module imports · R6 400 lines per Go file · R7 never edit an applied
migration · R8 tenant tables get RLS · R9 carry Arabic column comments

---

## Start

1. Read `docs/AUDIT_V7_CONSOLIDATION.md`.
2. Confirm `newRealUIHandler` exists (PLAN_V6 A.0). If not, build it.
3. Record the baseline counts in `PROGRESS.md`.
4. Open `01_FIX_BLOCKERS.md`.
