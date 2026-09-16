# PLAN V7 — Consolidation

Evidence: `docs/AUDIT_V7_CONSOLIDATION.md`. Supersedes PLAN_V5 / PLAN_V6 where
they conflict.

## Order

1. **`00_MASTER.md`** — the rule, the merge procedure, the test doctrine
2. `01_FIX_BLOCKERS.md` — weekly coverage (2 bugs) + cart availability
3. `02_MERGE_DUPLICATES.md` — 9 duplicate clusters → 9 single screens
4. `03_DELETE.md` — translations, cPanel, dead files, data-less pages, dead tables
5. `04_PRODUCT_MODEL.md` — category tree + category→brand relationship
6. `05_DESIGN_SYSTEM.md` — tokens, components, footer, navigation, final walk

Trackers: `PROGRESS.md`, `MERGE_LOG.md`, `DELETED.md`.

## The rule

> Every phase must end with **fewer** routes, pages, tables or lines than it
> started with — except Phase 1, which fixes two bugs.

| | Baseline | Target |
|---|---:|---:|
| Routes | 447 | ≤ 330 |
| Pages | 97 | ≤ 75 |
| Tables | 161 | ≤ 140 |
| Inline `style="` | 4,938 | ≤ 1,000 |
| Data-less pages | 31 | 0 |
| Duplicate clusters | 9 | 0 |

## The two blockers

**Weekly coverage is broken in two places.** `coverage_from`/`coverage_to` are
Postgres `TIME` columns scanned into Go `string` fields — the read fails, and the
write fails on a blank form field. The marketplace returns zero offers because of
it. `AUDIT_V7 §1.1`.

**The cart validates nothing.** No stock check that works, no coverage check, no
eligibility check, and `vendorOrgID = 1` as a silent fallback. `AUDIT_V7 §1.4`.

## Do not

- Add a screen. This plan removes them.
- Construct `ui.NewUIHandler(nil, nil, …)` in a test — that is what certified the
  last round of fake pages.
- Emit `"success"` on a path that did not write.
- Trust a green local test run: 22 integration tests skip without `DATABASE_URL`
  and still print `ok`.
