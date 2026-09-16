-- 071_merge_product_mappings (up)
--
-- Rebuild V2 §2.5 — product_clients, customer_product_mappings and
-- saving_products collapse into ONE catalog.customer_product_mappings:
--
--   * the live vendor→customer pricing rows (custom_price, discount_bps)
--     are renamed to the canonical price / discount (percent, 2dp);
--   * product_clients rows (customer's own name + price/discount + status)
--     fold in with customer_org_id NULL, raw_name = name, source 'manual';
--   * raw_name / branch_id / source / status columns are added so the
--     Laravel ETL can land legacy raw-name rows into the same table;
--   * saving_products (bundle qty + discount) has no row-level counterpart in
--     the mapping model and held no data — its semantics are superseded by
--     promo offers; the table is dropped (rows cannot be restored in down).
--
-- product_clients.user_id is deliberately not carried (no consumer; the V2
-- shape has no user column) — see down.

BEGIN;

-- 1. Expand the target table.
ALTER TABLE catalog.customer_product_mappings
    ADD COLUMN IF NOT EXISTS raw_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS branch_id BIGINT REFERENCES org.branches(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS price NUMERIC(12,2),
    ADD COLUMN IF NOT EXISTS discount NUMERIC(5,2),
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual'
        CHECK (source IN ('excel','csv','link','manual')),
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'processed'
        CHECK (status IN ('pending','processed','rejected'));

COMMENT ON COLUMN catalog.customer_product_mappings.raw_name IS
  'اسم المنتج لدى العميل كما ورد في ملفه (071)';
COMMENT ON COLUMN catalog.customer_product_mappings.branch_id IS
  'فرع العميل المرتبط بالتعيين (071)';
COMMENT ON COLUMN catalog.customer_product_mappings.price IS
  'السعر المخصص للعميل — NUMERIC(12,2)';
COMMENT ON COLUMN catalog.customer_product_mappings.discount IS
  'الخصم المخصص للعميل كنسبة مئوية — NUMERIC(5,2)';
COMMENT ON COLUMN catalog.customer_product_mappings.source IS
  'مصدر التعيين — excel/csv/link/manual (071)';
COMMENT ON COLUMN catalog.customer_product_mappings.status IS
  'حالة التعيين — pending/processed/rejected (071)';

-- 2. Fold product_clients rows (they never wrote custom_price/discount_bps).
INSERT INTO catalog.customer_product_mappings (
    organization_id, customer_org_id, product_id, raw_name, price, discount,
    source, status, is_active, created_at, updated_at
)
SELECT p.organization_id, NULL, p.product_id, p.name, p.price, p.discount,
       'manual', p.status, true, p.created_at, p.updated_at
FROM catalog.product_clients p
WHERE p.deleted_at IS NULL;

-- 3. Rename the live pricing columns (custom_price → price, bps → percent).
UPDATE catalog.customer_product_mappings
SET price    = custom_price,
    discount = ROUND(discount_bps::numeric / 100.0, 2)
WHERE custom_price IS NOT NULL;

ALTER TABLE catalog.customer_product_mappings
    DROP COLUMN IF EXISTS custom_price,
    DROP COLUMN IF EXISTS discount_bps;

-- 4. Lookup indexes for import matching.
CREATE INDEX IF NOT EXISTS customer_product_mappings_lookup_idx
    ON catalog.customer_product_mappings (customer_org_id, product_id);

CREATE INDEX IF NOT EXISTS customer_product_mappings_raw_name_idx
    ON catalog.customer_product_mappings (organization_id, raw_name)
    WHERE raw_name <> '';

-- 5. Drop the duplicated sources.
DROP TABLE catalog.product_clients;
DROP TABLE catalog.saving_products;

COMMIT;
