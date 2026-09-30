-- 235_savings_match_decision.up.sql
--
-- Adds product matching decision columns and audit fields to platform.import_run_rows and catalog.saving_products.
-- Supports deterministic matching results, AI states, and optimistic revision locking for savings imports.

BEGIN;

-- 1. platform.import_run_rows (staged savings)
ALTER TABLE platform.import_run_rows
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

CREATE INDEX IF NOT EXISTS import_run_rows_match_state
    ON platform.import_run_rows (run_id, match_state);

COMMENT ON COLUMN platform.import_run_rows.match_state IS 'حالة المطابقة الأساسية للصف في منصة الاستيراد';
COMMENT ON COLUMN platform.import_run_rows.match_basis IS 'مسار المطابقة المستخدم للصف';
COMMENT ON COLUMN platform.import_run_rows.match_granularity IS 'مستوى تفصيل المطابقة المتحقق: منتج عام أو عبوة محددة';
COMMENT ON COLUMN platform.import_run_rows.match_revision IS 'إصدار قرار المطابقة الحالي للقفل التفاؤلي';
COMMENT ON COLUMN platform.import_run_rows.human_locked IS 'هل تم قفل القرار بقرار بشري يمنع تعديل الذكاء الاصطناعي اللاحق';
COMMENT ON COLUMN platform.import_run_rows.ai_state IS 'حالة معالجة الذكاء الاصطناعي للصف';
COMMENT ON COLUMN platform.import_run_rows.match_run_id IS 'معرف عملية المطابقة المرتبطة في catalog.match_runs';
COMMENT ON COLUMN platform.import_run_rows.match_decision IS 'سجل تفاصيل قرار المطابقة الكامل بصيغة JSON v1';
COMMENT ON COLUMN platform.import_run_rows.baseline_decision IS 'قرار المطابقة الحسابي الأولي قبل أي تدخل للذكاء الاصطناعي';

-- 2. catalog.saving_products (committed savings)
ALTER TABLE catalog.saving_products
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

CREATE INDEX IF NOT EXISTS saving_products_match_state
    ON catalog.saving_products (organization_id, match_state) WHERE deleted_at IS NULL;

ALTER TABLE catalog.saving_products FORCE ROW LEVEL SECURITY;

COMMENT ON COLUMN catalog.saving_products.match_state IS 'حالة المطابقة الأساسية لمنتج التوفير';
COMMENT ON COLUMN catalog.saving_products.match_basis IS 'مسار المطابقة المستخدم لمنتج التوفير';
COMMENT ON COLUMN catalog.saving_products.match_granularity IS 'مستوى تفصيل المطابقة المتحقق: منتج عام أو عبوة محددة';
COMMENT ON COLUMN catalog.saving_products.match_revision IS 'إصدار قرار المطابقة الحالي';
COMMENT ON COLUMN catalog.saving_products.human_locked IS 'هل تم قفل القرار بقرار بشري';
COMMENT ON COLUMN catalog.saving_products.ai_state IS 'حالة الذكاء الاصطناعي عند اعتماد منتج التوفير';
COMMENT ON COLUMN catalog.saving_products.match_run_id IS 'معرف عملية المطابقة المرتبطة في catalog.match_runs';
COMMENT ON COLUMN catalog.saving_products.match_decision IS 'سجل تفاصيل قرار المطابقة الكامل بصيغة JSON v1';
COMMENT ON COLUMN catalog.saving_products.baseline_decision IS 'قرار المطابقة الحسابي الأولي قبل أي تدخل للذكاء الاصطناعي';

COMMIT;
