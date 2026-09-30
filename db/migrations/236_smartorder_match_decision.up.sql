-- 236_smartorder_match_decision.up.sql
--
-- Adds product matching decision columns, audit fields, and offer_state to smartorder.run_lines.
-- Supports deterministic matching results, AI states, optimistic revision locking, and offer states.

BEGIN;

ALTER TABLE smartorder.run_lines
    ADD COLUMN IF NOT EXISTS match_state TEXT NULL
        CHECK (match_state IN ('accepted','confirmed','needs_review','ambiguous','no_candidate','conflict','invalid_input','unavailable','excluded')),
    ADD COLUMN IF NOT EXISTS match_basis TEXT NULL,
    ADD COLUMN IF NOT EXISTS match_granularity TEXT NULL,
    ADD COLUMN IF NOT EXISTS match_revision INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS human_locked BOOL NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ai_state TEXT NULL,
    ADD COLUMN IF NOT EXISTS match_run_id BIGINT NULL REFERENCES catalog.match_runs(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS match_decision JSONB NULL,
    ADD COLUMN IF NOT EXISTS baseline_decision JSONB NULL,
    ADD COLUMN IF NOT EXISTS offer_state TEXT NULL
        CHECK (offer_state IN ('offered','identified_no_offer','blocked_pack'));

ALTER TABLE smartorder.run_lines DROP CONSTRAINT IF EXISTS run_lines_outcome_check;
ALTER TABLE smartorder.run_lines ADD CONSTRAINT run_lines_outcome_check
    CHECK (outcome IN ('ordered','no_supplier','coverage_blocked','institutional_blocked','out_of_stock','below_min_qty','unmatched','zero_qty','removed','quota_blocked','blocked_pack'));

CREATE INDEX IF NOT EXISTS run_lines_match_state
    ON smartorder.run_lines (run_id, match_state);

CREATE INDEX IF NOT EXISTS run_lines_offer_state
    ON smartorder.run_lines (run_id, offer_state);

COMMENT ON COLUMN smartorder.run_lines.match_state IS 'حالة المطابقة الأساسية لبند الطلب الذكي';
COMMENT ON COLUMN smartorder.run_lines.match_basis IS 'مسار المطابقة المستخدم للبند';
COMMENT ON COLUMN smartorder.run_lines.match_granularity IS 'مستوى تفصيل المطابقة المتحقق: منتج عام أو عبوة محددة';
COMMENT ON COLUMN smartorder.run_lines.match_revision IS 'إصدار قرار المطابقة الحالي للقفل التفاؤلي';
COMMENT ON COLUMN smartorder.run_lines.human_locked IS 'هل تم قفل القرار بقرار بشري يمنع تعديل الذكاء الاصطناعي اللاحق';
COMMENT ON COLUMN smartorder.run_lines.ai_state IS 'حالة معالجة الذكاء الاصطناعي للبند';
COMMENT ON COLUMN smartorder.run_lines.match_run_id IS 'معرف عملية المطابقة المرتبطة في catalog.match_runs';
COMMENT ON COLUMN smartorder.run_lines.match_decision IS 'سجل تفاصيل قرار المطابقة الكامل بصيغة JSON v1';
COMMENT ON COLUMN smartorder.run_lines.baseline_decision IS 'قرار المطابقة الحسابي الأولي قبل أي تدخل للذكاء الاصطناعي';
COMMENT ON COLUMN smartorder.run_lines.offer_state IS 'حالة العرض والتوريد للبند: متوفر، محدد بلا عرض، أو محجوب لاختلاف العبوة';

COMMIT;
