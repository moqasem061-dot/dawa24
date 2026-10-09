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

## Matched temporary warehouses and market search (2026-10-04)

Temporary warehouse imports, remapping and reprocessing resolve every extracted
product row through the shared catalog matching service before publishing it.
Accepted/confirmed decisions link to the catalog; ambiguous or unmatched rows
retain the file name without inventing an association. Canonical decisions and
run identifiers persist with the rows. Prices use exact decimal parsing.

Row replacement and the ready state commit in one transaction. A concurrent
rename, remap, archive or deletion rejects stale replacement. A failure preserves
previous rows, and a late background failure cannot overwrite completed work.
Stored uploads are located by their exact storage key, including the configured
temporary warehouse directory; a missing source never selects another workbook.

Market search uses public catalog matching candidates (no private organization
aliases), a short candidate cache, a 500 ms context deadline, and normalized
indexed text fallback. All query words must match a file or catalog name, in any
order; SQL wildcards are literal input. Relevance is the default sorting mode.
List and grid cards lead with the localized accepted catalog name and show the
original supplier text beneath it. The source remains ready, active temporary
warehouses only, never private comparison uploads.

Resumable sessions require the uploader's identity and organization. Mapping
requests must name a file belonging to that session; failed processing cannot
report successful mapping or commit. These rules apply through the shared admin,
my, team, user, plan and legacy warehouse route handlers.

Migration 243 adds the raw-name/catalog-name trigram indexes and catalog-link
index. Existing stored warehouses gain decisions when reprocessed/remapped;
this change does not silently rematch production history during migration.

## Upload processing and discount precision (2026-10-09)

All admin temporary-warehouse upload variants share the literal discount parser
and raw XLSX cell reader used by comparison imports. Invalid percentages above
100 are no longer clamped to 100. Parsing deadlines begin afresh after waiting
for a background worker slot, so earlier files cannot consume a later file's
processing budget. Column previews stop after a bounded sample; file security
validation still inspects the workbook before decoding it.
