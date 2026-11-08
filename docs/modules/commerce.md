# Module: Commerce

## Overview

The `commerce` bounded context handles shopping carts, checkout processing, immutable order line snapshots, multi-vendor shipment partitioning, and state machine validation.

## Schema Mapping

- **PostgreSQL Schemas:** `commerce`
- **Migrations:** `006_commerce.up.sql`, `086_order_rating.up.sql`
- **Tables Owned:**
  - `commerce.carts` & `commerce.cart_items` — Active customer cart state.
  - `commerce.orders` — Master customer order header with customer rating, review, and delivery timestamps.
  - `commerce.order_shipments` — Vendor-specific delivery partitions.
  - `commerce.order_lines` — Immutable historical snapshot of purchased items with per-line rating support.
  - `commerce.order_status_history` — Audit trail for status changes.

## Invariants & Rules

1. **Resolution of Defect D2:** The two separate legacy order systems (`main_orders` and `orders`/`adv_orders`) are unified into master `commerce.orders` with linked `commerce.order_shipments`.
2. **Exact Minor Unit Arithmetic:** All totals and line sums use `money.Amount`.
3. **Multi-Vendor Shipment Partitioning:** Checking out items from multiple vendors automatically generates separate `order_shipments` records.
4. **State Machine Validation:** Transitions between `pending`, `processing`, `confirmed`, `on_hold`, `shipped`, `in_transit`, `out_for_delivery`, `delivered`, `completed`, `cancelled`, `failed`, `returned`, and `refunded` are strictly validated before committing. The enum matches Laravel's `main_orders`/`adv_orders` exactly; the legacy `ready_for_pickup` was mapped to `out_for_delivery` in migration 063.
5. **Address Snapshot Invariant (Task 0.6.2):** `commerce.orders.user_address_id` explicitly references `identity.user_address_histories(id)` rather than the mutable `user_addresses(id)`. This ensures that if a user or organization updates their current address profile in the future, all historical order invoices, shipping manifests, and delivery receipts retain the exact, immutable address snapshot that was valid at the exact moment the order was placed.
6. **Order Rating & 3-Criteria Review Model (Task 1.3 / Audit §3.3):**
   - In Laravel, `adv_orders` held a per-line `rating` column while `orders` held `rating`, `review`, and `rated_at`.
   - Migration 086 adds `rating NUMERIC(3,2)`, `review TEXT`, `rated_at TIMESTAMPTZ`, and `delivered_at TIMESTAMPTZ` to `commerce.orders`, and `rating NUMERIC(3,2)` to `commerce.order_lines`.
   - The domain layer computes the exact 2-decimal scalar average of the three review criteria (`org.review_criteria`: representative rating, speed rating, quality rating) via `commerce.CalculateAverageRating`.
   - **Guards:** An order can only be rated if its status is `delivered` (or `completed`), and an order cannot be re-rated once `rated_at` is set.

## Supplier edits before acceptance (2026-09-27)

A supplier can change quantities or remove lines of its own shipment while
that shipment is `pending` (`POST /vendor/orders/{shipmentID}/edit`, permission
`vendor.order.update`). It shares the buyer edit's repository path
(`editPendingOrder`), so prices, discounts, stock, offer limits, quotas and
order/shipment totals are recomputed the same way. Differences from the buyer
edit: the supplier's shipment row is locked and must be `pending` (the order's
summary status may already reflect another supplier's acceptance); every line
id must belong to that shipment; the shipment cannot be emptied — a supplier
that can supply nothing rejects the order. Lines cannot be added. The wallet is
debited on acceptance, so it charges the edited total. The buyer is notified.
Standalone offer lines have no product or variant reference and are sold as one
unit; buyers and suppliers may remove the offer line but cannot change its
quantity. Product lines sold under an offer keep that offer's minimum and
maximum quantity rules in both edit forms and server validation.

Shipment organization rows are read and closed before per-vendor audit writes
start. pgx cannot issue another query on that transaction connection while
those rows remain open; leaving them open caused `conn busy` at commit.

## Product-detail cart repair (2026-10-12)

The purchase panel emitted `disabled="false"`, which HTML still treats as
disabled. Conditional boolean attributes now use templ's `?=` syntax for the
submit button, selected supplier and negotiation visibility. The page retains
the catalog's `/cart/add` handler, authoritative availability/pricing checks,
selected promotion, quantity limits, toast and `cartUpdated` event. A browser
regression test actually submits the purchase panel and verifies its payload.

## Percentages applied to money

Cost discounts and an edited order's tax are computed exactly: percentages are
converted to basis points (`money.PercentToBasisPoints`) and applied with
`money.Amount.ApplyPercent`; the edited-order tax keeps the original ratio
tax/taxable and applies it with `money.Amount.MulRatio`. Both round half away
from zero. No money value passes through `float64`.

## Endpoints

### Module API hardening (Batch 1A)

The REST checkout also applies two website rules to the buyer's own choices: the payment method must be active and enabled for checkout (none submitted means `cod`), and `user_address_id` must belong to the buyer. See `cmd/server/api_checkout_buyer.go`.

The REST checkout (`CheckoutAPI`) takes its prices, discounts, delivery charges,
tax, supplier and branch from the server: the catalog or approved offer, the
branch's delivery bands, and the availability and quota checks. The caller's
figures are only compared with that result. Any difference returns
`checkout.price_changed` (409) with the server amounts in `fields`, and no
order is written. The caller's `payment_status` is ignored; orders start
`unpaid`. The REST checkout refuses, for the whole request and without writing
an order:
- negotiations (`checkout.negotiation_on_website`);
- wallet payment (`checkout.wallet_on_website`), because the website's funds check
  does not run on this path;
- any line without a `product_variant_id`, such as a standalone bundle
  (`checkout.unverifiable_on_website`).

The website checkout is unchanged. Its `Checkout` path still accepts bundles,
negotiations and wallet payment, and it has no price review.
See [the checkout review backlog](../checkout-price-review-backlog.md) for the
separate website delivery-total and price-review work.

The order-status, cancel and shipment-status APIs require `vendor.order.update` for
suppliers and change only the requesting supplier's own shipment. A wallet-paid order can be
cancelled through these routes and the admin status route, and the cancel refunds the wallet
(Batch 1B, below). Every other status change on a wallet-paid order is refused
(`order.wallet_on_website`) until the wallet debit on confirmation is fixed (S02).

- `POST /api/v1/commerce/checkout` — Execute checkout and create order.
- `GET /api/v1/commerce/orders/{id}` — Retrieve order and shipment status.
- `GET /api/v1/commerce/orders` — List customer order history.
### Cancellation and wallet refunds (Batch 1B, S03)

One rule, `commerce.CheckCancellation`, decides every cancellation. An order can be cancelled only
while its status is pending, processing, confirmed or on hold, and no shipment is shipped, in
transit, out for delivery, delivered or completed. The website buyer cancel, the cancel and
status routes of the API, the admin status route (cancel) and a rejected negotiation all check it
under the order row lock. The website keeps its wording: a shipment in progress, or the order
status.

Locks: the order row is locked first, then its shipments. Every path that cancels, refunds or
moves a shipment takes the order lock first, so two transactions cannot wait on each other.

Refunds: a cancellation that leaves the order cancelled credits the wallet with the full net
debit. A supplier's partial cancel credits at most that shipment's goods, and never more than the
net debit. An admin refund (`/refund`, or status `refunded`) credits the net debit. The amount is
read from the wallet ledger (`billing.wallet_transactions`): the order's debits, less earlier order
credits and admin reversals. It is never the order total. The credit commits in the same
transaction as the status change, so a cancel never commits without its refund, and a refund never
commits without its cancel. Money credited sets `payment_status` to `refunded`, as the website
always did.

Once only: each cancellation event writes its credit with an idempotency key (`order-cancel:<id>`,
`order-refund:<id>`, `shipment-cancel:<order>:<shipment>`). A partial unique index (migration 247)
rejects a second credit with the same key. The order lock and the ledger cap also stop a second
credit.

Repeated or racing cancels: the second cancel of a cancelled order is refused, with the same 409 on
the API and the same message on the website. A cancel that races a supplier's shipment cancel
serialises on the order row.

Known limit (S02), closed by Batch 2 for orders confirmed after it. Orders debited before Batch 2
are listed by the owner queries (Q3 and Q4 in `reports/security-audit-2026-11-06/batch-2-owner-queries.sql`).

- `POST /api/v1/commerce/orders/{id}/status` — Apply a validated status transition.
- `POST /api/v1/commerce/orders/{id}/rate` — Rate a delivered order with 3 criteria or scalar score and review comment.
- `GET /api/v1/commerce/vendor/shipments` — List vendor shipment partitions.

## Purchase Requests (طلب الشراء — Plan V5 Task 3.1)

- **Migrations:** `089_purchase_requests.up.sql`
- **Tables Owned:**
  - `commerce.purchase_requests` — Formal multi-item procurement request header linking customer, organization, branch, and target supplier.
  - `commerce.purchase_request_lines` — Item lines capturing product ID, name, SKU, requested quantity, target price/discount, vendor counter-offered price/discount, status, and notes.
- **Institutional Work Filtering (Mode 2 — WithConnections):**
  - Customer product selection screens (`/customer/purchase-request/supplier/{id}` and `/customer/purchase-request/products`) strictly enforce `org.FilterWithConnections` (mode 2) without unrestricted fallback.
- **Lifecycle States:**
  - `pending` -> `approved` | `processing` | `completed` | `cancelled`
- **Web UI Routes:**
  - `GET /customer/purchase-request` (Wizard option selection)
  - `GET /customer/purchase-request/supplier` (Supplier directory with institutional filtering)
  - `GET /customer/purchase-request/supplier/{id}` & `GET /customer/suppliers/{id}` (Supplier catalog picker)
  - `GET /customer/purchase-request/products` (Cross-supplier catalog search)
  - `GET /customer/purchase-request/previous` (Submitted purchase request history & status tabs)
  - `POST /customer/purchase-request/submit` (Submit request)
  - `POST /customer/purchase-request/lines/{id}/edit` (Customer modify line price/discount)
  - `GET /vendor/purchase-requests` (Vendor incoming request list)
  - `GET /vendor/purchase-requests/{id}` (Vendor request detail)
  - `POST /vendor/purchase-requests/{id}/respond` (Vendor accept/modify/reject response)

### Wallet payments per supplier shipment (Batch 2, S02, S04)

- A supplier's confirmation of a wallet shipment charges that shipment to the buyer's wallet
  in the same transaction. The debit is written by `billing` (`DebitOrderShipment`) under the
  wallet lock. If the debit is refused, the shipment is not confirmed.
- Each shipment is charged once. `commerce.order_shipments.wallet_paid_at` is set in the same
  transaction as the debit, and the ledger key `order-shipment-debit:<order>:<shipment>` is unique.
  A repeated confirmation of a charged shipment does not charge again.
- The charge amount is the shipment's goods (`WalletGoodsAmount`), the same amount the
  website has always debited for a shipment.
- Order `payment_status` is `partially_paid` while some live shipments are uncharged and `paid`
  once all are charged. After refunds it is `partially_refunded` or `refunded`. Orders that
  have no per-shipment charge (cash on delivery, or wallet orders debited before Batch 2) keep
  their status rules.
- Refunds are per shipment. A supplier's cancel refunds only that shipment's open charge. A
  whole-order cancel refunds the sum of the open charges. Each credit is capped by the ledger.
- A wallet order debited before Batch 2 (no per-shipment charge on record): a whole-order cancel
  refunds the net debit as before. A supplier's partial cancel is refused with
  `order.wallet_review_required` until an administrator reviews it. It is never credited blind.
- Setting `refunded` on a wallet order (order or shipment) requires a real credit. With nothing to
  refund it is refused (`order.refund_nothing_to_refund`, `shipment.refund_nothing_to_refund`).
  An order that is already refunded stays refunded. Non-wallet orders are unchanged.
- `AcceptNegotiation` takes the accepting supplier's organisation. It refuses an order that is
  not in a pending negotiation or is closed (`order.negotiation_not_pending`,
  `order.negotiation_closed`), and a supplier that is not on the order (`order.negotiation_not_party`).
  It confirms and charges only that supplier's open shipments. Other suppliers' shipments are unchanged.
- Migration 248 adds `wallet_paid_amount`, `wallet_paid_at` and `wallet_refunded_amount` to
  `commerce.order_shipments`. They are internal and not part of the API.
- The composition root wires the payer with `WithWalletPayer` next to `WithWalletRefunder`. A wallet
  charge without a payer fails closed.

## Post-fix changes (2026-11-07, codex/post-fix-cleanup)

- **Negotiation, one supplier at a time (A10).** A supplier accepts only its own shipments. The
  order stays `negotiation_status = pending` until every supplier on it has accepted, and only
  then becomes `accepted` and `confirmed`. Negotiation orders from the website have one supplier.
- **Reject needs a pending negotiation.** `RejectNegotiation` refuses (`order.negotiation_not_pending`)
  an accepted or already rejected negotiation. It also no longer fails with `conn busy` when the
  order has a supplier: the supplier list is read before the audit rows are written (found while
  testing; rejecting any negotiation failed before this).
- **Admin confirmation of a wallet shipment (B2).** The admin shipment path runs in the shipment's
  own supplier tenant, resolves the buyer's wallet owner, and charges the buyer's wallet like a
  supplier's confirmation. A short balance gives a bilingual error. The bulk "all shipments"
  action reports how many shipments failed and no longer shows success when one failed.
- **Partial refund status (B3).** When one supplier's charge is refunded and another supplier's
  shipment is still open, the order is `partially_refunded`, not `refunded`.
- **Supplier earnings (A6).** A wallet order shows the supplier's own shipment state: paid once its
  shipment is charged (`wallet_paid_at`), otherwise unpaid.
- **Cancelled wallet order.** A cancelled order cannot be marked `refunded`: the state machine
  refuses it (`order.invalid_transition`). Earlier notes said the wallet check refused it with
  "no wallet payment to refund"; that message is for a non-cancelled wallet order with no debit.
- **REST API comments.** `ValidateAPIWalletOrder` refuses non-cancel status changes on wallet orders
  because the API cannot resolve the payer, not because confirmation debits were unreliable (C3).

## Order numbers from a sequence (ERP S7, migration 258)

Order numbers come from `commerce.order_number_seq` when the repository implements
`OrderNumberAllocator` (the Postgres repository does). Every server process draws from
the one sequence, so two processes cannot print the same number. The sequence starts
at the largest existing numeric order number plus 1000. Repositories without the
allocator keep the in-process counter. Numbers of 9 or more digits (the date format)
never collide with the sequence. Rolling back to the old image is safe: the old code
ignores the sequence and keeps numbering with its counter.
