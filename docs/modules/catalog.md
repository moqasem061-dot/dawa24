# Module: Product Catalog

## Overview

The `catalog` bounded context manages pharmaceutical products, packaging variants, hierarchical categories, brands, and Arabic trigram search.

## Schema Mapping

- **PostgreSQL Schemas:** `catalog`
- **Migrations:** `004_catalog.up.sql`, `083_saving_products.up.sql`, `084_employee_institutional_works.up.sql`, `085_product_index.up.sql`
- **Tables Owned:**
  - `catalog.categories` — Hierarchical product taxonomy with bilingual names.
  - `catalog.brands` — Pharmaceutical manufacturers and brands.
  - `catalog.products` — Tenant-owned master product records with full 34-column legacy parity (`dosage_form`, `scientific_name`, `pharmacology`, `active`, `concentration`, `unit`, `price`, `discount`, `institutional_work_ids`).
  - `catalog.product_variants` (renamed from legacy `product_childerns`) — Specific packaging, concentration, or SKU variations.
  - `catalog.product_infos` — Legacy 5-column key-value attribute bag (`id`, `product_id`, `info_key`, `info_value`, `created_at`).
  - `catalog.product_index` — High-performance 25-column denormalized read model for faceted search and instant product discovery.
  - `catalog.customer_product_mappings` — One per-customer pricing/mapping table (071): vendor-set pricing (`price`, `discount` percent, `customer_org_id`) and import rows (`raw_name`, `branch_id`, `source` `excel|csv|link|manual`, `status`).
  - `catalog.saving_products` — Customer/vendor savings tracking lists (reinstated in 083 per Plan V5 Phase 0 Task 0.6.1).

## Invariants & Rules

1. **Row Level Security & Marketplace Cross-Tenant Read:** Enforced with `FORCE ROW LEVEL SECURITY` on all tenant tables using `organization_id = app.current_tenant`. `catalog.product_index` has RLS enabled, and customer-facing discovery queries explicitly execute under `database.AsSystem` with code-level comments documenting that cross-tenant catalogue read is the explicit invariant of a multi-vendor marketplace.
2. **Name Collision Invariant (Plan V5 Phase 1 Task 1.2):**
   - Laravel's `product_infos` is a 25-column denormalized read model (also backing `all_products` view and `product_search_index`).
   - In Go, this read model is named `catalog.product_index` to prevent collision with Go's pre-existing 5-column `catalog.product_infos` key-value attribute bag.
3. **Deterministic Fallback (Rule R3 Spirit):** If the `catalog.product_index` read model is empty or stale, `catalog.Service.FastSearch` automatically falls back to querying `catalog.products` directly rather than returning an empty result.
4. **Read Model Carries Variants and Stock (repaired 2026-08-25):**
   - `catalog.product_index` holds **both** parent rows (`product_type='parent'`)
     and vendor variant rows (`product_type='variant'`). Without the variant half
     the read model cannot answer "who sells this", which is the question the
     marketplace is built around.
   - `variant_id` and `stock_quantity` were previously written as the literal
     `NULL` and `0` for every row. On the live database that produced 28,786
     indexed products of which zero had a variant and zero had stock, while
     `inventory.stocks` held 14,539 real rows — so any query of the documented
     form "active products with `stock_quantity > 0`" returned nothing at all,
     silently. See `internal/modules/catalog/jobs/reindex_sql.go`.
   - **Availability must still not be read from this table.** It is excellent at
     finding a product by name; the authority on whether anyone has it is
     `catalog.product_variants` joined to `inventory.stocks`. `smartorder` reads
     the authoritative tables for exactly this reason.

8. **Asynchronous Read Model Synchronization:**
   - Background updates are dispatched as River worker jobs (`catalog.reindex` via `internal/modules/catalog/jobs/reindex.go`) on product/variant/stock/branch mutations.
   - Batch full-rebuilds are executed via `dawa24 cli reindex` or `RebuildProductIndex`.
5. **Arabic Trigram & Fulltext Search:** Indexed with `pg_trgm` GIN indexes over `search_simple` (generated via `internal/shared/arabic.Normalize`) and Postgres `tsvector` over `(search_text, search_simple)`.
6. **Monetary Precision:** All prices and discounts are stored and computed via `money.Amount` (minor unit integer arithmetic).
7. **Saving Products Decision (Task 0.6.1):** `catalog.saving_products` is reinstated as a distinct customer/vendor-owned list of products with quantity and price used for savings tracking and procurement optimization, distinct from time-boxed promotional discount offers.

## Endpoints

- `GET /api/v1/catalog/search?q={query}&category_id={id}&brand_id={id}` — Fuzzy Arabic product search.
- `GET /api/v1/catalog/products/{id}` — Retrieve product details with all active variants.
- `POST /api/v1/catalog/products` — Create new product for current tenant.
- `POST /api/v1/catalog/products/{id}/variants` — Create new variant under product.
- `GET /api/v1/catalog/categories` — List taxonomy categories.
- `GET /api/v1/catalog/brands` — List manufacturers and brands.

## The master-catalogue import wizard

`/admin/products/import` — four steps, in `internal/ui/admin_import_*.go`,
`internal/ui/pages/admin_import_*.templ`, and `internal/modules/catalog/import_*.go`.

| Step | Route | What it does | Writes |
|---|---|---|---|
| 1 · upload | `POST /admin/products/import` | `AnalyzeImport`: decode the sheet, find its blocks and header, describe every column | one `catalog.import_sessions` row, status `draft`, plus the file |
| 2 · map | `GET/POST .../{id}/mapping`, `POST .../{id}/preview` | `PreviewImport`: re-read under the admin's bindings, show the first 25 products | the session's `structure`, `options`, `layout_overrides` |
| 3 · review | `POST .../{id}/prepare` → `GET .../{id}` | `PrepareImportAsync`: parse, match, stage | `catalog.import_staging_rows`, status `processing` → `ready` |
| 4 · commit | `POST .../{id}/commit` | `CommitImport` | `catalog.products`, status `ready` → `committed` |

Nothing before step 4 touches the catalogue. Steps 1–3 are re-runnable as often
as the admin likes.

### Invariants

- **The mapping step is not skippable.** Step 1 must never start processing.
  A run against a mapping nobody has seen is how an import of nine thousand
  products arrives as a review screen full of zeros with nothing to explain it.
  `AdminProductsImportReviewPage` redirects a `draft` session back to `/mapping`
  for the same reason: an unprocessed session is not an empty result.
- **`session.status` is the authority on what an import is doing.**
  `ProgressTracker` is a per-process convenience and its `Progress` reports
  whether an *entry exists*, not whether a run is live. Only a non-terminal live
  snapshot may override the row (`SessionProgress`).
- **Staging rows land before the status does.** `prepare` writes
  `ReplaceStagingRows` while the session is still `processing`, then flips to
  `ready`. The reverse order left a window in which the review screen, the
  progress poll and a commit all saw a finished import with an empty table.
- **One read per render.** `renderImportReview` takes the session once, through
  `SessionProgress`, and decides everything from it. Two reads let a run
  finishing between them render a progress bar for work that was already over.
- **`UseAI` is off by default** (`DefaultImportOptions`). An unaudited import
  must be judged on the deterministic engine's own match rate; the switch sits
  on the mapping screen beside everything else that changes the outcome.
- **A failed run is recoverable, not a dead end.** `IsRetryable` includes
  `failed`: the upload is still on the row and the usual cause is a mapping that
  read nothing, so the route out is `/mapping`, not another upload.
- **The mapping screen never re-decodes the workbook to draw itself.** The
  reading is stored on the session as `structure` (see `catalog.FileStructure`),
  and `sheetCache` holds the decoded sheet for fifteen minutes so successive
  previews are a map lookup rather than a 32 MB read plus a full decode.

### Known legacy quirks preserved

- A row with an identifier but no name is imported under `صنف دوائي #<code>`
  rather than dropped. Losing stock a pharmacy owns is worse than a bad label.
- A file whose header repeats every N rows (a print-paginated ERP export) is read
  as N blocks, each through its own header. `AnalyzeLayout` documents why.
