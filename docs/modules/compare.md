# Module: Compare (The AI Compare & Discount Engine)

## Overview

The `compare` bounded context implements the platform's flagship supplier-vs-supplier discount and price comparison engine. It manages pricing tiers, quotas (maximum active comparison spreadsheets, concurrent session device caps), self-serve upgrade requests, subscriptions, user seat assignments, and session eviction.

## Schema Mapping

- **PostgreSQL Schemas:** `compare`
- **Migrations:** `087_compare_plans.up.sql`, `088_compare_files.up.sql`
- **Tables Owned:**
  - `compare.plans` — Compare engine subscription plans (e.g. `compare-customer-basic`, `compare-vendor-pro`, `compare-enterprise`).
  - `compare.plan_features` — Plan quotas and capabilities (`max_active_files`, `max_concurrent_sessions`, `ai_matching_enabled`).
  - `compare.plan_requests` — Self-serve plan requests submitted by organizations for admin approval.
  - `compare.subscriptions` — Active and historical organization/user subscriptions with billing periods.
  - `compare.subscription_users` — Seat assignments for multi-seat subscriptions.
  - `compare.user_sessions` — Active client sessions and device tracking for concurrent login cap enforcement.
  - `compare.files` — Uploaded supplier spreadsheets (`.xlsx`, `.xls`, `.csv`) with statuses (`uploaded`, `mapping`, `ready`, `failed`, `archived`).
  - `compare.file_rows` — Parsed product rows, prices, discounts, and matched catalog product IDs.

## Invariants & Rules

1. **Currency Invariant (Rule R1):** `compare.plans.currency` defaults to `'EGP'`. The platform operates on single-currency `money.Amount`; the column exists for Laravel schema parity and must not introduce multi-currency conversion logic.
2. **Quota Invariant (Task 2.1 / `/what-in` parity):**
   - Customer Basic Plan: `max_active_files = 8`, `max_concurrent_sessions = 1`.
   - Vendor Pro Plan: `max_active_files = 22`, `max_concurrent_sessions = 5`.
   - Enterprise Plan: `max_active_files = 100`, `max_concurrent_sessions = 20`.
3. **Session Eviction Parity (§2.1.5 / `SessionService.php`):** When a user logs in and exceeds their allowed `max_concurrent_sessions`, the platform **evicts the oldest active session** (setting `is_active = false` and recording `logged_out_at`) rather than blocking the login attempt.
4. **Unified Entitlement Gate:** Feature checks in UI and HTTP handlers must read `compare.Service.EntitlementFor` rather than inspecting plan slugs directly.
5. **Archive Retention Policy (Task 2.2 / Laravel Parity):**
   - Active files are those with `status != 'archived'`.
   - When an upload exceeds `Entitlement.MaxActiveFiles`, the service layer automatically archives the oldest active files (`status = 'archived'`, sets `archived_at = now()`, appends ` - مؤرشف X` to supplier label, and sets reason).
   - The user receives a flash notification detailing which older files were archived.
   - Users can manually rename, archive, unarchive (restore), or delete files at any time.
6. **Upload Size Cap:** Direct uploads are capped at 20MB (`.xlsx`, `.xls`, `.csv`). This is an interim mechanism that will plug into the Phase 4 chunked upload pipeline.
7. **Row Level Security (Rule R8):** `compare.plan_requests`, `compare.subscriptions`, `compare.files`, and `compare.file_rows` enforce tenant isolation via `platform.tenant_visible(organization_id)`.
8. **Automatic Column Detection & Mapping (Task 2.3 / Laravel ColumnDetector Parity):**
   - Implements bilingual Arabic and English/Technical header recognition across 12 canonical fields (`product_id`, `product_name`, `description`, `price`, `cost_price`, `discount`, `quantity`, `sku`, `unique_id`, `barcode`, `alert_price`, `alert_discount`).
   - Cleans BOM and zero-width characters, runs Arabic normalization, and computes similarity scores (exact match = 1.0, threshold > 0.65).
   - Scans leading spreadsheet rows (`FindBestHeaderRow`) to automatically detect table headers when files contain banners or metadata rows.
   - Validation requires `product_name` and at least one of `price` or `discount` to be mapped.
   - Persists user-confirmed mappings via `POST /compare/files/{id}/mapping`.
9. **Deterministic Match Ladder (Task 2.4 / Laravel ProductMatcher Parity):**
   - Resolves uploaded rows against `catalog.product_index` (with fallback to `catalog.products`) in strict ladder order:
     - **Strategy 0**: Saved Customer Product Mappings (`catalog.customer_product_mappings`) $\to$ 100% confidence (`saved_mapping`).
     - **Strategy 1**: SKU / Barcode direct match $\to$ 100% confidence (`sku` / `barcode`).
     - **Strategy 2**: Exact normalized name match (Arabic/English) $\to$ 100% confidence (`exact_name`).
     - **Strategy 3**: Trigram / fuzzy candidate similarity (matching name, scientific name, manufacturer, pharmacology) $\to \ge 90\%$ (`fuzzy`), $\ge 60\%$ (`partial`).
     - **Strategy 4**: Token-subset / First meaningful word candidate expansion $\to \ge 55\%$ (`partial`).
     - **Strategy 5**: Below $55\% \to$ `unmatched` (0% confidence).
   - **Manual Correction Persistence**: Users can override/set matches via `POST /compare/rows/{id}/match`; corrections persist immediately to `catalog.customer_product_mappings` (`source='manual'`, `status='processed'`) and automatically auto-match future uploads.
   - **AI Gateway Independence (Rule R3)**: Deterministic matching runs 100% locally with no external AI dependency.
10. **Multi-Supplier & Head-to-Head Comparison (Task 2.5):**
    - Multi-supplier analysis over 1 to 10 files computes best price, best discount, best net price after discount, and best supplier per item.
    - Aggregated metrics include total unique products, average discount, and potential savings.
    - Head-to-head comparison between two suppliers computes shared product count, better count, quality score (`better/shared * 100`), source total, target total, and net savings.
    - 5 Market comparison filter modes (`all`, `lower_discount_than_market`, `equal_to_market`, `higher_discount_than_market`, `exclusives`).
    - Integrated into Customer sidebar (item 6 "مقارنة الخصومات") and Vendor sidebar (items 24 "مقارنة الخصومات" and 25 "خصومات السوق").
11. **Wave B — AI Enhancement (Task 2.6):**
    - AI matching capability (`CapProductMatch` via `aicapabilities.Service`) is invoked **only** for rows left below the deterministic confidence cutoff ($< 55\%$) when `Entitlement.AIMatchingEnabled` is active.
    - High-confidence deterministic matches are never overwritten by AI (efficiency and accuracy rules §2.6.3).
    - AI matches are tagged with `match_method='ai'` and their confidence score for clear UI auditability.
    - If the AI Gateway is unavailable or returns unrecognized candidates, matching gracefully falls back to deterministic unmatched without crashing or halting execution.

## Endpoints & Routes

- `GET /compare` — Public plans and comparison tier pricing.
- `POST /compare/subscribe?plan={slug}` — Subscribe / request compare plan.
- `GET /compare/tool` — Access the comparison tool interface (gated by `EntitlementFor`).
- `POST /compare/upload` — Upload supplier comparison spreadsheet (enforces 20MB limit and active file quota).
- `POST /compare/files/{id}/rename` — Rename supplier label for a file.
- `POST /compare/files/{id}/archive` — Manually archive a file.
- `POST /compare/files/{id}/unarchive` — Restore an archived file.
- `POST /compare/files/{id}/delete` — Soft-delete a file.
- `POST /compare/files/{id}/mapping` — Save and apply confirmed column mapping for a spreadsheet.
- `POST /compare/rows/{id}/match` — Manually link an uploaded row to a master catalog product.
- `POST /compare/run` — Execute multi-supplier analysis on selected files.
- `GET /compare/results` — View multi-supplier comparison results table and summaries.
- `GET /compare/head-to-head` — Head-to-head comparison view between two suppliers.
- `GET /market-discounts` — Market-wide approved discounts and supplier comparison.


## Shared Matching (2026-08-25)

`compare`'s normalisation and string-similarity helpers now live in
`internal/shared/productmatch` (`NormalizeText`, `FirstMeaningfulWord`,
`TextSimilarity`), carried across unchanged. They were duplicating the engine
that `ingest` already had, and two implementations of "are these the same product
name" drift invisibly until two features disagree about the same spreadsheet.

`compare`'s DB-backed strategy ladder (`MatchLadder`) stays in this module:
candidate retrieval needs SQL and is legitimately module-owned. What is shared is
the scorer, not the retrieval.

The engine itself was promoted from `modules/ingest/engine` to
`internal/shared/productmatch` so `ingest`, `compare` and `smartorder` all use
one implementation. See `docs/modules/smartorder.md`.
