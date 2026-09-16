-- 147_sponsorship_and_ads (down)
--
-- Reverts the sponsorship packages, requests, and enhanced ads migration.
-- Drop new tables and columns added by 147.

BEGIN;

DROP TABLE IF EXISTS promo.ad_impressions CASCADE;

-- Drop extended columns from offer_sponsorships.
ALTER TABLE promo.offer_sponsorships
    DROP COLUMN IF EXISTS item_type,
    DROP COLUMN IF EXISTS item_id,
    DROP COLUMN IF EXISTS credits_used,
    DROP COLUMN IF EXISTS admin_status,
    DROP COLUMN IF EXISTS admin_notes,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS sponsorship_request_id;

-- Drop extended columns from ads.
ALTER TABLE promo.ads
    DROP COLUMN IF EXISTS ad_text_ar,
    DROP COLUMN IF EXISTS ad_text_en,
    DROP COLUMN IF EXISTS media_type,
    DROP COLUMN IF EXISTS media_url,
    DROP COLUMN IF EXISTS thumbnail_url,
    DROP COLUMN IF EXISTS click_target_type,
    DROP COLUMN IF EXISTS click_target_id,
    DROP COLUMN IF EXISTS admin_status,
    DROP COLUMN IF EXISTS admin_notes,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS ad_plan_id,
    DROP COLUMN IF EXISTS duration_days,
    DROP COLUMN IF EXISTS title_ar,
    DROP COLUMN IF EXISTS title_en;

-- Drop sponsorship requests and purchases.
DROP TABLE IF EXISTS promo.sponsorship_requests CASCADE;
DROP TABLE IF EXISTS promo.sponsorship_purchases CASCADE;

-- Drop extended columns from offer_packages.
ALTER TABLE promo.offer_packages
    DROP COLUMN IF EXISTS tier_level,
    DROP COLUMN IF EXISTS credits,
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS is_featured,
    DROP COLUMN IF EXISTS badge_color,
    DROP COLUMN IF EXISTS description;

DROP INDEX IF EXISTS idx_offer_packages_tier;

COMMIT;
