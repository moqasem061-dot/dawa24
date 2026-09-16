-- 158_expand_platform_policies (up)
-- Expansion of platform legal policies into 5 distinct categories:
-- terms, privacy, shipping_return, cookies, payment

-- Unpublish older versions if any exist for the new policy keys
UPDATE platform_admin.policies
SET is_published = false
WHERE policy_key IN ('terms', 'privacy', 'shipping_return', 'cookies', 'payment');

-- Seed or update the 5 official platform policy documents
INSERT INTO platform_admin.policies (
    policy_key, version, title, content, summary, is_published, published_at, created_at, updated_at, policy_type
)
SELECT p.key, '2.0', p.title, p.content, p.summary, true, NOW(), NOW(), NOW(), 'platform'
FROM (
    VALUES 
    ('terms'::VARCHAR(64), '{"ar":"شروط وأحكام استخدام منصة Dawa24","en":"Terms & Conditions of Use"}'::jsonb, '{"ar":"شروط وأحكام استخدام منصة Dawa24 الشاملة","en":"Terms and conditions of use"}'::jsonb, '{"ar":"الإصدار الشامل لشروط الاستخدام والتزامات الأطراف واستقلال المنصة التقني","en":"Comprehensive terms of use"}'::jsonb),
    ('privacy'::VARCHAR(64), '{"ar":"سياسة الخصوصية وحماية البيانات الشخصية","en":"Privacy & Data Protection Policy"}'::jsonb, '{"ar":"سياسة الخصوصية وسرية البيانات التجارية والطبية طبقاً للقانون المصري رقم 151 لسنة 2020","en":"Privacy policy"}'::jsonb, '{"ar":"حماية البيانات الشخصية والتجارية وضمانات السرية","en":"Data protection"}'::jsonb),
    ('shipping_return'::VARCHAR(64), '{"ar":"سياسة الشحن والتسليم والاسترجاع والإلغاء","en":"Shipping, Returns & Cancellation Policy"}'::jsonb, '{"ar":"الضوابط المنظمة لشحن الأدوية، فحص الاستلام، حالات المرتجعات المسموحة، وإلغاء الطلبات","en":"Shipping and returns policy"}'::jsonb, '{"ar":"قواعد الشحن والاستلام والمرتجعات الصيدلانية وإلغاء الطلبات","en":"Shipping and return rules"}'::jsonb),
    ('cookies'::VARCHAR(64), '{"ar":"سياسة ملفات تعريف الارتباط (Cookies)","en":"Cookie & Session Policy"}'::jsonb, '{"ar":"بيان استخدام ملفات تعريف الارتباط وتقنيات حفظ الجلسات وأمان المنصة ومهلة الخمول","en":"Cookie policy"}'::jsonb, '{"ar":"ملفات تعريف الارتباط الضرورية والوظيفية ومهلة الخمول التلقائي","en":"Cookie usage details"}'::jsonb),
    ('payment'::VARCHAR(64), '{"ar":"سياسة الدفع والتعاملات المالية","en":"Payment & Financial Policy"}'::jsonb, '{"ar":"الضوابط المالية المنظمة لطرق الدفع، التحصيل المباشر، الفواتير الإلكترونية، وإخلاء المسؤولية","en":"Payment policy"}'::jsonb, '{"ar":"طرق الدفع والتحصيل والفواتير الضريبية والإلكترونية","en":"Payment and invoicing"}'::jsonb)
) AS p(key, title, content, summary)
ON CONFLICT (policy_key, version) DO UPDATE SET
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    is_published = true,
    published_at = NOW(),
    updated_at = NOW();