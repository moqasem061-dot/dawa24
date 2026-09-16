# Dawa24 Store — Remaining Work

**Measured at:** commit `f22c3d3`, against the live database
**Supersedes:** the status sections of `EXECUTION_PLAN.md` and `REVIEW_AND_PLAN_2026-08-16b.md`

---

# PART 0 — Correction to the previous review

Two findings in `REVIEW_AND_PLAN_2026-08-16b.md` were **my measurement errors**, not
defects in the work. Correcting them here because the plan depends on knowing
what is genuinely outstanding.

| I reported | Actually |
|---|---|
| `http/` coverage 9–17% | **53–68%** — the 50% target is met |
| Admin routes 37 | **47 registrations across 64 distinct paths** — the "60+" claim was fair |

Both were bad `awk` column parsing on my part. The two findings that **were**
real: migration 029 deleted from disk, and six repository suites skipping on
superuser. Those stand.

---

# PART 1 — Verified current state

| | Value | Assessment |
|---|---|---|
| Migrations | **32 of 32 applied** | Complete |
| Tables | 98 | Complete for current scope |
| API routes | **196** | Was 153 |
| Admin routes | **47** across 64 paths | Target met |
| templ files / UI routes | **42 / 29** | Breadth done, depth unverified |
| Service-layer coverage | 23%–75% | Uneven; catalog 23.6%, commerce 29.0% are the weak points |
| `http/` coverage | **53–68%** | Target met |
| `postgres/` coverage | **catalog 81.7%; all others 0%** | The real gap |
| ETL | 1,241 lines, 2 connections, 9 statements | Structure real, volume thin |

**The system runs.** Register, login, authenticated and anonymous access work
against PostgreSQL. Build, vet and the full unit suite pass.

## Two standing constraints

**The app connects as `postgres`.** Owner's decision. Consequence: the 53 RLS
policies are inert and tenant isolation rests entirely on `WHERE` clauses in Go.
**Do not write code that relies on RLS as its only guard.**

**Reading a green suite:** `ok ... 0.0% of statements` means the tests skipped
and executed nothing. Six suites reported exactly that. Check coverage, not `ok`.

---

# PART 2 — Module-by-module status

| Module | Routes | Admin | http test | pg test | Service cov | Outstanding |
|---|---|---|---|---|---|---|
| `catalog` | 34 | 9 | ✓ 63% | ✓ **81.7%** | **23.6%** | Service coverage |
| `commerce` | 25 | 7 | ✓ 56% | skips | **29.0%** | Split RLS out; service coverage |
| `org` | 23 | 10 | ✓ 60% | skips | 34.2% | Remove skip; fix ids |
| `identity` | 19 | 11 | ✓ 54% | skips | 33.3% | Remove skip; fix ids |
| `inventory` | 17 | 2 | ✓ 66% | **fails** | 64.1% | Fix pinned-id bug |
| `billing` | 16 | 6 | ✓ 63% | skips | 48.6% | Split RLS out |
| `promo` | 16 | 9 | ✓ | **none** | 43.9% | Write repository tests |
| `platform_admin` | 14 | 6 | ✓ | **none** | 52.6% | Write repository tests |
| `ingest` | 11 | 1 | ✓ 59% | **none** | 37.9% | Write repository tests |
| `hr` | 6 | 1 | ✓ 68% | **none** | 69.0% | Write repository tests |
| `notifications` | 6 | 1 | ✓ 64% | **none** | 64.1% | Write repository tests |
| `workflow` | 6 | 1 | ✓ | **none** | 48.5% | Write repository tests |
| `aicapabilities` | — | — | — | — | 75.0% | Complete (no HTTP surface by design) |
| `etl` | — | — | — | — | 51.4% | See Task 4 |

---

# PART 3 — Tasks, in dependency order

## TASK 1 — Repository tests: 1 of 12 modules actually runs

**This is the single largest correctness gap.** Of 32 SQL bugs found in this
project, every one was caught by a static parser or by running the server by
hand. Repository tests are what catch them at commit time.

### 1a. Fix the five existing suites

| Module | Problem | Fix |
|---|---|---|
| `inventory` | Runs, **fails**. Pinned ids (`ID: 88100`) while PostgreSQL assigns them. | Declare `warehouseID`, `stockID`, `transferID` at suite scope; populate from what was created. |
| `identity` | Skips on superuser | Remove the guard |
| `org` | Skips on superuser | Remove the guard |
| `billing` | Skips — **correctly** | Move its RLS assertions to `test/integration/rls_test.go`; what remains runs as any role |
| `commerce` | Skips — **correctly** | Same |

**Reference implementation:** `internal/modules/catalog/postgres/repository_test.go`.
It is the only suite that both runs and passes (81.7%). Copy its shape:

- **No superuser skip.** That guard belongs in the RLS suite, where a superuser
  genuinely cannot prove isolation. A repository test checks SQL correctness,
  which a superuser answers fine. Copying it made six suites report `ok` while
  executing nothing.
- **Never assert on an id you did not read back.** `CreateCategory` lets
  PostgreSQL assign the id and scans it into the struct. Pinning `88100` and
  then looking it up finds nothing, and every later subtest fails on a foreign
  key cascading from that.
- **Create the fixtures your foreign keys need** — organisation, customer
  organisation, user. See `testCustomerOrgID` / `testUserID` at 88190+.
- **Reset fixtures at the start**, not the end, so a failed run leaves data for
  inspection and the next run still works.

### 1b. Write six missing suites

`promo`, `platform_admin`, `ingest`, `hr`, `notifications`, `workflow`.

Each must cover every repository method and assert: money round-trips exactly to
the cent, a row with every optional field empty scans without error (that exact
failure took down `/org/organizations`), and list methods honour limit/offset.

**Verify:**

```bash
DATABASE_URL="postgres://postgres:<pw>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require" \
  go test -count=1 -cover ./internal/modules/.../postgres/
```

**Done when:** all 12 report **above 60%** and pass. A suite at `0.0%` has a skip
in it — find it before believing the `ok`.

---

## TASK 2 — Service-layer coverage on the two weakest modules

`catalog` 23.6% and `commerce` 29.0% are the least-tested code in the system,
and they hold the most business logic: pricing, discounts, order splitting,
status transitions.

Cover specifically:

- **`money.Allocate` across vendor shipments** — the sum of parts must equal the
  order total exactly, including the negative (refund) case.
- **Discount rules** — a discount exceeding price must be rejected; the check
  exists but is untested.
- **Order status machine** — every allowed transition, and every disallowed one
  returning a conflict rather than silently succeeding.
- **Two-phase transfers** — dispatch deducts source only, receive credits
  destination, cancel restores; concurrent receive produces one credit and one
  conflict.

**Done when:** both above 60%.

---

## TASK 3 — Verify the frontend has depth, not just breadth

42 templ files and 29 routes exist. Count is no longer the question; substance
is. For each of the 20 planned screens, confirm:

1. **All four states are implemented** — loading, empty, error, partial. This is
   the difference between a demo and a product, and retrofitting means touching
   every screen twice.
2. **It calls the real API**, with no scaffolding fixtures left behind. Grep for
   hardcoded arrays in the templ files.
3. **RTL verified with real Arabic product names**, which change line height and
   truncation. Not lorem ipsum.
4. **SSE sets `X-Accel-Buffering: no`** — without it the reverse proxy buffers
   every event until the stream closes, so progress looks frozen in production
   while working perfectly locally.
5. **Uploads are presigned** via `storage.PresignPut`, never proxied. A 200 MB
   vendor spreadsheet through the app holds a request goroutine for minutes.

**Budget:** under 40 KB JS, FCP under 1.2s on 3G.

**Done when:** a vendor can log in → create a product → run an import → see an
order, entirely through the UI, with every intermediate state rendering.

---

## TASK 4 — Finish the ETL

1,241 lines with real connections and 9 statements. The skeleton is genuine; the
volume is not.

**Resolve this before writing transform code:**

> Which legacy order system is authoritative: `orders`+`order_items`, or
> `main_orders`+`adv_orders`? The target schema assumed the latter. Verify with
> row counts and recent `created_at` in both. Getting it wrong migrates the wrong
> data and is discovered after cutover.

Then, per `../docs/rebuild/REBUILD_MASTER_PLAN.md` §11:

1. **Extract** every legacy table to newline-delimited JSON, chunked, resumable.
2. **Validate** — orphan sweep across the **36 `*_id` columns with no foreign
   key**, invalid JSON, out-of-range enums, duplicate emails, negative money.
   **Emit a defect report and stop.** Do not transform dirty data.
3. **Transform** — `users` decomposition, order unification, entitlement
   consolidation, blobs to object storage, **explicit UTC → `TIMESTAMPTZ`** (a
   naive cast shifts every timestamp by 2–3 hours).
4. **Load** — `COPY`, foreign keys deferred, indexes created after.
5. **Verify** — the gate computes **both sides itself**. It previously took the
   counts as parameters and compared them, which is why it reported success
   having read nothing. Row counts, checksums, money sums to the cent, zero FK
   orphans, 100% JSON parse, timestamp min/max, and
   `SUM(lines.line_total) = order.final_price`.
6. **Reconcile** — per-table go/no-go.

**Preserve legacy primary keys.** User 4417 stays user 4417.

---

## TASK 5 — Production hardening

1. **Switch the app to `dawa24_app`.** Owner's call; the work is done
   (`cmd/dbcheck -provision`). Until then RLS is inert.
2. **Rotate exposed credentials** — Gateway virtual key, Redis password, and the
   temporary `dawa24_app` password have all appeared in transcripts.
3. **Object storage** — MinIO service, bucket, `STORAGE_*`. Required before
   `APP_ENV=prod` will boot and before presigned uploads work.
4. **Backups** — PITR plus a **tested restore into a scratch database**. An
   untested backup is not a backup.
5. **Metrics** — RED per module, River queue depth, gateway latency and fallback
   rate, connection pool saturation.
6. **CI with a database** — the repository suites are worthless if CI runs them
   without `DATABASE_URL` and they skip. Add a `postgres` service to the workflow
   and fail when the suites report 0% coverage.

---

# PART 4 — Failure patterns seen in this codebase

Every one of these was real here. Check new code against them.

| Pattern | What it cost |
|---|---|
| Identity from the request | Four endpoints read the user or org from a query parameter; anyone could withdraw from any wallet |
| Trusting a header | `X-Dawa-Org-ID` honoured without checking membership; any caller could name any tenant |
| Capturing a dependency at construction | Repositories grabbed the DB pool before it connected and held nil forever |
| Casting a parameter in SQL | `set_config(…, $1::text, …)` with an int64 broke every tenant transaction |
| Type assertion through a wrapper | `w.(http.Flusher)` failed behind the logger middleware; all SSE was dead |
| Column that does not exist | 32 columns named in SQL but never created; compiles, fails at runtime |
| Nullable column into a non-pointer | `cannot scan NULL into *string` took down an endpoint on the first empty field |
| Hashing bytes instead of content | CRLF vs LF made the same migration hash two ways and failed two deploys with no schema change |
| A test that skips | Six suites reported `ok` at 0% coverage having executed nothing |
| A gate that checks nothing | The ETL verification compared two numbers handed to it |
| A verification query that errors | The RLS check used `pg_tables.forcerowsecurity`, which does not exist |
| Reporting without measuring | Claims of "100% coverage" and "60+ routes" — and my own "9–17%" and "37 routes", which were equally wrong in the other direction |

---

# PART 5 — Reporting

At the end of every session:

1. Task, and whether complete or in progress.
2. Files created or changed.
3. **Commands run and their real output** — especially coverage numbers, not
   just `ok`.
4. Anything contradicting this document, with evidence.
5. What is blocked and on what.
6. Confirmation that Part 1 of this file was updated to match reality.

**Measure before you write the summary.** Both the previous agent and I have now
published status figures that did not survive a second look — in both directions.
