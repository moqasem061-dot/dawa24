-- Reverses migration 189.
--
-- Dropping quota_limit removes every supplier's cap, so the down path is a
-- deliberate "this feature is gone", not a rollback that keeps the data.

BEGIN;

DROP TABLE IF EXISTS commerce.variant_branch_quota_releases;

DROP INDEX IF EXISTS commerce.order_lines_variant_quota_idx;
DROP INDEX IF EXISTS commerce.order_lines_org_variant_idx;
DROP INDEX IF EXISTS catalog.product_variants_quota_idx;

ALTER TABLE catalog.product_variants
    DROP CONSTRAINT IF EXISTS product_variants_quota_limit_chk;

ALTER TABLE catalog.product_variants
    DROP COLUMN IF EXISTS quota_limit;

COMMIT;
