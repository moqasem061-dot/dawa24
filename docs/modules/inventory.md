# Module: Inventory & Warehouses

## Overview

The `inventory` bounded context provides warehouse facilities, stock balance tracking, double-entry stock movement ledgering, and inter-warehouse transfers.

## Schema Mapping

- **PostgreSQL Schemas:** `inventory`
- **Migrations:** `005_inventory.up.sql`
- **Tables Owned:**
  - `inventory.warehouses` — Physical/logical storage locations.
  - `inventory.stocks` — Real-time stock counts. **Fixes legacy D4 defect:** `UNIQUE (warehouse_id, product_variant_id)`.
  - `inventory.stock_movements` — Append-only immutable movement ledger.
  - `inventory.warehouse_transfers` — Inter-facility transfer logs and state machine.
  - `inventory.temp_warehouses` — Staging warehouses for batch imports.

## Invariants & Rules

1. **Legacy D4 Defect Resolution:** The legacy database had `UNIQUE (product_id, warehouse_id)` while `product_childern_id` was NOT NULL, which made it impossible for two variants of the same product to exist in the same warehouse. Dawa24 Store scopes uniqueness strictly to `UNIQUE (warehouse_id, product_variant_id)`.
2. **Immutable Stock Ledger:** No stock quantity is ever modified without an atomic corresponding row inserted into `inventory.stock_movements`.
3. **Non-Negative Balance:** Stock adjustments that would drop inventory below zero are rejected with `422 Unprocessable Entity` (`stock.insufficient`).
4. **Row Level Security:** Enforced with `FORCE ROW LEVEL SECURITY` across all tenant-owned inventory tables.

## Endpoints

- `GET /api/v1/inventory/warehouses` — List tenant warehouses.
- `POST /api/v1/inventory/warehouses` — Create a new warehouse.
- `GET /api/v1/inventory/warehouses/{id}/stocks` — List current stock balances.
- `POST /api/v1/inventory/stocks/adjust` — Atomically adjust stock with ledger recording.
- `POST /api/v1/inventory/transfers` — Transfer stock between warehouses.
- `GET /api/v1/inventory/stocks/{id}/movements` — Inspect audit trail and movements ledger.
