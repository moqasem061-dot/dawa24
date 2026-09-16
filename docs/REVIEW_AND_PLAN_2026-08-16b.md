# Review of Gemini's execution, and the plan from here

> **CORRECTION (see docs/REMAINING_WORK.md).** Two findings below were my own
> measurement errors, not defects in the work:
>
> - `http/` coverage is **53–68%**, not 9–17%. The 50% target is met.
> - Admin routes are **47 registrations across 64 distinct paths**, not 37. The
>   "60+" claim was fair.
>
> Both were bad awk column parsing. The two findings that were real — migration
> 029 deleted from disk, and six repository suites skipping on superuser — stand.

**Reviewed at:** commit `38ed0d6` · **Method:** measurement against the live database, not the summary.

---

# PART A — Review

## A.1 Claims vs measurement

Gemini updated `EXECUTION_PLAN.md` Part 0 with a status report. Verified:

| Claim | Measured | Verdict |
|---|---|---|
| Migrations 31 of 31 | **30 on disk, 1 pending** | ✗ 029 deleted, 031 unapplied |
| API routes 165+ | **178** | ✓ exceeded |
| Admin routes 60+ | **37** | ✗ 62% of claim |
| templ files 38 | **42** | ✓ exceeded |
| UI page routes 24 | **29** | ✓ exceeded |
| `http/` — "100% of modules have authorization test suites" | **12 files, 9–17% coverage** | ~ files exist, coverage far below the 50% target |
| `postgres/` — "6 core modules have integration test suites" | **6 files, all skipping, 0% coverage** | ✗ they never executed |
| ETL — "real extract/transform/load/verify" | 740 lines, 2 `sql.Open`, 7 statements | ~ real structure, thin |

**Build, vet and the full unit suite pass cleanly.** That part is true.

## A.2 What was genuinely good

The route work is real and exceeded target: **178 routes**, up from 153. The UI
grew from 13 templ files and 7 page routes to **42 and 29**. Admin routes went
from 4 to 37 — short of 60 but a large, real increase. The ETL gained actual
database connections where before it had none. Handler test files exist for
every module. None of this is fabricated.

## A.3 What was wrong, in order of severity

### 1. Migration 029 was deleted from disk — **critical**

`029_rls_coverage.up.sql` and its `.down.sql` were gone from the working tree.
They exist in git history (`e62673a`).

Nothing failed, because the live database still had migration 29 recorded as
applied. **A fresh database would never have received those 15 row-level
security policies** — wallets, payments, subscriptions, carts, ads, offer
analytics, and five child tables would have shipped with no tenant isolation at
all, and `migrate` would have reported success.

Restored. Its checksum then mismatched, because the file was applied from a CRLF
working tree and git now stores it as LF under `.gitattributes`. Proved both
hash the same content (`488c13fef405` is exactly the CRLF encoding of
`b9a79c0e9743`), then corrected the recorded hash. `cmd/dbcheck -rehash` now
does this safely: it reports applied migrations whose hash differs **only** by
line endings and refuses to touch anything whose content genuinely changed.

### 2. All six repository test suites were skipping — **high**

Every one began with:

```go
if isSuper { t.Skip("connected as a superuser, which bypasses RLS") }
```

That guard is correct in `test/integration/rls_test.go`: a superuser bypasses
row-level security, so the suite cannot prove isolation either way. **It is
wrong in a repository test**, which checks that the SQL is correct — columns
exist, types scan, money round-trips — and which a superuser answers perfectly
well.

The pattern was copied without its reasoning. Result: six suites reported `ok`
in 1.7 seconds each, at 0% coverage, having executed nothing. **This is the same
failure mode the suites were written to prevent.**

### 3. The tests, once running, did not pass — **high**

Removing the skip from `catalog` surfaced real bugs *in the tests*:

- They pinned ids (`ID: 88100`) while `CreateCategory` correctly lets PostgreSQL
  assign them and scans the value back. Every lookup missed, and every later
  subtest failed on a foreign key cascading from the first.
- They lacked the customer organisation and user rows their foreign keys
  require, so `CustomerPricing` and `ProductAlerts` failed on constraints rather
  than on the behaviour under test.

Fixed. `catalog/postgres` now runs at **81.7% coverage**, genuinely.

### 4. Admin surface is 37 routes, not 60+ — **medium**

Real progress from 4, but the claim overstates it by a third.

### 5. `http/` coverage is 9–17%, not the 50% target — **medium**

Test files exist for all 12 modules and assert the authorization surface, which
is the highest-value part. But they only exercise rejection paths; the handlers
themselves are largely uncovered.

---

# PART B — The plan from here

Six tasks, in dependency order. **Task B1 is not optional and comes first.**

---

## B1 — Make the five remaining repository suites run and pass

**Status:** `catalog` done (81.7%). Five remain.

| Module | State | Work |
|---|---|---|
| `inventory` | Runs, **fails** | Same pinned-id bug as catalog. `GetWarehouseByID` misses because the id was assumed, not read back. |
| `identity` | Skips | Guard formatted differently; regex missed it. Remove by hand. |
| `org` | Skips | Same. |
| `billing` | Skips — **correctly** | Contains RLS assertions. See split below. |
| `commerce` | Skips — **correctly** | Same. |

**Do this:**

1. **Split RLS assertions out of `billing` and `commerce` repository tests.**
   Move them into `test/integration/rls_test.go`, where the superuser skip
   belongs. What remains in the repository suite is CRUD and SQL correctness,
   which runs as any role. Mixing the two concerns in one file is why those
   suites can only ever run under one specific credential.
2. **Remove the superuser guard** from `identity` and `org`.
3. **Fix the pinned-id assumption everywhere.** The rule: never assert on an id
   you did not read back from the repository. `catalog/postgres/repository_test.go`
   is the corrected reference — it declares `categoryID`, `brandID`, `productID`,
   `variantID` at suite scope and populates them from what was created.
4. **Add the prerequisite fixtures** each suite's foreign keys need: an
   organisation, a customer organisation, a user. `resetFixtures` in the catalog
   test shows the shape, including the `testCustomerOrgID` / `testUserID`
   constants at 88190+.

**Verify:**

```bash
DATABASE_URL="postgres://postgres:<pw>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require" \
  go test -count=1 -cover ./internal/modules/.../postgres/
```

**Done when:** all six report **above 60% coverage** and pass. A suite reporting
`ok` at `0.0%` is a failed suite — check for a skip before believing it.

---

## B2 — Raise handler coverage from ~13% to 50%

Files exist and assert authorization. Extend each to cover the handler bodies:

1. **Happy path** per endpoint, with a stub repository returning fixtures —
   assert status, `Content-Type`, and response shape.
2. **Validation failures**: missing required fields, negative money, quantity
   zero, discount exceeding price.
3. **Conflict paths**: the compare-and-swap refusals in
   `inventory.UpdateTransferStatus` and `commerce.UpdateShipmentStatus`.
4. **Pagination**: default limit applied, maximum enforced, `has_more` correct.

**Do not** replace the existing `stubRepo` whose methods call `t.Fatalf`. Add a
second, permissive stub for happy-path tests and keep the strict one for
authorization tests — its whole value is that reaching the repository is itself
the failure.

**Done when:** every `http/` package above 50%, mutation check from
`EXECUTION_PLAN.md` Task 2 still fails when both auth layers are removed.

---

## B3 — Finish the admin surface: 37 → ~60 routes

Audit what exists first:

```bash
grep -rhoE '"/api/v1/admin[^"]*"' internal/modules/*/http/*.go | sort -u
```

Then fill the gaps against the table in `EXECUTION_PLAN.md` Task 3. Likely
missing: cross-tenant order search, refunds, wallet adjustment with reason,
sponsorship approval, `ad_plans` management, translations CRUD, audit-log
viewer, session revocation.

**Two requirements that are easy to skip and matter:**

- **Cross-tenant reads use `database.AsSystem(ctx)`** with a comment saying why.
  It is deliberately greppable — that is the point.
- **Every admin mutation writes `platform.audit_log` in the same transaction as
  the change.** A compliance record written in a separate transaction can
  disagree with the thing it describes.

**Done when:** ~60 admin routes; a non-admin gets 403 on every one; every
mutation leaves an audit row with `before` and `after`.

---

## B4 — Finish the ETL

**Current:** 740 lines, real connections, 7 SQL statements. The skeleton is
there; the volume of work is not.

**Highest-priority correctness item — resolve before writing transform code:**

> Which legacy order system is authoritative: `orders`+`order_items`, or
> `main_orders`+`adv_orders`? The target schema assumed the latter. Verify by
> row counts and recent `created_at` in both. Getting this wrong migrates the
> wrong data and is discovered after cutover.

Then, per `../docs/rebuild/REBUILD_MASTER_PLAN.md` §11:

1. **Extract** every legacy table to newline-delimited JSON, chunked, resumable.
2. **Validate** — orphan sweep across the **36 `*_id` columns with no foreign
   key**; invalid JSON; out-of-range enums; duplicate emails; negative money.
   **Emit a defect report and stop.** Do not transform dirty data.
3. **Transform** — `users` decomposition, order unification, entitlement
   consolidation, blobs to object storage, **explicit UTC → `TIMESTAMPTZ`** (a
   naive cast shifts every timestamp by 2–3 hours).
4. **Load** — `COPY`, FKs deferred, indexes after.
5. **Verify** — the gate must compute **both sides itself**. It previously took
   the counts as parameters, which is why it reported success having read
   nothing. Row counts, checksums, money sums to the cent, zero FK orphans,
   100% JSON parse, timestamp min/max, and
   `SUM(lines.line_total) = order.final_price`.

**Preserve legacy primary keys.** User 4417 stays user 4417.

---

## B5 — Complete the frontend

29 page routes exist against 20 planned screens, so coverage is broad. What to
check now is depth, not count:

1. **Every screen has all four states** — loading, empty, error, partial. This
   is the difference between a demo and a product, and retrofitting means
   touching every screen twice.
2. **RTL verified with real Arabic product names**, which change line height and
   truncation. Not lorem ipsum.
3. **Every screen calls the real API.** Check for hardcoded fixtures left behind
   from scaffolding.
4. **SSE sets `X-Accel-Buffering: no`** or the reverse proxy buffers everything
   and progress looks frozen in production while working locally.
5. **Uploads are presigned** (`storage.PresignPut`), never proxied through the
   app.
6. **Budget:** under 40 KB JS, FCP under 1.2s on 3G.

---

## B6 — Production hardening

1. **Switch the app to `dawa24_app`.** Until then the 53 RLS policies are inert
   and tenant isolation rests entirely on `WHERE` clauses in Go. This is the
   owner's call; the work is already done (`cmd/dbcheck -provision`).
2. **Rotate exposed credentials** — the Gateway virtual key, the Redis password,
   and the temporary `dawa24_app` password have all appeared in transcripts.
3. **Object storage** — MinIO service, bucket, `STORAGE_*`. Required before
   `APP_ENV=prod` will boot.
4. **Backups** — PITR plus a **tested restore into a scratch database**.
5. **Metrics** — RED per module, queue depth, gateway latency and fallback rate,
   pool saturation.

---

# PART C — The rule this session keeps proving

Three separate times now, a test or a check has reported success while doing
nothing:

- the ETL verification gate compared two numbers handed to it
- the RLS suite skipped whenever `DATABASE_URL` was unset, which was always
- six repository suites skipped on superuser and reported `ok` at 0% coverage

**A passing test is not evidence. A passing test that executed the code is.**
Before believing a suite, check its coverage number. `ok ... 0.0% of statements`
means it ran nothing.

Corollary for status reports: **measure, then write.** Every claim in this
review was checked with a command, and three of eight did not survive.
