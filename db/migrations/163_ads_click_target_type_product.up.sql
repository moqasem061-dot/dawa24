-- Migration 163: Update promo.ads check constraint to include 'product'

ALTER TABLE promo.ads DROP CONSTRAINT IF EXISTS ads_click_target_type_check;

ALTER TABLE promo.ads
    ADD CONSTRAINT ads_click_target_type_check
    CHECK (click_target_type IN ('product', 'vendor_page', 'offer', 'external_url'));

COMMENT ON COLUMN promo.ads.click_target_type IS 'وجهة النقر: صنف دوائي بالكتالوج، صفحة المورد، عرض معين، أو رابط خارجي';