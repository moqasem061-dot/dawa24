-- 132_smartorder_ai_enhance (up)
--
-- The AI stage of smart ordering stopped being an adjudicator and became an
-- enhancer, and the record of what it did has to say so.
--
-- Two changes.
--
-- 1. run_events gains two stage names. The pipeline now reports the moment the
--    deterministic engine finishes ('initial_done') separately from the AI stage
--    that follows it ('ai_enhance'), because a buyer watching a run must be able
--    to see that the ordinary matching is complete and something else is still
--    working. 'adjudicate' stays permitted so that runs recorded before this
--    migration still render.
--
-- 2. runs gains the three counters that answer the only question a buyer has
--    about this feature: how many lines were sent for a second opinion, how many
--    came back better, and how many were answered from memory rather than paid
--    for. ai_lines_adjudicated is kept and now carries "lines the model actually
--    answered", so historic rows keep their meaning.

BEGIN;

ALTER TABLE smartorder.run_events DROP CONSTRAINT IF EXISTS run_events_stage_check;
ALTER TABLE smartorder.run_events ADD CONSTRAINT run_events_stage_check
    CHECK (stage IN ('parse','normalize','resolve','candidates','initial_done',
                     'adjudicate','ai_enhance','select','finalize'));

ALTER TABLE smartorder.runs
    ADD COLUMN IF NOT EXISTS ai_lines_reviewed INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ai_lines_improved INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ai_cache_hits     INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN smartorder.runs.ai_lines_reviewed IS 'عدد الأصناف المرسلة للتحسين بالذكاء الاصطناعي بعد استبعاد ما في الذاكرة';
COMMENT ON COLUMN smartorder.runs.ai_lines_improved IS 'عدد الأصناف التي تغيّرت مطابقتها فعلياً بفضل الذكاء الاصطناعي';
COMMENT ON COLUMN smartorder.runs.ai_cache_hits     IS 'عدد الأصناف التي أُجيبت من ذاكرة القرارات دون تكلفة';

COMMIT;
