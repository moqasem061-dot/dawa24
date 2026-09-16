-- 075_review_criteria_and_ratings (up)
--
-- Rebuild V2 §5.1 — Seed standard 3-criteria review metrics:
-- rep (تقييم المندوب والتواصل), speed (سرعة التوصيل والتجهيز), quality (جودة الأصناف والتعامل).

BEGIN;

INSERT INTO org.review_criteria (key, name, context, weight, sort_order, is_active) VALUES
    ('rep', '{"ar":"تقييم المندوب والتواصل","en":"Sales Rep & Support"}'::jsonb, 'supplier', 1.000, 1, true),
    ('speed', '{"ar":"سرعة التوصيل والتجهيز","en":"Delivery Speed"}'::jsonb, 'supplier', 1.000, 2, true),
    ('quality', '{"ar":"جودة الأصناف والتعامل","en":"Quality & Reliability"}'::jsonb, 'supplier', 1.000, 3, true)
ON CONFLICT (key) DO UPDATE SET is_active = true, name = EXCLUDED.name, context = EXCLUDED.context;

COMMIT;