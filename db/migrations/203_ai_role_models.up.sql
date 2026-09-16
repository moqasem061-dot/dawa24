BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.ai_role_models (
    role        TEXT PRIMARY KEY,
    model       TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    max_tokens  INT,
    notes       TEXT NOT NULL DEFAULT '',
    updated_by  BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed defaults so roles exist explicitly for operators to repoint
INSERT INTO platform_admin.ai_role_models (role, model, is_active, notes)
VALUES
    ('matching.vendor_import', 'qwen3.7-flash', true, 'استيراد فواتير ومخزون الموردين (/vendor/ingest)'),
    ('matching.saving_products', 'qwen3.7-flash', true, 'قوائم توفير الصيدليات والموردين'),
    ('matching.smart_order', 'qwen3.7-flash', true, 'الطلب الذكي للنواقص بالذكاء الاصطناعي (/customer/smart-order)'),
    ('matching.admin_catalog', 'qwen3.7-flash', true, 'استيراد الكتالوج العام للإدارة (/admin/products/import)'),
    ('matching.adjudicate', 'qwen3.7-flash', true, 'محرك مطابقة الأصناف العام (Fallback)'),
    ('assistant.primary', 'gemma-4-31b-it', true, 'المساعد الذكي للمحادثة ومعالجة الصور'),
    ('assistant.attachment', 'gemma-4-31b-it', true, 'معالجة مرفقات ومستندات المساعد الذكي'),
    ('assistant.transcribe', 'whisper-large-v3-turbo', true, 'التعرف على الصوت وتحويله إلى نصوص'),
    ('import.detect_columns', 'qwen3.7-flash', true, 'كشف أعمدة الملفات والجداول الذكي'),
    ('search.expand_query', 'qwen3.7-flash', true, 'توسيع مرادفات البحث بالكتالوج'),
    ('support.classify', 'qwen3.7-flash', true, 'تصنيف وتوجيه تذاكر الدعم الفني')
ON CONFLICT (role) DO NOTHING;

COMMIT;
