-- 100_review_criteria_dedupe (down)
--
-- Restores the six superseded supplier criteria to active. This reinstates the
-- nine-criteria state, which is why 100 exists in the first place.

BEGIN;

UPDATE org.review_criteria
   SET is_active = true
 WHERE context = 'supplier'
   AND key IN ('delivery_speed', 'product_quality', 'rep_service',
               'price_fairness', 'order_accuracy', 'packaging');

COMMIT;
