-- Migration 163 Down: Revert promo.ads check constraint

ALTER TABLE promo.ads DROP CONSTRAINT IF EXISTS ads_click_target_type_check;

ALTER TABLE promo.ads
    ADD CONSTRAINT ads_click_target_type_check
    CHECK (click_target_type IN ('vendor_page', 'offer', 'external_url'));