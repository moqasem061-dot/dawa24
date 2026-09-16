# Dawa24 Store — Engineering Handoff

**For:** the next agent or developer continuing this rebuild
**Repo state at handoff:** commit `81b9422`. Builds clean, `go vet` clean, all tests pass.
**Measured completion: ~55%.** See `docs/REVIEW_59e2451.md` for how that was measured.
**Written:** 2026-08-16

> **BUILD RUN UPDATE (2026-08-17):** Phases 1–6 of `docs/BUILD_PHASES.md` are
> now built and gated green (account types + three dashboards, the full account
> surface, public/content pages, all ten Phase-4 modules, the component library,
> catalogue/cart/checkout depth, and all fourteen admin screens). Ten migrations
> were added (`035`–`044`). Phase 7 (ETL) is **blocked on two owner decisions**
> (authoritative order system; legacy `company`/`agency` mapping); Phase 8
> hardening is partial (unit/service/repository tests + CI coverage floor done;
> credential rotation, MinIO/backups, RTL/a11y/perf remain, needing a live
> deployment). See `docs/BUILD_PHASES.md` Part 0 for the updated gap table.

Read Part 0 through Part 5 before writing any code. They are short, and skipping
them will cost you more time than reading them.

---

# PART 0 — What this project is

Dawa24 is an **Arabic-first B2B pharmaceutical marketplace** for the Egyptian
market. Pharmacies and clinics buy from suppliers and distributors.

There is an **existing, working Laravel 12 application** that implements the
whole thing. This repository is a **ground-up rewrite of that application in Go**.
It is the same product, the same business rules, and eventually the same data —
a different codebase, not a different system.

**Current reality: the rewrite is roughly 55% complete.** Do not treat it as
feature-complete — an earlier commit message claimed that and it was not true.

What genuinely works: 14 modules, 26 migrations, 98 tables, 40 tables under
`ENABLE`+`FORCE` row-level security, 126 API routes, River workers with real
bodies, and AI capabilities wired with a passing black-hole test.

What does not: the ETL is a 43-line shell (`cmd/etl/main.go` has no SQL at all),
the admin surface is 4 routes against 275 in Laravel, roughly two thirds of the
planned API endpoints are missing, the frontend has 13 templ files against a
20-screen plan, and **every `http/` and `postgres/` package sits at 0% test
coverage**.

**The migrations have still never been executed against a real PostgreSQL.**

**The Laravel app remains the only functioning marketplace.**

## Actors in the domain

From `users.role` in the legacy schema:
`admin` · `owner` · `manager` · `employee` · `vendor` · `customer` · `developer`

Suppliers are modelled as **organizations** with **branches** and **warehouses**.
Organizations are the tenant boundary.

---

# PART 1 — Reference map

Everything below is an absolute path on the machine this was built on.

## The three codebases

| What | Path | Role |
|---|---|---|
| **Legacy Laravel app** | `F:\Dawa 24\Laravel` | The source of truth for all business logic. **Read it constantly.** Not in git. Never deployed. |
| **This rewrite** | `F:\Dawa 24\dawa24-store` | → `github.com/muhiyatools/dawa24-store` |
| **AI Gateway** | `F:\MuhiyaWorkspace\MuhiyaWorkspace` | Existing Go service, 40+ migrations. **Do not fork it.** Deploy it a second time. |

## Planning and audit documents

| Document | Path | What it contains |
|---|---|---|
| **Master plan** | `F:\Dawa 24\docs\rebuild\REBUILD_MASTER_PLAN.md` | The full audit and target architecture. §2 legacy DB audit, §5 target schema, §11 data migration, §13 phase roadmap. |
| **Legacy schema inventory** | `F:\Dawa 24\docs\rebuild\SCHEMA_INVENTORY.md` | Every one of the 141 tables + 7 views: columns, types, indexes, FKs. **3,734 lines. Use this instead of re-parsing the SQL dump.** |
| **Raw SQL dump** | `F:\Dawa 24\u924222867_Testv5.sql` | MariaDB 11.8 schema-only dump, 6,804 lines. Source for the inventory above. |
| **ADRs** | `dawa24-store/docs/adr/decisions.md` | Five decisions with their reasoning. Read before proposing an alternative. |
| **Agent conventions** | `dawa24-store/AGENTS.md` | The working rules. Non-negotiable section is short. |
| **Deployment** | `dawa24-store/DEPLOY.md` | Elest.io steps. |
| **Ops / infra** | `F:\Dawa 24\dawa24-ops\` | Compose files and pgAdmin bootstrap SQL per pipeline. |

## Where to look in the Laravel app

Business logic lives in **Livewire components**, not in services. This is the
single most important fact about reading the legacy code.

| Concern | Legacy path |
|---|---|
| Business logic (353 components) | `Laravel\app\Livewire\{Admin,Customer,Employee,Front,Pages}` |
| Views (514 templates) | `Laravel\resources\views` |
| Models (114) | `Laravel\app\Models` |
| Services (~32, partially used) | `Laravel\app\Services` |
| Routes | `Laravel\routes\{web,admin,vendor,customer,api}.php` |
| **Globally autoloaded helpers (13 files)** | `Laravel\app\Helpers` — loaded via composer `files[]`, so these are global functions used everywhere |
| Queued jobs (8) | `Laravel\app\Jobs` |
| Middleware (18) | `Laravel\app\Http\Middleware` |
| **The one well-built module** | anything under an `Offers` subdirectory — use as the quality reference |

---

# PART 2 — What is DONE

All of this is committed, compiles, is `go vet` clean, and is formatted.
**Platform layer, 14 domain modules and the core invariants are in place. See
`docs/REVIEW_59e2451.md` for per-phase status and `docs/COMPLETION_PLAN.md` for
what remains.**

## Shared primitives (`internal/shared/`) — dependency-free leaves

| Package | File | State |
|---|---|---|
| `money` | `money.go` + `money_test.go` | **Complete. 10 tests.** Exact int64 minor units. `Parse`, `Add`, `Sub`, `MulInt`, `ApplyPercent` (basis points, half-up away from zero), `Allocate` (splits without losing a unit), SQL `Valuer`/`Scanner`, JSON as string. |
| `arabic` | `arabic.go` + `arabic_test.go` | **Complete. 11 tests.** Faithful port of `Laravel\app\Services\ArabicNormalizer.php`, including the exact similarity curve. |
| `i18n` | `i18n.go` + `i18n_test.go` | **Complete. 6 tests.** `Text` = `map[Lang]string` for the `{"ar","en"}` JSONB columns. RTL helpers, fallback resolution, Valuer/Scanner. |
| `apperr` | `apperr.go` + `apperr_test.go` | **Complete. 5 tests.** Closed error-kind set mapped to HTTP by `httpx`. `Wrap`, `WithDetail`, `KindOf`, `As`. |

## Platform (`internal/platform/`) — infrastructure

| Package | File | State |
|---|---|---|
| `config` | `config.go` | **Complete.** Typed, fail-fast, aggregates every problem. Prod-only strictness. |
| `database` | `database.go` | **Complete.** pgx pool + **the tenant isolation contract**: `InTx`/`InReadTx` issue `SET LOCAL app.current_org_id`. `AsSystem` for explicit cross-tenant. Error classifiers. |
| `database` | `migrate.go` | **Complete.** Embedded, checksummed, advisory-locked migration runner. Refuses edited migrations. |
| `cache` | `cache.go` | **Complete.** Redis. **All keys tenant-namespaced.** `Remember[T]` generic. `InvalidateTenant` via SCAN. |
| `storage` | `storage.go` + `storage_test.go` | **Complete. 2 tests.** S3 SDK v2 with MinIO path-style support, `Put`/`Get`/`Delete`/`PresignGet`/`PresignPut`, `KeyFor(orgID, path)` tenant-namespacing. |
| `queue` | `queue.go` + `queue_test.go` | **Complete. 1 test.** River queue client wrapper, `Enqueue`, transactional `EnqueueTx`, River migration runner, worker registration. |
| `gateway` | `gateway.go`, `errors.go`, `disabled.go` | **Complete.** Capability-based client, per-capability budgets, circuit breaker, closed error set, `ShouldFallback`, `Disabled` no-op implementation. |
| `httpx` | `middleware.go`, `respond.go` | **Complete.** RequestID, Recover, Logger, SecurityHeaders+CSP, Locale. JSON error envelope, strict `DecodeJSON`. |
| `observability` | `log.go` | **Complete.** slog JSON, context-aware, **redacts sensitive keys**. |

## Modules (`internal/modules/`)

| Module | Files | State |
|---|---|---|
| `identity` | `domain.go`, `session.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `bcrypt_test.go`, `service_test.go` | **Active.** User authentication, password hashing ($2y$ Laravel + $2a$ bcrypt support), Redis session lifecycle, lockout after 5 failures, TOTP MFA, unified RBAC permission resolution across platform and org roles, Chi middleware (`RequireAuth`, `RequirePermission`, `ResolveTenant`). |
| `org` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `org_test.go` | **Active.** Dedicated bounded context: Organizations (supplier, pharmacy, chain), approval workflows, branches (enforcing single main branch), members & RBAC roles, verified reviews, followers, policies, and social media handles. |
| `catalog` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `catalog_test.go` | **Active.** Categories, brands, products (34 pharma columns preserved), product variants, Arabic trigram fuzzy search with `pg_trgm` and `platform.normalize_arabic`, customer-specific custom pricing mappings, product price/stock alerts, tenant RLS. |
| `inventory` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `inventory_test.go` | **Active.** Warehouses, stocks (**D4 Fix: `UNIQUE(warehouse_id, product_variant_id)`** allowing sibling variants to coexist), double-entry immutable stock movement ledger, inter-warehouse transfers with validation, tenant RLS. |
| `commerce` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `commerce_test.go` | **Active.** Unified order system (D2 fix), shopping carts (with item addition, removal, clear), buyer quote requests, wishlists, vendor shipment splits, line item snapshots, order state machine validation. |
| `billing` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `billing_test.go` | **Active.** Double-entry append-only wallet ledger, B2B invoices (`billing.invoices`, `billing.invoice_lines`), payment methods, payments, unified subscription plans (D7 fix), feature entitlements. |
| `ingest` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `ingest_test.go` | **Active.** Bulk spreadsheet uploads (S3 key pointer, D5 fix), heuristic column detection, Arabic similarity product matching with AI gateway escalation and deterministic fallback. |
| `promo` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `promo_test.go` | **Active.** Vendor promotional offers, sponsorship tiers, display advertisements, engagement analytics (views, clicks), homepage highlight sections, and automated expiration sweeps. |
| `workflow` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `workflow_test.go` | **Active.** Automated purchase priority engine, weekly branch route coverage, issue tracking. |
| `hr` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `hr_test.go` | **Active.** Staff profiles, exact salary compensation, weekly operating business hours. |
| `platform_admin` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `platform_admin_test.go` | **Active.** System configuration settings, geographical reference data (countries, Egyptian cities), supported currencies, UI languages, public contact message inquiries, and uploaded verification documents. |
| `notifications` | `domain.go`, `repository.go`, `service.go`, `postgres/`, `http/`, `notifications_test.go` | **Active.** Multi-channel message delivery (SMS, WhatsApp, Email, In-App), template parameter interpolation, user notification inbox, unread counts. |
| `etl` | `domain.go`, `transformer.go`, `pipeline.go`, `etl_test.go` | **Helpers & Pipeline Foundation.** Datetime parsing, money parsing, i18n text transform helpers, validation contracts for live MariaDB extraction. |
| `aicapabilities` | `domain.go`, `service.go`, `aicapabilities_test.go` | **Active.** AI-augmented catalog matching and search expansion with deterministic fallback, wired to ingest and catalog. |

## Entrypoints (`cmd/`)

| Binary | Files | State |
|---|---|---|
| `server` | `main.go`, `health.go`, `deps.go`, `routes.go` | **Active.** Starts HTTP *before* dependencies connect; mounts all 12 module routes with tenant context resolution and middleware. |
| `worker` | `main.go` | **Active.** Uses `internal/platform/queue`, River self-migration, graceful shutdown, real `orderNotificationWorker` (with retry on failure), `ingestBatchWorker` (Arabic matching + progress tracking), and `expirePromotionsWorker`. |
| `cli` | `main.go`, `seed.go` | **Active.** `migrate`, `migrate-status`, `migrate-data`, `seed` (roles, permissions, currencies, languages, Egyptian cities, settings), `health`. |

## Database (`db/migrations/`)

| Migration | State |
|---|---|
| `001_foundation` | Extensions, 13 schemas, `platform.tenant_visible()` RLS helper, `platform.normalize_arabic()`, audit log. |
| `002_identity` | Unified RBAC, `users`, `user_security`, `user_mfa`, `user_identities`, `kyc_records`. |
| `003_organizations` | `org.organizations`, `org.branches`, `org.members`. |
| `004_catalog` | `catalog.categories`, `brands`, `products` (34 pharma columns), `product_variants`. |
| `005_inventory` | `inventory.warehouses`, `stocks` (D4 fix), `stock_movements` ledger, `warehouse_transfers`. |
| `006_commerce` | `commerce.carts`, `cart_items`, `orders`, `order_shipments`, `order_lines`, `order_status_history`. |
| `007_billing` | `billing.wallets`, `wallet_transactions` ledger, `payments`, `plans`, `subscriptions` (D7 fix). |
| `008_ingest` | `ingest.file_uploads` (D5 S3 pointers), `import_sessions`, `import_rows`. |
| `009_promo` | `promo.offers`, `offer_products`, `offer_packages`, `offer_sponsorships`, `ads`. |
| `010_workflow` | `workflow.purchase_priority_engines`, `weekly_coverages`, `report_issues`. |
| `011_hr` | `hr.employees`, `hr.work_times`. |
| `012_platform_admin` | `platform_admin.system_settings`, `countries`, `cities`. |
| `013_notifications` | `notifications.templates`, `notifications.logs`. |
| `014_enhancements` | `identity.user_addresses`, `commerce.wishlists`, `org.organization_reviews`, `org.organization_followers`. |
| `015_org_extensions` | `org.organization_social_media`, `org.organization_policies`, `org.user_organization_numbers`. |
| `016_billing_invoices` | `billing.invoices`, `billing.invoice_lines`, `billing.user_payment_methods`. |
| `017_catalog_extensions` | `catalog.customer_product_mappings`, `catalog.product_alerts`, `catalog.saving_products`. |
| `018_commerce_quotes` | `commerce.quote_requests`. |
| `019_promo_tracking` | `promo.offer_views`, `promo.offer_clicks`, `promo.highlight_sections`, `promo.highlight_section_items`. |
| `020_platform_extensions` | `platform_admin.currencies`, `platform_admin.languages`, `platform_admin.contact_messages`, `platform_admin.documents`. |

---

# PART 3 — What is NOT Done (Remaining Phases)

- **Phase R (Templ + HTMX Frontend)**: Admin shell, Vendor dashboard, Customer catalogue, cart & checkout, SSE import progress wizard.
- **Phase S (Full Real ETL)**: MariaDB extraction, validation (orphan sweep), transformation, COPY load, and 2-way verification gates against live data.

---

# PART 4 — Rules you must not break

Full version in `AGENTS.md`. The ones that cause real damage if ignored:

1. **Money never touches `float64`.** Use `money.Amount`. Database columns are
   `NUMERIC(p,2)`.

2. **No AI provider name outside `internal/platform/gateway/`.** Not `openai`,
   `anthropic`, `deepseek`, `gemini`, `groq`, `openrouter`. The Store asks for a
   *capability*. `make check-provider-isolation` enforces it.

3. **Every AI capability ships with a deterministic fallback in the same change.**
   A pharmacy must be able to order when the Gateway is down.

4. **Tenant-scoped queries go through `db.InTx` / `db.InReadTx`.** Never
   `db.Pool()` in a module. Cross-tenant work uses `database.AsSystem(ctx)`.

5. **Module boundaries** — `modules/A` must not import `modules/B`. `platform/`
   must not import `modules/`. `shared/` imports nothing internal. depguard
   enforces this.

6. **400 lines per Go file.**

7. **Never edit an applied migration.** The runner checksums them and refuses.
   Add a new one.

8. **Preserve legacy behaviour exactly during migration.** Business rules,
   primary key values, `order_number` formats, money semantics. Port bugs as-is
   and record them in `docs/modules/<module>.md`. Parity against the old system
   is how correctness is proven; "improving" a rule destroys that.

---

# PART 5 — Audit findings you must know before touching the schema

These came out of the legacy database audit. Each one is a trap.

| ID | Finding | Consequence for you |
|---|---|---|
| **D1** | `users` has **137 columns** spanning 11 concerns, including `base_salary` next to `password` | Already fixed in `002_identity`. Do not reintroduce. |
| **D2** | **Two complete, parallel order systems**: `orders`+`order_items` (5 statuses) and `main_orders`+`adv_orders` (13 statuses) | Phase D must unify into `orders`→`order_shipments`→`order_lines`. **Determine which is authoritative first** by row counts and recent `created_at`. |
| **D3** | **Two 2FA implementations** on one row: `two_factor_*` and `google2fa_*` | `002_identity` keeps one (`google2fa_*`, the one with a live service). ETL must reconcile rows where they disagree. |
| **D4** | **`stocks` unique constraint is wrong**: `UNIQUE(product_id, warehouse_id)` while `product_childern_id` is `NOT NULL` | Two variants of one product **cannot coexist in a warehouse**. Silent import failures. Target must be `UNIQUE(warehouse_id, product_variant_id)`. |
| **D5** | `temporary_file_uploads.file_content` is a **BLOB in the OLTP database** | Move to object storage in Phase F. |
| **D6** | **7 views used as tables**, no PK, doing runtime `json_extract` over joins: `all_products`, `products_view`, `product_importants`, `organizations_overview`, `basic_products`, `offer_importants`, `product_infos` | Cannot be indexed. Replace with materialised views or worker-refreshed projections. |
| **D7** | **Four overlapping subscription systems** + `session_plans` | Collapse into one entitlement model. Keep `source_system` on each row for auditability. |
| **D8** | **Two RBAC systems** plus `users.he_can_do_anything` and `full_powers` | Unified in `002_identity`. ETL must produce a **reconciliation report** of every case where the two disagreed. |
| **D11** | Denormalized counters with no transactional guard: `users.total_orders`, `total_spent`, `products.sold_times`, `organizations.rating`/`rank` | Rebuild as recomputable projections. |
| **D12** | **36 `*_id` columns with no FK constraint** | Orphans already exist. ETL needs a pre-migration orphan sweep. |
| **D13** | Misspellings in the schema: `product_childerns`, `user_temparte_warehouses`, `father_user_temparte_warehouses`, `removed_from_new_clinet_at` | Renamed in target (see master plan §5.6). Legacy names reachable via `compat` views during migration only. |

## What the legacy system got RIGHT — preserve it

- **Money is `DECIMAL` everywhere** (106 columns). Only geo coordinates are float.
- **Bilingual JSON** `{"ar","en"}` with `json_valid` CHECK — ~173 columns. Maps to JSONB.
- **Price snapshotting on order lines** — `order_items.price`, `adv_orders.old_product_childern_price`.
- **Arabic column comments** — often the only documentation. Carry them into `COMMENT ON COLUMN`.
- **The `Offers` module** — the one place with FormRequests, Resources, Policies, Enums, Events, Observers.

## Most-referenced tables (migration ordering)

```
users (91 inbound FKs) → organizations (41) → products (13) → organization_branches (10)
→ offers (8) → product_childerns (5) → cities (5) → plans (4)
```

---

# PART 6 — Remaining phases

Phases are lettered to avoid confusion with the master plan's numbering. Each is
several sessions. **Do one at a time and finish it.**

---

## PHASE A — Close the foundation gaps — [COMPLETE]
**Prereq:** none. **Completed.**

### Completed Tasks
1. `docker-compose.dev.yml` configured for PostgreSQL 16, Redis 7, and MinIO.
2. `sqlc.yaml` authored with postgres engine, pgx/v5, `NUMERIC` → `money.Amount`, `JSONB` name columns → `i18n.Text`, `CITEXT` → `string`. Initial `db/queries/` configured.
3. `internal/platform/storage/storage.go` implemented using AWS SDK v2, with MinIO path-style support, `KeyFor(orgID, path)` tenant isolation, `Put`/`Get`/`Delete`/`PresignGet`/`PresignPut`/`PublicURL`, and unit tests.
4. `internal/platform/queue/queue.go` implemented wrapping River queue client and `EnqueueTx` transactional insertion, and `cmd/worker/main.go` refactored to use it.
5. Cross-tenant RLS integration test suite authored in `test/integration/rls_test.go` covering `org.branches` and `org.members`.
6. Unit test suites written for `internal/shared/i18n` (6 tests) and `internal/shared/apperr` (5 tests).
7. `.github/workflows/ci.yml` authored with full pipeline gates: fmt check, vet, AI provider isolation check, 400-line limit check, migration execution, and race-detector test suite.

---

---

## PHASE B — Authentication and identity [COMPLETE]
**Completed & Verified.**
- Identity domain (`User`, `UserSecurity`, `UserMFA`, `Role`, `Permission`).
- Native bcrypt `$2y$` Laravel and `$2a$` hash compatibility (verified with test suite).
- Redis session store with tenant isolation and session sets.
- Lockout policy (5 failed logins -> 15 min lock).
- Unified RBAC permission resolution across platform roles, system roles, and organization memberships.
- Chi HTTP handlers and middleware (`RequireAuth`, `RequirePermission`, `ResolveTenant`).
- Unit tests in `internal/modules/identity/` pass.

---

## PHASE C — Catalog and inventory [COMPLETE]
**Completed & Verified.**
- Migrations `004_catalog.up.sql` and `005_inventory.up.sql` (+ `.down.sql`).
- Catalog module (`Product`, `ProductVariant`, `Category`, `Brand`) preserving all 34 pharma columns (`dosage_form`, `scientific_name`, `pharmacology`, `active`, `concentration`, etc.).
- In-database Arabic trigram fuzzy search with `pg_trgm` and `platform.normalize_arabic`.
- Inventory module (`Warehouse`, `Stock`, `StockMovement`, `WarehouseTransfer`).
- **Legacy defect D4 resolved:** `UNIQUE(warehouse_id, product_variant_id)` allowing sibling variants of the same product to coexist in the same warehouse.
- Atomic stock adjustment with immutable append-only stock movement ledger.
- Inter-warehouse transfer workflow with double-entry ledgering.
- Unit tests in `internal/modules/catalog/` and `internal/modules/inventory/` pass.

---

## PHASE D — Commerce
**Prereq:** C. **Migration 006. Highest-risk phase.**

### Tasks
1. **Resolve D2 first.** Query both legacy order systems for row counts and recent activity. Document the answer in `docs/modules/commerce.md` before writing any schema.
2. Migration `006_commerce`: `carts`, `orders` (was `main_orders`), `order_shipments` (was `orders`), `order_lines` (was `adv_orders` + `order_items`), `order_status_history`, `invoices`.
3. One canonical status enum, superset of both legacy sets, plus a legacy→target mapping table used by the ETL.
4. `internal/modules/commerce/` — cart, checkout, order placement, status machine, vendor fulfilment.
5. **Price snapshotting** on every line. Use `money.Allocate` to split totals across shipments — never percentage rounding per part.
6. Order status transitions validated by an explicit state machine, with history written in the same transaction.

### Read in legacy
`Laravel\app\Models\{Order,OrderItem,MainOrder,AdvOrder,CartWishlist,Invoice}.php`, `Laravel\app\Services\OrderService.php`, `Laravel\app\Http\Controllers\OrderController.php`, `Laravel\app\Observers\MainOrderObserver.php`, `Laravel\app\Http\Requests\ConfirmOrderRequest.php`

### Done when
1,000 historical orders reprice **to the cent**. Every order's `SUM(lines.line_total) = order.final_price`.

---

## PHASE E — Billing, payments, entitlements
**Prereq:** D. **Migration 007.**

> **⚠ BLOCKING UNKNOWN.** `payments`, `payment_histories`, `user_payment_methods` and a **47-column `payment_integrations`** table exist, but **`composer.json` contains no payment SDK**. Determine whether payments are manual, custom HTTP to a local processor (Paymob/Fawry?), or unimplemented. **This changes the phase from 1 week to 4.** Resolve by inspecting `payment_integrations` rows and the vendor payment Livewire components.

### Tasks
1. Migration `007_billing`: `wallets`, `wallet_transactions`, `payments`, `payment_histories`, `payment_integrations`, `user_payment_methods`, `plans`, `plan_features`, `subscriptions`, `entitlements`.
2. **Collapse the four subscription systems (D7)** into one model with `source_system`/`source_id` provenance.
3. `entitlements.Check(subject, feature_key)` — the single question application code asks.
4. Wallet ledger: append-only, balance is a projection, never a mutable column.

### Done when
Wallet balances match legacy exactly. Every legacy subscription maps to exactly one target subscription with recorded provenance.

---

## PHASE F — Ingest pipeline
**Prereq:** C. **Migration 008.** Can run parallel to E.

### Tasks
1. Migration `008_ingest`: `import_batches`, `import_sessions`, `import_rows`, `import_progress`, `file_uploads`. **Files go to object storage (D5), not a BLOB column.**
2. River jobs on the `imports` queue, chunked.
3. **Column detection**: heuristic matcher first, `import.detect_columns` capability only for what the heuristic cannot resolve.
4. **Product matching**: `arabic.Similarity` + Postgres trigram **first**; escalate only the non-matches to `product.match`. This is the largest AI cost saving available — expect it to remove 60–80% of calls.
5. Honour `import_sessions.min_similarity_score` (legacy default **0.85**, tuned against the exact curve in `internal/shared/arabic`).
6. SSE progress endpoint.

### Read in legacy
`Laravel\app\Jobs\` (all 8), `Laravel\app\Services\{ColumnDetector,ProductMatcher,ArabicNormalizer}.php`, `Laravel\app\Traits\HandlesExcelImport.php`

### Done when
A real vendor file imports with the same matches as the legacy importer, **and** the whole flow works with `GATEWAY_ENABLED=false`.

---

## PHASE G — Promo / offers / ads
**Prereq:** D. **Migration 009.**

The legacy `Offers` module is the best-built part of the old system. **Port its structure, do not redesign it.**

Tables: `offers`, `offer_products`, `offer_packages`, `offer_package_features`, `offer_promotions`, `offer_sponsorships`, `offer_views`, `offer_clicks`, `offer_location_covers`, `ads`, `ad_clicks`, `ad_plans`, `highlight_sections`.

Port `OfferRotationEngine`, `OfferSponsorshipService`, `OfferViewTrackingService`, `OfferClickTrackingService`, `OfferAnalyticsService`, and `ProcessExpiredSponsorshipsCommand` (→ River periodic job).

**Done when** offer pricing matches legacy for every active offer.

---

## PHASE H — The UI
**Prereq:** B onwards; build incrementally alongside each module.

1. templ + HTMX + Alpine + Tailwind 4. **Design RTL first**, verify LTR second.
2. Design tokens once: 6 spacing steps, 5 type sizes, 3 weights, one accent, one radius. No arbitrary values in templates.
3. Three shells: Admin, Vendor, Customer.
4. Tables are the product — density, sticky headers, sorting, saved filters, keyboard nav.
5. Money right-aligned, `tabular-nums`, always 2 decimals and a currency.
6. Every state designed: loading (skeleton), empty (with the action that fills it), error (with recovery), partial.
7. Budget: < 40 KB JS total, FCP < 1.2 s on 3G.
8. **No gradients, no glassmorphism, no decorative icons, no emoji.**

---

## PHASE I — Remaining domains
`workflow` (automation requests, `PurchasePriorityEngine`, `WeeklyCoverage`, `InstitutionalWork`), `hr` (employees, `work_times`, job board), `platform_admin` (settings, translations, countries/cities/currencies, system resources).

---

## PHASE J — The legacy data ETL
**Prereq:** all schema phases. **Master plan §11 is the detailed spec.**

A Go program under `cmd/cli` (`migrate-data`) — testable, resumable, idempotent. Six stages: Extract → Validate → Transform → Load → Verify → Reconcile.

**Preserve legacy primary key values.** User 4417 stays user 4417.

Verification gates (all must pass per table): row counts exact · checksums · money sums **to the cent** · zero FK orphans · 100% JSON parses · timestamps match after explicit UTC→TIMESTAMPTZ conversion · business invariants · 100 orders spot-checked by a human.

---

## PHASE K — AI capabilities
**Prereq:** F, and a Gateway virtual key.

Wire the six capabilities in `internal/modules/aicapabilities/`. Prompts versioned in-repo. **Write the black-hole test suite**: run the full order and import flows with `GATEWAY_BASE_URL` pointed at an unroutable address and assert nothing user-facing breaks.

---

## PHASE L — Cutover
**Prereq:** everything. **Master plan §11.5 and §13.**

Write and **test** the reverse-ETL (PostgreSQL → MariaDB) *before* cutover. Rehearse twice on staging. Point of no return is the first write accepted by the Go app.

---

# PART 7 — How to work in this repo

## Every session

1. Read `AGENTS.md` and this file's Part 4 and Part 5.
2. Read `docs/modules/<module>.md` if it exists for what you are touching.
3. **Read the legacy code before writing the Go code.** Business rules exist only there.
4. Work on **one module**. Do not touch two.
5. Run `make check` (or the raw `go` commands) before committing.
6. **Update this file's Part 2 and Part 3** so the next session starts accurate.
7. Write `docs/modules/<module>.md` as you go: entities, invariants, events, and **every legacy quirk you decided to preserve**.

## Verification commands

```bash
go build ./...
go vet ./...
go test -race -count=1 ./...
gofmt -l .
go run ./cmd/cli migrate-status
```

`make` was not installed on the original machine. On Windows use the raw
commands above; CI runs on Ubuntu where `make` works.

## Provider-isolation check (run manually if `make` is unavailable)

```bash
grep -riE '\b(openai|anthropic|deepseek|gemini|groq|openrouter)\b' --include='*.go' --include='*.sql' ./cmd ./internal ./db | grep -v '^./internal/platform/gateway/' | grep -v '_test.go'
```

Any output is a build-breaking violation.

---

# PART 8 — Open questions blocking later phases

| ID | Question | Blocks | How to resolve |
|---|---|---|---|
| **U1** | Is the Laravel app live in production with real vendors and money? | Phase L strategy — convert-and-cut vs strangler. **~8 weeks difference.** | Ask the owner; check MariaDB row counts |
| **U2** | How are payments actually settled? No SDK in `composer.json`. | Phase E scope (1 week vs 4) | Inspect `payment_integrations` rows + vendor payment Livewire |
| **U3** | Does any code path execute **LLM-generated SQL**? | Phase K, and a possible **critical vulnerability in the live system** | Read `SqlQueryHistory`, `ChatTree`, `AiTest` Livewire components |
| **U4** | Which of the 4 subscription systems are actually in use? | Phase E mapping | Row counts per family |
| **U5** | Which of the 2 order systems is authoritative? | **Phase D — blocking** | Row counts + recent `created_at` in both |
| **U6** | Are `routes/old_admin.php` / `old__vendor.php` registered? | Feature inventory | Read `Laravel\bootstrap\app.php` |
| **U10** | Is the `Testv5` dump representative of production schema? | All schema phases | Diff against production `SHOW CREATE TABLE` |

---

# PART 9 — Deployment status

- **Store:** never successfully deployed. Build previously failed on a missing
  `internal/ui/static` (fixed, `d5c46ea`). Needs PostgreSQL provisioned via
  `dawa24-ops/postgres/01-bootstrap.sql` and `02-extensions.sql`.
- **Gateway:** an instance is running at the `dawa24-store-u74003.vm.elestio.app`
  domain — **this is the Gateway, not the Store**, deployed against production
  Gateway data. Its `/admin` panel is publicly reachable and should be locked
  down or removed.
- **Ports** (chosen to avoid an existing project): Postgres `5442`, Redis `6389`,
  MinIO `9010`/`9011`, Gateway `8091`, Store `8081` (the working compose
  currently uses `8070` — match it to the reverse proxy target).
- **Redis** is deployed at `redis2-u74003.vm.elestio.app:26381`. Its password was
  exposed in a chat transcript and **should be rotated**.
