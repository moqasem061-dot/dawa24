-- 098_brand_categories (up)
--
-- Products carry both category_id and brand_id, but nothing constrained them to
-- agree, so a product could sit in "مستحضرات تجميل" with a brand that only makes
-- medical supplies. This adds the missing relationship so the product form can
-- offer a category first and then only that category's manufacturers.
--
-- DELIBERATE DEVIATION FROM LARAVEL: the legacy `brands` table has no
-- category link either (id, name, description, image, status, timestamps). This
-- is new structure, chosen because the segments the platform sells — أدوية,
-- مستحضرات تجميل, مستلزمات طبية — are routinely spanned by one manufacturer,
-- which a single category_id column on brands could not express.
--
-- PLAN_V7 Phase 4 Tasks 4.1-4.2.

BEGIN;

CREATE TABLE catalog.brand_categories (
    brand_id    BIGINT NOT NULL REFERENCES catalog.brands(id)     ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES catalog.categories(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (brand_id, category_id)
);

CREATE INDEX brand_categories_category_idx ON catalog.brand_categories (category_id);

COMMENT ON TABLE catalog.brand_categories IS
    'ربط الشركات المصنعة بالتصنيفات — تحدد أي شركات تظهر عند اختيار تصنيف';
COMMENT ON COLUMN catalog.brand_categories.brand_id IS 'الشركة المصنعة';
COMMENT ON COLUMN catalog.brand_categories.category_id IS 'التصنيف الذي تعمل فيه الشركة';

-- Backfill from what products already assert, so nothing disappears from a
-- brand selector the moment the filter goes live.
INSERT INTO catalog.brand_categories (brand_id, category_id)
SELECT DISTINCT p.brand_id, p.category_id
FROM catalog.products p
WHERE p.brand_id IS NOT NULL
  AND p.category_id IS NOT NULL
  AND p.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- A brand with no product yet would otherwise be invisible in every category.
-- Link those to every root category rather than stranding them.
INSERT INTO catalog.brand_categories (brand_id, category_id)
SELECT b.id, c.id
FROM catalog.brands b
CROSS JOIN catalog.categories c
WHERE b.deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND c.parent_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM catalog.brand_categories bc WHERE bc.brand_id = b.id)
ON CONFLICT DO NOTHING;

-- Reference data, like categories and brands themselves: no RLS. The catalog is
-- shared across tenants by design — a pharmacy browsing suppliers must see it.

COMMIT;
