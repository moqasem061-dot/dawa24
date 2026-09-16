# PHASE 2 — The Discount & Price Comparison Engine

**Depends on:** Phase 0 (error handling, permissions), Phase 1 (`catalog.product_index`, Arabic normaliser, institutional filter).
**Blocks:** nothing structurally, but it is the platform's advertised flagship.
**Tasks:** 6.
**Size:** the largest single feature in the plan. Budget accordingly.

## Why this phase exists

The `/what-in` page — the platform's own product spec — names this the flagship
capability and grants it to **all three roles**:

> "🌍 محرك مقارنة الخصومات والأسعار الذكي (AI Compare Engine)" — four steps:
> spreadsheet upload with auto-archiving (8 active files for customers, 22 for
> vendors) → manual column mapping (auto-recognising Arabic and foreign column
> names) → supplier-vs-supplier comparison → AI drug-name matching to 99%
> accuracy across spellings and Arabic/English variants.

**What exists in Go today:** `internal/ui/compare_handlers.go`, 75 lines, three
functions. It renders a pricing page, takes a subscription, then renders
`CompareToolPage(lang, dir, entitled)` — a template that receives **no data**
and has no upload. `grep -r compare_discount` across `internal/` and
`db/migrations/` returns nothing.

**You can currently charge a customer for a feature that does not exist.**

**What exists in Laravel:**

| Artefact | Size |
|---|---|
| `app/Livewire/Employee/CompareDiscounts.php` | 2,008 lines |
| `app/Livewire/Customer/CompareDiscounts.php` | 1,863 lines |
| `app/Livewire/Employee/MarketDiscounts.php` | 263 lines |
| `Employee/CompareDiscountsMarketing`, `CompareDiscountsPlan`, `CompareDiscountsPlanRequest`, `CompareDiscountsShowPlan`, `UploadGlobalDiscounts` | — |
| `Customer/CompareDiscountsMarketing`, `Customer/MarketDiscounts` | — |
| `app/Services/ProductMatcher.php` | 939 lines |
| `app/Services/ColumnDetector.php` | 814 lines |
| `app/Services/ArabicNormalizer.php` | 127 lines (ported in Phase 1) |
| 6 database tables | — |

---

## Scope decision (make this explicit before starting)

Ship in **two waves**. Wave A is fully usable without any AI.

| Wave | Contents | Gate |
|---|---|---|
| **A** | tables, plans, upload, archive policy, manual column mapping, deterministic matching, supplier-vs-supplier comparison, market comparison, results UI | Task 2.1–2.5 |
| **B** | AI-assisted name matching + AI search-term expansion, both as *enhancements* over Wave A's deterministic matcher | Task 2.6 |

Rule R3 requires the deterministic fallback anyway, so Wave A **is** the
fallback. Building it first means the feature works even if Wave B is delayed.

---

## TASK 2.1 — Compare-plan tables and entitlements

Resolves `PARITY_AUDIT_V4.md` §8.2 and the deferral recorded in Phase 0 Task 0.6.3.

### 2.1.1 Inspect first

```bash
for t in compare_discount_plans compare_discount_plan_features \
         compare_discount_plan_requests compare_discount_plan_subscriptions \
         compare_discount_plan_subscription_users compare_discount_user_sessions; do
  echo "=== $t ==="
  sed -n "/CREATE TABLE \`$t\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
done
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/CompareDiscountsPlan.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/CompareDiscountsPlanRequest.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Employee/CompareDiscountsShowPlan.php
cat F:/Dawa\ 24/Laravel/app/Services/SessionService.php
```

### 2.1.2 Migration

`db/migrations/087_compare_plans.up.sql` / `.down.sql`. New schema: **`compare`**.

Six tables, mirroring Laravel:

**`compare.plans`**
`id`, `public_id`, `name JSONB` (`{"ar":…,"en":…}`), `slug UNIQUE`,
`description JSONB`, `price_monthly NUMERIC(12,2)`, `price_yearly NUMERIC(12,2)`,
`price_lifetime NUMERIC(12,2)`, `currency TEXT DEFAULT 'EGP'`,
`trial_days INTEGER DEFAULT 0`, `is_active`, `is_public`, `is_recommended`,
`sort_order`, `created_by`, `updated_by`, timestamps, `deleted_at`.

> `currency` exists in Laravel but `money.Amount` is single-currency (rule R1).
> Keep the column for parity, default `'EGP'`, and **do not** build multi-currency
> logic on it. Record this in `docs/modules/compare.md`.

**`compare.plan_features`**
`id`, `plan_id → compare.plans`, `key TEXT`, `name JSONB`, `description JSONB`,
`value TEXT`, `value_type TEXT CHECK (value_type IN (…))` — read the Laravel
enum values from the dump — `is_active`, `sort_order`, `created_by`,
`updated_by`, timestamps, `deleted_at`.

The archive limits live here as feature rows. Seed at minimum:
- `max_active_files` = `8` for the customer-facing plan, `22` for the vendor plan
  (**verify these numbers against Laravel's seeder** — `/what-in` states 8 and 22)
- `max_concurrent_sessions`
- `ai_matching_enabled`

**`compare.plan_requests`** — a vendor/customer requests a plan; admin approves.
`id`, `public_id`, `plan_id`, `organization_id`, `user_id`, `status`
(pending/approved/rejected — read Laravel's enum), `notes`, `reviewed_by`,
`reviewed_at`, timestamps, `deleted_at`. RLS on `organization_id`.

**`compare.subscriptions`** — `id`, `public_id`, `plan_id`, `organization_id`,
`user_id`, `billing_period`, `starts_at`, `ends_at`, `status`, `seats INTEGER`,
timestamps, `deleted_at`. RLS.

**`compare.subscription_users`** — seat assignment.
`id`, `subscription_id`, `user_id`, `is_active`, timestamps, `deleted_at`.

**`compare.user_sessions`** — the device cap advertised on `/what-in`.
`id`, `public_id UUID`, `subscription_user_id`, `user_id`, `session_id`,
`is_active`, `device_name`, `device_type`, `platform`, `platform_version`,
`browser`, `browser_version`, `ip_address INET`, `user_agent`, `country`,
`city`, `logged_in_at`, `last_activity_at`, `logged_out_at`, timestamps,
`deleted_at`.

RLS on every tenant-owned table (R8). Arabic comments (R9).
`migratecheck -from 87 -roundtrip`.

### 2.1.3 Migrate the temporary `billing.plans` subscriptions

`compare_handlers.go:53` currently subscribes against `billing.plans` with a
`"compare"` string. Migration 087 must:
1. create the equivalent `compare.plans` rows
2. move any existing `billing.subscriptions` rows whose plan is the compare plan
   into `compare.subscriptions`
3. leave `billing.*` otherwise untouched

If no such rows exist in the target database, the migration is still written to
handle them — do not assume the environment is empty.

`.down.sql` must move them back.

### 2.1.4 Entitlement service

`internal/modules/compare/` — a **new module**. Layout per `AGENTS.md`:
`domain.go`, `service.go`, `repository.go`, `postgres/`, `http/`, `jobs/`.

```go
// Entitlement answers "what may this user do in the compare tool right now?"
type Entitlement struct {
    Active           bool
    PlanSlug         string
    MaxActiveFiles   int
    MaxSessions      int
    AIMatchingEnabled bool
    ExpiresAt        *time.Time
}
func (s *Service) EntitlementFor(ctx context.Context, userID, orgID int64) (Entitlement, error)
```

Every gate in this phase reads `Entitlement`. No feature check reads a plan slug
directly.

### 2.1.5 Session cap enforcement

On each compare-tool request:
1. upsert `compare.user_sessions` for the current session id + device fingerprint
2. count active sessions for the subscription seat
3. if over `MaxSessions`, either refuse or evict the oldest — **read
   `Laravel/app/Services/SessionService.php` to see which behaviour Laravel
   implements**, and match it. Record the answer.

Device fields come from the User-Agent. Do not add a fingerprinting library;
parse what is already available.

### 2.1.6 Tests

- T1: entitlement resolution — no subscription, expired, trial, active, seat not assigned
- T2: subscription CRUD round-trip
- T3: cross-tenant — org B cannot read org A's subscriptions or sessions
- T5: admin plan CRUD requires `compare.plan.manage`
- T6: session cap behaviour matches Laravel (refuse or evict)
- T12: the migration moves an existing `billing`-based compare subscription and the user keeps access

---

## TASK 2.2 — Upload and the archive retention policy

### 2.2.1 Inspect first

```bash
sed -n '1,200p' F:/Dawa\ 24/Laravel/app/Livewire/Customer/CompareDiscounts.php
grep -n "archive\|Archive\|max_active\|8\b\|22\b" F:/Dawa\ 24/Laravel/app/Livewire/Employee/CompareDiscounts.php | head -30
```

Record: what counts as an "active" file, what archiving does (soft delete? a
flag? move to another table?), whether the user is warned before an
auto-archive, and whether archived files can be restored.

### 2.2.2 Migration

`db/migrations/088_compare_files.up.sql`:

**`compare.files`** — `id`, `public_id`, `organization_id` (RLS), `user_id`,
`supplier_name TEXT NOT NULL` (the user-assigned label — Laravel lets it be
renamed, see `isRenaming`/`renamingOldName`/`renamingNewName`),
`original_filename`, `storage_key`, `mime_type`, `size_bytes`,
`row_count INTEGER`, `status TEXT` (uploaded/mapping/ready/failed/archived),
`mapping_config JSONB`, `archived_at`, `error_message`, timestamps, `deleted_at`.

**`compare.file_rows`** — the parsed rows.
`id`, `file_id → compare.files ON DELETE CASCADE`, `organization_id` (RLS),
`row_number INTEGER`, `raw_name TEXT`, `normalized_name TEXT`,
`sku TEXT`, `price NUMERIC(12,2)`, `discount NUMERIC(5,2)`,
`price_after_discount NUMERIC(12,2)`, `matched_product_id BIGINT REFERENCES catalog.products(id)`,
`match_confidence NUMERIC(5,2)`, `match_method TEXT` (exact/sku/normalized/fuzzy/ai/manual/none),
`meta JSONB`, `created_at`.

Indexes: `(file_id, row_number)`, `(organization_id, normalized_name)`,
`(matched_product_id)`, GIN on a tsvector of `normalized_name`.

### 2.2.3 Upload flow

Reuse the existing storage client (`internal/platform/storage`) and the
attachments pattern. **Do not** build a second upload mechanism — Phase 4 builds
the chunked pipeline and this must plug into it.

Interim (before Phase 4): direct multipart upload with a size cap. Record the
cap and the intent to replace it in `docs/modules/compare.md`.

Accepted formats: `.xlsx`, `.xls`, `.csv` — **confirm against Laravel**.

### 2.2.4 Archive policy enforcement

On upload:
1. count the user's non-archived `compare.files`
2. if `count >= Entitlement.MaxActiveFiles`, archive the oldest until there is
   room, **matching Laravel's behaviour** on warning/confirmation
3. write an audit row

This must be a **service-layer rule**, not a handler rule, so the chunked
pipeline in Phase 4 gets it for free.

### 2.2.5 UI

`internal/ui/pages/compare_tool.templ` — replace the current data-less shell.

Upload section:
- `@components.FileDropzone` (exists)
- a list of active files with: supplier label, row count, status badge, upload
  date, **rename** action (Laravel has it), **delete**, **archive**
- an archived-files section
- quota indicator: "3 / 8 ملفات نشطة"
- all five UI states (§0.7.4)

### 2.2.6 Tests

- T1: archive policy — at the limit, the oldest is archived; below, nothing is
- T2: file + rows insert/cascade-delete round-trip
- T3: cross-tenant — org B cannot list or download org A's files
- T6: upload → file appears; over-quota upload archives the oldest and says so
- T13: an unentitled user cannot upload (403/404 per the audience policy)

---

## TASK 2.3 — Column detection and manual mapping

This is `ColumnDetector.php`, 814 lines. It is the step `/what-in` calls
"Manual Column Mapping … automatic recognition of Arabic and foreign column names".

### 2.3.1 Inspect first — read the whole detector

```bash
cat F:/Dawa\ 24/Laravel/app/Services/ColumnDetector.php
grep -n "mappingConfig\|currentFileHeaders\|currentFilePreview\|showMapping\|currentFileIndex" \
  F:/Dawa\ 24/Laravel/app/Livewire/Customer/CompareDiscounts.php
```

Extract into `docs/modules/compare.md`:
- the complete list of header aliases per target field, **in both languages**
  (product name, price, discount, SKU/code, quantity, expiry, …)
- the scoring/priority when two headers both match
- how it handles a file with no header row
- how it handles merged cells / leading blank rows
- the preview size (how many rows Laravel shows)

### 2.3.2 Implementation

`internal/modules/compare/columns.go` (+ `columns_test.go`).

```go
// TargetField is a column the compare engine needs from an uploaded sheet.
type TargetField string
const (
    FieldProductName TargetField = "product_name"
    FieldPrice       TargetField = "price"
    FieldDiscount    TargetField = "discount"
    FieldSKU         TargetField = "sku"
    // ... every field ColumnDetector.php recognises
)

// DetectColumns proposes a header→field mapping. It is a pure function:
// same headers in, same mapping out. No I/O, no AI.
func DetectColumns(headers []string) map[int]TargetField
```

Alias table lives in `columns_data.go` (keeps `columns.go` under 400 lines,
rule R6). Arabic aliases go through the Phase 1 normaliser before matching, so
`السعر` and `السِعر` both hit.

### 2.3.3 Mapping UI

The mapping step is where the current wizard fails (`vendor_ingest.templ` does
`@click="step = 3"` with no server round-trip). Do not repeat that.

Required, matching Laravel's `showMapping` flow:
1. After upload, the server parses headers + the first N preview rows and
   returns them.
2. The UI shows a table: each detected column with a `<select>` of target fields,
   pre-selected by `DetectColumns`, and the preview rows underneath.
3. Multi-file: Laravel iterates with `currentFileIndex` — reproduce the
   "file 2 of 5" stepper using `@components.Stepper`.
4. **Confirm posts to the server.** `POST /compare/files/{id}/mapping` persists
   `mapping_config` and enqueues parsing. No client-only step advance.
5. Validation: required fields (name + at least one of price/discount) must be
   mapped, or the confirm button explains what is missing in Arabic.

### 2.3.4 Tests

- T1: `DetectColumns` over ≥40 real header rows taken from Laravel's aliases —
  Arabic, English, mixed, with and without diacritics
- T1b: ambiguous headers resolve the same way Laravel resolves them
- T6: mapping UI persists to the server; reload preserves the mapping;
  incomplete mapping is refused with an Arabic message

---

## TASK 2.4 — Deterministic product matching

This is `ProductMatcher.php` (939 lines) **minus** the AI layer. Wave B adds AI
on top; this must work without it (rule R3).

### 2.4.1 Inspect first

```bash
cat F:/Dawa\ 24/Laravel/app/Services/ProductMatcher.php
```

Record the match ladder in `docs/modules/compare.md`: the ordered list of
strategies, the confidence assigned to each, and the cut-off below which a row
is left unmatched.

Typical ladder (**verify against the file, do not assume**):
1. exact SKU / barcode
2. exact normalized name
3. normalized name + brand
4. trigram / Levenshtein similarity above a threshold
5. token-subset match
6. unmatched

### 2.4.2 Implementation

`internal/modules/compare/matching.go`:

```go
type Match struct {
    ProductID  *int64
    Confidence float64   // 0..100, matching Laravel's scale — verify
    Method     string
}
// MatchRow resolves one uploaded row against the catalog index.
func (s *Service) MatchRow(ctx context.Context, orgID int64, row FileRow) (Match, error)
```

Similarity uses Postgres `pg_trgm`. **Check whether the extension is available**
— migration 050 shipped without `cube`/`earthdistance`, so do not assume.
`CREATE EXTENSION IF NOT EXISTS pg_trgm;` in the migration, and if it fails in
the target environment, fall back to a Go-side Levenshtein over a candidate set
narrowed by the tsvector index. Record which path is live.

Matching reads `catalog.product_index` (Phase 1). If the index is empty, fall
back to `catalog.products` (Phase 1 T11 already requires this).

Matching runs as a **worker job** over the file's rows, not in the request.
Progress is reported so the UI can show it (Laravel has `isProcessing` and
`processingStep`).

### 2.4.3 Manual correction

Laravel lets the user fix a match (`selectedDbProduct`, `dbSearchResults`,
`showDbResults`). Reproduce:
- per-row "غير مطابق / تعديل المطابقة" action
- a search modal over the catalog
- selecting a product sets `matched_product_id`, `match_method='manual'`,
  `match_confidence=100`
- **corrections must persist** and must be reused: record them in
  `catalog.customer_product_mappings` (that table already exists and already has
  `raw_name`, `source`, `status` — this is exactly what it is for) so the next
  upload of the same name matches automatically

### 2.4.4 Tests

- T1: the match ladder, table-driven, one case per strategy plus a no-match case
- T1b: Arabic variants match (أ/ا, ة/ه, ى/ي, diacritics, tatweel)
- T2: matching job is idempotent and resumable
- T6: manual correction persists and is reused on the next upload
- T7: **the whole matcher runs with the AI Gateway disabled** (rule R3)

---

## TASK 2.5 — Comparison, results UI, and market discounts

This is the payload the customer actually came for. Laravel's component exposes
these behaviours — reproduce **all** of them.

### 2.5.1 Feature checklist extracted from Laravel

From `Customer/CompareDiscounts.php` public properties, every one of these is a
required behaviour:

| Laravel property | Required behaviour |
|---|---|
| `suppliers`, `selectedSuppliers` | pick which uploaded supplier files participate in the comparison |
| `results`, `summary` | the comparison result set plus an aggregate summary |
| `isProcessing`, `processingStep` | progress feedback during matching/comparison |
| `searchQuery` | free-text filter over results |
| `sortBy` | result sorting (name and others — read the allowed values) |
| `page`, `perPage` (25) | pagination, **default 25** |
| `marketPage`, `marketPerPage` (25) | separate pagination for the market view |
| `dbSearchQuery`, `dbSearchResults`, `selectedDbProduct`, `dbProductDetailTable`, `showDbResults` | catalog search + a product detail table |
| `advSearchSupplier`, `advSearchMinPrice`, `advSearchMaxPrice`, `advSearchMinDiscount`, `advSearchMaxDiscount`, `advSearchMappedOnly`, `showAdvFilters` | advanced filter panel: supplier, price range, discount range, mapped/unmapped/all |
| `viewingSupplierName`, `viewingSupplierProducts` | drill into one supplier's products |
| `viewingPriceSuppliers`, `viewingAllProductSuppliers`, `viewingProductCode` | per-product popup: every supplier offering it, with prices |
| `isRenaming`, `renamingOldName`, `renamingNewName` | rename a supplier label |
| `selectedSourceSupplier`, `selectedTargetSupplier`, `supplierComparisonResults`, `supplierComparisonStats` | **supplier-vs-supplier** head-to-head with stats |
| `marketCompareSupplier`, `marketComparisonFilter` | **market comparison** filtered by: `all`, `lower_discount_than_market`, `equal_to_market`, `higher_discount_than_market`, `exclusives` |
| `premiumNotifShown` | upsell notice for unentitled users |

**Read the Blade view** (`resources/views/livewire/customer/compare-discounts.blade.php`)
for the exact layout, column order, badge colours and Arabic labels.

### 2.5.2 Market discounts

`Employee/MarketDiscounts.php` (263 lines) and `Customer/MarketDiscounts.php`.
"خصومات السوق المعتمدة من الأدمن" per `/what-in`.

Determine from the code: what constitutes the "market" baseline — is it an
admin-uploaded global discount sheet (`UploadGlobalDiscounts`, admin route
`/admin/compare-discounts/upload`), or an aggregate of vendor offers? Record the
answer, then implement:
- admin upload screen for the market baseline (Phase 5 registers the route; the
  service lands here)
- `compare.market_prices` table if a baseline sheet is the answer
- the five `marketComparisonFilter` modes

### 2.5.3 Routes

```go
// RegisterSharedRoutes — both customers and vendors get the compare tool
r.Get ("/compare/tool",                       h.CompareToolPage)
r.Post("/compare/files",                      h.CompareFileUploadSubmit)
r.Get ("/compare/files/{id}/mapping",         h.CompareMappingPage)
r.Post("/compare/files/{id}/mapping",         h.CompareMappingSubmit)
r.Post("/compare/files/{id}/rename",          h.CompareFileRenameSubmit)
r.Post("/compare/files/{id}/archive",         h.CompareFileArchiveSubmit)
r.Post("/compare/files/{id}/delete",          h.CompareFileDeleteSubmit)
r.Get ("/compare/files/{id}/status",          h.CompareFileStatusPartial)   // polled during processing
r.Post("/compare/run",                        h.CompareRunSubmit)
r.Get ("/compare/results",                    h.CompareResultsPage)
r.Get ("/compare/results/product/{code}",     h.CompareProductSuppliersPartial)
r.Get ("/compare/supplier/{name}",            h.CompareSupplierProductsPage)
r.Post("/compare/rows/{id}/match",            h.CompareRowMatchSubmit)
r.Get ("/compare/search",                     h.CompareCatalogSearchPartial)
r.Get ("/market-discounts",                   h.MarketDiscountsPage)
```

Handlers go in `internal/ui/compare_handlers.go`, split across
`compare_handlers.go` / `compare_results_handlers.go` / `compare_files_handlers.go`
to respect rule R6.

**Audience:** shared (both types have it per the `/what-in` permission matrix),
gated on `Entitlement.Active`. An unentitled user gets the upsell page, not a
404 — Laravel shows a premium notice (`premiumNotifShown`), which is a product
decision, not a security boundary.

### 2.5.4 Sidebar

- Vendor position 24 ("مقارنة الخصومات") and 25 ("خصومات السوق")
- Customer position 6 ("مقارنة الخصومات")
per `00_MASTER.md` §0.7.3.

### 2.5.5 Tests

- T1: comparison arithmetic — best price, best discount, price-after-discount —
  exact-value assertions via `money.Amount` (rule R1, test T8)
- T1b: each of the five `marketComparisonFilter` modes classifies correctly
- T2: results query with every filter combination binds and returns the right shape
- T3: cross-tenant — comparison never includes another org's files
- T6: upload → map → run → results renders; each filter, sort and pagination works
- T13: unentitled user sees the upsell, not the tool

---

## TASK 2.6 — Wave B: AI enhancement

**Do not start until Wave A passes its tests.**

### 2.6.1 Rules

- Rule R2: no provider names outside `internal/platform/gateway/`. The compare
  module asks for a **capability**.
- Rule R3: every AI path has a deterministic fallback — which is Wave A.
- `make check-provider-isolation` must stay green.

### 2.6.2 Capabilities to add

| Capability | Use | Fallback |
|---|---|---|
| `product.match` | resolve a raw drug name to a catalog product when the deterministic ladder returns low confidence | the ladder's best guess, marked low-confidence and flagged for manual review |
| `search.expand` | Laravel's `isSmartSearch` / `currentExpandedTerms` — expand a search term into synonyms and spelling variants | the Arabic normaliser's variants only |

Check `internal/modules/aicapabilities` for what already exists — `product.match`
may already be defined. Extend rather than duplicate.

### 2.6.3 Behaviour

- AI runs **only** on rows the deterministic matcher left below the confidence
  cut-off. Never on every row — it is slow and expensive.
- AI results are written with `match_method='ai'` and their confidence, and are
  **visually distinguished** in the UI so a user knows to check them.
- Gated on `Entitlement.AIMatchingEnabled`.
- Timeout and circuit-break: a Gateway failure must degrade to Wave A silently
  for the user and loudly in the logs.

### 2.6.4 Tests

- T7: full pipeline with the Gateway disabled — results are still produced
- T7b: Gateway returns garbage → rows fall back, no crash, logged
- T1: AI results never overwrite a higher-confidence deterministic match
- `make check-provider-isolation` green

---

## PHASE 2 COMPLETION GATE

```bash
make check
make check-provider-isolation
go run ./cmd/migratecheck -from 87 -roundtrip
go test ./internal/modules/compare/... -race
go test ./test/integration/... -run Compare
```

- [ ] A customer can upload a sheet, map its columns on the server, run a comparison and see results
- [ ] A vendor can do the same
- [ ] Supplier-vs-supplier comparison works with stats
- [ ] All five market-comparison filters work
- [ ] The archive policy enforces the plan's file limit
- [ ] The session cap matches Laravel's behaviour
- [ ] Manual match corrections persist and are reused
- [ ] **The entire engine works with the AI Gateway disabled**
- [ ] Every property in the §2.5.1 checklist has a corresponding behaviour
- [ ] No unentitled user reaches the tool; they see the upsell
- [ ] `docs/modules/compare.md` records: the alias table, the match ladder, the market baseline decision, the session-cap decision, and the `currency` column note
- [ ] `PROGRESS.md` updated for 2.1–2.6
