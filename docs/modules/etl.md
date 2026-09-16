# Module: Legacy Data ETL

## Overview

The `etl` module drives the 6-stage migration pipeline (Extract, Validate, Transform, Load, Verify, Reconcile) migrating legacy MariaDB tables into modern partitioned PostgreSQL schema while preserving primary key values.

## Key Principles & Invariants

1. **Legacy Primary Key Preservation:** User 4417 remains User 4417 across all identity, commerce, and organization records.
2. **Explicit UTC Conversion:** Legacy MySQL dates with arbitrary local timezone storage or invalid `0000-00-00 00:00:00` values are safely normalized to UTC `TIMESTAMPTZ`.
3. **Exact Minor-Unit Conservation:** Monetary columns are mapped to integer minor units (`money.Amount`). Any reconciliation run with even a 1-cent discrepancy fails verification.
4. **Verification Gates:**
   - Exact row count matching between source and target.
   - Exact sum of monetary columns down to the cent.
   - Zero foreign key orphan records.
   - 100% JSON unmarshal success.
