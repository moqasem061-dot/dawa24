# Feature: Per-branch purchase quota (حصص الفروع)

A supplier caps how much of one of its variants any single **buying branch** may
ever purchase. Two branches of the same pharmacy chain each get the full
allowance, because the allocation is about where the stock physically lands.

Spans three modules; nothing about it lives in a fourth.

| Concern | Where |
| --- | --- |
| The cap | `catalog.product_variants.quota_limit` (nullable INTEGER) |
| What a branch has consumed | derived from `commerce.order_lines` + `commerce.orders` |
| The supplier's reset | `commerce.variant_branch_quota_releases` |
| The purchase gate | `commerce.Service.CheckAvailability` step 7 |
| The atomic gate | `enforceOrderQuotas`, inside `CreateOrder` / `UpdateCustomerPendingOrder` |
| The supplier's screen | `/vendor/quotas` |

## Invariants

1. **`quota_limit IS NULL` means no quota.** It is never `0`; a zero cap would be
   indistinguishable from "nobody may buy this", which `status = 'inactive'`
   already says. A CHECK constraint enforces `NULL OR > 0`, and
   `catalog.NormalizeQuotaLimit` folds every "no quota" spelling (nil, 0,
   negative, blank form field) onto `NULL` before a write.

2. **The quota is per (variant, buying branch).** Not per company, not per
   order. `commerce.orders.branch_id` is the branch it is charged to.

3. **Consumption is derived, never stored.** It is
   `SUM(order_lines.quantity)` over that branch's orders for that variant, minus
   nothing, where the order is not soft-deleted, its status is not one of
   `QuotaReleasingStatuses`, and it was placed after the last release.

   This is the load-bearing design decision. A stored counter would have to be
   moved by checkout, cancellation, a pharmacy editing a pending order, a
   return, a refund, a soft delete and two admin paths — every one a place the
   counter can drift from the orders it claims to describe, and the drift is
   invisible until a supplier is asked why a branch that cancelled everything
   still cannot buy. Summing the orders cannot drift, because the orders *are*
   the consumption.

4. **`QuotaReleasingStatuses` = cancelled, failed, returned, refunded.** Those
   four give the branch its units back. `delivered` and `completed` are
   deliberately absent: a completed order is the whole point of the quota.

5. **Releasing is a cut-off in time, not a credit.** One row per (variant,
   branch) carrying `released_at`; orders placed at or before it stop counting
   and the branch starts again from zero against the same cap. The row stores no
   quantity, so a release cannot be double-spent. `release_count` records how
   many times a pairing has been reset, which is what a supplier wants to see
   before doing it a third time. Undoing a release deletes the row, restoring
   the consumption it had forgiven.

6. **Releasing one branch never touches another.** Each branch has its own
   allowance, so there is nothing to hand over.

7. **A quota-carrying line needs a receiving branch.** An order with
   `branch_id IS NULL` cannot be attributed to a branch, so `enforceOrderQuotas`
   refuses it rather than letting an untracked purchase through a cap whose
   whole purpose is to be tracked.

8. **The gate and the report use one predicate.** `quotaCountsSQL` is shared by
   the sum behind `CheckAvailability`, the sum inside the order transaction, the
   supplier's report and its stat cards. A report with its own idea of which
   orders count is worse than no report: the supplier reads "3 of 10 used" while
   the buyer is refused at 4.

## Where it is enforced

Every buying surface funnels through `commerce.Service.CheckAvailability`, so
one check covers add-to-cart, the cart's quantity controls, the catalogue and
supplier-profile cards, offer storefronts, checkout revalidation and smart
ordering. On an allowed line, `MaxQuantity` is `min(stock, remaining quota)` —
this is what the quantity boxes use as their `max`, so the number a pharmacy is
offered is the number the server will accept.

None of those checks is atomic with the write. `enforceOrderQuotas` re-applies
the rule inside the order's own transaction, under
`pg_advisory_xact_lock(hashtextextended('dawa24:variant-branch-quota:<v>:<b>'))`,
taken in ascending variant order so two multi-line orders cannot deadlock. That
is the only place where "the branch may take this much" and "the lines are
written" are one act; a refusal there is a `Conflict`, because the caller did
nothing wrong.

The order editor passes the order being edited as `excludeOrderID`, so raising a
line from 2 to 3 is measured as 3 against the cap and not as 5 — an edit
replaces a commitment rather than adding to it.

## The supplier's screen

`/vendor/quotas`, two tabs over the same data:

- **استهلاك الفروع** — one row per (item, buying branch): the cap, what the
  branch has taken, what is left, how many orders, the last one, and whether the
  supplier has reset it. Carries the reset and undo-reset buttons.
- **الأصناف المقيّدة** — one row per restricted item: the cap, how many branches
  are consuming it, how many are at their limit. Carries "change quota" and
  "remove quota entirely".

`vendor.quota.view` opens the page; `vendor.quota.manage` is required for all
three POSTs, so a warehouse keeper can see who has taken what without being able
to hand out more.

The cap is also editable on the item itself — the edit dialog and both
create-item forms on `/vendor/products` carry a `quota_limit` field, blank
meaning no quota.

## Known sharp edge

`PUT /api/v1/catalog/products/{id}/variants/{variantId}` is a full-replacement
update: a body that omits `quota_limit` clears it, exactly as a body omitting
`sku` blanks the SKU. That is this endpoint's established contract for every
field and is left as it is, but it means an API client written before quotas
existed will lift them. The vendor UI does not have this problem — its form
carries the field, and `applyVariantEdit` leaves any key the form did not send
untouched.
