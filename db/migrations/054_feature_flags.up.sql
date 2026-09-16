-- Migration 054: Dynamic Platform Feature Flags
-- Enables real-time toggling of platform modules (jobs, seeker accounts, reviews, finder).

BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.feature_flags (
    key           TEXT PRIMARY KEY,
    name          JSONB NOT NULL,
    description   JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::JSONB,
    is_enabled    BOOLEAN NOT NULL DEFAULT true,
    updated_by    BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE platform_admin.feature_flags ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname = 'feature_flags_policy') THEN
        CREATE POLICY feature_flags_policy ON platform_admin.feature_flags
            USING (true)
            WITH CHECK (true);
    END IF;
END $$;

INSERT INTO platform_admin.feature_flags (key, name, description, is_enabled) VALUES
  ('jobs.enabled', '{"ar":"قسم الوظائف والتوظيف","en":"Jobs & Careers"}'::jsonb,
   '{"ar":"تفعيل قسم الوظائف وإعلانات التوظيف في الموقع والفوتر","en":"Job board and postings"}'::jsonb, true),
  ('jobs.seeker_accounts', '{"ar":"حسابات باحث عن عمل","en":"Job Seeker Accounts"}'::jsonb,
   '{"ar":"السماح بالتسجيل في المنصة كباحث عن عمل صيدلاني أو مهني","en":"Allow job seeker registration"}'::jsonb, true),
  ('reviews.enabled',  '{"ar":"التقييمات ومراجعات الجودة","en":"Reviews & Quality Ratings"}'::jsonb,
   '{"ar":"تفعيل التقييم متعدد المعايير للموردين والصيدليات","en":"Multi-criteria reviews"}'::jsonb, true),
  ('offers.enabled',   '{"ar":"العروض والتخفيضات","en":"Promotional Offers"}'::jsonb,
   '{"ar":"تفعيل قسم العروض والخصومات الخاصة","en":"Promotions and flash sales"}'::jsonb, true),
  ('finder.enabled',   '{"ar":"دليل وباحث الأدوية الذكي","en":"Product Finder"}'::jsonb,
   '{"ar":"تفعيل باحث الأدوية والمثائل والبدائل","en":"Medicine and active ingredient finder"}'::jsonb, true),
  ('services.enabled', '{"ar":"الخدمات المؤسسية","en":"Institutional Services"}'::jsonb,
   '{"ar":"تفعيل طلبات الخدمات المؤسسية للمستشفيات والشركات","en":"Corporate and institutional services"}'::jsonb, true),
  ('compare.enabled',  '{"ar":"مقارنة خطط التوريد","en":"Plan Comparison"}'::jsonb,
   '{"ar":"تفعيل جداول مقارنة الخطط والاشتراكات","en":"Subscription plan comparisons"}'::jsonb, true)
ON CONFLICT (key) DO UPDATE SET is_enabled = EXCLUDED.is_enabled;

COMMIT;
