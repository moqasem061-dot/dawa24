# PHASE 3 — Procurement: Purchase Request, Priority Engine, Automation

**Depends on:** Phase 1 (`catalog.product_index`, institutional filter — the
priority engine reads both), Phase 2 (column detection and matching are reused).
**Blocks:** Phase 7 (the customer sidebar has three entries from this phase).
**Tasks:** 4.

## Why this phase exists

The `/what-in` page lists three of the four customer "Operations & Smart Savings
Engine" pillars, and Go has **none** of them:

- محرك أولوية الشراء (Purchase Priority Engine) — table exists, no service, no handler, no screen
- طلب شراء أوتوماتيكي (Automatic Purchase Request) — 4,696-line Laravel service, nothing in Go
- طلب الشراء (Purchase Request) — 5 Laravel screens, nothing in Go

`workflow.purchase_priority_engines` is one of the 21 Go tables that no route
touches. This phase gives it a reason to exist.

---

## Shared prerequisite: the "product_infos" dependency

`PurchasePriorityEngineService` reads Laravel's `product_infos` **directly**:

```php
$query = DB::table('product_infos')
    ->select(['product_id','product_name','product_price','product_price_discount',
              'product_sku','product_barcode','organization_id','branch_id',
              'stock_quantity','parent_product_name','organization_name',
              'branch_name','institutional_work_ids'])
    ->where('product_status','active')->where('stock_quantity','>',0);
```

That is `catalog.product_index` from Phase 1. **Phase 1 must be complete.** Note
the column names differ slightly from the SQL dump (`product_name` vs `name_ar`,
`product_price` vs `price`) — the Laravel service may be reading a *view* over
the table. **Inspect and reconcile before writing the query**:

```bash
grep -rn "product_infos" F:/Dawa\ 24/Laravel/database/migrations/ F:/Dawa\ 24/Laravel/app/Models/ProductInfo.php
sed -n "/CREATE.*VIEW.*product/,/;/p" F:/Dawa\ 24/u924222867_Testv5.sql
```

If a view exists, reproduce it as a Postgres view over `catalog.product_index`
so the column names in this phase's queries match Laravel's.

---

## TASK 3.1 — Purchase Request (طلب الشراء)

Five Laravel screens; the customer's manual procurement path.

### 3.1.1 Inspect first

```bash
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/PurchaseRequest.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/PurchaseRequestSupplier.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/ShowPurchaseRequestSupplier.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/PurchaseRequestProducts.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/PurchaseRequestPrevious.php
cat F:/Dawa\ 24/Laravel/app/Models/RequestOffer.php
sed -n "/CREATE TABLE \`request_offers\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
# and every Blade view under resources/views/livewire/customer/
```

`PurchaseRequest.php` is a **wizard**: `$stap` (sic — Laravel's typo) is the step
number, `$option` selects the path. Extract:
- how many steps, and what each step asks
- the two or more `$option` paths (by supplier? by product?)
- filters on the product picker (`$searchTerm`, `$categoryFilter`)
- what a submitted request creates in the database
- what the vendor sees, and how they respond

### 3.1.2 Data model decision

Go has `commerce.quote_requests`, reachable only through
`POST /suppliers/{id}/quote`. Determine whether it is the right home for a
purchase request, or whether Laravel's `request_offers` is a distinct concept.

**Inspect both, then decide and record in `docs/modules/commerce.md`.** If
`quote_requests` fits, extend it. If not, add `commerce.purchase_requests` +
`commerce.purchase_request_lines`.

Whichever you choose, the shape must carry: requesting org, requesting user,
requesting branch, target supplier(s), line items (product, quantity, notes),
status, created/responded timestamps, and the vendor's response.

### 3.1.3 Institutional filter — **WithConnections mode**

`Laravel/docs/institutional_work_filter.md` states explicitly that
`/customer/purchase-request/products` and
`/customer/purchase-request/supplier/{id}` use **mode 2 (WithConnections)**,
which has **no fallback for unrestricted products**.

This is the asymmetry Phase 1 Task 1.1 ported. Use `FilterWithConnections` here.
Getting this wrong silently shows products the customer must not see.

### 3.1.4 Screens

| Route | Laravel source | Contents |
|---|---|---|
| `/customer/purchase-request` | `PurchaseRequest` | the wizard: option selection, then supplier or product path |
| `/customer/purchase-request/supplier` | `PurchaseRequestSupplier` | supplier list, filtered by coverage + institutional works |
| `/customer/purchase-request/supplier/{id}` | `ShowPurchaseRequestSupplier` | one supplier's catalog, WithConnections filter, add-to-request |
| `/customer/purchase-request/products` | `PurchaseRequestProducts` | cross-supplier product picker, WithConnections filter, search + category filter |
| `/customer/purchase-request/previous` | `PurchaseRequestPrevious` | history of submitted requests with status and responses |

Use `@components.Stepper` for the wizard. **Each step transition posts to the
server** — no client-only advance. That was the `vendor_ingest.templ` failure.

Also note: `/customer/suppliers/{id}` maps to the same component as
`/customer/purchase-request/supplier/{id}` in Laravel
(`Route::get('/suppliers/{id}', CustomerShowPurchaseRequestSupplier::class)`).
Reproduce that aliasing or record why not.

### 3.1.5 Vendor side

The vendor must see and respond to requests. `RequestRespondSubmit` already
exists (`POST /requests/{id}/respond` in `RegisterVendorRoutes`) — verify it
handles this request type, or extend it. The vendor's `/requests` page must list
incoming purchase requests with the customer, lines, and a respond action.

### 3.1.6 Tests

- T1: wizard state machine — invalid transitions rejected
- T2: request + lines round-trip
- T3: cross-tenant — customer B cannot read customer A's requests; vendor C cannot see a request addressed to vendor D
- T6: full flow — customer submits, vendor sees it, vendor responds, customer sees the response
- T10 (from Phase 1): **WithConnections mode is used on both product screens**, and an unrestricted product is *not* shown

---

## TASK 3.2 — Purchase Priority Engine (محرك أولوية الشراء)

`PurchasePriorityEngineService.php`, 379 lines. Table exists in Go; everything
else is missing.

### 3.2.1 Inspect first — read the whole service

```bash
cat F:/Dawa\ 24/Laravel/app/Services/PurchasePriorityEngineService.php
cat F:/Dawa\ 24/Laravel/app/Models/PurchasePriorityEngine.php
sed -n "/CREATE TABLE \`purchase_priority_engines\`/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
grep -n "priority" F:/Dawa\ 24/Laravel/app/Livewire/Customer/AutomationRequest.php
```

Extract into `docs/modules/workflow.md`:
- `getProductsByPriorities` — the full query and every filter
- `applyAIRanking` — the scoring formula. **This is the core business rule.**
  Write it out as pseudocode with the exact weights.
- `generateRecommendations` — what makes a recommendation
- `generateProcessingSummary` — the summary shape
- the status lifecycle: `markAsProcessing` / `markAsCompleted` / `markAsFailed`
- what `request_number` looks like (format matters — rule R7 says order-number
  formats are ported exactly; assume the same applies here)

### 3.2.2 Schema reconciliation

Go's `workflow.purchase_priority_engines` already exists. Compare it column by
column against Laravel's:

| Laravel column | Present in Go? |
|---|---|
| `request_number varchar` | verify |
| `status varchar default 'pending'` | verify |
| `priorities longtext(json)` | verify |
| `priority_highest_discount tinyint` | verify |
| `priority_lowest_price tinyint` | verify |
| `priority_fastest_delivery tinyint` | verify |
| `priority_preferred_suppliers_only tinyint` | verify |
| `budget_constraint decimal(12,2)` | verify |
| `processing_parameters longtext(json)` | verify |
| `matched_products longtext(json)` | verify |
| `ranking_results longtext(json)` | verify |
| `recommendations longtext(json)` | verify |
| `notes text` | verify |
| `meta longtext(json)` | verify |
| `processed_at timestamp` | verify |
| `processed_by bigint` | verify |

Add a migration for anything missing. JSON columns become `JSONB`.
`budget_constraint` is money → `NUMERIC(12,2)`, read as `money.Amount` (rule R1).

### 3.2.3 Implementation

New file set in `internal/modules/workflow/`:
- `priority_domain.go` — the four priority flags, the scoring type, pure ranking
- `priority_service.go` — orchestration
- `postgres/priority.go` — the product query (reads `catalog.product_index`)
- `jobs/priority.go` — a River worker; ranking is not request-time work

The scoring function must be **pure and unit-tested against the Laravel formula**:

```go
// Score ranks one candidate product against the requester's priorities.
// The weights reproduce PurchasePriorityEngineService::applyAIRanking exactly;
// see docs/modules/workflow.md for the derivation. Do not tune them.
func Score(p Candidate, prefs Priorities) float64
```

**Institutional filter:** Laravel's `getProductsByPriorities` applies
`employee_institutional_works` directly (see the code excerpt in
`PARITY_AUDIT_V4.md` §5.1). Use the Phase 1 gate. Determine from the code
whether it is Simple or WithConnections semantics — the excerpt shows a
`whereNull OR whereIn` shape, which is **Simple**. Verify.

**"Fastest delivery" priority** needs a delivery-time signal. Determine what
Laravel uses — `org.delivery_bands`? coverage distance? a supplier field? — and
map it. If Laravel has no real signal and the flag is vestigial, record that and
implement it as a no-op with a comment, rather than inventing a heuristic.

### 3.2.4 UI

Laravel exposes it inside `AutomationRequest` via `showPriorityEngine`,
`priorityPriorities`, `priorityBudgetConstraint`, the four flags,
`priorityEngines` (history), `expandedPriorityEngines`, `showPriorityHistory`.

Screen requirements:
- a form with the four priority toggles + budget constraint + free-form
  `priorities` ordering
- submit → creates a `purchase_priority_engines` row, enqueues the job
- a progress view (`status`) that polls
- results: ranked products with the score, the recommendation set, and the summary
- history list with expandable past runs
- "add ranked results to cart / to a purchase request" — **check what Laravel
  does with the results** and reproduce it. Results nobody can act on are useless.

### 3.2.5 Tests

- T1: `Score` matches the Laravel formula on ≥15 fixtures covering each flag alone and in combination
- T1b: budget constraint excludes over-budget candidates exactly at the boundary
- T2: engine row lifecycle pending → processing → completed / failed
- T3: cross-tenant — the candidate set never includes another org's private data beyond what a marketplace read allows
- T6: submit → job runs → results render → history shows the run
- T10: the institutional filter is applied to the candidate query

---

## TASK 3.3 — Automatic Purchase Request (طلب شراء أوتوماتيكي)

`EnhancedAutomationService.php` — **4,696 lines**, plus
`PharmaceuticalAutomationService.php` (625), `AutomatedOrderOptimizationService`
(370), `OrderOptimizationService` (305), `GeolocationSupplierOptimizer` (278),
and two queued jobs.

This is the single largest capability in the platform.

### 3.3.1 Scope it before you build it

**Do not attempt a line-by-line port.** Read the service, extract the *contract*,
and implement that contract on Go architecture.

```bash
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/AutomationRequest.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/AutomationPrevious.php
cat F:/Dawa\ 24/Laravel/app/Livewire/Customer/AutomationRequestComponent.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ProcessAutomationFile.php
cat F:/Dawa\ 24/Laravel/app/Jobs/ProcessEnhancedAutomationJob.php
cat F:/Dawa\ 24/Laravel/app/Models/AutomationRequest.php
sed -n "/CREATE TABLE \`automation/,/ENGINE=/p" F:/Dawa\ 24/u924222867_Testv5.sql
# read the big one in sections
sed -n '1,400p'    F:/Dawa\ 24/Laravel/app/Services/EnhancedAutomationService.php
sed -n '400,900p'  F:/Dawa\ 24/Laravel/app/Services/EnhancedAutomationService.php
# ... continue through the file
cat F:/Dawa\ 24/Laravel/app/Services/GeolocationSupplierOptimizer.php
cat F:/Dawa\ 24/Laravel/app/Services/OrderOptimizationService.php
cat F:/Dawa\ 24/Laravel/app/Services/AutomatedOrderOptimizationService.php
```

Write a **contract document** at `docs/modules/workflow-automation.md` before
coding, containing:
1. the input (file format, expected columns, `importTargetColumns` from the component)
2. every processing stage in order
3. the output (what the customer receives, and in what shape)
4. every decision rule, with its inputs and thresholds
5. the location/distance logic (`useLocation`, `userLatitude`, `userLongitude`,
   `maxDistance` default **50**)
6. the alert mechanism (`alertPrices`, `alertDiscounts`, `alertQuantities`)
7. the "user choice" step (`userChoiceRequestId`, `showUserChoiceModal`) — what
   is the user being asked to choose between?
8. what `automationCart` accumulates and where it goes
9. the relationship between "automation" and "enhanced automation" (the component
   has both `$automationFile` and `$enhancedAutomationFile` with separate flows)

**This document is the deliverable of the inspection step.** Do not skip it. If
the contract is unclear after reading, record the ambiguity in
`OPEN_QUESTIONS.md` with your chosen interpretation.

### 3.3.2 Component surface (from the Livewire properties)

Every one of these is a required behaviour:

| Property | Behaviour |
|---|---|
| `automationFile` | upload the customer's requirements sheet |
| `searchTerm` | filter within results |
| `useLocation`, `userLatitude`, `userLongitude`, `maxDistance` (50) | restrict suppliers to a radius around the customer |
| `automationCart` | accumulated selections |
| `alertPrices`, `alertDiscounts`, `alertQuantities` | thresholds that flag lines needing attention |
| `enhancedAutomationFile` | the enhanced flow's upload |
| `enhancedPriorities`, `enhancedBudgetConstraint`, `enhancedHighestDiscount`, `enhancedLowestPrice`, `enhancedFastestDelivery`, `enhancedPreferredSuppliersOnly` | the same four priorities as Task 3.2, applied to the automation flow |
| `enhancedProcessing`, `enhancedResults` | async processing + results |
| `userChoiceRequestId`, `showUserChoiceModal` | mid-flow user decision |
| `showAutomationHistory`, `automationRequests`, `expandedRequests` | history |
| `importTargetColumns` | the column mapping targets — **reuse Phase 2's `DetectColumns`** |

### 3.3.3 Architecture

| Laravel | Go |
|---|---|
| `EnhancedAutomationService` (one 4,696-line class) | `internal/modules/workflow/automation/` — split by stage, each file < 400 lines (rule R6) |
| `ProcessAutomationFile` job | River worker `internal/modules/workflow/jobs/automation.go` |
| `ProcessEnhancedAutomationJob` | second worker, or a stage flag on the first |
| chunked upload controller | Phase 4's pipeline |
| `GeolocationSupplierOptimizer` | reuse `workflow.CoverageService.ServesPoint` and `platform.distance_meters` — **do not write a second Haversine** |
| AI calls | `internal/platform/gateway` capabilities only (rule R2), each with a deterministic fallback (R3) |

Suggested file split:
```
automation/
  domain.go        types, the request lifecycle
  parse.go         file → rows (reuses Phase 2 column detection)
  match.go         rows → catalog products (reuses Phase 2 matcher)
  suppliers.go     candidate suppliers: coverage + distance + institutional filter
  optimize.go      the allocation/optimisation rule
  alerts.go        price/discount/quantity threshold flagging
  result.go        assembling the output
```

### 3.3.4 Migration

`db/migrations/089_automation_requests.up.sql` — reproduce Laravel's automation
tables (names found in §3.3.1). RLS, Arabic comments, `migratecheck -roundtrip`.

### 3.3.5 Screens

| Route | Laravel |
|---|---|
| `/customer/automation-request` | `AutomationRequest` |
| `/customer/automation-request/template` | `downloadTemplate` — a sample sheet |
| `/customer/automation-previous` | `AutomationPrevious` |
| `POST /customer/automation-request/upload-chunk` | chunked upload (Phase 4) |
| `POST /customer/automation-request/clear-session` | reset the wizard |

Note Laravel's route: `Route::get('/automation-request/template',
[CustomerAutomationRequest::class, 'downloadTemplate'])`. Reproduce the template
download — the sample sheet must match the columns the parser expects.

### 3.3.6 Tests

- T1: each processing stage, unit-tested in isolation with fixtures
- T1b: the optimisation rule matches the contract document on ≥10 scenarios
- T2: request lifecycle persistence, resumable after a worker restart
- T3: cross-tenant isolation on requests and results
- T6: upload → process → user choice → results → cart/order
- T7: **the whole flow with the AI Gateway disabled** (rule R3)
- T8: money exactness on every total
- T14: distance filtering — a supplier at 51km is excluded when `maxDistance` is 50, one at 49km is included

---

## TASK 3.4 — Order optimisation services

`OrderOptimizationService` (305), `AutomatedOrderOptimizationService` (370),
`GeolocationSupplierOptimizer` (278), and `Customer/OrderOptimization`.

### 3.4.1 Decide whether this is separate

Read all three. They may be sub-components of Task 3.3 rather than a distinct
feature. **If they are only called from the automation flow**, fold them into
`automation/optimize.go` and record that decision. **If Laravel exposes a
standalone `/customer/order-optimization` screen**, build it.

```bash
grep -rn "OrderOptimizationService\|AutomatedOrderOptimizationService\|GeolocationSupplierOptimizer" \
  F:/Dawa\ 24/Laravel/app/ F:/Dawa\ 24/Laravel/routes/
ls F:/Dawa\ 24/Laravel/app/Livewire/Customer/ | grep -i optim
```

### 3.4.2 Completion criteria

- [ ] The decision is recorded with the grep evidence
- [ ] If standalone: the screen exists, routed, tested
- [ ] If folded: `automation/optimize.go` covers the behaviour and the tests reference it

---

## PHASE 3 COMPLETION GATE

```bash
make check
make check-provider-isolation
go run ./cmd/migratecheck -from 89 -roundtrip
go test ./internal/modules/workflow/... -race
go test ./test/integration/... -run 'Purchase|Priority|Automation'
```

- [ ] `docs/modules/workflow-automation.md` exists and documents the full contract
- [ ] A customer can complete a purchase request end to end and a vendor can respond
- [ ] The priority engine runs, ranks, and its results can be acted on
- [ ] The automation flow runs end to end, including the user-choice step
- [ ] **WithConnections mode is used on both purchase-request product screens** (T10)
- [ ] Everything works with the AI Gateway disabled (T7)
- [ ] `workflow.purchase_priority_engines` is no longer a dead table
- [ ] Customer sidebar entries 8 and 9 are live
- [ ] `PROGRESS.md` updated for 3.1–3.4
