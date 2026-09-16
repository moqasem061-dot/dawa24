-- Smart Order System: additional indexes for deterministic matching.
--
-- The exact-name tier queries catalog.products by normalized Arabic and English
-- names. Without these btree indexes, every query scans the whole catalogue;
-- with them, the resolve pass costs one index lookup per name regardless of
-- catalogue size.

-- Exact match on normalized Arabic name.
CREATE INDEX IF NOT EXISTS idx_products_name_ar_norm_btree
ON catalog.products (platform.normalize_arabic(lower(trim(name->>'ar'))))
WHERE deleted_at IS NULL;

-- Exact match on lowercased English name.
CREATE INDEX IF NOT EXISTS idx_products_name_en_lower_btree
ON catalog.products (lower(trim(name->>'en')))
WHERE deleted_at IS NULL;

-- Saving-products lookup by normalized name, used by the saving-product tier.
CREATE INDEX IF NOT EXISTS idx_saving_products_norm_name
ON catalog.saving_products (platform.normalize_arabic(lower(trim(name_product))))
WHERE deleted_at IS NULL AND product_id IS NOT NULL;

-- Customer product mappings lookup by normalized raw name.
CREATE INDEX IF NOT EXISTS idx_customer_mappings_norm_name
ON catalog.customer_product_mappings (platform.normalize_arabic(lower(trim(raw_name))))
WHERE product_id IS NOT NULL AND is_active;

-- Language preference for matching (e.g. 'ar', 'en', or '' for auto/both).
ALTER TABLE smartorder.run_config ADD COLUMN IF NOT EXISTS match_language VARCHAR(10) NOT NULL DEFAULT '';
ALTER TABLE smartorder.criteria_profiles ADD COLUMN IF NOT EXISTS match_language VARCHAR(10) NOT NULL DEFAULT '';

