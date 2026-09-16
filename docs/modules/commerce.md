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

## Endpoints

- `POST /api/v1/commerce/checkout` — Execute checkout and create order.
- `GET /api/v1/commerce/orders/{id}` — Retrieve order and shipment status.
- `GET /api/v1/commerce/orders` — List customer order history.
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

