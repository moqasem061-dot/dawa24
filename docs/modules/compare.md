# Module: Compare (The AI Compare & Discount Engine)

## Overview

The `compare` bounded context implements the platform's flagship supplier-vs-supplier discount and price comparison engine. It manages pricing tiers, quotas (maximum active comparison spreadsheets, concurrent session device caps), self-serve upgrade requests, subscriptions, user seat assignments, and session eviction.

## Feedback fixes (2026-10-29)

- Comparison selection uses `EntitlementFor().MaxActiveFiles` in both submission and execution. The independent ten-file ceiling is removed; the current effective entitlement permits 100 files. These are supplier spreadsheet files, not a limit on product rows.
- Results support product/code search, offer availability, catalog linkage and winning supplier filters. Filter links and pagination preserve the selected criteria. Head-to-head filters use a responsive grid, explicit outcome selection and exact monetary price bounds.
- Head-to-head CSV exports every row matching the current filters before pagination, including all report columns. Export and result requests validate selected IDs against the caller's visible files. CSV text is protected against spreadsheet formula execution.
- Market upload-date filters include both ends of the selected date range in Africa/Cairo time. Listing, totals, cache keys and pagination share the date criteria. Market typing waits 250 ms; cancellation, composition handling and stale-response protection remain active.

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
9. **Deterministic Matching Engine & Knowledge Scoping:**
   - Resolves uploaded spreadsheet rows against master catalogue products using the unified matching architecture (`matching.Service` via `compare.Matcher`, scoped to `WorkflowCompare`).
   - In offline test environments where `compare.Matcher` is unwired, gracefully falls back to deterministic in-memory indexing (`productmatch.MatchAll`).
   - Match decisions project canonical columns via `matchflow.Columns(dec)` (`matched_product_id`, `match_state`, `match_basis`, `match_revision`). `matched_product_id` is set if and only if the decision is accepted/confirmed; for any other state (`needs_review`, `ambiguous`, `no_candidate`), it is reset to `NULL`.
   - **Knowledge Scoping & Manual Corrections**: Reviewer confirmations and manual row links persist to `catalog.match_knowledge` scoped to the organization (`Scope.EffectiveOrg`), auto-resolving future uploads with identical row fingerprints without cross-tenant data leakage.
   - **AI Gateway Independence (Rule R3)**: Deterministic matching runs 100% locally with zero external AI dependency.
10. **Multi-Supplier & Head-to-Head Comparison (Task 2.5):**
    - Multi-supplier analysis over files up to the effective entitlement limit computes best price, best discount, best net price after discount, and best supplier per item.
    - Aggregated metrics include total unique products, average discount, and potential savings.
    - Head-to-head comparison between two suppliers computes shared product count, better count, quality score (`better/shared * 100`), source total, target total, and net savings.
    - 5 Market comparison filter modes (`all`, `lower_discount_than_market`, `equal_to_market`, `higher_discount_than_market`, `exclusives`).
    - Integrated into Customer sidebar (item 6 "مقارنة الخصومات") and Vendor sidebar (items 24 "مقارنة الخصومات" and 25 "خصومات السوق").
11. **AI Enhancement Integration (US9, T127):**
    - Asynchronous AI enhancement is routed through `matching.Service.StartEnhancement` on unresolved-but-resolvable residue rows via `compare.AIApplier` when `Entitlement.AIMatchingEnabled` is active.
    - Settled deterministic rows are excluded from AI dispatch; high-confidence deterministic matches are never overwritten.
    - AI proposals are strictly validated through the product identity gate (`Gate.Evaluate`, path `AI`) and applied using revision compare-and-set. Answers outside a row's declared candidate options are rejected.
    - Strict deadline classes (30s/90s/180s) and cost budgets are enforced. If the AI Gateway is unavailable or timed out, file matching remains 100% functional with deterministic outcomes.

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

## Search stability (2026-09-30)

### Market-board repair (2026-10-23)

The default order is warehouse upload date descending, with a stable row-ID
tie-breaker, including during search. Relevance and explicit price/discount
orders remain selectable. The indexed search checks imported names and both
catalogue languages first. Fuzzy catalogue retrieval is attempted only when
the filtered direct search has no results, avoiding up to 500 ms of candidate
retrieval on ordinary searches. Totals use the same filter as rows, including
fuzzy candidates. Punctuation-only normalized terms cannot match every row.
Typing uses a 250 ms debounce; cancellation, stale-response suppression,
Arabic composition and input-focus retention are covered by browser tests.

Market-discount searches wait 250 ms after typing pauses; other GET filter searches retain 450 ms.
Each form owns its debounce timer, composition input waits until committed, and
editing or submitting cancels pending search work. Select controls with
`data-autosubmit` issue one request rather than also triggering the generic filter
submit. Short queries retain the existing three-character minimum; clearing the
query refreshes the unfiltered results.

Boosted filter responses preserve the active search input, focus, selection and
scroll position. Older responses are discarded when the form values have changed,
so a slow search cannot replace newer input or update the URL with an old query.
GET filters do not inherit the eight-second mutation submit lock. A server outage
or rate limit leaves the current screen usable without repeatedly reloading it.
The browser regressions in `scripts/test-session-search.cjs` exercise these flows
with the real HTMX, CSP/Trusted Types, Alpine and shared application scripts.


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

## Shared UI reliability (2026-09-30)

Remote comboboxes and the comparison tool's instant search invalidate pending
responses when input, selection, dependent filters or the owning page changes.
Aborting a request is an optimization; a request revision is the final guard
when cancellation is unavailable or the response has already arrived.

Flash notices and tab changes preserve HTMX history state. Mobile notifications
fit narrow RTL viewports, provide readable semantic borders, a 44px dismiss
target, and screen-reader status/alert announcements. The malformed half-step
margin utility no longer hides neighboring CSS declarations.

Import progress stores its snapshot callback before opening SSE, so failed
streams retain polling callbacks. Polls carry `X-Dawa-Background: 1`; detached
progress bars close their stream and cancel polling, drift and completion
callbacks. `scripts/test-ui-stability.cjs` contains the regression fixtures.

## Product Matching Rework & Equivalence Grouping (2026-10-01)

Per Product Matching Rework User Story 6 (FR-038, FR-040, FR-041):

1. **Grouping Gate & Equivalence Basis (`group_basis`)**:
   - Comparison grouping across head-to-head and multi-supplier analysis requires verified pack-level identity agreement.
   - Rows group only when they share the same master catalogue product ID at pack granularity (`canonical_pack`), or when `productmatch.SameSellableItem` confirms identity equivalence across source files (`source_equivalence`).
   - Shared prefix, manufacturer, or short brand alone never groups items into a comparison row.
   - Items absent from the master catalogue group safely with `source_equivalence` without fabricating a canonical catalog link.

2. **Price Comparability & Selling Units**:
   - Differing pack sizes and commercial selling units (e.g. Strip vs Box) are comparable only through validated conversion relationships from `catalog.pack_relationships` via `productmatch.Convert`.
   - Converted prices use exact rational arithmetic (`money.Amount.MulRatio`) without `float64`.
   - Differing packs lacking an active bridging relationship are marked "not directly comparable" (`not_comparable = true`, `group_basis = 'not_comparable'`), and excluded from savings totals (`TotalSavings` and `TotalPotentialSaving`).
   - The UI displays explicit bilingual badges for `group_basis` and marks non-comparable rows with "غير قابل للمقارنة المباشرة" (Not directly comparable).

3. **Stale Link Invalidation (FR-041)**:
   - Rerunning or rematching a row projects via `matchflow.Columns(d)`. When the latest outcome is non-accepted (`needs_review`, `ambiguous`, `unresolved`), `matched_product_id` is set to NULL, preventing stale associations from surviving.


## Matched temporary warehouses and market search (2026-10-04)

Super admins can run `POST /admin/temporary-warehouses/review-all` to review and
save columns and rematch every product in every non-deleted platform warehouse,
including vendor uploads and archived files, independently of pagination and
page filters. Saved mappings are retained; missing required columns use header
detection, and uncertain mappings are reported for manual review. Each file's
replacement is atomic and guarded against concurrent changes. Archived files
retain their status, timestamp and reason. Matching uses the existing shared
engine and deterministic fallback, without forcing unsafe catalog links.

Progress and failed file IDs are stored in a separate `temp_warehouse_review`
import run. Work runs sequentially in the server background; closing the browser
does not cancel it. After a server restart, the initiating admin can press the
button again to resume the saved checkpoint (stale runs recovered as failed can
be rerun). Migration 244 permits organization-free platform review runs and
prevents multiple active platform-wide review records. The active filter includes
every non-archived status; searching preserves status, and summary counts ignore
only status while retaining uploader, source and search filters.

Temporary warehouse imports, remapping and reprocessing resolve every extracted
product row through the shared catalog matching service before publishing it.
Accepted/confirmed decisions link to the catalog; ambiguous or unmatched rows
retain the file name without inventing an association. Canonical decisions and
run identifiers persist with the rows. Prices use exact decimal parsing.

Row replacement and the ready state commit in one transaction. A concurrent
rename, remap, archive or deletion rejects stale replacement. A failure preserves
previous rows, and a late background failure cannot overwrite completed work.
Stored uploads are located by their exact storage key, including the configured
temporary warehouse directory; a missing source never selects another workbook.

Market search uses public catalog matching candidates (no private organization
aliases), a short candidate cache, a 500 ms context deadline, and normalized
indexed text fallback. All query words must match a file or catalog name, in any
order; SQL wildcards are literal input. Relevance is the default sorting mode.
List and grid cards lead with the localized accepted catalog name and show the
original supplier text beneath it. The source remains ready, active temporary
warehouses only, never private comparison uploads.

**Fixed 2026-10-09: word matching lost its wildcards.** The word pattern was
normalised as a whole, and `platform.normalize_arabic` turns `%` into a space and
trims it. Each word therefore became an exact match on the whole name, so a
partial query such as one word of a longer product name found nothing. The
wildcards now sit outside the normalisation, `'%' || lower(normalize_arabic(word)) || '%'`,
and normalisation strips every LIKE metacharacter from the word itself, so no
escaping is needed. Regression: `TestMarketSearchKeepsWordWildcardsOutsideNormalisation`
(`internal/modules/compare/postgres/market_search_query_test.go`).

Resumable sessions require the uploader's identity and organization. Mapping
requests must name a file belonging to that session; failed processing cannot
report successful mapping or commit. These rules apply through the shared admin,
my, team, user, plan and legacy warehouse route handlers.

Migration 243 adds the raw-name/catalog-name trigram indexes and catalog-link
index. Existing stored warehouses gain decisions when reprocessed/remapped;
this change does not silently rematch production history during migration.

The market board's identity and pagination unit is `compare.file_rows.id`, not
`matched_product_id`. Several rows and warehouses may link to the same catalog
product. Each remains a separate card with its own supplier, prices and discount;
counts and pagination include every eligible offer. Unmatched offers remain
visible, with no catalog button. Catalog candidate IDs are deduplicated only for
search lookup; offer rows are never collapsed by product.

## Literal discount comparisons and bounded uploads (2026-10-09)

Discount comparisons now rank by the imported percentage points, independently
of list or net price. Head-to-head equality compares integer hundredths of a
point, avoiding binary rounding that could hide a 0.01 difference. Multi-supplier
winners use the highest discount; equal discounts use net price as a tie-breaker.
Market benchmark counts and detail lists share one classification and choose
one highest-discount offer per supplier, normalizing case and whitespace.

XLSX comparison uploads, temporary warehouses, previews, remapping and review
read stored cell values without Excel display formatting. A stored `0.25`
remains `0.25`, including when its Excel style displays `25%`. Text `25%` means
25 percentage points. Parsing is shared across both upload paths and supports
Arabic digits and decimal separators. No fraction-to-percent rescaling occurs.
Out-of-range or malformed cells retain the existing zero fallback rather than
being clamped into fabricated 100% offers. Database precision remains two
fraction digits; higher precision is rounded by the existing decimal parser.

Valid 100% discounts remain visible on the market board. Discount benchmarking
includes discount-only rows; monetary intelligence still excludes unusable
zero-net offers from its price baseline. Percentage displays and market filter
URLs preserve both fractional digits instead of rounding to one.

Detached comparison parsing is bounded to four concurrent files across requests.
Queued files get a fresh processing deadline after acquiring a slot. Late failure
updates use the guarded repository operation and a separate short context, so
completed/remapped/archived files cannot be overwritten by a stale failure.
Mapping previews inspect at most 200 rows after the mandatory security scan.
