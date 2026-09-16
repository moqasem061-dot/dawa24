# PLAN V6 — Remediation

Fixes what PLAN_V5's execution got wrong. Evidence: `docs/REVIEW_V5_IMPLEMENTATION.md`.

## Order

1. **`00_MASTER.md`** — the three laws, the test doctrine, the six-link chain. Read fully.
2. `01_PHASE_A_TRUTH.md` — delete the lies (start at **Task A.0**, the test harness)
3. `02_PHASE_B_CONSOLIDATE.md` — merge 7 duplicate clusters, rebuild navigation
4. `03_PHASE_C_CONNECT.md` — connect 44 pages, 21 handlers, 32 dead tables
5. `04_PHASE_D_SILENCE.md` — 233 silent-failure sites + the CI gate
6. `05_PHASE_E_VERIFY.md` — verification against the database

Trackers: `DECISIONS.md`, `PROGRESS.md`, `DELETED.md`, `CHAIN_AUDIT.md`.

## The three laws

1. **Delete before you build.** The problem is too much, not too little.
2. **No acceptance criterion may be satisfiable by a shell.** If a test still
   passes after you delete the handler's body, the test is worthless.
3. **A success message is a promise.** `"success"` may only be emitted after a
   service call returned nil error.

## What went wrong last time, in one line

The tests construct the handler with **every service `nil`** and assert HTTP 200
— which only a page that reads nothing can pass.

## The numbers that must move

| | Now | Target |
|---|---:|---:|
| Pages rendering zero data | 44 | 0 |
| Submit handlers that write nothing | 21 | 0 |
| Silent-failure sites | 233 | 0 |
| Dead tables | 32 | 0 |
| Duplicate route clusters | 7 | 0 |
| Admin routes not in navigation | 162 | 0 |
| Fabricated datasets | 5 | 0 |

Routes, pages and tables must all go **down**. If they go up, features were added
instead of connected.
