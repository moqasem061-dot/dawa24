-- 104_supplier_featured_sections.up.sql
-- Supplier profile featured sections: rich description/content, section category type, accent color, and header visibility.

BEGIN;

ALTER TABLE promo.highlight_sections
    ADD COLUMN IF NOT EXISTS description    JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::JSONB,
    ADD COLUMN IF NOT EXISTS section_type    TEXT NOT NULL DEFAULT 'about',
    ADD COLUMN IF NOT EXISTS color           TEXT NOT NULL DEFAULT '#0284c7',
    ADD COLUMN IF NOT EXISTS show_in_header  BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN promo.highlight_sections.description IS 'المحتوى أو الوصف التفصيلي للقسم بالعربية والإنجليزية';
COMMENT ON COLUMN promo.highlight_sections.section_type IS 'نوع القسم (vision, goals, about, why_us, features, services, achievements, certifications, stats, special_info)';
COMMENT ON COLUMN promo.highlight_sections.color IS 'اللون أو السمة المميزة للقسم';
COMMENT ON COLUMN promo.highlight_sections.show_in_header IS 'إظهار القسم في رأس / بروفايل المورد';

COMMIT;
