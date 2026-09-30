-- 230_catalog_revisions.up.sql
--
-- Tracks monotonic catalogue revision numbers to invalidate caches and trigger index rebuilds.
-- Includes statement-level trigger on catalog.products to increment revision on identity-relevant changes.

BEGIN;

CREATE TABLE IF NOT EXISTS catalog.catalog_revisions (
    scope       TEXT PRIMARY KEY,
    revision    BIGINT NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE catalog.catalog_revisions IS 'إصدارات كتالوج المنتجات لتتبع التغييرات وتحديث الفهارس';
COMMENT ON COLUMN catalog.catalog_revisions.scope IS 'نطاق الكتالوج (مثال: master)';
COMMENT ON COLUMN catalog.catalog_revisions.revision IS 'رقم الإصدار المتزايد مع كل تعديل على المنتجات';
COMMENT ON COLUMN catalog.catalog_revisions.updated_at IS 'تاريخ ووقت آخر تحديث للإصدار';

INSERT INTO catalog.catalog_revisions (scope, revision, updated_at)
VALUES ('master', 1, now())
ON CONFLICT (scope) DO NOTHING;

CREATE OR REPLACE FUNCTION catalog.bump_master_catalog_revision()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE catalog.catalog_revisions
    SET revision = revision + 1,
        updated_at = now()
    WHERE scope = 'master';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_catalog_products_revision_bump ON catalog.products;

CREATE TRIGGER trg_catalog_products_revision_bump
    AFTER INSERT OR DELETE OR UPDATE OF name, sku, barcode, dosage_form, scientific_name, active, concentration, unit, units, small_unit, volume, status, deleted_at
    ON catalog.products
    FOR EACH STATEMENT
    EXECUTE FUNCTION catalog.bump_master_catalog_revision();

COMMIT;
