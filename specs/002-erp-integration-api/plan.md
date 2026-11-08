# Implementation Plan: ERP Integration API

**Branch**: `codex/audit-batches` (feature folder `specs/002-erp-integration-api`; the Spec Kit scripts report branch `002-erp-integration-api`) | **Date**: 2026-11-07 | **Spec**: [spec.md](spec.md)

**Reader**: a model with no memory of this project. Read this file top to bottom, then `data-model.md`, `contracts/error-codes.md`, `contracts/openapi.yaml`, `research.md`. Do each slice in order; a slice is shippable on its own. Do not decide anything this plan already decided.

**Field names**: `contracts/orgasoft-samples.md` holds the real Orgasoft samples and overrides any other spelling in these files. Item fields are `arName` and `publicPrice`, not `name` and `price`. `quotaQuantity` may be 0.

## Summary

A new Go module `internal/modules/erp` gives ERP systems (first: Orgasoft) a machine API under `/erp/v1` with per-integration API keys, a queued product sync with per-item results, sales and purchase order feeds with import tracking, confirm and reject through the website rules, ERP customer codes taken from the supplier link page, purchase orders at Dawa24 prices, Reveal for sensitive values, and an OpenAPI file, Postman collection and docs. Provider differences sit behind `erp/provider`. Nothing in the existing `/api/v1` or n8n routes changes.

## Technical Context

**Language/Version**: Go 1.26 (`go.mod`), chi v5, pgx v5, River v0.43.0, templ v0.3.1020
**Storage**: PostgreSQL, new schema `erp` (migrations 255 to 258), Redis for rate limiting only
**Testing**: `go test` (table-driven unit tests, real-Postgres integration tests in `test/integration`), `make check`
**Target Platform**: Linux containers, one image, three binaries (`cmd/server`, `cmd/worker`, `cmd/cli`)
**Project Type**: modular monolith web service plus background worker
**Performance Goals**: 50,000-item batch accepted in under 5 s; processed in the 7-minute window; website latency unchanged with 50 tenants syncing
**Constraints**: 400 lines per Go file; money only through `internal/shared/money`; every message bilingual; no module imports another module; River reaper kills any job running over 15 minutes
**Scale/Scope**: about 50 tenants, 50,000 products per sync, a few thousand orders a day

## Constitution Check

*GATE: pass before research; re-checked after design (below).*

| Principle (`.specify/memory/constitution.md`) | Status | How the design satisfies it |
|---|---|---|
| I Money is exact | Pass | Every amount uses `money.Amount`; JSON decoded with `UseNumber()`; DB `NUMERIC(…,2)`; tax and discount are `NUMERIC(5,2)`/`(12,2)` percents parsed with `money.Parse`; no `float` anywhere in `erp` |
| II Arabic first | Pass | Every API message is `{ar,en}` from i18n keys (`catalog_erp.go`); every new column has an Arabic comment; UI pages are RTL first, built from existing components |
| III AI behind capabilities, with fallback | Pass | `erp` names no provider and never calls the Gateway. Sync calls `Matcher.Prepare` (deterministic, includes remembered knowledge). It never calls `StartEnhancement`. Gateway-off test `TestSyncWithGatewayOff` is required |
| IV Tenants isolated in the database | Pass with named exceptions | All `erp` tables have RLS plus tests `TestTenantIsolation_ERP*`. Cross-tenant access is limited to the seven exceptions (E1 to E8) in "Named exceptions" and each name ends in `AsSystem` or `AsStaff` |
| V Module boundaries | Pass | `erp` imports only `shared/*` and `platform/*`. Other modules are reached through ports in `erp/ports.go`, adapted in `internal/composition/erpports`, `cmd/server`, `cmd/worker`. `test/module_boundaries_test.go` stays green |
| VI Small files, safe migrations | Pass | File plan keeps every file under 400 lines; migrations 255 to 258 are expand-only |
| VII This platform is the source of truth | Pass | Behaviour changes are recorded in `docs/modules/erp.md`, `commerce.md` (order numbers), `org.md` (customer code guard), `catalog.md` (quota writer) |

### Named exceptions (not violations; each needs a comment citing this table)

| # | Function (all in `erp/postgres`) | Why it must be `AsSystem` | Guard |
|---|---|---|---|
| E1 | `FindKeyByPrefixAsSystem` | Authentication happens before the tenant is known | `WHERE key_prefix = $1`, returns one row |
| E2 | `ListSalesShipmentsAsSystem`, `GetSalesShipmentAsSystem` | `commerce.orders` RLS is keyed to the buyer organization, so a supplier cannot read the order header or the buyer's branch (`org.branches` is tenant-scoped to the buyer) | every statement has `s.organization_id = $1` taken from the principal; the branch join also requires `b.organization_id = o.organization_id` |
| E3 | `PurgeExpiredAsSystem` | retention job spans tenants; it also deletes `catalog.match_runs` rows created by ERP sync (research D-01, D-14) | fixed table list, `created_at` cut-off; the `catalog.match_runs` delete is limited to `consumer_ref LIKE 'erp.sync:%'` |
| E4 | `InsertCallsAsSystem` | failed-auth rows have no organization | insert only |
| E5 | `ListKeysAsStaff`, `RevokeKeyAsStaff` | staff see and revoke any key (FR-001) | called only by `/admin/integrations*` behind `platform.integration.*` |
| E6 | `ListOffersAsSystem`, `VariantOfferAsSystem` (in `erp/postgres/offers.go`) | the supplier variants belong to other tenants: `catalog.product_variants` policy is `tenant_visible(organization_id)` (`004_catalog.up.sql:161`), so a buyer under `WithTenant(buyer)` sees none; the website's own quote path (`apiQuoteResolver.quoteProduct`, `cmd/server/api_checkout_quote_line.go:65`) also uses `database.AsSystem` | `status='active' AND deleted_at IS NULL`, supplier organization type vendor and approved, `stock>0`, `LIMIT 50`; returns only public fields (variant public id, supplier name, product name, barcode, prices, in-stock flag, linked flag) |
| E7 | `ListPurchaseShipmentsAsSystem(ctx, buyerOrgID int64, orderIDs []int64)` (in `erp/postgres/orders_purchase.go`) | `commerce.order_shipments` and `commerce.order_lines` carry the supplier's organization id and their policies use `tenant_visible(organization_id)` (`006_commerce.up.sql:124,155`), so under `WithTenant(buyer)` the order header is visible but its shipments and lines are not | every statement joins `commerce.orders o ON o.id = s.order_id` with `o.organization_id = $1` (the buyer organization from the principal) |
| E8 | `SupplierERPStatusAsSystem(ctx, vendorOrgIDs []int64) (map[int64]bool, error)` (in `erp/postgres/customer_code.go`) | the checkout and the order feed run under the buyer's tenant, which cannot see a supplier's `erp.integrations` or `erp.api_keys` rows (`tenant_visible`), so "does this supplier use ERP" cannot be answered under the buyer | read-only; `organization_id = ANY($1)`, integration `status = 'active'`, and a key with `status = 'active'`, `revoked_at IS NULL` and `expires_at` unset or in the future (the same predicates as `authenticate.go`); returns one boolean per supplier, no key data |

Test `TestAsSystemOnlyInNamedFunctions` (in `erp/postgres`) parses the package and fails if `database.AsSystem(` appears in a function whose name does not end in `AsSystem` or `AsStaff`.

### Post-design re-check (after Phase 1)

All seven principles still pass. No unjustified violation. The three design choices that touch shared code are each expand-only and have tests: `AllowKey` on `httpx.Limiter`, two audit columns, and an optional repository interface for order numbers.

## Project Structure

### Documentation (this feature)

```text
specs/002-erp-integration-api/
├── plan.md  research.md  data-model.md  quickstart.md
├── contracts/openapi.yaml  contracts/error-codes.md
└── tasks.md            # generated
```

### Source code (exact paths; N = new, C = changed)

```text
db/migrations/
  N 255_erp_schema.up.sql / .down.sql
  N 256_audit_actor_kind.up.sql / .down.sql
  N 257_user_org_erp_code_index.up.sql / .down.sql
  N 258_order_number_sequence.up.sql / .down.sql

internal/modules/erp/                      package erp
  N limits.go scopes.go errors.go domain.go
  N keys.go keys_test.go keys_service.go
  N service.go ports.go repository.go repository_sync.go repository_orders.go
  N item.go item_test.go                   normalise + validate one product item
  N sync_intake.go sync_process.go sync_resolve.go sync_write.go sync_review.go
  N orders_service.go purchase_service.go customer_code.go
  N calllog.go cleanup.go
  N *_test.go                              colocated, table-driven
internal/modules/erp/provider/             N provider.go registry.go all/all.go
internal/modules/erp/provider/orgasoft/    N orgasoft.go fields.go ack.go orgasoft_test.go
internal/modules/erp/postgres/             N repository.go keys.go integrations.go sync.go sync_items.go review.go
                                             variants_lookup.go orders_sales.go orders_purchase.go imports.go
                                             idempotency.go customer_code.go offers.go calls.go cleanup.go audit.go
internal/modules/erp/http/                 N handler.go auth.go respond.go ratelimit.go cursor.go body.go health.go
                                             sync_handlers.go orders_handlers.go purchase_handlers.go spec.go
internal/modules/erp/http/spec/            N openapi.yaml            (copy of contracts/openapi.yaml)
internal/modules/erp/jobs/                 N sync_chunk_worker.go cleanup_worker.go

internal/composition/erpports/             N catalog.go inventory.go matching.go   (imports modules; imported only from cmd/)
internal/shared/mask/                      N mask.go mask_test.go                  (S8)
internal/shared/i18n/                      N catalog_erp.go    C catalog.go (call loadErpKeys)

internal/platform/config/                  N erp.go            C config.go (field ERP, loadERP, queue erpsync)
internal/platform/queue/                   C jobs.go (two Args) C queue.go (NewInsertOnly)
internal/platform/httpx/                   C ratelimit.go (AllowKey)
internal/platform/database/                C audit.go (ActorKind, ActorKeyID)
internal/platform/rbac/                    C catalog_vendor.go catalog_pharmacy.go catalog_admin.go
                                             nav_vendor.go nav_pharmacy.go nav_admin.go

internal/modules/catalog/                  N variant_quota.go  N postgres/variant_quota.go
internal/modules/commerce/                 N order_number.go   N postgres/order_number.go   C service.go   (S7)

internal/ui/                               N erp_keys_handlers.go erp_keys_forms.go erp_review_handlers.go
                                             erp_admin_handlers.go erp_customer_code.go erp_order_decision.go reveal_handlers.go
                                           C handlers.go (field erpSvc, SetERP) N vendor_routes_erp.go C vendor_routes.go (one call line only; file is 384 lines) C customer_routes.go
                                             C admin_routes_platform.go C buying_routes.go C handlers.go (RegisterApprovedSharedRoutes)
                                           C vendor_order_handlers.go C customer_checkout_core.go C customer_checkout_handlers.go
                                             C customer_user_org_handlers.go C vendor_user_org_handlers.go
internal/ui/pages/                         N erp_keys.templ erp_key_modals.templ erp_review.templ admin_erp_keys.templ
                                             checkout_erp_code_modal.templ  C customer_checkout.templ
                                           C admin_org_detail.templ vendor_order_detail.templ admin_developers_ai.templ (S8)
internal/ui/components/                    N reveal_field.templ (S8)    (run templ generate: *_templ.go files are committed)

cmd/server/                                N erp_wiring.go erp_ports.go erp_late.go   C routes.go C deps.go
cmd/worker/                                N erp.go           C main.go C matching.go
cmd/cli/                                   N erp_postman.go erp_postman_items.go erp_demo.go   C main.go (`erp-postman` in `main()` before `run()`, `erp-demo` in the `run()` switch)

test/integration/                          N erp_isolation_test.go erp_sync_flow_test.go erp_sync_scale_test.go erp_orders_test.go erp_po_test.go
test/load/                                 N erp_sync.js
docs/erp-api/                              N README.md dawa24-erp.postman_collection.json dawa24-erp.postman_environment.json
docs/modules/                              N erp.md   C commerce.md org.md catalog.md
docs/adr/decisions.md                      C append ADR-ERP-1
AGENTS.md                                  C add `erp` to the schema list
```

**Structure decision**: the repository's standard module layout (`domain/service/repository`, `postgres/`, `http/`, `jobs/`), plus `provider/` for the ERP seam.

## Names registry (use exactly these)

- **API**: prefix `/erp/v1`; header `Authorization: Bearer <key>`; key `d24_<live|test>_<8 chars>_<43 chars>`.
- **Scopes** (`erp/scopes.go`): `ScopeProductsSync = "erp.products.sync"`, `ScopeOrdersRead = "erp.orders.read"`, `ScopeOrdersAck = "erp.orders.ack"`, `ScopeOrdersDecide = "erp.orders.decide"`, `ScopePurchaseCreate = "erp.po.create"`. Defaults: vendor key gets the first four; customer key gets `erp.orders.read`, `erp.orders.ack`, `erp.po.create`.
- **Limits** (`erp/limits.go`, one constant each, nothing else in the code repeats a number; the only copies are the migration defaults in `data-model.md` and `config.ERP.DefaultRate` (600) and `config.ERP.MaxBatchItems` (50000), because `platform/config` cannot import `erp`, and `TestERPDefaultRateMatchesLimit` in `cmd/server/erp_wiring_test.go` asserts the config defaults equal the constants): `KeyPrefixLen=8`, `DefaultRatePerMinute=600`, `MinRatePerMinute=10`, `MaxRatePerMinute=6000`, `SyncBatchesPerMinute=6`, `AuthFailuresPerIPPerMinute=30`, `MaxBatchItems=50000`, `MaxBodyBytes=32<<20`, `MaxInflatedBytes=128<<20`, `SyncChunkSize=2500`, `SyncSnoozeSeconds=15`, `PageDefault=100`, `PageMax=500`, `SyncItemsPageDefault=1000`, `SyncItemsPageMax=5000`, `AckMax=500`, `PurchaseLinesMax=200`, `OffersMax=50`, `RotateGraceDefault=24h`, `RotateGraceMax=72h`, `LastUsedTouchEvery=1m`, `IdempotencyStaleAfter=2m`, `RetentionAPICalls=30d`, `RetentionSyncItems=7d`, `RetentionSyncBatches=90d`, `RetentionReview=180d`, `RetentionMatchRuns=30d`, `CleanupBatchRows=10000`.
- **Config keys**: `ERP_API_ENABLED` (default false), `ERP_KEY_PEPPER`, `ERP_DEFAULT_RATE_PER_MINUTE`, `ERP_MAX_BATCH_ITEMS`, `WORKER_ERPSYNC`.
- **Queue**: `erpsync` (workers `WORKER_ERPSYNC`, default 3). **Job kinds**: `erp.sync_chunk` (`queue.ERPSyncChunkArgs{BatchID, OrganizationID, ChunkNo int}`, `Queue: "erpsync", MaxAttempts: 3`), `erp.cleanup` (`queue.ERPCleanupArgs{}`, `Queue: "maintenance", MaxAttempts: 1`, periodic every 24 h).
- **Permission keys**: `vendor.integration.view`, `vendor.integration.manage`, `vendor.integration.review`, `pharmacy.integration.view`, `pharmacy.integration.manage`, `platform.integration.view`, `platform.integration.revoke`, `platform.reveal.sensitive`, `vendor.reveal.customer_contact`.
- **Web routes** (all new): `GET /vendor/integrations`, `POST /vendor/integrations/keys`, `POST /vendor/integrations/keys/{id}/rotate`, `POST /vendor/integrations/keys/{id}/revoke`, `POST /vendor/integrations/keys/{id}/limits`, `POST /vendor/integrations/pause`, `GET /vendor/integrations/review`, `POST /vendor/integrations/review/{id}/resolve`, `POST /vendor/integrations/review/{id}/dismiss`; the same six key routes under `/customer/integrations`; `GET /admin/integrations`, `POST /admin/integrations/keys/{id}/revoke`; `POST /checkout/erp-customer-code`; `POST /reveal`.
- **i18n keys**: `erp.err.*` (one per code, see error-codes.md), `erp.keys.*`, `erp.review.*`, `erp.checkout.*`, `reveal.*`, plus `customer.userorg.number_taken`.
- **Provider name**: `provider.DefaultProvider = "orgasoft"` (declared once in `erp/provider/registry.go`); every other place uses the constant or `provider.Names()` and never types the string.
- **Go**: `erp.Principal`, `erp.Service`, `erp.NewService(repo Repository, ports Ports, cfg Config, log *slog.Logger)`, `erp.Repository`, `erp.Ports`, `erp.Error` (`Code`, `Status`, `Details`), `erp.ErrWaitForEarlierBatch`, `erpHttp.NewHandler(svc *erp.Service, cfg erp.Config, lim *httpx.Limiter, log *slog.Logger)`, `postgres.New(db *database.DB) *Repository`, `jobs.NewSyncChunkWorker(svc *erp.Service, log *slog.Logger)`, `jobs.NewCleanupWorker(svc *erp.Service, log *slog.Logger)`.

### Ports (declare in `erp/ports.go`; adapters written in `internal/composition/erpports`)

```go
type VariantWriter interface {
    Write(ctx context.Context, orgID int64, rows []VariantWrite) (VariantWriteResult, error)   // catalog.Service.BulkWriteVariants
    SetQuotaLimits(ctx context.Context, orgID int64, limits map[int64]*int) error               // catalog.Service.SetVariantQuotaLimits (new)
}
type StockWriter interface {
    Replace(ctx context.Context, rows []StockWrite) (map[int]string, error)                    // inventory.Service.BulkWriteStocks(StockReplace); map = failures by Ref
}
type Matcher interface{ Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) }
type MatchMemory interface{ Remember(ctx context.Context, sc productmatch.Scope, k KnowledgeWrite) error }
type OrderDecider interface{ Decide(ctx context.Context, d ShipmentDecision) (ShipmentDecisionResult, error) }   // server only, S4
type PurchasePlacer interface{ Place(ctx context.Context, p PurchaseRequest) (PlacedOrder, error) }               // server only, S7
type Enqueuer interface{ EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) }
```
`erp` may import `productmatch` and `matchflow` (they are in `shared/`). `KnowledgeWrite` mirrors `matching.KnowledgeWrite` (`internal/modules/catalog/matching/domain.go:104`) so the adapter converts field by field.

### Existing code to call (verified)

| Need | Call | Where |
|---|---|---|
| Tenant transaction | `db.InTx`, `db.InReadTx`, `database.WithTenant` | `internal/platform/database/query_helpers.go:35,133,141` |
| Org status and type at auth time | `db.QueryRowUnscoped` on `org.organizations` | `query_helpers.go:115`; table listed in `UnscopedTables` |
| Audit row in the same tx | `database.WriteAudit(ctx, tx, AuditEntry)` | `internal/platform/database/audit.go:68` |
| Write variants in bulk | `catalog.Service.BulkWriteVariants(ctx, orgID, []catalog.VariantWriteRow)` | `internal/modules/catalog/vendor_import.go:90` |
| Write balances | `inventory.Service.BulkWriteStocks(ctx, inventory.StockReplace, []inventory.StockWriteRow)` | `internal/modules/inventory/vendor_import.go:90` |
| Match | `matching.Service.Prepare`, `matching.Service.Remember` | `internal/modules/catalog/matching/service.go:87`, `knowledge_service.go:12` |
| Row for the matcher | `productmatch.Row`, `productmatch.QuestionSignature`, `productmatch.ExtractRowFacts` | `internal/shared/productmatch/row.go:35`; used at `matching/service.go` step 3 |
| Insert job in a tx | `queue.Client.EnqueueTx` | `internal/platform/queue/queue.go:159` |
| Requeue later | `river.JobSnooze(d)` | River v0.43 |
| Supplier shipment transition | `commerce.Service.TransitionShipmentStatusWithWallet(ctx, shipmentID, to, changedBy *int64, notes, walletUserID)` | `internal/modules/commerce/fulfilment.go:79` |
| Wallet pre-check and buyer notice | `prepareWalletShipmentCharge`, `notifyOrderStatusChanged` | `internal/ui/customer_checkout_wallet.go:87`, `internal/ui/notifications_events_commerce.go:15` |
| Place a purchase order | `apiQuoteResolver.quote`, `commerce.Service.CheckoutAPI` | `cmd/server/api_checkout_quote.go:28`, `internal/modules/commerce/api_checkout.go:22` |
| Per-key limiter base | `httpx.Limiter.allow` (private; wrap it) | `internal/platform/httpx/ratelimit.go:149` |
| Late-bound handler pattern | `lateOrderAnnouncer` | `cmd/server/order_announce_wiring.go` |
| Optional-interface pattern | `shipmentWalletUpdater` type assertion | `internal/modules/commerce/api_cancel.go:107`, used at `fulfilment.go:140` |

## Traps (read all before coding)

1. **`AsSystem`**: never for tenant reads. Only E1 to E8 above, in functions named `…AsSystem`/`…AsStaff`. Purchase order headers are readable under `WithTenant(buyer)`; their shipments and lines are read through E7, and supplier offers through E6. Do not add any other system read. Everything else runs `database.WithTenant(ctx, principal.OrganizationID)` taken from the key row, never from a header, query or body.
2. **Orders RLS is keyed to the buyer** (`006_commerce.up.sql:86`). A supplier under `WithTenant` reads its own `order_shipments` and `order_lines` (their policies use the shipment's organization) but sees no `commerce.orders` header. That is why E2 exists. Every E2 statement filters `s.organization_id = $1`.
3. **Reaper vs timeout**: `queue.Client.Start` calls `StartStalledJobReaper(ctx, 3m, 15m)` (`queue.go:96`) while `DefaultJobTimeout` is 30 minutes. Any job running over 15 minutes is cancelled. Chunks are 2,500 items; never raise `SyncChunkSize` above 5,000 and never make one job process a whole batch.
4. **Queue must be configured**: add `"erpsync": getInt("WORKER_ERPSYNC", 3)` to `Worker.Queues` in `internal/platform/config/config.go` in the same commit as `ERPSyncChunkArgs`; `TestEveryInsertedQueueIsConfigured` scans for `Queue: "…"` literals and fails otherwise.
5. **No River unique jobs exist** in this repo. Do not use `river.UniqueOpts`. Ordering is by snooze (research D-09); duplicates are harmless because links and result rows are idempotent.
6. **Retirement mode must never run**: do not call `RetireVariantsExcept` or anything in `ingest/catalog_commit_retirement.go`. A product absent from a batch is untouched.
7. **Delivery cannot be set**: the status endpoint accepts only `confirmed` and `rejected`. `delivered|shipped|completed|out_for_delivery|in_transit` return 403 `ERP_DELIVERY_NOT_ALLOWED`. (`transitionShipment` also refuses `delivered` when `DeliveryPINRequired`; do not rely on that alone.)
8. **Wallet confirm charges the wallet**: confirm must go through `TransitionShipmentStatusWithWallet` with the wallet user from `prepareWalletShipmentCharge`, in one transaction. Never call `UpdateShipmentStatus` directly.
9. **Do not reuse `/api/v1` module routes**, `MODULE_API_ENABLED`, `httpx.Error` (single language, 422 for validation), or `RequireAPIPermission` (refuses non-staff).
10. **Module A may not import module B** (`test/module_boundaries_test.go`). `erp` imports no sibling module. Wire in `cmd/server`, `cmd/worker` and `internal/composition/erpports`. The `internal/ui` package may import modules (it already does) and may import `erp`.
11. **400-line limit** (`make check-file-size`); the plan's file split keeps each file small. Split before you reach 350.
12. **Bilingual messages, no Arabic literals in Go** (`make check-hardcoded-arabic`): add every string to `catalog_erp.go` and read it with `i18n.T`. The Arabic in `error-codes.md` is the source text.
13. **Money only via `internal/shared/money`**: `UseNumber()` on the JSON decoder; `money.Parse(string)`; percent values are parsed the same way and compared with `money.FromMajor(100)`. A `float64` anywhere in `erp` is a bug.
14. **Order number collisions**: `commerce.checkout` uses an in-process counter (`service.go:14-17,155`). Do not call `CheckoutAPI` from the ERP until migration 258 and `OrderNumberAllocator` ship (slice S7).
15. **Audit actor kind**: `authctx.ActivityAudit` returns early for non-user actors (`activity.go:25`), so machine calls need `ActorKind: "api_key"`, `ActorKeyID` on `database.AuditEntry` and a `WriteAudit` inside the same transaction as the change. Migration 256 comes before any code that writes those fields.
16. **Do not reuse the ingest pipeline**: `ingest.Service` runs staging in `go func()` in the web process around upload sessions (`catalog_stage_background.go`) and is not built in the worker. Compose `Matcher.Prepare` + `BulkWriteVariants` + `BulkWriteStocks` instead. Do not import or copy `ingest`.
17. **Decision memory target**: `catalog.match_knowledge`, `Kind: "supplier_code"` (human review: `"human_confirm"`), `SourceNamespace: "supplier:<orgID>"` (same as `ingest/catalog_stage_run.go:94`). The CHECK on `kind` already allows `supplier_code` (migration 239).
18. **AI fallback**: never call `StartEnhancement` or any Gateway method from `erp`. A test with no Gateway must pass (slice S2).
19. **Tenant isolation test per new tenant table**, named `TestTenantIsolation_ERP<Table>` so the CI step that runs `-run TestTenantIsolation` as the application role (`.github/workflows/ci.yml:85-88`) picks it up. Use time-based fixture ids (commit `2598436`), never small constants.
20. **The web process has no River client** (`queue.New` is called only in `cmd/worker/main.go:152`). Add `queue.NewInsertOnly(db *database.DB, log *slog.Logger) (*Client, error)` (no Queues, no Workers) and wrap it in a lazy `erpEnqueuer` in `cmd/server/erp_wiring.go`, because the database handle is not connected at mount time (`cmd/server/deps.go`).
21. **New schema needs the app-role grant** (block at the end of 255); `222_dawa24_app_role.up.sql` lists schemas by name.
22. **templ output is committed**: after editing a `.templ`, run `go run github.com/a-h/templ/cmd/templ generate` and commit the `_templ.go` files.
23. **Dead code ratchet** (`test/deadcode_ratchet_test.go`): do not add exported functions that nothing calls yet. Each slice adds only what it wires.
24. **Never log or store a key**: only `key_prefix`. The key goes in the `Authorization` header, never a query string. `erp.api_calls.route` stores the chi pattern, not the URL.
25. **Idempotent everywhere**: sync (links + hash), ack (unique row), decide (state check), PO (idempotency table). A retry or double delivery must never create a second row.
26. **`orders` GET must not leak other suppliers' lines**: build lines from `commerce.order_lines` filtered by the supplier's shipment id only.
27. **Cursor**: shipment/order `id` ascending only (research D-16). Do not sort by `updated_at`.
28. **Provider seam**: no code outside `erp/provider/orgasoft` may mention the string `orgasoft` except `erp/provider/registry.go` and `erp/provider/all/all.go` (the registry line and `DefaultProvider`), tests and docs. Elsewhere use `provider.DefaultProvider` or `provider.Names()`.
29. **Gate files**: new routes must be declared in the permission catalog before use (`TestEveryRouteGateNamesADeclaredPermission`); new UI registrars must stay inside the five audience functions (`TestUIRoutesAreAudienceGated`).
30. **Postgres closing line**: a `CREATE TABLE` in a migration must end with `);` alone on its line (`test/schema_consistency_test.go` regex).

## Phases

- **Phase 0 Research**: done (`research.md`, 28 decisions).
- **Phase 1 Design**: done (`data-model.md`, `contracts/`, `quickstart.md`).
- **Phase 2 Delivery**: the slices below. `/speckit-tasks` turns each step into a task.

### Cross-slice test rules

Each slice names its tests; these apply to all: table-driven unit tests next to the code; repository tests on real Postgres in `test/integration` (they skip without a database, so run them: `go test -p 1 ./test/integration/ -run <Name>`); exact money assertions (`"62.50"`, never a tolerance); every handler test asserts the error envelope has `ar` and `en`.

---

## S1: Keys and the machine principal (spec P1, US1)

Ships alone: owners and staff can create, rotate and revoke keys; `GET /erp/v1/health` works; nothing else is callable yet.

| # | Write this file first to last | What | Done when (command) |
|---|---|---|---|
| 1 | `db/migrations/255_erp_schema.up.sql` + `.down.sql` | all nine tables, RLS, comments, grants exactly as `data-model.md` | `make migrate` ok; `go test ./test/ -count=1` ok |
| 2 | `256_audit_actor_kind.up.sql` + `.down.sql` | two audit columns | `make migrate` ok |
| 3 | `internal/platform/database/audit.go` | add `ActorKind string`, `ActorKeyID *int64` to `AuditEntry`; default `"user"` when empty; extend the INSERT | `go test ./internal/platform/database/ -count=1` |
| 4 | `internal/platform/config/erp.go`, edit `config.go` | `type ERP struct{Enabled bool; KeyPepper string; DefaultRate, MaxBatchItems int}`; `loadERP(env, fail)`; default disabled; fail when enabled in prod and pepper < 32 bytes; add `erpsync` queue configuration now | `go test ./internal/platform/config/ ./internal/platform/queue/ -count=1` |
| 5 | `internal/platform/httpx/ratelimit.go` | `func (l *Limiter) AllowKey(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, count int64, retryAfter time.Duration, err error)` (fails open on Redis error: returns `allowed=true`) + test | `go test ./internal/platform/httpx/ -run AllowKey -count=1` |
| 6 | `internal/shared/i18n/catalog_erp.go`, edit `catalog.go` | `loadErpKeys(e)`: every code in `error-codes.md` as `erp.err.<code>`, plus `erp.keys.*` strings for the pages | `go test ./internal/shared/i18n/ -count=1` |
| 7 | rbac files | add the keys in "Permission keys" for S1 (`vendor.integration.view/manage`, `pharmacy.integration.view/manage`, `platform.integration.view/revoke`) in the same helper style as `vendor.quota.view` (`catalog_vendor.go:117`); `manage` implies `view`; nav entries `integrations` in `nav_vendor.go` (near line 109), `nav_pharmacy.go`, `nav_admin.go` | `go test ./internal/platform/rbac/ ./test/ -count=1` |
| 8 | `erp/limits.go`, `scopes.go`, `errors.go`, `domain.go` | constants and types from the registry; `erp/provider/registry.go` with only `const DefaultProvider = "orgasoft"` and `Names()` (task T042a; slice S2 extends it); `errors.go` has `type Error struct{Code string; Status int; Details map[string]any}`, `func NewError(code string, status int, details map[string]any) *Error`, `var AllCodes []string` | `go build ./internal/modules/erp/` |
| 9 | `erp/keys.go` + `keys_test.go` | `GenerateKey(tag string, pepper []byte) (full, prefix, hash string, err error)`, `ParseKey(raw string) (prefix, secret string, ok bool)`, `HashSecret(pepper []byte, secret string) string`, `VerifyHash(pepper []byte, secret, hash string) bool` (`hmac.Equal`). Tests: format regex, round trip, wrong secret, 1,000 keys have unique prefixes | `go test ./internal/modules/erp/ -run Key -count=1` |
| 10 | `erp/repository.go`, `service.go`, `ports.go` | interfaces and `Service`; `NewService` | build |
| 11 | `erp/postgres/repository.go`, `keys.go`, `integrations.go`, `audit.go`, `calls.go` | SQL for keys/integrations; `FindKeyByPrefixAsSystem`; `TouchLastUsed` (only if `last_used_at` older than 1 min); `InsertCallsAsSystem` (pgx `CopyFrom` inside `db.InTx(database.AsSystem(ctx), ...)`, never `db.Pool()`) | `go test ./internal/modules/erp/postgres/ -count=1` |
| 12 | `erp/keys_service.go` | `CreateKey(ctx, CreateKeyInput) (CreatedKey, error)` (owner only; validates warehouse and branch belong to the org, buying user is an active `org.members` row; creates the integration if none; audit in tx), `RotateKey(ctx, orgID, keyID, grace time.Duration, actorUserID, actorIsOwner)` (copies scopes, warehouse, branch, buying user, rate limit and max batch items from the old key), `RevokeKey(ctx, orgID, keyID, reason, actorUserID)`, `UpdateKeyLimits(ctx, orgID, keyID int64, ratePerMinute int, actorUserID int64, actorIsOwner bool) error` (owner only; range `MinRatePerMinute` to `MaxRatePerMinute`; audit row), `PauseIntegration`, `ListKeys(ctx, orgID)`, `ListKeysAsStaff`, `RevokeKeyAsStaff`, `Authenticate(ctx, raw, ip) (Principal, error)` | `go test ./internal/modules/erp/ -count=1` |
| 13 | `erp/calllog.go` | `CallLogger`: `Record(CallRecord)` into a 4,096-slot channel; goroutine flushes every 1 s or 200 rows; drops and counts when full; `Close(ctx)` flushes. Start from `erp_wiring.go` with `safe.Go` | unit test with a fake repo |
| 14 | `erp/http/respond.go`, `auth.go`, `ratelimit.go`, `health.go`, `handler.go`, `spec.go`, `spec/openapi.yaml` | envelope writer; auth middleware in this order: bearer present, parse, per-IP failure counter (`AllowKey`, 30/min), `FindKeyByPrefixAsSystem` (compute the HMAC even if the prefix is unknown, same error), compare, revoked/expired, integration active, organization `approved` (`QueryRowUnscoped` on `org.organizations`), per-key `AllowKey`, set `Principal`, `database.WithTenant`; after the handler: record the call. `GET /health`, `GET /openapi.yaml` | `go test ./internal/modules/erp/http/ -count=1` |
| 15 | `cmd/server/erp_wiring.go`, `erp_late.go`, edit `routes.go` | `mountERP(r, cfg, log, deps, db, idSvc, docsGate) *erp.Service` called in `mountModuleRoutes` right after `mountModuleRoutesAPI` (line 74), before `buildUIHandler`; inside it `lim := httpx.NewLazyLimiter(func() *redis.Client { return deps.CacheHandle().Redis() }, "dawa24:erp:ratelimit:")` is passed to `NewHandler` (pattern of `routes.go:78`); disabled mode mounts no `/erp/v1` routes and starts no erpsync worker; `uiHandler.SetERP(erpSvc)` after the UI handler exists | `go build ./... && go test ./cmd/server/ -run Router -count=1` |
| 16 | `internal/ui/handlers.go` (field `erpSvc *erp.Service`, `SetERP`), `erp_keys_handlers.go`, `erp_keys_forms.go`, `pages/erp_keys.templ`, `pages/erp_key_modals.templ`, `pages/admin_erp_keys.templ`, `erp_admin_handlers.go`, routes in the new `vendor_routes_erp.go` (`registerVendorERPRoutes(g chi.Router)`, called as the last statement of `registerVendorRoutes` in `vendor_routes.go`; that file is 384 lines and takes no other edit), `customer_routes.go`, `admin_routes_platform.go` | pages as in "Web routes". Create/rotate/limits require `actor.IsOwner`; the full key is shown once in a modal built with `components.Modal`; later loads show only `key_prefix`. Use `layouts.VendorShell`, `layouts.ShellFor` (pharmacy) and `layouts.AdminShell`. Run templ generate | `go test ./internal/ui/ -run Erp -count=1`; `make check-modal-handwritten check-hardcoded-arabic` |
| 17 | `test/integration/erp_isolation_test.go` | `TestTenantIsolation_ERPIntegrations`, `…ERPApiKeys`, `…ERPCalls` (and later tables as each slice adds them): org B transaction reads 0 rows of org A | `go test -p 1 ./test/integration/ -run TestTenantIsolation_ERP -count=1` |

**Tests this slice must add**: key format and hash; wrong secret equals unknown prefix (same error, similar timing path); revoked, expired, paused, org-not-approved each return their code; org A key asking for an org B record is indistinguishable from not found (US1.3); rotate gives both keys working until the old `expires_at`; staff list shows all keys and the owner list only theirs; every handler error is bilingual; no response or log contains a key (grep test on captured logs); `TestAsSystemOnlyInNamedFunctions`.

**Slice done**: owner creates a key on `/vendor/integrations`, Postman Health returns 200, staff revokes it, Health returns the uniform 401 authentication failure.

---

## S2: Product sync (spec P1, US2)

Depends on S1. Ships alone: the ERP can send batches, read per-item results, and the supplier can resolve review items.

| # | File (in order) | What | Done when |
|---|---|---|---|
| 1 | `internal/platform/queue/jobs.go`, `queue.go` | add `ERPSyncChunkArgs` (Kind `erp.sync_chunk`, `Queue: "erpsync", MaxAttempts: 3`), `ERPCleanupArgs` (Kind `erp.cleanup`, queue `maintenance`), and `NewInsertOnly` | `go test ./internal/platform/queue/ -count=1` (queue config test passes) |
| 2 | `internal/modules/catalog/variant_quota.go`, `postgres/variant_quota.go` | `type QuotaLimitWriter interface{ SetVariantQuotaLimits(ctx, orgID int64, limits map[int64]*int) error }`, `func (s *Service) SetVariantQuotaLimits(...)` that type-asserts the repo (pattern of `VariantWriter`, `vendor_import.go:90`), normalises with `NormalizeQuotaLimit`, one `UPDATE … FROM unnest($2::bigint[], $3::int[]) … WHERE organization_id=$1`. Doc in `docs/modules/catalog.md` | `go test ./internal/modules/catalog/... -run Quota -count=1` |
| 3 | `erp/provider/provider.go`, `registry.go`, `orgasoft/*` | `type Provider interface{ Name() string; DecodeProductBatch(body []byte) (Batch, error); DecodeAck(body []byte) ([]AckEntry, error) }`; `Batch{ClientBatchID string; Items []RawItem}`, `RawItem` is a typed struct whose number fields are `*json.Number` (not a map); `fields.go` holds the alias table (one `var productAliases = map[string][]string{…}` keyed by canonical name); PascalCase folded by lower-casing the first letter | `go test ./internal/modules/erp/provider/... -count=1` (cases: camelCase, PascalCase, number and string price, unknown fields ignored) |
| 3a | import rule | Providers self-register in `init()`, and `provider/all` blank-imports them. `registry.go` never imports a provider, because that is an import cycle. See T093. |
| 4 | `erp/item.go` + `item_test.go` | `type Item struct{ Code, ArName, Barcode, Unit, Manufacturer, Batch string; PublicPrice money.Amount; Discount, Tax *money.Amount; Stock int; Expiry *time.Time; IsQuota *bool; QuotaQty *int }`; `func NormalizeItem(raw provider.RawItem) (Item, *ItemError)`; `func (i Item) Hash(warehouseID, branchID int64) string`. Every rule in `error-codes.md` item table has a test row, including `12.345`, `-1` stock, discount 100.01, expiry `31/12/2027` | `go test ./internal/modules/erp/ -run Item -count=1` |
| 5 | `erp/repository_sync.go`, `postgres/sync.go`, `sync_items.go`, `review.go`, `variants_lookup.go` | batch insert (`CopyFrom` for items), chunk claim, result updates, links, review items, `FindVariantsBySKU(ctx, orgID, codes)`, `FindVariantsByBarcode(ctx, orgID, barcodes)` (live variants only, `deleted_at IS NULL`) | `go test -p 1 ./test/integration/ -run TestTenantIsolation_ERPSync -count=1` (add the four sync tables to the isolation test) |
| 6 | `erp/sync_intake.go` | `AcceptBatch(ctx, p Principal, body []byte, idemKey string) (Accepted, error)`: decode with the provider, normalise each item (invalid ones stored as `failed`), dedupe (last wins), enforce `p.MaxBatchItems`, check `Idempotency-Key` (same key and body hash: return the old accepted result; different hash: `ERP_IDEMPOTENCY_CONFLICT`), then in one `InTx`: insert batch, `CopyFrom` items, `EnqueueTx` one `ERPSyncChunkArgs` per chunk | unit test with fakes |
| 7 | `erp/sync_resolve.go`, `sync_write.go`, `sync_process.go` | `ProcessChunk(ctx, batchID int64, chunkNo int) error`. It loads the batch with `GetBatchByID` and builds `chunkCtx{OrganizationID, IntegrationID, WarehouseID, BranchID int64; Provider string}` from `erp.sync_batches` joined to `erp.integrations.provider`; it never re-reads the key, so a key revoked or rotated after acceptance does not stop an accepted batch. Order inside: (a) if an earlier batch of the same integration is `queued/processing` return `ErrWaitForEarlierBatch`; (b) mark batch `processing` once (`UPDATE … WHERE state='queued'`); (c) load pending items of the chunk and links by code in one query; (d) hash equal to `last_hash` means `unchanged`; (e) linked: update; (f) unlinked: tiers SKU, barcode, then `Matcher.Prepare` once for all remaining rows with `Scope{EffectiveOrg, SourceNamespace:"supplier:<org>", Workflow: productmatch.WorkflowVendor, EntryPoint:"erp_sync"}` and `ConsumerRef:"erp.sync:<batchID>:<chunkNo>"`; state `accepted`/`confirmed` with `ProductID` links, anything else goes to review; (g) one `VariantWriter.Write` for all create/update rows (price, discount, barcode, expiry, name; new variants `StatusActive`, `BranchID` from the key, `Unit` from the item when sent, and name `{ar: ArName, en: ArName}`), then `SetQuotaLimits`, then one `StockWriter.Replace` with `HasQuantity: true` and the key's `warehouse_id`; (h) write links (`last_hash`, `tax_percent`), results, memory (`Remember`); (i) `chunks_done = chunks_done + 1`; the chunk that reaches `chunk_count` sets `completed` and `finished_at` and the counters. A failure in one item writes that item's `failed` row and continues | `go test ./internal/modules/erp/ -run Sync -count=1` |
| 8 | `erp/sync_review.go` | `ListReview(ctx, orgID, page)`, `ResolveReview(ctx, orgID, id, productID, actorUserID)` (creates the variant through the same write path, links it, `Remember` with `human_confirm`, marks `resolved`, audit), `DismissReview` | unit test |
| 9 | `erp/jobs/sync_chunk_worker.go`, `cleanup_worker.go`; `erp/cleanup.go`; `postgres/cleanup.go` | worker sets `database.WithTenant(ctx, args.OrganizationID)`, calls `svc.ProcessChunk`, maps `ErrWaitForEarlierBatch` to `river.JobSnooze(time.Duration(erp.SyncSnoozeSeconds) * time.Second)`. Cleanup per `Retention*` constants, 10,000 rows per statement | `go test ./internal/modules/erp/... -count=1` |
| 10 | `internal/composition/erpports/catalog.go`, `inventory.go`, `matching.go` | adapters; no logic beyond field conversion; `NewMatcher(svc matching.Service) erp.Matcher` and `NewMatchMemory(svc matching.Service) erp.MatchMemory` take the interface value | `go build ./...` |
| 10a | `docs/adr/decisions.md` | append `ADR-ERP-1`: context (server and worker both need the same four ports), decision (`internal/composition/erpports`, imported only from `cmd/`), consequences (not covered by `TestModulesDoNotImportEachOther`; keep field conversion only) | `grep -n "ADR-ERP-1" docs/adr/decisions.md` prints a line |
| 11 | `cmd/worker/erp.go`, `main.go`, `matching.go` | `registerERPWorkers(workers, db, matchSvc, log)`; change `registerMatchingWorkers` to also return its `matching.Service` (an interface value, no pointer; `matching.Service` is declared as an interface at `internal/modules/catalog/matching/service.go:22`) and pass it in as `matchSvc matching.Service`; add a daily `river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour), …ERPCleanupArgs…)` to the returned periodic list | `go build ./cmd/worker/` |
| 12 | `erp/http/body.go`, `sync_handlers.go` | `POST /products/sync`: `http.MaxBytesReader(32 MiB)`, gzip when `Content-Encoding: gzip` with a 128 MiB inflate cap, `application/json` only; per-key limit 6/min; `GET /products/sync/{syncId}`; `GET /products/sync/{syncId}/items` (keyset by `sync_items.id`) | handler tests |
| 13 | `internal/ui/erp_review_handlers.go`, `pages/erp_review.templ`, routes (in `vendor_routes_erp.go`), rbac `vendor.integration.review` | list open review items with candidates; resolve by picking a candidate or searching with the existing `/vendor/catalog/search-json`; dismiss | `go test ./internal/ui/ -run ErpReview -count=1` |
| 14 | `test/integration/erp_sync_flow_test.go` | end-to-end on real Postgres with the worker function called directly (no River): new, known, invalid; repeat batch 100 times creates 0 duplicates (SC-004); stock 0; absent item unchanged; two batches in order; duplicate code in one batch | `go test -p 1 ./test/integration/ -run ERPSyncFlow -count=1` |
| 14a | `test/integration/erp_sync_scale_test.go` | SC-002 on real Postgres: `TestERPSyncFlow_Scale50000` accepts a batch of 50,000 valid items in under 5 s, then processes its 20 chunks one after the other with the matcher stub and finishes in under 7 minutes with every item in a terminal state. The HTTP-level 202 time and the multi-key run are in the load script (Polish) | `go test -p 1 ./test/integration/ -run TestERPSyncFlow_Scale50000 -count=1` |

**Required tests**: all rows above plus `TestSyncWithGatewayOff` (matcher stub returns `needs_review`; no Gateway object exists; all 20 items get a result) and `TestSyncNeverCallsEnhancement` (the `Matcher` port has no enhancement method, compile-time proof, plus a grep test that `erp` does not contain `StartEnhancement`). Tenant isolation for `product_links`, `sync_batches`, `sync_items`, `review_items`.

**Slice done**: Postman steps 2 to 5 of quickstart section 5.

---

## S3: Order feeds, tracking and acknowledgement (spec P1, US3)

Depends on S1 (and S2 only for `externalProductCode` in lines). Ships alone, read and ack.

| # | File | What | Done when |
|---|---|---|---|
| 1 | `erp/repository_orders.go`, `postgres/orders_sales.go`, `orders_purchase.go`, `imports.go`, `customer_code.go` | `ListSalesShipmentsAsSystem(ctx, orgID, integrationID int64, f OrderFilter) ([]SalesRow, error)`: shipments `WHERE s.organization_id=$1 AND s.id > $afterID`, joined to `commerce.orders` (header), `erp.order_imports` (state filter), the customer code (below), `ORDER BY s.id LIMIT n+1`. Lines are loaded with a second statement under `WithTenant` on `commerce.order_lines WHERE shipment_id = ANY($1)`, joined to `erp.product_links` for `externalProductCode`. `ListPurchaseOrders` runs under `WithTenant(buyer)` on `commerce.orders WHERE organization_id=$1` (headers only); the shipments and lines of those orders come from `ListPurchaseShipmentsAsSystem(ctx, buyerOrgID, orderIDs)` (plan.md E7). The customer code expression lives once, in `postgres/customer_code.go` as `customerCodeSQL`, per research D-20. Customer `name` comes from `org.organizations` of `commerce.orders.organization_id`; `branchName`, `address` and `phone` come from `org.branches` of `commerce.orders.branch_id` (the buyer's branch; `order_shipments.branch_id` is the supplier's own fulfilling branch), or from the buyer organization's own address and phone when the order has no branch; all inside the E2 statement | integration test |
| 2 | `erp/orders_service.go`, `cursor` in `erp/http/cursor.go` | `ListSales`, `GetSales`, `ListPurchase`, `GetPurchase`, `Ack(ctx, p, direction, entries)`; cursor `EncodeCursor(id)`/`DecodeCursor(s)` = base64url `v1:<id>`, bad input gives `ERP_CURSOR_INVALID` | unit tests (cursor, state mapping, ack results) |
| 3 | `erp/http/orders_handlers.go` | routes from `openapi.yaml`; sales routes need vendor capability and `erp.orders.read`/`erp.orders.ack`; mark the call `sensitive_read=true` (the call logger reads a flag set on the request context) | handler tests |
| 4 | `test/integration/erp_orders_test.go` | supplier A key sees only A's shipment and never B's lines or prices (US3.1); buyer key sees only its orders; `new` then ack then `imported`; status change gives `changed`; re-ack clears it; 5,000 orders paged by 100 with 20 inserted mid-paging: no repeat, no skip (SC-007); `TestTenantIsolation_ERPOrderImports` | `go test -p 1 ./test/integration/ -run 'ERPOrders|TestTenantIsolation_ERPOrderImports' -count=1` |

**Slice done**: quickstart steps 7 to 9.

---

## S4: Confirm and reject (spec P2, US4)

Depends on S3. Ships alone.

| # | File | What | Done when |
|---|---|---|---|
| 1 | `internal/ui/vendor_order_handlers.go` (line 250) | split `transitionVendorShipment` into `transitionVendorShipmentBy(ctx, orgID int64, isStaff bool, changedBy *int64, shipmentID int64, to commerce.OrderStatus, notes, carrier, tracking, vendorName string) error` holding the existing body, and keep `transitionVendorShipment` as a wrapper passing `&actor.UserID`. No behaviour change | `go test ./internal/ui/ -run 'Vendor.*Order|Shipment' -count=1` before and after: identical |
| 1a | `internal/ui/customer_checkout_wallet.go` | add `type walletShortfallError struct{ Available, Required money.Amount; msg string }` with `Error() string` returning exactly the current text, and return it from `prepareWalletCharge` at the shortfall branch (the branch `wallet.AvailableBalance.Minor() < goodsAmount.Minor()`). Existing callers keep working because it is an `error`. Test: `errors.As` yields the amounts | `go test ./internal/ui/ -run 'WalletShortfall|Wallet' -count=1` |
| 2 | `internal/ui/erp_order_decision.go` | `func (h *UIHandler) ErpDecideShipment(ctx context.Context, d erp.ShipmentDecision) (erp.ShipmentDecisionResult, error)`: load shipment and order (as `transitionVendorShipment` does), refuse non-pending (`ERP_ORDER_STATE_CONFLICT`, or `already_*` result), refuse `IsNegotiation && NegotiationStatus=="pending"`, map `confirmed` to `StatusConfirmed` and `rejected` to `StatusCancelled` with notes `"ERP: " + reason` (the buyer notification receives only `reason`, without the prefix, or the default text when no reason was sent), call `transitionVendorShipmentBy` with `changedBy=nil`; `errors.As(err, &walletShortfallError)` becomes `ERP_WALLET_INSUFFICIENT` with `requiredAmount` and `availableAmount` as `money.Amount.String()`, any other wallet error becomes `ERP_UNAVAILABLE` | `go test ./internal/ui/ -run ErpDecide -count=1` |
| 3 | `cmd/server/erp_late.go`, `erp_ports.go`, `deps.go`, `routes.go` | `lateERPDecider` (pattern of `lateOrderAnnouncer`) set after `buildUIHandler`: `deps.erpDecide.bind(uiHandler.ErpDecideShipment)`; `erp.Ports.Decider` is the late object | `go build ./...` |
| 4 | `erp/orders_service.go`, `http/orders_handlers.go` | `Decide(ctx, p, orderNumber, status, reason)`: reject any status other than the two (422 `ERP_STATUS_NOT_ALLOWED`; `delivered|shipped|completed|out_for_delivery|in_transit` give 403 `ERP_DELIVERY_NOT_ALLOWED`); resolve the shipment by `orderNumber` and `p.OrganizationID` through E2; audit row `erp.order.confirm`/`erp.order.reject` with `ActorKind: "api_key"` in the same flow | handler tests |
| 5 | `test/integration/erp_orders_test.go` (add) | confirm on a cash order; confirm on a wallet order charges the wallet and leaves the shipment pending when funds are short; reject refunds; repeat confirm returns `already_confirmed` and the wallet is charged once; website and API produce the same status history rows | `go test -p 1 ./test/integration/ -run ERPDecide -count=1` |

**Slice done**: quickstart steps 10 to 12.

---

## S5: Customer codes and the quick-entry modal (spec P2, US5)

Depends on S3 for the API field (the lookup exists since S3); this slice adds the UI and the guard.

| # | File | What | Done when |
|---|---|---|---|
| 1 | `db/migrations/257_user_org_erp_code_index.up.sql` + `.down.sql` | as `data-model.md` | `make migrate` |
| 2 | `erp/customer_code.go`, `postgres/customer_code.go` | `(*Service).SuppliersUsingERP(ctx, vendorOrgIDs []int64) (map[int64]bool, error)` (active integration and an active, unexpired key); `MissingCodes(ctx, buyerOrgID, buyerUserID int64, vendorOrgIDs []int64) ([]int64, error)`; `GuardCustomerCode(ctx, vendorOrgID int64, customerOrgID *int64, userID int64, code string, excludeLinkID int64) error` (returns `ERP_CODE_TAKEN` as `*erp.Error` 409; uses `pg_advisory_xact_lock(hashtextextended('erp-code:'||$vendor,0))` then checks other customers' `lower(organization_number)` among `pending/approved` links; no-op when the supplier does not use ERP) | unit + integration tests |
| 3 | `internal/ui/erp_customer_code.go` | `erpGuardCustomerCode(...)` helper; handler `CheckoutErpCodeSubmit` (`POST /checkout/erp-customer-code`: reads `vendor_org_id`, `organization_number`; guard; `orgSvc.CreateUserOrgLink(sysCtx, userID, &buyerOrg, vendorOrgID, code, org.UserOrgStatusPending)` when no link exists, else `UpdateUserOrgLink`; redirect to `/checkout`); register it in `buying_routes.go` beside `g.Post("/checkout", …)` (line 101) | `go test ./internal/ui/ -run ErpCustomerCode -count=1` |
| 4 | the four writers of the number | call `h.erpGuardCustomerCode` before `CreateUserOrgLink`/`UpdateUserOrgLink` in `customer_user_org_handlers.go` (lines 166, 201) and `vendor_user_org_handlers.go` (lines 168, 268); on `ERP_CODE_TAKEN` redirect with notice key `customer.userorg.number_taken` | tests for each of the four |
| 5 | `internal/ui/customer_checkout_core.go` (`planCheckout`, line 54) | after `prepareCheckoutItems`, collect vendor ids from `items` (`CheckoutLineItem.VendorOrgID` is filled by `resolveCheckoutVendors`; call it first), ask `MissingCodes`; if any, return `&checkoutFailure{Back: "/checkout?erp_code=1"}` with no message | test: supplier without integration: plan unchanged |
| 6 | `internal/ui/pages/customer_checkout.templ` + `checkout_erp_code_modal.templ`; `customer_checkout_handlers.go` (line 141) | keep `CustomerCheckout(...)` signature; add `CustomerCheckoutWithExtras(…same args…, extras CheckoutExtras)` and make `CustomerCheckout` call it with the zero value. `CheckoutExtras{MissingERPCodes []ErpCodeNeed}` (`VendorOrgID`, supplier name). The page handler recomputes `MissingCodes` itself (never trusts the query string) and renders `components.Modal(ModalProps{ID:"erp-code-modal", Open:true, …})` with one input per supplier, posting to `/checkout/erp-customer-code`. The cart is untouched. templ generate | `make check-modal-handwritten check-hardcoded-arabic`; manual quickstart step 6 |
| 7 | `test/integration/erp_orders_test.go` (add) | order with a code returns `erpCode`; without: `mapped=false`, `unmappedReason="no_link"`; code shared by two customers: `code_conflict`; supplier without ERP: no modal, same flow (SC-009) | pass |

---

## S6: OpenAPI, Postman and docs (spec P2, US8)

Depends on the endpoints it describes. Ships alone as the owner's test kit.

| # | File | What | Done when |
|---|---|---|---|
| 1 | `internal/modules/erp/http/spec/openapi.yaml` | final copy of `contracts/openapi.yaml`, kept in step | `TestOpenAPIMatchesRoutes` (walk the chi router, compare method+path with the spec) |
| 2 | `cmd/cli/erp_postman.go`, `erp_postman_items.go`, `main.go` (`erp-postman` dispatched in `main()` next to `match-bench`, before `run()`, because `run()` opens the database) | read the embedded spec; write `docs/erp-api/dawa24-erp.postman_collection.json` (Collection v2.1: folders `00 Health`, `01 Products`, `02 Sales orders`, `03 Decide`, `04 Purchase`, `05 Errors`; collection variables `baseUrl`, `apiKey`, `syncId`, `orderNumber`, `erpReference`, `externalReference` = `{{$guid}}`; every request has a `Bearer {{apiKey}}` header and `pm.test` scripts for status code, envelope shape, and saving ids) and `dawa24-erp.postman_environment.json` | golden test `TestPostmanCollectionIsCurrent` regenerates in memory and compares with the committed files |
| 3 | `cmd/cli/erp_demo.go`, `main.go` case `erp-demo` (in the `run()` switch: it needs the database and `cfg.Env`) | refuses when `cfg.Env == "prod"`; for the seeded `vendor@dawa24.test` organization: ensure a warehouse and branch, one variant with SKU `100245`, one pending `org.user_organizations` link from `pharmacy@dawa24.test` to the supplier with an empty number, an integration, a key with all four vendor scopes; prints the key once; `-count N` creates N keys | run twice: idempotent except for new keys |
| 4 | `docs/erp-api/README.md` | step-by-step in Arabic then English: get a key, first call, each endpoint (purpose, fields, example, errors, limits), polling pattern, retries and idempotency, rate limits, the field alias table | read-through against `quickstart.md` section 5 |
| 5 | `docs/modules/erp.md` | entities, invariants, named exceptions E1 to E8, unverified assumptions list | |
| 6 | `AGENTS.md` | add `erp` to the schema list | |

---

## S7: Purchase orders from the ERP (spec P3, US6)

Depends on S1. Carries the order-number fix. Ships alone.

| # | File | What | Done when |
|---|---|---|---|
| 1 | `db/migrations/258_order_number_sequence.up.sql` + `.down.sql` | as `data-model.md` | `make migrate` |
| 2 | `internal/modules/commerce/order_number.go`, `postgres/order_number.go`, edit `service.go` (lines 154-156) | `type OrderNumberAllocator interface{ NextOrderNumber(ctx context.Context) (string, error) }`; `func (s *Service) nextOrderNumber(ctx context.Context, now time.Time) (string, error)` in `order_number.go` (allocator when `s.repo` implements it, else `GenerateOrderNumber(now, orderCounter.Add(1))`); in `checkout` replace the two lines `seq := orderCounter.Add(1)` and `orderNumber := GenerateOrderNumber(now, seq)` with `orderNumber, err := s.nextOrderNumber(ctx, now)` and a 3-line `if err != nil { return nil, err }` (net +2 lines, because `service.go` is 397 lines and the limit is 400). `SELECT nextval('commerce.order_number_seq')::text`. Note in `docs/modules/commerce.md` | `go test ./internal/modules/commerce/... -count=1`; integration test: 200 concurrent checkouts across two `Service` instances produce 200 distinct numbers |
| 3 | `erp/postgres/offers.go`, `idempotency.go` | offers SQL `ListOffersAsSystem` and `VariantOfferAsSystem` (plan.md E6; read-only on `catalog.product_variants` with `status='active'`, `deleted_at IS NULL`, org type vendor and approved, stock > 0, barcode match, price ascending, `LIMIT 50`, public fields only); idempotency insert/get/finish/delete; `SupplierLinked` and `FindOrderByNoteMarker` | integration test |
| 4 | `erp/purchase_service.go` | `Offers(ctx, p, barcode)`, `CreatePurchaseOrder(ctx, p, req)`: require `buying_user_id` and active membership; validate lines (`ERP_PO_QUANTITY_INVALID`, `ERP_PO_ITEM_NOT_FOUND`, `ERP_PO_NO_ACTIVE_OFFER`, `ERP_PO_SELF_SUPPLY`, `ERP_PO_SUPPLIER_NOT_LINKED`); idempotency (insert `in_progress`; replay returns the stored response with HTTP 200; stale `in_progress` is repaired through the order note marker `[ERP:<provider>:<externalReference>]`); `Placer.Place`; store response; audit `erp.po.create` | unit tests with fakes |
| 5 | `cmd/server/erp_ports.go`, `erp_wiring.go` | `erpPurchasePlacer`: build a dedicated `commerce.Service` exactly like `routes.go:217-236` (wallet refunder and payer, docs gate, `OnOrderPlaced(deps.orders.announce)`, availability probe, `apiQuoteResolver` with `apiBuyerChoices`); `Place` resolves `variantId` (UUID) to variant id, quotes with `apiQuoteResolver.quote`, compares any `unitPrice`, calls `CheckoutAPI`; maps `apperr` codes to `ERP_PO_REFUSED` with `details.reason`, wallet to `ERP_PO_PAYMENT_NOT_SUPPORTED`, price to `ERP_PO_PRICE_MISMATCH` with the quote's amounts | `go build ./...` |
| 6 | `erp/http/purchase_handlers.go` | `GET /offers`, `POST /orders/purchase` (201 new, 200 replay) | handler tests |
| 7 | `test/integration/erp_po_test.go` | create then repeat 100 times: one order (SC-004); wrong price refused with correct amount and no order; no active offer; supplier not linked and no account created (FR-031); wallet refused; crash repair; `TestTenantIsolation_ERPIdempotency` | pass |

---

## S8: Reveal (spec P3, US7)

Needs only the i18n files and rbac test file of S1 (tasks T026 to T029); ships alone after them.

| # | File | What | Done when |
|---|---|---|---|
| 1 | `internal/shared/mask/mask.go` + test | `Phone(s)`, `Email(s)`, `Number(s)` (keep last 3 characters; short values fully masked), pure functions | `go test ./internal/shared/mask/ -count=1` |
| 2 | rbac: `platform.reveal.sensitive` (admin), `vendor.reveal.customer_contact` (vendor) | permission catalog entries | rbac tests |
| 3 | `internal/ui/reveal_handlers.go` | `POST /reveal` (form fields `kind`, `id`; CSRF protected) registered in `RegisterApprovedSharedRoutes` (`internal/ui/handlers.go:287`). A `map[string]revealKind` allowlist: `org.tax_number`, `org.commercial_register`, `org.pharmacist_license`, `org.phone`, `org.email` (admin), `branch.phone` (supplier, only for a branch on an order shipped by the caller's organization), `gateway.admin_username` (admin). Anything else, including gateway passwords, virtual keys, AI keys, ERP keys: `REVEAL_NOT_REVEALABLE`. Permission failure: `REVEAL_FORBIDDEN`. Every outcome writes an audit row `reveal.grant` or `reveal.deny` (entity kind and id, no value) in its own transaction. Response: an HTML fragment with the value (htmx swap) | handler tests for grant, forbid, not found, and audit rows |
| 4 | `internal/ui/components/reveal_field.templ` | `RevealField(kind, id, maskedValue, canReveal)`; the masked value only is rendered; a button posts to `/reveal` with htmx and swaps the span | templ generate |
| 5 | use it on `admin_org_detail.templ` (lines ~196-215: CR, tax, license; plus phone and email), `vendor_order_detail.templ` (lines 105-108, buyer branch phone; remove the `tel:` link until revealed), `admin_developers_ai.templ` (stored gateway username: keep the `admin_username` input empty with a placeholder and render `RevealField(kind=gateway.admin_username)` beside it as a read-only display of the stored value; the submit still posts only the typed value; nothing is shown when no value is stored) | the page source never contains the clear value (test with `strings.Contains` on rendered HTML) | `go test ./internal/ui/ -run Reveal -count=1` |

**Required**: SC-010 test (every reveal produces an audit row; clear value absent from page source).

---

## Order of slices and what blocks what

S1 first. S2, S3, S7, S8 depend only on S1 (S8 only on tasks T026 to T029 of S1). S4 on S3. S5 on S3. S6 last of the P2 group but can start once S1 to S3 exist and is re-run when S4, S7 land. Recommended order: S1, S2, S3, S4, S5, S6, S7, S8.

## Complexity Tracking

No unjustified violations. The named exceptions table above is the only deviation from the default tenant rule; the shared `internal/composition/erpports` package is the only addition to the composition root (decision D-28 in research.md).

## Extension hooks (`.specify/extensions.yml`, `after_plan`)

Optional, not run: `speckit.security-review.plan` ("Would you like to perform a security review on the proposed technical plan?"). Also registered for later: `after_tasks` `speckit.security-review.tasks`, `after_implement` `speckit.security-review.branch`.

**Deployment:** Deploy first with the ERP API off. Set `ERP_KEY_PEPPER` and `ERP_API_ENABLED=true` when ready. Any test or Postman flow needing the API enables it explicitly. Disabled mode leaves the website, n8n and existing routes unchanged.

Ownership signals for key creation, rotation and rate-limit changes come only from the signed-in `authctx.Actor.IsOwner`, never submitted input. The service rejects a false signal without mutation or audit and scopes key access to the caller organization.

**Authentication failure limiting:** use the trusted client IP helper. Before authentication read the ERP-specific failed-auth count without incrementing; a count at or above the existing limit blocks with 429 and Retry-After without extending the window. Increment only after a failed authentication; success neither increments nor resets. Keep this counter separate from ERP per-key and website route counters. Redis errors are logged and fail open; per-key limiting counts authenticated requests.

Middleware receives an authentication dependency; production uses the real Service.Authenticate. A real-router test exercises the real service and failure limiter, while layer tests count authenticate calls without exposing the private HMAC seam.

`GET /erp/v1/health` requires normal key authentication; the platform `/health` route is unchanged. `GET /erp/v1/openapi.yaml` remains public, as specified by its OpenAPI `security: []`. Define the handler shell before the auth middleware and register routes only in T065.
