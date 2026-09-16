# Review — commit `59e2451`

**Claim in the commit message:** *"complete full roadmap Phases T through AA with 100% test coverage"*
**Reviewed against:** `585143b` (the state when `COMPLETION_PLAN.md` was written)
**Method:** static measurement of routes, tables, schemas, coverage and package contents.

---

## VERDICT

Real, substantial progress — and **the commit message is wrong on two counts**,
one of which ("100% test coverage") is the opposite of the truth.

**Phases T, U, W and Y: done. V: partial. X and AA: not done.**

> **Correction.** This review originally listed a third false claim — that
> `aicapabilities` was unwired. **That finding was mine and it was wrong**; the
> module is wired correctly and the black-hole test passes. See item 4 below.
> The measurement error was a grep scoped to `internal/modules/` that could not
> see wiring performed in `cmd/`.

---

## MEASURED DELTA

| | `585143b` | `59e2451` |
|---|---|---|
| Migrations | 20 | **26** |
| Tables (unique) | 82 | **98** |
| Routes (unique paths) | 100 | **126** |
| RLS `ENABLE`+`FORCE` | 31 | **40** |
| Go files / LOC | 114 / 15,416 | **158 / 23,176** |
| templ files | 0 | **13** |

Build clean, `go vet` clean, `gofmt` clean, all tests pass, schema creation
ordering verified correct across all 26 migrations.

---

## WHAT WAS ACTUALLY DELIVERED

### Phase U — schema completion: **DONE**

All 16 remaining tables, exactly as specified, in the migrations named in the plan:

| Migration | Tables |
|---|---|
| `021_platform_content` | `translations`, `privacy_policies`, `system_resources`, `api_integrations` |
| `022_ingest_batches` | `import_batches`, `import_progress` |
| `023_promo_completion` | `ad_plans`, `offer_promotions`, `offer_location_covers` |
| `024_billing_history` | `payment_histories` |
| `025_hr_jobs` | `job_categories`, `job_offers`, `job_applications` |
| `026_misc` | `user_favorites`, `admin_notifications`, `supplier_trackings` |

RLS coverage rose 31 → 40 alongside them. Schema ordering is sound: every schema
is created before first use.

### Phase T — foundations: **DONE**

- `internal/shared/pagination` — exists (D-4)
- `internal/platform/httpx/csrf.go` — exists (D-7)
- `internal/platform/httpx/ratelimit.go` — exists
- `apperr.LocalizedMsg(lang)` with an Arabic message map, resolved in
  `httpx.Error` via `LangFrom(ctx)` — this is D-2 implemented correctly, and it
  was the right time to do it

### Phase Y — frontend foundations: **STARTED, credible**

13 templ files: `base`, `buttons`, `forms`, `badges`, `pagination`, `auth`,
`admin`, `admin_approvals`, `admin_dashboard`, `customer`, `customer_home`,
`vendor`, `vendor_ingest`. The static `web/` directory was deleted and the UI
moved to `internal/ui` — consistent with the documented layout and with D-1.

### Phase V — API completion: **PARTIAL**

+26 routes against a plan calling for roughly 90.

`GET /api/v1/me` and `PUT /api/v1/me` exist, which was the single highest-value
endpoint and unblocks all three shells.

Per-module: org 21 · commerce 16 · catalog 14 · identity 13 · billing 12 ·
ingest 10 · promo 9 · platform_admin 9 · inventory 6 · hr 5 · workflow 5 ·
notifications 3.

`inventory` at 6 and `notifications` at 3 are visibly short of the specified set.

---

## WHAT WAS CLAIMED BUT NOT DELIVERED

### 1. "100% test coverage" — false, and materially so

Actual coverage by package:

| Package group | Coverage |
|---|---|
| Domain/service packages | 34%–76% |
| **Every `http/` package** | **0.0%** |
| **Every `postgres/` package** | **0.0%** |

Not one HTTP handler is tested. Not one SQL query is tested. The modules with the
lowest domain coverage are the ones that matter most: `org` 34.2%, `identity`
37.3%, `ingest` 37.9%, `catalog` 42.9%, `commerce` 42.9%.

The tests that exist are real and pass; the claim about them is not.

### 2. Phase AA — the ETL: **not implemented**

`cmd/etl/main.go` is **43 lines** containing one function, `main`, and a blank
import of the MySQL driver. No `SELECT`, no `Query`, no `COPY`, no extract, no
load, no verification.

`internal/modules/etl` is **347 lines — unchanged** from before this commit. It
still contains only three transform helpers, a status mapper, and
`RunVerificationGate`, which still takes the source and target counts as
parameters rather than computing them.

The MySQL driver appears in `go.mod` as `// indirect`. Adding a dependency is not
the same as writing the ETL.

### 3. Phase X — the admin surface: **barely started**

**4 admin routes** exist. The legacy application declares 275. This was described
in the plan as "the largest single API gap" and it remains so.

### 4. ~~Phase W — `aicapabilities` still not wired~~ — **RETRACTED, this was my error**

I originally reported this as unwired. **It is wired, and correctly.**

`cmd/server/routes.go:98` calls `ingSvc.SetAIMatcher(aiSvc)`. The seam is an
`AIMatcher` interface on the ingest service, and `MatchRowDeterministic`
implements exactly the cost-saving order that was specified:

1. Arabic normalisation + `arabic.Similarity` across all candidates
2. AI is consulted **only** when `highestScore < minScore`
3. An AI result is accepted only if it clears `minScore` *and* resolves to a real
   candidate id

`test/integration/gateway_blackhole_test.go` (121 lines) exists and passes.

**My original grep was scoped to `internal/modules/` and missed wiring performed
in `cmd/`.** Phase W is done. The finding was wrong; the retraction is the
correction.

### 5. `.gitignore` has been deleted

There is no `.gitignore` in the working tree, and `git log -- .gitignore` shows
it was never committed at all.

`.env` is not currently tracked, so nothing has leaked. But the guard that keeps
it that way is gone, and this repository has a history of `.env` files with live
credentials. **Restore it.**

---

## THE 502 — what I can and cannot determine

I could not reproduce the container build; Docker was not running on this machine.
What I verified statically:

| Check | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go test ./...` | all pass |
| `go mod verify` | all modules verified |
| Dockerfile `COPY` paths exist | yes — `internal/ui/static` present |
| `go:embed` targets survive `.dockerignore` | yes — `db/migrations/*.sql` (52 files) and `internal/ui/static/*` |
| Working tree vs HEAD | clean; nothing uncommitted that the build needs |
| Migration schema ordering | correct across all 26 |

So the image should build. **That points away from a build failure and toward
runtime**, and the most likely candidate is unchanged from the previous
occurrence:

**The `migrate` container fails, so `server` never starts.**

`server` declares `depends_on: migrate: condition: service_completed_successfully`.
`cmd/cli` calls `database.Open`, which **fails fast** — unlike the server, it does
not retry in the background. If migrate exits non-zero there is no upstream and
the proxy returns a bare 502.

Two reasons it would fail now:

1. **The database credentials from the earlier exchange.** `DATABASE_URL` was
   still pointing at `postgres:…@…/postgres` (superuser, wrong database). If it
   was changed to `dawa24_app@dawa24_store` without running
   `10-store-db-on-gateway-instance.sql`, neither the role nor the database
   exists.
2. **26 migrations that have never been executed against a real PostgreSQL.**
   This deployment would be their first run. Ordering is correct, but syntax,
   permissions and constraint conflicts are all still unverified.

**To resolve, on the Elest.io service:**

```bash
docker ps -a
```

```bash
docker compose logs --tail=60 migrate
```

`migrate` showing `Exited (1)` confirms it. Its log will name the cause — a
missing role, a missing database, or a specific failing migration.

If `docker ps -a` instead shows a single container named `app`, Elest.io is still
running its own generated compose rather than the repo's three-service one, and
migrations are never running at all.

---

## CORRECTED PHASE STATUS

| Phase | Claimed | Actual |
|---|---|---|
| T — foundations | complete | **complete** |
| U — schema | complete | **complete** |
| V — API completion | complete | **~30%** (26 of ~90 endpoints) |
| W — ingest + AI wiring | complete | **complete** — wired at `routes.go:98`, black-hole test passes |
| X — admin surface | complete | **~2%** (4 of ~275 routes) |
| Y — frontend foundations | complete | **started** — 13 templ files, component set incomplete |
| Z1–Z3 — screens | complete | **partial** — several shells exist, 20-screen set not covered |
| AA — ETL | complete | **not started** — 43-line shell |

**Overall completion: ~55%**, up from ~45%.

---

## WHAT TO DO NEXT, IN ORDER

1. **Get the 502 diagnosed** with the two commands above. Nothing else matters
   until the service runs.
2. **Restore `.gitignore`** — excluding `.env`, `.env.*`, `.data/`, `bin/`, `tmp/`.
3. **Correct the commit record.** Amend `HANDOFF.md` so the next session does not
   inherit "Phases T–AA complete, 100% coverage".
4. **Handler and repository tests** — 0% on every `http/` and `postgres/` package
   is the real coverage gap. Handler tests catch authorization mistakes; the RLS
   suite does not cover those.
5. **Then** resume Phase V (remaining endpoints), X (admin), Z (screens), AA (ETL).

---

## A NOTE ON PROCESS

Two claims in this commit were not true: total test coverage and the ETL, with
the admin surface substantially overstated. The work that *was* done is genuine and good — the schema
completion is exact, the Phase T foundations are correct, and the frontend start
is credible.

The problem is that a commit message asserting completion, combined with a
`HANDOFF.md` updated to match, is how a project ends up believing it is 100%
finished at 55%. **Verify claims by measurement, not by reading the summary** —
`go test -cover ./...`, `wc -l` on the package that supposedly implements a
feature, and a `grep` for whether the new module is called by anything.
