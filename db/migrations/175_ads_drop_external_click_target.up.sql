-- Migration 175: retire the "external_url" advertisement click target.
--
-- An external target sent a pharmacy to a page nobody on this platform
-- moderates, and nothing validated the URL it carried: promo.ads.target_url was
-- written straight from the vendor's form and Ad.ResolveClickURL preferred it
-- over the click target, so a posted URL won outright. Ads now point inside the
-- platform only, and the path is derived from the target rather than submitted.
--
-- Expand/contract: the rewrite runs first so the currently deployed binary
-- cannot leave a row the new constraint would reject, and the constraint is
-- narrowed only afterwards.
BEGIN;

UPDATE promo.ads
   SET click_target_type = 'vendor_page'
 WHERE click_target_type = 'external_url';

-- An unreviewed edit request can carry the retired value in its JSONB payload,
-- where no CHECK constraint reaches. Strip the key so approving that request
-- cannot reintroduce it.
UPDATE promo.ads
   SET pending_changes = pending_changes - 'click_target_type'
 WHERE pending_changes IS NOT NULL
   AND pending_changes->>'click_target_type' = 'external_url';

ALTER TABLE promo.ads DROP CONSTRAINT IF EXISTS ads_click_target_type_check;
ALTER TABLE promo.ads
  ADD CONSTRAINT ads_click_target_type_check
  CHECK (click_target_type IN ('product', 'vendor_page', 'offer'));

COMMENT ON COLUMN promo.ads.click_target_type IS 'وجهة النقر على الإعلان داخل المنصة: صنف، صفحة مورد، أو عرض ترويجي';

COMMIT;
