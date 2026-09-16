-- Migration 175 Down: readmit the "external_url" click target.
--
-- Rows rewritten to 'vendor_page' by the up migration are not restored: the
-- external URLs they pointed at were never validated and are not recoverable
-- from the target alone.
BEGIN;

ALTER TABLE promo.ads DROP CONSTRAINT IF EXISTS ads_click_target_type_check;
ALTER TABLE promo.ads
  ADD CONSTRAINT ads_click_target_type_check
  CHECK (click_target_type IN ('product', 'vendor_page', 'offer', 'external_url'));

COMMIT;
