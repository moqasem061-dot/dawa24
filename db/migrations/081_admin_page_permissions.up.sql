BEGIN;

-- Granular Admin Page Permissions (Plan V5 Phase 0 Task 0.2)
INSERT INTO identity.permissions (key, name, module, description) VALUES
  -- Catalog
  ('catalog.product.view', '{"ar":"عرض المنتجات","en":"View Products"}', 'catalog', 'View product catalog and exports'),
  ('catalog.product.update', '{"ar":"تعديل المنتجات","en":"Update Products"}', 'catalog', 'Edit products, change status, import items'),
  ('catalog.product.delete', '{"ar":"حذف المنتجات","en":"Delete Products"}', 'catalog', 'Delete product records'),
  ('catalog.category.view', '{"ar":"عرض التصنيفات","en":"View Categories"}', 'catalog', 'View product categories'),
  ('catalog.category.update', '{"ar":"تعديل التصنيفات","en":"Update Categories"}', 'catalog', 'Create and edit product categories'),
  ('catalog.category.delete', '{"ar":"حذف التصنيفات","en":"Delete Categories"}', 'catalog', 'Delete product categories'),
  ('catalog.brand.view', '{"ar":"عرض العلامات التجارية","en":"View Brands"}', 'catalog', 'View pharmaceutical brands'),
  ('catalog.brand.update', '{"ar":"تعديل العلامات التجارية","en":"Update Brands"}', 'catalog', 'Create and edit pharmaceutical brands'),
  ('catalog.brand.delete', '{"ar":"حذف العلامات التجارية","en":"Delete Brands"}', 'catalog', 'Delete pharmaceutical brands'),
  ('catalog.saving_product.view', '{"ar":"عرض منتجات التوفير","en":"View Saving Products"}', 'catalog', 'View curated saving deals'),
  ('catalog.saving_product.update', '{"ar":"تعديل منتجات التوفير","en":"Update Saving Products"}', 'catalog', 'Moderate and update saving deals'),

  -- Organizations & Branches
  ('org.organization.view', '{"ar":"عرض المنظمات","en":"View Organizations"}', 'org', 'View pharmacies and vendor profiles'),
  ('org.organization.update', '{"ar":"تعديل المنظمات","en":"Update Organizations"}', 'org', 'Approve/reject orgs, edit credit limits and sponsorship'),
  ('org.organization.delete', '{"ar":"حذف المنظمات","en":"Delete Organizations"}', 'org', 'Delete organizations'),
  ('org.branch.view', '{"ar":"عرض الفروع والتغطية","en":"View Branches & Coverage"}', 'org', 'View branch locations and weekly delivery schedules'),
  ('org.branch.update', '{"ar":"تعديل الفروع","en":"Update Branches"}', 'org', 'Edit branch information and status'),
  ('org.branch.delete', '{"ar":"حذف الفروع","en":"Delete Branches"}', 'org', 'Delete branch locations'),
  ('org.institutional_work.view', '{"ar":"عرض الأعمال المؤسسية","en":"View Institutional Works"}', 'org', 'View corporate structures and classifications'),
  ('org.institutional_work.update', '{"ar":"تعديل الأعمال المؤسسية","en":"Update Institutional Works"}', 'org', 'Manage corporate structures and links'),
  ('org.institutional_work.delete', '{"ar":"حذف الأعمال المؤسسية","en":"Delete Institutional Works"}', 'org', 'Delete corporate structure types'),
  ('org.role.view', '{"ar":"عرض أدوار المؤسسات","en":"View Org Roles"}', 'org', 'View vendor and customer roles'),
  ('org.role.update', '{"ar":"تعديل أدوار المؤسسات","en":"Update Org Roles"}', 'org', 'Edit vendor and customer roles'),
  ('org.role.delete', '{"ar":"حذف أدوار المؤسسات","en":"Delete Org Roles"}', 'org', 'Delete custom organization roles'),
  ('org.review.view', '{"ar":"عرض التقييمات","en":"View Reviews"}', 'org', 'View customer feedback and ratings'),
  ('org.review.update', '{"ar":"إدارة التقييمات","en":"Moderate Reviews"}', 'org', 'Approve or reject customer reviews'),
  ('org.review.delete', '{"ar":"حذف التقييمات","en":"Delete Reviews"}', 'org', 'Delete customer reviews'),

  -- Identity & Users
  ('identity.user.view', '{"ar":"عرض المستخدمين","en":"View Users"}', 'identity', 'View registered user accounts and audit queues'),
  ('identity.user.update', '{"ar":"تعديل المستخدمين","en":"Update Users"}', 'identity', 'Edit user roles, status, password resets, account deletions'),
  ('identity.user.delete', '{"ar":"حذف المستخدمين","en":"Delete Users"}', 'identity', 'Delete user accounts'),
  ('identity.admin_role.view', '{"ar":"عرض أدوار المشرفين","en":"View Admin Roles"}', 'identity', 'View staff roles and permission maps'),
  ('identity.admin_role.update', '{"ar":"تعديل أدوار المشرفين","en":"Update Admin Roles"}', 'identity', 'Create and modify staff roles and permission assignments'),
  ('identity.admin_role.delete', '{"ar":"حذف أدوار المشرفين","en":"Delete Admin Roles"}', 'identity', 'Delete staff roles'),

  -- Commerce & Orders
  ('commerce.order.view', '{"ar":"عرض الطلبات","en":"View Orders"}', 'commerce', 'View marketplace orders and tracking'),
  ('commerce.order.update', '{"ar":"تعديل الطلبات","en":"Update Orders"}', 'commerce', 'Update order statuses and process cancellations'),
  ('commerce.quote.view', '{"ar":"عرض عروض الأسعار","en":"View Quotes"}', 'commerce', 'View procurement quotes and RFQs'),

  -- Billing & Finance
  ('billing.invoice.view', '{"ar":"عرض الفواتير","en":"View Invoices"}', 'billing', 'View transaction invoices and statements'),
  ('billing.payment.view', '{"ar":"عرض المدفوعات والمحافظ","en":"View Payments"}', 'billing', 'View payment transactions and wallet balances'),
  ('billing.payment.update', '{"ar":"تعديل الأرصدة والمحافظ","en":"Adjust Wallets"}', 'billing', 'Manual debit/credit adjustments to wallets'),
  ('billing.subscription_plan.view', '{"ar":"عرض خطط الاشتراك","en":"View Subscription Plans"}', 'billing', 'View vendor subscription tiers'),
  ('billing.subscription_plan.update', '{"ar":"تعديل خطط الاشتراك","en":"Update Subscription Plans"}', 'billing', 'Create and edit vendor subscription tiers'),
  ('billing.session_plan.view', '{"ar":"عرض خطط الجلسات","en":"View Session Plans"}', 'billing', 'View procurement session tiers'),
  ('billing.session_plan.update', '{"ar":"تعديل خطط الجلسات","en":"Update Session Plans"}', 'billing', 'Create and edit procurement session tiers'),

  -- Promo & Ads
  ('promo.offer.view', '{"ar":"عرض العروض الترويجية","en":"View Offers"}', 'promo', 'View supplier discount promotions'),
  ('promo.offer.update', '{"ar":"إدارة العروض الترويجية","en":"Moderate Offers"}', 'promo', 'Approve/reject supplier discount promotions'),
  ('promo.ad.view', '{"ar":"عرض الإعلانات","en":"View Ads"}', 'promo', 'View banner advertisements'),
  ('promo.ad.update', '{"ar":"تعديل الإعلانات","en":"Update Ads"}', 'promo', 'Create, edit, and approve advertisements'),
  ('promo.ad.delete', '{"ar":"حذف الإعلانات","en":"Delete Ads"}', 'promo', 'Delete advertisements'),
  ('promo.ad_plan.view', '{"ar":"عرض باقات الإعلانات","en":"View Ad Plans"}', 'promo', 'View advertising pricing packages'),
  ('promo.ad_plan.update', '{"ar":"تعديل باقات الإعلانات","en":"Update Ad Plans"}', 'promo', 'Create and edit advertising packages'),

  -- Inventory & Warehouses
  ('inventory.warehouse.view', '{"ar":"عرض المستودعات","en":"View Warehouses"}', 'inventory', 'View central and regional vendor warehouses'),
  ('inventory.warehouse.update', '{"ar":"تعديل المستودعات","en":"Update Warehouses"}', 'inventory', 'Edit warehouse details and cold storage status'),
  ('inventory.warehouse.delete', '{"ar":"حذف المستودعات","en":"Delete Warehouses"}', 'inventory', 'Delete warehouse records'),
  ('inventory.stock.view', '{"ar":"عرض المخزون","en":"View Stock"}', 'inventory', 'View inventory quantities and batch expiry dates'),
  ('inventory.transfer.view', '{"ar":"عرض التحويلات المخزنية","en":"View Stock Transfers"}', 'inventory', 'View intra-warehouse stock transfers'),
  ('inventory.transfer.update', '{"ar":"تعديل التحويلات المخزنية","en":"Update Stock Transfers"}', 'inventory', 'Approve and dispatch stock transfers'),

  -- Ingest & Bulk Uploads
  ('ingest.session.view', '{"ar":"عرض جلسات الاستيراد","en":"View Ingest Sessions"}', 'ingest', 'View batch uploads and error logs'),
  ('ingest.session.update', '{"ar":"تنفيذ الاستيراد والرفع","en":"Execute Ingest"}', 'ingest', 'Upload catalogs and commit batch sessions'),

  -- HR & Jobs & Documents
  ('hr.job.view', '{"ar":"عرض الوظائف والطلبات","en":"View Jobs & Applications"}', 'hr', 'View job vacancies and submitted CVs'),
  ('hr.job.update', '{"ar":"تعديل الوظائف والطلبات","en":"Update Jobs & Applications"}', 'hr', 'Post jobs and update applicant review states'),
  ('hr.job.delete', '{"ar":"حذف الوظائف","en":"Delete Jobs"}', 'hr', 'Remove job vacancy posts'),
  ('hr.document.view', '{"ar":"عرض المستندات والتراخيص","en":"View Documents"}', 'hr', 'View uploaded legal documents and pharmacy licenses'),
  ('hr.document.update', '{"ar":"اعتماد المستندات والتراخيص","en":"Verify Documents"}', 'hr', 'Verify and approve uploaded legal documents'),

  -- Workflow & Issues
  ('workflow.issue.view', '{"ar":"عرض التذاكر والشكاوى","en":"View Issues"}', 'workflow', 'View support defect and quality tickets'),
  ('workflow.issue.update', '{"ar":"معالجة التذاكر والشكاوى","en":"Resolve Issues"}', 'workflow', 'Update status and resolve support tickets'),
  ('workflow.request.view', '{"ar":"عرض طلبات الخدمات","en":"View Service Requests"}', 'workflow', 'View institutional service requests'),
  ('workflow.request.update', '{"ar":"تعديل طلبات الخدمات","en":"Update Service Requests"}', 'workflow', 'Process institutional service requests'),

  -- Platform Admin & Settings & Logs
  ('platform.setting.view', '{"ar":"عرض إعدادات المنصة","en":"View System Settings"}', 'platform', 'View global configuration, features, and gateway settings'),
  ('platform.setting.update', '{"ar":"تعديل إعدادات المنصة","en":"Update System Settings"}', 'platform', 'Modify global configuration, features, and gateway settings'),
  ('platform.content.view', '{"ar":"عرض المحتوى والمقالات","en":"View Content"}', 'platform', 'View banners, blog articles, FAQs, and policies'),
  ('platform.content.update', '{"ar":"تعديل المحتوى والمقالات","en":"Update Content"}', 'platform', 'Create and edit banners, articles, FAQs, and policies'),
  ('platform.content.delete', '{"ar":"حذف المحتوى والمقالات","en":"Delete Content"}', 'platform', 'Delete banners, articles, FAQs, and policies'),
  ('platform.activity_log.view', '{"ar":"عرض سجل النشاطات","en":"View Activity Logs"}', 'platform', 'View and export administrative audit trail'),
  ('platform.activity_log.delete', '{"ar":"حذف سجل النشاطات","en":"Clear Activity Logs"}', 'platform', 'Purge administrative audit trail'),
  ('platform.error_log.view', '{"ar":"عرض سجل الأخطاء","en":"View Error Logs"}', 'platform', 'View and export system error logs'),
  ('platform.error_log.update', '{"ar":"معالجة سجل الأخطاء","en":"Resolve Error Logs"}', 'platform', 'Mark system errors as reviewed or fixed'),
  ('platform.error_log.delete', '{"ar":"حذف سجل الأخطاء","en":"Clear Error Logs"}', 'platform', 'Purge system error logs'),
  ('platform.developer.sql', '{"ar":"لوحة استعلامات SQL","en":"Developer SQL Console"}', 'platform', 'Execute read-only diagnostic SQL queries'),
  ('platform.trash.view', '{"ar":"عرض سلة المحذوفات","en":"View Trash"}', 'platform', 'View soft-deleted records'),
  ('platform.trash.update', '{"ar":"استعادة وسحق المحذوفات","en":"Restore/Purge Trash"}', 'platform', 'Restore or permanently delete soft-deleted records')
ON CONFLICT (key) DO UPDATE SET
  name = EXCLUDED.name,
  module = EXCLUDED.module,
  description = EXCLUDED.description;

-- Grant all granular permissions to super_admin and admin
INSERT INTO identity.role_permissions (role_key, permission_key)
SELECT r.key, p.key
FROM (SELECT unnest(ARRAY['admin', 'super_admin']) AS key) r
CROSS JOIN identity.permissions p
WHERE p.key NOT LIKE 'platform.developer.%'
ON CONFLICT (role_key, permission_key) DO NOTHING;

-- Grant developer role developer SQL access + admin read permissions
INSERT INTO identity.role_permissions (role_key, permission_key)
SELECT 'developer', p.key
FROM identity.permissions p
WHERE p.key LIKE 'platform.developer.%' OR p.key LIKE '%.view'
ON CONFLICT (role_key, permission_key) DO NOTHING;

-- Grant support role read-only + support issue resolution permissions
INSERT INTO identity.role_permissions (role_key, permission_key)
SELECT 'support', p.key
FROM identity.permissions p
WHERE p.key IN (
  'catalog.product.view', 'catalog.category.view', 'catalog.brand.view',
  'org.organization.view', 'org.branch.view', 'org.review.view',
  'identity.user.view',
  'commerce.order.view', 'commerce.quote.view',
  'billing.invoice.view', 'billing.payment.view',
  'promo.offer.view', 'promo.ad.view',
  'inventory.warehouse.view', 'inventory.stock.view',
  'hr.job.view', 'hr.document.view',
  'workflow.issue.view', 'workflow.issue.update', 'workflow.request.view',
  'platform.content.view', 'platform.activity_log.view', 'platform.error_log.view'
)
ON CONFLICT (role_key, permission_key) DO NOTHING;

COMMIT;
