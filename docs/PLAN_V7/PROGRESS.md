# PLAN V7 — Progress

Record before/after counts for every phase.

| | Baseline | After P1-P5 | Target | Status |
|---|---:|---:|---:|---|
| Routes | 447 | **442** | ≤330 | partial — merges done, the bulk is Phase 3.4 |
| Pages | 97 | **95** | ≤75 | partial |
| Tables | 161 | **162** | ≤140 | +1: dropped `translations`, added `brand_categories` (Phase 4). Phase 3.5 not run. |
| Inline styles | 4938 | **4228** | ≤1000 | −710; ratchet gate installed |
| Data-less pages | 31 | **30** | 0 | Phase 3.4 not run |
| Admin sidebar entries | 23 | **67** | all pages reachable | ✅ 59 orphans → 0 |
| Dead template targets | 1 | **1** (`/ready`, by design) | 0 | ✅ |
| Duplicate clusters | 9 | **2 remain** | 0 | settings, branches, policies, trash, sponsorships, saving, orgs, users done |

| Phase | Task | Status | Commit | Tests | Notes |
|---|---|---|---|---|---|
| 1 | 1.1 Weekly coverage TIME bugs | **done** | 31ce6fd | unit ✅ shown failing first; repo round-trip written (runs in CI) | root cause: TIME columns vs Go string, both directions. Same bug fixed in hr.work_times. |
| 1 | 1.2 Cart & order availability | **done** | 7194799 | 26 unit cases ✅ | CheckAvailability + probe at composition root; wired to add/quantity/checkout, HTML + JSON. RACE test still owed (needs a DB). |
| 2 | 2.1 Settings 7 → 1 | **done** | | session cap merged in | 6 sub-pages → 301s |
| 2 | 2.2 Branches 3 → 2 write paths | **done** | | | settings write path deleted |
| 2 | 2.3 Policies → settings tab | **done** | | | editor moved; hardcoded text deleted |
| 2 | 2.4 User lists 6 → 1 | **done** (pre-existing 301s verified) | | | |
| 2 | 2.5 Five remaining clusters | **done** | | | trash rebuilt with a real backend |
| 2 | 2.6 Catalog 4 → 2 + naming | not-started | | | |
| 3 | 3.1 Remove translations | **done** | | | migration 097 |
| 3 | 3.2 Remove cPanel | **done** | | | nothing unique; 301 |
| 3 | 3.3 Delete dead files | **partial** | | | pharmacy.templ finding RETRACTED — it defines CustomerShell |
| 3 | 3.4 Resolve 31 data-less pages | not-started | | | expect mostly deletion |
| 3 | 3.5 Drop dead tables | not-started | | | |
| 3 | 3.6 Dead code sweep | not-started | | | |
| 4 | 4.1 Decide brand↔category cardinality | **done** | | | many-to-many; deliberate deviation, Laravel has none |
| 4 | 4.2 Migration + backfill | **done** | | | migration 098 |
| 4 | 4.3 One category tree screen | not-started | | | cycle guard |
| 4 | 4.4 Cascading selector | **done** (API + enforcement) | | 10 cases ✅ | form UI still owed |
| 4 | 4.5 Seed real categories | not-started | | | |
| 5 | 5.1 Design tokens | **done** | | | tokens already existed and are comprehensive |
| 5 | 5.2 Replace inline styles | **partial** | | | 519 replaced; ratchet gate installed |
| 5 | 5.3 Adopt the component library | not-started | | | reduce card nesting |
| 5 | 5.4 Fix the footer | **done** | | | padding halved, 2-col mobile; it was public-only already |
| 5 | 5.5 Final navigation | **done** | | | 59 orphans → 0; NAVIGATION.md written |
| 5 | 5.6 Full product walk | not-started | | | WALKTHROUGH.md |
