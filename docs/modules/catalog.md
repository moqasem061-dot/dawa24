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

## Fixed 2026-09-27: vendor central-catalog picker

- If a complete multi-word query has no direct matches, catalogue search retries
  the distinct words and ranks products by how many words matched. Search
  filters and master-catalogue visibility stay in force.
- The vendor picker cancels stale requests, reports search failures distinctly,
  and copies both canonical names plus SKU, barcode, and unit from the selected
  master product. Changing the selection clears its hidden fields.
- A repeated add for a master product already carried by that vendor is
  idempotent. If another vendor item owns the master SKU, the new variant gets
  a deterministic vendor SKU based on the master product ID, avoiding the
  organization-wide unique-index collision.
5. **Arabic Trigram & Fulltext Search:** Indexed with `pg_trgm` GIN indexes over `search_simple` (generated via `internal/shared/arabic.Normalize`) and Postgres `tsvector` over `(search_text, search_simple)`.
6. **Monetary Precision:** All prices and discounts are stored and computed via `money.Amount` (minor unit integer arithmetic).
7. **Saving Products Decision (Task 0.6.1):** `catalog.saving_products` is reinstated as a distinct customer/vendor-owned list of products with quantity and price used for savings tracking and procurement optimization, distinct from time-boxed promotional discount offers.

## Image import stability (2026-09-30)

Product-image uploads publish fully initialized sessions. Column-mapping changes
use the session store lock; progress polling and session listings receive
snapshots rather than live pointers. Concurrent image workers publish copies of
their completed-row slice, so readers cannot race with later worker updates.
The existing SKU and barcode upload flows pass with the race detector enabled.

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

### Fixed 2026-09-25: a second import crashed the server

- **Symptom.** The first import into an empty catalogue always succeeded; the
  next one stalled at the start of matching ("0 / 8,790", ~60%), fell back a few
  points on refresh, and was eventually failed as "interrupted by a restart".
- **Cause.** `productmatch.foldPackMultipliers` took the left factor's offset
  from its input and sliced the rebuilt output with it. After one fold the two
  differ in length, so any name whose text holds a multiplication twice panicked
  — including a single `3×1`, because the matcher reads the Arabic and English
  names joined and the English falls back to the Arabic. 38 of the 8,790 rows in
  the reported file did this. Matching only runs against a non-empty catalogue,
  which is why the first upload never hit it. It ran on `MatchAll` worker
  goroutines, where the prepare goroutine's `recover` cannot reach, so the whole
  process died. Every tool using the matcher was exposed (vendor ingest, smart
  order, compare, saving products).
- **Fix.** The factor is read from the output buffer (chains such as `2*3*4`
  now fold to 24), factors past `maxPackUnits` are refused before multiplying,
  and `MatchAll`/`RecallAll` recover per row: a row that panics comes back
  `MatchNone` and goes to human review. Regression:
  `TestSecondImportSurvivesMultipliedPackCounts`, `TestFoldPackMultipliers`,
  `FuzzFoldPackMultipliers`.
- **Heartbeat.** The session row is now re-written every `progressHeartbeat` by
  a ticker (`import_heartbeat.go`), not only when the pipeline reports, so a
  long silent stretch can no longer look orphaned to another replica.
- **Screen.** The step list follows live snapshots (it stayed on the phase the
  page was rendered in), and the failed card shows the recorded reason.

## Product identity rules

1. **Hard vs Soft Conflicts (contracts/identity-gate.md, SC-002):**
   - **Hard Conflicts**: Unconditionally veto candidate acceptance on every path (including identifier, exact, similarity, and human reviewer until corrected). Any hard conflict yields `Eligible=false` and state `needs_review`:
     - `strength`: different active ingredient dosage (e.g. 500 mg vs 1000 mg).
     - `strength_component`: combination components evaluated by position when component association is known.
     - `combination`: multi-component medicine differences (a row with fewer components is unstated, row with more is a hard conflict).
     - `modifier`: differing product line modifiers (e.g. Extra vs Plus).
     - `modifier_unstated_in_row`: row omits a line extension that the candidate product possesses. **Hard only when a plain sibling exists in the index**; if the extended product is the sole family member in the catalogue, it remains `unstated`.
     - `form`: differing pharmaceutical forms (e.g. syrup vs tablet, ointment vs cream).
     - `route`: differing administration routes (e.g. eye drops vs ear drops).
     - `release`: modified release profile (e.g. SR/XR/CR/retard) vs immediate release.
     - `letter`: distinguishing product suffix letters (e.g. Betnovate N vs Betnovate C).
     - `concentration`: exact rational fraction mismatch (`math/big.Rat`) for concentrations (mg↔g and mL↔L conversions normalized; percent and IU never converted).
     - `dimension`: device size or physical dimension mismatch.
     - `brand`: differing rare brand tokens (`df` <= 3); alternative mentions ("Product A, alternative to Product B") are never accepted as substitutions.
     - `identifier_vs_description`: an identifier pointing to a product whose identity attributes contradict the description.
     - `pack`: package count/structure conflict (enforced as a hard conflict only when `RequiredGranularity == Pack`).
   - **Soft Statuses**: `unstated` indicates missing information and never counts as agreement. Agreement requires positive corroboration.

2. **Required Granularity (`product` vs `pack`):**
   - `RequiredGranularity = Product`: Validates molecular identity, strength, form, release, route, and brand. Packaging count differences are soft.
   - `RequiredGranularity = Pack`: Requires both product identity and exact packaging alignment (`Count`, `Multiplier`, `SubCount`, `SubUnit`).
   - Structured multi-packs (e.g. `2×10` vs `20`) yield `needs_relationship` unless an explicit entry exists in `catalog.pack_relationships`. Pack conversion (`Convert`) uses exact integer arithmetic only.

3. **Curated Identity Overrides:**
   - Platform-curated facts in `catalog.curated_identity_facts` override parsed facts with `Prov=corrected`.
   - Applied at index construction (`NewIndexWithCuration`).
   - Statement triggers on `catalog.curated_identity_facts` and `catalog.pack_relationships` increment `catalog.catalog_revisions`, ensuring zero-downtime, automated singleflight cache invalidation.

4. **Tablet vs Capsule Distinction (Intentional Behaviour Change):**
   - Explicit tablet (`tablet`, `tab`, `أقراص`, `قرص`) vs explicit capsule (`capsule`, `cap`, `كبسول`, `كبسولات`) is an intentional hard conflict (`ConflictForm`).
   - Previously collapsed into `solid_oral`; separated to preserve clinical accuracy in B2B orders.
   - Either side omitting the form is treated as `unstated` (not a conflict).
   - Ordinary tablets vs Orally Disintegrating Tablets (ODT / discmelt / ذائبة) remains an unconditional conflict.

## Savings import

The savings products import (`catalog.saving_products`) enables pharmacies, vendors, and administrators (acting on behalf of target organisations) to import wishlists and deficit lists with pricing and quantity.

### Architecture & Matching Flow

1. **In-Module Catalog Matching Engine (`internal/modules/catalog/saving_matching.go`, `saving_match_engine.go`):**
   - Matching logic lives inside the `catalog` module rather than the UI layer. `internal/ui` delegates matching to `catSvc.MatchSavingsRows`.
   - Delegates directly to `matching.Service.Prepare` when available, with deterministic fallback to `SavingProductMatchEngine` (in-memory candidate index).
   - Identifiers come strictly from declared column mappings: a mapped column is never simultaneously assigned to both SKU and barcode tiers. Disambiguation (`isBarcodeCandidate`) routes pure GTINs to barcode and alphanumeric codes to SKU.

2. **Effective Organization Scoping:**
   - Every savings operation (upload, column mapping, matching, manual review linking, item toggles, and commit) derives `Scope.EffectiveOrg` from the target organization being imported for.
   - On administrator-assisted paths (`/admin/organizations/import/{orgID}/...`), `Scope.EffectiveOrg` is strictly the target `{orgID}` from the URL route, never the administrator's own session organization (`actor.OrganizationID`).
   - Tenant isolation tests (`TestAdminAssistedScope`, `TestSavingProductsConsistency_EqualContextAcrossEntryPoints`) prove zero-leakage between admin org X and target org Y.

3. **Pre-Commit Change Preview:**
   - Staged rows updating existing `catalog.saving_products` records attach a `SavingChangePreview` delta view model (`internal/ui/pages/saving_import_types.go`).
   - The review screen (`saving_import_review.templ`) displays old vs new product names, pack/unit changes (`OldPackUnit` → `NewPackUnit`), quantity adjustments, and exact monetary delta (`money.Amount` formatted, zero `float64`).

4. **Commit Revalidation (`internal/ui/saving_products_import_commit.go`):**
   - Before writing to `catalog.saving_products`, `RevalidateSavingItems` calls `catSvc.RevalidateSavingDecision` for every row.
   - Rows whose `match_revision` changed, whose decision state is unconfirmed or not accepted, or whose granularity is below `pack` or `product` are rejected before committing.

5. **Entry-Point Registry (`internal/modules/catalog/matching/entrypoints.go`):**
   - `savings_matching`: `internal/modules/catalog/saving_match_engine.go`
   - `savings_matching_svc`: `internal/modules/catalog/saving_matching.go`
   - `savings_commit_revalidate`: `internal/ui/saving_products_import_commit.go`
   - `savings_ai`: `internal/ui/saving_products_ai.go`



