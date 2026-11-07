# Research: ERP Integration API

Every open technical choice, decided. "Unverified" marks a value the owner or Orgasoft must confirm; each is isolated behind the named constant or function so changing it is one edit.

## D-01 Module shape
- **Decision**: new module `internal/modules/erp` (Postgres schema `erp`). It writes only its own tables, with one named exception: the retention delete of `catalog.match_runs` rows whose `consumer_ref` starts with `erp.sync:` (E3). Anything that changes another module's data goes through a port interface declared in `erp`, implemented by an adapter in `cmd/server` or `cmd/worker`. Reads of other modules' tables for order feeds, variant lookup and customer codes are read-only SQL inside `erp/postgres`.
- **Rationale**: AGENTS.md rule 5 forbids module imports, not SQL reads. `internal/modules/assistant/postgres/read_stage3_vendor.go` already reads `commerce.order_lines` this way. A SQL filter on "not yet imported" needs the join; a port would force fetch-all-then-filter and break paging.
- **Alternatives**: put tracking columns on `commerce.order_shipments` (rejected: one flag cannot serve two integrations); one port per read (rejected: 12 adapters, no isolation benefit).

## D-02 API prefix and mounting
- **Decision**: `/erp/v1`, mounted on the root chi router in `cmd/server/erp_wiring.go`, called from `mountModuleRoutes` (`cmd/server/routes.go`, next to the Telegram bridge mount). No session, CSRF or tenant middleware. Do not add the prefix to `httpx` `longRunningPrefixes`.
- **Rationale**: `grep '"/erp'` finds no existing route. `/api/v1/*` is the module API (`MODULE_API_ENABLED`) and n8n bridges; spec FR-006 forbids reuse.
- **Alternatives**: `/api/erp/v1` (rejected: `httpx.Error` treats `/api/` as JSON-only and the legacy gates key on that prefix).

## D-03 Key format and hash
- **Decision**: `d24_<tag>_<prefix>_<secret>`. `tag` is `live` when `APP_ENV=prod`, else `test`. `prefix` is 8 chars from `[a-z0-9]`. `secret` is 32 random bytes, base64url without padding (43 chars). Stored: `key_prefix` (unique) and `key_hash = hex(HMAC-SHA256(ERP_KEY_PEPPER, secret))`. Compare with `hmac.Equal`.
- **Rationale**: the secret has 256 bits of entropy, so a fast keyed hash is safe and costs no CPU per request (argon2 would let an attacker burn server CPU). The prefix is the lookup key and is safe to show.
- **Alternatives**: bcrypt/argon2 (rejected: DoS cost); JWT (rejected: cannot revoke instantly).

## D-04 Machine principal
- **Decision**: `erp.Principal` in the request context under a private key. The server runs `database.WithTenant(ctx, principal.OrganizationID)`. No `authctx.Actor` is set. The one cross-tenant lookup is `FindKeyByPrefixAsSystem`.
- **Rationale**: an `Actor` would let any code that checks `actor.OrganizationID` treat the machine as a user and pass RBAC checks. The org comes only from the key row (FR-004).
- **Alternatives**: a `SECURITY DEFINER` lookup function (rejected: FORCE RLS applies to the owner too, so it does not bypass).

## D-05 Scopes and capability
- **Decision**: scopes `erp.products.sync`, `erp.orders.read`, `erp.orders.ack`, `erp.orders.decide`, `erp.po.create`. Sales endpoints and `erp.products.sync` also require organization type vendor (`rbac.NormalizeOrgType(type) == rbac.OrgTypeVendor`). Purchase endpoints work for vendor and customer organizations.
- **Rationale**: a supplier also buys (`authctx.Actor.IsBuyer`), a pharmacy never sells.

## D-06 Error envelope
- **Decision**: own envelope, `{"status":"error","requestId","error":{"code","message":{"ar","en"},"details"}}`. Success is `{"status":"ok","requestId","data"}`. Written by `erp/http/respond.go`, not `httpx.Error`.
- **Rationale**: `httpx.ErrorBody` carries one language and has no `status` field; spec FR-033 wants bilingual and a clear status. Request id comes from `observability.RequestIDFrom(ctx)`; `httpx.RequestID` already sets the `X-Request-ID` header.

## D-07 Rate limits
- **Decision**: per-key fixed window in Redis, default 600 requests/minute, adjustable 10 to 6000 per key (`api_keys.rate_limit_per_minute`); the organization owner changes it after creation with `erp.Service.UpdateKeyLimits` (FR-005), and the next call reads the new value because the key row is read on every call. `POST /products/sync` has its own 6/minute/key. Failed authentication: 30/minute/IP. Headers `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After` on 429. Redis down: fail open (same as `httpx.Limiter.allow`).
- **Rationale**: matches `httpx.Limiter`. Needs one new exported method `(*Limiter).AllowKey` in `internal/platform/httpx/ratelimit.go`, because `allow` is private and `LimitUserOrIP` takes fixed limits.

## D-08 Sync intake
- **Decision**: `POST /erp/v1/products/sync`, max 50,000 items, body max 32 MiB, gzip request bodies accepted (decompressed cap 128 MiB). The handler validates JSON shape and each item, writes all rows to `erp.sync_items` in one `CopyFrom`, writes one `erp.sync_batches` row, enqueues `ceil(n/2500)` `erp.sync_chunk` jobs with `queue.EnqueueTx` in the same transaction, and returns 202 with `syncId`. Items that fail validation are stored with state `failed` at intake. Duplicate codes in one batch: the last wins; earlier rows get state `unchanged`, code `ERP_ITEM_SUPERSEDED`.
- **Rationale**: SC-002 (50,000 items in 5 s) rules out matching in the request. Storing items lets a restart resume and gives per-item results.
- **Alternatives**: keep items only in the job args (rejected: River args are JSON in `river_job`; 25 MB rows hurt the queue table).

## D-09 Per-tenant ordering without unique jobs
- **Decision**: no `river.UniqueOpts` (none exist in the repo; do not introduce the first one). A chunk job first runs `SELECT 1 FROM erp.sync_batches WHERE integration_id=$1 AND id < $2 AND state IN ('queued','processing') LIMIT 1`; if a row exists it returns `river.JobSnooze(time.Duration(erp.SyncSnoozeSeconds) * time.Second)` (`SyncSnoozeSeconds` is 15). Chunks of one batch run in parallel (their codes are distinct after intake dedup). The last chunk to finish marks the batch `completed`: `UPDATE ... SET chunks_done = chunks_done + 1 ... RETURNING` and the chunk that reaches `chunk_count` sets the state.
- **Rationale**: spec US2.9 asks for in-order, no corruption. Idempotency comes from `erp.product_links` unique keys, not from the queue.

## D-10 Queue, timeouts, reaper
- **Decision**: queue `erpsync`, workers `WORKER_ERPSYNC` default 3, added to `cfg.Worker.Queues` in `internal/platform/config/config.go`. Chunk size 2,500 items, MaxAttempts 3. Each chunk must finish in under 10 minutes.
- **Rationale**: `queue.Client.StartStalledJobReaper(ctx, 3m, 15m)` cancels any job `running` over 15 minutes, while `DefaultJobTimeout` is 30 minutes. A job between 15 and 30 minutes would be cancelled while still working. Small chunks keep every job far below 15. `TestEveryInsertedQueueIsConfigured` (`internal/platform/queue/queue_config_test.go`) fails the build if `erpsync` is inserted but not configured.
- **Capacity**: 3 workers, about 2,000 items/s per worker on the update path. 50 tenants each sending 50,000 changed items in the same minute will not finish in 7 minutes. Typical batches carry changed items only. Raise `WORKER_ERPSYNC` if the load check (quickstart section 9) shows a backlog.

## D-11 Product identity and the trust order
- **Decision**: identity is `(integration_id, external_code)` in `erp.product_links`, one code to one variant and one variant to one code (owner decision 2). Resolution order for an unlinked code: (1) the supplier's live variant whose `sku` equals the code and is unique; (2) the supplier's live variant whose non-empty `barcode` equals the item barcode and is unique; (3) `Matcher.Prepare` with the supplier scope (this consults `catalog.match_knowledge` first, then the engine); (4) otherwise a review item. A soft-deleted linked variant goes to review with `ERP_ITEM_DELETED_LINK`; nothing is restored silently.
- **Memory write**: after a link is created from tier 3 with decision state `accepted` or `confirmed`, or from a human review, write `matching.Service.Remember` with `Kind: "supplier_code"` (human review: `"human_confirm"`), `SourceNamespace: "supplier:<orgID>"`, `QuestionSignature = productmatch.QuestionSignature(productmatch.ExtractRowFacts(&row, nil), row.Name, productmatch.NormalizerVersion)`. Tiers 1 and 2 write no memory.
- **Rationale**: `Scope{SourceNamespace: "supplier:<org>", Workflow: WorkflowVendor}` is what `internal/modules/ingest/catalog_stage_run.go:94` uses, so the ERP and file imports share one memory. `KnowledgeKindRank` ranks `supplier_code` at 35 for an org.

## D-12 Field names, numbers, units
- **Decision**: canonical camelCase. Accepted aliases (case-insensitive after lower-casing the first letter): see the alias table in `internal/modules/erp/provider/orgasoft/fields.go` (listed in openapi `ProductItem` description). JSON is decoded with `UseNumber()`. Money fields go through `money.Parse(string)`; more than 2 decimals is `ERP_ITEM_PRICE_PRECISION`. Mapping: `price` to `product_variants.price` (public price); `discount` (percent 0 to 100, 2 decimals) to `product_variants.discount`; `tax` (percent) to `erp.product_links.tax_percent` only, never applied to totals; `stock` (integer >= 0) to `inventory.stocks.quantity` in the key's warehouse; `expiryDate` to `expiry_date` (date part only); `isQuota=true` with `quotaQuantity` > 0 to `quota_limit`, `isQuota=false` clears it to NULL, absent leaves it.
- **Source**: `contracts/orgasoft-samples.md` holds the real samples. Canonical product fields are `externalProductCode`, `arName`, `stock`, `publicPrice`, `discount`, `tax`, `isQuota`, `quotaQuantity`, `barcode`, `expiryDate`. PascalCase is accepted. `fields.go` is the only place to add other spellings.
- **Rationale**: the DB discount column is a percent (`docs/modules/catalog.md` line 360 recomputes it from public and sale prices).

## D-13 "Unchanged" detection
- **Decision**: `erp.product_links.last_hash` is `sha256` over the normalised item plus the key's `warehouse_id` and `branch_id`. Equal hash means nothing is written and the result is `unchanged`.
- **Rationale**: the ERP is authoritative for stock it sends. Re-writing an identical stale number would override a decrement made by an order that the ERP has not yet imported.

## D-14 Matching and the AI rule
- **Decision**: ERP sync calls `Matcher.Prepare` only, never `StartEnhancement`. `Prepare` is deterministic (knowledge plus engine); it never calls the Gateway. A decision other than `accepted` or `confirmed` creates or updates one open `erp.review_items` row and the item result is `needs_review`. `Prepare` is called once per chunk and only when the chunk has unlinked rows.
- **Rationale**: AGENTS.md rule 3. A supplier can always resolve a review item by hand, so the Gateway being off never blocks a sync. `catalog.match_runs` rows created by `Prepare` are purged after 30 days by `erp/postgres/cleanup.go` (named cross-schema delete `DELETE FROM catalog.match_runs WHERE consumer_ref LIKE 'erp.sync:%' AND created_at < $cut`, because `matching.Repository` has no purge for them and adding one would force mock changes in six test files). Every foreign key to `catalog.match_runs` is `ON DELETE CASCADE` (migrations 240, 242) or `ON DELETE SET NULL` (migrations 234 to 238), so no child delete is needed.

## D-15 Missing items and retirement
- **Decision**: an item absent from a batch is untouched. `ingest` retirement (`RetireVariantsExcept`, `catalog_commit_retirement.go`) is never called. Stock 0 writes quantity 0 and nothing else changes; the product reads as out of stock.
- **Rationale**: owner decision 5.

## D-16 Sales orders and the cursor
- **Decision**: a sales order is one `commerce.order_shipments` row of the key's organization (one shipment per supplier per order: `commerce/postgres/orders.go` keys `shipmentIDMap` by vendor). Its public id is the Dawa24 `orderNumber`. Order of results: shipment `id` ascending. Cursor: `base64url("v1:" + lastShipmentID)`. Page size default 100, max 500. A poller starts every cycle without a cursor and filters `state=new`; the cursor only pages inside one cycle.
- **States**: `new` (no `erp.order_imports` row), `imported` (row exists and shipment status equals `acked_status`), `changed` (row exists and status differs), `all`.
- **Rationale**: id order never reorders when rows update, so no order is skipped or repeated while paging. A late-committing lower id is picked up on the next cycle because it is still `new`.

## D-17 Purchase orders (read)
- **Decision**: `commerce.orders` rows where `organization_id` equals the key's organization. Same cursor over `orders.id`. Lines come grouped by supplier shipment. The order headers are read under `WithTenant(buyer)`; their shipments and lines carry the supplier's organization id, so they are read through the named exception E7 (`ListPurchaseShipmentsAsSystem`). Import tracking is by order id with `direction='purchase'`.

## D-18 Acknowledgement
- **Decision**: `POST /orders/{sales|purchase}/ack` takes up to 500 `{orderNumber, erpReference}`. First ack creates the row: `result=acknowledged`. Same reference again: `result=already_acknowledged`, and `acked_status`, `acked_at` and `ack_count` are refreshed (this clears `changed`). A different reference for an order already acked: `result=conflict`, `code=ERP_ACK_REFERENCE_CONFLICT`, `existingReference` set, nothing changed. Unknown order: `result=not_found`. The Orgasoft sample-3 shape (`order_number`, `orga_orderID`, `items`) is accepted as an alias in `provider/orgasoft` and `items` is ignored.
- **Unverified**: that sample 3 is the acknowledgement (spec Assumptions).

## D-19 Confirm and reject
- **Decision**: one endpoint `POST /orders/sales/{orderNumber}/status` with `{"status":"confirmed"|"rejected","reason"?}`. Confirm moves the shipment `pending` to `confirmed`; reject moves `pending` to `cancelled`. The status-history note is `ERP: <reason>`; the buyer notification receives only `<reason>` (without the prefix) when a reason was sent, and the default text when not. Any other value: 422 `ERP_STATUS_NOT_ALLOWED`; `delivered|shipped|completed|out_for_delivery|in_transit`: 403 `ERP_DELIVERY_NOT_ALLOWED`. Orders with `is_negotiation` and `negotiation_status='pending'`: 409 `ERP_ORDER_NEGOTIATION_PENDING` (the website uses a separate flow for those). A repeat returns 200 with `result: "already_confirmed"` or `"already_rejected"`.
- **Implementation**: reuse the website path. `transitionVendorShipment` in `internal/ui/vendor_order_handlers.go:250` carries the wallet pre-check (`prepareWalletShipmentCharge`) and the buyer notification. It is split into a core that takes `changedBy *int64` so the machine passes nil. The adapter in `cmd/server` calls the exported UI method; the `erp` module sees only a port.
- **Rationale**: the website rules are the owner's decision; copying them would drift.

## D-20 Customer code
- **Decision**: the code is `org.user_organizations.organization_number` for the link where `vendor_org_id` is the supplier and the link belongs to the buyer: `customer_org_id = order.organization_id`, or `customer_org_id IS NULL AND user_id = order.customer_id`. Links with status `pending` or `approved` count; `rejected` does not. Several matching links: prefer the one with `customer_org_id` set, then the newest `updated_at`. Uniqueness per supplier is enforced only for suppliers that have an active integration, by `erpSvc.GuardCustomerCode` called in the four UI handlers that write the number, and by an advisory lock on the supplier. Existing duplicates stay; an order whose buyer code is shared by two customers returns `customer.mapped=false`, `customer.unmappedReason="code_conflict"`.
- **Rationale**: `org.user_organizations` has no RLS and no organization column to key a unique index on; uniqueness only for ERP suppliers keeps other suppliers' flow unchanged (SC-009).
- **Alternative**: new `erp.customer_codes` table (rejected: owner said the link page field is the code; two sources would drift).

## D-21 Purchase order creation
- **Decision**: `POST /erp/v1/orders/purchase` with `externalReference` (required, 1 to 100 chars), `lines[{variantId, quantity, unitPrice?}]`, optional `notes`. `variantId` is the Dawa24 `public_id` UUID of a supplier product, found with `GET /erp/v1/offers?barcode=`. Payment is the platform default (`commerce.DefaultPaymentMethod`); wallet is refused by `CheckoutAPI`. Each line's supplier must have a `pending` or `approved` `org.user_organizations` link for the buyer, otherwise `ERP_PO_SUPPLIER_NOT_LINKED`. The adapter quotes with `apiQuoteResolver.quote` (`cmd/server/api_checkout_quote.go:28`), compares any `unitPrice` the ERP sent with the quote and returns `ERP_PO_PRICE_MISMATCH` with the correct amounts, then calls `commerce.Service.CheckoutAPI` with the quoted input.
- **Idempotency**: `erp.idempotency_records` unique `(integration_id, scope, external_reference)`. The order note starts with `[ERP:<provider>:<externalReference>]` so a crash between order creation and the record update is repaired by a lookup on retry.
- **Buying user**: `api_keys.buying_user_id`, validated at key creation as an active `org.members` row of the organization; a user who is not an active member gives `ERP_VALIDATION` with `details.fields=["buyingUserId"]`. `ERP_PO_BUYING_USER_MISSING` is used only when a purchase order is created and the key has no buying user or that user is no longer an active member.
- **Order numbers**: `commerce.Service.checkout` takes numbers from an in-process counter seeded with `time.Now().Unix() % 10000` (`commerce/service.go:14-17,155`), and `orders_order_number_key` is unique. Two replicas, or a restart, collide. Migration 258 adds `commerce.order_number_seq` and an optional `OrderNumberAllocator` interface on the repository (type-asserted, like `shipmentWalletUpdater`). The PO slice ships this fix because an ERP retrying creates orders at machine speed.
- **Unverified**: that Orgasoft wants `variantId` as the item identity. If it prefers a barcode, `provider.Decode...` resolves barcode to variant through the offers lookup; only that function changes.

## D-22 Audit and call log
- **Decision**: `platform.audit_log` gains `actor_kind` (`user`/`api_key`/`system`, default `user`) and `actor_key_id`. Key create, rotate, revoke, pause, order decisions, PO creation, review resolution and every reveal write an audit row inside the same transaction as the change (`database.WriteAudit`). Every API call writes one `erp.api_calls` row through a buffered writer (flush every 1 s or 200 rows). Order reads set `sensitive_read=true`. No row ever contains a key value or `Authorization` header.
- **Rationale**: `authctx.ActivityAudit` returns early when `actor.UserID <= 0`, so machine calls are never recorded by it.

## D-23 Reveal
- **Decision**: masked by default on server render; `POST /reveal` returns one value. Allowlisted kinds: `org.tax_number`, `org.commercial_register`, `org.pharmacist_license`, `org.phone`, `org.email`, `branch.phone`, `gateway.admin_username`. Only the gateway username can be revealed (owner decision, 2026-11-07, orgasoft-decisions item 15). Gateway passwords, virtual keys, AI provider keys and ERP keys are never rendered and never revealable (`REVEAL_NOT_REVEALABLE`). Permissions: `platform.reveal.sensitive` (staff), `vendor.reveal.customer_contact` (supplier sees buyer branch phone). Every grant and every refusal writes an audit row.
- **Rationale**: the gateway password is already never rendered (`admin_developers_ai.templ`), so only the username needs a reveal.

## D-24 Docs and Postman
- **Decision**: `contracts/openapi.yaml` is the source; a copy is embedded from `internal/modules/erp/http/spec/openapi.yaml` and served at `GET /erp/v1/openapi.yaml` (no auth). A test compares the spec's paths to the mounted chi routes. `cmd/cli` gets `erp-postman`, which writes `docs/erp-api/dawa24-erp.postman_collection.json` and `dawa24-erp.postman_environment.json` from the spec's examples. A test regenerates and compares with the committed files.

## D-25 Config keys
`ERP_API_ENABLED` (bool, default true), `ERP_KEY_PEPPER` (at least 32 bytes; required when enabled and `APP_ENV=prod`; in other environments a fixed development value with a warning), `ERP_DEFAULT_RATE_PER_MINUTE` (600), `ERP_MAX_BATCH_ITEMS` (50000), `WORKER_ERPSYNC` (3). When disabled, `/erp/v1/*` returns 503 `ERP_DISABLED` (same pattern as `mountTelegram` in `cmd/server/telegram_wiring.go:33`).

## D-26 Provider seam
- **Decision**: `provider.Provider` (decode product batch and decode ack; encoding of sales and purchase orders is the shared camelCase output, not provider specific) with a registry. Orgasoft is `provider/orgasoft`. A new ERP is a new package, one registry line, and no migration (the `provider` column has a format CHECK only).

## D-27 Retention
`erp.api_calls` 30 days; `erp.sync_items` 7 days (payload cleared when processed); `erp.sync_batches` 90 days; resolved or dismissed `erp.review_items` 180 days; `erp.idempotency_records` and `erp.order_imports` kept; ERP `catalog.match_runs` 30 days. Job `erp.cleanup` daily on `maintenance`, deleting 10,000 rows per statement until none remain.

## D-28 Shared adapters
- **Decision**: adapters that both `cmd/server` and `cmd/worker` need (catalog writer, stock writer, matcher, match memory) live in `internal/composition/erpports`. It imports modules; only `cmd/` imports it. Adapters used by one binary stay in that binary (`cmd/server/erp_ports.go`).
- **Rationale**: AGENTS.md names `cmd/server` as the composition root; the worker is a second root, and the review screen (server) and the sync (worker) write through the same four ports. One copy cannot drift. `depguard` only restricts `shared/` and `platform/`, and `test/module_boundaries_test.go` only walks `internal/modules`.
- **Alternatives**: duplicate in both `cmd/` packages (rejected: drift); put them in `internal/ui` (rejected: the worker would import the UI).
