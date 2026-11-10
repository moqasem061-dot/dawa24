# ERP v2 changes for Orgasoft (2026-11-10)

Four owner requests, turned into exact build steps. Read `AGENTS.md`, then
`docs/modules/erp.md`, before touching code. The 400-line file limit is waived
by the owner; every other rule in `AGENTS.md` applies (no float money, tenant
reads in `InTx`/`InReadTx`, bilingual text, no module-to-module imports).

Phase 1 = sections 1 and 2. Phase 2 = section 3. Section 4 lists the checks for
both phases.

---

## 1. Short IDs (20 characters at most, never `/` or `+`)

Orgasoft fails on long values. Every identifier the ERP API **returns or
accepts as ours** must be 1–20 characters from `[0-9A-Za-z_-]`. Values the ERP
sends us (Idempotency-Key, externalReference, erpReference, item codes) are
theirs and are stored as sent.

| Value | Today | New |
|---|---|---|
| `requestId` / `X-Request-ID` | 24 hex chars; inbound up to 64 | 16 hex chars (8 random bytes). Inbound header adopted only if 1–20 chars and matches `^[0-9A-Za-z_-]+$`, else a new one is generated. The crypto failure fallback becomes `"t" + strconv.FormatInt(time.Now().UnixNano(), 36)` (14 chars). File: `internal/platform/httpx/middleware.go`. |
| `syncId` (+ `statusUrl`) | UUID (36) | Decimal text of `erp.sync_batches.id`, e.g. `"4821"`. |
| `variantId` (sales lines, purchase lines, offers, purchase-order errors) | UUID (36) | Decimal text of `catalog.product_variants.id`, e.g. `"91530"`. |
| `page.nextCursor` | base64url of `v1:<id>` (up to 30) | Decimal text of the last row id, e.g. `"10542"`. |

Rules:

- **Input stays backward compatible.** Wherever we accept one of these values
  (`/products/sync/{syncId}`, `/products/sync/{syncId}/items`, purchase
  `lines[].variantId`, `cursor`), accept both forms:
  - all digits (`^[0-9]{1,19}$`, parses to int64 > 0; the cursor also allows 0) → look up by the numeric id;
  - a UUID (old form; the cursor's old base64 `v1:` form) → look up the old way;
  - anything else → the existing not-found / `ERP_CURSOR_INVALID` answer, unchanged.
- Add one helper in the erp package, `func ParseShortID(s string) (int64, bool)`
  (digits only, 1–19 chars, no sign, fits int64, > 0), with a table test.
- `purchase lines[].variantId` may arrive as a JSON number or a string. Accept
  both (decode into `json.RawMessage` and convert; a JSON number must be an
  integer).
- Sync lookups stay tenant-scoped: `WHERE b.organization_id = $1 AND b.id = $2`.
- `VariantOfferAsSystem` (E6) gets a numeric branch: `WHERE v.id = $1` vs the
  existing `WHERE v.public_id = $1::uuid`. Keep the function name (it is a named
  exception), and change its comment and parameter name to say "id or public id".
- Every SQL that today selects `v.public_id::text` for output selects
  `v.id::text` instead (`orders_sales.go`, `orders_purchase.go`, `offers.go`).
  Selecting the variant to *write* stays unchanged.
- `EncodeCursor(id)` returns `strconv.FormatInt(id, 10)`. `DecodeCursor` accepts
  digits, then the old base64 form, else `ERP_CURSOR_INVALID`.
- Update the contract (`specs/002-erp-integration-api/contracts/openapi.yaml`;
  copy it byte for byte to `internal/modules/erp/http/spec/openapi.yaml`). In it:
  - `SyncId` parameter: `{type: string, pattern: "^[0-9]{1,20}$"}`, with a description saying the old UUID form is still accepted;
  - every `variantId`: `{type: string, maxLength: 20}`;
  - `Cursor`: `maxLength: 20`;
  - every example uses short values (`syncId: "4821"`, `variantId: "91530"`, a 16-hex `requestId`);
  - the info text gets one line: "Every id Dawa24 returns is at most 20 characters of `0-9 A-Z a-z _ -`."
- Regenerate Postman: `python docs/erp-api/oas2postman.py` (read its header for
  the arguments). The collection's examples must contain no UUID.
- Tests:
  - `ParseShortID` and the cursor round trip, including the legacy base64 token;
  - the request-id middleware (generated id is 16 hex; a 21-char or `a/b` inbound id is replaced; `abc-123` is kept);
  - a contract-level test that walks every example in the OpenAPI file and fails if any `syncId`, `variantId`, `requestId` or `nextCursor` value is longer than 20 or contains `/` or `+`;
  - fix every existing test that asserted a UUID `variantId` or `syncId`.
  `grep -rn "public_id\|UUID\|uuid" internal/modules/erp` lists the call sites.

## 2. Orgasoft item codes and the Orgasoft order format

### 2a. Keep the item ID Orgasoft sends

- **Sales (supplier).** The product code from `POST /products/sync`
  (`externalProductCode`) is already stored in `erp.product_links.external_code`,
  and sales lines read it. Two changes in `provider/orgasoft/fields.go` and
  `orgasoft.go`:
  - add the aliases `itemId`, `itemCode`, `code`, `productCode` for
    `externalProductCode` (first-letter case folding already covers `ItemId` and the rest);
  - decoding must be **deterministic**: if a row has `externalProductCode`, it
    always wins over any alias, whatever the map iteration order. Implement this
    by collecting the code candidates per row and picking by a fixed priority
    order (`externalProductCode`, `itemId`, `itemCode`, `productCode`, `code`).
  - test with a row that has both `externalProductCode` and `itemId`, run in a
    loop of 50, and with a numeric `ItemId: 100001`.
- **Purchase (pharmacy or supplier buying).** New table, migration
  `260_erp_item_codes.up.sql` / `.down.sql`, wrapped in `BEGIN; … COMMIT;`, in the
  style of `255_erp_schema.up.sql` (copy its RLS policy and GRANT block form):

  ```sql
  CREATE TABLE erp.item_codes (
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    variant_id      BIGINT NOT NULL REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
    integration_id  BIGINT REFERENCES erp.integrations(id) ON DELETE SET NULL,
    external_code   TEXT   NOT NULL CHECK (length(external_code) BETWEEN 1 AND 64),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, variant_id)
  );
  -- ENABLE + FORCE RLS, policy item_codes_tenant_isolation with platform.tenant_visible(organization_id)
  -- Arabic COMMENT on the table and every column
  ```

  - `POST /orders/purchase` `lines[]` gets an optional `code` (aliases
    `itemId`, `itemCode`), string or JSON number, trimmed, 1–64 chars, else
    `ERP_VALIDATION` with `details.field = "lines[n].code"`.
  - `FinishIdempotency` upserts every line that has a code into `erp.item_codes`
    (`ON CONFLICT (organization_id, variant_id) DO UPDATE SET external_code, integration_id, updated_at = now()`),
    inside the transaction it already opens, under `WithTenant(buyer org)`.
    The variant is the line's resolved numeric variant id, not the text the ERP sent.
  - Purchase lines (`purchaseLinesSQL`) read `externalProductCode` from
    `erp.item_codes` for the buyer organization (`LEFT JOIN erp.item_codes c ON c.organization_id = $1 AND c.variant_id = ol.product_variant_id`).
    Because that statement runs AsSystem (E7), the join **must** pin
    `c.organization_id = $1`. It is null when no code is stored.
  - Add `erp.item_codes` to the ERP tenant-isolation integration test
    (`test/integration/erp_isolation_test.go`): a cross-tenant read returns zero rows.
  - Retention: none (it is a mapping, not a log).

### 2b. The Orgasoft order feed

Orgasoft gave this shape and wants it exactly (a bare JSON array, no envelope):

```json
[{"orderId":21845,"status":"PENDING","total":1264.25,"pharmacyId":1,"pharmacyName":"Direct",
  "pharmacyCode":263,"pharmacyNumber":"01008042755","pharmacyCity":"Fayoum","pharmacyAddress":"الفيوم",
  "items":[{"code":100001,"name":"سبيركودا 20 قرص","qty":1,"price":265.00,"discount":21.00}]}]
```

Their own numbers prove the meaning: `price` is the public (list) unit price,
`discount` is a **percent**, and `total = Σ qty × price × (1 − discount/100)`
(1×265×0.79 + 3×385×0.78 + 1×220×0.70 = 1264.25).

New routes, same scopes, query parameters and paging as the existing feeds
(`state`, `from`, `to`, `cursor`, `limit`):

- `GET /erp/v1/orgasoft/orders/sales` (scope `erp.orders.read`, suppliers only, same checks as `/orders/sales`)
- `GET /erp/v1/orgasoft/orders/purchase` (scope `erp.orders.read`, same checks as `/orders/purchase`)

Response:

- Body = the bare array.
- Headers:
  - `X-Next-Cursor`: the next cursor, or absent on the last page;
  - `X-Request-ID`;
  - `Content-Type: application/json`.
- Errors keep the normal error envelope.
- Reuse the existing service calls (`SalesOrders`/`PurchaseOrders` or whatever
  `listSales`/`listPurchase` call) and only add a second renderer. No new SQL
  for the headers except the fields below.

Field mapping. All money is written from `money.Amount.String()` as a raw JSON
number (`json.RawMessage`), so `265.00` stays `265.00`. No float anywhere.

| Field | Sales (one entry per supplier shipment) | Purchase (one entry per order) |
|---|---|---|
| `orderId` | order number | order number |
| `status` | shipment status, upper-case | order status, upper-case |
| `total` | shipment total (subtotal + shipping) | order total |
| `pharmacyId` | buyer organization id (0 if none) | the key's own organization id |
| `pharmacyName` | buyer org name, Arabic, else English | own org name, Arabic, else English |
| `pharmacyCode` | customer code (`Customer.ErpCode`), null when unmapped | null |
| `pharmacyNumber` | branch phone, else org phone | ordering branch phone, else org phone |
| `pharmacyCity` | branch city name, English, else Arabic, else `""` | same, for the ordering branch |
| `pharmacyAddress` | branch address, else org address | same, for the ordering branch |
| `items[].code` | `externalProductCode` from product links | `externalProductCode` from `erp.item_codes` |
| `items[].name` | line product name, Arabic, else English | same |
| `items[].qty` | quantity (integer) | same |
| `items[].price` | `list_price` if > 0, else `unit_price` | same |
| `items[].discount` | percent, see below | same |

More rules for the feed:

- **Number-or-text rule** (`orderId`, `pharmacyCode`, `items[].code`): if the
  value matches `^(0|[1-9][0-9]{0,17})$`, write it as a JSON number; otherwise
  write it as a JSON string; empty means `null`. One helper,
  `func orgasoftScalar(s string) json.RawMessage`, with a table test (`"263"`→`263`,
  `"007"`→`"007"`, `"ORGA-100"`→`"ORGA-100"`, `""`→`null`, 19 digits → string).
- **Discount percent**: a pure function in the erp package,
  `DiscountPercent(list, unit money.Amount) money.Amount`, integer math only:
  - if `list <= 0` or `unit >= list`, the result is 0;
  - otherwise `hundredths = ((list−unit)·10000 + list/2) / list` on minor units, returned as `money.FromMinor(hundredths)`.
  - Table test: 265.00/209.35 → 21.00, 385.00/300.30 → 22.00, 220.00/154.00 → 30.00, 62.50/54.69 → 12.50, 0/10 → 0, 10/10 → 0.
- The SQL needs `ol.list_price` (`COALESCE(ol.list_price, 0)`) added to both
  line statements, and a `ListPrice money.Amount` field on `erp.OrderLine`.
- The sales header needs the buyer organization id (already selected as
  `ref.CustomerOrgID`, so copy it onto the row) and the city:
  `LEFT JOIN platform_admin.cities ci ON ci.id = b.city_id`, selecting
  `COALESCE(NULLIF(ci.name->>'en',''), ci.name->>'ar', '')`. Check that
  `dawa24_app` can read `platform_admin.cities` (it is reference data). If it
  cannot, return `""` and note it in the doc.
- The purchase entry needs the ordering branch's phone, city and address, and
  the org name. Add them to the purchase header statement (`o.branch_id` →
  `org.branches`; org via `o.organization_id`).
- Line `discount_amount`, shipping and notes are not in the Orgasoft shape, so
  leave them out. Do not add fields Orgasoft did not ask for.
- **Ack**: Orgasoft acknowledges with the same id it read.
  - In `provider/orgasoft/ack.go`, add `orderId` to the order-number keys
    (`"orderNumber", "order_number", "orderId"`), after the existing ones.
  - Add `erpOrderId` and `orgaOrderId` to the reference keys.
  - A JSON number for `orderId` must work (it already does via `json.Number`).
    Test it.
- **Contract**:
  - add both routes to the OpenAPI file with the example above as the response example;
  - add a schema `OrgasoftOrder` and the `X-Next-Cursor` header;
  - document that the two Orgasoft-shape assumptions (purchase entries describe
    the ordering pharmacy, `pharmacyCity` uses the English city name) are still
    to be confirmed with Orgasoft;
  - `TestOpenAPIMatchesRoutes` must pass;
  - regenerate Postman.
- **Tests (http package)**, built from fake data with the three lines of the
  sample. The rendered JSON must equal, byte for byte after `json.Compact`:
  `[{"orderId":21845,"status":"PENDING","total":1264.25,…,"items":[{"code":100001,…,"price":265.00,"discount":21.00},…]}]`.
  Also test `X-Next-Cursor` present/absent, the scope refusal, and that a
  pharmacy key gets 403 on the sales route.

Docs: add a section "Orgasoft order feed and item codes (2026-11-10)" to
`docs/modules/erp.md`. Add the new endpoints and short ids to
`docs/erp-api/README.md` and `specs/002-erp-integration-api/quickstart.md`.

---

## 3. Integration activity screen (tenant monitoring)

Goal: a supplier or pharmacy sees every API call, every product sync with live
progress, and every order import, in one clear screen.

### 3a. Record more per call

Migration `261_erp_api_calls_detail.up.sql` / `.down.sql` (`BEGIN; … COMMIT;`, Arabic comments):

```sql
ALTER TABLE erp.api_calls
  ADD COLUMN query        TEXT  NOT NULL DEFAULT '',   -- raw query string, first 500 chars
  ADD COLUMN user_agent   TEXT  NOT NULL DEFAULT '',   -- first 200 chars
  ADD COLUMN bytes_out    INT   NOT NULL DEFAULT 0,
  ADD COLUMN ref          TEXT  NOT NULL DEFAULT '',   -- syncId / orderNumber / externalReference the call touched
  ADD COLUMN auth_reason  TEXT  NOT NULL DEFAULT '',   -- why a key was refused
  ADD COLUMN error_detail JSONB;                       -- the error "details" object, at most 2 KB
CREATE INDEX api_calls_org_id_idx ON erp.api_calls (organization_id, id DESC);
```

The down migration drops the index and the columns. Old images keep working:
their COPY names its columns.

Code (`internal/modules/erp/http/auth.go`, `respond.go`, `domain.go`, `postgres/calls.go`):

- `CallRecord` gets `Query, UserAgent, Ref, AuthReason string; BytesOut int; ErrorDetail []byte`.
- `callState` gets `errorCode string; errorDetail []byte; ref string`.
  - `writeError` stores the code and the details JSON (`nil` when there are no
    details; when the marshalled JSON is over 2048 bytes, store `{"truncated":true}`)
    on the call state, when one is in the context.
  - The recorder uses `state.errorCode` when the middleware's own `code` is empty.
  - **Today, handler errors such as `ERP_VALIDATION` are recorded with an empty error code. This fixes that.**
- A helper `setCallRef(ctx, ref string)`. Call it:
  - in the sync accept handler (the new syncId);
  - in sync status/items (the path id);
  - in get/decide order (the order number);
  - in purchase create (the externalReference).
- `statusWriter` counts bytes written into `BytesOut`.
- **Refused keys show up in the tenant's own log.**
  - In `authenticate.go`, add `type AuthRefusal struct { OrganizationID, KeyID int64; KeyPrefix, Reason string; Err *Error }`
    with `Error()` and `Unwrap()` returning `Err`.
  - When a key row was found (the reasons `secret_mismatch`, `key_status`,
    `key_expired`, `integration_status`, `org_lookup_error`, `org_status`,
    `touch_error`), `refuse` returns an `*AuthRefusal`.
    `malformed`, `lookup_error` and `unknown_prefix` keep returning the bare error.
  - The answer to the caller is unchanged: the same 401 `ERP_AUTH_INVALID` body.
    `writeError` already uses `errors.As`, so that must keep working. Test it.
  - The middleware, on `errors.As(authErr, &refusal)`, records `OrganizationID`,
    `APIKeyID`, `KeyPrefix` and `AuthReason` from the refusal.
  - The existing server log line stays.
- `InsertCallsAsSystem` writes the new columns.

### 3b. Reads for the screen (erp service + repository, all tenant-scoped `InReadTx(WithTenant(org))`)

- `ActivitySummary(ctx, org) (ActivitySummary, error)`:
  - integration status;
  - active key count;
  - last call (time, status, route);
  - calls, errors, and average duration in the last 24 h;
  - batches queued, processing, and failed in the last 24 h;
  - open review items;
  - sales and purchase orders in the `new` and `changed` import state.
  For the sales count, use the existing E2 feed functions with `state=new`,
  `limit=500` and count the result, or write a dedicated `...AsSystem` count
  added to the named-exception table in `docs/modules/erp.md`. Pick the first;
  it adds no exception.
- `ListCalls(ctx, org, CallFilter{BeforeID int64; Result string /* all|ok|error */; KeyPrefix, Route string; From, To *time.Time; Limit int})`.
  - Keyset on `id DESC`, `id < BeforeID` when it is > 0.
  - `ok` means `status < 400`; `error` means `status >= 400`.
  - Limit is 50 by default and 200 at most.
  - Join `erp.api_keys` for the key name (left join; key may be gone).
- `ListBatches(ctx, org, beforeID int64, limit int)`. Newest first, with the key
  name. The rows carry every `sync_batches` count and time column.
- `GetBatch(ctx, org, id)` and `ListBatchItems(ctx, org, batchID, state string, afterPosition int, limit int)`.
  Reuse the existing sync read code where it already does this.
- `ListOrderImports(ctx, org, limit)`: the latest `erp.order_imports` rows
  (direction, order number, erp reference, acked status, acked at, ack count).

### 3c. Pages

Routes:

- vendor: in `internal/ui/vendor_routes_erp.go`, under `vendor.integration.view`;
- pharmacy: in `internal/ui/customer_routes.go` next to `/customer/integrations`,
  under the same permission that page uses.

| Path | What |
|---|---|
| `/vendor/integrations/activity`, `/customer/integrations/activity` | The screen; `?tab=overview|syncs|calls|orders`, default `overview`. The pharmacy has no `syncs` tab. |
| `…/activity/syncs/{id}` | One batch with its items (vendor only). |
| `…/activity/live` | HTML fragment of the syncs-in-progress panel, for polling. |

The screen (`internal/ui/pages/erp_activity.templ`, RTL first, existing
glass-panel/card styles, no new colors):

- **Overview tab**:
  - status cards: integration active/paused (with the Resume button when paused,
    for the owner, posting to the existing pause route);
  - the summary numbers from 3b;
  - the 10 latest calls;
  - the live panel.
- **Live panel**: every queued or processing batch.
  - Show a progress bar: `processed = created+updated+unchanged+needs_review+failed`
    over `item_count`, as a percent, and `chunks_done/chunk_count`.
  - It refreshes itself with `hx-get="…/activity/live" hx-trigger="every 5s" hx-swap="outerHTML"`
    only while it holds an active batch; when there is none, the fragment
    renders without the trigger, so polling stops.
  - Copy the polling attribute pattern from `internal/ui/components/notifications_bell.templ`.
- **Syncs tab**: a table of batches with these columns:
  - syncId, accepted time, key name;
  - state badge (queued grey, processing blue, completed green, completed with failures amber, failed red);
  - progress bar, the five counts;
  - wait time (`started − accepted`), run time (`finished − started`);
  - error code with its bilingual message.
  "Older" paging uses `?before=`. A row links to the batch page.
- **Batch page**:
  - the header numbers;
  - a state filter (all/created/updated/unchanged/needs_review/failed);
  - an items table with position, code, state and message (`ar`/`en` by locale), 100 per page;
  - a "needs review" link to `/vendor/integrations/review`;
  - when items were purged (7-day retention), a note saying so.
- **Calls tab**:
  - filters (result, key, route, from, to) as a GET form;
  - a table with these columns: time, key (name + prefix), method, route,
    status (colored), error code and message, duration ms, request id with a
    `$copy` button (copy the pattern from `erp_key_modals.templ`), ref
    (linked to the batch page when it is a syncId);
  - each row expands with `<details>` to show query, user agent, IP, bytes in/out,
    the error details JSON (pretty-printed, escaped by templ) and the auth refusal
    reason explained in words. Add i18n keys
    `erp.activity.reason.<reason>` for every reason. Examples:
    - `integration_status`: "الربط موقوف مؤقتاً. اضغط «استئناف» في صفحة التكامل." / "The integration is paused. Press Resume on the integration page."
    - `secret_mismatch`: "المفتاح منسوخ بشكل خاطئ أو قديم." / "The key was copied wrong or is an old one."
  - "Older" paging uses `?before=`.
- **Orders tab**:
  - the counts of new/changed;
  - the latest 50 acknowledgements, with columns: direction, order number,
    ERP reference, acked status, acked time, ack count.
- **Empty states** in both languages for every table, e.g. "لم تصل أي طلبات من نظامك بعد".

Navigation: add an item `integration_activity` ("سجل التكامل" / "Integration
activity", icon `activity` if it exists in the icon set, else `layers`) to the
`integrations` section:

- in `internal/platform/rbac/nav_vendor.go`, between the two existing items;
- in `nav_pharmacy.go`, after the keys item.

Use the same permission as the screen. Set the shell active key to
`integration_activity`. Update `nav_integrations_test.go`.

All new text goes in `internal/shared/i18n/catalog_erp_ui.go`, with keys under
`erp.activity.*`, both languages.

### 3d. Tests for phase 2

- **http**:
  - the recorder stores the handler error code (an `ERP_VALIDATION` call);
  - the ref is set for sync accept;
  - a refused key with a found row records org, key, prefix and reason, while the
    response body is identical to an unknown-prefix refusal;
  - bytes_out > 0.
- **erp service**: `ListCalls` filter and limit clamping (fake repo).
- **ui**:
  - the activity page renders each tab for vendor and pharmacy;
  - the pharmacy gets no syncs tab, and its batch route is 404;
  - the live fragment has `hx-trigger` only when a batch is active;
  - a user without the permission is refused;
  - the calls row shows the reason text.
- **integration** (`test/integration`):
  - `ListCalls` and `ListBatches` return zero rows for another tenant;
  - migration 261 applies and the new columns are written by `InsertCallsAsSystem`.
- Add the new methods to every fake repository that implements the interfaces
  (`grep -rn "InsertCallsAsSystem" --include=*_test.go`).

Docs: in `docs/modules/erp.md`, add a section "Integration activity screen
(2026-11-10)" saying what is recorded, the retention (calls 30 days, items 7
days, batches 90 days), and that refused keys with a known prefix now appear in
the owner's log.

---

## 4. Checks for each phase (all must pass before reporting)

Host shell setup: `export GOTMPDIR='F:\dawa24\tmp\gotmp'; unset DATABASE_URL TEST_DATABASE_URL TEST_REDIS_URL`.

1. After any `.templ` change:
   `go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate -f <file>`.
2. `go build ./...` and `go vet ./internal/... ./cmd/...`.
3. `go test ./internal/modules/erp/... ./internal/platform/httpx/... ./internal/platform/rbac/... ./internal/ui/... ./cmd/cli/... ./test/ -count=1`.
   The `test/` package runs the module-boundary and OpenAPI gates.
4. The two OpenAPI copies are byte-identical (`cmp`).
5. After a template or CSS change, refresh the critical-CSS fingerprint
   (`TestHomepageCriticalCSSIsCurrent`):
   1. `go build -o tmp/pagespeed.exe ./scripts/pagespeed`;
   2. run it in the background (it listens on 8097);
   3. `npm run critical --prefix scripts/pagespeed`;
   4. stop it, then delete `tmp/pagespeed.exe`.
6. **Do not**:
   - commit;
   - stage `test/visual_baselines/*.png`;
   - read `.env`;
   - touch `muhiya-*` containers or production.
