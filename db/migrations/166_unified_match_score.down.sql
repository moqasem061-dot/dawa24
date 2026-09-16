-- 166_unified_match_score.down.sql

BEGIN;

ALTER TABLE smartorder.criteria_profiles DROP COLUMN IF EXISTS min_match_score;
ALTER TABLE smartorder.run_config DROP COLUMN IF EXISTS min_match_score;

COMMIT;
