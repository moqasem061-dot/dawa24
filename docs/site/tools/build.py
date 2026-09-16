import subprocess, re, json, os, collections, sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, '..', '..', '..'))
OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(HERE, '..', 'files.js')
os.chdir(REPO)

def load(name, prefix=''):
    d = {}
    for line in open(os.path.join(HERE, name), encoding='utf-8'):
        line = line.rstrip('\n')
        if '\t' not in line:
            continue
        k, v = line.split('\t', 1)
        d[k if k.startswith('M:') or k.startswith(('internal/', 'db/')) and not prefix else prefix + k] = v
    return d

desc = {}
desc.update(load('d1.tsv'))
desc.update(load('d2.tsv', 'internal/modules/'))
desc.update(load('d3.tsv', 'internal/ui/'))
desc.update(load('d4.tsv'))

hints = {}
for line in open(os.path.join(HERE, 'hints.tsv'), encoding='utf-8'):
    p = line.rstrip('\n').split('\t')
    if len(p) >= 3:
        hints[p[0]] = (int(p[1]), p[2])

PAGE = {
 'about': 'صفحة من نحن', 'account': 'بيانات الحساب', 'admin_ads': 'قائمة الإعلانات في الإدارة',
 'admin_adv_product_detail': 'تفاصيل رعاية منتج في الإدارة', 'admin_adv_products': 'رعايات المنتجات في الإدارة',
 'admin_ai_logs': 'سجلات استهلاك الذكاء الاصطناعي', 'admin_analytics': 'تحليلات الزوار', 'admin_approvals': 'قائمة الموافقات',
 'admin_askfor': 'المستندات المطلوبة', 'admin_audit': 'سجل التدقيق', 'admin_branches': 'فروع المنشآت في الإدارة',
 'admin_brands': 'الماركات والشركات المصنعة', 'admin_catalog': 'الكتالوج في الإدارة', 'admin_categories': 'التصنيفات',
 'admin_chat': 'محادثات المساعد للتدقيق', 'admin_chat_history': 'محادثات المساعد للتدقيق', 'admin_cities': 'المدن والإحداثيات',
 'admin_content': 'كتل المحتوى', 'admin_credit_accounts': 'حسابات رصيد الباقات', 'admin_dashboard': 'لوحة الإدارة الرئيسية',
 'admin_deletion_requests': 'طلبات الحذف', 'admin_developers': 'بوابة المطورين', 'admin_documents': 'المستندات',
 'admin_employee_activities': 'سجل نشاط الموظفين', 'admin_error_detail': 'تفاصيل خطأ', 'admin_finance': 'المالية في الإدارة',
 'admin_finance_deposits': 'الإيداعات', 'admin_finance_invoices': 'الفواتير', 'admin_finance_payments': 'المدفوعات',
 'admin_finance_statement': 'كشف حساب منشأة', 'admin_finance_transactions': 'المعاملات', 'admin_finance_wallets': 'المحافظ',
 'admin_finance_withdrawals': 'طلبات السحب', 'admin_full_user': 'إدارة المستخدم الكاملة', 'admin_import_mapping': 'ربط أعمدة استيراد الكتالوج',
 'admin_import_review': 'مراجعة استيراد الكتالوج', 'admin_import_structure': 'هيكل ملف الاستيراد', 'admin_import_wizard': 'معالج استيراد الكتالوج',
 'admin_institutional': 'الأعمال المؤسسية', 'admin_jobs': 'الوظائف', 'admin_jobs_detail': 'تفاصيل وظيفة', 'admin_match_decisions': 'ذاكرة قرارات المطابقة',
 'admin_messages': 'رسائل التواصل', 'admin_monitoring': 'مراقبة الأخطاء', 'admin_offers': 'العروض في الإدارة', 'admin_orders': 'الطلبات في الإدارة',
 'admin_org_changes': 'طلبات تغيير بيانات المنشآت', 'admin_org_detail': 'نظرة شاملة على منشأة', 'admin_org_edit': 'تعديل منشأة',
 'admin_org_import': 'الاستيراد نيابة عن منشأة', 'admin_org_registration_detail': 'تفاصيل تسجيل منشأة', 'admin_organizations': 'المنشآت',
 'admin_page_control': 'التحكم في صفحات النظام', 'admin_plans': 'خطط الاشتراك', 'admin_plans_subscriptions': 'المشتركين',
 'admin_policies': 'السياسات', 'admin_product': 'منتج الكتالوج', 'admin_product_detail': 'تفاصيل منتج الكتالوج',
 'admin_product_images': 'صور المنتجات', 'admin_product_images_import': 'استيراد صور المنتجات', 'admin_products': 'الكتالوج الرئيسي',
 'admin_reference_crud': 'البيانات المرجعية', 'admin_reviews': 'التقييمات', 'admin_saving': 'منتجات التوفير في الإدارة',
 'admin_saving_products': 'منتجات التوفير في الإدارة', 'admin_settings': 'إعدادات المنصة', 'admin_staff_dashboard': 'لوحة موظفي المنصة',
 'admin_stocks': 'مخزون الموردين', 'admin_temp_warehouses': 'المستودعات المؤقتة', 'admin_temp_warehouses_runs': 'تشغيلات المستودعات المؤقتة',
 'admin_temp_warehouses_runs_review': 'مراجعة تشغيل مستودع مؤقت', 'admin_temp_warehouses_upload': 'رفع المستودعات المؤقتة',
 'admin_translations': 'الترجمات', 'admin_trash': 'سلة المحذوفات', 'admin_user_organizations': 'ربط المستخدمين بالمنشآت',
 'admin_users': 'المستخدمين', 'admin_warehouses': 'المستودعات', 'admin_weekly_coverages': 'التغطية الأسبوعية',
 'ai_consumption_logs': 'استهلاك الذكاء الاصطناعي للمنشأة', 'auth': 'صفحات الدخول والتسجيل', 'auth_login': 'صفحة الدخول',
 'cart': 'السلة', 'catalog': 'الكتالوج', 'compare': 'أداة المقارنة', 'compare_benchmark_offers': 'الموردين وراء رقم المقارنة',
 'compare_head_to_head': 'مقارنة مورد ضد مورد', 'compare_mapping': 'ربط أعمدة ملفات المقارنة', 'compare_market_benchmark': 'المقارنة بالسوق',
 'compare_market_intelligence': 'ذكاء السوق', 'compare_plans': 'خطط المقارنة', 'compare_tool': 'أداة المقارنة', 'compare_results': 'نتائج المقارنة',
 'component_gallery': 'معرض المكونات', 'contact': 'صفحة التواصل', 'content': 'صفحة محتوى', 'credit_statement': 'كشف حساب رصيد الباقة',
 'customer': 'لوحة الصيدلية', 'customer_branches': 'فروع الصيدلية', 'customer_cart': 'سلة الصيدلية', 'customer_catalog': 'كتالوج الشراء',
 'customer_checkout': 'صفحة الدفع', 'customer_decision_memory': 'ذاكرة القرارات للصيدلية', 'customer_followed_suppliers': 'الموردين المتابَعين',
 'customer_home': 'الصفحة الرئيسية للصيدلية', 'customer_invoices': 'فواتير الصيدلية', 'customer_jobs': 'وظائف الصيدلية',
 'customer_negotiation': 'التفاوض على الطلب', 'customer_order_detail': 'تفاصيل طلب الصيدلية', 'customer_order_offer': 'العرض داخل الطلب',
 'customer_order_review': 'تقييم الطلب', 'customer_orders': 'طلبات الصيدلية', 'customer_product': 'منتج للصيدلية', 'customer_product_detail': 'تفاصيل المنتج',
 'customer_saving': 'منتجات التوفير للصيدلية', 'customer_team': 'فريق الصيدلية', 'customer_user_org': 'ربط الصيدلية بالموردين',
 'customer_user_organizations': 'الموردين المرتبطين', 'dashboard': 'لوحات التحكم', 'document_error': 'خطأ المستند', 'documents': 'المستندات',
 'error_page': 'صفحة الخطأ', 'faq': 'الأسئلة الشائعة', 'favorites': 'المفضلة', 'geo_options': 'خيارات المحافظات والمدن',
 'home_hero_preview': 'معاينة واجهة الصفحة الرئيسية', 'how_it_works': 'كيف تعمل المنصة', 'institutional_categories': 'فئات الأعمال المؤسسية',
 'invoice_payment': 'دفع الفاتورة', 'invoice_printable': 'الفاتورة المطبوعة', 'invoice_printable_a4': 'الفاتورة بمقاس A4',
 'invoice_printable_thermal': 'الفاتورة الحرارية', 'job_detail': 'تفاصيل وظيفة', 'jobs': 'لوحة الوظائف', 'market_discounts': 'خصومات السوق',
 'messages': 'الرسائل', 'mfa_settings': 'إعدادات التحقق بخطوتين', 'mfa_verify': 'التحقق بخطوتين عند الدخول', 'notifications': 'الإشعارات',
 'notifications_dropdown': 'قائمة الإشعارات المنسدلة', 'offers': 'العروض', 'offers_detail': 'تفاصيل العرض', 'onboarding': 'إكمال التسجيل',
 'onboarding_pending': 'انتظار اعتماد المنشأة', 'organization_documents': 'مستندات المنشأة', 'organization_profile': 'بيانات المنشأة',
 'password_reset': 'استعادة كلمة المرور', 'pharmacy_dashboard': 'لوحة الصيدلية', 'platform_hardening': 'خطط الأجهزة والجلسات',
 'policy_markdown': 'عرض السياسات المكتوبة', 'promo_revenue': 'مركز الباقات والإيرادات', 'public_pages': 'الصفحات العامة',
 'purchase_requests': 'طلبات الشراء', 'register': 'نموذج التسجيل', 'reltime': 'الوقت النسبي', 'report_issue': 'الإبلاغ عن مشكلة',
 'requests': 'صندوق الطلبات', 'roles': 'الأدوار والصلاحيات', 'saving_import': 'استيراد منتجات التوفير', 'saving_import_review': 'مراجعة استيراد التوفير',
 'saving_import_wizard': 'معالج استيراد التوفير', 'settings_telegram': 'كارت ربط تيليجرام', 'settings_unified': 'صفحة الإعدادات الموحدة',
 'settings_whatsapp': 'كارت ربط واتساب', 'shipment_lines_pagination': 'ترقيم بنود الشحنة', 'smart_order': 'الطلب الذكي',
 'smart_order_review': 'مراجعة الطلب الذكي', 'storefront': 'متجر المورد', 'subscription_gate': 'حاجز الاشتراك', 'suppliers': 'دليل الموردين',
 'suppliers_map': 'خريطة الموردين', 'suppliers_profile': 'صفحة المورد العامة', 'team_import': 'استيراد الفريق', 'team_import_wizard': 'معالج استيراد الفريق',
 'tenant_sessions': 'الأجهزة والجلسات', 'tenant_subscription': 'الاشتراك', 'tenant_subscription_plans': 'خطط الاشتراك للمنشأة',
 'time_format': 'تنسيق الوقت', 'vendor': 'لوحة المورد', 'vendor_activities': 'سجل نشاط موظفي المورد', 'vendor_ad_edit': 'تعديل الإعلان',
 'vendor_ads': 'إعلانات المورد', 'vendor_ads_page': 'صفحة إعلانات المورد', 'vendor_ads_wizard': 'معالج إنشاء إعلان',
 'vendor_branches': 'فروع المورد', 'vendor_catalog_select': 'اختيار منتج من الكتالوج', 'vendor_content': 'سياسات المورد',
 'vendor_coverage': 'التغطية الأسبوعية للمورد', 'vendor_dashboard': 'لوحة المورد', 'vendor_delivery': 'لوحة توزيع الشحنات',
 'vendor_delivery_assign': 'تعيين شحنة لمندوب', 'vendor_delivery_detail': 'تفاصيل شحنة', 'vendor_delivery_map': 'خريطة الشحنات',
 'vendor_delivery_route': 'مسار المندوب', 'vendor_earnings': 'أرباح المورد', 'vendor_ingest': 'استيراد كتالوج المورد',
 'vendor_ingest_admin': 'استيراد المورد من الإدارة', 'vendor_ingest_commit': 'حفظ استيراد المورد', 'vendor_ingest_mapping': 'ربط أعمدة استيراد المورد',
 'vendor_ingest_review': 'مراجعة استيراد المورد', 'vendor_ingest_stages': 'مراحل استيراد المورد', 'vendor_inventory': 'مخزون المورد',
 'vendor_invoice_create': 'إنشاء فاتورة', 'vendor_invoices': 'فواتير المورد', 'vendor_jobs': 'وظائف المورد', 'vendor_offer': 'عرض المورد',
 'vendor_offer_form': 'نموذج العرض الخاص', 'vendor_offer_locations': 'مناطق تغطية العرض', 'vendor_offers': 'العروض الخاصة للمورد',
 'vendor_offers_locations_page': 'صفحة مناطق العروض', 'vendor_order': 'طلب المورد', 'vendor_order_detail': 'تفاصيل طلب المورد',
 'vendor_order_shipment': 'شحنة الطلب', 'vendor_orders': 'طلبات المورد', 'vendor_packages': 'باقات الرعاية', 'vendor_payments': 'مدفوعات المورد',
 'vendor_pharmacy_coverage': 'الصيدليات داخل تغطية المورد', 'vendor_product_editor': 'محرر منتج المورد', 'vendor_products': 'منتجات المورد',
 'vendor_quotas': 'حصص الفروع', 'vendor_reviews': 'تقييمات المورد', 'vendor_roles': 'أدوار المورد', 'vendor_saving': 'منتجات التوفير للمورد',
 'vendor_sponsorship': 'رعاية المورد', 'vendor_sponsorship_requests': 'طلبات الرعاية', 'vendor_team': 'فريق المورد', 'vendor_transfers': 'تحويلات المخزون',
 'vendor_user_organizations': 'الصيدليات المرتبطة بالمورد', 'vendor_variant': 'متغيرات المورد', 'vendor_warehouse_detail': 'تفاصيل المستودع',
 'vendor_warehouses': 'مستودعات المورد', 'wallet': 'المحفظة', 'wallet_modal_deposit': 'نافذة الإيداع', 'wallet_modal_payment_method': 'نافذة وسيلة الدفع',
 'wallet_modal_withdraw': 'نافذة السحب', 'wallet_transactions': 'جدول معاملات المحفظة', 'wizard': 'المعالجات متعددة الخطوات',
}
SUFFIX = [
 ('_modals', 'النوافذ المنبثقة الخاصة بـ{}'), ('_modal', 'نافذة منبثقة: {}'), ('_tables', 'جداول {}'), ('_table', 'جدول {}'),
 ('_review_script', 'سكربت مراجعة {}'), ('_import_script', 'سكربت استيراد {}'), ('_import_modal', 'نافذة استيراد {}'),
 ('_script', 'سكربت الواجهة الخاص بـ{}'), ('_helpers', 'دوال مساعدة لعرض {}'), ('_models_extra', 'نماذج بيانات إضافية لـ{}'),
 ('_models', 'نماذج البيانات اللي بتتعرض في {}'), ('_view', 'تجهيز بيانات العرض لـ{}'), ('_types', 'أنواع البيانات الخاصة بـ{}'),
 ('_sidebar', 'الشريط الجانبي في {}'), ('_form', 'نموذج الإدخال في {}'), ('_list', 'قائمة {}'), ('_toolbar', 'شريط أدوات {}'),
 ('_results_row', 'صف نتيجة في {}'), ('_rows', 'صفوف {}'), ('_row', 'صف في {}'), ('_footer', 'تذييل {}'), ('_calc', 'حسابات {}'),
 ('_data', 'بيانات {}'), ('_stops', 'نقاط التوقف في {}'), ('_actions', 'أزرار الإجراءات في {}'), ('_cards', 'كروت {}'), ('_card', 'كارت {}'),
 ('_extensions', 'إضافات {}'), ('_admin_subpages', 'صفحات فرعية للإدارة في {}'), ('_subpages', 'الصفحات الفرعية لـ{}'), ('_media', 'وسائط {}'),
 ('_metrics', 'مؤشرات {}'), ('_widgets', 'عناصر {}'), ('_kpis', 'مؤشرات الأداء في {}'), ('_operations', 'التشغيل في {}'),
 ('_buybox', 'صندوق الشراء في {}'), ('_offers', 'العروض في {}'), ('_lines', 'بنود {}'), ('_badge', 'شارة {}'), ('_fields', 'حقول {}'),
 ('_styles', 'أنماط {}'), ('_grid', 'شبكة {}'), ('_tabs', 'تبويبات {}'), ('_reviews', 'التقييمات في {}'), ('_catalog', 'الكتالوج في {}'),
 ('_products', 'المنتجات في {}'), ('_pricing', 'التسعير في {}'), ('_bands', 'شرائح التوصيل في {}'), ('_builder', 'منشئ {}'),
 ('_closed', 'المغلق في {}'), ('_items', 'البنود في {}'), ('_excluded', 'البنود المستبعدة في {}'), ('_groups', 'التجميع بالمورد في {}'),
 ('_users', 'المستخدمين في {}'), ('_job_seekers', 'الباحثين عن عمل في {}'), ('_diagnostics', 'التشخيص في {}'), ('_sql', 'كونسول SQL في {}'),
 ('_seo', 'SEO في {}'), ('_ai', 'الذكاء الاصطناعي في {}'), ('_site', 'بيانات الموقع في {}'), ('_payment', 'الدفع في {}'),
 ('_policies', 'السياسات في {}'), ('_history', 'سجل {}'), ('_steps', 'خطوات {}'), ('_filter', 'فلاتر {}'), ('_results', 'نتائج {}'),
 ('_branch', 'اختيار الفرع في {}'), ('_add_custom', 'إضافة منتج مخصص في {}'), ('_children', 'المتغيرات التابعة في {}'),
 ('_completed', 'الجلسات المكتملة في {}'), ('_orders', 'الطلبات في {}'), ('_detail', 'تفاصيل {}'),
]

def page_desc(stem, ext):
    if stem in PAGE:
        base = PAGE[stem]
        kind = 'قالب templ: ' if ext == '.templ' else 'كود Go للصفحة: '
        return kind + base + '.'
    for suf, fmt in SUFFIX:
        if stem.endswith(suf):
            core = stem[:-len(suf)]
            if core in PAGE:
                return fmt.format(PAGE[core]) + '.'
            for suf2, fmt2 in SUFFIX:
                if core.endswith(suf2) and core[:-len(suf2)] in PAGE:
                    return fmt.format(fmt2.format(PAGE[core[:-len(suf2)]])) + '.'
    return None

I18N = {'admin': 'لوحة الإدارة', 'audit': 'سجل التدقيق', 'billing_hr': 'الفوترة والموارد البشرية', 'chat_attachments': 'الرسائل والمرفقات',
        'commerce_ingest': 'الطلبات والاستيراد', 'compare_promo': 'المقارنة والعروض', 'delivery': 'إدارة الشحنات والمندوبين',
        'developers_system': 'بوابة المطورين والنظام', 'errors_validation': 'رسائل الأخطاء والتحقق', 'frontend_ui': 'الواجهة العامة',
        'market_intel': 'تقرير ذكاء السوق', 'missing_keys': 'مفاتيح كانت ناقصة', 'modules': 'الموديولات', 'nav': 'القوائم والتنقل',
        'orders_ui': 'شاشات الطلبات', 'org_branches_geo': 'المنشآت والفروع والجغرافيا', 'org_profile': 'بيانات المنشأة',
        'pharmacy_ui': 'لوحة الصيدلية', 'promo_credits': 'رصيد الباقات', 'quota': 'حصص الفروع', 'repair_r1': 'دفعة إصلاحات النصوص',
        'shell_ui': 'هيكل اللوحات', 'subscriptions': 'الاشتراكات', 'telegram': 'كارت تيليجرام', 'whatsapp': 'كارت واتساب',
        'vendor_ui': 'لوحة المورد', 'wave3': 'دفعة نصوص الموجة الثالثة', 'wave4': 'دفعة نصوص الموجة الرابعة', 'admin_pagecontrol': 'التحكم في الصفحات'}

def docs_desc(path, title):
    name = os.path.basename(path)
    t = title or name
    if path.startswith('docs/modules/'):
        return 'توثيق موديول مكتوب مع الكود: ' + t
    if path.startswith('docs/adr/'):
        return 'سجل قرارات المعمارية وأسبابها. يتقري قبل اقتراح أي بديل.'
    if 'PROMPT' in name:
        return 'تعليمات كانت متكتبة لوكيل ذكاء اصطناعي أثناء بناء المشروع (مرجع تاريخي): ' + t
    if 'AUDIT' in name or 'REVIEW' in name or 'audit' in name:
        return 'مراجعة أو تدقيق سابق للمشروع (مرجع تاريخي): ' + t
    if 'PROGRESS' in name or 'DECISIONS' in name or 'DELETED' in name or 'MERGE_LOG' in name:
        return 'سجل متابعة لخطة سابقة: ' + t
    return 'خطة عمل أو وثيقة تخطيط من مراحل البناء (مرجع تاريخي): ' + t

files = [f for f in subprocess.run(['git', 'ls-files'], capture_output=True, text=True, encoding='utf-8').stdout.split('\n') if f]
entries = []
groups = collections.Counter()
missing = []
for f in files:
    if re.match(r'^internal/ui/data/uploads/', f):
        groups['internal/ui/data/uploads/products'] += 1; continue
    if '/vendor/leaflet' in f:
        groups['internal/ui/static/vendor/leaflet'] += 1; continue
    if f.startswith('test/corpus/files'):
        groups['test/corpus/files'] += 1; continue
    if 'visual_baselines' in f:
        groups['test/visual_baselines'] += 1; continue
    lines, hint = hints.get(f, (0, ''))
    ext = os.path.splitext(f)[1]
    d = desc.get(f)
    kind = 'src'
    if d is None:
        m = re.match(r'db/migrations/(\d+)_(.+)\.(up|down)\.sql$', f)
        if m:
            title = desc.get('M:' + m.group(1), m.group(2).replace('_', ' '))
            d = ('تطبيق التغيير: ' if m.group(3) == 'up' else 'التراجع عن التغيير: ') + title
            kind = 'migration'
        elif f.endswith('_templ.go'):
            d = 'ملف Go مولَّد تلقائيًا من ' + os.path.basename(f)[:-9] + '.templ بأمر templ generate. ما يتعدلش يدويًا.'
            kind = 'generated'
        elif f.endswith('_test.go'):
            src = f[:-8] + '.go'
            names = [n for n in hint.split('|')[-1].split(',') if n.strip()]
            d = ('اختبارات ' + (('للملف ' + os.path.basename(src)) if src in files else 'الحزمة'))
            if names:
                d += '. الحالات: ' + '، '.join(n.strip() for n in names[:6])
            kind = 'test'
        elif f.startswith('internal/shared/i18n/catalog_'):
            key = re.sub(r'_[a-j]$', '', os.path.basename(f)[8:-3])
            d = 'مفاتيح الترجمة بالعربي والإنجليزي الخاصة بـ' + I18N.get(key, key) + '.'
        elif f.startswith('internal/ui/pages/'):
            stem, e = os.path.splitext(os.path.basename(f))
            d = page_desc(stem, e)
        elif f.endswith('.md') and f.startswith('docs/'):
            d = docs_desc(f, hint)
            kind = 'doc'
        elif f.startswith('test/'):
            d = 'ملف دعم للاختبارات: ' + os.path.basename(f)
    if d is None:
        missing.append((f, hint[:100]))
        d = ''
    entries.append({'p': f, 'd': d, 'k': kind, 'n': lines})

for g, n in groups.items():
    label = {'internal/ui/data/uploads/products': 'صور منتجات مرفوعة أثناء التطوير ومضمنة في المستودع',
             'internal/ui/static/vendor/leaflet': 'مكتبة Leaflet للخرائط (نسخة محلية من طرف ثالث)',
             'test/corpus/files': 'ملفات Excel حقيقية من موردين تستخدم لقياس دقة المطابقة',
             'test/visual_baselines': 'لقطات مرجعية لاختبارات الشكل البصري'}[g]
    entries.append({'p': g + '/*', 'd': label + ' (' + str(n) + ' ملف).', 'k': 'group', 'n': 0})

entries.sort(key=lambda e: e['p'])
with open(OUT, 'w', encoding='utf-8') as w:
    w.write('window.DAWA_FILES=' + json.dumps(entries, ensure_ascii=False, separators=(',', ':')) + ';\n')
print('entries', len(entries), 'missing', len(missing))
for m in missing:
    print(m[0], '|', m[1])
