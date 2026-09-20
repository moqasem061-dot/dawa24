BEGIN;

-- B-Tree index for fast exact-normalized Arabic name matching in catalog imports
CREATE INDEX IF NOT EXISTS idx_products_name_ar_norm_btree
ON catalog.products (organization_id, platform.normalize_arabic(name->>'ar'))
WHERE deleted_at IS NULL;

COMMIT;
