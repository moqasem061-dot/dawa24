-- Migration 154: Down
-- Schema: catalog, commerce

ALTER TABLE commerce.order_lines
    DROP COLUMN IF EXISTS cost_discount_percentage,
    DROP COLUMN IF EXISTS cost_price;

ALTER TABLE catalog.product_variants
    DROP COLUMN IF EXISTS cost_discount_percentage;

