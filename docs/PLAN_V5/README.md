# PLAN V5 — Read Me First

The complete Laravel→Go completion plan, derived from `docs/PARITY_AUDIT_V4.md`.

## Execution order

1. **`00_MASTER.md`** — rules, conventions, the feature recipe, the definition
   of done, the sidebar specs. **Read this fully before anything else.**
2. `01_PHASE_0_FOUNDATION.md` — P0 blockers and security
3. `02_PHASE_1_VISIBILITY.md` — institutional works + the product read model
4. `03_PHASE_2_COMPARE_ENGINE.md` — the flagship
5. `04_PHASE_3_PROCUREMENT.md` — purchase request, priority engine, automation
6. `05_PHASE_4_INGEST.md` — chunked imports
7. `06_PHASE_5_ADMIN.md` — the admin panel (largest surface gap)
8. `07_PHASE_6_VENDOR.md` — vendor dashboard
9. `08_PHASE_7_CUSTOMER.md` — customer dashboard
10. `09_PHASE_8_REVENUE.md` — packages, sponsorships, ads
11. `10_PHASE_9_PLATFORM.md` — 2FA, sessions, PDF, AI providers
12. `11_PHASE_10_VERIFY.md` — systematic verification. **Mandatory.**

Trackers, updated continuously: `PROGRESS.md`, `OPEN_QUESTIONS.md`.

## The one rule that governs everything

> **Laravel decides WHAT the system does and HOW IT LOOKS.
> Go decides HOW IT IS BUILT.**

## The three things that block the most

| | Why |
|---|---|
| Phase 0 Task 0.1 — vendor coverage | every customer offer listing returns zero rows until a vendor can save coverage |
| Phase 0 Task 0.2 — admin permission gates | any staff account can currently drop any table |
| Phase 1 Task 1.1 — institutional filter | every product listing built later must carry it |

## Do not

- Guess product behaviour. Read the Laravel component. (`00_MASTER.md` §0.11)
- Ship a feature without the check that would have caught its absence. (§0.3 R10)
- Mark anything complete-but-stubbed. A stub that renders is worse than a
  missing page, because it looks done.
- Touch the eight components listed in `00_MASTER.md` §0.14.
