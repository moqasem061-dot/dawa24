# Quickstart: validate the ERP Integration API

Two readers: the implementer (sections 1 to 3 and 7 to 9) and the owner acting as Orgasoft (sections 4 to 6, Postman only). Windows shell is Git Bash; every command runs from `F:\dawa24`.

## 1. Build and migrate (implementer)
```
make up                      # postgres, redis, minio (docker compose)
make migrate                 # applies 255 to 258 and the River tables
go build ./... && go vet ./...
```
Done when `make migrate-status` shows 0 pending and `psql` lists schema `erp` with nine tables.

## 2. Start the services

The ERP API defaults to off. For this test flow, set `ERP_API_ENABLED=true` in the environment of the server and worker; configure `ERP_KEY_PEPPER` (at least 32 bytes in production). Deploy first with the API off, then set `ERP_KEY_PEPPER` and `ERP_API_ENABLED=true` when ready.
```
SEED_PASSWORD='choose-12-chars+' go run ./cmd/cli seed-users     # vendor@, pharmacy@, admin@ dawa24.test
make run          # terminal 1: web on :8080
make worker       # terminal 2: background jobs (queue erpsync must appear in its startup log)
```
There is no demo-data command: the key comes from the website (section 3), and the checkout screen creates the pharmacy's customer link. The first product send matches each item against the catalog; an item the catalog does not hold yet (on a fresh database, all of them) goes to the review screen `/vendor/integrations/review`, where the supplier searches for the product by name or barcode and links it. After that, every send of the same code updates it directly.

## 3. Create a key in the website (owner path, no command)
1. Sign in as the supplier owner (`vendor@dawa24.test`). If the supplier has no warehouse or branch yet, create one of each in the website first (`/vendor/warehouses`, `/vendor/branches`). Open `/vendor/integrations`.
2. Press "New key": provider Orgasoft, a name, then pick the warehouse and branch from the lists, tick the scopes `erp.products.sync`, `erp.orders.read`, `erp.orders.ack`, `erp.orders.decide`. Save.
3. Copy the key shown once (the Copy button, or select the field). Reload the page: only the first 8 characters (the prefix) remain.
3b. A pharmacy key (`pharmacy@dawa24.test`, `/customer/integrations`) asks only for the branch and the buying member (both lists); it has no warehouse.
3a. Press "Change limit" on the key row, enter a number from 10 to 6000, save: the new limit shows in the table and in Health (`rateLimitPerMinute`) on the next call.
4. Sign in as `admin@dawa24.test`, open `/admin/integrations`: the key is listed with its organization and a Revoke button.

## 4. Import Postman (owner)
1. Postman, Import, choose `docs/erp-api/dawa24-erp.postman_collection.json` and `docs/erp-api/dawa24-erp.postman_environment.json`.
2. Pick the environment "Dawa24 ERP - test server". Set `baseUrl` to `http://localhost:8080` and `apiKey` to the key from section 3.
3. Open the collection, press Run, keep the order of folders: 01 Start here, 02 Products, 03 Sales orders, 04 Purchase orders. The collection's requests are numbered inside each folder.

## 5. The journey as Orgasoft (owner, about 20 minutes)
Request names are the ones shown in the collection. Steps 9, 11 and 12 use the same request as an earlier step with a different query or body.
| Step | Request | You should see |
|---|---|---|
| 1 | 01 Start here / 1. Check my API key | 200, `organization`, `scopes`, `warehouse` |
| 2 | 02 Products / 1. Send products (2 items, SKUs `100245` and `100311`) | 202, `syncId` saved to `{{syncId}}` |
| 3 | 02 Products / 2. Check the upload (repeat until `completed`, under a minute with the worker running) | `completed`; on a fresh catalog both are `needsReview` |
| 4 | 02 Products / 3. See each product's result | each item's state; `ERP_ITEM_MATCH_REVIEW` with Arabic and English text for the review ones |
| 4a | Website, `/vendor/integrations/review` | each item shows its name, code, barcode and price; search the product, press link |
| 5 | 02 Products / 1. Send products, same batch again | the linked item is `updated`; nothing is duplicated in `/vendor/products` or in the review list |
| 6 | In the browser as `pharmacy@dawa24.test`: add the supplier's product to the cart, open `/checkout` | a modal asks for the customer code; enter `C-00871`; the cart is still there; place the order |
| 7 | 03 Sales orders / 1. Get new orders | the order, `customer.erpCode` = `C-00871`, `importState` = `new` |
| 8 | 03 Sales orders / 3. Mark orders as imported (`{{orderNumber}}`, `ORGA-1`) | `acknowledged` |
| 9 | 03 Sales orders / 1. Get new orders, again | the order is gone from `state=new`; with `state=imported` it shows |
| 10 | 03 Sales orders / 4. Confirm or reject an order (`status` `confirmed`) | `confirmed`; the website order page shows confirmed |
| 11 | 03 Sales orders / 4. Confirm or reject an order, again | 200 `already_confirmed` |
| 12 | 03 Sales orders / 4. Confirm or reject an order (`status` `delivered`) | 403 `ERP_DELIVERY_NOT_ALLOWED` |
| 13 | 04 Purchase orders / 4. Find supplier offers by barcode, then 5. Create a purchase order | Coming soon: 201 with an order number; send it again: 200, same number |

## 6. Failure examples (owner)
The collection has no errors folder; send these by hand (Postman or curl) and compare `error.code` with `contracts/error-codes.md`: no key (401 `ERP_AUTH_MISSING`), wrong key (`ERP_AUTH_INVALID`), a scope the key lacks (`ERP_SCOPE_MISSING`), a batch of 50,001 items (`ERP_BATCH_TOO_LARGE`), bad cursor (`ERP_CURSOR_INVALID`), wrong price on a purchase order (`ERP_PO_PRICE_MISMATCH`). Revoke the key in `/admin/integrations` and run Health again: 401 `ERP_KEY_REVOKED`.

Pass criteria: every Postman test is green, and SC-001 holds (key to acknowledged order in under 30 minutes).

## 7. Automated checks (implementer)
```
go test ./internal/modules/erp/... -count=1                      # unit + http
go test ./internal/platform/... ./internal/shared/... -count=1   # limiter, audit, i18n, queue config
go test ./test/ -run 'TestModulesDoNotImportEachOther|TestEveryRouteGateNamesADeclaredPermission|TestUIRoutesAreAudienceGated' -count=1
go test -p 1 ./test/integration/... -run 'ERP' -count=1          # real Postgres: isolation, cursors, idempotency
make check                                                       # every gate CI runs
```
Done when all pass and `make check-file-size` and `make check-hardcoded-arabic` print ok.

## 8. AI off (implementer)
```
go test ./internal/modules/erp/... -run 'TestSyncWithGatewayOff' -count=1
```
The test builds the sync service with no Gateway client and a matcher stub that returns `needs_review`, then asserts a batch of 20 items completes, each item has a state, and linked items update normally. For the manual check, start the worker without `GATEWAY_*` variables and repeat steps 2 to 5 of section 5: the batch finishes, unknown items are `needs_review` and appear in `/vendor/integrations/review`.

## 9. Load check (implementer, not in CI)
```
k6 run test/load/erp_sync.js -e BASE=http://localhost:8080 -e KEY=$KEY -e ITEMS=50000
```
`test/load/erp_sync.js` is new (the folder already holds `k6_pages.js`). Expect: the 202 in under 5 seconds (SC-002) and `completed` in under 7 minutes on the dev machine. Run again with `-e KEYS=k1,k2,...,k10` (ten keys created in the website, section 3) and watch `river_job` for the `erpsync` queue draining. If it does not drain in 7 minutes, raise `WORKER_ERPSYNC`.

SC-003 (website latency while syncs run): `k6 run test/load/erp_sync.js -e BASE=... -e KEYS=... -e EMAIL=... -e PASSWORD=... -e SYNC=0` records the baseline p95 of the website pages; the same command with `-e SYNC=1` repeats it while the keys send batches every 7 minutes. Pass when the p95 with `SYNC=1` is at most 1.2 times the p95 with `SYNC=0`. Keys made in one supplier account model one organization, so this models fewer tenants than the 50 in SC-003; record the number of keys used and have the owner confirm with real tenants before launch.

## 10. Rollback
Redeploy the previous image. The tables are unused by it; migrations are expand-only. To switch the API off without a rollback set `ERP_API_ENABLED=false`: no `/erp/v1` routes are mounted and the erpsync worker does not start. Existing n8n bridges and `/api/v1` routes are untouched either way.
