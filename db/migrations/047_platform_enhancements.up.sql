-- Migration 047: Platform Enhancements
-- 1. Add license attachment and approval metadata to org.organizations
ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS license_document_url TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS verification_notes TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS rejection_reason TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL;

-- 2. Create versioned platform policies table
CREATE TABLE IF NOT EXISTS platform_admin.policies (
    id BIGSERIAL PRIMARY KEY,
    policy_key VARCHAR(64) NOT NULL,
    version VARCHAR(32) NOT NULL DEFAULT '1.0',
    title JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    content JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    summary JSONB NOT NULL DEFAULT '{"ar":"","en":""}'::jsonb,
    is_published BOOLEAN NOT NULL DEFAULT false,
    published_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_policy_key_version UNIQUE (policy_key, version)
);

CREATE INDEX IF NOT EXISTS idx_policies_key_pub ON platform_admin.policies (policy_key, is_published);

-- Seed initial standard policies if not already present
INSERT INTO platform_admin.policies (policy_key, version, title, content, summary, is_published, published_at)
VALUES 
(
    'terms',
    '1.0',
    '{"ar":"شروط الاستخدام والخدمة","en":"Terms of Service"}',
    '{"ar":"مرحباً بكم في منصة دواء24، المنصة الرقمية المعتمدة لربط الصيدليات والمستودعات الطبية في جمهورية مصر العربية.\n\n1. يجب على جميع المنشآت الطبية تقديم ترخيص سارٍ من هيئة الدواء المصرية.\n2. يحظر تداول أو عرض أي أدوية غير مسجلة أو خاضعة للجدول بدون إذن صريح.\n3. تلتزم جميع الأطراف بأسعار التوريد والفواتير الإلكترونية المعتمدة.","en":"Welcome to DAWA24 platform."}',
    '{"ar":"الإصدار التأسيسي لشروط وأحكام المنصة","en":"Initial baseline terms of service"}',
    true,
    NOW()
),
(
    'privacy',
    '1.0',
    '{"ar":"سياسة الخصوصية وحماية البيانات","en":"Privacy Policy"}',
    '{"ar":"نلتزم في دواء24 بحماية سرية البيانات التجارية والطبية لكافة المنشآت والصيادلة المسجلين.\n\n1. لا يتم مشاركة الفواتير أو أسعار التوريد الخاصة إلا بين أطراف المعاملة المعتمدة.\n2. تطبق المنصة أحدث معايير التشفير والأمان المالي.","en":"Privacy policy and data protection guidelines."}',
    '{"ar":"الإصدار التأسيسي لسياسة الخصوصية","en":"Initial privacy policy"}',
    true,
    NOW()
),
(
    'refund',
    '1.0',
    '{"ar":"سياسة المرتجعات والتسويات الدوائية","en":"Return & Refund Policy"}',
    '{"ar":"تخضع جميع عمليات إرجاع الأدوية للضوابط الصادرة عن هيئة الدواء المصرية.\n\n1. تقبل مرتجعات الأدوية المنتهية الصلاحية وفق اتفاقيات التوريد المعتمدة.\n2. يتم تسوية المبالغ المستردة مباشرة في المحفظة الإلكترونية للصيدلية.","en":"Pharma return and refund terms."}',
    '{"ar":"الإصدار التأسيسي لسياسة المرتجعات","en":"Initial return policy"}',
    true,
    NOW()
),
(
    'vendor_agreement',
    '1.0',
    '{"ar":"اتفاقية التوريد وشروط الموردين","en":"Vendor Supply Agreement"}',
    '{"ar":"تحدد هذه الاتفاقية التزامات شركات التوزيع والمستودعات المعتمدة في سلاسل التوريد وسرعة الشحن وضمان جودة سلاسل التبريد (Cold-Chain).","en":"Vendor agreement terms."}',
    '{"ar":"الإصدار التأسيسي لاتفاقية الموردين","en":"Initial vendor supply agreement"}',
    true,
    NOW()
)
ON CONFLICT (policy_key, version) DO NOTHING;
