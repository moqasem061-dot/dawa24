-- 075_review_criteria_and_ratings (down)
BEGIN;
DELETE FROM org.review_criteria WHERE key IN ('rep', 'speed', 'quality');
COMMIT;