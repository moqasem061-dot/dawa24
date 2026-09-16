-- 037_content_blocks
--
-- CMS content blocks (legacy what_in_contents). These back the /about,
-- /how-it-works, /faq and /help pages so their copy is editable without a
-- redeploy. platform_admin.privacy_policies already exists (migration 021) and
-- backs /privacy and /terms.

BEGIN;

CREATE TABLE IF NOT EXISTS platform_admin.content_blocks (
    id         BIGSERIAL PRIMARY KEY,
    key        VARCHAR(64) NOT NULL,
    title      JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    body       JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    position   VARCHAR(64) NOT NULL DEFAULT 'page',
    sort_order INT NOT NULL DEFAULT 0,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_content_blocks_key UNIQUE (key)
);

COMMENT ON COLUMN platform_admin.content_blocks.key IS 'مفتاح الكتلة — what_in_contents key';
COMMENT ON COLUMN platform_admin.content_blocks.body IS 'محتوى الكتلة // {"ar":"","en":""}';

-- Seed the four public pages so they render something sensible out of the box.
INSERT INTO platform_admin.content_blocks (key, title, body, position, sort_order) VALUES
    ('about',   '{"ar":"من نحن","en":"About us"}',       '{"ar":"دواء 24 هو سوق دوائي موحد يربط الصيدليات بالموردين وشركات الأدوية في مصر.","en":"Dawa24 is a unified pharmaceutical marketplace connecting pharmacies with suppliers across Egypt."}', 'page', 10),
    ('how-it-works', '{"ar":"كيف يعمل","en":"How it works"}', '{"ar":"سجّل مؤسستك، تصفح المنتجات، اطلب مباشرة من الموردين، وتابع شحناتك.","en":"Register your organization, browse products, order directly from suppliers, and track your shipments."}', 'page', 20),
    ('faq',     '{"ar":"الأسئلة الشائعة","en":"FAQ"}',   '{"ar":"نجيب هنا عن الأسئلة الأكثر شيوعاً حول التسجيل والطلبات والدفع.","en":"Answers to common questions about registration, ordering and payment."}', 'page', 30),
    ('help',    '{"ar":"مركز المساعدة","en":"Help"}',     '{"ar":"تواصل مع فريق الدعم عبر صفحة تواصل معنا.","en":"Reach the support team through the contact page."}', 'page', 40)
ON CONFLICT (key) DO NOTHING;

COMMIT;
