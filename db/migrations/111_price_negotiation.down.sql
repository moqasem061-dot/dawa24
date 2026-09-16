-- 111_price_negotiation down

BEGIN;

ALTER TABLE commerce.order_lines
    DROP COLUMN IF EXISTS proposed_unit_price,
    DROP COLUMN IF EXISTS is_negotiated;

ALTER TABLE commerce.orders
    DROP COLUMN IF EXISTS negotiation_notes,
    DROP COLUMN IF EXISTS negotiation_status,
    DROP COLUMN IF EXISTS is_negotiation;

ALTER TABLE catalog.product_variants
    DROP COLUMN IF EXISTS is_negotiable;

COMMIT;
