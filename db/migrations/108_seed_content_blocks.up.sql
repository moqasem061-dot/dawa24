-- 108_seed_content_blocks.up.sql
--
-- Seed comprehensive bilingual content blocks for public pages and highlight sections.

BEGIN;

INSERT INTO platform_admin.content_blocks (key, title, body, position, sort_order, is_active) VALUES
    ('about',
     '{"ar":"من نحن — منصة التوريد الدوائي الذكية","en":"About Us — Smart Pharma Supply Platform"}'::jsonb,
     '{"ar":"دواء 24 هي المنظومة الرقمية الرائدة في جمهورية مصر العربية المصممة لربط الصيدليات، المستشفيات، والمراكز الطبية المرخصة مباشرة بمصانع وشركات ومستودعات توزيع الأدوية المعتمدة، لخلق سوق دوائي موثوق، شفاف، وفائق الكفاءة.","en":"Dawa24 is Egypt leading digital pharmaceutical supply and marketplace platform connecting licensed pharmacies and hospitals directly with verified manufacturers and distributors."}'::jsonb,
     'page', 10, true),
    ('about-vision',
     '{"ar":"رؤيتنا","en":"Our Vision"}'::jsonb,
     '{"ar":"أن نكون البنية التحتية الرقمية الأكثر أماناً وموثوقية لسلاسل الإمداد والتوريد الدوائي في الشرق الأوسط وإفريقيا، وضمان توافر الدواء بجودة قياسية وأسعار عادلة لكل مريض عبر تمكين الصيدلي والمورد.","en":"To be the most trusted and secure digital infrastructure for pharmaceutical supply chains across the Middle East and Africa."}'::jsonb,
     'page', 12, true),
    ('about-mission',
     '{"ar":"رسالتنا","en":"Our Mission"}'::jsonb,
     '{"ar":"القضاء على تعقيدات ونواقص التوريد التقليدي عبر توفير منصة إلكترونية ذكية تتيح مقارنة الأسعار، الشراء المباشر، الفواتير الإلكترونية المعتمدة، وسلاسل التبريد المتطورة بدعم تقني وصيدلي متواصل 24/7.","en":"To eliminate traditional supply chain friction through intelligent price comparison, direct ordering, verified e-invoicing, and cold-chain logistics."}'::jsonb,
     'page', 14, true),
    ('about-banner',
     '{"ar":"كن جزءاً من مستقبل التوريد الدوائي الذكي","en":"Be part of the future of smart pharma supply"}'::jsonb,
     '{"ar":"سواء كنت صيدلية تسعى لتأمين احتياجاتها بأفضل الأسعار، أو مورداً يريد توسيع قاعدة عملائه، دواء 24 هو شريكك الاستراتيجي.","en":"Whether you are a pharmacy securing your needs or a supplier expanding your reach, Dawa24 is your strategic partner."}'::jsonb,
     'page', 16, true),
    ('how-it-works',
     '{"ar":"كيف يعمل نظام دواء 24","en":"How Dawa24 Works"}'::jsonb,
     '{"ar":"سجّل مؤسستك، تصفح كتالوج الأدوية المعتمد، قارن الأسعار والخصومات، اطلب مباشرة من الموردين، وتابع شحنتك المبردة خطوة بخطوة.","en":"Register your organization, browse verified medicine catalogs, compare prices and discounts, order directly from suppliers, and track your cold-chain shipments."}'::jsonb,
     'page', 20, true),
    ('faq',
     '{"ar":"الأسئلة الشائعة وإرشادات الاستخدام","en":"Frequently Asked Questions"}'::jsonb,
     '{"ar":"كل ما تحتاج لمعرفته حول طلب وتوريد الأدوية، شروط التعامل، وسياسات الدفع والشحن عبر منصة دواء 24.","en":"Everything you need to know about pharmaceutical procurement, terms of service, payment methods, and cold-chain shipping on Dawa24."}'::jsonb,
     'page', 30, true),
    ('help',
     '{"ar":"مركز المساعدة والدعم","en":"Help & Support Center"}'::jsonb,
     '{"ar":"فريق الدعم الفني والخدمات الصيدلانية متاح للإجابة على استفسارات الصيدليات والموردين على مدار الساعة عبر صفحة تواصل معنا أو الخط الساخن.","en":"Our pharmacy customer support team is available 24/7 to assist pharmacies and suppliers via the contact page or hotline."}'::jsonb,
     'page', 40, true),
    ('home-hero',
     '{"ar":"سوق التوريد الدوائي المباشر للصيدليات والمستودعات","en":"Direct Pharmaceutical Supply Marketplace"}'::jsonb,
     '{"ar":"اطلب طلبيتك الدوائية مباشرة من كبرى شركات التوزيع ومصانع الأدوية المرخصة، مع ضمان فواتير إلكترونية معتمدة وسلاسل شحن مبردة (Cold-Chain).","en":"Procure medicines directly from licensed pharma manufacturers and top distributors with certified e-invoicing and temperature-controlled shipping."}'::jsonb,
     'section', 50, true),
    ('highlight-coldchain',
     '{"ar":"سلسلة تبريد معتمدة (Cold-Chain)","en":"Certified Cold-Chain Logistics"}'::jsonb,
     '{"ar":"أسطول مجهز بحاويات مبردة ومجسات حرارية لضمان وصول الأنسولين والأمصال بأعلى معايير الأمان الحيوي.","en":"Climate-controlled delivery vehicles with real-time temperature sensors for sensitive biologics and vaccines."}'::jsonb,
     'section', 60, true),
    ('highlight-fastdelivery',
     '{"ar":"توصيل سريع لكافة المحافظات","en":"Fast Nationwide Delivery"}'::jsonb,
     '{"ar":"شبكة لوجستية تغطي محافظات ومدن الجمهورية لضمان وصول طلبيات الصيدليات في المواعيد المحددة.","en":"Nationwide logistics network covering all Egyptian governorates for on-time pharmaceutical deliveries."}'::jsonb,
     'section', 70, true),
    ('highlight-einvoice',
     '{"ar":"تكامل الفاتورة الإلكترونية","en":"Certified E-Invoicing"}'::jsonb,
     '{"ar":"ربط مباشر مع مصلحة الضرائب المصرية لإصدار الفواتير الضريبية المعتمدة فور تسليم أمر التوريد.","en":"Direct integration with the Egyptian Tax Authority for instant compliant electronic tax invoices."}'::jsonb,
     'section', 80, true)
ON CONFLICT (key) DO UPDATE SET
    title = EXCLUDED.title,
    body = EXCLUDED.body,
    position = EXCLUDED.position,
    sort_order = EXCLUDED.sort_order,
    is_active = EXCLUDED.is_active,
    updated_at = now();

COMMIT;
