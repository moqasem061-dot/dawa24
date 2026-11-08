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

## Admin catalogue search and import screens (2026-10-09)

- **One filter for rows and total.** `/admin/products` ran two separately
  written predicates: the page had the first-word branches and trigram operators,
  the pager's count had a `regexp_replace` branch the page lacked. A total could
  therefore count rows the pages never list, and the last pages came up empty.
  `productFilterSQL` (`internal/modules/catalog/postgres/search.go`) is now the
  only WHERE body; the page adds `LIMIT $13 OFFSET $14`. `search_test.go` pins
  the placeholder contract (twelve values, same order).
- **Literal text.** Raw admin input is matched with `strpos(lower(…), lower(…))`
  rather than `ILIKE`, because in an `ILIKE` pattern the typed `%` and `_` are
  wildcards and `%` listed the whole catalogue. Normalised Arabic keeps `ILIKE`,
  since normalisation removes those characters.
- **Barcode and SKU** match as substrings, as the count already did and as the
  vendor search does. The page used to match them only exactly.
- **Dosage-form filter** is cached for 60 seconds (`dosageFormsCache`), like the
  brand and category lists. It groups the whole products table on each read.
- **Import upload screen** reads only the categories (`ImportCategories`). It
  previously loaded the brand and dosage vocabularies, which scan the products
  table, and resolved the master organisation, which can create one. Only the
  enrichment pass still uses `ImportVocabulary`.

The changed SQL has not been run against a database in this change. It is
covered by the placeholder and wildcard tests above. An `EXPLAIN ANALYZE` of the
page and count against production-sized data is still needed before calling the
admin listing fast.

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

## Master import

The master catalogue import wizard (`/admin/products/import`) protects canonical catalogue data by staging rows, classifying mutations, and enforcing human review before any commit. Four steps, in `internal/ui/admin_import_*.go`,
`internal/ui/pages/admin_import_*.templ`, and `internal/modules/catalog/import_*.go`.

### Architecture & Matching Flow

1. **Deterministic Preparation (`internal/modules/catalog/import_match_chunks.go`):**
   - Matching delegates directly to `matching.Service.Prepare` (`WorkflowCatalog`, required granularity `product`).
   - Staging table `catalog.import_staging_rows` stores consumer columns (migration `237_catalog_staging_match_decision.up.sql`): `match_state`, `match_basis`, `match_granularity`, `match_revision`, `human_locked`, `ai_state`, `match_run_id`, `match_decision`, `baseline_decision`, and `proposal`.

2. **Classification Logic (`internal/modules/catalog/import_classify.go`):**
   - `accepted` => proposal `update`: matches an existing canonical product with verified product identity.
   - `no_candidate` => proposal `possible_new`: no match found; labelled explicitly as "possible new" (never "new") to indicate unreviewed status.
   - $\ge 2$ catalogue candidates with identical identity facts => proposal `duplicate`: signals multiple identical records in the master catalogue requiring administrative resolution.
   - Otherwise => proposal `unresolved`: partial matches, soft/hard conflicts, or omitted attributes (`needs_review`) require manual operator attention.

3. **Placeholder Name Guard:**
   - Rows lacking a product name are assigned a placeholder (`صنف دوائي #<code>`).
   - Placeholder names are marked `Prov=inferred` and stripped of name evidence during matching to prevent phantom cross-row matches based on generic labels.

4. **Rerun Superseding:**
   - Any column-mapping change or prepare rerun increments `session.Options.MatchRevision`.
   - The prepare pipeline replaces all staging rows via `ReplaceStagingRows`, overwriting previous decisions at the new revision.
   - The canonical master catalogue (`catalog.products`) remains completely untouched across reruns until explicit commit.

5. **Pre-Commit Review & Diff Inspection:**
   - The review UI (`internal/ui/pages/admin_import_review.templ`) highlights proposed actions and proposal badges.
   - For `update` rows, the review interface renders:
     - Canonical identity diffs: side-by-side comparison of old vs new values for barcode, scientific name, dosage form, concentration/strength, unit, and manufacturer (`RowIdentityDiffs`).
     - Affected associations count: displays the count of active variants (`catalog.product_variants`) linked to the product being overwritten.

6. **Commit Worker Revalidation:**
   - Commit worker (`internal/modules/catalog/importjobs/commit_worker.go`) performs pre-commit revalidation (T060 rule) for every row before persisting:
     - Rows must have state `accepted` or `confirmed` with achieved granularity `product` or `pack`.
     - Rows with revision mismatches, unconfirmed states, or `catalog_changed` flags are refused from the commit batch.

### Steps

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

One conflict model decides identity: `conflictsOf` (`internal/shared/productmatch/discriminate.go`), extended for Egyptian naming in `identity_egypt.go`, `identity_brand_guard.go`, `shorthand.go` and `typo.go`. The scorer ranks with it and the gate (`hard_conflicts.go`) accepts with it, so the two cannot disagree. Every acceptance path uses it: identifier, knowledge, reviewer and AI (`IdentityConflict`).

- **Hard (never accepted):** strength, combination components, line extension, form or sub-form, identity letter (including brand numbers such as كيو-90, 50+, B12), route, size, and a known brand the candidate does not carry.
- **Soft (ranks the right candidate first, sends an only-candidate to review):** count, bottle volume, unstated extension or letter, an unknown brand word, a container difference (امبول/فيال, tie-break only), a word mismatch, a different company inside a generic's name.
- **Tablets and capsules are one solid-oral class.** Egyptian pharmacists write them interchangeably; tablet and capsule counts are compared as one class.
- **Shorthand** (ق، ك، ش، ف، امب، اق) is expanded before matching.
- **Loose brand links** (typo, sound-alike, cross-script) need corroboration from the row's dose, form, count, company, molecule or price. Cross-script links need one strong confirmation or two weak ones. Accepted loose links are flagged `loose_brand`.
- **What the AI is asked.** Every row except those already answered by the Match Decision Memory (a person's, a curator's or the platform's earlier decision, or a reviewer's) or by a checked identifier: unresolved rows to resolve, and every name match, full-score ones included, to check. The model is not told the engine's choice, so it cannot echo it. If it names another product, or none, for a matched row, the row goes to review; a confident disagreement never silently swaps the product. Every AI answer passes the same identity screen (`IdentityConflict`) as the engine's own.
- **Barcodes** must pass the GTIN check digit. Supplier codes must be corroborated by the name.

Measured results and the evaluation method: `docs/modules/matching-baseline.md` (`go run ./cmd/cli match-eval --split egypt|tuning --explain <file>`).

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
   - Rows are rejected only when their chosen product changed in the catalogue, their state is no longer accepted or confirmed, or their granularity is below what the decision required.

5. **Entry-Point Registry (`internal/modules/catalog/matching/entrypoints.go`):**
   - `savings_matching`: `internal/modules/catalog/saving_match_engine.go`
   - `savings_matching_svc`: `internal/modules/catalog/saving_matching.go`
   - `savings_commit_revalidate`: `internal/ui/saving_products_import_commit.go`
   - `savings_ai`: `internal/ui/saving_products_ai.go`

## Matching Data Retention & Purge Policy (US9 T129, US13 T157, research R14)

1. **30-Day Retention for AI Attempts and Shadow Outcomes:**
   - Detailed execution records in `catalog.match_ai_attempts` (token counts, request SHA-256 hashes, diagnostic excerpts, and compact answers) and `catalog.match_shadow_outcomes` (shadow policy comparisons) are retained for **30 calendar days**.
   - A daily automated River maintenance job (`catalog.matching.purge_attempts` / `matchingJobs.NewPurgeAttemptsWorker`) runs system-wide (`database.AsSystem`) to purge rows older than 30 days.
   - Long-term decision outcomes, human reviewer corrections, and audit logs (`catalog.match_policy_assignment_log`, `catalog.match_knowledge`) are preserved permanently as compliance records.

2. **Production Shadow Mode Sign-off:**
   - **Requirement**: Do NOT enable shadow matching mode in production environments until formal approval is signed below.
   - Product owner approval: ____________________ date ____________________

## Match decision memory: who writes it (2026-11-02)

The platform keeps the answers it has already paid for in one writable table and
one retired one, and conflating them is how a supplier's memory page came to be
empty after an upload.

- **`catalog.match_knowledge` — the memory.** One row per organisation per
  question, with the question's *signature* (`productmatch.QuestionSignature`
  over the row's own facts), a kind (`human_confirm`, `human_reject`,
  `curated_alias`, `platform_fact`, `ai_answer`), and the product it points at.
  `matching.Service` reads it before deciding anything (`KnowledgeResolver`) and
  is the only writer (`Remember`). `/admin/match-decisions` curates it;
  `/vendor/decision-memory` and `/customer/decision-memory` list it.
- **`catalog.match_decisions` — the retired ledger.** Read-only since US8 T114
  (`test/match_decisions_readonly_test.go` fails the build on any `INSERT` into
  it outside migrations). It is still read (`LookupDecisions`) so rows the ETL
  and `cli match-knowledge-backfill` left there keep working, and it is what the
  memory screens fall back to for an organisation whose decisions predate
  knowledge.

**What was broken.** Both import pipelines — the vendor catalogue upload
(`internal/modules/ingest`) and the smart order (`internal/modules/smartorder`) —
were handed a repository as their decision memory, and that repository's
`SaveDecisions` is a documented no-op (the ledger is read-only). The comment on it
said writes "route through `matching.Service`", but nothing did the routing for
those two pipelines: only the catalog import and the saving list got the
`matchflow.Memory` adapter. So:

- an answer the vendor's upload paid for was applied to the row and filed
  nowhere — the supplier's ذاكرة قرارات المطابقة stayed empty;
- the next upload of the same question paid the model again;
- a row a person matched by hand was forgotten the moment the import ended.

**What happens now** (`cmd/server/pipeline_memory.go`). Two adapters, wired at the
composition root — the only place allowed to know both an import module's
vocabulary and the matching service:

1. `LookupDecisions` and `SaveAlias` are the repository's (the legacy read and the
   alias ledger, both legitimate).
2. `SaveDecisions` goes to `matching.Service.Remember`, i.e. into
   `catalog.match_knowledge`, under the pipeline's own workflow namespace
   (`vendor`, `smart_order`) so `KnowledgeResolver` finds it on the next run and
   not in another tool's answers.
3. Only real memory is filed: a person's manual match becomes `human_confirm`
   (highest precedence), an accepted model answer becomes `ai_answer` (lowest,
   and inert while AI is off). Engine-accepted rows are **not** filed: the engine
   is deterministic, so an answer filed for one of them would be a remembered
   guess that outranks every future engine's evidence — permanently.
4. An answer with no signature is dropped rather than stored under its cache key,
   which nothing would ever look up. The signature travels on the decision
   (`CachedDecision.Signature`), computed where the parsed row is still in hand.
5. No organisation in scope means no write: a row without a tenant would be
   visible to every tenant.

Covered by `internal/modules/ingest/catalog_memory_write_test.go` (the stage hands
over signatures), `cmd/server/pipeline_memory_test.go` (the routing: kind,
namespace, signature, tenant), and against a real database
`test/integration/decision_memory_recall_test.go` — which files a decision exactly
as the import does and asserts the **next** `Prepare` returns it accepted on
`knowledge_human` basis, plus the cross-tenant read, which must return nothing.
## Catalog query work (2026-10-09)

Master-catalog administration and vendor linking opt out of stock-priority
sorting: these screens manage or identify products, so they do not need a
correlated stock existence check for every search result before pagination.
Buyer browsing retains its availability behavior.

Buyer catalog sponsorship ranking aggregates active, approved, unexpired
promotions once by product/variant target. The two joins share that aggregate,
using the highest applicable tier. Overlapping promotions cannot duplicate
buyer offers or change pagination totals. The repository integration fixture
covers duplicate variant promotions, a product promotion and an expired tier.
This change preserves buyer coverage, institutional-work and tenant predicates.

The admin import upload page fetches its category chooser and recent-session
history concurrently, keeping these independent reads off each other's latency path.

Saving-products import session reads return snapshots of mutable progress
metadata and editable row fields under the store lock. This fixes a race between
background completion and HTTP progress/review reads, detected by the race suite.

## Vendor sale-price input (2026-10-23)

Vendor product creation (editor, custom modal and central-catalog modal) and
editing accept a sale-price input alternative to a percentage. The server
recomputes the stored discount from the submitted public and sale prices;
client-computed percentages are never authoritative. Invalid price pairs and
unknown input types are refused. Both the percentage and the money preview use
exact decimal arithmetic; browser calculations use integer minor units.
Discounts retain the existing two-decimal percentage precision.

## Saving products tenant policy (audit batch 7, migration 254)

`catalog.saving_products` policies now use `platform.tenant_visible(organization_id)`
like every other tenant table. The former policy read `app.current_tenant`, which
the application never sets, so owners saw none of their own rows. Code paths that
use `database.AsSystem` now see every row, as the design intends.

## ERP sync quota writer

`SetVariantQuotaLimits` is called by the ERP product sync. It writes `quota_limit` only — one bulk `UPDATE` per call inside one transaction; `nil` (or a normalised 0/negative) clears the cap, and an empty map writes nothing. Variant ids outside the caller's organization are left untouched by the WHERE clause.
