-- 062_offer_commerce (up)
--
-- Rebuild V2 §3.1: offers become the merchandising unit Laravel sells.
--   * promo.offers gains the vendor branch it belongs to, the admin lifecycle
--     (approval), and the minimum order amount.
--   * promo.offer_products gains per-line custom pricing, discounts and
--     quantity rules that EffectivePrice() resolves.
--
-- Money columns follow the project rule (AGENTS.md): NUMERIC(p,2), scanned
-- into money.Amount. The plan's "minor units BIGINT" sketch is deliberately
-- applied as NUMERIC(10,2) so existing price handling stays coherent.

BEGIN;

ALTER TABLE promo.offers
    ADD COLUMN IF NOT EXISTS branch_id        BIGINT REFERENCES org.branches(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS admin_status     TEXT NOT NULL DEFAULT 'pending'
        CHECK (admin_status IN ('pending','approved','rejected')),
    ADD COLUMN IF NOT EXISTS admin_notes      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS approved_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_by      BIGINT REFERENCES identity.users(id),
    ADD COLUMN IF NOT EXISTS rejected_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS rejected_by      BIGINT REFERENCES identity.users(id),
    ADD COLUMN IF NOT EXISTS min_order_amount NUMERIC(10,2) NOT NULL DEFAULT 0.00;

-- Legacy surface is preserved: offers that existed before 062 were visible on
-- the storefront, so they are backfilled as approved. New offers start pending
-- until the platform approves them (AGENTS.md rule 7).
UPDATE promo.offers SET admin_status = 'approved' WHERE admin_status = 'pending' AND created_at < now();

CREATE INDEX IF NOT EXISTS offers_branch_idx        ON promo.offers (branch_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS offers_admin_status_idx  ON promo.offers (admin_status, is_active) WHERE deleted_at IS NULL;

ALTER TABLE promo.offer_products
    ADD COLUMN IF NOT EXISTS variant_id                 BIGINT REFERENCES catalog.product_variants(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS custom_price               NUMERIC(10,2),
    ADD COLUMN IF NOT EXISTS custom_discount_percentage NUMERIC(5,2),
    ADD COLUMN IF NOT EXISTS custom_discount_amount     NUMERIC(10,2),
    ADD COLUMN IF NOT EXISTS custom_qty                 INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS max_qty_per_order          INT;

COMMENT ON COLUMN promo.offers.min_order_amount IS 'أقل قيمة للطلب بالجنيه — minimum order amount';
COMMENT ON COLUMN promo.offers.branch_id IS 'فرع المورد المُقدِّم للعرض — the vendor branch publishing the offer';
COMMENT ON COLUMN promo.offer_products.custom_price IS 'سعر مخصص للوحدة داخل العرض — overrides the variant list price completely';
COMMENT ON COLUMN promo.offer_products.custom_discount_percentage IS 'نسبة خصم مخصصة على السطر — applied to the list price when no fixed amount is set';
COMMENT ON COLUMN promo.offer_products.custom_discount_amount IS 'خصم ثابت مخصص على السطر بالجنيه';
COMMENT ON COLUMN promo.offer_products.custom_qty IS 'الكمية المعروضة (أقل كمية للشراء داخل العرض)';
COMMENT ON COLUMN promo.offer_products.max_qty_per_order IS 'الحد الأقصى للكمية لكل طلب — NULL يعني بلا حد';

COMMIT;