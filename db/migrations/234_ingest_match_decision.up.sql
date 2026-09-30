-- 234_ingest_match_decision.up.sql
--
-- Adds product matching decision columns and audit fields to ingest.catalog_import_rows.
-- Supports deterministic matching results, AI states, and optimistic revision locking.

BEGIN;

ALTER TABLE ingest.catalog_import_rows
    ADD COLUMN IF NOT EXISTS match_state TEXT NULL
        CHECK (match_state IN ('accepted','confirmed','needs_review','ambiguous','no_candidate','conflict','invalid_input','unavailable','excluded')),
    ADD COLUMN IF NOT EXISTS match_basis TEXT NULL,
    ADD COLUMN IF NOT EXISTS match_granularity TEXT NULL,
    ADD COLUMN IF NOT EXISTS match_revision INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS human_locked BOOL NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ai_state TEXT NULL,
    ADD COLUMN IF NOT EXISTS match_run_id BIGINT NULL REFERENCES catalog.match_runs(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS match_decision JSONB NULL,
    ADD COLUMN IF NOT EXISTS baseline_decision JSONB NULL;

CREATE INDEX IF NOT EXISTS catalog_import_rows_match_state
    ON ingest.catalog_import_rows (import_id, match_state);

COMMENT ON COLUMN ingest.catalog_import_rows.match_state IS 'حالة المطابقة الأساسية: مقبولة، مؤكدة، تحتاج مراجعة، غامضة، لا يوجد مرشح، تعارض، مدخل غير صالح، غير متاح، مستبعد';
COMMENT ON COLUMN ingest.catalog_import_rows.match_basis IS 'مسار المطابقة: معرف، تطابق تام، تشابه، ذكاء اصطناعي، مراجع بشري';
COMMENT ON COLUMN ingest.catalog_import_rows.match_granularity IS 'مستوى تفصيل المطابقة المتحقق: منتج عام أو عبوة محددة';
COMMENT ON COLUMN ingest.catalog_import_rows.match_revision IS 'إصدار قرار المطابقة الحالي للقفل التفاؤلي';
COMMENT ON COLUMN ingest.catalog_import_rows.human_locked IS 'هل تم قفل القرار بقرار بشري يمنع تعديل الذكاء الاصطناعي اللاحق';
COMMENT ON COLUMN ingest.catalog_import_rows.ai_state IS 'حالة معالجة الذكاء الاصطناعي للصف: معطل، قيد المعالجة، مكتمل، متجاوز';
COMMENT ON COLUMN ingest.catalog_import_rows.match_run_id IS 'معرف عملية المطابقة المرتبطة في catalog.match_runs';
COMMENT ON COLUMN ingest.catalog_import_rows.match_decision IS 'سجل تفاصيل قرار المطابقة الكامل بصيغة JSON v1';
COMMENT ON COLUMN ingest.catalog_import_rows.baseline_decision IS 'قرار المطابقة الحسابي الأولي قبل أي تدخل للذكاء الاصطناعي';

COMMIT;
