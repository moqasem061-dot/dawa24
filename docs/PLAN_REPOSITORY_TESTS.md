# Dawa24 Store — Review and Plan

**Measured at:** commit `b371f4b` + Gemini's latest, against the live database
**Supersedes the status sections of:** `REMAINING_WORK.md`, `EXECUTION_PLAN.md`

---

# PART 0 — Review of the latest round

## 0.1 What changed

| | Before | Now |
|---|---|---|
| Repository test files | 6 | **12** — every module |
| Repository suites that **execute** | 1 (`catalog`) | **12** — the superuser skip is gone |
| Repository suites that **pass** | 1 | **2** (`catalog` 81.7%, `billing` 64.6%) |
| Routes | 196 | 196 |
| Migrations | 32 | 32 |

**This is real progress on the thing that mattered most.** The previous round's
central defect was six suites reporting `ok` at 0% coverage while executing
nothing. That is fixed: all twelve now run. Six new suites were written for
`promo`, `platform_admin`, `ingest`, `hr`, `notifications`, `workflow`.

`billing` also went from skipping to passing at 64.6%, which means its RLS
assertions were dealt with rather than worked around.

Build, vet, `gofmt` and the full unit suite pass.

## 0.2 What is broken

**10 of 12 repository suites fail.** Not skip — fail. That is a better problem
than the last one, but it is the problem.

Every failure is a **bug in the test**, not in the application. No application
code is implicated. They fall into exactly two causes.

### Cause A — a poisoned transaction, 8 of 10 failures

```
resetFixtures: database: commit: commit unexpectedly resulted in rollback
```

`resetFixtures` discards every statement error:

```go
_, _ = tx.Exec(txCtx, `DELETE FROM hr.work_times WHERE organization_id = $1`, testOrgID)
_, _ = tx.Exec(txCtx, `INSERT INTO identity.users (id, public_id, ...)
                       VALUES ($1, 'usr_hr_test_1', ...)`, testUserID)
```

`identity.users.public_id` is `UUID NOT NULL DEFAULT gen_random_uuid()`
(migration 002). `'usr_hr_test_1'` is not a UUID, so that statement raises
`22P02 invalid input syntax for type uuid`.

**In PostgreSQL, ignoring a statement error inside a transaction does not work.**
The first failure aborts the entire transaction; every subsequent statement
returns `25P02 current transaction is aborted`, and `COMMIT` reports
`commit unexpectedly resulted in rollback`. The `_, _ =` idiom is safe in some
databases. It is not safe here, and it hides the real cause — you see the commit
failure and never the UUID error that produced it.

This single pattern accounts for `hr`, `ingest`, `notifications`,
`platform_admin`, `promo`, `workflow`, `identity` and `inventory`.

### Cause B — fixtures that violate real constraints, 4 failures

The schema is doing its job; the fixtures do not satisfy it.

| Failure | Constraint |
|---|---|
| `null value in column "name" of relation "organizations"` | `name JSONB NOT NULL` |
| `null value in column "name" of relation "branches"` | `name JSONB NOT NULL` |
| `null value in column "role_key" of relation "members"` | `role_key TEXT NOT NULL REFERENCES identity.roles(key)` |
| `organization_policies_policy_type_check` | value outside the allowed set |
| `contact_messages_status_check` | value outside the allowed set |
| `organization_followers_organization_id_fkey` | org row absent |

## 0.3 Honest assessment

The direction is right and the hard part — making the suites execute at all —
is done. What remains is fixture correctness, which is mechanical.

One caution for the next round: **`2 of 12 passing` is the number, not
`12 suites exist`.** A suite that fails is worth more than one that skips,
because it tells you something. But neither is worth anything until it is green.

---

# PART 1 — Current verified state

| | Value |
|---|---|
| Migrations | 32 of 32 applied |
| Tables | 98 |
| API routes | 196 |
| Admin routes | 47 registrations / 64 paths |
| templ files / UI routes | 42 / 29 |
| `http/` coverage | 53%–68% — target met |
| `postgres/` coverage | `catalog` 81.7%, `billing` 64.6%, **ten at 0%** |
| Service-layer coverage | 23%–75%; `catalog` 23.6% and `commerce` 29.0% weakest |
| ETL | 1,241 lines, 2 connections, 9 statements |

**Standing constraint:** the app connects as `postgres`, so the 53 RLS policies
are inert and tenant isolation rests on `WHERE` clauses in Go. Do not write code
that relies on RLS as its only guard.

---

# PART 2 — Task 1: make the ten failing suites pass

**Nothing else should start until this is done.** Twelve suites that fail are a
blocked pipeline; two that pass are the only SQL verification the project has.

## 2.1 Fix `resetFixtures` in every suite — this alone fixes 8

**Never discard an error inside a transaction.** Replace every

```go
_, _ = tx.Exec(txCtx, ...)
```

with a checked call that returns immediately:

```go
if _, err := tx.Exec(txCtx, `DELETE FROM hr.work_times WHERE organization_id = $1`, testOrgID); err != nil {
    return fmt.Errorf("delete work_times: %w", err)
}
```

The point is not tidiness. In PostgreSQL the first failed statement aborts the
transaction, so a discarded error does not stay discarded — it resurfaces as an
unexplained commit failure several statements later, with the real cause gone.
Returning the error names the statement that actually broke.

**Where a statement is genuinely optional** — deleting a row that may not exist
is fine, because `DELETE` of nothing is not an error — the call still succeeds,
so checking costs nothing and catches the case where the table or column name is
wrong.

## 2.2 Fix the fixture data

| Fix | Detail |
|---|---|
| **`public_id`** | Omit it. Every table defaults it to `gen_random_uuid()`. If a test must set it, use a real UUID literal such as `'00000000-0000-0000-0000-000000088191'`. |
| **`name` on organizations and branches** | `JSONB NOT NULL`. Supply `'{"ar":"...","en":"..."}'::jsonb`, never NULL. |
| **`members.role_key`** | `NOT NULL` and a foreign key to `identity.roles(key)`. Use a seeded key: `org_owner`, `org_manager`, `org_employee`. |
| **Check constraints** | `organization_policies.policy_type` and `contact_messages.status` accept a fixed set. Read the allowed values from the migration before inventing one. |
| **Foreign keys** | Create the parent first — organisation before follower, user before employee. |

**Find the allowed values rather than guessing:**

```bash
grep -n "policy_type\|CHECK" db/migrations/*.up.sql | grep -i "policy_type"
go run ./cmd/dbcheck -cols "$DATABASE_URL" org.organization_policies platform_admin.contact_messages
```

## 2.3 Apply the rules the two passing suites already follow

`catalog/postgres/repository_test.go` (81.7%) and `billing/postgres/repository_test.go`
(64.6%) are the references. Both obey:

1. **No superuser skip.** That guard belongs only in `test/integration/rls_test.go`,
   where a superuser genuinely cannot prove isolation. A repository test checks
   SQL correctness, which a superuser answers fine.
2. **Never assert on an id you did not read back.** The repositories let
   PostgreSQL assign ids and scan them into the struct. Declare
   `categoryID`, `productID` and friends at suite scope and populate them from
   what was created. Pinning `88100` and looking it up finds nothing, and every
   later subtest fails on a foreign key cascading from that.
3. **Reset at the start, not the end**, so a failed run leaves data for
   inspection and the next run still works.
4. **Fixture ids at 88000+**, far above anything the application generates.

## 2.4 What each suite must actually assert

Beyond "it did not error":

- **Every repository method** — create, read, update, delete, list.
- **Money round-trips exactly.** `NUMERIC` → `money.Amount` → `NUMERIC`
  identical to the cent. This is a marketplace; a lost piastre per line is a
  vendor trust problem.
- **A row with every optional field empty scans without error.** That exact
  failure — `cannot scan NULL into *string` — took `/org/organizations` down.
- **List methods honour limit and offset**, and return an empty slice rather
  than nil where the API contract says so.
- **Compare-and-swap refusals** where they exist: `inventory.UpdateTransferStatus`
  and `commerce.UpdateShipmentStatus` must reject a stale `from` status.

## 2.5 Verification

```bash
DATABASE_URL="postgres://postgres:<pw>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require" \
  go test -count=1 -cover ./internal/modules/.../postgres/
```

**Done when all twelve report `ok` at above 60% coverage.**

Read the coverage number, not the `ok`. `ok ... 0.0%` means a skip crept back in.

Then confirm CI is unaffected:

```bash
go test ./...          # no DATABASE_URL — everything must still pass by skipping
```

---

# PART 3 — Task 2: service coverage on catalog and commerce

`catalog` 23.6% and `commerce` 29.0% are the least-tested code in the system and
hold the most business logic.

Cover specifically:

- **`money.Allocate` across vendor shipments.** Sum of parts equals the order
  total exactly, including the negative (refund) case. Independent rounding
  per shipment loses a piastre; the allocation exists to prevent that.
- **Discount validation** — a discount exceeding price is rejected. The check
  exists in `UpdateVariant` and is untested; without it a negative line total
  reaches `money.Allocate` and gets distributed across shipments.
- **Order status machine** — every allowed transition, and every disallowed one
  returning a conflict rather than silently succeeding.
- **Two-phase transfers** — dispatch deducts source only; receive credits
  destination; cancel restores source; a second receive is refused. Stock is
  conserved across the whole cycle.

**Done when both exceed 60%.**

---

# PART 4 — Task 3: frontend depth

42 templ files and 29 routes exist, so breadth is done. Substance is unverified.
For each of the 20 planned screens confirm:

1. **All four states implemented** — loading, empty, error, partial. Retrofitting
   these means touching every screen twice.
2. **It calls the real API.** Grep the templ files for hardcoded arrays left
   over from scaffolding.
3. **RTL verified with real Arabic product names**, which change line height and
   truncation. Not lorem ipsum.
4. **SSE sets `X-Accel-Buffering: no`.** Without it the reverse proxy buffers
   every event until the stream closes — progress looks frozen in production
   while working perfectly on localhost.
5. **Uploads presigned** via `storage.PresignPut`, never proxied.

**Budget:** under 40 KB JS, FCP under 1.2s on 3G.

**Done when** a vendor can log in → create a product → run an import → see an
order end to end through the UI, with every intermediate state rendering.

---

# PART 5 — Task 4: the ETL

1,241 lines, 2 connections, 9 statements. Skeleton genuine, volume thin.

**Resolve before writing transform code:**

> Which legacy order system is authoritative: `orders`+`order_items`, or
> `main_orders`+`adv_orders`? The target schema assumed the latter. Verify by
> row counts and recent `created_at` in both. Getting it wrong migrates the wrong
> data and is discovered after cutover.

Then per `../docs/rebuild/REBUILD_MASTER_PLAN.md` §11: Extract → Validate →
Transform → Load → Verify → Reconcile.

The two that get skipped and must not be:

- **Validate** sweeps orphans across the **36 `*_id` columns with no foreign
  key**, then **emits a defect report and stops.** Do not transform dirty data.
- **Verify** computes **both sides itself.** It previously took the counts as
  parameters and compared them, which is why it reported success having read
  nothing.

**Preserve legacy primary keys.** User 4417 stays user 4417.

---

# PART 6 — Task 5: production hardening

1. **CI with a database.** Add a `postgres` service to the workflow and export
   `DATABASE_URL`. Twelve repository suites that skip in CI are worth nothing —
   that is precisely how the last round shipped six suites testing nothing.
2. **Switch the app to `dawa24_app`** when ready. Until then RLS is inert.
3. **Rotate exposed credentials** — Gateway virtual key, Redis password, and the
   temporary `dawa24_app` password have all appeared in transcripts.
4. **Object storage** — MinIO, bucket, `STORAGE_*`. Required before
   `APP_ENV=prod` boots and before presigned uploads work.
5. **Backups** — PITR plus a **tested restore into a scratch database**.
6. **Metrics** — RED per module, queue depth, gateway latency and fallback rate,
   pool saturation.

---

# PART 7 — Failure patterns seen in this codebase

| Pattern | What it cost |
|---|---|
| Identity from the request | Four endpoints read user or org from a query parameter; anyone could withdraw from any wallet |
| Trusting a header | `X-Dawa-Org-ID` honoured without checking membership |
| Capturing a dependency at construction | Repositories held a nil DB pool forever |
| Casting a parameter in SQL | `$1::text` with an int64 broke every tenant transaction |
| Type assertion through a wrapper | `w.(http.Flusher)` failed behind middleware; all SSE dead |
| Column that does not exist | 32 columns named in SQL but never created |
| Nullable column into a non-pointer | `cannot scan NULL into *string` took down an endpoint |
| Hashing bytes instead of content | CRLF vs LF failed two deploys with no schema change |
| **Discarding an error inside a transaction** | **PostgreSQL aborts the whole transaction; the real cause vanishes and COMMIT fails unexplained — 8 of 10 current failures** |
| A test that skips | Six suites reported `ok` at 0% coverage |
| A gate that checks nothing | The ETL verification compared two numbers handed to it |
| A verification query that errors | The RLS check used a column that does not exist |
| Reporting without measuring | Claims of "100% coverage" and "60+ routes"; and my own "9–17%" and "37 routes", wrong in the other direction |

---

# PART 8 — Reporting

At the end of every session:

1. Task, complete or in progress.
2. Files created or changed.
3. **Commands run and their real output.** For test work that means the coverage
   number per package, not `ok`.
4. Anything contradicting this document, with evidence.
5. What is blocked and on what.
6. Confirmation that Part 1 was updated to match reality.

**Measure before writing the summary.** Every status figure in this project that
was written from memory has been wrong — in both directions.
