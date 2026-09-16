-- 035_account_types
--
-- Registration previously ignored account type entirely: every signup became a
-- customer with no organisation, so every tenant-scoped screen rendered empty.
-- Phase 1 makes registration produce a user + organisation + owner membership
-- + main branch in one step. This migration supplies the role/permission
-- surface the three account types need, and the two registration-only columns
-- that carry pharmacy- and chain-specific data.
--
-- It does NOT alter identity.users.role (that stays the low-privilege platform
-- role 'customer'); capability comes from the organisation membership, whose
-- role_key is what the permission resolver reads.

BEGIN;

-- ---------------------------------------------------------------------------
-- Organisation roles (org_owner / org_manager / org_employee exist from 002)
-- ---------------------------------------------------------------------------
-- Align the names with the registration flow and add the two missing roles.
INSERT INTO identity.roles (key, name, scope, is_system, description) VALUES
    ('org_owner',     '{"ar":"مالك المؤسسة","en":"Organization Owner"}', 'organization', true, 'Whoever registered the organization'),
    ('org_manager',   '{"ar":"مدير","en":"Manager"}',                    'organization', true, 'Day-to-day management'),
    ('org_employee',  '{"ar":"موظف","en":"Employee"}',                   'organization', true, 'Limited operational access'),
    ('org_accountant', '{"ar":"محاسب","en":"Accountant"}',               'organization', true, 'Billing and invoices only'),
    ('org_warehouse',  '{"ar":"أمين مخزن","en":"Warehouse Keeper"}',     'organization', true, 'Inventory and transfers only')
ON CONFLICT (key) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

-- ---------------------------------------------------------------------------
-- Fine-grained permissions, keyed <module>.<action>
-- ---------------------------------------------------------------------------
INSERT INTO identity.permissions (key, name, module) VALUES
    ('catalog.product.view',   '{"ar":"عرض المنتجات","en":"View products"}',       'catalog'),
    ('catalog.product.create', '{"ar":"إضافة منتج","en":"Create product"}',         'catalog'),
    ('catalog.product.update', '{"ar":"تعديل منتج","en":"Update product"}',         'catalog'),
    ('catalog.product.delete', '{"ar":"حذف منتج","en":"Delete product"}',           'catalog'),
    ('catalog.category.manage','{"ar":"إدارة التصنيفات","en":"Manage categories"}', 'catalog'),
    ('catalog.brand.manage',   '{"ar":"إدارة العلامات","en":"Manage brands"}',      'catalog'),
    ('inventory.stock.view',   '{"ar":"عرض المخزون","en":"View stock"}',           'inventory'),
    ('inventory.stock.adjust', '{"ar":"تعديل المخزون","en":"Adjust stock"}',       'inventory'),
    ('inventory.warehouse.manage','{"ar":"إدارة المخازن","en":"Manage warehouses"}','inventory'),
    ('inventory.transfer.create','{"ar":"إنشاء تحويل","en":"Create transfer"}',    'inventory'),
    ('inventory.transfer.approve','{"ar":"اعتماد تحويل","en":"Approve transfer"}', 'inventory'),
    ('commerce.order.view',    '{"ar":"عرض الطلبات","en":"View orders"}',          'commerce'),
    ('commerce.order.fulfil',  '{"ar":"تنفيذ الطلبات","en":"Fulfil orders"}',      'commerce'),
    ('commerce.order.dispatch','{"ar":"شحن الطلبات","en":"Dispatch orders"}',      'commerce'),
    ('commerce.quote.manage',  '{"ar":"إدارة طلبات التسعير","en":"Manage quotes"}','commerce'),
    ('billing.invoice.read',   '{"ar":"عرض الفواتير","en":"View invoices"}',       'billing'),
    ('billing.invoice.manage', '{"ar":"إدارة الفواتير","en":"Manage invoices"}',   'billing'),
    ('billing.wallet.read',    '{"ar":"عرض المحفظة","en":"View wallet"}',          'billing'),
    ('billing.wallet.manage',  '{"ar":"إدارة المحفظة","en":"Manage wallet"}',      'billing'),
    ('billing.payment.manage', '{"ar":"إدارة المدفوعات","en":"Manage payments"}',  'billing'),
    ('org.member.manage',      '{"ar":"إدارة الأعضاء","en":"Manage members"}',     'org'),
    ('org.profile.manage',     '{"ar":"إدارة الملف","en":"Manage profile"}',       'org'),
    ('ingest.import.run',      '{"ar":"تشغيل الاستيراد","en":"Run imports"}',      'ingest'),
    ('promo.offer.manage',     '{"ar":"إدارة العروض","en":"Manage offers"}',       'promo'),
    ('hr.job.manage',          '{"ar":"إدارة الوظائف","en":"Manage jobs"}',        'hr')
ON CONFLICT (key) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Role → permission mapping
-- ---------------------------------------------------------------------------
-- org_owner: every organization-scoped permission.
INSERT INTO identity.role_permissions (role_key, permission_key)
SELECT 'org_owner', key FROM identity.permissions
WHERE module IN ('catalog','inventory','commerce','billing','org','ingest','promo','hr')
ON CONFLICT DO NOTHING;

-- org_manager: runs the business, no payment/wallet mutation and no deleting.
INSERT INTO identity.role_permissions (role_key, permission_key) VALUES
    ('org_manager','catalog.product.view'),
    ('org_manager','catalog.product.create'),
    ('org_manager','catalog.product.update'),
    ('org_manager','catalog.category.manage'),
    ('org_manager','catalog.brand.manage'),
    ('org_manager','inventory.stock.view'),
    ('org_manager','inventory.stock.adjust'),
    ('org_manager','inventory.warehouse.manage'),
    ('org_manager','inventory.transfer.create'),
    ('org_manager','inventory.transfer.approve'),
    ('org_manager','commerce.order.view'),
    ('org_manager','commerce.order.fulfil'),
    ('org_manager','commerce.order.dispatch'),
    ('org_manager','commerce.quote.manage'),
    ('org_manager','billing.invoice.read'),
    ('org_manager','billing.wallet.read'),
    ('org_manager','org.member.manage'),
    ('org_manager','org.profile.manage'),
    ('org_manager','ingest.import.run'),
    ('org_manager','promo.offer.manage')
ON CONFLICT DO NOTHING;

-- org_employee: limited operational access, no billing, no member management.
INSERT INTO identity.role_permissions (role_key, permission_key) VALUES
    ('org_employee','catalog.product.view'),
    ('org_employee','inventory.stock.view'),
    ('org_employee','commerce.order.view'),
    ('org_employee','commerce.order.dispatch')
ON CONFLICT DO NOTHING;

-- org_accountant: billing and invoices only.
INSERT INTO identity.role_permissions (role_key, permission_key) VALUES
    ('org_accountant','commerce.order.view'),
    ('org_accountant','billing.invoice.read'),
    ('org_accountant','billing.invoice.manage'),
    ('org_accountant','billing.wallet.read'),
    ('org_accountant','billing.wallet.manage'),
    ('org_accountant','billing.payment.manage')
ON CONFLICT DO NOTHING;

-- org_warehouse: inventory and transfers only.
INSERT INTO identity.role_permissions (role_key, permission_key) VALUES
    ('org_warehouse','catalog.product.view'),
    ('org_warehouse','inventory.stock.view'),
    ('org_warehouse','inventory.stock.adjust'),
    ('org_warehouse','inventory.warehouse.manage'),
    ('org_warehouse','inventory.transfer.create'),
    ('org_warehouse','inventory.transfer.approve'),
    ('org_warehouse','commerce.order.view'),
    ('org_warehouse','commerce.order.dispatch')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Registration-only columns carried on the organisation
-- ---------------------------------------------------------------------------
ALTER TABLE org.organizations
    ADD COLUMN IF NOT EXISTS pharmacist_license TEXT,
    ADD COLUMN IF NOT EXISTS branch_count     INT;

COMMENT ON COLUMN org.organizations.pharmacist_license IS 'ترخيص الصيدلي — pharmacist licence number, required for pharmacy/chain signups';
COMMENT ON COLUMN org.organizations.branch_count IS 'عدد الفروع — branch count, set for chain_pharmacy signups';

COMMIT;
