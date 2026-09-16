-- 132_smartorder_ai_enhance (down)
--
-- The stage constraint goes back to its original set. Events recorded under the
-- new names are rewritten to 'adjudicate' first, because dropping to a narrower
-- CHECK with rows that violate it would fail the migration and leave the schema
-- half-reverted.

BEGIN;

UPDATE smartorder.run_events SET stage = 'candidates' WHERE stage = 'initial_done';
UPDATE smartorder.run_events SET stage = 'adjudicate'  WHERE stage = 'ai_enhance';

ALTER TABLE smartorder.run_events DROP CONSTRAINT IF EXISTS run_events_stage_check;
ALTER TABLE smartorder.run_events ADD CONSTRAINT run_events_stage_check
    CHECK (stage IN ('parse','normalize','resolve','candidates',
                     'adjudicate','select','finalize'));

ALTER TABLE smartorder.runs
    DROP COLUMN IF EXISTS ai_lines_reviewed,
    DROP COLUMN IF EXISTS ai_lines_improved,
    DROP COLUMN IF EXISTS ai_cache_hits;

COMMIT;
