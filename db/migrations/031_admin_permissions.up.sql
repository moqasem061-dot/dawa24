BEGIN;

-- Admin permissions: one per module
INSERT INTO identity.permissions (key, name, module, description) VALUES
  ('identity.admin', '{"ar":"إدارة المستخدمين","en":"Identity Administration"}', 'identity', 'Manage users, sessions, MFA'),
  ('catalog.admin', '{"ar":"إدارة الكاتالوج","en":"Catalog Administration"}', 'catalog', 'Cross-tenant product management'),
  ('commerce.admin', '{"ar":"إدارة التجارة","en":"Commerce Administration"}', 'commerce', 'Cross-tenant order management'),
  ('billing.admin', '{"ar":"إدارة الفواتير","en":"Billing Administration"}', 'billing', 'Wallet adjustments, payments'),
  ('promo.admin', '{"ar":"إدارة العروض","en":"Promo Administration"}', 'promo', 'Approve ads and sponsorships'),
  ('ingest.admin', '{"ar":"إدارة الاستيراد","en":"Ingest Administration"}', 'ingest', 'Cross-tenant import sessions'),
  ('org.admin', '{"ar":"إدارة المؤسسات","en":"Org Administration"}', 'org', 'Organization management'),
  ('platform.admin', '{"ar":"إدارة المنصة","en":"Platform Administration"}', 'platform', 'System settings, translations')
ON CONFLICT (key) DO NOTHING;

-- Grant to admin and super_admin roles
INSERT INTO identity.role_permissions (role_key, permission_key)
SELECT r.key, p.key
FROM (SELECT unnest(ARRAY['admin', 'super_admin']) AS key) r
CROSS JOIN identity.permissions p
WHERE p.key LIKE '%.admin'
ON CONFLICT (role_key, permission_key) DO NOTHING;

COMMIT;
