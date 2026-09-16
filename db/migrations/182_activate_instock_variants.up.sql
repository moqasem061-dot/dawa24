-- Migration 182: Ensure product variants with active stock are set to active status

BEGIN;

UPDATE catalog.product_variants v
SET status = 'active', updated_at = NOW()
FROM inventory.stocks s
WHERE s.product_variant_id = v.id
  AND s.deleted_at IS NULL
  AND s.quantity > 0
  AND v.status = 'inactive'
  AND v.deleted_at IS NULL;

COMMIT;