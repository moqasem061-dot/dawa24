-- 166_unified_match_score.up.sql
-- One "minimum match score" control, stored the same way by every import tool.
--
-- The vendor catalogue import and the admin main-catalogue import keep their
-- settings as JSON on the session row, so they needed no column. The smart
-- order keeps its configuration in real columns, and had none for this, which
-- meant a buyer's choice survived the request that made it and nothing longer.

BEGIN;

ALTER TABLE smartorder.run_config
    ADD COLUMN IF NOT EXISTS min_match_score NUMERIC(4,3) NOT NULL DEFAULT 0.500;

ALTER TABLE smartorder.criteria_profiles
    ADD COLUMN IF NOT EXISTS min_match_score NUMERIC(4,3) NOT NULL DEFAULT 0.500;

COMMENT ON COLUMN smartorder.run_config.min_match_score IS
    'أقل نسبة مطابقة يقبلها المحرك لعرض الصنف للمراجعة (0-1). لا تُستخدم لاعتماد الشراء تلقائياً';

COMMENT ON COLUMN smartorder.criteria_profiles.min_match_score IS
    'أقل نسبة مطابقة المحفوظة في ملف تفضيلات الصيدلية (0-1)';

COMMIT;
