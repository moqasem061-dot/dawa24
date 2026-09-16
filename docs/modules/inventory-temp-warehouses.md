# Temporary Warehouses (المستودعات المؤقتة) Subsystem

## Overview
The Temporary Warehouses subsystem allows pharmacy customers and vendor suppliers to upload, manage, and subscribe to ephemeral product price lists and inventory batches without polluting the primary permanent catalog.

## Architecture & Lifecycles

### 1. Ownership & Tenancy
- **Customer / Pharmacy Uploads**: `organization_id IS NULL` — personal inventory/price comparisons.
- **Vendor / Supplier Uploads**: `organization_id IS NOT NULL` — vendor stock staging and branch distribution.

### 2. Hierarchical Grouping (Father Temporary Warehouses)
- A "Father" warehouse (`father_user_temparte_warehouses`) groups multiple chunked uploads or related spreadsheets under a single parent entity.

### 3. Automated Lifecycle & Archiving
- Managed by `WarehouseLifecycleService` (daily cron/worker):
  - **Auto-Archive**: Uploads older than `warehouse_auto_archive_days` (default 30 days) are moved to `archived` status.
  - **Retention & Purging**: Archived warehouses older than `archived_warehouse_retention_days` (default 60 days) are purged.
  - **Quota Limits**: When a user exceeds `max_archived_warehouses_count`, oldest archives are evicted.

### 4. Subscription Plans (`plan_temparte_warehouses`)
- Defines capacity tiers (max warehouses, max rows, retention period).
- `user_plan_temparte_warehouses` tracks active user plan subscriptions, expiry dates, and payment confirmations.

## Admin Management Screens
- `/admin/user/temparte-warehouses`: Oversight of all temporary warehouses across customers and vendors.
- `/admin/my/temparte-warehouses`: Administrator-uploaded temporary warehouses.
- `/admin/import/temparte-warehouses`: Bulk spreadsheet uploader for temp warehouse staging.
- `/admin/admins/temparte-warehouses`: Staff-managed temp warehouses.
- `/admin/plan/temparte-warehouses`: Plans catalogue and subscription management.
- `/admin/saving-products`: Saving products (منتجات التوفير) directory with org and user filters.
