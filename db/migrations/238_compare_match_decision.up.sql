-- 238_compare_match_decision.up.sql
--
-- Adds product matching decision columns, audit fields, and group_basis to compare.file_rows.
-- Supports deterministic matching results, AI states, optimistic revision locking, and grouping bases.

BEGIN;

ALTER TABLE compare.file_rows
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
    ADD COLUMN IF NOT EXISTS group_basis TEXT NULL;

CREATE INDEX IF NOT EXISTS compare_file_rows_match_state
    ON compare.file_rows (file_id, match_state);

CREATE INDEX IF NOT EXISTS compare_file_rows_group_basis
    ON compare.file_rows (file_id, group_basis);

COMMENT ON COLUMN compare.file_rows.match_state IS 'حالة المطابقة الأساسية لصف ملف المقارنة';
COMMENT ON COLUMN compare.file_rows.match_basis IS 'مسار المطابقة المستخدم للصف';
COMMENT ON COLUMN compare.file_rows.match_granularity IS 'مستوى تفصيل المطابقة المتحقق: منتج عام أو عبوة محددة';
COMMENT ON COLUMN compare.file_rows.match_revision IS 'إصدار قرار المطابقة الحالي للقفل التفاؤلي';
COMMENT ON COLUMN compare.file_rows.human_locked IS 'هل تم قفل القرار بقرار بشري يمنع تعديل الذكاء الاصطناعي اللاحق';
COMMENT ON COLUMN compare.file_rows.ai_state IS 'حالة معالجة الذكاء الاصطناعي للصف';
COMMENT ON COLUMN compare.file_rows.match_run_id IS 'معرف عملية المطابقة المرتبطة في catalog.match_runs';
COMMENT ON COLUMN compare.file_rows.match_decision IS 'سجل تفاصيل قرار المطابقة الكامل بصيغة JSON v1';
COMMENT ON COLUMN compare.file_rows.baseline_decision IS 'قرار المطابقة الحسابي الأولي قبل أي تدخل للذكاء الاصطناعي';
COMMENT ON COLUMN compare.file_rows.group_basis IS 'أساس تجميع الصنف للمقارنة: مطابقة عبوة مركزية أو مكافأة مصدرية أو غير قابل للمقارنة';

COMMIT;
