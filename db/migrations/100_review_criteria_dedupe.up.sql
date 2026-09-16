-- 100_review_criteria_dedupe (up)
--
-- Migration 053 seeded six supplier review criteria; migration 075 then seeded
-- three more (rep / speed / quality) with ON CONFLICT (key) DO UPDATE, which
-- only touches its own keys. Nothing deactivated the earlier six, so the
-- supplier context carries NINE active criteria in the live database.
--
-- The product requires exactly three, and the review modal hardcodes them as a
-- fallback: تقييم المندوب, تقييم السرعة, تقييم التعامل والجودة. Any code path
-- that reads the criteria from the database instead of using that fallback
-- would render nine star inputs.
--
-- The six are deactivated, not deleted: org.review_ratings.criterion is a
-- foreign key onto review_criteria(key) with ON DELETE CASCADE, so removing
-- them would silently destroy any rating already recorded against them.
--
-- Verified against the live database on 2026-08-20: 15 rows total, 9 of them
-- in the supplier context.

BEGIN;

UPDATE org.review_criteria
   SET is_active = false
 WHERE context = 'supplier'
   AND key IN ('delivery_speed', 'product_quality', 'rep_service',
               'price_fairness', 'order_accuracy', 'packaging');

COMMIT;
