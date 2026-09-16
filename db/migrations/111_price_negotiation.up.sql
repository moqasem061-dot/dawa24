-- 111_price_negotiation
--
-- Enables price negotiation for product variants and captures negotiated orders and line items.

BEGIN;

-- 1. Add is_negotiable flag to catalog.product_variants
ALTER TABLE catalog.product_variants
    ADD COLUMN IF NOT EXISTS is_negotiable BOOLEAN NOT NULL DEFAULT false;

-- 2. Add negotiation metadata to commerce.orders
ALTER TABLE commerce.orders
    ADD COLUMN IF NOT EXISTS is_negotiation BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS negotiation_status TEXT NOT NULL DEFAULT 'none'
        CHECK (negotiation_status IN ('none', 'pending', 'accepted', 'rejected')),
    ADD COLUMN IF NOT EXISTS negotiation_notes TEXT;

-- 3. Add negotiation metadata to commerce.order_lines
ALTER TABLE commerce.order_lines
    ADD COLUMN IF NOT EXISTS is_negotiated BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS proposed_unit_price NUMERIC(12,2);

COMMIT;
