# Execution plan — server-side pagination for every dashboard table

**Audience:** an autonomous coding agent working in `dawa24-store`.
**Prerequisite:** read `AGENTS.md` first. Its non-negotiable rules are enforced by
CI and this plan does not repeat them all.

The UI half of this work is **done and merged**. `components.B2BPagination`
renders a full pagination bar with a rows-per-page selector, and
`internal/shared/pagination` owns the parsing. What remains is the server side:
roughly 94 tables still `SELECT` their whole result set and render every row.

Your job is to move those tables onto `LIMIT`/`OFFSET` with a total count, table
by table, without changing what any of them shows on page 1.

---

## 0. Ground rules

These are the ones you will break if you are not careful. All are CI gates.

| Rule | Why it exists here |
|---|---|
| **400 lines per Go file, max** (`make check-file-size`) | Adding a paginated variant to a repository file will usually push it over. Split by concern into `<name>_list.go` in the same package — do **not** shrink comments to fit. |
| **Tenant queries run inside `db.InTx` / `db.InReadTx`** | Those set the Postgres GUC that row-level security reads. A `COUNT(*)` outside the transaction counts **other tenants' rows** and the user sees a total they cannot page to. This is the single most likely bug in this work. |
| **Cross-tenant access needs `database.AsSystem(ctx)`** | Admin-scope lists only. Never add it to a vendor or pharmacy list to "make the count work". |
| **Never edit an applied migration** | The runner checksums them. Add a new numbered file. |
| **No user-facing Arabic in `.go`** (`make check-hardcoded-arabic`) | Strings go through `internal/shared/i18n`. Pagination adds no new strings — the component owns them. |
| **Money never touches `float64`** | Irrelevant to most of this, but several order/invoice lists select money columns. Use `internal/shared/money.Amount`. |

Run `make check` before you declare any batch finished. A green local `make
check` is exactly what CI runs.

---

## 1. What already exists — use it, do not reinvent it

### 1.1 `internal/shared/pagination`

```go
pagination.TableRows            // 25 — the standard dashboard page size
pagination.RowsPerPageOptions   // []int{10, 25, 50, 100}
pagination.RowsPerPage(r)       // parses ?limit=, falls back to TableRows
pagination.PageNumber(r)        // parses ?page=, clamped to >= 1
```

`RowsPerPage` honours **only** the four offered sizes. A query string is
user-supplied; do not "improve" it into a clamp, or a caller can request page
sizes the UI never offers and the count/offset arithmetic drifts from what the
control can express.

### 1.2 `components.B2BPagination`

```go
@components.B2BPagination(components.PaginationProps{
    CurrentPage: data.Page,
    PageSize:    data.PerPage,
    TotalCount:  data.Total,
    BaseURL:     "/admin/products",          // path only, no query string
    QueryValues: url.Values{"q": {data.Search}, "status": {data.Status}},
})
```

- `BaseURL` is a **path**. The component builds the query itself.
- `QueryValues` must carry **every filter the page supports**. Anything you omit
  is silently dropped when the reader clicks page 2 — this is the classic
  pagination bug and it is invisible in a screenshot.
- The component strips `page`, `limit` and `offset` out of `QueryValues` before
  re-adding them, so passing them through is harmless.
- It renders nothing useful when `TotalCount` is 0; that is intended.

Place it **immediately after the closing `</div>` of `.table-container`**, as a
sibling — not inside the scroll container, or it scrolls horizontally with the
table.

### 1.3 The canonical repository method

`internal/modules/catalog/postgres/match_decisions.go` → `ListMatchDecisions`.
**Read it before writing your first query.** Copy its shape exactly:

```go
func (r *Repository) ListXxx(ctx context.Context, filter XxxFilter, limit, offset int) ([]*module.XxxView, int, error) {
	var out []*module.XxxView
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"1=1"}
		var args []any

		if s := strings.TrimSpace(filter.Search); s != "" {
			args = append(args, "%"+s+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "(t.name ILIKE "+p+" OR t.sku ILIKE "+p+")")
		}
		clause := strings.Join(where, " AND ")

		// 1. COUNT over the SAME joins and the SAME where clause.
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM schema.table t
			 LEFT JOIN ... WHERE `+clause+`;`, args...).Scan(&total); err != nil {
			return err
		}

		// 2. The page. limit/offset are appended AFTER the filter args.
		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		rows, err := tx.Query(txCtx,
			`SELECT ... FROM schema.table t
			 LEFT JOIN ... WHERE `+clause+`
			 ORDER BY t.created_at DESC, t.id DESC
			 LIMIT `+limParam+` OFFSET `+offParam+`;`, args...)
		...
	})
	return out, total, err
}
```

Four things that method gets right and you must not lose:

1. **The count and the page share one `where` clause and one join set.** If they
   drift, the total and the rows disagree and the last page is empty.
2. **Both run inside one `InReadTx`.** Same snapshot, same RLS GUC.
3. **`ORDER BY` ends with a unique tiebreaker (`, t.id DESC`).** Without it
   Postgres may return the same row on two pages and drop another. Every
   `ORDER BY` you add in this work must end with the primary key.
4. **Parameters are numbered by position, never string-formatted.** No
   `fmt.Sprintf` into SQL, ever.

---

## 2. Phase A — introspect the database first

Do this before writing any query. Connect with:

```
postgres://postgres:<REDACTED-ROTATE-THIS-PASSWORD>@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require
```

This is a **live database**. It is read-only for this phase: run `SELECT` and
`EXPLAIN` only. Do not run DDL against it — schema changes go through a
migration file (Phase C) applied by `make migrate`.

### A1. Record real row counts

Cheap tables do not need pagination; expensive ones need indexes. Get the
planner's estimate for everything first:

```sql
SELECT n.nspname AS schema, c.relname AS table, c.reltuples::bigint AS est_rows
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r'
  AND n.nspname IN ('identity','org','catalog','inventory','commerce',
                    'promo','billing','ingest','workflow','hr','platform','ai','compare')
ORDER BY c.reltuples DESC;
```

Write the result to `docs/pagination-audit.md`. **Any table under ~500 rows
today still gets pagination** (it will grow, and the reader gets one consistent
control) but does **not** need a new index.

### A2. Record existing indexes

```sql
SELECT schemaname, tablename, indexname, indexdef
FROM pg_indexes
WHERE schemaname NOT IN ('pg_catalog','information_schema')
ORDER BY schemaname, tablename;
```

You need this to avoid creating a duplicate index in Phase C.

### A3. Confirm every ORDER BY column is indexed

For each table you are about to paginate, run:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM catalog.products
WHERE deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 25 OFFSET 0;
```

A `Seq Scan` followed by a `Sort` on a table over ~10k rows means you owe it an
index in Phase C. An `Index Scan` means you do not.

---

## 3. Phase B — classify every table

There are three cases and they cost very different amounts. Produce the full
inventory in `docs/pagination-audit.md` **before** starting work, as a table:

| page (`internal/ui/pages/*.templ`) | handler | service method | repository method | case |
|---|---|---|---|---|

### Case 1 — markup only (cheapest, do these first)

The handler already parses a limit and the service already returns a total; the
page just never rendered the bar.

**How to detect:** the page's data struct has both a total field
(`Total`/`TotalCount`) and a size field (`PerPage`/`PageSize`/`Limit`).

```bash
# candidates
grep -rlE '(Total|TotalCount)\s+int' internal/ui/pages/*.templ \
  | xargs grep -lE '(PerPage|PageSize|Limit)\s+int' \
  | xargs grep -L 'B2BPagination'
```

**Work:** add the `@components.B2BPagination(...)` call. Confirm the handler
uses `pagination.RowsPerPage(r)` / `pagination.PageNumber(r)` rather than its own
parsing. Nothing else.

Known example already done for you to copy: `internal/ui/pages/admin_chat_history.templ`.

### Case 2 — handler + service already support limit, page struct does not carry it

**How to detect:** the handler calls a service method whose signature ends in
`limit, offset int` and returns `(items, total, error)`, but the page struct
drops the total.

**Work:** add `Page`, `PerPage`, `Total` to the page's data struct, populate them
in the handler, render the bar. No SQL changes.

### Case 3 — the service returns everything (the bulk of the work)

**How to detect:** service calls with no limit at all, or a hardcoded one.
Real examples in the tree right now:

```go
// internal/ui/admin_users_handlers.go
allUsers, _ := h.idSvc.AdminListUsers(sysCtx, "", "")            // no limit
h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)             // hardcoded 500
```

**Work, in this order, per table:**

1. **Repository** — add a paginated sibling next to the existing method. Do not
   change the existing one; other callers depend on it.
   `ListXxx(ctx, filter, limit, offset int) ([]*View, int, error)`
2. **Repository interface** — add the method to
   `internal/modules/<context>/repository.go`.
3. **Mocks** — every mock implementing that interface must gain the method or the
   package stops compiling. They live in `internal/modules/<context>/*_test.go`
   and `internal/modules/<context>/http/*_test.go`.
4. **Service** — add the pass-through in `service.go`.
5. **Handler** — `pagination.PageNumber(r)` / `pagination.RowsPerPage(r)`,
   `offset := (page - 1) * limit`, populate `Page`/`PerPage`/`Total`.
6. **Page** — render `@components.B2BPagination(...)` with **all** filters in
   `QueryValues`.

### Priority order

Do them in this order. Stop and report after each batch.

**Batch 1 — highest row counts, highest traffic**
`catalog.products` (admin products, vendor products), `commerce.orders` (admin /
vendor / customer orders), `promo.offers` (admin offers, market discounts),
`identity.users` (admin users), `platform.audit_log` (admin audit).

**Batch 2 — moderate**
`org.organizations` (admin organisations + approvals), `inventory.stock`,
`notifications.*` (the notifications page), `billing.*` (wallet transactions,
invoices).

**Batch 3 — the long tail**
Reference data (brands, categories, cities, warehouses), settings sub-tables,
everything else with a `<table>`.

---

## 4. Phase C — the migration

**One migration for the whole job**, added after the current highest number.
Current highest is `158_expand_platform_policies`, so yours is:

```
db/migrations/159_pagination_indexes.up.sql
db/migrations/159_pagination_indexes.down.sql
```

### Rules

- Wrap in `BEGIN; ... COMMIT;` (match `157_compare_file_visibility.up.sql`).
- **Index naming:** `<table>_<columns>_idx`, matching the existing convention —
  `audit_log_org_idx`, `products_status_idx`, `users_created_at_idx`.
- Every index must be **backward compatible**: migrations run *before* the new
  image is promoted, and rollback is "redeploy the old image". An added index is
  safe by definition; do not also drop or rename anything in this migration.
- The `.down.sql` drops exactly what the `.up.sql` created, with
  `DROP INDEX IF EXISTS <schema>.<name>;`.
- Only add an index Phase A3 proved is needed. An unused index costs write
  throughput on every insert.

### What to add

For each paginated table, an index matching its **default `ORDER BY`**, which is
what page 1 hits on every load:

```sql
BEGIN;

-- Pagination reads every list newest-first with the primary key as tiebreaker.
-- Without the id column in the index the planner still sorts, and without the
-- tiebreaker the same row can appear on two pages.
CREATE INDEX IF NOT EXISTS orders_created_at_id_idx
    ON commerce.orders (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS products_created_at_id_idx
    ON catalog.products (created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

-- ... one per table proven to need it in Phase A3

COMMIT;
```

Prefer a **partial index** (`WHERE deleted_at IS NULL`) wherever the list filters
soft-deleted rows out — it matches the query and stays smaller.

For tenant-scoped tables, lead with `organization_id`, because RLS puts it in
every predicate:

```sql
CREATE INDEX IF NOT EXISTS offers_org_created_idx
    ON promo.offers (organization_id, created_at DESC, id DESC);
```

### Applying it

```bash
make migrate-status   # confirm 159 is pending and nothing else is
make migrate          # applies it
```

Never edit `159_*` once it has run anywhere. Add `160_*`.

---

## 5. Correctness checks per table

For every table you convert, verify all five. These are the failure modes that
do not show up in a screenshot:

1. **Filters survive paging.** Apply every filter, go to page 2, confirm the
   query string still carries them and the result set is still filtered.
2. **The count matches the filter.** `TotalCount` must be the count of the
   *filtered* set, not the whole table. Reader-visible symptom: "1–25 of 4,000"
   on a search that returns 3 rows.
3. **The last page is not empty.** `ceil(total/limit)` must land on rows. This
   breaks when the count and the page query use different `WHERE` clauses.
4. **No row appears twice.** Page through a table with a non-unique sort column
   (`created_at` with bulk-inserted rows share a timestamp) and confirm the
   tiebreaker holds.
5. **Tenant isolation.** For any tenant-owned table, add a test asserting a
   cross-tenant read returns **zero rows and a zero total**. This is already a CI
   gate for reads; the count is new surface and needs the same proof.

---

## 6. Tests to write

Per converted table, colocated with the repository:

```go
func TestListXxx_Pagination(t *testing.T) {
	// seed 30 rows
	// page 1, limit 25 -> 25 rows, total 30
	// page 2, limit 25 -> 5 rows,  total 30
	// page 3, limit 25 -> 0 rows,  total 30
}

func TestListXxx_FilterAppliesToCount(t *testing.T) {
	// seed 30 rows, 3 matching "aspirin"
	// search "aspirin" -> 3 rows, total 3   (NOT total 30)
}

func TestListXxx_TenantIsolation(t *testing.T) {
	// org A seeds 10 rows; read as org B -> 0 rows AND total 0
}
```

Repository tests run against real Postgres — follow the existing integration
test setup in the module you are working in. Domain-level logic stays
table-driven and colocated.

---

## 7. Definition of done

- [ ] `docs/pagination-audit.md` lists every table, its case, its row count and
      whether it needed an index.
- [ ] Every `<table>` in `internal/ui/pages/` either renders
      `@components.B2BPagination` or is documented in the audit as deliberately
      unpaginated (with the reason — e.g. a fixed 5-row summary).
- [ ] No handler parses `limit` or `page` by hand; all use
      `pagination.RowsPerPage` / `pagination.PageNumber`.
- [ ] `159_pagination_indexes` applied, `make migrate-status` clean.
- [ ] Every `ORDER BY` added in this work ends with the primary key.
- [ ] `make check` green.
- [ ] `go test ./...` green.

---

## 8. Things that will bite you

- **`COUNT(*)` outside the transaction** counts across tenants. Both queries go
  inside the one `InReadTx`.
- **`OFFSET` on a large table is slow** even with an index, because Postgres
  walks the skipped rows. That is acceptable at these sizes and at a 100-row
  maximum page. Do **not** substitute keyset pagination — the UI control is
  page-numbered and keyset cannot express "jump to page 7". The `pagination`
  package has cursor support for API endpoints; that is a different surface.
- **A page struct is shared between templates more often than it looks.** Adding
  fields is safe; renaming is not. Grep before you rename.
- **Mock implementations are compile-time coupled to the repository interface.**
  Adding one method breaks several `_test.go` files at once. Fix them in the same
  commit or the package will not build.
- **`make check-file-size` fails at 401 lines.** Split into `<name>_list.go`
  rather than compressing.
- **Do not add a rows-per-page control to a table you have not paginated
  server-side.** It would claim to change the page size and change nothing.
