-- 097_drop_unused_tables.up.sql
-- Drop speculative and unused legacy tables per PLAN_V6 Task C.1.2 & C.14-C.16.
-- catalog.product_infos: Legacy 5-column key-value attribute bag superseded by catalog.product_index.

BEGIN;

DROP TABLE IF EXISTS catalog.product_infos CASCADE;

COMMIT;
