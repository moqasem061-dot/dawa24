-- 237_catalog_staging_match_decision.up.sql
--
-- Adds product matching decision columns, audit fields, and proposal to catalog.import_staging_rows.
-- Supports deterministic matching results, AI states, optimistic revision locking, and catalog mutation proposals.

BEGIN;

ALTER TABLE catalog.import_staging_rows
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
    ADD COLUMN IF NOT EXISTS proposal TEXT NULL
        CHECK (proposal IN ('update','possible_new','duplicate','unresolved'));

CREATE INDEX IF NOT EXISTS catalog_import_staging_rows_match_state
    ON catalog.import_staging_rows (session_id, match_state);

CREATE INDEX IF NOT EXISTS catalog_import_staging_rows_proposal
    ON catalog.import_staging_rows (session_id, proposal);

COMMENT ON COLUMN catalog.import_staging_rows.match_state IS 'حالة المطابقة الأساسية لصف استيراد الكتالوج';
COMMENT ON COLUMN catalog.import_staging_rows.match_basis IS 'مسار المطابقة المستخدم للصف';
COMMENT ON COLUMN catalog.import_staging_rows.match_granularity IS 'مستوى تفصيل المطابقة المتحقق: منتج عام أو عبوة محددة';
COMMENT ON COLUMN catalog.import_staging_rows.match_revision IS 'إصدار قرار المطابقة الحالي للقفل التفاؤلي';
COMMENT ON COLUMN catalog.import_staging_rows.human_locked IS 'هل تم قفل القرار بقرار بشري يمنع تعديل الذكاء الاصطناعي اللاحق';
COMMENT ON COLUMN catalog.import_staging_rows.ai_state IS 'حالة معالجة الذكاء الاصطناعي للصف';
COMMENT ON COLUMN catalog.import_staging_rows.match_run_id IS 'معرف عملية المطابقة المرتبطة في catalog.match_runs';
COMMENT ON COLUMN catalog.import_staging_rows.match_decision IS 'سجل تفاصيل قرار المطابقة الكامل بصيغة JSON v1';
COMMENT ON COLUMN catalog.import_staging_rows.baseline_decision IS 'قرار المطابقة الحسابي الأولي قبل أي تدخل للذكاء الاصطناعي';
COMMENT ON COLUMN catalog.import_staging_rows.proposal IS 'اقتراح معالجة الصنف في الكتالوج المعتمد: تحديث، جديد محتمل، مكرر، أو غير محسوم';

COMMIT;
