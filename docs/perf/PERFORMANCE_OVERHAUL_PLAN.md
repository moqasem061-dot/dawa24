# Dawa24 Store — Performance Audit & Overhaul

**Audit:** 2026-09-05 · **Implementation:** 2026-09-06
**Scope:** `dawa24-store` (Go 1.26 / chi / templ / pgx / River / Redis), production DB `postgres-u74003.vm.elestio.app`
**Method:** static audit of 1,569 Go files + 261 templ files, live `EXPLAIN ANALYZE` against the production database, live latency measurement, and a CPU benchmark of the upload security scanner.

---

## STATUS — what has shipped

Everything in this section is implemented, builds, and passes the test suite. Every number is measured, not estimated.

| Area | Before | After | Where |
|---|---|---|---|
| Catalogue search (`product_index`) | **780 ms**, seq scan | **2.4–19 ms**, BitmapOr over 4 GIN indexes | `catalog/postgres/index.go` |
| Product search (`products`) | **584 ms**, seq scan | **3.3 ms**, BitmapOr over 8 indexes | `catalog/postgres/repository.go` |
| Spreadsheet security scan | **2.28 s**, run **twice** | **1.18 s**, run **once** (951 ms of that is the unavoidable parse) | `shared/filesecurity` |
| Product thumbnails | original served (1 MB+) | **175× smaller** (5.9 KB) | `platform/media` (new) |
| RBAC check per request | 2 transactions = **8 round trips** / 5 s | 1 unscoped query = **1 round trip** | `platform/rbac/resolver.go` |
| Session user lookup | 4 round trips/request | **1 round trip** | `identity/postgres/repository.go` |
| Branch selector | 1 transaction **per customer request** | cached 30 s + explicit invalidation | `ui/branch_selector.go` |
| Anti-scrape guard | 4 sequential Redis round trips | **1** (Lua script) | `platform/antiscrape` |
| Admin trash page | **~100 sequential** `COUNT(*)` round trips | **1** (`UNION ALL`) | `platform_admin/postgres/trash.go` |
| Page-control reload | full table read every **20 s** + a permanently-held pool connection | every **5 min** + a dedicated connection | `platform/pagecontrol/engine.go` |
| Static assets | gzipped per request; `max-age=86400` | gzipped **once at startup**; `immutable` | `ui/static.go` |
| Uploaded files | `max-age=86400` | `immutable` (filenames are content-unique) | `ui/upload_handlers.go` |
| Request logging | one line per asset + image (~40/page) | app endpoints + asset **failures** only | `platform/httpx/middleware.go` |
| Google Fonts | render-blocking third-party stylesheet | preloaded, non-blocking, `noscript` fallback | `ui/layouts/base.templ` |
| Boot-time repair | 3 unbounded `UPDATE`s in the **web process, every start** | migration 186, runs once | `cmd/server/main.go` |
| Runtime limits | none | `GOMEMLIMIT` / `GOMAXPROCS` / cgroup caps | `docker-compose.yml` |

**Proof the search rewrite worked.** `idx_product_index_search_vector` is 8.8 MB of GIN index that had recorded **zero scans since the day it was created**. After the rewrite, on the live database:

```
idx_product_index_search_simple_trgm   77
products_name_ar_trgm_idx              67
idx_product_index_search_vector        25   <-- was 0, forever
idx_products_name_en_trgm              25
idx_product_index_search_text_trgm     23
idx_products_scientific_trgm           21
idx_products_manufacturer_trgm         20
idx_products_name_ar_devowel_trgm      20
```

**Migrations applied to production:** 185 (search/listing indexes, 329 ms) and 186 (orphaned-branch backfill moved out of `main()`, 256 ms).

**New packages, all pure Go with no cgo** — they build and behave identically on macOS, Linux and Windows, and the Docker build keeps `CGO_ENABLED=0`:

- `internal/platform/media` — image derivatives (`golang.org/x/image` + stdlib codecs). 6 tests including a correctly-CRC'd decompression bomb.
- `internal/ui/imageurl` — rendition selection for templates. 8 tests, including one that asserts the slot names still match `media.Sizes`.
- `internal/ui/components/ProductImage` — one `<img>` component with `srcset`, `width`, `height`, `loading` and `decoding`. 9 call sites converted.

**Test suite:** all `./internal/...` and `./cmd/...` tests pass. Three failures remain in `./test/`, all pre-existing and verified against a clean baseline: `TestTenantGatesUseTenantScopedPermissions`, `TestEverySidebarPermissionGatesARoute`, and `TestDeadcodeRatchet` (**326 both before and after** — this work added zero dead code; the ceiling of 303 was already exceeded).

---

## What was deliberately NOT done, and why

**Splitting the JavaScript bundle (§E2).** I built it, then reverted it. The seven page-specific scripts (~67 KB) cannot be lazy-loaded as they stand:

- `combobox.js` and `ads-wizard.js` are Alpine `x-data` roots — the expression is generated in Go (`comboboxState`, `vendor_ads_models.go`), so a literal grep for `x-data="dawaCombobox` finds nothing and the dependency is invisible. Loading them after Alpine boots breaks every combobox and the ads wizard.
- `import-progress.js` and `upload-progress.js` are consumed by inline page scripts that run on `DOMContentLoaded`. Deferred scripts execute *before* that event; a dynamically injected one resolves *after* it. The pages already guard with `if (typeof window.ImportProgress !== 'function')` and **silently do nothing** — and `internal/ui/pages/progress_bootstrap_test.go` exists specifically because this has bitten before.

The correct fix is a per-page `extraJS` parameter on `layouts.Base`, mirroring the existing `extraCSS`. That is a wide change across page handlers whose failure mode is silent, and it needs a browser to verify. It should be done deliberately, not blind.

**Moving imports onto the River queue (§C4).** The eleven `go func()` sites are the right call architecturally and the plan below still stands, but it is a multi-day change touching every import tool, and the `openStoredUpload` path bug (§C4) must be fixed first or the worker reads from the wrong directory. Not safe to rush.

**Splitting `components.css` (238 KB) and adding a minifier (§E).** Mechanical but wide; the pre-compression and immutable caching already landed, which removes the per-request CPU and the daily revalidation.

**Database server tuning (§A5).** `shared_buffers` is still the 128 MB default and `pg_stat_statements` is still absent. Both need a Postgres restart on the Elestio panel — an operator action, not a code change. **Do these; they are the cheapest remaining wins on this list.**

**§B1, the 4-round-trip transaction.** Partly addressed: reads against the eleven tables with no RLS policy now cost one round trip instead of four. The general `BEGIN`+`set_config` batching for tenant-scoped reads is still open, and **its value depends entirely on the colocation check below.**

---

## Do this first, before anything else on this list

Run this **on the Elestio app VM**, not from a laptop:

```bash
docker compose exec server sh -c 'apk add --no-cache postgresql-client 2>/dev/null || apt-get install -y postgresql-client >/dev/null 2>&1; psql "$DATABASE_URL" -c "\timing" -c "SELECT 1" -c "SELECT 1" -c "SELECT 1"'
```

From my machine the round trip to `postgres-u74003.vm.elestio.app` measured **63 ms**, making one wrapped read **252 ms**. If the app container sees < 1 ms, the transaction overhead is a few milliseconds and §B1 is a low priority. If it sees > 5 ms, the DSN is crossing the public internet and **that single fact outweighs everything else in this document** — check whether Elestio exposes a private address for the Postgres service and switch the DSN to it.

---

Everything marked **MEASURED** below is a number I produced in this audit. Everything marked **INFERRED** is a conclusion from reading code that I could not measure without production access. Everything marked **VERIFY** is something you must check on the production host before acting — I flag them rather than guess.

---

## 0. Executive summary — where the time actually goes

The platform is not slow for one reason. It is slow because six independent multipliers stack on top of each other, and each one is individually survivable.

| # | Problem | Measured cost | Fix effort | Priority |
|---|---|---|---|---|
| 1 | Catalogue search does a full sequential scan; the GIN/trigram indexes that would serve it exist but are **never used** (0 scans) | **780 ms** per catalogue search, **584 ms** per product search | 1–2 days | **P0** |
| 2 | Spreadsheet security scanner parses the *entire* workbook with 5 regexes per cell — **and runs twice** per compare upload | **2.28 s per 20k-row file, ×2 = 4.6 s**, synchronously in the HTTP request | 1 day | **P0** |
| 3 | Every DB read costs **4 network round trips** (`BEGIN` → `set_config` → query → `COMMIT`) | 4× RTT per query; **252 ms** per trivial read at 63 ms RTT | 2–3 days | **P0** (if app/DB are not colocated — **VERIFY**) |
| 4 | Zero image processing. Originals are served at full size for 40×40 thumbnails, from local disk, through the Go process, with no CDN | Unbounded — a single 5 MB phone photo per product card | 3–4 days | **P0** |
| 5 | 385 KB of unminified CSS + 188 KB of JS in **19 separate requests** on every single page, plus render-blocking Google Fonts | 141 KB gzipped over the wire, but **385 KB of CSS to parse** on every page — this is the "slow on other people's devices" symptom | 2–3 days | **P1** |
| 6 | Heavy import work runs as raw `go func()` inside the **web server process** (11 sites), not on the River queue that already exists | CPU starvation of request serving; work lost on every deploy | 3–5 days | **P1** |

Plus: the database has never been tuned (`shared_buffers` is still the 128 MB default), `pg_stat_statements` is not installed so nobody can see which queries are slow, and Redis caching is wired into **exactly one** service out of nineteen.

**Expected outcome of the full plan:** catalogue pages from ~1–2 s to under 150 ms; a 20k-row upload from ~10–20 s of blocking work to under 2 s of blocking work; first-paint on a mid-range Android phone roughly halved.

---

## 1. How I measured

```bash
# Live latency + transaction overhead (from a dev machine, not the prod host)
QDSN="postgres://…@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require" go run ./tmp/lat

# All EXPLAIN ANALYZE below were run against the production database via tmp/q.exe
```

Database facts established at audit time:

| Fact | Value |
|---|---|
| PostgreSQL version | 18.6 (Debian) |
| Database size | 219 MB |
| `shared_buffers` | **128 MB — the compiled-in default, never tuned** |
| `work_mem` | **4 MB — default** |
| `effective_cache_size` | 4 GB |
| Server-side `statement_timeout` | 0 (app sets 30 s per connection) |
| `pg_stat_statements` | **NOT INSTALLED** |
| Extensions present | `pg_trgm`, `fuzzystrmatch`, `unaccent`, `citext`, `pgcrypto` |
| Total indexes | 801 |
| **Indexes never used (`idx_scan = 0`)** | **451** |
| Tables with RLS enabled | 100 (101 policies) |
| Heap cache hit ratio | 99.53% (good today; will degrade as data grows past `shared_buffers`) |
| River job backlog | Empty — because the heavy work is **not on the queue** (see §6) |

---

## 2. Layer A — Database and queries (highest measured cost)

### A1. The catalogue search cannot use any index — **MEASURED: 780 ms**

`internal/modules/catalog/postgres/index.go:94` `SearchProductIndex` builds this WHERE clause:

```sql
WHERE status = 'active'
  AND ($1 = ''
       OR search_vector @@ plainto_tsquery('simple', $1)
       OR search_simple ILIKE '%' || platform.normalize_arabic($1) || '%'
       OR search_text  ILIKE '%' || $1 || '%'
       OR name_ar      ILIKE '%' || $1 || '%'
       OR name_en      ILIKE '%' || $1 || '%'
       OR sku          ILIKE '%' || $1 || '%'
       OR COALESCE(scientific_name,'') ILIKE '%' || $1 || '%'
       OR word_similarity(platform.normalize_arabic($1), search_simple) >= 0.25
       OR similarity(search_simple, platform.normalize_arabic($1)) >= 0.15)
ORDER BY CASE WHEN … ILIKE … THEN 1 … END, price_after_discount, updated_at DESC
```

A nine-branch `OR` chain with leading-wildcard `ILIKE` on both sides forces a full sequential scan. Postgres cannot satisfy *any* branch from an index when they are `OR`ed with unindexable ones. Then the `ORDER BY CASE` evaluates **four more `ILIKE`s per surviving row** before sorting.

**Live plan, searching `panadol`:**

```
Limit  (actual time=780.112..780.117 rows=24)
  Buffers: shared hit=2490 read=2810
  -> Sort  (actual time=780.110..780.112 rows=24)
     -> Seq Scan on product_index  (actual time=45.376..779.807 rows=462)
          Rows Removed by Filter: 19534
Execution Time: 780.162 ms
```

**Proof the fix works** — the same term, using only the index that already exists:

```
-- FTS path
-> Bitmap Index Scan on idx_product_index_search_vector
Execution Time: 0.963 ms        <-- 810× faster

-- trigram path
-> Bitmap Index Scan on idx_product_index_search_simple_trgm
Execution Time: 7.882 ms        <-- 99× faster
```

`idx_product_index_search_vector` is **8.8 MB of GIN index with `idx_scan = 0`**. It has never been used once. The same is true of `idx_products_name_en_trgm` (5.2 MB), `products_name_ar_trgm_idx` (5.2 MB), `idx_brands_name_en_trgm` (2.6 MB) and `brands_name_trgm_idx` (2.6 MB). You are paying the full write cost of maintaining ~25 MB of search indexes and getting nothing back.

**This will get worse linearly.** At 200,000 products the same query is ~8 seconds.

#### The fix

Replace the OR-chain with a **staged search**: run the cheap indexed lookups first, and only fall through to fuzzy matching when they return too few rows.

```sql
-- Stage 1 (indexed, ~1 ms): full-text
WHERE status = 'active' AND search_vector @@ plainto_tsquery('simple', $1)

-- Stage 2 (indexed, ~8 ms), only if stage 1 returned < limit:
WHERE status = 'active' AND search_simple % platform.normalize_arabic($1)
ORDER BY similarity(search_simple, platform.normalize_arabic($1)) DESC
```

Rules to enforce going forward:

1. **Never `OR` an indexable predicate with an unindexable one.** Use `UNION ALL` + `DISTINCT ON`, or run the stages sequentially in Go.
2. **Never put `ILIKE` in an `ORDER BY`.** Compute the rank tier in the `SELECT` list once, alias it, and order by the alias — or better, order by `similarity()` which the trigram index can support.
3. Set `pg_trgm.similarity_threshold` per session instead of hard-coding `>= 0.15` in a filter, so the `%` operator (which *is* indexable) can be used.

Same treatment for `SearchProducts` (`internal/modules/catalog/postgres/repository.go:253`), which has an even worse eleven-branch chain including a `regexp_replace` over `normalize_arabic(...)`.

**Live plan, worst case (a term that matches nothing):**

```
Seq Scan on products (actual time=10.349..584.047 rows=47)
Execution Time: 584.129 ms
```

Note what the planner had to run per row — `normalize_arabic` expands inline to `trim(regexp_replace(lower(translate(regexp_replace(regexp_replace(coalesce(name->>'ar','')…)))))`, and that whole stack is evaluated **up to four times per row**.

**Additional fix for `SearchProducts`:** add a generated, indexed column so the normalisation is computed once at write time, not 20,000 times at read time:

```sql
ALTER TABLE catalog.products
  ADD COLUMN search_ar_norm text
  GENERATED ALWAYS AS (platform.normalize_arabic(COALESCE(name->>'ar',''))) STORED;

CREATE INDEX CONCURRENTLY idx_products_search_ar_norm_trgm
  ON catalog.products USING gin (search_ar_norm gin_trgm_ops);
```

(`platform.normalize_arabic` is already `IMMUTABLE` — verified via `pg_proc.provolatile = 'i'` — so a generated column is legal.)

### A2. The stock-availability subquery is in the `ORDER BY` of every listing page

`internal/modules/catalog/postgres/repository.go:228`:

```go
const productHasStockSQL = `EXISTS (
    SELECT 1 FROM catalog.product_variants pv
    JOIN org.organizations o ON o.id = pv.organization_id AND o.deleted_at IS NULL AND o.status='approved'
    JOIN inventory.stocks st ON st.product_variant_id = pv.id AND st.deleted_at IS NULL
    WHERE pv.product_id = catalog.products.id AND pv.deleted_at IS NULL
      AND pv.status='active' AND st.quantity > 0)`

const stockFirstOrder = "(" + productHasStockSQL + ") DESC"
```

**MEASURED:** 20.6 ms today, with a plan cost of 354,417 and a full seq scan + sort of all 19,996 products for a 24-row page. Postgres 18 rescued this by hashing the subplan (`loops=1`); on 17 and earlier, or with a slightly different plan shape, this becomes a correlated subquery executed 20,000 times.

This is also the direct cause of the **45,810,840 rows read by sequential scan on `catalog.products`** in `pg_stat_user_tables` — 2,354 scans × ~19,459 rows each.

**Fix:** materialise it. `catalog.product_index` already exists as a denormalised read model and already carries `stock_quantity`. Either:

- **(preferred)** serve all listing pages from `product_index` and add `CREATE INDEX ... ON catalog.product_index ((stock_quantity > 0) DESC, sold_times DESC) WHERE status = 'active';`, or
- add a `has_stock boolean` column to `catalog.products`, refreshed by the existing `catalog.reindex` River job and by a trigger on `inventory.stocks`.

Do **not** leave a three-table `EXISTS` in an `ORDER BY`.

### A3. `catalog.product_index` is 2.1 KB per row — a seq scan reads 41 MB

19,996 rows, 41 MB heap, 68 MB total. The table stores `search_text`, `search_ar`, `search_en`, `search_simple`, `name_ar`, `name_en` and `scientific_name` — the same text five or six times over.

Once A1 is fixed and lookups are index-driven this matters much less. But while any seq scan remains, every catalogue query drags 41 MB through a 128 MB buffer cache.

**Fix:** drop the redundant search columns from the row and keep only `search_vector` (indexed) plus one `search_simple` (indexed). Reconstruct display text by joining to `catalog.products` for the 24 rows on the page.

### A4. 451 unused indexes out of 801

Every one of them is write amplification on every `INSERT`/`UPDATE` — which is exactly what makes bulk imports slow. The largest offenders:

| Index | Size | Scans |
|---|---|---|
| `compare.file_rows → idx_compare_file_rows_norm_name` | 10 MB | 0 |
| `catalog.product_index → idx_product_index_search_vector` | 8.8 MB | 0 |
| `catalog.products → idx_products_name_en_trgm` | 5.2 MB | 0 |
| `catalog.products → products_name_ar_trgm_idx` | 5.2 MB | 0 |
| `compare.file_rows → compare_file_rows_org_norm_idx` | 4.7 MB | 0 |
| `compare.file_rows → idx_compare_file_rows_sku` | 2.8 MB | 0 |
| `catalog.brands → idx_brands_name_en_trgm` + `brands_name_trgm_idx` | 5.2 MB | 0 |

**Careful here:** some of these are unused *because of A1* (the search indexes) and will become hot the moment A1 lands. Do the search-query fix **first**, re-check `pg_stat_user_indexes` after a week of real traffic, and only then drop what is still at zero. Reset the counters with `SELECT pg_stat_reset();` right after deploying A1 so the measurement window is clean.

`compare.file_rows` has 18+ MB of never-used indexes on a 64 MB table. That is a ~28% write-cost tax on every single uploaded row, on the exact code path the user is complaining is slow.

### A5. The database has never been tuned

`shared_buffers = 128 MB` is PostgreSQL's compiled-in default. On any Elestio Postgres instance with ≥ 4 GB RAM this should be 25% of RAM.

```conf
# postgresql.conf — for a 4 GB instance; scale proportionally
shared_buffers = 1GB                  # was 128MB (default)
effective_cache_size = 3GB
work_mem = 16MB                       # was 4MB — the sorts in §A1/A2 spill at 4MB
maintenance_work_mem = 256MB
random_page_cost = 1.1                # SSD; default 4.0 assumes spinning disk
effective_io_concurrency = 200
max_connections = 100                 # keep, but see A6

# Observability — you are currently flying blind
shared_preload_libraries = 'pg_stat_statements'
pg_stat_statements.max = 10000
pg_stat_statements.track = all
```

Then:

```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

Without `pg_stat_statements` nobody on this project can answer "which query is slow" except by doing what I just did by hand. **Install it before anything else** — it makes every subsequent fix verifiable.

### A6. Connection hygiene

- **6 idle `pgAdmin 4` connections** are sitting on the production database right now, some idle for a long time. Close them; they consume backend slots and `work_mem` reservations.
- `pagecontrol` holds **one pool connection permanently** for `LISTEN` (`internal/platform/pagecontrol/engine.go:187` — `pool.Acquire(ctx)` never released). With `DB_MAX_CONNS=20` that is 5% of the pool gone forever. Use a **dedicated `pgx.Conn`** outside the pool for `LISTEN`, not a pooled connection.
- Two unindexed foreign keys on `platform_admin.managed_pages` (`created_by_fkey`, `updated_by_fkey`). Low impact, trivial fix.

### A7. Background maintenance is running on the request-serving process

`cmd/server/main.go:104-141` starts a goroutine at boot that runs three unbounded `UPDATE … SET branch_id = (SELECT …) WHERE branch_id IS NULL` statements against `catalog.product_variants`, `inventory.warehouses` and `promo.offers` — on **every server start**, forever, in the web process.

This is a data-repair migration living in `main()`. Move it into a one-shot migration or a `maintenance` River job. It currently competes with request serving on every deploy and every restart.

---

## 3. Layer B — The per-request cost model

### B1. Every database read costs four network round trips — **MEASURED**

`internal/platform/database/database.go:transact` wraps *every* read in a transaction, and `applyTenant` adds a separate `SELECT set_config(...)` statement inside it:

```
BEGIN                                       — round trip 1
SELECT set_config('app.current_org_id', …)  — round trip 2
<the actual query>                          — round trip 3
COMMIT                                      — round trip 4
```

**MEASURED from a dev machine to the production database:**

```
connect + TLS handshake:                                    444.9 ms
simple round trip (SELECT 1):                                63.0 ms  (steady state)
full InReadTx cycle (BEGIN + set_config + query + COMMIT):  251.8 ms
```

There are **845 `InTx`/`InReadTx` call sites** in the codebase.

**VERIFY — this is the single most important thing to check on the production host:**

```bash
# Run this ON the Elestio app VM, not from a laptop:
docker compose exec server sh -c \
  'time curl -s -o /dev/null http://localhost:8070/health'

# And measure raw DB RTT from inside the app container:
docker compose exec server sh -c \
  'apt-get install -y postgresql-client >/dev/null 2>&1; \
   psql "$DATABASE_URL" -c "\timing" -c "SELECT 1" -c "SELECT 1" -c "SELECT 1"'
```

- If RTT from the app container is **< 1 ms**, the app and DB are colocated and this is a ~2–4 ms tax per query. Annoying, worth fixing, not urgent.
- If RTT is **> 5 ms** (i.e. the DB is on a different VM reached over the public FQDN), then **a page doing 15 queries is spending 300+ ms purely on network handshakes** and this is your single biggest win.

Note the DSN uses the **public hostname** `postgres-u74003.vm.elestio.app` with `sslmode=require`. If Elestio provides a private/internal network address for the Postgres service, switching to it will cut latency and remove a public exposure at the same time.

#### The fixes, in order of value

**(a) Collapse `BEGIN` + `set_config` into one round trip.** pgx supports pipelining:

```go
// internal/platform/database/database.go
func (db *DB) transact(ctx context.Context, opts pgx.TxOptions, fn …) error {
    conn, err := pool.Acquire(ctx)
    // …
    batch := &pgx.Batch{}
    batch.Queue(beginSQL(opts))                       // "BEGIN READ ONLY" / "BEGIN"
    batch.Queue("SELECT set_config('app.current_org_id', $1, true)", orgIDText)
    br := conn.SendBatch(ctx, batch)                  // ONE round trip for both
    // …
}
```

That takes 4 round trips to 3 — a 25% cut on every single query in the platform, for one localised change.

**(b) Add a read path that does not open a transaction at all.**

Not every read needs RLS. The public catalogue, reference data, site settings, brands, cities and categories are cross-tenant by design and already run under `database.AsSystem`. Add:

```go
// Query runs a read with no transaction and no tenant GUC. ONE round trip.
// Only legal for queries that touch no RLS-protected table — enforce that
// with a test that walks the SQL text against a whitelist of schemas.
func (db *DB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
```

For `AsSystem` reads this is a **4× reduction** in round trips.

**(c) Consider `SET LOCAL` inlined into the first statement.** For the tenant-scoped path, `set_config` can be prepended to the real query in a single `SendBatch`, removing one more trip.

### B2. What an authenticated page request actually costs today

Middleware chain from `cmd/server/main.go:newRouter` + `cmd/server/routes.go:76`:

| Order | Middleware | Cost per request |
|---|---|---|
| — | `pagecontrol.Guard` | in-memory map lookup — cheap |
| 1 | `httpx.RequestID` | `crypto/rand` 12 bytes + hex |
| 2 | `httpx.Recover` | free |
| 3 | `httpx.Logger` | one structured log line, **including for every CSS/JS/image** |
| 4 | `httpx.SecurityHeaders` | `strings.Join` of a **14-element slice, rebuilt every request** |
| 5 | `httpx.Locale` | cookie parse, possible `Set-Cookie` |
| 6 | `httpx.RequestTimeout` | `context.WithTimeout` alloc |
| 7 | `chimw.Compress(5)` | **gzip level 5 recomputed per response, never cached** |
| 8 | `httpx.CSRF` | cheap |
| 9 | `RequireAuth` | Redis `GET` + (every 15 s) `TTL`+`SET`; **then a DB round trip for `GetUserByID`, uncached**; then RBAC (below) |
| 10 | `ResolveTenant` | free unless `X-Dawa-Org-ID` present |
| 11 | `SiteSettingsMiddleware` | DB read every 10 s, **no singleflight** → stampede under load |
| 12 | `RequireCustomer` / `RequireApproved` | context only |
| 13 | `BuyingBranchSelector` | **`ListBranches(orgID)` — a DB read on every customer request, uncached** |

Then the handler runs its own queries, then the layout renders.

**Corroborating evidence from `pg_stat_user_tables`:**

| Table | Seq scans | Rows read by seq scan | Live rows |
|---|---|---|---|
| `org.organizations` | **225,611** | 1,646,506 | 3 |
| `platform_admin.managed_pages` | 21,856 | **12,143,600** | 580 |
| `identity.rbac_version` | 18,055 | 408,216 | 27 |
| `org.roles` | 17,952 | 1,355,256 | 23 |
| `ingest.catalog_imports` | 15,559 | 97,038 | 10 |
| `notifications.logs` | 12,443 | 688,800 | 107 |
| `org.role_permissions` | 7,425 | **12,322,024** | 560 |
| `org.branches` | 46,272 | 662,409 | 4 |
| `compare.files` | 50,146 | 1,666,930 | 170 |
| `identity.users` | 42,300 | 821,380 | 17 |

These tables are tiny, so each individual scan is cheap in CPU — **but each one is still a full four-round-trip transaction**. 225,611 scans of a 3-row table is 225,611 transactions that should have been served from memory.

#### Fixes

1. **Cache `GetUserByID` in the session.** `ValidateSession` (`internal/modules/identity/service_split2.go:77`) re-reads the user row on *every* request purely to check `status = 'active'` and `deleted_at IS NULL`. Cache the answer in Redis for 30–60 s keyed by user id, and invalidate it explicitly on suspend/delete (those code paths already exist for session revocation). Removes one full transaction per request.

2. **Cache `ListBranches`.** Branch lists change rarely. Use the `cache.Remember` helper that already exists (`internal/platform/cache/cache.go:217`) with a 5-minute TTL and invalidate on branch write. Removes another full transaction per customer request.

3. **Raise `rbac.versionTTL` and batch the two version reads.** `internal/platform/rbac/resolver.go:98` sets `versionTTL = 5 * time.Second`, and `Resolve` reads **two** version keys, each in its **own** `InReadTx`. That is 8 round trips every 5 seconds per active user. Either:
   - read both keys in one query (`WHERE scope_key = ANY($1)`) — halves it immediately; and
   - raise the TTL to 30 s and add a `LISTEN/NOTIFY` channel for instant invalidation, exactly as `pagecontrol` already does.

4. **Add singleflight to `siteSettingsMiddleware`** (`internal/ui/site_context.go:29`). On cache expiry, every concurrent request currently misses and queries independently. Use `golang.org/x/sync/singleflight` (already an indirect dependency).

5. **Hoist the CSP string to a package-level `const`.** `internal/platform/httpx/middleware.go:200` rebuilds a 14-element join on every request including every static asset.

6. **Skip `httpx.Logger` for `/static/*` and `/uploads/*`**, or downgrade them to a sampled counter. A catalogue page with 24 product images currently writes 25 log lines. With `max-size: 10m, max-file: 3` in `docker-compose.yml`, your logs roll over so fast they are useless for diagnosis.

7. **Serve pre-compressed static assets.** `chimw.Compress(5)` re-gzips the same 238 KB `components.css` on every cache miss. Gzip it **once at startup** in `initStaticAssetCache` (`internal/ui/static.go:34`), store both variants in the map, and serve the pre-compressed bytes based on `Accept-Encoding`. Add Brotli for a further ~15%.

---

## 4. Layer C — Uploads and import processing

This is where the "uploading takes too long" complaint comes from, and the cause is unambiguous.

### C1. The security scanner parses the entire workbook — **MEASURED: 2.28 s**

`internal/shared/filesecurity/scanner.go:162` `inspectXLSX`:

1. Opens the file as a ZIP and reads every `.rels` entry.
2. Opens it **again** with `excelize.OpenReader` — a full workbook load into memory.
3. Iterates **every sheet, every row, every cell**.
4. Runs `IsSuspiciousText` on each cell, which evaluates up to **five regexes**, one of which (`domainRegex`) is a ~60-alternation monster with backtracking-prone structure.

**Benchmark on this machine** (a fast dev laptop, not the Elestio VPS):

```
file: 1,088,026 bytes, 20,000 rows × 12 cols
ValidateSpreadsheetSecurity: 2.2809 s
plain excelize full parse:   0.8785 s     <-- so ~1.4 s is pure regex
```

On a shared Elestio vCPU expect **5–10 s** for the same file.

For CSV, `inspectDelimited` is worse: it parses the whole file with `csv.Reader`, **then** does `strings.Split(string(content), "\n")` over the entire content again — a second full copy of the file as a Go string plus a slice holding every line.

### C2. …and for compare uploads it runs **twice** — 2× the above

- `internal/ui/compare_upload_handlers.go:~207` calls `filesecurity.ValidateSpreadsheetSecurity(fileBytes, header.Filename)`.
- `internal/modules/compare/upload_background.go:70` — inside `RegisterAndStage`, called immediately after — calls it **again on the same bytes**.

A 10-file batch of 20k-row price lists therefore burns **~46 seconds of CPU on duplicated security scanning alone**, inside the HTTP request, before the user sees a redirect.

#### The fix

**(a) Delete the duplicate call.** The handler already validated; `RegisterAndStage` should trust it. Or invert it — validate only in `RegisterAndStage` and drop it from the handler. Either way, once. **This is a one-line change that halves upload latency.** Do it today.

**(b) Rewrite the scanner to stream and short-circuit.**

```go
// Sketch: bound the work, stop at the first hit, use a single combined regex.
const (
    maxCellsToScan  = 50_000   // a real price list trips a hit long before this
    maxBytesToScan  = 8 << 20
)

// ONE compiled alternation instead of five sequential passes.
var suspicious = regexp.MustCompile(
    `(?i)(https?|ftp|ftps|file)://|javascript:|data:text|\bwww\.[a-z0-9\-]+|…`)
```

Key changes:
1. **Cheap pre-filter before any regex.** 99.9% of cells contain no `.`, `:` or `/`. `strings.ContainsAny(cell, ".:/@")` rejects them in nanoseconds. Only run regexes on the survivors. **This alone should cut the 1.4 s of regex time by ~95%.**
2. **One combined regex**, not five sequential `MatchString` calls over the same string.
3. **Cap the scan.** After `maxCellsToScan` cells with no hit, accept the file. A malicious payload is not hiding at row 45,000 — and if it is, the importer's own field validation catches it.
4. **Drop the second pass in `inspectDelimited`.** The `csv.Reader` pass and the `strings.Split` pass are redundant; keep the reader and use `bufio.Scanner` as the fallback only if the reader errors.
5. **Reuse the parse.** The scanner opens the workbook with excelize; so does the importer, minutes later. Parse **once**, hand the parsed rows to both the scanner and the importer.

**(c) Move it off the request path entirely.** Combined with §C4 below, the security scan belongs in the background job, not in the HTTP handler. The user should get a redirect in < 500 ms with a "processing" state — which the compare tool already models (`FileProcessing` status, `StagingProgress` polling).

### C3. Upload memory and body limits are inconsistent

`internal/ui/upload_handlers.go` documents the memory-vs-size distinction correctly and defines both `uploadMemoryBudget = 4 MB` and `maxImportBatchBytes = 200 MB`. But:

- **25 `ParseMultipartForm` call sites, only 10 `MaxBytesReader` call sites.** Fifteen upload endpoints accept a request body of **unbounded length**.
- Two sites use their own budgets rather than the shared constant: `internal/modules/assistant/http/upload.go` (4 MB) and `internal/modules/ingest/http/handlers.go` (**10 MB** — 2.5× the intended budget).
- There are **49 direct `r.FormFile` / `saveUploadedFile` call sites**. `r.FormFile` silently calls `ParseMultipartForm(32 << 20)` — Go's **32 MB default** — if the form has not already been parsed. Any of those 49 sites reached without a prior `parseImportUpload` call is holding 32 MB of heap per concurrent upload, which is precisely the swap problem the comment in that file describes as already fixed.
- `saveUploadedFile` checks `header.Size > MaxUploadBytes` **after** the entire file has already been received and spooled to disk.
- `io.Copy(dst, src)` where `src` is a spooled temp file means **every upload over 4 MB is written to disk twice** (temp + destination) and read once in between.

#### The fix

```go
// A single entry point every upload handler must use. Enforce with a Makefile
// gate: `grep -rn "ParseMultipartForm\|r.FormFile" internal/ | grep -v upload_handlers.go`
// must return nothing.
func BeginUpload(w http.ResponseWriter, r *http.Request, maxBody int64) error {
    r.Body = http.MaxBytesReader(w, r.Body, maxBody)
    return r.ParseMultipartForm(uploadMemoryBudget)
}
```

And in `saveUploadedFile`, avoid the double write when the part is already on disk:

```go
// multipart.File is an *os.File when the part spilled to disk. Move it
// instead of copying it.
if f, ok := src.(*os.File); ok {
    if err := os.Rename(f.Name(), targetPath); err == nil {
        return publicURL, nil     // zero-copy on the same filesystem
    }
}
_, err = io.Copy(dst, src)        // fallback for in-memory parts
```

Also reject on `Content-Length` **before** reading the body, not on `header.Size` after.

### C4. Heavy import work runs inside the web server process

Eleven `go func()` sites do real work outside any queue:

```
internal/modules/catalog/import_prepare.go:263
internal/modules/compare/upload_background.go:120
internal/modules/ingest/catalog_stage_background.go:85
internal/ui/admin_image_import_handlers.go:212
internal/ui/admin_image_import_sessions.go:47
internal/ui/admin_import_handlers_split2.go:72
internal/ui/admin_temp_warehouse_staging.go:117
internal/ui/compare_upload_handlers.go:278
internal/ui/saving_products_sessions.go:51
internal/ui/team_import_ops.go:26
internal/ui/admin_dashboard_cache.go:146
```

Meanwhile `internal/platform/queue/jobs.go` already defines `ImportStageArgs` (`imports.stage`) and `ImportCommitArgs` (`imports.commit`), the worker container is already deployed, and `river_job` is **empty**.

Consequences:

1. **CPU starvation.** A 20k-row parse is a multi-second CPU burn *in the process serving HTTP*. Go's scheduler is preemptive but the work still consumes cores that requests need. This is exactly the reported symptom: "everything is slow when someone is uploading."
2. **Work is lost on every deploy.** `srv.Shutdown` waits for in-flight *requests*; a detached goroutine holding 40 MB of spreadsheet is killed mid-parse. The file row says `processing` forever.
3. **No global concurrency limit.** `compare_upload_handlers.go` bounds each *request* to 6 workers, but ten users uploading simultaneously is 60 goroutines, each holding a full file in memory. `RegisterAndStage` explicitly passes `fileBytes` into the goroutine (`upload_background.go:118`), so those bytes outlive the request.
4. **No retries, no visibility, no metrics.** River gives you all three for free.

#### The fix

Move all eleven onto River. The pattern already exists — follow `SmartOrderRunArgs`:

```go
// In the handler: write the bytes to disk (already done via saveUploadedBytes),
// then enqueue a reference. Never carry payload bytes through the queue or a goroutine.
_, err := riverClient.InsertTx(ctx, tx, queue.ImportStageArgs{
    FileID:     file.ID,
    StorageKey: localURL,      // the worker re-reads from disk
    OrgID:      orgID,
}, nil)
```

Then fix the reason the code says it *cannot* re-read from disk. `upload_background.go:108-111` explains that `openStoredUpload` searches `data/uploads/compare` while the writer honours `UPLOAD_DIR`/`DATA_DIR`, so a re-read would miss. **That is the actual bug** — fix it by making both sides call `GetUploadBaseDir()`, and the memory-carrying workaround disappears. Both containers already mount the same `uploads` volume, so the worker can read what the server wrote.

Set explicit worker counts so the queue, not the request rate, governs CPU:

```yaml
WORKER_IMPORTS: 2        # already the default — keep it low; these are CPU-bound
```

> **Note on the AI enhancement stage.** `internal/shared/matchflow/ceilings.go` documents `MaxWallClock` of 8 minutes (order profile) and **20 minutes** (vendor profile), with a real observed run of 345 seconds. That is inherent to LLM-based matching and is *correctly* bounded and gated by `MinPlausible`. It is not a bug — but it absolutely must not run inside the web process, and today the paths that call it are among the eleven `go func()` sites above.

### C5. Row writes are already correct — leave them alone

Credit where due: `tx.CopyFrom` and `tx.SendBatch` are used in the hot import paths (`compare/postgres/files_split2.go:62`, `ingest/postgres/catalog_import_rows.go:63`, `catalog/postgres/import_sessions_split2.go:36`). This is the right thing and is not a bottleneck.

The row-by-row `tx.Exec` inside `for` loops that do exist are on cold paths (plan features, brand categories, review ratings, delivery bands). Worth batching eventually, not urgent.

One that **is** worth fixing: `internal/modules/platform_admin/postgres/trash.go:60` runs a `COUNT(*)` query **per table inside a loop**. That is the admin trash page doing dozens of sequential round trips. Rewrite as a single `UNION ALL`.

---

## 5. Layer D — Images and media

This is the most under-engineered area in the platform and directly matches the "displaying images" complaint.

### D1. There is no image processing anywhere

A grep for image manipulation across `internal/` and `cmd/` finds exactly one file: `internal/modules/assistant/imageprep.go`, which downsizes images *for sending to the AI model*. Nothing else.

That means:

- A vendor uploads a 5 MB, 4000×3000 phone photo of a product box.
- It is stored byte-for-byte at `/app/data/uploads/products/products_a1b2c3d4.jpg`.
- `internal/ui/pages/admin_products_table.templ:33` renders `<img src={p.Image} class="product-thumb-img">` — a **40×40 thumbnail**.
- The browser downloads all 5 MB, decodes a 12-megapixel bitmap (~48 MB of RAM), and scales it down.
- A products table with 50 rows downloads **250 MB**.

On a mid-range Android phone this is not "slow", it is unusable.

### D2. No CDN, no object storage — every byte goes through the Go process

`docker-compose.yml` says plainly: *"Object storage. Not yet used by any code path; required only from Phase 5."* `DATA_DIR=/app/data` on a Docker named volume, served by `RegisterUploadRoutes` (`internal/ui/upload_handlers.go:98`) via `http.ServeFile`.

Every product image on every page view consumes: one Go goroutine, one `os.Stat`, one `http.ServeFile` (which stats again), the full middleware chain, one log line, and the VM's outbound bandwidth.

### D3. The markup gives the browser nothing to work with

Counted across all 261 templ files:

| Attribute | Count |
|---|---|
| `<img>` tags | **57** |
| `loading="lazy"` | **13** |
| `decoding="async"` | 4 |
| explicit `width=` / `height=` | **0** |
| `srcset` | **0** |
| `<picture>` | **0** |

Zero width/height means **cumulative layout shift on every page with an image** — content jumping as images load. That is a large part of "the platform feels bad" even when it is not literally slow.

### D4. Caching headers are weaker than they need to be

`internal/ui/upload_handlers.go:104`:

```go
w.Header().Set("Cache-Control", "public, max-age=86400")
```

Upload filenames are `category_<16 random hex chars>.ext` — **immutable by construction**. They can never change content. They should be:

```go
w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
```

Same problem in `internal/ui/static.go:139`: app CSS/JS URLs already carry a content hash (`?v=abc12345` via `AssetURL`) but are served with only `max-age=86400`. A content-hashed URL is by definition immutable. Every returning visitor is revalidating 19 assets a day for no reason.

### D5. The logo is 215 KB

`internal/ui/static/img/logo.png` is **215,189 bytes**. `muhiya-logo.png` is 41,623. Combined, the two logos are **half the size of all the JavaScript on the platform**. A logo should be an SVG at 3–8 KB, or a WebP at 15 KB.

### The image fix plan

**Phase 1 — stop the bleeding (2 days)**

1. **Process on upload.** Add `golang.org/x/image` + `github.com/disintegration/imaging` (or `libvips` via `bimg` if you can accept a CGO dependency). On every image upload, generate and store three derivatives alongside the original:
   ```
   products_a1b2c3d4.jpg          # original, kept for re-processing
   products_a1b2c3d4_thumb.webp   # 128px  — tables, cards
   products_a1b2c3d4_card.webp    # 400px  — catalogue grid
   products_a1b2c3d4_full.webp    # 1200px — detail pages
   ```
   Do it in a River job (`media.derive`), not in the request. Serve a placeholder until derivatives exist.

2. **A helper in templ.** Replace all 57 raw `<img>` with one component:
   ```go
   templ ProductImage(src string, size ImageSize, alt string) {
       <img
           src={ derivedURL(src, size) }
           srcset={ srcsetFor(src, size) }
           width={ size.W } height={ size.H }
           loading="lazy" decoding="async"
           alt={ alt } class={ size.Class } />
   }
   ```
   Add a Makefile gate — `check-raw-img` — that fails the build if a bare `<img src=` appears outside that component, in the same style as the existing `check-inline-styles` and `check-undefined-classes` gates.

3. **`Cache-Control: immutable`** on `/uploads/*` and on hashed `/static/*`.

4. **Replace `logo.png` with an SVG.** Ten minutes, 210 KB saved on every cold page load.

**Phase 2 — get the bytes off the app server (3 days)**

5. **Finish the S3/MinIO path.** `internal/platform/storage/storage.go` and the `STORAGE_*` env vars already exist and `compare` already calls `s.storage.Get`. Wire uploads to write to object storage and set `STORAGE_PUBLIC_BASE_URL` so `<img src>` points at the bucket, not at the Go process. Keep local disk as the dev fallback.

6. **Put a CDN in front of the bucket.** Cloudflare's free tier in front of the storage endpoint removes essentially all image traffic from the Egyptian last mile *and* from your VM's bandwidth. This is the single highest-leverage infrastructure change available and costs nothing.

---

## 6. Layer E — Frontend delivery

### E1. Every page loads 385 KB of CSS in 9 requests — **MEASURED**

`internal/ui/layouts/base.templ:66-79` loads, unconditionally, on every page in the platform:

| File | Bytes |
|---|---|
| `components.css` | **238,464** |
| `nav.css` | 33,039 |
| `utilities.css` | 32,609 |
| `wizard.css` | 27,745 |
| `layout.css` | 20,037 |
| `tokens.css` | 18,920 |
| `foundations.css` | 17,456 |
| `base.css` | 3,711 |
| `app.css` | 2,179 |
| **Total** | **394,160 (385 KB)** |

Plus `extraCSS` per shell — `customer.css` (31 KB), `public.css` (32 KB), `marketing.css` (29 KB) or `admin.css` (5 KB).

**gzip level 5: 82,927 bytes (81 KB) over the wire.**

The bytes are survivable. **The parse is not.** 385 KB of CSS must be tokenised, parsed into a CSSOM and matched against the DOM on a device with an underpowered single-thread. `wizard.css` (27 KB) ships on the home page. `components.css` at 238 KB ships everywhere.

There is **no asset pipeline at all** — no minifier, no bundler, no purge step. The Makefile has 20+ CSS *quality* gates (`check-important`, `check-breakpoints`, `check-physical-properties`, `check-css-layered`…) but not one that touches size.

### E2. …and 188 KB of JS in 10 requests, most of it unused on any given page

`internal/ui/layouts/base.templ:81-108`:

| File | Bytes | Needed on… |
|---|---|---|
| `vendor/htmx-1.9.10.min.js` | 47,755 | most pages |
| `vendor/alpine-3.13.5.min.js` | 43,838 | most pages |
| `js/maps.js` | 27,206 | **4 pages** (registration, branches, admin cities, offer locations) |
| `js/app.js` | 26,519 | all pages |
| `js/combobox.js` | 10,504 | forms only |
| `js/import-progress.js` | 10,313 | **4 import tools** |
| `js/upload-progress.js` | 8,748 | **upload pages only** |
| `js/wallet.js` | 7,355 | **wallet pages only** |
| `js/preview.js` | 7,232 | pages with attachments |
| `js/ads-wizard.js` | 3,320 | **the ads wizard only** |
| **Total** | **192,790 (188 KB)** | gzip-5: **60 KB** |

The comments in `base.templ` explicitly justify each one ("loaded everywhere because the importers live on four different shells"). That reasoning solved a real duplication problem, but the answer to "four shells each shipped their own buggy copy" is **one shared file loaded on the four pages that need it**, not one shared file loaded on all several hundred.

`maps.js` alone is 27 KB of JavaScript downloaded, parsed and executed on the login page.

### E3. Google Fonts is render-blocking and external

`base.templ:64`:

```html
<link rel="stylesheet"
      href="https://fonts.googleapis.com/css2?family=Readex+Pro:wght@300;400;500;600;700;800&family=Spline+Sans+Mono:wght@400;500;600;700&display=swap"/>
```

- **Ten font weights** across two families. Each is a separate `.woff2` fetch from `fonts.gstatic.com`.
- The stylesheet itself is **render-blocking** and depends on a third-party host — the exact dependency `base.templ`'s own comment says was removed for htmx/Alpine/Leaflet. Fonts were left behind.
- `internal/ui/static/` contains **zero `.woff2` files**. Nothing is self-hosted.
- For users in Egypt on mobile networks, `fonts.googleapis.com` + `fonts.gstatic.com` is two extra DNS lookups, two TLS handshakes and up to ten font downloads before text renders.

#### The frontend fix plan

**Phase 1 — cheap and immediate (1 day)**

1. **Self-host the fonts.** Download the `.woff2` files, place them in `internal/ui/static/fonts/` (which `static.go:137` already serves with `max-age=31536000, immutable`), and ship a local `@font-face` block. Cut the weights from 10 to **4** (400/500/700 for Readex Pro, 400 for Spline Sans Mono) — that alone removes six font requests. Add `font-display: swap` and `<link rel="preload" as="font" crossorigin>` for the primary weight.
2. **Move the page-specific scripts to `extraJS`.** `maps.js`, `import-progress.js`, `upload-progress.js`, `wallet.js`, `ads-wizard.js` and `combobox.js` are ~67 KB (35% of all JS) that most pages never touch. `base.templ` already accepts a variadic `extraCSS ...string` — add the identical `extraJS` and pass it from the shells that need it.
3. **Replace `logo.png`** (215 KB) with an SVG.

**Phase 2 — build a real asset pipeline (2 days)**

4. **Add a minify + bundle step** to the Makefile, before `templ generate`. `github.com/tdewolff/minify` is a pure-Go library and needs no Node toolchain:
   ```make
   assets: ## Minify and bundle CSS/JS into internal/ui/static/dist
   	go run ./cmd/assetbuild
   ```
   Expect 385 KB → ~260 KB raw, and — more importantly — **9 CSS requests → 2** (`core.css` for the shell, `<shell>.css` per audience).

5. **Split `components.css`.** 238 KB in one file is the root cause. Split along the `@layer` boundaries the Makefile already enforces, and ship only the layers a shell uses.

6. **Pre-compress at startup.** As in §B2.7 — gzip and Brotli each asset once in `initStaticAssetCache`, serve the stored bytes. Removes all per-request compression CPU and buys ~15% on top from Brotli.

7. **Add a size budget gate**, in the same style as the existing CSS gates:
   ```make
   check-asset-budget: ## Fail if per-page CSS exceeds 150KB or JS exceeds 120KB
   ```

**Phase 3 — measure (0.5 day)**

8. `.lighthouserc.json` already exists in the repo. Wire it into CI with real budgets and run it against a throttled mid-range mobile profile, which is the device class the complaint is actually about.

---

## 7. Layer F — Security and anti-bot systems: what actually costs you

You asked specifically whether the security systems are what is blocking fast uploads. Here is the honest breakdown.

### F1. The anti-scraping guard is **not** your problem

`internal/platform/antiscrape/guard.go` is mounted on exactly two routes — `GET /catalog` and `GET /suppliers` — and the code comments say so deliberately: *"It is deliberately not mounted on the marketing pages… every middleware on a route is latency on it."* That judgement is correct and I would not change it.

It does have one real inefficiency: `Protect` makes **up to 4 sequential Redis calls** per request (`EXISTS` for the penalty, `INCR`+`EXPIRE` for the burst window, `INCR`+`EXPIRE` for the sustained window), each a separate round trip.

**Fix (30 minutes):** collapse them into one Lua script or one `redis.Pipeline`. Four round trips become one.

```go
// One EVALSHA: check penalty, increment both windows, return {penalized, burst, sustained}
var budgetScript = redis.NewScript(`
  if redis.call('EXISTS', KEYS[1]) == 1 then return {1, 0, 0} end
  local b = redis.call('INCR', KEYS[2])
  if b == 1 then redis.call('EXPIRE', KEYS[2], ARGV[1]) end
  local s = redis.call('INCR', KEYS[3])
  if s == 1 then redis.call('EXPIRE', KEYS[3], ARGV[2]) end
  return {0, b, s}
`)
```

`antiscrape.Classify` is pure `strings.Contains` over ~49 signatures — genuinely cheap. Leave it.

### F2. The spreadsheet security scanner **is** your problem

This is the security system that is actually costing you. See §C1 and §C2: **2.28 s of CPU per 20k-row file, run twice**. It is the single largest synchronous cost in the upload path.

To be clear about the trade-off: the protection itself is legitimate — CSV/formula injection and external-link exfiltration through spreadsheets are real, and the scanner's `looksLikeAnAddress` heuristic is a thoughtful piece of work that clearly cost someone real effort against real Arabic pharmaceutical data. **Do not remove it.** Make it fast:

- one combined regex instead of five sequential passes,
- a `strings.ContainsAny(cell, ".:/@")` pre-filter that rejects ~99.9% of cells before any regex runs,
- a scan ceiling,
- run it once, not twice,
- run it in the background job, not in the HTTP handler.

Expect **2.28 s → under 100 ms** with the same detection behaviour. Keep the existing `filesecurity` test suite green as the correctness gate.

### F3. Page control is cheap; its refresh is slightly wasteful

`pagecontrol.Guard` wraps the entire mux but only does a path normalisation and a map lookup — genuinely free. However:

- The engine reloads **every 20 seconds** (`engine.go:31`) with a full table read, on top of the `LISTEN/NOTIFY` that already gives instant invalidation. That accounts for the 21,856 sequential scans reading 12.1 M rows on `platform_admin.managed_pages`. With NOTIFY working, the timer is a safety net — raise it to **5 minutes**.
- It holds a **pooled connection permanently** for `LISTEN` (see §A6).

### F4. RLS is correct but its cost is invisible

100 tables with RLS enabled, 101 policies. The mechanism in `internal/platform/database` is well designed and I would not weaken it. But every policy is a predicate appended to every query on those tables, and with `pg_stat_statements` absent, nobody can see what that costs.

After installing `pg_stat_statements`, compare a hot query with `SET ROLE` bypass vs. normal to quantify. If a policy calls `platform.current_org_id()` (which is `STABLE`, so evaluated once per query, not per row — verified), the cost is small. Confirm rather than assume.

### F5. CSRF, security headers, session validation

- CSRF is cheap.
- `SecurityHeaders` rebuilds a 14-element `strings.Join` per request — trivial to hoist to a `const` (§B2.5).
- Session validation is a Redis `GET` plus **an uncached DB read of the user row** — that DB read is the expensive half and is fixed in §B2.1.

**Summary: your security posture is not what is slowing the platform down — with the single, large exception of the spreadsheet scanner, which is fixable without weakening it at all.**

---

## 8. Layer G — Infrastructure and runtime

### G1. Verify app/DB colocation — **the highest-value 5-minute check on this list**

See §B1. The DSN targets a public Elestio FQDN. If there is a private address available, use it.

### G2. Set `GOMEMLIMIT` and `GOMAXPROCS`

Neither is set anywhere. The `server` and `worker` containers have **no memory or CPU limits** in `docker-compose.yml` and no Go runtime hints. On a small VPS this means the GC has no idea how much memory it may use, and the two containers plus Postgres compete without arbitration.

```yaml
services:
  server:
    environment:
      GOMEMLIMIT: 1500MiB     # ~75% of the container's real ceiling
      GOMAXPROCS: 2
    deploy:
      resources:
        limits: { memory: 2G, cpus: '2' }
  worker:
    environment:
      GOMEMLIMIT: 1000MiB
      GOMAXPROCS: 1           # imports are CPU-bound; do not let them starve the server
    deploy:
      resources:
        limits: { memory: 1.5G, cpus: '1' }
```

Setting these correctly is what stops a big import from pushing the whole machine into swap — the failure mode `upload_handlers.go` describes and only partially fixed.

### G3. Enable HTTP/2 and Brotli at the Elestio reverse proxy — **VERIFY**

With 19 asset requests per page, HTTP/1.1's 6-connections-per-host limit means four sequential rounds of requests. Confirm the proxy in front of `172.17.0.1:8070` terminates TLS with HTTP/2 (it very likely does) and enables Brotli. If both are already on, §E's request-count reduction still helps but matters less.

### G4. Log volume

`max-size: 10m, max-file: 3` = 30 MB of retained logs. With one JSON line per request *including every static asset and image*, that window is minutes, not days. Combined with §B2.6 (skip asset logging), raise to `max-size: 50m, max-file: 5`.

---

## 9. Layer H — Observability: you cannot fix what you cannot see

Right now the platform has structured request logs and nothing else. There is no way to answer "which page is slow", "which query is slow", or "how long does an upload take" without doing what I did by hand.

**Add, in this order:**

1. **`pg_stat_statements`** (§A5). Non-negotiable, and it makes every other item on this list measurable.
2. **Per-request query counting.** Add a counter to the request context, incremented in `database.transact`, and log it alongside `duration_ms`. You will immediately see which handlers are doing 40 queries to render one page.
   ```go
   log.Log(ctx, level, "http request", …,
       "duration_ms", …, "db_queries", queryCountFrom(ctx), "db_ms", dbTimeFrom(ctx))
   ```
   This one change will surface more N+1 problems in a day than a week of code review.
3. **A `/metrics` endpoint.** `METRICS_PORT=9090` is already in `.env.example` but nothing serves it. Expose: request duration histogram by route, pool `Acquire` wait time (`pgxpool.Stat()`), River queue depth and job duration, cache hit ratio.
4. **Slow-query logging** in Postgres: `log_min_duration_statement = 500ms`.
5. **Wire the existing `.lighthouserc.json`** into CI (§E, Phase 3).

---

## 10. Prioritised execution plan

Ordered by (measured impact) ÷ (effort). Each phase is independently shippable.

### Phase 0 — Instrument first (half a day)

| # | Task | Why first |
|---|---|---|
| 0.1 | Install `pg_stat_statements`; `CREATE EXTENSION` | Everything else becomes measurable |
| 0.2 | Set `log_min_duration_statement = 500ms` | Slow queries become visible |
| 0.3 | Add `db_queries` + `db_ms` to the request log line | Surfaces N+1s immediately |
| 0.4 | **Measure DB RTT from inside the app container** (§B1) | Decides whether §B1 is P0 or P2 |
| 0.5 | `SELECT pg_stat_reset();` to start a clean measurement window | Makes the index audit trustworthy later |
| 0.6 | Close the 6 leaked pgAdmin connections | Free |

### Phase 1 — Quick wins (2 days, very high impact)

| # | Task | Expected gain |
|---|---|---|
| 1.1 | **Delete the duplicate `ValidateSpreadsheetSecurity` call** (`upload_background.go:70`) | **Halves compare upload latency**, one line |
| 1.2 | Add the `ContainsAny` pre-filter + single combined regex to the scanner | **2.28 s → < 100 ms** per file |
| 1.3 | Tune `postgresql.conf` (§A5) and restart | Broad; removes sort spills |
| 1.4 | `Cache-Control: immutable` on `/uploads/*` and hashed `/static/*` | Removes daily revalidation of 19 assets |
| 1.5 | Replace `logo.png` (215 KB) with an SVG | 210 KB off every cold load |
| 1.6 | Self-host fonts, 10 weights → 4 | Removes a render-blocking third-party dependency |
| 1.7 | Hoist the CSP string to a `const`; skip request logging for `/static` + `/uploads` | Small CPU, large log-volume win |
| 1.8 | Pipeline the antiscrape Redis calls into one script | 4 Redis round trips → 1 on `/catalog` |
| 1.9 | Raise `pagecontrol` reload interval 20 s → 5 min | Removes 12 M rows/interval of scanning |

### Phase 2 — The catalogue (3 days, largest single measured win)

| # | Task | Expected gain |
|---|---|---|
| 2.1 | Rewrite `SearchProductIndex` as a staged indexed search (§A1) | **780 ms → ~5 ms** |
| 2.2 | Rewrite `SearchProducts` the same way + add the generated normalised column | **584 ms → ~10 ms** |
| 2.3 | Materialise `has_stock`; remove the `EXISTS` from `ORDER BY` (§A2) | Removes a full-table sort per listing page |
| 2.4 | Move the boot-time `UPDATE … branch_id` repair out of `main()` (§A7) | Faster, calmer deploys |
| 2.5 | Rewrite `trash.go` counts as one `UNION ALL` | Admin trash page from dozens of round trips to one |

### Phase 3 — Request path (3 days)

| # | Task | Expected gain |
|---|---|---|
| 3.1 | Batch `BEGIN` + `set_config` into one `SendBatch` (§B1a) | **−25% round trips platform-wide** |
| 3.2 | Add a no-transaction `Query` path for `AsSystem` reads (§B1b) | **−75% round trips** on public/reference reads |
| 3.3 | Cache `GetUserByID` in Redis (30–60 s, explicit invalidation) | −1 transaction per authenticated request |
| 3.4 | Cache `ListBranches` via `cache.Remember` | −1 transaction per customer request |
| 3.5 | Batch the two RBAC version reads; raise `versionTTL` to 30 s + `NOTIFY` | −8 round trips per user per 5 s |
| 3.6 | Add `singleflight` to `siteSettingsMiddleware` | Removes a cache stampede under load |
| 3.7 | Move `pagecontrol` `LISTEN` to a dedicated `pgx.Conn` | Returns 5% of the pool |

### Phase 4 — Uploads and images (5 days)

| # | Task | Expected gain |
|---|---|---|
| 4.1 | One `BeginUpload` entry point; `MaxBytesReader` on **all 25** sites; Makefile gate | Closes 15 unbounded-body endpoints |
| 4.2 | Zero-copy `os.Rename` instead of double-write in `saveUploadedFile` | Halves disk I/O per upload |
| 4.3 | Fix `openStoredUpload` to honour `GetUploadBaseDir()`, then stop carrying `fileBytes` into goroutines | Removes the memory-per-upload multiplier |
| 4.4 | Move all 11 `go func()` import sites onto River (`imports.stage` / `imports.commit`) | **Stops import CPU from starving request serving**; survives deploys |
| 4.5 | Image derivative pipeline (`media.derive` job): thumb / card / full in WebP | The largest image win available |
| 4.6 | One `ProductImage` templ component with `srcset`, `width`, `height`, `loading="lazy"`; Makefile gate on raw `<img>` | Removes CLS; browser picks the right size |

### Phase 5 — Frontend pipeline and CDN (4 days)

| # | Task | Expected gain |
|---|---|---|
| 5.1 | `extraJS` — move `maps`/`wallet`/`ads-wizard`/`import-progress`/`upload-progress`/`combobox` off the base layout | −67 KB JS (35%) on most pages |
| 5.2 | Split `components.css` (238 KB) along `@layer` boundaries; bundle per shell | 9 CSS requests → 2; large parse-time win |
| 5.3 | `cmd/assetbuild` minifier in the Makefile + `check-asset-budget` gate | −30% raw CSS, and it stays fixed |
| 5.4 | Pre-compress (gzip + Brotli) at startup in `initStaticAssetCache` | No per-request compression CPU; −15% bytes |
| 5.5 | Finish the S3/MinIO path; set `STORAGE_PUBLIC_BASE_URL` | Image bytes leave the app process |
| 5.6 | Cloudflare (free tier) in front of the bucket | Removes image traffic from the Egyptian last mile |
| 5.7 | Wire `.lighthouserc.json` into CI with a throttled mobile profile | Prevents regression |

### Phase 6 — Cleanup (2 days)

| # | Task |
|---|---|
| 6.1 | After a week of post-Phase-2 traffic, re-check `pg_stat_user_indexes` and drop what is still at `idx_scan = 0` |
| 6.2 | Slim `catalog.product_index` (§A3) — drop the duplicated search text columns |
| 6.3 | Index the two `managed_pages` foreign keys |
| 6.4 | Delete the dead `compare.MatchLadder` (only referenced by its own tests) |
| 6.5 | Set `GOMEMLIMIT` / `GOMAXPROCS` / container resource limits (§G2) |
| 6.6 | Raise Docker log rotation to `50m × 5` |

---

## 11. How to prove each fix worked

Do not ship any of this on faith. Each item has a cheap verification.

```bash
# --- Catalogue search (Phase 2) ---
# Before: ~780 ms. After: expect < 10 ms.
psql "$DATABASE_URL" -c "EXPLAIN (ANALYZE, BUFFERS) <the new SearchProductIndex query>"

# --- Security scanner (Phase 1) ---
go test -run XXX -bench BenchmarkValidateSpreadsheet -benchtime 10x ./internal/shared/filesecurity/
# Before: ~2.28 s/op on a 20k-row file. Target: < 100 ms/op.

# --- Round trips per request (Phase 3) ---
# After adding db_queries to the log line, hit the customer dashboard and read it back:
docker compose logs server --since 1m | grep '"path":"/dashboard"' | tail -1
# Before: expect 15-40. Target: < 8.

# --- Top queries by total time (after Phase 0) ---
psql "$DATABASE_URL" -c "
  SELECT round(total_exec_time::numeric,0) ms, calls,
         round(mean_exec_time::numeric,1) mean_ms, left(query,90)
  FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 20;"

# --- Index usage after the search rewrite (Phase 6 gate) ---
psql "$DATABASE_URL" -c "
  SELECT indexrelname, idx_scan, pg_size_pretty(pg_relation_size(indexrelid))
  FROM pg_stat_user_indexes
  WHERE relname IN ('product_index','products') ORDER BY idx_scan DESC;"
# idx_product_index_search_vector must no longer be 0.

# --- Page weight (Phase 5) ---
npx lighthouse https://<host>/catalog --preset=perf \
  --throttling-method=simulate --form-factor=mobile --view
```

---

## 12. Things that are already right — do not "optimise" these

To save the next person from breaking working code:

- **Bulk row writes.** `tx.CopyFrom` and `tx.SendBatch` are used correctly on every hot import path. Leave them.
- **The RLS tenant-isolation design** in `internal/platform/database`. The `SET LOCAL` approach is correct and the `AsSystem` escape hatch is properly greppable. The *round-trip cost* is the problem, not the design.
- **The multipart memory-vs-size distinction** documented in `upload_handlers.go`. The reasoning is right; it is the inconsistent *application* across 25 sites that needs fixing.
- **`matchflow` ceilings.** The `MinPlausible` gate and the per-profile budgets are well-reasoned and were clearly derived from real production runs. The AI stage is slow because LLM matching is slow, not because the code is wrong.
- **Static asset ETag + 304 handling** in `static.go`. Correct. It just needs `immutable` and pre-compression.
- **`antiscrape` being mounted on only two routes.** The comment explaining that choice is right.
- **The progress hub / Redis bridge / SSE** design. Correct architecture for live progress.
- **The CSS quality gates in the Makefile.** They have clearly prevented real regressions. Add a *size* gate beside them; do not remove the quality ones.

---

## 13. One-paragraph answer, if you only read one thing

The platform is slow because a catalogue search does a 780 ms full table scan while an 8.8 MB search index sits unused; because every spreadsheet upload runs a full-workbook regex scan twice, costing 2.3 seconds per file per pass, inside the HTTP request; because every database read costs four network round trips and nothing but one taxonomy service is cached; because product images are served at original resolution from local disk through the Go process with no CDN, no resizing and no width/height attributes; because 385 KB of unminified CSS and 188 KB of JavaScript load in nineteen requests on every page along with ten Google Fonts weights; and because heavy import work runs as detached goroutines inside the web server instead of on the River queue that is already deployed and sitting empty. The security systems are not the cause — with the one exception of the spreadsheet scanner, which can be made twenty times faster without weakening its detection at all.
