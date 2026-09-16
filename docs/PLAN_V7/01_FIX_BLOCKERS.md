# PHASE 1 — Fix the two blockers

**The only phase that may add code.** Two bugs, both of which make core commerce
non-functional.

---

## TASK 1.1 — Weekly coverage: fix the TIME column mismatch

**Audit §1.1.** Two bugs on the same pair of columns. The marketplace returns
zero offers because of them.

### 1.1.1 The facts

`workflow.weekly_coverages` (migration `010_workflow.up.sql:39`):
```sql
coverage_from   TIME,     -- nullable
coverage_to     TIME,     -- nullable
```

`workflow.WeeklyCoverage` (`internal/modules/workflow/domain.go:41`):
```go
CoverageFrom   string     // wrong type, and not nullable
CoverageTo     string
```

**Bug A — read.** `ListCoverageForOrganization`
(`postgres/repository.go:217`) scans `TIME` into `*string`. pgx maps Postgres
`TIME` (OID 1083) to `pgtype.Time`, never to `string`. Every row fails.
Also affects `GetBranchCoverage` (`:159`) and the query at `:188`.

**Bug B — write.** `SaveWeeklyCoverage` (`:81`) and `UpdateWeeklyCoverage`
(`:97`) pass a Go `string` into the `TIME` column. A blank form field sends `""`
→ `invalid input syntax for type time: ""`.

### 1.1.2 Decide the representation, once

The domain wants a wall-clock time-of-day like `"09:00"`. Two options:

| | Change | Pros | Cons |
|---|---|---|---|
| **A** *(recommended)* | Keep `TIME` columns; change the Go fields to `*string` and convert through `pgtype.Time` in the repository | correct DB type; ordering and comparison work in SQL | conversion code in the repository |
| **B** | Migrate the columns to `TEXT` with a `CHECK (coverage_from ~ '^\d{2}:\d{2}$')` | trivial Go code | loses time semantics in SQL; a migration on a live column |

**Pick A.** Record the decision in `docs/modules/workflow.md`.

### 1.1.3 Implement

1. `domain.go`: `CoverageFrom *string`, `CoverageTo *string`. Update
   `Validate()` to check the `HH:MM` shape when non-nil, and to require
   `from < to` when both are set.
2. Repository — a helper pair, used by every read and write of these columns:
   ```go
   // timeOfDay converts an "HH:MM" form value to a pgtype.Time for a TIME
   // column. A blank field means "no window", which is NULL, not "".
   func timeOfDay(v *string) (pgtype.Time, error)
   func timeOfDayString(t pgtype.Time) *string
   ```
3. Apply to **all four** sites: `SaveWeeklyCoverage`, `UpdateWeeklyCoverage`,
   `GetBranchCoverage`, `ListCoverageForOrganization`, and the query at `:188`.
4. Handler (`coverage_handlers.go`): a blank form field must produce `nil`, not
   `""`.

### 1.1.4 Audit the same bug class everywhere

This is not a one-off. Find every other column/field type mismatch:

```bash
# every TIME / DATE / INTERVAL column in the schema
grep -rnE '\b(TIME|DATE|INTERVAL)\b' db/migrations/*.up.sql | grep -v TIMESTAMPTZ

# then, for each, find the Go field it scans into and check the type
```

Also re-check nullable columns scanned into non-pointer Go fields — the same
failure mode. `test/nullscan_test.go` exists; **extend it to cover
`weekly_coverages`**, which it currently does not.

### 1.1.5 Tests — through HTTP, not through the repository

The existing `test/integration/coverage_chain_test.go` seeds via raw SQL with
valid times, which is why it never caught this. Add:

| Test | Assertion |
|---|---|
| **D2a** | POST `/vendor/coverage` with **blank** times → row created, `coverage_from IS NULL` |
| **D2b** | POST with `09:00`/`17:00` → row created with those times |
| **D1** | GET `/vendor/coverage` → the page body contains the branch name and `09:00` |
| **D4** | invalid time (`"25:99"`) → inline Arabic validation error, **no** row created |
| **D3** | vendor B cannot create coverage on vendor A's branch |
| **CHAIN** | rewrite `coverage_chain_test.go` to create coverage **through the HTTP handler**, then assert an in-range customer sees the offer and an out-of-range one does not |

The CHAIN rewrite is mandatory. Seeding via SQL is how this bug survived.

### 1.1.6 Completion

- [ ] A vendor creates coverage through the UI, with and without a time window
- [ ] The coverage page lists it
- [ ] An in-range customer sees the offer; out-of-range does not
- [ ] `nullscan_test.go` covers `weekly_coverages`
- [ ] The TIME/DATE audit found and fixed every other instance
- [ ] All tests run with `DATABASE_URL` set — **zero skips**

---

## TASK 1.2 — Cart & order availability

**Audit §1.4.** Five defects in eleven lines, and no validation anywhere else.

### 1.2.1 Build one availability rule, in the domain

Every surface must call the same function. Put it in `commerce`, not in the
handler:

```go
// Availability answers "may this customer branch buy this quantity of this
// variant from this vendor, right now?" It is the single source of truth for
// stock, coverage and eligibility. Every surface — product page, cart add,
// quantity change, checkout — calls it.
type AvailabilityRequest struct {
    VariantID        int64
    VendorOrgID      int64
    CustomerOrgID    int64
    CustomerBranchID int64
    Quantity         int
    When             time.Time   // determines the coverage weekday
}

type AvailabilityResult struct {
    Allowed       bool
    MaxQuantity   int      // what the customer may actually order
    Reason        string   // machine key, e.g. "out_of_stock", "not_covered"
    MessageAr     string
    MessageEn     string
}

func (s *Service) CheckAvailability(ctx context.Context, req AvailabilityRequest) (AvailabilityResult, error)
```

### 1.2.2 The checks, in order

Each returns a distinct `Reason` so the UI can explain itself:

| # | Check | Fails with |
|---|---|---|
| 1 | `VendorOrgID > 0` and the org exists, is `type='vendor'`, `status='approved'` | `vendor_invalid` |
| 2 | The variant exists, is active, and belongs to `VendorOrgID` | `variant_invalid` |
| 3 | `StockQty >= Quantity` — **`StockQty == 0` fails**, it does not skip | `out_of_stock` |
| 4 | The vendor's weekly coverage reaches the customer branch's coordinates on `When`'s weekday — **reuse `workflow.CoverageService.ServesPoint`**, do not write a second distance calculation | `not_covered` |
| 5 | Institutional-work eligibility (the Phase 1 gate from PLAN_V5) permits this customer to see this vendor's products | `not_eligible` |
| 6 | The customer branch belongs to `CustomerOrgID` | `branch_invalid` |
| 7 | Offer-level limits where an offer is involved: `min_order_amount`, per-offer quantity caps, `starts_at`/`expires_at` | `offer_limit` |

**Never clamp silently.** If `Quantity > MaxQuantity`, return `Allowed: false`
with `MaxQuantity` set, and let the UI say *"المتاح لدى المورد: 3 فقط"*.

### 1.2.3 Delete the hardcoded vendor fallback

```go
if vendorOrgID <= 0 { vendorOrgID = 1 }   // DELETE THIS
```
A missing vendor is a validation error, not a default. Then check the database
for rows created by it:
```sql
SELECT COUNT(*) FROM commerce.cart_items WHERE vendor_org_id = 1;
```
If any exist and are not genuinely org 1's, they are corrupt. Record the count.

### 1.2.4 Apply it at every surface

| Surface | Handler | Required behaviour |
|---|---|---|
| Product detail | `CustomerProductDetailPage` | the `+` control's max comes from `CheckAvailability`; out-of-stock disables the add button with a reason |
| Add to cart | `AddToCartSubmit` | refuse with the Arabic reason; never clamp |
| Quantity change | `UpdateCartQuantitySubmit` | re-check on every increment — **server-side**; the client `+` is a hint, not the rule |
| Cart page | `CustomerCartPage` | re-check every line on render; flag lines that became unavailable |
| Checkout | `CheckoutSubmit` | re-check **all** lines inside the order transaction; a line that fails aborts with a clear message |
| Offer checkout | offer checkout handler | same, plus offer window and `min_order_amount` |

**Checkout must re-validate inside the same transaction that writes the order**,
or two customers race for the last unit.

### 1.2.5 Tests

| Test | Assertion |
|---|---|
| D1 | product page for a 3-stock variant shows max 3 |
| D2a | adding 5 of a 3-stock variant is **refused**; the cart is empty |
| D2b | adding 3 succeeds; `commerce.cart_items` has quantity 3 |
| D2c | adding a `StockQty == 0` variant is refused |
| D2d | quantity `+` past stock is refused server-side even if the client posts it |
| D3 | a customer outside the vendor's coverage cannot add — `not_covered` |
| D3b | a customer whose institutional works exclude the vendor cannot add — `not_eligible` |
| D4 | checkout of a cart whose stock dropped to 0 fails with a message; no order row |
| **RACE** | two concurrent checkouts for the last unit: exactly one succeeds |

The RACE test is the one that proves the transaction boundary is right.

### 1.2.6 Completion

- [ ] `CheckAvailability` is the only place these rules live
- [ ] All six surfaces call it
- [ ] No silent clamping anywhere; every refusal carries an Arabic reason
- [ ] `vendorOrgID = 1` is gone; existing corrupt rows counted and reported
- [ ] Checkout re-validates inside the order transaction
- [ ] RACE passes

---

## PHASE 1 GATE

```bash
DATABASE_URL="postgres://..." go test ./... -race
make check
```

- [ ] Vendor creates coverage through the UI → customer sees the offer (chain test, via HTTP)
- [ ] Cart refuses out-of-stock, out-of-coverage and ineligible items with distinct reasons
- [ ] Zero test skips in the runs above
- [ ] `PROGRESS.md` records the before/after counts (this phase may add lines; the
      next three must remove them)
