-- 073_hr_schema_fixes (down)
--
-- Remove the reference categories. The VARCHAR(64) widenings are NOT
-- reverted: the VARCHAR(32) defaults overflow by design (logically the
-- same choice 048's down made, which never reverted them either).

BEGIN;

DELETE FROM hr.job_categories
WHERE slug IN ('pharmacists', 'medical-reps', 'supply-chain', 'pharmacy-assistants');

COMMIT;